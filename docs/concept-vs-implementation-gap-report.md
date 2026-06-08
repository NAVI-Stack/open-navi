# Concept vs Implementation Gap Report

<!-- markdownlint-disable MD060 -->

**Source:** [Conceptual Design Overview](canonical/conceptual-design-overview.md)  
**Codebase:** NAVI (Go daemon `navid`, CLI `navi`, internal packages)  
**Generated:** 2026-04-01

This document maps the canonical conceptual design to the current implementation and lists gaps by section. Status key: **implemented** | **partial** | **missing**.

---

## 1. Architectural Layers → Package Mapping

| Conceptual Layer   | Primary Responsibility                               | Implementation Mapping                                                                                                                                 | Status    |
| ------------------ | ----------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ | --------- |
| **Experience**     | Role selection, persona, tone, presentation           | `internal/navi/persona.go`, `internal/navi/experience`, `internal/navi/loop.go`, `internal/gateway`. Dedicated Experience package exists; adaptation/storage/debugger streams remain incomplete. | **partial** |
| **Cognitive**      | Conscious + Subconscious; reasoning, reflection        | `internal/orchestrator`, `internal/navi`, `internal/navi/reflection`, worker agents. Consolidation/Deep pipelines in research status. | **partial** |
| **World Model**    | Single source of truth for entities and state         | `internal/store` (SQLite). Multiple subsystems write; design says only Cognitive should.                                                                | **partial** |
| **Capability**     | Commands, Connectors, Plugins                         | `internal/schema/command.go`, `internal/navi/skill`, `connectors/`. Plugins not separate from skills; connector taxonomy narrower.                     | **partial** |

**Summary:** All four layers have code that can be assigned to them, but boundaries are still not fully strict. Experience now has a distinct runtime package, while World Model is still written by multiple subsystems. No single package or binary is named by conceptual layer.

---

## 2. World Model and Entity Classes

### 2.1 World Entities

| Entity | Design Description | Implementation | Status |
|--------|--------------------|----------------|--------|
| **Contacts** | Contact graph; owner + others; default attribute name | `schema.Contact`, `store/contact.go`, table `contacts` (id, name, kind, metadata, created_at, updated_at) | **implemented** |
| **Events** | Scheduling, temporal awareness | `schema.WorldModelEvent`, `store/wm_event.go`, table `wm_events` | **implemented** |
| **History** | Append-oriented interaction log; interaction events + execution outcome records; retention/deletion deferred | No single "History" entity. Conversation: `directive_messages`. Audit/flow: `events` (append-only). Execution outcomes: `execution_outcomes`. Design’s “two classes of event” (interaction vs execution outcome) split across directive_messages and execution_outcomes. Implemented as unified `history` view (directive_messages + events + execution_outcomes) with class and fidelity. | **implemented** |
| **Knowledge** | Curated facts, attributed, mutable | `schema.Fact`, table `facts`; `fact_supersessions` for supersede. Provenance via `entity_provenance`. No first-class “deprecate” soft-delete on Knowledge; supersession exists. | **partial** |
| **Memories** | Significant experiences, emotional weight; Forget path | `schema.Memory`, table `memories`. `store.ForgetMemory` applies Forget, auto-declines proposals, and calls `FlagDerivativesForReEvaluation`; table `re_evaluation_flags` stores flagged derivatives for next Consolidation/Deep. | **implemented** |
| **Artifacts** | Files, documents, notes | `schema.Artifact`, table `artifacts` (versioned history, Materialization API OMN-118) | **implemented** |
| **Proposals** | First-class queue; full lifecycle; serialized action | `schema.Proposal`, table `proposals` (all design fields present). Gateway: `GET /api/proposals`, `POST /api/proposals/{id}/resolve`. Resolution returns to NAVI `ExecuteApprovedProposal` / `ResolveProposal`. | **implemented** |

### 2.2 Cognitive Entities

| Entity         | Design Description                          | Implementation                                                                 | Status        |
| -------------- | -------------------------------------------- | ------------------------------------------------------------------------------ | ------------- |
| **User Model** | Composite, derived at query time; never stored | `schema.UserModel`, `store/usermodel.go` assembles from DB                     | **implemented** |

### 2.3 Control Entities

