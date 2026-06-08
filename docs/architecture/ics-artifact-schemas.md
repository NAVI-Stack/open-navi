# ICS Artifact Schemas
**Status:** Draft Normative  
**Applies to:** Canonical conscious-loop control artifacts, schema-level invariants, and artifact interpretation rules.
**Related corpus:** [ICS docs index](inference-control-system/INDEX.md)

## 1. Purpose

This document defines the canonical artifact layer for the current Inference Control System.

This document exists so the architecture is not only described by lifecycle and ownership rules, but also by explicit control artifacts and their interpretation constraints.

This document is normative.

## 2. Canonical artifact set

The canonical conscious-loop artifact set is:

- `InferenceInput`
- `DecisionSynthesis`
- `DecisionEnvelope`
- `ExecutionSnapshot`

These are the artifact types the current branch is functionally built around. The controller interface consumes `InferenceInput`, emits `DecisionSynthesis`, then progressively transforms the active control state through `DecisionEnvelope`, while runtime execution is observed through `ExecutionSnapshot`.

## 3. `InferenceInput`

## 3.1 Role

`InferenceInput` is the canonical assembled input to one ICS cycle.

It is the only artifact that may carry assembled session, runtime, checkpoint, posture, recovery, governance, and surfaced capability context into `Decide`.

## 3.2 Canonical sections

`InferenceInput` contains:

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

This is the implemented schema surface in the current branch.

## 3.3 Interpretation rules

`InferenceInput` is:
- assembled context
- not authority
- not persistence
- not an execution order
- not an approval grant

### Hard rules
- `InferenceInput` must not be treated as executable authority.
- `InferenceInput` must not be persisted as the active control artifact in place of `DecisionEnvelope`.
- `InferenceInput` may include prior governance/recovery state as context, but that state must still re-enter the lifecycle before further execution.

## 4. `DecisionSynthesis`

## 4.1 Role

`DecisionSynthesis` is the output of `Decide`.

It captures decision synthesis only:
- focus
- mode
- candidate evaluation
- rationale
- decision timestamp

That is the implemented output shape in the current branch.

## 4.2 Canonical sections

`DecisionSynthesis` contains:

- `Focus`
- `Mode`
- `Candidate`
- `Rationale`
- `OccurredAt`

## 4.3 Interpretation rules

`DecisionSynthesis` is:
- the declarative output of reasoning
- not runtime authority
- not a proposal record
- not a permit
- not a persisted execution authorization

### Hard rules
- `DecisionSynthesis` must not be used directly by runtime execution.
- `DecisionSynthesis` must pass through governance and envelope shaping before any model or tool authority is emitted.
- `DecisionSynthesis` may contain declarative constructs like `ExecutionIntent` or `GovernanceHandoff` inside `Rationale`, but those remain declarative until later authoritative fields are emitted.

## 5. `DecisionEnvelope`

## 5.1 Role

`DecisionEnvelope` is the canonical governed runtime control artifact.

It is the only artifact that may be treated as the active persisted control state for one conscious-loop cycle.

That matches the current branch, where runtime persists, reloads, and continues from the envelope.

## 5.2 Canonical sections

`DecisionEnvelope` contains:

- `Rationale`
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
- `LastResult`
- `ReplyMessage`

## 5.3 Interpretation rules

`DecisionEnvelope` is:
- the active control artifact
- stage-owned by field groups
- persistable
- replayable only through lifecycle re-entry

It is not:
- a permanent approval grant
- a bypass around re-authorization
- a blanket execution permit for future cycles

### Hard rules
- runtime may only act on authoritative state contained in the current `DecisionEnvelope`
- persisted `DecisionEnvelope` must not be treated as durable execution authority across cycles without replay/re-entry
- `DecisionEnvelope` may be broad, but each field group must have a single owner stage

## 6. `ExecutionSnapshot`

## 6.1 Role

`ExecutionSnapshot` is the canonical observed execution outcome artifact.

It is emitted by execution observation and consumed by outcome supervision.

## 6.2 Canonical sections

`ExecutionSnapshot` contains:

- `Outcome`
- `FailureClass`
- `ApprovalOutcome`
- `ProposalID`
- `ExecutedCapability`
- `CommandType`
- `CheckpointRef`
- `Summary`

This is the current implemented snapshot surface.

## 6.3 Interpretation rules

`ExecutionSnapshot` is:
- factual observation
- post-execution evidence
- input to supervision

It is not:
- approval
- permission
- execution intent
- a permit
- a substitute for governance

### Hard rules
- `ExecutionSnapshot` must not be mutated into authority.
- `ObserveOutcome` may interpret `ExecutionSnapshot`, but may not rewrite its factual meaning.

## 7. Canonical subordinate artifacts inside the envelope

The current branch also relies on subordinate authoritative artifacts inside `DecisionEnvelope`.

## 7.1 `ExecutionBoundary`

### Role
`ExecutionBoundary` is the authoritative executable boundary emitted after governance.

### Canonical sections
- `AllowedCapabilities`
- `TargetCapability`
- `ToolChoice`
- `FailClosedReason`

