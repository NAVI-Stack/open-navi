package worldmodel

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
	"github.com/google/uuid"
)

// CreateProjectTask validates and persists a task under a project. Mutative
// classes are admitted as blocked when the project workspace is not ready.
func (wm *WorldModel) CreateProjectTask(ctx context.Context, projectID string, task schema.Task) (schema.Task, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return schema.Task{}, fmt.Errorf("worldmodel: project id required")
	}
	project, err := store.GetProject(ctx, wm.db, projectID)
	if err != nil {
		return schema.Task{}, err
	}
	task = normalizeProjectTask(task, project)
	if task.WorkspaceID == "" {
		task.WorkspaceID = wm.bestEffortProjectWorkspaceID(ctx, project.ID)
	}
	if project.Status == schema.ProjectStatusArchived && task.TaskClass.RequiresWorkspaceBinding() {
		task.Status = schema.TaskStatusBlocked
		task.LifecyclePhase = "intake_blocked"
		task.BlockReason = "project is archived"
	}
	if task.TaskClass.RequiresWorkspaceBinding() {
		workspaceID, ready, reasons := wm.projectWorkspaceReadinessForTask(ctx, project)
		task.WorkspaceID = workspaceID
		if !ready && task.BlockReason == "" {
			task.Status = schema.TaskStatusBlocked
			task.LifecyclePhase = "intake_blocked"
			task.BlockReason = strings.Join(reasons, "; ")
			if task.BlockReason == "" {
				task.BlockReason = "task class requires workspace binding"
			}
		}
	}
	if err := store.PersistTask(ctx, wm.db, task); err != nil {
		return schema.Task{}, err
	}
	if err := wm.TouchRelationship(ctx, "project:"+project.ID, "task:"+task.ID, "contains", "project_task_intake", 0.6); err != nil {
		return schema.Task{}, err
	}
	if task.WorkspaceID != "" {
		if err := wm.TouchRelationship(ctx, "workspace:"+task.WorkspaceID, "task:"+task.ID, "scopes", "project_task_intake", 0.4); err != nil {
			return schema.Task{}, err
		}
	}
	return task, nil
}

func (wm *WorldModel) ListProjectTasks(ctx context.Context, projectID string) ([]schema.Task, error) {
	if _, err := store.GetProject(ctx, wm.db, projectID); err != nil {
		return nil, err
	}
	return store.ListTasksByProject(ctx, wm.db, projectID)
}

func (wm *WorldModel) GetProjectTask(ctx context.Context, projectID, taskID string) (schema.Task, error) {
	if _, err := store.GetProject(ctx, wm.db, projectID); err != nil {
		return schema.Task{}, err
	}
	task, ok, err := store.GetProjectTaskByID(ctx, wm.db, projectID, taskID)
	if err != nil {
		return schema.Task{}, err
	}
	if !ok {
		return schema.Task{}, fmt.Errorf("worldmodel: project task not found: %s", taskID)
	}
	return task, nil
}

func (wm *WorldModel) CancelProjectTask(ctx context.Context, projectID, taskID string) (schema.Task, error) {
	if _, err := store.GetProject(ctx, wm.db, projectID); err != nil {
		return schema.Task{}, err
	}
	return store.CancelProjectTask(ctx, wm.db, projectID, taskID, time.Now().UTC())
}

func normalizeProjectTask(task schema.Task, project schema.Project) schema.Task {
	now := time.Now().UTC()
	task.ID = strings.TrimSpace(task.ID)
	if task.ID == "" {
		task.ID = uuid.NewString()
	}
	task.ProjectID = project.ID
	task.Title = strings.TrimSpace(task.Title)
	task.Description = strings.TrimSpace(task.Description)
	task.RawInput = strings.TrimSpace(task.RawInput)
	task.AcceptanceTarget = strings.TrimSpace(task.AcceptanceTarget)
	task.WorkspaceID = strings.TrimSpace(task.WorkspaceID)
	task.BlockReason = strings.TrimSpace(task.BlockReason)
	task.LifecyclePhase = strings.TrimSpace(task.LifecyclePhase)
	if task.Description == "" {
		task.Description = task.RawInput
	}
	if task.RawInput == "" {
		task.RawInput = task.Description
	}
	if task.Status == "" {
		task.Status = schema.TaskStatusPending
	}
	if task.Risk == "" {
		task.Risk = schema.RiskLow
	}
	if task.AssignedTo == "" {
		task.AssignedTo = schema.AgentNavi
	}
	if task.TaskClass == "" {
		task.TaskClass = schema.TaskClassPlanning
	}
	if task.LifecyclePhase == "" {
		task.LifecyclePhase = "intake_accepted"
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = task.CreatedAt
	}
	return task
}

func (wm *WorldModel) bestEffortProjectWorkspaceID(ctx context.Context, projectID string) string {
	workspace, err := wm.GetProjectWorkspace(ctx, projectID)
	if err != nil || workspace.ID == "" {
		return ""
	}
	return workspace.ID
}

func (wm *WorldModel) projectWorkspaceReadinessForTask(ctx context.Context, project schema.Project) (string, bool, []string) {
	if project.Kind == schema.ProjectKindCoding {
		readiness, err := wm.ProjectReadiness(ctx, project.ID)
		if err != nil {
			return "", false, []string{"project readiness unavailable"}
		}
		return readiness.WorkspaceID, readiness.ReadyForCoding, readiness.DegradedReasons
	}
	workspace, err := wm.GetProjectWorkspace(ctx, project.ID)
	if err != nil {
		return "", false, []string{"task class requires workspace binding"}
	}
	if workspace.Status != schema.WorkspaceStatusActive {
		return workspace.ID, false, []string{"bound workspace is not active"}
	}
	if len(workspace.LocalRoots)+len(workspace.RepoRoots) == 0 {
		return workspace.ID, false, []string{"bound workspace has no local or repo roots"}
	}
	return workspace.ID, true, nil
}