| Entity           | Design Description                | Implementation                                                                 | Status        |
| ---------------- | ---------------------------------- | ------------------------------------------------------------------------------ | ------------- |
| **Configuration**| Explicit + inferred preferences    | `schema.ConfigurationEntry`, table `configuration` (source: explicit/owner_set/inferred) | **implemented** |
| **Priorities**   | Stated and observed goals          | `schema.Priority`, table `priorities`                                          | **implemented** |

### 2.4 State Kinds (Explicit, Owner-set, Inferred, Derived)

Design defines four state kinds. `schema.StateKind` enum: Explicit, OwnerSet, Inferred, Derived. Configuration and Priorities use `source` aligned to these; governor autonomy treats owner-set as requiring confirmation. **Implemented.**

### 2.5 Relationship Layer

| Field        | Design              | Implementation                                                                 | Status     |
| ------------ | ------------------- | ------------------------------------------------------------------------------ | ---------- |
| Type         | Relationship category | `schema.Relationship.RelationshipType`, `entity_relationships.relationship_type` | **implemented** |
| Confidence   | Strength of belief  | `Relationship.Confidence`                                                      | **implemented** |
| Recency      | Last reinforced     | `Relationship.Recency`                                                         | **implemented** |
| Provenance   | Source/process      | `Relationship.Provenance`; full chain in `entity_provenance` per entity          | **partial**   |

---

## 3. Governance, Validation, and Proposal Queue

| Feature | Design | Implementation | Status |
|---------|--------|----------------|--------|
| **Validation order** | Permissions → Policy → Configuration → Priority Alignment → Risk Assessment | `governor/validate.go`: `Pipeline` with Permissions, Policy, Configuration, PriorityAlignment, RiskAssessment in that order. Steps can be nil (treated as Approved). | **implemented** |
| **Validation outcomes** | Approved, Requires Confirmation, Modified, Rejected | `ValidationOutcome` enum and `ValidationResult` in `governor/validate.go`. Modified carries `ModifiedAction` for re-evaluation. | **implemented** |
| **Constraint feedback loop** | Rejected → constraint context back to Decide, not user error | NAVI loop returns rejection as tool result to LLM with Replan instruction; constraint context flows to next Decide step. | **implemented** |
| **Tier authority** | System > Owner > Plugin | `GovernanceTier` in `governor/validate.go`. Pipeline step tier assignment exists; conflict resolution (most restrictive within tier) not fully wired in policy engine. | **partial** |
| **Proposal creation** | Validate/Govern (Requires Confirmation) and Subconscious (proposed changes to explicit/owner-set) | NAVI loop: `SaveProposal` when validation returns RequiresConfirmation. Reflection: `store.PrepareProposalFromReflection` + `SaveProposal` for consolidation/deep_reflection; worker stubs document that owner-set changes must go through proposals. | **partial** |
| **Proposal resolution** | Owner or system only; re-evaluate governance once before executing on approve | `ExecuteApprovedProposal` re-runs validation before executing; on reject/requires-confirmation declines proposal. | **implemented** |
| **Proposal lifecycle** | pending → approved/declined/expired/superseded | Schema and store support all statuses. `ExpireProposals` called from gateway before list; `SupersedeOldProposals` called from `SaveProposal` when saving pending. | **implemented** |
| **Proposal cascade** | Auto-decline when affected entity archived/tombstoned | `store.ArchiveEntity`, `ForgetMemory`, `TombstoneEntity` call `AutoDeclineProposalsForEntity` for affected entity. | **implemented** |

---

## 4. Conscious and Subconscious Process Loops

| **Conscious loop** | Perceive → Interpret → Contextualize → Decide → Validate/Govern → Execute → Reflect | NAVI loop: message in → LLM (with context) → tool/skill calls (Execute) → GapDetector (OMN-20) → Decide → Validate. | **implemented** |
| **Reflect step** | Produces reflection payload (what happened, learned, suggested tier) | `emitReflect` in `navi/loop.go` publishes `FactReflectionQueued` with `ReflectionPayload` (tier Shallow only in current emit). | **partial** |
| **Reflection tiers** | Shallow (frequent), Consolidation (periodic), Deep (infrequent); mutation permissions per tier | `schema.ReflectionTier`: shallow, consolidation, deep. `navi/reflection/worker.go`: implemented for Shallow/Consolidation. Deep (infrequent) is partial. | **partial** |
| **Escalation** | Manual (“remember this”) and automatic (significance); bypass cadence | Manual: emitReflect sets tier Deep and EscalationReason. Automatic significance implemented. | **implemented** |
| **Subconscious → Conscious** | World Model updates; Conscious picks up in Contextualize | Gap Detection (OMN-20) wakes higher-order reflection. | **implemented** |
| **Urgent interruption** | Contradiction + high confidence + material harm; one per decision cycle | `SubconsciousInterruption` schema and `FactSubconsciousInterruption` event; recursion guard in reflection worker. No real contradiction detection or injection into Conscious loop. | **partial** |

