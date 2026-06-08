# NAVI — Conceptual Design Overview

*Status: complete.*
*Last Updated: March 10, 2026 @ 2:00 PM*

> [!TIP]
> Looking for the current system maturity and implementation status? See the [NAVI Systems Map](../architecture/navi-systems-map.md).

---

## The Problem

Build a single unified AI system serving three user archetypes—the everyday person wanting a virtual companion, the business/productivity user needing task automation, and the developer needing an agentic coding partner—through one coherent, deeply personalized experience.

---

## Three Layered Roles

1. **Character** — Social companion, personality, warmth. The foundation.
2. **Assistant** — Productivity, scheduling, task management layered on top.
3. **Coder** — Agentic coding, codebase management, multi-AI orchestration on top of that.

Each layer builds on the previous. The character is always present as connective tissue.

---

## Core Design Principle

NAVI lives in a symbiotic relationship with its owner. It learns the user the way the user learns it. Everything starts minimal and grows continuously—nothing is fixed, everything is dynamic and emergent.

**Entities are the core modeling unit; attributes, relationships, and provenance deepen them over time.**

---

## Architectural Layers

NAVI's architecture is organized into four distinct layers, each with a primary responsibility and a defined interface to adjacent layers.

```
┌───────────────────────────────────────┐
│         Experience Layer              │  ← Roles, Personas, Tone, Presentation
│   (modulates cognition + output)      │
├───────────────────────────────────────┤
│         Cognitive Layer               │  ← Conscious + Subconscious Processes
│    ↕ reads/writes ↕                   │
│         World Model                   │  ← Entities, Relationships, State
├───────────────────────────────────────┤
│         Capability Layer              │  ← Commands, Connectors, Plugins
│   (execution arm, called by Decide)   │
└───────────────────────────────────────┘
```

| Layer | Primary Responsibility | Interface |
| :---- | :---- | :---- |
| **Experience** | Determines how NAVI presents itself—role selection, persona, tone, interaction style. Modulates the Cognitive Layer's output but does not own reasoning. Roles define what kind of work NAVI is doing; Personas define how NAVI behaves. Experience may bias attention and presentation but does not own decision logic. | Reads Cognitive output; applies behavioral policy before delivery. Can bias Cognitive attention (e.g., Character mode weights emotional context higher). |
| **Cognitive** | Runs the Conscious and Subconscious processes. Owns reasoning, reflection, and decision-making. | Reads/writes the World Model. Receives modulation from Experience. Issues Commands to the Capability Layer. |
| **World Model** | The single source of truth for all entity state, relationships, and structured knowledge. | Accessed by the Cognitive Layer. Not directly modified by Experience or Capability—changes flow through Cognitive processes. |
| **Capability** | Executes actions in the world. Commands, Connectors, Plugins. | Invoked by Cognitive (specifically the Execute step). Returns results to Cognitive for Reflection. |

Each layer has primary responsibility over its domain. Cross-layer interactions happen through defined interfaces, not arbitrary access.

---

## Entity Classes

*All start minimal and grow over time.*

### World Entities

Represent the state of the world as NAVI understands it.

| Entity | Description |
| :---- | :---- |
| **Contacts** | NAVI's own contact graph of people and entities. Starts with the owner. Default attribute is a name. |
| **Events** | Scheduling and temporal awareness. Drives applications like calendar. |
| **History** | Append-oriented interaction log that enriches over time. Authoritative for what was recorded, though not infallible—recording fidelity is itself an attribute. Minimally interpreted at write time; interpretation belongs to Knowledge and Memory. History records two classes of event: interaction events (what happened between NAVI and the user or world) and execution outcome records (the result of every command attempt, whether succeeded, failed, partially succeeded, cancelled, timed out, or rejected). Every command attempt produces a History entry regardless of outcome — failure is a first-class recorded state, not an absence of record. A retention and deletion policy (user-directed, privacy, compliance) is required but deferred to the provenance model. |
| **Knowledge** | Curated facts plus emergent understanding. Derived from History and external sources. Always attributed—every knowledge node carries provenance (source, confidence, derivation chain). Mutable: can be revised, merged, or deprecated as understanding evolves. |
| **Memories** | Experiences and moments that carry significance and emotional weight. Subjective and interpretive—two interactions can produce different Memories depending on context. Durable by default but not permanent: a deletion path (user-directed, privacy, compliance, false memory correction) is required and deferred to the provenance model. |
| **Artifacts** | Files, documents, notes—anything tangible NAVI creates or manages. |
| **Projects** | Durable work containers that scope context, objectives, and artifacts for a specific body of work. Defined in the [Project System Specification](../specs/project-system-v1.md). |
| **Proposals** | First-class entities representing actions that require user authorization before execution. Generated by Validate/Govern and by Subconscious reflection processes. Carry full provenance, a defined lifecycle, and a serialized representation of the proposed action sufficient to execute without reconstruction. Detailed in the Proposal Queue section of Tier 3. |

### Cognitive Entities

Internal to the reasoning system. Not directly visible to the user.

| Entity | Description |
| :---- | :---- |
| **User Model** | A composite projection, not a standalone entity. Assembled at query time from Contacts (the owner record), Knowledge (facts about the user), Memories (shared experiences), Configuration (stated preferences), and Priorities (goals and intentions). The User Model is never stored as a single object—it is always derived. |

### Capability Entities

Executable, versioned, composable units of behavior.

| Entity | Description |
| :---- | :---- |
| **Skills** | Modular, expandable capability registry. Each Skill is versioned, carries its own permission profile, and declares its input/output contract. Skills are the intersection of what NAVI knows how to do and what it is allowed to do. |

### Control Entities

System-level configuration that governs behavior.

The system distinguishes four kinds of state that appear throughout the architecture. These definitions apply everywhere the terms are used:

- **Explicit** — Directly and intentionally set by the user. Authoritative. Cannot be overridden by any reflection process without user confirmation.
- **Owner-set** — A subset of Explicit. State that the owner has specifically authored (as opposed to values that are explicit because they were system-defaulted and never changed). The distinction matters for governance: Owner-set state requires user confirmation to modify; other explicit state may have lower confirmation requirements depending on context.
- **Inferred** — Learned by NAVI from observed behavior, patterns, or history. May be updated autonomously by reflection processes within their permitted mutation scope.
- **Derived** — Assembled or computed at query time from underlying entities. Never stored directly as its own entity; always reconstructed on demand (e.g., the User Model).

