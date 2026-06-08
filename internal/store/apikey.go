package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// APIKey stores a hashed API credential and associated metadata.
type APIKey struct {
	ID         string
	OwnerID    string
	KeyHash    string
	Name       string
	Scopes     []string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
}

// CreateAPIKey persists a new API key record. KeyHash must be a hash, never the raw key.
func CreateAPIKey(ctx context.Context, db *sql.DB, key APIKey) error {
	if key.ID == "" {
		return fmt.Errorf("store: api key id is required")
	}
	if key.OwnerID == "" {
		return fmt.Errorf("store: api key owner_id is required")
	}
	if key.KeyHash == "" {
		return fmt.Errorf("store: api key hash is required")
	}
	if key.CreatedAt.IsZero() {
		key.CreatedAt = time.Now().UTC()
	}

	scopesJSON, err := json.Marshal(key.Scopes)
	if err != nil {
		return fmt.Errorf("store: marshal api key scopes: %w", err)
	}

	var lastUsedAt any
	if key.LastUsedAt != nil {
		lastUsedAt = key.LastUsedAt.UTC().Format(timeFormat)
	}
	var revokedAt any
	if key.RevokedAt != nil {
		revokedAt = key.RevokedAt.UTC().Format(timeFormat)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO api_keys (id, owner_id, key_hash, name, scopes, created_at, last_used_at, revoked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, key.ID, key.OwnerID, key.KeyHash, key.Name, string(scopesJSON), key.CreatedAt.UTC().Format(timeFormat), lastUsedAt, revokedAt)
	if err != nil {
		return fmt.Errorf("store: create api key: %w", err)
	}
	return nil
}

// GetActiveAPIKeyByHash returns a non-revoked API key by hash.
func GetActiveAPIKeyByHash(ctx context.Context, db *sql.DB, keyHash string) (APIKey, bool, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, owner_id, key_hash, name, scopes, created_at, last_used_at, revoked_at
		FROM api_keys
		WHERE key_hash = ? AND revoked_at IS NULL
	`, keyHash)

	key, found, err := scanAPIKey(row)
	if err != nil {
		return APIKey{}, false, fmt.Errorf("store: get api key by hash: %w", err)
	}
	return key, found, nil
}

// ListActiveAPIKeys returns all non-revoked API keys newest-first.
func ListActiveAPIKeys(ctx context.Context, db *sql.DB) ([]APIKey, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, owner_id, key_hash, name, scopes, created_at, last_used_at, revoked_at
		FROM api_keys
		WHERE revoked_at IS NULL
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list api keys: %w", err)
	}
	defer rows.Close()

	var out []APIKey
	for rows.Next() {
		var (
			key           APIKey
			name          sql.NullString
			scopesJSON    string
			createdAtRaw  string
			lastUsedAtRaw sql.NullString
			revokedAtRaw  sql.NullString
		)

		if err := rows.Scan(
			&key.ID,
			&key.OwnerID,
			&key.KeyHash,
			&name,
			&scopesJSON,
			&createdAtRaw,
			&lastUsedAtRaw,
			&revokedAtRaw,
		); err != nil {
			return nil, fmt.Errorf("store: scan api key: %w", err)
		}
		if name.Valid {
			key.Name = name.String
		}
		if err := json.Unmarshal([]byte(scopesJSON), &key.Scopes); err != nil {
			return nil, fmt.Errorf("store: unmarshal api key scopes: %w", err)
		}

		createdAt, err := parseTime(createdAtRaw)
		if err != nil {
			return nil, fmt.Errorf("store: parse api key created_at: %w", err)
		}
		key.CreatedAt = createdAt

		if lastUsedAtRaw.Valid {
			lastUsedAt, err := parseTime(lastUsedAtRaw.String)
			if err != nil {
				return nil, fmt.Errorf("store: parse api key last_used_at: %w", err)
			}
			key.LastUsedAt = &lastUsedAt
		}
		if revokedAtRaw.Valid {
			revokedAt, err := parseTime(revokedAtRaw.String)
			if err != nil {
				return nil, fmt.Errorf("store: parse api key revoked_at: %w", err)
			}
			key.RevokedAt = &revokedAt
		}

		out = append(out, key)
	}
	return out, rows.Err()
}

// RevokeAPIKey marks an API key revoked. This operation is idempotent.
func RevokeAPIKey(ctx context.Context, db *sql.DB, id string, revokedAt time.Time) error {
	if id == "" {
		return fmt.Errorf("store: api key id is required")
	}
	if revokedAt.IsZero() {
		revokedAt = time.Now().UTC()
	}
	_, err := db.ExecContext(ctx, `
		UPDATE api_keys
		SET revoked_at = ?
		WHERE id = ? AND revoked_at IS NULL
	`, revokedAt.UTC().Format(timeFormat), id)
	if err != nil {
		return fmt.Errorf("store: revoke api key: %w", err)
	}
	return nil
}

// TouchAPIKeyLastUsed updates last_used_at for a key. Caller decides whether failures are fatal.
func TouchAPIKeyLastUsed(ctx context.Context, db *sql.DB, id string, usedAt time.Time) error {
	if id == "" {
		return fmt.Errorf("store: api key id is required")
	}
	if usedAt.IsZero() {
		usedAt = time.Now().UTC()
	}
	_, err := db.ExecContext(ctx, `
		UPDATE api_keys
		SET last_used_at = ?
		WHERE id = ? AND revoked_at IS NULL
	`, usedAt.UTC().Format(timeFormat), id)
	if err != nil {
		return fmt.Errorf("store: touch api key: %w", err)
	}
	return nil
}

func scanAPIKey(row *sql.Row) (APIKey, bool, error) {
	var (
		key          APIKey
		name         sql.NullString
		scopesJSON   string
		createdAtRaw string
		lastUsedRaw  sql.NullString
		revokedRaw   sql.NullString
	)

	if err := row.Scan(
		&key.ID,
		&key.OwnerID,
		&key.KeyHash,
		&name,
		&scopesJSON,
		&createdAtRaw,
		&lastUsedRaw,
		&revokedRaw,
	); err != nil {
		if err == sql.ErrNoRows {
			return APIKey{}, false, nil
		}
		return APIKey{}, false, err
	}

	if name.Valid {
		key.Name = name.String
	}
	if err := json.Unmarshal([]byte(scopesJSON), &key.Scopes); err != nil {
		return APIKey{}, false, fmt.Errorf("unmarshal api key scopes: %w", err)
	}

	createdAt, err := parseTime(createdAtRaw)
	if err != nil {
		return APIKey{}, false, fmt.Errorf("parse api key created_at: %w", err)
	}
	key.CreatedAt = createdAt

	if lastUsedRaw.Valid {
		lastUsedAt, err := parseTime(lastUsedRaw.String)
		if err != nil {
			return APIKey{}, false, fmt.Errorf("parse api key last_used_at: %w", err)
		}
		key.LastUsedAt = &lastUsedAt
	}
	if revokedRaw.Valid {
		revokedAt, err := parseTime(revokedRaw.String)
		if err != nil {
			return APIKey{}, false, fmt.Errorf("parse api key revoked_at: %w", err)
		}
		key.RevokedAt = &revokedAt
	}

	return key, true, nil
}

// --- Key generation and lookup helpers (used by claim flow and middleware) ---

// GenerateAPIKey creates a cryptographically random "navi_..." key.
// Returns the raw key (shown once to caller) and its SHA-256 hex hash for storage.
func GenerateAPIKey() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", fmt.Errorf("store: generate api key: %w", err)
	}
	raw = "navi_" + base64.RawURLEncoding.EncodeToString(b)
	hash = hashKey(raw)
	return raw, hash, nil
}

// HashAPIKey returns the SHA-256 hex hash of any string.
// Used by middleware to hash the incoming header value before DB lookup.
func HashAPIKey(raw string) string { return hashKey(raw) }

func hashKey(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// CreateAPIKeySimple is a convenience wrapper used by the claim flow.
// scopes is a []string; id is auto-generated if empty.
func CreateAPIKeySimple(ctx context.Context, db *sql.DB, ownerID, keyHash, name string, scopes []string) (APIKey, error) {
	id := newKeyID()
	now := time.Now().UTC()
	key := APIKey{
		ID:        id,
		OwnerID:   ownerID,
		KeyHash:   keyHash,
		Name:      name,
		Scopes:    scopes,
		CreatedAt: now,
	}
	if err := CreateAPIKey(ctx, db, key); err != nil {
		return APIKey{}, err
	}
	return key, nil
}

// LookupAPIKeyByRaw finds an active key by its raw (unhashed) value.
func LookupAPIKeyByRaw(ctx context.Context, db *sql.DB, raw string) (APIKey, bool, error) {
	return GetActiveAPIKeyByHash(ctx, db, hashKey(raw))
}

// ListAPIKeysByOwner returns active keys scoped to a specific owner.
func ListAPIKeysByOwner(ctx context.Context, db *sql.DB, ownerID string) ([]APIKey, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, owner_id, key_hash, name, scopes, created_at, last_used_at, revoked_at
		FROM api_keys
		WHERE owner_id = ? AND revoked_at IS NULL
		ORDER BY created_at DESC
	`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("store: list api keys by owner: %w", err)
	}
	defer rows.Close()

	var out []APIKey
	for rows.Next() {
		var (
			key           APIKey
			name          sql.NullString
			scopesJSON    string
			createdAtRaw  string
			lastUsedAtRaw sql.NullString
			revokedAtRaw  sql.NullString
		)
		if err := rows.Scan(&key.ID, &key.OwnerID, &key.KeyHash, &name, &scopesJSON,
			&createdAtRaw, &lastUsedAtRaw, &revokedAtRaw); err != nil {
			return nil, err
		}
		if name.Valid {
			key.Name = name.String
		}
		if err := json.Unmarshal([]byte(scopesJSON), &key.Scopes); err != nil {
			return nil, fmt.Errorf("store: unmarshal scopes: %w", err)
		}
		key.CreatedAt, _ = parseTime(createdAtRaw)
		if lastUsedAtRaw.Valid {
			t, _ := parseTime(lastUsedAtRaw.String)
			key.LastUsedAt = &t
		}
		out = append(out, key)
	}
	return out, rows.Err()
}

