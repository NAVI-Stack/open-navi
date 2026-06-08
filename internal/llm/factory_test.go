package llm

import (
	"testing"

	"github.com/open-navi/navi/internal/config"
)

func TestFromConfig_OllamaOnly(t *testing.T) {
	cfg := &config.Config{
		LLM: config.LLMConfig{
			OllamaURL:   "http://localhost:11434/v1",
			OllamaModel: "llama3.2",
		},
	}

	p, err := FromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "ollama" {
		t.Errorf("expected ollama, got %s", p.Name())
	}
	// Single provider — should not be a FallbackChain
	if _, ok := p.(*FallbackChain); ok {
		t.Error("single provider should not be wrapped in FallbackChain")
	}
}

func TestFromConfig_MultiProvider(t *testing.T) {
	cfg := &config.Config{
		LLM: config.LLMConfig{
			OllamaURL:    "http://localhost:11434/v1",
			AnthropicKey: "sk-ant-test",
		},
	}

	p, err := FromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fc, ok := p.(*FallbackChain)
	if !ok {
		t.Fatal("expected FallbackChain for multiple providers")
	}
	if len(fc.candidates) != 2 {
		t.Errorf("expected 2 candidates, got %d", len(fc.candidates))
	}
	// Ollama should be first
	if fc.candidates[0].Provider.Name() != "ollama" {
		t.Errorf("expected ollama first, got %s", fc.candidates[0].Provider.Name())
	}
}

func TestFromConfig_NoProviders(t *testing.T) {
	cfg := &config.Config{}
	_, err := FromConfig(cfg)
	if err == nil {
		t.Fatal("expected error when no providers configured")
	}
}

func TestFromConfig_OpenRouterModelPreserved(t *testing.T) {
	cfg := &config.Config{
		LLM: config.LLMConfig{
			OpenRouterKey:   "or-key",
			OpenRouterModel: "anthropic/claude-3.5-haiku",
		},
	}

	p, err := FromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "openrouter" {
		t.Errorf("expected openrouter, got %s", p.Name())
	}
}