---

## 5. Capability Layer: Commands, Connectors, Plugins

| Concept | Design | Implementation | Status |
|---------|--------|----------------|--------|
| **10 primitive Commands** | Query, Create, Update, Delete, Invoke, Send, Acquire, Schedule, Delegate, Compose | `schema.CommandType` in `schema/command.go`: all 10 present. | **implemented** |
| **Command usage** | Execute step issues Commands; composition via Compose | Tasks and tool calls map to Invoke; no systematic issuance of Create/Update/Delete/Send/Acquire/Schedule/Delegate. Compose exists in schema only. | **partial** |
| **Idempotency expectations** | Per-command semantics (design table) | `schema.IdempotencyExpectation` and `CommandType.Idempotency()` in `schema/command.go`. | **implemented** |
| **Connectors** | Communication, Information, Service, Device/Environment; Identity and Access plane | Connectors: Slack, Telegram (communication). No taxonomy in code matching design (Files, Databases, Web, Streams, Productivity, etc.). Identity: `internal/identity`, keystore; not a separate “connector” type. | **partial** |
| **Plugins** | Packaged capability: Capabilities, Skills, Commands, Connectors, Permissions, Interfaces, Policies | Skills: `internal/navi/skill` (OSS27 spec, interfaces, effects, security). No separate “Plugin” entity; skills act as capability units. Plugin lifecycle (Discover, Install, Register, Activate, Invoke, Learn, Update, Retire) not fully implemented. | **partial** |
| **Skill** | Versioned, permission profile, I/O contract | `navi/skill`: spec with semver, effects (reversibility, requires_confirmation, risk_tier), interfaces with input/output schema. Provenance in skill loader. | **implemented** |

---

## 6. Failure, Provenance, and Autonomy Models

### 6.1 Failure Model

| **Native token streaming** | Provider-level streaming capability | Implemented by provider plugins under `plugins/llm-anthropic` and `plugins/llm-openai`; `internal/llm` keeps the provider interfaces and routing/control plane. | **implemented** |
| **Failure taxonomy**         | Eight failure classes                        | `schema.FailureClass` in `schema/command.go`: all eight (OMN-20 Gap Detection uses A-F classes) | **implemented** |
| **Execution outcome record** | Every attempt → History; full field set      | `schema.ExecutionOutcome`, table; workers and NAVI call Save      | **implemented** |
| **Reversibility classes**    | Reversible, Compensable, Irreversible        | `schema.ReversibilityClass`; skill spec reversibility              | **implemented** |
| **Compose failure semantics**| Rollback/compensation/surface partial        | RunCompensation stub; no Compose runner                            | **partial**     |
| **Degradation visibility**   | Silent Retry, Advisory, Blocking, Deferred   | Schema helpers; not wired to gateway/UX                             | **partial**     |
| **Plugin/connector isolation** | Crash → structured failure; WM only Cognitive | Errors returned; no process isolation; governor path checks       | **partial**     |

### 6.2 Provenance Model

| Feature                        | Design                              | Implementation summary                                    | Status          |
| ------------------------------ | ----------------------------------- | --------------------------------------------------------- | --------------- |
| **Core fields**                | source, timestamp, confidence, …    | EntityProvenance + table; reinforcement_count column and store support. | **implemented** |
| **Proposal in mutation history** | proposal_id when authorized         | entity_provenance.proposal_id; callers set on apply       | **implemented** |
| **Confidence propagation**     | Attenuation rule                    | `store.ConfidenceFromChain`; `SaveEntityProvenance` caps confidence by min of derivation chain. | **implemented** |