| Entity | Description |
| :---- | :---- |
| **Configuration** | Explicit and inferred preferences and settings. Explicit Configuration is set directly by the user. Inferred Configuration is learned from behavior and may be updated by reflection within permitted scope. |
| **Priorities** | Stated and observed priorities defining goals and intentions. Stated Priorities are Owner-set. Observed Priorities are inferred from behavior. |

---

## Relationship Layer

Relationships between entities are flexible and extensible rather than rigidly fixed. NAVI actively creates, modifies, and dissolves connections as understanding evolves. However, every relationship carries defined structural metadata so the system can reason about, strengthen, decay, and audit connections:

| Field | Purpose |
| :---- | :---- |
| **Type** | Relationship category (knows, owns, occurred-at, prefers, related-to, derived-from, etc.). Types are extensible—new categories emerge as NAVI encounters novel connections. |
| **Confidence** | How strongly NAVI believes the relationship is valid. Numeric score, updated by reinforcement or contradiction. |
| **Recency** | When the relationship was last reinforced by observation or interaction. Drives natural decay—relationships that are never reinforced weaken over time. |
| **Provenance** | Which event, source, or reflection tier produced the relationship. Full provenance metadata (derivation chain, mutation history) is defined in the Provenance Model (Tier 3). |

Without this metadata, the Subconscious has no reliable mechanism for strengthening, decaying, or pruning relationships. The metadata is minimal by design—it grows richer as the Provenance Model is implemented.

---

## Governance and Validation

Governance is the system-wide principle that constrains what NAVI is permitted to do, regardless of what it is capable of doing. It applies at the boundary between decision and execution.

### Validation Order

When the Conscious Process reaches the Validate/Govern step, checks execute in deterministic order:

1. **Permissions** — Does NAVI have access to the required entities, connectors, and scopes?
2. **Policy** — Does the action comply with system-wide rules (safety, privacy, legal, ethical)?
3. **Configuration** — Does the action align with user-stated preferences and constraints?
4. **Priority Alignment** — Does the action serve the user's current goals and priorities?
5. **Risk Assessment** — Does the action carry potential for unintended, irreversible, or disproportionate consequences?

Each check produces a pass/fail with a reason. Failure at any step short-circuits: later checks are not evaluated.

### Validation Outcomes

| Outcome | Description |
| :---- | :---- |
| **Approved** | All checks pass. Proceed to Execute. |
| **Requires Confirmation** | Checks pass but risk or ambiguity exceeds the autonomous threshold. Action is paused pending user confirmation. A Proposal is created and enters the Proposal Queue with priority Blocking if an active action is paused, or Queued if async. |
| **Modified** | Checks partially pass. The action is adjusted to satisfy constraints and re-evaluated. If the modification only reduces scope (e.g., removing a restricted attachment), proceed automatically. If the modification changes the action's intent (e.g., converting a send to a draft), user confirmation is required before proceeding. |
| **Rejected** | A check fails with no viable modification. Action does not execute. Constraint context (which check failed and why) is returned to Decide for replanning. |

### Constraint Feedback Loop

When validation rejects an action, the constraint context flows back to the Decide step—not to the user as an error. The Conscious Process replans with the constraint as new input. Only if replanning fails repeatedly does the system surface the constraint to the user.

### Tier Authority

Governance tiers determine who can modify which constraints:

| Tier | Authority | Examples |
| :---- | :---- | :---- |
| **System** | Immutable. Set by NAVI's core design. Cannot be overridden by user or plugins. | Safety policy, permission model, core ethical constraints. |
| **Owner** | Set by the user. Overrides Plugin-level but not System-level. | Privacy preferences, notification rules, autonomy boundaries. |
| **Plugin** | Set by installed plugins. Overrides nothing—only adds constraints within its scope. | API rate limits, service-specific policies, integration guardrails. |

Conflicts resolve by tier: System > Owner > Plugin. Within a tier, the most restrictive constraint wins.

---

## The Conscious Process

*The foreground process. Active during engagement. Drives real-time interaction.*

### Loop

**Perceive → Interpret → Contextualize → Decide → Validate/Govern → Execute → Reflect**

| Step | Description |
| :---- | :---- |
| **Perceive** | Take in input from active channels—voice, text, sensor data, notifications. Raw intake. |
| **Interpret** | Parse intent, context, emotional tone, urgency. What is actually being asked or needed? |
| **Contextualize** | Pull relevant entities from the World Model. Who's involved, what's the history, what are active priorities, what knowledge or artifacts are relevant? |
| **Decide** | Choose a response or action. Which role is most appropriate—Character, Assistant, Coder? Which Skill or Capability gets invoked? |
| **Validate/Govern** | Evaluate whether the selected action is permitted, safe, and consistent with system constraints. Checks permissions, policy, configuration, priority alignment, and risk in deterministic order. Outcome is Approved, Requires Confirmation, Modified, or Rejected. Rejected actions return to Decide with constraint context. Actions requiring confirmation generate a Proposal and enter the Proposal Queue. |
| **Execute** | Act. Respond, create, schedule, connect, delegate. |
| **Reflect** | Brief immediate post-interaction reflection. Queues anything learned for Subconscious processing at the appropriate depth and cadence. |

---

## The Subconscious Process

*The background process. Runs continuously regardless of active engagement. Orchestrates learning, reorganization, and self-evaluation.*

### Reflection Tiers

| Tier | Cadence | Description | Mutation Permissions |
| :---- | :---- | :---- | :---- |
| **Shallow Reflections** | Frequent — post-interaction or short timer | Lightweight surface-level updates. Flags patterns, updates immediate context. Quick entity updates based on recent interactions. | May update entity attributes. May create new relationships. May not delete or reclassify entities. May not modify Configuration or Priorities. |
| **Consolidation** | Periodic — daily | Connects dots across recent Shallow Reflections. Updates Knowledge graphs, strengthens or weakens relationship mappings, surfaces emerging patterns for deeper consideration. Bridges reactive and strategic understanding. | May update and create entities and relationships. May deprecate (soft-delete) Knowledge nodes. May update purely inferred Configuration autonomously within permitted scope. Must generate a Proposal when a proposed inferred Configuration change would affect explicit or Owner-set behavior. May not modify explicit or Owner-set Configuration or Priorities under any circumstance. |
| **Deep Reflections** | Infrequent — weekly or event-triggered | Comprehensive reorganization. Revisits Priorities, reshapes core models, rewires entity relationships, evaluates whether Configuration and rule sets themselves need updating. NAVI stepping back to ask: what have I learned and does my understanding still hold? | Full mutation access on inferred state. May modify inferred Configuration and inferred Priorities. May reclassify or archive entities. May restructure relationship topology. May *propose* changes to explicit Owner-set Configuration and Priorities, but cannot apply them without user confirmation. Owner-tier governance constraints always require user confirmation. |

