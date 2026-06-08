package navi

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/navi/filetools"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
	navitool "github.com/open-navi/navi/internal/tool"
	"github.com/open-navi/navi/internal/worldmodel"
)

func TestMaybeMaterializeArtifactOutputs_FileWriteCreatesArtifactAndHistory(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	seedMaterializationWorkspace(t, ctx, db)
	wm := worldmodel.New(db)
	loop := NewAgentLoop(LoopConfig{
		WorldModel:     wm,
		ResolveOwnerID: func(ctx context.Context, chatID string) string { return "owner-1" },
		SaveExecutionOutcome: func(ctx context.Context, eo schema.ExecutionOutcome) error {
			return store.SaveExecutionOutcome(ctx, db, eo)
		},
	})

	refs, err := loop.maybeMaterializeArtifactOutputs(ctx, "sess-1", nil, llm.ToolCall{
		ID:   "tc-file",
		Name: filetools.WriteFileToolName,
		Arguments: map[string]any{
			"path":    "notes/report.md",
			"content": "# report\nbody",
		},
	}, &navitool.Tool{Name: filetools.WriteFileToolName, Source: navitool.ToolSourceFileTools}, "created notes/report.md")
	if err != nil {
		t.Fatalf("maybeMaterializeArtifactOutputs: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 artifact ref, got %d", len(refs))
	}
	artifact, err := wm.GetArtifact(ctx, "file:owner-1:notes/report.md")
	if err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}
	if artifact == nil {
		t.Fatal("expected artifact to exist")
	}
	if artifact.Status != worldmodel.ArtifactStatusReviewReady {
		t.Fatalf("expected review_ready status, got %q", artifact.Status)
	}
	versions, err := wm.ListArtifactVersions(ctx, artifact.ID, 10)
	if err != nil {
		t.Fatalf("ListArtifactVersions: %v", err)
	}
	if len(versions) != 1 || versions[0].Version != 1 {
		t.Fatalf("expected one committed version, got %+v", versions)
	}
	if artifact.SourceExecutionID == "" {
		t.Fatal("expected source execution id to be populated")
	}
	eo, err := store.GetExecutionOutcomeByAttemptID(ctx, db, artifact.SourceExecutionID)
	if err != nil {
		t.Fatalf("GetExecutionOutcomeByAttemptID: %v", err)
	}
	if eo == nil || eo.Outcome != schema.ExecutionOutcomeSucceeded {
		t.Fatalf("expected successful materialization history, got %+v", eo)
	}
}

func TestMaybeMaterializeArtifactOutputs_SkillLifecycleCreatesCheckpointVersions(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	seedMaterializationWorkspace(t, ctx, db)
	wm := worldmodel.New(db)
	loop := NewAgentLoop(LoopConfig{
		WorldModel:     wm,
		ResolveOwnerID: func(ctx context.Context, chatID string) string { return "owner-1" },
		SaveExecutionOutcome: func(ctx context.Context, eo schema.ExecutionOutcome) error {
			return store.SaveExecutionOutcome(ctx, db, eo)
		},
	})
	tool := &navitool.Tool{Name: "draft_skill_run", Source: navitool.ToolSourceSkill, Metadata: navitool.ToolMetadata{SkillName: "draft-skill"}}

	for _, tc := range []struct {
		status   string
		snapshot string
	}{
		{status: worldmodel.ArtifactStatusDraft, snapshot: `{"step":"draft"}`},
		{status: worldmodel.ArtifactStatusInProgress, snapshot: `{"step":"checkpoint"}`},
		{status: worldmodel.ArtifactStatusReviewReady, snapshot: `{"step":"final"}`},
	} {
		_, err := loop.maybeMaterializeArtifactOutputs(ctx, "sess-1", nil, llm.ToolCall{
			ID:   "tc-skill-" + tc.status,
			Name: "draft_skill_run",
		}, tool, map[string]any{
			"artifact": map[string]any{
				"artifact_id": "artifact:owner-1:workflow-report",
				"kind":        "document",
				"location":    "artifact://workflow-report",
				"description": "Workflow report",
				"status":      tc.status,
				"snapshot":    tc.snapshot,
			},
		})
		if err != nil {
			t.Fatalf("status %s: %v", tc.status, err)
		}
	}

	artifact, err := wm.GetArtifact(ctx, "artifact:owner-1:workflow-report")
	if err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}
	if artifact == nil || artifact.Version != 3 || artifact.Status != worldmodel.ArtifactStatusReviewReady {
		t.Fatalf("unexpected artifact final state: %+v", artifact)
	}
	versions, err := wm.ListArtifactVersions(ctx, artifact.ID, 10)
	if err != nil {
		t.Fatalf("ListArtifactVersions: %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(versions))
	}
	if versions[0].Status != worldmodel.ArtifactStatusReviewReady || versions[1].Status != worldmodel.ArtifactStatusInProgress || versions[2].Status != worldmodel.ArtifactStatusDraft {
		t.Fatalf("unexpected version lifecycle: %+v", versions)
	}
}

