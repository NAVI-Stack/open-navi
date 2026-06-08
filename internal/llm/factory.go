package llm

import (
	"fmt"
	"strings"

	"github.com/open-navi/navi/internal/config"
)

// FromConfig builds a Provider from the runtime config.
// Returns either a single Provider, a FallbackChain if multiple providers are
// configured, or a Router when logical model routes are defined.
// Priority for the base chain: Ollama → Anthropic → OpenAI → OpenRouter.
func FromConfig(cfg *config.Config) (Provider, error) {
	var candidates []Candidate
	providers := make(map[string]Provider)

	// Ollama — always first if configured (no key required)
	if cfg.LLM.OllamaURL != "" {
		p, err := BuildProviderForSelection(&cfg.LLM, "ollama")
		if err != nil {
			return nil, err
		}
		model := cfg.LLM.OllamaModel
		if model == "" {
			model = "llama3:latest"
		}
		candidates = append(candidates, Candidate{Provider: p, Model: model})
		providers[p.Name()] = p
	}

	// Anthropic
	if cfg.LLM.AnthropicKey != "" {
		p, err := BuildProviderForSelection(&cfg.LLM, "anthropic")
		if err != nil {
			return nil, err
		}
		model := cfg.LLM.AnthropicModel
		if model == "" {
			model = "claude-sonnet-4-20250514"
		}
		candidates = append(candidates, Candidate{Provider: p, Model: model})
		providers[p.Name()] = p
	}

	// OpenAI
	if cfg.LLM.OpenAIKey != "" {
		p, err := BuildProviderForSelection(&cfg.LLM, "openai")
		if err != nil {
			return nil, err
		}
		model := cfg.LLM.OpenAIModel
		if model == "" {
			model = "gpt-4o-mini"
		}
		candidates = append(candidates, Candidate{Provider: p, Model: model})
		providers[p.Name()] = p
	}

	// OpenRouter
	if cfg.LLM.OpenRouterKey != "" {
		p, err := BuildProviderForSelection(&cfg.LLM, "openrouter")
		if err != nil {
			return nil, err
		}
		model := cfg.LLM.OpenRouterModel
		if model == "" {
			model = "anthropic/claude-3.5-haiku"
		}
		candidates = append(candidates, Candidate{Provider: p, Model: model})
		providers[p.Name()] = p
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("llm: no providers configured (set ollama_url, anthropic_key, openai_key, or openrouter_key)")
	}

	// Build base provider: single backend or multi-provider fallback chain.
	var base Provider
	if len(candidates) == 1 {
		base = candidates[0].Provider
	} else {
		base = NewFallbackChain(candidates)
	}

	// Derive logical model routes. Explicit config wins; otherwise derive from
	// legacy *Model fields when present.
	routes := cfg.LLM.Routes
	if len(routes) == 0 {
		routes = defaultModelRoutes(&cfg.LLM)
	}
	if len(routes) == 0 {
		return base, nil
	}

	router := NewRouter(base)

	for name, mr := range routes {
		if name == "" {
			continue
		}

		cands, providerQualified, err := buildRouteCandidates(mr, providers)
		if err != nil {
			return nil, fmt.Errorf("llm: invalid route %q: %w", name, err)
		}

		// Provider-qualified specs (e.g. "anthropic/claude-...") get their own
		// fallback chain with provider-specific models.
		if providerQualified && len(cands) > 0 {
			router.AddRoute(name, NewFallbackChain(cands), "")
			continue
		}

		// Legacy / implicit routes fall back to the base provider with a
		// concrete model string. We honor Primary first, then the first fallback.
		model := mr.Primary
		if model == "" && len(mr.Fallbacks) > 0 {
			model = mr.Fallbacks[0]
		}
		if model == "" {
			continue
		}
		router.AddRoute(name, base, model)
	}

	return router, nil
}

// defaultModelRoutes builds logical model routes from legacy *Model fields.
// When none are set, it returns nil.
func defaultModelRoutes(llmCfg *config.LLMConfig) map[string]config.ModelRoute {
	routes := make(map[string]config.ModelRoute)

	if llmCfg.OrchestratorModel != "" {
		routes["orchestrator"] = config.ModelRoute{Primary: llmCfg.OrchestratorModel}
	}
	if llmCfg.CoderModel != "" {
		routes["coder"] = config.ModelRoute{Primary: llmCfg.CoderModel}
	}
	if llmCfg.ChatModel != "" {
		routes["chat"] = config.ModelRoute{Primary: llmCfg.ChatModel}
	}

	if len(routes) == 0 {
		return nil
	}
	return routes
}

// buildRouteCandidates parses provider-qualified model strings like
// "anthropic/claude-sonnet-4-20250514" into concrete Candidates using the
// configured provider instances. It returns whether any provider-qualified
// specs were found.
func buildRouteCandidates(route config.ModelRoute, providers map[string]Provider) ([]Candidate, bool, error) {
	var cands []Candidate
	providerQualified := false

	addSpec := func(spec string) error {
		if spec == "" {
			return nil
		}
		parts := strings.SplitN(spec, "/", 2)
		if len(parts) != 2 {
			// Not provider-qualified; let the caller handle it as legacy.
			return nil
		}
		provName, modelName := parts[0], parts[1]
		p, ok := providers[provName]
		if !ok {
			return fmt.Errorf("provider %q not configured", provName)
		}
		cands = append(cands, Candidate{
			Provider: p,
			Model:    modelName,
		})
		providerQualified = true
		return nil
	}

	if err := addSpec(route.Primary); err != nil {
		return nil, false, err
	}
	for _, fb := range route.Fallbacks {
		if err := addSpec(fb); err != nil {
			return nil, false, err
		}
	}

	return cands, providerQualified, nil
}
