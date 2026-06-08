# ICS Repo Audit Checklist
**Status:** Draft Normative  
**Applies to:** Repository audits of the Inference Control System implementation, architecture compliance reviews, and branch acceptance decisions.
**Related corpus:** [ICS docs index](inference-control-system/INDEX.md)

## 1. Purpose

This document defines the mandatory audit checklist for reviewing the ICS implementation in the repository.

It exists to make branch review mechanical instead of interpretive.

This document is normative. If the repository fails any blocking audit item, the branch is non-compliant.

## 2. Audit objective

The audit must determine whether the repository implements the canonical ICS control model end to end:

`AssembleInput -> Decide -> Validate/Govern -> PrepareModelCall -> CallModel -> AuthorizeModelResponse -> ExecuteAuthorizedAction -> ObserveOutcome -> PersistState`

The audit must also confirm that:

- `DecisionEnvelope` is the canonical persisted runtime control artifact
- declarative fields are not treated as executable authority
- runtime acts only on authoritative control state
- replay/resume re-enters the lifecycle instead of bypassing it
- capability surfaces do not own governance, proposal, or permit authority

## 3. Audit verdicts

Every audit must end with one of the following verdicts:

- `Compliant`
- `Non-compliant`

### `Compliant`
Use only when:
- all blocking items pass
- no forbidden path remains callable
- no conflicting authority model remains in the repo

### `Non-compliant`
Use when:
- any blocking item fails
- any required path is missing
- any parallel or alternate control path exists
- any stage owns fields it is not allowed to own

## 4. Blocking audit items

A single failure in this section is an automatic non-compliance.

## 4.1 Canonical lifecycle exists and is unique

Confirm that the repository implements exactly one conscious-loop lifecycle:

`AssembleInput -> Decide -> Validate/Govern -> PrepareModelCall -> CallModel -> AuthorizeModelResponse -> ExecuteAuthorizedAction -> ObserveOutcome -> PersistState`

### Fail if
- there is a direct `Decide -> Execute` path
- there is a `CallModel -> Execute` path without authorization
- there is a `ProposalApproved -> Execute` path without re-authorization
- there is more than one callable conscious-loop control lifecycle

## 4.2 `DecisionEnvelope` is the canonical persisted runtime control artifact

Confirm that the repository persists, reloads, and continues from `DecisionEnvelope`, not from an alternate control artifact.

### Fail if
- another artifact is treated as the authoritative persisted control state
- runtime bypasses the envelope during replay/resume
- persisted state is reconstructed ad hoc from unrelated runtime fields instead of the canonical envelope path

## 4.3 `Decide` is decision synthesis only

Confirm that `Decide` emits only declarative reasoning state.

### Must confirm
- `Decide` does not persist proposals
- `Decide` does not set runtime disposition
- `Decide` does not emit tool permits
- `Decide` does not emit authorized tool invocations
- `Decide` does not execute model or tool calls

### Fail if
- `Decide` mutates authoritative runtime control fields
- `Decide` emits proposal-backed pause authority
- `Decide` is used directly as execution authority

## 4.4 `Validate/Govern` is the sole owner of governance result and proposal-backed pause authority

Confirm that governance validation owns:
- deterministic validation ordering
- fail-closed governance outcome
- proposal creation for approval boundaries
- executable boundary narrowing
- governance result emission
- block/pause disposition emission

### Fail if
- proposal creation exists outside ICS validation/authorization
- another package synthesizes governance approval outcome
- downstream stages widen the governed execution boundary
- governance ordering is duplicated or contradicted elsewhere

## 4.5 `PrepareModelCall` is the sole owner of model directive shaping

Confirm that model-call preparation owns:
- `ModelDirective`
- model-visible tool narrowing
- executable tool contract requirement
- fail-closed behavior when surfaced executable tools are unsafe or incomplete

### Fail if
- tool surface is widened after governance
- surfaced executable tools lack authoritative contracts
- model preparation silently tolerates missing contract state
- another package shapes the model boundary outside ICS

## 4.6 `AuthorizeModelResponse` is the sole owner of tool authorization and permit emission

Confirm that authorization owns:
- rejection of out-of-bound tool calls
- requirement for resolved attempt metadata
- requirement for authoritative executable tool contracts
- creation of `AuthorizedToolInvocation`
- creation of `ToolPermit`
- proposal-backed pause for approval-required tool attempts

### Fail if
- raw model tool calls can reach runtime execution
- permits are emitted outside authorization
- out-of-bound tool calls are tolerated
- missing contract or missing attempt metadata is tolerated

## 4.7 Runtime executes only authorized tool invocations

Confirm that runtime executes only when:
- an authorized invocation exists
- a matching permit exists
- a valid contract exists
- proposal linkage matches when required

### Fail if
- runtime executes raw tool calls
- runtime executes with missing permit
- runtime executes with missing or invalid contract
- runtime executes with stale or mismatched proposal approval state

## 4.8 Pause and resume remain ICS-mediated

Confirm that paused runs:
- persist checkpoint state
- persist envelope state
- preserve proposal and pending tool state when relevant

Confirm that resumed runs:
- reload persisted state
- re-enter authorization before execution
- do not directly execute pending tools on approval

### Fail if
- replay/resume skips ICS authorization
- proposal approval directly triggers execution
- decline path executes pending work
- persisted pending tool state is blindly trusted as execution authority

## 4.9 Outcome supervision is authoritative for post-execution reconciliation

Confirm that `ObserveOutcome` owns:
- `LastResult`
- post-execution reconciliation of plan/recovery/goal/focus/trace state

### Fail if
- outcome supervision fabricates approval
- outcome supervision rewrites factual execution history
- post-execution control state is updated outside supervision in conflicting ways

