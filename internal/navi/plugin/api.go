package plugin

import (
	"log/slog"
	"net/http"

	"github.com/open-navi/navi/internal/bus"
	connreg "github.com/open-navi/navi/internal/connectors"
	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/navi/hooks"
)

// API is the unified registration surface for code-based extensions.
type API struct {
	ID                string
	Registry          *Registry
	Bus               bus.Bus
	Logger            *slog.Logger
	ConnectorRegistry  *connreg.Registry
}

// NewAPI creates a plugin API that writes to the given registry.
// ConnectorRegistry is optional; if set, RegisterConnector will register factories there.
func NewAPI(id string, registry *Registry, b bus.Bus, logger *slog.Logger) *API {
	if logger == nil {
		logger = slog.Default()
	}
	return &API{
		ID:       id,
		Registry: registry,
		Bus:      b,
		Logger:   logger,
	}
}

// RegisterTool registers a tool definition for the LLM.
func (a *API) RegisterTool(tool llm.ToolDefinition) {
	a.Registry.AddTool(tool)
}

// RegisterHook registers a hook handler with the given priority (lower = runs first).
func (a *API) RegisterHook(name hooks.HookName, priority int, handler hooks.HookHandler) {
	a.Registry.RegisterHook(name, priority, handler)
}

// RegisterHTTPRoute registers an HTTP path and handler with the gateway.
func (a *API) RegisterHTTPRoute(path string, handler http.Handler) {
	a.Registry.AddHTTPRoute(path, handler)
}

// RegisterService registers a lifecycle service (Start/Stop).
func (a *API) RegisterService(svc Service) {
	a.Registry.AddService(svc)
}

// RegisterManifest records a plugin manifest and its capability metadata in
// the shared registry so discovery and governance layers can reason about
// plugin categories, triggers, and trust tiers.
func (a *API) RegisterManifest(m Manifest) {
	a.Registry.AddManifest(m)
}

// RegisterConnector registers a connector factory with the connector registry (if set).
func (a *API) RegisterConnector(name string, factory connreg.ConnectorFactory) {
	if a.ConnectorRegistry != nil {
		a.ConnectorRegistry.RegisterFactory(name, factory)
	}
}
