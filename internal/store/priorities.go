package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ceoai/navi/internal/schema"
)

// ListPrioritiesByScope returns priorities for a given scope and scope_id.
func ListPrioritiesByScope(ctx context.Context, db *sql.DB, scope, scopeID string, limit int) ([]schema.Priority, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, scope, scope_id, name, description, source, created_at, updated_at
		FROM priorities
		WHERE scope = ? AND scope_id = ?
		ORDER BY updated_at DESC
		LIMIT ?
	`, scope, scopeID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list priorities by scope: %w", err)
	}
	defer rows.Close()

	var out []schema.Priority
	for rows.Next() {
		var (
			p          schema.Priority
			createdStr string
			updatedStr string
		)
		if err := rows.Scan(&p.ID, &p.Scope, &p.ScopeID, &p.Name, &p.Description, &p.Source, &createdStr, &updatedStr); err != nil {
			return nil, fmt.Errorf("store: scan priority: %w", err)
		}
		p.CreatedAt, _ = parseTime(createdStr)
		p.UpdatedAt, _ = parseTime(updatedStr)
		out = append(out, p)
	}
	return out, rows.Err()
}

