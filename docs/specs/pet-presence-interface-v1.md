# PET / NAVI Presence Interface Spec v1

**Status:** Active  
**Last Updated:** 2026-04-18  
**Updated By:** agent

## Purpose

This document defines the canonical interface contract for presence synchronization between **PET** and **NAVI**.

It standardizes:

- authority and ownership boundaries
- the canonical presence domain model
- REST snapshot surfaces
- WebSocket realtime event surfaces
- revision and staleness rules
- public vs private visibility rules
- runtime-to-presence normalization rules for NAVI

This is the **normative** spec for the PET/NAVI presence protocol. PET-facing integration notes may restate or narrow this contract, but they must not redefine it.

---

## Design goals

1. Preserve clear authority boundaries.
2. Allow realtime presence updates over WebSocket.
3. Allow snapshot bootstrap, fallback, and resync over REST.
4. Prevent UI-only inferred state from overwriting authoritative state.
5. Support future expansion for richer public/private presence without breaking the transport contract.
6. Keep transport semantics identical across REST and WebSocket.

---

## Authority model

### PET-authoritative state

PET is the sole authority for **user presence**.

PET-owned state includes:

- user public status
- user private/internal status
- user-authored status text or subtext
- user-originated presence revisions

NAVI may observe this state, but must not mutate it.

### NAVI-authoritative state

NAVI is the sole authority for **NAVI presence**.

NAVI-owned state includes:

- normalized public NAVI presence
- internal runtime status
- active work detail
- attention/proposal state
- health/degradation state
- NAVI-originated presence revisions

PET may render or locally cache this state, but must not mutate it.

### PET-derived overlay state

PET may derive **transport overlay state** when NAVI is unreachable or stale.

Allowed PET-local overlays:

- `transport_state = reconnecting`
- `transport_state = stale`
- `transport_state = offline`

These overlays do **not** replace NAVI-authoritative presence fields. They are a local rendering aid only.

### Prohibited cross-authority mutation

- PET must not write NAVI authoritative presence.
- NAVI must not write user authoritative presence.
- Neither side may silently assume ownership of a field it did not originate.

---

## Terminology

### Presence

User-facing or peer-facing availability/presence state.

### Runtime state

Internal NAVI execution/activity state derived from runtime truth.

### Transport state

Observed connection state between PET and NAVI.

### Public status

Status intended for normal presentation.

### Private status

Status visible only to the owner and NAVI.

### Snapshot

A complete REST representation of current presence state.

### Event

A WebSocket message representing a presence-related change or stream lifecycle signal.

---

## Canonical model

### PresenceEnvelope

Every authoritative presence message, whether sent via REST or WebSocket, must conform to the following logical shape.

```json
{
  "version": "v1",
  "source": "pet",
  "subject_type": "user",
  "subject_id": "owner",
  "authority": "pet",
  "transport_observed_at": "2026-04-16T18:00:00Z",
  "state_updated_at": "2026-04-16T18:00:00Z",
  "state_revision": 12,
  "visibility": "mixed",
  "payload": {}
}
```

### Envelope fields

| Field | Type | Meaning |
|------|------|---------|
| `version` | string | Protocol version. Initial value: `v1`. |
| `source` | enum | Emitter of the envelope: `pet` or `navi`. |
| `subject_type` | enum | `user` or `navi`. |
| `subject_id` | string | Stable identifier for the subject. |
| `authority` | enum | Declared authority over the payload. Must match owner domain. |
| `transport_observed_at` | RFC3339 datetime | When this envelope was emitted or observed on transport. |
| `state_updated_at` | RFC3339 datetime | When the authoritative subject state last changed. |
| `state_revision` | integer | Monotonic per-subject revision number. |
| `visibility` | enum | `public`, `private`, or `mixed`. |
| `payload` | object | Subject-specific payload. |

### Revision rules

- Revisions are monotonic per authoritative subject.
- A newer `state_revision` supersedes an older revision for the same `(subject_type, subject_id)`.
- Equal revision values must be treated as the same authoritative state unless `state_updated_at` differs, in which case the newer timestamp wins.
- Consumers must ignore out-of-order older revisions.

---

## User presence payload

```json
{
  "public_status": "available",
  "private_status": "focused",
  "status_text": "Heads down",
  "subtext": "Available for urgent interruptions only"
}
```

### User presence fields

| Field | Type | Required | Meaning |
|------|------|----------|---------|
| `public_status` | enum | yes | User-visible/public status. |
| `private_status` | enum/string | no | Internal/owner-visible status. |
| `status_text` | string | no | Short free-text status. |
| `subtext` | string | no | Supplemental descriptive text. |

### User public status enum v1

- `available`
- `busy`
- `do_not_disturb`
- `away`
- `offline`

### User private status guidance

