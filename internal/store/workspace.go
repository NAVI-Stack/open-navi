package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

var workspaceRuleActionTypes = map[string]struct{}{
	"all":         {},
	"read":        {},
	"write":       {},
	"create":      {},
	"modify":      {},
	"rename_move": {},
	"delete":      {},
	"execute":     {},
}

type workspaceScanner interface {
	Scan(dest ...any) error
}

// SaveWorkspace inserts or updates a workspace.
func SaveWorkspace(ctx context.Context, db *sql.DB, w schema.Workspace) error {
	normalized, err := normalizeAndValidateWorkspace(w)
	if err != nil {
		return err
	}
	if err := validateWorkspaceProjectRelationship(ctx, db, normalized.ID, normalized.RelatedProjectID); err != nil {
		return err
	}

	localRoots, err := json.Marshal(normalized.LocalRoots)
	if err != nil {
		return fmt.Errorf("store: marshal workspace local_roots: %w", err)
	}
	repoRoots, err := json.Marshal(normalized.RepoRoots)
	if err != nil {
		return fmt.Errorf("store: marshal workspace repo_roots: %w", err)
	}
	protectedPaths, err := json.Marshal(normalized.ProtectedPaths)
	if err != nil {
		return fmt.Errorf("store: marshal workspace protected_paths: %w", err)
	}
	allowedActions, err := json.Marshal(normalized.AllowedActions)
	if err != nil {
		return fmt.Errorf("store: marshal workspace allowed_actions: %w", err)
	}
	boundaryPolicy, err := json.Marshal(normalized.BoundaryPolicy)
	if err != nil {
		return fmt.Errorf("store: marshal workspace boundary_policy: %w", err)
	}
	tags, err := json.Marshal(normalized.Tags)
	if err != nil {
		return fmt.Errorf("store: marshal workspace tags: %w", err)
	}

	query := `
		INSERT INTO workspaces (
			id, name, description, kind, status, local_roots, repo_roots, protected_paths,
			allowed_actions, boundary_policy, audit_enabled, created_at, updated_at, created_by,
			tags, related_project_id, notes, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			kind = EXCLUDED.kind,
			status = EXCLUDED.status,
			local_roots = EXCLUDED.local_roots,
			repo_roots = EXCLUDED.repo_roots,
			protected_paths = EXCLUDED.protected_paths,
			allowed_actions = EXCLUDED.allowed_actions,
			boundary_policy = EXCLUDED.boundary_policy,
			audit_enabled = EXCLUDED.audit_enabled,
			updated_at = EXCLUDED.updated_at,
			tags = EXCLUDED.tags,
			related_project_id = EXCLUDED.related_project_id,
			notes = EXCLUDED.notes,
			metadata = EXCLUDED.metadata
	`

	_, err = db.ExecContext(ctx, query,
		normalized.ID,
		normalized.Name,
		normalized.Description,
		string(normalized.Kind),
		string(normalized.Status),
		string(localRoots),
		string(repoRoots),
		string(protectedPaths),
		string(allowedActions),
		string(boundaryPolicy),
		normalized.AuditEnabled,
		normalized.CreatedAt.UTC().Format(timeFormat),
		normalized.UpdatedAt.UTC().Format(timeFormat),
		normalized.CreatedBy,
		string(tags),
		nullIfEmptyWorkspaceValue(normalized.RelatedProjectID),
		nullIfEmptyWorkspaceValue(normalized.Notes),
		normalized.Metadata,
	)
	if err != nil {
		if strings.Contains(err.Error(), "idx_workspaces_active_related_project_id") ||
			strings.Contains(err.Error(), "workspaces.related_project_id") {
			return &ValidationError{
				Entity:  "workspace",
				Field:   "related_project_id",
				Message: fmt.Sprintf("project %q is already bound to another active workspace", normalized.RelatedProjectID),
			}
		}
		return fmt.Errorf("store: save workspace: %w", err)
	}

	return nil
}

// GetWorkspace retrieves a workspace by ID, including whitelist rules.
func GetWorkspace(ctx context.Context, db *sql.DB, id string) (schema.Workspace, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return schema.Workspace{}, &ValidationError{Entity: "workspace", Field: "workspace_id", Message: "is required"}
	}

	query := `
		SELECT
			id, name, description, kind, status, local_roots, repo_roots, protected_paths,
			allowed_actions, boundary_policy, audit_enabled, created_at, updated_at, created_by,
			tags, related_project_id, notes, metadata
		FROM workspaces WHERE id = ?
	`

	w, err := scanWorkspace(db.QueryRowContext(ctx, query, id))
	if err == sql.ErrNoRows {
		return schema.Workspace{}, fmt.Errorf("store: workspace not found: %s", id)
	}
	if err != nil {
		return schema.Workspace{}, err
	}
	rules, err := ListWhitelistRules(ctx, db, w.ID)
	if err != nil {
		return schema.Workspace{}, fmt.Errorf("store: get workspace rules: %w", err)
	}
	w.WhitelistRules = rules
	return w, nil
}