// RevokeAPIKeyByID soft-deletes a key owned by ownerID.
func RevokeAPIKeyByID(ctx context.Context, db *sql.DB, id, ownerID string) error {
	now := time.Now().UTC()
	res, err := db.ExecContext(ctx, `
		UPDATE api_keys SET revoked_at = ?
		WHERE id = ? AND owner_id = ? AND revoked_at IS NULL
	`, now.UTC().Format(timeFormat), id, ownerID)
	if err != nil {
		return fmt.Errorf("store: revoke api key: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("store: api key not found or already revoked")
	}
	return nil
}

func newKeyID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("store: crypto/rand unavailable: %v", err))
	}
	return hex.EncodeToString(b)
}

// --- Owner secret (onboarding) ---

// SetOwnerSecret hashes and stores the owner secret. Returns error if already set.
func SetOwnerSecret(ctx context.Context, db *sql.DB, secret string) error {
	_, already, err := GetSetting(ctx, db, "owner_secret_hash")
	if err != nil {
		return err
	}
	if already {
		return fmt.Errorf("store: owner secret already set")
	}
	return SetSetting(ctx, db, "owner_secret_hash", hashKey(secret))
}

// IsOwnerSecretSet returns true if the owner secret has been set.
func IsOwnerSecretSet(ctx context.Context, db *sql.DB) (bool, error) {
	_, found, err := GetSetting(ctx, db, "owner_secret_hash")
	return found, err
}

// VerifyOwnerSecret constant-time compares the provided secret against the stored hash.
func VerifyOwnerSecret(ctx context.Context, db *sql.DB, provided string) (bool, error) {
	stored, found, err := GetSetting(ctx, db, "owner_secret_hash")
	if err != nil || !found {
		return false, err
	}
	return subtle.ConstantTimeCompare([]byte(stored), []byte(hashKey(provided))) == 1, nil
}

// OwnerSecretFingerprint returns the first 8 hex chars of the owner secret hash —
// safe to display as a visual identifier without exposing the secret.
func OwnerSecretFingerprint(secret string) string {
	h := hashKey(secret)
	if len(h) >= 8 {
		return h[:8]
	}
	return h
}
