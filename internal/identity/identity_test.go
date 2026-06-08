package identity

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/identity/keystore"
	"github.com/open-navi/navi/internal/store"
)

func TestEnsureInitializedFirstGeneration(t *testing.T) {
	ctx := context.Background()
	db := store.InitTestDB(t)
	ks := keystore.NewFileEncryptedStore(t.TempDir()+"/identity/private.key.enc", keystore.FileEncryptedConfig{
		Passphrase:       "test-passphrase",
		PBKDF2Iterations: 1_000,
	})

	id, created, err := EnsureInitialized(ctx, db, ks)
	if err != nil {
		t.Fatalf("EnsureInitialized error: %v", err)
	}
	if !created {
		t.Fatalf("expected first boot to create identity")
	}
	if id.KeyType != keyTypeEd25519 {
		t.Fatalf("expected key_type=%q, got %q", keyTypeEd25519, id.KeyType)
	}
	if id.Status != store.AgentIdentityStatusActive {
		t.Fatalf("expected status=%q, got %q", store.AgentIdentityStatusActive, id.Status)
	}
	if id.PublicKey == "" || id.Fingerprint == "" {
		t.Fatalf("expected public metadata to be populated")
	}
	if id.CreatedAt.IsZero() {
		t.Fatalf("expected created_at to be set")
	}

	stored, found, err := store.GetActiveAgentIdentity(ctx, db)
	if err != nil {
		t.Fatalf("GetActiveAgentIdentity error: %v", err)
	}
	if !found {
		t.Fatalf("expected active identity metadata in store")
	}
	if stored.Fingerprint != id.Fingerprint {
		t.Fatalf("expected stored fingerprint %q, got %q", id.Fingerprint, stored.Fingerprint)
	}

	privateKey, err := ks.Load(ctx, id.ID)
	if err != nil {
		t.Fatalf("keystore load error: %v", err)
	}
	if len(privateKey) != ed25519.PrivateKeySize {
		t.Fatalf("expected private key to be persisted")
	}
	publicKeyRaw, err := base64.RawStdEncoding.DecodeString(id.PublicKey)
	if err != nil {
		t.Fatalf("decode stored public key: %v", err)
	}
	if !bytes.Equal(ed25519.PrivateKey(privateKey).Public().(ed25519.PublicKey), publicKeyRaw) {
		t.Fatalf("private/public key mismatch")
	}
}

func TestEnsureInitializedRestartReuse(t *testing.T) {
	ctx := context.Background()
	db := store.InitTestDB(t)
	ks := keystore.NewFileEncryptedStore(t.TempDir()+"/identity/private.key.enc", keystore.FileEncryptedConfig{
		Passphrase:       "test-passphrase",
		PBKDF2Iterations: 1_000,
	})

	first, created, err := EnsureInitialized(ctx, db, ks)
	if err != nil {
		t.Fatalf("first EnsureInitialized error: %v", err)
	}
	if !created {
		t.Fatalf("expected first call to create identity")
	}
	privateKeyBefore, err := ks.Load(ctx, first.ID)
	if err != nil {
		t.Fatalf("keystore load before restart error: %v", err)
	}

	// Restart path must reuse existing identity and never touch RNG.
	second, created, err := ensureInitialized(ctx, db, ks, failingReader{err: errors.New("rng should not be used")}, time.Now)
	if err != nil {
		t.Fatalf("second ensureInitialized error: %v", err)
	}
	if created {
		t.Fatalf("expected restart to reuse identity")
	}
	if second.Fingerprint != first.Fingerprint || second.PublicKey != first.PublicKey {
		t.Fatalf("expected same identity metadata across restart")
	}

	privateKeyAfter, err := ks.Load(ctx, second.ID)
	if err != nil {
		t.Fatalf("keystore load after restart error: %v", err)
	}
	if string(privateKeyAfter) != string(privateKeyBefore) {
		t.Fatalf("expected private key to remain unchanged across restart")
	}
}

