# NAVI Project System — V1 Comprehensive Spec

> [!NOTE]
> Part of the [NAVI Systems Map](../architecture/navi-systems-map.md).


## 1. Purpose

A **Project** is NAVI's canonical work container for a named, durable body of work.

A Project exists to give NAVI a stable boundary for:

* context
* objectives
* artifacts
* chats
* project-specific knowledge
* scoped operating instructions
* scoped governance/autonomy preferences
* scoped capabilities and resource access
* shared collaboration state

The Project is the unit NAVI uses to understand "what work am I doing right now, for whom, with what context, under what constraints, and toward which outcomes?"

## 2. Core Definition

**Definition**

A Project is a first-class World Model entity representing an ongoing or completed body of work with its own identity, scope, state, relationships, memory boundary, and operating policy.

**Decision**

* A Project is a **World Entity**, not just UI state and not just configuration.
* A Project may own or reference chats, artifacts, tasks/objectives, decisions, risks, proposals, events, contacts, and project-scoped knowledge.
* A Project may also carry project-scoped control state, but that control state is subordinate to system and owner governance.

This fits NAVI's existing architecture, where the World Model is the source of truth for structured state, while Capability handles execution and Experience handles presentation.

### 2.1 Attribute Model

Projects follow NAVI's universal primitive: **everything has attributes; everything is composed of attributes.**

A Project is modeled as:

**`Project = Entity + Attribute graph + typed relations`**

All fields in the Project schema (Section 5) are expressed as Attributes or relationships to Attribute-backed entities. The YAML schema in Section 5 is **illustrative only** — it shows the logical shape of a Project for human comprehension. The normative model is the Attribute graph.

Rules:

* Every Project field is an Attribute with provenance, confidence, and mutation history per the Provenance Model.
* Relationships between Projects and other entities (chats, artifacts, objectives, etc.) are modeled using NAVI's Relationship Layer with type, confidence, recency, and provenance metadata.
* Project Attributes follow the same state classifications used everywhere in NAVI: Explicit, Owner-set, Inferred, and Derived.
* No Project field is special-cased outside the Attribute system. If a new field is added to a Project, it is added as an Attribute — not as a hardcoded schema column.

This ensures Projects are not a structurally exceptional entity type. They compose, query, decay, and audit the same way every other World Model entity does.

## 3. Relationship to Existing NAVI Concepts

### 3.1 Project vs Workspace

A **Workspace** is the environment/trust/resource boundary.

A **Project** is the work boundary.

V1 rule:

* Every Project is bound to exactly **one** Workspace.
* A Workspace may contain zero or many Projects.
* A chat may exist in a Workspace without belonging to a Project.
* Cross-workspace Projects are out of scope for V1.

Why:

* Workspace governs access to repos, files, connectors, and environment surfaces.
* Project governs what body of work NAVI is operating on inside that environment.

See **Section 22** for the Workspace stub definition.

### 3.2 Project vs Chat

A chat is a conversational thread.

A project is the persistent container those threads belong to.

Rules:

* A Project can contain many chats.
* A chat belongs to at most one Project at a time.
* Moving a chat into a Project rebinds it to that Project's instructions, retrieval scope, and policy **from that point forward**.
* Historical chat content can be indexed into project retrieval with provenance; it should not be blindly treated as curated project knowledge.

### 3.3 Project vs Artifact

Artifacts are first-class outputs NAVI creates or manages. Projects are the containers that group artifacts into a work context. NAVI already treats Artifacts as first-class entities, so Projects relate to them — they do not replace them.

Rules:

* Artifacts may be project-bound or unbound.
* Project-bound artifacts inherit project metadata and access policy.
* NAVI Library remains the cross-project artifact manager surface; Project views are filtered slices of NAVI Library.

### 3.4 Project vs Skill / Plugin / Connector

Projects do not define new execution primitives. Skills remain governed capabilities in the Capability Layer, and Plugins/Connectors remain integration/runtime surfaces. Projects only scope **which** capabilities are relevant, preferred, allowed, or pinned for a given body of work.

### 3.5 Project vs Proposal

A Proposal is a first-class authorization object in the World Model. Projects do not replace the Proposal Queue; instead, proposals can be associated with a Project so approvals, declines, and pending decisions are visible in project context.

### 3.6 Project vs Configuration

