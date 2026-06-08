package store

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/schema"
)

func TestWorkspaceCRUDAndWhitelistLifecycle(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()

	createdAt := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(5 * time.Minute)
	if err := SaveProject(ctx, db, schema.Project{
		ID:        "proj-1",
		Title:     "Workspace Project",
		Kind:      schema.ProjectKindGeneral,
		Status:    schema.ProjectStatusActive,
		Health:    schema.ProjectHealthUnknown,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
		CreatedBy: "owner-1",
	}); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	workspace := schema.Workspace{
		ID:             "ws-1",
		Name:           " Test Workspace ",
		Description:    "A test workspace",
		Kind:           schema.WorkspaceKindProject,
		Status:         schema.WorkspaceStatusActive,
		LocalRoots:     []string{"/tmp/navi-test", "/tmp/navi-test"},
		RepoRoots:      []string{"github.com/ceoai/navi", " github.com/ceoai/navi "},
		ProtectedPaths: []string{"/tmp/navi-test/secrets", "/tmp/navi-test/secrets"},
		AllowedActions: schema.AllowedActions{
			Read: true, Write: true, Create: true, Modify: true, RenameMove: true, Delete: false, Execute: false,
		},
		BoundaryPolicy:   schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		AuditEnabled:     true,
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
		CreatedBy:        "owner-1",
		Tags:             []string{"alpha", " alpha "},
		RelatedProjectID: "proj-1",
		Notes:            "Primary workspace",
		Metadata:         `{"tier":"gold"}`,
	}

	if err := SaveWorkspace(ctx, db, workspace); err != nil {
		t.Fatalf("SaveWorkspace(create): %v", err)
	}

	fetched, err := GetWorkspace(ctx, db, workspace.ID)
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	if fetched.Name != "Test Workspace" {
		t.Fatalf("expected trimmed name, got %q", fetched.Name)
	}
	if !reflect.DeepEqual(fetched.LocalRoots, []string{"/tmp/navi-test"}) {
		t.Fatalf("unexpected local_roots: %#v", fetched.LocalRoots)
	}
	if !reflect.DeepEqual(fetched.RepoRoots, []string{"github.com/ceoai/navi"}) {
		t.Fatalf("unexpected repo_roots: %#v", fetched.RepoRoots)
	}
	if !reflect.DeepEqual(fetched.Tags, []string{"alpha"}) {
		t.Fatalf("unexpected tags: %#v", fetched.Tags)
	}
	if fetched.Metadata != `{"tier":"gold"}` {
		t.Fatalf("unexpected metadata: %s", fetched.Metadata)
	}
	if fetched.BoundaryPolicy.OutOfScopeDefault != schema.BoundaryPolicyOutOfScopePrompt {
		t.Fatalf("unexpected boundary policy: %q", fetched.BoundaryPolicy.OutOfScopeDefault)
	}

	workspace.Name = "Updated Workspace"
	workspace.UpdatedAt = updatedAt.Add(10 * time.Minute)
	workspace.BoundaryPolicy.OutOfScopeDefault = schema.BoundaryPolicyOutOfScopeDeny
	workspace.Metadata = `{"tier":"platinum"}`
	if err := SaveWorkspace(ctx, db, workspace); err != nil {
		t.Fatalf("SaveWorkspace(update): %v", err)
	}

	updated, err := GetWorkspace(ctx, db, workspace.ID)
	if err != nil {
		t.Fatalf("GetWorkspace(updated): %v", err)
	}
	if updated.Name != "Updated Workspace" {
		t.Fatalf("expected updated name, got %q", updated.Name)
	}
	if updated.BoundaryPolicy.OutOfScopeDefault != schema.BoundaryPolicyOutOfScopeDeny {
		t.Fatalf("expected deny policy, got %q", updated.BoundaryPolicy.OutOfScopeDefault)
	}

	activeList, err := ListWorkspaces(ctx, db, string(schema.WorkspaceStatusActive))
	if err != nil {
		t.Fatalf("ListWorkspaces(active): %v", err)
	}
	if len(activeList) != 1 {
		t.Fatalf("expected 1 active workspace, got %d", len(activeList))
	}

	byProject, err := GetWorkspaceByProjectID(ctx, db, "proj-1")
	if err != nil {
		t.Fatalf("GetWorkspaceByProjectID: %v", err)
	}
	if byProject.ID != workspace.ID {
		t.Fatalf("expected project-bound workspace %s, got %s", workspace.ID, byProject.ID)
	}

	expiresAt := createdAt.Add(24 * time.Hour)
	rule := schema.WhitelistRule{
		RuleID:      "rule-1",
		Scope:       "/tmp/outside",
		ActionTypes: []string{"read", "write", "read"},
		Status:      schema.WhitelistRuleStatusActive,
		CreatedAt:   createdAt,
		ExpiresAt:   &expiresAt,
		CreatedBy:   "owner-1",
	}
	if err := SaveWhitelistRule(ctx, db, rule, workspace.ID); err != nil {
		t.Fatalf("SaveWhitelistRule(active): %v", err)
	}

	withRule, err := GetWorkspace(ctx, db, workspace.ID)
	if err != nil {
		t.Fatalf("GetWorkspace(with rule): %v", err)
	}
	if len(withRule.WhitelistRules) != 1 {
		t.Fatalf("expected 1 whitelist rule, got %d", len(withRule.WhitelistRules))
	}
	if !reflect.DeepEqual(withRule.WhitelistRules[0].ActionTypes, []string{"read", "write"}) {
		t.Fatalf("unexpected whitelist action types: %#v", withRule.WhitelistRules[0].ActionTypes)
	}

	revokedAt := createdAt.Add(2 * time.Hour)
	rule.Status = schema.WhitelistRuleStatusRevoked
	rule.RevokedAt = &revokedAt
	if err := SaveWhitelistRule(ctx, db, rule, workspace.ID); err != nil {
		t.Fatalf("SaveWhitelistRule(revoked): %v", err)
	}

	rules, err := ListWhitelistRules(ctx, db, workspace.ID)
	if err != nil {
		t.Fatalf("ListWhitelistRules: %v", err)
	}
	if len(rules) != 1 || rules[0].Status != schema.WhitelistRuleStatusRevoked || rules[0].RevokedAt == nil {
		t.Fatalf("expected revoked whitelist rule, got %#v", rules)
	}

	archiveTime := createdAt.Add(48 * time.Hour)
	if err := ArchiveWorkspace(ctx, db, workspace.ID, archiveTime); err != nil {
		t.Fatalf("ArchiveWorkspace: %v", err)
	}

	archived, err := GetWorkspace(ctx, db, workspace.ID)
	if err != nil {
		t.Fatalf("GetWorkspace(archived): %v", err)
	}
	if archived.Status != schema.WorkspaceStatusArchived {
		t.Fatalf("expected archived status, got %q", archived.Status)
	}
	if !archived.UpdatedAt.Equal(archiveTime) {
		t.Fatalf("expected updated_at %s, got %s", archiveTime, archived.UpdatedAt)
	}

	activeList, err = ListWorkspaces(ctx, db, string(schema.WorkspaceStatusActive))
	if err != nil {
		t.Fatalf("ListWorkspaces(active after archive): %v", err)
	}
	if len(activeList) != 0 {
		t.Fatalf("expected no active workspaces after archive, got %d", len(activeList))
	}
}

