# Presence Runtime Implementation Plan

**Status:** Active  
**Last Updated:** 2026-04-16  
**Updated By:** agent

## Objective

Implement the NAVI-side presence subsystem in a way that:

- preserves current runtime status behavior
- adds a dedicated presence service and public presence contract
- keeps `working` vs `busy` deterministic
- creates a clean seam for future `dreaming` introspection
- avoids broad rewrites or drift during rollout

This plan implements the specs:

- `docs/specs/pet-presence-interface-v1.md`
- `docs/specs/presence-runtime-framework-v1.md`
- `docs/specs/presence-status-decision-policy-v1.md`

---

## Implementation strategy

Start in **NAVI**, not PET.

Reason:

- NAVI owns NAVI presence authority
- PET cannot integrate cleanly until the NAVI presence surfaces exist
- the biggest drift risk is backend semantics, not frontend rendering

Rollout rule:

- build new presence functionality **alongside** existing status/runtime routes
- do not break `/api/agent/status`
- do not replace `StatusTracker`
- layer presence on top first

---

## Success criteria

### Minimum viable success

- `internal/presence` package exists
- `PresenceService` composes presence from `StatusTracker`
- `GET /api/presence/navi` returns canonical NAVI presence
- `GET /api/presence` returns combined presence snapshot shape
- presence revisions are stable and monotonic
- no current operator route is broken

### Phase-complete success

- work classification source exists
- `working` vs `busy` is determined by documented rules
- WebSocket emits structured presence events
- attention integration exists
- dreaming hook interface exists even if full dreaming subsystem does not

---

## Non-goals for first implementation slice

Do not attempt in the first slice:

- full dreaming engine
- broad connector-health orchestration redesign
- multi-agent presence federation
- PET UI rewrite in the same change set
- removal of `/api/agent/status`

---

## Recommended rollout phases

## Phase 0 — Scaffolding and contracts in code

### Goal

Create the new package and type boundaries without changing runtime behavior yet.

### Files to add

Recommended new files:

- `internal/presence/types.go`
- `internal/presence/service.go`
- `internal/presence/policy.go`
- `internal/presence/events.go`
- `internal/presence/normalize.go`

### Concrete tasks

1. Add canonical presence-facing internal types:
   - `PresenceState`
   - `PresenceAttention`
   - `PresenceHealth`
   - `WorkClassificationSummary`
   - `PresenceDecisionInputs`
   - `PresenceDecisionOutput`

2. Add interfaces:
   - `PresenceService`
   - `WorkClassificationSource`
   - `ProposalAttentionSource`
   - `PresenceHealthSource`
   - `DreamingStateSource`
   - `PresenceDecisionPolicy`

3. Add a default decision policy implementation that follows the current docs.

### Acceptance

- code compiles
- package is present
- no runtime wiring yet

---

## Phase 1 — Presence service from StatusTracker only

### Goal

Create a working presence service backed only by current runtime truth.

### Concrete tasks

1. Implement `DefaultPresenceService`.
2. Feed it with:
   - `StatusTracker`
   - default no-op attention source
   - default no-op health source beyond tracker-derived health
   - default no-op dreaming source
   - default no-op work classification source

3. Implement initial normalization:
   - `idle -> idle`
   - `processing -> active`
   - `tool_executing -> working`
   - `waiting_for_input -> wants_attention` or `idle` fallback depending on current source availability
   - `heartbeat -> active` or `heartbeat`-internal only
   - `offline -> offline`
   - `unresponsive -> internal unresponsive`

4. Add material-change detection and revision bump logic.

### Acceptance

- service returns stable snapshots
- revisions only change when presence materially changes
- existing runtime behavior is unaffected

---

## Phase 2 — Gateway route integration

### Goal

Expose the new presence surfaces without removing existing ones.

### Routes to add

- `GET /api/presence/navi`
- `GET /api/presence`

### Concrete tasks

