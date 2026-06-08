package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/prompts"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
	"github.com/open-navi/navi/internal/worldmodel"
)

// mockLLM implements llm.Provider for testing.
type mockLLM struct {
	resp *llm.Response
	err  error
	msgs []llm.Message // captured messages from last call
}

func (m *mockLLM) Name() string { return "mock" }
func (m *mockLLM) Chat(_ context.Context, _ string, messages []llm.Message, _ []llm.ToolDefinition, _ llm.Options) (*llm.Response, error) {
	m.msgs = messages
	return m.resp, m.err
}

func mustNewLLMDirectiveAdapter(t *testing.T, provider llm.Provider, model string, db *sql.DB, wm *worldmodel.WorldModel, workspaceDir string, pm *prompts.Manager) *LLMDirectiveAdapter {
	t.Helper()
	a, err := NewLLMDirectiveAdapter(provider, model, db, wm, workspaceDir, pm)
	if err != nil {
		t.Fatalf("NewLLMDirectiveAdapter: %v", err)
	}
	return a
}

func TestAdapter_DirectiveReply(t *testing.T) {
	db := store.InitTestDB(t)

	// Create a CHAT directive with a message
	d := schema.NewDirective("Chat about auth", schema.DirectiveModeChat, "cli")
	if err := store.SaveDirective(context.Background(), db, d); err != nil {
		t.Fatalf("save directive: %v", err)
	}

	msg := schema.NewDirectiveMessage(d.DirectiveID, "owner", "How should we implement JWT?")
	if err := store.AppendMessage(context.Background(), db, msg); err != nil {
		t.Fatalf("append message: %v", err)
	}

	mock := &mockLLM{
		resp: &llm.Response{
			Content:      "I recommend using HMAC-signed JWTs with 24h expiry.",
			InputTokens:  50,
			OutputTokens: 30,
		},
	}

	wm := worldmodel.New(db)
	adapter := mustNewLLMDirectiveAdapter(t, mock, "test-model", db, wm, "", nil)
	decision, err := adapter.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}

	if len(decision.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(decision.Events))
	}
	if decision.Events[0].Type != schema.FactDirectiveReplied {
		t.Errorf("expected FactDirectiveReplied, got %s", decision.Events[0].Type)
	}
	if decision.Cost <= 0 {
		t.Error("expected non-zero cost")
	}

	// Verify LLM was called with the message history
	if len(mock.msgs) < 2 {
		t.Fatalf("expected at least 2 messages to LLM, got %d", len(mock.msgs))
	}
	if mock.msgs[0].Role != "system" {
		t.Errorf("first message should be system, got %s", mock.msgs[0].Role)
	}

	// Verify the reply was persisted
	msgs, err := store.GetMessages(context.Background(), db, d.DirectiveID, 10)
	if err != nil {
		t.Fatalf("get messages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (owner + orchestrator), got %d", len(msgs))
	}
}

func TestAdapter_EmptySnapshot(t *testing.T) {
	db := store.InitTestDB(t)

	mock := &mockLLM{
		resp: &llm.Response{Content: "should not be called"},
	}

	wm := worldmodel.New(db)
	adapter := mustNewLLMDirectiveAdapter(t, mock, "test-model", db, wm, "", nil)
	decision, err := adapter.Decide(context.Background())
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if len(decision.Events) != 0 {
		t.Errorf("expected 0 events for empty snapshot, got %d", len(decision.Events))
	}
	if mock.msgs != nil {
		t.Error("LLM should not be called when no DISCUSS directives exist")
	}
}

func TestAdapter_LLMError(t *testing.T) {
	db := store.InitTestDB(t)

	d := schema.NewDirective("Error test", schema.DirectiveModeChat, "cli")
	if err := store.SaveDirective(context.Background(), db, d); err != nil {
		t.Fatalf("save directive: %v", err)
	}

	msg := schema.NewDirectiveMessage(d.DirectiveID, "owner", "Hi")
	if err := store.AppendMessage(context.Background(), db, msg); err != nil {
		t.Fatalf("append message: %v", err)
	}

	mock := &mockLLM{
		err: errors.New("llm connection failed"),
	}

	wm := worldmodel.New(db)
	adapter := mustNewLLMDirectiveAdapter(t, mock, "test-model", db, wm, "", nil)
	_, err := adapter.Decide(context.Background())
	if err == nil {
		t.Fatal("expected error when LLM fails")
	}
}

