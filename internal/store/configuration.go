package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ceoai/navi/internal/schema"
)

// ListConfigurationByScope returns configuration entries for a given scope and scope_id.
func ListConfigurationByScope(ctx context.Context, db *sql.DB, scope, scopeID string, limit int) ([]schema.ConfigurationEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, scope, scope_id, key, value, source, created_at, updated_at
		FROM configuration
		WHERE scope = ? AND scope_id = ?
		ORDER BY updated_at DESC
		LIMIT ?
	`, scope, scopeID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list configuration by scope: %w", err)
	}
	defer rows.Close()

	var out []schema.ConfigurationEntry
	for rows.Next() {
		var (
			e          schema.ConfigurationEntry
			createdStr string
			updatedStr string
		)
		if err := rows.Scan(&e.ID, &e.Scope, &e.ScopeID, &e.Key, &e.Value, &e.Source, &createdStr, &updatedStr); err != nil {
			return nil, fmt.Errorf("store: scan configuration: %w", err)
		}
		e.CreatedAt, _ = parseTime(createdStr)
		e.UpdatedAt, _ = parseTime(updatedStr)
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetConfigurationValue returns the value for a given scope, scope_id, and key.
func GetConfigurationValue(ctx context.Context, db *sql.DB, scope, scopeID, key string) (string, bool, error) {
	var value string
	err := db.QueryRowContext(ctx, `
		SELECT value FROM configuration
		WHERE scope = ? AND scope_id = ? AND key = ?
		ORDER BY updated_at DESC LIMIT 1
	`, scope, scopeID, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: get configuration value: %w", err)
	}
	return value, true, nil
}

// SaveConfigurationEntry inserts or updates a configuration entry.
func SaveConfigurationEntry(ctx context.Context, db *sql.DB, e schema.ConfigurationEntry) error {
	query := `
		INSERT INTO configuration (id, scope, scope_id, key, value, source, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			value = EXCLUDED.value,
			source = EXCLUDED.source,
			updated_at = EXCLUDED.updated_at
	`
	_, err := db.ExecContext(ctx, query,
		e.ID, e.Scope, e.ScopeID, e.Key, e.Value, e.Source,
		e.CreatedAt.Format(time.RFC3339), e.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("store: save configuration entry: %w", err)
	}
	return nil
}

// DeleteConfigurationEntriesByScopeAndKeyPrefix removes configuration entries for
// a scope/scope_id pair whose key matches the provided prefix.
func DeleteConfigurationEntriesByScopeAndKeyPrefix(ctx context.Context, db *sql.DB, scope, scopeID, keyPrefix string) error {
	_, err := db.ExecContext(ctx, `
		DELETE FROM configuration
		WHERE scope = ? AND scope_id = ? AND key LIKE ?
	`, scope, scopeID, keyPrefix+"%")
	if err != nil {
		return fmt.Errorf("store: delete configuration entries by scope/key prefix: %w", err)
	}
	return nil
}
