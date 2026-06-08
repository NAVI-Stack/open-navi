# ICS End-to-End Control Flow Specification

**Status:** Draft Normative
**Applies to:** Conscious-loop control from input assembly through reasoning, governance, model shaping, tool authorization, execution, pause/resume, and outcome supervision.
**Related corpus:** [ICS docs index](inference-control-system/INDEX.md)

## 1. Purpose

The Inference Control System is the authoritative conscious-loop control plane for NAVI.

Its job is to take assembled runtime context, synthesize a decision, validate that decision against governance, constrain the model/tool boundary, authorize concrete runtime actions, supervise the observed outcome, and persist the resulting control state for replay and continuation. The current branch already implements that broader lifecycle through the controller interface and runtime loop.  

This document is normative. If implementation behavior conflicts with this document, the implementation is wrong.

## 2. Canonical control lifecycle

The only valid conscious-loop lifecycle is:

`AssembleInput -> Decide -> Validate/Govern -> PrepareModelCall -> CallModel -> AuthorizeModelResponse -> ExecuteAuthorizedAction -> ObserveOutcome -> PersistState`

The branch currently exposes this lifecycle through the `DecisionController` interface and uses it inside the runtime loop.  

### Forbidden shortcuts

The following transitions are invalid:

* `Decide -> Execute`
* `Decide -> ProposalPersist`
* `CallModel -> Execute` without ICS authorization
* `ProposalApproved -> Execute` without ICS re-authorization
* `Executor -> ProposalPersist`
* `Skill/Plugin/Connector -> GovernanceOutcome`
* `Connector -> PolicyDecision`
* `Runtime -> widen tool surface after ICS has narrowed it`

## 3. Canonical artifacts

This spec adopts the artifact model the branch actually uses now.

### 3.1 `InferenceInput`

`InferenceInput` is the canonical assembled input to one ICS cycle.

It is owned only by runtime input assembly.

It contains:

* NCOS request
* goal stack
* prior plan state
* prior focus
* posture
* recovery state
* governance state
* session context
* runtime context
* visible capabilities
* relevant context references
* extensions

That is the implemented contract in `internal/navi/inference/types.go`. 

### 3.2 `DecisionSynthesis`

`DecisionSynthesis` is the output of `Decide`.

It is owned only by decision synthesis.

It may contain:

* focus selection
* mode selection
* candidate evaluation
* rationale
* decision timestamp

That is the current implemented shape returned by `Decide`.  

### 3.3 `DecisionEnvelope`

`DecisionEnvelope` is the canonical governed runtime control artifact.

It is the only artifact that runtime may persist and replay as the active ICS control state.

It currently contains:

* `Rationale`
* `GovernanceResult`
* `Proposal`
* `RuntimeDisposition`
* `ExecutionBoundary`
* `ModelDirective`
* `AuthorizedToolCalls`
* `AuthorizedTools`
* `ToolPermit`
* `PendingToolCall`
* `PendingToolPermit`
* `LastResult`
* `ReplyMessage`

That is the implemented envelope in the branch today. 

### 3.4 `ExecutionSnapshot`

`ExecutionSnapshot` is the canonical observed execution outcome artifact.

It is owned only by runtime execution observation and post-execution supervision.

It contains:

* execution outcome
* failure class
* approval outcome
* proposal ID when relevant
* executed capability
* command type
* checkpoint ref
* summary

That is the implemented shape in the branch. 

## 4. Stage ownership

Every field and decision in the lifecycle must have one owner.

### 4.1 AssembleInput

Owner: runtime assembly

Responsibilities:

* load run state
* load checkpoint state
* load prior persisted envelope
* derive current governance state from prior envelope and runtime state
* derive current recovery state
* derive goal stack and visible capabilities
* construct `InferenceInput`

Must not:

* choose action
* authorize execution
* create proposal
* emit tool permits

The runtime loop currently builds `InferenceInput` this way in `runtime_inference.go`. 

### 4.2 Decide

Owner: ICS decision synthesis

Responsibilities:

