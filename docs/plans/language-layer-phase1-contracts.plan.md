# Language-Layer Phase 1 — Shared Contracts & Governed Reads

**Status:** In Progress — implementation tasks T1–T5 merged; T6 (docs & index wiring) is the only remaining task
**Last Updated:** 2026-06-03
**Owner (direction):** Architecture
**Implementation:** delegated to sub-agents (see §7 task breakdown)
**Contract:** [Language-Layer Contract](../architecture/language-layer-contract.md)
**Companions:** [Intake Synthesis Seam](../design/intake-synthesis-seam.md) · [Python Skill Runtime](python-skill-runtime-implementation.md) · [Skill Python Runtime concept](../concepts/skill-python-runtime.md)

> This is the **smallest, highest-leverage, lowest-risk** phase of the Go/Python/TypeScript evolution. It
> ships no agent-behavior change. It establishes the typed contract the other phases depend on and the one
> governed-read primitive Python deciders need. **Implementation is delegated; this document is the
> direction, scope, and acceptance bar.**

---

## 1. Why this phase first

The [Language-Layer Contract](../architecture/language-layer-contract.md) §7 names the generated contract
as the *foundation* of the split: without it the three languages drift into subtly incompatible systems.
Today the governed types live only in Go (`internal/schema/`), TypeScript hand-writes Zod
(`web-src/navi-console/src/types/api`), and Python passes raw dicts (`python/intake/worker.py`). Phase 1
removes that drift surface before any orchestration behavior moves across the boundary.

Phase 1 also delivers the first **governed read** primitive (`query_context(purpose, scope)`), because the
contract treats reads as authority-bearing (§4 of the contract) and the validation experiment (eval
scoring) needs scoped, redacted context to operate safely.

This phase does **not** move the run loop, add a Python decider that influences execution, or change
governance semantics. Those are later phases.

---

## 2. Scope

**In scope**

1. A shared **generated contract** for governed types, emitted from `internal/schema/` to Python and
   TypeScript targets.
2. A thin **governed read** surface (`query_context`) exposed by the Go kernel: purpose-bound,
   scope-limited, redacted, provenance-tagged, audit-attributed.
3. A minimal **`navi` Python SDK package** wrapping that read surface with the typed contract — reads only,
   no effects.
4. CI **conformance guards** for the contract's enforcement rules (§8 of the contract).

**Out of scope (later phases)**

- Any Python decider that produces effects or influences execution (Phase 2 / validation experiment).
- The standalone `@navi/sdk` TypeScript package extraction (Phase 3).
- Moving worker behavior into Python (Phase 4).
- A long-lived Python host or local socket/UDS control API (only if profiling later demands it).

---

## 3. Deliverables

| # | Deliverable | Lands in |
|---|---|---|
| D1 | Generated governed types for Python | `schema/python/` (extend `make generate-python`) |
| D2 | Generated governed types for TypeScript | new TS emitter target + output consumed by `web-src/navi-console` |
| D3 | `query_context` governed read endpoint/handler (purpose + scope + redaction + provenance + audit) | `internal/gateway` + a kernel read-mediation helper |
| D4 | `navi` Python SDK (reads only): `query_context(...)` typed against D1 | `python/navi/` (new package) |
| D5 | CI conformance guards | `.github/workflows/` + a small checker (Go or script) |
| D6 | Index + cross-link updates | `docs/architecture/README.md`, `docs/plans/INDEX.md` |

### 3.1 Generated governed types (D1, D2)

The authoritative source is `internal/schema/`. Generate exactly these (no hand authoring of governed
types):

```text
CommandType            (internal/schema/command.go)
ValidationOutcome      FailureClass        ReversibilityClass    (governor/schema)
ExecutionOutcome       (schema.ExecutionOutcome)
ActionDescriptor       MutationDescriptor
Proposal DTOs          Run DTOs            Capability DTOs       Telemetry/event DTOs
```

