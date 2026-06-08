package openai

import (
	"context"
	"fmt"
	"strings"

	"github.com/open-navi/navi/internal/llm"
)

// knownOpenAIModels is the static catalog of OpenAI models.
var knownOpenAIModels = []llm.ModelDescriptor{
	{Name: "gpt-4o", Family: "gpt-4"},
	{Name: "gpt-4o-mini", Family: "gpt-4"},
	{Name: "gpt-4-turbo", Family: "gpt-4"},
	{Name: "o4-mini", Family: "o4"},
	{Name: "o3", Family: "o3"},
	{Name: "o3-mini", Family: "o3"},
	{Name: "o1", Family: "o1"},
	{Name: "o1-mini", Family: "o1"},
}

// HealthCheck validates connectivity by checking that the API key is set.
func (p *Provider) HealthCheck(_ context.Context) (llm.ProviderHealth, error) {
	if p.apiKey == "" {
		return llm.ProviderHealth{
			Provider: p.name,
			Healthy:  false,
			Message:  "API key not configured",
		}, nil
	}
	return llm.ProviderHealth{
		Provider:   p.name,
		Healthy:    true,
		Message:    "API key configured",
		ModelCount: len(knownOpenAIModels),
	}, nil
}

// ListModels returns the static catalog of known OpenAI models.
func (p *Provider) ListModels(_ context.Context) ([]llm.ModelDescriptor, error) {
	out := make([]llm.ModelDescriptor, len(knownOpenAIModels))
	copy(out, knownOpenAIModels)
	return out, nil
}

// GetModel returns a descriptor for a specific model by name.
func (p *Provider) GetModel(_ context.Context, model string) (llm.ModelDescriptor, error) {
	model = strings.ToLower(strings.TrimSpace(model))
	for _, m := range knownOpenAIModels {
		if strings.ToLower(m.Name) == model || strings.HasPrefix(strings.ToLower(m.Name), model) {
			return m, nil
		}
	}
	return llm.ModelDescriptor{Name: model}, fmt.Errorf("openai: model %q not in known catalog (may still work)", model)
}
