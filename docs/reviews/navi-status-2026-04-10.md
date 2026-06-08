# NAVI Status Report — 2026-04-10

**Generated:** Automated weekly review  
**Phase:** 15 (Active)  
**Next Action:** OMN-72 — Ollama tool-calling fallback

---

## 1. Recent Changes (Last 7 Days)

**76 commits** landed in the past week — an unusually high volume. Two major subsystems dominate the log:

### Inference Control System (ICS) — `internal/navi/inference/`
The week's primary effort. ICS is a new decision-governance layer integrated into `AgentLoop`. It adds:
- **Candidate evaluation** — multi-candidate inference path scoring before execution
- **Focus arbitration** — `focus_arbiter.go` manages competing inference goals
- **Plan graph** — explicit DAG for inference decision sequencing
- **Recovery management** — state recovery when inference fails or degrades
- **Posture and arbitration management** — capability bounding and tool authorization
- **Worker governance refactor** — inference authority now mediated through ICS, not directly in loop

ICS now has 30 files across `authority.go`, `controller.go`, `candidate_evaluator.go`, `focus_arbiter.go`, `plan_graph.go`, `recovery_manager.go`, `tool_authorization.go`, and supporting types/tests. It is integrated into `AgentLoop` as the primary decision controller.

### NCOS Phase 2 — Context Orchestration System
NCOS finalized routing and closed remaining orchestration gaps, including:
- Capability surface resolution through the full pipeline
- Instruction stack routing with provenance metadata
- Legacy prompt path removal (NCOS is now the sole authority for prompt assembly)

### Other notable commits this week
- `ToolChoice` option added to all LLM providers (Anthropic, OpenAI/OpenRouter, Ollama) — enables forcing tool-use mode per call
- `Draining Timeout Executor` added to run coordinator — improves clean shutdown under load
- `ParseWorkspaceStatus` tests added
- TLS fix in skill normalization (`🔒 Fix insecure TLS configuration`)
- Telegram connector setup refactored for advanced configuration

---

## 2. Current Phase & Roadmap

**Phase 15** is active. Key state:

| Area | Status |
|------|--------|
| CoderAgent | Real, wired, runs LLM-backed tasks |
| CriticAgent | Real, wired |
| StrategistAgent | Real, wired |
| ScoutAgent | Exists (`internal/scout/runner.go`, `writer.go`) — minimal |
| Heartbeat autonomy | Partial (infrastructure present, hardening in progress) |
| Skill transports | `internal`, `rest`, `mcp_tool`, `subprocess_python` all implemented |
| Ollama tool-calling fallback | IN_PROGRESS — OMN-72 (active next action) |
| Browser UI | Placeholder only (`web/index.html`) |
| Native streaming | Partial — compatibility streaming exists, end-to-end not complete |

**NEXT_ACTION.md** confirms OMN-72 is the in-progress item: restoring reliable tool-calling behavior when Ollama returns no usable tool calls. Telegram startup reliability and artifact tracking were completed prior to this.

---

## 3. Bug & Blocker Inventory

From `docs/tasks/blockers.md` (reviewed 2026-03-27) and `outstanding-work.md`:

### Open Blockers

| # | Issue | Severity | Impact |
|---|-------|----------|--------|
| B-1 | Browser UI is still a placeholder | High | Browser-first workflows not usable; operator work requires CLI or API |
| B-2 | Ollama tool-calling fallback reliability | High | Tool-requiring requests can stall or devolve to plain text on Ollama paths |
| B-3 | Native token streaming partial | Medium | No uniform end-to-end streaming; compatibility events only |
| B-4 | Production hardening (SQLite + embedded NATS, local-first) | Medium | Not suitable for multi-tenant or broader production hosting |

### Outstanding Work (from `outstanding-work.md`)

| ID | Issue | Priority |
|----|-------|----------|
| O-1 | Skill execution hardening — reliability gaps remain for external-runtime skills | Critical |
| O-2 | No streamed LLM output from providers | Major |
| O-3 | NATS + SQLite dual-write not transactional (no outbox pattern) | Major |
| O-4 | SQLite event table can grow unbounded (no pruning) | Major |
| O-5 | Memory v2/context governance not formalized in ADRs | Major |

No blockers have been resolved in the last 7 days (the week was absorbed by ICS development).

---

## 4. Test Health

`make test` could not be executed in this review environment (Go toolchain not available in the automated runner sandbox). **Manual verification required.**

