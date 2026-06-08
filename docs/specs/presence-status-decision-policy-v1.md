# NAVI Presence Status Decision Policy v1

**Status:** Active  
**Last Updated:** 2026-04-18  
**Updated By:** agent

## Purpose

This document defines the deterministic policy NAVI uses to choose its own:

- **public presence**
- **internal presence mode**

Its purpose is to prevent semantic drift in the places that are easiest to get sloppy:

- `active` vs `working`
- `working` vs `busy`
- `dreaming` vs `idle`
- `needs_attention` vs `wants_attention`
- public presence vs health vs internal mode

This document is the decision-policy companion to:

- `docs/specs/pet-presence-interface-v1.md`
- `docs/specs/presence-runtime-framework-v1.md`

---

## Core rule

NAVI presence is **runtime-inferred, code-owned state**.

It is not:

- an LLM-authored free-form label
- a PET-authored status
- a plugin-authored override
- a UI-only interpretation

Descriptive text fields may exist, but the actual selected presence enum must be chosen by deterministic code.

---

## Terminology

### Public presence

The outward-facing state consumers such as PET should use.

Examples:

- `active`
- `working`
- `busy`
- `dreaming`
- `needs_attention`

### Internal mode

A richer NAVI-internal interpretation of current behavior.

Examples:

- `interactive`
- `working`
- `waiting`
- `heartbeat`
- `dreaming`

### Activity

Low-level runtime truth from `StatusTracker`.

Examples:

- `processing`
- `tool_executing`
- `waiting_for_input`
- `heartbeat`

### Work classification

The higher-level meaning of active work.

Examples:

- conversational vs execution vs background
- interruptible vs non-interruptible
- low vs high interruption/resume cost

### Attention

Whether owner awareness or action is now the dominant truth.

### Health

Whether NAVI is healthy, degraded, unresponsive, or offline.

These must remain distinct.

---

## Decision outputs

The policy produces two outputs.

### 1. Internal presence mode

A richer internal classification used by NAVI and operator-facing surfaces.

### 2. Public presence

A normalized PET-facing state chosen from the canonical public enum.

Public presence is not a lossy copy of internal mode. It is a deliberate outward presentation chosen from deterministic rules.

---

## Inputs

The policy reads summarized inputs from the presence subsystem.

### Activity snapshot

Low-level runtime truth from `StatusTracker`.

### Work classification summary

A higher-level summary that answers questions the activity tracker cannot answer alone, including:

- is NAVI doing substantive work?
- is the current work interruptible?
- what is the interruption cost?
- what is the resume/reentry cost?
- does the current work demand a focus lock?
- is the work conversational, executional, or background?

### Attention summary

Blocking vs non-blocking owner-relevant state.

### Health summary

Whether NAVI is healthy, degraded, unresponsive, or offline.

### Dreaming summary

Whether NAVI is in a background internal-processing mode that should normalize to `dreaming`.

---

## Why work classification is required

`StatusTracker` alone cannot reliably distinguish `working` from `busy`.

Examples:

- both may appear as `tool_executing`
- one may be safely interruptible and easy to redirect
- another may be technically stoppable, but costly or awkward to resume cleanly
- another may be in a high-focus phase where interruption is possible but clearly undesirable

If `busy` is inferred directly from low-level activity enums alone, the meaning will drift.

Therefore NAVI needs an explicit work-classification seam.

---

## Recommended work classification model

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

### Field meanings

| Field | Meaning |
|------|---------|
| `HasActiveWork` | True when NAVI is engaged in meaningful work rather than idle presence. |
| `WorkType` | Broad class of work. |
| `Interruptibility` | Whether the current work can be interrupted without material disruption. |
| `InterruptionCost` | Cost of stopping, pausing, or redirecting the work in the current phase. |
| `ResumeCost` | Cost of cleanly resuming or reconstructing progress after interruption. |
| `FocusLock` | Whether the current phase demands sustained focus/resource concentration. |
| `ReasonCode` | Stable machine-readable reason. |
| `StatusText` | Short bounded human-readable label. |
| `Subtext` | Optional secondary explanation. |

---

## Public presence semantics

### `active`

