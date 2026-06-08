package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ceoai/navi/internal/schema"
	"github.com/google/uuid"
)

// CIP P5 — vault_sync_log store (raw SQL, no ORM). Append-only record of every
// Vault diff pass, per file (Vault §13). P5 ships the SCHEMA and the store; the
// Memory Vault deliverable is the populator. The Console renders these rows in
// the per-file Vault sync log. Until the Vault lands, the table is simply empty.

// AppendVaultSyncLog inserts one Vault diff-pass row and returns its id.
// Append-only. Provided so the Vault deliverable has a ready store seam; P5 does
// not call it in production (no Vault writer yet) but tests exercise it.
func AppendVaultSyncLog(ctx context.Context, db *sql.DB, e schema.VaultSyncLogEntry) (string, error) {
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
		INSERT INTO vault_sync_log (
			id, file_path, started_at, ended_at, diff_summary,
			mutations_proposed, mutations_approved, proposals_raised,
			errors, terminal_status, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, e.ID, e.FilePath, e.StartedAt.UTC().Format(timeFormat), endedAt, e.DiffSummary,
		e.MutationsProposed, e.MutationsApproved, e.ProposalsRaised,
		e.Errors, string(e.TerminalStatus), e.CreatedAt.UTC().Format(timeFormat))
	if err != nil {
		return "", fmt.Errorf("store: append vault sync log: %w", err)
	}
	return e.ID, nil
}

// ListVaultSyncLog returns Vault sync-log rows, newest first. path == "" returns
// all files. limit <= 0 defaults to 50.
func ListVaultSyncLog(ctx context.Context, db *sql.DB, path string, limit int) ([]schema.VaultSyncLogEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	const cols = `id, file_path, started_at, ended_at, diff_summary,
		mutations_proposed, mutations_approved, proposals_raised,
		errors, terminal_status, created_at`
	var (
		rows *sql.Rows
		err  error
	)
	if path == "" {
		rows, err = db.QueryContext(ctx, `SELECT `+cols+`
			FROM vault_sync_log ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
	} else {
		rows, err = db.QueryContext(ctx, `SELECT `+cols+`
			FROM vault_sync_log WHERE file_path = ?
			ORDER BY created_at DESC, id DESC LIMIT ?`, path, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("store: list vault sync log: %w", err)
	}
	defer rows.Close()

	var out []schema.VaultSyncLogEntry
	for rows.Next() {
		var (
			e         schema.VaultSyncLogEntry
			startedAt string
			endedAt   sql.NullString
			status    string
			createdAt string
		)
		if err := rows.Scan(&e.ID, &e.FilePath, &startedAt, &endedAt, &e.DiffSummary,
			&e.MutationsProposed, &e.MutationsApproved, &e.ProposalsRaised,
			&e.Errors, &status, &createdAt); err != nil {
			return nil, fmt.Errorf("store: scan vault sync log: %w", err)
		}
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