func TestGetWorkspaceIDUsesExplicitSelectionOnly(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()
	baseTime := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)

	for _, ws := range []schema.Workspace{
		{
			ID:             "ws-1",
			Name:           "Workspace One",
			Kind:           schema.WorkspaceKindGeneral,
			Status:         schema.WorkspaceStatusActive,
			BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
			CreatedAt:      baseTime,
			UpdatedAt:      baseTime,
			CreatedBy:      "owner",
		},
		{
			ID:             "ws-2",
			Name:           "Workspace Two",
			Kind:           schema.WorkspaceKindGeneral,
			Status:         schema.WorkspaceStatusActive,
			BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
			CreatedAt:      baseTime,
			UpdatedAt:      baseTime,
			CreatedBy:      "owner",
		},
	} {
		if err := SaveWorkspace(ctx, db, ws); err != nil {
			t.Fatalf("SaveWorkspace(%s): %v", ws.ID, err)
		}
	}

	workspaceID, err := GetWorkspaceID(ctx, db)
	if err != nil {
		t.Fatalf("GetWorkspaceID(no selection): %v", err)
	}
	if workspaceID != "" {
		t.Fatalf("expected no silent workspace selection, got %q", workspaceID)
	}

	if err := SaveConfigurationEntry(ctx, db, schema.ConfigurationEntry{
		ID:        "active_workspace_id",
		Scope:     "owner",
		ScopeID:   "active",
		Key:       "active_workspace_id",
		Value:     "ws-2",
		Source:    "explicit",
		CreatedAt: baseTime,
		UpdatedAt: baseTime,
	}); err != nil {
		t.Fatalf("SaveConfigurationEntry(active_workspace_id): %v", err)
	}

	workspaceID, err = GetWorkspaceID(ctx, db)
	if err != nil {
		t.Fatalf("GetWorkspaceID(selected): %v", err)
	}
	if workspaceID != "ws-2" {
		t.Fatalf("expected explicit workspace selection ws-2, got %q", workspaceID)
	}
}