NAVI has Configuration as a core Control Entity. Project-level behavioral settings — instructions, capability policy, autonomy overrides — are modeled as **project-scoped Configuration entities** or Configuration-backed Attributes.

The inline fields shown in the Project schema (Section 5) are a **logical view** for human comprehension. The storage truth is Configuration entities scoped to the Project, following the same Explicit/Owner-set/Inferred distinction used in global Configuration.

This prevents a parallel configuration system. Project settings inherit, override, and audit identically to global Configuration — they just carry a `project_id` scope.

### 3.7 Project vs Priorities

NAVI has Priorities as a core Control Entity representing goals and intentions. Project Objectives and Milestones are **not identical to Priorities**, but they emit and inform priority signals:

* Active project objectives generate project-scoped priority signals.
* These signals feed into the global Priorities resolution during Contextualize, alongside stated and observed global priorities.
* A project objective can influence which global priority is most relevant in context, but it does not replace or override stated Owner-set Priorities.

### 3.8 Project vs History, Knowledge, and Memories

Projects do not create parallel memory, knowledge, or history systems. They scope existing ones:

* **Project history** = a scoped view of the existing History entity, filtered to interactions and execution outcomes that occurred within the project.
* **Project memory** = scoped retrieval over the existing Memories entity, plus project-linked state (objectives, decisions, etc.). Not a separate memory store.
* **Knowledge pack** = a curated collection of existing Knowledge entities and linked Artifacts, grouped under the project for retrieval purposes. Knowledge entities in a pack carry the same provenance, confidence, and lifecycle as any other Knowledge node.

No parallel memory system exists. The Project is a retrieval boundary, not a storage boundary.

## 4. Locked V1 Decisions

These should be treated as locked unless there is a strong architectural reason to reopen them.

1. **Project is a first-class World Entity, composed of Attributes and typed relations.**
2. **Project is bound to exactly one Workspace in V1.**
3. **Project owns scope, not execution.**
4. **Project memory is scoped retrieval over existing entities; it is not a parallel store.**
5. **Project instructions override personal defaults inside the project, but cannot override system policy or owner hard constraints.**
6. **Project context must be retrieval-based, not prompt-dump-based.**
7. **Shared projects are isolated by default from unrelated personal/project context.**
8. **Projects must support both personal and collaborative modes.**
9. **Projects must store explicit work state: objectives, decisions, risks, status.**
10. **Project switching must be explicit or high-confidence only; otherwise NAVI asks.**
11. **Project behavioral settings are Configuration entities scoped to the project, not a separate config system.**
12. **Project objectives feed into Priorities but are not identical to them.**

## 5. Project Entity Schema (Illustrative)

This schema is **illustrative only**. The normative model is `Project = Entity + Attribute graph + typed relations`. All fields below are Attributes or references to Attribute-backed entities.

```yaml
project_id: proj_xxx
workspace_id: ws_xxx
owner_contact_id: contact_xxx

identity:
  title: "NAVI Project System"
  slug: "navi-project-system"
  description: "Define projects as a first-class entity for NAVI"
  type_tags: ["product", "engineering"]    # extensible typed tags, not a closed enum
  tags: ["core-platform", "navi"]

status:
  lifecycle: draft|active|on_hold|completed|archived|deleted
  health: on_track|at_risk|blocked|unknown
  priority: low|medium|high|critical

scope:
  visibility: private|shared|org
  memory_mode: blended|isolated
  sharing_mode: personal|collaborative
  default_chat_visibility: shared|private
  default_artifact_visibility: shared|private

# --- The following blocks are logical views over project-scoped Configuration entities ---

instructions:                              # → project-scoped Configuration
  project_instructions: "Canonical project rules and desired behavior"
  output_conventions: []
  terminology: []
  quality_bar: []
  preferred_persona: optional
  preferred_models: []
  forbidden_patterns: []

capability_policy:                         # → project-scoped Configuration
  pinned_skill_ids: []
  preferred_skill_ids: []
  denied_skill_ids: []
  allowed_connector_ids: []
  sandbox_profile_id: optional

autonomy:                                  # → project-scoped Configuration
  preset_override: optional
  domain_overrides: {}

# --- Knowledge and work state: references to existing entity types ---

knowledge:
  knowledge_pack_ids: []                   # → Knowledge entity references
  pinned_source_ids: []                    # → Artifact / Knowledge references
  decision_ids: []                         # → Decision entity references
  glossary_ids: []                         # → Knowledge entity references

work_state:
  objective_ids: []                        # → Objective entities (Section 10)
  milestone_ids: []                        # → Milestone entities (Section 10)
  decision_log_ids: []                     # → Decision entities (Section 10)
  risk_ids: []                             # → Risk entities (Section 10)
  open_question_ids: []                    # → OpenQuestion entities (Section 10)
  proposal_ids: []                         # → Proposal entity references

resources:
  repo_ids: []
  folder_ids: []
  connector_bindings: []
  external_system_refs: []

collaboration:
  members:                                 # → Contact/User entities with role Attributes
    - contact_id: contact_xxx
      role: owner|editor|contributor|viewer
  shared_at: optional

relations:
  chat_ids: []
  artifact_ids: []
  event_ids: []
  contact_ids: []

provenance:
  created_at: timestamp
  updated_at: timestamp
  created_by: contact_xxx
  mutation_history: []                     # per Provenance Model
```

