package identity

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ceoai/navi/internal/identity/keystore"
	"github.com/ceoai/navi/internal/store"
)

const keyTypeEd25519 = "ed25519"

var (
	ErrNoActiveIdentity = errors.New("identity: no active identity")
	ErrUnsupportedKeyType = errors.New("identity: unsupported key type")
	ErrInvalidSignature = errors.New("identity: invalid signature")
)

// Keypair is a generated Ed25519 keypair.
type Keypair struct {
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
}

// SignedEnvelope holds a signed payload with identity metadata for transport or storage.
type SignedEnvelope struct {
	Payload       []byte
	Signature     string
	KeyFingerprint string
	KeyType       string
	CreatedAt     time.Time
}

// GenerateKeypair generates an Ed25519 keypair using crypto/rand.
func GenerateKeypair() (Keypair, error) {
	return generateKeypair(rand.Reader)
}

// EnsureInitialized ensures an active local identity exists. If absent, it
// generates and persists one atomically.
func EnsureInitialized(ctx context.Context, db *sql.DB, ks keystore.Store) (store.AgentIdentity, bool, error) {
	return ensureInitialized(ctx, db, ks, rand.Reader, time.Now)
}

func ensureInitialized(ctx context.Context, db *sql.DB, ks keystore.Store, reader io.Reader, now func() time.Time) (store.AgentIdentity, bool, error) {
	if ks == nil {
		return store.AgentIdentity{}, false, fmt.Errorf("identity: keystore is required")
	}
	existing, found, err := store.GetActiveAgentIdentity(ctx, db)
	if err != nil {
		return store.AgentIdentity{}, false, fmt.Errorf("identity: read active identity: %w", err)
	}
	if found {
		hasKey, err := ks.Exists(ctx, existing.ID)
		if err != nil {
			return store.AgentIdentity{}, false, fmt.Errorf("identity: check keystore key presence: %w", err)
		}
		if !hasKey {
			return store.AgentIdentity{}, false, fmt.Errorf("identity: active identity exists but private key is missing")
		}
		return existing, false, nil
	}

	keypair, err := generateKeypair(reader)
	if err != nil {
		return store.AgentIdentity{}, false, fmt.Errorf("identity: generate keypair: %w", err)
	}

	publicKey := base64.RawStdEncoding.EncodeToString(keypair.PublicKey)
	identity := store.AgentIdentity{
		KeyType:     keyTypeEd25519,
		PublicKey:   publicKey,
		Fingerprint: fingerprint(keypair.PublicKey),
		CreatedAt:   now().UTC(),
		Status:      store.AgentIdentityStatusActive,
	}

	stored, created, err := store.CreateInitialAgentIdentity(ctx, db, identity)
	if err != nil {
		return store.AgentIdentity{}, false, fmt.Errorf("identity: persist identity: %w", err)
	}
	if err := ks.Save(ctx, stored.ID, keypair.PrivateKey); err != nil {
		return store.AgentIdentity{}, false, fmt.Errorf("identity: persist private key: %w", err)
	}
	return stored, created, nil
}

// LoadActiveKeypair loads the active identity metadata and corresponding private key from the keystore.
// Returns descriptive errors if no active identity exists or the keystore does not hold the key.
func LoadActiveKeypair(ctx context.Context, db *sql.DB, ks keystore.Store) (store.AgentIdentity, ed25519.PrivateKey, error) {
	if ks == nil {
		return store.AgentIdentity{}, nil, fmt.Errorf("identity: keystore is required")
	}
	id, found, err := store.GetActiveAgentIdentity(ctx, db)
	if err != nil {
		return store.AgentIdentity{}, nil, fmt.Errorf("identity: read active identity: %w", err)
	}
	if !found {
		return store.AgentIdentity{}, nil, ErrNoActiveIdentity
	}
	if id.KeyType != keyTypeEd25519 {
		return store.AgentIdentity{}, nil, fmt.Errorf("%w: %q", ErrUnsupportedKeyType, id.KeyType)
	}
	raw, err := ks.Load(ctx, id.ID)
	if err != nil {
		if errors.Is(err, keystore.ErrNotFound) {
			return store.AgentIdentity{}, nil, fmt.Errorf("identity: active identity %q has no private key in keystore", id.ID)
		}
		return store.AgentIdentity{}, nil, fmt.Errorf("identity: load private key: %w", err)
	}
	if len(raw) != ed25519.PrivateKeySize {
		return store.AgentIdentity{}, nil, fmt.Errorf("identity: invalid private key length for %q", id.KeyType)
	}
	pub, err := decodePublicKey(id.PublicKey)
	if err != nil {
		return store.AgentIdentity{}, nil, fmt.Errorf("identity: decode stored public key: %w", err)
	}
	priv := ed25519.PrivateKey(raw)
	if !ed25519.PublicKey(pub).Equal(priv.Public().(ed25519.PublicKey)) {
		return store.AgentIdentity{}, nil, fmt.Errorf("identity: keystore key does not match identity %q", id.ID)
	}
	return id, priv, nil
}

// Sign signs msg with the active identity's private key and returns the signature as base64 and the identity.
func Sign(ctx context.Context, db *sql.DB, ks keystore.Store, msg []byte) (signatureBase64 string, id store.AgentIdentity, err error) {
	id, priv, err := LoadActiveKeypair(ctx, db, ks)
	if err != nil {
		return "", store.AgentIdentity{}, err
	}
	sig := ed25519.Sign(priv, msg)
	return base64.RawStdEncoding.EncodeToString(sig), id, nil
}

