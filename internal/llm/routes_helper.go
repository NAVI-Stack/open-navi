package llm

import "github.com/ceoai/navi/internal/config"

func EffectiveRoutes(cfg *config.LLMConfig) map[string]config.ModelRoute {
	if cfg == nil {
		return nil
	}
	if len(cfg.Routes) > 0 {
		return cfg.Routes
	}
	return defaultModelRoutes(cfg)
}
