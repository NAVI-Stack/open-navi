package main

import (
	"testing"

	"github.com/open-navi/navi/internal/connectors"
	"github.com/open-navi/navi/internal/llm"
)

func TestPluginBootstrapRegistersProviderFactories(t *testing.T) {
	got := map[string]bool{}
	for _, key := range llm.RegisteredProviderKeys() {
		got[key] = true
	}
	for _, key := range []string{"anthropic", "ollama", "openai", "openrouter"} {
		if !got[key] {
			t.Fatalf("expected bootstrap to register LLM provider %q; got %v", key, llm.RegisteredProviderKeys())
		}
	}
}

func TestPluginBootstrapRegistersConnectorFactories(t *testing.T) {
	registry := connectors.NewRegistry()
	registerTelegramSetupFactory(registry, nil, nil)
	registerSlackFactory(registry, nil)

	for _, id := range []string{"telegram", "slack"} {
		if _, ok := registry.GetDriver(id); !ok {
			t.Fatalf("expected bootstrap connector driver %q to be registered", id)
		}
	}
}