## 6. State Model

NAVI already distinguishes **Explicit**, **Owner-set**, **Inferred**, and **Derived** state. Projects follow that exactly.

### 6.1 Explicit / Owner-set project state

Examples:

* title, description, workspace binding
* visibility, memory mode
* project instructions
* pinned resources, pinned skills
* autonomy overrides
* member roles (via Contact role bindings)
* explicit objectives
* manual status changes

### 6.2 Inferred project state

Examples:

* likely next milestone
* likely stale objective
* inferred type tags
* inferred health
* inferred relevant contacts
* inferred active files/repos

### 6.3 Derived project state

Examples:

* project summary / brief
* latest status snapshot
* "what changed since last session"
* open blockers
* recommended next actions
* effective capability policy after inheritance
* compiled Project Runtime Profile

Derived state is never the source of truth. It is regenerated from explicit, inferred, and linked entities.

## 7. Inheritance and Scope Model

Projects behave as scoped policy/context layers.

Inheritance order for behavior inside a project:

**System policy**
→ **Owner constraints / global Configuration**
→ **Workspace policy**
→ **Project policy (project-scoped Configuration)**
→ **Chat-local instructions**
→ **Turn-level user request**

Project policy can narrow, specialize, or focus behavior. It cannot expand beyond system, owner, or workspace constraints.

This follows NAVI's existing governance model, where higher tiers remain authoritative and the most restrictive applicable constraint wins.

## 8. Cognitive Layer Integration

### 8.1 Conscious Process interaction

The active Project is consumed by the Conscious Process at multiple steps:

| Step | Project interaction |
|------|-------------------|
| **Perceive** | No direct project influence. Raw intake is project-agnostic. |
| **Interpret** | Project goals and active constraints may **bias** interpretation. Example: in an engineering project with an active "reduce latency" objective, an ambiguous user request about "making it faster" is interpreted in that context rather than as a generic question. |
| **Contextualize** | **Primary injection point.** Project Runtime Profile, objectives, recent decisions, blockers, knowledge pack, and scoped retrieval results are assembled here. This is where the active project shapes what entities, history, and knowledge are pulled into the reasoning context. |
| **Decide** | Project capability policy and autonomy overrides influence which Skills, Connectors, and execution paths are preferred, allowed, or denied. Project objectives inform priority weighting when multiple valid actions compete. |
| **Validate/Govern** | Project-scoped Configuration participates in the standard validation order (Section 7 inheritance). Project autonomy overrides affect threshold checks. |
| **Execute** | No special project behavior. Commands execute through normal Capability Layer paths. |
| **Reflect** | Immediate reflection may tag observations with `project_id` for scoped subconscious processing. |

### 8.2 Project switching

When the active project changes (explicitly by user request, or by high-confidence resolution per Section 17), NAVI must:

1. **Re-contextualize** before processing any further turns. The new project's Runtime Profile, instructions, capability policy, and autonomy overrides replace the prior project's.
2. Write a handoff marker to History noting the project transition, so both the source and destination project histories remain coherent.
3. Do not carry forward prior project context into the new project's reasoning scope unless the user explicitly requests cross-project reference.

Project switching is a Contextualize-level operation. It does not restart the Conscious Process — it re-runs Contextualize with the new project's scope, then proceeds to Decide.

### 8.3 Subconscious process interaction

