package petbridge

import "os"

// LoadConfigFromEnv is a simple development fallback.
// Production should use NAVI's config/secret broker instead of reading env directly.
func LoadConfigFromEnv() Config {
	return Config{
		BridgeToken: os.Getenv("NAVI_PET_BRIDGE_TOKEN"),
		BridgeURL:   os.Getenv("NAVI_PET_BRIDGE_URL"),
	}
}
