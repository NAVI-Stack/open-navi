# NAVI Status Report — 2026-05-29

**Generated:** Automated weekly review
**Period:** Last 7 days
**Go test:** ⚠️ Could not run — Go toolchain not available in CI sandbox (see §4)

---

## 1. Recent Changes (last 7 days — 46 commits)

High-impact work this week:

**Ollama tool-calling fallback (OMN-72, IN PROGRESS)**
- `feat: implement Ollama tool recovery fallback mechanism` — core of the current next-action; adds a recovery path when Ollama returns no tool calls
- `feat: enhance LLM chat options with tool calling requirements` — tightens tool-call enforcement in request options

**Telegram connector hardening**
- Multimodal message pipeline support added
- Group configuration support added
- `owner_chat_id` now optional in setup

**Console UI — Phase 2 chat features (substantial week)**
- Edit-and-resend for user messages
- Regenerate last reply
- Assistant message feedback (up/down)
- Chat deletion and continuation
- Contextual chat bar for appearance modifications
- Model browser UX polish + real-time provider config wired to backend

**Governor**
- `MaxRepetitions` configuration field added

**Plugin system**
- Sorting and date tracking in plugin management
- Programmer contract metadata exposed via plugin API

**Docs/ADRs**
- ADR-011 introduced for Memory v2 and Context Window Governance
- Build artifact policy enforced in CLAUDE.md and AGENTS.md
- Phase 2 chat handoff docs added

**Contacts**
- `navi.contacts.manager` consolidated into `navi.contacts`
- Trust level filter added

---

## 2. Current Phase & Roadmap

**Phase:** Post-skeleton hardening / reliability + UI completeness

The active next-action is **OMN-72: Ollama tool-calling fallback**. Status is IN_PROGRESS. A fallback mechanism was partially shipped this week but regression tests and full end-to-end coverage are still outstanding per the NEXT_ACTION.md success criteria.

The broader roadmap (readiness-assessment.md) puts the project at:
- ✅ **Local dev readiness:** strong
- ✅ **Single-owner operator workflow:** usable
- ⚠️ **Broad production readiness:** not yet

The console frontend has moved well past the "placeholder" stage — it now has a real multi-page app with chat, runs, proposals, plugins, models, scheduler, artifacts, and appearance pages — but the `web/` build output in the repo still reflects what was last compiled.

---

## 3. Bug & Blocker Inventory

### Open Blockers (from blockers.md, updated 2026-03-27)

| # | Blocker | Severity | Impact |
|---|---------|----------|--------|
| B-1 | Browser UI is only a placeholder in `web/` | High | Operators can't use the browser surface without building the console |
| B-2 | Ollama tool-calling fallback still being hardened | High | Tool-requiring requests can stall or devolve to plain text on Ollama paths |
| B-3 | Native token streaming is partial | Medium | No uniform end-to-end streaming; compatibility streaming exists |
| B-4 | Production hardening behind local-first | Medium | Single-owner local only; no multi-tenant, no broad deployment |

### Open Work Items (outstanding-work.md)

| # | Item | Severity |
|---|------|----------|
| O-1 | Skill execution hardening (reliability/deps, not first impl) | Critical |
| O-2 | No streamed LLM output | Major |
| O-3 | NATS + SQLite dual-write not transactional outbox | Major |
| O-4 | Event retention policy not implemented (SQLite can grow unbounded) | Major |
| O-5 | Memory v2 / context governance not formalized | Major |

No new bugs found in the git log this week. One build fix: `console: fix router casing build break` (resolved in branch).

---

## 4. Test Health

**Status: UNKNOWN — Go not available in report sandbox**

The automated report runner does not have Go installed, so `make test` could not execute. Last known test state (from codebase review):

- `navi: restore unit-test coverage lost in the core-cleanup deletions` was committed this week — indicates a prior regression was caught and addressed
- Test files confirmed present in: `internal/llm/`, `internal/runtime/`, `internal/command/`, `connectors/`, `internal/navi/`, `web-src/navi-console/`

**Action required:** Run `make test` locally to confirm green before merging OMN-72.

---

## 5. Architecture Check

New packages added under `internal/` since last week (per git diff-filter):
- No new top-level packages; changes were additive within existing packages (`internal/gateway`, `internal/navi`, `internal/navi/store`)

New plugin added: none confirmed this week. Existing plugin surface remains: `navi-coder`, `navi-contacts`, `navi-programmer`, `navi-search`, `verification-loop`, `llm-*`, `telegram`, `slack`, `github`, `workspace-surface-audit`, `self-diagnostic`, `skill-creator`, `document-knowledge`, `core-scheduler`, `llm-embed`, `llm-router`.

`internal/worldmodel`, `internal/runtime`, `internal/command` — all active and shipping changes.

---

## 6. Capabilities Snapshot

| Component | Status |
|-----------|--------|
| **CoderAgent** (`internal/coder/`) | ✅ Real — subscribes to `CmdTaskAssign`, runs tasks with file tools + LLM |
| **CriticAgent** (`internal/critic/`) | ✅ Real runner wired |
| **StrategistAgent** (`internal/strategist/`) | ✅ Real runner wired |
| **ScoutAgent** (`internal/scout/`) | ✅ Real runner wired (runner.go present) |
| **Skill transports** | ✅ `internal`, `rest`, `mcp_tool`, `subprocess_python`, `subprocess` all implemented |
| **LLM providers** | ✅ Anthropic, OpenAI, Ollama (plugin-based); FallbackChain + DynamicProvider live |
| **Ollama tool-calling** | ⚠️ Fallback mechanism shipped this week; hardening in progress |
| **Native token streaming** | ⚠️ Partial — compatibility streaming exists, full native path incomplete |
| **Telegram connector** | ✅ Real — long-polling + webhook, multimodal pipeline added this week |
| **Slack connector** | ⚠️ Stub — basic structure only |
| **Browser UI** | ⚠️ Full console app in `web-src/navi-console/`; `web/` needs a fresh build to deploy |
| **Memory v2 / ADR-011** | 📋 ADR authored; implementation not yet started |

---

## 7. Top Items Needing Attention This Week

**Ranked by impact:**

1. **Finish OMN-72 (Ollama tool-calling fallback) + add regression tests.** The fallback mechanism landed but the success criteria in NEXT_ACTION.md explicitly call for regression tests covering the no-tool-call path. Without these, Telegram-triggered tool workflows remain unreliable on Ollama.

2. **Build and deploy the console frontend to `web/`.** The console is a full application but `web/` is still a placeholder. Any operator trying to use the browser UI hits the old static file. Run `npm run build` in `web-src/navi-console/` and commit the output (or wire it into the Docker build).

3. **Confirm test suite is green after this week's 46 commits.** The core-cleanup regression was patched, but 46 commits in a week with significant cross-cutting changes (governor, plugin API, connector pipeline, model browser backend) warrants a deliberate `make test` pass.

4. **Address O-3: NATS + SQLite dual-write inconsistency.** This is the most dangerous silent failure mode — a crash between publish and commit can leave state permanently inconsistent. Outbox pattern or WAL-based at-least-once delivery should be the next architectural item after OMN-72.

5. **Start Memory v2 implementation (ADR-011).** The ADR was authored this week. Memory and context governance are foundational for the always-on use case. Beginning implementation soon will prevent context window reliability from becoming a hard blocker as usage grows.
