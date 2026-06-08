# NAVI Presence Runtime Framework v1

**Status:** Active  
**Last Updated:** 2026-04-18  
**Updated By:** agent

## Purpose

This document defines the **NAVI-side presence subsystem**.

It explains:

- what presence is and is not
- how presence relates to activity, work classification, attention, and health
- what the current implementation already provides
- what remains intentionally deferred

This document is the NAVI-internal companion to:

- `docs/specs/pet-presence-interface-v1.md`
- `docs/specs/presence-status-decision-policy-v1.md`

The interface spec defines the transport contract. This framework defines how NAVI should produce and own that contract internally.

---

## Subsystem overview

Presence is a **NAVI-owned subsystem**, not a UI convenience.

Its job is to turn low-level runtime truth into a stable, externally consumable state model that other products and services can read and subscribe to.

In practical terms:

- `StatusTracker` remains the low-level runtime truth
- presence sits above it as a synthesized, revisioned read model
- PET and other surfaces read presence
- NAVI owns presence writes for NAVI state
- PET owns user presence and NAVI observes it

Presence is therefore best understood as a **first-class subsystem within NAVI**, not as a separate standalone product or deployable service.

---

## Terminology

These terms must stay distinct.

### Activity

Low-level runtime truth.

Examples:

- `processing`
- `tool_executing`
- `waiting_for_input`
- `heartbeat`

Activity answers: **what is the runtime doing right now?**

### Work classification

Higher-level meaning of active work.

Examples:

- conversational
- execution
- background
- low interruption cost vs high interruption cost

Work classification answers: **what kind of work is this, and how interruptible is it?**

### Presence

Externally exposed state model used by PET and other consumers.

Examples:

- `active`
- `working`
- `busy`
- `dreaming`
- `needs_attention`

Presence answers: **what state should NAVI present outward right now?**

### Attention

Owner-relevant demand for input or awareness.

Examples:

- `none`
- `wants_attention`
- `needs_attention`

Attention answers: **does the owner need to know or act?**

### Health

Runtime viability and degradation state.

Examples:

- `healthy`
- `degraded`
- `unresponsive`
- `offline`

Health answers: **is NAVI functioning normally at all?**

### Internal mode

NAVI-internal synthesized mode used to describe the nature of current behavior.

Examples:

- `interactive`
- `working`
- `waiting`
- `heartbeat`
- `dreaming`

Internal mode answers: **what broad operating mode is NAVI in?**

---

## Problem this subsystem solves

The current `StatusTracker` is useful but too narrow by itself.

It provides:

- a single current activity state
- timestamps
- active session ID
- one detail string
- staleness tracking

That is enough for `/api/agent/status`, but it is not enough for:

- PET/NAVI presence synchronization
- clean public presence selection
- attention-aware status
- richer health/degradation reporting
- future internal modes like `dreaming`
- a clean `working` vs `busy` distinction

Without a dedicated subsystem, those concerns drift into handlers, UI code, or ad hoc string fields.

---

## Goals

1. Preserve current status functionality without breaking operator/runtime surfaces.
2. Keep presence deterministic and code-owned.
3. Separate low-level runtime truth from externally exposed presence.
4. Support future states like `dreaming` cleanly.
5. Support `working` vs `busy` without abusing raw activity enums.
6. Expose stable REST and WebSocket presence surfaces.
7. Keep ownership boundaries explicit between NAVI-owned and PET-owned state.
8. Allow later evolution without redefining the model.

---

## Non-goals

This framework does **not**:

- make presence LLM-owned
- allow PET to author NAVI presence
- allow plugins/connectors to mutate presence directly
- treat presence as merely a tooltip or UI badge
- force presence into a separate deployable microservice

Presence is runtime-governed system state inside NAVI.

---

## Architectural model

NAVI should keep **three distinct layers**.

### 1. Activity tracking layer

Low-level runtime truth:

- current activity state
- state start time
- active session
- detail
- last activity
- staleness

This is the `StatusTracker` layer and should remain close to runtime execution.

### 2. Work classification layer

A higher-level summary that determines what active work means.

This layer exists because activity alone cannot reliably distinguish:

- `active` vs `working`
- `working` vs `busy`
- foreground execution vs background maintenance
- pivot-friendly work vs high-resume-cost work

### 3. Presence composition layer

The synthesized subsystem that produces:

