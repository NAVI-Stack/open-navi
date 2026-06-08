package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/ceoai/navi/connectors"
	"github.com/ceoai/navi/internal/bus"
	"github.com/ceoai/navi/internal/config"
)

// CapabilityDescriptor describes a single connector capability such as
// "messaging.send" or "artifact.search". It mirrors the spec fields in
// docs/canonical/specs/connectors.md and is intentionally generic so that
// different drivers can expose their own capability sets.
type CapabilityDescriptor struct {
	Name               string          `json:"name"`
	InputSchema        json.RawMessage `json:"input_schema,omitempty"`
	OutputSchema       json.RawMessage `json:"output_schema,omitempty"`
	SideEffectClass    string          `json:"side_effect_class,omitempty"`
	IdempotencyClass   string          `json:"idempotency_class,omitempty"`
	Reversibility      string          `json:"reversibility,omitempty"`
	RiskHint           string          `json:"risk_hint,omitempty"`
	RequiredPermissions []string       `json:"required_permissions,omitempty"`
	SupportsDryRun     bool            `json:"supports_dry_run,omitempty"`
	SupportsRetry      bool            `json:"supports_retry,omitempty"`
}

// ResultEnvelope is the normalized result for a connector action execution.
// It is intentionally parallel to the skill execution envelope and the
// canonical connector result envelope in the v2 spec.
type ResultEnvelope struct {
	Status             string          `json:"status"` // "ok", "error"
	Output             json.RawMessage `json:"output,omitempty"`
	Error              string          `json:"error,omitempty"`
	Retryable          bool            `json:"retryable"`
	ConnectorInstanceID string         `json:"connector_instance_id"`
	Capability         string          `json:"capability"`
	CorrelationID      string          `json:"correlation_id,omitempty"`
	DurationMS         int64           `json:"duration_ms"`
	ProviderMetadata   json.RawMessage `json:"provider_metadata,omitempty"`
}

// EventEnvelope is the normalized shape for inbound connector events.
type EventEnvelope struct {
	ConnectorInstanceID string          `json:"connector_instance_id"`
	EventType           string          `json:"event_type"`
	Payload             json.RawMessage `json:"payload"`
	ProviderMetadata    json.RawMessage `json:"provider_metadata,omitempty"`
	ReceivedAt          time.Time       `json:"received_at"`
	CorrelationID       string          `json:"correlation_id,omitempty"`
}

// ConnectorDriver is a reusable driver for a provider or environment type
// (e.g. builtin.telegram, builtin.slack). It is not tied to a specific
// account or credential set; those are modeled as instances.
type ConnectorDriver interface {
	DriverID() string
	Kind() string
	DisplayName() string
	Capabilities() []CapabilityDescriptor

	// SetupDescriptor returns the parameters required to instantiate a connector of this type.
	SetupDescriptor() SetupDescriptor

	// CreateInstance instantiates a connector using provided parameters. Driver-supplied
	// factories or bridge logic are triggered here.
	CreateInstance(cfg *config.Config, b bus.Bus, params map[string]string) (connectors.Connector, error)
}

// ConnectorInstance represents a configured runtime deployment of a driver.
// For now, it is a thin wrapper around the existing connectors.Connector
// interface so that the v1 implementation can incrementally migrate toward
// the v2 model without breaking callers.
type ConnectorInstance interface {
	InstanceID() string
	DriverID() string
	Connector() connectors.Connector
}

// ConnectorActionHandler executes capability-based actions against a specific
// connector instance and returns normalized result envelopes.
type ConnectorActionHandler interface {
	InvokeAction(ctx context.Context, instanceID string, capability string, input json.RawMessage) (ResultEnvelope, error)
}

// basicDriverAdapter is a minimal ConnectorDriver implementation that reuses
// the existing connector registry factories. It exposes a small,
// messaging-oriented capability surface that can be expanded over time.
type basicDriverAdapter struct {
	id          string
	kind        string
	displayName string
	factory     ConnectorFactory
	setup       SetupDescriptor
	caps        []CapabilityDescriptor
}

func (d *basicDriverAdapter) DriverID() string   { return d.id }
func (d *basicDriverAdapter) Kind() string       { return d.kind }
func (d *basicDriverAdapter) DisplayName() string { return d.displayName }

func (d *basicDriverAdapter) SetupDescriptor() SetupDescriptor {
	return d.setup
}

func (d *basicDriverAdapter) CreateInstance(cfg *config.Config, b bus.Bus, params map[string]string) (connectors.Connector, error) {
	if d.factory == nil {
		return nil, fmt.Errorf("builtin driver %s has no factory", d.id)
	}
	// For legacy builtin drivers, we assume they read from global cfg or are
	// configured via side effects in main.go for now.
	return d.factory(cfg, b)
}

func (d *basicDriverAdapter) Capabilities() []CapabilityDescriptor {
	if len(d.caps) > 0 {
		return d.caps
	}
	// Fallback for un-extended drivers.
	return []CapabilityDescriptor{
		{
			Name: "messaging.send",
		},
	}
}

// BaseConnector provides a common foundation for many connector types.
type BaseConnector struct {
	name      string
	allowList []string
	running   atomic.Bool
}

// NewBaseConnector creates a new BaseConnector.
func NewBaseConnector(name string, allowList []string) BaseConnector {
	return BaseConnector{
		name:      name,
		allowList: allowList,
	}
}

func (c *BaseConnector) Name() string           { return c.name }
func (c *BaseConnector) IsRunning() bool        { return c.running.Load() }
func (c *BaseConnector) SetRunning(running bool) { c.running.Store(running) }

// basicInstanceAdapter is a ConnectorInstance that wraps a v1 connectors.Connector
// and treats its name as both driver_id and instance_id. This keeps existing
// behavior intact while allowing the registry and manager to reason in terms
// of instances.
type basicInstanceAdapter struct {
	instanceID string
	driverID   string
	conn       connectors.Connector
}

func (i *basicInstanceAdapter) InstanceID() string          { return i.instanceID }
func (i *basicInstanceAdapter) DriverID() string            { return i.driverID }
func (i *basicInstanceAdapter) Connector() connectors.Connector { return i.conn }

// SetupParam describes a single connector setup parameter (required or optional).
// Used by GET /api/connectors/setup-schema for metadata-driven CLI prompts.
type SetupParam struct {
	Key            string `json:"key"`
	Label          string `json:"label"`
	Description    string `json:"description,omitempty"`
	Secret         bool   `json:"secret"`
	Placeholder    string `json:"placeholder,omitempty"`
	ValidationHint string `json:"validation_hint,omitempty"`
}

// SetupDescriptor is the authoritative schema for configuring one connector type.
// Server returns these from GET /api/connectors/setup-schema; CLI uses them for prompts and param collection.
type SetupDescriptor struct {
	Type          string      `json:"type"`
	DisplayName   string      `json:"display_name"`
	RequiredParams []SetupParam `json:"required_params"`
	OptionalParams []SetupParam `json:"optional_params"`
	SetupHint     string      `json:"setup_hint,omitempty"`
}