### Escalation Trigger

Significant events can bypass the standard reflection cadence and escalate to the appropriate depth immediately.

- **Manual Escalation** — User explicitly signals significance. "Remember this." "This is important." "Process this deeply." NAVI immediately escalates to the appropriate reflection depth.
- **Automatic Escalation** — NAVI detects significance autonomously based on signals—emotional weight, major life events, conflicts with existing Priorities or Knowledge, anomalies in behavior patterns.

Escalation is graduated: a flagged item may go straight to Deep Reflection or fast-track through Shallow then Consolidation depending on urgency and context.

---

## Inter-Process Communication

The Conscious and Subconscious operate in parallel with a defined handoff channel.

### Conscious → Subconscious

The Reflect step produces a **reflection payload** containing: what happened, what was learned, what seemed surprising, and a suggested reflection tier. The Subconscious triages the payload—it may accept the suggested tier or escalate/demote based on its own pattern detection.

### Subconscious → Conscious

Updated understanding is written to the World Model. The Conscious Process picks up changes naturally during the Contextualize step of the next interaction. Under normal operation there is no push mechanism—the World Model is the shared surface. Urgent interruptions are the only exception, and are governed by strict criteria defined below.

### Subconscious → Conscious (Urgent Interruption)

The Subconscious may surface a finding to the Conscious Process mid-interaction when all of the following are true:

1. The finding directly contradicts information the Conscious Process is actively using.
2. The contradiction is high-confidence (not speculative).
3. Acting on the outdated information would produce a materially wrong or harmful outcome.

Interruptions are not suggestions. They are corrections. The Subconscious does not interrupt to offer "you might also want to know" insights—those go through the normal World Model update path.

**Recursion guard:** Only one interruption may occur per decision cycle. If the Subconscious detects additional contradictions during the same cycle, they are queued and surfaced at the start of the next cycle. This prevents interrupt → re-evaluate → new contradiction → interrupt loops.

### Interruption Delivery

| Mode | Description |
| :---- | :---- |
| **Advisory** | Insight is surfaced to the user; action is not halted. |
| **Blocking** | Action is paused; user confirmation required to proceed. |

The Experience Layer determines how interruptions are communicated. Governance determines when they are permitted.

### Process Priority

- The Conscious process takes precedence during active engagement.
- The Subconscious can interrupt with urgent corrections when the criteria above are met.
- Outside active engagement, the Subconscious has full processing priority.

---

## Capability Layer

The Capability Layer sits below the Cognitive Layer. It is the execution arm of NAVI—how NAVI does things in the world. It is composed of three components in a defined hierarchy:

- **Commands** define what NAVI wants to do
- **Connectors** define where and how that action crosses system boundaries
- **Plugins** define packaged capabilities built on top of Commands, Connectors, and Skills

*Note: While connectors, skills, policies, runtimes, and LLM providers are distinct conceptual entities, repo-owned concrete capability implementations are physically grouped within plugin packages such as `plugins/telegram/` or `plugins/llm-ollama/`. The plugin package is the lifecycle and install boundary; core framework code remains in `internal/*`, root `connectors/`, and root `skills/` support surfaces.*

---

### Commands

Commands are the instruction primitives of NAVI. Every time the Conscious Process reaches the Execute step, it issues a Command. Commands are organized into three categories to keep the system predictable and prevent overlap.

NAVI has 10 primitive Commands—small enough to remain stable, expressive enough to build complex behaviors through composition.

#### 1. State Commands

Operations that read or modify structured entities within NAVI's internal system.

| Command | Description |
| :---- | :---- |
| **Query** | Retrieve information from internal entities or indexed knowledge sources. |
| **Create** | Instantiate a new entity such as an artifact, memory, task, event, or contact. |
| **Update** | Modify attributes or state of an existing entity. |
| **Delete** | Remove or archive an entity. Default behavior is soft-delete (archive). Hard delete requires explicit confirmation and is governed by the provenance model. |

#### 2. Effect Commands

Operations that produce side effects outside NAVI's internal data model.

| Command | Description |
| :---- | :---- |
| **Invoke** | Execute a skill, tool, API, or external system capability. |
| **Send** | Deliver information to an external recipient—message, notification, response. |
| **Acquire** | Fetch external resources or data not already represented as internal entities. |
| **Schedule** | Register a time-based or trigger-based action for future execution. |

#### 3. Coordination Commands

Commands used to manage multi-step execution across agents or workflows.

| Command | Description |
| :---- | :---- |
| **Delegate** | Assign a task or command sequence to another agent, service, or subsystem. |
| **Compose** | Group multiple commands into a coordinated workflow or execution unit. |

Complex behaviors are achieved through composition rather than expanding the primitive command set. The 10 primitives remain stable.

#### Idempotency Expectations

In distributed and retry scenarios, Commands carry explicit idempotency semantics:

| Command | Expectation |
| :---- | :---- |
| **Query** | Idempotent. Repeated calls return the same result (given unchanged state). |
| **Create** | Not inherently idempotent. May produce duplicates unless the caller provides a deduplication key. |
| **Update** | Should be version-aware. Concurrent updates must detect conflicts rather than silently overwriting. |
| **Delete** | Idempotent. Deleting an already-archived entity is a no-op. Soft-delete is the default. |
| **Invoke** | Depends on the underlying Skill or service. The Skill contract must declare whether the operation is safe to retry. |
| **Send** | Not idempotent. Duplicate sends produce duplicate deliveries unless guarded by a message ID. |
| **Acquire** | Idempotent only when the target resource is versioned or content-addressed. Repeated fetches of live web content, mutable APIs, or dynamic sources may return updated data. |
| **Schedule** | Not inherently idempotent. Duplicate scheduling must be guarded by the scheduler (deduplication by trigger + action signature). |
| **Delegate** | Depends on the delegate. The delegation contract must specify retry semantics. |
| **Compose** | Inherits the idempotency profile of its constituent commands. A Compose is only idempotent if all its children are. |

---

### Connectors

Connectors are integration adapters that allow NAVI to interact with external channels, systems, services, devices, and identity layers. They translate Effect Commands into system-specific operations while handling authentication, permissions, protocols, and data normalization.

#### 1. Communication Connectors

Interfaces for exchanging information with people across external channels.