Reflection processes can operate both globally and project-scoped. When a reflection payload from the Conscious Process carries a `project_id`, the Subconscious routes it appropriately:

| Tier | Project-scoped behavior | Mutation permissions |
|------|------------------------|---------------------|
| **Shallow Reflection** | Summarize recent project interactions. Flag stale objectives. Update project-local entity attributes (recency, confidence). | Same as global: may update attributes, create relationships. May not delete or reclassify entities. May not modify Configuration or Priorities. |
| **Consolidation** | Promote reusable project knowledge — e.g., elevate a pattern observed across multiple project chats into a Knowledge entity linked to the project. Strengthen or weaken project-internal relationships. | Same as global: may update/create entities and relationships. May deprecate Knowledge. May update inferred Configuration within scope. Must generate a Proposal for changes affecting explicit/Owner-set behavior. |
| **Deep Reflection** | May **propose** mutations to project objectives, risks, status, health, and decisions. May propose project-scoped Configuration changes. | Full mutation access on inferred state. May *propose* changes to Owner-set project state (objectives, explicit decisions, project instructions) but **cannot apply them directly** — must generate a Proposal. Owner-tier governance constraints always require user confirmation. |

Project objectives are **explicit project state**. Deep Reflection may propose changes to them (e.g., "Objective X appears completed based on recent activity"), but the proposal must go through the Proposal Queue for owner resolution.

## 9. Context and Memory Model

### 9.1 Project context inputs

When a Project is active, NAVI assembles context from:

* project instructions (project-scoped Configuration)
* project summary / brief (Derived)
* project objectives and milestones (Objective/Milestone entities)
* project decision log (Decision entities)
* project risks and blockers (Risk entities)
* project chats (scoped History view)
* project artifacts (Artifact entity references)
* project knowledge pack (Knowledge entity collection)
* project-bound contacts/events (Contact/Event entity references)
* workspace-bound resources attached to the project
* project capability policy (project-scoped Configuration)

### 9.2 Memory modes

V1 supports two project memory modes:

**blended**

* NAVI may use project context plus relevant personal/global memory.
* Best for private personal projects.

**isolated**

* NAVI may use only project-internal chats, artifacts, and knowledge, plus workspace-allowed resources.
* Best for shared/team/sensitive projects.

Default:

* private personal project → `blended`
* shared/collaborative project → `isolated`

### 9.3 Curated knowledge vs raw chat history

Raw chat history is not the same thing as project knowledge.

Project maintains three distinct layers, each backed by existing NAVI entities:

* **Conversation history**: scoped view of History — raw threads and transcripts.
* **Knowledge pack**: curated collection of Knowledge entities, promoted summaries, specs, and Decision entities linked to the project.
* **Runtime brief**: compact Derived project summary for fast startup context (part of the Project Runtime Profile).

NAVI supports promotion from chat history into durable project knowledge through reflection (Consolidation promoting a Knowledge entity) or explicit user action (user marking content as project knowledge).

### 9.4 Retrieval rule

Project context must be retrieval-first:

* Inject a compact project brief and critical constraints at Contextualize.
* Retrieve the rest on demand based on the current turn's relevance signals.

Do not stuff all project chats/files into prompt context. NAVI's architecture points toward retrieval, dynamic discovery, and bounded runtime surfaces.

## 10. Work State Entities (Stub Schemas)

These entities are substantial enough to warrant their own dedicated spec. The stub schemas below define the minimum viable shape for V1 implementation within the Project System. Each is a first-class World Entity composed of Attributes per NAVI's universal primitive.

### 10.1 Objective

```yaml
objective_id: obj_xxx
project_id: proj_xxx

identity:
  title: "Reduce API response latency below 200ms"
  description: "..."
  type_tags: ["performance", "engineering"]

state:
  status: active|completed|cancelled|deferred
  priority: low|medium|high|critical
  target_date: optional timestamp

relations:
  milestone_ids: []
  decision_ids: []
  risk_ids: []
  artifact_ids: []

mutation_authority:
  # Objectives are explicit project state.
  # Owner and Editors may mutate directly.
  # Deep Reflection may propose changes via Proposal Queue.

provenance:
  created_at: timestamp
  updated_at: timestamp
  created_by: contact_xxx
  mutation_history: []
```

### 10.2 Milestone