func TestWorkspaceValidationErrors(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()
	baseTime := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		ws   schema.Workspace
	}{
		{
			name: "invalid kind",
			ws: schema.Workspace{
				ID:             "ws-invalid-kind",
				Name:           "Bad Kind",
				Kind:           schema.WorkspaceKind("global"),
				Status:         schema.WorkspaceStatusActive,
				BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
				CreatedAt:      baseTime,
				UpdatedAt:      baseTime,
				CreatedBy:      "owner",
			},
		},
		{
			name: "invalid status",
			ws: schema.Workspace{
				ID:             "ws-invalid-status",
				Name:           "Bad Status",
				Kind:           schema.WorkspaceKindGeneral,
				Status:         schema.WorkspaceStatus("deleted"),
				BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
				CreatedAt:      baseTime,
				UpdatedAt:      baseTime,
				CreatedBy:      "owner",
			},
		},
		{
			name: "invalid boundary policy",
			ws: schema.Workspace{
				ID:             "ws-invalid-policy",
				Name:           "Bad Policy",
				Kind:           schema.WorkspaceKindGeneral,
				Status:         schema.WorkspaceStatusActive,
				BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopeDefault("allow")},
				CreatedAt:      baseTime,
				UpdatedAt:      baseTime,
				CreatedBy:      "owner",
			},
		},
		{
			name: "invalid metadata",
			ws: schema.Workspace{
				ID:             "ws-invalid-metadata",
				Name:           "Bad Metadata",
				Kind:           schema.WorkspaceKindGeneral,
				Status:         schema.WorkspaceStatusActive,
				BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
				CreatedAt:      baseTime,
				UpdatedAt:      baseTime,
				CreatedBy:      "owner",
				Metadata:       "{",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := SaveWorkspace(ctx, db, tc.ws)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !IsValidationError(err) {
				t.Fatalf("expected ValidationError, got %T: %v", err, err)
			}
		})
	}

	if _, err := ListWorkspaces(ctx, db, "bogus"); err == nil || !IsValidationError(err) {
		t.Fatalf("expected validation error for invalid status filter, got %v", err)
	}
	if err := ArchiveWorkspace(ctx, db, "", time.Time{}); err == nil || !IsValidationError(err) {
		t.Fatalf("expected validation error for empty archive id, got %v", err)
	}
}

