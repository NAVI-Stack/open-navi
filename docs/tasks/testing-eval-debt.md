# Testing & Evaluation Debt

**Status:** Active
**Last Updated:** 2026-05-29
**Purpose:** Track the gaps between what the code does and what is actually verified by tests/evals, so "Done" never silently means "untested."

## Current CI baseline (what is actually run)

- **Go tests** — `go test ./cmd/... ./internal/... ./connectors/... -count=1` (see `Makefile` `test` target).
- **Docker health checks** — Compose stack health verification.
- **`navi-programmer` starter evals** — the first-party programmer plugin ships an evaluation harness (`plugins/navi-programmer/`, see `docs/plans/navi-programmer-evaluation.plan.md`). These are starter evals, not broad coverage.

## Known coverage gaps

| Area | Gap | Evidence / location |
|------|-----|---------------------|
| Console e2e | No browser-level e2e harness (no Playwright/Cypress); `ChatPage.tsx` orchestration is untested | `web-src/navi-console/src/pages/ChatPage.tsx` (no `*.test.tsx`) |
| Inference Control System | `outcome_supervisor.go`, `controller_authority.go`, and the tool/proposal authorization seams have no unit tests | `internal/navi/inference/outcome_supervisor.go`, `controller_authority.go`, `tool_authorization.go`, `proposal_authorization.go` |
| Route-doc parity | No automated check that `docs/specs/gateway-api.md` matches the routes registered in `internal/gateway/server.go` | manual today |
| Skill contract parity | No automated check that documented transports/spec fields match `internal/navi/skill/spec.go` and the executors | manual today |
| Governance / proposal integration | Runtime pause/resume on proposals is wired but lacks end-to-end integration tests across gateway → runtime → ICS | `internal/navi/runtime_executor.go:752-780` |
| Connector lifecycle | Connector start/stop/health/diagnostics paths vary in maturity; lifecycle coverage is uneven | `internal/connectors/lifecycle.go`, `manager.go` |
| Runtime outcome granularity | Execution outcomes are recorded but the granularity/assertions in tests are limited | `internal/runtime/*`, `internal/store` outcomes |
| Background services | Heartbeat, reflection, scheduler/backlog pollers are wired but lightly tested | `internal/navi/heartbeat`, `internal/navi/reflection`, `internal/backlog` |

## Suggested next steps (not commitments)

1. Add a `ChatPage.tsx` integration test (send/receive, live events, variant switch, feedback) before an e2e harness.
2. Add unit tests for the untested ICS seams (start with `controller_authority.go` and `tool_authorization.go`).
3. Add a lightweight route-doc parity check (a Go test that diffs registered route patterns against a generated list) — this is the one place a small validation helper is clearly justified.

[tasks INDEX](INDEX.md) | [docs INDEX](../INDEX.md)
