# NAVI Artifact System Specification

> [!NOTE]
> Part of the [NAVI Systems Map](../architecture/navi-systems-map.md).


**Status:** Proposed
**Version:** 2.0
**Primary surface:** NAVI Library (Experience Layer)
**Primary objective:** deliver a production-grade artifact platform optimized for agentic workflow, persistent work products, governed mutation, and future multi-artifact execution.

> All TypeScript type definitions, Go package layouts, and SQL schema fragments in this document are **illustrative, not normative**. Wire formats, storage schema, and interface signatures will be finalized during implementation.

## 1. Purpose

The Artifact System defines how NAVI creates, stores, renders, updates, versions, reopens, and governs user-facing work products such as documents, code, plans, structured outputs, datasets, and presentations.

The system is designed for:

* agent-generated outputs that must persist beyond a chat turn
* user-edited artifacts that remain revisable by NAVI
* multi-step workflows that produce intermediate and final work products
* workspace/project-aware organization, access control, and retrieval
* future export, sharing, sync, execution, and collaboration

## 2. Scope

### 2.1 In scope

V1 includes:

* artifact entity model (Attribute-composed)
* version history
* branching
* project/workspace-aware containment
* Library as Experience Layer artifact manager surface
* artifact creation from chat, user action, tools, and workflows
* user edits and NAVI edits
* renderer/editor registry
* persistence and rehydration
* chat linkage
* provenance and snapshot records
* governance integration with Proposal Queue and Autonomy Model
* export
* cross-entity relationships (History, Knowledge, Memory)
* Subconscious Process participation
* observability and testing

### 2.2 Out of scope for V1

Deferred:

* real-time multi-user coauthoring
* automatic merges across concurrent branches
* public publishing marketplace
* arbitrary bidirectional external sync for every artifact type
* comment threads and fine-grained review markup
* live collaborative presence
* full CRDT/OT editing

## 3. Design goals

The system must optimize for:

* durable work products
* explicit mutation history
* clean agentic workflow integration
* typed rendering and editing
* safe conflict handling
* recoverability after failure
* strong provenance
* extensibility without redesign

The system should prefer:

* command-driven state mutation over ad hoc writes
* immutable versions over mutable blobs
* explicit conflict signaling over silent merge
* traceable operations over hidden magic
* type-aware patching over one-size-fits-all text replacement

## 4. Architectural placement in NAVI

### 4.1 Layer mapping

The Artifact System spans NAVI's four layers:

**Experience Layer**

* chat affordances for artifact references
* artifact workspace UI
* NAVI Library — the artifact manager surface (browse, search, filter, inspect, compare, restore, branch, export, share, recover failed runs, reopen from prior conversation or project context)
* visual save, sync, error, and version state
* `ArtifactWorkspaceState` — session/UI state that lives here, not in the World Model

**Cognitive Layer**

* decides whether to create/update/branch/open artifacts (including artifact qualification heuristics)
* selects target artifact in ambiguous situations
* plans multi-step workflows around artifacts
* generates Proposals when governance thresholds require confirmation
* Reflect step emits artifact payloads for Subconscious processing

**World Model**

* stores Artifact, ArtifactVersion, ArtifactBranch, ArtifactReference, Snapshot, and related provenance objects as Attribute-composed entities
* Artifacts are World Model entity siblings to Knowledge, Memory, History, Events, Contacts, and other core entities

**Capability Layer**

* executes governed commands and Skills that read, create, or mutate artifacts indirectly
* never writes artifact durable truth directly
* Skills produce outputs through Capability; Cognitive decides whether to materialize outputs as artifacts

### 4.2 Command compilation

Artifact operations compile down to existing NAVI primitive commands. The mapping:

| Artifact Operation | Compiled Command(s) |
|-|-|
| `create_artifact` | `Compose(Create artifact, Create version, Create refs, Emit History)` |
| `replace_content` | `Compose(Query head, Validate, Create version, Update head, Emit History)` |
| `patch_content` | `Compose(Query head, Validate subtype-specific, Create version, Update head, Emit History)` |
| `append_content` | `Compose(Query head, Validate append-safe, Create version, Update head, Emit History)` |
| `rename_artifact` | `Update metadata + Emit History` |
| `update_metadata` | `Update metadata + Emit History` |
| `archive_artifact` | `Update lifecycle + Emit History` |
| `restore_artifact` | `Update lifecycle + Emit History` |
| `branch_artifact` | `Compose(Create branch, Create version from base, Emit History)` |
| `restore_version` | `Compose(Query target version, Create new version with content, Update head, Emit History)` |
| `export_artifact` | `Invoke export adapter + Proposal when required` |
| `share_artifact` | `Invoke share adapter + Proposal when required` |
| `sync_artifact` | `Compose(Invoke sync adapter, Validate, Create version if incoming, Emit History) + Proposal when required` |

Every artifact mutation emits a History record. Artifact mutation must remain version-aware because NAVI's update semantics already require conflict detection rather than silent overwrite.

## 5. Core concepts

### 5.1 Workspace

Top-level security and organizational boundary. Controls:

* visibility
* connector scope
* storage policy
* default governance
* membership and sharing rules

> **Dependency note:** This specification assumes a formal Workspace entity/container exists or will be defined. If no canonical Workspace spec exists yet, one must be produced before artifact implementation begins.

### 5.2 Project

Optional but first-class contextual container inside a workspace, defined in the [Project System Specification](project-system-v1.md). Controls:

* project instructions
* linked references and sources
* artifact collections
* project-level naming/publishing/export defaults
* default agent working context

### 5.3 Artifact

Durable user-meaningful work object with its own lifecycle, type, history, provenance, and supported operations. Artifacts are Attribute-composed World Model entities, sibling to Knowledge, Memory, History, Events, Contacts, and other core entities.

### 5.4 Version

Immutable saved state of an artifact. Stores either full attribute snapshots or attribute deltas.

### 5.5 Branch

Named line of evolution for an artifact. Used for alternate revisions, experiments, or conflict resolution.

### 5.6 Reference

Stable link between artifact and message, conversation, project, source, export, share, or external system.

### 5.7 Snapshot

Frozen context record attached to a version. Captures the state of surrounding context at version creation time. Snapshot types:

* `project_context_snapshot` — project instructions and linked sources at the moment of version creation
* `policy_snapshot` — workspace governance/autonomy policy in effect
* `source_snapshot` — revisions of source material used to produce the version
* `artifact_input_snapshot` — other artifact versions consumed as inputs

Snapshots are immutable once created. Versions reference snapshots for provenance.

## 6. Artifact qualification policy

> This section describes **Cognitive Layer heuristics** for artifact creation decisions. It is not part of the core domain model.

### 6.1 When content becomes an artifact

The Cognitive Layer should create an artifact when at least two of these are true:

* the content is self-contained
* the user asked for a deliverable
* the content is likely to be edited
* the content is likely to be reused
* the content is likely to be shared/exported
* the content has a typed structure
* a tool or workflow produced it
* it is substantial enough to justify persistence
* it is an intermediate or final output of a multi-step workflow

### 6.2 Automatic creation triggers

Automatic artifact creation is allowed when:

* the user asks for a doc, report, plan, spec, deck, table, app, or code asset
* the response crosses the artifact qualification threshold
* a tool or Skill returns a file-like or structured work product
* a long-running workflow begins and should expose progress in a draft artifact
* NAVI predicts the user will revise or export the result

### 6.3 Manual promotion

Any chat response may be promoted into an artifact by:

* explicit user command
* explicit NAVI recommendation with user acceptance
* internal workflow policy when a run needs durable output

## 7. Artifact types

### 7.1 V1 types

V1 supports four canonical types:

**document**

* markdown, rich text, brief, memo, note, report, spec, checklist, plan

**code**

* plain source file, config file, structured code snippet, small code project manifest, previewable web component

**data**

* tabular analysis, csv-backed table, structured result set, json/json-schema output, lightweight dataset summary

**presentation**

* slide outline, slide deck content model, presentation notes, sectioned narrative for slides

### 7.2 Deferred V1.5+ types

* diagram
* template/form
* multi-file bundle
* executable app artifact
* synced external artifact

### 7.3 Type system

Each artifact has:

* `type` — high-level family
* `subtype` — concrete renderer/editor contract
* `schema_version` — payload schema version
* `content_format` — serialization format

These are fixed identity fields on the Artifact entity, not Attributes. Example:

```json
{
  "type": "document",
  "subtype": "markdown",
  "schema_version": "1.0",
  "content_format": "text/markdown"
}
```

## 8. Containment and ownership model

### 8.1 Ownership

Each artifact belongs to exactly one workspace and may optionally belong to one project.

### 8.2 Conversation linkage

Artifacts may be linked to one or more conversations and messages. A single artifact can be:

* created in one conversation
* revised in another
* reused across the same project
* opened from Library independently of chat

### 8.3 Project context inheritance

At creation time an artifact captures context via Snapshots:

* workspace id
* project id if present
* conversation id
* creating message id
* project context snapshot
* policy snapshot
* source snapshots

Project updates after creation do not retroactively rewrite artifact content. They may influence future revisions only through explicit update runs.

## 9. Lifecycle model

### 9.1 Artifact lifecycle states

* `draft`
* `in_progress`
* `review_ready`
* `approved`
* `published`
* `archived`
* `errored`

### 9.2 Version lifecycle states

* `pending`
* `committed`
* `superseded`
* `restored`
* `failed`

### 9.3 Archive and delete

Archive is the default removal path. Hard delete is not part of normal artifact lifecycle flow and must use higher-governance deletion semantics consistent with NAVI's broader Data Lifecycle Model.

## 10. Domain model

Artifacts are **Attribute-composed** World Model entities. The entity carries a small set of fixed identity/ownership/lifecycle fields. All other descriptors — metadata, content descriptors, export settings, sync metadata, renderer hints — are Attributes.

### 10.1 Artifact (fixed fields)

```ts
// Illustrative — not normative wire schema
type Artifact = {
  artifact_id: string
  workspace_id: string
  project_id?: string | null

  canonical_title: string
  display_title: string
  type: ArtifactType           // fixed identity
  subtype: string              // fixed identity
  schema_version: string       // fixed identity
  content_format: string       // fixed identity

  lifecycle_state: ArtifactLifecycleState

  current_branch_id: string
  current_version_id: string
  head_version_number: number

  created_by_actor_type: ActorType
  created_by_actor_id?: string | null

  provenance_root_id: string

  created_at: string
  updated_at: string
  archived_at?: string | null
}

// Everything else is Attributes:
// - summary, tags, labels, icon, color, language, mime_type
// - estimated_size_bytes, word_count, line_count
// - section_index, export_formats, renderer_hints
// - sync_state, share_state, execution_state
// - permissions_policy_id
// - custom metadata
```

