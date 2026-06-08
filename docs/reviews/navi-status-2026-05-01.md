# NAVI Status Report — 2026-05-01

**Generated:** automated weekly review  
**Phase:** 15 (active) — ScoutAgent, heartbeat autonomy, skill execution hardening  
**Codebase:** `github.com/ceoai/navi`, Go 1.24+

---

## 1. Recent Changes (past 7 days — 22 commits)

### High-impact

| Commit | Summary |
|--------|---------|
| `2b6785b` | **`extensions/anthropic` provider** — native Anthropic Messages API, SSE streaming, extended thinking, vision content blocks, tool use; 19 httptest unit tests. Replaces `internal/llm/anthropic.go`. |
| `ea7b9a7` | **OllamaFactory pattern** — legacy `internal/llm/ollama.go` and `ollama_control.go` retired; `OllamaFactory` var now lives in `internal/llm/ollama_factory.go`; panics at startup if not set. |
| `1e3fe63` / `b2319b9` | **NAVI Programmer plugin** — plugin scaffolded under `plugins/navi-programmer/` with six skill families (`repo-inspect`, `file-mutate`, `patch-apply`, `run-validation`, `git-lifecycle`, `task-normalize`) and three workflow contracts (`bounded-mutation`, `ticket-driven-coding`, `self-update-candidate`). |
| `18660d3` / `87b24c4` / `7288b3b` | **Project-aware coding** — project session management added to gateway (`internal/gateway/projects.go`), operator overview endpoint, project-scoped coding execution. |
| `a1887c2` | **SQL injection fix** in schema migrations (security patch — high priority). |
| `92b99f6` | Consolidated connectors and plugins into `extensions/` directory — then partially reorganized. Directory structure still settling (see §7). |

### Notable smaller changes

- `8d61451` — Media metadata parsing + unit tests for Telegram bot extension.
- `6972570` — Presence service refresh rate control.
- `64b9edc` / `603b2c4` — Session store message handling improvements.
- `2f5a870` — Experience: support locked owner modules.
- `62cdf4a` — Schema: `ValidateEventKind` tests + `FactInferenceDecisionTrace` mapping fix.
- `8515d39` — Removed deprecated `fix-ollama.sh`; enhanced onboarding logic.

---

## 2. Current Phase & Roadmap

**Phase 15 is active.** CLAUDE.md and outstanding-work.md describe the remaining focus areas:

- ScoutAgent hardening (`internal/scout/runner.go` exists but was listed as stub in CLAUDE.md — confirm real vs stub status)
- Heartbeat autonomy (background scheduler, `internal/navi/heartbeat/`)
- Non-file skill execution reliability (`subprocess_python`, `mcp_tool`, `rest` transports)
- NAVI Programmer plugin is the new dominant workstream — first-end-to-end plugin proving ground

Next documented action (`plugins/navi-programmer/NEXT_ACTION.md`): live-verify `navi.programmer` discovery in a running NaviD instance and invoke `repo-inspect` end-to-end via `subprocess_python`. This has **never been tested against a live instance**.

---

## 3. Bug & Blocker Inventory

### Open blockers (from `docs/tasks/blockers.md`)

| # | Blocker | Severity | Impact |
|---|---------|----------|--------|
| B-1 | Browser UI is a placeholder | High | All operator browser workflows blocked; CLI/API only |
| B-2 | Ollama tool-calling fallback not fully hardened | High | Requests requiring tools may fail silently on weaker Ollama models |
| B-3 | Native token streaming incomplete | Medium | Full-response buffering; degrades perceived responsiveness in all connectors |
| B-4 | Production hardening behind local-first | Low/Future | SQLite + embedded NATS; single-owner only; multi-tenant and hosting unready |

### Open architecture risks (from `docs/tasks/architecture-risks.md`)

| # | Risk | Linked work |
|---|------|------------|
| AR-1 | Non-atomic NATS + SQLite dual-write | NAVI-AUTO-008 |
| AR-3 | No streaming token path | NAVI-AUTO-007 |
| AR-4 | Event log retention / no pruning | NAVI-AUTO-009 |
| AR-5 | Memory v2 / context governance not in ADRs | NAVI-AUTO-010 |

### Outstanding critical work (from `docs/tasks/outstanding-work.md`)

| ID | Item |
|----|------|
| O-1 | Skill execution hardening — `subprocess_python`, `mcp_tool` transports have first impl but need reliability/smoke coverage |
| O-2 | No streamed LLM output (NAVI-AUTO-007) |
| O-3 | NATS + SQLite dual-write inconsistency (NAVI-AUTO-008) |
| O-LLM-3 | `extensions/openai` provider not yet built — mirrors Ollama/Anthropic pattern |

---

## 4. Test Health

**Tests could not be run** — Go toolchain is not available in the automated review environment. The sandbox only has shell/Python.

**Mitigation:** Run `make test` manually before the next commit. Last known test surface: `./cmd/...`, `./internal/...`, `./connectors/...`.

