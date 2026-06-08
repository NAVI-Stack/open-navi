package coder

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-navi/navi/internal/bus"
	"github.com/open-navi/navi/internal/cognitive"
	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/navi/filetools"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

type mockLLM struct {
	response *llm.Response
	err      error
}

func (m *mockLLM) Chat(ctx context.Context, model string, messages []llm.Message, tools []llm.ToolDefinition, opts llm.Options) (*llm.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func (m *mockLLM) Name() string { return "mock" }

type sequenceLLM struct {
	responses []*llm.Response
	calls     int
}

func (m *sequenceLLM) Chat(ctx context.Context, model string, messages []llm.Message, tools []llm.ToolDefinition, opts llm.Options) (*llm.Response, error) {
	if m.calls >= len(m.responses) {
		return nil, fmt.Errorf("unexpected chat call %d", m.calls+1)
	}
	resp := m.responses[m.calls]
	m.calls++
	return resp, nil
}

func (m *sequenceLLM) Name() string { return "sequence" }

type mockRecorder struct{}

func (m *mockRecorder) UpdateTask(ctx context.Context, task schema.Task) error { return nil }
func (m *mockRecorder) SaveExecutionOutcome(ctx context.Context, eo schema.ExecutionOutcome) error {
	return nil
}
func (m *mockRecorder) SaveFact(ctx context.Context, fact schema.Fact, prov *schema.EntityProvenance) error {
	return nil
}

func TestRunner_Execute_CompletesTask(t *testing.T) {
	ctx := context.Background()
	db := store.InitTestDB(t)
	memBus := bus.NewMemBus(db)

	task := schema.NewTask("test task", "description", schema.RiskLow, schema.AgentCoder)
	task.DirectiveID = "dir-1"
	directive := schema.NewDirective("Implement test task", schema.DirectiveModeAct, "test")
	directive.DirectiveID = task.DirectiveID
	if err := store.SaveDirective(ctx, db, directive); err != nil {
		t.Fatalf("SaveDirective: %v", err)
	}
	if err := store.PersistTask(ctx, db, task); err != nil {
		t.Fatalf("PersistTask: %v", err)
	}

	r := NewRunner(Config{
		WorkspaceDir:      t.TempDir(),
		LLM:               &mockLLM{response: &llm.Response{Content: "Done.", ToolCalls: nil}},
		Model:             "chat",
		DB:                db,
		ExecutionRecorder: cognitive.StoreExecutionRecorder(db, nil),
		DirectiveWriter:   cognitive.StoreDirectiveWriter(db),
		Bus:               memBus,
	})

	if err := r.Execute(ctx, task); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	got, ok, err := store.GetTaskByID(ctx, db, task.ID)
	if err != nil {
		t.Fatalf("GetTaskByID: %v", err)
	}
	if !ok {
		t.Fatal("task not found")
	}
	if got.Status != schema.TaskStatusCompleted {
		t.Errorf("status = %s, want completed", got.Status)
	}

	facts, err := store.ListFacts(ctx, db, "directive", task.DirectiveID, false, 10, false)
	if err != nil {
		t.Fatalf("ListFacts: %v", err)
	}
	if len(facts) == 0 {
		t.Fatal("expected task outcome fact to be recorded")
	}
	if facts[0].Category != "task_outcome" {
		t.Fatalf("expected task_outcome fact, got %q", facts[0].Category)
	}

	msgs, err := store.GetMessages(ctx, db, task.DirectiveID, 10)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 directive message, got %d", len(msgs))
	}

	events, err := store.EventsSince(ctx, db, 0, 20)
	if err != nil {
		t.Fatalf("EventsSince: %v", err)
	}
	var foundReflection bool
	for _, ev := range events {
		if ev.Type == schema.FactReflectionQueued {
			foundReflection = true
			break
		}
	}
	if !foundReflection {
		t.Fatal("expected task execution reflection event")
	}
}

func TestRunner_Execute_RejectsNonCoderTask(t *testing.T) {
	ctx := context.Background()
	task := schema.NewTask("x", "y", schema.RiskLow, schema.AgentNavi)

	r := NewRunner(Config{ExecutionRecorder: &mockRecorder{}, DirectiveWriter: cognitive.StoreDirectiveWriter(store.InitTestDB(t))})
	err := r.Execute(ctx, task)
	if err == nil {
		t.Fatal("expected error for non-coder task")
	}
}

