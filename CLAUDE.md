# CLAUDE.md — NAVI AI Codebase Guide

> Mandatory reading for AI coding assistants working in this repository.
> For the full product vision and manifesto, see [`docs/VISION.md`](docs/VISION.md) and [`docs/AGENTS.md`](docs/AGENTS.md).

---

## What This Project Is

NAVI is a **two-binary** personal AI system written in Go. **NaviD** (`navid`) is the long-running server; **NaviExe** (`navi`) and PET (UI) are clients. The **supported NaviD runtime** is Docker Compose only — running `navid` directly on the host is unsupported and deprecated.

| Component | Role |
|-----------|------|
| `navid` (NaviD) | Server — orchestration, HTTP/WebSocket API, embedded NATS + SQLite (run via **Docker Compose**) |
| `navi` (NaviExe) | CLI client — connects to NaviD over HTTP/WebSocket (never starts the server) |
| PET | UI client — connects to NaviD (never starts the server) |

NAVI is **LLM-agnostic**: it supports Anthropic, OpenAI, OpenRouter, and Ollama through a unified `llm.Provider` interface, with a `FallbackChain` that tries providers in order (Ollama → Anthropic → OpenAI → OpenRouter by default).

Current state: CoderAgent, CriticAgent, and StrategistAgent are real LLM-backed workers. File tools are real; full MCP/REST (non-file) skill execution is not yet implemented. The console frontend lives in `web-src/navi-console/` and builds into `web/`. Shared UI (design tokens, theming, primitives, interaction patterns — Dialog/Toast/ConfirmDialog — and a pluggable generative-UI runtime) lives in the `@navi/ui` library at `web-src/packages/navi-ui/`, consumed by the Console **as source** via the `web-src/` pnpm workspace alias. As of M2 the library also builds a standalone `dist/` artifact (`pnpm --filter @navi/ui run build`, via `vite.lib.config.ts`) for external consumers, ships an opt-in `@navi/ui/openui` OpenUI-Lang adapter (depends on the optional peer `@openuidev/react-lang`; the core stays dependency-free), supports streaming/progressive `<GenUI>`, and has a Ladle workbench (`pnpm --filter @navi/ui run ladle`) — see `docs/architecture/navi-ui-library.md`.

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
│   ├── llm/            # Provider adapters + FallbackChain
│   ├── navi/           # Agent runtime: loop, chats, runtime sessions, heartbeat, skills, personas, plugins
│   ├── orchestrator/   # Orchestration loop, planner, LLM adapter, tools
│   ├── schema/         # Go types — canonical source of truth for Directive, Task, Agent
│   └── store/          # SQLite WAL — tasks, directives, API keys, event log, compatibility stores
├── connectors/         # External connector implementations
│   ├── connector.go    # Connector interface
│   ├── base.go         # Helper base struct
│   ├── capabilities.go # Optional capability interfaces (type assertions)
│   ├── errors.go       # Connector error types
│   ├── telegram/       # REAL implementation (long-polling + webhook, multi-account)
│   └── slack/          # STUB implementation
├── config/
│   ├── runtime.yaml    # Default runtime configuration
│   └── personas/       # YAML experience-profile definitions (navi, wizard)
├── skills/             # Zero-code skill definitions (.md/.yaml files, OSS27 spec)
│   ├── github/         # GitHub API operations (SKILL.md)
│   ├── i2c/            # I2C bus protocol (SKILL.yaml)
│   ├── skill-creator/  # Meta-skill for creating skills (SKILL.yaml)
│   ├── spi/            # SPI bus protocol (SKILL.yaml)
│   ├── summarize/      # Text summarization (SKILL.md)
│   └── telegram/       # Telegram operations (SKILL.yaml)
├── schema/             # JSON Schema definitions + Python codegen
│   ├── jsonschema/     # JSON Schema files (agent, event, task, claim, enums)
│   └── python/         # Generated Python models (DO NOT EDIT — run make generate-python)
├── docs/               # VISION.md, AGENTS.md, ADRs, task lists, blockers
├── test/
│   └── e2e/
│       └── smoke_test.go  # E2E smoke tests (health, chat, gateway)
├── web/                # Built browser Console (compiled output of web-src/navi-console/)
├── web-src/            # pnpm workspace (pnpm-workspace.yaml + lockfile)
│   ├── packages/
│   │   └── navi-ui/    # @navi/ui — shared UI library (tokens, theme, primitives, generative UI)
│   └── navi-console/   # React 19 + Vite Console source; consumes @navi/ui
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
# NaviD (supported runtime)
docker compose -f compose.yml up -d --build
# Isolated mode (requires NAVI_OLLAMA_URL; do not combine with compose.yml — see docs/runbooks/run-navi.md)
#   NAVI_OLLAMA_URL=https://... docker compose -f compose.strict.yml up -d --build

