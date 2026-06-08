package store

import (
	"context"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

func TestDetectProjectReferenceOrphans(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)

	if _, err := db.ExecContext(ctx, `
		CREATE TABLE navi_chats (
			chat_id TEXT PRIMARY KEY,
			owner_id TEXT NOT NULL DEFAULT '',
			project_id TEXT,
			title TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			visibility TEXT NOT NULL DEFAULT 'private',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			archived_at TEXT,
			deleted_at TEXT,
			message_count INTEGER NOT NULL DEFAULT 0,
			user_message_count INTEGER NOT NULL DEFAULT 0,
			assistant_message_count INTEGER NOT NULL DEFAULT 0,
			metadata_json TEXT NOT NULL DEFAULT '{}'
		)
	`); err != nil {
		t.Fatalf("create navi_chats: %v", err)
	}

	if err := SaveWorkspace(ctx, db, schema.Workspace{
		ID:             "ws-audit",
		Name:           "Audit Workspace",
		Kind:           schema.WorkspaceKindProject,
		Status:         schema.WorkspaceStatusActive,
		LocalRoots:     []string{"/workspace/audit"},
		AllowedActions: schema.AllowedActions{Read: true, Write: true, Create: true, Modify: true, RenameMove: true, Delete: false, Execute: false},
		BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		AuditEnabled:   true,
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      "owner",
		Metadata:       "{}",
	}); err != nil {
		t.Fatalf("SaveWorkspace: %v", err)
	}
	if err := SaveProject(ctx, db, schema.Project{
		ID:          "proj-real",
		Title:       "Real Project",
		Kind:        schema.ProjectKindGeneral,
		Status:      schema.ProjectStatusActive,
		Health:      schema.ProjectHealthUnknown,
		WorkspaceID: "ws-audit",
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "owner",
	}); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO artifacts (
			id, workspace_id, project_id, owner_id, canonical_title, display_title, type, subtype,
			schema_version, content_format, lifecycle_state, current_branch_id, current_version_id,
			head_version_number, created_by_actor_type, created_by_actor_id, provenance_root_id,
			attributes, created_at, updated_at, archived_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "artifact-orphan", "ws-audit", "proj-missing", "owner", "artifact-orphan", "artifact-orphan", "document", "md",
		"1.0", "text/markdown", string(schema.ArtifactLifecycleDraft), "main", "", 1, string(schema.ActorAgent), "navi",
		"artifact-orphan", "{}", now.Format(timeFormat), now.Format(timeFormat), nil); err != nil {
		t.Fatalf("insert orphan artifact: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE workspaces SET related_project_id = ? WHERE id = ?`, "proj-missing-workspace", "ws-audit"); err != nil {
		t.Fatalf("update workspace related_project_id: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO tasks (
			id, title, description, status, risk, assigned_to, directive_id, dependencies, surfaces,
			verification, cost, created_at, updated_at, project_id, workspace_id, task_class, raw_input, acceptance_target, lifecycle_phase, block_reason
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "task-orphan", "Task orphan", "task with orphan project", string(schema.TaskStatusPending), string(schema.RiskLow), "", "dir-1",
		"[]", "[]", "{}", "{}", now.Format(timeFormat), now.Format(timeFormat), "proj-missing-task", "", "", "", "", "", ""); err != nil {
		t.Fatalf("insert orphan task: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO navi_chats (chat_id, title, status, visibility, owner_id, created_at, updated_at, project_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, "sess-orphan", "Orphan Chat", "active", "private", "", now.Format(timeFormat), now.Format(timeFormat), "proj-missing-session"); err != nil {
		t.Fatalf("insert orphan chat: %v", err)
	}

	report, err := DetectProjectReferenceOrphans(ctx, db)
	if err != nil {
		t.Fatalf("DetectProjectReferenceOrphans: %v", err)
	}
	if len(report.Orphans) != 4 {
		t.Fatalf("expected 4 orphaned references, got %+v", report.Orphans)
	}

	got := map[string]string{}
	for _, orphan := range report.Orphans {
		got[orphan.Table+":"+orphan.RowID] = orphan.ProjectID
		if orphan.Reason == "" {
			t.Fatalf("expected reason for orphan %+v", orphan)
		}
	}
	want := map[string]string{
		"artifacts:artifact-orphan": "proj-missing",
		"tasks:task-orphan":         "proj-missing-task",
		"workspaces:ws-audit":       "proj-missing-workspace",
		"navi_chats:sess-orphan":    "proj-missing-session",
	}
	for key, projectID := range want {
		if got[key] != projectID {
			t.Fatalf("expected %s -> %s, got %q", key, projectID, got[key])
		}
	}
}

func TestDetectProjectReferenceOrphans_SkipsMissingOptionalTables(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)

	report, err := DetectProjectReferenceOrphans(ctx, db)
	if err != nil {
		t.Fatalf("DetectProjectReferenceOrphans: %v", err)
	}
	if report.HasOrphans() {
		t.Fatalf("expected no orphans, got %+v", report.Orphans)
	}
}
