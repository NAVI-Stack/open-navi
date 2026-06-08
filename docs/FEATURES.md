# Current Feature Reference

**Status:** Active
**Last Updated:** 2026-04-01
**Source of truth:** current packages under `cmd/`, `internal/`, `connectors/`, and `plugins/`

This is a code-backed summary of the features that are actually present in the repository today.

## 1. Runtime And Chat Engine

**Packages:** `internal/runtime`, `internal/navi`, `internal/navi/store`

| Feature | Status | Notes |
|---------|--------|-------|
| Chat creation and message intake | Done | Chat APIs and connector ingress both feed the runtime inbox |
| Run coordinator | Done | Supports active runs, paused runs, resume signals, cancellation, and scheduling |
| Proposal-aware execution | Done | Runs can pause on a proposal and resume after owner resolution |
| Legacy persona system | Removed | Fixed presets (Buddy, Jarvis, Orchestrator) have been replaced by the modular experience layer |
| Modular Experience Layer | Done | Trait-based persona system with dynamic merging and cognitive modulation |
| Chat summarization/compaction | Done | Chat summarizer and compaction-related tests are present |
| Prompt hot reload | Done | Prompt files can be watched and reloaded from `prompts_dir` |
| Structured runtime metrics | Done | Runtime metrics endpoint exists under the gateway debug surface |

## 2. Orchestration And Workers

**Packages:** `internal/orchestrator`, `internal/coder`, `internal/critic`, `internal/strategist`, `internal/scout`

| Feature | Status | Notes |
|---------|--------|-------|
| Directive loop | Done | Polls active directives and reconstructs state from persistence |
| Task decomposition | Done | `ACT` directives decompose into tasks and publish assignment events |
| Coder worker | Done | File-oriented execution worker |
| Critic worker | Done | Review worker |
| Strategist worker | Done | Planning and design worker |
| Scout worker | Done | Research worker with tool-calling support |
| Worker governance checks | Done | Worker execution passes through governor validation |

## 3. Skills And Tool Execution

**Packages:** `internal/navi/skill`, `internal/tool`, `plugins/*/skills`, `skills/_runtime`

| Feature | Status | Notes |
|---------|--------|-------|
| Skill loading from disk | Done | Built-in and workspace skill loading are implemented |
| Skill validation and synthesis | Done | OSS-27 style skill processing is live |
| Internal transport | Done | Built-in internal handlers can execute directly |
| REST transport | Done | HTTP execution path is implemented |
| MCP bridge transport | Done | `mcp_tool` transport is implemented through an HTTP bridge |
| Python subprocess transport | Done | Local Python runtime with venv provisioning is implemented |
| Generic subprocess transport | Done | JSON-RPC stdio worker processes can be launched through skill interfaces |
| Tool registry | Done | Unified registry is available to runtime and gateway surfaces |
| Gap detection and skill building | Done | Gap detection and scaffold/build flows are present |

The built-in advisory skill set is plugin-owned. Examples include `plugins/workspace-surface-audit/skills/workspace-surface-audit/SKILL.md` and `plugins/verification-loop/skills/verification-loop/SKILL.md`, which port high-signal setup and verification patterns into NAVI without coupling the repo to Claude-specific command shims.

## 4. LLM Stack

**Packages:** `internal/llm`, `internal/prompts`, `plugins/llm-*`

| Feature | Status | Notes |
|---------|--------|-------|
| Ollama provider | Done | Built-in `llm-provider` plugin under `plugins/llm-ollama` |
| Anthropic provider | Done | Built-in `llm-provider` plugin under `plugins/llm-anthropic` |
| OpenAI provider | Done | Built-in `llm-provider` plugin under `plugins/llm-openai` |
| OpenRouter provider | Done | Registered by the OpenAI-compatible `plugins/llm-openai` provider plugin |
| Dynamic provider swapping | Done | Active provider/model can change at runtime |
| Provider catalog and profiles | Done | Catalog, profiles, and routing preferences are exposed |
| Fallback chain | Done | Multiple providers can be chained |
| Task-aware routing surface | Done | Preferences and profile APIs are present |
| Native end-to-end token streaming | Partial | Compatibility streaming exists, but full native streaming is not finished |

## 5. Persistence And World Model

**Packages:** `internal/store`, `internal/worldmodel`, `internal/schema`

