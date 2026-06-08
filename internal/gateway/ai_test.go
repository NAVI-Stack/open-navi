package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ceoai/navi/internal/llm"
)

type mockLLMService struct {
	LLMService // Embed to satisfy interface
	chatProv   llm.Provider
}

func (m *mockLLMService) ChatProvider() llm.Provider {
	return m.chatProv
}

func (m *mockLLMService) GetActive(ctx context.Context) (llm.Active, error) {
	return llm.Active{Provider: "mock", Model: "gpt-4"}, nil
}

func TestHandleGenerateConversationTitle(t *testing.T) {
	srv, _, _ := testServer(t)
	
	mockChat := &mockProvider{
		chatFunc: func(ctx context.Context, model string, messages []llm.Message, tools []llm.ToolDefinition, opts llm.Options) (*llm.Response, error) {
			return &llm.Response{
				Content: `{"title": "Test Title", "confidence": 0.9, "reason": "test"}`,
			}, nil
		},
	}
	
	srv.cfg.LLM = &mockLLMService{chatProv: mockChat}

	// 1. Unauthorized request
	res := doJSONReq(t, srv, "POST", "/api/ai/conversations/title", "", "192.168.1.1:1234", map[string]any{
		"messages": []map[string]string{
			{"role": "user", "content": "hello"},
		},
		"mode": "create",
	})
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", res.StatusCode)
	}

	// 2. Authorized loopback request
	res = doJSONReq(t, srv, "POST", "/api/ai/conversations/title", "", "127.0.0.1:1234", map[string]any{
		"messages": []map[string]string{
			{"role": "user", "content": "hello"},
		},
		"mode": "create",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}

	var resp map[string]any
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["title"] != "Test Title" {
		t.Errorf("expected title 'Test Title', got '%s'", resp["title"])
	}
}

// mockProvider repeated here because it's in a different package in the actual implementation
// but for the sake of test simplicity in this file:
type mockProvider struct {
	chatFunc func(ctx context.Context, model string, messages []llm.Message, tools []llm.ToolDefinition, opts llm.Options) (*llm.Response, error)
}

func (m *mockProvider) Chat(ctx context.Context, model string, messages []llm.Message, tools []llm.ToolDefinition, opts llm.Options) (*llm.Response, error) {
	return m.chatFunc(ctx, model, messages, tools, opts)
}

func (m *mockProvider) Name() string {
	return "mock"
}
