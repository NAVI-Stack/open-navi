package examplemessaging

import "os"

// LoadConfigFromEnv is a simple development fallback.
// Production should use NAVI's config/secret broker instead of reading env directly.
func LoadConfigFromEnv() Config {
	return Config{
		APIKey:  os.Getenv("EXAMPLE_MESSAGING_API_KEY"),
		BaseURL: os.Getenv("EXAMPLE_MESSAGING_BASE_URL"),
	}
}