func TestAdapter_ACTMode_DecomposeTasks(t *testing.T) {
	ctx := context.Background()
	db := store.InitTestDB(t)

	d := schema.NewDirective("Implement health API", schema.DirectiveModeAct, "cli")
	if err := store.SaveDirective(ctx, db, d); err != nil {
		t.Fatalf("save directive: %v", err)
	}
	msg := schema.NewDirectiveMessage(d.DirectiveID, "owner", "Add a health check endpoint.")
	if err := store.AppendMessage(ctx, db, msg); err != nil {
		t.Fatalf("append message: %v", err)
	}

	mock := &mockLLM{
		resp: &llm.Response{
			Content:      "Decomposing.",
			InputTokens:  10,
			OutputTokens: 5,
			ToolCalls: []llm.ToolCall{{
				ID:   "tc-1",
				Name: "decompose_tasks",
				Arguments: map[string]any{
					"tasks": []any{
						map[string]any{
							"title":        "Add health handler",
							"description":  "Implement GET /health handler",
							"surface_path": "internal/gateway/",
							"agent_type":   "coder",
						},
					},
				},
			}},
		},
	}

	wm := worldmodel.New(db)
	adapter := mustNewLLMDirectiveAdapter(t, mock, "test-model", db, wm, "", nil)
	decision, err := adapter.Decide(ctx)
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}

	var assignCount int
	var reflectionCount int
	for _, ev := range decision.Events {
		if ev.Type == schema.CmdTaskAssign {
			assignCount++
		}
		if ev.Type == schema.FactReflectionQueued {
			reflectionCount++
		}
	}
	if assignCount != 1 {
		t.Errorf("expected 1 CmdTaskAssign event, got %d", assignCount)
	}
	if reflectionCount == 0 {
		t.Error("expected decomposition reflection event")
	}

	tasks, err := store.GetTasksByDirective(ctx, db, d.DirectiveID)
	if err != nil {
		t.Fatalf("GetTasksByDirective: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Title != "Add health handler" || tasks[0].AssignedTo != schema.AgentCoder {
		t.Errorf("task: title=%q assigned_to=%s", tasks[0].Title, tasks[0].AssignedTo)
	}
}

func TestAdapter_ACTMode_DecomposeUsesWorkspaceListing(t *testing.T) {
	ctx := context.Background()
	db := store.InitTestDB(t)
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "marker.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(tmp, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}

	d := schema.NewDirective("Implement health API", schema.DirectiveModeAct, "cli")
	if err := store.SaveDirective(ctx, db, d); err != nil {
		t.Fatalf("save directive: %v", err)
	}
	if err := store.AppendMessage(ctx, db, schema.NewDirectiveMessage(d.DirectiveID, "owner", "Add a health check endpoint.")); err != nil {
		t.Fatalf("append message: %v", err)
	}

	mock := &mockLLM{
		resp: &llm.Response{
			Content:      "Decomposing.",
			InputTokens:  10,
			OutputTokens: 5,
			ToolCalls: []llm.ToolCall{{
				ID:   "tc-1",
				Name: "decompose_tasks",
				Arguments: map[string]any{
					"tasks": []any{
						map[string]any{
							"title":        "Add health handler",
							"description":  "Implement GET /health handler",
							"surface_path": "internal/gateway/",
							"agent_type":   "coder",
						},
					},
				},
			}},
		},
	}

	adapter := mustNewLLMDirectiveAdapter(t, mock, "test-model", db, worldmodel.New(db), tmp, nil)
	if _, err := adapter.Decide(ctx); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if len(mock.msgs) == 0 || mock.msgs[0].Role != "system" {
		t.Fatalf("expected system message")
	}
	sys := mock.msgs[0].Content
	if !strings.Contains(sys, "Workspace Structure") {
		t.Fatalf("expected decompose template in system prompt, got %q", sys)
	}
	if !strings.Contains(sys, "marker.txt") || !strings.Contains(sys, "pkg/") {
		t.Fatalf("expected workspace listing entries in system prompt, got %q", sys)
	}
}

func TestAdapter_OwnerContextInSystemPrompt(t *testing.T) {
	ctx := context.Background()
	db := store.InitTestDB(t)

	if err := store.CreateOwner(ctx, db, store.Owner{ID: "owner-1", Name: "Test", Handle: "test"}); err != nil {
		t.Fatalf("create owner: %v", err)
	}
	if err := store.SaveFact(ctx, db, store.Fact{
		Scope: "owner", ScopeID: "owner-1", Category: "user_preference", Key: "editor", Value: "vscode", Source: "explicit",
	}); err != nil {
		t.Fatalf("save fact: %v", err)
	}

	d := schema.NewDirective("Chat", schema.DirectiveModeChat, "cli")
	if err := store.SaveDirective(ctx, db, d); err != nil {
		t.Fatalf("save directive: %v", err)
	}
	if err := store.AppendMessage(ctx, db, schema.NewDirectiveMessage(d.DirectiveID, "owner", "Hi")); err != nil {
		t.Fatalf("append message: %v", err)
	}

	mock := &mockLLM{
		resp: &llm.Response{Content: "Hello.", InputTokens: 10, OutputTokens: 5},
	}
	wm := worldmodel.New(db)
	adapter := mustNewLLMDirectiveAdapter(t, mock, "test-model", db, wm, "", nil)
	_, err := adapter.Decide(ctx)
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if len(mock.msgs) == 0 || mock.msgs[0].Role != "system" {
		t.Fatalf("expected system message, got %d messages", len(mock.msgs))
	}
	sys := mock.msgs[0].Content
	if !strings.Contains(sys, "Owner context") {
		t.Errorf("system prompt should include Owner context block, got: ...%s...", sys)
	}
	if !strings.Contains(sys, "editor") || !strings.Contains(sys, "vscode") {
		t.Errorf("system prompt should include owner fact editor:vscode, got: ...%s...", sys)
	}
}

