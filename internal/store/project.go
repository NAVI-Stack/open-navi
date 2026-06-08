package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/open-navi/navi/internal/schema"
)

func SaveProject(ctx context.Context, db *sql.DB, project schema.Project) error {
	normalized, err := normalizeAndValidateProject(project)
	if err != nil {
		return err
	}
	if err := validateProjectWorkspaceBinding(ctx, db, normalized.ID, normalized.WorkspaceID); err != nil {
		return err
	}
	if err := validateProjectSandboxProfileBinding(ctx, db, normalized.SandboxProfileID); err != nil {
		return err
	}
	attrs, err := json.Marshal(normalized.Attributes)
	if err != nil {
		return &ValidationError{Field: "attributes", Message: "attributes must be valid JSON"}
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO projects (
			id, title, slug, description, project_kind, status, health, workspace_id, sandbox_profile_id,
			icon, color, memory_scope,
			created_at, updated_at, created_by, attributes
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title = excluded.title,
			slug = excluded.slug,
			description = excluded.description,
			project_kind = excluded.project_kind,
			status = excluded.status,
			health = excluded.health,
			workspace_id = excluded.workspace_id,
			sandbox_profile_id = excluded.sandbox_profile_id,
			icon = excluded.icon,
			color = excluded.color,
			memory_scope = excluded.memory_scope,
			updated_at = excluded.updated_at,
			created_by = excluded.created_by,
			attributes = excluded.attributes
	`, normalized.ID, normalized.Title, normalized.Slug, normalized.Description, normalized.Kind, normalized.Status,
		normalized.Health, normalized.WorkspaceID, normalized.SandboxProfileID,
		normalized.Icon, normalized.Color, normalized.MemoryScope,
		normalized.CreatedAt.UTC().Format(timeFormat), normalized.UpdatedAt.UTC().Format(timeFormat),
		normalized.CreatedBy, string(attrs))
	if err != nil {
		return fmt.Errorf("store: save project: %w", err)
	}
	return nil
}

func GetProject(ctx context.Context, db *sql.DB, id string) (schema.Project, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return schema.Project{}, &ValidationError{Field: "project_id", Message: "project_id is required"}
	}
	row := db.QueryRowContext(ctx, `
		SELECT id, title, slug, description, project_kind, status, health, COALESCE(workspace_id, ''), COALESCE(sandbox_profile_id, ''),
			COALESCE(icon, 'folder'), COALESCE(color, '#3b82f6'), COALESCE(memory_scope, 'default'),
			created_at, updated_at, created_by, attributes
		FROM projects
		WHERE id = ?
	`, id)
	project, err := scanProject(row)
	if err == sql.ErrNoRows {
		return schema.Project{}, fmt.Errorf("store: project not found: %s", id)
	}
	if err != nil {
		return schema.Project{}, err
	}
	return project, nil
}

func ListProjects(ctx context.Context, db *sql.DB, status schema.ProjectStatus, includeArchived bool) ([]schema.Project, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if status != "" {
		if !status.IsValid() {
			return nil, &ValidationError{Field: "status", Message: "invalid project status"}
		}
		rows, err = db.QueryContext(ctx, `
			SELECT id, title, slug, description, project_kind, status, health, COALESCE(workspace_id, ''), COALESCE(sandbox_profile_id, ''),
				COALESCE(icon, 'folder'), COALESCE(color, '#3b82f6'), COALESCE(memory_scope, 'default'),
				created_at, updated_at, created_by, attributes
			FROM projects
			WHERE status = ?
			ORDER BY updated_at DESC, id ASC
		`, status)
	} else if includeArchived {
		rows, err = db.QueryContext(ctx, `
			SELECT id, title, slug, description, project_kind, status, health, COALESCE(workspace_id, ''), COALESCE(sandbox_profile_id, ''),
				COALESCE(icon, 'folder'), COALESCE(color, '#3b82f6'), COALESCE(memory_scope, 'default'),
				created_at, updated_at, created_by, attributes
			FROM projects
			ORDER BY updated_at DESC, id ASC
		`)
	} else {
		rows, err = db.QueryContext(ctx, `
			SELECT id, title, slug, description, project_kind, status, health, COALESCE(workspace_id, ''), COALESCE(sandbox_profile_id, ''),
				COALESCE(icon, 'folder'), COALESCE(color, '#3b82f6'), COALESCE(memory_scope, 'default'),
				created_at, updated_at, created_by, attributes
			FROM projects
			WHERE status <> ?
			ORDER BY updated_at DESC, id ASC
		`, schema.ProjectStatusArchived)
	}
	if err != nil {
		return nil, fmt.Errorf("store: list projects: %w", err)
	}
	defer rows.Close()

	var projects []schema.Project
	for rows.Next() {
		project, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list projects rows: %w", err)
	}
	if projects == nil {
		projects = []schema.Project{}
	}
	return projects, nil
}

func ArchiveProject(ctx context.Context, db *sql.DB, id string, archivedAt time.Time) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return &ValidationError{Field: "project_id", Message: "project_id is required"}
	}
	if archivedAt.IsZero() {
		archivedAt = time.Now().UTC()
	}
	res, err := db.ExecContext(ctx, `
		UPDATE projects
		SET status = ?, updated_at = ?
		WHERE id = ?
	`, schema.ProjectStatusArchived, archivedAt.UTC().Format(timeFormat), id)
	if err != nil {
		return fmt.Errorf("store: archive project: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: archive project rows: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("store: project not found: %s", id)
	}
	return nil
}

func DeleteProject(ctx context.Context, db *sql.DB, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return &ValidationError{Field: "project_id", Message: "project_id is required"}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: delete project transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "UPDATE workspaces SET related_project_id = NULL WHERE related_project_id = ?", id); err != nil {
		return fmt.Errorf("store: delete project (update workspaces): %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE navi_chats SET project_id = NULL WHERE project_id = ?", id); err != nil {
		return fmt.Errorf("store: delete project (update navi_chats): %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE runtime_sessions SET project_id = NULL WHERE project_id = ?", id); err != nil {
		return fmt.Errorf("store: delete project (update runtime_sessions): %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE tasks SET project_id = NULL WHERE project_id = ?", id); err != nil {
		return fmt.Errorf("store: delete project (update tasks): %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE artifacts SET project_id = NULL WHERE project_id = ?", id); err != nil {
		return fmt.Errorf("store: delete project (update artifacts): %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM entity_relationships WHERE from_entity_id = ? OR to_entity_id = ?", "project:"+id, "project:"+id); err != nil {
		return fmt.Errorf("store: delete project (delete relationships): %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM entity_provenance WHERE entity_type = 'project' AND entity_id = ?", id); err != nil {
		return fmt.Errorf("store: delete project (delete provenance): %w", err)
	}
	res, err := tx.ExecContext(ctx, "DELETE FROM projects WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("store: delete project: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete project rows: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("store: project not found: %s", id)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: delete project commit: %w", err)
	}
	return nil
}

func BindProjectWorkspace(ctx context.Context, db *sql.DB, projectID, workspaceID string, updatedAt time.Time) error {
	project, err := GetProject(ctx, db, projectID)
	if err != nil {
		return err
	}
	project.WorkspaceID = strings.TrimSpace(workspaceID)
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	project.UpdatedAt = updatedAt
	return SaveProject(ctx, db, project)
}

func UnbindProjectWorkspace(ctx context.Context, db *sql.DB, projectID string, updatedAt time.Time) error {
	project, err := GetProject(ctx, db, projectID)
	if err != nil {
		return err
	}
	project.WorkspaceID = ""
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	project.UpdatedAt = updatedAt
	return SaveProject(ctx, db, project)
}

type projectScanner interface {
	Scan(dest ...interface{}) error
}

func scanProject(scanner projectScanner) (schema.Project, error) {
	var (
		project                             schema.Project
		description, workspaceID, sandboxID sql.NullString
		kind, status, health                string
		icon, color, memoryScope            string
		createdAt, updatedAt, attrs         string
	)
	if err := scanner.Scan(&project.ID, &project.Title, &project.Slug, &description, &kind, &status, &health, &workspaceID, &sandboxID,
		&icon, &color, &memoryScope,
		&createdAt, &updatedAt, &project.CreatedBy, &attrs); err != nil {
		return schema.Project{}, err
	}
	project.Description = description.String
	project.Kind = schema.ProjectKind(kind)
	project.Status = schema.ProjectStatus(status)
	project.Health = schema.ProjectHealth(health)
	project.WorkspaceID = workspaceID.String
	project.SandboxProfileID = sandboxID.String
	project.Icon = icon
	project.Color = color
	project.MemoryScope = schema.MemoryScope(memoryScope)
	if createdAt != "" {
		if ts, err := parseTime(createdAt); err == nil {
			project.CreatedAt = ts
		}
	}
	if updatedAt != "" {
		if ts, err := parseTime(updatedAt); err == nil {
			project.UpdatedAt = ts
		}
	}
	if attrs == "" {
		attrs = "{}"
	}
	if err := json.Unmarshal([]byte(attrs), &project.Attributes); err != nil {
		return schema.Project{}, fmt.Errorf("store: decode project attributes: %w", err)
	}
	if project.Attributes == nil {
		project.Attributes = map[string]interface{}{}
	}
	return project, nil
}

func normalizeAndValidateProject(project schema.Project) (schema.Project, error) {
	project.ID = strings.TrimSpace(project.ID)
	project.Title = strings.TrimSpace(project.Title)
	project.Slug = strings.TrimSpace(project.Slug)
	project.CreatedBy = strings.TrimSpace(project.CreatedBy)
	project.WorkspaceID = strings.TrimSpace(project.WorkspaceID)
	project.SandboxProfileID = strings.TrimSpace(project.SandboxProfileID)
	if project.ID == "" {
		return project, &ValidationError{Field: "project_id", Message: "project_id is required"}
	}
	if project.Title == "" {
		return project, &ValidationError{Field: "title", Message: "title is required"}
	}
	if project.Slug == "" {
		project.Slug = slugifyProjectTitle(project.Title)
	}
	if project.Slug == "" {
		return project, &ValidationError{Field: "slug", Message: "slug is required"}
	}
	if project.Icon == "" {
		project.Icon = "folder"
	}
	if project.Color == "" {
		project.Color = "#3b82f6"
	}
	if project.MemoryScope == "" {
		project.MemoryScope = schema.MemoryScopeDefault
	}
	if parsed, err := schema.ParseMemoryScope(string(project.MemoryScope)); err == nil {
		project.MemoryScope = parsed
	} else {
		return project, &ValidationError{Field: "memory_scope", Message: err.Error()}
	}
	if project.Kind == "" {
		project.Kind = schema.ProjectKindGeneral
	}
	if parsed, err := schema.ParseProjectKind(string(project.Kind)); err == nil {
		project.Kind = parsed
	} else {
		return project, &ValidationError{Field: "project_kind", Message: err.Error()}
	}
	if project.Status == "" {
		project.Status = schema.ProjectStatusActive
	}
	if parsed, err := schema.ParseProjectStatus(string(project.Status)); err == nil {
		project.Status = parsed
	} else {
		return project, &ValidationError{Field: "status", Message: err.Error()}
	}
	if project.Health == "" {
		project.Health = schema.ProjectHealthUnknown
	}
	if parsed, err := schema.ParseProjectHealth(string(project.Health)); err == nil {
		project.Health = parsed
	} else {
		return project, &ValidationError{Field: "health", Message: err.Error()}
	}
	now := time.Now().UTC()
	if project.CreatedAt.IsZero() {
		project.CreatedAt = now
	}
	if project.UpdatedAt.IsZero() {
		project.UpdatedAt = project.CreatedAt
	}
	if project.CreatedBy == "" {
		return project, &ValidationError{Field: "created_by", Message: "created_by is required"}
	}
	if project.Attributes == nil {
		project.Attributes = map[string]interface{}{}
	}
	return project, nil
}

func validateProjectWorkspaceBinding(ctx context.Context, db *sql.DB, projectID, workspaceID string) error {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil
	}
	workspace, err := GetWorkspace(ctx, db, workspaceID)
	if err != nil {
		return &ValidationError{Field: "workspace_id", Message: "workspace not found"}
	}
	if workspace.Status != schema.WorkspaceStatusActive {
		return &ValidationError{Field: "workspace_id", Message: "workspace must be active"}
	}
	if strings.TrimSpace(workspace.RelatedProjectID) != "" && strings.TrimSpace(workspace.RelatedProjectID) != strings.TrimSpace(projectID) {
		return &ValidationError{
			Field:   "workspace_id",
			Message: fmt.Sprintf("workspace %q is already linked to project %q", workspaceID, workspace.RelatedProjectID),
		}
	}
	return nil
}

func validateProjectSandboxProfileBinding(ctx context.Context, db *sql.DB, sandboxProfileID string) error {
	sandboxProfileID = strings.TrimSpace(sandboxProfileID)
	if sandboxProfileID == "" {
		return nil
	}
	profile, err := GetSandboxProfile(ctx, db, sandboxProfileID)
	if err != nil {
		return &ValidationError{Field: "sandbox_profile_id", Message: "sandbox profile not found"}
	}
	if profile.Status != schema.SandboxProfileStatusActive {
		return &ValidationError{Field: "sandbox_profile_id", Message: "sandbox profile must be active"}
	}
	return nil
}

func slugifyProjectTitle(title string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case unicode.IsSpace(r) || r == '-' || r == '_' || r == '.':
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