// GetWorkspaceByProjectID retrieves a workspace bound to the given project ID.
func GetWorkspaceByProjectID(ctx context.Context, db *sql.DB, projectID string) (schema.Workspace, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return schema.Workspace{}, &ValidationError{Entity: "workspace", Field: "related_project_id", Message: "is required"}
	}

	query := `
		SELECT
			id, name, description, kind, status, local_roots, repo_roots, protected_paths,
			allowed_actions, boundary_policy, audit_enabled, created_at, updated_at, created_by,
			tags, related_project_id, notes, metadata
		FROM workspaces
		WHERE related_project_id = ? AND status = ?
		LIMIT 1
	`

	w, err := scanWorkspace(db.QueryRowContext(ctx, query, projectID, string(schema.WorkspaceStatusActive)))
	if err == sql.ErrNoRows {
		return schema.Workspace{}, fmt.Errorf("store: no active workspace bound to project: %s", projectID)
	}
	if err != nil {
		return schema.Workspace{}, err
	}
	rules, err := ListWhitelistRules(ctx, db, w.ID)
	if err != nil {
		return schema.Workspace{}, fmt.Errorf("store: get workspace rules: %w", err)
	}
	w.WhitelistRules = rules
	return w, nil
}

// ListWorkspaces returns workspaces, optionally filtered by status.
func ListWorkspaces(ctx context.Context, db *sql.DB, status string) ([]schema.Workspace, error) {
	status = strings.TrimSpace(status)
	query := `
		SELECT
			id, name, description, kind, status, local_roots, repo_roots, protected_paths,
			allowed_actions, boundary_policy, audit_enabled, created_at, updated_at, created_by,
			tags, related_project_id, notes, metadata
		FROM workspaces
	`
	var args []any
	if status != "" {
		parsed, err := schema.ParseWorkspaceStatus(status)
		if err != nil {
			return nil, &ValidationError{Entity: "workspace", Field: "status", Message: err.Error()}
		}
		query += " WHERE status = ?"
		args = append(args, string(parsed))
	}
	query += " ORDER BY updated_at DESC, id ASC"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list workspaces: %w", err)
	}
	defer rows.Close()

	var workspaces []schema.Workspace
	for rows.Next() {
		w, err := scanWorkspace(rows)
		if err != nil {
			return nil, err
		}
		workspaces = append(workspaces, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list workspaces rows: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("store: close workspace rows: %w", err)
	}
	for i := range workspaces {
		rules, err := ListWhitelistRules(ctx, db, workspaces[i].ID)
		if err != nil {
			return nil, fmt.Errorf("store: get workspace rules: %w", err)
		}
		workspaces[i].WhitelistRules = rules
	}
	return workspaces, nil
}

// ArchiveWorkspace marks a workspace as archived.
func ArchiveWorkspace(ctx context.Context, db *sql.DB, id string, archivedAt time.Time) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return &ValidationError{Entity: "workspace", Field: "workspace_id", Message: "is required"}
	}
	if archivedAt.IsZero() {
		archivedAt = time.Now().UTC()
	}
	res, err := db.ExecContext(ctx, `
		UPDATE workspaces
		SET status = ?, updated_at = ?
		WHERE id = ?
	`, string(schema.WorkspaceStatusArchived), archivedAt.UTC().Format(timeFormat), id)
	if err != nil {
		return fmt.Errorf("store: archive workspace: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: archive workspace rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("store: workspace not found: %s", id)
	}
	return nil
}

