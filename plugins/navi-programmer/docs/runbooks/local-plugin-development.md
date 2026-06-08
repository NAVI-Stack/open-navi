# Local Plugin Development

**Status:** Active
**Last Updated:** 2026-05-08
**Related Task:** NP-001

## Purpose

This runbook defines how to use the NAVI Programmer source package with NAVI
core before NAVI Store exists.

## Current State

The source package exists at:

```text
C:\Users\evirg\codespace\NAVI-Ecosystem\projects\navi\plugins\navi-programmer
```

NAVI core has a plugin loader that scans configured plugin roots for one of:

```text
plugin.yaml
manifest.json
```

NAVI Programmer currently provides:

```text
plugin.yaml
```

## NAVI Core Plugin Roots

NAVI core currently creates its plugin loader in `cmd/navid/main.go` with three
roots, in priority order:

1. Workspace plugin root:

   ```text
   <NAVI_WORKSPACE_DIR>/plugins
   ```

   With default local config, this is usually:

   ```text
   C:\Users\evirg\codespace\NAVI-Ecosystem\projects\navi\workspace\plugins
   ```

2. Global plugin root:

   ```text
   <user home>\.navi\plugins
   ```

   On this Windows workstation:

   ```text
   C:\Users\evirg\.navi\plugins
   ```

3. Built-in plugin root:

   ```text
   plugins
   ```

   This is relative to the NaviD working directory.

Duplicate plugin IDs are resolved by priority. A workspace plugin wins over a
global plugin, and a global plugin wins over a built-in plugin.

## Recommended Native Development Path

When running NaviD from the NAVI repository, no copy or link is required:
`plugins/navi-programmer/plugin.yaml` is under the built-in plugin root.

Use the global plugin root with a directory junction only when testing this
plugin from another NAVI checkout or a packaged copy.

This keeps this repo as the source of truth while allowing another NAVI runtime
to discover it without a core code change.

From PowerShell:

```powershell
$Source = "C:\Users\evirg\codespace\NAVI-Ecosystem\projects\navi\plugins\navi-programmer"
$TargetRoot = Join-Path $env:USERPROFILE ".navi\plugins"
$Target = Join-Path $TargetRoot "navi-programmer"

New-Item -ItemType Directory -Force -Path $TargetRoot | Out-Null
New-Item -ItemType Junction -Path $Target -Target $Source
```

If `New-Item -ItemType Junction` is not appropriate for the environment, use a
copy fallback:

```powershell
$Source = "C:\Users\evirg\codespace\NAVI-Ecosystem\projects\navi\plugins\navi-programmer"
$TargetRoot = Join-Path $env:USERPROFILE ".navi\plugins"
$Target = Join-Path $TargetRoot "navi-programmer"

New-Item -ItemType Directory -Force -Path $TargetRoot | Out-Null
robocopy $Source $Target /E /XD .git
```

The copy fallback must be repeated after source changes. The junction/symlink
path is better for active development.

## Workspace-Local Alternative

Use this when you want the plugin to be visible only to a specific NAVI
workspace.

```powershell
$Source = "C:\Users\evirg\codespace\NAVI-Ecosystem\projects\navi\plugins\navi-programmer"
$TargetRoot = "C:\Users\evirg\codespace\NAVI-Ecosystem\projects\navi\workspace\plugins"
$Target = Join-Path $TargetRoot "navi-programmer"

New-Item -ItemType Directory -Force -Path $TargetRoot | Out-Null
New-Item -ItemType Junction -Path $Target -Target $Source
```

This root has higher priority than the global root.

## Docker Compose Development Path

The default NAVI Docker Compose file does not mount host plugin directories into
the container. For Docker-based development, add a local Compose override beside
`projects/navi/compose.yml`:

```yaml
services:
  navid:
    volumes:
      - ./plugins/navi-programmer:/root/.navi/plugins/navi-programmer:ro
```

Then start NaviD from `projects/navi` with:

```powershell
docker compose -f compose.yml -f compose.plugins.yml up -d --build
```

The mount target uses `/root/.navi/plugins` because the container runs NaviD as
root, and NAVI core's global plugin root is based on the process user home.

## Compose Validation Contract

The supported default `compose.yml` runtime now supports bounded validation
through `navi-programmer.run-validation` without a second source mount. The
contract is:

- the runtime image includes `docker-cli`
- `compose.yml` mounts `/var/run/docker.sock`
- `compose.yml` sets `NAVI_SANDBOX_CONTAINER_NAME=navid`
- `internal/sandbox.DockerRunner` uses `docker exec` against that running
  container when `NAVI_SANDBOX_CONTAINER_NAME` is set

This choice is specific to the default Compose runtime. It works because the
repository source is already present inside the running `navid` container, so a
bounded validation command can execute in-place without trying to bind-mount a
container-only filesystem path into a sibling container.

`compose.strict.yml` intentionally does not provide this contract. Strict mode
keeps its isolated runtime shape and may still return structured
`sandbox_blocked` evidence for validation commands until a stricter sandbox path
is designed explicitly.

## Restart Requirement

NAVI core loads plugin manifests during NaviD startup. After adding, removing,
or changing plugin metadata, restart NaviD.

Native local run:

```powershell
# Restart whichever navid process you are using.
```

Docker Compose:

```powershell
docker compose restart navid
```

## Verification

First verify the package manifest parses locally:

```powershell
Select-String -Path "C:\Users\evirg\codespace\NAVI-Ecosystem\projects\navi\plugins\navi-programmer\plugin.yaml" `
  -Pattern "^(id|name|version|trust_tier):"
```

Expected key values:

```text
id: navi.programmer
name: NAVI Programmer
version: 0.1.0
trust_tier: builtin
```

After NaviD is running, verify through the gateway:

```powershell
$plugins = (Invoke-RestMethod -Uri "http://localhost:6284/api/plugins").plugins
$plugins | Where-Object { $_.id -eq "navi.programmer" }
```

If loopback auth is not available in the active deployment, pass an API key or
the development shared secret:

```powershell
$headers = @{ "X-API-Key" = "navi-secret-stable" }
$plugins = (Invoke-RestMethod -Uri "http://localhost:6284/api/plugins" -Headers $headers).plugins
$plugins | Where-Object { $_.id -eq "navi.programmer" }
```

The result should include the `navi.programmer` manifest with the capabilities
declared in [../../plugin.yaml](../../plugin.yaml).

## Current Limitations

- Plugin manifest discovery is startup-time, not hot-reloaded.
- NAVI core currently discovers plugin manifests; full live skill invocation and
  workflow orchestration still belong to future NAVI core integration work.
- This package is source-of-truth. Avoid editing synced or copied plugin
  directories as if they were canonical.
- Docker development requires a bind mount or an image build step because host
  global plugin roots are not automatically available inside the container.

## Decision

When running NaviD from this NAVI checkout, use the built-in `plugins/` root.
Use a global or workspace-local junction only when testing the package from a
different checkout or packaged runtime. Use a Docker Compose bind mount when
running NaviD in Docker and the image does not already include the updated
plugin package.
