# Task T5 — CI conformance guards

> Self-contained prompt for a sub-agent. No prior conversation context is assumed.
> **Blocked by T1–T4** (the guards lint the artifacts those tasks produce).

## Who/where you are

NAVI codebase at `C:\Users\evirg\codespace\NAVI-Ecosystem\projects\navi`. Read `CLAUDE.md`. CI lives under
`.github/workflows/`. These guards mechanically enforce the language-layer contract so violations fail the
build instead of relying on review.

## Required reading

1. `docs/architecture/language-layer-contract.md` §8 (Compliance & enforcement) — these are the rules you are
   enforcing. Also §3.1 (forbidden list) and §6 (dependency rules).
2. `docs/plans/language-layer-phase1-contracts.plan.md` §4.5, §5, §7 (T5).
3. The artifacts to lint: `python/` (esp. `python/navi/` from T4), `schema/python/` and the TS generated
   output (from T1/T2), and the `make generate-python` / codegen targets.

## The job

Add a CI job (extend or add a workflow in `.github/workflows/`) plus a small checker (a Go program under
`cmd/` built to `bin/`, or a script) that **fails the build** on any of:

1. **Forbidden Python imports.** Any module under `python/` imports a store/DB handle, a database driver, NAVI
   Go internals, or a privileged connector. (Static scan of imports; maintain an explicit allowlist/denylist.)
2. **Ungoverned Python reads.** A Python context read that omits `purpose` or `scope`, or that hits the
   gateway context endpoint directly instead of going through the `navi` SDK's typed `query_context`
   (whose kwargs are required). Prefer enforcing via the SDK signature + a scan for raw endpoint calls.
3. **Hand-edited generated contracts.** A governed DTO was hand-written or a generated file was manually
   edited. Implement as a **drift check**: re-run codegen (`make generate-python` + the TS emitter) in CI and
   `git diff --exit-code` the generated paths; a non-empty diff fails.

## Acceptance criteria

1. The CI job runs on PRs and fails on each of the three violation classes.
2. Each guard is proven: add a deliberately planted violation locally (a `python/` file with `import store`; a
   raw context call missing `purpose`; a manual edit to a generated file), confirm the guard trips, then
   remove it and confirm green.
3. The checker is fast, deterministic, and documented (a short README or comments explaining each rule and how
   to fix a failure).
4. If the checker is Go, it builds to `bin/` via a make target (never bare `go build` to repo root);
   `make test` still passes.

## Out of scope / do not touch

- Do not change the contract's rules; you are enforcing them, not redefining them.
- No new runtime behavior.

## Definition of done / report back

Report: the workflow file + checker location; how each of the three rules is detected; the exact commands a
developer runs locally to reproduce CI; and evidence each guard tripped on a planted violation and passed once
removed.
