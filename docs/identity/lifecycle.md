# NAVI Identity — Lifecycle

This page describes how identities are created, loaded, rotated, and revoked.

## Statuses

An identity has one of:

| Status | Meaning |
|--------|--------|
| `active` | The single current identity for this NAVI instance. Used for signing and API exposure. |
| `superseded` | Replaced by a new identity via key rotation. Links to the new identity via `superseded_by_id` and `rotation_link_sig`. |
| `revoked` | No longer trusted. Optional `revoked_at` and `revocation_reason` record when and why. |

At most one row in `agent_identities` has status `active` (enforced by a unique partial index).

## Creation and load

- **First boot**: No row with `status = 'active'`. `EnsureInitialized` generates an Ed25519 keypair, inserts one row (status `active`), and saves the private key to the keystore.
- **Subsequent boots**: An active row exists. `EnsureInitialized` verifies the keystore still has the private key for that identity; if not, it returns an error (no silent regeneration).

## Key rotation

Rotation replaces the active identity with a new keypair while proving continuity:

1. Load the current active identity and its private key.
2. Generate a new Ed25519 keypair.
3. Build a canonical rotation payload: `navi:rotation:v1:<old_id>:<old_fingerprint>:<new_id>:<new_fingerprint>:<timestamp>`.
4. Sign the payload with the **old** private key; store the signature as `rotation_link_sig` on the old row.
5. In a single DB transaction: mark the old identity as `superseded` (set `superseded_by_id`, `rotation_link_sig`) and insert the new identity as `active` (with `supersedes_id` pointing to the old).
6. Save the new private key to the keystore.

Peers that already trust the old key can verify the rotation signature and trust the new key. API: `identity.RotateIdentity(ctx, db, ks)`.

## Revocation

Revocation marks an identity as no longer trusted (e.g. compromise or decommission):

1. Call `identity.RevokeIdentity(ctx, db, id, reason, at)`.
2. The identity’s status is set to `revoked`; `revoked_at` and `revocation_reason` are stored when provided.

After revoking the current active identity, there is no active identity until a new one is created (e.g. by re-running initialization in a controlled way or restoring from backup). Revocation is intended for reporting and future trust/registry use; it does not by itself create a new key.

## Trust recovery (future)

- **Old key still valid**: Use signed rotation; peers verify `rotation_link_sig` and adopt the new identity.
- **Old key compromised or lost**: Trust must be re-established out-of-band (e.g. user confirms new fingerprint, or multiple trusted peers attest to the new key).
