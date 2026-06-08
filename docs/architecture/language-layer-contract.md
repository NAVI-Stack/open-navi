# Language-Layer Contract (Go / Python / TypeScript)

> [!NOTE]
> Part of the [NAVI Systems Map](navi-systems-map.md).

**Status:** Active (architectural contract)
**Last Updated:** 2026-06-03
**Source of truth:** this document is the canonical boundary contract for NAVI's three language layers.
**Companions:** [LLM Dual-Plane Architecture](llm-dual-plane.md) · [Intake Synthesis Seam](../design/intake-synthesis-seam.md) · [Skill Python Runtime concept](../concepts/skill-python-runtime.md) · [Architecture Overview](README.md) · [Gateway API](../specs/gateway-api.md)

---

## 1. Purpose

NAVI is one system implemented in three languages. This document fixes **which language owns which
responsibility** and **how the layers are allowed to talk to each other**, so that the split stays a
clean architecture rather than drifting into three subtly incompatible systems.

This contract generalizes a boundary NAVI already runs in production for the context-intake seam
([Intake Synthesis Seam](../design/intake-synthesis-seam.md) §9) and lifts it to a system-wide rule.

It is a **contract**, not a roadmap. The migration sequencing lives in
[Language-Layer Phase 1 — Shared Contracts](../plans/language-layer-phase1-contracts.plan.md).

---

## 2. The three layers

```text
Go         = authoritative engine / kernel / durable runtime
Python     = governed orchestration layer / scripting host / decision hooks
TypeScript = UI / web / extension SDK / client surface
```

Stated as invariants:

```text
Go owns authority.
Python supplies governed behavior.
TypeScript presents and controls client surfaces.
```

**Python is a runtime *layer*, not "the runtime."** It hosts orchestration behavior, but it *depends on*
the Go kernel and does not stand beside it as a peer authority. The mental model is a game engine: Go is
the engine core, Python is the gameplay-scripting/behavior layer, TypeScript is the editor/launcher/web
surface — but stricter, because scripts must not mutate authoritative state directly.

### 2.1 Go — authoritative engine (never moves)

Go is the single source of truth for everything durable, privileged, scheduled, or side-effecting:

| Responsibility | Owner package |
|---|---|
| World-model writes | `internal/worldmodel` (façade over `internal/store`) |
| Governance: outcomes, autonomy, hard floors, tier resolution | `internal/governor` (`validate.go`, `autonomy.go`) |
| Command execution + `ExecutionOutcome` ledger | `internal/command` (`executor.go`), `internal/schema/command.go` |
| Scheduler truth | `internal/cron`, `internal/runtime/scheduler.go` |
| Capability/skill registry + sandbox execution boundary | `internal/tool`, `internal/navi/skill`, `internal/sandbox` |
| Persistence | `internal/store` (SQLite, raw SQL) |
| Telemetry / provenance / audit | `internal/runtime/metrics.go`, execution-outcome + provenance stores |
| Privileged connector execution | `internal/connectors`, `connectors/` |
| **Run loop, checkpointing, execution recording** | `internal/runtime` (`run.go`, `coordinator.go`) |

The run loop stays in Go. Only its **decision hooks** become swappable (see §4).

### 2.2 Python — governed orchestration layer (grows seam by seam)

Python hosts the high-change, model-sensitive behavior: context strategies, tool/skill selection,
workflow composition, eval scoring, self-improvement experiments, and skill implementations. Python
**decides, composes, requests, evaluates, and proposes**, then returns typed envelopes. Go adjudicates and
executes.

### 2.3 TypeScript — client / extension surface (pure client)

TypeScript **owns**: console UI, web client, the `@navi/sdk` extension SDK, developer tooling, client
events, operator surfaces, marketplace/extension UX. It consumes the same generated contracts and calls
the same gateway surfaces as any other client.

TypeScript **does not own**: agent behavior, governance decisions, world-model mutation rules, scheduler
truth, connector authority, execution retries, or proposal-resolution authority beyond *relaying* a
user/client request.

---

