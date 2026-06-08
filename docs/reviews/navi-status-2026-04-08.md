# NAVI Status Report — 2026-04-08

**Generated:** 2026-04-08 (automated weekly review)
**Commit range:** 7 days (2026-04-01 → 2026-04-08)
**Total commits this week:** 61

---

## 1. Recent Changes

This was a high-velocity week — 61 commits across two major new subsystems.

### Inference Control System (ICS) — `internal/navi/inference/` — Apr 8 (today, 10 commits)
The biggest single addition. ICS is a structured decision-making layer inserted into the AgentLoop. It adds:
- **Goal stack** (`goal_stack.go`) — maintains active goals and priority ordering
- **Focus arbiter** (`focus_arbiter.go`) — resolves which goal has runtime focus
- **Mode router** (`mode_router.go`) — selects reasoning mode based on context posture
- **Candidate evaluator** (`candidate_evaluator.go`) — scores candidate actions
- **Plan graph** (`plan_graph.go`) — explicit plan graph management
- **Recovery manager** (`recovery_manager.go`) — handles loop recovery after failures
- **Controller** (`controller.go`) — integrates all ICS components; wired into `AgentLoop`

ICS Phase 2 features (posture and arbitration management) were also included in the same day's commits. This is substantial and has zero days of runtime soak time as of this report.

### NAVI Context Orchestration System (NCOS) — `internal/navi/orchestration/` — Apr 6–7 (merged as PRs #15–17)
The second major addition. NCOS owns prompt assembly and routes all `ExecuteRun` and `ProcessTurn` calls through a typed pipeline. Key changes:
- Typed context assembly with provenance metadata
- Instruction layer compiler (replaces legacy prompt paths)
- Capability surface resolution (Phase 2)
- Orchestration trace event handling
- Runtime regression tests and unit tests for provenance/budget ordering
- Legacy direct prompt paths removed — NCOS is now the sole authority