func TestEnsureInitializedRNGFailureHandling(t *testing.T) {
	ctx := context.Background()
	db := store.InitTestDB(t)
	ks := keystore.NewFileEncryptedStore(t.TempDir()+"/identity/private.key.enc", keystore.FileEncryptedConfig{
		Passphrase:       "test-passphrase",
		PBKDF2Iterations: 1_000,
	})

	_, created, err := ensureInitialized(ctx, db, ks, failingReader{err: io.ErrUnexpectedEOF}, time.Now)
	if err == nil {
		t.Fatalf("expected rng failure error")
	}
	if created {
		t.Fatalf("expected no identity creation on rng failure")
	}
	if !strings.Contains(err.Error(), "generate keypair") {
		t.Fatalf("expected generation error context, got: %v", err)
	}

	_, found, err := store.GetActiveAgentIdentity(ctx, db)
	if err != nil {
		t.Fatalf("GetActiveAgentIdentity error: %v", err)
	}
	if found {
		t.Fatalf("expected no metadata row on rng failure")
	}
	found, err = ks.Exists(ctx, "any")
	if err != nil {
		t.Fatalf("keystore exists error: %v", err)
	}
	if found {
		t.Fatalf("expected no private key file on rng failure")
	}
}

func TestEnsureInitializedFailsWhenMetadataExistsButKeyMissing(t *testing.T) {
	ctx := context.Background()
	db := store.InitTestDB(t)
	ks := keystore.NewFileEncryptedStore(t.TempDir()+"/identity/private.key.enc", keystore.FileEncryptedConfig{
		Passphrase:       "test-passphrase",
		PBKDF2Iterations: 1_000,
	})

	id, _, err := store.CreateInitialAgentIdentity(ctx, db, store.AgentIdentity{
		KeyType:     keyTypeEd25519,
		PublicKey:   "public",
		Fingerprint: "fingerprint",
		Status:      store.AgentIdentityStatusActive,
	})
	if err != nil {
		t.Fatalf("create metadata: %v", err)
	}
	if id.ID == "" {
		t.Fatalf("expected created identity id")
	}

	_, _, err = EnsureInitialized(ctx, db, ks)
	if err == nil {
		t.Fatalf("expected error when metadata exists but key is missing")
	}
	if !strings.Contains(err.Error(), "private key is missing") {
		t.Fatalf("expected missing private key error, got %v", err)
	}
}

func TestLoadActiveKeypair(t *testing.T) {
	ctx := context.Background()
	db := store.InitTestDB(t)
	ks := keystore.NewFileEncryptedStore(t.TempDir()+"/identity/private.key.enc", keystore.FileEncryptedConfig{
		Passphrase:       "test-passphrase",
		PBKDF2Iterations: 1_000,
	})

	_, _, err := EnsureInitialized(ctx, db, ks)
	if err != nil {
		t.Fatalf("EnsureInitialized: %v", err)
	}

	id, priv, err := LoadActiveKeypair(ctx, db, ks)
	if err != nil {
		t.Fatalf("LoadActiveKeypair: %v", err)
	}
	if id.KeyType != keyTypeEd25519 || id.Fingerprint == "" {
		t.Fatalf("unexpected identity: key_type=%q fingerprint=%q", id.KeyType, id.Fingerprint)
	}
	if len(priv) != ed25519.PrivateKeySize {
		t.Fatalf("expected private key length %d, got %d", ed25519.PrivateKeySize, len(priv))
	}
	if !ed25519.PublicKey(priv.Public().(ed25519.PublicKey)).Equal(priv.Public().(ed25519.PublicKey)) {
		t.Fatalf("private key public mismatch")
	}
}

func TestLoadActiveKeypairNoIdentity(t *testing.T) {
	ctx := context.Background()
	db := store.InitTestDB(t)
	ks := keystore.NewFileEncryptedStore(t.TempDir()+"/identity/private.key.enc", keystore.FileEncryptedConfig{
		Passphrase:       "test-passphrase",
		PBKDF2Iterations: 1_000,
	})

	_, _, err := LoadActiveKeypair(ctx, db, ks)
	if err == nil {
		t.Fatalf("expected error when no identity")
	}
	if !errors.Is(err, ErrNoActiveIdentity) {
		t.Fatalf("expected ErrNoActiveIdentity, got %v", err)
	}
}

