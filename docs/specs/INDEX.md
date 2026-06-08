**Status:** Active  
**Last Updated:** 2026-06-05
**Updated By:** Data-driven UI / artifact alignment pass

# Feature & API Specs (Mutable)

Implementation-level feature and API specifications. For foundational system specs (canonical), see [canonical/specs/](../canonical/specs/).

---

| Document | Purpose |
|----------|---------|
| [navi-console-v2.md](navi-console-v2.md) | NAVI Console V2 implementation contract: Chat-first console UX, project workbench, runtime/control inspector, route/API expectations, terminology migration, and phased acceptance criteria. |
| [navi-console-v1.md](navi-console-v1.md) | NAVI Console V1 implementation contract: existing gateway hosting on `:6284`, static frontend contract, page/API mapping, `/ws/live` usage, backend limits, acceptance criteria, and implementation order. |
| [capability-expansion-loop.md](capability-expansion-loop.md) | Capability Expansion Loop: states, APIs, commands, data shapes, implementation status. |
| [gateway-api.md](gateway-api.md) | Gateway HTTP route list and auth (source: `internal/gateway/server.go`). |
| [configuration.md](configuration.md) | Config layout, env vars, connector section. |
| [streaming-messaging-architecture.md](streaming-messaging-architecture.md) | Streaming and messaging runtime: canonical event envelope, concurrency/queue semantics, and streaming UX state machine (Tasks 1.1–1.3). |
| [persona-system.md](persona-system.md) | Persona system specification (ratified): Experience Layer subsystem, 18-trait v1 system, merge engine, compiler/serializer, adaptation model. |
| [workspace-v1.md](workspace-v1.md) | Workspace V1 spec (Draft 2): execution/context boundaries, operating modes, entity model, scope enforcement, boundary crossing, protected paths, audit. |
| [artifact-system-v1.md](artifact-system-v1.md) | Artifact System V1 spec (v2.0): entity model, versioning, branching, Library, renderers, agentic orchestration, governance, provenance, export. |
| [artifact-generated-ui-and-live-artifact-alignment.md](artifact-generated-ui-and-live-artifact-alignment.md) | Alignment addendum for generated UI artifacts, OpenUI renderer boundaries, snapshot/live artifact archetypes, data-source honesty, promotion flow, and weather-display proving slice. |
| [project-system-v1.md](project-system-v1.md) | Project System V1 spec: canonical work container, work boundary, entity model, work state entities (objectives, decisions, risks), and project lifecycle. |
| [inference-control-system-v1.md](inference-control-system-v1.md) | Inference Control System v1 foundational spec; pair with [ICS architecture corpus](../architecture/inference-control-system/INDEX.md) for normative contract and compliance docs. |
| [pet-presence-interface-v1.md](pet-presence-interface-v1.md) | Canonical PET/NAVI presence protocol: authority model, envelopes, REST/WS contract, revision rules, visibility, and normalization surfaces. |
| [presence-runtime-framework-v1.md](presence-runtime-framework-v1.md) | NAVI-side presence subsystem framework: PresenceService, composition inputs, gateway surfaces, revision/event behavior, and dreaming-ready runtime structure. |
| [presence-status-decision-policy-v1.md](presence-status-decision-policy-v1.md) | Deterministic NAVI status-selection policy: precedence rules, working vs busy semantics, attention overrides, dreaming rules, and work-classification requirements. |
| [session-compaction-system-v1.md](session-compaction-system-v1.md) | V1 implementation contract for chat compaction: state objects, storage model, selection rules, rehydration order, invariants, and acceptance criteria. |
| [telegram-upgrades.md](telegram-upgrades.md) | Telegram connector upgrade inventory synthesized from the openclaw reference: gap catalog (group gating, reply/forward context, media ingestion, command menu, topics, multi-account, approvals), tiered and phased. |
| [context-intake-pipeline-v1.md](context-intake-pipeline-v1.md) | Context Intake Pipeline (CIP) v1: first-class, governed loop turning external source data into provenance-tagged World Model state. Stage model (sync→admit→canonicalize→chunk→distill→score→embed/extract→synthesize→fold→retrieve), intake envelope, per-connector sync policies, Content Distillation primitive, Local/Hybrid/Cloud privacy modes, Cognitive write seam + Proposal gating, phased rollout and acceptance criteria. Pairs with planned `memory-projection-v1.md` (Vault). |
| [memory-projection-v1.md](memory-projection-v1.md) | Memory Vault projection: entities-only Markdown projection of the World Model (Contacts, Knowledge, Memories, Artifacts in V1), structured-diff round-trip via the [synthesis seam](../design/intake-synthesis-seam.md), generic Markdown + YAML frontmatter (Obsidian / Nextcloud / plain-editor compatible), file-watcher + debounce on owner edits, idempotent reprojection on entity write, conflict handling between owner edits and external deltas. The "second mouth" on the intake pipeline. |
| [multimodal-pipeline-v1.md](multimodal-pipeline-v1.md) | Multimodal (image/file) message pipeline design: typed `llm.Message` content blocks + per-provider encoding/fallback, attachment plumbing (gateway → MessageInput → InboxItem → runtime), blob storage + prompt assembly, phased rollout. Unblocks Telegram T1-5. |

---

[docs INDEX](../INDEX.md) · [canonical specs](../canonical/specs/INDEX.md)
