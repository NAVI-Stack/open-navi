**Status:** Active
**Last Updated:** 2026-06-04
**Updated By:** OpenUI / data-driven UI decision

# Architecture Decision Records

| ADR | Summary |
|-----|---------|
| [ADR-001](ADR-001-go-orchestration-python-ai.md) | Go for orchestration core, Python for AI layer. |
| [ADR-002](ADR-002-nats-jetstream-bus.md) | NATS JetStream as the event bus. |
| [ADR-003](ADR-003-stateless-orchestrator.md) | Orchestrator stateless at model boundary. |
| [ADR-004](ADR-004-skill-format.md) | SKILL.md vs. SKILL.yaml (OSS-27) - dual format policy. |
| [ADR-005](ADR-005-sqlite-single-writer.md) | SQLite WAL mode with single-writer connection pool. |
| [ADR-006](ADR-006-streaming-runtime.md) | Streaming UX & Message Runtime - inbox-driven, event-native, RunCoordinator as primary loop. |
| [ADR-007](ADR-007-dual-plane-llm.md) | LLM Dual-Plane Architecture - Inference vs. Control planes. |
| [ADR-008](ADR-008-ncos-capability-surface-policy.md) | NCOS Phase 2 capability-surface policy - resolver, fail-closed rules, routing interaction, and path parity. |
| [ADR-009](ADR-009-session-compaction-runtime.md) | Chat Compaction Runtime - structured continuity state, checkpoints, rehydration, and migration away from rolling-summary-first chat handling. |
| [ADR-010](ADR-010-navi-console-gateway-hosting.md) | NAVI Console Gateway Hosting - serve Console from the existing gateway on `:6284`, reuse `gateway.static_dir`, consume existing routes first, and keep public exposure out of V1. |
| [ADR-011](ADR-011-memory-v2.md) | Memory v2 and Context Window Governance — entity-based World Model, Zettelkasten note graph, three-tier reflection pipeline, and MemoryGovernor context injection limits. Tracks NAVI-AUTO-010. |
| [ADR-012](ADR-012-context-intake-and-memory-projection.md) | Personal Continuity Layer — Context Intake Pipeline as the named acquisition seam; synthesis extends `internal/governor` via sibling `MutationDescriptor` (no parallel authority); Python resolution matching → Go governor outcome; Memory Vault as the second mouth on the pipeline with structured-diff round-trip; entities-only projection; explicit Local/Hybrid/Cloud privacy modes. |
| [ADR-013](ADR-013-openui-data-driven-rendering.md) | OpenUI as NAVI's first-class React/web data-driven rendering lane, while NAVI retains authority over meaning, component semantics, skills, governance, artifacts, and action execution. |

---

[docs INDEX](../INDEX.md) · [architecture](../architecture/README.md)
