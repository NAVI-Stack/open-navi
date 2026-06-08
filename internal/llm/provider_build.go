package llm

import "github.com/ceoai/navi/internal/config"

func BuildProviderForSelection(cfg *config.LLMConfig, providerKey string) (Provider, error) {
	return BuildProviderByKey(cfg, providerKey)
}
