---
name: Presence Runtime Implementation Plan v1
overview: Implement and stabilize the NAVI-side presence subsystem in narrow phases so runtime truth, presence composition, and transport exposure do not drift.
todos:
  - id: add-presence-package
    content: Create internal/presence with base types, normalization, and revisioned service composed from runtime activity snapshots.
    status: completed
  - id: stabilize-presence-service
    content: Make DefaultService nil-safe, revision-safe, and conservative about heartbeat/dreaming classification.
    status: completed
  - id: expose-navi-presence-methods
    content: Add NAVI-facing presence accessors so gateway and other callers can consume PresenceService without duplicating normalization logic.
    status: completed
  - id: add-presence-http-surface
    content: Add GET /api/presence/navi plus related snapshot/update routes while keeping /api/agent/status unchanged.
    status: completed
  - id: add-work-classification-source
    content: Introduce WorkClassificationSource and a conservative default implementation that under-classifies rather than overclaims busy/dreaming.
    status: completed
  - id: add-attention-source
    content: Introduce ProposalAttentionSource and map proposal blocking/non-blocking state into wants_attention vs needs_attention.
    status: completed
  - id: split-presence-revisions
    content: Split authoritative NAVI revision from combined snapshot revision so PET/user updates do not mutate NAVI state_revision.
    status: completed
  - id: align-live-presence-events
    content: Align websocket presence events with the canonical interface event model and remove branch-local transport drift.
    status: partial
  - id: add-dreaming-classification-hook
    content: Add DreamingStateSource and initial adapter points for already-existing background processing once introspection can classify them.
    status: partial
  - id: remediation-pass
    content: Run a cleanup pass after agentic-tool-system stabilizes; tighten the remaining live/session naming drift and extract gateway presence handlers from server.go if desired.
    status: pending
isProject: false
---

# Presence Runtime Implementation Plan v1

## Goal

Implement and stabilize the NAVI-side presence subsystem in a way that preserves current runtime behavior while creating a clean, authoritative seam for:

- PET-facing presence reporting
- deterministic status selection
- future `working` vs `busy` refinement
- future dreaming introspection
- attention-aware public state
- stable REST and WebSocket transport surfaces

The plan intentionally moves in narrow slices. The goal is to keep one authoritative presence seam rather than letting runtime, gateway, and PET each invent their own presence logic.

---

## Branch scope

This work belongs on:

- `feat/agentic-tool-system`

Do not continue this presence effort on `master` for this branch line. If presence-related commits exist elsewhere, treat them as source material only and port intentionally after review.

---

## Current repo snapshot

Today the repo already contains:

### Runtime truth

- `StatusTracker` as low-level runtime truth
- `NAVI.Status()` returning `schema.AgentStatusSnapshot`
- `GET /api/agent/status` as an operator/runtime endpoint

### Presence subsystem foundation

- `internal/presence/types.go`
- `internal/presence/normalize.go`
- `internal/presence/service.go`
- `internal/presence/attention.go`
- `internal/presence/dreaming.go`
- `internal/navi/presence.go`

### Presence HTTP surfaces

- `GET /api/presence/navi`
- `GET /api/presence`
- `POST /api/presence/user`
- `GET /api/presence/heartbeat`

### Current correctness already in place

- heartbeat is not surfaced as `dreaming` by default
- `tool_executing` defaults to `working`, not `busy`
- read-only presence snapshots do not advance revision unless payload changes materially
- NAVI now explicitly owns one stable presence service per instance
- authoritative NAVI revision is separate from combined snapshot revision
- gateway consumes NAVI presence rather than rebuilding normalization in handlers
- a no-op dreaming seam now exists explicitly in the presence subsystem

This means the subsystem foundation is already present. The remaining work is now mostly alignment, refinement, and cleanup rather than greenfield creation.

---

## Phase 1 — Base presence seam

### Objective

Create a dedicated presence package and base service that composes presence from runtime truth.

### Deliverables

- `internal/presence/types.go` — completed
- `internal/presence/normalize.go` — completed
- `internal/presence/service.go` — completed
- `internal/navi/presence.go` — completed

### What this phase established

- a revisioned presence service
- conservative default normalization
- NAVI-owned accessors for presence reads
- a place to observe PET-owned user presence

### Acceptance criteria

- NAVI can produce a presence snapshot and envelope without duplicating logic in gateway handlers
- repeated reads do not increment revision unnecessarily
- `StatusTracker` remains unchanged and authoritative for low-level activity truth

---

## Phase 2 — NAVI accessor wiring

### Objective

Give the NAVI runtime a stable accessor for the presence subsystem.

### Deliverables

- `PresenceService()` — completed
- `PresenceEnvelope(ctx)` — completed
- `PresenceSnapshot(ctx)` — completed
- `ObserveUserPresence(ctx, envelope)` — completed

### Current implementation note

The runtime now owns the presence service explicitly through `NAVI` rather than the earlier branch-local cache indirection. The accessor methods remain the stable read/write seam used by gateway and other callers.

---

## Phase 3 — HTTP exposure

### Objective

