package connectors

import (
	"sync"
	"time"

	"github.com/ceoai/navi/connectors"
	"github.com/ceoai/navi/internal/bus"
	"github.com/ceoai/navi/internal/config"
)

// ConnectorFactory creates a connector from runtime config and bus.
type ConnectorFactory func(cfg *config.Config, b bus.Bus) (connectors.Connector, error)

// ConnectorInfo describes a registered connector instance.
type ConnectorInfo struct {
	Name        string    `json:"name"`
	Category    string    `json:"category,omitempty"` // taxonomy category when connector implements Categorizable
	Status      string    `json:"status"`
	ConnectedAt time.Time `json:"connected_at"`
}

// Registry manages connector factories and live instances.
type Registry struct {
	mu             sync.RWMutex
	factories      map[string]ConnectorFactory
	instances      map[string]connectors.Connector
	startedAt      map[string]time.Time
	httpRegistered map[string]time.Time // names that called Register() via HTTP (no instance)

	// v2 model: drivers and instances as explicit first-class concepts. For the
	// current built-in connectors, the driver_id and instance_id are both the
	// connector name; this keeps behavior stable while exposing the richer
	// model to callers that need it.
	drivers      map[string]ConnectorDriver
	instancesV2  map[string]ConnectorInstance
	instanceMeta map[string]InstanceMetadata
}

// NewRegistry creates a new connector registry.
func NewRegistry() *Registry {
	return &Registry{
		factories:      make(map[string]ConnectorFactory),
		instances:      make(map[string]connectors.Connector),
		startedAt:      make(map[string]time.Time),
		httpRegistered: make(map[string]time.Time),
		drivers:        make(map[string]ConnectorDriver),
		instancesV2:    make(map[string]ConnectorInstance),
		instanceMeta:   make(map[string]InstanceMetadata),
	}
}

// RegisterFactory registers a connector factory by name. Called at init or startup.
func (r *Registry) RegisterFactory(name string, f ConnectorFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[name] = f

	// Also register a basic driver adapter so the v2 runtime model can reason
	// about drivers even for the existing built-in connectors.
	if _, exists := r.drivers[name]; !exists {
		r.drivers[name] = &basicDriverAdapter{
			id:          name,
			kind:        "builtin",
			displayName: name,
			factory:     f,
		}
	}
}

// RegisterDriver registers a connector driver without requiring a built-in
// factory. This is used by runtime plugin manifests that contribute metadata
// and capability surfaces before an instance is configured.
func (r *Registry) RegisterDriver(driver ConnectorDriver) {
	if driver == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.drivers[driver.DriverID()] = driver
}

// ExtendDriver enhances an existing driver with additional metadata (setup
// schema and capabilities) from a plugin manifest. This is used for built-in
// drivers that are registered via RegisterFactory but need the rich metadata
// from the plugin system.
func (r *Registry) ExtendDriver(id string, setup SetupDescriptor, caps []CapabilityDescriptor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.drivers[id]
	if !ok {
		return
	}
	if ad, ok := d.(*basicDriverAdapter); ok {
		ad.setup = setup
		if len(caps) > 0 {
			ad.caps = caps
		}
	}
}

// GetDriver returns the registered driver for the given ID.
func (r *Registry) GetDriver(id string) (ConnectorDriver, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.drivers[id]
	return d, ok
}

// Create instantiates a connector using the driver's CreateInstance method and stores it.
func (r *Registry) Create(name string, cfg *config.Config, b bus.Bus, params map[string]string) error {
	r.mu.RLock()
	driver, ok := r.drivers[name]
	r.mu.RUnlock()
	if !ok {
		return ErrUnknownConnector
	}

	conn, err := driver.CreateInstance(cfg, b, params)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.instances[name] = conn

	// Create a v2 instance wrapper keyed by the same name for now.
	inst := &basicInstanceAdapter{
		instanceID: name,
		driverID:   driver.DriverID(),
		conn:       conn,
	}
	r.instancesV2[name] = inst
	r.instanceMeta[name] = InstanceMetadata{
		InstanceID:    inst.InstanceID(),
		DriverID:      inst.DriverID(),
		Status:        "configured",
		HealthState:   "unknown",
		AuthState:     "unconfigured",
		GrantedScopes: nil,
		Labels:        map[string]string{},
		PolicyRefs:    nil,
	}
	return nil
}

// Get returns the connector instance for the given name, or nil.
func (r *Registry) Get(name string) connectors.Connector {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.instances[name]
}

// GetInfo returns ConnectorInfo for the given name if present (instance or httpRegistered).
func (r *Registry) GetInfo(name string) (ConnectorInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if conn, ok := r.instances[name]; ok {
		status := "disconnected"
		if conn.IsRunning() {
			status = "connected"
		}
		info := ConnectorInfo{Name: name, Status: status, ConnectedAt: r.startedAt[name]}
		if cat, ok := conn.(connectors.Categorizable); ok {
			info.Category = cat.Category()
		}
		return info, true
	}
	if at, ok := r.httpRegistered[name]; ok {
		return ConnectorInfo{
			Name:        name,
			Status:      "connected",
			ConnectedAt: at,
		}, true
	}
	return ConnectorInfo{}, false
}

