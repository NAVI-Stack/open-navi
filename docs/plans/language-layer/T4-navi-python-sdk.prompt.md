# Task T4 — `navi` Python SDK (reads only)

> Self-contained prompt for a sub-agent. No prior conversation context is assumed.
> **Blocked by T1** (generated Python types) and **T3** (`query_context` endpoint).

## Who/where you are

NAVI codebase at `C:\Users\evirg\codespace\NAVI-Ecosystem\projects\navi`. Read `CLAUDE.md`. Today `python/`
contains exactly one module (`python/intake/worker.py`, stdlib-only). You are adding a new `python/navi/`
package.

## Required reading

1. `docs/architecture/language-layer-contract.md` §3.2 (governed operations, not raw access), §4 (governed
   reads), §6 (dependency rules — Python holds **no** DB handle and **no** privileged connector).
2. `docs/plans/language-layer-phase1-contracts.plan.md` §3.3 and §7 (T4).
3. T1's generated Python types under `schema/python/` (you will type against these).
4. T3's `query_context` endpoint contract (path, request fields `run_id`/`purpose`/`scope`, response shape).
5. `internal/gateway/server.go` auth model: a **loopback** caller gets owner-like access; non-loopback uses
   `X-API-Key` or `Authorization: Bearer navi_...`.

## The job

Create `python/navi/` exposing exactly **one** governed read method:

```python
async def query_context(*, run_id: str, purpose: str, scope: str) -> Context: ...
```

- It calls the T3 gateway endpoint over **loopback HTTP** (the kernel decides and mediates; the SDK is a thin
  typed client). `purpose` and `scope` are **required keyword arguments** — there must be no way to call it
  without them.
- Responses are parsed into the T1-generated types.
- Stdlib-preferred (e.g. `urllib`/`http.client` or `asyncio` + a minimal client) to keep the dependency
  footprint near-zero, consistent with `python/intake/worker.py`. If an async HTTP lib is introduced, justify
  it and keep it minimal.

**No effect methods in Phase 1.** No `create`, `update`, `propose`, `schedule`, `invoke`. The package must
not import any DB/store handle, any NAVI Go internals, or any privileged connector. It only speaks to the
gateway over HTTP.

## Acceptance criteria

1. `python/navi/` package with `query_context(run_id=, purpose=, scope=)` typed against generated models;
   `purpose`/`scope` are required (calling without them is a static/`TypeError`, not a silent default).
2. End-to-end check against a running kernel: `query_context(purpose="eval_scoring",
   scope="current_run_summary")` returns redacted, provenance-tagged context and triggers a T3 audit record.
3. A test (stdlib `unittest` or the project's Python test style) covering: required-kwarg enforcement; happy
   path against a stubbed/loopback endpoint; and that the package exposes **no** effect method.
4. No forbidden imports (no store/DB, no connector, no Go internals).

## Out of scope / do not touch

- No effect/write methods (Phase 2). No subprocess/decider integration (the eval-scorer experiment is a
  separate, later task). No CI guards (T5).

## Definition of done / report back

Report: the package layout and public surface (should be just `query_context`); the HTTP/auth approach used
for loopback; any dependency you added and why; and the test names. Confirm there is no effect method and no
forbidden import.
