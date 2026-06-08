# Task T3 — Governed read surface: `query_context`

> Self-contained prompt for a sub-agent. No prior conversation context is assumed.
> **Blocked by T1** (consumes generated types). Can run in parallel with T2.

## Who/where you are

NAVI codebase at `C:\Users\evirg\codespace\NAVI-Ecosystem\projects\navi`. Read `CLAUDE.md`. The Go kernel
owns all authority; this task adds a **read-only** mediation surface — it must never write, mutate, or
schedule anything.

## Required reading

1. `docs/architecture/language-layer-contract.md` §4 ("Reads are governed too") — this is your behavioral
   spec. Also §2.1 (kernel packages) and §6 (dependency rules).
2. `docs/plans/language-layer-phase1-contracts.plan.md` §3.2 and §7 (T3).
3. `internal/worldmodel/worldmodel.go` — the façade with the existing read paths you will reuse (e.g.
   `ContextBlockForChat`, `AssembleUserModel`, `ListFacts`, `ListMemories`). **Reuse these; do not add new
   raw store queries.**
4. `internal/gateway/server.go` (route registration starts ~line 174) — how routes/handlers are wired,
   CORS, and the auth model (`protect(...)`, loopback owner-access, `X-API-Key` / `Authorization: Bearer`).
5. `internal/store/eventlog.go` (or the existing audit/provenance path) — for audit attribution of each read.

## The job

Implement a single governed read operation, `query_context`, exposed as a gateway endpoint (e.g.
`POST /api/context/query`) backed by a small kernel **read-mediation helper** (a new function/type that wraps
the world-model read paths with the governance below). Request carries at minimum `run_id`, `purpose`,
`scope`.

Required behavior (all mandatory):

```text
purpose binding        — `purpose` is required; an unknown/unsupported purpose is REJECTED (400-class)
scope limits           — `scope` is required; the returned context is bounded by scope (e.g.
                         "current_run_summary" returns only that run's summary, not the whole world model)
redaction              — strip sensitive fields per purpose/scope policy before returning
least-context          — return the minimum that satisfies the purpose; no "return everything" path
provenance tagging     — returned context carries provenance markers
audit attribution      — every call records an attributable audit entry (who/purpose/scope/when)
policy-aware filtering — respect existing governance/visibility rules
```

Define an initial small, **enumerated** set of `purpose` and `scope` values (start with what the eval-scoring
experiment needs: `purpose="eval_scoring"`, `scope="current_run_summary"`). Unknown values are rejected, not
silently widened. Use the T1/T2 generated DTOs for the response shape where applicable.

## Hard constraints

- **Read-only.** No code path may write the world model, mutate execution history, schedule, or call a
  connector. There must be no way to reach a generic world-model- or DB-shaped query through this endpoint.
- Reuse `internal/worldmodel` read methods behind the mediation layer; do not bypass the façade into
  `internal/store` with new ad-hoc SQL.

## Acceptance criteria

1. Endpoint rejects requests missing `purpose` or `scope`, and rejects unknown enumerated values.
2. Returns redacted, scope-bounded, least-context, provenance-tagged results; records an audit entry per call.
3. Unit/integration tests cover: missing purpose → reject; unknown scope → reject; happy path returns redacted
   + provenance + audit-recorded context; and a test asserting no write/schedule path is reachable.
4. `make test` passes; compile-check clean (`go build -o /dev/null ./cmd/navid/ && go build -o /dev/null ./cmd/navi/`).
5. New route documented in `docs/specs/gateway-api.md`.

## Out of scope / do not touch

- No effect/write operations (no `create`/`update`/`propose`/`schedule`) — Phase 2+.
- No Python SDK (that is T4, which will call this endpoint).
- Do not change existing read endpoints' behavior.

## Definition of done / report back

Report: the endpoint path + request/response shape; the enumerated `purpose`/`scope` values you defined and
where they're validated; which world-model read methods you reused; how redaction/provenance/audit are
implemented; and the test names proving rejection + read-only.