```yaml
milestone_id: ms_xxx
project_id: proj_xxx
objective_id: optional obj_xxx    # may be independent of a specific objective

identity:
  title: "Alpha release"
  description: "..."

state:
  status: upcoming|active|completed|missed|cancelled
  target_date: optional timestamp
  completed_at: optional timestamp

relations:
  objective_ids: []
  artifact_ids: []
  decision_ids: []

mutation_authority:
  # Same as Objective: explicit project state.
  # Owner/Editor direct mutation. Deep Reflection proposes.

provenance:
  created_at: timestamp
  updated_at: timestamp
  created_by: contact_xxx
  mutation_history: []
```

### 10.3 Decision

```yaml
decision_id: dec_xxx
project_id: proj_xxx

identity:
  title: "Use JWT for service-to-service auth"
  description: "..."
  rationale: "..."

state:
  status: proposed|accepted|superseded|reversed
  decided_at: optional timestamp
  decided_by: optional contact_xxx

relations:
  objective_ids: []
  risk_ids: []
  open_question_ids: []       # question this decision resolved
  superseded_by: optional dec_xxx
  artifact_ids: []            # supporting documents

mutation_authority:
  # Decisions are explicit project state once accepted.
  # Owner/Editor may accept, supersede, or reverse.
  # Deep Reflection may propose supersession or reversal.

provenance:
  created_at: timestamp
  updated_at: timestamp
  created_by: contact_xxx
  mutation_history: []
```

### 10.4 Risk

```yaml
risk_id: risk_xxx
project_id: proj_xxx

identity:
  title: "Upstream API deprecation"
  description: "..."

state:
  status: open|mitigated|accepted|closed
  severity: low|medium|high|critical
  likelihood: low|medium|high

relations:
  objective_ids: []
  decision_ids: []          # decisions made to address this risk
  mitigation_artifact_ids: []

mutation_authority:
  # Risks may be inferred by reflection or explicitly created.
  # Inferred risks: reflection may update severity/likelihood.
  # Explicit risks: Owner/Editor mutate; Deep Reflection proposes.

provenance:
  created_at: timestamp
  updated_at: timestamp
  created_by: contact_xxx | shallow_reflection | consolidation | deep_reflection
  mutation_history: []
```

### 10.5 Open Question

```yaml
question_id: oq_xxx
project_id: proj_xxx

identity:
  title: "Should we support multi-workspace projects in V1?"
  description: "..."
  context: "..."

state:
  status: open|resolved|deferred|abandoned
  resolved_by_decision_id: optional dec_xxx

relations:
  objective_ids: []
  risk_ids: []

mutation_authority:
  # Open questions are explicit project state.
  # Owner/Editor may resolve, defer, or abandon.
  # Deep Reflection may propose resolution.

provenance:
  created_at: timestamp
  updated_at: timestamp
  created_by: contact_xxx
  mutation_history: []
```

### 10.6 Status Update

```yaml
status_update_id: su_xxx
project_id: proj_xxx

identity:
  title: "Week 12 Update"
  summary: "..."
  period_start: timestamp
  period_end: timestamp

content:
  accomplishments: []
  blockers: []
  next_steps: []
  health_assessment: on_track|at_risk|blocked

relations:
  objective_ids: []
  milestone_ids: []
  decision_ids: []
  risk_ids: []

mutation_authority:
  # Status updates are explicit once published.
  # May be generated by NAVI (Derived) and promoted to Explicit on user confirmation.
  # Owner/Editor/Contributor may create.

provenance:
  created_at: timestamp
  updated_at: timestamp
  created_by: contact_xxx | consolidation
  mutation_history: []
```

## 11. Project Runtime Profile

At session start or project switch, NAVI compiles a **Project Runtime Profile** (Derived state).

Fields:

* project identity
* workspace binding
* effective visibility/memory mode
* top objectives (from Objective entities)
* latest decisions (from Decision entities)
* current blockers (from Risk entities, Open Question entities)
* pinned artifacts and sources
* preferred skills/connectors (from project-scoped Configuration)
* autonomy overrides (from project-scoped Configuration)
* required approvals (from Proposal Queue, filtered by project_id)
* recommended next actions (Derived)
* priority signals emitted to global Priorities

This is analogous to the skill snapshot concept already used in NAVI's capability system, but specialized for work context.

## 12. Artifact Behavior

### 12.1 Project-bound artifacts

