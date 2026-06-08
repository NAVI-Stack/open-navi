package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ceoai/navi/internal/schema"
)

// SaveIntakeChunk inserts c into intake_chunks. The insert is idempotent: a
// chunk whose stable id (or whose (intake_record_id, chunk_index) pair) already
// exists is silently ignored, so re-processing the same IntakeRecord never
// produces duplicate chunks (CIP P2 resumability requirement).
//
// Returns (true, nil) when a new row was written, (false, nil) when the chunk
// already existed.
func SaveIntakeChunk(ctx context.Context, db *sql.DB, c schema.IntakeChunk) (bool, error) {
	if c.ID == "" {
		return false, fmt.Errorf("store: save intake chunk: missing ID")
	}
	if c.IntakeRecordID == "" {
		return false, fmt.Errorf("store: save intake chunk: missing IntakeRecordID")
	}
	if c.ContentMIME == "" {
		c.ContentMIME = "text/markdown"
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	provJSON, err := json.Marshal(c.Provenance)
	if err != nil {
		return false, fmt.Errorf("store: save intake chunk: marshal provenance: %w", err)
	}
	res, err := db.ExecContext(ctx, `
		INSERT OR IGNORE INTO intake_chunks
			(id, intake_record_id, connector_id, source_id, chunk_index,
			 parent_chunk_id, content, content_mime, start_offset, end_offset,
			 token_estimate, trust, privacy_class, provenance, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.IntakeRecordID, c.ConnectorID, c.SourceID, c.ChunkIndex,
		c.ParentChunkID, c.Content, c.ContentMIME, c.StartOffset, c.EndOffset,
		c.TokenEstimate, string(c.Trust), string(c.PrivacyClass), string(provJSON),
		c.CreatedAt.Format(timeFormat),
	)
	if err != nil {
		return false, fmt.Errorf("store: save intake chunk: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// GetIntakeChunk fetches a single chunk by primary key.
func GetIntakeChunk(ctx context.Context, db *sql.DB, id string) (schema.IntakeChunk, error) {
	row := db.QueryRowContext(ctx, intakeChunkSelect+` WHERE id = ?`, id)
	return scanIntakeChunk(row.Scan)
}

// ListIntakeChunksByRecord returns all chunks for a record ordered by chunk_index.
func ListIntakeChunksByRecord(ctx context.Context, db *sql.DB, recordID string) ([]schema.IntakeChunk, error) {
	rows, err := db.QueryContext(ctx, intakeChunkSelect+` WHERE intake_record_id = ? ORDER BY chunk_index ASC`, recordID)
	if err != nil {
		return nil, fmt.Errorf("store: list intake chunks: %w", err)
	}
	defer rows.Close()
	var out []schema.IntakeChunk
	for rows.Next() {
		c, err := scanIntakeChunk(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list intake chunks: iterate: %w", err)
	}
	return out, nil
}

// ListAllIntakeChunks returns the retrieval candidate pool: every persisted
// chunk, newest first, bounded by limit (0 defaults to 5000 — personal-assistant
// scale). This is the source set for P4 hybrid retrieval; un-promoted chunks
// (chunks that never synthesized into entities) are included, since synthesis is
// promotion, not gating (synthesis seam §14).
func ListAllIntakeChunks(ctx context.Context, db *sql.DB, limit int) ([]schema.IntakeChunk, error) {
	if limit <= 0 {
		limit = 5000
	}
	rows, err := db.QueryContext(ctx, intakeChunkSelect+` ORDER BY created_at DESC, id ASC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list all intake chunks: %w", err)
	}
	defer rows.Close()
	var out []schema.IntakeChunk
	for rows.Next() {
		c, err := scanIntakeChunk(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list all intake chunks: iterate: %w", err)
	}
	return out, nil
}

// CountIntakeChunksByRecord returns how many chunks already exist for a record.
// Used by the worker to decide whether downstream processing is already done.
func CountIntakeChunksByRecord(ctx context.Context, db *sql.DB, recordID string) (int, error) {
	var n int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM intake_chunks WHERE intake_record_id = ?`, recordID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: count intake chunks: %w", err)
	}
	return n, nil
}

const intakeChunkSelect = `
	SELECT id, intake_record_id, connector_id, source_id, chunk_index,
	       parent_chunk_id, content, content_mime, start_offset, end_offset,
	       token_estimate, trust, privacy_class, provenance, created_at
	FROM intake_chunks`

func scanIntakeChunk(scan func(dest ...any) error) (schema.IntakeChunk, error) {
	var c schema.IntakeChunk
	var trust, privClass, provJSON, createdAt string
	if err := scan(
		&c.ID, &c.IntakeRecordID, &c.ConnectorID, &c.SourceID, &c.ChunkIndex,
		&c.ParentChunkID, &c.Content, &c.ContentMIME, &c.StartOffset, &c.EndOffset,
		&c.TokenEstimate, &trust, &privClass, &provJSON, &createdAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return schema.IntakeChunk{}, fmt.Errorf("store: intake chunk not found")
		}
		return schema.IntakeChunk{}, fmt.Errorf("store: scan intake chunk: %w", err)
	}
	c.Trust = schema.ContentTrust(trust)
	c.PrivacyClass = schema.PrivacyClass(privClass)
	c.CreatedAt, _ = parseTime(createdAt)
	_ = json.Unmarshal([]byte(provJSON), &c.Provenance)
	return c, nil
}