## 3. The central invariant

> **Python and TypeScript may request, advise, rank, evaluate, compose, or propose.
> Go approves, rejects, modifies, executes, records, and persists.**

Direction of dataflow for any effect is always **propose → Go decides → Go executes/records**, mirroring
the intake seam's `Python → Go, never the reverse` rule.

### 3.1 Forbidden for Python *and* TypeScript

```text
write world model state
bypass governance
resolve proposals independently
mutate execution history
own scheduler truth
call privileged connectors directly
invent tool identities outside the registry
retry irreversible effects without approval
persist hidden durable state
access raw DB/store internals
```

### 3.2 The API exposes governed operations, not raw backend access

```python
# Good — governed, purpose-scoped operations
await navi.query_context(run_id=run_id, purpose="eval_scoring", scope="current_run_summary")
await navi.evaluate_run(...)
await navi.rank_tools(...)
await navi.propose_action(...)
await navi.invoke_skill(...)

# Bad — raw backend reach-through
world_model.db.insert(...)
governor.skip(...)
connector.gmail.send(...)
scheduler.raw_insert(...)
```

The same principle binds TypeScript clients and extensions.

---

## 4. Reads are governed too ("read-only" is not "safe")

A decider with broad read access can still leak sensitive state into prompts, logs, external model calls,
traces, eval artifacts, or tool outputs — gaining *indirect* authority through unrestricted context. Read
access from Python (and any extension surface) must be **purpose-scoped and kernel-mediated** — never a
generic world-model- or DB-shaped query.

Every read API must enforce:

```text
purpose binding          scope limits           redaction
provenance tagging       audit attribution      least-context retrieval
policy-aware filtering
```

Preferred shape:

```python
await navi.query_context(run_id=run_id, purpose="eval_scoring", scope="current_run_summary")
```

---

## 5. The boundary mechanism (what already exists)

NAVI does **not** need new transport to honor this contract. The pieces exist:

1. **Outbound decision transport — JSON-RPC over stdio.** `internal/navi/skill/subprocess_runtime.go`
   launches protocol-speaking Python (and Node/TypeScript, shell, or external) workers over JSON-RPC stdio,
   with Docker sandbox profiles, network-egress policy, filesystem RO/RW policy, timeouts, output caps, and
   policy injection (`subprocessPolicy`). Skill transports `subprocess_python` and `subprocess` already use
   it (see [Architecture Overview](README.md) §"Skill execution flow" and
   [Skill Python Runtime concept](../concepts/skill-python-runtime.md)).
2. **Proven instance.** `python/intake/worker.py` is a stdlib-only worker that extracts/resolves entities,
   returns an envelope, and **never writes the world model**. Go's governor decides the outcome.
3. **Authority engine.** `governor.Pipeline.Run` returns a `ValidationOutcome`
   (`Approved` / `RequiresConfirmation` / `Modified` / `Rejected`); `ApplyAutonomy` may upgrade but can
   never override hard floors; tiers resolve System > Owner > Plugin.
4. **Execution ledger.** `command.Executor.Execute` wraps every effect, records a `schema.ExecutionOutcome`
   (with `CommandType`, failure class, retryability, reversibility), and is the only sanctioned effect path.
5. **Primitive vocabulary.** `internal/schema/command.go` already defines the command verbs
   (`query, create, update, delete, invoke, send, acquire, schedule, delegate, compose`) the SDK shape maps
   onto.
6. **Governed read surface.** `internal/contextread` now implements `query_context` (§4): a read-only,
   purpose-bound, scope-limited, redacted, provenance-tagged, audit-attributed mediation surface exposed via
   `internal/gateway`. It holds no DB handle and offers only an enumerated set of `(purpose, scope)` pairs —
   no generic world-model- or DB-shaped query. (Phase 1 / T3.)

**Transport rule:** reuse JSON-RPC-over-stdio as transport v1. Introduce a long-lived Python host with a
local socket/UDS control API only if profiling later demands it. The decides-in-Go rule is the invariant;
the wire format is not.

---

