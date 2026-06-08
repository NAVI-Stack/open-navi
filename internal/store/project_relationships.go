package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/ceoai/navi/internal/schema"
)

func validateProjectReference(ctx context.Context, db *sql.DB, entity, field, projectID string) (schema.Project, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return schema.Project{}, nil
	}
	project, err := GetProject(ctx, db, projectID)
	if err != nil {
		if strings.Contains(err.Error(), "project not found") {
			return schema.Project{}, &ValidationError{
				Entity:  entity,
				Field:   field,
				Message: fmt.Sprintf("project %q not found", projectID),
			}
		}
		return schema.Project{}, err
	}
	return project, nil
}

func validateWorkspaceRelationship(ctx context.Context, db *sql.DB, entity, field, workspaceID string) (schema.Workspace, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return schema.Workspace{}, &ValidationError{
			Entity:  entity,
			Field:   field,
			Message: "is required",
		}
	}
	workspace, err := GetWorkspace(ctx, db, workspaceID)
	if err != nil {
		if strings.Contains(err.Error(), "workspace not found") {
			return schema.Workspace{}, &ValidationError{
				Entity:  entity,
				Field:   field,
				Message: fmt.Sprintf("workspace %q not found", workspaceID),
			}
		}
		return schema.Workspace{}, err
	}
	return workspace, nil
}

func validateWorkspaceProjectRelationship(ctx context.Context, db *sql.DB, workspaceID, relatedProjectID string) error {
	relatedProjectID = strings.TrimSpace(relatedProjectID)
	if relatedProjectID == "" {
		return nil
	}
	project, err := validateProjectReference(ctx, db, "workspace", "related_project_id", relatedProjectID)
	if err != nil {
		return err
	}
	if boundWorkspaceID := strings.TrimSpace(project.WorkspaceID); boundWorkspaceID != "" && boundWorkspaceID != strings.TrimSpace(workspaceID) {
		return &ValidationError{
			Entity:  "workspace",
			Field:   "related_project_id",
			Message: fmt.Sprintf("project %q is bound to workspace %q", relatedProjectID, boundWorkspaceID),
		}
	}
	return nil
}

func validateArtifactProjectRelationship(ctx context.Context, db *sql.DB, workspaceID, projectID string) error {
	workspace, err := validateWorkspaceRelationship(ctx, db, "artifact", "workspace_id", workspaceID)
	if err != nil {
		return err
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil
	}
	project, err := validateProjectReference(ctx, db, "artifact", "project_id", projectID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(project.WorkspaceID) == "" {
		return &ValidationError{
			Entity:  "artifact",
			Field:   "project_id",
			Message: fmt.Sprintf("project %q has no workspace binding", projectID),
		}
	}
	if strings.TrimSpace(project.WorkspaceID) != workspace.ID {
		return &ValidationError{
			Entity:  "artifact",
			Field:   "project_id",
			Message: fmt.Sprintf("project %q is bound to workspace %q, not %q", projectID, project.WorkspaceID, workspace.ID),
		}
	}
	return nil
}