**Risk flag:** The LLM layer refactor (Anthropic + Ollama moved to extension factory pattern) is a structural change that warrants a full test pass. If `AnthropicFactory` or `OllamaFactory` are not set before `FromConfig`, both will panic at startup — integration test coverage of this wiring path is important.

---

## 5. Architecture Changes (new packages vs CLAUDE.md baseline)

CLAUDE.md lists the following as the known `internal/` layout. New or undocumented packages now present:

| Package | Status |
|---------|--------|
| `internal/worldmodel` | Active — project and entity model; multiple commits this week |
| `internal/presence` | Active — presence service, refresh rate added this week |
| `internal/runtime` | Present — runtime executor with interrupt handling |
| `internal/cognitive` | Present — not in CLAUDE.md; likely memory/reasoning layer |
| `internal/artifact` | Present — not in CLAUDE.md |
| `internal/backlog` | Present — not in CLAUDE.md |
| `internal/blob` | Present — not in CLAUDE.md |
| `internal/cliui` | Present — not in CLAUDE.md |
| `internal/command` | Present — executor, compose, compensation |
| `internal/identity` | Present — not in CLAUDE.md |
| `internal/llmkb` | Present — LLM knowledge base; not in CLAUDE.md |
| `internal/onboarding` | Present — not in CLAUDE.md |
| `internal/prompts` | Present — not in CLAUDE.md |
| `internal/tool` | Present — not in CLAUDE.md |

**CLAUDE.md is significantly out of date.** The repository layout section does not reflect the current `internal/` surface. Several of these packages are load-bearing (worldmodel, command, runtime) and the docs gap creates onboarding and AI assistant friction.

**`plugins/` directory** now has 15 plugins including `navi-programmer`, `llm-anthropic`, `llm-ollama`, `llm-openai`, `telegram`, `slack`, `github`, `navi-search`, and others. No corresponding `extensions/` directory exists on disk despite commit `92b99f6` referencing consolidation — the reorganization is incomplete.

**Git repository has a corrupted `.git/packed-refs` file** (truncated line for ref `refs/heads/codex/fix-ics-re`). All `git log` calls after the initial scan failed with `fatal: unterminated line in .git/packed-refs`. This needs to be repaired: `git pack-refs --all` or manually fix the truncated line.

---

## 6. Capabilities Snapshot

| Capability | Status |
|-----------|--------|
| **CoderAgent** (`internal/coder/runner.go`) | Real — executes tasks with file tools + LLM; subscribes to `CmdTaskAssign` |
| **CriticAgent** (`internal/critic/runner.go`) | Real — LLM-backed |
| **StrategistAgent** (`internal/strategist/runner.go`) | Real — LLM-backed |
| **ScoutAgent** (`internal/scout/runner.go`) | File exists — CLAUDE.md listed it as stub; actual status unclear without running tests |
| **Skill transports** | `internal`, `rest`, `mcp_tool`, `subprocess_python` all implemented; hardening ongoing |
| **LLM providers** | Anthropic (new native extension), Ollama (factory pattern, legacy deleted), OpenAI (factory stub — `O-LLM-3` open), FallbackChain operational |
| **Telegram connector** | Real — long-polling + webhook, media metadata parsing added this week |
| **Slack connector** | Stub only |
| **Browser UI** | Placeholder `web/index.html` only — no functional frontend |

---

## 7. Top Items Needing Attention This Week

**Ranked by impact:**

### 1. Fix corrupted `.git/packed-refs`
Git is broken. Any operation that traverses packed refs will fail. Fix immediately:
```bash
# Option A — safest
git fsck
# Then manually edit .git/packed-refs and remove the truncated line, or:
git pack-refs --all --prune
```

### 2. Live-verify NAVI Programmer plugin end-to-end
The plugin has complete skill contracts but has **never been verified against a running NaviD**. The documented next action is to mount the plugin, start NaviD, confirm discovery via gateway API, and invoke `repo-inspect` via `subprocess_python`. This is the single highest-value validation step for the new capability system.

### 3. Confirm `extensions/openai` build (`O-LLM-3`)
Anthropic and Ollama extensions are done. OpenAI extension is still `internal/llm`. With the factory pattern in place, completing `extensions/openai` closes the provider parity gap and makes the full FallbackChain (Ollama → Anthropic → OpenAI → OpenRouter) run through extension-native paths.

### 4. Update CLAUDE.md to reflect current `internal/` layout
At least 12 undocumented packages exist. AI coding assistants (including this review system) working off the stale CLAUDE.md will make wrong assumptions about the architecture. A one-time pass to add `worldmodel`, `command`, `runtime`, `cognitive`, `presence`, `identity`, `llmkb`, `onboarding`, `prompts`, `tool`, `artifact`, `backlog` to the repository layout section is low-effort and high-value.

### 5. Stabilize `plugins/` reorganization
Commit `09d0eea` explicitly notes: *"Next Tasks: Organization, refactor, and standardization."* The `extensions/` consolidation commit exists but the directory doesn't — the migration is in an intermediate state. Pick a canonical layout (either `plugins/` or `extensions/`) and finish the move so the build and discovery paths are unambiguous.

---

*Report generated by automated weekly review task `navi-weekly-status`.*
