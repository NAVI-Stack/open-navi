package openai

import (
	"fmt"

	"github.com/open-navi/navi/internal/config"
	"github.com/open-navi/navi/internal/llm"
)

func init() {
	llm.RegisterProviderFactory("openai", newOpenAIProvider)
	llm.RegisterProviderFactory("openrouter", newOpenAIProvider)
}

func newOpenAIProvider(cfg *config.LLMConfig, providerKey string) (llm.Provider, error) {
	switch providerKey {
	case "openai":
		apiKey := ""
		if cfg != nil {
			apiKey = cfg.OpenAIKey
		}
		return New(Config{Name: "openai", APIKey: apiKey}), nil
	case "openrouter":
		apiKey := ""
		if cfg != nil {
			apiKey = cfg.OpenRouterKey
		}
		return New(Config{Name: "openrouter", APIKey: apiKey}), nil
	default:
		return nil, fmt.Errorf("openai plugin: unsupported provider key %q", providerKey)
	}
}