### 10.2 ArtifactVersion

Versions store either full attribute snapshots or attribute deltas.

```ts
// Illustrative — not normative wire schema
type ArtifactVersion = {
  version_id: string
  artifact_id: string
  branch_id: string
  version_number: number

  parent_version_id?: string | null
  base_version_id?: string | null

  author_actor_type: ActorType
  author_actor_id?: string | null

  source_message_id?: string | null
  source_conversation_id?: string | null
  source_operation_id: string

  change_type: ChangeType
  change_mode: ChangeMode
  change_summary: string

  content_ref: ContentRef
  rendered_ref?: ContentRef | null
  diff_ref?: ContentRef | null
  checksum_sha256: string
  content_size_bytes: number

  patch_strategy?: PatchStrategy | null
  patch_metadata?: PatchMetadata | null

  validation_state: ValidationState
  commit_state: VersionCommitState

  // Snapshot references for provenance
  project_snapshot_id?: string | null
  policy_snapshot_id: string
  source_snapshot_ids: string[]
  artifact_input_snapshot_ids: string[]

  // History linkage — points to existing execution records
  history_command_id?: string | null
  history_attempt_id?: string | null

  created_at: string
}
```

### 10.3 ArtifactBranch

```ts
// Illustrative — not normative wire schema
type ArtifactBranch = {
  branch_id: string
  artifact_id: string
  name: string
  base_version_id: string
  head_version_id: string
  status: "active" | "merged" | "abandoned"
  created_by_actor_type: ActorType
  created_by_actor_id?: string | null
  created_at: string
  updated_at: string
}
```

### 10.4 ArtifactReference

```ts
// Illustrative — not normative wire schema
type ArtifactReference = {
  reference_id: string
  artifact_id: string
  source_kind:
    | "message"
    | "conversation"
    | "project"
    | "source"
    | "export"
    | "share"
    | "external_target"

  source_id: string
  relationship_type:
    | "created"
    | "updated"
    | "mentioned"
    | "opened"
    | "derived_from"
    | "used_source"
    | "exported_to"
    | "shared_with"
    | "synced_to"

  version_id?: string | null
  branch_id?: string | null
  metadata?: Record<string, unknown>
  created_at: string
}
```

### 10.5 Snapshot

```ts
// Illustrative — not normative wire schema
type Snapshot = {
  snapshot_id: string
  snapshot_type:
    | "project_context_snapshot"
    | "policy_snapshot"
    | "source_snapshot"
    | "artifact_input_snapshot"

  // What this snapshot captured
  captured_entity_type: string
  captured_entity_id: string
  captured_version?: string | null

  // Frozen content
  content_ref: ContentRef
  checksum_sha256: string

  created_at: string
}
```

Snapshots are immutable. Once committed, content behind `content_ref` must never be modified.

### 10.6 ContentRef

```ts
// Illustrative — not normative wire schema
type ContentRef = {
  uri: string          // e.g. "artifact://artifacts/{artifact_id}/versions/{version_id}/content"
  storage_key: string  // object store key
  checksum_sha256: string
}
```

Content reference URI scheme:

* `artifact://artifacts/{artifact_id}/versions/{version_id}/content` — version content
* `artifact://artifacts/{artifact_id}/versions/{version_id}/rendered` — rendered preview
* `artifact://artifacts/{artifact_id}/versions/{version_id}/diff` — diff payload
* `artifact://snapshots/{snapshot_id}/content` — snapshot content

Backed by object storage. Blob content is immutable once committed.

### 10.7 ArtifactExport

```ts
// Illustrative — not normative wire schema
type ArtifactExport = {
  export_id: string
  artifact_id: string
  version_id: string
  format: ExportFormat
  target_kind: "download" | "external_system" | "internal_attachment"
  content_ref: ContentRef
  status: "pending" | "completed" | "failed"
  created_by_actor_type: ActorType
  created_at: string
}
```

### 10.8 ArtifactShare

```ts
// Illustrative — not normative wire schema
type ArtifactShare = {
  share_id: string
  artifact_id: string
  version_id?: string | null
  scope: "workspace" | "project" | "org" | "link"
  access_level: "read" | "comment" | "edit" | "copy"
  status: "active" | "revoked" | "expired"
  created_by_actor_type: ActorType
  created_at: string
  expires_at?: string | null
}
```

## 11. Actor model

```ts
type ActorType = "user" | "assistant" | "tool" | "system" | "migration" | "agent"
```

All artifact changes must record:

* actor type
* actor id when available
* source message id when applicable
* history command/attempt id

## 12. Update semantics

### 12.1 Supported operations

```ts
type ArtifactOperation =
  | "create_artifact"
  | "replace_content"
  | "patch_content"
  | "append_content"
  | "rename_artifact"
  | "update_metadata"
  | "archive_artifact"
  | "restore_artifact"
  | "branch_artifact"
  | "restore_version"
  | "export_artifact"
  | "share_artifact"
  | "sync_artifact"
```

### 12.2 Operation envelope

```ts
// Illustrative — not normative wire schema
type ArtifactOperationEnvelope = {
  operation_id: string
  operation: ArtifactOperation
  target_artifact_id?: string
  target_branch_id?: string
  expected_base_version_id?: string
  artifact_type?: ArtifactType
  artifact_subtype?: string
  title?: string
  payload?: unknown
  metadata_patch?: Record<string, unknown>
  reason?: string
  confirmation_mode?: "none" | "required" | "proposal"
}
```

