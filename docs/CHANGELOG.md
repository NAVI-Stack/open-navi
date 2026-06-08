# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Changed
- **Major Rebranding**: Consolidated project identity as **NAVI**.
- Renamed `internal/ceo` package to `internal/orchestrator`.
- Renamed `cmd/open-navi` to `cmd/navid`.
- Rebranded legacy naming to "Orchestrator" or "NAVI".
- Updated NATS subjects and streams to `navi.*` and `NAVI_*` naming.
- Updated environment variables to use `NAVI_` prefix.
- Refactored `AgentCEO` to `AgentOrchestrator` in schema and bus.
- Updated all project documentation (`README.md`, `VISION.md`, etc.) to reflect NAVI-centric branding.

### Added
- Initial project structure and core functionality.
- Documentation parity with example projects.
- `.agents` directory for agent-specific configurations.

---

## Phase 12 — Gateway, Auth, Connectors

### Added
- Gateway API: REST and WebSocket server, session and directive endpoints.
- Authentication: JWT, shared secret, API keys, owner model.
- Connectors: Telegram and Slack connector implementations and registration.
- Health endpoint `GET /health`, connector health and diagnostics.

---

## Phase 11 — Governor

### Added
- Governor: action budget, cost ceiling, retry limit, duration limit.
- Per-session action tracking and path sandboxing.
- Governor stats API and `fact.governor.tripped` bus event.

---

## Phase 10 — Event Bus

### Added
- JetStream bus over NATS with durable consumers.
- MemBus for tests and single-binary mode.
- Dual-write to SQLite event log, schema version enforcement, typed subjects.

---

## Phase 9 — State Engine (SQLite)

### Added
- SQLite store with WAL mode and idempotent migrations.
- Events, directives, tasks, API keys, owners, settings, agents tables.
- EventsSince query for pagination and reply polling.

---

## Phase 8 — Session Management

### Added
- Session creation, message append, history retrieval, session listing.
- Active session pointer and meta key/value store.

---

## Phase 7 — Hook System

### Added
- Seven hook points (before/after tool call, message received/sending, session start/end, before prompt build).
- Priority ordering and payload threading; chain abort on error.

---

## Phase 6 — Heartbeat Service

### Added
- Periodic heartbeat tick, HEARTBEAT.md task file, heartbeat prompt construction.
- heartbeat.log and `navi.fact.heartbeat.done` bus event.

---

## Phase 5 — Experience Engine

### Added
- Runtime experience profile definitions and per-session assignment.
- Experience-aware LLM shaping and graceful fallback.
- Initial built-in profiles for standard NAVI behavior and onboarding wizard behavior.

---

## Phase 4 — Skill System

### Added
- Zero-code skill registration, three-tier skill hierarchy (workspace / ~/.navi / built-in).
- SKILL.md and SKILL.yaml formats, dependency and OS filtering.
- Skill registry, LLM tool injection, policy engine, provenance tracking.

---

## Phase 3 — LLM Provider Layer

### Added
- Anthropic, OpenAI, OpenRouter, Ollama providers with tool-use.
- FallbackChain, LLM Router, provider-qualified model routing.
- Hot-swap via API, streaming (partial/bridge).

---

## Phase 2 — Orchestration

### Added
- Ticking orchestration loop, directive modes (DISCUSS, DESIGN, IMPLEMENT, REVIEW, AUDIT).
- DecomposeTasksTool, CoderAgent two-pass execution, governor integration, cost tracking.
- Event-driven output; CriticAgent and StrategistAgent stubs.

---

## Phase 1 — Agent Runtime

### Added
- Persistent agent loop (event-driven wake/sleep), multi-turn LLM conversation, tool-call loop.
- Graceful LLM error handling, experience-tuned temperature, security rules in system prompt.
- Proactive session intro, bus-published reply events.
