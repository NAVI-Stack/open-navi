package navi

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/schema"
)

// extractorLLMProvider is a fake llm.Provider that returns a canned response and
// records what it was called with.
type extractorLLMProvider struct {
	response *llm.Response
	model    string
	messages []llm.Message
}

func (p *extractorLLMProvider) Chat(ctx context.Context, model string, messages []llm.Message, tools []llm.ToolDefinition, opts llm.Options) (*llm.Response, error) {
	p.model = model
	p.messages = append([]llm.Message(nil), messages...)
	return p.response, nil
}

func (p *extractorLLMProvider) Name() string { return "extractor-test" }

func TestExtractFacts_ParsesJSONArrayAndDefaultsOwnerScope(t *testing.T) {
	provider := &extractorLLMProvider{
		response: &llm.Response{
			Content: "```json\n[{\"key\":\"preferred_language\",\"value\":\"Go\",\"category\":\"technical_context\"}]\n```",
		},
	}
	loop := NewAgentLoop(LoopConfig{
		ExperienceManager: NewExperienceManager("", ExperienceModeStandard),
		LLM:               provider,
		Model:             "chat",
	})

	facts, err := loop.extractFacts(context.Background(), "Remember this: I prefer Go.", "Understood. I'll prefer Go when suggesting implementation examples.")
	if err != nil {
		t.Fatalf("extractFacts: %v", err)
	}
	if provider.model != "chat" {
		t.Fatalf("expected extractor to use chat model route, got %q", provider.model)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
	if facts[0].Scope != "owner" {
		t.Fatalf("expected missing scope to default to owner, got %q", facts[0].Scope)
	}
	if facts[0].Category != "technical_context" {
		t.Fatalf("expected category technical_context, got %q", facts[0].Category)
	}
}

func TestReflectionDetailsForTurn_AssignsScopeIDs(t *testing.T) {
	provider := &extractorLLMProvider{
		response: &llm.Response{
			Content: `[{"key":"preferred_language","value":"Go","category":"technical_context","scope":"owner"},{"key":"active_file","value":"internal/navi/loop.go","category":"technical_context","scope":"session"}]`,
		},
	}
	loop := NewAgentLoop(LoopConfig{
		ExperienceManager: NewExperienceManager("", ExperienceModeStandard),
		LLM:               provider,
		Model:             "chat",
		ResolveOwnerID:    func(ctx context.Context, chatID string) string { return "owner-42" },
	})

	raw := loop.reflectionDetailsForTurn(context.Background(), "chat-7", "Turn completed", "Remember this: default to Go.", "Understood. I will default to Go and keep this chat focused on loop.go.")
	var details schema.ReflectionDetails
	if err := json.Unmarshal([]byte(raw), &details); err != nil {
		t.Fatalf("unmarshal reflection details: %v", err)
	}
	if len(details.Facts) != 2 {
		t.Fatalf("expected 2 facts, got %d", len(details.Facts))
	}
	if details.Facts[0].ScopeID != "owner-42" {
		t.Fatalf("expected owner fact scope_id owner-42, got %q", details.Facts[0].ScopeID)
	}
	if details.Facts[1].ScopeID != "chat-7" {
		t.Fatalf("expected session fact scope_id chat-7, got %q", details.Facts[1].ScopeID)
	}
}

func TestShouldExtractFacts_SkipsTrivialTurns(t *testing.T) {
	tests := []struct {
		name    string
		summary string
		user    string
		reply   string
		expect  bool
	}{
		{
			name:    "skip greeting",
			summary: "Turn completed",
			user:    "hey",
			reply:   "Hi there!",
			expect:  false,
		},
		{
			name:    "skip router query",
			summary: "Turn completed (provider/model query)",
			user:    "what models are available?",
			reply:   "Current: ollama/llama3:latest",
			expect:  false,
		},
		{
			name:    "extract substantive turn",
			summary: "Turn completed",
			user:    "Remember that the workspace uses Go and SQLite.",
			reply:   "Noted. I will treat Go and SQLite as the default stack for this workspace going forward.",
			expect:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldExtractFacts(tc.summary, tc.user, tc.reply)
			if got != tc.expect {
				t.Fatalf("shouldExtractFacts() = %v, want %v", got, tc.expect)
			}
		})
	}
}

func TestReflectionDetailsForTurn_CapturesPreferenceSignalsWithoutFactExtraction(t *testing.T) {
	loop := NewAgentLoop(LoopConfig{
		ExperienceManager: NewExperienceManager("", ExperienceModeStandard),
		ResolveOwnerID:    func(ctx context.Context, chatID string) string { return "owner-7" },
	})

	raw := loop.reflectionDetailsForTurn(context.Background(), "chat-9", "Turn completed", "Please be brief and use bullets.", "Sure.")
	var details schema.ReflectionDetails
	if err := json.Unmarshal([]byte(raw), &details); err != nil {
		t.Fatalf("unmarshal reflection details: %v", err)
	}
	if len(details.PreferenceSignals) == 0 {
		t.Fatal("expected preference signals to be captured")
	}
	if got := details.PreferenceSignals[0].ScopeID; got != "owner-7" {
		t.Fatalf("expected owner scope_id owner-7, got %q", got)
	}
	if len(details.Facts) != 0 {
		t.Fatalf("expected no extracted facts for terse reply, got %+v", details.Facts)
	}
}