- public presence
- internal mode
- attention summary
- health summary
- revisioned envelopes and snapshots
- transport-facing event payloads

This is the actual presence subsystem.

---

## Ownership model

### NAVI owns

- NAVI public presence
- NAVI internal mode
- NAVI attention state
- NAVI health state
- NAVI revision and envelope generation

### PET owns

- user public presence
- user private/internal presence
- user-authored status text/subtext

### NAVI may observe

- PET-owned user presence

### PET may derive locally

- offline/stale transport overlays when NAVI is unreachable

PET must not author NAVI presence. NAVI must not author PET user presence.

---

## Current implementation status

The repo already contains a real presence subsystem foundation.

### Present now

- `internal/presence/types.go`
- `internal/presence/normalize.go`
- `internal/presence/service.go`
- `internal/presence/attention.go`
- `internal/navi/presence.go`
- gateway routes for:
  - `GET /api/presence/navi`
  - `GET /api/presence`
  - `POST /api/presence/user`
  - `GET /api/presence/heartbeat`

### Also true now

- presence reads do not advance revision unless material state changes
- default classification is conservative
- heartbeat is **not** surfaced as dreaming by default
- `tool_executing` defaults to `working`, not `busy`
- NAVI accessors cache one stable presence service per `*NAVI` instance

### Still intentionally incomplete

- explicit dreaming classification seam
- richer work-classification semantics beyond conservative defaults
- canonical WebSocket event envelope alignment
- PET transport overlay integration
- multi-agent/shared presence

---

## Subsystem layout

Current/recommended layout:

- `internal/presence/types.go`
- `internal/presence/service.go`
- `internal/presence/normalize.go`
- `internal/presence/attention.go`
- `internal/presence/policy.go` (recommended next home for richer decision logic)
- `internal/presence/events.go` (recommended once WS event model is finalized)
- `internal/presence/ws.go` (optional)

NAVI-facing accessors live in:

- `internal/navi/presence.go`

Gateway consumption should stay thin and consume NAVI presence accessors rather than rebuilding presence itself.

---

## Core data model

### PresenceState

Recommended internal composed state:

```go
type PresenceState struct {
    Revision         int64
    StateUpdatedAt   time.Time

    PublicStatus     string
    InternalMode     string
    InternalActivity schema.AgentActivityState

    StatusText       string
    Subtext          string

    ActiveSessionID  string
    CurrentDetail    string

    Attention        PresenceAttention
    Health           PresenceHealth
}
```

### PresenceAttention

```go
type PresenceAttention struct {
    Level      string // none | wants_attention | needs_attention
    ReasonCode string
    ProposalID string
    Blocking   bool
}
```

### PresenceHealth

```go
type PresenceHealth struct {
    State          string // healthy | degraded | unresponsive | offline
    LastActivityAt time.Time
    StaleAfterMs   int64
}
```

### WorkClassificationSummary

```go
type WorkClassificationSummary struct {
    HasActiveWork      bool
    WorkType           string // conversational | execution | background | maintenance
    Interruptibility   string // interruptible | queue_only | non_interruptible
    InterruptionCost   string // low | medium | high
    ResumeCost         string // low | medium | high
    FocusLock          string // none | soft | hard
    ReasonCode         string
    StatusText         string
    Subtext            string
}
```

---

## Service responsibilities

### PresenceService

The subsystem owns a service with responsibilities like:

```go
type PresenceService interface {
    Snapshot(ctx context.Context) PresenceSnapshot
    NaviPresence(ctx context.Context) PresenceEnvelope
    UpdateUserPresence(ctx context.Context, envelope PresenceEnvelope) error
    Revision() int64
}
```

Implementation details can vary, but the service must always:

1. read low-level runtime truth
2. compose public presence and internal mode deterministically
3. maintain stable revision semantics
4. expose canonical read surfaces
5. avoid side effects on ordinary reads

---

## Inputs to presence composition

Presence composition draws from:

### 1. StatusTracker

Primary runtime activity source.

Relevant fields include:

- `State`
- `Since`
- `LastActivityAt`
- `ActiveSessionID`
- `CurrentDetail`
- `UpdatedAt`

### 2. Work classification summary

Used to distinguish:

- `active`
- `working`
- `busy`

This is required because raw activity is not enough.

### 3. Proposal / attention summary

Used to determine:

