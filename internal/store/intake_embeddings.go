package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// IntakeEmbedding is the persisted embedding reference for an intake chunk
// (CIP stage 7, §13). P3 persists the vector and producing model as an opaque
// reference alongside intake_chunks; the vector index engine is deferred, so this
// row is a reference, not an index entry.
type IntakeEmbedding struct {
	ChunkID   string
	Model     string
	Dim       int
	Vector    []float64
	CreatedAt time.Time
}

// SaveIntakeEmbedding upserts an embedding reference keyed by chunk id. Re-embedding
// the same chunk overwrites in place, keeping the operation idempotent.
func SaveIntakeEmbedding(ctx context.Context, db *sql.DB, e IntakeEmbedding) error {
	if e.ChunkID == "" {
		return fmt.Errorf("store: intake embedding chunk_id required")
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	vec, err := json.Marshal(e.Vector)
	if err != nil {
		return fmt.Errorf("store: marshal embedding vector: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO intake_embeddings (chunk_id, model, dim, vector, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(chunk_id) DO UPDATE SET
			model=excluded.model,
			dim=excluded.dim,
			vector=excluded.vector,
			created_at=excluded.created_at
	`, e.ChunkID, e.Model, e.Dim, string(vec), e.CreatedAt.Format(timeFormat))
	if err != nil {
		return fmt.Errorf("store: save intake embedding: %w", err)
	}
	return nil
}

// GetIntakeEmbedding returns the embedding reference for a chunk, if any.
func GetIntakeEmbedding(ctx context.Context, db *sql.DB, chunkID string) (IntakeEmbedding, bool, error) {
	var (
		e         IntakeEmbedding
		vec       string
		createdAt string
	)
	err := db.QueryRowContext(ctx, `
		SELECT chunk_id, model, dim, vector, created_at
		FROM intake_embeddings WHERE chunk_id = ?
	`, chunkID).Scan(&e.ChunkID, &e.Model, &e.Dim, &vec, &createdAt)
	if err == sql.ErrNoRows {
		return IntakeEmbedding{}, false, nil
	}
	if err != nil {
		return IntakeEmbedding{}, false, fmt.Errorf("store: get intake embedding: %w", err)
	}
	if vec != "" {
		_ = json.Unmarshal([]byte(vec), &e.Vector)
	}
	e.CreatedAt, _ = parseTime(createdAt)
	return e, true, nil
}

// ListAllIntakeEmbeddings returns every persisted embedding reference. P4's
// vector index engine (the pure-Go brute-force cosine index in
// internal/intake/embed) is built directly from this table — the persisted
// reference IS the index source, so no separate index migration is required.
func ListAllIntakeEmbeddings(ctx context.Context, db *sql.DB) ([]IntakeEmbedding, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT chunk_id, model, dim, vector, created_at FROM intake_embeddings
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list all intake embeddings: %w", err)
	}
	defer rows.Close()
	var out []IntakeEmbedding
	for rows.Next() {
		var (
			e         IntakeEmbedding
			vec       string
			createdAt string
		)
		if err := rows.Scan(&e.ChunkID, &e.Model, &e.Dim, &vec, &createdAt); err != nil {
			return nil, fmt.Errorf("store: scan intake embedding: %w", err)
		}
		if vec != "" {
			_ = json.Unmarshal([]byte(vec), &e.Vector)
		}
		e.CreatedAt, _ = parseTime(createdAt)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list all intake embeddings: iterate: %w", err)
	}
	return out, nil
}

// CountIntakeEmbeddings returns the number of persisted embedding references.
func CountIntakeEmbeddings(ctx context.Context, db *sql.DB) (int, error) {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(1) FROM intake_embeddings`).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count intake embeddings: %w", err)
	}
	return n, nil
}
