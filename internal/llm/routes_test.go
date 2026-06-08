package llm

import (
	"testing"

	"github.com/open-navi/navi/internal/config"
)

func TestFromConfig_DefaultModelRoutesUseLegacyModels(t *testing.T) {
	cfg := &config.Config{
		LLM: config.LLMConfig{
			OpenAIKey:         "sk-test",
			OpenAIModel:       "gpt-test-base",
			OrchestratorModel: "orch-model",
			CoderModel:        "coder-model",
			ChatModel:         "chat-model",
		},
	}

	p, err := FromConfig(cfg)
	if err != nil {
		t.Fatalf("FromConfig error: %v", err)
	}

	r, ok := p.(*Router)
	if !ok {
		t.Fatalf("expected Router from FromConfig, got %T", p)
	}

	if r.defaultProv == nil {
		t.Fatalf("expected default provider to be set")
	}

	if len(r.routes) != 3 {
		t.Fatalf("expected 3 logical routes (orchestrator, coder, chat), got %d", len(r.routes))
	}

	check := func(name, wantModel string) {
		binding, ok := r.routes[name]
		if !ok {
			t.Fatalf("missing route %q", name)
		}
		if binding.provider != r.defaultProv {
			t.Fatalf("route %q should use default provider", name)
		}
		if binding.model != wantModel {
			t.Fatalf("route %q model = %q, want %q", name, binding.model, wantModel)
		}
	}

	check("orchestrator", "orch-model")
	check("coder", "coder-model")
	check("chat", "chat-model")
}

func TestFromConfig_ProviderQualifiedRoutesBuildFallbackChain(t *testing.T) {
	cfg := &config.Config{
		LLM: config.LLMConfig{
			OpenAIKey:       "sk-openai",
			OpenAIModel:     "gpt-4o-mini",
			OpenRouterKey:   "sk-openrouter",
			OpenRouterModel: "anthropic/claude-3.5-haiku",
			Routes: map[string]config.ModelRoute{
				"chat": {
					Primary:   "openai/gpt-4.1",
					Fallbacks: []string{"openrouter/anthropic/claude-3.5-haiku"},
				},
			},
		},
	}

	p, err := FromConfig(cfg)
	if err != nil {
		t.Fatalf("FromConfig error: %v", err)
	}

	r, ok := p.(*Router)
	if !ok {
		t.Fatalf("expected Router from FromConfig, got %T", p)
	}

	binding, ok := r.routes["chat"]
	if !ok {
		t.Fatalf("expected chat route to be configured")
	}

	fc, ok := binding.provider.(*FallbackChain)
	if !ok {
		t.Fatalf("expected chat route provider to be FallbackChain, got %T", binding.provider)
	}

	if len(fc.candidates) != 2 {
		t.Fatalf("expected 2 candidates in chat route chain, got %d", len(fc.candidates))
	}

	if fc.candidates[0].Provider.Name() != "openai" || fc.candidates[0].Model != "gpt-4.1" {
		t.Fatalf("unexpected primary candidate: %s / %s", fc.candidates[0].Provider.Name(), fc.candidates[0].Model)
	}
	if fc.candidates[1].Provider.Name() != "openrouter" || fc.candidates[1].Model != "anthropic/claude-3.5-haiku" {
		t.Fatalf("unexpected fallback candidate: %s / %s", fc.candidates[1].Provider.Name(), fc.candidates[1].Model)
	}
}
