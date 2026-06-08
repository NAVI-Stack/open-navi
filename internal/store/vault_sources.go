package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/open-navi/navi/internal/schema"
)

// NAVI-VAULT-V1 — read-only projection sources.
//
// The Memory Vault projects four entity classes (Contacts, Knowledge, Memories,
// Artifacts) to Markdown. The projector needs to enumerate all entities of each
// class regardless of scope/workspace. These are read-only helpers (raw SQL, no
// ORM) used only by the Vault projector; they never write the World Model.

// ListAllFacts returns all knowledge facts across scopes, newest first.
// Deprecated facts are excluded unless includeDeprecated is set.
func ListAllFacts(ctx context.Context, db *sql.DB, includeDeprecated bool, limit int) ([]Fact, error) {
	if limit <= 0 {
		limit = 1000
	}
	where := ""
	if !includeDeprecated {
		where = "WHERE COALESCE(deprecated, 0) = 0"
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, scope, scope_id, category, key, value,
		       COALESCE(keywords, '[]'), COALESCE(tags, '[]'), COALESCE(embedding, '[]'),
		       source, COALESCE(deprecated, 0), created_at, updated_at
		FROM facts `+where+`
		ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list all facts: %w", err)
	}
	defer rows.Close()
	var out []Fact
	for rows.Next() {
		var f Fact
		var createdAt, updatedAt, keywordsJSON, tagsJSON, embeddingJSON string
		var deprecated int
		if err := rows.Scan(&f.ID, &f.Scope, &f.ScopeID, &f.Category, &f.Key, &f.Value,
			&keywordsJSON, &tagsJSON, &embeddingJSON, &f.Source, &deprecated, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("store: scan fact: %w", err)
		}
		f.Deprecated = deprecated != 0
		f.Keywords = ParseJSONStringSlice(keywordsJSON)
		f.Tags = ParseJSONStringSlice(tagsJSON)
		f.Embedding = ParseJSONVector(embeddingJSON)
		f.CreatedAt, _ = parseTime(createdAt)
		f.UpdatedAt, _ = parseTime(updatedAt)
		out = append(out, f)
	}
	return out, rows.Err()
}

// ListAllMemories returns all memories across scopes, newest first.
func ListAllMemories(ctx context.Context, db *sql.DB, limit int) ([]schema.Memory, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, scope, scope_id, summary, details,
		       COALESCE(keywords, '[]'), COALESCE(tags, '[]'), COALESCE(embedding, '[]'),
		       significance, source, created_at, updated_at
		FROM memories ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list all memories: %w", err)
	}
	defer rows.Close()
	var out []schema.Memory
	for rows.Next() {
		var m schema.Memory
		var createdStr, updatedStr, keywordsJSON, tagsJSON, embeddingJSON string
		if err := rows.Scan(&m.ID, &m.Scope, &m.ScopeID, &m.Summary, &m.Details,
			&keywordsJSON, &tagsJSON, &embeddingJSON, &m.Significance, &m.Source, &createdStr, &updatedStr); err != nil {
			return nil, fmt.Errorf("store: scan memory: %w", err)
		}
		m.Keywords = ParseJSONStringSlice(keywordsJSON)
		m.Tags = ParseJSONStringSlice(tagsJSON)
		m.Embedding = ParseJSONVector(embeddingJSON)
		m.CreatedAt, _ = parseTime(createdStr)
		m.UpdatedAt, _ = parseTime(updatedStr)
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListAllArtifacts returns all artifacts across workspaces, newest first.
// Archived artifacts are included; the projector decides how to render them.
func ListAllArtifacts(ctx context.Context, db *sql.DB, limit int) ([]schema.Artifact, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, workspace_id, project_id, owner_id, canonical_title, display_title, type, subtype,
		       schema_version, content_format, lifecycle_state, current_branch_id, current_version_id,
		       head_version_number, created_by_actor_type, created_by_actor_id, provenance_root_id,
		       attributes, created_at, updated_at, archived_at
		FROM artifacts ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list all artifacts: %w", err)
	}
	defer rows.Close()
	var out []schema.Artifact
	for rows.Next() {
		a, err := scanArtifactRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}