func TestSaveWorkspace_RejectsDuplicateActiveProjectBinding(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()
	baseTime := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)

	if err := SaveProject(ctx, db, schema.Project{
		ID:        "proj-shared",
		Title:     "Shared Project",
		Kind:      schema.ProjectKindGeneral,
		Status:    schema.ProjectStatusActive,
		Health:    schema.ProjectHealthUnknown,
		CreatedAt: baseTime,
		UpdatedAt: baseTime,
		CreatedBy: "owner",
	}); err != nil {
		t.Fatalf("SaveProject(proj-shared): %v", err)
	}

	first := schema.Workspace{
		ID:               "ws-project-1",
		Name:             "Project One",
		Kind:             schema.WorkspaceKindProject,
		Status:           schema.WorkspaceStatusActive,
		BoundaryPolicy:   schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		CreatedAt:        baseTime,
		UpdatedAt:        baseTime,
		CreatedBy:        "owner",
		RelatedProjectID: "proj-shared",
	}
	if err := SaveWorkspace(ctx, db, first); err != nil {
		t.Fatalf("SaveWorkspace(first): %v", err)
	}

	second := schema.Workspace{
		ID:               "ws-project-2",
		Name:             "Project Two",
		Kind:             schema.WorkspaceKindProject,
		Status:           schema.WorkspaceStatusActive,
		BoundaryPolicy:   schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		CreatedAt:        baseTime,
		UpdatedAt:        baseTime,
		CreatedBy:        "owner",
		RelatedProjectID: "proj-shared",
	}
	err := SaveWorkspace(ctx, db, second)
	if err == nil {
		t.Fatal("expected duplicate active project binding to be rejected")
	}
	if !IsValidationError(err) {
		t.Fatalf("expected validation error, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "already bound") {
		t.Fatalf("expected duplicate binding message, got %v", err)
	}
}

func TestSaveWorkspace_AllowsArchivedWorkspaceToReuseProjectBinding(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()
	baseTime := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)

	if err := SaveProject(ctx, db, schema.Project{
		ID:        "proj-reused",
		Title:     "Reused Project",
		Kind:      schema.ProjectKindGeneral,
		Status:    schema.ProjectStatusActive,
		Health:    schema.ProjectHealthUnknown,
		CreatedAt: baseTime,
		UpdatedAt: baseTime,
		CreatedBy: "owner",
	}); err != nil {
		t.Fatalf("SaveProject(proj-reused): %v", err)
	}

	archived := schema.Workspace{
		ID:               "ws-archived-project",
		Name:             "Archived Project",
		Kind:             schema.WorkspaceKindProject,
		Status:           schema.WorkspaceStatusArchived,
		BoundaryPolicy:   schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		CreatedAt:        baseTime,
		UpdatedAt:        baseTime,
		CreatedBy:        "owner",
		RelatedProjectID: "proj-reused",
	}
	if err := SaveWorkspace(ctx, db, archived); err != nil {
		t.Fatalf("SaveWorkspace(archived): %v", err)
	}

	active := schema.Workspace{
		ID:               "ws-active-project",
		Name:             "Active Project",
		Kind:             schema.WorkspaceKindProject,
		Status:           schema.WorkspaceStatusActive,
		BoundaryPolicy:   schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		CreatedAt:        baseTime,
		UpdatedAt:        baseTime.Add(time.Minute),
		CreatedBy:        "owner",
		RelatedProjectID: "proj-reused",
	}
	if err := SaveWorkspace(ctx, db, active); err != nil {
		t.Fatalf("SaveWorkspace(active): %v", err)
	}

	resolved, err := GetWorkspaceByProjectID(ctx, db, "proj-reused")
	if err != nil {
		t.Fatalf("GetWorkspaceByProjectID: %v", err)
	}
	if resolved.ID != active.ID {
		t.Fatalf("expected active workspace %q, got %q", active.ID, resolved.ID)
	}
}