func TestLoadActiveKeypairNilKeystore(t *testing.T) {
	ctx := context.Background()
	db := store.InitTestDB(t)
	_, _, err := LoadActiveKeypair(ctx, db, nil)
	if err == nil {
		t.Fatalf("expected error for nil keystore")
	}
}

func TestSignAndVerify(t *testing.T) {
	ctx := context.Background()
	db := store.InitTestDB(t)
	ks := keystore.NewFileEncryptedStore(t.TempDir()+"/identity/private.key.enc", keystore.FileEncryptedConfig{
		Passphrase:       "test-passphrase",
		PBKDF2Iterations: 1_000,
	})

	_, _, err := EnsureInitialized(ctx, db, ks)
	if err != nil {
		t.Fatalf("EnsureInitialized: %v", err)
	}

	msg := []byte("hello navi")
	sigB64, id, err := Sign(ctx, db, ks, msg)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if sigB64 == "" || id.Fingerprint == "" {
		t.Fatalf("expected signature and identity")
	}

	if err := VerifyWithIdentity(id, msg, sigB64); err != nil {
		t.Fatalf("VerifyWithIdentity: %v", err)
	}
	if err := VerifyWithPublicKey(id.PublicKey, msg, sigB64); err != nil {
		t.Fatalf("VerifyWithPublicKey: %v", err)
	}

	wrongMsg := []byte("wrong")
	verifyErr := VerifyWithIdentity(id, wrongMsg, sigB64)
	if verifyErr == nil {
		t.Fatalf("expected VerifyWithIdentity to fail on wrong message")
	}
	if !errors.Is(verifyErr, ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature, got %v", verifyErr)
	}
}

func TestSignEnvelope(t *testing.T) {
	ctx := context.Background()
	db := store.InitTestDB(t)
	ks := keystore.NewFileEncryptedStore(t.TempDir()+"/identity/private.key.enc", keystore.FileEncryptedConfig{
		Passphrase:       "test-passphrase",
		PBKDF2Iterations: 1_000,
	})

	_, _, err := EnsureInitialized(ctx, db, ks)
	if err != nil {
		t.Fatalf("EnsureInitialized: %v", err)
	}

	payload := []byte("envelope payload")
	env, err := SignEnvelope(ctx, db, ks, payload)
	if err != nil {
		t.Fatalf("SignEnvelope: %v", err)
	}
	if env.Signature == "" || env.KeyFingerprint == "" || env.KeyType != keyTypeEd25519 {
		t.Fatalf("unexpected envelope: %+v", env)
	}
	if !bytes.Equal(env.Payload, payload) {
		t.Fatalf("payload mismatch")
	}

	id, found, _ := store.GetActiveAgentIdentity(ctx, db)
	if !found || id.Fingerprint != env.KeyFingerprint {
		t.Fatalf("envelope fingerprint should match active identity")
	}
	if err := VerifyWithIdentity(id, payload, env.Signature); err != nil {
		t.Fatalf("VerifyWithIdentity(envelope): %v", err)
	}
}

func TestNaviID(t *testing.T) {
	if got := NaviID("abc123"); got != "navi:abc123" {
		t.Fatalf("NaviID(abc123)=%q", got)
	}
}