* focus arbitration
* dominant mode routing
* candidate generation and selection
* rationale construction
* declarative execution intent construction
* declarative governance handoff construction
* declarative recovery/reflection scaffolding

Must not:

* persist proposals
* set runtime disposition
* authorize tool calls
* issue permits
* mark a candidate as executable authority
* execute model or tool calls

The branch already keeps proposal persistence and runtime dispositions out of `Decide`, but `Decide` still builds broad declarative artifacts like `ExecutionIntent`, `GovernanceHandoff`, and `RequiredApprovals`. That is allowed only as declarative synthesis, not runtime authority. 

### 4.3 Validate/Govern

Owner: ICS governance validation

Responsibilities:

* validate the selected governed boundary
* run deterministic validation ordering
* synthesize governance outcome
* persist proposals when approval is required
* fail closed when no governed capability survives
* emit the governed `DecisionEnvelope`
* set `RuntimeDisposition`
* set `ExecutionBoundary`
* set `GovernanceResult`
* attach `Proposal` when one is created

Must not:

* call the model
* execute tools
* widen capability boundaries after validation
* mutate the world model except proposal persistence

The branch currently implements candidate/tool validation and proposal persistence in `validate_govern.go`, and final envelope shaping in `controller_authority.go`.  

### 4.4 PrepareModelCall

Owner: ICS model-call shaping

Responsibilities:

* consume the governed envelope
* derive the exact `ModelDirective`
* narrow visible tools to the governed boundary
* require authoritative executable tool contracts for surfaced executable tools
* block the run if the surfaced tool set is unsafe or incomplete

Must not:

* execute anything
* create proposals
* widen the tool surface beyond the execution boundary
* silently allow executable tools with missing contracts

The branch already does this in `PrepareModelCall` and `BuildPreparedModelDirective`. 

### 4.5 CallModel

Owner: runtime model invocation

Responsibilities:

* compile the prepared request
* call the selected model
* normalize the model response

Must not:

* treat model tool calls as executable
* bypass ICS authorization on returned tool calls

The runtime loop currently compiles, calls, normalizes, then re-enters ICS through `AuthorizeModelResponse`. 

### 4.6 AuthorizeModelResponse

Owner: ICS response authorization

Responsibilities:

* inspect normalized model response
* reject out-of-bound tool calls
* require execution metadata for each tool attempt
* require an authoritative `ToolContract` for each surfaced executable tool
* validate each concrete tool attempt through governance
* emit `AuthorizedToolInvocation` and `ToolPermit`
* pause for proposal when authorization requires approval
* block when authorization fails

Must not:

* execute the tool call directly
* authorize a tool call with no contract
* allow a tool outside the governed tool boundary
* silently downgrade authorization failures

The branch already does this in `AuthorizeModelResponse`, including out-of-bound rejection, missing-contract rejection, per-tool validation, proposal pause, and permit emission. 

### 4.7 ExecuteAuthorizedAction

Owner: runtime executor

Responsibilities:

* execute only a tool call that has a matching permit and contract
* fail closed on permit mismatch
* fail closed on missing contract
* emit structured execution result
* return pause state if a proposal boundary is encountered during execution continuation

Must not:

* create proposals
* synthesize governance outcomes
* reinterpret the contract
* widen capabilities
* execute raw model tool calls

The runtime executor currently checks for matching permit and contract before execution and blocks fail-closed otherwise. 

### 4.8 ObserveOutcome

Owner: ICS outcome supervision

Responsibilities:

* reconcile actual execution outcome into the active envelope
* update plan graph
* update recovery checkpoint
* update goal stack
* update post-outcome control state
* update reflection hooks
* update decision trace
* preserve prior governance result as part of the supervised envelope

Must not:

* fabricate authorization
* retroactively approve execution
* mutate historical execution facts

The branch currently performs this in `ObserveOutcome`. 

### 4.9 PersistState

Owner: runtime persistence

Responsibilities:

* persist `DecisionEnvelope`
* append ICS history
* attach persisted ICS state to run/checkpoint state
* reload persisted envelope on future cycles

Must not:

* treat persisted state as durable approval authority
* skip re-authorization on resume