NAVI is actively engaged, but not in a substantive execution phase that should be rendered as `working` or `busy`.

Typical cases:

- interpreting user input
- short conversational reasoning
- lightweight context assembly
- interactive turn handling without meaningful long-running execution

### `working`

NAVI is actively doing substantive work and may still be interrupted, redirected, or re-scoped without major disruption.

Typical cases:

- tool execution with low or medium interruption cost
- repository inspection
- read-heavy acquisition or analysis
- coding/search/lookup work where the user can redirect the task midstream
- agentic work where pivots are normal and low-friction

### `busy`

NAVI is actively doing substantive work and interruption would be materially disruptive, expensive to resume, awkward to reconstruct, or harmful to the current focus/resource posture.

Typical cases:

- guarded multi-step execution in flight
- deep research or synthesis phases with high reentry cost
- image generation or other generation phases that do not pivot cleanly mid-run
- external mutation with compensable or irreversible consequences underway
- agentic workflows that can technically be interrupted but where doing so would meaningfully disrupt progress or waste effort
- work explicitly marked `non_interruptible`

### `dreaming`

NAVI is doing background internal work, not foreground interactive work.

Typical cases:

- reflection
- memory consolidation
- synthesis
- background maintenance that is meaningfully internal rather than externally task-directed
- already-existing background processing once it is classified and surfaced through introspection

### `needs_attention`

NAVI is blocked or urgently requires owner input to proceed.

### `wants_attention`

NAVI is not hard blocked, but has something owner-relevant to surface.

### `idle`

NAVI is available but not actively engaged in substantive work.

### `offline`

NAVI is unavailable as a runtime.

---

## Precedence order

Public presence selection must follow this order, top to bottom.

### 1. Hard health overrides

If health is:

- `offline` -> public presence = `offline`
- `unresponsive` -> internal mode = `unresponsive`; public presence may remain last known meaningful status unless runtime policy explicitly forces offline-style presentation

This layer wins over all lower layers.

### 2. Dreaming override

If dreaming is active:

- internal mode = `dreaming`
- public presence = `dreaming`

Dreaming outranks normal idle/working classification.

### 3. Blocking attention override

If attention indicates blocking owner input:

- public presence = `needs_attention`

This wins over `working`, `busy`, and `active` because owner action is now the dominant truth.

### 4. Non-blocking attention override

If attention indicates meaningful non-blocking owner relevance:

- public presence = `wants_attention`

This is appropriate when NAVI has something to surface but is not blocked.

### 5. Active work classification

If `HasActiveWork = true`:

- `Interruptibility = non_interruptible` -> `busy`
- `Interruptibility = queue_only` -> `busy` by default
- `InterruptionCost = high` -> `busy`
- `ResumeCost = high` -> `busy`
- `FocusLock = hard` -> `busy`
- otherwise -> `working`

### 6. Interactive activity classification

If NAVI is active but not classified as substantive work:

- public presence = `active`

### 7. Default idle

If none of the above applies:

- public presence = `idle`

---

## Internal mode selection

Internal mode should be selected separately from public presence.

### Recommended mapping

| Condition | Internal mode |
|----------|---------------|
| health offline | `offline` |
| health unresponsive | `unresponsive` |
| dreaming active | `dreaming` |
| blocking/non-blocking wait for owner input | `waiting` |
| active execution work | `working` |
| heartbeat-specific cycle | `heartbeat` |
| degraded but running runtime | `degraded` |
| interactive turn handling | `interactive` |
| none of the above | `idle` |

Important:

- public `busy` does **not** require a separate internal `busy` mode
- in v1, `busy` is a public rendering of `working + high interruption/resume/focus cost`
- health and attention may override public presence without changing internal mode in the same way

---

## Working vs busy rule

This distinction must stay stable.

### `working`

Use `working` when:

- NAVI is executing meaningful work
- interruption is acceptable
- the system can pivot, redirect, or queue safely
- resume/reentry cost is low or moderate
- the current phase is not heavily focus-locked

### `busy`

Use `busy` when:

- NAVI is executing meaningful work
- interruption is not acceptable **or**
- interruption is technically possible but costly to resume cleanly **or**
- the current phase is sufficiently focus-locked or resource-concentrated that interruption would meaningfully degrade work quality or progress

