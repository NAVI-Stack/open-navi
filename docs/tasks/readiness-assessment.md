# Readiness Assessment

**Review Date:** 2026-05-29
**Scope:** current local-first runtime, worker, skill, gateway, and operator surfaces

## Executive Summary

NAVI is now meaningfully beyond the earlier "core skeleton" stage. The daemon, runtime coordinator, worker subscriptions, skill transports, world-model facade, operator gateway, onboarding flow, and CLI are all live in code.

The system is usable today for local development and operator-driven workflows. The main remaining readiness issues are reliability and product-completeness issues rather than missing core subsystems:

- browser Console is feature-complete for chat but lacks e2e/integration test coverage
- native token streaming is partial
- Ollama tool-calling fallback still needs hardening
- deployment posture is still local-first rather than broadly production-hardened

## Current Snapshot

| Area | Current status | Verdict |
|------|----------------|---------|
| Daemon startup | `navid` boots config, identity, SQLite, embedded NATS, runtime, orchestrator, gateway, workers, connectors, heartbeat, and reflection | PASS |
| Foreground runtime | `internal/runtime` supports inbox acceptance, runs, pause/resume, proposal resolution, cancellation, scheduled message delivery, and metrics | PASS |
| Session agent loop | `internal/navi` supports persona-aware sessions, prompt rendering, tool use, fact extraction, summarization, and gap detection | PASS |
| Directive orchestration | `internal/orchestrator` reconstructs directive state, plans work, and assigns tasks to workers | PASS |
| Worker execution | coder, critic, strategist, and scout runners are wired and subscribed to task assignments | PASS |
| Skill transport execution | `internal`, `rest`, `mcp_tool`, `subprocess_python`, and generic `subprocess` transports are implemented | PASS |
| World model and persistence | SQLite-backed stores plus the `internal/worldmodel` facade are live | PASS |
| Operator gateway | session, directive, proposal, run, activity, LLM, connector, skill, tool, knowledge, and error APIs are live | PASS |
| CLI operator surface | chat, ask, init, status, sessions, models, logs, proposals, connectors, skills, activity, runs, doctor, and console exist | PASS |
| Connector system | Telegram, Slack, bridge connectors, and workspace subprocess connectors are supported | PASS |
| Browser Console | React 19 + Vite app (`web-src/navi-console/`) served from `web/`; chat UX feature-complete (markdown, copy, edit/resend, regenerate, continue, variants, feedback, tool chips, streaming states). Gap: no e2e harness, `ChatPage.tsx` untested | PARTIAL |
| Native streaming | compatibility streaming exists, but full end-to-end native token streaming is not finished | PARTIAL |
| Ollama tool-calling reliability | active hardening item | PARTIAL |
| Multi-tenant / production posture | local-first, single-owner oriented | PARTIAL |

## Readiness Verdict

- Local development readiness: strong
- Single-owner operator workflow readiness: usable
- Broad production readiness: not yet there

The major change from earlier assessments is that skill execution breadth is no longer the primary blocker. The current focus has moved to runtime reliability, UI completeness, and production hardening.

## What Should Happen Next

1. Finish the Ollama tool-calling fallback work so tool-requiring requests degrade predictably.
2. Add e2e/integration coverage for the browser Console (especially `ChatPage.tsx`) and continue product-polish hardening.
3. Continue improving streaming behavior and operator feedback.
4. Keep exercising the gateway, worker, and connector paths with integration and end-to-end coverage.

[tasks INDEX](INDEX.md) | [docs INDEX](../INDEX.md)
