package llm

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockProvider is a test helper that returns configurable responses/errors.
type mockProvider struct {
	name   string
	resp   *Response
	err    error
	called bool
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) Chat(_ context.Context, _ string, _ []Message, _ []ToolDefinition, _ Options) (*Response, error) {
	m.called = true
	return m.resp, m.err
}

func TestFallback_FirstSucceeds(t *testing.T) {
	p1 := &mockProvider{name: "p1", resp: &Response{Content: "ok"}}
	p2 := &mockProvider{name: "p2", resp: &Response{Content: "fallback"}}

	fc := NewFallbackChain([]Candidate{
		{Provider: p1, Model: "m1"},
		{Provider: p2, Model: "m2"},
	})

	resp, err := fc.Chat(context.Background(), "", []Message{{Role: "user", Content: "hi"}}, nil, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("expected ok, got %s", resp.Content)
	}
	if !p1.called {
		t.Error("p1 should have been called")
	}
	if p2.called {
		t.Error("p2 should not have been called")
	}
}

func TestFallback_RateLimitFallsThrough(t *testing.T) {
	p1 := &mockProvider{
		name: "p1",
		err:  &ProviderError{Reason: ReasonRateLimit, Provider: "p1", Model: "m1", Status: 429, Err: errors.New("rate limited")},
	}
	p2 := &mockProvider{name: "p2", resp: &Response{Content: "from p2"}}

	fc := NewFallbackChain([]Candidate{
		{Provider: p1, Model: "m1"},
		{Provider: p2, Model: "m2"},
	})

	resp, err := fc.Chat(context.Background(), "", []Message{{Role: "user", Content: "hi"}}, nil, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "from p2" {
		t.Errorf("expected from p2, got %s", resp.Content)
	}
}

func TestFallback_AuthStopsChain(t *testing.T) {
	p1 := &mockProvider{
		name: "p1",
		err:  &ProviderError{Reason: ReasonAuth, Provider: "p1", Model: "m1", Status: 401, Err: errors.New("auth failed")},
	}
	p2 := &mockProvider{name: "p2", resp: &Response{Content: "from p2"}}

	fc := NewFallbackChain([]Candidate{
		{Provider: p1, Model: "m1"},
		{Provider: p2, Model: "m2"},
	})

	_, err := fc.Chat(context.Background(), "", []Message{{Role: "user", Content: "hi"}}, nil, Options{})
	if err == nil {
		t.Fatal("expected error for auth failure")
	}
	if p2.called {
		t.Error("p2 should NOT be called after non-retriable auth error")
	}
}

func TestFallback_AllFail(t *testing.T) {
	p1 := &mockProvider{
		name: "p1",
		err:  &ProviderError{Reason: ReasonRateLimit, Provider: "p1", Model: "m1", Status: 429, Err: errors.New("rate limited")},
	}
	p2 := &mockProvider{
		name: "p2",
		err:  &ProviderError{Reason: ReasonTimeout, Provider: "p2", Model: "m2", Status: 500, Err: errors.New("timeout")},
	}

	fc := NewFallbackChain([]Candidate{
		{Provider: p1, Model: "m1"},
		{Provider: p2, Model: "m2"},
	})

	_, err := fc.Chat(context.Background(), "", []Message{{Role: "user", Content: "hi"}}, nil, Options{})
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}

	var exhausted *FallbackExhaustedError
	if !errors.As(err, &exhausted) {
		t.Errorf("expected FallbackExhaustedError, got %T: %v", err, err)
	}
}

func TestFallback_CooldownRespected(t *testing.T) {
	p1 := &mockProvider{name: "p1", resp: &Response{Content: "from p1"}}
	p2 := &mockProvider{name: "p2", resp: &Response{Content: "from p2"}}

	fc := NewFallbackChain([]Candidate{
		{Provider: p1, Model: "m1"},
		{Provider: p2, Model: "m2"},
	})

	// Put p1 in cooldown
	fc.cooldown.Set("p1", 5*time.Minute)

	resp, err := fc.Chat(context.Background(), "", []Message{{Role: "user", Content: "hi"}}, nil, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "from p2" {
		t.Errorf("expected from p2 (p1 in cooldown), got %s", resp.Content)
	}
	if p1.called {
		t.Error("p1 should be skipped when in cooldown")
	}
}

func TestFallbackChain_Name(t *testing.T) {
	fc := NewFallbackChain([]Candidate{
		{Provider: &mockProvider{name: "ollama"}, Model: "llama3.2"},
		{Provider: &mockProvider{name: "anthropic"}, Model: "claude"},
	})
	if fc.Name() != "ollama/anthropic" {
		t.Errorf("expected ollama/anthropic, got %s", fc.Name())
	}
}

func TestFallback_OllamaMissingToolCalls(t *testing.T) {
	// A response that omits a "required" tool call is a content-level outcome,
	// not a provider failure. The chain must return the first provider's
	// response as-is (the runtime executor owns recovery), and must NOT cool
	// down the provider or fail over to a later candidate — cooling the shared
	// chain here would starve every other concurrent chat.
	ollama := &mockProvider{
		name: "ollama",
		resp: &Response{Content: "I cannot do that directly, but I will talk about it."},
	}
	p2 := &mockProvider{
		name: "anthropic",
		resp: &Response{
			Content: "",
			ToolCalls: []ToolCall{
				{
					ID:        "call_123",
					Name:      "test_tool",
					Arguments: map[string]any{},
				},
			},
		},
	}

	fc := NewFallbackChain([]Candidate{
		{Provider: ollama, Model: "llama3"},
		{Provider: p2, Model: "claude"},
	})

	tools := []ToolDefinition{{Name: "test_tool", Description: "Test tool description"}}
	opts := Options{ToolCallingRequired: true}

	resp, err := fc.Chat(context.Background(), "", []Message{{Role: "user", Content: "run test_tool"}}, tools, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "I cannot do that directly, but I will talk about it." {
		t.Errorf("expected Ollama content returned as-is, got %q", resp.Content)
	}

	if !ollama.called {
		t.Error("ollama should have been called first")
	}

	if p2.called {
		t.Error("anthropic should NOT be used as fallback for a missing tool call")
	}

	if fc.cooldown.IsInCooldown("ollama") {
		t.Error("ollama must NOT be cooled down for omitting a required tool call")
	}
}

func TestFallback_OllamaMissingToolCalls_Exhausted(t *testing.T) {
	// Ollama is the only provider and returns text instead of the expected tool call.
	ollama := &mockProvider{
		name: "ollama",
		resp: &Response{Content: "I cannot do that directly, but I will talk about it."},
	}

	fc := NewFallbackChain([]Candidate{
		{Provider: ollama, Model: "llama3"},
	})

	tools := []ToolDefinition{{Name: "test_tool", Description: "Test tool description"}}
	opts := Options{ToolCallingRequired: true}

	resp, err := fc.Chat(context.Background(), "", []Message{{Role: "user", Content: "run test_tool"}}, tools, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "I cannot do that directly, but I will talk about it." {
		t.Errorf("expected Ollama content, got %q", resp.Content)
	}

	if len(resp.ToolCalls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(resp.ToolCalls))
	}

	if fc.cooldown.IsInCooldown("ollama") {
		t.Error("ollama must NOT be cooled down for omitting a required tool call")
	}
}
