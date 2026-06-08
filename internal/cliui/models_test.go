package cliui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/open-navi/navi/internal/llm"
	"github.com/stretchr/testify/assert"
)

func TestFetchCatalog(t *testing.T) {
	catalog := llm.LLMCatalog{
		Providers: []llm.LLMProviderInfo{
			{Key: "openai", DisplayName: "OpenAI", Models: []llm.LLMModelInfo{{Name: "gpt-4o"}}},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/llm/catalog", r.URL.Path)
		assert.Equal(t, "test-api-key", r.Header.Get("X-API-Key"))
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(catalog)
	}))
	defer server.Close()

	got, err := FetchCatalog(server.URL, "test-api-key")
	assert.NoError(t, err)
	assert.Len(t, got.Providers, 1)
	assert.Equal(t, "openai", got.Providers[0].Key)
}

func TestProviderSelectFormAlwaysNonNil(t *testing.T) {
	// Even with an empty catalog, ProviderSelectForm must return a non-nil form
	// because hardcoded defaults are always injected.
	state := NewOnboardingState()

	t.Run("empty_catalog", func(t *testing.T) {
		form := ProviderSelectForm(llm.LLMCatalog{}, state)
		assert.NotNil(t, form)
	})

	t.Run("nil_providers", func(t *testing.T) {
		form := ProviderSelectForm(llm.LLMCatalog{Providers: nil}, state)
		assert.NotNil(t, form)
	})
}

func TestProviderSelectIncludesManualFallback(t *testing.T) {
	// The form must always include a manual path. First-run setup requires a
	// provider, so there is intentionally no skip option.
	state := NewOnboardingState()
	assert.NotNil(t, ProviderSelectForm(llm.LLMCatalog{}, state))
	assert.NotNil(t, ManualProviderForm(state))
	assert.NotNil(t, ManualModelForm(state))
}

func TestParseOllamaList(t *testing.T) {
	t.Run("typical_output", func(t *testing.T) {
		input := `NAME                    ID              SIZE      MODIFIED
llama3.2:latest         a80c4f17acd5    2.0 GB    2 days ago
mistral:latest          f974a74358d6    4.1 GB    5 days ago
codellama:7b            8fdf8f752f6e    3.8 GB    1 week ago
`
		got := parseOllamaList(input)
		assert.Equal(t, []string{"llama3.2:latest", "mistral:latest", "codellama:7b"}, got)
	})

	t.Run("empty_output", func(t *testing.T) {
		got := parseOllamaList("")
		assert.Empty(t, got)
	})

	t.Run("header_only", func(t *testing.T) {
		got := parseOllamaList("NAME    ID    SIZE    MODIFIED\n")
		assert.Empty(t, got)
	})

	t.Run("single_model", func(t *testing.T) {
		input := "NAME    ID    SIZE    MODIFIED\nllama3:latest    abc123    2.0 GB    1 day ago\n"
		got := parseOllamaList(input)
		assert.Equal(t, []string{"llama3:latest"}, got)
	})
}

func TestOllamaModelSelectForm(t *testing.T) {
	t.Run("includes_discovered_models_and_manual", func(t *testing.T) {
		state := NewOnboardingState()
		models := []string{"llama3.2:latest", "mistral:latest"}
		form := OllamaModelSelectForm(models, state)
		assert.NotNil(t, form)
	})

	t.Run("empty_models_still_has_manual_option", func(t *testing.T) {
		state := NewOnboardingState()
		form := OllamaModelSelectForm([]string{}, state)
		assert.NotNil(t, form)
	})
}

func TestModelSelectionFallback(t *testing.T) {
	state := NewOnboardingState()
	catalog := llm.LLMCatalog{
		Providers: []llm.LLMProviderInfo{
			{Key: "empty-provider", DisplayName: "Empty", Models: []llm.LLMModelInfo{}},
		},
	}

	t.Run("empty_provider_no_form", func(t *testing.T) {
		state.SelectedProvider = "empty-provider"
		form := ModelSelectForm(catalog, state)
		assert.Nil(t, form)
	})

	t.Run("manual_provider_manual_form", func(t *testing.T) {
		assert.NotNil(t, ManualProviderForm(state))
		assert.NotNil(t, ManualModelForm(state))
	})

}