### 12.3 Operation validity rules

#### create_artifact

Valid when no target artifact exists. Produces:

* Artifact entity
* initial Branch (`main`)
* initial Version
* References for creating message/conversation/project
* History record
* Snapshots for project context and policy

#### replace_content

Valid when caller specifies target artifact and base version. Produces new version with full content replacement.

Use when:

* entire content is being regenerated
* diff patch is low confidence
* artifact subtype lacks reliable patch semantics

#### patch_content

Valid when subtype supports patch strategy and target version matches expected base.

Use when:

* document section edits
* code block edits
* structured field edits
* table row/cell edits

Must fail cleanly if patch cannot be validated.

#### append_content

Valid for append-safe subtypes:

* markdown/doc sections
* logs
* plans/checklists
* outlines
* list-like structured payloads

Not valid for subtypes where append corrupts structure.

#### rename_artifact

Metadata-only update. Does not create content diff but does create a version event record unless implementation chooses metadata-version compression for low-risk fields.

#### update_metadata

Valid only for allowed metadata Attributes. Security-critical Attributes require higher governance.

#### archive_artifact

Transitions artifact lifecycle to `archived`. No content mutation. Creates lifecycle event and History record.

#### restore_artifact

Transitions artifact lifecycle from `archived` to prior active state.

#### branch_artifact

Creates new branch from selected version.

#### restore_version

Creates a new committed version whose content equals a prior version. Never mutates historical versions in place.

#### export_artifact

Produces export object and optional side effects. May require Proposal depending on target and Autonomy Model.

#### share_artifact

Creates or modifies share record. Access policy checks apply. May require Proposal depending on share scope and Autonomy Model.

#### sync_artifact

Reads/writes external target. Requires sync adapter and permission checks. May require Proposal.

### 12.4 Version creation policy

A new version is created for:

* create
* replace
* patch
* append
* restore_version
* branch head creation
* metadata changes that affect user-visible state or execution/export behavior

A lightweight event record may be used instead of full version content only for:

* view/open events
* share link revoke/extend
* non-user-visible metadata changes

### 12.5 Conflict policy

Artifact updates are optimistic but server-confirmed.

Client or agent must send `expected_base_version_id`.

If current head differs:

* reject update with `version_conflict`
* return latest head version
* optionally auto-branch if policy allows
* never silently overwrite

### 12.6 Patch strategies by type

#### document

1. block/section patch using stable block ids
2. text-range patch
3. full replace fallback

#### code

1. AST-aware patch where parser exists
2. block/function patch
3. text patch with validation
4. full replace fallback

#### data

1. schema-aware row/cell patch
2. structured object patch
3. full replace fallback

#### presentation

1. slide-level patch
2. section-level patch
3. outline replace
4. full replace fallback

#### html preview / executable artifacts

Use structured source patch against canonical source, never DOM patch against rendered preview.

### 12.7 Partial update validation

Patch application must validate:

* target version match
* subtype compatibility
* schema validity
* parser validity if structured
* renderer acceptance
* safety checks if HTML/code previewable

If validation fails:

* reject patch
* preserve prior version
* optionally emit recovery draft or suggestion to retry as replace

## 13. Provenance model

Every committed version must carry provenance sufficient to answer:

* who changed this
* when
* why
* from which message
* using which agent/model
* with which tools
* from which sources
* against which project/workspace snapshot
* with which policy outcome

### 13.1 Required provenance fields

* source message id
* source conversation id
* history command id / attempt id (linking to History execution records)
* initiator actor type/id
* tool invocation refs (via History)
* snapshot refs (project context, policy, source, artifact input)
* change summary
* change intent / reason
* validation result

### 13.2 Provenance guarantees

* no committed version without provenance root
* no external sync without external target reference
* no export without export record
* no tool-produced mutation without History execution record
* all snapshots immutable after creation

## 14. Cross-entity relationships

Artifacts are World Model entity siblings to Knowledge, Memory, History, Events, Contacts, and other core entities. The relationships:

### 14.1 Artifacts → History

Every artifact create, update, and archive emits a History record. History remains the authoritative execution record. ArtifactVersions link to History via `history_command_id` and `history_attempt_id` rather than maintaining parallel execution tracking.

### 14.2 Artifacts → Knowledge

Artifact content may become source material for Knowledge, but artifacts are not a subtype of Knowledge by default. The Subconscious Consolidation process may derive reusable Knowledge entries from artifact content when patterns justify it.

### 14.3 Artifacts → Memory

Artifact interactions may influence Memory formation through the normal reflection pipeline. NAVI may form Memories about user artifact preferences, editing patterns, or domain-specific content patterns.

### 14.4 Artifacts → Events

Artifact deadlines, review dates, or scheduled exports may generate Events. Workflow milestones tied to artifacts may also surface as Events.

### 14.5 Artifacts → Contacts

Artifacts may reference Contacts as collaborators, reviewers, or share targets via ArtifactReference and ArtifactShare.

## 15. Message linkage model

Messages and artifacts must maintain bidirectional references.

### 15.1 Message-side representation

A message may contain zero or more artifact cards. Each card includes:

* title
* type/subtype
* lifecycle badge
* save/sync badge
* open action
* last updated
* created or updated label

### 15.2 Artifact-side linkage

Each artifact stores via References:

* creating conversation/message
* modifying messages
* linked conversations
* linked sources

### 15.3 Stable identifiers

Chat should reference artifacts using stable ids, not position-based concepts like "the thing above".

## 16. Agentic workflow model

This is the most important section.

### 16.1 Planning model

The LLM does not mutate artifacts directly. It produces an artifact intent. The orchestrator resolves and validates that intent, then compiles it into governed commands and/or Skill invocations.

This matches NAVI's broader execution model: probabilistic planning, deterministic governed execution.

### 16.2 Artifact orchestration pipeline

Each step has explicit layer ownership:

1. **Detect intent** — *Cognitive Layer*

   * create new artifact
   * update existing artifact
   * branch artifact
   * open artifact
   * export/share/sync artifact

2. **Resolve target** — *Cognitive Layer*

   * artifact id if explicit
   * infer from conversation/project context
   * if ambiguous, ask or propose options

3. **Plan operation** — *Cognitive Layer*

   * select operation type
   * select patch strategy
   * choose branch behavior

4. **Validate/Govern** — *Cognitive Layer (deterministic governance step)*

   * permissions
   * Autonomy Model check (global preset + per-domain overrides)
   * workspace/project scope
   * risk assessment
   * side effects
   * Proposal Queue entry if confirmation required

5. **Execute** — *Capability Layer*

   * compile to NAVI primitive commands
   * create History execution record (command_id / attempt_id)
   * invoke tools/Skills if needed
   * validate output
   * commit version or fail cleanly
   * link version to History record

6. **Reflect/record** — *Cognitive Layer, with handoff to Subconscious*

   * create References
   * emit artifact payloads for Subconscious processing
   * update Library and chat UI (Experience Layer)

### 16.3 Assistant action contract

```json
{
  "artifact_intent": {
    "intent_type": "create_or_update",
    "target_hint": {
      "artifact_id": null,
      "project_scope": "current",
      "conversation_scope": "current",
      "title_hint": "Artifact System Spec"
    },
    "desired_type": "document",
    "desired_subtype": "markdown",
    "operation_preference": "replace_or_patch",
    "reason": "User requested a durable implementation-ready spec",
    "content_payload": { "markdown": "..." },
    "metadata_patch": {
      "tags": ["spec", "artifacts", "navi"]
    }
  }
}
```

The orchestrator must normalize this into a concrete `ArtifactOperationEnvelope`.

### 16.4 Skill-generated artifact flow

When a Skill produces an output through the Capability Layer:

* Cognitive Layer decides whether to materialize the output as an artifact
* Skills never commit artifact truth directly — durable truth belongs in NAVI entities, not hidden Skill state
* if materialized: create History execution record, validate subtype/renderability, commit artifact version
* if invalid: mark execution record failed and preserve recoverable outputs

### 16.5 Long-running workflow flow

For workflows expected to take multiple steps:

* create draft artifact immediately
* mark artifact `in_progress`
* commit checkpoint versions after major milestones
* surface run progress in Library inspector
* finalize to `review_ready` when stable

### 16.6 Ambiguous target policy

If user says "update the spec" and multiple candidate artifacts exist:

1. prefer currently open artifact
2. else prefer artifact explicitly linked in recent messages
3. else prefer project-local artifact with strongest recency + title match
4. if confidence below threshold, ask user

## 17. Governance integration

Artifact operations integrate with NAVI's existing governance system. No parallel risk/governance infrastructure.

### 17.1 Mutation authority

Direct user edits are permitted within current workspace/project permissions.

Assistant/tool/agent/Skill edits must execute through governed artifact operations, producing History execution records for every mutation.

### 17.2 Autonomy Model integration

Artifact actions use the existing Autonomy Model with global preset and per-domain overrides. Artifacts constitute a domain. Suggested default risk levels:

**low** (typically auto-approved)

* direct local edit
* rename
* metadata Attribute change
* local branch creation

**medium** (behavior depends on Autonomy preset)

* full content replacement by assistant
* export to local download
* renderer-driven transform
* code preview refresh

**high** (typically requires Proposal)

* publish/share outside project/workspace
* sync overwrite to external system
* destructive replace of externally sourced artifact
* code execution with network/filesystem effects

These are defaults. Per-domain overrides in the Autonomy Model may raise or lower any of these.

### 17.3 Proposal Queue integration

When governance requires confirmation, the artifact operation serializes as a Proposal in the existing Proposal Queue. The Proposal contains:

* full serialized proposed action (ArtifactOperationEnvelope)
* affected entities (artifact id, version, branch, workspace, project)
* risk rationale
* expected side effects

Proposal resolution follows NAVI's existing Proposal Queue flow.

### 17.4 Permission checks

Every operation must validate:

* workspace membership
* project access
* artifact lifecycle compatibility
* share scope
* external target auth if exporting/syncing
* policy snapshot compatibility
* renderer capability for target subtype

### 17.5 HTML and code preview boundaries

HTML/code preview requires sandboxing:

* separate origin when rendered in browser
* CSP restrictions
* no privileged app context access
* no connector/session token access
* explicit bridge for permitted preview messaging only

### 17.6 Oversized payloads

Large payloads:

* stream into blob store
* cap inline preview size
* defer expensive diff/render operations
* allow background render jobs, but artifact creation/version commit remains synchronous at metadata level

### 17.7 Invalid or malicious content

If payload fails validator or sandbox policy:

* reject commit or mark artifact version `failed`
* preserve prior stable version
* surface exact failure in History execution record

## 18. Subconscious Process participation

Artifacts participate in NAVI's Subconscious Process:

### 18.1 Shallow Reflections

Triggered after artifact create/update/archive operations. May:

* update tags and relationship links on the artifact
* strengthen or weaken cross-artifact associations
* flag patterns for Consolidation

### 18.2 Consolidation

Periodic deeper processing. May:

* derive reusable Knowledge entries from recurring artifact content patterns
* strengthen links across artifacts in the same project or domain
* identify artifact clusters that suggest project reorganization

### 18.3 Deep Reflections

Infrequent, high-cost analysis. May:

* **propose** reclassification of artifacts (type/subtype changes)
* **propose** archival of stale artifacts
* **propose** project reorganization based on artifact clustering
* **propose** inferred workflow rules (e.g., "user always exports specs as PDF after approval")

Deep Reflections follow NAVI's existing rule: they can only *propose* changes to Owner-tier Configuration and Priorities, never apply them directly.

### 18.4 Escalation

If any Subconscious finding exceeds the escalation threshold, it enters the Conscious Process through the standard Escalation Trigger mechanism.

## 19. Library specification

### 19.1 Library views

V1 Library must provide:

* `Recent`
* `Current Conversation`
* `Current Project`
* `Current Workspace`
* `Drafts / In Progress`
* `Shared / Exported`
* `Archived`

### 19.2 Artifact list item

Each list item shows:

* title
* type/subtype
* project
* status
* last editor
* last updated
* branch badge if not main
* sync state
* pending execution state if applicable

### 19.3 Artifact inspector

Inspector tabs:

* Overview
* Versions
* Branches
* Provenance (snapshot links, History execution records)
* Sources
* Exports
* Shares
* Permissions
* Sync

### 19.4 Open modes

* read
* edit
* diff
* history
* inspector split view

### 19.5 Conversation integration

Conversation-level artifact rail or panel should show:

* artifacts created/updated in conversation
* quick reopen
* active artifact indicator
* pending execution state

## 20. Workspace UI composition

### 20.1 Required components

* `ArtifactWorkspaceShell`
* `ArtifactHeader`
* `ArtifactNavigator`
* `ArtifactCanvas`
* `ArtifactInspector`
* `ArtifactVersionTimeline`
* `ArtifactDiffViewer`
* `ArtifactReferenceChip`
* `ArtifactMessageCard`
* `SaveStateBadge`
* `SyncStateBadge`

### 20.2 Behavior

* opening an artifact should not collapse chat context
* switching artifacts preserves unsaved-draft state locally until save/reject
* version browsing should not mutate current edit session unless restore/checkout is explicit
* failed execution state must be visible without digging through logs

## 21. Renderer/editor registry

### 21.1 Registry contract

```ts
// Illustrative — not normative wire schema
type ArtifactRendererDefinition = {
  type: ArtifactType
  subtype: string

  viewer: ReactComponent
  editor?: ReactComponent
  diff_viewer?: ReactComponent
  preview?: ReactComponent

  parse: (content: StoredContent) => ParsedContent
  serialize: (value: ParsedContent) => StoredContent
  validate: (value: ParsedContent) => ValidationResult

  patch_strategy: PatchStrategyDescriptor[]
  export_formats: ExportFormat[]
  command_affordances?: CommandAffordance[]

  supports_direct_edit: boolean
  supports_ai_patch: boolean
  supports_branch_diff: boolean
}
```

### 21.2 Registry rules

* registry lookup is by `(type, subtype)`
* renderer missing => artifact is still retrievable, with raw fallback viewer
* editor missing => artifact is read-only in V1
* renderer versioning must be independent from artifact content versioning

### 21.3 Fallback behavior

If renderer unavailable:

* show metadata
* show raw payload
* block unsupported editing
* allow export of raw content where safe

## 22. Persistence model

### 22.1 Storage strategy

Use a hybrid storage model.

**Relational store** — metadata and queryable relations:

* `artifacts` (fixed fields + Attribute references)
* `artifact_attributes` (Attribute-composed metadata)
* `artifact_versions`
* `artifact_branches`
* `artifact_references`
* `artifact_exports`
* `artifact_shares`
* `snapshots`

**Blob/object store** — large content payloads and rendered artifacts:

* large markdown/rich text
* source bundles
* rendered previews
* exports
* diff payloads if large
* snapshot content

**Inline content threshold**

Small payloads may be stored inline in `artifact_versions` up to a configured threshold, such as 128–256 KB. Larger payloads move to blob storage via `ContentRef`.

**Why hybrid**

* Library and search need relational metadata
* payload sizes vary dramatically by type
* version history grows quickly
* rendered previews and exports are naturally blob-like
* future code bundles and slide decks are not clean relational payloads

### 22.2 Rehydration

On conversation/project reopen:

* fetch artifact list
* fetch current artifact metadata and Attributes
* fetch selected version or head version
* resolve renderer
* rebuild UI workspace state (Experience Layer)
* load pending execution state and sync badges
* reattach message cards in transcript

## 23. Frontend state model

`ArtifactWorkspaceState` is **Experience Layer session state**, not a World Model entity. It is not persisted as domain truth.

Frontend must track:

* current workspace
* current project
* open artifact id
* selected branch
* selected version
* mode (read/edit/diff/history)
* local edit buffer
* dirty state
* save state
* pending execution ids
* artifact list scope
* current filters
* load error
* renderer resolution error
* conflict state
* sync state

