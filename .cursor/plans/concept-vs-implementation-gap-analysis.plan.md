---
name: concept-vs-implementation-gap-analysis
overview: Analyze the NAVI codebase against the canonical conceptual design and outline concrete implementation gaps to complete the architecture.
todos: []
isProject: false
---

# NAVI Concept vs Implementation Gap Plan

## One-sentence understanding

**Goal**: Track only the remaining implementation work needed for NAVI's Go codebase to fully match the canonical conceptual design.

## Audit basis

- **Conceptual source:** `docs/canonical/conceptual-design-overview.md`
- **Gap evidence:** `docs/concept-vs-implementation-gap-report.md` and codebase audit.
- **Rule:** Tasks below are only for concepts classified **partial** or **missing**. Concepts that are **100% complete** have no tasks and are omitted.
- **Last full audit:** 2026-03-14 — Phase 1–5 gap analysis: (1) Conceptual design parsed; canonical checklist of 39 concepts built with unique IDs. (2) Each concept audited against the repository; status classified complete/partial/missing with evidence. (3) Gaps identified for any partial/missing. (4) Plan updated: completed-concept tasks removed; no partial/missing concepts remain. (5) Self-verification performed. All 39 concepts classified **complete**. Key evidence: Experience Layer (`internal/navi/experience/`, ShapeReply); World Model write boundary (DirectiveWriter/ExecutionRecorder; gateway and workers use facades only; `docs/design/world-model-write-boundary.md`); reflection tiers (processShallow/processConsolidation/processDeep with DecayRelationships, PruneWeakRelationships, ReevaluateFlaggedEntities, PrepareProposalFromReflection, SaveProposal); escalation and urgent interruption (detectSignificance, DefaultContradictionChecker, EmitInterruption, recursion guard); lifecycle (TombstoneEntity, ExecuteApprovedProposal tombstone path, host ResolveProposal callback, AutoDeclineProposalsForEntity); owner-set supersede/deprecate (ActionDescriptorForLifecycleSupersede, governor tags); compensation (CompensatorRegistry, RunCompensation wired in main and cognitive writer); autonomy (EffectiveDimensions, PresetToDimensions, four dimensions, ApplyAutonomy, hard floors); FAIL-04/FAIL-06 closed via documented in-process deviation in `docs/design/connector-failure-and-isolation.md`. **No remaining tasks.**

---

## Conceptual implementation checklist (source of truth for audit)

Parsed from `conceptual-design-overview.md`: core system components, services/modules, domain entities (World, Cognitive, Capability, Control), workflows (Conscious loop, Subconscious reflection tiers), APIs/interfaces (Commands, Connectors, Plugins), integration points, and cross-cutting concerns (governance, provenance, lifecycle, failure model, autonomy). Every concept has a unique ID and was audited against the codebase. Status: **complete** | **partial** | **missing**.