func TestWhitelistRuleValidationErrors(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()
	baseTime := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)

	workspace := schema.Workspace{
		ID:             "ws-rules",
		Name:           "Rules",
		Kind:           schema.WorkspaceKindGeneral,
		Status:         schema.WorkspaceStatusActive,
		BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		CreatedAt:      baseTime,
		UpdatedAt:      baseTime,
		CreatedBy:      "owner",
	}
	if err := SaveWorkspace(ctx, db, workspace); err != nil {
		t.Fatalf("SaveWorkspace: %v", err)
	}

	tests := []struct {
		name string
		rule schema.WhitelistRule
	}{
		{
			name: "unsupported action type",
			rule: schema.WhitelistRule{
				RuleID:      "rule-bad-action",
				Scope:       "/tmp/outside",
				ActionTypes: []string{"deploy"},
				Status:      schema.WhitelistRuleStatusActive,
				CreatedAt:   baseTime,
				CreatedBy:   "owner",
			},
		},
		{
			name: "revoked without revoked_at",
			rule: schema.WhitelistRule{
				RuleID:      "rule-no-revoked-at",
				Scope:       "/tmp/outside",
				ActionTypes: []string{"read"},
				Status:      schema.WhitelistRuleStatusRevoked,
				CreatedAt:   baseTime,
				CreatedBy:   "owner",
			},
		},
		{
			name: "expires before created_at",
			rule: func() schema.WhitelistRule {
				expiresAt := baseTime.Add(-time.Minute)
				return schema.WhitelistRule{
					RuleID:      "rule-expired",
					Scope:       "/tmp/outside",
					ActionTypes: []string{"read"},
					Status:      schema.WhitelistRuleStatusActive,
					CreatedAt:   baseTime,
					ExpiresAt:   &expiresAt,
					CreatedBy:   "owner",
				}
			}(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := SaveWhitelistRule(ctx, db, tc.rule, workspace.ID)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !IsValidationError(err) {
				t.Fatalf("expected ValidationError, got %T: %v", err, err)
			}
		})
	}

	if _, err := ListWhitelistRules(ctx, db, ""); err == nil || !IsValidationError(err) {
		t.Fatalf("expected validation error for empty workspace id, got %v", err)
	}
}

func TestSaveWorkspace_RejectsMissingOrMismatchedRelatedProject(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()
	baseTime := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)

	workspace := schema.Workspace{
		ID:             "ws-related",
		Name:           "Related Workspace",
		Kind:           schema.WorkspaceKindProject,
		Status:         schema.WorkspaceStatusActive,
		BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		CreatedAt:      baseTime,
		UpdatedAt:      baseTime,
		CreatedBy:      "owner",
	}

	missingProject := workspace
	missingProject.RelatedProjectID = "proj-missing"
	if err := SaveWorkspace(ctx, db, missingProject); err == nil || !IsValidationError(err) {
		t.Fatalf("expected missing related project validation error, got %v", err)
	}

	otherWorkspace := workspace
	otherWorkspace.ID = "ws-other"
	otherWorkspace.Name = "Other Workspace"
	if err := SaveWorkspace(ctx, db, otherWorkspace); err != nil {
		t.Fatalf("SaveWorkspace(other): %v", err)
	}
	if err := SaveProject(ctx, db, schema.Project{
		ID:          "proj-bound-other",
		Title:       "Bound Elsewhere",
		Kind:        schema.ProjectKindCoding,
		Status:      schema.ProjectStatusActive,
		Health:      schema.ProjectHealthUnknown,
		WorkspaceID: otherWorkspace.ID,
		CreatedAt:   baseTime,
		UpdatedAt:   baseTime,
		CreatedBy:   "owner",
	}); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}

	mismatchedProject := workspace
	mismatchedProject.RelatedProjectID = "proj-bound-other"
	if err := SaveWorkspace(ctx, db, mismatchedProject); err == nil || !IsValidationError(err) {
		t.Fatalf("expected mismatched related project validation error, got %v", err)
	}
}
