package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/llm"
)

// ---------------------------------------------------------------------------
// test helpers
// ---------------------------------------------------------------------------

func newTestProvider(t *testing.T, routes map[string]http.HandlerFunc) (*Provider, *httptest.Server) {
	t.Helper()
	mux := http.NewServeMux()
	for path, h := range routes {
		mux.HandleFunc(path, h)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{BaseURL: srv.URL, CapCacheTTL: time.Minute})
	return p, srv
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// showHandler returns an /api/show response advertising the given capabilities
// and context window.
func showHandler(caps []string, contextWindow int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, ollamaShowResponse{
			Capabilities: caps,
			ModelInfo: map[string]any{
				"llama.context_length": float64(contextWindow),
			},
		})
	}
}

// ---------------------------------------------------------------------------
// Chat — happy path
// ---------------------------------------------------------------------------

func TestChat_HappyPath(t *testing.T) {
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": showHandler([]string{"tools"}, 4096),
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			// Verify request shape.
			var req ollamaChatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if req.Stream {
				t.Errorf("expected stream=false, got true")
			}
			if req.Options != nil && req.Options.NumCtx != 0 {
				t.Errorf("expected provider to leave num_ctx on Ollama default, got %v", req.Options)
			}
			writeJSON(w, ollamaChatResponse{
				Message:         ollamaMessage{Role: "assistant", Content: "Hello"},
				Done:            true,
				DoneReason:      "stop",
				PromptEvalCount: 10,
				EvalCount:       5,
			})
		},
	})

	resp, err := p.Chat(context.Background(), "llama3", []llm.Message{
		{Role: "user", Content: "hi"},
	}, nil, llm.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello" {
		t.Errorf("expected 'Hello', got %q", resp.Content)
	}
	if resp.InputTokens != 10 || resp.OutputTokens != 5 {
		t.Errorf("token counts wrong: in=%d out=%d", resp.InputTokens, resp.OutputTokens)
	}
}

func TestChat_ThinkingModelSendsThinkFalseByDefault(t *testing.T) {
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": showHandler([]string{"thinking"}, 0),
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			var req ollamaChatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if req.Think == nil || *req.Think {
				t.Fatalf("expected think=false for thinking-capable chat model, got %v", req.Think)
			}
			writeJSON(w, ollamaChatResponse{
				Message:    ollamaMessage{Role: "assistant", Content: "pong"},
				Done:       true,
				DoneReason: "stop",
			})
		},
	})

	resp, err := p.Chat(context.Background(), "gemma4", []llm.Message{
		{Role: "user", Content: "Reply with exactly: pong"},
	}, nil, llm.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "pong" {
		t.Fatalf("expected pong, got %q", resp.Content)
	}
}

func TestChat_ThinkingOptionCanEnableThinking(t *testing.T) {
	think := true
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": showHandler([]string{"thinking"}, 0),
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			var req ollamaChatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if req.Think == nil || !*req.Think {
				t.Fatalf("expected think=true when explicitly enabled, got %v", req.Think)
			}
			writeJSON(w, ollamaChatResponse{
				Message:    ollamaMessage{Role: "assistant", Content: "pong"},
				Done:       true,
				DoneReason: "stop",
			})
		},
	})

	_, err := p.Chat(context.Background(), "gemma4", []llm.Message{
		{Role: "user", Content: "Reply with exactly: pong"},
	}, nil, llm.Options{Think: &think})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Chat — model not found fallback
// ---------------------------------------------------------------------------

func TestChat_ModelNotFound_Fallback(t *testing.T) {
	callCount := 0
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": showHandler(nil, 0),
		"/api/tags": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, ollamaTagsResponse{
				Models: []ollamaTagModel{{Name: "llama3:latest"}},
			})
		},
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			var req ollamaChatRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			callCount++
			if callCount == 1 {
				// First call: simulate model not found.
				http.Error(w, `{"error":"model not found"}`, http.StatusNotFound)
				return
			}
			// Second call: fallback model succeeds.
			if req.Model != "llama3:latest" {
				t.Errorf("expected fallback model 'llama3:latest', got %q", req.Model)
			}
			writeJSON(w, ollamaChatResponse{
				Message:    ollamaMessage{Role: "assistant", Content: "fallback"},
				Done:       true,
				DoneReason: "stop",
			})
		},
	})

	resp, err := p.Chat(context.Background(), "missing-model", nil, nil, llm.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "fallback" {
		t.Errorf("expected 'fallback', got %q", resp.Content)
	}
	if callCount != 2 {
		t.Errorf("expected 2 chat calls, got %d", callCount)
	}
}

// ---------------------------------------------------------------------------
// Chat — tools omitted when model doesn't support them
// ---------------------------------------------------------------------------

func TestChat_ToolsOmittedWhenUnsupported(t *testing.T) {
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		// No "tools" in capabilities.
		"/api/show": showHandler([]string{}, 0),
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			var req ollamaChatRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if len(req.Tools) > 0 {
				t.Errorf("expected tools to be omitted, but got %d tools", len(req.Tools))
			}
			writeJSON(w, ollamaChatResponse{
				Message:    ollamaMessage{Role: "assistant", Content: "ok"},
				Done:       true,
				DoneReason: "stop",
			})
		},
	})

	tools := []llm.ToolDefinition{{Name: "my_tool", Description: "a tool", Parameters: map[string]any{}}}
	resp, err := p.Chat(context.Background(), "llama3", nil, tools, llm.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("expected 'ok', got %q", resp.Content)
	}
}