## 4.10 Capability surfaces remain downstream only

Confirm that skills, plugins, and connectors:
- expose metadata
- execute handlers
- return structured results

Confirm they do not:
- synthesize governance outcomes
- create approval proposals
- emit permits
- widen tool scope
- own world-model authority changes directly

### Fail if
- a skill/plugin/connector acts as an alternate policy engine
- a capability surface creates approval boundaries directly
- a capability surface bypasses permit/contract enforcement

## 5. Audit lanes

The repo audit must be performed across these lanes.

## 5.1 Lane A — Input assembly and persisted state

Review:
- runtime input assembly
- persisted state load/save
- checkpoint reload
- replay/resume entry

Questions:
- Is `InferenceInput` assembled only from runtime/session/checkpoint state?
- Is `DecisionEnvelope` the canonical persisted artifact?
- Does replay re-enter the lifecycle cleanly?

## 5.2 Lane B — Decision synthesis

Review:
- controller decision code
- focus selection
- mode routing
- candidate evaluation
- rationale building

Questions:
- Does `Decide` stay declarative?
- Does `Decide` avoid runtime authority?
- Does `Decide` avoid proposal persistence?

## 5.3 Lane C — Governance validation

Review:
- candidate validation path
- tool attempt validation path
- proposal persistence path
- governance boundary shaping

Questions:
- Is validation order fixed?
- Is proposal creation ICS-owned only?
- Does governance narrow or fail closed?
- Is any extra hidden gate outside the declared pipeline?

## 5.4 Lane D — Model boundary shaping

Review:
- model-call preparation
- model directive shaping
- surfaced tool narrowing
- contract compilation requirements

Questions:
- Is `ModelDirective` the only model boundary artifact?
- Are executable tools contract-backed?
- Can surfaced tools be widened outside ICS?

## 5.5 Lane E — Model-response authorization

Review:
- normalized response authorization
- out-of-bound tool rejection
- missing-attempt rejection
- contract validation
- permit emission
- proposal-backed pause creation

Questions:
- Can unauthorized tool calls slip through?
- Are permits emitted only here?
- Does authorization fail closed?

## 5.6 Lane F — Runtime execution

Review:
- executor permit matching
- contract matching
- proposal approval matching
- fail-closed behavior

Questions:
- Can runtime execute without current authority artifacts?
- Is execution consuming only authorized invocations?
- Does runtime ever synthesize governance?

## 5.7 Lane G — Outcome supervision

Review:
- post-execution reconciliation
- plan graph update
- recovery checkpoint update
- goal stack update
- decision trace update
- reflection update

Questions:
- Is `ObserveOutcome` the single post-execution reconciliation owner?
- Are post-execution control transitions derived from actual snapshot state?
- Is approval ever fabricated after the fact?

## 6. Required evidence format

Every blocking finding must include:

- `ID`
- `Rule violated`
- `Where found`
- `Why it is drift`
- `Required fix`
- `Delete vs Refactor`

### `Where found` must name:
- package
- file
- symbol/function
- relevant call path

### `Why it is drift` must explain:
- which contract rule it violates
- whether it introduces alternate authority
- whether it widens a boundary
- whether it bypasses replay/governance/authorization

## 7. Delete vs refactor rules

Use `Delete` when:
- an alternate path exists
- a legacy control path remains callable
- an ad hoc proposal path exists
- a duplicate policy/permit/authorization engine exists

Use `Refactor` when:
- the correct path exists but ownership is blurred
- a field is mutated by the wrong stage
- a boundary is implemented in the wrong layer
- lifecycle re-entry exists but is under-specified

## 8. Static architecture checks

The audit must include static checks for all of the following:

- no direct `Decide -> Execute` path
- no `CallModel -> Execute` path without authorization
- no proposal creation outside ICS validation/authorization
- no permit emission outside authorization
- no tool surface widening after governance/model preparation
- no semantic mutation inside persistence/replay code
- no capability-surface governance ownership
- no raw model tool execution path

## 9. Required audit output template

Use this output shape exactly.

### Verdict
- `Compliant`
- `Non-compliant`

### Blocking findings
For each finding:

- `ID:`
- `Rule violated:`
- `Where found:`
- `Why it is drift:`
- `Required fix:`
- `Delete vs Refactor:`

### Non-blocking findings
List only after all blockers are exhausted.

### Final disposition
One of:
- `Reject branch`
- `Reject until blockers fixed`
- `Accept after targeted cleanup`
- `Accept`

## 10. Branch acceptance rules

A branch must be rejected if any of the following are true:

- any alternate control lifecycle remains callable
- any raw tool call can execute without authorization
- any proposal path exists outside ICS validation/authorization
- any permit path exists outside authorization
- replay/resume can execute without re-authorization
- outcome supervision is bypassed or contradicted
- capability surfaces own governance or approval logic

## 11. Reviewer checklist

A reviewer must be able to answer yes to all of the following:

- Is there exactly one canonical conscious-loop lifecycle?
- Is `DecisionEnvelope` the canonical persisted runtime control artifact?
- Is `Decide` decision-only?
- Is governance ICS-owned and fail-closed?
- Is model shaping ICS-owned and narrowing-only?
- Is tool authorization ICS-owned and permit-based?
- Does runtime execute only authorized invocations?
- Does replay re-enter authorization before execution?
- Does `ObserveOutcome` own post-execution reconciliation?
- Do capability surfaces remain downstream only?

If any answer is “no”, the branch is non-compliant.

---

[ICS docs index](inference-control-system/INDEX.md) · [ICS compliance test bill](ics-compliance-test-bill.md) · [docs INDEX](../INDEX.md)
