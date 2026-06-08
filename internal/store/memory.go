package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/open-navi/navi/internal/schema"
)

// SaveMemory inserts or updates a memory (significant experience). Returns the saved memory with ID set when inserting.
func SaveMemory(ctx context.Context, db *sql.DB, m schema.Memory) (schema.Memory, error) {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	inferMemoryKnowledge(ctx, &m)
	now := time.Now().UTC()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now
	_, err := db.ExecContext(ctx, `
		INSERT INTO memories (id, scope, scope_id, summary, details, keywords, tags, embedding, significance, source, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			scope=excluded.scope,
			scope_id=excluded.scope_id,
			summary=excluded.summary,
			details=excluded.details,
			keywords=excluded.keywords,
			tags=excluded.tags,
			embedding=excluded.embedding,
			significance=excluded.significance,
			source=excluded.source,
			updated_at=excluded.updated_at
	`, m.ID, m.Scope, m.ScopeID, m.Summary, m.Details,
		marshalJSONStringSlice(m.Keywords), marshalJSONStringSlice(m.Tags), marshalJSONVector(m.Embedding),
		m.Significance, m.Source, m.CreatedAt.Format(timeFormat), m.UpdatedAt.Format(timeFormat))
	if err != nil {
		return schema.Memory{}, fmt.Errorf("store: save memory: %w", err)
	}
	return m, nil
}

// ListMemories returns memories for a scope (e.g. owner or chat), most recent first.
func ListMemories(ctx context.Context, db *sql.DB, scope, scopeID string, limit int) ([]schema.Memory, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, scope, scope_id, summary, details, COALESCE(keywords, '[]'), COALESCE(tags, '[]'), COALESCE(embedding, '[]'), significance, source, created_at, updated_at
		FROM memories
		WHERE scope = ? AND scope_id = ?
		ORDER BY updated_at DESC
		LIMIT ?
	`, scope, scopeID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list memories: %w", err)
	}
	defer rows.Close()
	var out []schema.Memory
	for rows.Next() {
		var m schema.Memory
		var createdStr, updatedStr, keywordsJSON, tagsJSON, embeddingJSON string
		if err := rows.Scan(&m.ID, &m.Scope, &m.ScopeID, &m.Summary, &m.Details, &keywordsJSON, &tagsJSON, &embeddingJSON, &m.Significance, &m.Source, &createdStr, &updatedStr); err != nil {
			return nil, err
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

// GetMemory returns a single memory by ID.
func GetMemory(ctx context.Context, db *sql.DB, id string) (schema.Memory, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, scope, scope_id, summary, details, COALESCE(keywords, '[]'), COALESCE(tags, '[]'), COALESCE(embedding, '[]'), significance, source, created_at, updated_at
		FROM memories WHERE id = ?
	`, id)
	var m schema.Memory
	var createdStr, updatedStr, keywordsJSON, tagsJSON, embeddingJSON string
	if err := row.Scan(&m.ID, &m.Scope, &m.ScopeID, &m.Summary, &m.Details, &keywordsJSON, &tagsJSON, &embeddingJSON, &m.Significance, &m.Source, &createdStr, &updatedStr); err != nil {
		if err == sql.ErrNoRows {
			return schema.Memory{}, fmt.Errorf("store: memory not found: %w", err)
		}
		return schema.Memory{}, err
	}
	m.Keywords = ParseJSONStringSlice(keywordsJSON)
	m.Tags = ParseJSONStringSlice(tagsJSON)
	m.Embedding = ParseJSONVector(embeddingJSON)
	m.CreatedAt, _ = parseTime(createdStr)
	m.UpdatedAt, _ = parseTime(updatedStr)
	return m, nil
}
