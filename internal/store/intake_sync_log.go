package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ceoai/navi/internal/schema"
	"github.com/google/uuid"
)

// CIP P5 — intake_sync_log store (raw SQL, no ORM). Append-only record of every
// intake sync pass (CIP §7, §11). One row per connector pass; the intake worker
// is the populator. Powers the Console connector-detail sync state and lets the
// owner answer "what did the last Telegram pass do?".

// AppendIntakeSyncLog inserts one sync-log row and returns its id. Append-only:
// callers never update a closed row; an in-progress pass is written once at
// close with its terminal status.
func AppendIntakeSyncLog(ctx context.Context, db *sql.DB, e schema.IntakeSyncLogEntry) (string, error) {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	if e.StartedAt.IsZero() {
		e.StartedAt = e.CreatedAt
	}
	var endedAt any
	if e.EndedAt != nil {
		endedAt = e.EndedAt.UTC().Format(timeFormat)
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO intake_sync_log (
			id, connector_id, parent_id, job_mode, started_at, ended_at,
			records_admitted, records_deduped, records_distilled, records_synthesized,
			errors, terminal_status, note, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, e.ID, e.ConnectorID, e.ParentID, string(e.JobMode),
		e.StartedAt.UTC().Format(timeFormat), endedAt,
		e.RecordsAdmitted, e.RecordsDeduped, e.RecordsDistilled, e.RecordsSynthesized,
		e.Errors, string(e.TerminalStatus), e.Note, e.CreatedAt.UTC().Format(timeFormat))
	if err != nil {
		return "", fmt.Errorf("store: append intake sync log: %w", err)
	}
	return e.ID, nil
}

// ListIntakeSyncLog returns sync-log rows, newest first. connectorID == ""
// returns all connectors. limit <= 0 defaults to 50.
func ListIntakeSyncLog(ctx context.Context, db *sql.DB, connectorID string, limit int) ([]schema.IntakeSyncLogEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	var (
		rows *sql.Rows
		err  error
	)
	const cols = `id, connector_id, parent_id, job_mode, started_at, ended_at,
		records_admitted, records_deduped, records_distilled, records_synthesized,
		errors, terminal_status, note, created_at`
	if connectorID == "" {
		rows, err = db.QueryContext(ctx, `SELECT `+cols+`
			FROM intake_sync_log ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
	} else {
		rows, err = db.QueryContext(ctx, `SELECT `+cols+`
			FROM intake_sync_log WHERE connector_id = ?
			ORDER BY created_at DESC, id DESC LIMIT ?`, connectorID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("store: list intake sync log: %w", err)
	}
	defer rows.Close()

	var out []schema.IntakeSyncLogEntry
	for rows.Next() {
		var (
			e         schema.IntakeSyncLogEntry
			jobMode   string
			startedAt string
			endedAt   sql.NullString
			status    string
			createdAt string
		)
		if err := rows.Scan(&e.ID, &e.ConnectorID, &e.ParentID, &jobMode, &startedAt, &endedAt,
			&e.RecordsAdmitted, &e.RecordsDeduped, &e.RecordsDistilled, &e.RecordsSynthesized,
			&e.Errors, &status, &e.Note, &createdAt); err != nil {
			return nil, fmt.Errorf("store: scan intake sync log: %w", err)
		}
		e.JobMode = schema.JobMode(jobMode)
		e.TerminalStatus = schema.SyncTerminalStatus(status)
		e.StartedAt, _ = parseTime(startedAt)
		e.CreatedAt, _ = parseTime(createdAt)
		if endedAt.Valid {
			t, _ := parseTime(endedAt.String)
			e.EndedAt = &t
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// LatestIntakeSyncLog returns the most recent pass for a connector, or ok=false
// if none exist. Used by the Console connector-detail "last pass" panel.
func LatestIntakeSyncLog(ctx context.Context, db *sql.DB, connectorID string) (schema.IntakeSyncLogEntry, bool, error) {
	list, err := ListIntakeSyncLog(ctx, db, connectorID, 1)
	if err != nil {
		return schema.IntakeSyncLogEntry{}, false, err
	}
	if len(list) == 0 {
		return schema.IntakeSyncLogEntry{}, false, nil
	}
	return list[0], true, nil
}
