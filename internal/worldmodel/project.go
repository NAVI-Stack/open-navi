package worldmodel

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

// UpsertProject inserts or updates a project and returns the persisted value.
func (wm *WorldModel) UpsertProject(ctx context.Context, p schema.Project) (schema.Project, error) {
	if strings.TrimSpace(p.ID) == "" {
		return schema.Project{}, fmt.Errorf("worldmodel: project id required")
	}
	if err := store.SaveProject(ctx, wm.db, p); err != nil {
		return schema.Project{}, err
	}
	persisted, err := store.GetProject(ctx, wm.db, p.ID)
	if err != nil {
		return schema.Project{}, err
	}
	if err := wm.recordProjectProvenance(ctx, persisted, "project_crud"); err != nil {
		return schema.Project{}, err
	}
	return persisted, nil
}

// GetProject returns a single project by ID.
func (wm *WorldModel) GetProject(ctx context.Context, id string) (schema.Project, error) {
	return store.GetProject(ctx, wm.db, id)
}

// ListProjects returns projects, optionally filtered by status.
func (wm *WorldModel) ListProjects(ctx context.Context, status schema.ProjectStatus, includeArchived bool) ([]schema.Project, error) {
	return store.ListProjects(ctx, wm.db, status, includeArchived)
}

// ArchiveProject marks a project as archived and returns the persisted value.
func (wm *WorldModel) ArchiveProject(ctx context.Context, id string) (schema.Project, error) {
	if strings.TrimSpace(id) == "" {
		return schema.Project{}, fmt.Errorf("worldmodel: project id required")
	}
	if err := store.ArchiveProject(ctx, wm.db, id, time.Now().UTC()); err != nil {
		return schema.Project{}, err
	}
	return store.GetProject(ctx, wm.db, id)
}

// DeleteProject removes a project after validating its existence, cleaning up active references.
func (wm *WorldModel) DeleteProject(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("worldmodel: project id required")
	}
	// 1. Verify existence
	_, err := store.GetProject(ctx, wm.db, id)
	if err != nil {
		return err
	}
	// 2. Clear if active
	if wm.GetActiveProjectID(ctx) == id {
		if err := wm.SetActiveProjectID(ctx, ""); err != nil {
			return fmt.Errorf("worldmodel: clear active project: %w", err)
		}
	}
	// 3. Delete from store
	return store.DeleteProject(ctx, wm.db, id)
}

// BindProjectWorkspace makes the project-owned workspace binding authoritative.
func (wm *WorldModel) BindProjectWorkspace(ctx context.Context, projectID, workspaceID string) (schema.Project, error) {
	if err := store.BindProjectWorkspace(ctx, wm.db, projectID, workspaceID, time.Now().UTC()); err != nil {
		return schema.Project{}, err
	}
	persisted, err := store.GetProject(ctx, wm.db, projectID)
	if err != nil {
		return schema.Project{}, err
	}
	if err := wm.recordProjectProvenance(ctx, persisted, "project_workspace_binding"); err != nil {
		return schema.Project{}, err
	}
	return persisted, nil
}

// UnbindProjectWorkspace clears the project-owned workspace binding.
func (wm *WorldModel) UnbindProjectWorkspace(ctx context.Context, projectID string) (schema.Project, error) {
	if err := store.UnbindProjectWorkspace(ctx, wm.db, projectID, time.Now().UTC()); err != nil {
		return schema.Project{}, err
	}
	persisted, err := store.GetProject(ctx, wm.db, projectID)
	if err != nil {
		return schema.Project{}, err
	}
	if err := wm.recordProjectProvenance(ctx, persisted, "project_workspace_unbinding"); err != nil {
		return schema.Project{}, err
	}
	return persisted, nil
}

// GetProjectWorkspace resolves the project-owned workspace binding, with legacy
// workspaces.related_project_id as a read-only compatibility fallback.
func (wm *WorldModel) GetProjectWorkspace(ctx context.Context, projectID string) (schema.Workspace, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return schema.Workspace{}, fmt.Errorf("worldmodel: project id required")
	}
	project, err := store.GetProject(ctx, wm.db, projectID)
	if err == nil {
		if project.WorkspaceID != "" {
			return store.GetWorkspace(ctx, wm.db, project.WorkspaceID)
		}
		return store.GetWorkspaceByProjectID(ctx, wm.db, project.ID)
	}
	if !strings.Contains(err.Error(), "project not found") {
		return schema.Workspace{}, err
	}
	return store.GetWorkspaceByProjectID(ctx, wm.db, projectID)
}