- **Python:** plain typed models (stdlib `dataclass`/`TypedDict` preferred to avoid a third-party
  dependency, matching `python/intake/worker.py`'s stdlib-only posture).
- **TypeScript:** types + Zod schemas that **replace** the hand-written `web-src/navi-console/src/types/api`
  governed types. UI-only state stays hand-written.

### 3.2 Governed read surface (D3)

`query_context` is the only new kernel surface in Phase 1. Required behavior (contract §4):

```text
purpose binding      — caller must declare purpose (e.g. "eval_scoring"); unknown purpose → reject
scope limits         — caller must declare scope (e.g. "current_run_summary"); scope bounds what is returned
redaction            — sensitive fields removed per purpose/scope policy
provenance tagging    — returned context carries provenance
audit attribution    — each read recorded/attributable
least-context         — return the minimum that satisfies the purpose
policy-aware filtering — respect existing governance/visibility rules
```

It is **read-only**: it must never write, mutate, or schedule. It reuses existing world-model/store read
paths behind the mediation layer; it does **not** expose generic world-model- or DB-shaped queries.

### 3.3 `navi` Python SDK (D4)

A new `python/navi/` package: one governed read method (`async def query_context(...)`) typed against D1,
talking to D3. **No effect methods in Phase 1.** This proves the `await navi.query_context(...)` ergonomics
and the governed-read contract against the real kernel, with minimal surface.

---

## 4. Acceptance criteria

1. `make generate-python` emits the governed types (§3.1) to `schema/python/`; regenerating is idempotent
   and the build is clean.
2. The TypeScript emitter produces governed types/Zod consumed by `web-src/navi-console`, and the
   hand-written governed Zod in `src/types/api` is removed in favor of generated output. UI-only types are
   untouched.
3. `query_context` rejects calls missing `purpose` or `scope`, returns redacted + provenance-tagged +
   least-context results, records an audit attribution, and has **no** write/schedule path.
4. The `navi` Python SDK calls `query_context` end-to-end and round-trips the generated types; it exposes
   no effect method.
5. CI guards fail the build when: Python imports a store/DB handle or a privileged connector; a Python read
   call omits `purpose`/`scope`; a governed DTO is hand-written instead of generated.
6. No change to the run loop, governance semantics, scheduler, or execution recording.

---

## 5. Risks & guardrails (Phase-1-specific)

| Risk | Guardrail |
|---|---|
| Generated/handwritten contract divergence during cutover | Land Go→Python→TS generation in one change; delete the superseded hand-written governed Zod in the same PR; contract round-trip test. |
| `query_context` becoming a generic data tap | Enforce purpose+scope as required inputs; deny unknown purposes; least-context default; CI guard on call sites. |
| Scope creep into effects | Phase 1 SDK is reads-only by construction; effect methods are explicitly Phase 2. |
| Codegen tool sprawl | Prefer stdlib Python output and a single TS emitter wired into existing `make` targets; no new heavyweight framework. |

---

## 6. Verification

- `make generate-python` then `make test`; add a contract round-trip test (`ExecutionOutcome` and
  `ValidationOutcome` serialize Go→Python→Go and Go→TS with no field loss).
- Drive the `navi` Python SDK against a running kernel: a `query_context(purpose="eval_scoring",
  scope="current_run_summary")` returns redacted, provenance-tagged context and writes an audit record.
- Run the CI conformance checker locally; confirm it fails on a deliberately planted violation (a Python
  `import store`, a `purpose`-less read, a hand-written governed DTO) and passes when removed.

---

## 7. Delegable task breakdown (for sub-agents)

Each task is self-contained and references the contract. Sequencing: **T1 → (T2, T3) → T4 → T5 → T6**.
T2 and T3 may run in parallel after T1.

- **T1 — Governed-type codegen (Go → Python).** ✅ Merged. Extends `make generate-python` to emit the §3.1
  governed types to `schema/python/` as stdlib-typed models, with a round-trip test. *(Foundation.)*
- **T2 — Governed-type codegen (Go → TypeScript).** ✅ Merged. TS emitter (`schema/ts/gen`, `make
  generate-ts`) produces governed types/Zod consumed by the console; UI-only types untouched.
- **T3 — `query_context` governed read surface.** ✅ Merged. Purpose/scope/redaction/provenance/audit/
  least-context read mediation lives in `internal/contextread`, exposed via `internal/gateway`; read-only,
  enumerated (purpose, scope) pairs, no generic query passthrough.
- **T4 — `navi` Python SDK (reads only).** ✅ Merged. `python/navi/` package with a single typed
  `query_context(...)` against T1 types and the T3 endpoint; no effect methods.
- **T5 — CI conformance guards.** ✅ Merged. `cmd/langguard` (`make conformance`) + CI job guard against
  Python store/DB or privileged-connector imports, `purpose`/`scope`-less reads, and hand-written governed
  DTOs.
- **T6 — Docs & index wiring.** Register both docs in `docs/architecture/README.md` and
  `docs/plans/INDEX.md`; link the contract from the systems map if appropriate.

---

## 8. What this unblocks

With Phase 1 landed, the **validation experiment** (a Python **eval scorer** — advisory, recordable, no
execution authority — followed by a shadow-only tool-ranking decider) can be built on a typed contract and
a governed read primitive, with conformance enforced by CI. See the
[Language-Layer Contract](../architecture/language-layer-contract.md) and the master evaluation in
`~/.claude/plans/` for the full phase sequence.

---

[docs plans INDEX](INDEX.md)
