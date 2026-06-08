package worldmodel

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

func TestWorldModelProjectReadiness(t *testing.T) {
	db := store.InitTestDB(t)
	wm := New(db)
	ctx := context.Background()
	now := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	general, err := wm.UpsertProject(ctx, schema.Project{
		ID:        "proj-general",
		Title:     "General Planning",
		Kind:      schema.ProjectKindGeneral,
		Status:    schema.ProjectStatusActive,
		Health:    schema.ProjectHealthUnknown,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "owner-1",
	})
	if err != nil {
		t.Fatalf("UpsertProject(general): %v", err)
	}
	generalReady, err := wm.ProjectReadiness(ctx, general.ID)
	if err != nil {
		t.Fatalf("ProjectReadiness(general): %v", err)
	}
	if !generalReady.ReadyForChat || !generalReady.ReadyForPlanning || generalReady.WorkspaceBindingRequired || generalReady.ReadyForCoding {
		t.Fatalf("unexpected general readiness: %#v", generalReady)
	}

	coding, err := wm.UpsertProject(ctx, schema.Project{
		ID:        "proj-coding",
		Title:     "Coding Work",
		Kind:      schema.ProjectKindCoding,
		Status:    schema.ProjectStatusActive,
		Health:    schema.ProjectHealthUnknown,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "owner-1",
	})
	if err != nil {
		t.Fatalf("UpsertProject(coding): %v", err)
	}
	codingReady, err := wm.ProjectReadiness(ctx, coding.ID)
	if err != nil {
		t.Fatalf("ProjectReadiness(coding no workspace): %v", err)
	}
	if codingReady.ReadyForCoding || !codingReady.WorkspaceBindingRequired || !containsReadinessReason(codingReady, "workspace binding") {
		t.Fatalf("expected coding project without workspace to degrade, got %#v", codingReady)
	}

	workspace := worldModelProjectTestWorkspace("ws-active", schema.WorkspaceStatusActive, []string{"/tmp/ws-active"})
	if err := store.SaveWorkspace(ctx, db, workspace); err != nil {
		t.Fatalf("SaveWorkspace(active): %v", err)
	}
	if _, err := wm.BindProjectWorkspace(ctx, coding.ID, workspace.ID); err != nil {
		t.Fatalf("BindProjectWorkspace: %v", err)
	}
	boundReady, err := wm.ProjectReadiness(ctx, coding.ID)
	if err != nil {
		t.Fatalf("ProjectReadiness(bound): %v", err)
	}
	if boundReady.ReadyForCoding || !containsReadinessReason(boundReady, "sandbox profile") {
		t.Fatalf("expected coding readiness to require sandbox profile, got %#v", boundReady)
	}

	profile := worldModelProjectTestSandboxProfile("local-docker-default", schema.SandboxProfileStatusActive)
	if err := store.SaveSandboxProfile(ctx, db, profile); err != nil {
		t.Fatalf("SaveSandboxProfile(active): %v", err)
	}
	coding.SandboxProfileID = profile.ID
	coding.WorkspaceID = workspace.ID
	coding, err = wm.UpsertProject(ctx, coding)
	if err != nil {
		t.Fatalf("UpsertProject(sandbox): %v", err)
	}
	sandboxReady, err := wm.ProjectReadiness(ctx, coding.ID)
	if err != nil {
		t.Fatalf("ProjectReadiness(sandbox): %v", err)
	}
	if !sandboxReady.ReadyForCoding || len(sandboxReady.DegradedReasons) != 0 || sandboxReady.SandboxProfileID != profile.ID {
		t.Fatalf("expected coding readiness with workspace and sandbox profile, got %#v", sandboxReady)
	}

	workspace.Status = schema.WorkspaceStatusInactive
	workspace.UpdatedAt = workspace.UpdatedAt.Add(time.Minute)
	if err := store.SaveWorkspace(ctx, db, workspace); err != nil {
		t.Fatalf("SaveWorkspace(inactive): %v", err)
	}
	inactiveReady, err := wm.ProjectReadiness(ctx, coding.ID)
	if err != nil {
		t.Fatalf("ProjectReadiness(inactive): %v", err)
	}
	if inactiveReady.ReadyForCoding || !containsReadinessReason(inactiveReady, "not active") {
		t.Fatalf("expected inactive workspace to degrade readiness, got %#v", inactiveReady)
	}
	resolved, source, err := wm.ResolveActiveWorkspace(ctx, coding.ID)
	if err != nil {
		t.Fatalf("ResolveActiveWorkspace(inactive project binding): %v", err)
	}
	if resolved != nil || source != "none" {
		t.Fatalf("inactive project workspace should not resolve as active, got workspace=%#v source=%q", resolved, source)
	}

	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatalf("disable foreign keys: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE projects SET workspace_id = ? WHERE id = ?`, "ws-missing", coding.ID); err != nil {
		t.Fatalf("force missing workspace binding: %v", err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys=ON`); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	missingReady, err := wm.ProjectReadiness(ctx, coding.ID)
	if err != nil {
		t.Fatalf("ProjectReadiness(missing): %v", err)
	}
	if missingReady.ReadyForCoding || !containsReadinessReason(missingReady, "not found") {
		t.Fatalf("expected missing workspace to degrade readiness, got %#v", missingReady)
	}
}

func TestWorldModelGetProjectWorkspaceFallsBackToLegacyBinding(t *testing.T) {
	db := store.InitTestDB(t)
	wm := New(db)
	ctx := context.Background()

	if _, err := wm.UpsertProject(ctx, schema.Project{
		ID:        "legacy-project",
		Title:     "Legacy Project",
		Kind:      schema.ProjectKindGeneral,
		Status:    schema.ProjectStatusActive,
		Health:    schema.ProjectHealthUnknown,
		CreatedAt: time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
		CreatedBy: "owner-1",
	}); err != nil {
		t.Fatalf("UpsertProject(legacy): %v", err)
	}
	legacy := worldModelProjectTestWorkspace("ws-legacy", schema.WorkspaceStatusActive, []string{"/tmp/ws-legacy"})
	legacy.RelatedProjectID = "legacy-project"
	if err := store.SaveWorkspace(ctx, db, legacy); err != nil {
		t.Fatalf("SaveWorkspace(legacy): %v", err)
	}

	resolved, err := wm.GetProjectWorkspace(ctx, "legacy-project")
	if err != nil {
		t.Fatalf("GetProjectWorkspace legacy fallback: %v", err)
	}
	if resolved.ID != legacy.ID {
		t.Fatalf("expected legacy workspace %q, got %q", legacy.ID, resolved.ID)
	}
}

func TestWorldModelActiveProjectResolution(t *testing.T) {
	db := store.InitTestDB(t)
	wm := New(db)
	ctx := context.Background()
	now := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	if _, err := wm.UpsertProject(ctx, schema.Project{
		ID:        "proj-active",
		Title:     "Active Project",
		Kind:      schema.ProjectKindGeneral,
		Status:    schema.ProjectStatusActive,
		Health:    schema.ProjectHealthUnknown,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "owner-1",
	}); err != nil {
		t.Fatalf("UpsertProject: %v", err)
	}
	if err := wm.SetActiveProjectID(ctx, "proj-active"); err != nil {
		t.Fatalf("SetActiveProjectID: %v", err)
	}
	if got := wm.GetActiveProjectID(ctx); got != "proj-active" {
		t.Fatalf("GetActiveProjectID = %q", got)
	}
	project, source, err := wm.ResolveActiveProject(ctx, "")
	if err != nil {
		t.Fatalf("ResolveActiveProject: %v", err)
	}
	if project == nil || project.ID != "proj-active" || source != "active" {
		t.Fatalf("unexpected active project resolution: project=%#v source=%q", project, source)
	}
	if err := wm.SetActiveProjectID(ctx, "missing"); err == nil {
		t.Fatalf("expected missing active project to fail")
	}
}

func TestWorldModelCreateProjectTaskGatesMutativeWork(t *testing.T) {
	db := store.InitTestDB(t)
	wm := New(db)
	ctx := context.Background()
	now := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	if _, err := wm.UpsertProject(ctx, schema.Project{
		ID:        "proj-coding-task",
		Title:     "Coding Task Project",
		Kind:      schema.ProjectKindCoding,
		Status:    schema.ProjectStatusActive,
		Health:    schema.ProjectHealthUnknown,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "owner-1",
	}); err != nil {
		t.Fatalf("UpsertProject: %v", err)
	}

	blocked, err := wm.CreateProjectTask(ctx, "proj-coding-task", schema.Task{
		Title:            "Mutate code",
		Description:      "Change code",
		RawInput:         "Change code",
		TaskClass:        schema.TaskClassMutative,
		Risk:             schema.RiskLow,
		AssignedTo:       schema.AgentNavi,
		AcceptanceTarget: "tests pass",
	})
	if err != nil {
		t.Fatalf("CreateProjectTask(blocked): %v", err)
	}
	if blocked.Status != schema.TaskStatusBlocked || blocked.BlockReason == "" {
		t.Fatalf("expected blocked mutative coding task, got %+v", blocked)
	}

	workspace := worldModelProjectTestWorkspace("ws-task", schema.WorkspaceStatusActive, []string{"/tmp/ws-task"})
	if err := store.SaveWorkspace(ctx, db, workspace); err != nil {
		t.Fatalf("SaveWorkspace: %v", err)
	}
	if _, err := wm.BindProjectWorkspace(ctx, "proj-coding-task", workspace.ID); err != nil {
		t.Fatalf("BindProjectWorkspace: %v", err)
	}
	stillBlocked, err := wm.CreateProjectTask(ctx, "proj-coding-task", schema.Task{
		Title:            "Mutate code with workspace only",
		Description:      "Change code",
		RawInput:         "Change code",
		TaskClass:        schema.TaskClassMutative,
		Risk:             schema.RiskLow,
		AssignedTo:       schema.AgentNavi,
		AcceptanceTarget: "tests pass",
	})
	if err != nil {
		t.Fatalf("CreateProjectTask(workspace only): %v", err)
	}
	if stillBlocked.Status != schema.TaskStatusBlocked || !strings.Contains(stillBlocked.BlockReason, "sandbox profile") {
		t.Fatalf("expected sandbox-gated blocked task, got %+v", stillBlocked)
	}
	profile := worldModelProjectTestSandboxProfile("local-docker-task", schema.SandboxProfileStatusActive)
	if err := store.SaveSandboxProfile(ctx, db, profile); err != nil {
		t.Fatalf("SaveSandboxProfile: %v", err)
	}
	project, err := wm.GetProject(ctx, "proj-coding-task")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	project.SandboxProfileID = profile.ID
	if _, err := wm.UpsertProject(ctx, project); err != nil {
		t.Fatalf("UpsertProject(sandbox): %v", err)
	}
	pending, err := wm.CreateProjectTask(ctx, "proj-coding-task", schema.Task{
		Title:            "Mutate code with workspace",
		Description:      "Change code",
		RawInput:         "Change code",
		TaskClass:        schema.TaskClassMutative,
		Risk:             schema.RiskLow,
		AssignedTo:       schema.AgentNavi,
		AcceptanceTarget: "tests pass",
	})
	if err != nil {
		t.Fatalf("CreateProjectTask(pending): %v", err)
	}
	if pending.Status != schema.TaskStatusPending || pending.WorkspaceID != workspace.ID {
		t.Fatalf("expected pending workspace-bound task, got %+v", pending)
	}
}

func worldModelProjectTestSandboxProfile(id string, status schema.SandboxProfileStatus) schema.SandboxProfile {
	now := time.Date(2026, 4, 1, 9, 30, 0, 0, time.UTC)
	return schema.SandboxProfile{
		ID:                   id,
		Name:                 "Sandbox " + id,
		Runtime:              schema.SandboxRuntimeDocker,
		Status:               status,
		Image:                "golang:1.22",
		NetworkMode:          schema.SandboxNetworkNone,
		WorkspaceMountTarget: "/workspace",
		CommandAllowlist:     []string{"go", "npm"},
		EnvAllowlist:         []string{"CI"},
		DefaultTimeoutMS:     30000,
		MaxTimeoutMS:         120000,
		CreatedAt:            now,
		UpdatedAt:            now,
		CreatedBy:            "owner-1",
		Metadata:             "{}",
	}
}

func worldModelProjectTestWorkspace(id string, status schema.WorkspaceStatus, roots []string) schema.Workspace {
	now := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	return schema.Workspace{
		ID:             id,
		Name:           "Workspace " + id,
		Kind:           schema.WorkspaceKindProject,
		Status:         status,
		LocalRoots:     roots,
		RepoRoots:      nil,
		ProtectedPaths: nil,
		AllowedActions: schema.AllowedActions{
			Read: true, Write: true, Create: true, Modify: true, RenameMove: true, Delete: false, Execute: false,
		},
		BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		AuditEnabled:   true,
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      "owner-1",
		Metadata:       "{}",
	}
}

func containsReadinessReason(readiness schema.ProjectReadiness, needle string) bool {
	for _, reason := range readiness.DegradedReasons {
		if strings.Contains(reason, needle) {
			return true
		}
	}
	return false
}
