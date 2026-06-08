# Current Blockers

**Review Date:** 2026-05-29
**Definition:** issues that still materially reduce launch confidence or day-to-day reliability even though the core runtime is functional.

## Open Blockers

### Browser console needs hardening and end-to-end test coverage

> Status: **Partially Implemented / Untested at the e2e level.** This is no longer a "placeholder UI" gap — the console is real. The remaining work is polish, hardening, and automated coverage.

Evidence:

- the browser Console is a real React 19 + Vite 6 app under `web-src/navi-console/` (`web-src/navi-console/package.json`) that builds into `web/` (`web-src/navi-console/vite.config.ts` `outDir: '../../web'`)
- the gateway serves the built console from `web/` and redirects to `/onboarding` on first run (`internal/gateway/server.go`; see `docs/specs/gateway-api.md` `GET /`)
- the chat surface implements markdown rendering, code-copy, edit/resend, regenerate, continue, response variants, feedback, tool chips, and streaming/pending/failure states (`web-src/navi-console/src/components/chat/ChatMessage.tsx`, `web-src/navi-console/src/components/chat/MarkdownRenderer.tsx`)
- component tests exist for `ChatMessage` and `MarkdownRenderer`, but there is **no** test for the orchestrating `ChatPage.tsx` and **no** e2e harness (no Playwright/Cypress config)

Impact:

- browser-first workflows are usable, but the main chat orchestration layer is untested and there is no end-to-end coverage of send/receive, live events, variant switching, or feedback flows
- operator-grade polish (error surfaces, edge-case states) is still maturing

### Ollama tool-calling fallback reliability is still being hardened

Evidence:

- the active next-action item is still focused on restoring reliable agentic behavior when an Ollama model returns no usable tool calls
- the codebase includes routing, tools, and worker execution, but not all Ollama paths degrade cleanly yet

Impact:

- some tool-requiring requests can still underperform or fail to execute correctly on weaker Ollama model paths

### Native token streaming is still partial

Evidence:

- the gateway exposes a live feed and the OpenAI-compatible surface simulates streaming
- provider-native token streaming is not yet a complete, end-to-end runtime path across the stack

Impact:

- clients can receive useful progress and compatibility events, but not a fully uniform native streaming experience

### Production hardening remains behind local-first functionality

Evidence:

- the default stack is SQLite plus embedded NATS
- loopback-friendly auth and local filesystem paths are first-class
- the codebase is optimized for a single-owner local deployment

Impact:

- NAVI is strong for local development and operator workflows
- broader production hosting, multi-tenant isolation, and full deployment hardening are still future work

## Recently Resolved Or Reclassified

### Missing `web/` directory

Resolved. The gateway serves `web/` and no longer fails on a missing static directory.

### Missing Compose definition for NaviD

Resolved. `compose.yml` (permissive) and `compose.strict.yml` (isolated) are the supported Docker-mode NaviD stacks; Makefile targets wrap `docker compose`.

### CLI and server port mismatch

Resolved. The daemon and CLI both default to port `6284`.

### No local NATS fallback

Resolved. `nats.url: "embedded"` is the standard local default and `cmd/navid` starts embedded NATS automatically.

### Skill transport execution gap

Resolved as an execution gap. The current code executes:

- `internal`
- `rest`
- `mcp_tool`
- `subprocess_python`

Hardening still remains around external dependencies, governance, and runtime reliability, but the old "non-file skills are not executable" statement is no longer accurate.

### CLI logs build regression

Resolved. The CLI logging path is implemented against the current WebSocket client library and the command surface is live.

[tasks INDEX](INDEX.md) | [docs INDEX](../INDEX.md)