func TestRotateIdentity(t *testing.T) {
	ctx := context.Background()
	db := store.InitTestDB(t)
	ks := keystore.NewFileEncryptedStore(t.TempDir()+"/identity/private.key.enc", keystore.FileEncryptedConfig{
		Passphrase:       "test-passphrase",
		PBKDF2Iterations: 1_000,
	})

	_, _, err := EnsureInitialized(ctx, db, ks)
	if err != nil {
		t.Fatalf("EnsureInitialized: %v", err)
	}

	oldID, newID, linkSig, err := RotateIdentity(ctx, db, ks)
	if err != nil {
		t.Fatalf("RotateIdentity: %v", err)
	}
	if len(linkSig) != ed25519.SignatureSize {
		t.Fatalf("expected link signature length %d, got %d", ed25519.SignatureSize, len(linkSig))
	}
	if oldID.Fingerprint == newID.Fingerprint {
		t.Fatalf("old and new fingerprints must differ")
	}
	if newID.Status != store.AgentIdentityStatusActive {
		t.Fatalf("new identity must be active, got %q", newID.Status)
	}
	if newID.SupersedesID != oldID.ID {
		t.Fatalf("new identity must supersede old id")
	}

	active, found, err := store.GetActiveAgentIdentity(ctx, db)
	if err != nil || !found {
		t.Fatalf("expected one active identity after rotation")
	}
	if active.ID != newID.ID || active.Fingerprint != newID.Fingerprint {
		t.Fatalf("active identity should be the new one")
	}

	oldRow, found, err := store.GetAgentIdentityByID(ctx, db, oldID.ID)
	if err != nil || !found {
		t.Fatalf("GetAgentIdentityByID(old): %v", err)
	}
	if oldRow.Status != store.AgentIdentityStatusSuperseded {
		t.Fatalf("old identity status should be superseded, got %q", oldRow.Status)
	}
	if oldRow.SupersededByID != newID.ID {
		t.Fatalf("old identity superseded_by_id should be new id")
	}
	if oldRow.RotationLinkSig == "" {
		t.Fatalf("old identity should have rotation_link_sig")
	}

	newPriv, err := ks.Load(ctx, newID.ID)
	if err != nil {
		t.Fatalf("keystore load new key: %v", err)
	}
	if len(newPriv) != ed25519.PrivateKeySize {
		t.Fatalf("new private key should be in keystore")
	}
	msg := []byte("post-rotation sign")
	sigB64, id, err := Sign(ctx, db, ks, msg)
	if err != nil {
		t.Fatalf("Sign after rotation: %v", err)
	}
	if id.Fingerprint != newID.Fingerprint {
		t.Fatalf("Sign should use new identity")
	}
	if err := VerifyWithIdentity(id, msg, sigB64); err != nil {
		t.Fatalf("VerifyWithIdentity after rotation: %v", err)
	}
}

func TestRevokeIdentity(t *testing.T) {
	ctx := context.Background()
	db := store.InitTestDB(t)
	ks := keystore.NewFileEncryptedStore(t.TempDir()+"/identity/private.key.enc", keystore.FileEncryptedConfig{
		Passphrase:       "test-passphrase",
		PBKDF2Iterations: 1_000,
	})

	agentID, _, err := EnsureInitialized(ctx, db, ks)
	if err != nil {
		t.Fatalf("EnsureInitialized: %v", err)
	}

	reason := "test revocation"
	at := time.Now().UTC().Add(-time.Hour)
	err = RevokeIdentity(ctx, db, agentID.ID, reason, at)
	if err != nil {
		t.Fatalf("RevokeIdentity: %v", err)
	}

	_, found, err := store.GetActiveAgentIdentity(ctx, db)
	if err != nil {
		t.Fatalf("GetActiveAgentIdentity: %v", err)
	}
	if found {
		t.Fatalf("expected no active identity after revoke")
	}

	revoked, found, err := store.GetAgentIdentityByID(ctx, db, agentID.ID)
	if err != nil || !found {
		t.Fatalf("GetAgentIdentityByID: %v", err)
	}
	if revoked.Status != store.AgentIdentityStatusRevoked {
		t.Fatalf("expected status revoked, got %q", revoked.Status)
	}
	if revoked.RevocationReason != reason {
		t.Fatalf("expected revocation_reason %q, got %q", reason, revoked.RevocationReason)
	}
	if revoked.RevokedAt == "" {
		t.Fatalf("expected revoked_at set")
	}
}

type failingReader struct {
	err error
}

func (r failingReader) Read(_ []byte) (int, error) {
	return 0, r.err
}
