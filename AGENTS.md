# AGENTS.md — NAVI AI Codebase Guide

**Status:** Active
**Last Updated:** 2026-06-01
**Current Phase:** 15 — Reliability & Self-Extension

> Mandatory reading for AI coding assistants working in this repository.
> For the full product vision and manifesto, see [`docs/VISION.md`](docs/VISION.md) and [`docs/AGENTS.md`](docs/AGENTS.md).

---

## What This Project Is

NAVI is a **two-binary** personal AI system written in Go. **NaviD** (`navid`) is the long-running server; **NaviExe** is the native CLI client used by packaged npm/pip entrypoints, and PET (UI) is a client. NAVI supports two valid NaviD runtime modes: Docker mode and local daemon mode.

| Component | Role |
|-----------|------|
| `navid` (NaviD) | Server — orchestration, HTTP/WebSocket API, embedded NATS + SQLite (run via **Docker Compose** or **local daemon mode**) |
| packaged `navi` CLI | npm/pip user CLI — delegates to NaviExe, connects to NaviD over HTTP/WebSocket, and manages local daemon lifecycle with `navi daemon ...` |
| PET | UI client — connects to NaviD |

Current state: CoderAgent, CriticAgent, and StrategistAgent are real LLM-backed workers. File tools are real; full MCP/REST (non-file) skill execution is not yet implemented. The console frontend lives in `web-src/navi-console/` and builds into `web/`.

### Architectural Mandate: Dual-Plane LLM
All LLM integration must strictly separate the **Inference Plane** (stateless `llm.Provider`) from the **Control Plane** (stateful `LLMService`).
- **Provider plugins** (`plugins/llm-*`) handle provider-specific tokens, APIs, and streaming.
- **Service** (`internal/llm/controlplane.go`) handles profiles, selection, and routing policy.
Never put stateful logic (selection, retries across models) inside a Provider implementation, and never import concrete provider plugins from `internal/llm`.

Built-in provider plugins register through `cmd/navid/plugin_bootstrap.go`.

---

## Repository Layout

```
NAVI/
├── cmd/
│   ├── navid/          # Daemon entrypoint (main.go)
│   └── navi/           # CLI client entrypoint (main.go)
├── internal/
│   ├── bus/            # NATS JetStream — 8 streams, pub/sub primitives
│   ├── config/         # YAML + env config loader (Config struct)
│   ├── connectors/     # Connector manager, registry, lifecycle, HITL bridge
│   ├── gateway/        # HTTP/WS API server, JWT auth, connector routes
│   ├── governor/       # Hard limits: action budget, cost ceiling, TTL, retries
│   ├── hooks/          # Lifecycle hook system (connector_started, message_sent, …)
│   ├── llm/            # Provider interfaces, registry, routing, FallbackChain
│   ├── navi/           # Agent runtime: loop, chats, runtime sessions, heartbeat, skills, personas, plugins
│   ├── orchestrator/   # Orchestration loop, planner, LLM adapter, tools
│   ├── schema/         # Go types — canonical source of truth for Directive, Task, Agent
│   └── store/          # SQLite WAL — tasks, directives, API keys, event log, compatibility stores
├── connectors/         # Shared connector interfaces/types only
│   ├── connector.go    # Connector interface
│   ├── base.go         # Helper base struct
│   ├── capabilities.go # Optional capability interfaces (type assertions)
│   ├── errors.go       # Connector error types
│   └── ...             # Framework/support surfaces, not plugin-owned connectors
├── config/
│   ├── runtime.yaml    # Default runtime configuration
│   └── personas/       # YAML experience-profile definitions (navi, wizard)
├── plugins/            # Repo-owned installable capability packages
│   ├── telegram/       # Plugin-owned skills/connectors/policies
│   ├── slack/          # Plugin-owned connector package
│   └── llm-*/          # Built-in llm-provider plugins
├── skills/             # Skill-system support only, e.g. shared runtimes/docs
├── schema/             # JSON Schema definitions + Python codegen
│   ├── jsonschema/     # JSON Schema files (agent, event, task, claim, enums)
│   └── python/         # Generated Python models (DO NOT EDIT — run make generate-python)
├── docs/               # VISION.md, AGENTS.md, ADRs, task lists, blockers
├── test/
│   └── e2e/
│       └── smoke_test.go  # E2E smoke tests (health, chat, gateway)
├── web/
│   └── index.html      # Minimal placeholder (BLOCKER-1 partially resolved)
├── workspace/          # Default agent workspace directory (gitignored data)
├── Makefile
├── Dockerfile
├── compose.yml              # Default permissive NaviD stack
├── compose.strict.yml       # Isolated NaviD stack (standalone file)
├── compose.custom.example.yml
└── go.mod              # module: github.com/ceoai/navi, Go 1.24+
```

