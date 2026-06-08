# NAVI Status Report — 2026-05-08

**Generated:** Automated weekly review  
**Phase:** 15 (active)  
**Branch:** master

---

## 1. Recent Changes (past 7 days)

Four commits landed this week, all significant:

| Date | Commit | Summary |
|------|--------|---------|
| 2026-05-08 | `418c65a` | **Feat/programmer plugin rework (#48)** — Consolidates plugin components under `plugins/`, adds `ManifestDiagnostic` for resilient loading (bad manifests skipped, not fatal), removes built-in Telegram/Slack from hardcoded registration, adds NAVI Console design docs + gateway hosting ADR + Console V1 spec |
| 2026-05-01 | `8d61451` | **Telegram media metadata** — New `parseMediaMetadata` method handles photos, videos, audio, documents, stickers; rewrote ~420 lines of `bot.go`; new test file `bot_migration_test.go` |
| 2026-05-01 | `2b6785b` | **Anthropic provider extension** — Full `extensions/anthropic` package: Messages API, extended thinking, vision content blocks, native tool use, streaming. Companion `extensions/openai` package added in same commit. Marks O-LLM-2 complete. ~2,700 LOC of new production + test code |
| 2026-05-01 | `ea7b9a7` | **Ollama adapter retirement** — Removes `internal/llm/ollama.go` and `ollama_control.go`; introduces `OllamaFactory` func var pattern; migrates integration tests to `extensions/ollama/e2e_test.go` (build tag: `integration`). O-LLM-1 marked DONE |

**Notable structural change from earlier in the branch:** ~375 files and 32K LOC added across the `plugins/` rework and `web-src/navi-console/` — a React/TypeScript NAVI Console frontend now exists under `web-src/navi-console/src/` (components, pages, hooks, API types, timeline types). This is a major milestone — the browser UI is no longer just a placeholder at the source level.

---

## 2. Current Phase & Roadmap

**Phase 15** is active. The primary NEXT_ACTION target is **OMN-72** (Tool calling fallback for Ollama — `NAVI-BF-009`).

Key phase status:
- Phases 1–14: Done (infra, orchestrator, coder/critic/strategist workers)
- Phase 15 active items: ScoutAgent hardening, Ollama tool-call fallback, non-file skill execution hardening
- Phase 16+: Postgres, sandboxing, skills marketplace, production hardening

The readiness assessment (2026-03-27) puts the system at **"usable for local/single-owner operator workflows"** — not yet broadly production-hardened.

---

## 3. Bug & Blocker Inventory

### Open Blockers (from `docs/tasks/blockers.md`)

| # | Blocker | Impact |
|---|---------|--------|
| B-1 | **Browser UI still placeholder** in `web/` (though `web-src/navi-console` now has real React source — unclear if built/deployed) | Browser-first workflows not usable |
| B-2 | **Ollama tool-calling fallback not yet hardened** (NEXT_ACTION target) | Tool-requiring requests stall or devolve to plain text on Ollama models |
| B-3 | **Native token streaming partial** | No fully uniform streaming across the stack |
| B-4 | **Production hardening is local-first** | Not ready for multi-tenant or broad deployment |

### Critical Bugs (from `TASK_QUEUE.md`)

| ID | Title | Status |
|----|-------|--------|
| NAVI-BF-008 | LLM Output Runaway — Infinite Repetition Loop | PENDING |
| NAVI-BF-009 | Tool Calling Non-Functional with Ollama | PENDING (NEXT_ACTION) |
| NAVI-BF-004 | Duplicate Orphaned "Thinking…" Placeholders (every message) | PENDING |
| NAVI-BF-005 | Silent Timeout — No Error Feedback, Stuck Placeholders | PENDING |
| NAVI-BF-002 | Telegram Session Creation Timeout (gateway unreachable) | PENDING |
| NAVI-BF-003 | LLM Model Switching Fails via Telegram (catalog mismatch) | PENDING |

Six CRITICAL bugs remain open. Three directly affect unattended/Telegram operation (BF-002, BF-003, BF-009). BF-008 (infinite repetition) and BF-004/BF-005 (thinking placeholders + silent timeouts) degrade the user-facing experience meaningfully.

There are also **15 open PET web client bugs** — most rated Urgent or High — covering wizard deadlock, spinner hang, double bootstrap, stale state on rapid messages, and security (loopback bypass).

---

## 4. Test Health

**`make test` could not be executed** — Go is not installed in the automated review environment. This is a gap in the review process; test results could not be verified this run.

**Observed signal from commits:**
- `extensions/anthropic/provider_test.go` (613 LOC) and `extensions/openai/provider_test.go` (503 LOC) were added this week — good test coverage on new provider extensions
- `extensions/ollama/e2e_test.go` (189 LOC, build tag: `integration`) was added for Ollama native path
- `bot_migration_test.go` (59 LOC) covers Telegram media parsing
- `internal/llm/main_test.go` added `TestMain` with Ollama stub to prevent panics in unit tests

No regressions were reported in commit messages. The Makefile test target is `go test ./cmd/... ./internal/... ./connectors/... ./plugins/... -count=1` — note `extensions/` is not in this default target.

---

## 5. Architecture Check

### New/notable packages since last status reports:

| Package | Status | Notes |
|---------|--------|-------|
| `internal/cognitive` | Exists | Interfaces used by all four agent runners |
| `internal/command` | Exists | Executor, compose runner, compensation |
| `internal/runtime` | Exists | Foreground runtime coordinator |
| `internal/worldmodel` | Exists | Façade over store; artifact materialization |
| `internal/presence` | Exists | Presence/refresh rate control |
| `internal/llmkb` | Exists | LLM knowledge base |
| `internal/ai` | Exists | AI subsystem |
| `internal/backlog` | Exists | Backlog tracking |
| `web-src/navi-console/` | **NEW this week** | React/TypeScript console frontend with full component tree, API types, hooks, timeline types |
| `plugins/navi-programmer` | **Reworked this week** | Plugin-based programmer; `ManifestDiagnostic` pattern for resilient loading |

The move to `plugins/` for LLM providers (`llm-anthropic`, `llm-openai`, `llm-ollama`, `llm-router`) is a meaningful architectural shift — providers are now first-class plugins with manifests rather than internal packages. The `OllamaFactory` func-var pattern is the bridge mechanism during this transition.

---

## 6. Capabilities Snapshot

| Capability | Status |
|------------|--------|
| **CoderAgent** | Real LLM-backed; `internal/coder/runner.go` subscribes to `CmdTaskAssign`, executes with file tools + LLM |
| **CriticAgent** | Real LLM-backed; `internal/critic/runner.go` wired; uses filetools for path checking |
| **StrategistAgent** | Real LLM-backed; `internal/strategist/runner.go` wired |
| **ScoutAgent** | Wired (`internal/scout/runner.go` exists with full deps); Phase 15 hardening in progress |
| **Skill transports** | `internal`, `rest`, `mcp_tool`, `subprocess_python` all implemented; reliability hardening ongoing (O-1) |
| **LLM Providers** | Anthropic ✅ (new native extension this week), OpenAI ✅ (new native extension), Ollama ✅ (OllamaFactory pattern), OpenRouter ✅ (via OpenAI-compat path); FallbackChain intact |
| **Streaming** | Provider-level streaming code exists; end-to-end native streaming not complete |
| **Connectors** | Telegram: real (long-polling + webhook, multi-account, media parsing enhanced this week); Slack: stub |
| **Browser UI** | `web/index.html` still placeholder served by gateway; `web-src/navi-console/` React source exists but build/deploy status unclear |

---

## 7. Top Items Needing Attention This Week

Ranked by impact on unattended/Telegram operation and overall reliability:

### 1. Verify and deploy the NAVI Console frontend
`web-src/navi-console/` has a full React/TypeScript source tree but `web/index.html` is still the placeholder being served. If the build step exists (check for `vite.config.ts` build output target), run it and wire the built output to `web/`. This converts B-1 from a blocker to resolved.

### 2. Complete Ollama tool-calling fallback (OMN-72 / NAVI-BF-009)
The NEXT_ACTION is already pointing here. This is the highest-impact reliability gap for unattended operation. Without it, Ollama-backed agentic tasks devolve to plain text silently.

### 3. Fix infinite repetition loop (NAVI-BF-008)
A CRITICAL bug with no assigned priority date. An LLM that enters an output runaway loop will either exhaust cost ceiling or require manual intervention — both are bad for always-on operation. Needs investigation and a loop-detection guard in the agent loop or governor.

### 4. Add `extensions/` to the Makefile test target
`make test` does not cover `extensions/anthropic`, `extensions/openai`, or `extensions/ollama`. Three new packages with ~1,300 LOC of production code landed this week and won't run in CI unless the Makefile target is updated. Quick fix: add `./extensions/...` to the test command.

### 5. Audit PET web client bug list for Telegram-equivalent workarounds
15 PET bugs are open, including Urgent items (wizard deadlock, spinner hang, factory reset leaving `claimed:true`). Until the Console is deployed, Telegram is the primary operator interface — but BF-002 and BF-003 (Telegram session timeout and model switching failure) mean even that path is unreliable. Consider triaging these Telegram-path bugs before the PET list.

---

*Next automated review: 2026-05-15*