Expose canonical presence routes without replacing runtime/operator status.

### Deliverables

- `GET /api/presence/navi` — completed
- `GET /api/presence` — completed as combined snapshot surface
- `POST /api/presence/user` — completed for PET-owned user presence observation
- `GET /api/presence/heartbeat` — completed as user-presence observation heartbeat

### Rules preserved

- `/api/agent/status` remains unchanged
- presence handlers do not rebuild normalization logic
- gateway resolves presence through NAVI accessors first

### Current implementation note

`gateway.Config.Presence` still exists as a fallback seam for tests or alternate server construction, but the main runtime path flows through `cfg.Navi.PresenceService()`.

---

## Phase 4 — Work classification seam

### Objective

Introduce the seam that keeps `working` and `busy` meaningful.

### Deliverables

- `WorkClassificationSource` — completed
- conservative default implementation — completed
- integration into `PresenceService` — completed

### Current default stance

The current implementation remains deliberately conservative:

- under-classify by default
- prefer `working` over `busy` when interruption/resume/focus semantics are unknown
- avoid surfacing `busy` unless there is real evidence for it

### Rule preserved

`busy` is never inferred purely from `tool_executing`.

---

## Phase 5 — Attention integration

### Objective

Map proposal blocking state into canonical attention states.

### Deliverables

- database-backed attention source — completed
- blocking vs non-blocking proposal summary integration — completed
- `needs_attention` / `wants_attention` mapping — completed

### Current implementation note

The attention source is intentionally narrow:

- blocking pending proposal -> `needs_attention`
- queued/pending non-blocking proposal -> `wants_attention`
- no pending proposal -> `none`

It does not leak full proposal records into presence and does not yet scope attention perfectly to the active session/run.

### Follow-up question left for remediation

Decide whether attention should remain global-ish or become more tightly correlated to the active session/run once runtime correlation is stronger.

---

## Phase 6 — Revision ownership cleanup

### Objective

Ensure subject ownership remains clean even when the service observes PET-owned user presence.

### Deliverables

- separate authoritative NAVI revision — completed
- separate combined snapshot revision — completed
- prevent PET/user updates from mutating NAVI `state_revision` — completed

### Rule preserved

NAVI owns NAVI presence. PET/user updates may change the combined snapshot, but they must not silently advance NAVI’s authoritative revision.

---

## Phase 7 — Live event alignment

### Objective

Expose presence changes over the live websocket without producing transport drift.

### Current state

Websocket presence frames now use the canonical `type` / `sent_at` / `data` envelope shape, and NAVI updates are keyed from NAVI revision rather than the mixed snapshot revision.

### Remaining work

- review whether additional canonical events such as `presence.resync_required` should be emitted explicitly
- remove any remaining branch-local shape differences from the live transport path
- decide whether transport state should be emitted only on connect/reconnect or whenever state changes materially

### Event rule

Emit only when the relevant composed revision changes.

### Done means

- WebSocket presence updates reflect the same logical contract as REST
- repeated polling or repeated reads do not generate duplicate event spam
- remaining drift is naming/surface cleanup rather than ownership/revision confusion

---

## Phase 8 — Dreaming classification hook

### Objective

Add the explicit classification seam that will allow already-existing background processing to be surfaced as `dreaming` when introspection is trustworthy.

### Current state

The no-op seam is now present:

- `DreamingSource`
- `DreamingSummary`
- `NoOpDreamingSource`
- service-level dreaming override when a real source reports active dreaming

### Remaining deliverables

- a real runtime-backed dreaming source
- integration with actual background-processing/dreaming signals
- any follow-up eventing needed once dreaming can transition materially at runtime

### Rule preserved

Do not surface public `dreaming` solely because background processing exists.

Only surface `dreaming` once the runtime can classify that work intentionally.

---

## Remediation pass

After `feat/agentic-tool-system` stabilizes, run a cleanup pass on these questions:

1. Should `PresenceEnvelope.Payload any` remain flexible or be narrowed further?
2. Should gateway presence handlers move out of `server.go` into a dedicated `internal/gateway/presence.go` file?
3. Should attention scoping become more tightly tied to active session/run correlation?
4. Should transport-state emission be made fully event-driven instead of poll-observed?
5. Should the gateway fallback to `cfg.Presence` remain, or should runtime construction always flow through `cfg.Navi`?

---

## Guardrails

Do not:

- replace `StatusTracker`
- collapse runtime status and presence into one struct
- define `busy` from raw activity enums alone
- surface `dreaming` without a real classification seam
- move normalization logic into gateway handlers
- instantiate a new presence service per request
- let PET invent NAVI presence semantics locally

---

## What is intentionally deferred

- PET-side adapter work
- WebSocket transport overlay logic
- multi-agent/shared presence
- full dreaming subsystem design

---

## Recommended next implementation order

1. decide whether to finish the remaining live transport cleanup now or defer it
2. implement a real runtime-backed dreaming source when background-processing introspection is ready
3. run remediation cleanup after branch churn drops

That order keeps the subsystem honest without reopening already-stable foundations.