// DeleteWorkspace permanently removes a workspace. Foreign-key constraints are
// allowed to reject deletion when projects or other durable records still point
// at the workspace.
func DeleteWorkspace(ctx context.Context, db *sql.DB, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return &ValidationError{Entity: "workspace", Field: "workspace_id", Message: "is required"}
	}
	res, err := db.ExecContext(ctx, `DELETE FROM workspaces WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete workspace: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete workspace rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("store: workspace not found: %s", id)
	}
	return nil
}

// GetWorkspaceID returns the explicitly selected active workspace ID, if any.
// V1 does not silently choose a workspace from the set of active workspaces.
func GetWorkspaceID(ctx context.Context, db *sql.DB) (string, error) {
	id, ok, err := GetConfigurationValue(ctx, db, "owner", "active", "active_workspace_id")
	if err != nil {
		return "", err
	}
	if !ok {
		return "", nil
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return "", nil
	}
	workspace, err := GetWorkspace(ctx, db, id)
	if err != nil || workspace.Status != schema.WorkspaceStatusActive {
		return "", nil
	}
	return workspace.ID, nil
}

// SaveWhitelistRule inserts or updates a whitelist rule for a workspace.
func SaveWhitelistRule(ctx context.Context, db *sql.DB, r schema.WhitelistRule, workspaceID string) error {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return &ValidationError{Entity: "whitelist_rule", Field: "workspace_id", Message: "is required"}
	}

	normalized, err := normalizeAndValidateWhitelistRule(r)
	if err != nil {
		return err
	}

	actionTypes, err := json.Marshal(normalized.ActionTypes)
	if err != nil {
		return fmt.Errorf("store: marshal whitelist rule action_types: %w", err)
	}

	var revokedAt any
	if normalized.RevokedAt != nil {
		revokedAt = normalized.RevokedAt.UTC().Format(timeFormat)
	}
	var expiresAt any
	if normalized.ExpiresAt != nil {
		expiresAt = normalized.ExpiresAt.UTC().Format(timeFormat)
	}

	query := `
		INSERT INTO workspace_whitelist_rules (
			rule_id, workspace_id, scope, action_types, status, created_at, revoked_at, expires_at, created_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(rule_id) DO UPDATE SET
			scope = EXCLUDED.scope,
			action_types = EXCLUDED.action_types,
			status = EXCLUDED.status,
			revoked_at = EXCLUDED.revoked_at,
			expires_at = EXCLUDED.expires_at,
			created_by = EXCLUDED.created_by
	`

	_, err = db.ExecContext(ctx, query,
		normalized.RuleID,
		workspaceID,
		normalized.Scope,
		string(actionTypes),
		string(normalized.Status),
		normalized.CreatedAt.UTC().Format(timeFormat),
		revokedAt,
		expiresAt,
		normalized.CreatedBy,
	)
	if err != nil {
		return fmt.Errorf("store: save whitelist rule: %w", err)
	}

	return nil
}

// ListWhitelistRules returns all rules for a workspace.
func ListWhitelistRules(ctx context.Context, db *sql.DB, workspaceID string) ([]schema.WhitelistRule, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, &ValidationError{Entity: "whitelist_rule", Field: "workspace_id", Message: "is required"}
	}

	query := `
		SELECT
			rule_id, scope, action_types, status, created_at, revoked_at, expires_at, created_by
		FROM workspace_whitelist_rules
		WHERE workspace_id = ?
		ORDER BY created_at DESC, rule_id ASC
	`

	rows, err := db.QueryContext(ctx, query, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("store: list whitelist rules: %w", err)
	}
	defer rows.Close()

	var rules []schema.WhitelistRule
	for rows.Next() {
		var (
			r           schema.WhitelistRule
			actionTypes string
			status      string
			createdAt   string
			revokedAt   sql.NullString
			expiresAt   sql.NullString
		)

		if err := rows.Scan(
			&r.RuleID,
			&r.Scope,
			&actionTypes,
			&status,
			&createdAt,
			&revokedAt,
			&expiresAt,
			&r.CreatedBy,
		); err != nil {
			return nil, fmt.Errorf("store: scan whitelist rule: %w", err)
		}

		parsedStatus, err := schema.ParseWhitelistRuleStatus(status)
		if err != nil {
			return nil, fmt.Errorf("store: parse whitelist rule status: %w", err)
		}
		r.Status = parsedStatus

		r.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, fmt.Errorf("store: parse whitelist rule created_at: %w", err)
		}
		if revokedAt.Valid {
			t, err := parseTime(revokedAt.String)
			if err != nil {
				return nil, fmt.Errorf("store: parse whitelist rule revoked_at: %w", err)
			}
			r.RevokedAt = &t
		}
		if expiresAt.Valid {
			t, err := parseTime(expiresAt.String)
			if err != nil {
				return nil, fmt.Errorf("store: parse whitelist rule expires_at: %w", err)
			}
			r.ExpiresAt = &t
		}
		if err := json.Unmarshal([]byte(actionTypes), &r.ActionTypes); err != nil {
			return nil, fmt.Errorf("store: decode whitelist rule action_types: %w", err)
		}

		rules = append(rules, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list whitelist rules rows: %w", err)
	}

	return rules, nil
}

func scanWorkspace(scanner workspaceScanner) (schema.Workspace, error) {
	var (
		w                schema.Workspace
		kind             string
		status           string
		localRoots       string
		repoRoots        string
		protectedPaths   string
		allowedActions   string
		boundaryPolicy   string
		auditEnabled     int
		createdAt        string
		updatedAt        string
		tags             string
		description      sql.NullString
		relatedProjectID sql.NullString
		notes            sql.NullString
		metadata         sql.NullString
	)

	if err := scanner.Scan(
		&w.ID,
		&w.Name,
		&description,
		&kind,
		&status,
		&localRoots,
		&repoRoots,
		&protectedPaths,
		&allowedActions,
		&boundaryPolicy,
		&auditEnabled,
		&createdAt,
		&updatedAt,
		&w.CreatedBy,
		&tags,
		&relatedProjectID,
		&notes,
		&metadata,
	); err != nil {
		if err == sql.ErrNoRows {
			return schema.Workspace{}, err
		}
		return schema.Workspace{}, fmt.Errorf("store: scan workspace: %w", err)
	}

	w.Description = description.String
	w.RelatedProjectID = relatedProjectID.String
	w.Notes = notes.String
	w.Metadata = metadata.String
	if w.Metadata == "" {
		w.Metadata = "{}"
	}

	parsedKind, err := schema.ParseWorkspaceKind(kind)
	if err != nil {
		return schema.Workspace{}, fmt.Errorf("store: parse workspace kind: %w", err)
	}
	w.Kind = parsedKind

	parsedStatus, err := schema.ParseWorkspaceStatus(status)
	if err != nil {
		return schema.Workspace{}, fmt.Errorf("store: parse workspace status: %w", err)
	}
	w.Status = parsedStatus
	w.AuditEnabled = auditEnabled != 0

	w.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return schema.Workspace{}, fmt.Errorf("store: parse workspace created_at: %w", err)
	}
	w.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return schema.Workspace{}, fmt.Errorf("store: parse workspace updated_at: %w", err)
	}

	if err := json.Unmarshal([]byte(localRoots), &w.LocalRoots); err != nil {
		return schema.Workspace{}, fmt.Errorf("store: decode workspace local_roots: %w", err)
	}
	if err := json.Unmarshal([]byte(repoRoots), &w.RepoRoots); err != nil {
		return schema.Workspace{}, fmt.Errorf("store: decode workspace repo_roots: %w", err)
	}
	if err := json.Unmarshal([]byte(protectedPaths), &w.ProtectedPaths); err != nil {
		return schema.Workspace{}, fmt.Errorf("store: decode workspace protected_paths: %w", err)
	}
	if err := json.Unmarshal([]byte(allowedActions), &w.AllowedActions); err != nil {
		return schema.Workspace{}, fmt.Errorf("store: decode workspace allowed_actions: %w", err)
	}
	if err := json.Unmarshal([]byte(boundaryPolicy), &w.BoundaryPolicy); err != nil {
		return schema.Workspace{}, fmt.Errorf("store: decode workspace boundary_policy: %w", err)
	}
	if err := json.Unmarshal([]byte(tags), &w.Tags); err != nil {
		return schema.Workspace{}, fmt.Errorf("store: decode workspace tags: %w", err)
	}

	return w, nil
}

func normalizeAndValidateWorkspace(w schema.Workspace) (schema.Workspace, error) {
	var err error

	w.ID = strings.TrimSpace(w.ID)
	if w.ID == "" {
		return schema.Workspace{}, &ValidationError{Entity: "workspace", Field: "workspace_id", Message: "is required"}
	}
	w.Name = strings.TrimSpace(w.Name)
	if w.Name == "" {
		return schema.Workspace{}, &ValidationError{Entity: "workspace", Field: "name", Message: "is required"}
	}
	w.Description = strings.TrimSpace(w.Description)

	kind, err := schema.ParseWorkspaceKind(string(w.Kind))
	if err != nil {
		return schema.Workspace{}, &ValidationError{Entity: "workspace", Field: "workspace_kind", Message: err.Error()}
	}
	w.Kind = kind

	status, err := schema.ParseWorkspaceStatus(string(w.Status))
	if err != nil {
		return schema.Workspace{}, &ValidationError{Entity: "workspace", Field: "status", Message: err.Error()}
	}
	w.Status = status

	w.LocalRoots = normalizeStringList(w.LocalRoots)
	w.RepoRoots = normalizeStringList(w.RepoRoots)
	w.ProtectedPaths = normalizeStringList(w.ProtectedPaths)
	w.Tags = normalizeStringList(w.Tags)
	w.RelatedProjectID = strings.TrimSpace(w.RelatedProjectID)
	w.Notes = strings.TrimSpace(w.Notes)

	outOfScope, err := schema.ParseBoundaryPolicyOutOfScopeDefault(string(w.BoundaryPolicy.OutOfScopeDefault))
	if err != nil {
		return schema.Workspace{}, &ValidationError{Entity: "workspace", Field: "boundary_policy.out_of_scope_default", Message: err.Error()}
	}
	w.BoundaryPolicy.OutOfScopeDefault = outOfScope

	w.CreatedBy = strings.TrimSpace(w.CreatedBy)
	if w.CreatedBy == "" {
		return schema.Workspace{}, &ValidationError{Entity: "workspace", Field: "created_by", Message: "is required"}
	}

	now := time.Now().UTC()
	if w.CreatedAt.IsZero() {
		w.CreatedAt = now
	}
	if w.UpdatedAt.IsZero() {
		w.UpdatedAt = now
	}
	if w.UpdatedAt.Before(w.CreatedAt) {
		return schema.Workspace{}, &ValidationError{Entity: "workspace", Field: "updated_at", Message: "must be greater than or equal to created_at"}
	}

	w.Metadata = strings.TrimSpace(w.Metadata)
	if w.Metadata == "" {
		w.Metadata = "{}"
	}
	if !json.Valid([]byte(w.Metadata)) {
		return schema.Workspace{}, &ValidationError{Entity: "workspace", Field: "metadata", Message: "must be valid JSON"}
	}

	return w, nil
}

func normalizeAndValidateWhitelistRule(r schema.WhitelistRule) (schema.WhitelistRule, error) {
	var err error

	r.RuleID = strings.TrimSpace(r.RuleID)
	if r.RuleID == "" {
		return schema.WhitelistRule{}, &ValidationError{Entity: "whitelist_rule", Field: "rule_id", Message: "is required"}
	}
	r.Scope = strings.TrimSpace(r.Scope)
	if r.Scope == "" {
		return schema.WhitelistRule{}, &ValidationError{Entity: "whitelist_rule", Field: "scope", Message: "is required"}
	}

	r.ActionTypes, err = normalizeWhitelistActionTypes(r.ActionTypes)
	if err != nil {
		return schema.WhitelistRule{}, err
	}

	status, err := schema.ParseWhitelistRuleStatus(string(r.Status))
	if err != nil {
		return schema.WhitelistRule{}, &ValidationError{Entity: "whitelist_rule", Field: "status", Message: err.Error()}
	}
	r.Status = status

	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	r.CreatedBy = strings.TrimSpace(r.CreatedBy)
	if r.CreatedBy == "" {
		return schema.WhitelistRule{}, &ValidationError{Entity: "whitelist_rule", Field: "created_by", Message: "is required"}
	}
	if r.ExpiresAt != nil && !r.ExpiresAt.After(r.CreatedAt) {
		return schema.WhitelistRule{}, &ValidationError{Entity: "whitelist_rule", Field: "expires_at", Message: "must be after created_at"}
	}
	if r.Status == schema.WhitelistRuleStatusRevoked && r.RevokedAt == nil {
		return schema.WhitelistRule{}, &ValidationError{Entity: "whitelist_rule", Field: "revoked_at", Message: "is required when status is revoked"}
	}
	if r.Status == schema.WhitelistRuleStatusActive && r.RevokedAt != nil {
		return schema.WhitelistRule{}, &ValidationError{Entity: "whitelist_rule", Field: "revoked_at", Message: "must be empty when status is active"}
	}

	return r, nil
}

func normalizeWhitelistActionTypes(actionTypes []string) ([]string, error) {
	if len(actionTypes) == 0 {
		return []string{"all"}, nil
	}
	normalized := make([]string, 0, len(actionTypes))
	seen := make(map[string]struct{}, len(actionTypes))
	for _, actionType := range actionTypes {
		value := strings.ToLower(strings.TrimSpace(actionType))
		if value == "" {
			continue
		}
		if value != "all" {
			if _, err := schema.ParseWorkspaceActionType(value); err != nil {
				return nil, &ValidationError{Entity: "whitelist_rule", Field: "action_types", Message: fmt.Sprintf("contains unsupported action type %q", actionType)}
			}
		}
		if _, ok := workspaceRuleActionTypes[value]; !ok {
			return nil, &ValidationError{Entity: "whitelist_rule", Field: "action_types", Message: fmt.Sprintf("contains unsupported action type %q", actionType)}
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return []string{"all"}, nil
	}
	return normalized, nil
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return []string{}
	}
	return normalized
}

func nullIfEmptyWorkspaceValue(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}
