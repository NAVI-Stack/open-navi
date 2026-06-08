package store

import (
	"context"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

func TestSaveArtifact_RoundTripWithBoundProject(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)

	workspace := schema.Workspace{
		ID:             "ws-artifact",
		Name:           "Artifact Workspace",
		Kind:           schema.WorkspaceKindProject,
		Status:         schema.WorkspaceStatusActive,
		LocalRoots:     []string{"/workspace/artifact"},
		AllowedActions: schema.AllowedActions{Read: true, Write: true, Create: true, Modify: true, RenameMove: true, Delete: false, Execute: false},
		BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		AuditEnabled:   true,
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      "owner-1",
		Metadata:       "{}",
	}
	if err := SaveWorkspace(ctx, db, workspace); err != nil {
		t.Fatalf("SaveWorkspace: %v", err)
	}
	project := schema.Project{
		ID:          "proj-artifact",
		Title:       "Artifact Project",
		Kind:        schema.ProjectKindCoding,
		Status:      schema.ProjectStatusActive,
		Health:      schema.ProjectHealthUnknown,
		WorkspaceID: workspace.ID,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "owner-1",
	}
	if err := SaveProject(ctx, db, project); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}

	artifact := schema.Artifact{
		ID:                 "art-1",
		WorkspaceID:        workspace.ID,
		ProjectID:          project.ID,
		OwnerID:            "owner-1",
		CanonicalTitle:     "spec-draft",
		DisplayTitle:       "Spec Draft",
		Type:               schema.ArtifactTypeDocument,
		Subtype:            "markdown",
		SchemaVersion:      "1.0",
		ContentFormat:      "text/markdown",
		LifecycleState:     schema.ArtifactLifecycleDraft,
		CurrentBranchID:    "branch-1",
		CurrentVersionID:   "version-1",
		HeadVersionNumber:  1,
		CreatedByActorType: schema.ActorAgent,
		CreatedByActorID:   "navi",
		ProvenanceRootID:   "version-1",
		Attributes:         map[string]any{"lane": "docs"},
		CreatedAt:          now,
	}
	if err := SaveArtifact(ctx, db, artifact); err != nil {
		t.Fatalf("SaveArtifact: %v", err)
	}

	stored, err := GetArtifact(ctx, db, artifact.ID)
	if err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}
	if stored == nil {
		t.Fatal("expected stored artifact")
	}
	if stored.ProjectID != project.ID || stored.WorkspaceID != workspace.ID {
		t.Fatalf("artifact binding mismatch: %+v", stored)
	}
	if stored.DisplayTitle != artifact.DisplayTitle || stored.CanonicalTitle != artifact.CanonicalTitle {
		t.Fatalf("artifact titles did not round-trip: %+v", stored)
	}
}

func TestSaveArtifact_RejectsMissingOrMismatchedProjectBindings(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)

	workspaceA := schema.Workspace{
		ID:             "ws-a",
		Name:           "Workspace A",
		Kind:           schema.WorkspaceKindProject,
		Status:         schema.WorkspaceStatusActive,
		LocalRoots:     []string{"/workspace/a"},
		AllowedActions: schema.AllowedActions{Read: true, Write: true, Create: true, Modify: true, RenameMove: true, Delete: false, Execute: false},
		BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		AuditEnabled:   true,
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      "owner-1",
		Metadata:       "{}",
	}
	workspaceB := workspaceA
	workspaceB.ID = "ws-b"
	workspaceB.Name = "Workspace B"
	workspaceB.LocalRoots = []string{"/workspace/b"}
	if err := SaveWorkspace(ctx, db, workspaceA); err != nil {
		t.Fatalf("SaveWorkspace(workspaceA): %v", err)
	}
	if err := SaveWorkspace(ctx, db, workspaceB); err != nil {
		t.Fatalf("SaveWorkspace(workspaceB): %v", err)
	}

	project := schema.Project{
		ID:          "proj-bound",
		Title:       "Bound Project",
		Kind:        schema.ProjectKindCoding,
		Status:      schema.ProjectStatusActive,
		Health:      schema.ProjectHealthUnknown,
		WorkspaceID: workspaceA.ID,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "owner-1",
	}
	if err := SaveProject(ctx, db, project); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}

	tests := []struct {
		name     string
		artifact schema.Artifact
	}{
		{
			name: "missing project",
			artifact: schema.Artifact{
				ID:                 "art-missing-project",
				WorkspaceID:        workspaceA.ID,
				ProjectID:          "proj-missing",
				OwnerID:            "owner-1",
				CanonicalTitle:     "missing-project",
				DisplayTitle:       "Missing Project",
				Type:               schema.ArtifactTypeDocument,
				Subtype:            "markdown",
				SchemaVersion:      "1.0",
				ContentFormat:      "text/markdown",
				LifecycleState:     schema.ArtifactLifecycleDraft,
				CurrentBranchID:    "branch-1",
				CurrentVersionID:   "version-1",
				HeadVersionNumber:  1,
				CreatedByActorType: schema.ActorAgent,
				ProvenanceRootID:   "version-1",
			},
		},
		{
			name: "mismatched workspace",
			artifact: schema.Artifact{
				ID:                 "art-mismatch",
				WorkspaceID:        workspaceB.ID,
				ProjectID:          project.ID,
				OwnerID:            "owner-1",
				CanonicalTitle:     "wrong-workspace",
				DisplayTitle:       "Wrong Workspace",
				Type:               schema.ArtifactTypeDocument,
				Subtype:            "markdown",
				SchemaVersion:      "1.0",
				ContentFormat:      "text/markdown",
				LifecycleState:     schema.ArtifactLifecycleDraft,
				CurrentBranchID:    "branch-1",
				CurrentVersionID:   "version-1",
				HeadVersionNumber:  1,
				CreatedByActorType: schema.ActorAgent,
				ProvenanceRootID:   "version-1",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := SaveArtifact(ctx, db, tc.artifact)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !IsValidationError(err) {
				t.Fatalf("expected validation error, got %T: %v", err, err)
			}
		})
	}
}
