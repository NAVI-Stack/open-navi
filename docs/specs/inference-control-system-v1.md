# Inference Control System (ICS) v1

**Status:** Draft
**Layer:** Cognitive Layer
**Purpose:** Control kernel for focus arbitration, reasoning mode selection, plan management, execution supervision, and recovery.
**Depends on:** NCOS, World Model, Governor / Validate-Govern, Capability Layer, Runtime RunState / Checkpoint.  

**Related corpus:** [ICS docs index](../architecture/inference-control-system/INDEX.md), [ICS architectural contract](../architecture/ics-architectural-contract.md), [ICS implementation status](../architecture/ics-v1-implementation-status.md)

## 1. Purpose

The Inference Control System (ICS) is NAVI’s canonical decision-control subsystem inside the Cognitive Layer.
ICS converts contextualized input and current control state into one governed decision artifact that can be validated, executed, recovered, and reflected on.

ICS exists to replace implicit decision behavior spread across loop/runtime code with explicit, structured control contracts. NAVI is a persistent agent, not a chatbot that sometimes calls tools, so its decision system must be durable, inspectable, and resumable.  

## 2. Placement in Architecture

ICS lives **inside the Cognitive Layer**.

* **Experience Layer** may bias presentation, initiative style, and attention weighting, but does not own decision logic. 
* **ICS** owns control: focus, mode selection, candidate evaluation, planning depth, governance handoff, execution supervision, and recovery.
* **World Model** remains the durable source of truth.
* **Capability Layer** remains the execution arm through Commands, Skills, Connectors, and Plugins. 
* **NCOS** remains the structured context/orchestration input seam; ICS does not replace NCOS. 

## 3. Hard Constraints

ICS MUST obey the following:

### 3.1 Skills

Skills are governed capability modules and do not own reasoning, scheduling, or policy. ICS MAY choose a Skill interface, but Skill execution remains a Capability Layer concern.  

### 3.2 Governance

ICS MUST route material action candidates through deterministic validation in the defined order:

1. permissions
2. policy
3. configuration
4. priority alignment
5. risk assessment 

### 3.3 Proposal Queue

If confirmation is required, ICS MUST create or update a Proposal-compatible record and MUST NOT execute past that boundary without resolution. 

### 3.4 Failure Model

ICS MUST treat rejection, failure, timeout, partial execution, compensation, and recovery as first-class states and MUST preserve them through runtime state and history. 

### 3.5 Autonomy

Autonomy MAY bias thresholds, initiative, and confirmation sensitivity, but MUST NOT bypass governance hard floors, irreversible action constraints, or proposal-required categories. 

### 3.6 Runtime compatibility

ICS MUST integrate with existing runtime state objects (`RunState`, `Checkpoint`, `ExecuteInput`, `ExecuteResult`) and SHOULD extend rather than replace them. 

## 4. Core Responsibilities

ICS MUST:

* select the active focus target
* select dominant reasoning mode and chained submodes
* choose planning depth and planning style
* invoke subreasoners selectively
* generate and evaluate decision candidates
* commit one canonical `Rationale`
* compile a governance handoff
* supervise execution through existing runtime state
* maintain recovery checkpoints and recovery cases
* emit structured reflection hooks

## 5. Processing Model

ICS operates as the Cognitive control kernel and runs this canonical cycle:

1. ingest
2. interpret
3. arbitrate focus
4. select mode
5. invoke subreasoners
6. generate candidates
7. evaluate candidates
8. commit decision
9. emit rationale
10. hand off to governance / execution / reflection

This is the operational control form of NAVI’s conceptual loop:
**Perceive → Interpret → Contextualize → Decide → Validate/Govern → Execute → Reflect**. 

## 6. Core Module Contracts

## 6.1 Focus Arbiter

Responsible for selecting the current foreground focus from:

* current user request
* active goals
* pending recovery
* pending proposal
* scheduled trigger
* urgent contradiction
* reflection follow-up

### Contract

**Input:** goal stack, runtime state, contextual input, posture state
**Output:** `FocusFrame`

### MUST