// ---------------------------------------------------------------------------
// Chat — vision model extracts images from data URI
// ---------------------------------------------------------------------------

func TestChat_Vision_ImagesExtracted(t *testing.T) {
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": showHandler([]string{"vision"}, 0),
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			var req ollamaChatRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			userMsg := req.Messages[0]
			if len(userMsg.Images) != 1 {
				t.Errorf("expected 1 image, got %d", len(userMsg.Images))
			}
			if userMsg.Images[0] != "abc123" {
				t.Errorf("expected base64 'abc123', got %q", userMsg.Images[0])
			}
			if userMsg.Content != "" {
				t.Errorf("expected empty text after image extraction, got %q", userMsg.Content)
			}
			writeJSON(w, ollamaChatResponse{
				Message:    ollamaMessage{Role: "assistant", Content: "image received"},
				Done:       true,
				DoneReason: "stop",
			})
		},
	})

	resp, err := p.Chat(context.Background(), "llava", []llm.Message{
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
// Chat — thinking extracted from <think> tags
// ---------------------------------------------------------------------------

func TestChat_ThinkingStripped(t *testing.T) {
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": showHandler([]string{}, 0),
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, ollamaChatResponse{
				Message:    ollamaMessage{Role: "assistant", Content: "<think>reasoning</think>answer"},
				Done:       true,
				DoneReason: "stop",
			})
		},
	})

	resp, err := p.Chat(context.Background(), "deepseek-r1", nil, nil, llm.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "answer" {
		t.Errorf("expected 'answer' with <think> stripped, got %q", resp.Content)
	}
}

// ---------------------------------------------------------------------------
// ChatStream — text deltas forwarded via onChunk
// ---------------------------------------------------------------------------

func TestChatStream_TextDeltas(t *testing.T) {
	chunks := []string{"He", "llo", " world"}
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": showHandler(nil, 0),
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/x-ndjson")
			enc := json.NewEncoder(w)
			for i, c := range chunks {
				done := i == len(chunks)-1
				chunk := ollamaStreamChunk{
					Message:    ollamaMessage{Role: "assistant", Content: c},
					Done:       done,
					DoneReason: "stop",
				}
				if done {
					chunk.PromptEvalCount = 3
					chunk.EvalCount = 3
				}
				_ = enc.Encode(chunk)
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
		},
	})

	var received []string
	resp, err := p.ChatStream(context.Background(), "llama3", nil, nil, llm.Options{}, func(delta string) {
		if delta != "" {
			received = append(received, delta)
		}
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", resp.Content)
	}
	if strings.Join(received, "") != "Hello world" {
		t.Errorf("onChunk deltas don't match: %v", received)
	}
}

// ---------------------------------------------------------------------------
// ChatStream — thinking chunks accumulated, NOT forwarded
// ---------------------------------------------------------------------------