### Hard rule

`busy` is **not** just “working but more intense.”

It specifically means the current work has **high interruption cost, high resume cost, or hard focus lock**.

If implementers cannot justify `busy` in those terms, they should not use it.

---

## Attention rule

### `needs_attention`

Use when:

- owner input is required to continue
- a blocking proposal is active
- proceeding autonomously is not allowed

### `wants_attention`

Use when:

- owner input is not required to continue
- but NAVI has something meaningful to surface
- examples include a non-blocking recommendation, queued advisory, or recoverable degraded state worth surfacing

### Hard rule

Attention states outrank work-display states.

If NAVI is working but blocked on owner input, the correct public presence is `needs_attention`, not `working`.

---

## Dreaming rule

Dreaming is a first-class internal mode and public presence.

Use `dreaming` when:

- NAVI is engaged in background internal cognition or maintenance
- the work is not foreground conversational handling
- the work is not best represented as waiting, idle, or external active execution

### Existing foundation rule

NAVI may already perform background processing before formal dreaming introspection exists.

That existing background processing is the **behavioral foundation** of dreaming, but it must not be surfaced as `dreaming` until the runtime has a clean introspection or classification seam for it.

### Hard rule

Do not fake dreaming by mapping heartbeat directly to `dreaming`.

Heartbeat may be a signal, but dreaming must remain a separate mode source.

---

## Health and degraded-state rule

Health is not the same thing as public presence.

### Degraded

When degraded:

- keep `health.state = degraded`
- preserve the most semantically accurate public presence unless owner awareness becomes the dominant truth

Examples:

- degraded + active work -> may still present `working`
- degraded + owner action needed -> elevate to `needs_attention`

### Unresponsive

When unresponsive:

- internal mode = `unresponsive`
- health must reflect it
- transport/UI may separately present offline-like behavior if the runtime is effectively unreachable

---

## Status text and subtext policy

`status_text` and `subtext` are descriptive fields, not state authority.

### Rules

- must be bounded and short
- should come from structured sources where possible
- must not be treated as the source of enum truth
- should not expose raw internal secrets or unstable debug-only strings

Examples:

- `working` + status text: `Reviewing repo state`
- `busy` + subtext: `Deep research in progress`
- `busy` + subtext: `Generation phase in progress`
- `dreaming` + status text: `Consolidating recent context`

---

## Current conservative implementation stance

Until richer classification seams exist, implementation should stay conservative.

### Default-safe behavior

If richer classification sources are not yet available:

- prefer `working` over `busy`
- prefer `active` over overclaiming `working`
- prefer `idle` over inventing `dreaming`

In other words: under-classify rather than overclaim.

### Current concrete implications

- `tool_executing` should default to `working`
- heartbeat must not default to public `dreaming`
- `busy` should not appear without evidence from work classification
- dreaming should not appear without a real classification seam

---

## Suggested implementation interfaces

```go
type WorkClassificationSource interface {
    CurrentWorkClassification(ctx context.Context) WorkClassificationSummary
}
```

```go
type PresenceDecisionPolicy interface {
    Decide(ctx context.Context, in PresenceDecisionInputs) PresenceDecisionOutput
}
```

Where inputs include:

- activity snapshot
- work classification summary
- attention summary
- health summary
- dreaming summary

---

## Anti-patterns

Do not:

- map all `tool_executing` to `busy`
- map all `processing` to `working`
- use `busy` as a cosmetic synonym for “serious work”
- use `CurrentDetail` alone to decide enum state
- let PET reinterpret NAVI public presence semantics locally
- let the LLM choose presence enums directly
- let health silently replace public-presence semantics

---

## Bottom line

NAVI status selection must be deterministic and precedence-driven.

The stable mental model is:

- **health** decides whether NAVI is fundamentally available
- **dreaming** decides whether NAVI is in background internal cognition
- **attention** decides whether owner relevance is the dominant truth
- **work classification** decides `working` vs `busy`
- **activity** fills in the remaining `active` vs `idle` distinction

That is the decision framework that keeps implementation and design aligned as the system grows.
