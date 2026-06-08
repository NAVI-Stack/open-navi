# ICS Architectural Contract
**Status:** Normative  
**Applies to:** Conscious-loop decision, governance, model shaping, model-response authorization, execution gating, pause/resume, outcome supervision, and persistence.
**Related corpus:** [ICS docs index](inference-control-system/INDEX.md)

## 1. Purpose

The Inference Control System is the authoritative conscious-loop control plane for NAVI.

It exists to enforce a hard separation between:

- input assembly
- decision synthesis
- governance validation
- model-call shaping
- model-response authorization
- execution gating
- outcome supervision
- persistence and replay

This document is normative. If implementation behavior conflicts with this contract, the implementation is wrong.

## 2. Canonical control model

The canonical control model for the current ICS is:

- `InferenceInput` — canonical assembled input for one ICS cycle
- `DecisionSynthesis` — output of decision synthesis only
- `DecisionEnvelope` — canonical governed runtime control artifact
- `ExecutionSnapshot` — canonical observed execution outcome artifact

### Hard rule
`DecisionEnvelope` is the canonical persisted runtime control artifact for the current ICS.

No other artifact may be treated as the authoritative persisted conscious-loop control state.

## 3. Canonical lifecycle

The only valid conscious-loop lifecycle is:

`AssembleInput -> Decide -> Validate/Govern -> PrepareModelCall -> CallModel -> AuthorizeModelResponse -> ExecuteAuthorizedAction -> ObserveOutcome -> PersistState`

### Forbidden shortcuts

The following transitions are invalid:

- `Decide -> Execute`
- `Decide -> ProposalPersist`
- `Validate/Govern -> Execute` without either model preparation or direct authorized execution semantics
- `CallModel -> Execute` without ICS authorization
- `ProposalApproved -> Execute` without ICS re-authorization
- `Executor -> ProposalPersist`
- `Skill/Plugin/Connector -> GovernanceOutcome`
- `ResumeReplay -> Execute` without re-entry through ICS authorization

## 4. Ownership boundaries

## 4.1 `AssembleInput` owns input assembly only

`AssembleInput` may:
- gather session state
- gather checkpoint state
- gather prior persisted ICS state
- derive runtime context
- derive governance context
- derive recovery context
- derive visible capabilities
- emit `InferenceInput`

`AssembleInput` must not:
- choose action
- authorize action
- create proposals
- issue permits
- execute tools
- rewrite prior execution outcomes

## 4.2 `Decide` owns decision synthesis only

`Decide` may:
- arbitrate focus
- select dominant mode
- evaluate candidates
- construct rationale
- express declarative execution intent
- express declarative governance handoff
- express declarative recovery and reflection scaffolding

`Decide` must not:
- persist proposals
- set runtime disposition
- set executable permits
- authorize model tool calls
- emit pending tool state
- execute anything
- treat declarative reasoning as runtime authority

## 4.3 `Validate/Govern` owns governance validation only

`Validate/Govern` may:
- evaluate governance checks
- apply deterministic validation ordering
- narrow the execution boundary
- fail closed
- create and persist proposals
- emit governance result
- emit runtime block or pause
- emit execution boundary

`Validate/Govern` must not:
- call the model
- execute tools
- emit model directives
- emit tool permits
- rewrite observed execution facts

## 4.4 `PrepareModelCall` owns model-call shaping only

`PrepareModelCall` may:
- consume the governed envelope
- derive the exact `ModelDirective`
- narrow the model-visible tool surface
- require authoritative executable tool contracts for surfaced executable tools
- block the run when the surfaced tool set is unsafe or incomplete

`PrepareModelCall` must not:
- create proposals
- authorize tool calls
- execute anything
- widen the executable tool surface beyond the governed execution boundary

## 4.5 `AuthorizeModelResponse` owns model-response authorization only

`AuthorizeModelResponse` may:
- inspect normalized model output
- reject out-of-bound tool calls
- require resolved execution metadata for concrete tool attempts
- require authoritative executable tool contracts
- validate each tool attempt through governance
- emit authorized tool invocations
- emit exact tool permits
- open a proposal-backed pause when approval is required
- block fail-closed when authorization fails

`AuthorizeModelResponse` must not:
- execute tools
- widen tool scope
- authorize tools without permit and contract semantics
- bypass governance for tool attempts

## 4.6 `ExecuteAuthorizedAction` owns execution only

`ExecuteAuthorizedAction` may:
- consume an authorized tool invocation
- execute a tool only when a matching permit and contract exist
- emit `ExecutionSnapshot`

`ExecuteAuthorizedAction` must not:
- create proposals
- synthesize governance outcomes
- widen capability boundaries
- reinterpret or mutate the authorized contract
- treat model output as executable authority by itself

## 4.7 `ObserveOutcome` owns outcome supervision only