func TestChatStream_ThinkingAccumulated(t *testing.T) {
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": showHandler([]string{"thinking"}, 0),
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			enc := json.NewEncoder(w)
			// Thinking delta.
			_ = enc.Encode(ollamaStreamChunk{
				Message: ollamaMessage{Role: "assistant", Thinking: "let me think"},
			})
			// Text delta.
			_ = enc.Encode(ollamaStreamChunk{
				Message: ollamaMessage{Role: "assistant", Content: "the answer"},
			})
			// Done.
			_ = enc.Encode(ollamaStreamChunk{Done: true, DoneReason: "stop"})
		},
	})

	var textDeltas []string
	resp, err := p.ChatStream(context.Background(), "qwen3", nil, nil, llm.Options{}, func(delta string) {
		if delta != "" {
			textDeltas = append(textDeltas, delta)
		}
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "the answer" {
		t.Errorf("expected 'the answer', got %q", resp.Content)
	}
	// Thinking should NOT appear in text deltas.
	for _, d := range textDeltas {
		if strings.Contains(d, "let me think") {
			t.Errorf("thinking content leaked into text deltas: %q", d)
		}
	}
}

// ---------------------------------------------------------------------------
// ChatStream — tool calls assembled from final chunk
// ---------------------------------------------------------------------------

func TestChatStream_ToolCallsAssembled(t *testing.T) {
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": showHandler([]string{"tools"}, 0),
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			enc := json.NewEncoder(w)
			_ = enc.Encode(ollamaStreamChunk{
				Message: ollamaMessage{
					Role: "assistant",
					ToolCalls: []ollamaToolCall{
						{Function: ollamaToolCallFunc{Name: "get_weather", Arguments: map[string]any{"city": "Paris"}}},
					},
				},
				Done:       true,
				DoneReason: "tool_use",
			})
		},
	})

	tools := []llm.ToolDefinition{{Name: "get_weather", Description: "weather", Parameters: map[string]any{}}}
	resp, err := p.ChatStream(context.Background(), "llama3", nil, tools, llm.Options{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "get_weather" {
		t.Errorf("expected 'get_weather', got %q", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].Arguments["city"] != "Paris" {
		t.Errorf("expected city=Paris, got %v", resp.ToolCalls[0].Arguments)
	}
}

// ---------------------------------------------------------------------------
// ChatStream — context cancellation
// ---------------------------------------------------------------------------

func TestChatStream_ContextCancel(t *testing.T) {
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": showHandler(nil, 0),
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			// Server streams infinitely until client disconnects.
			enc := json.NewEncoder(w)
			for i := 0; ; i++ {
				if r.Context().Err() != nil {
					return
				}
				_ = enc.Encode(ollamaStreamChunk{
					Message: ollamaMessage{Content: fmt.Sprintf("chunk%d", i)},
				})
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				time.Sleep(5 * time.Millisecond)
			}
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := p.ChatStream(ctx, "llama3", nil, nil, llm.Options{}, nil)
	if err == nil {
		t.Error("expected error from cancelled context, got nil")
	}
}

// ---------------------------------------------------------------------------
// Capability cache
// ---------------------------------------------------------------------------

func TestCapabilities_ParseExplicit(t *testing.T) {
	show := ollamaShowResponse{
		Capabilities: []string{"vision", "tools", "thinking"},
		ModelInfo:    map[string]any{"llama.context_length": float64(8192)},
	}
	caps := parseCapabilities(show)
	if !caps.SupportsVision {
		t.Error("expected vision=true")
	}
	if !caps.SupportsTools {
		t.Error("expected tools=true")
	}
	if !caps.SupportsThinking {
		t.Error("expected thinking=true")
	}
	if caps.ContextWindow != 8192 {
		t.Errorf("expected context_window=8192, got %d", caps.ContextWindow)
	}
}

func TestCapabilities_ParseFallback(t *testing.T) {
	show := ollamaShowResponse{
		Details:  ollamaModelDetail{Family: "qwen"},
		Template: "before <think> after",
		ModelInfo: map[string]any{
			"clip.vision.image_size": 224,
		},
		Parameters: "num_ctx 2048\ntemperature 0.7",
	}
	caps := parseCapabilities(show)
	if !caps.SupportsVision {
		t.Error("expected vision from clip key")
	}
	if !caps.SupportsThinking {
		t.Error("expected thinking from qwen+<think>")
	}
	if caps.ContextWindow != 2048 {
		t.Errorf("expected context_window=2048 from parameters, got %d", caps.ContextWindow)
	}
}

func TestCapabilities_CacheHit(t *testing.T) {
	showCalls := 0
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": func(w http.ResponseWriter, r *http.Request) {
			showCalls++
			writeJSON(w, ollamaShowResponse{Capabilities: []string{"tools"}})
		},
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, ollamaChatResponse{
				Message:    ollamaMessage{Role: "assistant", Content: "ok"},
				Done:       true,
				DoneReason: "stop",
			})
		},
	})

	for i := 0; i < 3; i++ {
		_, err := p.Chat(context.Background(), "llama3", nil, nil, llm.Options{})
		if err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
	}
	if showCalls != 1 {
		t.Errorf("expected 1 /api/show call (cached), got %d", showCalls)
	}
}

func TestCapabilities_InvalidateAfterDelete(t *testing.T) {
	showCalls := 0
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": func(w http.ResponseWriter, r *http.Request) {
			showCalls++
			writeJSON(w, ollamaShowResponse{})
		},
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, ollamaChatResponse{Message: ollamaMessage{Content: "ok"}, Done: true})
		},
		"/api/delete": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	})

	// First chat: populates cache.
	_, _ = p.Chat(context.Background(), "llama3", nil, nil, llm.Options{})
	// Delete invalidates cache.
	_, _ = p.DeleteModel(context.Background(), "llama3")
	// Second chat: should re-fetch /api/show.
	_, _ = p.Chat(context.Background(), "llama3", nil, nil, llm.Options{})

	if showCalls != 2 {
		t.Errorf("expected 2 /api/show calls after delete, got %d", showCalls)
	}
}

// ---------------------------------------------------------------------------
// HealthCheck
// ---------------------------------------------------------------------------

func TestHealthCheck_Healthy(t *testing.T) {
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/": func(w http.ResponseWriter, r *http.Request) {
			// Only match the root ping, not /api/* routes.
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(http.StatusOK)
		},
		"/api/tags": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, ollamaTagsResponse{
				Models: []ollamaTagModel{{Name: "llama3"}},
			})
		},
	})

	health, err := p.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !health.Healthy {
		t.Errorf("expected healthy=true, got message: %s", health.Message)
	}
	if health.ModelCount != 1 {
		t.Errorf("expected model_count=1, got %d", health.ModelCount)
	}
}

// ---------------------------------------------------------------------------
// ListModels
// ---------------------------------------------------------------------------

func TestListModels(t *testing.T) {
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/tags": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, ollamaTagsResponse{
				Models: []ollamaTagModel{
					{Name: "llama3", Size: 4000000000, Details: ollamaModelDetail{Family: "llama"}},
					{Name: "mistral", Size: 3000000000},
				},
			})
		},
	})

	models, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].Name != "llama3" || models[0].Family != "llama" {
		t.Errorf("unexpected first model: %+v", models[0])
	}
}

// ---------------------------------------------------------------------------
// PullModel progress
// ---------------------------------------------------------------------------

