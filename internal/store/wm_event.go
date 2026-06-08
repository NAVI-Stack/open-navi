package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ceoai/navi/internal/schema"
	"github.com/google/uuid"
)

// SaveWorldModelEvent inserts or updates a world-model event (scheduling/temporal).
func SaveWorldModelEvent(ctx context.Context, db *sql.DB, e schema.WorldModelEvent) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	e.UpdatedAt = now
	var endTime *string
	if e.EndTime != nil {
		s := e.EndTime.UTC().Format(timeFormat)
		endTime = &s
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO wm_events (id, owner_id, title, start_time, end_time, kind, source, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			owner_id=excluded.owner_id,
			title=excluded.title,
			start_time=excluded.start_time,
			end_time=excluded.end_time,
			kind=excluded.kind,
			source=excluded.source,
			metadata=excluded.metadata,
			updated_at=excluded.updated_at
	`, e.ID, e.OwnerID, e.Title, e.StartTime.UTC().Format(timeFormat), endTime, e.Kind, e.Source, e.Metadata, e.CreatedAt.Format(timeFormat), e.UpdatedAt.Format(timeFormat))
	if err != nil {
		return fmt.Errorf("store: save wm_event: %w", err)
	}
	return nil
}

// ListWorldModelEvents returns events for an owner in a time range (inclusive start, exclusive end).
func ListWorldModelEvents(ctx context.Context, db *sql.DB, ownerID string, start, end time.Time, limit int) ([]schema.WorldModelEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, owner_id, title, start_time, end_time, kind, source, metadata, created_at, updated_at
		FROM wm_events
		WHERE owner_id = ? AND start_time >= ? AND start_time < ?
		ORDER BY start_time ASC
		LIMIT ?
	`, ownerID, start.UTC().Format(timeFormat), end.UTC().Format(timeFormat), limit)
	if err != nil {
		return nil, fmt.Errorf("store: list wm_events: %w", err)
	}
	defer rows.Close()
	var out []schema.WorldModelEvent
	for rows.Next() {
		var e schema.WorldModelEvent
		var startStr, createdStr, updatedStr string
		var endNull sql.NullString
		if err := rows.Scan(&e.ID, &e.OwnerID, &e.Title, &startStr, &endNull, &e.Kind, &e.Source, &e.Metadata, &createdStr, &updatedStr); err != nil {
			return nil, err
		}
		e.StartTime, _ = parseTime(startStr)
		e.CreatedAt, _ = parseTime(createdStr)
		e.UpdatedAt, _ = parseTime(updatedStr)
		if endNull.Valid {
			t, _ := parseTime(endNull.String)
			e.EndTime = &t
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