func TestRunner_Execute_UsesProjectWorkspaceRootForFileTools(t *testing.T) {
	ctx := context.Background()
	db := store.InitTestDB(t)
	memBus := bus.NewMemBus(db)
	globalRoot := t.TempDir()
	projectRoot := t.TempDir()

	workspace := coderTestWorkspace("ws-code", schema.WorkspaceStatusActive, []string{projectRoot})
	if err := store.SaveWorkspace(ctx, db, workspace); err != nil {
		t.Fatalf("SaveWorkspace: %v", err)
	}
	project := coderTestProject("proj-code", schema.ProjectKindCoding)
	project.WorkspaceID = workspace.ID
	if err := store.SaveProject(ctx, db, project); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	task := schema.NewTask("write project file", "create a project file", schema.RiskLow, schema.AgentCoder)
	task.DirectiveID = "dir-project"
	task.ProjectID = project.ID
	task.WorkspaceID = workspace.ID
	task.TaskClass = schema.TaskClassMutative
	directive := schema.NewDirective("Implement project file", schema.DirectiveModeAct, "test")
	directive.DirectiveID = task.DirectiveID
	if err := store.SaveDirective(ctx, db, directive); err != nil {
		t.Fatalf("SaveDirective: %v", err)
	}
	if err := store.PersistTask(ctx, db, task); err != nil {
		t.Fatalf("PersistTask: %v", err)
	}

	llmSeq := &sequenceLLM{responses: []*llm.Response{
		{
			Content: "Writing the file.",
			ToolCalls: []llm.ToolCall{
				{
					ID:   "call-write",
					Name: filetools.WriteFileToolName,
					Arguments: map[string]any{
						"path":    "nested/output.txt",
						"content": "from project workspace",
					},
				},
			},
		},
		{Content: "Done."},
	}}
	r := NewRunner(Config{
		WorkspaceDir:      globalRoot,
		LLM:               llmSeq,
		Model:             "chat",
		DB:                db,
		ExecutionRecorder: cognitive.StoreExecutionRecorder(db, nil),
		DirectiveWriter:   cognitive.StoreDirectiveWriter(db),
		Bus:               memBus,
	})

	if err := r.Execute(ctx, task); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	gotContent, err := os.ReadFile(filepath.Join(projectRoot, "nested", "output.txt"))
	if err != nil {
		t.Fatalf("ReadFile project root: %v", err)
	}
	if string(gotContent) != "from project workspace" {
		t.Fatalf("project file content = %q", string(gotContent))
	}
	if _, err := os.Stat(filepath.Join(globalRoot, "nested", "output.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected no write in global workspace, stat err=%v", err)
	}
	got, ok, err := store.GetTaskByID(ctx, db, task.ID)
	if err != nil {
		t.Fatalf("GetTaskByID: %v", err)
	}
	if !ok {
		t.Fatal("task not found")
	}
	if got.Status != schema.TaskStatusCompleted {
		t.Fatalf("status = %s, want completed", got.Status)
	}
	if got.WorkspaceID != workspace.ID {
		t.Fatalf("workspace_id = %q, want %q", got.WorkspaceID, workspace.ID)
	}
	if llmSeq.calls != 2 {
		t.Fatalf("chat calls = %d, want 2", llmSeq.calls)
	}
}

func TestRunner_Execute_BlocksProjectTaskWhenWorkspaceInactive(t *testing.T) {
	ctx := context.Background()
	db := store.InitTestDB(t)
	memBus := bus.NewMemBus(db)

	workspace := coderTestWorkspace("ws-inactive", schema.WorkspaceStatusInactive, []string{t.TempDir()})
	if err := store.SaveWorkspace(ctx, db, workspace); err != nil {
		t.Fatalf("SaveWorkspace: %v", err)
	}
	project := coderTestProject("proj-blocked", schema.ProjectKindCoding)
	if err := store.SaveProject(ctx, db, project); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	task := schema.NewTask("blocked write", "should not run", schema.RiskLow, schema.AgentCoder)
	task.DirectiveID = "dir-blocked"
	task.ProjectID = project.ID
	task.WorkspaceID = workspace.ID
	task.TaskClass = schema.TaskClassMutative
	directive := schema.NewDirective("Blocked task", schema.DirectiveModeAct, "test")
	directive.DirectiveID = task.DirectiveID
	if err := store.SaveDirective(ctx, db, directive); err != nil {
		t.Fatalf("SaveDirective: %v", err)
	}
	if err := store.PersistTask(ctx, db, task); err != nil {
		t.Fatalf("PersistTask: %v", err)
	}

	llmSeq := &sequenceLLM{}
	r := NewRunner(Config{
		WorkspaceDir:      t.TempDir(),
		LLM:               llmSeq,
		Model:             "chat",
		DB:                db,
		ExecutionRecorder: cognitive.StoreExecutionRecorder(db, nil),
		DirectiveWriter:   cognitive.StoreDirectiveWriter(db),
		Bus:               memBus,
	})

	err := r.Execute(ctx, task)
	if err == nil {
		t.Fatal("expected execution root error")
	}
	if !strings.Contains(err.Error(), "not active") {
		t.Fatalf("error = %v, want inactive workspace reason", err)
	}
	if llmSeq.calls != 0 {
		t.Fatalf("chat calls = %d, want 0", llmSeq.calls)
	}
	got, ok, err := store.GetTaskByID(ctx, db, task.ID)
	if err != nil {
		t.Fatalf("GetTaskByID: %v", err)
	}
	if !ok {
		t.Fatal("task not found")
	}
	if got.Status != schema.TaskStatusBlocked {
		t.Fatalf("status = %s, want blocked", got.Status)
	}
	if got.LifecyclePhase != "execution_blocked" {
		t.Fatalf("lifecycle_phase = %q", got.LifecyclePhase)
	}
	if !strings.Contains(got.BlockReason, "not active") {
		t.Fatalf("block_reason = %q", got.BlockReason)
	}
}

func coderTestWorkspace(id string, status schema.WorkspaceStatus, roots []string) schema.Workspace {
	return schema.Workspace{
		ID:         id,
		Name:       id,
		Kind:       schema.WorkspaceKindProject,
		Status:     status,
		LocalRoots: roots,
		AllowedActions: schema.AllowedActions{
			Read:    true,
			Write:   true,
			Create:  true,
			Modify:  true,
			Execute: true,
		},
		BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopeDeny},
		CreatedBy:      "tester",
	}
}

func coderTestProject(id string, kind schema.ProjectKind) schema.Project {
	return schema.Project{
		ID:         id,
		Title:      id,
		Slug:       id,
		Kind:       kind,
		Status:     schema.ProjectStatusActive,
		Health:     schema.ProjectHealthUnknown,
		CreatedBy:  "tester",
		Attributes: map[string]interface{}{},
	}
}