func TestPullModel_Progress(t *testing.T) {
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/pull": func(w http.ResponseWriter, r *http.Request) {
			enc := json.NewEncoder(w)
			_ = enc.Encode(ollamaPullProgress{Status: "pulling", Total: 100, Completed: 50})
			_ = enc.Encode(ollamaPullProgress{Status: "pulling", Total: 100, Completed: 100})
			_ = enc.Encode(ollamaPullProgress{Status: "success"})
		},
	})

	var progresses []float64
	p.SetPullProgressCallback(func(id string, status llm.OperationStatus, progress float64, msg, errMsg string) {
		progresses = append(progresses, progress)
	})

	op, err := p.PullModel(context.Background(), "llama3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if op.Status != llm.OperationStatusRunning {
		t.Errorf("expected status running, got %s", op.Status)
	}

	// Give background goroutine time to finish.
	time.Sleep(100 * time.Millisecond)

	if len(progresses) == 0 {
		t.Error("expected progress callbacks, got none")
	}
}

// ---------------------------------------------------------------------------
// Image extraction helpers
// ---------------------------------------------------------------------------

func TestExtractImages_DataURI(t *testing.T) {
	text, images := extractImages("data:image/png;base64,abc123")
	if text != "" {
		t.Errorf("expected empty text, got %q", text)
	}
	if len(images) != 1 || images[0] != "abc123" {
		t.Errorf("expected ['abc123'], got %v", images)
	}
}

func TestExtractImages_JSONBlocks(t *testing.T) {
	content := `[{"type":"text","text":"hello"},{"type":"image_url","image_url":{"url":"data:image/jpeg;base64,xyz789"}}]`
	text, images := extractImages(content)
	if text != "hello" {
		t.Errorf("expected 'hello', got %q", text)
	}
	if len(images) != 1 || images[0] != "xyz789" {
		t.Errorf("expected ['xyz789'], got %v", images)
	}
}

func TestExtractImages_PlainText(t *testing.T) {
	text, images := extractImages("just plain text")
	if text != "just plain text" {
		t.Errorf("expected passthrough, got %q", text)
	}
	if len(images) != 0 {
		t.Errorf("expected no images, got %v", images)
	}
}

// ---------------------------------------------------------------------------
// Thinking extraction helpers
// ---------------------------------------------------------------------------

func TestExtractThinking_WithTag(t *testing.T) {
	text, thinking := extractThinking("<think>I am reasoning</think>The answer is 42")
	if text != "The answer is 42" {
		t.Errorf("expected 'The answer is 42', got %q", text)
	}
	if thinking != "I am reasoning" {
		t.Errorf("expected 'I am reasoning', got %q", thinking)
	}
}

func TestExtractThinking_NoTag(t *testing.T) {
	text, thinking := extractThinking("plain response")
	if text != "plain response" {
		t.Errorf("expected passthrough, got %q", text)
	}
	if thinking != "" {
		t.Errorf("expected no thinking, got %q", thinking)
	}
}

func TestChat_ToolsRequired_SupportsTools_FailedToCallTools(t *testing.T) {
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": showHandler([]string{"tools"}, 4096),
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, ollamaChatResponse{
				Message: ollamaMessage{Role: "assistant", Content: "Hello without calling tools"},
				Done:    true,
			})
		},
	})

	tools := []llm.ToolDefinition{{Name: "my_tool", Description: "a tool", Parameters: map[string]any{}}}
	_, err := p.Chat(context.Background(), "llama3", []llm.Message{
		{Role: "user", Content: "do something"},
	}, tools, llm.Options{ToolsRequired: true})

	if err == nil {
		t.Fatal("expected error because ToolsRequired was true but model called no tools, got nil")
	}
	if !strings.Contains(err.Error(), "failed to call tools") {
		t.Errorf("expected error message to contain 'failed to call tools', got %q", err.Error())
	}
}

func TestChat_ToolsRequired_SupportsTools_SucceedsWithToolCalls(t *testing.T) {
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": showHandler([]string{"tools"}, 4096),
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, ollamaChatResponse{
				Message: ollamaMessage{
					Role: "assistant",
					ToolCalls: []ollamaToolCall{
						{Function: ollamaToolCallFunc{Name: "my_tool", Arguments: map[string]any{}}},
					},
				},
				Done: true,
			})
		},
	})

	tools := []llm.ToolDefinition{{Name: "my_tool", Description: "a tool", Parameters: map[string]any{}}}
	resp, err := p.Chat(context.Background(), "llama3", []llm.Message{
		{Role: "user", Content: "do something"},
	}, tools, llm.Options{ToolsRequired: true})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) == 0 {
		t.Error("expected tool calls in response, got none")
	}
}

func TestChat_ToolsRequired_SupportsTools_NotStartOfTurn(t *testing.T) {
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": showHandler([]string{"tools"}, 4096),
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, ollamaChatResponse{
				Message: ollamaMessage{Role: "assistant", Content: "done with tool response summary"},
				Done:    true,
			})
		},
	})

	tools := []llm.ToolDefinition{{Name: "my_tool", Description: "a tool", Parameters: map[string]any{}}}
	resp, err := p.Chat(context.Background(), "llama3", []llm.Message{
		{Role: "user", Content: "do something"},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{Name: "my_tool"}}},
		{Role: "tool", Content: "tool response content"},
	}, tools, llm.Options{ToolsRequired: true})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "done with tool response summary" {
		t.Errorf("expected summary content, got %q", resp.Content)
	}
}