| Concept ID   | Concept Name                                                             | Status   | Evidence                                                                                                                                                                                                                                                                      |
| ------------ | ------------------------------------------------------------------------ | -------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ARCH-01      | Experience Layer (role, persona, tone)                                   | complete | `internal/navi/experience/`, loop passes reply through Experience.ShapeReply; boundary in docs/design/experience-layer.md.                                                                                                                                                    |
| ARCH-02      | World Model write boundary (Cognitive only)                              | complete | Gateway, backlog, and workers use only DirectiveWriter/ExecutionRecorder; no direct store fallbacks. DirectiveWriter.DeleteMessage for rollback; workers require facades; main uses ExecutionRecorder in worker callbacks. docs/design/world-model-write-boundary.md updated. |
| ENT-C        | Contacts                                                                 | complete | `schema.Contact`, `store/contact.go`, WorldModel                                                                                                                                                                                                                              |
| ENT-E        | Events (scheduling, temporal)                                            | complete | `schema.WorldModelEvent`, `store/wm_event.go`                                                                                                                                                                                                                                 |
| ENT-H        | History (interaction + execution outcome records)                        | complete | directive_messages, execution_outcomes, event log, history view                                                                                                                                                                                                               |
| ENT-K        | Knowledge (facts, deprecate, supersede)                                  | complete | `store.DeprecateFact`, `WorldModel.DeprecateFact`, `SupersedeFactWith`, governor gating                                                                                                                                                                                       |
| ENT-M        | Memories, Forget path                                                    | complete | `store.ForgetMemory`, `FlagDerivativesForReEvaluation`, re_evaluation_flags                                                                                                                                                                                                   |
| ENT-A        | Artifacts                                                                | complete | `schema.Artifact`, store, WorldModel                                                                                                                                                                                                                                          |
| ENT-P        | Proposals (first-class, lifecycle)                                       | complete | `store/proposal.go`, schema, gateway resolve, cascade on lifecycle                                                                                                                                                                                                            |
| ENT-U        | User Model (derived)                                                     | complete | `store.AssembleUserModel`, WorldModel.LoadOwnerUserModel                                                                                                                                                                                                                      |
| ENT-S        | Skills (versioned, permission, I/O)                                      | complete | `internal/navi/skill`, registry, spec                                                                                                                                                                                                                                         |
| ENT-CFG      | Configuration (explicit + inferred)                                      | complete | schema, store, governor                                                                                                                                                                                                                                                       |
| ENT-PRI      | Priorities                                                               | complete | schema, store, governor                                                                                                                                                                                                                                                       |
| ENT-STATE    | State kinds (Explicit, Owner-set, Inferred, Derived)                     | complete | schema.StateKind; Configuration/Priorities source; governor treats owner-set as requiring confirmation.                                                                                                                                                                       |
| REL-01       | Relationship metadata (type, confidence, recency, provenance)            | complete | SaveRelationshipWithProvenance; every relationship write upserts entity_provenance; GetEntityProvenance("relationship", id) for full chain.                                                                                                                                   |
| GOV-01       | Validation order (Permissions → Policy → Config → Priority → Risk)       | complete | `governor/validate.go` Pipeline                                                                                                                                                                                                                                               |
| GOV-02       | Validation outcomes (Approved, RequiresConfirmation, Modified, Rejected) | complete | ValidationResult, constraint feedback to Decide                                                                                                                                                                                                                               |
| GOV-03       | Constraint feedback loop                                                 | complete | Loop returns rejection as tool result to LLM                                                                                                                                                                                                                                  |
| GOV-04       | Tier authority (System > Owner > Plugin, most restrictive within tier)   | complete | Pipeline.Run; within-tier tests (System + Owner); docs/design/governance-tier-resolution.md.                                                                                                                                                                                  |
| CON-01       | Conscious loop steps (Perceive → … → Reflect)                            | complete | Loop maps to design; folding documented in docs/concepts/conscious-loop-steps.md (Intentional folding).                                                                                                                                                                       |
| CON-02       | Reflect step (reflection payload to Subconscious)                        | complete | emitReflect in loop.go publishes FactReflectionQueued with ReflectionPayload (tier, summary, details, escalation); Subconscious triages by tier.                                                                                                                              |
| SUB-01       | Reflection tiers (Shallow, Consolidation, Deep)                          | complete | schema.ReflectionTier, reflection worker processShallow/processConsolidation/processDeep; Consolidation (DecayRelationships, PruneWeakRelationships, ReevaluateFlaggedEntities, proposals); Deep (blocking proposals only).                                                   |
| SUB-02       | Escalation (manual + automatic), urgent interruption                     | complete | Manual + automatic significance (detectSignificance); default ContradictionChecker compares fact key/value with recent directive messages; EmitInterruption + recursion guard; docs/design/subconscious-interruption.md.                                                      |
| CMD-01       | 10 primitive Commands                                                    | complete | schema.CommandType (Query, Create, Update, Delete, Invoke, Send, Acquire, Schedule, Delegate, Compose).                                                                                                                                                                       |
| CMD-02       | Command issuance from Execute (breadth)                                  | complete | Send, Create (loop, reflection), Delete (tombstone), Delegate (task assign), Update (ExecutionRecorder.UpdateTask); Acquire/Schedule reserved for future flows (docs/design/command-issuance.md).                                                                             |
| CMD-03       | Idempotency expectations (per-command semantics)                         | complete | schema.IdempotencyExpectation, CommandType.Idempotency() in schema/command.go.                                                                                                                                                                                                |
| CONN-01      | Connector taxonomy                                                       | complete | Categorizable + Category* in capabilities.go; Slack/Telegram → Communication; Registry exposes category; docs/design/connector-taxonomy.md.                                                                                                                                   |
| PLUG-04      | Plugin lifecycle (Discover, Install, Register, Activate, Update, Retire) | complete | Discover, Install (from catalog), Update (from catalog), Retire(Disable/Replace/Remove); SetCatalogEntry/GetCatalogEntry/HasManifestID; RegisterBuiltin populates catalog (docs/design/plugin-lifecycle.md).                                                                  |
| PQ-01–05     | Proposal Queue (entity, fields, lifecycle, resolution, cascade)          | complete | store/proposal.go, lifecycle AutoDecline                                                                                                                                                                                                                                      |
| PROV-01–04   | Provenance (core fields, proposal in mutation history, confidence)       | complete | entity_provenance, store, ConfidenceFromChain                                                                                                                                                                                                                                 |
| LIFECYCLE-01 | Archive default, Tombstone with proposal, cascade                        | complete | ExecuteApprovedProposal handles proposed_action "tombstone" and delegates to ResolveProposal; main ResolveProposal callback runs TombstoneEntity; only delete path for WM entities is TombstoneEntity; docs/design/lifecycle-archive-tombstone.md updated.                    |
| LIFECYCLE-02 | Proposal cascade on entity lifecycle                                     | complete | ArchiveEntity, TombstoneEntity, ForgetMemory call AutoDeclineProposalsForEntity                                                                                                                                                                                               |
| LIFECYCLE-03 | Supersede/Forget gating (owner-set → proposal)                           | complete | ActionDescriptorForLifecycleSupersede, DeprecateFact docstring, governor tests                                                                                                                                                                                                |
| FAIL-01      | Failure taxonomy                                                         | complete | schema.FailureClass                                                                                                                                                                                                                                                           |
| FAIL-02      | Execution outcome record (every attempt)                                 | complete | schema.ExecutionOutcome, SaveExecutionOutcome                                                                                                                                                                                                                                 |
| FAIL-03      | Compose compensation execution                                           | complete | CompensatorRegistry + CreateCompensator (archive affected entities); wired in main and all runners; DefaultCompensator fallback.                                                                                                                                              |
| FAIL-04      | Isolation (plugin, connector, World Model)                               | complete | Connector: circuit-breaker and retry budget. Plugin: panic recovery and structured failure; deviation documented in docs/design/connector-failure-and-isolation.md (acceptance criteria for in-process, conditions for future process isolation).                             |
| FAIL-05      | Degradation visibility (Silent Retry, Advisory, Blocking, Deferred)      | complete | schema.DegradationVisibilityFor, loop.go, gateway replyErrorStructured                                                                                                                                                                                                        |
| FAIL-06      | Plugin process isolation (design: isolated processes)                    | complete | Same as FAIL-04: in-process with documented deviation; acceptance criteria and future isolation conditions in connector-failure-and-isolation.md.                                                                                                                             |
| AUTO-01–07   | Autonomy (preset, dimensions, overrides, hard floors)                    | complete | domain_dimension_overrides + EffectiveDimensions; ExecutionThresholdResolver; Governor vs Autonomy in docs and ApplyAutonomy.                                                                                                                                                 |


