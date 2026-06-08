package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/schema"
)

const taskSelectColumns = `id, title, description, status, risk, assigned_to, directive_id, dependencies, surfaces, verification, cost,
	created_at, updated_at, COALESCE(project_id, ''), COALESCE(workspace_id, ''), COALESCE(task_class, ''),
	COALESCE(raw_input, ''), COALESCE(acceptance_target, ''), COALESCE(lifecycle_phase, ''), COALESCE(block_reason, '')`

// Persist inserts a task. Caller must have validated the task.
func PersistTask(ctx context.Context, db *sql.DB, task schema.Task) error {
	if err := task.Validate(); err != nil {
		return err
	}
	deps, err := json.Marshal(task.Dependencies)
	if err != nil {
		return fmt.Errorf("store: marshal dependencies: %w", err)
	}
	surfaces, err := json.Marshal(task.Surfaces)
	if err != nil {
		return fmt.Errorf("store: marshal surfaces: %w", err)
	}
	ver, err := json.Marshal(task.Verification)
	if err != nil {
		return fmt.Errorf("store: marshal verification: %w", err)
	}
	cost, err := json.Marshal(task.Cost)
	if err != nil {
		return fmt.Errorf("store: marshal cost: %w", err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO tasks (
			id, title, description, status, risk, assigned_to, directive_id, dependencies, surfaces, verification, cost,
			created_at, updated_at, project_id, workspace_id, task_class, raw_input, acceptance_target, lifecycle_phase, block_reason
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.Title, task.Description, string(task.Status), string(task.Risk), string(task.AssignedTo), task.DirectiveID,
		string(deps), string(surfaces), string(ver), string(cost),
		task.CreatedAt.UTC().Format(timeFormat), task.UpdatedAt.UTC().Format(timeFormat),
		task.ProjectID, task.WorkspaceID, string(task.TaskClass), task.RawInput, task.AcceptanceTarget, task.LifecyclePhase, task.BlockReason,
	)
	if err != nil {
		return fmt.Errorf("store: persist task: %w", err)
	}
	return nil
}

// GetTaskByID returns the task and true if found.
func GetTaskByID(ctx context.Context, db *sql.DB, taskID string) (schema.Task, bool, error) {
	var t schema.Task
	var deps, surfaces, ver, cost string
	var createdAt, updatedAt string
	err := db.QueryRowContext(ctx, `SELECT `+taskSelectColumns+` FROM tasks WHERE id = ?`, taskID).Scan(
		&t.ID, &t.Title, &t.Description, (*string)(&t.Status), (*string)(&t.Risk), (*string)(&t.AssignedTo), &t.DirectiveID,
		&deps, &surfaces, &ver, &cost, &createdAt, &updatedAt,
		&t.ProjectID, &t.WorkspaceID, (*string)(&t.TaskClass), &t.RawInput, &t.AcceptanceTarget, &t.LifecyclePhase, &t.BlockReason,
	)
	if err == sql.ErrNoRows {
		return schema.Task{}, false, nil
	}
	if err != nil {
		return schema.Task{}, false, fmt.Errorf("store: get task: %w", err)
	}
	if err := parseTaskJSON(&t, deps, surfaces, ver, cost, createdAt, updatedAt); err != nil {
		return schema.Task{}, false, err
	}
	return t, true, nil
}

// UpdateTask replaces a task by id. Caller must have validated the task.
func UpdateTask(ctx context.Context, db *sql.DB, task schema.Task) error {
	if err := task.Validate(); err != nil {
		return err
	}
	deps, err := json.Marshal(task.Dependencies)
	if err != nil {
		return fmt.Errorf("store: marshal dependencies: %w", err)
	}
	surfaces, err := json.Marshal(task.Surfaces)
	if err != nil {
		return fmt.Errorf("store: marshal surfaces: %w", err)
	}
	ver, err := json.Marshal(task.Verification)
	if err != nil {
		return fmt.Errorf("store: marshal verification: %w", err)
	}
	cost, err := json.Marshal(task.Cost)
	if err != nil {
		return fmt.Errorf("store: marshal cost: %w", err)
	}
	res, err := db.ExecContext(ctx, `UPDATE tasks SET title=?, description=?, status=?, risk=?, assigned_to=?, directive_id=?,
			dependencies=?, surfaces=?, verification=?, cost=?, updated_at=?, project_id=?, workspace_id=?, task_class=?,
			raw_input=?, acceptance_target=?, lifecycle_phase=?, block_reason=?
		WHERE id=?`,
		task.Title, task.Description, string(task.Status), string(task.Risk), string(task.AssignedTo), task.DirectiveID,
		string(deps), string(surfaces), string(ver), string(cost), task.UpdatedAt.UTC().Format(timeFormat),
		task.ProjectID, task.WorkspaceID, string(task.TaskClass), task.RawInput, task.AcceptanceTarget, task.LifecyclePhase, task.BlockReason,
		task.ID,
	)
	if err != nil {
		return fmt.Errorf("store: update task: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("store: task not found: %s", task.ID)
	}
	return nil
}

// ListTasksByStatus returns tasks with the given status.
func ListTasksByStatus(ctx context.Context, db *sql.DB, status schema.TaskStatus) ([]schema.Task, error) {
	return listTasks(ctx, db, `SELECT `+taskSelectColumns+` FROM tasks WHERE status = ? ORDER BY updated_at DESC`, string(status))
}

// ListAllTasks returns all tasks.
func ListAllTasks(ctx context.Context, db *sql.DB) ([]schema.Task, error) {
	return listTasks(ctx, db, `SELECT `+taskSelectColumns+` FROM tasks ORDER BY updated_at DESC`)
}

// ListTasksByAgent returns tasks assigned to the given agent type.
func ListTasksByAgent(ctx context.Context, db *sql.DB, agentType schema.AgentType) ([]schema.Task, error) {
	return listTasks(ctx, db, `SELECT `+taskSelectColumns+` FROM tasks WHERE assigned_to = ? ORDER BY updated_at DESC`, string(agentType))
}

// ListTasksByRisk returns tasks with the given risk level.
func ListTasksByRisk(ctx context.Context, db *sql.DB, risk schema.RiskLevel) ([]schema.Task, error) {
	return listTasks(ctx, db, `SELECT `+taskSelectColumns+` FROM tasks WHERE risk = ? ORDER BY updated_at DESC`, string(risk))
}

// GetTasksByDirective returns tasks for a specific directive.
func GetTasksByDirective(ctx context.Context, db *sql.DB, directiveID string) ([]schema.Task, error) {
	return listTasks(ctx, db, `SELECT `+taskSelectColumns+` FROM tasks WHERE directive_id = ? ORDER BY created_at ASC`, directiveID)
}

// ListTasksByProject returns tasks scoped to the given project.
func ListTasksByProject(ctx context.Context, db *sql.DB, projectID string) ([]schema.Task, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, &ValidationError{Entity: "task", Field: "project_id", Message: "is required"}
	}
	return listTasks(ctx, db, `SELECT `+taskSelectColumns+` FROM tasks WHERE project_id = ? ORDER BY updated_at DESC`, projectID)
}

// GetProjectTaskByID returns a task only when it belongs to the given project.
func GetProjectTaskByID(ctx context.Context, db *sql.DB, projectID, taskID string) (schema.Task, bool, error) {
	task, ok, err := GetTaskByID(ctx, db, taskID)
	if err != nil || !ok {
		return task, ok, err
	}
	if strings.TrimSpace(task.ProjectID) != strings.TrimSpace(projectID) {
		return schema.Task{}, false, nil
	}
	return task, true, nil
}

// CancelProjectTask marks a project-scoped task as cancelled.
func CancelProjectTask(ctx context.Context, db *sql.DB, projectID, taskID string, cancelledAt time.Time) (schema.Task, error) {
	task, ok, err := GetProjectTaskByID(ctx, db, projectID, taskID)
	if err != nil {
		return schema.Task{}, err
	}
	if !ok {
		return schema.Task{}, fmt.Errorf("store: task not found: %s", taskID)
	}
	if cancelledAt.IsZero() {
		cancelledAt = time.Now().UTC()
	}
	task.Status = schema.TaskStatusCancelled
	task.LifecyclePhase = "cancelled"
	task.UpdatedAt = cancelledAt
	if err := UpdateTask(ctx, db, task); err != nil {
		return schema.Task{}, err
	}
	return task, nil
}

func listTasks(ctx context.Context, db *sql.DB, query string, args ...any) ([]schema.Task, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list tasks: %w", err)
	}
	defer rows.Close()
	var out []schema.Task
	for rows.Next() {
		var t schema.Task
		var deps, surfaces, ver, cost string
		var createdAt, updatedAt string
		if err := rows.Scan(&t.ID, &t.Title, &t.Description, (*string)(&t.Status), (*string)(&t.Risk), (*string)(&t.AssignedTo), &t.DirectiveID, &deps, &surfaces, &ver, &cost, &createdAt, &updatedAt, &t.ProjectID, &t.WorkspaceID, (*string)(&t.TaskClass), &t.RawInput, &t.AcceptanceTarget, &t.LifecyclePhase, &t.BlockReason); err != nil {
			return nil, fmt.Errorf("store: scan task: %w", err)
		}
		if err := parseTaskJSON(&t, deps, surfaces, ver, cost, createdAt, updatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func parseTaskJSON(t *schema.Task, deps, surfaces, ver, cost, createdAt, updatedAt string) error {
	if err := json.Unmarshal([]byte(deps), &t.Dependencies); err != nil {
		return fmt.Errorf("store: unmarshal dependencies: %w", err)
	}
	if err := json.Unmarshal([]byte(surfaces), &t.Surfaces); err != nil {
		return fmt.Errorf("store: unmarshal surfaces: %w", err)
	}
	if err := json.Unmarshal([]byte(ver), &t.Verification); err != nil {
		return fmt.Errorf("store: unmarshal verification: %w", err)
	}
	if err := json.Unmarshal([]byte(cost), &t.Cost); err != nil {
		return fmt.Errorf("store: unmarshal cost: %w", err)
	}
	var err error
	t.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return fmt.Errorf("store: parse created_at: %w", err)
	}
	t.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return fmt.Errorf("store: parse updated_at: %w", err)
	}
	return nil
}

// GetSchedulableTasks previously combined task dependency resolution with surface
// claim conflict checks. The Helm-style task/worker/claim orchestration layer has
// been removed from NAVI, so this function is now a simple alias for querying
// pending tasks. It is retained only for backward compatibility with older tools
// and tests that still call it.
func GetSchedulableTasks(ctx context.Context, db *sql.DB) ([]schema.Task, error) {
	return ListTasksByStatus(ctx, db, schema.TaskStatusPending)
}
