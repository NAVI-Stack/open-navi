package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"
)

func TestAPIKeyCRUD(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()

	raw := "navi_sk_test_value_123"
	sum := sha256.Sum256([]byte(raw))
	hash := hex.EncodeToString(sum[:])

	now := time.Now().UTC()
	err := CreateAPIKey(ctx, db, APIKey{
		ID:        "key-1",
		OwnerID:   "owner-a",
		KeyHash:   hash,
		Name:      "ci key",
		Scopes:    []string{"read", "execute"},
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	// Verify raw key material is not stored in hash column.
	var storedHash string
	if err := db.QueryRowContext(ctx, `SELECT key_hash FROM api_keys WHERE id = ?`, "key-1").Scan(&storedHash); err != nil {
		t.Fatalf("query key_hash: %v", err)
	}
	if storedHash == raw {
		t.Fatalf("raw key should never be stored")
	}
	if storedHash != hash {
		t.Fatalf("expected hash %q, got %q", hash, storedHash)
	}

	key, found, err := GetActiveAPIKeyByHash(ctx, db, hash)
	if err != nil {
		t.Fatalf("GetActiveAPIKeyByHash: %v", err)
	}
	if !found {
		t.Fatalf("expected key to be found")
	}
	if key.ID != "key-1" || key.OwnerID != "owner-a" {
		t.Fatalf("unexpected key: %+v", key)
	}

	if err := TouchAPIKeyLastUsed(ctx, db, "key-1", now.Add(time.Minute)); err != nil {
		t.Fatalf("TouchAPIKeyLastUsed: %v", err)
	}
	keys, err := ListActiveAPIKeys(ctx, db)
	if err != nil {
		t.Fatalf("ListActiveAPIKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 active key, got %d", len(keys))
	}
	if keys[0].LastUsedAt == nil {
		t.Fatalf("expected last_used_at to be set")
	}

	if err := RevokeAPIKey(ctx, db, "key-1", now.Add(2*time.Minute)); err != nil {
		t.Fatalf("RevokeAPIKey: %v", err)
	}
	_, found, err = GetActiveAPIKeyByHash(ctx, db, hash)
	if err != nil {
		t.Fatalf("GetActiveAPIKeyByHash after revoke: %v", err)
	}
	if found {
		t.Fatalf("revoked key should not be found as active")
	}

	// Idempotent revoke should not fail.
	if err := RevokeAPIKey(ctx, db, "key-1", now.Add(3*time.Minute)); err != nil {
		t.Fatalf("RevokeAPIKey idempotent: %v", err)
	}
}
