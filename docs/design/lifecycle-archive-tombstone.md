**Status:** Active  
**Last Updated:** 2026-03-14

# Lifecycle: Archive and Tombstone

Data lifecycle for world-model entities: **Delete defaults to Archive** (soft-delete); **Tombstone** (hard-delete) requires a proposal and owner approval.

---

## Contract

| Action | Meaning | Default / Gate |
|--------|---------|-----------------|
| **Archive** | Soft-delete: record lifecycle action, auto-decline pending proposals affecting the entity. Entity row remains; list/query logic may exclude archived entities (see implementation notes). | **Default for Delete.** Any delete path that removes or hides a world-model entity should use Archive unless an explicit hard-delete is requested. |
| **Tombstone** | Hard-delete: record lifecycle action, auto-decline proposals, then remove the row from the store. | **Requires a Proposal.** Callers must run `ValidateAction` with `ActionDescriptorForLifecycleTombstone`; when outcome is `RequiresConfirmation`, create a proposal and only call `TombstoneEntity` after owner approval. |

---

## Universal archive path

**Archive by default** applies to world-model entities: memory, fact, contact, artifact, wm_event. All delete paths for these entities must go through lifecycle: use `ArchiveEntity` for soft-delete unless Tombstone is explicitly requested and approved via proposal.

**Documented exceptions** (no Archive/Tombstone): **Directive messages** (`DeleteDirectiveMessage` — transcript); **Relationships** (`DeleteRelationship`); **Execution outcomes** (`DeleteExecutionOutcomesOlderThan` — operational purge); **Reset/internal** where documented.

**Audit:** The only code paths that delete rows from `memories`, `facts`, `contacts`, `artifacts`, or `wm_events` are inside `store.TombstoneEntity`. Compensation for failed executions uses `ArchiveEntity`. All world-model entity removal therefore goes through lifecycle.

---

## Implementation

- **Store:** `store.ArchiveEntity(ctx, db, entityType, entityID)` inserts a row into `tombstones` with `action = 'archive'` and calls `AutoDeclineProposalsForEntity`. It does not remove or hide the source row; list operations do not yet filter by archive (future: exclude entity IDs present in `tombstones` with `action = 'archive'`).
- **Store:** `store.TombstoneEntity(ctx, db, entityType, entityID, proposalID)` inserts a tombstone row with `action = 'tombstone'`, optional `proposal_id` (for audit when executed from an approved proposal), auto-declines proposals, then deletes the row from the corresponding table (memory, fact, contact, artifact, wm_event).
- **WorldModel:** `ArchiveEntity` and `TombstoneEntity` are exposed on the WorldModel façade; only Cognitive-layer callers (orchestrator, NAVI, reflection) should perform these writes.
- **Governor:** `ActionDescriptorForLifecycleTombstone(actorKind, entityType, entityID)`; `ValidateAction` returns `RequiresConfirmation` for any Tombstone, so the proposal flow is mandatory before hard-delete.

---

## Tombstone full flow (owner-initiated hard-delete)

1. Owner (or gateway/NAVI on behalf of owner) requests hard-delete of an entity.
2. Call `ValidateAction(pe, ActionDescriptorForLifecycleTombstone("gateway", entityType, entityID))` → outcome is `RequiresConfirmation`.
3. Create a proposal (e.g. `source_trigger = "hard_delete_requested"`, `proposed_action = "tombstone"`, `affected_entities` = JSON array of `"entityType:entityID"`).
4. When owner approves via `POST /api/proposals/{id}/resolve` with action `approve`, the gateway calls `ExecuteApprovedProposal`. For `proposed_action = "tombstone"`, NAVI calls the configured `ResolveProposal(ctx, proposalID, Approved, …)` without executing a skill; the host's `ResolveProposal` callback then runs `WorldModel.TombstoneEntity(ctx, entityType, entityID, proposalID)` for each entry in `affected_entities`.
5. Tombstone record is written with `proposal_id` set (for audit); the row is removed; pending proposals for that entity are auto-declined.

**Implementation:** `internal/navi/navi.go` — `ExecuteApprovedProposal` treats `proposed_action == "tombstone"` as a lifecycle action and delegates to `ResolveProposal(Approved)` so the host executes tombstone. `cmd/navid/main.go`: `ResolveProposal` callback executes tombstone on approval and passes the proposal ID into `TombstoneEntity`.

---

## Exceptions

- **Directive messages:** `DeleteDirectiveMessage` is a direct delete (no Archive). Directive messages are conversation transcript, not long-lived world-model entities; direct delete is acceptable.
- **Forget (memory):** `ForgetMemory` is the dedicated lifecycle for memories (design deletion path); it records in tombstones and flags derivatives for re-evaluation; it is not Archive.
- **Knowledge deprecate:** Facts use a `deprecated` flag (soft-delete without replacement); see ENT-K. Supersede and Tombstone remain separate.

---

## Relevant files

- `internal/store/lifecycle.go` — ArchiveEntity, TombstoneEntity, ForgetMemory, AutoDeclineProposalsForEntity
- `internal/worldmodel/worldmodel.go` — ArchiveEntity, TombstoneEntity
- `internal/governor/validate.go` — ActionDescriptorForLifecycleTombstone, riskCheck Tombstone gating