| Feature | Status | Notes |
|---------|--------|-------|
| SQLite WAL persistence | Done | Core persistence model |
| Shared schema package | Done | Shared runtime types live in `internal/schema` |
| World-model facade | Done | Contacts, memories, facts, artifacts, workspaces, relationships, configuration, priorities |
| Structured error log | Done | Error log tables and gateway endpoints are present |
| Execution outcomes | Done | Runs and tool execution record outcomes |
| Scheduled tasks | Done | Persistent scheduled task model exists |
| LLM-KB storage and routing data | Done | LLM profiles and proposal surfaces are present |

## 6. Gateway And Operator APIs

**Packages:** `internal/gateway`

| Feature | Status | Notes |
|---------|--------|-------|
| Health and version routes | Done | Public health plus schema version |
| Onboarding and owner claim flow | Done | Browser and CLI bootstrap paths exist |
| Chat API | Done | Create, list, message, archive |
| Directive API | Done | Create, inspect, message, change mode |
| Proposal API | Done | Review and resolve proposals |
| LLM operator API | Done | Catalog, active model, profiles, preferences, proposals |
| Skills and tools API | Done | Introspection and reload surfaces |
| Connectors API | Done | Registration, setup schema, health, diagnostics |
| Runs and activity API | Done | Operator visibility into runtime work |
| Error and debug APIs | Done | Structured errors, event tail, runtime metrics, LLM-KB debug |
| WebSocket live feed | Done | Live runtime event surface |
| OpenAI-compatible API | Done | `/v1/chat/completions` and `/v1/models` |
| Browser Console UI | Partial | React 19 + Vite console under `web-src/navi-console/` builds into `web/`; gateway serves it from `web/`. Chat UX is feature-complete; e2e/integration coverage is the gap — see `docs/architecture/navi-console-frontend.md` |

## 7. Plugins And Extensibility

**Packages:** `plugins/`, `connectors`, `internal/connectors`, `internal/navi/plugin`

`plugins/` is the repo-owned capability package boundary. Plugin-owned skills are exposed only through valid, active `plugin.yaml` manifests; root `skills/` and `connectors/` are framework/support surfaces only.

| Feature | Status | Notes |
|---------|--------|-------|
| Telegram connector | Done | Built-in implementation exists |
| Slack connector | Done | Built-in implementation exists |
| Connector manager and registry | Done | Runtime manager and registry are implemented |
| Bridge connectors | Done | Gateway bridge registration path exists |
| Workspace subprocess connectors | Done | `workspace/connectors/<name>/CONNECTOR.yaml` is supported |
| Connector diagnostics and health | Done | Gateway surfaces exist |
| Plugin manifest discovery | Done | Plugin manifests are exposed through the gateway |

## 8. Background Services

**Packages:** `internal/navi/heartbeat`, `internal/navi/reflection`, `internal/backlog`

| Feature | Status | Notes |
|---------|--------|-------|
| Heartbeat service | Done | Background heartbeat with interval and quiet-hour controls |
| Reflection worker | Done | Reflection worker is started by the daemon |
| Backlog polling and scheduled execution | Done | Scheduler and backlog pieces are present |
| Quiet hours / DND for heartbeat surfacing | Done | Config-backed behavior exists |

## 9. CLI And Local Operations

**Packages:** `cmd/navi`, `Makefile`

| Feature | Status | Notes |
|---------|--------|-------|
| Interactive chat | Done | Default CLI flow |
| One-shot ask | Done | `navi ask` |
| Init/onboarding flow | Done | `navi init` |
| Status, models, chats, logs | Done | Operator commands exist |
| Skills, connectors, activity, runs | Done | Operator subcommands exist |
| Doctor and console modes | Done | Additional operator tooling exists |
| Native and Docker make targets | Done | Build, run, test, docker, reset targets are present |

## 10. Current Gaps

These are the main places where the code is still incomplete or being hardened:

- the browser Console (`web-src/navi-console/`) is real and feature-complete for chat, but lacks e2e coverage and `ChatPage.tsx` orchestration tests
- native token streaming is still partial
- Ollama tool-calling fallback reliability still needs work
- the deployment posture is still optimized for local, single-owner usage rather than broader production hosting

[docs INDEX](INDEX.md) | [architecture](architecture/README.md)
