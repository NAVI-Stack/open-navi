# NAVI Status Report — 2026-05-28

**Generated:** 2026-05-28 (automated weekly review)  
**Branch:** fix/console-plugins  
**Scope:** Past 7 days of commits + docs audit + architecture snapshot

---

## 1. Recent Changes (Last 7 Days)

50 commits landed this week. The three dominant themes:

### Model Browser (highest velocity)
Four commits built and polished a new **model browser UI** in navi-console — a rail/panel for browsing, adding, and configuring LLM providers and models in real time. This includes backend wiring (`internal/llm`, `internal/gateway`) for live provider configuration without daemon restart.

**Key commits:**
- `5d8afad` — navi-console: model browser UX polish pass
- `018598d` — llm,gateway,navi-console: real-time provider configuration and model browser rail
- `eb4ca636` — navi-console,llm: wire model browser TODOs to backend
- `cebd7a5a` — navi-console: add model browser to chat composer

### Chat Phase 2 Features
Edit-and-resend, assistant feedback (👍/👎), regenerate last reply, and message variants were all shipped. The handoff prompts for remaining Phase 2 work (Continue, Variants, tool-invocation streaming) are documented in `docs/tasks/chat-phase2-handoffs.md`.

**Key commits:**
- `aa5a00d` — chat deletion, continuation, and message variants
- `83b738a` — edit-and-resend
- `692f3af` — assistant message feedback
- `cd10306` — regenerate last reply

### Infrastructure / Governance
- `f3c29fe` — `MaxRepetitions` config added to Governor
- `f2627315` — Runtime repo/workspace binding enforced for programmer mutations
- `0e3ae169` — Self-update workflow for programmer tasks
- `f2627315` — ADR-011 (Memory v2 / Context Window Governance) — **accepted 2026-05-25**

---

## 2. Current Phase & Roadmap

**Phase:** Post-launch hardening + UI buildout. Core runtime is functional; the current focus is:

1. **Chat surface** — Phase 2 features mostly shipped; Continue/Variants/tool streaming remain
2. **Model browser** — Just shipped real-time provider config
3. **Browser UI** — Still the largest open blocker (index.html placeholder)
4. **Memory v2** — ADR-011 accepted this week; implementation not yet started
5. **Streaming** — Partial; compatibility layer exists but native end-to-end is incomplete

**From `docs/tasks/readiness-assessment.md`:**
- Local dev readiness: **strong**
- Single-owner operator workflow: **usable**
- Broad production readiness: **not yet**

---

## 3. Bug & Blocker Inventory

### Open Blockers (`docs/tasks/blockers.md`)

| # | Blocker | Severity |
|---|---------|----------|
| B-1 | Browser UI is still a placeholder (`web/index.html`) | **High** — browser-first workflows blocked |
| B-2 | Ollama tool-calling fallback reliability hardening | **High** — tool-requiring requests can fail on weak models |
| B-3 | Native token streaming is partial | **Medium** — UX degraded for long replies |
| B-4 | Production hardening (multi-tenant, deployment) | **Low-Medium** — local-first only |

### Open Architecture Risks (`docs/tasks/architecture-risks.md`)

| # | Risk | Impact |
|---|------|--------|
| AR-1 | Non-atomic dual-write (bus + SQLite, no outbox) | Event/replay divergence under failure |
| AR-2 | Skill execution reliability/isolation for subprocess skills | Reliability gap |
| AR-3 | No streaming token path at provider level | Long reply latency |
| AR-4 | Event log retention unimplemented (unbounded growth) | Ops/storage risk |
| AR-5 | Memory v2 not implemented (ADR-011 accepted, not built) | Autonomy ceiling |
| AR-6 | No memory-only degraded runtime mode | Operational flexibility gap |

### Outstanding Work (`docs/tasks/outstanding-work.md`)

- **O-1 (Critical):** Skill execution hardening — subprocess/Python reliability, sandbox, smoke coverage
- **O-2 (Major):** No streamed LLM output (full buffer before return)
- **O-3 (Major):** NATS + SQLite dual-write inconsistency risk
- **O-4 (Major):** Event retention policy not implemented
- **O-5 (Major):** Memory v2 / context governance not yet formalized in code

---

## 4. Test Health

**`make test` could not be executed** — Go toolchain is not installed in the CI sandbox environment used for this report. Tests must be run natively or via CI.

