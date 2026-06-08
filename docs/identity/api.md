# NAVI Identity — API Reference

Public Go APIs in `internal/identity` and related types in `internal/store`.

## Initialization and loading

- **`EnsureInitialized(ctx, db, ks)`** — Ensures an active identity exists; creates one if missing. Returns `(AgentIdentity, created bool, error)`.
- **`LoadActiveKeypair(ctx, db, ks)`** — Loads the active identity and its private key from the keystore. Returns `(AgentIdentity, ed25519.PrivateKey, error)`. Use when you need to sign or need the raw key.

## Signing and verification

- **`Sign(ctx, db, ks, msg []byte)`** — Signs `msg` with the active identity’s private key. Returns `(signatureBase64 string, AgentIdentity, error)`. Signature is Ed25519, 64 bytes, base64-encoded.
- **`SignEnvelope(ctx, db, ks, payload []byte)`** — Signs and returns a `SignedEnvelope`: `Payload`, `Signature` (base64), `KeyFingerprint`, `KeyType`, `CreatedAt`.
- **`VerifyWithIdentity(id AgentIdentity, msg []byte, signatureBase64 string) error`** — Verifies a signature using the identity’s public key. Returns `ErrInvalidSignature` (wrapped) on failure; supports `errors.Is`.
- **`VerifyWithPublicKey(pubKeyBase64, msg []byte, signatureBase64 string) error`** — Same verification using a raw base64 Ed25519 public key.

## Identity helpers

- **`NaviID(fingerprint string) string`** — Returns `"navi:" + fingerprint` (canonical identity string).

## Rotation and revocation

- **`RotateIdentity(ctx, db, ks)`** — Generates a new keypair, marks the current identity as superseded, and makes the new one active. Returns `(oldID, newID AgentIdentity, linkSig []byte, error)`. The old key signs a rotation payload; the signature is stored as `rotation_link_sig`.
- **`RevokeIdentity(ctx, db, id, reason string, at time.Time) error`** — Sets the identity’s status to `revoked` and optionally sets `revoked_at` and `revocation_reason`. Use empty `reason` to leave reason unchanged; `at` defaults to now if zero.

## Errors

- `ErrNoActiveIdentity` — No active identity in the DB.
- `ErrUnsupportedKeyType` — Key type is not supported (e.g. not Ed25519).
- `ErrInvalidSignature` — Signature verification failed (decode or crypto).

Use `errors.Is(err, identity.ErrInvalidSignature)` etc. for checks.

## Store types (`internal/store`)

- **`AgentIdentity`** — ID, KeyType, PublicKey, Fingerprint, CreatedAt, Status; RevokedAt, RevocationReason, SupersedesID, SupersededByID, RotationLinkSig (for rotation/revocation).
- **`GetActiveAgentIdentity(ctx, db)`** — Returns the active identity, if any.
- **`GetAgentIdentityByID(ctx, db, id)`** — Returns an identity by ID (including revocation/rotation fields).
- **`UpdateAgentIdentity(ctx, db, id, status, opts *AgentIdentityUpdateOpts)`** — Updates status and optional revocation/rotation fields. `opts` pointer fields (e.g. `RevokedAt`, `RevocationReason`) are optional; nil means “do not change”.

## Example: sign and verify

```go
sigB64, id, err := identity.Sign(ctx, db, ks, []byte("message"))
if err != nil { ... }

err = identity.VerifyWithIdentity(id, []byte("message"), sigB64)
if err != nil { ... }
```

## Example: verify with public key only

```go
err := identity.VerifyWithPublicKey(peerPublicKeyBase64, msg, signatureBase64)
if errors.Is(err, identity.ErrInvalidSignature) { ... }
```
