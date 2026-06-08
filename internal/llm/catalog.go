package llm

import (
	"sort"
	"strings"

	"github.com/open-navi/navi/internal/config"
)

// LLMModelInfo describes a single model exposed by a provider.
type LLMModelInfo struct {
	Name string `json:"name"`
}

// LLMProviderInfo describes one provider and its available models.
type LLMProviderInfo struct {
	Key         string         `json:"key"`
	DisplayName string         `json:"display_name"`
	Models      []LLMModelInfo `json:"models"`
}

// LLMCatalog is a read-only snapshot of all configured providers and models.
type LLMCatalog struct {
	Providers []LLMProviderInfo `json:"providers"`
}

// BuildCatalog derives an LLMCatalog from the runtime LLM config.
//
// Precedence:
//   - When cfg.Providers is non-empty, it is treated as the canonical
//     catalog and returned directly (after normalization).
//   - Otherwise, a catalog is synthesized from the legacy *Model fields
//     and provider keys so existing configs require no changes.
func BuildCatalog(cfg *config.LLMConfig) LLMCatalog {
	if cfg == nil {
		return LLMCatalog{}
	}

	if len(cfg.Providers) > 0 {
		return buildCatalogFromProviders(cfg.Providers)
	}

	return buildCatalogFromLegacy(cfg)
}

func buildCatalogFromProviders(providers map[string]config.ProviderConfig) LLMCatalog {
	if len(providers) == 0 {
		return LLMCatalog{}
	}

	out := LLMCatalog{
		Providers: make([]LLMProviderInfo, 0, len(providers)),
	}

	keys := make([]string, 0, len(providers))
	for k := range providers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		pc := providers[key]
		display := pc.DisplayName
		if strings.TrimSpace(display) == "" {
			display = key
		}

		modelSet := make(map[string]struct{})
		for _, m := range pc.Models {
			m = strings.TrimSpace(m)
			if m == "" {
				continue
			}
			modelSet[m] = struct{}{}
		}

		models := make([]LLMModelInfo, 0, len(modelSet))
		for m := range modelSet {
			models = append(models, LLMModelInfo{Name: m})
		}
		sort.Slice(models, func(i, j int) bool {
			return models[i].Name < models[j].Name
		})

		out.Providers = append(out.Providers, LLMProviderInfo{
			Key:         key,
			DisplayName: display,
			Models:      models,
		})
	}

	return out
}

func buildCatalogFromLegacy(cfg *config.LLMConfig) LLMCatalog {
	var providers []LLMProviderInfo

	// Helper to append a provider entry when at least one model is present.
	addProvider := func(key, display string, models ...string) {
		modelSet := make(map[string]struct{})
		for _, m := range models {
			m = strings.TrimSpace(m)
			if m == "" {
				continue
			}
			modelSet[m] = struct{}{}
		}
		if len(modelSet) == 0 {
			return
		}
		list := make([]LLMModelInfo, 0, len(modelSet))
		for m := range modelSet {
			list = append(list, LLMModelInfo{Name: m})
		}
		sort.Slice(list, func(i, j int) bool {
			return list[i].Name < list[j].Name
		})
		providers = append(providers, LLMProviderInfo{
			Key:         key,
			DisplayName: display,
			Models:      list,
		})
	}

	// Ollama: available when URL is set.
	if strings.TrimSpace(cfg.OllamaURL) != "" {
		addProvider("ollama", "Ollama",
			cfg.OllamaModel,
			cfg.OllamaDiscussModel,
		)
	}

	// Anthropic.
	if cfg.AnthropicKey != "" {
		addProvider("anthropic", "Anthropic",
			cfg.AnthropicModel,
		)
	}

	// OpenAI.
	if cfg.OpenAIKey != "" {
		addProvider("openai", "OpenAI",
			cfg.OpenAIModel,
		)
	}

	// OpenRouter.
	if cfg.OpenRouterKey != "" {
		addProvider("openrouter", "OpenRouter",
			cfg.OpenRouterModel,
		)
	}

	return LLMCatalog{Providers: providers}
}

// ListProviders returns the provider keys in stable order.
func (c LLMCatalog) ListProviders() []string {
	out := make([]string, 0, len(c.Providers))
	for _, p := range c.Providers {
		out = append(out, p.Key)
	}
	return out
}

// ListModels returns all model names for the given provider key.
func (c LLMCatalog) ListModels(providerKey string) []string {
	for _, p := range c.Providers {
		if p.Key == providerKey {
			out := make([]string, 0, len(p.Models))
			for _, m := range p.Models {
				out = append(out, m.Name)
			}
			return out
		}
	}
	return nil
}

// ResolveModelAlias maps user input (e.g. "llama3.1") to a catalog model name (e.g. "llama3.1:latest").
// Returns the resolved catalog name, or userInput unchanged if no match.
func ResolveModelAlias(catalog LLMCatalog, providerKey, userInput string) string {
	userInput = strings.TrimSpace(userInput)
	providerKey = strings.TrimSpace(strings.ToLower(providerKey))
	if userInput == "" {
		return userInput
	}
	var models []LLMModelInfo
	for _, p := range catalog.Providers {
		if strings.ToLower(p.Key) == providerKey {
			models = p.Models
			break
		}
	}
	if len(models) == 0 {
		return userInput
	}
	// Exact match (case-insensitive)
	for _, m := range models {
		if strings.EqualFold(m.Name, userInput) {
			return m.Name
		}
	}
	// userInput without ":" -> try userInput + ":latest"
	if !strings.Contains(userInput, ":") {
		for _, m := range models {
			if strings.EqualFold(m.Name, userInput+":latest") {
				return m.Name
			}
		}
		// Prefix match: catalog name is userInput or userInput:tag
		for _, m := range models {
			if m.Name == userInput || strings.HasPrefix(strings.ToLower(m.Name), strings.ToLower(userInput)+":") {
				return m.Name
			}
		}
	}
	return userInput
}
