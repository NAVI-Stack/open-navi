# ICS v1 Implementation Status

**Status:** Active implementation note  
**Last Updated:** 2026-04-15  
**Applies To:** `feat/inference-control-system`  
**Related corpus:** [ICS docs index](inference-control-system/INDEX.md)

This document records the current implementation-truth for ICS v1 where the branch is already materially implemented but the original spec still leaves some items in draft language or open-question form.

It does **not** introduce a second decision path.
It does **not** weaken fail-closed behavior.
It exists to close residual design/spec drift by making current v1 boundaries explicit.

---

## Overall state

ICS is the authoritative control path for runtime decision control on this branch.

Implemented now:
- canonical ICS types and controller
- explicit focus arbitration
- explicit mode routing
- explicit candidate evaluation
- structured governance handoff
- fail-closed model/tool authorization
- persisted ICS state/history
- recovery checkpoint and route generation
- outcome supervision after execution
- decision trace / recovery / reflection event emission

Partially implemented now:
- documentation status in the original v1 spec
- named extraction of the execution supervisor seam
- richer compensation metadata at the execution snapshot boundary
- separate durable goal entity outside persisted ICS state/history

Intentionally deferred from v1:
- separate durable goal-store entity
- pluggable subreasoner modules
- richer recovery/compensation orchestration beyond the current route contract

---

## 1. Execution Supervisor boundary

### v1 policy

The current concrete **Execution Supervisor** implementation is:
- `internal/navi/runtime_executor.go:ExecuteRun`
- `internal/navi/runtime_inference.go:superviseInferenceOutcome`
- `internal/navi/inference/controller.ObserveOutcome`

In v1, this seam is **named conceptually but implemented inline**. That is acceptable.

### Why this is acceptable in v1

The execution supervisor already exists as one authoritative runtime path. It:
- binds execution to `RunState`
- checkpoints through runtime state
- routes through ICS-issued permits/contracts only
- supervises outcomes after execution
- persists updated ICS state/history
- emits recovery / reflection / decision-trace artifacts

### v1 constraint

Do **not** extract a second executor or alternate supervision path.
If this seam is extracted later, it must be a pure refactor of the current path and remain singular.

---

## 2. Recovery / compensation

### v1 policy

The smallest concrete recovery contract for v1 is the existing `RecoveryRoute` plus `RecoveryCheckpoint` contract.

Current deterministic route rules:
- approval boundary present for relevant recovery -> `propose`
- approval denied -> `replan`
- partial execution + compensable external capability + no approval required -> `compensate`
- timeout / connector unavailable + no approval required -> `retry`
- stale-state / contradiction / policy block -> `replan`
- open recovery with blockers and defer candidate -> `defer`
- otherwise open recovery -> `recover`
- otherwise -> `none`

### v1 retry floor

Irreversible or approval-gated work must **not** be retried autonomously.

Practical rule in v1:
- if recovery needs approval, route to `propose`
- if partial execution is compensable, route to `compensate`
- if partial execution is irreversible/non-compensable, keep recovery open and route to `recover` unless an approval boundary exists, in which case route to `propose`

### v1 limitation

Execution snapshots do not yet carry a richer first-class compensation plan object. That is an implementation follow-up, not a reason to reopen execution semantics.

---

## 3. Durable long-running goals

### v1 policy

V1 does **not** define a separate durable goal entity/store.

Durability for long-running goals and plans is provided through:
- persisted ICS decision envelope state via `ICSStateStore`
- appended ICS history via `AppendICSHistory`
- embedded copies on `RunState` / `Checkpoint`
- `GoalStack` and `PlanGraph` carried inside the persisted rationale/envelope

### Resume semantics

Resume reconstructs control state from persisted ICS state/history and runtime checkpoint state.
There is no separate goal-store authority in v1.

### v1 follow-up

A separate durable goal entity is a possible v2+ design only if it adds durability/query value without splitting control truth away from ICS.

---

## 4. Subreasoner semantics

### v1 policy

In v1, **subreasoners are control labels with policy-driven invocation**, not pluggable modules.

Concrete meaning in v1:
- represented by `Subreasoner` enum values
- activated/suppressed by policy in `subreasoner_policy.go`
- pinned through `SubreasonerPin`
- decayed over cycles through `DecayRemaining`

### non-goal for v1

Do not imply that subreasoners are separate model runtimes, agents, or plugin modules.
They are structured control roles inside one ICS decision path.

---

## 5. Anti-thrashing / hysteresis

### v1 policy

Anti-thrashing is implemented, not deferred.

Current explicit rules:
- focus arbitration default hysteresis: `0.08`
- continuity bonus for previous focus candidate: `+0.08`
- hard-preempt candidates bypass hysteresis retention
- non-preemptible current focus is retained unless hard-preempted
- otherwise previous focus is retained when `previous_score + hysteresis >= best_new_score`

Subreasoner pin decay is also explicit in v1:
- default pin decay window: `2` cycles
- triggers refresh pins back to full decay
- untriggered pins decrement and then release

### v1 scope

This is sufficient to call focus arbitration deterministic enough to test.
Future scoring refinements are allowed, but they must preserve explicit hysteresis semantics rather than replacing them with opaque heuristics.

---

## 6. Proactive message boundary

### v1 policy

`SendProactiveMessage` is a **system delivery path only**.
It is **not** an ICS decision path.
It is **not** a tool-execution path.
It is **not** a runtime resume path.

Current allowed use:
- append a proactive/system assistant message to an already owner-facing session
- source is system/heartbeat delivery
- message kind is proactive delivery

### hard boundary

This helper must not be used to:
- execute tools
- resolve proposals
- resume blocked runs
- bypass model/tool authorization
- inject hidden decisions that should have gone through ICS

Any proactive behavior that contains decision-making, execution, approval handling, or resume semantics must go through the normal ICS/coordinator/runtime path.

---

## 7. Documentation drift status

### Current status by gap

| Area | Status |
|---|---|
| Execution supervisor boundary | implemented, seam explicit here |
| Recovery route semantics | implemented for v1, richer compensation metadata deferred |
| Durable goals via ICS state/history | implemented for v1 |
| Separate durable goal entity | intentionally deferred |
| Subreasoners as labels | implemented |
| Pluggable subreasoner modules | intentionally deferred |
| Anti-thrashing / hysteresis | implemented |
| Proactive system delivery boundary | implemented, explicit here |
| Original `inference-control-system-v1.md` status/checklist text | partially stale |

### Repo truth rule

Where the original v1 spec still reads like an open design question, this file records the implementation-truth for the current branch.
It should be used as the authoritative drift-closure note until the base spec is rewritten in place.

---

## Follow-up recommendations

Small follow-ups that remain valid without changing v1 authority model:

1. Inline code comment in `ExecuteRun` explicitly marking it as the current concrete execution-supervisor seam.
2. Extend execution outcome metadata so recovery/compensation routes are easier to inspect outside the full ICS envelope.
3. Continue tightening `docs/specs/inference-control-system-v1.md` status/checklist sections so they stay aligned with the normative ICS contract set.

---

[ICS docs index](inference-control-system/INDEX.md) · [ICS foundational spec](../specs/inference-control-system-v1.md) · [docs INDEX](../INDEX.md)
