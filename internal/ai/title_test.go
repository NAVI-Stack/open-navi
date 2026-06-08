package ai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/open-navi/navi/internal/llm"
)

type mockProvider struct {
	chatFunc func(ctx context.Context, model string, messages []llm.Message, tools []llm.ToolDefinition, opts llm.Options) (*llm.Response, error)
}

func (m *mockProvider) Chat(ctx context.Context, model string, messages []llm.Message, tools []llm.ToolDefinition, opts llm.Options) (*llm.Response, error) {
	return m.chatFunc(ctx, model, messages, tools, opts)
}

func (m *mockProvider) Name() string {
	return "mock"
}

func TestGenerateConversationTitle(t *testing.T) {
	ctx := context.Background()

	t.Run("Basic title generation", func(t *testing.T) {
		mock := &mockProvider{
			chatFunc: func(ctx context.Context, model string, messages []llm.Message, tools []llm.ToolDefinition, opts llm.Options) (*llm.Response, error) {
				return &llm.Response{
					Content: `{"title": "Navi Title Generator", "confidence": 0.9, "reason": "User wants to create a title generator."}`,
				}, nil
			},
		}

		input := TitleGeneratorInput{
			Messages: []llm.Message{
				{Role: "user", Content: "Most AIs generate a title for the conversation after the first response. I want to create an AI function for Navi that does the same thing."},
			},
			Mode: ModeCreate,
		}

		resp, err := GenerateConversationTitle(ctx, mock, "gpt-4", input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Title != "Navi Title Generator" {
			t.Errorf("expected title 'Navi Title Generator', got '%s'", resp.Title)
		}
	})

	t.Run("Debugging title", func(t *testing.T) {
		mock := &mockProvider{
			chatFunc: func(ctx context.Context, model string, messages []llm.Message, tools []llm.ToolDefinition, opts llm.Options) (*llm.Response, error) {
				return &llm.Response{
					Content: `{"title": "Debug Playwright Auth Tests", "confidence": 0.9, "reason": "User is debugging Playwright tests."}`,
				}, nil
			},
		}

		input := TitleGeneratorInput{
			Messages: []llm.Message{
				{Role: "user", Content: "Can you help me debug why my auth login test fails in Playwright?"},
			},
			Mode: ModeCreate,
		}

		resp, err := GenerateConversationTitle(ctx, mock, "gpt-4", input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Title != "Debug Playwright Auth Tests" {
			t.Errorf("expected title 'Debug Playwright Auth Tests', got '%s'", resp.Title)
		}
	})

	t.Run("Manual title protection", func(t *testing.T) {
		mock := &mockProvider{
			chatFunc: func(ctx context.Context, model string, messages []llm.Message, tools []llm.ToolDefinition, opts llm.Options) (*llm.Response, error) {
				t.Error("LLM should not be called when manual title exists and protection is active")
				return nil, errors.New("should not be called")
			},
		}

		input := TitleGeneratorInput{
			ExistingTitle:           "My Custom Title",
			TitleManuallyOverridden: true,
			Mode:                    ModeCreate,
			Messages: []llm.Message{
				{Role: "user", Content: "Something else"},
			},
		}

		resp, err := GenerateConversationTitle(ctx, mock, "gpt-4", input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Title != "My Custom Title" {
			t.Errorf("expected title 'My Custom Title', got '%s'", resp.Title)
		}
	})

	t.Run("Invalid LLM response fallback", func(t *testing.T) {
		mock := &mockProvider{
			chatFunc: func(ctx context.Context, model string, messages []llm.Message, tools []llm.ToolDefinition, opts llm.Options) (*llm.Response, error) {
				return &llm.Response{
					Content: `invalid json`,
				}, nil
			},
		}

		input := TitleGeneratorInput{
			Messages: []llm.Message{
				{Role: "user", Content: "Can you help me debug why my auth login test fails in Playwright?"},
			},
			Mode: ModeCreate,
		}

		resp, err := GenerateConversationTitle(ctx, mock, "gpt-4", input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Fallback logic should extract something useful
		if !strings.Contains(resp.Title, "Debug Why My Auth Login Test") {
			t.Errorf("expected title to contain 'Debug Why My Auth Login Test', got '%s'", resp.Title)
		}
	})

	t.Run("Generic title rejection", func(t *testing.T) {
		mock := &mockProvider{
			chatFunc: func(ctx context.Context, model string, messages []llm.Message, tools []llm.ToolDefinition, opts llm.Options) (*llm.Response, error) {
				return &llm.Response{
					Content: `{"title": "AI Discussion", "confidence": 0.5, "reason": "Generic title."}`,
				}, nil
			},
		}

		input := TitleGeneratorInput{
			Messages: []llm.Message{
				{Role: "user", Content: "I want to talk about AI in general."},
			},
			Mode: ModeCreate,
		}

		resp, err := GenerateConversationTitle(ctx, mock, "gpt-4", input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Title == "AI Discussion" {
			t.Error("expected 'AI Discussion' to be rejected")
		}
	})

	t.Run("Sensitive detail avoidance", func(t *testing.T) {
		mock := &mockProvider{
			chatFunc: func(ctx context.Context, model string, messages []llm.Message, tools []llm.ToolDefinition, opts llm.Options) (*llm.Response, error) {
				return &llm.Response{
					Content: `{"title": "My password is 123", "confidence": 0.9, "reason": "Sensitive title."}`,
				}, nil
			},
		}

		input := TitleGeneratorInput{
			Messages: []llm.Message{
				{Role: "user", Content: "My password is 123"},
			},
			Mode: ModeCreate,
		}

		resp, err := GenerateConversationTitle(ctx, mock, "gpt-4", input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if strings.Contains(strings.ToLower(resp.Title), "password") {
			t.Error("expected title to not contain sensitive details")
		}
	})
	t.Run("Refresh mode without topic drift", func(t *testing.T) {
		mock := &mockProvider{
			chatFunc: func(ctx context.Context, model string, messages []llm.Message, tools []llm.ToolDefinition, opts llm.Options) (*llm.Response, error) {
				return &llm.Response{
					Content: `{"title": "Conversate Q&A Workflow", "confidence": 1.0, "reason": "No topic shift detected."}`,
				}, nil
			},
		}

		input := TitleGeneratorInput{
			ExistingTitle: "Conversate Q&A Workflow",
			Mode:          ModeRefresh,
			Messages: []llm.Message{
				{Role: "user", Content: "I need help planning the Q&A ingestion workflow for Conversate."},
				{Role: "assistant", Content: "Sure, let's start with the data sources."},
				{Role: "user", Content: "We have PDFs and web pages."},
			},
		}

		resp, err := GenerateConversationTitle(ctx, mock, "gpt-4", input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Title != "Conversate Q&A Workflow" {
			t.Errorf("expected title to be preserved, got '%s'", resp.Title)
		}
	})

	t.Run("Refresh mode with topic drift", func(t *testing.T) {
		mock := &mockProvider{
			chatFunc: func(ctx context.Context, model string, messages []llm.Message, tools []llm.ToolDefinition, opts llm.Options) (*llm.Response, error) {
				return &llm.Response{
					Content: `{"title": "Feature Flag Strategy", "confidence": 0.9, "reason": "Topic shifted from Q&A to feature flags."}`,
				}, nil
			},
		}

		input := TitleGeneratorInput{
			ExistingTitle: "Conversate Q&A Workflow",
			Mode:          ModeRefresh,
			Messages: []llm.Message{
				{Role: "user", Content: "I need help planning the Q&A ingestion workflow for Conversate."},
				{Role: "assistant", Content: "Sure, let's start with the data sources."},
				{Role: "user", Content: "Actually, let's talk about feature flags instead."},
			},
		}

		resp, err := GenerateConversationTitle(ctx, mock, "gpt-4", input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resp.Title != "Feature Flag Strategy" {
			t.Errorf("expected new title 'Feature Flag Strategy', got '%s'", resp.Title)
		}
	})
}