* support ranked focus selection
* support soft and hard preemption
* preserve resume state before non-emergency preemption
* apply anti-thrashing rules

## 6.2 Mode Router

Responsible for selecting:

* dominant decision mode
* chained submodes
* planning depth
* reasoning depth
* pinned subreasoners

### Supported chosen action / dominant mode types

* respond
* clarify
* plan
* execute
* monitor
* defer
* reject
* propose
* recover
* replan

### MUST

* choose one dominant mode per cycle
* allow zero or more chained submodes
* make mode transitions explicit
* support bounded chain depth

## 6.3 Plan Graph Manager

Responsible for explicit, resumable work structure.

### Contract

**Input:** focus frame, mode, candidate action, runtime context
**Output:** `PlanGraph` or updated `PlanGraph`

### MUST

* support adaptive decomposition
* support checkpoints
* represent dependencies and blockers
* support shallow upfront planning and progressive expansion

## 6.4 Candidate Evaluator

Responsible for generating and ranking candidate actions.

### MUST evaluate using:

Primary:

* urgency
* importance
* user explicitness

Secondary:

* dependency readiness
* reversibility
* risk
* staleness
* effort / cost
* continuity value
* interruption pressure

### MUST support:

* veto signals
* thresholds
* candidate rejection summaries
* compact traceable reasoning

## 6.5 Governance Handoff

Responsible for converting the chosen candidate into a deterministic governance input.

### Contract

**Input:** selected candidate, execution context, skill/capability metadata
**Output:** `GovernanceHandoff`

### MUST

* preserve intent
* preserve scope
* preserve target capability
* preserve expected side effects
* preserve reversibility/risk hints
* preserve confirmation requirement status

### MUST NOT

* execute actions itself
* replace Governor logic
* bypass Validate/Govern order

## 6.6 Execution Supervisor

Responsible for supervising execution after governance approval.

### MUST

* bind execution to the existing runtime `RunState`
* compile or update `ExecuteInput`
* checkpoint before risky or irreversible transitions
* dispatch only through approved Commands / Skills / runtime mechanisms
* capture outputs and failures into runtime and trace state

### SHOULD

* unify duplicated execution branching currently spread through `AgentLoop` and runtime executor paths. 

## 6.7 Recovery Manager

Responsible for deciding what to do after degraded, failed, timed-out, or partial work.

### MUST support:

* retry
* compensate
* recover
* replan
* defer
* propose recovery
* leave recovery open when not resolved

### MUST align with:

* existing failure taxonomy
* execution outcome semantics
* compensation / reversibility classes
* proposal-mediated recovery when required 

## 6.8 Decision Trace Emitter

Responsible for producing the canonical audit artifact for each cycle.

### MUST emit:

* focus chosen
* mode chosen
* subreasoners invoked
* candidates considered
* thresholds / veto state
* governance outcome
* execution outcome ref
* recovery / reflection refs

## 7. Core Runtime / State Objects

## 7.1 `InferenceInput`

Unified ICS input contract.

### MUST include:

* contextualized NCOS output
* active goal stack
* posture state
* recovery state
* approvals / governance state
* relevant memory/runtime state
* active user/session/task context
* current capability availability

### MUST NOT directly include:

* raw chain-of-thought
* uncontrolled prompt blobs
* unvalidated memory writes
* direct unnormalized tool output
* policy bypass signals
* mutable identity core

## 7.2 `FocusFrame`

Represents what ICS is attending to now.

### Required fields

* `focus_id`
* `active_goal_id`
* `focus_reason`
* `dominant_mode`
* `submode_chain`
* `priority_score`
* `started_at`
* `preemptible`
* `pinned_subreasoners`

## 7.3 `GoalStack`

Represents ranked foreground and background objectives.

### MUST support

* one active focus goal
* ready goals
* latent goals
* promote
* demote
* suspend
* resume
* merge
* split
* retire
* abandon

## 7.4 `PlanGraph`

Represents structured work.

### Required fields

* `plan_id`
* `goal_id`
* `planning_style`
* `nodes`
* `edges`
* `checkpoints`
* `current_node_id`
* `success_criteria`
* `failure_criteria`
* `status`