---

## Build & Run Commands

```bash
# NaviD Docker mode
docker compose -f compose.yml up -d --build
# Isolated mode (requires NAVI_OLLAMA_URL; do not combine with compose.yml — see docs/runbooks/run-navi.md)
#   NAVI_OLLAMA_URL=https://... docker compose -f compose.strict.yml up -d --build

# NaviD local daemon mode (user-facing)
npm install open-navi
navi daemon start
navi daemon status
navi daemon stop
# or:
pip install open-navi
navi daemon start

# Native developer builds
make build           # → bin/navid (bin/navid.exe on Windows)
make build-cli       # → bin/navi.exe (NaviExe)
NAVI_DISTRIBUTION_CHANNEL=dev ./bin/navi.exe daemon start
NAVI_DISTRIBUTION_CHANNEL=dev ./bin/navi.exe daemon status
NAVI_DISTRIBUTION_CHANNEL=dev ./bin/navi.exe daemon stop

# Native builds
make build-all       # both

make cli             # go run ./cmd/navi/

# Test
make test            # go test ./cmd/... ./internal/... ./connectors/... -count=1

# Docker shortcuts
make docker-up       # docker compose defaults (compose.yml)
make docker-up-strict
make docker-down

# Local daemon shortcuts
make daemon-start
make daemon-status
make daemon-stop

# Code generation
make generate-python # sync Python schema models from internal/schema/

# Cleanup
make clean           # rm -rf bin/
```

### Go Cache Policy

Use the default Go cache locations from `go env GOCACHE` and `go env GOMODCACHE`.
Do not set `GOCACHE` or `GOMODCACHE` to repo-local directories such as `.gocache`,
`.gomodcache`, `.codex-gocache`, `.codex-gomodcache`, `.codex-go-cache`, or
`.codex-go-modcache`. Repo-local Go caches are bloat and should be deleted, not
preserved or recreated.

After NaviD is running, the packaged CLI launches an interactive setup wizard on first use. Use `navi -new` to start a fresh chat after setup is complete.

---

## Configuration

Configuration is layered: `config/runtime.yaml` → environment variable overrides.

**Key environment variables** (env vars always win over YAML):

| Env Var | Purpose |
|---------|---------|
| `NAVI_GATEWAY_ADDR` | Gateway listen address (default `:6284`) |
| `NAVI_ANTHROPIC_KEY` | Anthropic API key |
| `NAVI_OPENAI_KEY` | OpenAI API key |
| `NAVI_OPENROUTER_KEY` | OpenRouter API key |
| `NAVI_OLLAMA_URL` | Ollama endpoint (default `http://localhost:11434/v1`) |
| `NAVI_SQLITE_PATH` | SQLite database path (default `navi.db`) |
| `NAVI_GATEWAY_SHARED_SECRET` | Connector API key for X-API-Key header |
| `NAVI_TELEGRAM_BOT_TOKEN` | Telegram bot token |
| `NAVI_DEBUG` | Set `1` or `true` to enable slog debug output |
| `NAVI_NATS_URL` | NATS URL (default `embedded` — runs in-process) |
| `NAVI_HEARTBEAT_INTERVAL` | Heartbeat interval, e.g. `30m` |