The branch currently persists and reloads ICS state through `runtime_inference.go`. 

## 5. Stage-owned fields inside `DecisionEnvelope`

`DecisionEnvelope` is allowed to be broad, but ownership must be explicit.

### Written only by `Decide`

* `Rationale`

### Written only by `Validate/Govern`

* `GovernanceResult`
* `Proposal`
* `RuntimeDisposition`
* `ExecutionBoundary`
* `ReplyMessage` when the disposition is block or pause

### Written only by `PrepareModelCall`

* `ModelDirective`

### Written only by `AuthorizeModelResponse` / `AuthorizeToolCall`

* `AuthorizedToolCalls`
* `AuthorizedTools`
* `ToolPermit`
* `PendingToolCall`
* `PendingToolPermit`
* `Proposal` when a tool authorization path opens an approval boundary
* `ReplyMessage` when model-response/tool authorization blocks or pauses

### Written only by `ObserveOutcome`

* `LastResult`
* post-execution updates inside `Rationale`

Any other stage writing those fields is architectural drift.

## 6. Runtime dispositions

`RuntimeDisposition` is a closed enum with fixed semantics.

### `call_model`

Meaning:

* runtime may call the model under the current `ModelDirective`

### `pause_for_proposal`

Meaning:

* runtime must stop
* runtime must persist checkpoint state
* runtime must persist envelope state
* runtime must preserve pending proposal and pending tool state when present

### `block_with_reply`

Meaning:

* runtime must stop
* runtime may return a user-visible explanation
* no execution may proceed

### `proceed_direct`

Meaning:

* runtime may directly execute an already-authorized tool invocation without another model call

These are the actual dispositions the branch uses. 

## 7. Governance validation pipeline

The branch currently declares a fixed validation order and then adds follow-on execution-scope checks for certain tool attempts. The spec must reflect the full pipeline, not just the first five stages. 

The normative validation pipeline is:

1. permissions
2. policy
3. configuration
4. priority alignment
5. risk assessment
6. execution-scope checks
7. explicit-confirmation checks

### Execution-scope checks

These include:

* target-path/workspace checks
* execution-boundary/tool-boundary checks
* contract completeness checks

### Explicit-confirmation checks

These include:

* tool contract requires confirmation
* action requires an already-approved proposal ID
* approval must be rechecked on resumed execution

No hidden post-validation checks are allowed outside this declared pipeline.

## 8. Model and tool contract layer

This layer must be first-class in the spec.

### 8.1 `ModelDirective`

`ModelDirective` is the exact model-call boundary for one cycle.

It defines:

* whether tool calls are allowed
* which tool names are allowed
* optional tool choice
* authoritative tool contracts for surfaced executable tools

### 8.2 `ToolContract`

`ToolContract` is the authoritative execution contract for one surfaced executable tool.

It defines:

* contract identity
* tool identity
* execution kind
* command type
* domain
* actor kind
* workspace action
* target path semantics
* confirmation requirement
* optional skill name

### 8.3 `ToolPermit`

`ToolPermit` is the exact authorization for one concrete tool call.

It binds:

* a specific contract
* a specific tool call ID
* a specific tool name
* an approved proposal ID when required
* an optional resolved target path

### 8.4 `AuthorizedToolInvocation`

`AuthorizedToolInvocation` is the only executable tool-invocation artifact.

Runtime must not execute any tool call unless a matching `AuthorizedToolInvocation` and `ToolPermit` exist in the current envelope. The current branch already enforces permit/contract matching at execution time.  

## 9. Proposal, pause, and resume semantics

### 9.1 Proposal creation

Only governance/authorization stages may create proposals.

Candidate-level proposals and tool-attempt proposals must be persisted through ICS-owned validation logic. The current branch does this in `ValidateCandidate` and `ValidateToolAttempt`. 

### 9.2 Pause semantics

When approval is required:

* create proposal
* set `RuntimeDisposition = pause_for_proposal`
* persist proposal in envelope
* persist checkpoint
* persist pending tool state when relevant
* return paused run result

The runtime loop already follows this pattern. 

### 9.3 Resume after approval

Approval does not imply direct execution.

