package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

func TestProjectCRUDLifecycle(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()

	createdAt := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	project := schema.Project{
		ID:          "proj-1",
		Title:       " NAVI Coding Pass ",
		Description: "Initial implementation slice",
		Kind:        schema.ProjectKindCoding,
		Status:      schema.ProjectStatusDraft,
		Health:      schema.ProjectHealthOnTrack,
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
		CreatedBy:   "owner-1",
		Attributes:  map[string]interface{}{"phase": "mvp"},
	}

	if err := SaveProject(ctx, db, project); err != nil {
		t.Fatalf("SaveProject(create): %v", err)
	}

	fetched, err := GetProject(ctx, db, project.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if fetched.Title != "NAVI Coding Pass" {
		t.Fatalf("expected trimmed title, got %q", fetched.Title)
	}
	if fetched.Slug != "navi-coding-pass" {
		t.Fatalf("expected generated slug, got %q", fetched.Slug)
	}
	if fetched.Kind != schema.ProjectKindCoding || fetched.Status != schema.ProjectStatusDraft || fetched.Health != schema.ProjectHealthOnTrack {
		t.Fatalf("unexpected project classification: kind=%q status=%q health=%q", fetched.Kind, fetched.Status, fetched.Health)
	}
	if got := fetched.Attributes["phase"]; got != "mvp" {
		t.Fatalf("expected attributes to round-trip, got %#v", fetched.Attributes)
	}

	fetched.Title = "NAVI Project CRUD"
	fetched.Status = schema.ProjectStatusActive
	fetched.Health = schema.ProjectHealthAtRisk
	fetched.UpdatedAt = createdAt.Add(time.Hour)
	if err := SaveProject(ctx, db, fetched); err != nil {
		t.Fatalf("SaveProject(update): %v", err)
	}
	updated, err := GetProject(ctx, db, project.ID)
	if err != nil {
		t.Fatalf("GetProject(updated): %v", err)
	}
	if updated.Title != "NAVI Project CRUD" || updated.Status != schema.ProjectStatusActive || updated.Health != schema.ProjectHealthAtRisk {
		t.Fatalf("project update did not persist: %#v", updated)
	}

	active, err := ListProjects(ctx, db, schema.ProjectStatusActive, false)
	if err != nil {
		t.Fatalf("ListProjects(active): %v", err)
	}
	if len(active) != 1 || active[0].ID != project.ID {
		t.Fatalf("expected active project in list, got %#v", active)
	}

	if err := ArchiveProject(ctx, db, project.ID, createdAt.Add(2*time.Hour)); err != nil {
		t.Fatalf("ArchiveProject: %v", err)
	}
	defaultList, err := ListProjects(ctx, db, "", false)
	if err != nil {
		t.Fatalf("ListProjects(default): %v", err)
	}
	if len(defaultList) != 0 {
		t.Fatalf("expected archived project to be hidden by default, got %#v", defaultList)
	}
	archived, err := ListProjects(ctx, db, schema.ProjectStatusArchived, false)
	if err != nil {
		t.Fatalf("ListProjects(archived): %v", err)
	}
	if len(archived) != 1 || archived[0].Status != schema.ProjectStatusArchived {
		t.Fatalf("expected archived project, got %#v", archived)
	}
}

func TestProjectValidationErrors(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name    string
		project schema.Project
	}{
		{
			name: "invalid kind",
			project: schema.Project{
				ID: "proj-bad-kind", Title: "Bad Kind", Kind: schema.ProjectKind("software"),
				Status: schema.ProjectStatusActive, Health: schema.ProjectHealthUnknown, CreatedAt: now, UpdatedAt: now, CreatedBy: "owner-1",
			},
		},
		{
			name: "invalid status",
			project: schema.Project{
				ID: "proj-bad-status", Title: "Bad Status", Kind: schema.ProjectKindGeneral,
				Status: schema.ProjectStatus("deleted"), Health: schema.ProjectHealthUnknown, CreatedAt: now, UpdatedAt: now, CreatedBy: "owner-1",
			},
		},
		{
			name: "invalid health",
			project: schema.Project{
				ID: "proj-bad-health", Title: "Bad Health", Kind: schema.ProjectKindGeneral,
				Status: schema.ProjectStatusActive, Health: schema.ProjectHealth("fine"), CreatedAt: now, UpdatedAt: now, CreatedBy: "owner-1",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := SaveProject(ctx, db, tc.project)
			if err == nil || !IsValidationError(err) {
				t.Fatalf("expected validation error, got %v", err)
			}
		})
	}

	if err := SaveProject(ctx, db, schema.Project{
		ID: "proj-general", Title: "General Project", Kind: schema.ProjectKindGeneral,
		Status: schema.ProjectStatusActive, Health: schema.ProjectHealthUnknown, CreatedAt: now, UpdatedAt: now, CreatedBy: "owner-1",
	}); err != nil {
		t.Fatalf("general project without workspace should be allowed: %v", err)
	}
	if err := SaveProject(ctx, db, schema.Project{
		ID: "proj-coding", Title: "Coding Project", Kind: schema.ProjectKindCoding,
		Status: schema.ProjectStatusActive, Health: schema.ProjectHealthUnknown, CreatedAt: now, UpdatedAt: now, CreatedBy: "owner-1",
	}); err != nil {
		t.Fatalf("coding project without workspace should be allowed before readiness: %v", err)
	}
	if err := SaveProject(ctx, db, schema.Project{
		ID: "proj-missing-sandbox", Title: "Missing Sandbox", Kind: schema.ProjectKindCoding,
		Status: schema.ProjectStatusActive, Health: schema.ProjectHealthUnknown, SandboxProfileID: "missing", CreatedAt: now, UpdatedAt: now, CreatedBy: "owner-1",
	}); err == nil || !IsValidationError(err) {
		t.Fatalf("expected missing sandbox profile validation error, got %v", err)
	}
}