`private_status` is intentionally extensible. v1 does not hard-freeze the value set. Consumers must tolerate unknown values.

Suggested values include:

- `focused`
- `resting`
- `sleeping`
- `stressed`
- `deep_work`
- `unknown`

---

## NAVI presence payload

```json
{
  "public_status": "working",
  "internal_status": "tool_executing",
  "status_text": "Reviewing repository state",
  "subtext": "Interruptible",
  "active_session_id": "session_123",
  "current_detail": "github.diff.inspect",
  "attention": {
    "level": "none",
    "reason_code": null,
    "proposal_id": null,
    "blocking": false
  },
  "health": {
    "state": "healthy",
    "last_activity_at": "2026-04-16T18:00:00Z",
    "stale_after_ms": 45000
  }
}
```

### NAVI presence fields

| Field | Type | Required | Meaning |
|------|------|----------|---------|
| `public_status` | enum | yes | PET-facing normalized NAVI presence. |
| `internal_status` | enum | yes | Internal runtime truth. |
| `status_text` | string | no | Short descriptive text. |
| `subtext` | string | no | Supplemental text for UI surfaces. |
| `active_session_id` | string | no | Current active session if any. |
| `current_detail` | string | no | Short work detail or capability identifier. |
| `attention` | object | yes | Attention/proposal state. |
| `health` | object | yes | Runtime health summary. |

### NAVI public status enum v1

- `active`
- `idle`
- `dreaming`
- `working`
- `busy`
- `offline`
- `needs_attention`
- `wants_attention`

### Future extensibility note

The v1 public-status enum is the canonical foundation, not the permanent ceiling. Future protocol revisions may extend the NAVI status set when justified by real runtime semantics and classification support, including states such as `sleeping`, `browsing`, or other future internal/public presence states. New statuses must come from explicit subsystem classification and contract evolution, not arbitrary string invention.

### NAVI internal status enum v1

- `idle`
- `processing`
- `tool_executing`
- `waiting_for_input`
- `heartbeat`
- `degraded`
- `offline`
- `unresponsive`

### NAVI attention object

| Field | Type | Required | Meaning |
|------|------|----------|---------|
| `level` | enum | yes | `none`, `wants_attention`, `needs_attention`. |
| `reason_code` | string/null | no | Machine-readable reason. |
| `proposal_id` | string/null | no | Proposal reference when relevant. |
| `blocking` | boolean | yes | True when current state blocks progress. |

### NAVI health object

| Field | Type | Required | Meaning |
|------|------|----------|---------|
| `state` | enum | yes | `healthy`, `degraded`, `unresponsive`, `offline`. |
| `last_activity_at` | RFC3339 datetime | no | Last meaningful runtime activity. |
| `stale_after_ms` | integer | yes | Suggested freshness window for consumers. |

---

## NAVI normalization rules

NAVI must expose both internal runtime truth and normalized public presence.

### Default runtime-to-public mapping

| Internal status | Default public status |
|-----------------|-----------------------|
| `idle` | `idle` |
| `processing` | `active` |
| `tool_executing` | `working` |
| `heartbeat` | `active` |
| `offline` | `offline` |

`heartbeat` must not default to `dreaming`.
Public `dreaming` requires a distinct background-processing introspection/classification seam.

### Waiting-for-input mapping

When `internal_status = waiting_for_input`:

- if there is a blocking proposal or urgent action required, `public_status = needs_attention`
- otherwise, `public_status = wants_attention`

### Degraded mapping

When `internal_status = degraded`:

- `health.state` must be `degraded`
- `public_status` should remain semantically accurate to current work if possible
- if degraded state materially demands owner awareness, `public_status` may be elevated to `needs_attention`

### Unresponsive mapping

When `internal_status = unresponsive`:

- `health.state` must be `unresponsive`
- `public_status` should remain the last known authoritative normalized presence until transport or runtime rules force an `offline` transition
- PET may apply a local transport overlay independently

---

## Visibility rules

### User visibility

User presence may contain both public and private/internal fields.

- `public_status` is safe for normal presentation.
- `private_status` is visible to PET and NAVI only.
- third-party or future shared surfaces must not expose private fields unless explicitly authorized.

### NAVI visibility

NAVI presence may include both public and internal fields.

- `public_status` is intended for PET presentation.
- `internal_status`, `current_detail`, and detailed health fields may be shown in PET owner-facing UI.
- external surfaces must not assume all NAVI fields are public.

---

## Combined snapshot model

REST snapshot surfaces may return a combined view:

```json
{
  "version": "v1",
  "snapshot_revision": 42,
  "generated_at": "2026-04-16T18:00:00Z",
  "user": { "...PresenceEnvelope": true },
  "navi": { "...PresenceEnvelope": true },
  "transport": {
    "state": "connected",
    "last_ws_message_at": "2026-04-16T18:00:00Z",
    "stale_after_ms": 45000
  }
}
```

