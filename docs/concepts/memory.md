# NAVI AI — Memory and World Model

> **How NAVI remembers: the entity-based World Model, current SQLite implementation, and the path to the full conceptual design.**

**Status:** Active
**Last Updated:** 2026-03-21
**Source of Truth:** [Conceptual Design Overview](../canonical/conceptual-design-overview.md) (Entity Classes, Relationship Layer, Provenance Model)
**Package:** `internal/store/` · `internal/navi/store/`
**See also:** [Orchestrator Loop](./orchestration-loop.md) · [Architecture Overview](../architecture/README.md)

---

## Why Memory Matters

Most AI tools are stateless: each conversation starts from zero. NAVI AI is designed to remember — not just within a session, but across sessions, across weeks, across projects. The memory system is what transforms a capable AI tool into a *personal* agent.

---

## Conceptual Design: The World Model

The [Conceptual Design Overview](../canonical/conceptual-design-overview.md) defines memory as part of the **World Model** — the single source of truth for all entity state, relationships, and structured knowledge. The World Model sits between the Cognitive Layer and the Capability Layer.

**Only the Cognitive Layer may write to the World Model.** Neither the Experience Layer nor the Capability Layer modifies it directly — changes flow through Cognitive processes.

### Entity Classes

All entities start minimal and grow over time. This is the canonical entity model from the conceptual design:

#### World Entities

| Entity | Description |
| :---- | :---- |
| **Contacts** | NAVI's contact graph of people and entities. Starts with the owner. Default attribute is a name. |
| **Events** | Scheduling and temporal awareness. Drives calendar-like applications. |
| **History** | Append-oriented interaction log that enriches over time. Records both interaction events and execution outcome records. Every command attempt produces a History entry regardless of outcome — failure is a first-class recorded state. |
| **Knowledge** | Curated facts plus emergent understanding. Derived from History and external sources. Always attributed — every node carries provenance (source, confidence, derivation chain). Mutable: can be revised, merged, or deprecated. |
| **Memories** | Experiences and moments that carry significance and emotional weight. Subjective and interpretive. Durable by default but not permanent — a deletion path (Forget) exists. |
| **Artifacts** | Files, documents, notes — anything tangible NAVI creates or manages. |
| **Proposals** | First-class entities representing actions that require user authorization before execution. See Proposal Queue in Tier 3 of the conceptual design. |

#### Cognitive Entities

| Entity | Description |
| :---- | :---- |
| **User Model** | A composite projection, not a standalone entity. Assembled at query time from Contacts (owner record), Knowledge (facts about user), Memories (shared experiences), Configuration (preferences), and Priorities (goals). Never stored as a single object — always derived. |

#### Capability Entities

| Entity | Description |
| :---- | :---- |
| **Skills** | Modular, expandable capability registry. Each Skill is versioned, carries its own permission profile, and declares its input/output contract. |

#### Control Entities

The system distinguishes four kinds of state: **Explicit** (directly set by user), **Owner-set** (a subset of Explicit, specifically authored by the owner), **Inferred** (learned from behavior), and **Derived** (computed at query time, never stored).

| Entity | Description |
| :---- | :---- |
| **Configuration** | Explicit and inferred preferences and settings. Explicit is set by the user. Inferred is learned and may be updated by reflection within permitted scope. |
| **Priorities** | Stated and observed priorities defining goals and intentions. Stated are Owner-set. Observed are inferred from behavior. |

### Relationship Layer

Relationships between entities carry structural metadata:

| Field | Purpose |
| :---- | :---- |
| **Type** | Relationship category (knows, owns, occurred-at, prefers, related-to, derived-from, etc.). Extensible. |
| **Confidence** | How strongly NAVI believes the relationship is valid. Updated by reinforcement or contradiction. |
| **Recency** | When the relationship was last reinforced. Drives natural decay. |
| **Provenance** | Which event, source, or reflection tier produced the relationship. |

### Provenance Model

