# Skills System

**Status:** Active  
**Last Updated:** 2026-03-27  
**Primary package:** `internal/navi/skill`

This document describes how skills work in the current implementation.

## What A Skill Is

A skill is a capability definition stored on disk as either:

- `SKILL.md` for advisory or legacy context-only behavior
- `SKILL.yaml` for structured, executable OSS-27 style behavior

Skills are discovered at runtime and injected into NAVI without adding new Go package registrations.

## Two Skill Formats

### `SKILL.md`

Use this for lightweight instructional capabilities that are surfaced to the model as context. These are useful for advisory or conversational patterns, but they are not the structured execution path for governed side effects.

Current repo-owned examples live under plugin packages, such as `plugins/github/skills/github/SKILL.md`, `plugins/summarize/skills/summarize/SKILL.md`, `plugins/workspace-surface-audit/skills/workspace-surface-audit/SKILL.md`, and `plugins/verification-loop/skills/verification-loop/SKILL.md`.

### `SKILL.yaml`

Use this for executable capabilities. This format carries:

- stable skill identity
- interface definitions
- transport configuration
- effect and security metadata
- performance limits
- governance metadata

## Loading Priority

The loader scans two tiers, in priority order (`internal/navi/skill/loader.go:21-34`):

1. workspace-level skills — `<workspace>/skills/`
2. user-global skills — `~/.navi/skills/` (or `NAVI_SKILLS_DIR`)

Repo-owned plugin skills are **not** discovered by scanning the root `skills/` directory. They are loaded separately, only through paths declared by active `plugin.yaml` manifests, via `LoadPluginSkills` / `PluginSkillSource` (`internal/navi/skill/loader.go:204`). The root `skills/` directory holds skill-system support material (shared runtimes, docs), not a scanned built-in tier.

When the same skill ID exists in multiple tiers, the higher-priority tier wins.

## Execution In The Current Codebase

Executable skills are dispatched through `internal/navi/skill/executor.go`.

Supported transport types:

| Transport | Current behavior |
|-----------|------------------|
| `internal` | Calls a registered Go handler |
| `rest` | Performs HTTP requests using the declared config |
| `mcp_tool` | Calls an MCP bridge over HTTP |
| `subprocess_python` | Runs the packaged Python entrypoint through the Python runtime |
| `subprocess` | Launches a protocol-speaking worker process over JSON-RPC stdio |

This is one of the major areas where the docs had drifted before: the current runtime does execute non-file skill transports.

## Reloading Skills

Adding a skill folder is not enough by itself for an already-running daemon. In practice, you reload skills through the operator surface:

- gateway: `POST /api/skills/reload`
- CLI: `navi skills reload`

No Go recompilation or manual registration is needed.

## Internal Handlers

Several capabilities are wired through the `internal` transport today, including scheduler-style behaviors and self-diagnostic or skill-building flows.

That means a skill can be fully governed and tool-callable without leaving the process.

## Governance And Metadata

Structured skills carry metadata that the rest of NAVI can reason about:

- risk tier
- confirmation requirements
- idempotency and reversibility
- auth and network requirements
- publisher and trust metadata

The skill spec does not make policy decisions on its own, but it gives the runtime and governor enough structure to apply policy meaningfully.

## Python-Backed Skills

Skills that use `subprocess_python` add a `python_runtime` block describing environment and execution constraints.

See [skill-python-runtime.md](skill-python-runtime.md) for the transport-specific details.

Skills that use `subprocess` declare `runtime`, `command`, and `protocol` directly under the interface transport. This path is for Python, TypeScript/Node, shell, or external coding harness workers that speak a stable protocol while Go remains the control plane.

See [skill-subprocess-runtime.md](skill-subprocess-runtime.md) for the generic worker contract.

One current hybrid example is `document-knowledge`: the raw document parsing path runs in Python, while higher-level summarization and knowledge persistence use internal handlers so extracted facts can be written back into NAVI's memory store safely.

## Current Caveats

- `SKILL.md` remains advisory rather than fully executable
- executable skills still depend on their real external requirements, such as Python, API keys, or reachable bridge endpoints
- skill execution exists, but reliability hardening continues, especially around external environments and model behavior

## Related Docs

- [skill-python-runtime.md](skill-python-runtime.md)
- [skill-subprocess-runtime.md](skill-subprocess-runtime.md)
- [../FEATURES.md](../FEATURES.md)
- [../architecture/README.md](../architecture/README.md)
- [../canonical/skills.md](../canonical/skills.md)