func TestMaybeMaterializeArtifactOutputs_InvalidSkillArtifactPreservesRecoverableOutput(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	seedMaterializationWorkspace(t, ctx, db)
	wm := worldmodel.New(db)
	loop := NewAgentLoop(LoopConfig{
		WorldModel:     wm,
		ResolveOwnerID: func(ctx context.Context, chatID string) string { return "owner-1" },
		SaveExecutionOutcome: func(ctx context.Context, eo schema.ExecutionOutcome) error {
			return store.SaveExecutionOutcome(ctx, db, eo)
		},
	})
	tool := &navitool.Tool{Name: "broken_skill_run", Source: navitool.ToolSourceSkill, Metadata: navitool.ToolMetadata{SkillName: "broken-skill"}}

	_, err = loop.maybeMaterializeArtifactOutputs(ctx, "sess-1", nil, llm.ToolCall{
		ID:   "tc-bad",
		Name: "broken_skill_run",
	}, tool, map[string]any{
		"artifact": map[string]any{
			"artifact_id": "artifact:owner-1:broken",
			"kind":        "unsupported",
			"description": "Broken output",
		},
		"payload": map[string]any{"draft": true},
	})
	if err == nil {
		t.Fatal("expected invalid materialization to fail")
	}
	artifact, err := wm.GetArtifact(ctx, "artifact:owner-1:broken")
	if err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}
	if artifact == nil {
		t.Fatal("expected failed artifact placeholder")
	}
	if artifact.Status != worldmodel.ArtifactStatusFailed {
		t.Fatalf("expected failed artifact status, got %q", artifact.Status)
	}
	if artifact.RecoverableOutput == "" {
		t.Fatal("expected recoverable output to be preserved")
	}
	eo, err := store.GetExecutionOutcomeByAttemptID(ctx, db, artifact.SourceExecutionID)
	if err != nil {
		t.Fatalf("GetExecutionOutcomeByAttemptID: %v", err)
	}
	if eo == nil || eo.Outcome != schema.ExecutionOutcomeFailed {
		t.Fatalf("expected failed history record, got %+v", eo)
	}
}

func seedMaterializationWorkspace(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	if err := store.SaveWorkspace(ctx, db, schema.Workspace{
		ID:             "ws-default",
		Name:           "Materialization Workspace",
		Kind:           schema.WorkspaceKindProject,
		Status:         schema.WorkspaceStatusActive,
		LocalRoots:     []string{"/workspace/materialization"},
		AllowedActions: schema.AllowedActions{Read: true, Write: true, Create: true, Modify: true, RenameMove: true, Delete: false, Execute: false},
		BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		AuditEnabled:   true,
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      "owner-1",
		Metadata:       "{}",
	}); err != nil {
		t.Fatalf("SaveWorkspace(ws-default): %v", err)
	}
}
