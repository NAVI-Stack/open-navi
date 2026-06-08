package llm

import "github.com/open-navi/navi/internal/config"

func BuildProviderForSelection(cfg *config.LLMConfig, providerKey string) (Provider, error) {
	return BuildProviderByKey(cfg, providerKey)
}
