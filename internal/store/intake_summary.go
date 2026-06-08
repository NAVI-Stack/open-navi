package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// IntakeSummaryNode is one entry in the Fold stage's summary index (CIP stage 9,
// §13): a flat, by-entity-type rollup of what synthesis has written. It is
// DERIVED state — rebuildable from entity_provenance / intake_provenance and never
// authoritative. P3 keeps it deliberately shallow; hierarchical depth iterates
// later.
type IntakeSummaryNode struct {
	EntityType   string
	EntityCount  int
	LastEntityID string
	LastSummary  string
	UpdatedAt    time.Time
}

// BumpIntakeSummary increments the entity_count for entity_type and records the
// most recent entity id/summary. It is an additive rollup; losing it loses no
// authoritative state.
func BumpIntakeSummary(ctx context.Context, db *sql.DB, entityType, lastEntityID, lastSummary string, delta int) error {
	now := time.Now().UTC().Format(timeFormat)
	_, err := db.ExecContext(ctx, `
		INSERT INTO intake_summary (entity_type, entity_count, last_entity_id, last_summary, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(entity_type) DO UPDATE SET
			entity_count = entity_count + excluded.entity_count,
			last_entity_id = excluded.last_entity_id,
			last_summary = excluded.last_summary,
			updated_at = excluded.updated_at
	`, entityType, delta, lastEntityID, lastSummary, now)
	if err != nil {
		return fmt.Errorf("store: bump intake summary: %w", err)
	}
	return nil
}

// GetIntakeSummary returns the summary node for an entity type, if any.
func GetIntakeSummary(ctx context.Context, db *sql.DB, entityType string) (IntakeSummaryNode, bool, error) {
	var (
		n         IntakeSummaryNode
		lastID    sql.NullString
		lastSum   sql.NullString
		updatedAt string
	)
	err := db.QueryRowContext(ctx, `
		SELECT entity_type, entity_count, last_entity_id, last_summary, updated_at
		FROM intake_summary WHERE entity_type = ?
	`, entityType).Scan(&n.EntityType, &n.EntityCount, &lastID, &lastSum, &updatedAt)
	if err == sql.ErrNoRows {
		return IntakeSummaryNode{}, false, nil
	}
	if err != nil {
		return IntakeSummaryNode{}, false, fmt.Errorf("store: get intake summary: %w", err)
	}
	n.LastEntityID = lastID.String
	n.LastSummary = lastSum.String
	n.UpdatedAt, _ = parseTime(updatedAt)
	return n, true, nil
}

// ListIntakeSummary returns all summary nodes ordered by entity type.
func ListIntakeSummary(ctx context.Context, db *sql.DB) ([]IntakeSummaryNode, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT entity_type, entity_count, last_entity_id, last_summary, updated_at
		FROM intake_summary ORDER BY entity_type ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list intake summary: %w", err)
	}
	defer rows.Close()
	var out []IntakeSummaryNode
	for rows.Next() {
		var (
			n         IntakeSummaryNode
			lastID    sql.NullString
			lastSum   sql.NullString
			updatedAt string
		)
		if err := rows.Scan(&n.EntityType, &n.EntityCount, &lastID, &lastSum, &updatedAt); err != nil {
			return nil, fmt.Errorf("store: scan intake summary: %w", err)
		}
		n.LastEntityID = lastID.String
		n.LastSummary = lastSum.String
		n.UpdatedAt, _ = parseTime(updatedAt)
		out = append(out, n)
	}
	return out, rows.Err()
}