### Earlier in the week (Apr 1–5)
- Telegram connector refactored for advanced multi-account config and improved message handling
- CLI onboarding enhanced with dynamic connector field handling
- Experience management updated with module handling and database integration
- Security fix: insecure TLS configuration in skill normalization (PR #12)
- Experience snapshot recording for session management
- Fix: NAVI hallucinating tool calls and echoing user messages

---

## 2. Current Phase & Roadmap

**Phase:** 15+ (active; CLAUDE.md is behind the current state)

The `docs/tasks/readiness-assessment.md` (2026-03-27) reflects a more accurate picture than CLAUDE.md:

| Area | Status |
|------|--------|
| Daemon startup (config, identity, NATS, SQLite, runtime, gateway) | PASS |
| Session agent loop (persona, tool use, fact extraction, summarization) | PASS |
| Directive orchestration | PASS |
| Worker execution (Coder, Critic, Strategist, Scout) | PASS — all four wired |
| Skill transport execution (internal, rest, mcp_tool, subprocess_python) | PASS |
| World model + SQLite persistence | PASS |
| Operator gateway (sessions, directives, proposals, runs, connectors, skills, etc.) | PASS |
| CLI surface (chat, ask, status, sessions, proposals, connectors, skills, runs, etc.) | PASS |
| Inference Control System (ICS) | NEW — integrated today, no soak |
| NCOS prompt orchestration pipeline | NEW — merged this week |
| Browser UI | PARTIAL — placeholder only |
| Native token streaming | PARTIAL |
| Ollama tool-calling reliability | PARTIAL — active hardening |
| Multi-tenant / production posture | PARTIAL |

---

## 3. Bug & Blocker Inventory

### Open blockers (from `docs/tasks/blockers.md`, reviewed 2026-03-27)

| Blocker | Impact |
|---------|--------|
| **Browser UI is a placeholder** | Browser-first workflows not functional; operator work depends on CLI or direct API |
| **Ollama tool-calling fallback reliability** | Some tool-requiring requests fail or underperform on weaker Ollama paths |
| **Native token streaming is partial** | Full end-to-end native streaming not complete; compatibility streaming only |
| **Production hardening is local-first** | Single-owner SQLite+NATS only; multi-tenant, isolation, deployment hardening are future work |

### Architecture risks (from `docs/tasks/architecture-risks.md`, reviewed 2026-03-08)

| Risk | Severity | Status |
|------|----------|--------|
| AR-1: Non-atomic dual-write bus + SQLite | High | Open — no outbox pattern |
| AR-2: Skill execution capability mismatch (subprocess_python limited) | Medium | Open |
| AR-3: No streaming token path | Medium | Open |
| AR-4: Event log has no pruning policy | Medium | Open — unbounded growth |
| AR-5: Memory v2 / context governance not codified in ADRs | Medium | Open |
| AR-6: No degraded runtime mode (NATS embedded only) | Low | Open |

### Outstanding critical/major work (`docs/tasks/outstanding-work.md`, reviewed 2026-03-28)

- **O-1 (Critical):** Skill execution hardening — reliability, dependency management, smoke coverage for Python-backed skills
- **O-2 (Major):** LLM responses still buffered — no streamed output
- **O-3 (Major):** Non-transactional NATS + SQLite dual-write (AR-1)
- **O-4 (Major):** SQLite event log retention not implemented
- **O-5 (Major):** Memory v2 context governance not formalized

---

## 4. Test Health

**Go toolchain not available in the review sandbox** — `make test` could not be executed. This is a sandbox limitation, not a codebase issue.

What can be observed from the repository:
- Both ICS and NCOS include test files (`*_test.go`) at the unit level. The commits describe "add NCOS unit tests", "add ICS core components and tests", "refactor ICS decision-making tests".
- The integration/runtime test surface in `internal/navi/` is extensive (`loop_test.go`, `navi_runtime_test.go`, `capability_surface_parity_test.go`, etc.)
- **Risk:** ICS was merged into `AgentLoop` today with zero days of runtime soak. Unit tests exist but integration confidence is untested in this review.

**Recommendation:** Run `make test` locally immediately and confirm ICS integration tests pass.

---

## 5. Architecture — New Packages This Week

The `internal/` tree has grown significantly beyond what CLAUDE.md describes. New packages added (some this week, some in the last few weeks):

| Package | Purpose |
|---------|---------|
| `internal/navi/inference/` | ICS — goal stack, focus, mode routing, candidate evaluation, plan graph, recovery |
| `internal/navi/orchestration/` | NCOS — prompt pipeline, capability surface, instruction compiler |
| `internal/navi/selfmod/` | Self-modification capabilities |
| `internal/navi/reflection/` | Runtime reflection / introspection |
| `internal/cognitive/` | Cognitive layer (writer, artifact materialization, knowledge, workspace enforcement) |
| `internal/worldmodel/` | World model façade over SQLite stores |
| `internal/runtime/` | Foreground runtime coordinator (inbox, runs, scheduler, metrics) |
| `internal/identity/` | Identity management + keystore |
| `internal/artifact/` | Artifact type system, codec, diff, registry, service |
| `internal/blob/` | Blob storage (parser, poller, scheduler) |
| `internal/backlog/` | Backlog management |
| `internal/llmkb/` | LLM knowledge base |
| `internal/scout/` | ScoutAgent runner |
| `internal/tool/` | Tool runtime helpers |
| `internal/command/` | Command executor + compose runner |

**CLAUDE.md is substantially out of date** — the `internal/` layout it describes is no longer accurate. This is a documentation debt item.

---

## 6. Capabilities Snapshot

| Capability | Status |
|-----------|--------|
| **CoderAgent** | Real — subscribed to CmdTaskAssign, executes via file tools + LLM |
| **CriticAgent** | Real — LLM-backed |
| **StrategistAgent** | Real — LLM-backed |
| **ScoutAgent** | Real runner wired (`internal/scout/`) — Phase 15+ |
| **Skill transports** | `internal`, `rest`, `mcp_tool`, `subprocess_python` — all implemented; reliability hardening ongoing (O-1) |
| **LLM providers** | Anthropic, OpenAI, OpenRouter, Ollama — FallbackChain active; Ollama tool-calling path being hardened |
| **Telegram connector** | Real — long-polling + webhook, multi-account; refactored this week |
| **Slack connector** | Stub |
| **ICS (new)** | Integrated into AgentLoop today — zero soak time |
| **NCOS (new)** | Sole prompt assembly authority — merged this week |

---

## 7. Top Items Needing Attention This Week

Ranked by impact:

### 1. Validate ICS + NCOS integration in production-like conditions (Critical)
ICS was merged into `AgentLoop` today; NCOS became the sole prompt authority a few days ago. Both are large, complex additions. Until they've been exercised across real user sessions and multi-turn directive workflows, regression risk is high. Run `make test` locally, then run a full smoke session with each directive mode (CHAT, ASSIST, ACT).

### 2. Ollama tool-calling fallback reliability (High)
Still the primary reliability gap for users running without cloud LLM keys. The fix for hallucinating tool calls landed this week, but the fallback path still needs hardening for the full tool-requiring request lifecycle.

### 3. Update CLAUDE.md and docs to reflect current architecture (Medium)
The `internal/` package layout, phase description, and capability snapshot in CLAUDE.md are significantly out of date. Any AI assistant or new contributor following it will have an incorrect mental model. At minimum, update the repository layout table and capabilities snapshot.

### 4. Browser UI — move beyond placeholder (Medium)
Still the most visible product gap. The gateway serves static files correctly, but there's no usable operator UI in the browser. This gates browser-first workflows entirely.

### 5. Implement SQLite event log retention (Medium)
O-4 / AR-4: the event log grows unbounded. This is a ticking time bomb for long-running local deployments. A simple TTL pruner would close this risk with minimal effort.

---

*Report generated by automated weekly status task. Go toolchain unavailable in review sandbox — `make test` was not executed; run locally to verify.*