## 6. Dependency rules (enforced)

Following the precedent set by [LLM Dual-Plane Architecture](llm-dual-plane.md) §2:

1. **Go depends on nothing above it.** The kernel must compile and run with no Python and no TypeScript
   present. Python deciders are optional, hot-swappable behavior; TypeScript is an optional client.
2. **Python depends on the Go kernel, never the reverse at the authority level.** Go may *invoke* a Python
   decider and consume its envelope, but Go never imports decision logic from Python and never trusts a
   Python result as an authoritative write.
3. **Python holds no DB handle and no privileged connector handle.** Enforced structurally by the sandbox
   and by CI lint (§8).
4. **TypeScript holds no privileged path.** It authenticates through the gateway's existing
   API-key/bearer model (`X-API-Key` / `Authorization: Bearer navi_...`; loopback gets owner-like access)
   and calls the same routes any client would.
5. **Governed contracts are generated, not hand-written** (§7).

---

## 7. Generated vs. hand-maintained contracts

The generated contract is the **foundation** of the split. The following governed types are generated from
`internal/schema/` (extend `make generate-python`; add a TypeScript emitter) into both
`schema/python/` and a TypeScript target:

```text
CommandType            ValidationOutcome      FailureClass
ReversibilityClass     ExecutionOutcome       ActionDescriptor
MutationDescriptor     Proposal DTOs          Run DTOs
Capability DTOs        Telemetry/event DTOs
```

**Hand-maintained types must never define governed behavior.** Hand authoring is acceptable for UI-only
state, prompt text, persona content, and per-skill input schemas (the latter belong to the skill, per
`IdempotencyDependsOnSkill`) — but never for command/governance/execution contracts. Generated types
replace the hand-written Zod in `web-src/navi-console/src/types/api` and the raw dict passing in
`python/intake/worker.py`.

---

## 8. Compliance & enforcement

A change conforms to this contract when:

1. Every Python/TS-originated **effect** is expressed as a typed envelope that passes through
   `governor.Pipeline.Run` and is executed/recorded by `command.Executor`. No effect bypasses the engine.
2. `RequiresConfirmation` produces a Proposal; `Modified` writes the softened action; `Rejected` drops and
   logs; hard floors are never auto-approved.
3. Python imports **no** store/DB handle and **no** privileged connector. (CI lint guard.)
4. Every Python read call carries `purpose` + `scope`; generic world-model/DB-shaped reads are not exposed.
   (CI lint guard.)
5. Governed DTOs consumed by Python or TypeScript are **generated**, not hand-written.
6. Behavior moved into Python satisfies the migration rule in the
   [Phase 1 plan](../plans/language-layer-phase1-contracts.plan.md) (high-change / model-sensitive /
   safe-to-shadow / non-authoritative) — never "moved because it was easier to change."
7. The run loop, checkpointing, scheduler truth, and execution recording remain in Go.

---

## 9. Relationship to existing architecture

- **Intake Synthesis Seam** — the first concrete instance of this contract; this document generalizes it
  beyond entity resolution. Do not fork its governance: one `governor.Pipeline`, one Proposal Queue.
- **LLM Dual-Plane** — orthogonal and complementary. Dual-Plane separates stateless inference from stateful
  control *inside Go*; this contract separates *languages by authority*. A Python decider still calls the
  Go Control Plane for any model routing decision.
- **Skill transports** — Python/TS deciders ride the same `subprocess`/`subprocess_python` transport and
  result-normalization path already documented in [README](README.md) and
  [Skill Python Runtime](../concepts/skill-python-runtime.md).
- **Gateway** — the Go↔TS contract. Hardened and versioned over time; no new privileged surface for clients.

---

## 10. Non-goals

- Not a rewrite. The Go kernel and run loop stay; Python earns surface area one governed seam at a time.
- No second authority, second governance engine, or second Proposal Queue.
- No authoritative state, scheduling truth, or execution history in Python or TypeScript — ever.
- No heuristic risk/authority logic in prompts; authority lives in Go (`internal/governor`).
