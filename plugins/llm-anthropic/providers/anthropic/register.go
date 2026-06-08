package anthropic

import (
	"fmt"

	"github.com/ceoai/navi/internal/config"
	"github.com/ceoai/navi/internal/llm"
)

func init() {
	llm.RegisterProviderFactory("anthropic", newAnthropicProvider)
}

func newAnthropicProvider(cfg *config.LLMConfig, providerKey string) (llm.Provider, error) {
	if providerKey != "anthropic" {
		return nil, fmt.Errorf("anthropic plugin: unsupported provider key %q", providerKey)
	}
	apiKey := ""
	if cfg != nil {
		apiKey = cfg.AnthropicKey
	}
	return New(Config{APIKey: apiKey}), nil
}