### 23.1 Save state machine

* `clean`
* `dirty`
* `saving`
* `saved`
* `save_failed`
* `conflicted`

### 23.2 Reconnect handling

On reconnect:

* refetch artifact head/version pointers
* compare with local edit buffer base version
* if diverged, enter `conflicted`
* allow user to branch or reapply diff

### 23.3 Persistence of user preferences

Only explicit user-owned preferences (e.g., preferred open mode, default filters, pinned artifacts) may be persisted as user-scoped Attributes or Configuration entries if needed for reopen continuity. Ephemeral session state (scroll position, transient selections) is not persisted.

## 24. API / service contract

Use internal service methods first; expose HTTP/RPC externally if needed.

### 24.1 Artifact service interface

```ts
// Illustrative — not normative wire schema
interface ArtifactService {
  createArtifact(input: CreateArtifactInput): Promise<ArtifactWithVersion>
  getArtifact(artifactId: string): Promise<ArtifactRecord>
  listArtifacts(query: ListArtifactsQuery): Promise<ArtifactSummary[]>
  updateArtifact(op: ArtifactOperationEnvelope): Promise<ArtifactUpdateResult>
  getVersionHistory(artifactId: string, branchId?: string): Promise<ArtifactVersion[]>
  getDiff(input: ArtifactDiffQuery): Promise<ArtifactDiffResult>
  branchArtifact(input: BranchArtifactInput): Promise<ArtifactBranch>
  restoreVersion(input: RestoreVersionInput): Promise<ArtifactUpdateResult>
  archiveArtifact(input: ArchiveArtifactInput): Promise<void>
  exportArtifact(input: ExportArtifactInput): Promise<ArtifactExport>
  shareArtifact(input: ShareArtifactInput): Promise<ArtifactShare>
}
```

### 24.2 Renderer registry interface

```ts
// Illustrative — not normative wire schema
interface ArtifactRendererRegistry {
  register(def: ArtifactRendererDefinition): void
  get(type: ArtifactType, subtype: string): ArtifactRendererDefinition | null
  list(): ArtifactRendererDefinition[]
}
```

### 24.3 Search query examples

* list artifacts by conversation
* list artifacts by project
* list artifacts by workspace
* search by title/content/tag/type/status
* list pending in-progress artifacts
* list artifacts linked to message id

## 25. Failure model

Artifact operations inherit NAVI's broader failure principles: partial, failed, and degraded outcomes are first-class and must never be hidden as success.

### 25.1 Failure classes

* validation_rejection
* permission_denial
* policy_blocked
* version_conflict
* renderer_missing
* patch_apply_failed
* schema_invalid
* sandbox_blocked
* storage_failure
* export_failure
* sync_failure
* unknown

### 25.2 Recovery rules

**create failure before first commit** — No artifact record unless transaction completed; keep History execution failure record if execution started.

**create failure after artifact row but before version commit** — Artifact state becomes `errored`; recovery draft may be attached.

**patch failure** — Prior version remains head; History execution record marked failed.

**partial workflow completion** — Artifact stays `in_progress`; successful intermediate versions remain committed; failed step recorded in History.

**sync/export failure** — Artifact content unaffected unless sync operation explicitly included artifact mutation and commit completed.

## 26. Observability

Artifact system observability is mandatory.

### 26.1 Required events

* artifact.created
* artifact.updated
* artifact.archived
* artifact.restored
* artifact.branched
* artifact.version.committed
* artifact.version.restore_requested
* artifact.execution.started
* artifact.execution.completed
* artifact.execution.failed
* artifact.patch.failed
* artifact.renderer.resolved
* artifact.renderer.missing
* artifact.workspace.loaded
* artifact.workspace.load_failed
* artifact.export.completed
* artifact.sync.completed
* artifact.sync.failed
* artifact.conflict.detected

### 26.2 Required dimensions

* workspace_id
* project_id
* artifact_id
* version_id
* branch_id
* history_command_id
* history_attempt_id
* actor_type
* operation
* subtype
* renderer_key
* latency
* result status
* failure class/code

### 26.3 Debug surfaces

Library inspector should expose:

* latest History execution records for this artifact
* latest validation failures
* latest sync/export attempts
* version ancestry
* source message links
* renderer key and registry result
* snapshot chain

## 27. Testing strategy

### 27.1 Unit tests

Cover:

* operation validation
* lifecycle transitions
* conflict detection
* version ancestry
* patch strategy selection
* renderer registry resolution
* provenance attachment
* policy gating
* Attribute composition/retrieval
* snapshot immutability

### 27.2 Integration tests

Cover:

* create artifact from message
* create from Skill output
* patch existing artifact
* replace content
* branch and restore
* archive and reopen
* export and share record creation
* storage rehydration after refresh
* History record linkage
* Snapshot creation and reference integrity

### 27.3 Frontend tests

Cover:

* open artifact from chat
* switch between chat and Library
* edit/save cycle
* conflict UI
* version history navigation
* renderer fallback
* failed execution surfacing
* reconnect state restoration

### 27.4 Concurrency tests

Cover:

* two user sessions editing same artifact
* user + assistant race
* stale version patch rejection
* branch fallback on conflict
* reconnect after remote head change

### 27.5 API contract tests

Validate:

* request schema
* response schema
* status codes
* error classes
* backward-compatible field addition rules