func TestChat_ToolsRequired_UnsupportedTools_Fails(t *testing.T) {
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": showHandler(nil, 4096), // no tools capability
	})

	tools := []llm.ToolDefinition{{Name: "my_tool", Description: "a tool", Parameters: map[string]any{}}}
	_, err := p.Chat(context.Background(), "llama3", []llm.Message{
		{Role: "user", Content: "do something"},
	}, tools, llm.Options{ToolsRequired: true})

	if err == nil {
		t.Fatal("expected error because ToolsRequired was true but model doesn't support tools, got nil")
	}
	if !strings.Contains(err.Error(), "does not support tool calling") {
		t.Errorf("expected error message to contain 'does not support tool calling', got %q", err.Error())
	}
}

func TestChat_ToolsRequired_FallbackToToolCapableModel_OnUnsupported(t *testing.T) {
	callCount := 0
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": func(w http.ResponseWriter, r *http.Request) {
			var req ollamaShowRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Name == "llama3-supported" {
				writeJSON(w, ollamaShowResponse{Capabilities: []string{"tools"}})
			} else {
				writeJSON(w, ollamaShowResponse{Capabilities: []string{}})
			}
		},
		"/api/tags": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, ollamaTagsResponse{
				Models: []ollamaTagModel{
					{Name: "llama3-unsupported"},
					{Name: "llama3-supported"},
				},
			})
		},
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			var req ollamaChatRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			callCount++
			if req.Model != "llama3-supported" {
				t.Errorf("expected fallback model 'llama3-supported', got %q", req.Model)
			}
			writeJSON(w, ollamaChatResponse{
				Message: ollamaMessage{
					Role: "assistant",
					ToolCalls: []ollamaToolCall{
						{Function: ollamaToolCallFunc{Name: "my_tool", Arguments: map[string]any{}}},
					},
				},
				Done: true,
			})
		},
	})

	tools := []llm.ToolDefinition{{Name: "my_tool", Description: "a tool", Parameters: map[string]any{}}}
	resp, err := p.Chat(context.Background(), "llama3-unsupported", []llm.Message{
		{Role: "user", Content: "do something"},
	}, tools, llm.Options{ToolsRequired: true})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) == 0 {
		t.Error("expected tool calls in fallback response, got none")
	}
	if callCount != 1 {
		t.Errorf("expected 1 call to fallback model, got %d", callCount)
	}
}

func TestChat_ToolsRequired_FallbackToToolCapableModel_OnFailureToCallTools(t *testing.T) {
	callCount := 0
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, ollamaShowResponse{Capabilities: []string{"tools"}})
		},
		"/api/tags": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, ollamaTagsResponse{
				Models: []ollamaTagModel{
					{Name: "llama3-unsupported"},
					{Name: "llama3-supported"},
				},
			})
		},
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			var req ollamaChatRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			callCount++
			if req.Model == "llama3-unsupported" {
				// Return no tool calls
				writeJSON(w, ollamaChatResponse{
					Message: ollamaMessage{Role: "assistant", Content: "Plain response"},
					Done:    true,
				})
				return
			}
			if req.Model != "llama3-supported" {
				t.Errorf("expected fallback model 'llama3-supported', got %q", req.Model)
			}
			writeJSON(w, ollamaChatResponse{
				Message: ollamaMessage{
					Role: "assistant",
					ToolCalls: []ollamaToolCall{
						{Function: ollamaToolCallFunc{Name: "my_tool", Arguments: map[string]any{}}},
					},
				},
				Done: true,
			})
		},
	})

	tools := []llm.ToolDefinition{{Name: "my_tool", Description: "a tool", Parameters: map[string]any{}}}
	resp, err := p.Chat(context.Background(), "llama3-unsupported", []llm.Message{
		{Role: "user", Content: "do something"},
	}, tools, llm.Options{ToolsRequired: true})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) == 0 {
		t.Error("expected tool calls in fallback response, got none")
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls (1 original + 1 fallback), got %d", callCount)
	}
}

func TestChatStream_ToolsRequired_FallbackToToolCapableModel_OnUnsupported(t *testing.T) {
	callCount := 0
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": func(w http.ResponseWriter, r *http.Request) {
			var req ollamaShowRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Name == "llama3-supported" {
				writeJSON(w, ollamaShowResponse{Capabilities: []string{"tools"}})
			} else {
				writeJSON(w, ollamaShowResponse{Capabilities: []string{}})
			}
		},
		"/api/tags": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, ollamaTagsResponse{
				Models: []ollamaTagModel{
					{Name: "llama3-unsupported"},
					{Name: "llama3-supported"},
				},
			})
		},
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			var req ollamaChatRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			callCount++
			if req.Model != "llama3-supported" {
				t.Errorf("expected fallback model 'llama3-supported', got %q", req.Model)
			}
			writeJSON(w, ollamaChatResponse{
				Message: ollamaMessage{
					Role: "assistant",
					ToolCalls: []ollamaToolCall{
						{Function: ollamaToolCallFunc{Name: "my_tool", Arguments: map[string]any{}}},
					},
				},
				Done: true,
			})
		},
	})

	tools := []llm.ToolDefinition{{Name: "my_tool", Description: "a tool", Parameters: map[string]any{}}}
	resp, err := p.ChatStream(context.Background(), "llama3-unsupported", []llm.Message{
		{Role: "user", Content: "do something"},
	}, tools, llm.Options{ToolsRequired: true}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) == 0 {
		t.Error("expected tool calls in fallback response, got none")
	}
	if callCount != 1 {
		t.Errorf("expected 1 call to fallback model, got %d", callCount)
	}
}

