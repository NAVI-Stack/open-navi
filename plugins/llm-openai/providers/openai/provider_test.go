package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ceoai/navi/internal/llm"
)

// newTestProvider returns a Provider pointing at the given httptest server.
func newTestProvider(ts *httptest.Server) *Provider {
	return New(Config{
		Name:    "openai",
		APIKey:  "test-key-123",
		BaseURL: ts.URL,
	})
}

// serveChatResponse returns a handler that serves a static chat completion.
func serveChatResponse(content string, toolCalls []oaiToolCall) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := chatResponse{
			Choices: []chatChoice{{
				Message:      chatChoiceMessage{Role: "assistant", Content: content, ToolCalls: toolCalls},
				FinishReason: "stop",
			}},
			Usage: chatUsage{PromptTokens: 10, CompletionTokens: 20},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// serveSSE returns a handler that serves SSE events from the given lines.
func serveSSE(events []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, ev := range events {
			fmt.Fprintf(w, "data: %s\n\n", ev)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}
}

// ---------- Non-streaming tests ----------

func TestChat_HappyPath(t *testing.T) {
	ts := httptest.NewServer(serveChatResponse("Hello from GPT!", nil))
	defer ts.Close()

	p := newTestProvider(ts)
	resp, err := p.Chat(context.Background(), "gpt-4o", nil, nil, llm.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello from GPT!" {
		t.Errorf("content = %q, want %q", resp.Content, "Hello from GPT!")
	}
	if resp.InputTokens != 10 || resp.OutputTokens != 20 {
		t.Errorf("tokens = (%d, %d), want (10, 20)", resp.InputTokens, resp.OutputTokens)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("finish_reason = %q, want %q", resp.FinishReason, "stop")
	}
}

func TestChat_ToolCallResponse(t *testing.T) {
	toolCalls := []oaiToolCall{{
		ID:   "call_abc",
		Type: "function",
		Function: oaiToolCallFunc{
			Name:      "get_weather",
			Arguments: `{"city":"London"}`,
		},
	}}
	ts := httptest.NewServer(serveChatResponse("", toolCalls))
	defer ts.Close()

	p := newTestProvider(ts)
	resp, err := p.Chat(context.Background(), "gpt-4o", nil, nil, llm.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "call_abc" {
		t.Errorf("tool call ID = %q, want %q", tc.ID, "call_abc")
	}
	if tc.Name != "get_weather" {
		t.Errorf("tool call name = %q, want %q", tc.Name, "get_weather")
	}
	if tc.Arguments["city"] != "London" {
		t.Errorf("tool call args = %v, want city=London", tc.Arguments)
	}
}

func TestChat_AuthHeaders(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key-123" {
			t.Errorf("Authorization = %q, want %q", auth, "Bearer test-key-123")
		}
		ct := r.Header.Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("Content-Type = %q, want %q", ct, "application/json")
		}
		serveChatResponse("ok", nil)(w, r)
	}))
	defer ts.Close()

	p := newTestProvider(ts)
	_, err := p.Chat(context.Background(), "gpt-4o", nil, nil, llm.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChat_ToolChoice(t *testing.T) {
	tests := []struct {
		name     string
		choice   string
		wantType string // "string" or "object"
		wantVal  string // the string value, or function name for object
	}{
		{"auto", "auto", "string", "auto"},
		{"none", "none", "string", "none"},
		{"required", "required", "string", "required"},
		{"specific", "get_weather", "object", "get_weather"},
		{"empty", "", "nil", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var req map[string]any
				json.Unmarshal(body, &req)

				tc := req["tool_choice"]
				switch tt.wantType {
				case "nil":
					if tc != nil {
						t.Errorf("tool_choice = %v, want nil", tc)
					}
				case "string":
					if tc != tt.wantVal {
						t.Errorf("tool_choice = %v, want %q", tc, tt.wantVal)
					}
				case "object":
					obj, ok := tc.(map[string]any)
					if !ok {
						t.Fatalf("tool_choice is not an object: %T", tc)
					}
					fn := obj["function"].(map[string]any)
					if fn["name"] != tt.wantVal {
						t.Errorf("tool_choice.function.name = %v, want %q", fn["name"], tt.wantVal)
					}
				}
				serveChatResponse("ok", nil)(w, r)
			}))
			defer ts.Close()

			p := newTestProvider(ts)
			tools := []llm.ToolDefinition{{Name: "get_weather", Description: "Get weather", Parameters: map[string]any{"type": "object"}}}
			_, err := p.Chat(context.Background(), "gpt-4o", nil, tools, llm.Options{ToolChoice: tt.choice})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestChat_VisionImageBlocks(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)

		msgs := req["messages"].([]any)
		msg := msgs[0].(map[string]any)
		content := msg["content"].([]any)
		if len(content) != 1 {
			t.Fatalf("content blocks = %d, want 1", len(content))
		}
		block := content[0].(map[string]any)
		if block["type"] != "image_url" {
			t.Errorf("block type = %v, want image_url", block["type"])
		}
		imgURL := block["image_url"].(map[string]any)
		if !strings.HasPrefix(imgURL["url"].(string), "data:image/png;base64,") {
			t.Error("image URL does not start with data:image/png;base64,")
		}

		serveChatResponse("I see an image", nil)(w, r)
	}))
	defer ts.Close()

	p := newTestProvider(ts)
	msgs := []llm.Message{{
		Role:    "user",
		Content: "data:image/png;base64,iVBORw0KGgo=",
	}}
	_, err := p.Chat(context.Background(), "gpt-4o", msgs, nil, llm.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChat_ReasoningModel(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)

		// Should use max_completion_tokens, not max_tokens.
		if _, ok := req["max_tokens"]; ok {
			t.Error("reasoning model should not have max_tokens")
		}
		if mct, ok := req["max_completion_tokens"]; !ok || mct != float64(1000) {
			t.Errorf("max_completion_tokens = %v, want 1000", mct)
		}

		// Should not have temperature.
		if _, ok := req["temperature"]; ok {
			t.Error("reasoning model should not have temperature")
		}

		// System messages should be converted to developer role.
		msgs := req["messages"].([]any)
		firstMsg := msgs[0].(map[string]any)
		if firstMsg["role"] != "developer" {
			t.Errorf("first message role = %v, want developer", firstMsg["role"])
		}

		serveChatResponse("reasoning output", nil)(w, r)
	}))
	defer ts.Close()

	p := newTestProvider(ts)
	msgs := []llm.Message{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Think hard."},
	}
	_, err := p.Chat(context.Background(), "o3", msgs, nil, llm.Options{
		MaxTokens:   1000,
		Temperature: 0.7,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChat_OpenRouterKeepsPrefix(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)

		if req["model"] != "anthropic/claude-3.5-haiku" {
			t.Errorf("model = %v, want anthropic/claude-3.5-haiku", req["model"])
		}
		serveChatResponse("ok", nil)(w, r)
	}))
	defer ts.Close()

	p := New(Config{
		Name:    "openrouter",
		APIKey:  "test-key",
		BaseURL: ts.URL,
	})
	_, err := p.Chat(context.Background(), "anthropic/claude-3.5-haiku", nil, nil, llm.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------- Streaming tests ----------

func TestChatStream_TextDeltas(t *testing.T) {
	events := []string{
		`{"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}`,
		`{"choices":[{"delta":{"content":" world"},"finish_reason":null}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":10}}`,
	}
	ts := httptest.NewServer(serveSSE(events))
	defer ts.Close()

	p := newTestProvider(ts)
	var chunks []string
	resp, err := p.ChatStream(context.Background(), "gpt-4o", nil, nil, llm.Options{}, func(delta string) {
		chunks = append(chunks, delta)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello world" {
		t.Errorf("content = %q, want %q", resp.Content, "Hello world")
	}
	if len(chunks) != 2 {
		t.Errorf("chunks = %d, want 2", len(chunks))
	}
	if resp.InputTokens != 5 || resp.OutputTokens != 10 {
		t.Errorf("tokens = (%d, %d), want (5, 10)", resp.InputTokens, resp.OutputTokens)
	}
}

func TestChatStream_ToolCallsAssembled(t *testing.T) {
	events := []string{
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"ci"}}]},"finish_reason":null}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ty\":\"London\"}"}}]},"finish_reason":null}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
	}
	ts := httptest.NewServer(serveSSE(events))
	defer ts.Close()

	p := newTestProvider(ts)
	resp, err := p.ChatStream(context.Background(), "gpt-4o", nil, nil, llm.Options{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "call_1" || tc.Name != "get_weather" {
		t.Errorf("tool call = {ID: %q, Name: %q}, want {call_1, get_weather}", tc.ID, tc.Name)
	}
	if tc.Arguments["city"] != "London" {
		t.Errorf("args = %v, want city=London", tc.Arguments)
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("finish_reason = %q, want %q", resp.FinishReason, "tool_calls")
	}
}

func TestChatStream_ContextCancel(t *testing.T) {
	// Server that blocks forever.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer ts.Close()

	p := newTestProvider(ts)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := p.ChatStream(ctx, "gpt-4o", nil, nil, llm.Options{}, nil)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

// ---------- ManagedProvider tests ----------

func TestHealthCheck(t *testing.T) {
	p := New(Config{APIKey: "sk-test"})
	h, err := p.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.Healthy {
		t.Error("expected healthy when API key set")
	}

	p2 := New(Config{})
	h2, _ := p2.HealthCheck(context.Background())
	if h2.Healthy {
		t.Error("expected unhealthy when no API key")
	}
}

func TestListModels(t *testing.T) {
	p := New(Config{APIKey: "sk-test"})
	models, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != len(knownOpenAIModels) {
		t.Errorf("models = %d, want %d", len(models), len(knownOpenAIModels))
	}
	// Verify it's a copy, not a reference.
	models[0].Name = "mutated"
	if knownOpenAIModels[0].Name == "mutated" {
		t.Error("ListModels returned a reference, not a copy")
	}
}

// ---------- convert.go unit tests ----------

func TestIsReasoningModel(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"o1", true},
		{"o1-mini", true},
		{"o3", true},
		{"o3-mini", true},
		{"o4-mini", true},
		{"gpt-4o", false},
		{"gpt-4o-mini", false},
		{"gpt-4-turbo", false},
	}
	for _, tt := range tests {
		if got := isReasoningModel(tt.model); got != tt.want {
			t.Errorf("isReasoningModel(%q) = %v, want %v", tt.model, got, tt.want)
		}
	}
}

