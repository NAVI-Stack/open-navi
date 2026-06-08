# NAVI Architecture Overview

> [!NOTE]
> Part of the [NAVI Systems Map](navi-systems-map.md).


**Status:** Active
**Last Updated:** 2026-04-19
**Source of truth:** live package layout in `cmd/` and `internal/`

This document describes the current implementation architecture rather than the aspirational model. For the visual end-to-end architecture guide, see [NAVI System Architecture Guide](navi-system-architecture-guide.md). For higher-level product principles, see [../canonical/conceptual-design-overview.md](../canonical/conceptual-design-overview.md). For the current runtime surface, pair this document with [../specs/configuration.md](../specs/configuration.md) and [../specs/gateway-api.md](../specs/gateway-api.md).

For Inference Control System coverage, use the [ICS documentation corpus index](inference-control-system/INDEX.md) to navigate spec, contract, compliance, and implementation-mapping documents.

For the Go / Python / TypeScript language-layer boundary (which language owns which authority and how the layers may communicate), see the [Language-Layer Contract](language-layer-contract.md); its first migration phase is [Language-Layer Phase 1 — Shared Contracts](../plans/language-layer-phase1-contracts.plan.md).

For chat compaction coverage on `feat/compaction-system`, use these files, whose filenames retain historical `session-compaction` names:

- [session-compaction-architectural-contract.md](session-compaction-architectural-contract.md)
- [session-compaction-implementation-mapping.md](session-compaction-implementation-mapping.md)
- [session-compaction-evaluation-rubric.md](session-compaction-evaluation-rubric.md)
- [../adr/ADR-009-session-compaction-runtime.md](../adr/ADR-009-session-compaction-runtime.md)

## Runtime At A Glance

NAVI currently runs as a local-first, single-owner system with two entrypoints:

- `cmd/navid`: the daemon
- `cmd/navi`: the operator CLI

`navid` is responsible for:

- loading config and persisted settings
- initializing SQLite, the world model, and identity
- starting embedded NATS when `nats.url` is `embedded`
- building the dynamic LLM provider and routing catalog
- starting the foreground NAVI runtime and directive orchestrator
- wiring worker subscriptions for coder, critic, strategist, and scout
- starting heartbeat, reflection, scheduler pollers, connectors, and the HTTP gateway

`navi` is responsible for:

- onboarding and owner claim flows
- interactive chat and one-shot asks
- operator commands for models, runs, skills, connectors, activity, logs, and status

## Main Packages

| Package | Role |
|---------|------|
| `internal/runtime` | Foreground run coordinator, inbox classification, pause/resume, scheduled delivery, runtime metrics |
| `internal/navi` | Chat-facing agent loop, tool/runtime integration, prompts, experience/persona application, summarization, gap detection (OMN-20), artifact handling (OMN-118) |
| `internal/navi/experience` | Experience Layer / Persona System runtime: behavioral identity, profile/module configuration, output shaping, and owner-facing experience controls |
| `internal/orchestrator` | Directive-level loop, LLM adapter, task decomposition, task assignment events |
| `internal/coder` | File-oriented execution worker |
| `internal/critic` | Review worker |
| `internal/strategist` | Planning and design worker |
| `internal/scout` | Research worker with skill tool-calling |
| `internal/navi/skill` | Skill loading, validation, synthesis, transport execution, internal handlers, Python runtime, and SkillBuilder (Self-Extension) |
| `internal/tool` | Unified tool registry surfaced through the gateway and runtime |
| `internal/worldmodel` | Facade over persisted contacts, memories, facts, artifacts, relationships, configuration, and priorities |
| `internal/store` | SQLite persistence for directives, tasks, settings, identity, errors, execution outcomes, knowledge, proposals, scheduled tasks, and more |
| `internal/navi/store` | Chat, runtime-session, runtime queue, and compaction persistence |
| `internal/gateway` | HTTP API, WebSocket live feed, onboarding endpoints, operator surfaces, OpenAI-compatible endpoints |
| `internal/llm` | Provider implementations, catalog, router, selection state, dynamic swapping, fallback chain |
| `internal/governor` | Hard limits plus governance validation and autonomy controls |
| `internal/bus` | Embedded or remote NATS JetStream bus integration |
| `connectors` and `internal/connectors` | Built-in connectors, manager, registry, workspace connector loading, subprocess connector support |

## LLM Dual-Plane Architecture

NAVI distinguishes between the **Inference Plane** and the **Control Plane** to decouple stateless execution from stateful policy.

