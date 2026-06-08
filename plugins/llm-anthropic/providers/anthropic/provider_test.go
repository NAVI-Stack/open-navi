package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/open-navi/navi/internal/llm"
)

// ---------------------------------------------------------------------------
// test helpers
// ---------------------------------------------------------------------------

func newTestProvider(t *testing.T, handler http.Handler) *Provider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(Config{APIKey: "test-key", BaseURL: srv.URL})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// messagesHandler returns an HTTP handler for /messages that responds with
// the given content blocks, stop reason, and token counts.
func messagesHandler(content []map[string]any, stopReason string, inTok, outTok int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"id":          "msg_test",
			"type":        "message",
			"content":     content,
			"stop_reason": stopReason,
			"usage":       map[string]any{"input_tokens": inTok, "output_tokens": outTok},
		})
	}
}

// ---------------------------------------------------------------------------
// Chat — happy path
// ---------------------------------------------------------------------------

func TestChat_HappyPath(t *testing.T) {
	p := newTestProvider(t, messagesHandler(
		[]map[string]any{{"type": "text", "text": "Hello from Claude!"}},
		"end_turn", 15, 8,
	))

	resp, err := p.Chat(context.Background(), "claude-sonnet-4-20250514", []llm.Message{
		{Role: "user", Content: "Hi"},
	}, nil, llm.Options{MaxTokens: 100})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello from Claude!" {
		t.Errorf("expected 'Hello from Claude!', got %q", resp.Content)
	}
	if resp.InputTokens != 15 {
		t.Errorf("expected 15 input tokens, got %d", resp.InputTokens)
	}
	if resp.OutputTokens != 8 {
		t.Errorf("expected 8 output tokens, got %d", resp.OutputTokens)
	}
	if resp.FinishReason != "end_turn" {
		t.Errorf("expected finish_reason 'end_turn', got %q", resp.FinishReason)
	}
}

// ---------------------------------------------------------------------------
// Chat — tool call response
// ---------------------------------------------------------------------------

