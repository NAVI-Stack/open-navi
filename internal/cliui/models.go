package cliui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"github.com/ceoai/navi/internal/llm"
	"github.com/charmbracelet/huh"
)

// FetchCatalog fetches the LLM catalog from the gateway.
func FetchCatalog(gatewayURL string, apiKey string) (llm.LLMCatalog, error) {
	req, err := http.NewRequest("GET", gatewayURL+"/api/llm/catalog", nil)
	if err != nil {
		return llm.LLMCatalog{}, err
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return llm.LLMCatalog{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return llm.LLMCatalog{}, fmt.Errorf("failed to fetch catalog: %d", resp.StatusCode)
	}

	var catalog llm.LLMCatalog
	if err := json.NewDecoder(resp.Body).Decode(&catalog); err != nil {
		return llm.LLMCatalog{}, err
	}
	return catalog, nil
}

// ProviderSelectForm returns a form for selecting an LLM provider.
func ProviderSelectForm(catalog llm.LLMCatalog, state *OnboardingState) *huh.Form {
	var options []huh.Option[string]

	// Add catalog providers if any
	for _, p := range catalog.Providers {
		options = append(options, huh.NewOption(p.DisplayName, p.Key))
	}

	// Ensure default providers are present if catalog is empty or missing them
	defaults := []struct{ Key, Name string }{
		{"anthropic", "Anthropic"},
		{"openai", "OpenAI"},
		{"ollama", "Ollama"},
		{"openrouter", "Open Router"},
	}

	for _, d := range defaults {
		found := false
		for _, o := range options {
			if o.Value == d.Key {
				found = true
				break
			}
		}
		if !found {
			options = append(options, huh.NewOption(d.Name, d.Key))
		}
	}

	options = append(options, huh.NewOption("Manual Entry / Custom", "manual"))

	return huh.NewForm(
		huh.NewGroup(
			Select(
				"Select an AI Provider",
				"This is the 'brain' of NAVI.",
				options,
				&state.SelectedProvider,
			),
		),
	)
}

// ManualProviderForm returns a form for entering a provider name manually.
func ManualProviderForm(state *OnboardingState) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			Input(
				"AI Provider",
				"Enter a provider name manually (e.g., openai, anthropic, ollama).",
				"openai",
				&state.SelectedProvider,
			),
		),
	)
}

// ModelSelectForm returns a form for selecting a model for the chosen provider.
func ModelSelectForm(catalog llm.LLMCatalog, state *OnboardingState) *huh.Form {
	var provider *llm.LLMProviderInfo
	for i := range catalog.Providers {
		if catalog.Providers[i].Key == state.SelectedProvider {
			provider = &catalog.Providers[i]
			break
		}
	}

	if provider == nil || len(provider.Models) == 0 {
		return nil
	}

	var options []huh.Option[string]
	for _, m := range provider.Models {
		options = append(options, huh.NewOption(m.Name, m.Name))
	}
	options = append(options, huh.NewOption("Custom Model Name", "custom"))

	return huh.NewForm(
		huh.NewGroup(
			Select(
				fmt.Sprintf("%s Models", provider.DisplayName),
				"Choose the model you'd like me to use.",
				options,
				&state.SelectedModel,
			),
		),
	)
}

// FetchOllamaModels runs `ollama list` and returns the discovered model names.
func FetchOllamaModels() ([]string, error) {
	cmd := exec.Command("ollama", "list")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return parseOllamaList(out.String()), nil
}

// parseOllamaList parses the output of `ollama list` and returns model names.
// The first line is a header; each subsequent non-empty line has the model name
// as its first whitespace-delimited field.
func parseOllamaList(output string) []string {
	lines := strings.Split(output, "\n")
	var models []string
	for i, line := range lines {
		if i == 0 { // skip header
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		models = append(models, fields[0])
	}
	return models
}

// OllamaModelSelectForm returns a Huh Select form populated with locally
// installed Ollama models. An "Enter manually" option is always appended so
// the user can type a model name that isn't installed yet.
func OllamaModelSelectForm(models []string, state *OnboardingState) *huh.Form {
	options := make([]huh.Option[string], 0, len(models)+1)
	for _, m := range models {
		options = append(options, huh.NewOption(m, m))
	}
	options = append(options, huh.NewOption("Enter model name manually", "custom"))

	return huh.NewForm(
		huh.NewGroup(
			Select(
				"Ollama Models",
				"Choose a locally installed model or enter one manually.",
				options,
				&state.SelectedModel,
			),
		),
	)
}

// ManualModelForm returns a form for entering a model name manually.
func ManualModelForm(state *OnboardingState) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			Input(
				"Model Name",
				"Enter the model name (e.g., llama3.2, gpt-4o).",
				"model-name",
				&state.SelectedModel,
			),
		),
	)
}

// ProviderAPIKeyForm returns a form for entering a provider API key if needed.
func ProviderAPIKeyForm(state *OnboardingState) *huh.Form {
	if state.SelectedProvider == "ollama" || state.SelectedProvider == "manual" || state.SelectedProvider == "" {
		return nil
	}

	return huh.NewForm(
		huh.NewGroup(
			Secret(
				fmt.Sprintf("%s API Key", strings.Title(state.SelectedProvider)),
				"Your secret key for accessing the provider.",
				"sk-...",
				&state.ProviderAPIKey,
			),
		),
	)
}