func TestNormalizeModel(t *testing.T) {
	tests := []struct {
		model   string
		baseURL string
		want    string
	}{
		{"openai/gpt-4o", "https://api.openai.com/v1", "gpt-4o"},
		{"ollama/llama3", "https://api.openai.com/v1", "llama3"},
		{"anthropic/claude-3.5-haiku", "https://openrouter.ai/api/v1", "anthropic/claude-3.5-haiku"},
		{"gpt-4o", "https://api.openai.com/v1", "gpt-4o"},
	}
	for _, tt := range tests {
		if got := normalizeModel(tt.model, tt.baseURL); got != tt.want {
			t.Errorf("normalizeModel(%q, %q) = %q, want %q", tt.model, tt.baseURL, got, tt.want)
		}
	}
}

func TestBuildImageContent_PlainText(t *testing.T) {
	if blocks := buildImageContent("Hello world"); blocks != nil {
		t.Errorf("expected nil for plain text, got %v", blocks)
	}
}

func TestBuildImageContent_DataURI(t *testing.T) {
	blocks := buildImageContent("data:image/png;base64,abc123")
	if len(blocks) != 1 {
		t.Fatalf("blocks = %d, want 1", len(blocks))
	}
	if blocks[0].Type != "image_url" {
		t.Errorf("type = %q, want image_url", blocks[0].Type)
	}
}