When an artifact is created inside a project, it automatically inherits:

* project_id (relationship)
* workspace_id (inherited from project)
* visibility defaults (from project-scoped Configuration)
* relevant output conventions (from project instructions)
* provenance linking to the active project

### 12.2 Artifact movement

Artifacts may be:

* added to a project
* removed from a project
* duplicated into another project
* forked into another project

Rules:

* Moving changes default retrieval scope.
* Duplication preserves source provenance.
* Forks create a new artifact lineage branch.

### 12.3 NAVI Library integration

NAVI Library is the global artifact manager. Projects are a core facet:

* filter by project
* create from project
* attach artifact to project
* see "unassigned artifacts"

## 13. Chat Behavior

### 13.1 New chat in project

A new project chat inherits:

* project instructions (project-scoped Configuration)
* project memory mode
* project knowledge retrieval scope
* project capability policy (project-scoped Configuration)
* project workspace binding

### 13.2 Moving chat into project

When a chat is moved into a project:

* Future turns use project context.
* Prior turns remain historically intact.
* Prior transcript may be indexed into project retrieval with provenance marking the historical boundary.
* System should surface that the chat now follows project policy.

### 13.3 Shared project chat visibility

V1 policy:

* Personal projects: chats private by default.
* Collaborative/shared projects: chats shared by default, with optional private thread flag.

## 14. Capability Policy

Projects scope capability use without inventing new capability types.

Project capability policy (stored as project-scoped Configuration) can declare:

* pinned skills
* preferred skills
* denied skills
* preferred connectors
* denied connectors
* repo/folder allowlists
* model preferences
* sandbox profile
* execution ceilings
* approval rules

Skills remain governed, typed capabilities; the project only changes their relevance and bounds.

## 15. Governance and Autonomy

Projects can carry owner-set scoped autonomy overrides, but only within existing governance hard floors. This matches NAVI's Autonomy Model: autonomy changes thresholds within allowed bounds; it never bypasses hard floors.

### 15.1 Project-level autonomy overrides

Stored as project-scoped Configuration (Owner-set). Follows the per-domain override model defined in the Autonomy Model.

Allowed:

* execution threshold override
* plugin/skill invocation authority override
* messaging/scheduling/coding domain overrides
* project-specific confirmation policies

Not allowed:

* bypassing irreversible-action confirmation
* bypassing owner-set global hard constraints
* bypassing workspace/system policy

### 15.2 Project approvals

If a proposal touches project-scoped work, it should:

* appear in the global Proposal Queue
* be linked in the Project view
* carry project_id in metadata
* respect project visibility/access

## 16. Collaboration and Sharing

### 16.1 Personal

* single-owner default
* no shared members
* blended memory allowed
* private chats by default

### 16.2 Collaborative

* owner + editor + contributor + viewer roles
* isolated memory by default
* shared chats/artifacts by default
* project knowledge and instructions shared
* approvals governed by member role

### 16.3 Roles

Members are **Contacts or Users with project-scoped role Attributes**. No parallel identity model.

**Owner**

* full control
* can archive/delete
* can change workspace binding in draft stage only
* can manage members and policy

**Editor**

* can edit instructions, knowledge pack, objectives, decisions, and project settings
* cannot delete project unless delegated

**Contributor**

* can create chats, artifacts, notes, status updates
* can propose edits
* cannot change core policy

**Viewer**

* can read project contents allowed by visibility settings
* cannot mutate

## 17. Lifecycle

Project lifecycle states:

**draft** — being created; workspace binding still editable.

**active** — normal working state.

**on_hold** — paused but retained; reduced background activity.

**completed** — work finished; project remains queryable; new activity limited unless reopened.

**archived** — read-only by default; hidden from active lists; retrievable for audit/reference; sharing may be frozen or downgraded.

**deleted** — removed according to retention policy; tombstone retained where required per Data Lifecycle Model.

### 17.1 State transitions

* draft → active
* active → on_hold
* active → completed
* on_hold → active
* completed → active
* any non-deleted → archived
* archived → active (owner only)
* archived → deleted (policy permitting)

### 17.2 Archive behavior

On archive:

* Project leaves active routing pool.
* Autonomous background execution pauses unless explicitly allowed.
* Capabilities attached remain registered but project no longer selected by default.
* Pending proposals may remain visible but must be flagged against archived state.
* Per the Data Lifecycle Model, archived projects follow Archive semantics: hidden from active queries, preserved for audit and recovery.

