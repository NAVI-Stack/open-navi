package llm

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/ceoai/navi/internal/config"
)

// ProviderFactory constructs a concrete inference provider for a provider key.
// Concrete implementations live in plugins/llm-* packages and register through
// the bootstrap import boundary.
type ProviderFactory func(cfg *config.LLMConfig, providerKey string) (Provider, error)

var providerFactories = struct {
	sync.RWMutex
	byKey      map[string]ProviderFactory
	activeKeys map[string]bool
}{byKey: make(map[string]ProviderFactory)}

func RegisterProviderFactory(providerKey string, factory ProviderFactory) {
	key := strings.ToLower(strings.TrimSpace(providerKey))
	if key == "" || factory == nil {
		return
	}
	providerFactories.Lock()
	defer providerFactories.Unlock()
	providerFactories.byKey[key] = factory
}

func BuildProviderByKey(cfg *config.LLMConfig, providerKey string) (Provider, error) {
	key := strings.ToLower(strings.TrimSpace(providerKey))
	providerFactories.RLock()
	activeKeys := providerFactories.activeKeys
	if activeKeys != nil && !activeKeys[key] {
		providerFactories.RUnlock()
		return nil, fmt.Errorf("llm: provider %q is not active", providerKey)
	}
	factory := providerFactories.byKey[key]
	providerFactories.RUnlock()
	if factory == nil {
		return nil, fmt.Errorf("llm: provider %q is not registered", providerKey)
	}
	return factory(cfg, key)
}

func SetActiveProviderKeys(keys map[string]bool) {
	providerFactories.Lock()
	defer providerFactories.Unlock()
	if keys == nil {
		providerFactories.activeKeys = nil
		return
	}
	active := make(map[string]bool, len(keys))
	for key, enabled := range keys {
		key = strings.ToLower(strings.TrimSpace(key))
		if key != "" && enabled {
			active[key] = true
		}
	}
	providerFactories.activeKeys = active
}

func RegisteredProviderKeys() []string {
	providerFactories.RLock()
	defer providerFactories.RUnlock()
	keys := make([]string, 0, len(providerFactories.byKey))
	for key := range providerFactories.byKey {
		if providerFactories.activeKeys != nil && !providerFactories.activeKeys[key] {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
