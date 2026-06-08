package llm

import (
	"context"
	"testing"
)

type streamRecorderProvider struct {
	streamCalls int
	chatCalls   int
	models      []string
	chunks      []string
}

func (p *streamRecorderProvider) Chat(ctx context.Context, model string, messages []Message, tools []ToolDefinition, opts Options) (*Response, error) {
	p.chatCalls++
	p.models = append(p.models, model)
	return &Response{Content: "chat"}, nil
}

func (p *streamRecorderProvider) ChatStream(ctx context.Context, model string, messages []Message, tools []ToolDefinition, opts Options, onChunk func(delta string)) (*Response, error) {
	p.streamCalls++
	p.models = append(p.models, model)
	for _, chunk := range p.chunks {
		if onChunk != nil {
			onChunk(chunk)
		}
	}
	return &Response{Content: "stream"}, nil
}

func (p *streamRecorderProvider) Name() string { return "stream-recorder" }

func TestRouterChatStream_UsesRoutedStreamingProvider(t *testing.T) {
	prov := &streamRecorderProvider{chunks: []string{"hel", "lo"}}
	router := NewRouter(prov)
	router.AddRoute("chat", prov, "llama3.1:latest")

	var got string
	resp, err := router.ChatStream(context.Background(), "chat", []Message{{Role: "user", Content: "hi"}}, nil, Options{}, func(delta string) {
		got += delta
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if resp.Content != "stream" {
		t.Fatalf("expected streamed content, got %q", resp.Content)
	}
	if got != "hello" {
		t.Fatalf("expected streamed chunks, got %q", got)
	}
	if prov.streamCalls != 1 {
		t.Fatalf("expected one stream call, got %d", prov.streamCalls)
	}
	if prov.chatCalls != 0 {
		t.Fatalf("expected no fallback chat call, got %d", prov.chatCalls)
	}
	if len(prov.models) != 1 || prov.models[0] != "llama3.1:latest" {
		t.Fatalf("expected routed model llama3.1:latest, got %v", prov.models)
	}
}