func TestChat_ToolCallResponse(t *testing.T) {
	p := newTestProvider(t, messagesHandler(
		[]map[string]any{
			{"type": "text", "text": "Let me check the weather."},
			{
				"type":  "tool_use",
				"id":    "toolu_01",
				"name":  "get_weather",
				"input": map[string]any{"city": "NYC"},
			},
		},
		"tool_use", 20, 15,
	))

	resp, err := p.Chat(context.Background(), "claude-sonnet-4-20250514", []llm.Message{
		{Role: "user", Content: "Weather in NYC?"},
	}, nil, llm.Options{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Let me check the weather." {
		t.Errorf("expected text content, got %q", resp.Content)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "get_weather" {
		t.Errorf("expected get_weather, got %s", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].Arguments["city"] != "NYC" {
		t.Errorf("expected NYC, got %v", resp.ToolCalls[0].Arguments["city"])
	}
}

// ---------------------------------------------------------------------------
// Chat — system message extraction
// ---------------------------------------------------------------------------

func TestChat_SystemExtraction(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)

		if req["system"] != "You are a helpful assistant." {
			t.Errorf("system field missing or wrong: %v", req["system"])
		}

		msgs := req["messages"].([]any)
		for _, m := range msgs {
			msg := m.(map[string]any)
			if msg["role"] == "system" {
				t.Error("system message should not be in messages array")
			}
		}

		writeJSON(w, map[string]any{
			"content":     []map[string]any{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))

	_, err := p.Chat(context.Background(), "claude-sonnet-4-20250514", []llm.Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "Hi"},
	}, nil, llm.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Chat — auth headers
// ---------------------------------------------------------------------------

func TestChat_AuthHeaders(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if key := r.Header.Get("x-api-key"); key != "test-key" {
			t.Errorf("expected x-api-key 'test-key', got %q", key)
		}
		if ver := r.Header.Get("anthropic-version"); ver != "2023-06-01" {
			t.Errorf("expected anthropic-version '2023-06-01', got %q", ver)
		}

		writeJSON(w, map[string]any{
			"content":     []map[string]any{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))

	_, err := p.Chat(context.Background(), "claude-sonnet-4-20250514", []llm.Message{
		{Role: "user", Content: "Hi"},
	}, nil, llm.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Chat — tool choice routing
// ---------------------------------------------------------------------------

func TestChat_ToolChoice(t *testing.T) {
	cases := []struct {
		choice   string
		expected any // nil means tool_choice should be absent
	}{
		{"auto", map[string]any{"type": "auto"}},
		{"required", map[string]any{"type": "any"}},
		{"any", map[string]any{"type": "any"}},
		{"none", nil},
		{"get_weather", map[string]any{"type": "tool", "name": "get_weather"}},
	}

	for _, tc := range cases {
		t.Run(tc.choice, func(t *testing.T) {
			p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var req map[string]any
				json.Unmarshal(body, &req)

				toolChoice := req["tool_choice"]
				if tc.expected == nil {
					if toolChoice != nil {
						t.Errorf("expected no tool_choice, got %v", toolChoice)
					}
				} else {
					expected := tc.expected.(map[string]any)
					actual, ok := toolChoice.(map[string]any)
					if !ok {
						t.Errorf("expected map tool_choice, got %T", toolChoice)
					} else {
						if actual["type"] != expected["type"] {
							t.Errorf("expected type %v, got %v", expected["type"], actual["type"])
						}
					}
				}

				writeJSON(w, map[string]any{
					"content":     []map[string]any{{"type": "text", "text": "ok"}},
					"stop_reason": "end_turn",
					"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1},
				})
			}))

			tools := []llm.ToolDefinition{{Name: "get_weather", Description: "weather", Parameters: map[string]any{"type": "object"}}}
			_, err := p.Chat(context.Background(), "claude-sonnet-4-20250514", []llm.Message{
				{Role: "user", Content: "Hi"},
			}, tools, llm.Options{ToolChoice: tc.choice})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Chat — vision image blocks
// ---------------------------------------------------------------------------

func TestChat_VisionImageBlocks(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)

		msgs := req["messages"].([]any)
		userMsg := msgs[0].(map[string]any)
		content := userMsg["content"]

		// Should be a content block array, not a string.
		blocks, ok := content.([]any)
		if !ok {
			t.Fatalf("expected content block array, got %T", content)
		}
		if len(blocks) != 1 {
			t.Fatalf("expected 1 block, got %d", len(blocks))
		}
		block := blocks[0].(map[string]any)
		if block["type"] != "image" {
			t.Errorf("expected image block, got %v", block["type"])
		}
		source := block["source"].(map[string]any)
		if source["type"] != "base64" {
			t.Errorf("expected base64 source, got %v", source["type"])
		}
		if source["data"] != "abc123" {
			t.Errorf("expected data 'abc123', got %v", source["data"])
		}

		writeJSON(w, map[string]any{
			"content":     []map[string]any{{"type": "text", "text": "image received"}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))

	resp, err := p.Chat(context.Background(), "claude-sonnet-4-20250514", []llm.Message{
		{Role: "user", Content: "data:image/png;base64,abc123"},
	}, nil, llm.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "image received" {
		t.Errorf("got %q", resp.Content)
	}
}

// ---------------------------------------------------------------------------
// ChatStream — text deltas
// ---------------------------------------------------------------------------

func TestChatStream_TextDeltas(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, `event: message_start`)
		fmt.Fprintln(w, `data: {"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":10}}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `event: content_block_start`)
		fmt.Fprintln(w, `data: {"type":"content_block_start","index":0,"content_block":{"type":"text"}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `event: content_block_delta`)
		fmt.Fprintln(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `event: content_block_delta`)
		fmt.Fprintln(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `event: content_block_stop`)
		fmt.Fprintln(w, `data: {"type":"content_block_stop","index":0}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `event: message_delta`)
		fmt.Fprintln(w, `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `event: message_stop`)
		fmt.Fprintln(w, `data: {"type":"message_stop"}`)
	}))

	var received []string
	resp, err := p.ChatStream(context.Background(), "claude-sonnet-4-20250514", []llm.Message{
		{Role: "user", Content: "Hi"},
	}, nil, llm.Options{}, func(delta string) {
		received = append(received, delta)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", resp.Content)
	}
	if resp.InputTokens != 10 {
		t.Errorf("expected 10 input tokens, got %d", resp.InputTokens)
	}
	if resp.OutputTokens != 5 {
		t.Errorf("expected 5 output tokens, got %d", resp.OutputTokens)
	}
	if strings.Join(received, "") != "Hello world" {
		t.Errorf("onChunk deltas don't match: %v", received)
	}
}

// ---------------------------------------------------------------------------
// ChatStream — tool calls assembled
// ---------------------------------------------------------------------------

func TestChatStream_ToolCallsAssembled(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, `event: message_start`)
		fmt.Fprintln(w, `data: {"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":5}}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `event: content_block_start`)
		fmt.Fprintln(w, `data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_01","name":"get_weather"}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `event: content_block_delta`)
		fmt.Fprintln(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `event: content_block_delta`)
		fmt.Fprintln(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"Paris\"}"}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `event: content_block_stop`)
		fmt.Fprintln(w, `data: {"type":"content_block_stop","index":0}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `event: message_delta`)
		fmt.Fprintln(w, `data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":10}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `event: message_stop`)
		fmt.Fprintln(w, `data: {"type":"message_stop"}`)
	}))

	resp, err := p.ChatStream(context.Background(), "claude-sonnet-4-20250514", []llm.Message{
		{Role: "user", Content: "Weather in Paris?"},
	}, []llm.ToolDefinition{{Name: "get_weather", Description: "weather", Parameters: map[string]any{}}},
		llm.Options{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "get_weather" {
		t.Errorf("expected 'get_weather', got %q", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].ID != "toolu_01" {
		t.Errorf("expected ID 'toolu_01', got %q", resp.ToolCalls[0].ID)
	}
	if resp.ToolCalls[0].Arguments["city"] != "Paris" {
		t.Errorf("expected city=Paris, got %v", resp.ToolCalls[0].Arguments)
	}
}

// ---------------------------------------------------------------------------
// ChatStream — thinking accumulated (NOT forwarded)
// ---------------------------------------------------------------------------

func TestChatStream_ThinkingAccumulated(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, `event: message_start`)
		fmt.Fprintln(w, `data: {"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":5}}}`)
		fmt.Fprintln(w)
		// Thinking block.
		fmt.Fprintln(w, `event: content_block_start`)
		fmt.Fprintln(w, `data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `event: content_block_delta`)
		fmt.Fprintln(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"let me reason"}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `event: content_block_stop`)
		fmt.Fprintln(w, `data: {"type":"content_block_stop","index":0}`)
		fmt.Fprintln(w)
		// Text block.
		fmt.Fprintln(w, `event: content_block_start`)
		fmt.Fprintln(w, `data: {"type":"content_block_start","index":1,"content_block":{"type":"text"}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `event: content_block_delta`)
		fmt.Fprintln(w, `data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"the answer"}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `event: content_block_stop`)
		fmt.Fprintln(w, `data: {"type":"content_block_stop","index":1}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `event: message_delta`)
		fmt.Fprintln(w, `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":8}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `event: message_stop`)
		fmt.Fprintln(w, `data: {"type":"message_stop"}`)
	}))

	var textDeltas []string
	resp, err := p.ChatStream(context.Background(), "claude-opus-4-20250514", []llm.Message{
		{Role: "user", Content: "Think about this"},
	}, nil, llm.Options{MaxTokens: 4096}, func(delta string) {
		textDeltas = append(textDeltas, delta)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "the answer" {
		t.Errorf("expected 'the answer', got %q", resp.Content)
	}
	// Thinking should NOT appear in text deltas.
	for _, d := range textDeltas {
		if strings.Contains(d, "let me reason") {
			t.Errorf("thinking content leaked into text deltas: %q", d)
		}
	}
}

// ---------------------------------------------------------------------------
// ChatStream — error event
// ---------------------------------------------------------------------------

func TestChatStream_ErrorEvent(t *testing.T) {
	p := newTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, `event: error`)
		fmt.Fprintln(w, `data: {"type":"error","error":{"type":"overloaded_error","message":"API is overloaded"}}`)
	}))

	_, err := p.ChatStream(context.Background(), "claude-sonnet-4-20250514", []llm.Message{
		{Role: "user", Content: "Hi"},
	}, nil, llm.Options{}, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "overloaded") {
		t.Errorf("expected overloaded error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// HealthCheck
// ---------------------------------------------------------------------------

func TestHealthCheck(t *testing.T) {
	p := New(Config{APIKey: "test-key"})
	health, err := p.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !health.Healthy {
		t.Error("expected healthy=true")
	}

	// No key → unhealthy.
	p2 := New(Config{})
	health2, _ := p2.HealthCheck(context.Background())
	if health2.Healthy {
		t.Error("expected healthy=false with no API key")
	}
}

// ---------------------------------------------------------------------------
// ListModels
// ---------------------------------------------------------------------------

func TestListModels(t *testing.T) {
	p := New(Config{APIKey: "test-key"})
	models, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected at least one model in catalog")
	}
	found := false
	for _, m := range models {
		if strings.Contains(m.Name, "claude") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected at least one 'claude' model in catalog")
	}
}

// ---------------------------------------------------------------------------
// Image extraction helpers
// ---------------------------------------------------------------------------

func TestParseDataURI(t *testing.T) {
	src := parseDataURI("data:image/png;base64,abc123")
	if src == nil {
		t.Fatal("expected non-nil source")
	}
	if src.MediaType != "image/png" {
		t.Errorf("expected image/png, got %q", src.MediaType)
	}
	if src.Data != "abc123" {
		t.Errorf("expected abc123, got %q", src.Data)
	}
}

func TestParseDataURI_NotDataURI(t *testing.T) {
	src := parseDataURI("just plain text")
	if src != nil {
		t.Errorf("expected nil for non-data URI, got %+v", src)
	}
}

func TestBuildUserContentBlocks_PlainText(t *testing.T) {
	blocks := buildUserContentBlocks("just plain text")
	if blocks != nil {
		t.Errorf("expected nil for plain text, got %v", blocks)
	}
}

func TestBuildUserContentBlocks_JSONBlocks(t *testing.T) {
	content := `[{"type":"text","text":"hello"},{"type":"image_url","image_url":{"url":"data:image/jpeg;base64,xyz789"}}]`
	blocks := buildUserContentBlocks(content)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].Type != "text" || blocks[0].Text != "hello" {
		t.Errorf("unexpected text block: %+v", blocks[0])
	}
	if blocks[1].Type != "image" || blocks[1].Source == nil {
		t.Errorf("unexpected image block: %+v", blocks[1])
	}
	if blocks[1].Source.Data != "xyz789" {
		t.Errorf("expected xyz789, got %q", blocks[1].Source.Data)
	}
}

// ---------------------------------------------------------------------------
// Convert helpers
// ---------------------------------------------------------------------------

func TestConvertToolChoice(t *testing.T) {
	if v := convertToolChoice(""); v != nil {
		t.Errorf("expected nil for empty, got %v", v)
	}
	if v := convertToolChoice("none"); v != nil {
		t.Errorf("expected nil for none, got %v", v)
	}
	auto := convertToolChoice("auto").(map[string]any)
	if auto["type"] != "auto" {
		t.Errorf("expected auto, got %v", auto)
	}
	req := convertToolChoice("required").(map[string]any)
	if req["type"] != "any" {
		t.Errorf("expected any for required, got %v", req)
	}
	specific := convertToolChoice("my_tool").(map[string]any)
	if specific["type"] != "tool" || specific["name"] != "my_tool" {
		t.Errorf("expected tool/my_tool, got %v", specific)
	}
}

func TestConvertMessages_ToolResult(t *testing.T) {
	system, msgs := convertMessages([]llm.Message{
		{Role: "system", Content: "Be helpful"},
		{Role: "user", Content: "Use the tool"},
		{Role: "assistant", Content: "Using tool", ToolCalls: []llm.ToolCall{{ID: "tc1", Name: "search", Arguments: map[string]any{"q": "test"}}}},
		{Role: "tool", Content: "result data", ToolCallID: "tc1"},
	})

	if system != "Be helpful" {
		t.Errorf("expected system 'Be helpful', got %q", system)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages (user, assistant, tool_result), got %d", len(msgs))
	}

	// Tool result should be "user" role with tool_result content block.
	toolResultMsg := msgs[2]
	if toolResultMsg.Role != "user" {
		t.Errorf("expected user role for tool result, got %q", toolResultMsg.Role)
	}
}