The Go config struct lives at `internal/config/config.go`. `LoadOrDefault(path)` reads the YAML file if it exists, then calls `applyEnvOverrides()`. Config validation is intentionally minimal — required secrets are auto-generated at startup if absent.

**Current defaults in `config/runtime.yaml`** (notable values):

```yaml
nats:
  url: "embedded"          # In-process NATS (no external server needed for local dev)

governor:
  max_action_budget: 1000  # Actions per runtime session
  max_retries: 5
  cost_ceiling: 50.0       # USD per runtime session
  autonomous_duration: "72h"

navi:
  workspace_dir: "workspace"
  heartbeat_enabled: true
  heartbeat_interval: "30m"
```

---

## Core Packages

### `internal/schema` — Canonical Data Types

**This is the single source of truth.** Never manually edit generated Python models; run `make generate-python` instead.

Key types:

- `Directive` — a user's high-level goal (modes: `CHAT`, `ADVISE`, `ASSIST`, `ACT`, `WATCH`)
- `DirectiveMessage` — a single turn in a directive conversation (`role`: `"owner"` or `"navi"`)
- `Task` — a work unit with status lifecycle (`pending → running → blocked/completed/failed/cancelled`)
- `AgentType` — identifies which worker executes a task (`navi`, `heartbeat`, `connector`)
- `RiskLevel` — `low`, `medium`, `high`, `critical`
- `DAGEdge` — task dependency edge (from → to)
- `SurfaceDeclaration` — filesystem surface a task may access (`path`, `access_mode`)
- `VerificationContract` — how to verify task completion (commands, success code, timeout)
- `CostAttribution` — token usage and USD cost tracking per task
- `CostTier` — `free`, `cheap`, `moderate`, `expensive`

JSON Schema exports live in `schema/jsonschema/` (agent, event, task, claim, enums).

### `internal/bus` — NATS JetStream

Subjects follow the pattern `navi.<stream>.<event>`. Eight streams:

| Stream | Subjects | Policy | Retention |
|--------|----------|--------|-----------|
| `NAVI` | `navi.core.inbox`, `navi.core.outbox` | Limits | 24h |
| `NAVI_AGENTS` | `navi.agents.>` | WorkQueue | 48h |
| `NAVI_REFINERY` | `navi.refinery.queue` | WorkQueue | 48h |
| `NAVI_HITL` | `navi.hitl.requests` | Limits | 7d |
| `NAVI_AUDIT` | `navi.audit.>` | Limits | 30d |
| `NAVI_GOVERNOR` | `navi.governor.>` | Limits | 7d |
| `NAVI_FACTS` | `navi.fact.>`, `navi.navi.fact.>` | Limits | 7d |
| `NAVI_COMMANDS` | `navi.cmd.>`, `navi.navi.cmd.>` | Limits | 7d |

`bus.EnsureStreams(js)` is idempotent and called on every startup.

`MemBus` exists for **testing only** — not exposed at runtime.

### `internal/llm` — LLM Provider Abstraction

```go
type Provider interface {
    Chat(ctx context.Context, model string, messages []Message, tools []ToolDefinition, opts Options) (*Response, error)
    Name() string
}
```

Concrete provider implementations are built-in `llm-provider` plugins:
- `plugins/llm-anthropic/providers/anthropic` — Anthropic Messages API.
- `plugins/llm-openai/providers/openai` — OpenAI and OpenRouter-compatible APIs.
- `plugins/llm-ollama/providers/ollama` — Local Ollama-compatible endpoint.
- `FallbackChain` — Tries multiple providers in sequence with per-provider cooldowns and error classification (retriable vs. non-retriable).
- `DynamicProvider` — Hot-swappable provider wrapper (updates runtime without restart).

**Default fallback chain:** Ollama → Anthropic → OpenAI → OpenRouter

Model routing is configured via `LLMConfig.Routes` (map of logical role → `ModelRoute{Primary, Fallbacks}`). Legacy `*Model` fields are supported but deprecated in favor of Routes.