func TestBuildImageContent_JSONBlocks(t *testing.T) {
	input := `[{"type":"text","text":"What is this?"},{"type":"image_url","image_url":{"url":"data:image/png;base64,abc"}}]`
	blocks := buildImageContent(input)
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(blocks))
	}
	if blocks[0].Type != "text" || blocks[0].Text != "What is this?" {
		t.Errorf("block[0] = {%q, %q}, want {text, What is this?}", blocks[0].Type, blocks[0].Text)
	}
	if blocks[1].Type != "image_url" {
		t.Errorf("block[1].type = %q, want image_url", blocks[1].Type)
	}
}

func TestConvertToolChoice(t *testing.T) {
	tests := []struct {
		input string
		isNil bool
		isStr bool
	}{
		{"", true, false},
		{"auto", false, true},
		{"none", false, true},
		{"required", false, true},
		{"get_weather", false, false},
	}
	for _, tt := range tests {
		result := convertToolChoice(tt.input)
		if tt.isNil && result != nil {
			t.Errorf("convertToolChoice(%q) = %v, want nil", tt.input, result)
		}
		if tt.isStr {
			if s, ok := result.(string); !ok || s != tt.input {
				t.Errorf("convertToolChoice(%q) = %v, want %q", tt.input, result, tt.input)
			}
		}
		if !tt.isNil && !tt.isStr {
			obj, ok := result.(map[string]any)
			if !ok {
				t.Fatalf("convertToolChoice(%q) is not a map", tt.input)
			}
			if obj["type"] != "function" {
				t.Errorf("type = %v, want function", obj["type"])
			}
		}
	}
}
