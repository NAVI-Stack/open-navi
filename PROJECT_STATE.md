# Project State

**Status:** Active  
**Last Updated:** 2026-04-01  

## Current Phase

Phase 15 - Self-Extension Pipeline and runtime reliability hardening
**Autonomy Level:** ~55% (verified via audit 2026-04-01; up from ~50% on 2026-03-27)

## What Is Done

- Orchestrator loop with governor and directive reply
- Task store (SQLite CRUD)
- NAVI agent loop with skills and file tools (ReadFileTool, ListDirTool, WriteFileTool)
- Python `subprocess_python` skill runtime (venv provisioning, wrapper dispatch, timeout/error envelopes)
- Shipped Python skill execution validated (testdata python-health-check fixture)
- Governor path sandboxing (RestrictToWorkspace, CheckPath)
- Agent workflow state files and structure
- Internal skill transport registry and dispatch implemented
- MCP transport (bridge client) implemented and validated with tests
- Gap Detection Pipeline: `gap_detector.go` with Signal A (LLM inability) and Signal B (tool failure classification A-F), wired into loop and gateway API (OMN-20, Done)
- Gap -> Skill Scaffolding Pipeline (Self-Build Loop): `SkillBuilder` wired into the loop, uses LLM to synthesize and install new skills (OMN-21, Done — see [docs/architecture/self-extension.md](docs/architecture/self-extension.md))
- Telegram Connector Fixes: `answerCallbackQuery`, `sendMessage` error body surfacing, and `ParseMode` validation (OMN-22, Done)
- Skill Synthesizer imports now map OpenClaw-style manifests and Claude/OpenAI tool definitions into validated OSS-27 `SKILL.yaml` output (OMN-23, Done)
- Codebase Self-Modification: `GitStatus`, `GitDiff`, `GitCommit`, `GoBuild`, and `GoTest` tools integrated into the agent loop with mandated governance (OMN-24, Done)
- Scout Web Search Tool: `scout-search` skill implemented using Brave Search API; Scout runner updated to support tool-calling turns for research (OMN-25, Done)
- Connector Plugin System: runtime connector manifests under `workspace/connectors/<name>/CONNECTOR.yaml`, subprocess connector inbox/outbound stdio wiring, and connector scaffolding through the self-build pipeline are now implemented (OMN-26, Done)
- Cron / Scheduled Tasks: persistent SQLite-based task scheduler and LLM tool created, leveraging cron patterns (OMN-27, Done)
- Skill Registry / Hub: remote skill discovery, search (`search_hub`), and automated install fallback for gaps (OMN-28, Done)
- Fact Extraction from Conversations: `fact_extractor.go` with `buildReflectionDetails()` wired into `emitReflect()`, regex-based extraction of preferences/editor/language/project (OMN-56, Done — regex only, LLM upgrade pending)
- Memory Scope Fix: manual "remember this" escalation now defaults to owner scope; `ContextBlockForSession()` includes session-scoped data (OMN-57, Done)
- Task Outcome Facts: coder/runner.go writes `task_outcome` facts after task completion; orchestrator reads them back for context (OMN-58, Done)
- Orchestrator Reflection: `adapter.go` emits `FactReflectionQueued` after task decomposition, execution, and directive completion (OMN-59, Done)
- skill-creator as OSS-27 Tool: Full SKILL.yaml with internal transport, wired to SkillBuilder (OMN-60, Done)
- Structured Error Logging: `error_log` table, `store/error_log.go`, `recordError()` in loop.go, `GET /api/errors` + `/api/errors/summary`, self-diagnostic skill (OMN-52, Done)
- LLM Timeout Fix: 35s → 120s configurable via `llmCallTimeout()` (OMN-51, Done)
- LLM Model Profiles + Task Classifier: `llm/profiles.go` — ModelProfile, ClassifyTask() heuristic (5 classes), ModelSelector.Select() with weighted scoring, SeedProfiles() (OMN-54, Partially Done — profiles built, per-turn integration pending)
- Telegram model switching reliability: normalized `_set_active` args, Ollama catalog refresh, and direct `/model` command path (OMN-62, Done)
- Telegram session bootstrap reliability: gateway readiness probe, session creation retry/backoff, recovered-session reuse, and SQLite pool expansion for WAL readers (OMN-63, Done)
- Artifact materialization + workflow lifecycle: tracked artifact state/version history, cognitive-layer materialization for file/skill outputs, failed-materialization preservation, and `GET /api/artifacts` / `GET /api/artifacts/{id}` operator inspection (OMN-118, Done)

## What Is In Progress

- OMN-72: Ollama tool calling fallback and agentic reliability
- OMN-73: system prompt cleanup to stop persona interference

## What Is Next

- Finish the remaining urgent Telegram and Ollama regressions, then resume broader memory and knowledge-base work
- Continue Memory v2 planning and implementation (NAVI-MC-001 / OMN-29) after urgent reliability issues
- See `TASK_QUEUE.md` and Linear for the current prioritized backlog