Use `NewFallbackChain(candidates)` — do **not** hard-code provider selection in Go logic.

### `internal/navi` — Agent Runtime

The `NAVI` struct is the core agent. It owns:
- `ChatStore` — CRUD for durable chat transcripts and messages
- `RuntimeSessionStore` — CRUD for execution/runtime lifetimes linked to chats
- `SkillRegistry` — loads workspace/user skills and active plugin-declared skills from valid `plugin.yaml` manifests
- `PersonaEngine` — loads YAML experience profiles from `config/personas/` and compiles per-turn experience controls
- `AgentLoop` — wake-driven loop; processes one turn per `Wake()` call

`AgentLoop.Wake()` is non-blocking (buffered channel size 1). It fires on every inbound user message and on runtime-session creation.

**Subdirectory structure within `internal/navi/`:**

| Subdir | Purpose |
|--------|---------|
| `skill/` | OSS27 skill spec, loader, registry, policy engine, tool synthesizer |
| `heartbeat/` | Autonomous background scheduler (stub, not yet implemented) |
| `hooks/` | Hook registration and execution runner |
| `plugin/` | Plugin loading, registry, API, manifest parsing |
| `store/` | SQLite-backed chat, runtime-session, and runtime queue implementation |

**Tool execution:** The agent loop exposes **file tools** (ReadFileTool, ListDirTool, WriteFileTool) from `internal/navi/filetools/` when `WorkspaceDir` is set; these perform real read/list/write under governor path checks. Skills without an executor return an honest "not yet executable" message (no false success). Onboarding `onboarding_set_setup_complete` is real. Full MCP/REST skill execution is not yet implemented.

### `internal/orchestrator` — Orchestration Loop

`RunLoop(ctx, cfg)` ticks at a configured interval. Each tick:
1. `Governor.CheckDuration()` — verify autonomous duration not exceeded
2. `Governor.RecordAction()` — increment action counter; abort if budget exceeded
3. `LLM.Decide(ctx)` → `Decision{Events, Cost}` — ask LLM what to do
4. `Governor.RecordCost(decision.Cost)` — track USD cost; abort if ceiling exceeded
5. Publish `decision.Events` to the NATS bus

`RunOnce` is the testing variant (single tick, no ticker).

**Stateless design:** All state (directives, messages, tasks) is reconstructed from SQLite on every tick. There is no in-memory state machine in the orchestrator.

Key files:

| File | Purpose |
|------|---------|
| `loop.go` | Main `RunLoop` / `RunOnce` tick logic with governor checks |
| `adapter.go` | `LLMDirectiveAdapter` — reads directives, reconstructs conversation, calls LLM; ACT mode invokes `handleImplement` (DecomposeTasksTool → tasks + CmdTaskAssign) when no pending tasks |
| `planner.go` | Directive decomposition into task DAGs; dependency analysis |
| `prompts.go` | System prompt engineering for orchestrator decision-making |
| `tools.go` | Tool definitions (JSON Schema) exposed to orchestrator LLM |
| `stub_adapter.go` | Mock adapter for testing |

**IMPLEMENT mode:** When the active directive has mode ACT and no pending tasks, the adapter calls the LLM with `DecomposeTasksTool`, persists tasks, and publishes `CmdTaskAssign` per task. The **coder worker** (`internal/coder/`) subscribes to `CmdTaskAssign`, runs each task with file tools + LLM, and updates task status to completed/failed.

### `internal/store` — SQLite State Engine

Raw SQL only — **no ORM**. WAL mode. Key store files:

| File | Responsibility |
|------|---------------|
| `db.go` | Open/migrate the database |
| `directive.go` | Directive CRUD |
| `task.go` | Task CRUD + status transitions |
| `agent.go` | Agent registration and heartbeat tracking |
| `apikey.go` | API key management (create, validate, revoke) |
| `eventlog.go` | Append-only audit log |
| `settings.go` | Key-value settings (LLM config, setup wizard state) |
| `owner.go` | Owner identity |

