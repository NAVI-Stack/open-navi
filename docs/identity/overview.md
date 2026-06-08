# NAVI Identity — Overview

Every NAVI instance is a **cryptographically identifiable agent**. The identity layer provides a unique keypair per instance, secure storage of the private key, and public metadata for verification and future trust flows.

## Concepts

- **Agent identity**: Public metadata (ID, key type, public key, fingerprint, status) stored in SQLite (`agent_identities`). One identity is **active** at a time per instance.
- **Keypair**: Ed25519 public/private key generated at first boot. The private key is stored in a **keystore**; the public key and fingerprint are stored in the database.
- **Canonical identity string**: `navi:<fingerprint>` where fingerprint is `hex(sha256(public_key))`. Used in logs, APIs, and future NAVI Net messaging.
- **Keystore**: Pluggable backend for persisting the private key. Default is encrypted file; dev and hardware backends are available.

## Boot flow

On startup, `navid` (see `cmd/navid/main.go`):

1. Opens the SQLite DB and creates/migrates tables (including `agent_identities`).
2. Builds a keystore from config (backend, data dir, optional passphrase).
3. Calls `identity.EnsureInitialized(ctx, db, keystore)`.
4. If no active identity exists, a new Ed25519 keypair is generated, public metadata is written to the DB, and the private key is saved to the keystore.
5. If an active identity exists, the keystore is checked for the matching private key; if it is missing, startup fails.

Identity is not regenerated on restart; the same key is reused until rotation or revocation.

## Where identity lives

| Item | Location |
|------|----------|
| Public metadata | SQLite `agent_identities` (id, key_type, public_key, fingerprint, created_at, status, plus revocation/rotation columns) |
| Private key | Keystore (default: `identity/private.key.enc` under the data directory, AES-256-GCM encrypted) |
| Backend selection | Env `NAVI_IDENTITY_KEYSTORE_BACKEND`; optional `NAVI_IDENTITY_KEYSTORE_PASSPHRASE` or `NAVI_IDENTITY_KEYSTORE_OS_SECRET` |

## Related docs

- [Lifecycle](lifecycle.md) — creation, rotation, revocation.
- [API](api.md) — Go APIs for signing, verification, and identity helpers.
- [Security](security.md) — algorithms, threat model, and operational guidance.
