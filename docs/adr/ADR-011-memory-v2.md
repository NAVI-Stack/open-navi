# ADR-011 — Memory v2 and Context Window Governance

> Formal decision record for upgrading NAVI AI's memory system from flat SQLite facts to an entity-based World Model with Zettelkasten-inspired graph storage, three-tier reflection, and governed context injection.

**Status:** Accepted
**Date:** 2026-05-25
**Deciders:** Eric (Owner)
**Tracks:** NAVI-AUTO-010 · NAVI-MC-001
**See also:** [ADR-005 SQLite Single Writer](ADR-005-sqlite-single-writer.md) · [ADR-009 Chat Compaction Runtime](ADR-009-session-compaction-runtime.md) · [Memory Concept Doc](../concepts/memory.md) · [Conceptual Design Overview](../canonical/conceptual-design-overview.md)

---

## Context

The current memory implementation is a structured SQLite facts store. It covers the most critical operational needs: session-scoped facts, owner-scoped preferences, reflection-based extraction from chat turns, and shallow semantic recall via embedding vectors stored in the `facts` and `memories` tables.

This system has proven adequate for Phase 14–15 work, but it carries architectural debt that blocks the full autonomy roadmap:

**Structural limitations of the current model:**

1. **Flat facts, no entity model.** Memory is stored as key-value-like rows. There is no first-class concept of a Contact, Event, Knowledge node, Memory, or Artifact. Entity types are implicit in the `type` and `scope` columns, not enforced as distinct schema.

2. **No relationship layer.** Facts exist in isolation. There is no graph of how entities relate to each other, no confidence scores on those relationships, no recency decay, and no derivation chains that record how one fact was inferred from others.

3. **No note-graph architecture.** Memory nodes are not linked. NAVI cannot traverse from a task outcome to the user preference that motivated it to the prior conversation that established that preference. This limits higher-order reasoning and consolidation.

4. **Unstructured context injection.** At each loop turn, the `ContextBlockForSession()` and `AssembleUserModel()` functions inject memory into the system prompt without budget awareness. Under high-memory conditions, the context window is filled indiscriminately, crowding out task context.

5. **Reflection is shallow.** The current `emitReflect` pipeline extracts regex-detected preferences and writes shallow facts. There is no periodic consolidation pass that strengthens, weakens, or archives nodes based on accumulated evidence.

These gaps are tolerable while NAVI AI is at Phase 15 autonomy levels, but they become blockers as the system moves toward multi-session continuity, cross-project knowledge, and deeper user modeling (NAVI-MC-001).

**Relationship to ADR-009:** ADR-009 (Chat Compaction) addresses *online continuity* within an active chat session — how to keep the active context window correct under token pressure. This ADR addresses *durable, cross-session memory* — the long-term knowledge graph that persists between sessions and grows over weeks. These are orthogonal concerns with a defined seam: the compaction subsystem feeds structured state into the World Model via the reflection pipeline; it does not own durable memory.

---

## Decisions

### D1 — Entity-based World Model replaces flat facts as the target architecture

The target memory architecture is the **World Model** defined in the Conceptual Design Overview. This replaces the current flat-facts model as the north-star design.

The World Model has seven first-class entity classes: Contacts, Events, History, Knowledge, Memories, Artifacts, and Proposals. Each entity has:

- A stable UUID (`entity_id`)
- A typed attribute set that grows incrementally
- Relationships to other entities (typed, confidence-scored, recency-tracked)
- Full provenance metadata: source, derivation chain, reinforcement count, mutation history

**What does NOT change:** The SQLite WAL backend (ADR-005) remains unchanged. The World Model is stored in SQLite — the upgrade is schema depth and query patterns, not the storage engine.

### D2 — Zettelkasten-inspired note graph for the Knowledge entity class

The Knowledge entity class is the primary candidate for graph-traversal memory. Each Knowledge node is a "note" in the Zettelkasten sense:

- Self-contained: carries its own title, content, provenance, and confidence
- Linked: holds references to other Knowledge nodes via the relationship layer
- Composable: can be surfaced individually or traversed as a neighborhood

This model is inspired by A-Mem v2 patterns (see research in `docs/research/`) and is suited to NAVI's use case because most valuable knowledge is compositional — understanding why a user prefers a pattern requires linking the preference node to the project context node to the past decision node.

**Note graph implementation target:** `internal/store/knowledge/` — separate from the current `internal/store/` fact tables. Existing fact rows are migrated into Knowledge nodes during Phase 1 of Memory v2.

### D3 — Three-tier reflection pipeline governs memory evolution

Memory evolves through a structured three-tier reflection pipeline, not ad-hoc writes:

| Tier | Cadence | Trigger | Writes Permitted | Notes |
|------|---------|---------|-----------------|-------|
| **Shallow Reflection** | Post-interaction | `emitReflect()` after each loop turn | Create/update entity attributes; create relationships | May not delete or reclassify entities |
| **Consolidation** | Daily (Heartbeat-triggered) | `consolidation_job.go` | Strengthen/weaken relationship confidence; deprecate Knowledge nodes; update purely-inferred Configuration | Runs offline; results are proposals unless auto-approvable |
| **Deep Reflection** | Weekly or event-triggered | `deep_reflection_job.go` | Reclassify/archive entities; modify inferred Configuration and Priorities; propose changes to Owner-set state | Owner confirmation required before any Owner-set state mutation |

The current `emitReflect` shallow path is the implementation seed for Tier 1. The consolidation and deep reflection jobs are Phase 2 targets.

### D4 — Semantic retrieval is the primary recall path; keyword lookup is fallback

The current `RecallKnowledge()` function supports vector similarity search. This becomes the **primary recall path** in Memory v2.

