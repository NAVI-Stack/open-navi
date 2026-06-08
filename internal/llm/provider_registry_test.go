package llm

import (
	"context"
	"testing"

	"github.com/open-navi/navi/internal/config"
)

func TestProviderRegistryBuildsByProviderKey(t *testing.T) {
	t.Cleanup(func() { SetActiveProviderKeys(nil) })
	key := "registry-test-provider"
	RegisterProviderFactory(key, func(cfg *config.LLMConfig, providerKey string) (Provider, error) {
		return registryTestProvider{name: providerKey}, nil
	})

	p, err := BuildProviderByKey(&config.LLMConfig{}, key)
	if err != nil {
		t.Fatalf("BuildProviderByKey: %v", err)
	}
	if p.Name() != key {
		t.Fatalf("Name = %q, want %q", p.Name(), key)
	}
}

func TestProviderRegistryBuildRequiresActiveProviderWhenAllowlistSet(t *testing.T) {
	t.Cleanup(func() { SetActiveProviderKeys(nil) })
	RegisterProviderFactory("active-registry-test-provider", func(cfg *config.LLMConfig, providerKey string) (Provider, error) {
		return registryTestProvider{name: providerKey}, nil
	})
	RegisterProviderFactory("inactive-registry-test-provider", func(cfg *config.LLMConfig, providerKey string) (Provider, error) {
		return registryTestProvider{name: providerKey}, nil
	})
	SetActiveProviderKeys(map[string]bool{"active-registry-test-provider": true})

	if _, err := BuildProviderByKey(&config.LLMConfig{}, "active-registry-test-provider"); err != nil {
		t.Fatalf("active provider should build: %v", err)
	}
	if _, err := BuildProviderByKey(&config.LLMConfig{}, "inactive-registry-test-provider"); err == nil {
		t.Fatal("inactive provider should not build")
	}
	keys := RegisteredProviderKeys()
	for _, key := range keys {
		if key == "inactive-registry-test-provider" {
			t.Fatalf("inactive provider should be hidden from registered keys: %+v", keys)
		}
	}
}

type registryTestProvider struct{ name string }

func (p registryTestProvider) Name() string { return p.name }

func (p registryTestProvider) Chat(ctx context.Context, model string, messages []Message, tools []ToolDefinition, opts Options) (*Response, error) {
	return &Response{Content: "ok", FinishReason: "stop"}, nil
}