`ObserveOutcome` may:
- consume `ExecutionSnapshot`
- reconcile plan graph state
- reconcile recovery state
- reconcile goal-stack state
- reconcile post-execution control state
- reconcile reflection hooks
- reconcile decision trace
- write `LastResult`

`ObserveOutcome` must not:
- fabricate authorization
- fabricate proposals
- retroactively approve execution
- change factual execution history

## 4.8 `PersistState` owns persistence only

`PersistState` may:
- serialize the envelope
- store the envelope
- append ICS history
- attach version metadata
- reload stored ICS state for future cycles

`PersistState` must not:
- mutate semantic contents
- convert persisted state into executable authority
- bypass replay rules

## 5. Canonical artifacts

## 5.1 `InferenceInput`

`InferenceInput` is the canonical assembled input to one ICS cycle.

It includes:
- NCOS request
- goal stack
- plan state
- previous focus
- posture
- recovery state
- governance state
- session context
- runtime context
- visible capabilities
- relevant context references
- extensions

## 5.2 `DecisionSynthesis`

`DecisionSynthesis` is the output of `Decide`.

It includes:
- focus selection
- mode selection
- candidate evaluation
- rationale
- occurrence timestamp

### Hard rule
`DecisionSynthesis` is not executable authority.

## 5.3 `DecisionEnvelope`

`DecisionEnvelope` is the canonical governed runtime control artifact.

It includes:
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

### Hard rule
`DecisionEnvelope` may be broad, but its fields are stage-owned and may only be written by the stages that own them.

## 5.4 `ExecutionSnapshot`

`ExecutionSnapshot` is the canonical observed outcome artifact.

It includes:
- execution outcome
- failure class
- approval outcome
- proposal ID when relevant
- executed capability
- command type
- checkpoint ref
- summary

### Hard rule
`ExecutionSnapshot` is factual observation, not authority.

## 6. Declarative vs authoritative control state

This distinction is mandatory.

## 6.1 Declarative control state
The following are declarative and may express reasoning, intent, or projected control interpretation:

- `Rationale`
- `GovernanceHandoff`
- `ExecutionIntent`
- `RequiredApprovals`
- `RecoveryCheckpoint`
- `ReflectionHooks`
- `DecisionTrace`

### Hard rule
Declarative control state must not be treated as executable authority by itself.

## 6.2 Authoritative control state
The following are authoritative and may grant, constrain, or deny runtime authority:

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
Runtime may act only on authoritative control state.

## 7. Runtime dispositions

`RuntimeDisposition` is a closed enum.

Allowed values:

- `call_model`
- `pause_for_proposal`
- `block_with_reply`
- `proceed_direct`

### `call_model`
Meaning:
- runtime may call the model under the current model directive

### `pause_for_proposal`
Meaning:
- runtime must stop
- runtime must persist proposal/checkpoint/envelope state
- runtime must not execute pending action

### `block_with_reply`
Meaning:
- runtime must stop
- runtime may surface a user-visible explanation
- runtime must not proceed

### `proceed_direct`
Meaning:
- runtime may directly execute an already-authorized tool invocation

No other runtime disposition is valid.

## 8. Execution boundary contract

`ExecutionBoundary` is the authoritative executable boundary emitted after governance.

It defines:
- allowed capabilities
- optional target capability
- optional tool choice
- fail-closed reason

### Hard rules
- runtime must not execute outside `ExecutionBoundary`
- downstream stages may narrow `ExecutionBoundary`
- downstream stages must not widen `ExecutionBoundary`

## 9. Model directive contract

`ModelDirective` is the exact model-call boundary for one cycle.

It defines:
- whether tool calls are allowed
- which tool names are allowed
- optional tool choice
- authoritative tool contracts for executable tools

### Hard rules
- executable surfaced tools must have valid authoritative contracts
- model-visible executable tools must be a subset of the governed execution boundary
- missing executable contract must fail closed

## 10. Tool contract and permit contract

## 10.1 `ToolContract`
`ToolContract` is the authoritative execution contract for one surfaced executable tool.

It defines:
- contract identity
- tool identity
- execution kind
- command type
- domain
- actor kind
- workspace action
- target path semantics
- confirmation requirement
- optional skill name

### Hard rule
A surfaced executable tool without a valid `ToolContract` must not be authorized.

## 10.2 `ToolPermit`
`ToolPermit` is the exact authorization for one concrete tool call.

It binds:
- contract identity
- contract payload
- tool call ID
- tool name
- approved proposal ID when required
- resolved target path when relevant

### Hard rule
Runtime must not execute a tool call unless a matching `ToolPermit` exists.

## 10.3 `AuthorizedToolInvocation`
`AuthorizedToolInvocation` is the only executable tool-invocation artifact.

### Hard rule
Runtime must not execute raw model tool calls. Runtime may execute only `AuthorizedToolInvocation`.

## 11. Governance pipeline