| Type | Description |
| :---- | :---- |
| **Messaging** | Email, SMS, chat platforms, direct messages. |
| **Voice** | Speech input, speech output, telephony, voice assistants. Conceptually a communication type but operationally depends on Device Connectors and speech services. |
| **Social** | Social media platforms, posting, monitoring, and interaction flows. |

#### 2. Information Connectors

Interfaces for reading, writing, and syncing external information sources.

| Type | Description |
| :---- | :---- |
| **Files and Storage** | Local filesystem, cloud storage, shared drives, document repositories. |
| **Databases** | Structured external data stores, queryable systems, transactional sources. |
| **Web and Content** | Web pages, search, crawling, indexing, content retrieval. |
| **Streams and Feeds** | Real-time event feeds, pub/sub systems, live APIs, telemetry streams. |

#### 3. Service Connectors

Interfaces to external software platforms that expose domain-specific capabilities.

| Type | Description |
| :---- | :---- |
| **Productivity** | Calendars, tasks, notes, contacts, office tools. |
| **Business and Operations** | CRM, finance tools, support systems, internal business platforms. |
| **Developer Tools** | Repositories, issue trackers, CI/CD, observability systems. |
| **AI and Model Services** | External models, inference APIs, embeddings, agent frameworks. |

#### 4. Device and Environment Connectors

Interfaces to user devices, local operating systems, and physical-world signals.

| Type | Description |
| :---- | :---- |
| **Location** | GPS, geofencing, spatial context. |
| **Sensors** | Camera, microphone, biometrics, environmental inputs. |
| **Device Controls** | OS-level actions, app control, hardware interaction, local automation. |

#### Identity and Access Plane

Identity and Access is a cross-cutting concern, not a peer Connector category. Every other Connector category depends on this layer. It underpins authentication, permissions, and account linkage across all Connectors.

| Type | Description |
| :---- | :---- |
| **Authentication** | OAuth, API keys, account linking, session handling. |
| **Permissions** | Scopes, consent, access verification, policy enforcement. |

---

### Plugins

Plugins are the top layer of the Capability Layer stack. A Plugin is a self-contained, installable capability package that combines Skills, Commands, and Connectors into a reusable unit of behavior. Plugins are the mechanism by which NAVI extends its abilities over time without changing its core architecture.

A Plugin does not define a new execution model. It packages existing primitives into a deployable module.

#### What a Plugin Contains

| Component | Description |
| :---- | :---- |
| **Capabilities** | What the plugin enables NAVI to do. |
| **Skills** | The internal reasoning or action routines the plugin uses. |
| **Commands** | The primitive operations the plugin is allowed to issue. |
| **Connectors** | The external systems the plugin depends on. |
| **Permissions** | What access, scopes, and consent are required. |
| **Interfaces** | What actions, entities, triggers, or outputs the plugin exposes back to NAVI. |
| **Policies and Constraints** | Usage boundaries, safety rules, and operational limits. Subject to Governance tier authority (Plugin-tier, overridden by Owner and System). |

#### Plugin Categories

Plugins are organized by functional role in the system, not by user domain.

**1. Domain Plugins** — Packages capabilities for a specific problem domain. *Examples: personal finance, health tracking, scheduling, developer workflows, CRM support.*

**2. Workflow Plugins** — Coordinates multi-step processes across Skills and Connectors. *Examples: onboarding flow, daily planning routine, meeting preparation, incident response workflow, automated reporting.*

**3. Integration Plugins** — Exposes a new external platform or service to NAVI. Often Connector-heavy. *Examples: Gmail, GitHub, Slack, Stripe, home automation integrations.*

**4. Agentic Plugins** — Extends NAVI's autonomous execution and orchestration abilities. *Examples: multi-agent delegation, scheduled task runners, monitoring and trigger systems, external model routing, long-running automation.*

**5. Sensory Plugins** — Extends NAVI's perception of the user, device, or physical environment. *Examples: location awareness, voice interaction, camera-based understanding, biometric monitoring, device-state awareness.*

#### Plugin Discovery

NAVI does not know Plugins by implementation details. It knows them by declared capability contracts. NAVI reasons about what it needs, then resolves the matching Plugin.

A Plugin is discoverable through four things:

- **Capability Metadata** — A machine-readable description of what the plugin can do.
- **Triggers** — Conditions under which the plugin becomes relevant.
- **Permission Profile** — What access it requires before invocation.
- **Invocation Interface** — What Command surface NAVI uses to call it.

#### Plugin Lifecycle

| Stage | Description |
| :---- | :---- |
| **Discover** | NAVI or the user identifies a capability gap and finds a matching plugin. |
| **Install** | The plugin is added to the system, dependencies resolved, permissions requested. |
| **Register** | The plugin publishes its capabilities, interfaces, triggers, and constraints to NAVI's capability registry. |
| **Activate** | The plugin becomes available for invocation by the Conscious Process. |
| **Invoke** | The plugin is selected and executed when its capability is needed. |
| **Learn** | NAVI refines when, why, and how to use the plugin based on outcomes and experience. |
| **Update** | The plugin evolves as its capabilities, dependencies, or policies change. |
| **Retire** | The plugin is disabled, replaced, or removed when obsolete or no longer trusted. |

---

## Tier 3 — Data Integrity and Failure Handling

### Proposal Queue

The Proposal Queue is a first-class entity in the World Model. It is not transient state on other entities—it is stored, queryable, carries its own provenance, and has a defined lifecycle.

A Proposal is created whenever an action cannot proceed autonomously and requires user authorization. Two processes generate Proposals: Validate/Govern (when outcome is Requires Confirmation) and the Subconscious (when Deep Reflection or Consolidation proposes a change that crosses an explicit or Owner-set boundary).

Plugin-tier processes may not generate Proposals directly. Proposals are resolved by one of two actors: the owner (via explicit user action) or the system (via auto-expiry on TTL elapsed, or auto-decline triggered by a lifecycle event on an affected entity). No reflection process, plugin, or Connector may resolve a Proposal — resolution is exclusively owner-action or system-event driven.

#### Proposal Entity Fields