Every entity and relationship tracks lifecycle metadata (defined in Tier 3 of the conceptual design):

| Field | Purpose |
| :---- | :---- |
| **source** | What produced this entity: `user_input`, `sensor`, `inference`, `shallow_reflection`, `consolidation`, `deep_reflection`, `plugin`, `proposal_approved`, `execution_succeeded`, `execution_failed`, etc. |
| **timestamp** | Creation and last-modified times. |
| **confidence** | Numeric score for accuracy belief. |
| **derivation_chain** | Ordered list of upstream entity/event IDs. Enables full audit. |
| **reinforcement_count** | Independent confirmation count. Strengthens confidence without duplicating sources. |
| **mutation_history** | Append-only log of changes: what changed, when, by which process, and why. |

---

## Current Implementation: SQLite Facts Store

The current memory system is a structured SQLite store. While the full entity model from the conceptual design is the target, the current implementation covers a subset:

| Conceptual Entity | Current Implementation |
|---|---|
| History | `events` table (append-only event log with correlation IDs, causal parent, sequence number) |
| Directives + Messages | `directives` and `directive_messages` tables |
| Tasks | `tasks` table with status lifecycle |
| Sessions | `sessions` and `session_messages` tables in `internal/navi/store/` |
| Configuration | `settings` table (key-value with `updated_at`) |
| Skills | `SkillRegistry` (in-memory, loaded from filesystem at startup) |
| Contacts (partial) | `owners` table (single-owner model) |
| Agents | `agents` table (registration + heartbeat tracking) |

### How Memory Is Used Today

At the start of each agent loop turn, relevant owner and session facts are loaded and included in the system prompt context via `LoopConfig.FactsBlock`. The LLM receives this as part of its context window.

Knowledge storage is now embedding-aware as well:

- Facts and memories persist `keywords`, `tags`, and `embedding` vectors in SQLite.
- NAVI prefers the builtin `navi.llm.embed` skill for embeddings when it is available and configured, with the older local heuristic embedding kept as a fallback so recall still works offline or during degraded startup.
- Semantic recall is active in the World Model via `RecallKnowledge(...)`, and prompt assembly uses it to inject a `Relevant knowledge` section based on the current session's recent transcript rather than exact key matching alone.
- Related facts and memories are auto-linked into `memory_links` when their vectors are sufficiently similar, so semantic neighborhoods can be inspected later through the knowledge API.

Conversation turns now feed that store through the reflection pipeline instead of relying only on explicit writes. `emitReflect` publishes a structured JSON envelope that can include:

- The turn summary and free-form details.
- The last user message and final assistant reply.
- A `facts` array of inferred facts extracted from the conversation.

In shallow reflection, those facts are promoted into the World Model with `source=shallow_reflection`, so stable preferences and technical context can become available on later turns through `ContextBlockForSession()`.

Manual memory now has a dedicated path as well:

- Natural prompts like `remember this`, `remember that`, `don't forget`, and `keep this in mind` are treated as explicit memory signals.
- Those turns are stored with owner scope when an owner ID is available, so the memory survives across sessions.
- Session-scoped facts and memories are now appended into the active session prompt block, so short-lived session context is still visible even when it should not become owner memory.

Task execution now feeds the same memory surface:

- Worker completion writes a directive-scoped `task_outcome` fact so the orchestrator can see what succeeded or failed on the next tick.
- Coder task completion also appends a directive message with the implementation summary, so the directive thread and the durable facts stay aligned.
- Orchestrated ACT work now emits structured `FactReflectionQueued` events as well: after decomposition, after coder task execution, and after directive completion. Those payloads use the same shallow-reflection JSON envelope as conversation turns, so orchestrator work can promote facts through the same reflection worker.
- When an ACT directive reaches only terminal task states, the orchestrator writes an owner-scoped learning summary (`post_directive_reflection`) before marking the directive complete or stalled.

Session history now has an initial rolling summarization path as well:

