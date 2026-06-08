# ICS Field Ownership and Mutation Rules
**Status:** Draft Normative  
**Applies to:** Conscious-loop ICS artifacts, field ownership, mutation boundaries, replay behavior, and persistence rules.
**Related corpus:** [ICS docs index](inference-control-system/INDEX.md)

## 1. Purpose

This document defines exactly which stage of the Inference Control System is allowed to create, write, narrow, or observe each field in the canonical control artifacts.

This document exists to remove interpretive ambiguity. If a field is mutated by a stage that does not own it, the implementation is non-compliant.

This document is normative.

## 2. Canonical artifacts

The canonical conscious-loop artifacts are:

- `InferenceInput`
- `DecisionSynthesis`
- `DecisionEnvelope`
- `ExecutionSnapshot`

`DecisionEnvelope` is the canonical persisted runtime control artifact.

## 3. Lifecycle stages

The only recognized conscious-loop stages are:

1. `AssembleInput`
2. `Decide`
3. `Validate/Govern`
4. `PrepareModelCall`
5. `CallModel`
6. `AuthorizeModelResponse`
7. `ExecuteAuthorizedAction`
8. `ObserveOutcome`
9. `PersistState`
10. `ResumeReplay`

No other stage may claim ownership of ICS control fields.

## 4. Ownership rules by artifact

## 4.1 `InferenceInput`

### Owner
`AssembleInput`

### Fields owned by `AssembleInput`
- `Version`
- `NCOS`
- `GoalStack`
- `PlanState`
- `PreviousFocus`
- `Posture`
- `Recovery`
- `Governance`
- `Session`
- `Runtime`
- `Capabilities`
- `RelevantContext`
- `Extensions`

### Allowed operations
- create
- populate
- normalize
- carry forward prior persisted context as input state

### Forbidden operations
- choosing action
- authorizing action
- emitting runtime authority
- creating proposal
- issuing permit
- mutating execution result

## 4.2 `DecisionSynthesis`

### Owner
`Decide`

### Fields owned by `Decide`
- `Focus`
- `Mode`
- `Candidate`
- `Rationale`
- `OccurredAt`

### Allowed operations
- create
- replace as a whole
- derive declarative control interpretation

### Forbidden operations
- proposal persistence
- runtime disposition assignment
- model directive assignment
- tool permit assignment
- authorized tool emission
- pending tool state assignment
- last result assignment

## 4.3 `DecisionEnvelope`

`DecisionEnvelope` is stage-partitioned. Ownership is by field group, not by artifact as a whole.

### Envelope fields written only by `Decide`
- `Rationale`

### Envelope fields written only by `Validate/Govern`
- `GovernanceResult`
- `Proposal`
- `RuntimeDisposition`
- `ExecutionBoundary`
- `ReplyMessage` when blocking or pausing is caused by governance

### Envelope fields written only by `PrepareModelCall`
- `ModelDirective`

### Envelope fields written only by `AuthorizeModelResponse`
- `AuthorizedToolCalls`
- `AuthorizedTools`
- `ToolPermit`
- `PendingToolCall`
- `PendingToolPermit`
- `Proposal` when authorization creates an approval boundary
- `RuntimeDisposition` when authorization causes pause or block
- `ReplyMessage` when authorization causes pause or block

### Envelope fields written only by `ObserveOutcome`
- `LastResult`
- post-execution updates within `Rationale`, including:
  - `PlanGraph`
  - `RecoveryCheckpoint`
  - `GoalStack`
  - `DominantMode`
  - `SubmodeChain`
  - `RequiredApprovals`
  - `ChosenAction`
  - `ExecutionIntent`
  - `Governance`
  - `Rejection`
  - `ReflectionHooks`
  - `DecisionTrace`
  - `Focus` when outcome supervision changes mode/focus semantics

### Envelope fields written only by `PersistState`
No semantic fields may be changed by `PersistState`.
`PersistState` may only serialize, store, version, and reload the current envelope.