### `internal/gateway` — HTTP/WS Server

Default listen address is from config (e.g. `config/runtime.yaml`), typically `:6284`; server code fallback when `Addr` is empty is `:8080`. Routes:

| Route | Auth | Notes |
|-------|------|-------|
| `GET /health` | None | Always public |
| `GET /api/version` | None | Always public |
| `GET /api/onboarding/status` | None | Public, CORS-enabled |
| `POST /api/onboarding/claim` | None | Public, CORS-enabled |
| `GET /api/onboarding/passport` | None | Public, CORS-enabled |
| `POST /auth/token` | Rate-limited | Shared secret exchange for JWT |
| `GET /` | None | Static file server from `web/` directory |
| `/api/*` (all others) | JWT | Claims-based authentication |
| Connector webhooks | X-API-Key | Shared secret header |

Full route list and auth: see [docs/specs/gateway-api.md](docs/specs/gateway-api.md).

The `web/` directory now has a minimal `index.html` placeholder. Full frontend does not exist yet (BLOCKER-1 partially resolved).

### `internal/governor` — Hard Constraint Enforcement

The `Governor` enforces limits that **the LLM cannot bypass**:

| Limit | Default (runtime.yaml) |
|-------|------------------------|
| `MaxActionBudget` | 1000 actions |
| `MaxRetries` | 5 |
| `CostCeiling` | $50.00 USD |
| `AutonomousDuration` | 72h |
| `RestrictToWorkspace` | `true` (path sandbox) |

Returns `*ErrGovernorTripped` when any limit is exceeded. This terminates the agent loop cleanly.

### `internal/hooks` — Lifecycle Hook System

Named hook points: `connector_started`, `connector_stopped`, `connector_error`, `message_received`, `message_sending` (mutable — can cancel or modify content), `message_sent`, `message_send_failed`, `health_changed`.

### `connectors/` and Plugin Connector Packages

The root `connectors/` package contains shared connector interfaces and types only. Concrete repo-owned connectors live under their owning plugin package, such as `plugins/telegram/connectors/telegram` and `plugins/slack/connectors/slack`.

The `Connector` interface requires `Name()`, `Start()`, `Stop()`, `Send()`, `IsRunning()`. Optional capability interfaces (typing indicators, media, webhooks, etc.) are discovered via type assertion — see `connectors/capabilities.go`.

| Connector | Status | Notes |
|-----------|--------|-------|
| `plugins/telegram/connectors/telegram` | Real | Long-polling + webhook modes; multi-account support via `TelegramAccountConfig` slices |
| `plugins/slack/connectors/slack` | Stub | Placeholder; basic structure only |

### `skills/` and Plugin Skill Packages

Skills are Markdown/YAML files following the OSS27 spec (`internal/navi/skill/spec.go`). Repo-owned capability skills live under `plugins/<name>/skills/...` and are exposed only through valid, active `plugin.yaml` manifests. The root `skills/` directory is reserved for skill-system support such as shared runtimes and documentation. The `ToolSynthesizer` converts loaded skills to LLM `ToolDefinition` objects.

**File tools** (ReadFile, ListDir, WriteFile) are real. Skills without an executor return an honest "not yet executable" message. Full MCP/REST or in-process skill execution is not yet implemented.

---

## Directive Modes

| Mode | Behavior |
|------|----------|
| `CHAT` | Conversational only. No actions. |
| `ADVISE` | Analysis and planning. No actions. |
| `ASSIST` | Routine tasks within pre-approved boundaries. |
| `ACT` | Full workflow execution with defined permissions. |
| `WATCH` | Monitor conditions; trigger alerts or actions on threshold. |

---

## Design Principles (Enforced)

1. **Explicit Cognitive Control** — Conversational generation, narration, and open-ended reasoning may live in prompts, but authoritative runtime control must live in code. The ICS control kernel in `internal/navi/inference/` owns focus arbitration, mode selection, governance handoff, execution supervision, recovery, and plan progression. Do not push hard control-state transitions or approval-boundary logic back into prompts.

