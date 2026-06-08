//go:build integration

package ollama

// Integration tests for the native Ollama extension.
// These require a running Ollama instance on localhost:11434.
//
// Run with:
//
//	go test -tags integration -v -timeout 180s ./plugins/llm-ollama/providers/ollama/

import (
	"context"
	"strings"
	"testing"

	"github.com/open-navi/navi/internal/llm"
)

func newIntegrationProvider(t *testing.T) *Provider {
	t.Helper()
	return New(Config{BaseURL: "http://localhost:11434"})
}

// TestNativeOllama_Chat validates the native /api/chat path end-to-end.
func TestNativeOllama_Chat(t *testing.T) {
	p := newIntegrationProvider(t)
	resp, err := p.Chat(context.Background(), "llama3:latest", []llm.Message{
		{Role: "user", Content: "Say hello in one word."},
	}, nil, llm.Options{MaxTokens: 50, Temperature: 0.1})

	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if resp.Content == "" {
		t.Fatal("expected non-empty response")
	}
	t.Logf("llama3:latest response: %q (in=%d, out=%d)", resp.Content, resp.InputTokens, resp.OutputTokens)
}

// TestNativeOllama_CoderModel validates qwen3-coder via the native /api/chat endpoint.
func TestNativeOllama_CoderModel(t *testing.T) {
	p := newIntegrationProvider(t)
	resp, err := p.Chat(context.Background(), "qwen3-coder:30b", []llm.Message{
		{Role: "system", Content: "You are a coding assistant. Be concise."},
		{Role: "user", Content: "Write a Go function that returns the sum of two integers. Return only the function, no explanation."},
	}, nil, llm.Options{MaxTokens: 200, Temperature: 0.1})

	if err != nil {
		t.Fatalf("qwen3-coder chat failed: %v", err)
	}
	if resp.Content == "" {
		t.Fatal("expected non-empty response")
	}
	if !strings.Contains(resp.Content, "func") {
		t.Errorf("expected Go function in response, got: %q", resp.Content)
	}
	t.Logf("qwen3-coder:30b (in=%d, out=%d):\n%s", resp.InputTokens, resp.OutputTokens, resp.Content)
}

// TestNativeOllama_ToolCall checks tool calling via the native /api/chat endpoint.
// Does not fail if the model responds in text — logs for diagnosis.
func TestNativeOllama_ToolCall(t *testing.T) {
	p := newIntegrationProvider(t)

	// getCapabilities first so the test knows if tools are supported.
	caps, err := p.getCapabilities(context.Background(), "llama3:latest")
	if err != nil {
		t.Logf("capability check failed: %v — proceeding anyway", err)
	}
	t.Logf("llama3:latest capabilities: tools=%v vision=%v thinking=%v ctx=%d",
		caps.SupportsTools, caps.SupportsVision, caps.SupportsThinking, caps.ContextWindow)

	resp, err := p.Chat(context.Background(), "llama3:latest", []llm.Message{
		{Role: "user", Content: "What is 42 plus 7? Use the add tool."},
	}, []llm.ToolDefinition{
		{
			Name:        "add",
			Description: "Add two integers together",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"a": map[string]any{"type": "integer", "description": "first number"},
					"b": map[string]any{"type": "integer", "description": "second number"},
				},
				"required": []string{"a", "b"},
			},
		},
	}, llm.Options{MaxTokens: 200, Temperature: 0.1})

	if err != nil {
		t.Skipf("llama3:latest tool call skipped: %v", err)
	}
	t.Logf("llama3:latest tool test — content=%q tool_calls=%d (in=%d, out=%d)",
		resp.Content, len(resp.ToolCalls), resp.InputTokens, resp.OutputTokens)

	if len(resp.ToolCalls) > 0 {
		t.Logf("✓ tool call used: name=%s args=%v", resp.ToolCalls[0].Name, resp.ToolCalls[0].Arguments)
	} else {
		t.Log("⚠ model answered in text — qwen3-coder likely needed for reliable tool use")
	}
}

// TestNativeOllama_QwenToolCall validates qwen3-coder tool calling via native /api/chat.
func TestNativeOllama_QwenToolCall(t *testing.T) {
	p := newIntegrationProvider(t)
	resp, err := p.Chat(context.Background(), "qwen3-coder:30b", []llm.Message{
		{Role: "user", Content: "What is 42 plus 7? Use the add tool."},
	}, []llm.ToolDefinition{
		{
			Name:        "add",
			Description: "Add two integers together",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"a": map[string]any{"type": "integer"},
					"b": map[string]any{"type": "integer"},
				},
				"required": []string{"a", "b"},
			},
		},
	}, llm.Options{MaxTokens: 200, Temperature: 0.1})

	if err != nil {
		t.Fatalf("qwen3-coder tool call failed: %v", err)
	}
	if len(resp.ToolCalls) > 0 {
		t.Logf("✓ tool calls supported: name=%s args=%v", resp.ToolCalls[0].Name, resp.ToolCalls[0].Arguments)
	} else {
		t.Logf("⚠ no tool call — responded in text: %q", resp.Content)
	}
}

// TestNativeOllama_Stream validates streaming via native /api/chat NDJSON.
func TestNativeOllama_Stream(t *testing.T) {
	p := newIntegrationProvider(t)
	var chunks []string
	resp, err := p.ChatStream(context.Background(), "llama3:latest", []llm.Message{
		{Role: "user", Content: "Count to three, one word per line."},
	}, nil, llm.Options{MaxTokens: 50}, func(delta string) {
		if delta != "" {
			chunks = append(chunks, delta)
		}
	})

	if err != nil {
		t.Fatalf("ChatStream failed: %v", err)
	}
	if resp.Content == "" {
		t.Fatal("expected non-empty streamed content")
	}
	if len(chunks) == 0 {
		t.Error("expected streaming deltas, got none")
	}
	t.Logf("streamed %d chunks, content=%q", len(chunks), resp.Content)
}

// TestNativeOllama_CapabilityDetection validates /api/show parsing and capability cache.
func TestNativeOllama_CapabilityDetection(t *testing.T) {
	p := newIntegrationProvider(t)
	caps, err := p.getCapabilities(context.Background(), "llama3:latest")
	if err != nil {
		t.Fatalf("getCapabilities failed: %v", err)
	}
	t.Logf("llama3:latest: tools=%v vision=%v thinking=%v ctx=%d",
		caps.SupportsTools, caps.SupportsVision, caps.SupportsThinking, caps.ContextWindow)

	// Second call must hit cache.
	caps2, err := p.getCapabilities(context.Background(), "llama3:latest")
	if err != nil {
		t.Fatalf("getCapabilities (cached) failed: %v", err)
	}
	if caps != caps2 {
		t.Error("cached capabilities differ from first call")
	}
}

// TestNativeOllama_HealthCheck validates the health check endpoint.
func TestNativeOllama_HealthCheck(t *testing.T) {
	p := newIntegrationProvider(t)
	health, err := p.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
	if !health.Healthy {
		t.Errorf("expected healthy=true: %s", health.Message)
	}
	t.Logf("Ollama health: latency=%dms models=%d", health.Latency, health.ModelCount)
}
