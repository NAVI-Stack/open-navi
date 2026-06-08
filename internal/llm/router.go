package llm

import (
	"context"
	"fmt"
	"sync"
)

// modelRouteBinding binds a logical model id to a concrete provider and model string.
// If model is empty, the provider's own defaults (or fallback chain) are used.
type modelRouteBinding struct {
	provider Provider
	model    string
}

// Router multiplexes logical model ids (aliases) onto underlying providers.
// If the model argument to Chat matches a configured alias, Router invokes the
// associated provider with its configured model string. Otherwise it delegates
// to the default provider, passing the model through unchanged.
type Router struct {
	mu          sync.RWMutex
	defaultProv Provider
	routes      map[string]modelRouteBinding
}

// NewRouter creates a Router with the given default provider.
func NewRouter(defaultProv Provider) *Router {
	return &Router{
		defaultProv: defaultProv,
		routes:      make(map[string]modelRouteBinding),
	}
}

// AddRoute registers or replaces a logical model route.
func (r *Router) AddRoute(name string, prov Provider, model string) {
	if name == "" || prov == nil {
		return
	}
	r.mu.Lock()
	r.routes[name] = modelRouteBinding{
		provider: prov,
		model:    model,
	}
	r.mu.Unlock()
}

// Chat dispatches calls either to a named route or to the default provider.
func (r *Router) Chat(ctx context.Context, model string, messages []Message, tools []ToolDefinition, opts Options) (*Response, error) {
	r.mu.RLock()
	route, ok := r.routes[model]
	defProv := r.defaultProv
	r.mu.RUnlock()

	if ok {
		m := route.model
		return route.provider.Chat(ctx, m, messages, tools, opts)
	}

	if defProv == nil {
		return nil, fmt.Errorf("llm: no provider configured")
	}
	return defProv.Chat(ctx, model, messages, tools, opts)
}

// ChatStream preserves streaming behavior across logical model routes.
// Without this, callers that use aliases like "chat" silently fall back to
// non-streaming Chat even when the routed provider supports token streaming.
func (r *Router) ChatStream(ctx context.Context, model string, messages []Message, tools []ToolDefinition, opts Options, onChunk func(delta string)) (*Response, error) {
	r.mu.RLock()
	route, ok := r.routes[model]
	defProv := r.defaultProv
	r.mu.RUnlock()

	if ok {
		m := route.model
		if sp, ok := route.provider.(StreamingProvider); ok {
			return sp.ChatStream(ctx, m, messages, tools, opts, onChunk)
		}
		return route.provider.Chat(ctx, m, messages, tools, opts)
	}

	if defProv == nil {
		return nil, fmt.Errorf("llm: no provider configured")
	}
	if sp, ok := defProv.(StreamingProvider); ok {
		return sp.ChatStream(ctx, model, messages, tools, opts, onChunk)
	}
	return defProv.Chat(ctx, model, messages, tools, opts)
}

// Name reports the name of the default provider when available.
func (r *Router) Name() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.defaultProv != nil {
		return r.defaultProv.Name()
	}
	return "router"
}