| Field | Description |
| :---- | :---- |
| **proposal\_id** | Unique identifier. Stable across status changes. |
| **source\_process** | Which process generated the proposal: `validate_govern`, `deep_reflection`, or `consolidation`. |
| **source\_trigger** | The specific governance check or reflection finding that caused the proposal—e.g., "Risk Assessment threshold exceeded" or "Inferred Priority conflicts with Owner-set Priority." |
| **proposed\_action** | Fully serialized command or mutation. Contains enough information to execute the action exactly as proposed if approved—no reconstruction required at resolution time. |
| **affected\_entities** | List of entity IDs that would be mutated if the proposal is approved. Used to detect conflicts with other pending proposals and to cascade auto-decline when a target entity is archived or tombstoned. |
| **rationale** | Human-readable explanation of why this action is being proposed and why it requires confirmation rather than autonomous execution. Surfaced to the user through the Experience Layer. |
| **priority** | `blocking` — an active Conscious Process action is paused waiting on this proposal. `queued` — proposal is async; no active action is blocked. |
| **status** | `pending` / `approved` / `declined` / `expired` / `superseded`. |
| **created\_at** | Timestamp when the proposal was created. |
| **expires\_at** | Computed from proposal type and priority. Blocking proposals have short TTLs; queued proposals from Deep Reflection have longer TTLs. Stale proposals are auto-declined rather than left open indefinitely. |
| **resolved\_at** | Timestamp when status left `pending`. Null while pending. |
| **resolved\_by** | Who or what resolved the proposal: `owner` (user action), `system` (auto-expiry, auto-decline from entity lifecycle event). |
| **resolution\_note** | Optional free-text context. Set by the user when manually declining, or by the system when auto-declining (e.g., "Target entity archived"). |

#### Proposal Lifecycle States

```
pending → approved   (user confirms)
        → declined   (user rejects, or system auto-declines)
        → expired    (TTL elapsed with no resolution)
        → superseded (a newer proposal covers the same entity + action)
```

Supersede applies when a Consolidation or Deep Reflection produces a new proposal whose `proposed_action` and `affected_entities` overlap with a pending proposal from an earlier reflection cycle. The older proposal moves to `superseded`; the newer one becomes the authoritative pending item. This prevents the user from facing stale confirmation requests for decisions that have already been reconsidered.

#### Proposal Resolution

Approving a proposal does not immediately execute the action. The approved proposal is returned to the originating process (Validate/Govern or the appropriate Subconscious tier), which re-evaluates governance once before executing. This guards against state changes that occurred between proposal creation and approval that might invalidate the original checks.

Declining a proposal returns constraint context to the originating process—the same feedback loop used for Rejected governance outcomes.

#### Batching and Surfacing

The Experience Layer controls how proposals are presented to the user. The Queue itself imposes no batching policy—that is an Experience Layer concern. The Queue does enforce ordering: Blocking proposals always surface before Queued proposals regardless of creation order.

Open questions: maximum queue depth before the system downgrades non-blocking proposals, and whether Blocking proposals from interrupted Conscious Process actions should time out and auto-decline if the user does not respond within a defined window.

---

### 3.1 Provenance Model

Every entity and relationship tracks lifecycle metadata. Without provenance, Knowledge drifts and relationships become unauditable.

#### Core Fields

| Field | Purpose |
| :---- | :---- |
| **source** | What produced this entity or relationship: `user_input`, `sensor`, `inference`, `shallow_reflection`, `consolidation`, `deep_reflection`, `plugin`, `proposal_approved`, `execution_succeeded`, `execution_failed`, `execution_partial`, `execution_cancelled`, `execution_timed_out`, `execution_rejected`. Execution source types apply specifically to History execution outcome records and to World Model mutations that resulted from (or were blocked by) a command attempt. |
| **timestamp** | When the entity was created and last modified. |
| **confidence** | How strongly NAVI believes the data is accurate. Numeric score. |
| **derivation\_chain** | Ordered list of upstream entity IDs or event IDs this was derived from. Enables full audit of how a piece of knowledge or memory originated. |
| **reinforcement\_count** | How many times independent observations have confirmed this data. Strengthens confidence without duplicating source records. |
| **mutation\_history** | Append-only log of changes: what changed, when, by which process, and why. Each entry references the process that made the change and, where applicable, the proposal that authorized it. |

#### Proposal Authorization in Mutation History

When a mutation is authorized by a user-approved Proposal, the mutation history entry for the affected entity includes a `proposal_id` reference. This creates a durable audit trail:

> Entity X was modified by `deep_reflection` on [date] under authorization of Proposal [ID], approved by owner on [date].