// ProjectReadiness returns deterministic project readiness before task intake or
// coding execution. It degrades explicitly instead of treating missing bindings
// as success.
func (wm *WorldModel) ProjectReadiness(ctx context.Context, projectID string) (schema.ProjectReadiness, error) {
	project, err := store.GetProject(ctx, wm.db, projectID)
	if err != nil {
		return schema.ProjectReadiness{}, err
	}
	readiness := schema.ProjectReadiness{
		ProjectID:        project.ID,
		ProjectKind:      string(project.Kind),
		Status:           string(project.Status),
		ReadyForChat:     project.Status != schema.ProjectStatusArchived,
		ReadyForPlanning: project.Status != schema.ProjectStatusArchived,
		WorkspaceID:      project.WorkspaceID,
		SandboxProfileID: project.SandboxProfileID,
		DegradedReasons:  []string{},
	}
	if project.Status == schema.ProjectStatusArchived {
		readiness.DegradedReasons = append(readiness.DegradedReasons, "project is archived")
		return readiness, nil
	}
	if project.Kind != schema.ProjectKindCoding {
		return readiness, nil
	}

	readiness.WorkspaceBindingRequired = true
	if project.WorkspaceID == "" {
		readiness.DegradedReasons = append(readiness.DegradedReasons, "coding project requires workspace binding")
		return readiness, nil
	}
	workspace, err := store.GetWorkspace(ctx, wm.db, project.WorkspaceID)
	if err != nil {
		readiness.DegradedReasons = append(readiness.DegradedReasons, "bound workspace not found")
		return readiness, nil
	}
	if workspace.Status != schema.WorkspaceStatusActive {
		readiness.DegradedReasons = append(readiness.DegradedReasons, "bound workspace is not active")
		return readiness, nil
	}
	if len(workspace.LocalRoots)+len(workspace.RepoRoots) == 0 {
		readiness.DegradedReasons = append(readiness.DegradedReasons, "bound workspace has no local or repo roots")
		return readiness, nil
	}
	if project.SandboxProfileID == "" {
		readiness.DegradedReasons = append(readiness.DegradedReasons, "coding project requires sandbox profile")
		return readiness, nil
	}
	profile, err := store.GetSandboxProfile(ctx, wm.db, project.SandboxProfileID)
	if err != nil {
		readiness.DegradedReasons = append(readiness.DegradedReasons, "bound sandbox profile not found")
		return readiness, nil
	}
	if profile.Status != schema.SandboxProfileStatusActive {
		readiness.DegradedReasons = append(readiness.DegradedReasons, "bound sandbox profile is not active")
		return readiness, nil
	}
	if profile.Runtime != schema.SandboxRuntimeDocker {
		readiness.DegradedReasons = append(readiness.DegradedReasons, "bound sandbox profile runtime is unsupported")
		return readiness, nil
	}
	readiness.ReadyForCoding = true
	return readiness, nil
}

// GetActiveProjectID returns the explicit active project ID, if any.
func (wm *WorldModel) GetActiveProjectID(ctx context.Context) string {
	val, ok, err := store.GetConfigurationValue(ctx, wm.db, "owner", "active", "active_project_id")
	if err != nil || !ok {
		return ""
	}
	return strings.TrimSpace(val)
}

// SetActiveProjectID sets the explicit active project. Empty clears it.
func (wm *WorldModel) SetActiveProjectID(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id != "" {
		project, err := store.GetProject(ctx, wm.db, id)
		if err != nil {
			return err
		}
		if project.Status == schema.ProjectStatusArchived {
			return fmt.Errorf("worldmodel: active project must not be archived")
		}
	}
	now := time.Now().UTC()
	entry := schema.ConfigurationEntry{
		ID:        "active_project_id",
		Scope:     "owner",
		ScopeID:   "active",
		Key:       "active_project_id",
		Value:     id,
		Source:    "explicit",
		CreatedAt: now,
		UpdatedAt: now,
	}
	return store.SaveConfigurationEntry(ctx, wm.db, entry)
}

// ResolveActiveProject resolves explicit input first, then the owner's active project.
func (wm *WorldModel) ResolveActiveProject(ctx context.Context, projectID string) (*schema.Project, string, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID != "" {
		project, err := store.GetProject(ctx, wm.db, projectID)
		if err != nil {
			return nil, "none", err
		}
		return &project, "request", nil
	}
	activeID := wm.GetActiveProjectID(ctx)
	if activeID == "" {
		return nil, "none", nil
	}
	project, err := store.GetProject(ctx, wm.db, activeID)
	if err != nil {
		if strings.Contains(err.Error(), "project not found") {
			return nil, "none", nil
		}
		return nil, "none", err
	}
	if project.Status == schema.ProjectStatusArchived {
		return nil, "none", nil
	}
	return &project, "active", nil
}

func (wm *WorldModel) recordProjectProvenance(ctx context.Context, p schema.Project, source string) error {
	now := time.Now().UTC()
	if source == "" {
		source = "project_crud"
	}
	prov := schema.EntityProvenance{
		Source:             source,
		Timestamp:          now,
		Confidence:         1.0,
		DerivationChain:    []string{source},
		ReinforcementCount: 1,
	}
	if err := store.SaveEntityProvenance(ctx, wm.db, "project", p.ID, prov); err != nil {
		return err
	}
	ownerID := strings.TrimSpace(p.CreatedBy)
	if ownerID != "" {
		if err := wm.TouchRelationship(ctx, "owner:"+ownerID, "project:"+p.ID, "owns", source, 0.5); err != nil {
			return err
		}
	}
	if p.WorkspaceID != "" {
		if err := wm.TouchRelationship(ctx, "project:"+p.ID, "workspace:"+p.WorkspaceID, "bound_to", source, 0.7); err != nil {
			return err
		}
	}
	if p.SandboxProfileID != "" {
		if err := wm.TouchRelationship(ctx, "project:"+p.ID, "sandbox_profile:"+p.SandboxProfileID, "uses", source, 0.6); err != nil {
			return err
		}
	}
	return nil
}