### Nodes SHOULD carry

* intent
* target capability / command
* dependencies
* reversibility
* retry policy
* checkpoint boundary

## 7.5 `Rationale`

Canonical final decision artifact.

### MUST include

* selected goal
* dominant mode
* submode chain
* invoked subreasoners
* planning style / depth
* chosen action
* confidence
* veto / threshold status
* required approvals
* execution intent
* recovery checkpoint
* reflection hooks
* compact candidate summary

## 7.6 `CandidateSummary`

### Required fields

* `candidate_id`
* `candidate_type`
* `description`
* `score`
* `supporting_factors`
* `blocking_factors`
* `approval_needed`
* `selection_reason`
* `rejection_reason`

## 7.7 `ExecutionIntent`

Structured handoff to execution.

### Required fields

* `action_type`
* `target`
* `required_inputs`
* `expected_side_effects`
* `reversibility`
* `risk_level`
* `approval_requirement`
* `success_condition`
* `failure_condition`
* `fallback_or_rollback_path`

## 7.8 `RecoveryCheckpoint`

Structured resume state.

### Required fields

* `current_goal_id`
* `current_phase`
* `current_task`
* `current_step`
* `checkpoint_ref`
* `pending_blockers`
* `pending_approvals`
* `resume_conditions`
* `rollback_point`
* `expiry_or_staleness`

### Mapping rule

This SHOULD reuse or reference runtime `Checkpoint` rather than creating a disconnected checkpoint system. 

## 7.9 `ReflectionHookSet`

Structured post-action payload.

### Required fields

* `what_happened`
* `expected_vs_actual`
* `success_failure_state`
* `surprises`
* `lesson_candidate`
* `memory_candidate`
* `proposal_candidate`
* `followup_trigger`

## 8. Subreasoners

Initial subreasoner set:

* interpreter
* planner
* clarifier
* critic
* risk assessor
* uncertainty assessor
* recovery reasoner
* reflection reasoner

## 8.1 Invocation rules

* selective by default
* may be pinned by context
* pinning must decay if not reinforced
* no permanently always-on subreasoners by default

## 8.2 Pinning triggers

* session mode
* active goal type
* dominant mode
* uncertainty pattern
* risk level
* repeated task structure
* interaction history / behavior patterns

## 8.3 Unpinning triggers

* task/context ends
* trigger disappears
* decay timeout
* stronger competing need
* explicit mode shift
* successful resolution

## 8.4 Arbitration

ICS SHOULD use layered disagreement resolution:

1. governance / hard veto first
2. weighted arbitration second
3. executive tie-break third
4. clarify / decompose / defer if still unresolved

## 8.5 Initial veto policy

* risk assessor → soft veto, hard veto above threshold
* uncertainty assessor → soft veto
* critic → advise
* clarifier → route-to-clarify
* recovery reasoner → veto if state is compromised

## 9. Threshold and Uncertainty Model

## 9.1 Threshold model

ICS MUST support:

* hard thresholds
* soft thresholds
* weighted score bands
* context-adjusted thresholds
* governance/policy override thresholds

## 9.2 Uncertainty classes

ICS MUST support typed uncertainty:

* intent
* factual
* procedural
* environmental
* policy
* outcome
* self-state
* operational
* situational

## 9.3 Uncertainty handling flow

1. assess
2. tolerate / reduce / route around / escalate / defer / reject

## 10. Rejection Model

## 10.1 Rejection reasons

* unsafe
* unauthorized
* impossible with current capability
* missing critical constraints
* policy-blocked
* governance-blocked
* self-state compromised
* environment not trustworthy enough

## 10.2 Post-rejection responses

* no explanation
* brief refusal
* brief explanation
* safer alternative
* request missing requirement
* escalate / propose
* defer and monitor
* terminate cleanly

## 10.3 Audit and posture adaptation

ICS SHOULD preserve for rejected, paused, or deferred paths:

* reason
* intent
* affected goal
* checkpoint/state
* blockers
* resume conditions
* expiry/TTL
* follow-up requirement

For repeated harmful or repeated good-faith friction patterns, ICS SHOULD emit structured audit/posture signals rather than ad hoc hidden lists.