func TestAdapter_DirectiveTaskOutcomeFactsAppearInPrompt(t *testing.T) {
	ctx := context.Background()
	db := store.InitTestDB(t)

	d := schema.NewDirective("Implement feature", schema.DirectiveModeChat, "cli")
	if err := store.SaveDirective(ctx, db, d); err != nil {
		t.Fatalf("save directive: %v", err)
	}
	if err := store.AppendMessage(ctx, db, schema.NewDirectiveMessage(d.DirectiveID, "owner", "What happened with the last task?")); err != nil {
		t.Fatalf("append message: %v", err)
	}
	if err := store.SaveFact(ctx, db, store.Fact{
		Scope:    "directive",
		ScopeID:  d.DirectiveID,
		Category: "task_outcome",
		Key:      "task_1_outcome",
		Value:    "Task 'Add health handler' FAILED: missing dependency",
		Source:   "coder_agent",
	}); err != nil {
		t.Fatalf("save fact: %v", err)
	}

	mock := &mockLLM{
		resp: &llm.Response{Content: "Let's adjust the plan.", InputTokens: 10, OutputTokens: 5},
	}
	adapter := mustNewLLMDirectiveAdapter(t, mock, "test-model", db, worldmodel.New(db), "", nil)
	if _, err := adapter.Decide(ctx); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if len(mock.msgs) == 0 {
		t.Fatal("expected LLM call")
	}
	if !strings.Contains(mock.msgs[0].Content, "Task 'Add health handler' FAILED: missing dependency") {
		t.Fatalf("expected task outcome fact in system prompt, got %q", mock.msgs[0].Content)
	}
}

func TestAdapter_FinalizeDirectiveWritesOwnerLearning(t *testing.T) {
	ctx := context.Background()
	db := store.InitTestDB(t)

	if err := store.CreateOwner(ctx, db, store.Owner{ID: "owner-1", Name: "Test", Handle: "test"}); err != nil {
		t.Fatalf("create owner: %v", err)
	}

	d := schema.NewDirective("Implement health API", schema.DirectiveModeAct, "cli")
	if err := store.SaveDirective(ctx, db, d); err != nil {
		t.Fatalf("save directive: %v", err)
	}
	task := schema.NewTask("Add health handler", "Implement GET /health", schema.RiskLow, schema.AgentCoder)
	task.DirectiveID = d.DirectiveID
	task.Status = schema.TaskStatusCompleted
	if err := store.PersistTask(ctx, db, task); err != nil {
		t.Fatalf("persist task: %v", err)
	}
	if err := store.SaveFact(ctx, db, store.Fact{
		Scope:    "directive",
		ScopeID:  d.DirectiveID,
		Category: "task_outcome",
		Key:      "task_" + task.ID + "_outcome",
		Value:    "Task 'Add health handler' completed: added endpoint and tests",
		Source:   "coder_agent",
	}); err != nil {
		t.Fatalf("save fact: %v", err)
	}

	mock := &mockLLM{resp: &llm.Response{Content: "unused"}}
	adapter := mustNewLLMDirectiveAdapter(t, mock, "test-model", db, worldmodel.New(db), "", nil)
	decision, err := adapter.Decide(ctx)
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	var replied, reflected bool
	for _, ev := range decision.Events {
		if ev.Type == schema.FactDirectiveReplied {
			replied = true
		}
		if ev.Type == schema.FactReflectionQueued {
			reflected = true
		}
	}
	if !replied || !reflected {
		t.Fatalf("expected directive replied and reflection events, got %+v", decision.Events)
	}

	updated, ok, err := store.GetDirective(ctx, db, d.DirectiveID)
	if err != nil || !ok {
		t.Fatalf("GetDirective: ok=%v err=%v", ok, err)
	}
	if updated.Status != schema.DirectiveStatusComplete {
		t.Fatalf("expected directive COMPLETE, got %s", updated.Status)
	}

	facts, err := store.ListFacts(ctx, db, "owner", "owner-1", false, 10, false)
	if err != nil {
		t.Fatalf("ListFacts owner: %v", err)
	}
	if len(facts) == 0 {
		t.Fatal("expected owner learning fact")
	}
	if !strings.Contains(facts[0].Key, "learned_from_directive_") {
		t.Fatalf("expected owner learning fact key, got %s", facts[0].Key)
	}

	msgs, err := store.GetMessages(ctx, db, d.DirectiveID, 10)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 completion message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "Directive task execution summary") {
		t.Fatalf("expected completion summary message, got %q", msgs[0].Content)
	}
}