Retrieval at query time:
1. Embed the current turn context (using `navi.llm.embed` skill or local fallback)
2. Vector similarity search against Knowledge nodes (`cosine_similarity > threshold`)
3. Graph traversal: expand to 1-hop neighbors of top-k results
4. Rank expanded set by (similarity × confidence × recency weight)
5. Trim to context budget (see D5)

Keyword/tag search remains available as a fallback for cases where embeddings are unavailable or cold (new knowledge with no embedding yet).

### D5 — Context Window Governance: hard governor cap on memory injection

Memory injection into the system prompt is governed by a **MemoryGovernor** with hard limits the agent loop cannot bypass:

| Constraint | Default | Notes |
|---|---|---|
| `memory_budget_tokens` | 2,048 | Hard cap on total tokens injected from memory per turn |
| `max_knowledge_nodes` | 10 | Maximum distinct Knowledge nodes injected per turn |
| `max_relationship_hops` | 1 | Maximum graph traversal depth for context expansion |
| `shallow_facts_reserved` | 512 | Tokens reserved for user preferences and session-scoped facts (always injected if non-empty) |

When recall returns more than `max_knowledge_nodes` candidates, the set is trimmed by score (similarity × confidence × recency). The `shallow_facts_reserved` block is always injected ahead of Knowledge nodes to guarantee user preferences are always present.

These defaults are configurable in `config/runtime.yaml` under `memory.governor.*`. They are not agent-adjustable — only the human owner may change them.

### D6 — Four lifecycle actions for entity archival

The World Model defines four lifecycle actions, matching the conceptual design:

| Action | Meaning | Human Confirmation Required? |
|---|---|---|
| **Archive** | Soft-delete; preserved for audit and recovery. Default for all Delete commands. | No (can be system-initiated) |
| **Forget** | Remove significance from a Memory entity. The entity is archived but its emotional/significance weight is zeroed. | Yes — always |
| **Supersede** | Replace a Knowledge node with a newer version. Derivation chain from old node is preserved. | No (system can supersede inferred Knowledge) |
| **Tombstone** | Hard-delete with audit trail. Reserved for PII removal or explicit owner request. | Yes — always |

### D7 — Migration is incremental; existing fact tables are not removed in Phase 1

Phase 1 of Memory v2 focuses on:
1. Introducing the new entity schema alongside the existing tables
2. Writing new facts through entity paths while preserving existing recall paths
3. Migrating existing flat facts to Knowledge nodes in a background job

Existing `facts`, `memories`, and `memory_links` tables are kept read-accessible during migration. They are deprecated (not removed) when the Knowledge entity layer covers their full surface.

---

## Consequences

### 1 — New package and schema required

A new `internal/store/knowledge/` package is required with:
- Entity tables: `entities`, `entity_attributes`, `entity_relationships`
- Knowledge-specific: `knowledge_nodes`, `knowledge_links`
- Provenance: `entity_provenance`, `derivation_chains`
- Embedding index: extending the existing `embedding` column pattern to all entity types

This requires SQLite migrations. Migration safety must follow ADR-005 rules: WAL mode, single-writer connection pool, migration applied through the existing `store/migrations/` runner.

### 2 — MemoryGovernor is a new Governor type

The MemoryGovernor (D5) requires a new governor implementation in `internal/governor/`. It must be wired into the agent loop before prompt assembly, not after, so budget enforcement is deterministic.

### 3 — Reflection pipeline becomes a background runner

The Consolidation and Deep Reflection tiers (D3) require Heartbeat-triggered background runners, not inline loop execution. These will be registered as Heartbeat jobs, similar to the existing cron scheduler.

### 4 — Context assembly function signatures change

`ContextBlockForSession()` and `AssembleUserModel()` in `internal/navi/store/` gain a budget parameter. Call sites in `internal/navi/loop.go` must be updated to pass the MemoryGovernor's budget.

### 5 — Prerequisite for NAVI-MC-001

The Memory v2 entity model is the prerequisite for NAVI-MC-001 (multi-session knowledge continuity, user model projection). NAVI-MC-001 must not begin until Phase 1 migration is complete and the Knowledge entity layer is stable.

---

## Rejected Alternatives

### A — Extend flat facts with more indexes and columns

**Rejected** because it deepens the structural debt without resolving the relationship-layer gap. Flat facts with richer indexes still cannot represent "this preference was reinforced by these three conversations" without encoding structure into string values.

### B — Vector-only retrieval without note graph

**Rejected** because vector similarity alone loses structured relationships. Two facts with similar embeddings are not necessarily causally related — graph edges encode the *why* of relationships, not just their topical similarity.

### C — External vector database (Qdrant, Weaviate, Chroma)

**Rejected** for Phase 1. Adding an external dependency for memory contradicts the Zero Framework Cognition principle and the design goal of a self-contained, single-binary NAVI instance. SQLite with embedding columns is sufficient for Phase 1. A pluggable vector backend can be added as a Phase 2 capability once the entity model is stable.

### D — Merge Memory v2 with Chat Compaction (ADR-009) into a unified "context system"

**Rejected** because the concerns are genuinely separate. Compaction manages online token-window continuity for an active session. Memory v2 manages the durable cross-session knowledge graph. Merging them creates a monolith that cannot be tested, evolved, or failed gracefully in isolation.

---

## Status History

| Date | Change |
|---|---|
| 2026-05-25 | Accepted — entity-based World Model, Zettelkasten note graph, three-tier reflection, and MemoryGovernor context governance adopted as the Memory v2 target architecture |

---

*See also: [Memory Concept Doc](../concepts/memory.md) · [ADR-009 Chat Compaction](ADR-009-session-compaction-runtime.md) · [ADR-005 SQLite Single Writer](ADR-005-sqlite-single-writer.md)*  
*Last updated: 2026-05-25*
