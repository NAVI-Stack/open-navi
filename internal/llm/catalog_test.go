package llm

import (
	"testing"
)

func TestResolveModelAlias(t *testing.T) {
	catalog := LLMCatalog{Providers: []LLMProviderInfo{
		{Key: "ollama", DisplayName: "Ollama", Models: []LLMModelInfo{
			{Name: "llama3.1:latest"},
			{Name: "llama3:latest"},
			{Name: "mistral:7b"},
		}},
	}}

	t.Run("exact_match", func(t *testing.T) {
		got := ResolveModelAlias(catalog, "ollama", "llama3.1:latest")
		if got != "llama3.1:latest" {
			t.Errorf("ResolveModelAlias(ollama, llama3.1:latest) = %q, want llama3.1:latest", got)
		}
	})
	t.Run("alias_without_tag_resolves_to_latest", func(t *testing.T) {
		got := ResolveModelAlias(catalog, "ollama", "llama3.1")
		if got != "llama3.1:latest" {
			t.Errorf("ResolveModelAlias(ollama, llama3.1) = %q, want llama3.1:latest", got)
		}
	})
	t.Run("no_match_returns_unchanged", func(t *testing.T) {
		got := ResolveModelAlias(catalog, "ollama", "nonexistent")
		if got != "nonexistent" {
			t.Errorf("ResolveModelAlias(ollama, nonexistent) = %q, want nonexistent", got)
		}
	})
	t.Run("unknown_provider_returns_unchanged", func(t *testing.T) {
		got := ResolveModelAlias(catalog, "anthropic", "llama3.1")
		if got != "llama3.1" {
			t.Errorf("ResolveModelAlias(anthropic, llama3.1) = %q, want llama3.1", got)
		}
	})
}