### Envelope fields written only by `ResumeReplay`
No semantic fields may be changed by `ResumeReplay`.
`ResumeReplay` may only reload persisted envelope/checkpoint state and re-enter the normal lifecycle.

## 4.4 `ExecutionSnapshot`

### Owner
`ExecuteAuthorizedAction` creates the observed snapshot.
`ObserveOutcome` consumes it but does not rewrite its factual contents.

### Fields owned by execution observation
- `Outcome`
- `FailureClass`
- `ApprovalOutcome`
- `ProposalID`
- `ExecutedCapability`
- `CommandType`
- `CheckpointRef`
- `Summary`

### Forbidden operations
- retroactive approval synthesis
- proposal creation
- permit mutation
- execution-boundary widening

## 5. Declarative vs authoritative fields

This distinction is mandatory.

## 5.1 Declarative fields
These fields may express intent, reasoning, or projected control expectations, but they do not grant runtime authority by themselves.

Declarative fields include:
- `Rationale`
- `GovernanceHandoff`
- `ExecutionIntent`
- `RequiredApprovals`
- `RecoveryCheckpoint`
- `ReflectionHooks`
- `DecisionTrace`

## 5.2 Authoritative fields
These fields grant, constrain, or deny runtime authority.

Authoritative fields include:
- `GovernanceResult`
- `Proposal`
- `RuntimeDisposition`
- `ExecutionBoundary`
- `ModelDirective`
- `AuthorizedToolCalls`
- `AuthorizedTools`
- `ToolPermit`
- `PendingToolCall`
- `PendingToolPermit`

### Hard rule
Declarative fields must not be treated as executable authority without the corresponding authoritative field being emitted by the stage that owns it.

## 6. Field mutation rules by stage

## 6.1 `AssembleInput`
May:
- populate `InferenceInput`

Must not:
- mutate `DecisionEnvelope`
- emit permits
- create proposals

## 6.2 `Decide`
May:
- create `DecisionSynthesis`
- write `DecisionEnvelope.Rationale` when synthesis is wrapped into a default envelope

Must not:
- set `RuntimeDisposition`
- set `Proposal`
- set `GovernanceResult`
- set `ModelDirective`
- set `AuthorizedTools`
- set `ToolPermit`
- set `PendingToolCall`
- set `LastResult`

## 6.3 `Validate/Govern`
May:
- replace `GovernanceResult`
- set `RuntimeDisposition`
- set or clear `ExecutionBoundary`
- set `Proposal`
- set `ReplyMessage`
- fail closed
- narrow allowed capabilities
- block executable continuation

Must not:
- call the model
- execute a tool
- emit `ModelDirective`
- emit tool permits
- set `LastResult`

## 6.4 `PrepareModelCall`
May:
- set `ModelDirective`
- clear stale authorization fields before shaping the next model boundary

Must not:
- create proposal
- widen allowed tools beyond `ExecutionBoundary`
- set `AuthorizedTools`
- set `PendingToolCall`
- set `LastResult`

## 6.5 `AuthorizeModelResponse`
May:
- clear stale authorization fields before processing a new model response
- set `AuthorizedToolCalls`
- set `AuthorizedTools`
- set `ToolPermit`
- set `PendingToolCall`
- set `PendingToolPermit`
- set `Proposal`
- set `RuntimeDisposition`
- set `ReplyMessage`
- update `GovernanceResult` to reflect the latest governing authorization result

Must not:
- execute tools
- widen tool surface beyond `ModelDirective`
- authorize a tool with missing contract
- authorize a tool with missing permit semantics

## 6.6 `ExecuteAuthorizedAction`
May:
- consume `AuthorizedToolInvocation`
- produce `ExecutionSnapshot`

Must not:
- mutate `DecisionEnvelope` semantic fields directly
- create proposal
- create permit
- reinterpret contract meaning
- widen authorized tool scope

## 6.7 `ObserveOutcome`
May:
- set `LastResult`
- update post-execution control state in `Rationale`

