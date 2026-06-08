package llm

import (
	"context"
	"os"
	"testing"

	"github.com/open-navi/navi/internal/config"
)

// TestMain sets up the package-level test environment.
// It wires provider factories to minimal stubs so tests in internal/llm can
// exercise control-plane behavior without importing concrete plugin packages.
func TestMain(m *testing.M) {
	RegisterProviderFactory("ollama", func(cfg *config.LLMConfig, providerKey string) (Provider, error) {
		return &testOllamaStub{}, nil
	})
	for _, key := range []string{"anthropic", "openai", "openrouter"} {
		name := key
		RegisterProviderFactory(name, func(cfg *config.LLMConfig, providerKey string) (Provider, error) {
			return testProvider{name: name}, nil
		})
	}

	os.Exit(m.Run())
}

type testProvider struct{ name string }

func (p testProvider) Name() string { return p.name }

func (p testProvider) Chat(ctx context.Context, model string, messages []Message, tools []ToolDefinition, opts Options) (*Response, error) {
	return &Response{Content: "test", FinishReason: "stop"}, nil
}

// testOllamaStub satisfies LocalLifecycleProvider for unit-test purposes.
// Lifecycle methods are no-ops.
type testOllamaStub struct{}

var _ LocalLifecycleProvider = (*testOllamaStub)(nil)

func (s *testOllamaStub) Name() string { return "ollama" }

func (s *testOllamaStub) Chat(ctx context.Context, model string, messages []Message, tools []ToolDefinition, opts Options) (*Response, error) {
	return &Response{Content: "test", FinishReason: "stop"}, nil
}

func (s *testOllamaStub) ChatStream(ctx context.Context, model string, messages []Message, tools []ToolDefinition, opts Options, onChunk func(string)) (*Response, error) {
	if onChunk != nil {
		onChunk("test")
	}
	return &Response{Content: "test", FinishReason: "stop"}, nil
}

func (s *testOllamaStub) HealthCheck(ctx context.Context) (ProviderHealth, error) {
	return ProviderHealth{Provider: "ollama", Healthy: true}, nil
}

func (s *testOllamaStub) ListModels(ctx context.Context) ([]ModelDescriptor, error) {
	return nil, nil
}

func (s *testOllamaStub) GetModel(ctx context.Context, model string) (ModelDescriptor, error) {
	return ModelDescriptor{Name: model}, nil
}

func (s *testOllamaStub) ListRunningModels(ctx context.Context) ([]RunningModel, error) {
	return nil, nil
}

func (s *testOllamaStub) PullModel(ctx context.Context, model string) (ProviderOperation, error) {
	return ProviderOperation{Status: OperationStatusCompleted}, nil
}

func (s *testOllamaStub) DeleteModel(ctx context.Context, model string) (ProviderOperation, error) {
	return ProviderOperation{Status: OperationStatusCompleted}, nil
}

func (s *testOllamaStub) CopyModel(ctx context.Context, source, target string) (ProviderOperation, error) {
	return ProviderOperation{Status: OperationStatusCompleted}, nil
}

func (s *testOllamaStub) CreateModel(ctx context.Context, req CreateModelRequest) (ProviderOperation, error) {
	return ProviderOperation{Status: OperationStatusCompleted}, nil
}

func (s *testOllamaStub) WarmModel(ctx context.Context, model string) error {
	return nil
}