## 11. Experience Interaction

Experience MAY modulate:

* initiative threshold
* verbosity
* clarification strictness
* surfacing style
* monitoring aggressiveness
* interruption presentation

Experience MUST NOT override:

* governance requirements
* policy hard floors
* proposal rules
* failure semantics
* audit integrity
* truthfulness / epistemic honesty 

## 12. Integration Boundaries

### 12.1 ICS ↔ NCOS

NCOS provides structured contextualized input.
ICS decides what to do with it. 

### 12.2 ICS ↔ Runtime

ICS SHOULD integrate with `RunState`, `Checkpoint`, `ExecuteInput`, and `ExecuteResult` rather than creating duplicate run semantics. 

### 12.3 ICS ↔ Skills

ICS consumes skill metadata for decision eligibility, but execution remains in the Capability Layer. 

### 12.4 ICS ↔ Governor

ICS emits a structured governance handoff; Governor remains the approving authority. ICS does not replace the governor. 

### 12.5 ICS ↔ Proposal Queue

ICS must create or update Proposal-compatible state when confirmation is required. 

### 12.6 ICS ↔ Failure / Recovery

ICS consumes execution outcome semantics and recovery state defined elsewhere and adds control logic on top. 

## 13. Invariants

ICS MUST preserve:

* no governance bypass
* no direct durable world-model writes by skills
* no silent dropping of failed or partial execution state
* no retry of irreversible work without required approval
* no posture adaptation beyond hard invariants
* no hidden second decision path outside ICS contracts
* no raw hidden-reasoning persistence

## 14. Repo Structure

Recommended root:

* `internal/navi/inference/`

Recommended packages:

* `types.go`
* `interfaces.go`
* `focus/`
* `modes/`
* `goals/`
* `planning/`
* `candidates/`
* `governance/`
* `execution/`
* `recovery/`
* `trace/`
* `state/`

## 15. Acceptance Criteria

### Phase 1 — Control kernel foundation

* [ ] canonical `InferenceInput`, `FocusFrame`, `Rationale`, `ExecutionIntent`, `RecoveryCheckpoint`, and `ReflectionHookSet` contracts exist
* [ ] `internal/navi/inference/` exists
* [ ] one authoritative runtime path consumes ICS for focus + mode selection
* [ ] decision traces are emitted
* [ ] no second hidden decision seam is added

### Phase 2 — Goal / candidate control

* [ ] ranked goal stack exists
* [ ] focus arbitration is explicit
* [ ] candidate generation and scoring are explicit
* [ ] threshold and veto state are represented in code

### Phase 3 — Planning / runtime alignment

* [ ] `PlanGraph` exists
* [ ] adaptive decomposition exists
* [ ] runtime checkpoints are reused or referenced cleanly
* [ ] execution intent compiles into existing runtime execution structures

### Phase 4 — Governance / proposal integration

* [ ] ICS emits a structured governance handoff
* [ ] confirmation-required actions create Proposal-compatible records
* [ ] rejected actions return structured constraint context for replanning

### Phase 5 — Recovery / reflection

* [ ] recovery checkpoints are produced for interruption/failure
* [ ] recovery manager supports retry / compensate / replan / propose
* [ ] reflection hooks are emitted after material decision/execution outcomes

### Phase 6 — Advanced control

* [ ] selective subreasoner pinning exists
* [ ] posture adaptation exists within invariants
* [ ] long-running objectives are durable and resumable
* [ ] focus arbitration avoids thrashing under competing demands

## 16. Open Questions

* exact persistence surface for `GoalStack`, `FocusFrame`, `PlanGraph`, and `DecisionTrace`
* whether `DecisionTrace` should be a World Model entity or adjacent control-log entity
* exact scoring/hysteresis formula for focus arbitration
* exact runtime migration path from `AgentLoop.ProcessTurn`
* exact contract boundaries between ICS and existing runtime coordinator

---

[ICS docs index](../architecture/inference-control-system/INDEX.md) · [specs INDEX](INDEX.md) · [docs INDEX](../INDEX.md)