func TestChat_ToolCallingRequired_FallbackToToolCapableModel_OnUnsupported(t *testing.T) {
	callCount := 0
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": func(w http.ResponseWriter, r *http.Request) {
			var req ollamaShowRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Name == "llama3-supported" {
				writeJSON(w, ollamaShowResponse{Capabilities: []string{"tools"}})
			} else {
				writeJSON(w, ollamaShowResponse{Capabilities: []string{}})
			}
		},
		"/api/tags": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, ollamaTagsResponse{
				Models: []ollamaTagModel{
					{Name: "llama3-unsupported"},
					{Name: "llama3-supported"},
				},
			})
		},
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			var req ollamaChatRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			callCount++
			if req.Model != "llama3-supported" {
				t.Errorf("expected fallback model 'llama3-supported', got %q", req.Model)
			}
			writeJSON(w, ollamaChatResponse{
				Message: ollamaMessage{
					Role: "assistant",
					ToolCalls: []ollamaToolCall{
						{Function: ollamaToolCallFunc{Name: "my_tool", Arguments: map[string]any{}}},
					},
				},
				Done: true,
			})
		},
	})

	tools := []llm.ToolDefinition{{Name: "my_tool", Description: "a tool", Parameters: map[string]any{}}}
	resp, err := p.Chat(context.Background(), "llama3-unsupported", []llm.Message{
		{Role: "user", Content: "do something"},
	}, tools, llm.Options{ToolCallingRequired: true})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) == 0 {
		t.Error("expected tool calls in fallback response, got none")
	}
	if callCount != 1 {
		t.Errorf("expected 1 call to fallback model, got %d", callCount)
	}
}

func TestChat_ToolCallingRequired_FallbackToToolCapableModel_OnFailureToCallTools(t *testing.T) {
	callCount := 0
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, ollamaShowResponse{Capabilities: []string{"tools"}})
		},
		"/api/tags": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, ollamaTagsResponse{
				Models: []ollamaTagModel{
					{Name: "llama3-unsupported"},
					{Name: "llama3-supported"},
				},
			})
		},
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			var req ollamaChatRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			callCount++
			if req.Model == "llama3-unsupported" {
				// Return no tool calls
				writeJSON(w, ollamaChatResponse{
					Message: ollamaMessage{Role: "assistant", Content: "Plain response"},
					Done:    true,
				})
				return
			}
			if req.Model != "llama3-supported" {
				t.Errorf("expected fallback model 'llama3-supported', got %q", req.Model)
			}
			writeJSON(w, ollamaChatResponse{
				Message: ollamaMessage{
					Role: "assistant",
					ToolCalls: []ollamaToolCall{
						{Function: ollamaToolCallFunc{Name: "my_tool", Arguments: map[string]any{}}},
					},
				},
				Done: true,
			})
		},
	})

	tools := []llm.ToolDefinition{{Name: "my_tool", Description: "a tool", Parameters: map[string]any{}}}
	resp, err := p.Chat(context.Background(), "llama3-unsupported", []llm.Message{
		{Role: "user", Content: "do something"},
	}, tools, llm.Options{ToolCallingRequired: true})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) == 0 {
		t.Error("expected tool calls in fallback response, got none")
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls (1 original + 1 fallback), got %d", callCount)
	}
}

func TestChatStream_ToolCallingRequired_FallbackToToolCapableModel_OnUnsupported(t *testing.T) {
	callCount := 0
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": func(w http.ResponseWriter, r *http.Request) {
			var req ollamaShowRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Name == "llama3-supported" {
				writeJSON(w, ollamaShowResponse{Capabilities: []string{"tools"}})
			} else {
				writeJSON(w, ollamaShowResponse{Capabilities: []string{}})
			}
		},
		"/api/tags": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, ollamaTagsResponse{
				Models: []ollamaTagModel{
					{Name: "llama3-unsupported"},
					{Name: "llama3-supported"},
				},
			})
		},
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			var req ollamaChatRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			callCount++
			if req.Model != "llama3-supported" {
				t.Errorf("expected fallback model 'llama3-supported', got %q", req.Model)
			}
			writeJSON(w, ollamaChatResponse{
				Message: ollamaMessage{
					Role: "assistant",
					ToolCalls: []ollamaToolCall{
						{Function: ollamaToolCallFunc{Name: "my_tool", Arguments: map[string]any{}}},
					},
				},
				Done: true,
			})
		},
	})

	tools := []llm.ToolDefinition{{Name: "my_tool", Description: "a tool", Parameters: map[string]any{}}}
	resp, err := p.ChatStream(context.Background(), "llama3-unsupported", []llm.Message{
		{Role: "user", Content: "do something"},
	}, tools, llm.Options{ToolCallingRequired: true}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) == 0 {
		t.Error("expected tool calls in fallback response, got none")
	}
	if callCount != 1 {
		t.Errorf("expected 1 call to fallback model, got %d", callCount)
	}
}