# Native builds (development / tests only — not a supported NaviD runtime)
make build           # → bin/navid
make build-cli       # → bin/navi.exe (NaviExe)
make build-all       # both

make cli             # go run ./cmd/navi/

# Test
make test            # go test ./cmd/... ./internal/... ./connectors/... -count=1

# Docker shortcuts
make docker-up       # docker compose defaults (compose.yml)
make docker-up-strict
make docker-down

# Code generation
make generate-python # sync Python schema models from internal/schema/

# Cleanup
make clean           # rm -rf bin/
```

### Build Artifact Policy — STRICTLY ENFORCED

**All compiled binaries MUST land in `bin/`. Never anywhere else.**

- **Always use Make targets** — `make build`, `make build-cli`, `make build-all`. These write to `bin/` and `bin/` is gitignored.
- **Never run bare `go build ./cmd/...`** without an explicit `-o` flag. The Go toolchain drops the output binary into the current working directory (the repo root), creating an untracked file that pollutes `git status` and breaks repository hygiene.
- **To check compilation only** (no binary on disk): `go build -o /dev/null ./cmd/navid/ && go build -o /dev/null ./cmd/navi/`
- **To build to the correct location explicitly**: `go build -o bin/navid ./cmd/navid/` — never omit `-o bin/<name>`.
- If you find a `navid` or `navi.exe` binary in the repo root, **delete it** (`rm navid navi.exe`). It is a build artifact left by a bare `go build` call that used the wrong output path.

```bash
# WRONG — drops binary in repo root as untracked file
go build ./cmd/navid/

# RIGHT — compile-check only, no file produced
go build -o /dev/null ./cmd/navid/