This applies to all mutations that flowed through the Proposal Queue—whether they originated from Validate/Govern, Consolidation, or Deep Reflection. Mutations that did not require a proposal (i.e., autonomous mutations within the reflection tier's permitted scope) record the source process directly without a proposal reference.

#### Proposals as Provenance Entities

Proposals themselves carry provenance fields—`source_process`, `source_trigger`, and `created_at`—making the proposal record independently auditable. A complete audit of any mutation can trace: the observation that triggered reflection → the reflection tier that produced the proposal → the proposal that captured the intent → the user approval that authorized execution → the mutation history entry on the affected entity.

#### Confidence Propagation

Confidence scores propagate through derivation chains with attenuation: a Knowledge node derived from a low-confidence source inherits a confidence ceiling. The propagation rule (multiplicative, minimum-of-chain, or weighted average) is an open question deferred to implementation, but the principle is that derived confidence cannot exceed source confidence.

Open questions: granularity of mutation history (per-attribute vs. per-entity version snapshots), storage cost tradeoffs at scale, and the precise confidence propagation algorithm.

---

### 3.2 Data Lifecycle Model

Defines the semantics of removal and replacement. Without clear lifecycle actions, "delete" is ambiguous and audit trails break.

#### Lifecycle Actions

| Action | Meaning | Requires Proposal? |
| :---- | :---- | :---- |
| **Archive** | Soft-delete. Entity is hidden from active queries but preserved for audit and recovery. Default for all Delete commands. | No — within autonomous scope for permitted processes. |
| **Forget** | Remove the significance and emotional weight from a Memory. The interaction record in History may persist, but the Memory interpretation is dissolved. Downstream derivatives (Knowledge nodes, relationship weightings) that were sourced from this Memory are flagged for re-evaluation. | Always — Forget is irreversible in effect and always requires owner confirmation. |
| **Supersede** | Replace outdated Knowledge with a newer version. The old node is marked deprecated with a pointer to its replacement. Derivation chain is preserved on both the deprecated and replacement node. | Depends on whether the superseded node is Owner-set. Inferred Knowledge may be superseded autonomously within reflection tier scope. Owner-set Knowledge requires a Proposal. |
| **Tombstone** | Preserve a record that deletion occurred without preserving the deleted content. Used for compliance, privacy, and audit trail integrity. The tombstone record carries the entity ID, deletion timestamp, source process, and proposal reference if applicable. Content is destroyed. | Always for hard-delete tombstoning — requires owner confirmation. |

#### Proposal Cascade on Entity Lifecycle Events

When an entity transitions to Archived or Tombstoned, the system checks the Proposal Queue for pending proposals whose `affected_entities` list includes the transitioning entity. Those proposals are auto-declined with a system-generated `resolution_note` identifying the lifecycle event that caused the decline. This prevents orphaned proposals from resolving against entities that no longer exist in an actionable state.

#### Forget and Downstream Derivatives

When a Memory is Forgotten, the system does not automatically scrub downstream Knowledge or relationship weightings that were derived from it. Instead, it flags those derivatives for re-evaluation at the next appropriate Consolidation or Deep Reflection cycle. The reflection process then determines whether the derivative still holds on independent grounds. This approach preserves potentially valid derived understanding while removing the Forgotten Memory as an authoritative source.

Open questions: retention durations per lifecycle state, user-facing controls for each action type, and whether Tombstone records themselves should have an expiry (e.g., after a statutory retention period).

---

### 3.3 Failure Model

Defines system behavior when execution does not succeed. Without a failure model, partial execution and silent failures corrupt state and erode trust.

**Anchor principle: NAVI never hides failure by pretending work completed. Partial, failed, and degraded outcomes are first-class states.**

---

#### A. Failure Taxonomy

Failures are classified by origin. Classification determines recovery path.

| Class | Description | Recovery Path |
| :---- | :---- | :---- |
| **Validation Rejection** | Governance blocked the action before execution began. No side effects have occurred. | Constraint context returned to Decide for replanning. No compensation required. |
| **Permission Denial** | The required access, scope, or consent was not available at execution time. Sub-types: `not_yet_authorized` (prompt for consent) and `permanently_forbidden` (constraint context to Decide, no retry). | Depends on sub-type. Not-yet-authorized may generate a Proposal. Permanently-forbidden closes the path. |
| **Connector Unavailable** | An external system is unreachable or returning errors. Sub-types: `transient` (retry eligible) and `permanent` (circuit-open, surface to user). | Transient: retry with backoff up to budget. Permanent: circuit-breaker opens, capability marked degraded, user notified. |
| **Execution Failure** | The command reached the target system but the operation failed — API error, constraint violation, data rejection. | Depends on reversibility class (see Section C). May require compensation. |
| **Partial Execution** | A Compose command succeeded on some steps and failed on others. Some side effects may have occurred. | Compensation-first (see Section C). Partial completion surfaced explicitly. Recovery state written to World Model. |
| **Timeout** | A command or sub-command did not complete within its time budget. State at timeout is unknown for external targets. | Treat as partial execution. Do not assume success or failure for external targets. Write timeout record to History. |
| **Plugin Crash** | A plugin process terminated abnormally during or before execution. | Plugin is isolated — crash must not propagate to World Model or Conscious loop. Structured failure result returned. Plugin marked degraded pending restart or recovery. |
| **Contradiction / Stale-State Invalidation** | The Subconscious detects that the state the Conscious Process acted on was outdated or contradicted by new information. | If pre-execution: interruption (per Inter-Process Communication rules). If post-execution: write contradiction to History, trigger Subconscious reconciliation at appropriate reflection tier. |

---

#### B. Execution Outcome Model

Every command attempt produces an execution outcome record written to History, regardless of whether the command succeeded, failed, or never reached execution. Failure is a first-class recorded state — not an absence of record.

| Field | Description |
| :---- | :---- |
| **command\_id** | Logical identity of the command across its entire retry chain. Stable from first attempt through all retries to final resolution. Used to group all attempt records for the same logical operation. |
| **attempt\_id** | Unique identifier for this specific execution attempt. Each retry produces a new attempt record with a new `attempt_id`. This is the primary key of the execution outcome record. |
| **attempt\_number** | Ordinal position within the retry chain. First attempt is 1. Monotonically increasing within a single `command_id` retry chain and not globally meaningful across commands. |
| **retry\_of** | The `attempt_id` of the immediately preceding attempt in the retry chain. Null on first attempt. Enables full retry chain traversal without relying on `attempt_number` alone. |
| **command\_type** | The primitive command type: `query`, `create`, `update`, `delete`, `invoke`, `send`, `acquire`, `schedule`, `delegate`, `compose`. |
| **start\_time** | When this attempt was issued by the Conscious Process. |
| **end\_time** | When the outcome for this attempt was determined. Null if still in flight. |
| **outcome** | `succeeded` / `failed` / `partially_succeeded` / `cancelled` / `timed_out` / `rejected_pre_execution`. |
| **failure\_reason** | Structured failure class from the taxonomy above, plus a human-readable description. Null on success. |
| **affected\_entities** | Entity IDs that were mutated, or were intended to be mutated. On failure, records what would have changed. On partial success, distinguishes completed vs. incomplete mutations. |
| **retryable** | Boolean. Derived from failure class and command type. Validation rejections and permanent permission denials are not retryable. Transient connector failures are. |
| **compensation\_required** | Boolean. True when side effects occurred and a compensation action is defined. |
| **compensation\_status** | `not_required` / `pending` / `completed` / `failed` / `not_possible`. |
| **recovery\_status** | `not_required` / `open` / `resolved`. Set to `open` on `partially_succeeded` outcomes where the partial completion has not been resolved. Remains `open` until explicitly resolved by the user or by an approved Proposal. `resolved` means one of: compensation completed successfully; a user-approved recovery action completed; the user explicitly accepted the partial outcome with no further action; or the system closed the item with recorded rationale because no further recovery is possible. A Proposal is generated when user action is required to resolve — the execution outcome record is the authoritative state; the Proposal is the resolution vehicle. |
| **proposal\_id** | Reference to the Proposal that authorized this command attempt, if applicable. Null for autonomous actions. |

Execution outcome records carry provenance source types from the taxonomy defined in the Provenance Model (`execution_succeeded`, `execution_failed`, `execution_partial`, etc.). This makes execution history queryable by outcome — NAVI can reason about patterns of degraded behavior, persistent failures, and retry costs.

---

#### C. Compose Failure Semantics

Compose commands are not naively transactional. Many side effects that occur mid-composition cannot be undone. The failure model therefore classifies every command and plugin operation by reversibility, and applies compensation-first semantics rather than assuming rollback is possible.

**Reversibility Classes**

Every command in a Compose sequence — and every plugin operation — carries a declared reversibility class. This is part of the command contract and the plugin contract.

| Class | Definition | Failure Handling |
| :---- | :---- | :---- |
| **Reversible Internal** | Mutates only NAVI's internal World Model. No external side effects have occurred. | Full rollback of internal state on failure. Compensation not required. |
| **Compensable External** | Has crossed a system boundary but a defined compensation action exists (e.g., a created draft can be deleted, a scheduled task can be cancelled). | Do not roll back. Execute defined compensation action. Record compensation status in outcome record. |
| **Irreversible** | Side effects cannot be undone and no compensation action exists (e.g., email sent, webhook fired, external API mutated without undo endpoint, delegated task accepted by a third party). | Do not fake rollback. Surface partial completion explicitly. Write recovery state to World Model. Generate a Proposal for any user-actionable recovery path. |

**Compose Failure Decision Rules**

1. If failure occurs before any external side effects: roll back all internal mutations, write a single `rejected_pre_execution` or `failed` outcome record, return constraint or failure context to Decide.
2. If failure occurs after reversible-only steps: roll back all completed reversible steps, treat as clean failure.
3. If failure occurs after one or more compensable steps: do not roll back. Execute compensation actions for completed compensable steps in reverse order. Record each compensation outcome.
4. If failure occurs after any irreversible step: compensation is not possible for those steps. Surface the partial completion state explicitly — what succeeded, what failed, what cannot be undone. Write recovery state to World Model. Generate a Proposal for any user-actionable recovery if one exists.
5. A Compose command with `partially_succeeded` outcome has its `recovery_status` set to `open`. It is never silently closed. The execution outcome record is the authoritative home for this state. A Proposal is generated when user action is required to resolve the partial completion. Recovery status remains `open` until the user resolves it directly or an approved Proposal closes it. Only then is `recovery_status` set to `resolved`.

---

#### D. Isolation Boundaries

Failures in the Capability Layer must not propagate upward into the Cognitive Layer or corrupt the World Model.

**Plugin Isolation**

- Plugins execute in isolated processes. A plugin crash cannot block the Conscious loop or write directly to the World Model.
- All plugin invocations return a structured result object. If the plugin process crashes or times out, the invoking layer receives a structured failure result — not an unhandled exception.
- The World Model is never modified by a plugin directly. Plugins return results; the Cognitive Layer decides whether and how to apply those results. A crashed plugin cannot produce a partial or corrupted write.
- A plugin that crashes during a Compose step is treated as an `execution_failed` outcome for that step. Compose failure semantics apply from that point forward.
- Failed plugins are marked degraded in the capability registry. The Conscious Process treats degraded plugins as unavailable until recovery is confirmed.

**Connector Isolation**

- Connector failures are caught at the Connector boundary. A failing Connector returns a structured error to the Effect Command layer — it does not propagate raw exceptions.
- Transient failures trigger retry with exponential backoff up to a per-connector retry budget (open question: specific budget values per connector class).
- Persistent failures open a circuit breaker. The Connector is marked unavailable. The circuit breaker attempts periodic recovery probes; it does not flood a failing external system with retries.
- Capabilities dependent on an unavailable Connector are surfaced as degraded, not silently absent.

**World Model Integrity**

- No process other than the Cognitive Layer may write to the World Model.
- Partial writes are not permitted. A mutation either completes fully or does not occur. Atomicity at the entity level is required.
- If a contradiction is detected in the World Model (two entities carrying conflicting state), the contradiction is recorded in History and the Subconscious is triggered for reconciliation. The World Model does not silently accept the contradiction.

---

#### E. User-Visible Degradation Policy

Failures have four visibility levels. The system selects a visibility level based on failure class, user impact, and whether the failure is recoverable.

| Level | When Applied | What the User Sees |
| :---- | :---- | :---- |
| **Silent Retry** | Transient connector failure. Retry is in progress. User impact is only latency. No side effects have occurred. | Nothing. NAVI retries transparently. If retry budget exhausts, escalates to Advisory. |
| **Advisory** | Capability is degraded but NAVI has partially or fully completed the task through an alternative path. Or: a non-critical background operation has failed and the user should be aware. | Non-blocking notification surfaced through Experience Layer. No action required from user. |
| **Blocking** | The requested action could not complete and there is no autonomous recovery path. User decision is required to proceed, retry, or abandon. | Blocking surface through Experience Layer. The failed or partial outcome is described clearly. Available recovery paths are presented. Generates a Proposal if a recovery action is available. |
| **Deferred Recovery Proposal** | An irreversible partial execution has left the system in a state that requires user-directed recovery, but the user is not currently in an active interaction. | A Proposal is generated and enters the Queue with priority `queued`. The user sees it at next interaction. The partial completion state is preserved in History and the World Model until resolved. |

The Experience Layer controls presentation. The Failure Model defines when each level applies — not how it looks.

**What NAVI never does:**

- Report success for a partially completed action.
- Silently discard a failed command without a History record.
- Retry an irreversible command without explicit user confirmation.
- Allow a plugin crash to surface as a generic error with no context.
- Leave a partially executed Compose in an unresolved state indefinitely.

---

### 3.4 Autonomy Model

The Autonomy Model is the user-facing configuration surface that controls how much NAVI acts without asking. It does not change what NAVI is permitted to do — Governance determines that. It changes how much NAVI does autonomously within what is already permitted.

**Anchor principle: Autonomy controls the threshold for autonomous action within Governance bounds. It never expands what is permitted. It never bypasses Governance hard floors.**

---

#### A. Structure

The model has two layers:

1. **Global preset** — A single top-level setting that establishes a baseline across all dimensions. Three options: Conservative, Balanced, High Autonomy. Default for new users is Balanced.
2. **Per-domain overrides** — Domain-specific settings that override the global preset for a defined scope. Overrides are optional. Without them, the global preset applies everywhere.

This gives users a simple control surface by default, and precise control when they want it. NAVI ships with Balanced and no overrides.

---

#### B. Autonomy Dimensions

Four dimensions govern autonomous behavior. Each dimension is independently configurable via per-domain overrides. The global preset sets all four simultaneously.

| Dimension | What It Controls |
| :---- | :---- |
| **Execution Threshold** | The risk level below which NAVI executes actions without asking. Higher autonomy raises this threshold — NAVI acts on more things without confirmation. Lower autonomy lowers it — more actions generate Proposals before executing. |
| **Insight Surfacing Aggressiveness** | How readily NAVI surfaces insights, patterns, and observations from the Subconscious back through the Experience Layer. Range from corrections-only (only surface high-confidence contradictions with material consequences via the urgent interruption path) to proactive (surface relevant insights and context through normal advisory delivery even when no immediate action is blocked). Autonomy settings do not change the strict criteria for urgent interruptions defined in Inter-Process Communication. |
| **Memory Promotion Sensitivity** | How aggressively observations are advanced through the reflection pipeline toward potential Knowledge or Memory promotion. Higher sensitivity means faster learning and richer context, at the cost of more noise and more inferred state accumulating without explicit confirmation. Lower sensitivity means slower, higher-confidence promotion. Shallow Reflections generate promotion candidates; Consolidation and Deep Reflection decide actual Knowledge or Memory promotion. |
| **Plugin Invocation Authority** | Whether plugins can be invoked autonomously when their trigger conditions are met, or whether each invocation requires per-invocation user confirmation. Always subject to Governance hard floors and plugin trust / permission requirements. May be set globally or per plugin trust tier. |

---

#### C. Global Presets

| Dimension | Conservative | Balanced | High Autonomy |
| :---- | :---- | :---- | :---- |
| **Execution Threshold** | Low. Most actions that carry any external side effect generate a Proposal before executing. Internally reversible actions proceed autonomously. | Moderate. Actions with well-understood, low-risk side effects proceed autonomously. Actions with ambiguous scope, significant external effect, or irreversibility generate Proposals. | High. NAVI executes broadly unless the action is in a proposal-required category (see Hard Floors) or the risk assessment explicitly flags it. |
| **Insight Surfacing Aggressiveness** | Corrections only. Subconscious surfaces findings only when the active Conscious Process is using materially wrong information that meets the urgent interruption criteria. No proactive advisory insights. | Balanced. Corrections always surface via the urgent interruption path. Additional relevant insights surface through normal advisory delivery when confidence is high and the user is not mid-task. | Proactive. NAVI actively surfaces patterns, observations, and relevant context through advisory Experience-layer delivery. User may see frequent insights, but the strict urgent interruption criteria remain unchanged. |
| **Memory Promotion Sensitivity** | Low. Observations accumulate in History. Advancement toward Knowledge or Memory promotion requires high confidence and multiple independent reinforcements. | Moderate. Clear patterns and significant interactions advance toward promotion at standard confidence thresholds. Ambiguous observations stay in History longer. | High. NAVI advances observations aggressively. Learning is fast. Inferred state accumulates quickly and may require more frequent Deep Reflection reconciliation. |
| **Plugin Invocation Authority** | Confirmation required per invocation for all plugins regardless of trust tier. | Trusted plugins (explicitly approved by owner and granted required permissions) invoke autonomously within Governance bounds. Unverified plugins require per-invocation confirmation. | All installed plugins that meet Governance hard floors and trust / permission requirements may invoke autonomously when their trigger conditions are met. New plugin installs still require owner confirmation and permission grant. |

---

#### D. Per-Domain Overrides

Per-domain overrides let the user apply a different autonomy level to a specific domain without changing the global preset. Domains map to the command types and capability scopes most relevant to that area of behavior.

| Domain | Scope | Example Override |
| :---- | :---- | :---- |
| **Messaging** | Send commands targeting communication channels — email, SMS, chat. | "Auto-send routine replies; always confirm new outbound emails to contacts I haven't messaged before." |
| **Scheduling** | Schedule commands and calendar-related Invoke commands. | "Auto-schedule based on my calendar rules; always confirm when the invite includes external participants." |
| **Coding and Execution** | Invoke, Delegate, and Compose commands in the Coder role. File and repository mutations. | "High autonomy in my local dev environment; require confirmation for any push to remote." |
| **Memory and Learning** | Memory promotion sensitivity and Knowledge supersede authority. | "Conservative memory promotion — I want to review what you've learned about me before it sticks." |
| **Plugins** | Plugin invocation authority, scoped to specific plugins or plugin categories. | "All Domain Plugins autonomous; Agentic Plugins always require confirmation." |

Per-domain overrides are stored in Configuration as Owner-set state. Changing them follows the same governance path as any other explicit Configuration change — they cannot be silently modified by any reflection process.

---

#### E. Hard Floors

Autonomy settings operate within Governance bounds. No autonomy level — including High Autonomy — bypasses the following:

| Floor | Rule |
| :---- | :---- |
| **System-tier policy** | Immutable. Safety, ethical, and permission constraints defined at the System tier are never affected by autonomy settings. |
| **Owner-set constraints** | Explicit user-set constraints are never bypassed autonomously. If the user has stated "never send emails without confirmation," that constraint holds regardless of the Messaging domain autonomy setting. |
| **Proposal-required categories** | Certain action categories always require a Proposal regardless of autonomy level. These are defined by Governance, not by the user: Hard Delete, Tombstone, Forget, Owner-set Configuration changes, Owner-set Priority changes, and any action flagged as irreversible by the Failure Model. Autonomy settings cannot remove these from the Proposal-required list. |
| **Irreversible commands** | The Failure Model's Irreversible reversibility class always generates a Proposal before first execution. High Autonomy does not change this — it only affects the threshold for reversible and compensable actions. |
| **Risk Assessment override** | If Validate/Govern's Risk Assessment check produces a high-severity flag, the action generates a Proposal regardless of the autonomy setting. Autonomy raises the threshold for what triggers Risk Assessment flags — it does not suppress them once raised. |

---

#### F. Autonomy and the Proposal Queue

Autonomy settings directly affect the rate at which Proposals are generated, but not the mechanics of how Proposals work once created.

- **Conservative** produces more Proposals, more frequently. The queue is more active. More actions pause for user input.
- **High Autonomy** produces fewer Proposals. Actions execute and the queue is quieter. But when a Proposal is generated — because a hard floor was hit or Risk Assessment flagged the action — it carries the same weight and follows the same resolution path as any other Proposal.

The Proposal Queue is not a mechanism that autonomy can disable. It is the channel through which Governance communicates required decisions to the user. Autonomy determines how often that channel is used; it does not close it.

---

#### G. Autonomy Preset Changes

Changing the global preset is an Owner-set Configuration change and is applied immediately. NAVI does not require a reflection cycle to honor a new preset.

Shifting from High Autonomy to Conservative does not retroactively undo actions already taken. It affects the threshold for future actions only.

If the user has never set a preference, Balanced applies. If the user's stated preferences conflict with per-domain overrides (e.g., global Conservative but Coding domain set to High), the per-domain override wins for that domain. All other domains remain at Conservative.