**Coverage indicators from repo scan:**
- **234 test files** found across `internal/`, `cmd/`, `connectors/` (excluding `example-code/`)
- Recent test additions this week: `chat_phase2_actions_test.go`, `chat_variants_test.go`, `chat_feedback_test.go`, `loop_test.go`, `navi_test.go`, `tool_registry_test.go`, `compaction_runtime_test.go`, `fact_extractor_test.go`
- Commit `5ac1c536` explicitly restored unit-test coverage lost in a prior core-cleanup pass
- CI runs `go test ./cmd/... ./internal/... ./connectors/... -count=1` on push + docker compose smoke test

**Recommendation:** Verify CI is green on `fix/console-plugins` before merging — the model browser rail and chat Phase 2 features touched multiple packages concurrently.

---

## 5. Architecture Snapshot

### New/Active Internal Packages (since ~2 weeks ago)

| Package | Status | Notes |
|---------|--------|-------|
| `internal/navi/inference` | New | Inference-layer abstraction (split from loop) |
| `internal/cron` | Active | Cron/scheduler service with backoff, stagger, timers |
| `internal/artifact` | Active | Codec, diff, registry, service for artifact management |
| `internal/capability` | Active | UI capability surface |
| `internal/runtime` | Active | Runtime coordinator, classifier |
| `internal/worldmodel` | Active | World model façade (projects, knowledge, sandbox) |
| `internal/cognitive` | Active | Execution recorder, directive writer |
| `internal/prompts` | Active | Prompt manager + defaults |

### Structural Changes of Note
- **Plugins directory** (`plugins/`) is a first-class directory alongside `internal/` — LLM providers (`llm-anthropic`, `llm-ollama`, `llm-openai`, `llm-router`, `llm-embed`) and agent workers (`navi-coder`, `navi-programmer`, `navi-search`, `navi-contacts`, etc.) are plugin packages
- **`internal/navi/skill`** — new testdata for `python-health-check` SKILL.yaml + bridge.py added this week
- **Model browser** (`web-src/navi-console/src/components/model-browser/`) — fully new component tree landed this week

---

## 6. Capabilities Snapshot

| Capability | Status |
|------------|--------|
| **CoderAgent** | Live — `internal/coder/runner.go`, subscribes to `CmdTaskAssign` |
| **CriticAgent** | Live — `internal/critic/runner.go` |
| **StrategistAgent** | Live — `internal/strategist/runner.go` |
| **ScoutAgent** | Live — `internal/scout/runner.go` |
| **Skill transports** | `internal`, `rest`, `mcp_tool`, `subprocess_python`, `subprocess` all implemented; reliability hardening ongoing |
| **LLM providers** | Anthropic, OpenAI, Ollama, OpenRouter via plugin packages; `FallbackChain` active; real-time model config via new model browser |
| **Telegram connector** | Real — long-polling + webhook, multi-account, group config, multimodal pipeline (enhanced this week) |
| **Slack connector** | Stub only |
| **Gateway** | Full API surface live (session, directive, proposal, run, LLM, connector, skill, tool, knowledge, activity, error endpoints) |
| **Browser UI** | **Placeholder only** — major open blocker |
| **CLI** | Live — chat, ask, init, status, sessions, models, logs, proposals, connectors, skills, activity, runs, doctor, console |
| **Streaming** | Compatibility layer live; native end-to-end streaming **not complete** |

---

## 7. Top Items Needing Attention This Week

**Ranked by impact:**

### 1. Verify CI green on `fix/console-plugins` before merge
The model browser + chat Phase 2 features touched `internal/llm`, `internal/gateway`, `internal/navi`, and the frontend simultaneously. High integration risk. Confirm CI passes before merging.

### 2. Browser UI — move off the placeholder
B-1 is the largest user-facing blocker. `web/index.html` is a placeholder. The navi-console frontend exists in `web-src/` but is not being served. Setting up the build pipeline to populate `web/` from `web-src/` would unblock browser-first workflows entirely.

### 3. Begin Memory v2 implementation (ADR-011 accepted)
ADR-011 was accepted 2026-05-25 and is the biggest architectural investment on the autonomy roadmap. The sooner implementation starts, the sooner deeper user modeling and cross-session continuity become available. First step: entity schema + World Model store layer.

### 4. Ollama tool-calling fallback hardening
B-2 / O-LLM-2 — tool-requiring requests can silently fail or underperform on Ollama paths. This affects unattended operation quality directly. Prioritize before expanding autonomous workloads.

### 5. Chat Phase 2 — remaining features (Continue, Variants, tool-invocation streaming)
Handoff prompts are ready in `docs/tasks/chat-phase2-handoffs.md`. These are well-scoped and would complete the chat surface. Continue is the highest-value item (resumes generation without losing prior reply).

---

*Report generated by automated weekly review task (`navi-weekly-status`).*