func TestProjectWorkspaceBindingRequiresExistingActiveWorkspace(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	activeWorkspace := projectTestWorkspace("ws-active", schema.WorkspaceStatusActive)
	inactiveWorkspace := projectTestWorkspace("ws-inactive", schema.WorkspaceStatusInactive)
	if err := SaveWorkspace(ctx, db, activeWorkspace); err != nil {
		t.Fatalf("SaveWorkspace(active): %v", err)
	}
	if err := SaveWorkspace(ctx, db, inactiveWorkspace); err != nil {
		t.Fatalf("SaveWorkspace(inactive): %v", err)
	}
	project := schema.Project{
		ID:        "proj-coding",
		Title:     "Coding Project",
		Kind:      schema.ProjectKindCoding,
		Status:    schema.ProjectStatusActive,
		Health:    schema.ProjectHealthUnknown,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "owner-1",
	}
	if err := SaveProject(ctx, db, project); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}

	if err := BindProjectWorkspace(ctx, db, project.ID, "ws-missing", now.Add(time.Minute)); err == nil || !IsValidationError(err) {
		t.Fatalf("expected missing workspace validation error, got %v", err)
	}
	if err := BindProjectWorkspace(ctx, db, project.ID, inactiveWorkspace.ID, now.Add(time.Minute)); err == nil || !IsValidationError(err) {
		t.Fatalf("expected inactive workspace validation error, got %v", err)
	}
	if err := BindProjectWorkspace(ctx, db, project.ID, activeWorkspace.ID, now.Add(time.Minute)); err != nil {
		t.Fatalf("BindProjectWorkspace(active): %v", err)
	}
	bound, err := GetProject(ctx, db, project.ID)
	if err != nil {
		t.Fatalf("GetProject(bound): %v", err)
	}
	if bound.WorkspaceID != activeWorkspace.ID {
		t.Fatalf("expected workspace binding %q, got %q", activeWorkspace.ID, bound.WorkspaceID)
	}
	if err := UnbindProjectWorkspace(ctx, db, project.ID, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("UnbindProjectWorkspace: %v", err)
	}
	unbound, err := GetProject(ctx, db, project.ID)
	if err != nil {
		t.Fatalf("GetProject(unbound): %v", err)
	}
	if strings.TrimSpace(unbound.WorkspaceID) != "" {
		t.Fatalf("expected cleared workspace binding, got %q", unbound.WorkspaceID)
	}
}

func projectTestWorkspace(id string, status schema.WorkspaceStatus) schema.Workspace {
	now := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	return schema.Workspace{
		ID:             id,
		Name:           "Workspace " + id,
		Kind:           schema.WorkspaceKindProject,
		Status:         status,
		LocalRoots:     []string{"/tmp/" + id},
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
		Tags:           nil,
		Metadata:       "{}",
	}
}