2. **Schema Truth** — `internal/schema/` is the master. Never manually edit generated models; run `make generate-python`.

3. **No ORM** — Raw SQL only in `internal/store/`. No Gorm, no SQLX wrappers.

4. **No Synchronous Blocking in the Core Loop** — All blocking network calls go to background workers or NATS topics. The state loop must never block.

5. **No Nested Agent Hierarchies** — No "Manager of Managers." Agents are task-specific workers attached to a centralized orchestration loop.

6. **No Embedded MCP Runtime** — External tools use bridge patterns.

7. **Determinism** — Orchestrator loop logic must be deterministic.

8. **Execution Over Orchestration** — Prefer specialized task-oriented agents over generic "Manager" agents.

---

## Development Workflow

Follow: **spec → plan → implementation → validation → revision**. Documentation drives design.

```bash
# Full workflow
make test            # always run before committing
make build-all       # verify both binaries build
```

**Rules:**
- Write failing tests first. Run `make test` often.
- Use actual implementations in tests. Mock **only** LLM calls via fixtures.
- If you hit missing design decisions or broken inherited state, stop and notify the user — do not approximate.
- Commit message format: `<package>: <description>` (e.g., `orchestrator: add WATCH mode handler`)

## Agentic Workflow Imports

NAVI now carries a manually ported, repo-local subset of high-signal agentic workflow patterns.

- **Skills-first workflow surface:** Prefer adding or refining a workspace/user skill or plugin-owned skill before inventing a parallel command shim or harness-specific compatibility layer.
- **Workspace execution truth:** Use `WORKING_CONTEXT.md` in active workspaces to record current sprint truth, blockers, active queues, and short-lived constraints.
- **Surface inventory before recommendations:** When the user asks what NAVI can do, inspect the repo surface, skill surface, connector surface, MCP/config surface, and env-backed services before recommending new integrations.
- **Verification before closure:** Before claiming work is done, run the appropriate build/test/diff/safety checks for the scope of the change and summarize any blockers plainly.
- **Route to the right worker early:** Use `strategist` for planning, `coder` for implementation, `critic` for review, and `scout` for research instead of overloading one generic execution path.

Reference the NAVI-native integration guide at [`docs/agents/everything-claude-code-integration.md`](docs/agents/everything-claude-code-integration.md) and the imported advisory skills under [`plugins/workspace-surface-audit/skills/workspace-surface-audit/SKILL.md`](plugins/workspace-surface-audit/skills/workspace-surface-audit/SKILL.md) and [`plugins/verification-loop/skills/verification-loop/SKILL.md`](plugins/verification-loop/skills/verification-loop/SKILL.md).

- **Self-Extension Pipeline:** When a task fails due to a missing capability (Signal B) or the LLM cannot solve it with available tools (Signal A), use the **GapDetector** to trigger the **SkillBuilder**. Do not manually implement ad-hoc tools if a skill can be synthesized.
- **Artifact Materialization:** Use the `artifacts` store (OMN-118) to persist file outputs, research reports, and skill results. Ensure artifacts are linked to the correct `run_id`.


---

## Naming Conventions

| Context | Convention |
|---------|-----------|
| Product headings, badges | `NAVI` (uppercase) |
| Public package name | `open-navi` |
| CLI commands, Python import package, paths | `navi` (lowercase) |
| Go package names | snake_case directories, standard Go naming |
| Directive modes | `ALL_CAPS` constants (e.g., `DirectiveModeAct`) |
| NATS subjects | `navi.<stream>.<event>` |

---

## Current Blockers (→ Launch)

See `docs/tasks/blockers.md` for full details.