# RIGHT — build to the correct location
make build
# or explicitly:
go build -o bin/navid ./cmd/navid/
```

### Go Cache Policy

Use the default Go cache locations from `go env GOCACHE` and `go env GOMODCACHE`.
Do not set `GOCACHE` or `GOMODCACHE` to repo-local directories such as `.gocache`,
`.gomodcache`, `.codex-gocache`, `.codex-gomodcache`, `.codex-go-cache`, or
`.codex-go-modcache`. Repo-local Go caches are bloat and should be deleted, not
preserved or recreated.

After NaviD is running, the CLI launches an interactive setup wizard on first use. Use `./bin/navi.exe -new` to start a fresh chat after setup is complete.

---

## Configuration

Configuration is layered: `config/runtime.yaml` → environment variable overrides.

**Key environment variables** (env vars always win over YAML):

| Env Var | Purpose |
|---------|---------|
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

Implementations:
- `AnthropicProvider` — Claude models (claude-opus-4-6, claude-sonnet-4)
- `OpenAIProvider` — GPT models + OpenRouter compatibility (shared implementation)
- `OllamaProvider` — Local inference endpoint
- `FallbackChain` — Tries multiple providers in sequence with per-provider cooldowns and error classification (retriable vs. non-retriable)
- `DynamicProvider` — Hot-swappable provider wrapper (updates runtime without restart)

**Default fallback chain:** Ollama → Anthropic → OpenAI → OpenRouter

Model routing is configured via `LLMConfig.Routes` (map of logical role → `ModelRoute{Primary, Fallbacks}`). Legacy `*Model` fields are supported but deprecated in favor of Routes.

Use `NewFallbackChain(candidates)` — do **not** hard-code provider selection in Go logic.

### `internal/navi` — Agent Runtime

The `NAVI` struct is the core agent. It owns:
- `ChatStore` — CRUD for durable chat transcripts and messages
- `RuntimeSessionStore` — CRUD for execution/runtime lifetimes linked to chats
- `SkillRegistry` — loads `.md`/`.yaml` skill definitions from `skills/`
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

The `web/` directory holds the **built** browser Console (compiled output of `web-src/navi-console/`, a React 19 + Vite app). The gateway serves it from `web/` at `GET /` and redirects to `/onboarding` on first run. The Console chat UX is feature-complete (markdown rendering, code copy, edit/resend, regenerate, continue, response variants, feedback, tool chips, streaming/pending/failure states); the remaining gap is e2e/integration test coverage. See `docs/architecture/navi-console-frontend.md`.

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

### `connectors/` — External Channel Connectors

The `Connector` interface requires `Name()`, `Start()`, `Stop()`, `Send()`, `IsRunning()`. Optional capability interfaces (typing indicators, media, webhooks, etc.) are discovered via type assertion — see `connectors/capabilities.go`.

| Connector | Status | Notes |
|-----------|--------|-------|
| `telegram/` | Real | Long-polling + webhook modes; multi-account support via `TelegramAccountConfig` slices |
| `slack/` | Stub | Placeholder; basic structure only |

### `skills/` — Zero-Code Agent Capabilities

Skills are Markdown/YAML files following the OSS27 spec (`internal/navi/skill/spec.go`). The `SkillRegistry` loads them from the `skills/` directory at startup. The `ToolSynthesizer` converts skills to LLM `ToolDefinition` objects.

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

1. **Zero Framework Cognition** — All decision logic belongs in LLM prompts and task specs, not Go code. If you find yourself encoding heuristics in Go, move them to a prompt.

2. **Schema Truth** — `internal/schema/` is the master. Never manually edit generated models; run `make generate-python`.

3. **No ORM** — Raw SQL only in `internal/store/`. No Gorm, no SQLX wrappers.

4. **No Synchronous Blocking in the Core Loop** — All blocking network calls go to background workers or NATS topics. The state loop must never block.

5. **No Nested Agent Hierarchies** — No "Manager of Managers." Agents are task-specific workers attached to a centralized orchestration loop.

6. **No Embedded MCP Runtime** — External tools use bridge patterns.

7. **Determinism** — Orchestrator loop logic must be deterministic.
 
8. **Execution Over Orchestration** — Prefer specialized task-oriented agents over generic "Manager" agents.

9. **LLM Dual-Plane Separation** — Maintain a strict boundary between the **Inference Plane** (`llm.Provider`: stateless, per-call) and the **Control Plane** (`LLMService`: stateful, policy-aware). Mixing both in the same package or component is a "smell" and should be flagged in review. Refer to `docs/architecture/llm-dual-plane.md` for details.

---

## Development Workflow

Follow: **spec → plan → implementation → validation → revision**. Documentation drives design.

```bash
# Full workflow
make test            # always run before committing
make build-all       # verify both binaries build (outputs to bin/, never repo root)
```

**Rules:**
- Write failing tests first. Run `make test` often.
- Use actual implementations in tests. Mock **only** LLM calls via fixtures.
- If you hit missing design decisions or broken inherited state, stop and notify the user — do not approximate.
- Commit message format: `<package>: <description>` (e.g., `orchestrator: add WATCH mode handler`)
- **Never run `go build ./cmd/...` without `-o bin/<name>`** — bare `go build` drops binaries in the repo root. Use `make build` / `make build-all`, or `go build -o /dev/null` for compile-only checks. See "Build Artifact Policy" above.

---

## Naming Conventions

| Context | Convention |
|---------|-----------|
| Product headings, badges | `NAVI` (uppercase) |
| Package names, CLI commands, paths | `navi` (lowercase) |
| Go package names | snake_case directories, standard Go naming |
| Directive modes | `ALL_CAPS` constants (e.g., `DirectiveModeAct`) |
| NATS subjects | `navi.<stream>.<event>` |

---

## Current Blockers (→ Launch)

See `docs/tasks/blockers.md` for full details.

| # | Blocker | Status |
|---|---------|--------|
| 1 | `web/` frontend missing — gateway returned 404 for all UI requests | **RESOLVED** — React + Vite Console (`web-src/navi-console/`) builds into `web/` and is served by the gateway; remaining work is e2e test coverage, not a missing UI |
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
| `internal/llm/types.go` | `Provider` interface — extend here for new LLMs |
| `internal/llm/fallback.go` | FallbackChain logic — understand before touching LLM wiring |
| `internal/bus/streams.go` | All NATS stream definitions |
| `internal/navi/navi.go` | NAVI agent struct — top-level agent lifecycle |
| `internal/navi/chat.go` | Durable chat transcript domain model |
| `internal/navi/runtime_session.go` | Runtime-session domain model linking execution lifetimes to chats |
| `internal/navi/message_intake.go` | Message intake surface that routes chats into runtime sessions |
| `internal/navi/loop.go` | `AgentLoop` — conversational turn processing + tool execution |
| `internal/navi/filetools/filetools.go` | ReadFileTool, ListDirTool, WriteFileTool — workspace file I/O with governor |
| `internal/coder/runner.go` | Coder worker — executes tasks with file tools + LLM; subscribes to CmdTaskAssign |
| `internal/orchestrator/loop.go` | Orchestrator tick — directive decomposition |
| `internal/orchestrator/adapter.go` | LLM directive adapter — stateless conversation reconstruction |
| `internal/governor/governor.go` | Hard limits — never bypass these |
| `internal/gateway/server.go` | HTTP routes and server config |
| `connectors/connector.go` | `Connector` interface — extend for new channels |
| `docs/AGENTS.md` | Directive rules for AI assistants |
| `docs/VISION.md` | Product strategy and architectural guardrails |
| `docs/tasks/blockers.md` | Active blockers before launch |
