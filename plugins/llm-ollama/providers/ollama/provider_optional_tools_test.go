package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ceoai/navi/internal/llm"
)

func TestChat_OptionalToolsUnsupportedDoesNotFallback(t *testing.T) {
	tagsCalled := false
	chatModels := make([]string, 0, 1)
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": showHandler([]string{}, 0),
		"/api/tags": func(w http.ResponseWriter, r *http.Request) {
			tagsCalled = true
			writeJSON(w, ollamaTagsResponse{
				Models: []ollamaTagModel{{Name: "tool-capable"}},
			})
		},
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			var req ollamaChatRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			chatModels = append(chatModels, req.Model)
			if len(req.Tools) > 0 {
				t.Errorf("expected optional tools to be omitted for unsupported model, got %d tools", len(req.Tools))
			}
			writeJSON(w, ollamaChatResponse{
				Message:    ollamaMessage{Role: "assistant", Content: "direct"},
				Done:       true,
				DoneReason: "stop",
			})
		},
	})

	tools := []llm.ToolDefinition{{Name: "my_tool", Description: "a tool", Parameters: map[string]any{}}}
	resp, err := p.Chat(context.Background(), "gemma3", []llm.Message{
		{Role: "user", Content: "hello"},
	}, tools, llm.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "direct" {
		t.Fatalf("expected direct response, got %q", resp.Content)
	}
	if tagsCalled {
		t.Fatal("optional tools should not trigger model-list fallback")
	}
	if len(chatModels) != 1 || chatModels[0] != "gemma3" {
		t.Fatalf("expected one chat call to selected model, got %#v", chatModels)
	}
}
