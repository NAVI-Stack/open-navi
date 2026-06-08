package llm

import (
	"context"
	"fmt"
	"sync"
)

// DynamicProvider wraps a Provider and allows hot-swapping the underlying
// implementation without restarting the server. Safe for concurrent use.
type DynamicProvider struct {
	mu          sync.RWMutex
	current     Provider
	preferModel string // if set, overrides the model passed by callers
}

// NewDynamicProvider creates a DynamicProvider wrapping the given initial provider.
// initial may be nil, in which case Chat returns an error until Swap is called.
func NewDynamicProvider(initial Provider) *DynamicProvider {
	return &DynamicProvider{current: initial}
}

// Swap replaces the underlying provider. Future Chat calls use the new provider.
func (d *DynamicProvider) Swap(p Provider) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.current = p
}

// SwapWithModel replaces the provider and sets a preferred model.
// All subsequent Chat calls will use preferModel instead of the caller-supplied model.
// Pass an empty preferModel to stop overriding.
func (d *DynamicProvider) SwapWithModel(p Provider, preferModel string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.current = p
	d.preferModel = preferModel
}

// Chat delegates to the current provider.
func (d *DynamicProvider) Chat(ctx context.Context, model string, messages []Message, tools []ToolDefinition, opts Options) (*Response, error) {
	d.mu.RLock()
	p := d.current
	m := d.preferModel
	d.mu.RUnlock()
	if p == nil {
		return nil, fmt.Errorf("llm: no provider configured — run navi init or open /onboarding to configure an LLM provider")
	}
	if m != "" {
		model = m
	}
	return p.Chat(ctx, model, messages, tools, opts)
}

// ChatStream delegates to the current provider when it supports streaming.
// If the underlying provider does not implement StreamingProvider, it falls
// back to Chat and returns the full response without invoking onChunk.
func (d *DynamicProvider) ChatStream(ctx context.Context, model string, messages []Message, tools []ToolDefinition, opts Options, onChunk func(delta string)) (*Response, error) {
	d.mu.RLock()
	p := d.current
	m := d.preferModel
	d.mu.RUnlock()
	if p == nil {
		return nil, fmt.Errorf("llm: no provider configured — run navi init or open /onboarding to configure an LLM provider")
	}
	if m != "" {
		model = m
	}
	if sp, ok := p.(StreamingProvider); ok {
		return sp.ChatStream(ctx, model, messages, tools, opts, onChunk)
	}
	// Non-streaming provider: just return a normal Chat response.
	return p.Chat(ctx, model, messages, tools, opts)
}

// Name returns the name of the current underlying provider.
func (d *DynamicProvider) Name() string {
	d.mu.RLock()
	p := d.current
	d.mu.RUnlock()
	if p == nil {
		return "none"
	}
	return p.Name()
}

// NameWithModel returns a human-readable "provider/model" string, e.g. "ollama/llama3:latest".
// If no model override is set it returns just the provider name.
func (d *DynamicProvider) NameWithModel() string {
	d.mu.RLock()
	p := d.current
	m := d.preferModel
	d.mu.RUnlock()
	if p == nil {
		return "none"
	}
	name := p.Name()
	if m != "" {
		return name + "/" + m
	}
	return name
}
