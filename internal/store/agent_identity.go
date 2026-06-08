package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	AgentIdentityStatusActive     = "active"
	AgentIdentityStatusRevoked    = "revoked"
	AgentIdentityStatusSuperseded = "superseded"
)

// AgentIdentity stores public metadata for the local agent identity.
type AgentIdentity struct {
	ID               string
	KeyType          string
	PublicKey        string
	Fingerprint      string
	CreatedAt        time.Time
	Status           string
	RevokedAt        string
	RevocationReason string
	SupersedesID     string
	SupersededByID   string
	RotationLinkSig  string
}

// AgentIdentityUpdateOpts holds optional fields for updating an identity (revocation/rotation).
// Pointer fields are optional; when nil the column is not updated.
type AgentIdentityUpdateOpts struct {
	RevokedAt        *time.Time
	RevocationReason *string
	SupersedesID     *string
	SupersededByID   *string
	RotationLinkSig  *string
}

// GetActiveAgentIdentity returns the currently active identity metadata.
func GetActiveAgentIdentity(ctx context.Context, db *sql.DB) (AgentIdentity, bool, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, key_type, public_key, fingerprint, created_at, status
		FROM agent_identities
		WHERE status = ?
		LIMIT 1
	`, AgentIdentityStatusActive)
	return scanAgentIdentity(row)
}

// GetAgentIdentityByID returns identity metadata by id, including revocation/rotation fields if present.
func GetAgentIdentityByID(ctx context.Context, db *sql.DB, id string) (AgentIdentity, bool, error) {
	if strings.TrimSpace(id) == "" {
		return AgentIdentity{}, false, fmt.Errorf("store: agent identity id is required")
	}
	row := db.QueryRowContext(ctx, `
		SELECT id, key_type, public_key, fingerprint, created_at, status,
		       revoked_at, revocation_reason, supersedes_id, superseded_by_id, rotation_link_sig
		FROM agent_identities
		WHERE id = ?
	`, id)
	return scanAgentIdentityFull(row)
}

// UpdateAgentIdentity updates an identity's status and optional revocation/rotation metadata.
// When opts is nil or a field is nil, that column is not changed.
func UpdateAgentIdentity(ctx context.Context, db *sql.DB, id string, status string, opts *AgentIdentityUpdateOpts) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("store: agent identity id is required")
	}
	if strings.TrimSpace(status) == "" {
		return fmt.Errorf("store: agent identity status is required")
	}
	var revokedAt interface{}
	var revocationReason, supersedesID, supersededByID, rotationLinkSig interface{}
	if opts != nil {
		if opts.RevokedAt != nil {
			revokedAt = opts.RevokedAt.UTC().Format(timeFormat)
		}
		if opts.RevocationReason != nil {
			revocationReason = *opts.RevocationReason
		}
		if opts.SupersedesID != nil {
			supersedesID = *opts.SupersedesID
		}
		if opts.SupersededByID != nil {
			supersededByID = *opts.SupersededByID
		}
		if opts.RotationLinkSig != nil {
			rotationLinkSig = *opts.RotationLinkSig
		}
	}
	_, err := db.ExecContext(ctx, `
		UPDATE agent_identities
		SET status = ?,
		    revoked_at = COALESCE(?, revoked_at),
		    revocation_reason = COALESCE(?, revocation_reason),
		    supersedes_id = COALESCE(?, supersedes_id),
		    superseded_by_id = COALESCE(?, superseded_by_id),
		    rotation_link_sig = COALESCE(?, rotation_link_sig)
		WHERE id = ?
	`, status, revokedAt, revocationReason, supersedesID, supersededByID, rotationLinkSig, id)
	if err != nil {
		return fmt.Errorf("store: update agent identity: %w", err)
	}
	return nil
}

// CreateInitialAgentIdentity atomically stores public metadata.
// If an active identity already exists, it returns that identity with created=false.
func CreateInitialAgentIdentity(ctx context.Context, db *sql.DB, identity AgentIdentity) (AgentIdentity, bool, error) {
	if strings.TrimSpace(identity.KeyType) == "" {
		return AgentIdentity{}, false, fmt.Errorf("store: agent identity key_type is required")
	}
	if strings.TrimSpace(identity.PublicKey) == "" {
		return AgentIdentity{}, false, fmt.Errorf("store: agent identity public_key is required")
	}
	if strings.TrimSpace(identity.Fingerprint) == "" {
		return AgentIdentity{}, false, fmt.Errorf("store: agent identity fingerprint is required")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return AgentIdentity{}, false, fmt.Errorf("store: begin tx: %w", err)
	}
	defer tx.Rollback()

	existing, found, err := getActiveAgentIdentityTx(ctx, tx)
	if err != nil {
		return AgentIdentity{}, false, err
	}
	if found {
		if err := tx.Commit(); err != nil {
			return AgentIdentity{}, false, fmt.Errorf("store: commit tx: %w", err)
		}
		return existing, false, nil
	}

	if identity.ID == "" {
		identity.ID = newKeyID()
	}
	if identity.CreatedAt.IsZero() {
		identity.CreatedAt = time.Now().UTC()
	}
	if strings.TrimSpace(identity.Status) == "" {
		identity.Status = AgentIdentityStatusActive
	}
	if identity.Status != AgentIdentityStatusActive {
		return AgentIdentity{}, false, fmt.Errorf("store: initial identity status must be %q", AgentIdentityStatusActive)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO agent_identities (id, key_type, public_key, fingerprint, created_at, status)
		VALUES (?, ?, ?, ?, ?, ?)
	`, identity.ID, identity.KeyType, identity.PublicKey, identity.Fingerprint, identity.CreatedAt.UTC().Format(timeFormat), identity.Status)
	if err != nil {
		return AgentIdentity{}, false, fmt.Errorf("store: create agent identity: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return AgentIdentity{}, false, fmt.Errorf("store: commit tx: %w", err)
	}
	return identity, true, nil
}

