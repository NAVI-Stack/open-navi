package plugin

import (
	"net/http"
	"path/filepath"
	"sync"

	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/navi/hooks"
)

// Service is a lifecycle service that can be started and stopped.
type Service interface {
	Start() error
	Stop() error
}

// HTTPRouteRegistration is a path and handler for the gateway.
type HTTPRouteRegistration struct {
	Path    string
	Handler http.Handler
}

type toolEntry struct {
	pluginID string
	tool     llm.ToolDefinition
}

// ToolRegistration is the canonical plugin registry projection used when
// higher-level registries need both the tool definition and its source plugin.
type ToolRegistration struct {
	PluginID   string
	Definition llm.ToolDefinition
}

// CatalogEntry is a plugin manifest plus its tools, used by Install and Update.
type CatalogEntry struct {
	Manifest Manifest
	Tools    []llm.ToolDefinition
}

// Registry holds plugin-registered tools, hooks, services, and HTTP routes.
type Registry struct {
	mu        sync.RWMutex
	tools     []toolEntry
	routes    []HTTPRouteRegistration
	services  []Service
	manifests []Manifest
	disabled  map[string]bool         // plugin IDs that are deactivated
	catalog   map[string]CatalogEntry // id -> entry for Install/Update
	runner    *hooks.Runner
}

// NewRegistry creates a new plugin registry. Pass a hooks.Runner to share with the agent.
func NewRegistry(runner *hooks.Runner) *Registry {
	if runner == nil {
		runner = hooks.NewRunner()
	}
	return &Registry{
		tools:     nil,
		routes:    nil,
		services:  nil,
		manifests: nil,
		disabled:  nil,
		catalog:   nil,
		runner:    runner,
	}
}

// AddTool appends a tool definition (no plugin ID; always active).
func (r *Registry) AddTool(tool llm.ToolDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools = append(r.tools, toolEntry{tool: tool})
}

// AddToolForPlugin appends a tool for the given plugin ID. Activate/Deactivate applies to pluginID.
func (r *Registry) AddToolForPlugin(pluginID string, tool llm.ToolDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools = append(r.tools, toolEntry{pluginID: pluginID, tool: tool})
}

// SetPluginEnabled enables or disables a plugin by ID. Disabled plugins' tools are excluded from Tools().
func (r *Registry) SetPluginEnabled(pluginID string, enabled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.disabled == nil {
		r.disabled = make(map[string]bool)
	}
	if enabled {
		delete(r.disabled, pluginID)
	} else {
		r.disabled[pluginID] = true
	}
}

// IsPluginEnabled returns whether the plugin is enabled (default true if never set).
func (r *Registry) IsPluginEnabled(pluginID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return !r.disabled[pluginID]
}

// Tools returns a copy of registered tools, excluding those from disabled plugins.
func (r *Registry) Tools() []llm.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []llm.ToolDefinition
	for _, e := range r.tools {
		if e.pluginID != "" && r.disabled[e.pluginID] {
			continue
		}
		out = append(out, e.tool)
	}
	return out
}

// ToolEntries returns the active tool definitions together with their source plugin IDs.
func (r *Registry) ToolEntries() []ToolRegistration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ToolRegistration, 0, len(r.tools))
	for _, e := range r.tools {
		if e.pluginID != "" && r.disabled[e.pluginID] {
			continue
		}
		out = append(out, ToolRegistration{
			PluginID:   e.pluginID,
			Definition: e.tool,
		})
	}
	return out
}

// RegisterHook adds a hook handler to the shared runner.
func (r *Registry) RegisterHook(name hooks.HookName, priority int, handler hooks.HookHandler) {
	r.runner.Register(name, priority, handler)
}

// Runner returns the hook runner for direct Run calls.
func (r *Registry) Runner() *hooks.Runner {
	return r.runner
}

// AddHTTPRoute appends an HTTP route.
func (r *Registry) AddHTTPRoute(path string, handler http.Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes = append(r.routes, HTTPRouteRegistration{Path: path, Handler: handler})
}

// HTTPRoutes returns a copy of registered routes.
func (r *Registry) HTTPRoutes() []HTTPRouteRegistration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]HTTPRouteRegistration, len(r.routes))
	copy(out, r.routes)
	return out
}

// AddService appends a lifecycle service.
func (r *Registry) AddService(svc Service) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.services = append(r.services, svc)
}

// Services returns a copy of registered services.
func (r *Registry) Services() []Service {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Service, len(r.services))
	copy(out, r.services)
	return out
}

// AddManifest registers a plugin manifest with the registry. Callers are
// responsible for ensuring IDs are stable; the registry does not enforce
// uniqueness beyond append semantics.
// AddManifest registers a manifest. It is idempotent by plugin ID: if a
// manifest with the same ID is already registered, it is replaced in place
// (preserving order) rather than duplicated. This keeps reloads safe — re-running
// the loader refreshes metadata instead of accumulating duplicate entries.
func (r *Registry) AddManifest(m Manifest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m.Normalize(m.ID, m.RootDir)
	for i := range r.manifests {
		if r.manifests[i].ID == m.ID {
			r.manifests[i] = m
			return
		}
	}
	r.manifests = append(r.manifests, m)
}