- **Inference Plane (`llm.Provider`)**: Stateless, low-level, per-call. Owns token generation and streaming.
- **Control Plane (`LLMService`)**: Stateful, policy-aware. Owns provider selection, routing, and catalog management.

See [LLM Dual-Plane Architecture](llm-dual-plane.md) for the full formalization.

## Experience Layer / Persona System

The Experience Layer is a first-class runtime pillar, not a cosmetic add-on. It controls NAVI's behavioral identity, presentation, output preferences, profile/module configuration, and owner-facing experience inspection.

Use **Experience Layer** as the technical term. Use **persona system** as the common product term.

Boundary rules:

- The Governor decides what is allowed.
- The Inference Control System controls the model/tool execution lifecycle.
- The Experience Layer shapes how NAVI presents, responds, and adapts inside those constraints.
- The Experience Layer must not authorize actions, widen tool access, bypass proposal boundaries, or own hard policy decisions.

Primary paths:

- `internal/navi/experience`
- `internal/navi/experience_manager.go`
- `config/personas`
- gateway experience endpoints under `internal/gateway`

## Execution Flows

### 1. Foreground chat/runtime flow

1. A client sends a message through `/api/navi/chats/{id}/message` or a connector.
2. `internal/runtime` accepts the inbox item and wakes the runtime session linked to the chat.
3. The run coordinator launches or resumes a run for that runtime session.
4. `internal/navi` builds prompt context, applies experience/persona shaping, runs the LLM, executes tools and skills, and may pause for proposals when governance requires approval.
5. The runtime appends assistant messages, emits run events, and records execution outcomes.
6. Reflection and world-model updates follow after the foreground turn.

### 2. Directive and worker flow

1. A directive is created through the gateway or persistence layer.
2. `internal/orchestrator` polls active directives.
3. In `ACT` mode, the adapter decomposes work into tasks and publishes `CmdTaskAssign` events.
4. Worker packages subscribe to assignments and execute only the tasks addressed to their agent type.
5. Workers persist task updates and append their outputs back onto the directive conversation.

### 3. Skill execution flow

Skills are loaded from built-in and workspace sources, converted into tool definitions, and executed through `internal/navi/skill`.

Currently supported transport types:

- `internal`
- `rest`
- `mcp_tool`
- `subprocess_python`
- `subprocess`

The Python runtime provisions a local virtual environment when needed and returns structured execution envelopes. The generic subprocess runtime launches protocol-speaking Python, Node/TypeScript, shell, or external harness workers over JSON-RPC stdio while keeping governance and result normalization in Go. Internal handlers are registered for built-in capabilities such as scheduling.

## Persistence Surfaces

SQLite is the primary source of truth. The store layer currently persists:

- directives and directive messages
- tasks and task outcomes
- chats, runtime sessions, runs, inbox items, checkpoints, and runtime events
- API keys and owner identity
- settings and onboarding state
- execution outcomes and structured errors
- world-model entities such as contacts, facts, memories, artifacts, workspaces, relationships, and proposals
- scheduled tasks and LLM routing data

JetStream is the event backbone. The current streams are defined in `internal/bus/streams.go`.

## Gateway Surface

The gateway is more than a chat API. It currently exposes:

- onboarding and owner-claim endpoints
- chat and directive APIs
- proposals, activity, run, errors, knowledge, tools, and skills APIs
- LLM catalog, active-model, profile, and routing-preference APIs
- experience/persona inspection and configuration APIs
- connector registration and diagnostics APIs
- OpenAI-compatible endpoints
- WebSocket live feed

See [../specs/gateway-api.md](../specs/gateway-api.md) for the route inventory.

## Auth Model

The current auth model is API-key based, not JWT-based:

- loopback requests get owner-like access for local CLI usage
- onboarding endpoints are public
- setup endpoints are open until setup is complete
- authenticated requests use `X-API-Key` or `Authorization: Bearer navi_...`
- the shared gateway secret is also accepted for connector compatibility

## Current Boundaries And Gaps

- The browser Console (`web-src/navi-console/`, a React 19 + Vite app that builds into `web/` and is served from `web/`) is feature-complete for chat but undertested at the e2e level — see [navi-console-frontend.md](navi-console-frontend.md).
- The Experience Layer is runtime-wired and should be treated as first-class, but it still needs stronger architecture docs, test coverage, and contributor-facing ownership boundaries.
- Ollama tool-calling fallback behavior is still being hardened.
- Integration tests for cross-node NAVI Net discovery are pending.

That said, the codebase already includes a working runtime coordinator, worker system, skill transports, world-model facade, experience/persona layer, and operator gateway, so the implementation has moved meaningfully beyond the earlier minimal phase descriptions.
