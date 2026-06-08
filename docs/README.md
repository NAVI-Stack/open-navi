# NAVI Documentation

**Status:** Active  
**Last Updated:** 2026-05-20

This directory now has two kinds of documents:

- implementation-facing docs that are expected to match the current codebase
- design, planning, and review docs that capture NAVI intent, history, or future direction

If you are trying to understand what NAVI does today, start with the implementation-facing set below.

## Start Here

| Document | Why it matters |
|----------|----------------|
| [../README.md](../README.md) | Repository overview, current runtime shape, quick start |
| [architecture/README.md](architecture/README.md) | Current implementation architecture and package map |
| [architecture/navi-system-architecture-guide.md](architecture/navi-system-architecture-guide.md) | Visual architecture guide covering NAVI pillars, runtime layers, governance, ICS, tools, world model, Experience Layer, and deployment boundaries |
| [specs/configuration.md](specs/configuration.md) | Runtime config layout and environment overrides |
| [specs/gateway-api.md](specs/gateway-api.md) | Live gateway route surface and auth model |
| [runbooks/run-navi.md](runbooks/run-navi.md) | Practical run and operator workflow |
| [tasks/blockers.md](tasks/blockers.md) | Current launch and hardening blockers |
| [tasks/readiness-assessment.md](tasks/readiness-assessment.md) | Current readiness snapshot |
| [design/navi-systems-map.md](design/navi-systems-map.md) | Living architecture inventory: what systems NAVI has, what is partial, what is missing, and what matters next. |

## Product Boundary

NAVI is maintained here as its own product and agent runtime. Helm-related framing, Helm Navigator history, and Helm-focused orchestration docs are legacy or external to this repository unless a document explicitly describes a current integration contract.

Use the **Experience Layer** as the technical term for NAVI's presentation and behavioral modulation system. Use **persona system** as the common product term for the same user-facing concept. Do not reintroduce preset legacy persona modes such as CEO Mode, Buddy Mode, or Jarvis Mode.

## Documentation Trust Levels

### 1. Current implementation

Use these when you need docs that should line up with the code today:

- [architecture/](architecture/README.md)
- [specs/](specs/INDEX.md)
- [connectors/](connectors/README.md)
- [runbooks/](runbooks/INDEX.md)
- [tasks/](tasks/INDEX.md)
- [FEATURES.md](FEATURES.md)

### 2. Product and canonical design

Use these for stable product principles and conceptual system shape:

- [VISION.md](VISION.md)
- [canonical/](canonical/INDEX.md)
- [adr/](adr/INDEX.md)
- [AGENTS.md](AGENTS.md)

### 3. Design, planning, and historical material

These are still useful, but many are intentionally ahead of or adjacent to the current implementation:

- [design/](design/INDEX.md)
- [concepts/](concepts/INDEX.md)
- [plans/](plans/INDEX.md)
- [reviews/](reviews/INDEX.md)
- [prompts/](prompts/INDEX.md)
- [_archive/](./_archive/INDEX.md)
- [_deprecated/](./_deprecated/INDEX.md)

## Organization Notes

- `architecture/` explains how the live packages fit together.
- `specs/` is for code-backed surfaces such as config and HTTP APIs.
- `runbooks/` is for operational tasks.
- `tasks/` tracks blockers, readiness, and outstanding work.
- `canonical/` captures higher-level system principles that change more slowly than the implementation.
- `agents/` now also carries the NAVI-native integration notes for imported agentic workflow patterns such as skills-first setup audits and verification loops.

Some older documents still describe the target architecture rather than the exact current runtime. When in doubt, treat the implementation-facing set above as authoritative first, then use the conceptual docs for intent and roadmap context.

See the full index at [INDEX.md](INDEX.md).