1. Add handler DTOs matching the presence spec.
2. Wire gateway to the new service.
3. Keep `/api/agent/status` intact and operator-oriented.
4. Update gateway API docs after route addition.

### Acceptance

- new routes work
- old routes still work
- schemas match the written spec

---

## Phase 3 — Work classification integration

### Goal

Make `working` vs `busy` real instead of heuristic sludge.

### Concrete tasks

1. Implement a default `WorkClassificationSource`.
2. Add the following fields to classification output:
   - `Interruptibility`
   - `InterruptionCost`
   - `ResumeCost`
   - `FocusLock`

3. Use conservative defaults:
   - prefer `working` over `busy` when uncertain
   - never infer `busy` from `tool_executing` alone

4. Add initial classification rules for examples such as:
   - deep research / synthesis -> likely `busy`
   - image generation / generation phase -> likely `busy`
   - redirectable search, lookup, repository inspection, coding flow -> likely `working`

### Acceptance

- public `busy` only appears when justified by cost/focus semantics
- `working` vs `busy` behavior matches the decision policy doc

---

## Phase 4 — WebSocket presence events

### Goal

Make presence available over the existing live socket as structured events.

### Event types

- `presence.snapshot`
- `presence.navi.updated`
- `presence.navi.attention`
- `presence.transport.state`
- `presence.resync.required`

### Concrete tasks

1. Add presence event DTOs.
2. Add event fanout from `PresenceService` revision changes.
3. Emit snapshot on subscribe/connect.
4. Do not emit on every read or touch.

### Acceptance

- events emit only on material change
- reconnection gets a full snapshot

---

## Phase 5 — Attention integration

### Goal

Make proposal state part of presence.

### Concrete tasks

1. Add `ProposalAttentionSource` implementation.
2. Define blocking vs non-blocking proposal summary rules.
3. Map:
   - blocking -> `needs_attention`
   - non-blocking -> `wants_attention`

### Acceptance

- blocking proposal state outranks work-display state
- attention transitions bump revisions correctly

---

## Phase 6 — Dreaming seam

### Goal

Support formal dreaming classification without pretending it is already fully introspected.

### Concrete tasks

1. Add `DreamingStateSource` implementation seam.
2. Add an initial adapter that can classify existing background-processing signals when identifiable.
3. Keep this conservative:
   - do not mark public `dreaming` unless the seam has an actual source of truth

### Acceptance

- dreaming hook exists
- no fake dreaming status is emitted
- subsystem is ready for future introspection work

---

## File touch map

### New package

- `internal/presence/*`

### Likely existing touch points

- `internal/gateway/server.go`
- `internal/navi/status.go` (only if needed for wiring, not broad redesign)
- runtime bootstrap / dependency wiring location(s)
- any live WebSocket fanout path currently serving `/ws/live`

### Docs to update after implementation

- `docs/specs/gateway-api.md`
- `docs/specs/presence-runtime-framework-v1.md` if code-backed refinements are discovered
- `docs/tasks/` only if blockers or follow-up debt emerge

---

## Guardrails

1. Do not turn `StatusTracker` into the full presence subsystem.
2. Do not let handlers compose presence ad hoc.
3. Do not infer `busy` from raw activity alone.
4. Do not expose public `dreaming` before a real classification source exists.
5. Do not remove `/api/agent/status` during initial rollout.
6. Do not bundle PET rewiring into the same change set unless Phase 1/2 is already stable.

---

## Suggested first code slice

The best first implementation slice is:

- add `internal/presence`
- add `DefaultPresenceService`
- wire `/api/presence/navi`
- leave PET untouched

That gives a small blast radius and a real surface to validate before the rest of the stack moves.

---

## After NAVI Phase 1/2

Once NAVI presence routes are stable, move to PET integration:

- replace connection-only hook
- add presence adapter/store
- consume REST snapshot + WebSocket events
- wire dual-profile button to real presence

That work belongs in the PET repo plan, not as a hidden dependency in this one.