// NewKeyID returns a new random key ID for use when creating agent identities (e.g. rotation).
func NewKeyID() string {
	b := make([]byte, 12)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// InsertAgentIdentity inserts a single identity row. Used for key rotation after demoting the previous active identity.
// Caller must ensure at most one active identity (e.g. by updating the old identity to superseded first).
func InsertAgentIdentity(ctx context.Context, db *sql.DB, identity AgentIdentity) error {
	if strings.TrimSpace(identity.KeyType) == "" {
		return fmt.Errorf("store: agent identity key_type is required")
	}
	if strings.TrimSpace(identity.PublicKey) == "" {
		return fmt.Errorf("store: agent identity public_key is required")
	}
	if strings.TrimSpace(identity.Fingerprint) == "" {
		return fmt.Errorf("store: agent identity fingerprint is required")
	}
	if strings.TrimSpace(identity.Status) == "" {
		identity.Status = AgentIdentityStatusActive
	}
	if identity.ID == "" {
		identity.ID = NewKeyID()
	}
	if identity.CreatedAt.IsZero() {
		identity.CreatedAt = time.Now().UTC()
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO agent_identities (id, key_type, public_key, fingerprint, created_at, status)
		VALUES (?, ?, ?, ?, ?, ?)
	`, identity.ID, identity.KeyType, identity.PublicKey, identity.Fingerprint, identity.CreatedAt.UTC().Format(timeFormat), identity.Status)
	if err != nil {
		return fmt.Errorf("store: insert agent identity: %w", err)
	}
	return nil
}

// ApplyRotation atomically marks the old identity as superseded and inserts the new active identity.
func ApplyRotation(ctx context.Context, db *sql.DB, oldID, linkSigB64, supersededByID string, newIdentity AgentIdentity) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin tx: %w", err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
		UPDATE agent_identities SET status = ?, superseded_by_id = ?, rotation_link_sig = ? WHERE id = ?
	`, AgentIdentityStatusSuperseded, supersededByID, linkSigB64, oldID)
	if err != nil {
		return fmt.Errorf("store: update old identity: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO agent_identities (id, key_type, public_key, fingerprint, created_at, status, supersedes_id)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, newIdentity.ID, newIdentity.KeyType, newIdentity.PublicKey, newIdentity.Fingerprint, newIdentity.CreatedAt.UTC().Format(timeFormat), newIdentity.Status, newIdentity.SupersedesID)
	if err != nil {
		return fmt.Errorf("store: insert new identity: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit tx: %w", err)
	}
	return nil
}

func getActiveAgentIdentityTx(ctx context.Context, tx *sql.Tx) (AgentIdentity, bool, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT id, key_type, public_key, fingerprint, created_at, status
		FROM agent_identities
		WHERE status = ?
		LIMIT 1
	`, AgentIdentityStatusActive)
	return scanAgentIdentity(row)
}

func scanAgentIdentity(row *sql.Row) (AgentIdentity, bool, error) {
	var (
		identity   AgentIdentity
		createdRaw string
	)
	if err := row.Scan(&identity.ID, &identity.KeyType, &identity.PublicKey, &identity.Fingerprint, &createdRaw, &identity.Status); err != nil {
		if err == sql.ErrNoRows {
			return AgentIdentity{}, false, nil
		}
		return AgentIdentity{}, false, fmt.Errorf("store: scan agent identity: %w", err)
	}
	createdAt, err := parseTime(createdRaw)
	if err != nil {
		return AgentIdentity{}, false, fmt.Errorf("store: parse agent identity created_at: %w", err)
	}
	identity.CreatedAt = createdAt
	return identity, true, nil
}

func scanAgentIdentityFull(row *sql.Row) (AgentIdentity, bool, error) {
	var (
		identity        AgentIdentity
		createdRaw      string
		revokedAt       sql.NullString
		revReason       sql.NullString
		supersedes      sql.NullString
		supersededBy    sql.NullString
		rotationLinkSig sql.NullString
	)
	err := row.Scan(&identity.ID, &identity.KeyType, &identity.PublicKey, &identity.Fingerprint, &createdRaw, &identity.Status,
		&revokedAt, &revReason, &supersedes, &supersededBy, &rotationLinkSig)
	if err != nil {
		if err == sql.ErrNoRows {
			return AgentIdentity{}, false, nil
		}
		return AgentIdentity{}, false, fmt.Errorf("store: scan agent identity: %w", err)
	}
	createdAt, err := parseTime(createdRaw)
	if err != nil {
		return AgentIdentity{}, false, fmt.Errorf("store: parse agent identity created_at: %w", err)
	}
	identity.CreatedAt = createdAt
	if revokedAt.Valid {
		identity.RevokedAt = revokedAt.String
	}
	if revReason.Valid {
		identity.RevocationReason = revReason.String
	}
	if supersedes.Valid {
		identity.SupersedesID = supersedes.String
	}
	if supersededBy.Valid {
		identity.SupersededByID = supersededBy.String
	}
	if rotationLinkSig.Valid {
		identity.RotationLinkSig = rotationLinkSig.String
	}
	return identity, true, nil
}