## 18. Active Project Resolution

### 18.1 Selection order

When processing a turn, active project resolution should be:

1. Explicit project specified by user.
2. Current chat's bound project.
3. Explicit artifact/task/repo that maps to a single project.
4. Recent active project in current workspace, if confidence is high.
5. No project selected.

### 18.2 Ambiguity behavior

If confidence is low:

* Do not guess silently.
* Ask which project to use.
* Or proceed unscoped if the request does not require a project.

### 18.3 V1 boundary rule

No automatic cross-workspace project hopping in V1.

## 19. Failure Model

Projects explicitly inherit NAVI's Failure Model. The following project-specific failure cases are additive:

| Failure case | Class | Recovery path |
|-------------|-------|--------------|
| **Missing workspace** | Connector Unavailable (permanent sub-type) | Project marked degraded. User notified. Cannot execute workspace-dependent operations until workspace is restored or project is rebound (owner only, draft state only). |
| **Failed profile compilation** | Execution Failure | Proceed with partial profile. Missing sections noted as degraded. Retry compilation at next interaction. Advisory visibility level. |
| **Empty retrieval** | Not a failure — normal state | NAVI proceeds with available context. Does not fabricate missing context. May note to user that project knowledge is sparse. |
| **Stale bindings** | Contradiction / Stale-State Invalidation | Resource references (repos, folders, connectors) that no longer resolve are flagged. Shallow Reflection cleans up on next pass. Advisory notification. |
| **Inaccessible resources** | Permission Denial or Connector Unavailable | Per standard Failure Model. Capability marked degraded. User notified if blocking. |
| **Project switch during active execution** | Partial Execution | Current action completes or is cancelled before re-contextualization. No silent context swap mid-execution. |

NAVI never reports success when project context assembly was incomplete. If the Runtime Profile compiled with missing sections, that degradation is surfaced, not hidden.

## 20. UI / Product Surface

Minimum project surface should include:

### 20.1 Project Overview

* title, description, status
* active objectives
* recent decisions
* blockers/risks
* recent artifacts
* recent chats
* pending proposals
* linked resources
* project summary / "pick up where you left off"

### 20.2 Chats

* all project chats
* create new project chat
* move in / remove chat
* shared/private flags

### 20.3 Knowledge

* files (Artifact references)
* notes (Knowledge entities)
* links
* promoted decisions (Decision entities)
* glossary / terminology (Knowledge entities)
* curated sources

### 20.4 Work State

* objectives (Objective entities)
* milestones (Milestone entities)
* decisions (Decision entities)
* risks (Risk entities)
* open questions (OpenQuestion entities)
* status updates (StatusUpdate entities)

### 20.5 Artifacts

* project-filtered artifact list from NAVI Library
* create artifact in project
* attach existing artifact

### 20.6 Resources

* linked repos
* folders
* connectors
* environments
* external systems

### 20.7 Settings

* visibility
* memory mode
* instructions (project-scoped Configuration)
* autonomy overrides (project-scoped Configuration)
* capability policy (project-scoped Configuration)
* members (Contact role bindings)

## 21. API / System Behavior

### 21.1 Core operations

* create_project
* update_project
* archive_project
* restore_project
* delete_project
* add_chat_to_project
* remove_chat_from_project
* add_artifact_to_project
* remove_artifact_from_project
* bind_resource_to_project
* set_project_instructions
* set_project_memory_mode
* set_project_capability_policy
* set_project_autonomy_profile
* add_project_member (creates Contact role binding)
* remove_project_member (removes Contact role binding)
* compile_project_profile

### 21.2 Events (implementation guidance, not locked architecture)

The following events are recommended but treated as implementation guidance. The specific event surface may evolve during implementation.

* project.created
* project.updated
* project.status_changed
* project.member_added / removed
* project.instructions_updated
* project.knowledge_updated
* project.chat_attached / artifact_attached
* project.profile_compiled
* project.archived / deleted

### 21.3 Storage

Projects are stored as durable World Model entities with:

* Attribute graph (per Section 2.1)
* typed relations to linked entities
* provenance (per Provenance Model)
* mutation history (per Provenance Model)
* optional compiled Runtime Profile cache (Derived, regenerated on demand)

