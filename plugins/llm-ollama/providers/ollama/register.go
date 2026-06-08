package ollama

import (
	"fmt"

	"github.com/ceoai/navi/internal/config"
	"github.com/ceoai/navi/internal/llm"
)

func init() {
	llm.RegisterProviderFactory("ollama", newOllamaProvider)
}

func newOllamaProvider(cfg *config.LLMConfig, providerKey string) (llm.Provider, error) {
	if providerKey != "ollama" {
		return nil, fmt.Errorf("ollama plugin: unsupported provider key %q", providerKey)
	}
	baseURL := ""
	if cfg != nil {
		baseURL = cfg.OllamaURL
	}
	return New(Config{BaseURL: baseURL}), nil
}
