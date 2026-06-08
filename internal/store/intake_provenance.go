package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// IntakeProvenanceLink records that intake synthesis (CIP stage 8) produced or
// touched a World Model entity from a specific source record / chunk / derivation.
// It is the provenance spine the spec requires (CIP §15 #2): every synthesized
// entity links back to its IntakeRecord, fetch time, and cursor, with a blended
// confidence. The unique DerivationKey is also the idempotency anchor — a
// replayed synthesis of the same (record, chunk, candidate, kind) is a no-op.
type IntakeProvenanceLink struct {
	ID             string
	IntakeRecordID string
	ChunkID        string
	ConnectorID    string
	SourceID       string
	Cursor         string
	FetchedAt      time.Time
	EntityType     string
	EntityID       string // "" when the synthesis raised a Proposal rather than writing
	MutationKind   string
	Outcome        string // approved | modified | proposal | rejected
	DerivationKey  string // stable hash of (record, chunk, candidate, kind) — UNIQUE
	Confidence     float64
	ProposalID     string
	CreatedAt      time.Time
}

// SaveIntakeProvenanceLink inserts a provenance link. It is idempotent on
// DerivationKey: a second insert for the same derivation is ignored and reports
// inserted=false, which is how the synthesizer detects a replay.
func SaveIntakeProvenanceLink(ctx context.Context, db *sql.DB, l IntakeProvenanceLink) (bool, error) {
	if l.ID == "" {
		return false, fmt.Errorf("store: intake provenance id required")
	}
	if l.DerivationKey == "" {
		return false, fmt.Errorf("store: intake provenance derivation_key required")
	}
	if l.CreatedAt.IsZero() {
		l.CreatedAt = time.Now().UTC()
	}
	res, err := db.ExecContext(ctx, `
		INSERT OR IGNORE INTO intake_provenance (
			id, intake_record_id, chunk_id, connector_id, source_id, cursor, fetched_at,
			entity_type, entity_id, mutation_kind, outcome, derivation_key, confidence, proposal_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, l.ID, l.IntakeRecordID, l.ChunkID, l.ConnectorID, l.SourceID, l.Cursor,
		l.FetchedAt.UTC().Format(timeFormat), l.EntityType, l.EntityID, l.MutationKind,
		l.Outcome, l.DerivationKey, l.Confidence, l.ProposalID, l.CreatedAt.Format(timeFormat))
	if err != nil {
		return false, fmt.Errorf("store: save intake provenance: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: intake provenance rows affected: %w", err)
	}
	return n > 0, nil
}

// IntakeProvenanceExists reports whether a derivation has already been synthesized.
func IntakeProvenanceExists(ctx context.Context, db *sql.DB, derivationKey string) (bool, error) {
	var one int
	err := db.QueryRowContext(ctx,
		`SELECT 1 FROM intake_provenance WHERE derivation_key = ? LIMIT 1`, derivationKey).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: intake provenance exists: %w", err)
	}
	return true, nil
}

func scanIntakeProvenance(rows *sql.Rows) ([]IntakeProvenanceLink, error) {
	var out []IntakeProvenanceLink
	for rows.Next() {
		var (
			l         IntakeProvenanceLink
			fetchedAt string
			createdAt string
		)
		if err := rows.Scan(&l.ID, &l.IntakeRecordID, &l.ChunkID, &l.ConnectorID, &l.SourceID,
			&l.Cursor, &fetchedAt, &l.EntityType, &l.EntityID, &l.MutationKind, &l.Outcome,
			&l.DerivationKey, &l.Confidence, &l.ProposalID, &createdAt); err != nil {
			return nil, fmt.Errorf("store: scan intake provenance: %w", err)
		}
		l.FetchedAt, _ = parseTime(fetchedAt)
		l.CreatedAt, _ = parseTime(createdAt)
		out = append(out, l)
	}
	return out, rows.Err()
}

const intakeProvenanceColumns = `id, intake_record_id, chunk_id, connector_id, source_id, cursor, fetched_at,
	entity_type, entity_id, mutation_kind, outcome, derivation_key, confidence, proposal_id, created_at`

// ListIntakeProvenanceByRecord returns all provenance links for a source record.
func ListIntakeProvenanceByRecord(ctx context.Context, db *sql.DB, recordID string) ([]IntakeProvenanceLink, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT `+intakeProvenanceColumns+` FROM intake_provenance WHERE intake_record_id = ? ORDER BY created_at ASC`, recordID)
	if err != nil {
		return nil, fmt.Errorf("store: list intake provenance by record: %w", err)
	}
	defer rows.Close()
	return scanIntakeProvenance(rows)
}

// ListIntakeProvenanceByChunk returns the synthesis links derived from a chunk.
// P4 retrieval uses this to attach the synthesized entity link(s) and blended
// confidence to a retrieved chunk without re-running synthesis.
func ListIntakeProvenanceByChunk(ctx context.Context, db *sql.DB, chunkID string) ([]IntakeProvenanceLink, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT `+intakeProvenanceColumns+` FROM intake_provenance WHERE chunk_id = ? ORDER BY created_at ASC`, chunkID)
	if err != nil {
		return nil, fmt.Errorf("store: list intake provenance by chunk: %w", err)
	}
	defer rows.Close()
	return scanIntakeProvenance(rows)
}

// ListIntakeProvenanceByEntity returns the source links that produced an entity.
func ListIntakeProvenanceByEntity(ctx context.Context, db *sql.DB, entityType, entityID string) ([]IntakeProvenanceLink, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT `+intakeProvenanceColumns+` FROM intake_provenance WHERE entity_type = ? AND entity_id = ? ORDER BY created_at ASC`,
		entityType, entityID)
	if err != nil {
		return nil, fmt.Errorf("store: list intake provenance by entity: %w", err)
	}
	defer rows.Close()
	return scanIntakeProvenance(rows)
}