- Every 20 persisted session messages, NAVI runs a lightweight LLM summarizer over the next unsummarized batch instead of waiting for the whole conversation to disappear behind the 50-message session window.
- That summary is stored twice: as a session-scoped memory checkpoint for the active conversation and, when the owner is known, as an owner-scoped memory so future sessions can inherit the topic summary.
- Structured `facts_learned` and `key_decisions` from the summary are promoted into owner-scoped facts, so cross-session recall can happen through the normal `AssembleUserModel()` and `ContextBlockForSession()` paths.
- When NAVI switches to a newly created session, it now forces one final summary pass over the previously active session so short trailing conversations do not get stranded below the 20-message threshold.
- Owner-scoped session-summary memories are now parsed for `unresolved_items`, and those open threads appear in prompt assembly as a `Carryover from previous sessions` section for future conversations.

Document ingestion now feeds the same knowledge path:

- The `document-knowledge` skill can parse local documents, summarize them through the LLM, and persist durable facts with `source=document_ingestion`.
- Ingested document summaries are also stored as session memories when a session context is available, so the current conversation can immediately build on the imported material.

### Limitations of the Current Model

The current store is a solid foundation but does not yet implement:

- **Full entity classes** — Contacts, Events, Knowledge, Memories, Artifacts, and Proposals are not yet distinct entity types with their own tables and lifecycle.
- **Relationship layer** — No explicit relationship tracking between entities with confidence, recency, and provenance metadata.
- **Provenance tracking** — No derivation chains, reinforcement counts, or mutation histories.
- **Confidence decay** — No confidence scores or natural decay on relationships.
- **Cross-session graph reasoning** — Semantic recall works across owner/session/global knowledge, but the higher-level graph still lacks richer typed relationships and consolidation policies beyond the current auto-linking pass.

---

## Target: Full World Model

The planned memory upgrade implements the conceptual design's entity model:

### Entity-Based Storage

Instead of flat key/value facts, each entity has a stable UUID identity, typed attributes that grow over time, relationships to other entities (with confidence, recency, provenance), and full provenance metadata (source, derivation chain, mutation history).

### Subconscious Reflection Pipeline

The Subconscious Process (defined in the conceptual design) drives memory evolution:

| Tier | Cadence | What it does to memory |
|------|---------|----------------------|
| **Shallow Reflections** | Post-interaction | Updates entity attributes, creates new relationships. May not delete or reclassify entities. |
| **Consolidation** | Daily | Connects dots, strengthens/weakens relationships, surfaces patterns. May deprecate Knowledge nodes. May update purely inferred Configuration. |
| **Deep Reflections** | Weekly/event-triggered | Comprehensive reorganization. May modify inferred Configuration and Priorities. May reclassify or archive entities. May *propose* changes to Owner-set state (requires user confirmation). |

### Data Lifecycle

The conceptual design defines four lifecycle actions for entities:

| Action | Meaning |
|--------|---------|
| **Archive** | Soft-delete. Preserved for audit and recovery. Default for all Delete commands. |
| **Forget** | Remove significance from a Memory. Always requires owner confirmation. |
| **Supersede** | Replace outdated Knowledge with newer version. Derivation chain preserved. |
| **Tombstone** | Hard-delete with audit trail. Requires owner confirmation. |

---

## Memory Privacy and Scope

Memory is **per-user** and **per-agent-instance**. There is no shared global memory between users.

In future NAVI Net phases, users will be able to export/import memory graphs, choose which notes to make project-public, and delete individual entities. All memory operations are append-only audit-logged.

---

## ADR Note

The decision to upgrade memory from flat SQLite facts to the full entity-based World Model is informed by the Conceptual Design Overview and AIOS A-Mem research. A formal ADR for this change (`ADR-006-world-model`) should be written before implementation begins.

---

*NAVI AI Memory and World Model — aligned with the [Conceptual Design Overview](../canonical/conceptual-design-overview.md).*
*Last updated: 2026-03-21*