### Hard rules
- `ExecutionBoundary` defines the maximum executable scope for the current cycle.
- downstream stages may narrow it
- downstream stages must not widen it

## 7.2 `ModelDirective`

### Role
`ModelDirective` is the exact model-call boundary for one cycle.

### Canonical sections
- `AllowToolCalls`
- `ToolNames`
- `ToolChoice`
- `ToolContracts`

### Hard rules
- executable surfaced tools must have authoritative contracts
- model-visible executable tools must remain within the governed execution boundary
- missing executable contract must fail closed

The current branch already enforces these semantics in model preparation.

## 7.3 `ToolContract`

### Role
`ToolContract` is the authoritative execution contract for one surfaced executable tool.

### Canonical sections
- `ID`
- `ToolName`
- `ExecutionKind`
- `CommandType`
- `Domain`
- `ActorKind`
- `WorkspaceAction`
- `TargetPathArg`
- `WorkspaceScopedPath`
- `RequiresConfirmation`
- `SkillName`

### Hard rules
- surfaced executable tools must not exist without valid contracts
- invalid contracts must be rejected, not tolerated
- runtime must not reinterpret the contract after authorization

## 7.4 `ToolPermit`

### Role
`ToolPermit` is the exact authorization for one concrete tool call.

### Canonical sections
- `ContractID`
- `Contract`
- `ToolCallID`
- `ToolName`
- `ApprovedProposalID`
- `TargetPath`

### Hard rules
- runtime must not execute without matching permit
- permit must bind to the exact tool call being executed
- proposal-gated permits must require matching approved proposal linkage

## 7.5 `AuthorizedToolInvocation`

### Role
`AuthorizedToolInvocation` is the only executable tool invocation artifact.

### Canonical sections
- `ToolCall`
- `Permit`

### Hard rules
- raw model tool calls are not executable
- runtime must consume only `AuthorizedToolInvocation`

## 8. Declarative vs authoritative schema split

This split is mandatory.

## 8.1 Declarative artifacts and fields

Declarative control state includes:
- `DecisionSynthesis`
- `Rationale`
- `GovernanceHandoff`
- `ExecutionIntent`
- `RequiredApprovals`
- `RecoveryCheckpoint`
- `ReflectionHooks`
- `DecisionTrace`

These express reasoning, intent, and projected control interpretation.

### Hard rule
Declarative control state must never be treated as executable authority by itself.

## 8.2 Authoritative artifacts and fields

Authoritative control state includes:
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

These express the actual control authority the runtime may act on.

### Hard rule
Runtime may act only on authoritative control state.

## 9. Artifact persistence rules

## 9.1 Persisted artifact
Only `DecisionEnvelope` is the canonical persisted active control artifact.

## 9.2 Non-persisted authority rule
Persisted envelope state is historical control state, not permanent execution authority.

### Hard rules
- persisted proposal presence does not grant executable authority
- persisted pending tool state does not grant executable authority
- persisted authorized tool state must re-enter authorization/replay rules
- persisted last result is historical observation only

## 10. Artifact replay rules

On replay/resume:
- reload `DecisionEnvelope`
- rebuild `InferenceInput`
- reconstruct pending state as context
- re-enter the lifecycle
- re-authorize pending executable work before execution

### Hard rules
- replay must fail closed on stale or missing permit
- replay must fail closed on stale or missing contract
- replay must fail closed on missing proposal linkage for proposal-gated execution

## 11. Artifact schema invariants

The following invariants are mandatory:

1. `InferenceInput` is input only.
2. `DecisionSynthesis` is decision-only and non-authoritative.
3. `DecisionEnvelope` is the only canonical persisted runtime control artifact.
4. `ExecutionSnapshot` is factual observation only.
5. `ExecutionBoundary` may narrow but not widen downstream.
6. `ModelDirective` may narrow but not widen downstream.
7. executable surfaced tools require valid `ToolContract`.
8. concrete tool execution requires matching `ToolPermit`.
9. raw model tool calls are never executable.
10. persisted artifacts must re-enter the lifecycle before further execution.

## 12. Non-compliance conditions

The implementation is non-compliant if any of the following are true:

- `InferenceInput` is treated as execution authority
- `DecisionSynthesis` is treated as execution authority
- runtime executes without a current `DecisionEnvelope`
- runtime executes raw model tool calls
- runtime executes without matching permit or contract
- persisted envelope state is used as blind replay authority
- declarative fields are treated as authoritative without the corresponding envelope fields

## 13. Relationship to other ICS documents

This document defines the artifact layer.

It is subordinate to:
- `ics-architectural-contract.md`

It supports:
- `ics-end-to-end-control-flow.md`
- `ics-field-ownership-and-mutation-rules.md`
- `ics-compliance-test-bill.md`

If this document conflicts with `ics-architectural-contract.md`, the architectural contract wins.

---

[ICS docs index](inference-control-system/INDEX.md) · [architecture README](README.md) · [docs INDEX](../INDEX.md)
