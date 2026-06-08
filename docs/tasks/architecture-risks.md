# Architecture Risks

**Review Date:** 2026-03-08  
**Scope:** Structural risks that remain after the recent stabilization pass.

## AR-1 - Non-atomic dual-write between bus and SQLite

- Event publication is not outbox-backed.
- Failure modes can diverge live bus state and replay log state.
- Planned mitigation: `NAVI-AUTO-008`.

## AR-2 - Skill execution reliability

- Executor now supports `internal`, `rest`, `mcp_tool`, `subprocess_python`, and generic `subprocess` transports.
- Remaining risk is reliability and isolation hardening for subprocess-backed skills, especially dependency setup, sandbox boundaries, and smoke coverage.
- Planned mitigation: `NAVI-AUTO-003`, `NAVI-AUTO-004`, `NAVI-AUTO-005`.

## AR-3 - No streaming token path

- Provider contract is request/response only.
- Long replies reduce perceived responsiveness and connector UX quality.
- Planned mitigation: `NAVI-AUTO-007`.

## AR-4 - Event log retention and replay posture

- SQLite event log has no implemented pruning policy.
- Recovery posture is not yet codified around authoritative replay strategy.
- Planned mitigation: `NAVI-AUTO-009` plus runbook/ADR follow-up.

## AR-5 - Memory and context governance debt

- Memory v2 graph model, scratchpad boundaries, and context aging policy are not codified in ADRs.
- Planned mitigation: `NAVI-AUTO-010`.

## AR-6 - Runtime mode coupling

- Embedded NATS solved local startup but memory-only degraded runtime mode is still absent.
- This is not a launch blocker, but remains an operational flexibility gap.

Queue of record: `docs/tasks/autonomous-agent-readiness-backlog.md`.