### 27.6 Minimum end-to-end scenario

1. user requests a spec
2. assistant emits create artifact intent
3. orchestrator creates artifact + version + message card + History record + snapshots
4. user opens artifact from chat
5. user edits artifact directly
6. save commits version 2 with History record
7. assistant patches section 3
8. version 3 committed with History record
9. version history shows versions 1–3 with provenance chains
10. user restores version 2
11. version 4 committed as restore with History record

## 28. V1 implementation plan

### 28.1 Phase 1 — domain foundation

Build:

* Artifact entity with Attribute composition
* version model
* branch model
* reference model
* snapshot model
* ContentRef and storage abstraction
* History integration
* basic governance hooks (Autonomy Model domain registration, Proposal serialization)

### 28.2 Phase 2 — renderer foundation

Build:

* registry
* raw fallback renderer
* document markdown renderer/editor
* code text renderer/editor
* structured json/data renderer/editor
* diff viewer foundation

### 28.3 Phase 3 — Library and workspace

Build:

* Library navigator (Experience Layer surface)
* artifact workspace shell
* inspector (including snapshot/provenance views)
* version timeline
* chat message cards
* conversation artifact rail

### 28.4 Phase 4 — orchestration integration

Build:

* assistant artifact intent contract
* orchestrator target resolution
* patch strategy selection
* History execution record linkage
* draft artifact for long workflows
* Skill output → artifact materialization path

### 28.5 Phase 5 — export, recovery, Subconscious, polish

Build:

* export pipeline
* failure recovery views
* conflict UI
* archived view
* Subconscious integration (Shallow Reflection hooks, Consolidation rules)
* metrics/tracing
* regression tests

## 29. Suggested package/module layout

Illustrative layout:

```text
internal/navi/artifacts/
  model/
    artifact.go
    version.go
    branch.go
    reference.go
    snapshot.go
    export.go
    content_ref.go
  service/
    artifact_service.go
    diff_service.go
    policy_service.go       // delegates to existing governance
  storage/
    artifact_repo.go
    version_repo.go
    snapshot_repo.go
    blob_store.go
  orchestration/
    intent.go
    resolver.go
    executor.go             // compiles to NAVI commands, creates History records
    patcher.go
  renderers/
    registry.go
    document/
    code/
    data/
    presentation/
  api/
    artifact_handlers.go
    artifact_schemas.go
```

Frontend:

```text
ui/artifacts/
  components/
    ArtifactWorkspaceShell.tsx
    ArtifactNavigator.tsx
    ArtifactCanvas.tsx
    ArtifactInspector.tsx
    ArtifactVersionTimeline.tsx
    ArtifactDiffViewer.tsx
    ArtifactMessageCard.tsx
  renderers/
    registry.ts
    document/
    code/
    data/
    presentation/
  state/
    artifactStore.ts          // Experience Layer session state
    workspaceState.ts         // Experience Layer session state
  api/
    artifacts.ts
```

## 30. V1 acceptance criteria

V1 is complete when:

* artifacts are first-class Attribute-composed World Model entities
* Library can browse/reopen artifacts across conversation/project/workspace
* at least document, code, and data types are supported via registry
* chat can create and reference artifacts
* user edits and assistant edits both produce versions
* branching exists
* conflict detection blocks silent overwrite
* provenance via snapshots and History records exists for all assistant/tool/Skill changes
* every artifact mutation emits a History record
* Subconscious Shallow Reflection hooks exist
* persistence survives refresh/reopen
* failures are surfaced explicitly via History execution records
* export works for initial supported formats
* tests cover critical flows

## 31. Roadmap after V1

### V1.5

* presentation renderer/editor maturity
* diagram type
* compare/merge UX improvements
* template artifacts
* comments/annotations
* Subconscious Consolidation rules for cross-artifact Knowledge derivation

### V2

* external sync adapters
* share permissions expansion
* code/app preview execution isolation
* artifact bundles
* review workflows
* branch merge assistance
* Deep Reflection artifact proposals

### V3

* collaborative editing
* reusable artifact workflows
* artifact graphs across projects
* automated background sync jobs
* artifact-level agents

## 32. Locked decisions

1. **NAVI Library is an Experience Layer surface, not a fifth layer.**
2. **Artifacts are Attribute-composed World Model entities, sibling to Knowledge, Memory, History, Events.**
3. **All assistant/tool/Skill mutations go through governed artifact operations, compile to NAVI primitive commands, and produce History execution records plus immutable versions.**
4. **Branching is included in V1; merging is not.**
5. **Version-aware optimistic concurrency is mandatory.**
6. **Renderer/editor behavior is registry-driven by type and subtype.**
7. **Long-running workflows create draft artifacts early and checkpoint progress.**
8. **Archive is the default removal path.**
9. **Provenance is mandatory: snapshots, History linkage, and actor records for all committed changes.**
10. **The storage model is hybrid: relational metadata plus blob-backed payloads via ContentRef.**
11. **Governance uses the existing Autonomy Model and Proposal Queue. No parallel risk system.**
12. **ArtifactWorkspaceState is Experience Layer session state, not World Model.**
13. **Skills produce outputs through Capability; Cognitive decides artifact materialization; Skills never commit artifact truth directly.**
14. **Subconscious Process participates via Shallow Reflections, Consolidation, and Deep Reflection proposals.**
15. **Workspace entity is a dependency. If no canonical Workspace spec exists, one must be produced before artifact implementation begins.**
