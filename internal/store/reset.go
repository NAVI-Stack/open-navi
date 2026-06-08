package store

import (
	"context"
	"database/sql"
	"fmt"
)

// ResetInstance wipes all instance state so the instance becomes unclaimed.
// Deletes owner, API keys, directives, tasks, settings, and facts in one transaction.
// After ResetInstance, OwnerExists returns false and first-run status becomes uninitialized.
func ResetInstance(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin reset tx: %w", err)
	}
	defer tx.Rollback()

	tables := []string{
		"directive_messages",
		"tasks",
		"directives",
		"api_keys",
		"owners",
		"settings",
		"facts",
	}
	for _, table := range tables {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return fmt.Errorf("store: reset delete %s: %w", table, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: reset commit: %w", err)
	}
	return nil
}
