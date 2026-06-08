package anthropic

import (
	"context"
	"fmt"
	"strings"

	"github.com/ceoai/navi/internal/llm"
)

// knownModels is the static catalog of Anthropic Claude models.
// Updated periodically; not exhaustive but covers the main production models.
var knownModels = []llm.ModelDescriptor{
	{Name: "claude-opus-4-20250514", Family: "claude"},
	{Name: "claude-sonnet-4-20250514", Family: "claude"},
	{Name: "claude-haiku-4-20250506", Family: "claude"},
}

// HealthCheck validates connectivity by checking that the API key is set.
// A full /messages roundtrip is unnecessary for health — if the key is blank,
// every call will fail.
func (p *Provider) HealthCheck(_ context.Context) (llm.ProviderHealth, error) {
	if p.apiKey == "" {
		return llm.ProviderHealth{
			Provider: "anthropic",
			Healthy:  false,
			Message:  "API key not configured",
		}, nil
	}
	return llm.ProviderHealth{
		Provider:   "anthropic",
		Healthy:    true,
		Message:    "API key configured",
		ModelCount: len(knownModels),
	}, nil
}

// ListModels returns the static catalog of known Claude models.
func (p *Provider) ListModels(_ context.Context) ([]llm.ModelDescriptor, error) {
	out := make([]llm.ModelDescriptor, len(knownModels))
	copy(out, knownModels)
	return out, nil
}

// GetModel returns a descriptor for a specific model by name.
// Matches by exact name or by prefix (e.g. "claude-sonnet-4" matches
// "claude-sonnet-4-20250514").
func (p *Provider) GetModel(_ context.Context, model string) (llm.ModelDescriptor, error) {
	model = strings.ToLower(strings.TrimSpace(model))
	for _, m := range knownModels {
		if strings.ToLower(m.Name) == model || strings.HasPrefix(strings.ToLower(m.Name), model) {
			return m, nil
		}
	}
	// Return a minimal descriptor for unknown models rather than erroring,
	// since Anthropic may have newer models not in our static catalog.
	return llm.ModelDescriptor{Name: model}, fmt.Errorf("anthropic: model %q not in known catalog (may still work)", model)
}
