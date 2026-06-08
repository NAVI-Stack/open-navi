# Subconscious Escalation and Urgent Interruption

**Status:** Accepted (roadmap)  
**Concept:** [SUB-02] in concept-vs-implementation gap plan.

## Rule

Escalation can be manual (“remember this”) or **automatic** (NAVI detects significance). Subconscious → Conscious urgent interruption when: finding contradicts active information, high confidence, and material harm would result. Recursion guard: one interruption per decision cycle.

## Current implementation

- **Manual escalation:** `isManualEscalationHint` in loop sets tier Deep and escalation_reason "manual"; reflection worker routes by tier.
- **Interruption schema and emission:** `SubconsciousInterruption` (mode, summary, details, contradiction) and `EmitInterruption` with recursion guard in `internal/navi/reflection/worker.go`. Reflection uses it for blocking/advisory proposals (e.g. lifecycle actions).
- **Event:** `FactSubconsciousInterruption` published to bus; Experience layer or gateway should subscribe and present; `ClearInterruptionGuard` at next turn or on acknowledge.
- **Recursion guard:** One interruption per decision cycle; `EmitInterruption` uses a compare-and-swap guard; `ClearInterruptionGuard()` resets at next turn or on acknowledge.
- **Automatic significance detection:** `detectSignificance` in the reflection worker inspects summary/details for signals (conflict, priority, configuration, anomaly, risk keywords; long payloads). When detected, significance is set to high and payload is escalated to Consolidation without user phrasing.
- **Contradiction detection:** Optional `ContradictionChecker` hook; when set, the worker calls it before applying fact writes. A **default implementation** (`NewDefaultContradictionChecker`) compares the proposed fact key/value with recent Conscious context (last N directive messages via `RecentMessagesFunc`). If the key appears in recent context with a different or absent value, it returns contradiction and material harm so the worker emits an interruption. Main wires the default checker with `store.GetMessages` and limit 10.

**Not yet implemented:**

1. **Consumption in Conscious loop** — No wiring that ensures the next turn’s context includes the last interruption (e.g. gateway or loop subscribing to `FactSubconsciousInterruption` and injecting into context or session).

## Roadmap

1. ~~**Automatic significance:**~~ Implemented. Keyword/length-based detection in worker; high significance triggers Consolidation.
2. ~~**Contradiction detection:**~~ Implemented. Default checker compares fact key/value with recent directive messages; host wires via `SetContradictionChecker(NewDefaultContradictionChecker(getMessages, limit))`.
3. **Conscious consumption:** Gateway or NAVI loop subscribes to `FactSubconsciousInterruption`; on receive, store or attach to session and ensure next `Wake()` or next request includes the interruption in context; call `ClearInterruptionGuard()` when turn starts or user acknowledges.

## Relevant files

- `internal/navi/loop.go` (escalation hint, future context injection)
- `internal/navi/reflection/worker.go` (EmitInterruption, ClearInterruptionGuard)
- `internal/schema/reflection.go` (SubconsciousInterruption, InterruptionMode)
- `internal/store/` (history, world model reads for contradiction comparison)