The commit log shows active test authorship this week:
- ICS test files accompany most new subsystem files (controller_test.go, candidate_evaluator_test.go, focus_arbiter_test.go, goal_stack_test.go, plan_graph_test.go, recovery_manager_test.go, mode_router_test.go)
- `ParseWorkspaceStatus` tests added
- Runtime coordinator error handling tests added

**Action required:** Run `make test` locally before merging the `feat/inference-control-system` branch to confirm no regressions from ICS integration into AgentLoop.

---

## 5. Architecture Check

### New packages since last period

| Package | Description |
|---------|-------------|
| `internal/navi/inference/` | **ICS** — 30-file inference governance subsystem (new this week) |
| `internal/cognitive/` | Cognitive layer (present, newly visible in scan) |
| `internal/worldmodel/` | World model facade (SQLite-backed) |
| `internal/llmkb/` | LLM knowledge base |
| `internal/backlog/` | Backlog tracking infrastructure |
| `internal/blob/` | Blob storage |
| `internal/tool/` | Tool infrastructure |
| `internal/command/` | Command execution layer |
| `internal/prompts/` | Prompt management (post-NCOS refactor) |

The codebase has grown significantly in scope. The `internal/navi/` directory now contains ~50 files directly plus multiple subdirectories (inference, skill, heartbeat, hooks, plugin, filetools, inference, orchestration, experience, selfmod, reflection). This is a meaningful complexity increase.

### Structural notes
- AgentLoop has been refactored multiple times this week (ICS integration, runtime coordinator error handling, executor updates). High churn on a core file warrants careful review.
- NCOS is now the sole authority for prompt assembly — legacy prompt paths were deleted.

---

## 6. Capabilities Snapshot

| Capability | Status |
|------------|--------|
| CoderAgent | ✅ Real LLM-backed, subscribes to CmdTaskAssign |
| CriticAgent | ✅ Real LLM-backed |
| StrategistAgent | ✅ Real LLM-backed |
| ScoutAgent | ⚠️ Skeleton only (`runner.go`, `writer.go`) |
| Skill: `internal` transport | ✅ Implemented |
| Skill: `rest` transport | ✅ Implemented |
| Skill: `mcp_tool` transport | ✅ Implemented |
| Skill: `subprocess_python` transport | ✅ Implemented |
| LLM: Anthropic | ✅ |
| LLM: OpenAI / OpenRouter | ✅ |
| LLM: Ollama | ✅ (tool-calling hardening in progress) |
| LLM: FallbackChain | ✅ |
| LLM: ToolChoice option | ✅ Added this week |
| Connector: Telegram | ✅ Long-polling + webhook, multi-account |
| Connector: Slack | ⚠️ Stub only |
| Browser UI | ⚠️ Placeholder only |
| Native streaming | ⚠️ Partial |
| Heartbeat autonomy | ⚠️ Infrastructure present, not fully hardened |

---

## 7. Top Items Needing Attention This Week

Ranked by impact:

**1. Merge and validate `feat/inference-control-system` — HIGH RISK**  
76 commits on a new 30-file subsystem integrated directly into `AgentLoop` is the highest-risk change of the week. Before merging to master: run the full test suite, do a manual smoke test of a tool-using session end-to-end, and confirm ICS doesn't change observable behavior for working paths. The sheer commit velocity suggests the design was still stabilizing.

**2. Complete OMN-72: Ollama tool-calling fallback**  
Active next action. This directly affects reliability of tool-using sessions on the most common local-inference path. The `ToolChoice` option added this week to LLM providers is likely a prerequisite — wire it into the fallback logic and add the Telegram-triggered regression tests called out in NEXT_ACTION.md.

**3. Run `make test` and fix any regressions**  
The automated runner couldn't execute Go tests. With AgentLoop touched in multiple commits this week, confirming the test suite is clean before continuing is critical. Pay special attention to `loop_test.go` and the new ICS tests.

**4. Resolve O-3: NATS + SQLite dual-write consistency**  
The current publish path is not transactional. Under load or crash, events can be published to NATS without a corresponding SQLite write (or vice versa). This is a latent data-integrity bug that will surface as the system handles more autonomous work. Implementing an outbox pattern is the standard fix.

**5. Address O-4: SQLite event log pruning**  
The event table will grow unbounded in long-running deployments. A simple time-based retention policy (e.g., delete events older than 30 days) would prevent this from becoming an operational problem. Low implementation cost, high risk if left open as autonomous operation ramps up.

---

*Report generated automatically from git log, docs/tasks/, and codebase scan. Tests not executed — run `make test` manually to confirm current health.*