### 6.3 Data Lifecycle Model

| Action | Design | Implementation | Status |
|--------|--------|----------------|--------|
| **Archive** | Default for Delete; soft-delete | `LifecycleAction` archive; tombstones table has action. No universal “archive” path for all entity deletes. | **partial** |
| **Forget** | Memory; downstream re-evaluation | `ForgetMemory` + `FlagDerivativesForReEvaluation`; `re_evaluation_flags` table. | **implemented** |
| **Supersede** | Knowledge replace; proposal if Owner-set | `fact_supersessions` table. No proposal gating for owner-set. | **partial** |
| **Tombstone** | Hard-delete record; proposal required | Table `tombstones`. No full flow with proposal. | **partial** |
| **Cascade proposals on entity lifecycle** | Auto-decline when entity archived/tombstoned | `store.ArchiveEntity`, `ForgetMemory`, `TombstoneEntity` call `AutoDeclineProposalsForEntity`. | **implemented** |

### 6.4 Autonomy Model

| Feature | Design | Implementation | Status |
|---------|--------|----------------|--------|
| **Global preset** | Conservative, Balanced, High Autonomy | `config.AutonomyConfig`: GlobalPreset, PerDomainOverrides. `EffectivePreset(domain)` used by governor’s `ApplyAutonomy`. | **implemented** |
| **Per-domain overrides** | Domain-specific autonomy | Config and resolver support; DomainForSkill in NAVI config. | **implemented** |
| **Four dimensions** | Execution Threshold, Insight Surfacing, Memory Promotion, Plugin Invocation | PresetToDimensions in config maps preset to dimensions; design’s four dimensions not all exposed as separate knobs. | **partial** |
| **Hard floors** | System policy, owner-set, proposal-required, irreversible, risk override | `governor/autonomy.go`: HardFloorReason checks requires_confirmation, reversibility, risk_tier. ApplyAutonomy does not upgrade when hard floor present. | **implemented** |
| **Governor vs autonomy** | Governor = what is permitted; Autonomy = how much without asking | Governor enforces budgets (action, retry, cost, duration); autonomy only applied in validation path (RequiresConfirmation → Approved when high preset and no hard floor). | **partial** |

---

## 7. Tier 3 (Data Integrity and Failure) — Summary

- **Proposal Queue:** Schema and API implemented; lifecycle implemented: auto-expiry on list, supersede on SaveProposal, cascade on archive/tombstone via lifecycle helpers; re-evaluate-on-approve in ExecuteApprovedProposal.
- **Provenance:** Entity-level provenance table and schema with reinforcement_count; confidence propagation not implemented.
- **Failure model:** Taxonomy and execution outcome records implemented; compensation and Compose failure semantics stubbed; degradation visibility not wired to UX.
- **Autonomy:** Presets and hard floors implemented; full four-dimension and per-domain behavior aligned to design is partial.

---

## 8. High-Impact Recommendations

1. **World Model write boundary:** Restrict direct store writes to a single “Cognitive” facade (orchestrator + NAVI + reflection workers) so that the design rule “only Cognitive writes World Model” is enforced and auditability is clear.
2. **History entity:** ~~Introduce a unified History view or table.~~ Done: the store exposes a `history` view (directive_messages + events + execution_outcomes) with `class` and `fidelity`; docs updated.
3. **Proposal lifecycle:** ~~Implement auto-expiry and supersede logic for proposals; implement cascade auto-decline when an affected entity is archived or tombstoned.~~ Done: ExpireProposals on list, SupersedeOldProposals on SaveProposal, AutoDeclineProposalsForEntity in lifecycle.
4. **Reflection tiers:** Implement Consolidation and Deep reflection pipelines with tier-appropriate mutation permissions and proposal creation when touching owner-set/explicit state.
5. **Constraint feedback loop:** ~~When validation rejects, pass constraint context back into a dedicated replan/Decide path.~~ Done: rejection returned as tool result to LLM with Replan instruction.
6. **Compose and compensation:** Implement a Compose runner that applies reversibility and compensation rules and call RunCompensation (or equivalent) when compensation_required is true.
7. **Execution outcome coverage:** Ensure every command attempt (including from NAVI loop tool calls and connector actions) writes an execution outcome record so that “failure is first-class” holds everywhere.