// List returns info for all live connector instances plus any HTTP-registered names.
func (r *Registry) List() []ConnectorInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var list []ConnectorInfo
	seen := make(map[string]bool)
	for name, conn := range r.instances {
		seen[name] = true
		status := "disconnected"
		if conn.IsRunning() {
			status = "connected"
		}
		at := r.startedAt[name]
		info := ConnectorInfo{Name: name, Status: status, ConnectedAt: at}
		if cat, ok := conn.(connectors.Categorizable); ok {
			info.Category = cat.Category()
		}
		list = append(list, info)
	}
	for name, at := range r.httpRegistered {
		if seen[name] {
			continue
		}
		list = append(list, ConnectorInfo{
			Name:        name,
			Status:      "connected",
			ConnectedAt: at,
		})
	}
	return list
}

// Register marks a connector as connected (e.g. after self-registration via HTTP).
func (r *Registry) Register(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.httpRegistered[name] = time.Now().UTC()
}

// Deregister removes a connector from HTTP-registered set (e.g. on HTTP deregister).
func (r *Registry) Deregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.httpRegistered, name)
	delete(r.startedAt, name)
}

// Remove deletes a connector instance and any associated runtime metadata,
// while leaving the factory registered so the connector can be created again.
func (r *Registry) Remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.httpRegistered, name)
	delete(r.startedAt, name)
	delete(r.instances, name)
	delete(r.instancesV2, name)
	delete(r.instanceMeta, name)
}

// RegisterInstance stores a connector instance directly. Used by the gateway
// to register bridge connectors created from HTTP registration payloads.
func (r *Registry) RegisterInstance(name string, conn connectors.Connector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.instances[name] = conn

	// Also surface as a v2 instance. When called for bridge connectors we
	// still treat the name as both driver_id and instance_id.
	if _, ok := r.drivers[name]; !ok {
		r.drivers[name] = &basicDriverAdapter{
			id:          name,
			kind:        "remote",
			displayName: name,
		}
	}
	inst := &basicInstanceAdapter{
		instanceID: name,
		driverID:   name,
		conn:       conn,
	}
	r.instancesV2[name] = inst
	meta := r.instanceMeta[name]
	meta.InstanceID = inst.InstanceID()
	meta.DriverID = inst.DriverID()
	if meta.Status == "" {
		meta.Status = "configured"
	}
	if meta.HealthState == "" {
		meta.HealthState = "unknown"
	}
	if meta.AuthState == "" {
		meta.AuthState = "unconfigured"
	}
	r.instanceMeta[name] = meta
}

// StartedAt records the time a connector was started. Call from StartAll for each started connector.
func (r *Registry) setStartedAt(name string, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.startedAt[name] = at

	// Best-effort update of instance metadata.
	if meta, ok := r.instanceMeta[name]; ok {
		meta.Status = "connected"
		meta.HealthState = "healthy"
		meta.LastHealthyAt = at
		r.instanceMeta[name] = meta
	}
}

// FactoryNames returns the list of registered factory names.
func (r *Registry) FactoryNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.factories))
	for n := range r.factories {
		names = append(names, n)
	}
	return names
}

// Drivers returns all known connector drivers in the registry.
func (r *Registry) Drivers() []ConnectorDriver {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ConnectorDriver, 0, len(r.drivers))
	for _, d := range r.drivers {
		out = append(out, d)
	}
	return out
}

// SetupDescriptors returns setup metadata for all drivers that support it.
func (r *Registry) SetupDescriptors() []SetupDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]SetupDescriptor, 0, len(r.drivers))
	for _, d := range r.drivers {
		desc := d.SetupDescriptor()
		if desc.Type != "" {
			out = append(out, desc)
		}
	}
	return out
}

// InstancesV2 returns all known connector instances using the v2 model.
func (r *Registry) InstancesV2() []ConnectorInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ConnectorInstance, 0, len(r.instancesV2))
	for _, inst := range r.instancesV2 {
		out = append(out, inst)
	}
	return out
}

// InstanceMetadata returns v2-style metadata for a specific connector instance.
func (r *Registry) InstanceMetadata(name string) (InstanceMetadata, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	meta, ok := r.instanceMeta[name]
	return meta, ok
}

// UpdateHealth updates the health-related fields for a connector instance's
// metadata. Callers should pass a best-effort health_state label consistent
// with the v2 spec (e.g. "healthy", "degraded", "down").
func (r *Registry) UpdateHealth(name, healthState string, lastErrorAt time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	meta, ok := r.instanceMeta[name]
	if !ok {
		return
	}
	if healthState != "" {
		meta.HealthState = healthState
	}
	if !lastErrorAt.IsZero() {
		meta.LastErrorAt = lastErrorAt
	}
	r.instanceMeta[name] = meta
}

// InstanceMetadata holds the runtime metadata described in the v2 connector
// spec (status, health, auth state, scopes, and policy attachment points).
type InstanceMetadata struct {
	InstanceID    string            `json:"instance_id"`
	DriverID      string            `json:"driver_id"`
	Status        string            `json:"status"`
	HealthState   string            `json:"health_state"`
	AuthState     string            `json:"auth_state"`
	GrantedScopes []string          `json:"granted_scopes,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	PolicyRefs    []string          `json:"policy_refs,omitempty"`
	LastHealthyAt time.Time         `json:"last_healthy_at,omitempty"`
	LastErrorAt   time.Time         `json:"last_error_at,omitempty"`
}
