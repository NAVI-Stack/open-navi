# Run NAVI

**Status:** Active
**Last Updated:** 2026-06-01

This runbook describes the two supported ways to run **NaviD** and connect with the packaged `navi` CLI or **PET**:

- **Docker mode** - containerized runtime for reproducible local or deployment-style operation.
- **Local daemon mode** - native host process for persistent local operation without Docker, launched and controlled through the npm or pip CLI package.

Default client URL: `http://localhost:6284`.

## Prerequisites

| Requirement | Docker mode | Local daemon mode |
|-------------|-------------|-------------------|
| Working directory | Repository root containing `compose.yml` | Any directory with access to the packaged CLI and runtime config |
| Runtime dependency | Docker + Docker Compose | Node.js or Python package install that carries the native NAVI binary |
| LLM access | `compose.yml` defaults to `http://host.docker.internal:11434/v1` | `config/runtime.yaml` defaults to `http://localhost:11434/v1` |
| State location | Bind mount / named volume under `/navi/data` | Files relative to daemon cwd unless env overrides are set |

## Start NaviD: Docker Mode

**Permissive default:** editable host-backed data/config/skills, published on `:6284`.

```bash
docker compose -f compose.yml up -d --build
```

**Strict isolated:** use only `compose.strict.yml`; loopback binding, data volume only, internal bridge network, no host bind mounts, no `host.docker.internal`.

```bash
NAVI_OLLAMA_URL=https://api.example.com/v1 docker compose -f compose.strict.yml up -d --build
```

Do not run `docker compose -f compose.yml -f compose.strict.yml` together. Docker Compose merges `ports`, `volumes`, and `extra_hosts` additively, so the strict file cannot subtract permissive mounts.

Stop Docker mode:

```bash
docker compose -f compose.yml down
```

If you use strict mode:

```bash
docker compose -f compose.strict.yml down
```

Shortcuts: `make docker-up`, `make docker-up-strict`, `make docker-down`, `make docker-down-strict`.

Docker logs:

```bash
docker compose -f compose.yml logs -f navid
```

## Start NaviD: Local Daemon Mode

Install the public `open-navi` package through Node or Python. Both packages expose the installed command as `navi`:

```bash
npm install open-navi
```

or:

```bash
pip install open-navi
```

Start the daemon:

```bash
navi daemon start
```

Start on an alternate local port:

```bash
navi daemon start -addr 127.0.0.1:6285
```

Check status:

```bash
navi daemon status
```

Stop the daemon:

```bash
navi daemon stop
```

Restart:

```bash
navi daemon restart
```

View logs:

```bash
navi daemon logs
```

Local daemon mode manages only the host process started through packaged `navi daemon start`. It does not stop Docker containers or unmanaged foreground `navid` processes. Direct `./bin/navi.exe daemon ...` is a developer-only repo workflow and is rejected unless `NAVI_DISTRIBUTION_CHANNEL=dev` is set explicitly.

### Local Daemon Paths

Lifecycle metadata:

| Path | Purpose |
|------|---------|
| `~/.navi/daemon/navid.pid` | PID for the managed host daemon |
| `~/.navi/daemon/navid.json` | command, cwd, gateway URL, and log path |
| `~/.navi/daemon/navid.log` | stdout/stderr from the daemon |

Runtime state defaults:

| Path / setting | Default |
|----------------|---------|
| Config | `config/runtime.yaml` |
| SQLite | `navi.db` |
| Embedded NATS / JetStream | `jetstream/` beside SQLite |
| Workspace | `workspace/` |
| Gateway | `:6284` |
| LLM URL | `http://localhost:11434/v1` |

Override local daemon state with environment variables before starting:

```bash
NAVI_SQLITE_PATH=/tmp/navi/navi.db navi daemon start
NAVI_WORKSPACE_DIR=/tmp/navi/workspace navi daemon start
NAVI_OLLAMA_URL=http://localhost:11434/v1 navi daemon start
```

PowerShell examples:

```powershell
$env:NAVI_SQLITE_PATH = "C:\navi\data\navi.db"; navi daemon start
```

Use `./bin/navid` directly only for foreground developer debugging. Use packaged `navi daemon start` for user-facing persistent background operation.

For wrapper development, set `NAVI_NATIVE_BIN` to point the npm/pip wrapper at a locally built native `navi` executable.

## Health Check

Both modes expose:

```bash
curl http://localhost:6284/health
```

Expect `200`.

## Connect with NaviExe or PET

Clients connect to a running NaviD in either runtime mode.

Run onboarding:

```bash
navi init
```

Open the Console:

```bash
navi console
```

Start chat:

```bash
navi chat
```

Useful flags: `-url`, `-api-key`, `-new`, `-debug`.

## Configuration Shortcuts

Typical environment variables:

```bash
NAVI_OLLAMA_URL=...
NAVI_ANTHROPIC_KEY=...
NAVI_OPENAI_KEY=...
NAVI_OPENROUTER_KEY=...
NAVI_GATEWAY_ADDR=...
NAVI_GATEWAY_SHARED_SECRET=...
NAVI_SQLITE_PATH=...
NAVI_WORKSPACE_DIR=...
```

See [../specs/configuration.md](../specs/configuration.md) for the full map.

## Custom Compose

Copy [../../compose.custom.example.yml](../../compose.custom.example.yml) to `compose.local.yml` (gitignored) and run:

```bash
docker compose -f compose.yml -f compose.local.yml up -d --build
```

## Resetting a Local Instance

See [reset-instance.md](reset-instance.md). Use Docker reset steps for Docker mode and local file reset steps for local daemon mode.

## Troubleshooting

| Symptom | Check |
|---------|-------|
| Container exits | `docker compose logs navid`; LLM URL and secrets |
| Strict mode errors on start | Set `NAVI_OLLAMA_URL` before `compose.strict.yml up` |
| Local daemon does not start | `navi daemon logs`; confirm the packaged native binary exists and port `6284` is free |
| CLI cannot connect | NaviD running; `-url`; firewall; `navi daemon status` |
| Remote auth | Pass `-api-key` / `NAVI_API_KEY` |

[runbooks INDEX](INDEX.md) | [docs INDEX](../INDEX.md)