- `needs_attention`
- `wants_attention`
- `none`

This must stay a narrow summary projection, not a dump of full proposal state.

### 4. Health summary

Used to determine:

- `healthy`
- `degraded`
- `unresponsive`
- `offline`

### 5. Dreaming classification source

Future seam for surfacing existing background processing as true `dreaming` once introspection becomes trustworthy.

---

## Normalization rules

Normalization is governed by:

- `docs/specs/presence-status-decision-policy-v1.md`

This framework owns the subsystem shape. The decision policy owns the precedence and meaning.

### Current conservative defaults

Current implementation behavior should remain conservative:

- `processing` -> `active`
- `tool_executing` -> `working`
- `waiting_for_input` -> attention-aware waiting state
- `heartbeat` -> `heartbeat` internally, not public `dreaming`
- no public `busy` without real work-classification support
- no public `dreaming` without real dreaming-classification support

### Internal mode vs public status

Keep these separate.

Examples:

- internal mode may remain `working` while public status becomes `busy`
- health may be `degraded` while public status remains `working`
- attention may override work-display states publicly while internal mode remains `working` or `waiting`

---

## Dreaming framework rule

Dreaming is a first-class **future** internal mode and public status.

Important rule:

- background processing may already exist behaviorally
- that behavior should be treated as the foundation of dreaming
- but it must not be surfaced publicly as `dreaming` until NAVI has a clean introspection/classification seam for it

Do **not** hardwire heartbeat to dreaming.

---

## Gateway surfaces

### Current routes

The repo currently exposes:

- `GET /api/agent/status` — operator/runtime snapshot
- `GET /api/presence/navi` — canonical NAVI presence envelope
- `GET /api/presence` — combined snapshot
- `POST /api/presence/user` — PET-owned user presence observation/update path
- `GET /api/presence/heartbeat` — user presence observation heartbeat

### Surface rule

`/api/agent/status` must remain distinct from presence.

- `/api/agent/status` = low-level runtime truth
- `/api/presence*` = presence subsystem read model

Do not collapse them.

---

## WebSocket direction

Presence should be emitted as dedicated event types, not ad hoc strings.

Target event family:

- `presence.snapshot`
- `presence.navi.updated`
- `presence.navi.attention`
- `presence.transport.state`
- `presence.resync.required`

Current branch-local websocket work is partial. It should eventually align to the canonical presence event model rather than remaining transport-specific.

---

## Change triggers

Presence recomputation should happen on:

### Runtime lifecycle triggers

- turn start
- tool execution start
- tool execution end
- turn complete
- heartbeat start/end if used

### Work classification triggers

- interruptibility changes
- interruption-cost changes
- resume-cost changes
- focus-lock changes
- work type changes materially

### Proposal triggers

- proposal created
- proposal resolved
- blocking proposal enters active path
- blocking proposal cleared

### Health triggers

- subsystem degradation detected
- subsystem recovered
- staleness threshold crossed

### Dreaming triggers

Future:

- dreaming entered
- dreaming exited
- dreaming detail changed
- existing background processing becomes classifiable as dreaming

---

## State transition rules

1. Runtime code does not write public presence directly.
2. Presence composition remains centralized in the presence subsystem.
3. Reads are side-effect free except for harmless observation timestamps.
4. Revisions advance only on material state change.
5. Older states must never overwrite newer revisions.

---

## Anti-patterns

Do not:

- replace `StatusTracker`
- collapse health, attention, activity, and work meaning into one enum
- define `busy` directly from raw activity enums alone
- define `dreaming` from heartbeat alone
- move normalization logic into gateway handlers
- instantiate a new presence service per request
- let PET invent NAVI presence semantics locally
- let the LLM choose NAVI presence enums directly

---

## Backward compatibility

The following remain stable during migration:

- `schema.AgentActivityState`
- `StatusTracker`
- `/api/agent/status`

Presence layers on top of those surfaces. It should not replace them blindly.

---

## Bottom line

The correct model is:

- keep `StatusTracker` as low-level runtime truth
- keep presence as a NAVI-owned subsystem above it
- keep work classification separate so `working` and `busy` stay meaningful
- keep dreaming separate until a real classification seam exists
- keep gateway and PET consuming presence rather than reconstructing it

That is how NAVI gets a real stateful awareness layer without turning one status enum into a trash fire.
