package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// NAVI-VAULT-V1 — vault_state store (raw SQL, no ORM).
//
// vault_state maps each Vault file on disk to the World Model entity it projects
// and records the hash of the last projector-owned content written to that file.
// It is projector bookkeeping, not authority: entities are truth; this table is
// a rebuildable index used to (a) resolve an edited file back to its entity, and
// (b) detect drift (file content diverging from canonical) during the periodic
// sweep. The Vault never writes the World Model from this table.

// VaultState is one file ↔ entity mapping row.
type VaultState struct {
	FilePath      string
	EntityType    string
	EntityID      string
	ProjectedHash string // hash of the last projector-owned bytes written to FilePath
	ProjectedAt   time.Time
	CreatedAt     time.Time
}

// UpsertVaultState records (or updates) the file ↔ entity mapping and the hash of
// the projector-owned content last written to the file.
func UpsertVaultState(ctx context.Context, db *sql.DB, s VaultState) error {
	if s.FilePath == "" {
		return fmt.Errorf("store: vault_state file_path required")
	}
	if s.EntityType == "" || s.EntityID == "" {
		return fmt.Errorf("store: vault_state entity_type and entity_id required")
	}
	now := time.Now().UTC()
	if s.ProjectedAt.IsZero() {
		s.ProjectedAt = now
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO vault_state (file_path, entity_type, entity_id, projected_hash, projected_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(file_path) DO UPDATE SET
			entity_type=excluded.entity_type,
			entity_id=excluded.entity_id,
			projected_hash=excluded.projected_hash,
			projected_at=excluded.projected_at
	`, s.FilePath, s.EntityType, s.EntityID, s.ProjectedHash,
		s.ProjectedAt.UTC().Format(timeFormat), s.CreatedAt.UTC().Format(timeFormat))
	if err != nil {
		return fmt.Errorf("store: upsert vault_state: %w", err)
	}
	return nil
}

// GetVaultState returns the mapping for a file path, ok=false when absent.
func GetVaultState(ctx context.Context, db *sql.DB, filePath string) (VaultState, bool, error) {
	row := db.QueryRowContext(ctx, `
		SELECT file_path, entity_type, entity_id, projected_hash, projected_at, created_at
		FROM vault_state WHERE file_path = ?
	`, filePath)
	var (
		s                      VaultState
		projectedAt, createdAt string
	)
	if err := row.Scan(&s.FilePath, &s.EntityType, &s.EntityID, &s.ProjectedHash, &projectedAt, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return VaultState{}, false, nil
		}
		return VaultState{}, false, fmt.Errorf("store: get vault_state: %w", err)
	}
	s.ProjectedAt, _ = parseTime(projectedAt)
	s.CreatedAt, _ = parseTime(createdAt)
	return s, true, nil
}

// GetVaultStateByEntity returns the file mapping for a given entity, ok=false
// when the entity has no projected file yet.
func GetVaultStateByEntity(ctx context.Context, db *sql.DB, entityType, entityID string) (VaultState, bool, error) {
	row := db.QueryRowContext(ctx, `
		SELECT file_path, entity_type, entity_id, projected_hash, projected_at, created_at
		FROM vault_state WHERE entity_type = ? AND entity_id = ?
		ORDER BY created_at ASC LIMIT 1
	`, entityType, entityID)
	var (
		s                      VaultState
		projectedAt, createdAt string
	)
	if err := row.Scan(&s.FilePath, &s.EntityType, &s.EntityID, &s.ProjectedHash, &projectedAt, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return VaultState{}, false, nil
		}
		return VaultState{}, false, fmt.Errorf("store: get vault_state by entity: %w", err)
	}
	s.ProjectedAt, _ = parseTime(projectedAt)
	s.CreatedAt, _ = parseTime(createdAt)
	return s, true, nil
}

// ListVaultState returns all file ↔ entity mappings, oldest first.
func ListVaultState(ctx context.Context, db *sql.DB) ([]VaultState, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT file_path, entity_type, entity_id, projected_hash, projected_at, created_at
		FROM vault_state ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list vault_state: %w", err)
	}
	defer rows.Close()
	var out []VaultState
	for rows.Next() {
		var (
			s                      VaultState
			projectedAt, createdAt string
		)
		if err := rows.Scan(&s.FilePath, &s.EntityType, &s.EntityID, &s.ProjectedHash, &projectedAt, &createdAt); err != nil {
			return nil, fmt.Errorf("store: scan vault_state: %w", err)
		}
		s.ProjectedAt, _ = parseTime(projectedAt)
		s.CreatedAt, _ = parseTime(createdAt)
		out = append(out, s)
	}
	return out, rows.Err()
}

// DeleteVaultState removes the mapping for a file path. Used when a file is
// deleted by the owner (after the corresponding Forget Proposal is raised) so the
// drift sweep does not treat the now-absent file as missing-but-expected.
func DeleteVaultState(ctx context.Context, db *sql.DB, filePath string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM vault_state WHERE file_path = ?`, filePath)
	if err != nil {
		return fmt.Errorf("store: delete vault_state: %w", err)
	}
	return nil
}