### Snapshot rules

- Snapshot revision is scoped to the snapshot surface, not to an individual subject.
- Subject-level reconciliation must still use each envelope's `state_revision`.
- Combined snapshot is a transport convenience, not a replacement for per-subject authority.

---

## REST surfaces

### Required NAVI-owned surfaces

#### `GET /api/presence/navi`

Returns the current NAVI authoritative `PresenceEnvelope` for subject type `navi`.

#### `GET /api/presence`

Returns the current combined presence snapshot, including:

- latest user presence visible to NAVI
- current NAVI presence
- transport metadata if available

### REST behavior rules

REST is the source for:

- bootstrap
- fallback polling
- reconnect hydration
- resync after event loss

REST must return the same canonical structures used by WebSocket events.

REST must not invent alternate field names or alternate enums.

---

## WebSocket surfaces

### Channel purpose

WebSocket is the primary realtime transport for presence updates.

It is used for:

- edge-triggered presence changes
- attention changes
- transport lifecycle notifications
- resync requests

It is not the sole source of truth. REST remains authoritative for bootstrap and resync.

### Required event types

#### `presence.snapshot`

A full combined snapshot sent immediately after subscription or resubscription.

#### `presence.user.updated`

Carries a user `PresenceEnvelope`.

#### `presence.navi.updated`

Carries a NAVI `PresenceEnvelope`.

#### `presence.navi.attention`

Carries a NAVI `PresenceEnvelope` when attention state changes materially.

#### `presence.transport.state`

Emits transport lifecycle state such as connected, reconnecting, stale, or offline.

#### `presence.resync_required`

Signals that the consumer must discard incremental assumptions and rehydrate via REST.

### Event envelope

```json
{
  "type": "presence.navi.updated",
  "sent_at": "2026-04-16T18:00:00Z",
  "data": {}
}
```

### Event rules

- Every event must include `type`, `sent_at`, and `data`.
- Subject updates must carry canonical `PresenceEnvelope` objects.
- Consumers must be able to reconcile events using `state_revision`.
- Providers may coalesce multiple rapid runtime transitions into the newest revision.
- Providers must send `presence.snapshot` after successful subscription.

---

## Transport and staleness rules

### Transport state enum

- `connected`
- `reconnecting`
- `stale`
- `offline`

### Staleness rules

PET may mark transport stale when:

- no WebSocket message has been received within the applicable `stale_after_ms` window
- and no newer REST snapshot has been obtained

### Offline overlay rules

PET may apply an offline overlay when:

- WebSocket is disconnected beyond reconnect tolerance, or
- REST snapshot refresh fails beyond retry tolerance

### Critical constraint

A PET transport overlay does **not** mutate NAVI authoritative presence.

It is a rendering concern, not a state ownership change.

---

## Conflict handling

### Out-of-order updates

Consumers must ignore any envelope with an older `state_revision` for the same subject.

### Equal revisions

If revision numbers are equal:

- prefer the envelope with the newer `state_updated_at`
- if timestamps are equal, prefer the latest `transport_observed_at`

### Authority mismatch

If an envelope declares an authority that does not match the subject domain, consumers must reject it.

Examples:

- `subject_type = navi` and `authority = pet` -> invalid
- `subject_type = user` and `authority = navi` -> invalid

---

## Error handling

### Invalid payload

If a payload fails schema validation:

- reject it
- record diagnostics
- request resync if stream continuity is uncertain

### Resync required

When a consumer detects unrecoverable drift or a provider knows the stream is no longer trustworthy, emit or act on `presence.resync_required` and rehydrate via REST.

---

## Security and governance notes

- Presence is state, not informal UI decoration.
- Presence authorities must be enforced by implementation boundaries.
- Private fields must not be exposed outside authorized owner-facing surfaces.
- Future plugin or connector integrations must observe this authority model and must not bypass it.

---

## Non-goals for v1

This spec does not define:

- external third-party presence federation
- multi-owner or multi-tenant shared presence semantics
- notification UX behavior
- UI animation or visual treatment
- arbitrary presence writes by plugins or connectors

---

## Implementation guidance

- Treat this document as the contract source.
- Keep operator/debug endpoints distinct from product presence endpoints.
- Prefer a dedicated presence adapter/service on the NAVI side instead of exposing raw runtime structs directly.
- Preserve backward compatibility by adding fields, not renaming or repurposing existing fields.

---

## Versioning

Breaking changes require a new protocol version.

Examples of breaking changes:

- renaming enums
- changing authority rules
- removing required fields
- changing event names

Non-breaking changes include:

- adding optional fields
- adding new private status values
- adding new attention reason codes
- adding new transport metadata fields