On resume after approval:

* reload the persisted envelope
* reconstruct the pending tool attempt
* re-enter ICS authorization
* re-authorize the concrete tool attempt
* execute only if the attempt is authorized again

The current branch already does re-authorization on resume instead of blind execution. 

### 9.4 Resume after decline

On decline:

* do not execute pending tool
* emit rejected-pre-execution outcome
* supervise that outcome through `ObserveOutcome`
* continue through normal post-outcome control flow

The runtime loop currently models this. 

## 10. Outcome supervision semantics

`ObserveOutcome` must map actual outcomes back into control state.

It updates:

* plan node status
* plan checkpoint refs
* plan status
* recovery openness and route
* pending blockers
* pending approvals
* resume conditions
* goal stack promotion/retirement/resume
* next dominant mode
* next candidate type
* post-outcome governance and intent
* reflection hooks
* decision trace

The branch already does this in `outcome_supervisor.go`. 

## 11. Persistence and replay rules

Because the branch persists ICS state, replay behavior must be explicit.

### Hard rules

* persisted envelopes are historical control state, not permanent execution authority
* persisted proposals do not grant executable authority on their own
* pending tool calls must be re-authorized after reload
* missing permit or contract on replay must fail closed
* `LastResult` is observational only and must not be used as execution authority
* restored governance state is contextual memory, not a bypass

The current runtime already reloads and supervises persisted state, but the spec must make these replay constraints explicit. 

## 12. Capability-surface rules

Skills, plugins, and connectors remain downstream capability surfaces.

They may:

* expose metadata
* implement execution handlers
* return structured results

They must not:

* decide whether they should be used
* synthesize policy outcomes
* create proposals
* bypass permits
* write world-model state directly as part of execution authority

This is consistent with the canonical skills constraints and the ICS control split. 

## 13. Required compliance tests

A branch is non-compliant if any of these are missing or failing.

### Input assembly

* input assembly must not create proposals
* input assembly must not emit permits
* input assembly must not choose action

### Decide

* `Decide` must not persist proposals
* `Decide` must not set runtime disposition
* `Decide` must not authorize tools
* `Decide` must not mutate execution authority fields in the envelope

### Validate/Govern

* validation order must be deterministic
* proposal persistence must happen only through ICS validation
* failed validation must fail closed
* no executable capability may survive without governed approval

### PrepareModelCall

* model directive must not widen tool surface
* surfaced executable tools must have valid contracts
* missing executable contract must block

### AuthorizeModelResponse

* out-of-bound tool calls must be rejected
* missing attempt metadata must be rejected
* missing contract must be rejected
* approval-required tool attempts must pause rather than execute

### ExecuteAuthorizedAction

* runtime must reject execution without matching permit
* runtime must reject execution without matching contract
* runtime must not create proposals
* runtime must fail closed on mismatched proposal approval state

### ObserveOutcome

* outcome supervision must not invent approval
* post-outcome control state must be derived from actual snapshot
* persisted envelope must reflect supervised result, not pre-execution assumptions

### Pause and resume

* approved proposal resume must re-authorize before execute
* declined proposal resume must not execute
* persisted pending tool state must round-trip cleanly through checkpoint reload

## 14. Merge criteria

A branch is compliant only if all are true:

* one end-to-end conscious-loop control lifecycle exists
* no model tool call executes without ICS authorization
* no tool executes without matching permit and contract
* proposal creation is ICS-owned only
* pause and resume remain ICS-mediated
* persisted ICS state fails closed on replay
* outcome supervision updates control state after execution
* skills/plugins/connectors remain downstream capability surfaces only

## 15. Placement

This document lives at:

`docs/architecture/ics-end-to-end-control-flow.md`

`ics-architectural-contract.md` should be revised so it no longer claims a narrower artifact split than the branch actually implements today. Right now the branch reality is envelope-based, so the contract should either acknowledge that or the implementation needs a later refactor to match the stricter three-artifact model.

---

[ICS docs index](inference-control-system/INDEX.md) · [ICS architectural contract](ics-architectural-contract.md) · [docs INDEX](../INDEX.md)