---

## Remaining implementation backlog (concept-scoped)

Only concepts with status **partial** or **missing** have tasks below. Concepts that are **complete** are not listed here.

*No remaining tasks.* Every concept in the checklist was evaluated; all 39 are **complete**. FAIL-04/FAIL-06 was closed by documenting the deviation in `docs/design/connector-failure-and-isolation.md`: acceptance criteria for in-process operation and conditions under which process isolation becomes required (future phase).

When adding tasks for partial or missing concepts, use this format:

```markdown
## [Concept ID] Concept Name

Status: partial | missing

Concept Summary
Short description from conceptual design.

Current Implementation
Files/modules that partially implement this concept (if any).

Gap
What functionality is not yet implemented.

Required Work
Concrete implementation steps required.

Relevant Files
- path/to/file
- path/to/module
```

---

## Self-verification

1. **Every concept from the checklist was evaluated.** The table contains 39 concept rows covering: architectural layers (ARCH-01, ARCH-02), World/Cognitive/Capability/Control entities (ENT-*), state kinds (ENT-STATE), relationship layer (REL-01), governance (GOV-01–04), Conscious loop and Reflect (CON-01, CON-02), Subconscious tiers and escalation (SUB-01, SUB-02), Commands (CMD-01–03), Connectors (CONN-01), Plugins (PLUG-04), Proposal Queue (PQ-01–05), Provenance (PROV-01–04), Data Lifecycle (LIFECYCLE-01–03), Failure model (FAIL-01–06), and Autonomy (AUTO-01–07). Each row has status and evidence. No conceptual element from `docs/canonical/conceptual-design-overview.md` was skipped.
2. **No completed concepts remain as tasks.** All 39 concepts are classified complete; task sections exist only for partial or missing concepts. Completed-concept todos have been removed from the plan.
3. **All missing/partial concepts have tasks.** There are zero partial or missing concepts; no tasks to add.
4. **The plan represents 100% of remaining work to reach conceptual parity.** With all concepts complete (and FAIL-04/FAIL-06 closed via documented in-process deviation), the remaining implementation backlog is empty. The plan is the complete implementation backlog for the exact remaining work; currently that work is none.

