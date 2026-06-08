**Status:** Active  
**Last Updated:** 2026-06-05
**Updated By:** Data-driven UI / artifact planning discussion

# Evolving Designs

In-progress designs, RFCs, and options under discussion. Documents here are typically **Evolving** and may intentionally run ahead of the code. Use [../architecture/README.md](../architecture/README.md) and [../specs/INDEX.md](../specs/INDEX.md) when you need implementation-facing behavior.

---

| Document | Purpose |
|----------|---------|
| [generated-ui-artifacts-and-live-artifacts.md](generated-ui-artifacts-and-live-artifacts.md) | Direction for generated UI, snapshot artifacts, live artifact archetypes, OpenUI-rendered displays, data-source honesty, and weather-display proving slice. |
| [navi-console-control-plane.md](navi-console-control-plane.md) | NAVI Console V2 product/UX direction: owner/admin interface, AI workspace control plane, Chat/Project/RuntimeSession model, shell layout, right inspector, phased redesign, and source-of-truth policy. |
| [navi-console-information-architecture.md](navi-console-information-architecture.md) | NAVI Console V2 information architecture: top-level navigation, route hierarchy, primary workflows, object priority, progressive disclosure, and phase mapping. |
| [navi-console-shell-and-layout.md](navi-console-shell-and-layout.md) | NAVI Console V2 shell/layout direction: left rail, main pane, collapsible right inspector, optional bottom drawer, layout states, responsive behavior, and route baselines. |
| [navi-console.md](navi-console.md) | Backend-hosted NAVI Console design: local agent-first control surface, runtime inspection pages, port 6284 default, security baseline, OpenClaw-inspired page taxonomy, and phased implementation plan. |
| [streaming-runtime.md](streaming-runtime.md) | Streaming UX & Message Runtime: RunCoordinator, inbox, event lifecycle, phased implementation. ([ADR-006](../adr/ADR-006-streaming-runtime.md)) |
| [delayed-and-multi-message.md](delayed-and-multi-message.md) | Delayed, scheduled, and multi-message sending: one run can emit multiple assistant messages with optional delays; store contract, scheduler, send_reply tool, limits. |
| [experience-layer.md](experience-layer.md) | Experience Layer boundary, contract, and mapping (persona + gateway). |
| [onboarding-ui-guide.md](onboarding-ui-guide.md) | Onboarding UI design and flows. |
| [plugin-connector-isolation.md](plugin-connector-isolation.md) | Plugin/connector isolation. |
| [plugin-lifecycle.md](plugin-lifecycle.md) | Plugin lifecycle. |
| [lifecycle-archive-tombstone.md](lifecycle-archive-tombstone.md) | Archive and tombstone flow and contract. |
| [world-model-write-boundary.md](world-model-write-boundary.md) | World Model write boundary. |
| [relationship-provenance.md](relationship-provenance.md) | Relationship provenance. |
| [subconscious-interruption.md](subconscious-interruption.md) | Subconscious escalation and urgent interruption. |
| [command-issuance.md](command-issuance.md) | Command issuance breadth and roadmap. |
| [governance-tier-resolution.md](governance-tier-resolution.md) | Governance tier order and conflict resolution. |
| [intake-synthesis-seam.md](intake-synthesis-seam.md) | Synthesis stage of the [Context Intake Pipeline](../specs/context-intake-pipeline-v1.md): extension of `internal/governor` via a sibling `MutationDescriptor`; disposition ladder as Risk rubric; Python resolution matching → Go governor outcome (game-engine framing); grouped Proposals for backfill; idempotency on replay; un-promoted chunks remain retrievable. |
| [ncos-capability-surface-contract.md](ncos-capability-surface-contract.md) | NCOS Phase 2 capability-surface contract. |
| [connector-taxonomy.md](connector-taxonomy.md) | Connector categories and mapping. |
| [connector-failure-and-isolation.md](connector-failure-and-isolation.md) | Connector failure model and isolation. |
| [navi-systems-map.md](navi-systems-map.md) | Living systems map and maturity matrix for NAVI. |
| [navi-programmer-plugin-architecture.md](navi-programmer-plugin-architecture.md) | Moved pointer for NAVI Programmer plugin architecture. |
| [navi-programmer-self-update-safety.md](navi-programmer-self-update-safety.md) | Moved pointer for NAVI Programmer self-update safety. |
| [session-compaction.md](session-compaction.md) | Session compaction architecture. |

---

[docs INDEX](../INDEX.md) - [canonical/specs/connectors](../canonical/specs/connectors.md) - [lifecycle-rules](../_meta/lifecycle-rules.md)