func TestChatStream_ToolCallingRequired_FallbackToToolCapableModel_OnFailureToCallTools(t *testing.T) {
	callCount := 0
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, ollamaShowResponse{Capabilities: []string{"tools"}})
		},
		"/api/tags": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, ollamaTagsResponse{
				Models: []ollamaTagModel{
					{Name: "llama3-unsupported"},
					{Name: "llama3-supported"},
				},
			})
		},
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			var req ollamaChatRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			callCount++
			if req.Model == "llama3-unsupported" {
				// Return no tool calls
				writeJSON(w, ollamaChatResponse{
					Message: ollamaMessage{Role: "assistant", Content: "Plain response"},
					Done:    true,
				})
				return
			}
			if req.Model != "llama3-supported" {
				t.Errorf("expected fallback model 'llama3-supported', got %q", req.Model)
			}
			writeJSON(w, ollamaChatResponse{
				Message: ollamaMessage{
					Role: "assistant",
					ToolCalls: []ollamaToolCall{
						{Function: ollamaToolCallFunc{Name: "my_tool", Arguments: map[string]any{}}},
					},
				},
				Done: true,
			})
		},
	})

	tools := []llm.ToolDefinition{{Name: "my_tool", Description: "a tool", Parameters: map[string]any{}}}
	resp, err := p.ChatStream(context.Background(), "llama3-unsupported", []llm.Message{
		{Role: "user", Content: "do something"},
	}, tools, llm.Options{ToolCallingRequired: true}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) == 0 {
		t.Error("expected tool calls in fallback response, got none")
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls (1 original + 1 fallback), got %d", callCount)
	}
}

func TestChatStream_ToolCallingRequired_DoesNotEmitProseBeforeFallback(t *testing.T) {
	callCount := 0
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, ollamaShowResponse{Capabilities: []string{"tools"}})
		},
		"/api/tags": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, ollamaTagsResponse{
				Models: []ollamaTagModel{
					{Name: "llama3-unsupported"},
					{Name: "llama3-supported"},
				},
			})
		},
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			var req ollamaChatRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			callCount++
			if req.Model == "llama3-unsupported" {
				enc := json.NewEncoder(w)
				_ = enc.Encode(ollamaStreamChunk{Message: ollamaMessage{Role: "assistant", Content: "I can answer without a tool."}})
				_ = enc.Encode(ollamaStreamChunk{Done: true, DoneReason: "stop"})
				return
			}
			if req.Model != "llama3-supported" {
				t.Errorf("expected fallback model 'llama3-supported', got %q", req.Model)
			}
			enc := json.NewEncoder(w)
			_ = enc.Encode(ollamaStreamChunk{
				Message: ollamaMessage{
					Role: "assistant",
					ToolCalls: []ollamaToolCall{
						{Function: ollamaToolCallFunc{Name: "my_tool", Arguments: map[string]any{}}},
					},
				},
			})
			_ = enc.Encode(ollamaStreamChunk{Done: true, DoneReason: "stop"})
		},
	})

	tools := []llm.ToolDefinition{{Name: "my_tool", Description: "a tool", Parameters: map[string]any{}}}
	var chunks []string
	resp, err := p.ChatStream(context.Background(), "llama3-unsupported", []llm.Message{
		{Role: "user", Content: "do something"},
	}, tools, llm.Options{ToolCallingRequired: true}, func(delta string) {
		if delta != "" {
			chunks = append(chunks, delta)
		}
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) == 0 {
		t.Error("expected tool calls in fallback response, got none")
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls (1 original + 1 fallback), got %d", callCount)
	}
	if len(chunks) != 0 {
		t.Fatalf("required-tool streaming leaked provisional prose before fallback: %q", strings.Join(chunks, ""))
	}
}

func TestChat_TryParseToolCallFromText_JsonBlock(t *testing.T) {
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": showHandler([]string{"tools"}, 4096),
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, ollamaChatResponse{
				Message: ollamaMessage{
					Role:    "assistant",
					Content: "Sure, let's call the tool:\n```json\n{\n  \"name\": \"my_tool\",\n  \"arguments\": {\n    \"param1\": \"value1\"\n  }\n}\n```",
				},
				Done: true,
			})
		},
	})

	tools := []llm.ToolDefinition{{Name: "my_tool", Description: "a tool", Parameters: map[string]any{}}}
	resp, err := p.Chat(context.Background(), "llama3", []llm.Message{
		{Role: "user", Content: "do something"},
	}, tools, llm.Options{ToolsRequired: true})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "my_tool" {
		t.Errorf("expected tool 'my_tool', got %q", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].Arguments["param1"] != "value1" {
		t.Errorf("expected argument 'param1' to be 'value1', got %v", resp.ToolCalls[0].Arguments["param1"])
	}
}