## 22. Workspace Stub Definition

Workspace is a first-class World Entity representing a trust and resource boundary. It contains resources, policies, connectors, and projects.

This is a **stub definition** for the Project System spec. Workspace warrants its own dedicated spec.

```yaml
workspace_id: ws_xxx
owner_contact_id: contact_xxx

identity:
  title: "Personal Workspace"
  description: "..."
  type_tags: ["personal"]

trust_boundary:
  visibility: private|shared|org
  access_policy: "..."

resources:
  repo_ids: []
  folder_ids: []
  connector_ids: []
  environment_ids: []

policy:
  workspace_instructions: optional
  capability_constraints: optional
  autonomy_floor: optional

relations:
  project_ids: []
  contact_ids: []

provenance:
  created_at: timestamp
  updated_at: timestamp
  created_by: contact_xxx
  mutation_history: []
```

**V1 rules:**

* Every Project binds to exactly one Workspace.
* A Workspace may contain zero or many Projects.
* A chat may exist in a Workspace without belonging to a Project.
* Workspace policy sits between Owner constraints and Project policy in the inheritance order.
* Cross-workspace operations are out of scope for V1.

## 23. Bootstrapping and Migration

V1 must support organizing existing chats and artifacts into projects. This is critical because users will have pre-project content that should be organizable.

### 23.1 Migration paths

**Create project from chat**

* User selects an existing chat and creates a project from it.
* Chat becomes the first project chat. Chat history is indexed into project retrieval with provenance marking the pre-project boundary.
* User sets project identity and initial instructions.

**Create project from artifact set**

* User selects one or more existing artifacts and creates a project to contain them.
* Artifacts are bound to the new project. Provenance records the binding event.

**Bulk assign existing chats/artifacts**

* User selects multiple chats and/or artifacts and assigns them to an existing project in a single operation.
* Each assignment is recorded with provenance.

**Suggested project groupings**

* NAVI may infer likely project groupings from patterns in existing chats, artifacts, and activity clusters.
* Suggestions are surfaced as recommendations, never auto-applied.
* V1 requires user confirmation before any grouping is enacted. No autonomous project creation from detected work streams in V1.

### 23.2 Migration provenance

All migration actions carry provenance:

* source: `user_input` (explicit assignment) or `inference` (suggested grouping, confirmed by user)
* timestamp of assignment
* boundary marker distinguishing pre-project content from in-project content

## 24. Retrieval and Scaling Rules

As projects grow, NAVI should not degrade into giant context blobs.

V1 scaling strategy:

* compact project brief (Derived, cached)
* retrieval index over project chats/artifacts/knowledge
* promotion pipeline for important chat content into Decision/Knowledge records (via Consolidation or explicit user action)
* periodic project summary regeneration (via Consolidation)
* staleness detection for outdated objectives and sources (via Shallow Reflection)

## 25. V1 Scope

V1 should include:

* Project entity (Attribute-based World Entity)
* Workspace stub entity
* one-workspace-per-project binding
* project chats
* project artifacts
* project knowledge pack (Knowledge entity collection)
* project instructions (project-scoped Configuration)
* project memory mode
* project capability policy (project-scoped Configuration)
* project autonomy overrides (project-scoped Configuration)
* active project routing (Section 18)
* Cognitive Layer integration (Section 8)
* project overview UI
* collaboration roles (Contact role bindings)
* work state entities: Objective, Milestone, Decision, Risk, OpenQuestion, StatusUpdate
* project-linked proposals
* archive / restore
* bootstrapping / migration paths
* Failure Model integration

## 26. Explicitly Deferred

Defer these to V2:

* multi-workspace projects
* nested subprojects
* project templates with automatic provisioning
* project branching/merging
* automatic project creation from detected work streams
* per-project billing/cost centers
* cross-project dependency graphs
* org-wide project analytics dashboards
* autonomous project splitting/merging by reflection
* dedicated Work State Entities spec (expanded schemas, lifecycle rules, cross-project queries)
* dedicated Workspace spec

## 27. Final Position

**Workspace = where NAVI is allowed to operate (trust/resource boundary).**
**Project = what body of work NAVI is operating on (work boundary).**
**Chat = one conversational thread inside that work.**
**Artifact = one durable output of that work.**

All are World Entities. All are composed of Attributes. All carry provenance. All follow NAVI's governance model.