The validation pipeline is closed and ordered.

The normative order is:

1. permissions
2. policy
3. configuration
4. priority alignment
5. risk assessment
6. execution-scope checks
7. explicit-confirmation checks

### Execution-scope checks
These include:
- workspace/path scope checks
- execution-boundary checks
- contract completeness checks

### Explicit-confirmation checks
These include:
- contract-level explicit confirmation requirements
- approved-proposal requirements for resumed execution

No hidden post-validation gates are allowed outside this declared pipeline.

## 12. Proposal ownership

Proposal ownership is exclusive.

### Proposal creation
Only:
- `Validate/Govern`
- `AuthorizeModelResponse`

may create proposal-backed pause states.

### Proposal persistence
Proposal persistence for conscious-loop approval gating is owned only by ICS validation/authorization paths.

### Proposal resolution
Proposal resolution is not execution authority.

### Hard rule
Proposal approval must not cause direct execution. Approved proposal replay must re-enter authorization.

## 13. Pause and resume rules

## 13.1 Pause
When approval is required:
- create proposal
- emit `pause_for_proposal`
- persist envelope
- persist checkpoint
- preserve pending tool state when relevant

## 13.2 Resume after approval
When a proposal is approved:
- reload persisted state
- reconstruct pending tool state
- re-enter authorization
- re-authorize before execution
- fail closed if permit/contract/proposal linkage is stale or invalid

## 13.3 Resume after decline
When a proposal is declined:
- do not execute pending tool
- emit rejected-pre-execution outcome
- supervise the outcome through `ObserveOutcome`
- continue through the normal lifecycle

## 14. Persistence and replay rules

Persisted ICS state is historical control memory, not durable execution authority.

### Hard rules
- persisted proposal presence does not authorize execution
- persisted authorized tool state does not authorize blind replay
- persisted pending tool state must re-enter authorization
- persisted last result is observational only
- replay must fail closed on missing or stale permit/contract/proposal linkage

## 15. Capability surface rules

Skills, plugins, and connectors remain downstream capability surfaces.

They may:
- expose metadata
- implement execution handlers
- return structured results

They must not:
- decide whether they should be used
- synthesize governance approval outcomes
- create approval proposals
- bypass permits
- widen tool scope
- write world-model authority state directly

## 16. Anti-drift invariants

These are audit gates.

1. There is one canonical conscious-loop control lifecycle.
2. `DecisionEnvelope` is the canonical persisted runtime control artifact.
3. Declarative control state is not executable authority.
4. Authoritative control state is stage-owned and may be mutated only by its owning stage.
5. No tool executes without matching `ToolPermit` and `ToolContract`.
6. Proposal creation is ICS-owned only.
7. Pause and resume remain ICS-mediated.
8. Replay fails closed on stale or missing authority.
9. Outcome supervision reconciles state after execution and does not fabricate approval.
10. Capability surfaces do not own governance or proposal authority.

## 17. Merge criteria

A branch is non-compliant unless all are true:

- runtime follows the canonical lifecycle
- `Decide` does not emit runtime authority
- governance owns governance fields and proposal creation
- model preparation does not widen tool scope
- model-response authorization owns permits and authorized invocations
- runtime executes only authorized invocations
- replay re-authorizes before executing resumed work
- outcome supervision updates control state after execution
- persistence does not mutate semantics
- capability surfaces remain downstream only

## 18. Relationship to supporting documents

This document defines the top-level architectural contract.

The following supporting documents are normative and subordinate to this contract:

- `ics-end-to-end-control-flow.md`
- `ics-field-ownership-and-mutation-rules.md`
- `ics-compliance-test-bill.md`

## Supporting normative documents

The following documents are normative and subordinate to this contract:

- `docs/architecture/ics-end-to-end-control-flow.md`
- `docs/architecture/ics-field-ownership-and-mutation-rules.md`
- `docs/architecture/ics-compliance-test-bill.md`
- `docs/architecture/ics-artifact-schemas.md`

The earlier Appendix A–C material is superseded by these documents and must not be duplicated elsewhere in conflicting form.

### Document responsibilities

- `ics-architectural-contract.md` defines the top-level architectural contract, stage boundaries, anti-drift invariants, and merge criteria.
- `ics-end-to-end-control-flow.md` defines the canonical conscious-loop lifecycle and transition rules.
- `ics-field-ownership-and-mutation-rules.md` defines field ownership, mutation boundaries, narrowing rules, persistence semantics, and replay semantics.
- `ics-compliance-test-bill.md` defines the mandatory test suite and compliance gates.
- `ics-artifact-schemas.md` defines the canonical artifact layer and schema-level invariants for the current ICS control model.

If any subordinate document conflicts with this contract, this contract wins.

---

[ICS docs index](inference-control-system/INDEX.md) · [architecture README](README.md) · [docs INDEX](../INDEX.md)
