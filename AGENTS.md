# AGENTS.md — NAVI Codebase Guide

NAVI is a **two-binary** personal AI runtime (Go 1.24, module `github.com/ceoai/navi`).
`navid` (server: SQLite + embedded NATS + agent loop + orchestrator + gateway) and
`navi` (CLI client via NaviExe). Two supported runtime modes: Docker Compose and
local daemon. PET is an optional UI client.

This file supersedes `CLAUDE.md` — new agents should read this one.

---

## Build Artifact Policy — Strict

**Never run bare `go build ./cmd/...`.** It drops the binary in the repo root as an
untracked file, polluting `git status`.

- **Always use Make targets:** `make build` (→ `bin/navid`), `make build-cli` (→ `bin/navi.exe`), `make build-all`
- Compile-only check (no artifact): `go build -o /dev/null ./cmd/navid/`
- Explicit correct build: `go build -o bin/navid ./cmd/navid/`

---

## Key Dev Commands

```bash
make test              # go test ./cmd/... ./internal/... ./connectors/... ./plugins/... -count=1
make build-all         # builds both binaries + frontend Console
make conformance       # language-layer contract guard (builds + runs cmd/langguard)
make test-e2e          # tagged e2e scenarios (requires docker)
make test-frontend     # pnpm typecheck + vitest for web-src/ workspace
make generate          # regenerate Python + TypeScript governed types from Go sources
make docker-up         # docker compose -f compose.yml up --build -d
make docker-up-strict  # isolated compose (requires NAVI_OLLAMA_URL)
make daemon-start      # build + NAVI_DISTRIBUTION_CHANNEL=dev ./bin/navi.exe daemon start
```

CI runs: `test` → `conformance` (on push/PR), plus `docker-navid` + `docker-strict`
integration health checks, and a `frontend` job (typecheck + vitest, pnpm workspace).

---

## Architecture Essentials

### Dual-Plane LLM
**Inference Plane** (stateless `llm.Provider`) and **Control Plane** (stateful
`LLMService`) must be strictly separated. Never put selection/routing/retry logic in
a Provider. Never import concrete provider plugins from `internal/llm`. Provider
plugins register through `cmd/navid/plugin_bootstrap.go`.

### Plugin Connectors
- `connectors/` (root) — shared connector interfaces & types only
- `internal/connectors/` — manager, registry, lifecycle, worker orchestration
- `plugins/<name>/connectors/<name>/` — actual implementations (e.g. `plugins/telegram/connectors/telegram/`)
- Capabilities discovered via type assertion on optional interfaces (`connectors/capabilities.go`)

### Schema Truth
`internal/schema/` is the canonical source. Python models in `schema/python/` and
TypeScript types in `web-src/navi-console/src/types/generated/` are generated.
**Never edit them by hand** — run `make generate`.

### Persistence
Raw SQL only in `internal/store/` — no ORM, no GORM, no SQLX. WAL mode.

---

## Key Env Vars

| Var | Default |
|-----|---------|
| `NAVI_GATEWAY_ADDR` | `:6284` |
| `NAVI_ANTHROPIC_KEY` | — |
| `NAVI_OPENAI_KEY` | — |
| `NAVI_OPENROUTER_KEY` | — |
| `NAVI_OLLAMA_URL` | `http://localhost:11434/v1` |
| `NAVI_SQLITE_PATH` | `navi.db` |
| `NAVI_GATEWAY_SHARED_SECRET` | — |
| `NAVI_NATS_URL` | `embedded` (in-process) |
| `NAVI_DEBUG` | — (set `1` for debug logs) |
| `NAVI_HEARTBEAT_INTERVAL` | `30m` |

Env vars always override `config/runtime.yaml`.

---

## Frontend

- **Source:** `web-src/` pnpm workspace (`web-src/package.json`)
  - `packages/navi-ui/` — `@navi/ui` shared library (tokens, theme, primitives, GenUI runtime)
  - `navi-console/` — React 19 + Vite Console app
- **Build output:** `web/` (compiled, served by gateway at `GET /`)
- **Test:** `pnpm --filter @navi/ui test`, `pnpm --filter navi-console test`
- **Dev server:** `localhost:5173` (Vite, CORS origin patterns in runtime.yaml)

---

## Go Cache Policy

Use default paths from `go env GOCACHE` and `go env GOMODCACHE`. Do not set
repo-local Go caches (`.gocache`, `.gomodcache`, `.codex-gocache`, etc.) — they
are bloat and should be deleted, not recreated.

---

## Current State

| Area | Status |
|------|--------|
| CoderAgent, CriticAgent, StrategistAgent | Real LLM-backed workers |
| File tools (ReadFile, ListDir, WriteFile) | Real, governor-sandboxed |
| Skills without executor | Honest "not yet executable" |
| MCP/REST skill execution | Not implemented |
| Telegram connector | Real (long-polling + webhook, multi-account) |
| Slack connector | Stub |
| Heartbeat scheduler | Stub |
| Frontend Console | Feature-complete for chat, lacks e2e test coverage |

---

## Naming Conventions

| Context | Convention |
|---------|-----------|
| Product headings, badges | `NAVI` (uppercase) |
| Public package name | `open-navi` |
| CLI command, Python import package | `navi` (lowercase) |
| Go package dirs | snake_case |
| NATS subjects | `navi.<stream>.<event>` |

---

## Learned Workspace Facts

- On Windows, a process on port 6284 may be Docker Desktop's port-forwarding, not
  `navid` itself. Force-killing that PID before `docker compose down` can
  destabilize Docker Desktop.
- Docker Compose binds state from `.env` vars `NAVI_DATA_DRIVE` / `NAVI_DATA_DIR`
  (defaults to repo `./.navi`). No named volume is used.
- `scripts/reset-navi.sh` (and `make force-reset` / `make force-reset-docker`)
  load `.env`, wipe the data root, tear down all compose files, then run
  `docker compose down` to release port 6284 before any host kill.
- `scripts/relaunch.sh` preflights `docker version` and runs
  `docker compose down --remove-orphans` before `docker compose up --build -d`.
- Local Cursor/Claude state (`.claude/settings.json`, `.cursor/hooks/state/...`)
  should stay out of Git. If committed, `git rm --cached` after confirming
  `.gitignore` entries.
- Scheduling has two paths: in-chat delays (`navi.messaging.send_reply` with
  `delay_seconds`) and persistent cron jobs via `internal/cron` +
  `core-scheduler` plugin. Both prompt and tool exposure must be updated when
  adding schedule capabilities.