// RegisterPlugin adds a manifest and its tools under the same plugin ID (manifest.ID).
// Tools can be toggled with SetPluginEnabled(manifest.ID, enabled).
func (r *Registry) RegisterPlugin(m Manifest, tools []llm.ToolDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m.Normalize(m.ID, m.RootDir)
	r.manifests = append(r.manifests, m)
	for _, t := range tools {
		r.tools = append(r.tools, toolEntry{pluginID: m.ID, tool: t})
	}
}

// Manifests returns a copy of all registered manifests, preserving insertion
// order. This provides a single inventory for discovery surfaces (CLI, docs,
// UI) to reason about plugin capabilities, triggers, and trust tiers.
func (r *Registry) Manifests() []Manifest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Manifest, len(r.manifests))
	for i, m := range r.manifests {
		m.Active = m.IsActive() && (r.disabled == nil || !r.disabled[m.ID])
		m.Enabled = r.disabled == nil || !r.disabled[m.ID]
		out[i] = m
	}
	return out
}

// ActivePluginSkillPaths returns manifest-declared skill component paths for
// valid active plugins. This is the only repo-plugin skill source consumed by
// the skill registry; it intentionally avoids raw plugins/* folder scanning.
//
// Current gating: (1) ID non-empty, (2) not registry-disabled, (3) manifest
// status active, (4) manifest passes schema validation.
// Permission-based gating (Manifest.Permissions) is planned but not yet enforced.
func (r *Registry) ActivePluginSkillPaths() map[string][]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string][]string)
	for _, m := range r.manifests {
		m.Normalize(m.ID, m.RootDir)
		if m.ID == "" || r.disabled[m.ID] || !m.IsActive() || m.Validate() != nil {
			continue
		}
		for _, ref := range m.Components.Skills {
			if ref.Path == "" || m.RootDir == "" {
				continue
			}
			path := filepath.Join(m.RootDir, filepath.FromSlash(ref.Path))
			out[m.ID] = append(out[m.ID], path)
		}
	}
	return out
}

// ActivePluginConnectorIDs returns connector IDs declared by valid active
// plugin manifests. Built-in bootstrap code uses this to avoid exposing
// concrete connector factories for disabled or invalid plugins.
//
// Current gating: (1) ID non-empty, (2) not registry-disabled, (3) manifest
// status active, (4) manifest passes schema validation.
// Permission-based gating (Manifest.Permissions) is planned but not yet enforced.
func (r *Registry) ActivePluginConnectorIDs() map[string]bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]bool)
	for _, m := range r.manifests {
		m.Normalize(m.ID, m.RootDir)
		if m.ID == "" || r.disabled[m.ID] || !m.IsActive() || m.Validate() != nil {
			continue
		}
		if m.Connector != nil && m.Connector.DriverID != "" {
			out[m.Connector.DriverID] = true
		}
		for _, ref := range m.Components.Connectors {
			if ref.ConnectorID != "" {
				out[ref.ConnectorID] = true
			}
		}
	}
	return out
}

// ActivePluginProviderIDs returns provider IDs declared by valid active
// plugin manifests. The LLM registry uses this as an allowlist so bootstrap
// imports alone are not enough to construct a provider.
//
// Current gating: (1) ID non-empty, (2) not registry-disabled, (3) manifest
// status active, (4) manifest passes schema validation.
// Permission-based gating (Manifest.Permissions) is planned but not yet enforced.
func (r *Registry) ActivePluginProviderIDs() map[string]bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]bool)
	for _, m := range r.manifests {
		m.Normalize(m.ID, m.RootDir)
		if m.ID == "" || r.disabled[m.ID] || !m.IsActive() || m.Validate() != nil {
			continue
		}
		for _, ref := range m.Components.Providers {
			if ref.ProviderID != "" {
				out[ref.ProviderID] = true
			}
		}
	}
	return out
}

// SetCatalogEntry adds or updates a catalog entry for Install/Update. The host can
// register installable plugins (e.g. from a bundle or config); Install(manifestID) and
// Update(manifestID) then use this catalog.
func (r *Registry) SetCatalogEntry(id string, m Manifest, tools []llm.ToolDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.catalog == nil {
		r.catalog = make(map[string]CatalogEntry)
	}
	r.catalog[id] = CatalogEntry{Manifest: m, Tools: tools}
}

// GetCatalogEntry returns the catalog entry for id, if any. Used by Install and Update.
func (r *Registry) GetCatalogEntry(id string) (Manifest, []llm.ToolDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.catalog == nil {
		return Manifest{}, nil, false
	}
	entry, ok := r.catalog[id]
	if !ok {
		return Manifest{}, nil, false
	}
	return entry.Manifest, entry.Tools, true
}

// HasManifestID returns true if a manifest with the given ID is already registered.
func (r *Registry) HasManifestID(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, m := range r.manifests {
		if m.ID == id {
			return true
		}
	}
	return false
}

// RemovePlugin unregisters a plugin by ID: removes its manifest and all tools
// with that pluginID. Used by Retire(..., RetireModeRemove).
func (r *Registry) RemovePlugin(pluginID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var newManifests []Manifest
	for _, m := range r.manifests {
		if m.ID != pluginID {
			newManifests = append(newManifests, m)
		}
	}
	r.manifests = newManifests
	var newTools []toolEntry
	for _, e := range r.tools {
		if e.pluginID != pluginID {
			newTools = append(newTools, e)
		}
	}
	r.tools = newTools
	if r.disabled != nil {
		delete(r.disabled, pluginID)
	}
}