| # | Blocker | Status |
|---|---------|--------|
| 1 | `web/` frontend missing — gateway returned 404 for all UI requests | **RESOLVED** — `web/index.html` placeholder added |
| 2 | Compose definition for NaviD was missing | **RESOLVED** — `compose.yml` (+ `compose.strict.yml`) |
| 3 | CLI default port mismatch | **RESOLVED** — unified at 6284 |
| 4 | NATS hard runtime dependency — process exits if NATS not reachable | OPEN (mitigation: `runtime.yaml` now defaults to embedded NATS) |
| 5 | All skill/tool execution mocked — false success signals | **MITIGATED** — honest "not yet executable" for skills; ReadFile/ListDir/WriteFile real; coder worker runs tasks. MCP/REST still not implemented |

---

## Key Files for Onboarding

| File | Why it matters |
|------|---------------|
| `internal/config/config.go` | All configurable knobs + env var mapping |
| `internal/schema/directive.go` | Directive modes and status lifecycle |
| `internal/schema/task.go` | Task structure, risk levels, status FSM, cost attribution |
| `internal/llm/types.go` | `Provider` interface and shared LLM types |
| `internal/llm/fallback.go` | FallbackChain logic — understand before touching LLM wiring |
| `internal/bus/streams.go` | All NATS stream definitions |
| `internal/navi/navi.go` | NAVI agent struct — top-level agent lifecycle |
| `internal/navi/loop.go` | `AgentLoop` — conversational turn processing + tool execution |
| `internal/navi/filetools/filetools.go` | ReadFileTool, ListDirTool, WriteFileTool — workspace file I/O with governor |
| `internal/coder/runner.go` | Coder worker — executes tasks with file tools + LLM; subscribes to CmdTaskAssign |
| `internal/orchestrator/loop.go` | Orchestrator tick — directive decomposition |
| `internal/orchestrator/adapter.go` | LLM directive adapter — stateless conversation reconstruction |
| `internal/governor/governor.go` | Hard limits — never bypass these |
| `internal/gateway/server.go` | HTTP routes and server config |
| `connectors/connector.go` | Shared `Connector` interface; add new channel implementations under `plugins/<name>/connectors/<name>/` |
| `docs/AGENTS.md` | Directive rules for AI assistants |
| `docs/VISION.md` | Product strategy and architectural guardrails |
| `docs/tasks/blockers.md` | Active blockers before launch |

---

## Learned User Preferences

- When `docker compose` or `./scripts/relaunch.sh` fails on Windows, treat missing `dockerDesktopLinuxEngine` / absent Docker `Server` in `docker version` as Docker Desktop or WSL engine health, not as a NAVI code defect, until the engine is verified up.
- Keep local Cursor/Claude state (`.claude/settings.json`, `.claude/settings.local.json`, `.cursor/hooks/state/continual-learning.json`) out of Git; when already committed, use `git rm --cached` after confirming `.gitignore` entries.

## Learned Workspace Facts

- On Windows with Docker Desktop, a process listening on host port 6284 may be Docker’s port-forwarding for the published `6284:6284` mapping, not `navid` itself; force-killing that PID before `docker compose down` can destabilize Docker Desktop until it restarts.
- Docker Compose persists navid state via bind mounts from `NAVI_DATA_DRIVE` and `NAVI_DATA_DIR` in `.env` (defaults to repo `./.navi` when unset); the legacy `navi_navid-data` named volume is no longer used.
- `scripts/reset-navi.sh` (and `make force-reset` / `make force-reset-docker`) load `.env`, wipe the resolved data root, tear down `compose.yml` and `compose.strict.yml`, and run `docker compose down` before any host port kill so Compose releases port 6284 first.
- `scripts/relaunch.sh` preflights `docker version` (with a targeted message when the `dockerDesktopLinuxEngine` pipe is missing) and runs `docker compose down --remove-orphans` before `docker compose up --build -d`.
- Scheduling spans two paths: in-chat delays via `navi.messaging.send_reply` (`delay_seconds`, per-message cap in runtime executor), and persistent jobs via `internal/cron` plus the `core-scheduler` plugin (`skill.core-scheduler.schedule_task`); wiring the runtime alone does not make the chat agent aware — update prompts, skill docs, and tool exposure when adding schedule capabilities.