func TestChat_TryParseToolCallFromText_OutermostBraces(t *testing.T) {
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": showHandler([]string{"tools"}, 4096),
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, ollamaChatResponse{
				Message: ollamaMessage{
					Role:    "assistant",
					Content: "Here is the raw call: { \"tool\": \"my_tool\", \"parameters\": { \"key\": \"val\" } }",
				},
				Done: true,
			})
		},
	})

	tools := []llm.ToolDefinition{{Name: "my_tool", Description: "a tool", Parameters: map[string]any{}}}
	resp, err := p.Chat(context.Background(), "llama3", []llm.Message{
		{Role: "user", Content: "do something"},
	}, tools, llm.Options{ToolCallingRequired: true})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "my_tool" {
		t.Errorf("expected tool 'my_tool', got %q", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].Arguments["key"] != "val" {
		t.Errorf("expected argument 'key' to be 'val', got %v", resp.ToolCalls[0].Arguments["key"])
	}
}

func TestChat_TryParseToolCallFromText_SingleToolArgsOnly(t *testing.T) {
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": showHandler([]string{"tools"}, 4096),
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, ollamaChatResponse{
				Message: ollamaMessage{
					Role:    "assistant",
					Content: "Call with: { \"arg1\": \"xyz\" }",
				},
				Done: true,
			})
		},
	})

	tools := []llm.ToolDefinition{{Name: "my_tool", Description: "a tool", Parameters: map[string]any{}}}
	resp, err := p.Chat(context.Background(), "llama3", []llm.Message{
		{Role: "user", Content: "do something"},
	}, tools, llm.Options{ToolsRequired: true})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "my_tool" {
		t.Errorf("expected tool 'my_tool', got %q", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].Arguments["arg1"] != "xyz" {
		t.Errorf("expected argument 'arg1' to be 'xyz', got %v", resp.ToolCalls[0].Arguments["arg1"])
	}
}

func TestChat_ToolsRequired_PromptFallback_OnUnsupported(t *testing.T) {
	callCount := 0
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": showHandler([]string{}, 4096),
		"/api/tags": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, ollamaTagsResponse{
				Models: []ollamaTagModel{
					{Name: "llama3-unsupported"},
				},
			})
		},
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			var req ollamaChatRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			callCount++

			if len(req.Tools) > 0 {
				t.Errorf("expected tools to be empty for prompt fallback request, got %v", req.Tools)
			}
			hasInstruction := false
			for _, m := range req.Messages {
				if strings.Contains(m.Content, "[INSTRUCTION]") && strings.Contains(m.Content, "my_tool") {
					hasInstruction = true
					break
				}
			}
			if !hasInstruction {
				t.Errorf("expected message to contain injected prompt instructions, got %v", req.Messages)
			}

			writeJSON(w, ollamaChatResponse{
				Message: ollamaMessage{
					Role:    "assistant",
					Content: "```json\n{\n  \"name\": \"my_tool\",\n  \"arguments\": {\n    \"param1\": \"val1\"\n  }\n}\n```",
				},
				Done: true,
			})
		},
	})

	tools := []llm.ToolDefinition{{Name: "my_tool", Description: "a tool", Parameters: map[string]any{}}}
	resp, err := p.Chat(context.Background(), "llama3-unsupported", []llm.Message{
		{Role: "user", Content: "do something"},
	}, tools, llm.Options{ToolsRequired: true})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "my_tool" {
		t.Errorf("expected tool name 'my_tool', got %q", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].Arguments["param1"] != "val1" {
		t.Errorf("expected argument 'param1' to be 'val1', got %v", resp.ToolCalls[0].Arguments["param1"])
	}
	if callCount != 1 {
		t.Errorf("expected 1 chat call, got %d", callCount)
	}
}

func TestChat_ToolsRequired_PromptFallback_OnFailureToCallTools(t *testing.T) {
	callCount := 0
	p, _ := newTestProvider(t, map[string]http.HandlerFunc{
		"/api/show": showHandler([]string{"tools"}, 4096),
		"/api/tags": func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, ollamaTagsResponse{
				Models: []ollamaTagModel{
					{Name: "llama3-unsupported"},
				},
			})
		},
		"/api/chat": func(w http.ResponseWriter, r *http.Request) {
			var req ollamaChatRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			callCount++

			if callCount == 1 {
				writeJSON(w, ollamaChatResponse{
					Message: ollamaMessage{
						Role:    "assistant",
						Content: "Sure, I can help with that without calling any tools.",
					},
					Done: true,
				})
				return
			}

			if len(req.Tools) > 0 {
				t.Errorf("expected tools to be empty for prompt fallback request, got %v", req.Tools)
			}

			writeJSON(w, ollamaChatResponse{
				Message: ollamaMessage{
					Role:    "assistant",
					Content: "```json\n{\n  \"name\": \"my_tool\",\n  \"arguments\": {\n    \"param1\": \"val2\"\n  }\n}\n```",
				},
				Done: true,
			})
		},
	})

	tools := []llm.ToolDefinition{{Name: "my_tool", Description: "a tool", Parameters: map[string]any{}}}
	resp, err := p.Chat(context.Background(), "llama3-unsupported", []llm.Message{
		{Role: "user", Content: "do something"},
	}, tools, llm.Options{ToolsRequired: true})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "my_tool" {
		t.Errorf("expected tool name 'my_tool', got %q", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].Arguments["param1"] != "val2" {
		t.Errorf("expected argument 'param1' to be 'val2', got %v", resp.ToolCalls[0].Arguments["param1"])
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls (1 native + 1 prompt fallback), got %d", callCount)
	}
}
