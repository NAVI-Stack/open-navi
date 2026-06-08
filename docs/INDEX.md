# Documentation Index

**Status:** Active
**Last Updated:** 2026-06-05
**Updated By:** NAVI Coder canonical update

Single entry point for the documentation corpus.

## Fast Path

| Document | Purpose |
|----------|---------|
| [README.md](README.md) | Documentation overview and trust levels |
| [../README.md](../README.md) | Repository overview and quick start |
| [canonical/navi-coder.md](canonical/navi-coder.md) | Canonical definition of NAVI Coder as a first-class programming product domain |
| [architecture/navi-coder-product-architecture.md](architecture/navi-coder-product-architecture.md) | Product, UI, backend, runtime, and worker architecture for NAVI Coder |
| [plans/navi-coder-migration.plan.md](plans/navi-coder-migration.plan.md) | Migration plan from the legacy Programmer prototype into NAVI Coder |
| [architecture/README.md](architecture/README.md) | Current implementation architecture |
| [architecture/navi-system-architecture-guide.md](architecture/navi-system-architecture-guide.md) | Visual architecture guide for pillars, layers, governance, ICS, tools, world model, Experience Layer, first-class React/web data-driven rendering, and deployment boundaries |
| [canonical/data-driven-ui-rendering.md](canonical/data-driven-ui-rendering.md) | Canonical principle: NAVI owns meaning; OpenUI renders first-class React/web data-driven meaning |
| [architecture/data-driven-ui-rendering.md](architecture/data-driven-ui-rendering.md) | Data-driven UI rendering architecture for render intent, NaviDataView, chat payloads, OpenUI rendering, governed toolProvider, and fallback behavior |
| [architecture/navi-ui-library.md](architecture/navi-ui-library.md) | `@navi/ui` architecture, generative UI runtime, native NAVI UI spec, and OpenUI adapter strategy |
| [adr/ADR-013-openui-data-driven-rendering.md](adr/ADR-013-openui-data-driven-rendering.md) | Accepted decision: OpenUI is NAVI's first-class React/web data-driven rendering lane while NAVI retains authority over meaning, governance, artifacts, and action execution |
| [architecture/inference-control-system/INDEX.md](architecture/inference-control-system/INDEX.md) | Inference Control System corpus index (spec + normative contracts + audit docs) |
| [specs/configuration.md](specs/configuration.md) | Runtime configuration and env overrides |
| [specs/gateway-api.md](specs/gateway-api.md) | Live HTTP and WebSocket API surface |
| [runbooks/run-navi.md](runbooks/run-navi.md) | Run and operate NAVI locally |
| [tasks/blockers.md](tasks/blockers.md) | Current blockers and hardening items |
| [tasks/chat-artifact-context-and-materialization.md](tasks/chat-artifact-context-and-materialization.md) | Chat context and artifact materialization task note for explicit promotion and auto-created durable outputs |
| [tasks/readiness-assessment.md](tasks/readiness-assessment.md) | Current readiness snapshot |
| [agents/everything-claude-code-integration.md](agents/everything-claude-code-integration.md) | NAVI-native mapping of imported ECC workflow patterns |

## Sections

| Section | Purpose |
|---------|---------|
| [canonical/](canonical/INDEX.md) | Stable principles, canonical specs, and foundational documents |
| [architecture/](architecture/README.md) | Implementation architecture and package relationships |
| [adr/](adr/INDEX.md) | Architecture decisions and rationale |
| [specs/](specs/INDEX.md) | Code-backed feature and API specs |
| [runbooks/](runbooks/INDEX.md) | Operator procedures |
| [tasks/](tasks/INDEX.md) | Blockers, readiness, and backlog summaries |
| [concepts/](concepts/INDEX.md) | Product and system concepts, some ahead of implementation |
| [design/](design/INDEX.md) | Draft designs and evolving proposals |
| [reviews/](reviews/INDEX.md) | Review artifacts and audits |
| [plans/](plans/INDEX.md) | Project plans and phased work |
| [identity/](identity/INDEX.md) | Identity and keystore docs |
| [prompts/](prompts/INDEX.md) | Prompt-related references |
| [agents/](agents/INDEX.md) | Agent workflow and documentation SOPs |
| [_meta/](./_meta/lifecycle-rules.md) | Documentation process, audit log, and meta rules |
| [_archive/](./_archive/INDEX.md) | Archived docs |
| [_deprecated/](./_deprecated/INDEX.md) | Deprecated docs |

## Usage Guidance

- Prefer `architecture/`, `specs/`, `runbooks/`, and `tasks/` when you need docs that should match the code now.
- Prefer `canonical/`, `VISION.md`, and the ADRs when you need the product or architectural intent.
- Treat `design/`, `plans`, and many `concepts/` docs as directional unless they explicitly cite implementation files.
- Use `agents/everything-claude-code-integration.md` when you need the skills-first, verification, or workspace-audit patterns that were localized into NAVI.
- Treat new references to “NAVI Programmer” as stale unless they are explicitly marked as legacy or migration context; the canonical product name is NAVI Coder.

[docs README](README.md) | [root README](../README.md)
