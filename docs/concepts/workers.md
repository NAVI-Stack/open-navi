# Capability Layer And Workers

**Status:** Active  
**Last Updated:** 2026-03-27  
**See also:** [../architecture/README.md](../architecture/README.md), [skills.md](skills.md), [../canonical/connectors.md](../canonical/connectors.md)

This document explains the current Capability Layer shape in the codebase and how it relates to the broader command model.

## What The Capability Layer Means In Practice

Conceptually, the Capability Layer is where NAVI turns decisions into action. In the current implementation, that happens through a mix of:

- runtime tools exposed to the foreground NAVI loop
- skills loaded from disk and executed through declared transports
- connectors that cross system boundaries
- worker packages that execute directive-assigned tasks

The Cognitive Layer decides what should happen. The Capability Layer is where that work actually gets executed.

## The 10 Primitive Commands

NAVI still frames action in terms of 10 command types:

| Category | Commands |
|----------|----------|
| State | `Query`, `Create`, `Update`, `Delete` |
| Effect | `Invoke`, `Send`, `Acquire`, `Schedule` |
| Coordination | `Delegate`, `Compose` |

The codebase does not hard-map every one of these to a single package, but they are still a useful way to reason about behavior:

- runtime chat turns are heavy on `Compose`, `Invoke`, `Query`, and `Send`
- directive execution is heavy on `Delegate`
- world-model and artifact flows cover `Create`, `Update`, and `Delete`
- scheduling flows cover `Schedule`

## Current Worker Packages

The older generic `internal/worker` package is gone. The current execution model uses dedicated worker packages subscribed to task assignments from the orchestrator.

| Worker | Package | Current role |
|--------|---------|--------------|
| CoderAgent | `internal/coder` | File-oriented implementation work |
| CriticAgent | `internal/critic` | Reviews and critique on task surfaces |
| StrategistAgent | `internal/strategist` | Planning and design-note generation |
| ScoutAgent | `internal/scout` | Research and tool-assisted research notes (legacy, Helm-derived; wired but not actively developed) |

The daemon wires these subscriptions directly in `cmd/navid/main.go`.

> **Note:** ScoutAgent is carried over from the legacy Helm codebase. It is left in
> place and may be adapted later, but it is not part of the current development direction.

## How Worker Execution Flows

1. A directive enters `ACT` mode.
2. `internal/orchestrator` reconstructs directive state and decomposes it into tasks.
3. Task assignment events are published on the bus.
4. Each worker package ignores tasks not addressed to its agent type.
5. The matching worker executes the task, updates task state, and records outcomes back through the persistence layer.

Workers are not free-floating managers. They are specialized executors attached to explicit task assignments.

## Skills In The Capability Layer

Skills complement workers rather than replacing them.

- the foreground NAVI runtime can call skills directly during a session
- scout can use skill tool-calling for research workflows
- skill execution is transport-backed today, not just descriptive

Current transport types:

- `internal`
- `rest`
- `mcp_tool`
- `subprocess_python`
- `subprocess`

See [skills.md](skills.md) for details.

## Connectors In The Capability Layer

Connectors handle outbound and inbound interaction with external systems.

Current built-in connector surfaces:

| Connector | Package | Status |
|-----------|---------|--------|
| Telegram | `plugins/telegram/connectors/telegram` | Live |
| Slack | `plugins/slack/connectors/slack` | Live |
| Bridge/runtime connectors | `internal/connectors` | Live |
| Workspace subprocess connectors | `internal/connectors`, `workspace/connectors/*` | Live |

Connectors are managed through the registry and manager in `internal/connectors`, then exposed through the gateway and daemon boot sequence.

## Plugins

Plugins are still lighter-weight in implementation than the higher-level conceptual model. Today, the main concrete plugin surface is manifest discovery and registry exposure rather than a fully separate lifecycle with install and isolation stages.

That means:

- plugin manifests exist
- plugin discovery is exposed through the gateway
- plugin behavior is still closely tied to the rest of the runtime rather than a fully isolated platform model

## Historical Note

Earlier documentation referenced a generic worker fleet and supervisor package. That was accurate for an earlier architecture, but the live code now uses:

- dedicated worker packages
- direct daemon wiring
- explicit runtime and orchestrator coordination

Use [../architecture/README.md](../architecture/README.md) for the current package-level picture.