// SignEnvelope signs msg and returns a SignedEnvelope (for internal use or APIs).
func SignEnvelope(ctx context.Context, db *sql.DB, ks keystore.Store, payload []byte) (SignedEnvelope, error) {
	sigB64, id, err := Sign(ctx, db, ks, payload)
	if err != nil {
		return SignedEnvelope{}, err
	}
	return SignedEnvelope{
		Payload:        payload,
		Signature:      sigB64,
		KeyFingerprint: id.Fingerprint,
		KeyType:        id.KeyType,
		CreatedAt:      time.Now().UTC(),
	}, nil
}

// VerifyWithIdentity verifies a signature over msg using the identity's public key.
func VerifyWithIdentity(id store.AgentIdentity, msg []byte, signatureBase64 string) error {
	if id.KeyType != keyTypeEd25519 {
		return fmt.Errorf("%w: %q", ErrUnsupportedKeyType, id.KeyType)
	}
	pub, err := decodePublicKey(id.PublicKey)
	if err != nil {
		return fmt.Errorf("identity: decode public key: %w", err)
	}
	sig, err := base64.RawStdEncoding.DecodeString(signatureBase64)
	if err != nil {
		return fmt.Errorf("identity: decode signature: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("%w", ErrInvalidSignature)
	}
	if !ed25519.Verify(pub, msg, sig) {
		return fmt.Errorf("%w", ErrInvalidSignature)
	}
	return nil
}

// VerifyWithPublicKey verifies a signature over msg using a base64-encoded Ed25519 public key.
func VerifyWithPublicKey(pubKeyBase64 string, msg []byte, signatureBase64 string) error {
	pub, err := decodePublicKey(pubKeyBase64)
	if err != nil {
		return fmt.Errorf("identity: decode public key: %w", err)
	}
	sig, err := base64.RawStdEncoding.DecodeString(signatureBase64)
	if err != nil {
		return fmt.Errorf("identity: decode signature: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("%w", ErrInvalidSignature)
	}
	if !ed25519.Verify(pub, msg, sig) {
		return fmt.Errorf("%w", ErrInvalidSignature)
	}
	return nil
}

// NaviID returns the canonical identity string for the given fingerprint: "navi:<fingerprint>".
func NaviID(fingerprint string) string {
	return "navi:" + fingerprint
}

const rotationPayloadPrefix = "navi:rotation:v1:"

// RotateIdentity generates a new keypair, marks the current identity as superseded, and makes the new identity active.
// The old key signs a canonical rotation payload linking old and new; the signature is stored as rotation_link_sig.
// Returns the old identity, the new identity, and the raw link signature bytes.
func RotateIdentity(ctx context.Context, db *sql.DB, ks keystore.Store) (oldID, newID store.AgentIdentity, linkSig []byte, err error) {
	oldID, oldPriv, err := LoadActiveKeypair(ctx, db, ks)
	if err != nil {
		return store.AgentIdentity{}, store.AgentIdentity{}, nil, err
	}
	newKeypair, err := GenerateKeypair()
	if err != nil {
		return store.AgentIdentity{}, store.AgentIdentity{}, nil, fmt.Errorf("identity: generate new keypair: %w", err)
	}
	newIDStr := store.NewKeyID()
	newPubB64 := base64.RawStdEncoding.EncodeToString(newKeypair.PublicKey)
	newFingerprint := fingerprint(newKeypair.PublicKey)
	now := time.Now().UTC()
	payload := fmt.Sprintf("%s%s:%s:%s:%s:%s", rotationPayloadPrefix, oldID.ID, oldID.Fingerprint, newIDStr, newFingerprint, now.Format(time.RFC3339Nano))
	linkSig = ed25519.Sign(oldPriv, []byte(payload))
	linkSigB64 := base64.RawStdEncoding.EncodeToString(linkSig)

	newIdentity := store.AgentIdentity{
		ID:          newIDStr,
		KeyType:     keyTypeEd25519,
		PublicKey:   newPubB64,
		Fingerprint: newFingerprint,
		CreatedAt:   now,
		Status:      store.AgentIdentityStatusActive,
		SupersedesID: oldID.ID,
	}
	if err := store.ApplyRotation(ctx, db, oldID.ID, linkSigB64, newIDStr, newIdentity); err != nil {
		return store.AgentIdentity{}, store.AgentIdentity{}, nil, fmt.Errorf("identity: apply rotation: %w", err)
	}
	if err := ks.Save(ctx, newIDStr, newKeypair.PrivateKey); err != nil {
		return store.AgentIdentity{}, store.AgentIdentity{}, nil, fmt.Errorf("identity: persist new private key: %w", err)
	}
	newID = newIdentity
	newID.CreatedAt = now
	return oldID, newID, linkSig, nil
}

// RevokeIdentity marks the given identity as revoked and records the reason and timestamp.
func RevokeIdentity(ctx context.Context, db *sql.DB, id string, reason string, at time.Time) error {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	reasonPtr := &reason
	if reason == "" {
		reasonPtr = nil
	}
	return store.UpdateAgentIdentity(ctx, db, id, store.AgentIdentityStatusRevoked, &store.AgentIdentityUpdateOpts{
		RevokedAt:        &at,
		RevocationReason: reasonPtr,
	})
}

func decodePublicKey(b64 string) (ed25519.PublicKey, error) {
	raw, err := base64.RawStdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid ed25519 public key length %d", len(raw))
	}
	return ed25519.PublicKey(raw), nil
}

func generateKeypair(reader io.Reader) (Keypair, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(reader)
	if err != nil {
		return Keypair{}, fmt.Errorf("generate ed25519 keypair: %w", err)
	}
	return Keypair{PublicKey: publicKey, PrivateKey: privateKey}, nil
}

func fingerprint(publicKey ed25519.PublicKey) string {
	sum := sha256.Sum256(publicKey)
	return hex.EncodeToString(sum[:])
}