Must not:
- change historical execution facts in `ExecutionSnapshot`
- fabricate approval
- reopen blocked authority without re-entering normal lifecycle stages

## 6.8 `PersistState`
May:
- serialize envelope
- store envelope
- append history
- attach state version metadata

Must not:
- change semantic contents

## 6.9 `ResumeReplay`
May:
- reload persisted envelope
- reload pending proposal/tool state
- reconstruct the next input state

Must not:
- directly authorize execution
- directly execute pending tools
- convert persisted proposal presence into executable authority

## 7. Clear vs preserve rules

Each stage must know which fields it clears before proceeding.

## 7.1 Before `PrepareModelCall`
The following must be cleared because they are per-response/per-cycle authorization artifacts:
- `AuthorizedToolCalls`
- `AuthorizedTools`
- `ToolPermit`

## 7.2 Before `AuthorizeModelResponse`
The following must be cleared because they are tied to one specific model response:
- `AuthorizedToolCalls`
- `AuthorizedTools`
- `ToolPermit`
- `PendingToolCall`
- `PendingToolPermit`
- `Proposal` created by a previous tool-authorization boundary

## 7.3 Before `ObserveOutcome`
No authoritative pre-execution field may be widened or synthesized.
Outcome supervision may only reconcile state based on actual observed result.

## 8. Narrowing rules

Narrowing is allowed. Widening is forbidden unless the lifecycle restarts from `AssembleInput`.

### Allowed narrowing examples
- `Validate/Govern` may reduce `ExecutionBoundary.AllowedCapabilities`
- `PrepareModelCall` may reduce `ModelDirective.ToolNames`
- `AuthorizeModelResponse` may reject some or all tool attempts and emit a smaller authorized set

### Forbidden widening examples
- runtime adds a tool not present in `ModelDirective.ToolNames`
- executor runs a capability not present in `AuthorizedTools`
- resume path executes pending tool without re-authorization

## 9. Persistence and replay rules

## 9.1 Persisted envelope semantics
A persisted envelope is control memory, not durable authority.

### Hard rules
- persisted `Proposal` does not grant executable authority
- persisted `AuthorizedTools` do not grant replay authority unless revalidated in the current cycle
- persisted `PendingToolCall` must re-enter authorization before execution
- persisted `LastResult` is historical observation only

## 9.2 Replay semantics
On replay/resume:
- reload checkpoint
- reload envelope
- rebuild `InferenceInput`
- re-enter lifecycle
- re-authorize pending tool if one exists
- fail closed if contract or permit is missing or stale

## 10. Proposal and permit ownership rules

### Proposal ownership
Only:
- `Validate/Govern`
- `AuthorizeModelResponse`

may create proposal-backed pause states.

### Permit ownership
Only:
- `AuthorizeModelResponse`

may emit `ToolPermit` and `AuthorizedToolInvocation`.

### Execution ownership
Only:
- `ExecuteAuthorizedAction`

may consume a `ToolPermit`.

## 11. Compliance failures

The following are automatic non-compliance:

- `Decide` writes `RuntimeDisposition`
- `Decide` writes `Proposal`
- `PrepareModelCall` emits `AuthorizedTools`
- `ExecuteAuthorizedAction` creates a proposal
- `PersistState` mutates semantic fields
- `ResumeReplay` executes a pending tool directly
- runtime executes without matching `ToolPermit`
- runtime executes without matching `ToolContract`
- any stage widens the tool surface after governance narrowing

## 12. Audit checklist

A reviewer must be able to answer all of the following with “yes”:

- Is each field owned by exactly one stage?
- Is each mutation performed only by the owning stage?
- Are declarative fields never treated as authority by themselves?
- Are persisted artifacts replayed only through the lifecycle?
- Are proposals and permits owned only by the allowed stages?
- Does runtime fail closed on missing authority artifacts?

If any answer is “no”, the branch is non-compliant.

---

[ICS docs index](inference-control-system/INDEX.md) · [ICS architectural contract](ics-architectural-contract.md) · [docs INDEX](../INDEX.md)
