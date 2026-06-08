# NAVI Identity — Security

Cryptographic choices, threat model, and operational guidance for the identity layer.

## Cryptographic choices

| Component | Choice | Notes |
|-----------|--------|--------|
| Key agreement / signing | Ed25519 | Standard curve; 32-byte public key, 64-byte signature. |
| Private key encryption | AES-256-GCM | Authenticated encryption for the keystore envelope. |
| Key derivation | PBKDF2-HMAC-SHA256 | KEK derived from passphrase or OS-bound material. Iterations and salt are versioned in the file header. |
| Randomness | `crypto/rand` | Used for key generation, nonces, and salts. Never overridden in production. |

Public keys and fingerprints are stored as base64 (RawStdEncoding) and hex respectively. Signatures are 64-byte Ed25519, transported as base64.

## Threat model

**Assets**

- Ed25519 private key (highest impact: full impersonation if compromised).
- Keystore file or dev env variable holding the encrypted (or plain) key.
- Agent identity metadata (public); exposure is acceptable but enables correlation.

**Threats**

- **Key theft**: Attacker obtains the decrypted private key (host compromise, weak permissions, unsafe backup).
- **Keystore offline attack**: Attacker steals the encrypted key file and brute-forces the KEK (e.g. weak passphrase).
- **Impersonation**: Attacker uses a stolen key or claims another agent’s identity without a valid key.
- **Replay**: Signed messages are replayed in a different context.
- **Rotation abuse**: Attacker triggers or manipulates rotation to confuse identity mapping.

**Mitigations**

- Strong PBKDF2 parameters (e.g. 600k iterations), versioned for future increases.
- Strict filesystem permissions (0700 dir, 0600 file) and atomic writes for the keystore.
- Clear separation between production (`file_encrypted`) and dev (`env/dev`); dev is not for production.
- Signed payloads should include timestamps, nonces, or context (e.g. session ID) where replay is a concern; document this for callers of `Sign`/`SignEnvelope`.
- Revocation and rotation metadata allow higher layers to react to compromise and key change.

## Backends and secrets

- **file_encrypted (default)**  
  KEK is derived from, in order: explicit passphrase, `NAVI_IDENTITY_KEYSTORE_OS_SECRET`, or an OS-bound seed (hostname, user, paths, etc.). OS-bound binding limits decryption to the same host/user context.

- **env/dev**  
  Private key in an env var (e.g. base64). For tests and ephemeral environments only; not suitable for production.

- **hardware**  
  Placeholder for future TPM/HSM; returns `ErrNotImplemented`.

## Backup and restore

- Back up the keystore file and the SQLite DB together so identity metadata and the encrypted key stay in sync.
- If using a passphrase, store it securely; without it (or the same OS context for OS-bound), the encrypted key cannot be decrypted.
- Restoring to a different host may require the passphrase or `NAVI_IDENTITY_KEYSTORE_OS_SECRET` if the OS-bound path was used.

## Key rotation and revocation

- Rotate proactively with `RotateIdentity`; the old key signs the rotation so peers can verify the transition.
- If the key is compromised, revoke with `RevokeIdentity` and establish a new identity through a controlled process (and out-of-band trust if needed).
