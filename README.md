# NAVI

**Status:** Active
**Last Updated:** 2026-06-01
**Current Phase:** 15 - runtime reliability and self-extension hardening

NAVI is a local-first personal AI runtime written in Go. **NaviD** is the long-running server. **NaviExe** is the native client binary used by the packaged Node and Python CLIs, and **PET** connects to NaviD through the gateway.

NaviD has two supported runtime modes:

- **Docker mode:** containerized, reproducible, deployment-friendly runtime.
- **Local daemon mode:** native host process for persistent local operation without Docker, launched and controlled through the npm or pip `navi` CLI.

## What Ships Today

- **NaviD** (`navid`): boots SQLite, embedded NATS, the runtime coordinator, the agent loop, the orchestrator, the gateway, workers, reflection, onboarding, and heartbeat.
- **Packaged CLI** (`navi` from the `open-navi` npm or pip package): user-facing CLI for chat, onboarding, status, sessions, models, connectors, skills, activity, runs, diagnostics, and local daemon lifecycle.
- **PET:** UI client using the same gateway contract.
- Single-owner, local-first defaults in `config/runtime.yaml` (embedded NATS, SQLite).

Default client URL: `http://localhost:6284`.

## Quick Start: Docker Mode

From this repository root (`projects/navi/`, where `compose.yml` lives):

```bash
docker compose -f compose.yml up -d --build
```

Strict isolated mode binds loopback only, uses a named data volume, and requires an explicit reachable LLM URL:

```bash
NAVI_OLLAMA_URL=https://api.openai.com/v1 docker compose -f compose.strict.yml up -d --build
```

Stop Docker mode:

```bash
docker compose -f compose.yml down
```

Shortcuts: `make docker-up`, `make docker-up-strict`, `make docker-down`, `make docker-down-strict`.

## Quick Start: Local Daemon Mode

Install the public `open-navi` package through Node or Python, then run the installed `navi` command:

```bash
npm install open-navi
navi daemon start
```

or:

```bash
pip install open-navi
navi daemon start
```

Useful local daemon commands are the same from either package:

```bash
navi daemon status
navi daemon logs
navi daemon stop
navi daemon restart
```

The local daemon command manages only a host `navid` process it started. It stores lifecycle metadata under `~/.navi/daemon/`:

- `navid.pid` - managed process ID
- `navid.json` - command, cwd, gateway URL, and log path
- `navid.log` - daemon stdout/stderr

By default, local runtime data remains relative to the daemon working directory:

- SQLite: `navi.db`
- embedded NATS JetStream: `jetstream/`
- workspace: `workspace/`
- configuration: `config/runtime.yaml` plus environment overrides
- gateway: `:6284`

Direct `./bin/navid` and `./bin/navi.exe daemon ...` execution is developer-only. User daemon control enters through the npm or pip `open-navi` package; repo-local developer shortcuts set `NAVI_DISTRIBUTION_CHANNEL=dev` explicitly. Wrapper developers can set `NAVI_NATIVE_BIN` to point the npm/pip wrapper at a locally built native `navi` executable.

For an alternate local port:

```bash
navi daemon start -addr 127.0.0.1:6285
```

## Health

Both runtime modes expose the same health endpoint:

```bash
curl http://localhost:6284/health
```

Expect `200`.

## Quick Start: NaviExe

After NaviD is up in either runtime mode:

```bash
navi init
navi status
navi chat
```

Use `-url` or `NAVI_GATEWAY_URL` if NaviD is not on `http://localhost:6284`.

## Current Architecture

| Area | Packages | What it does |
|------|----------|--------------|
| Entry points | `cmd/navid`, `cmd/navi` | NaviD daemon and NaviExe CLI |
| Foreground runtime | `internal/runtime`, `internal/navi` | Sessions, runs, tools, proposals, summarization |
| Directive orchestration | `internal/orchestrator` | Planning, decomposition, task assignment |
| Workers | `internal/coder`, `internal/critic`, `internal/strategist`, `internal/scout` | Task execution |
| State | `internal/store`, `internal/worldmodel`, `internal/schema` | SQLite, world model, types |
| Experience Layer | `internal/navi/experience`, `config/personas`, gateway experience endpoints | Behavioral identity, persona/profile shaping, output preferences, and owner-facing experience controls |
| Gateway | `internal/gateway` | HTTP/WebSocket API, onboarding |
| Capability execution | `plugins/<name>`, `internal/navi/skill`, `skills/_runtime` | Plugin-owned skills plus skill framework/runtime support |
| Connectors | `plugins/<name>/connectors`, `connectors`, `internal/connectors` | Plugin-owned connector implementations plus shared interfaces/framework |
| LLM providers | `plugins/llm-*`, `internal/llm` | Provider plugins plus LLM control-plane/framework |

## Configuration

Loads `config/runtime.yaml` then environment overrides. See [`internal/config/config.go`](internal/config/config.go) and [docs/specs/configuration.md](docs/specs/configuration.md).

Docker mode overrides state paths in Compose (`/navi/data/...`). Local daemon mode uses the YAML defaults unless overridden with `navi daemon start -addr ...` or environment variables such as `NAVI_GATEWAY_ADDR`, `NAVI_SQLITE_PATH`, `NAVI_WORKSPACE_DIR`, `NAVI_NATS_URL`, `NAVI_OLLAMA_URL`, and `NAVI_GATEWAY_SHARED_SECRET`.

## Documentation

- [docs/README.md](docs/README.md)
- [Visual Architecture Guide](docs/architecture/navi-system-architecture-guide.md)
- [docs/runbooks/run-navi.md](docs/runbooks/run-navi.md)
- [docs/specs/gateway-api.md](docs/specs/gateway-api.md)
- [docs/VISION.md](docs/VISION.md)

## Current Gaps

- The browser Console (`web-src/navi-console/`, React + Vite, built into `web/`) is feature-complete for chat but lacks e2e/integration test coverage.
- Native token streaming is partial in places.
- Deployment remains single-node / owner-centric rather than multi-tenant hosting.
