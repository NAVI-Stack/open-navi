# ICS Compliance Test Bill
**Status:** Draft Normative  
**Applies to:** Conscious-loop ICS behavior, field ownership, lifecycle transitions, pause/resume, permit/contract enforcement, and persistence/replay semantics.
**Related corpus:** [ICS docs index](inference-control-system/INDEX.md)

## 1. Purpose

This document defines the mandatory compliance test suite for the Inference Control System.

A branch is non-compliant if:
- any required test is missing
- any required test fails
- any required test is weakened so that drift can pass silently

This document is normative.

## 2. Test categories

The required test suite is divided into:

1. lifecycle and transition tests
2. field ownership and mutation tests
3. governance and proposal tests
4. model directive and tool contract tests
5. tool authorization and execution-boundary tests
6. pause/resume and replay tests
7. outcome supervision tests
8. capability-surface boundary tests
9. static architecture tests

## 3. Lifecycle and transition tests

## 3.1 `TestICS_EndToEndLifecycle_UsesCanonicalStagesOnly`
Assert that the runtime follows:

`AssembleInput -> Decide -> Validate/Govern -> PrepareModelCall -> CallModel -> AuthorizeModelResponse -> ExecuteAuthorizedAction -> ObserveOutcome -> PersistState`

Reject any direct:
- `Decide -> Execute`
- `CallModel -> Execute`
- `ProposalApproved -> Execute`

## 3.2 `TestICS_BlockingDisposition_StopsExecution`
Assert that `block_with_reply` prevents:
- model continuation
- tool execution
- pending tool replay

## 3.3 `TestICS_PauseDisposition_PersistsCheckpointAndEnvelope`
Assert that `pause_for_proposal`:
- persists checkpoint
- persists envelope
- preserves proposal ID
- preserves pending tool state when relevant
- does not execute the pending action

## 3.4 `TestICS_ProceedDirect_OnlyConsumesAuthorizedToolInvocation`
Assert that `proceed_direct` is accepted only when:
- `AuthorizedTools` is present
- `ToolPermit` matches the tool call
- contract is valid

## 4. Field ownership and mutation tests

## 4.1 `TestAssembleInput_DoesNotEmitAuthority`
Assert that `AssembleInput` does not:
- create proposal
- set runtime disposition
- issue permits
- authorize tools

## 4.2 `TestDecide_WritesRationaleOnly`
Assert that `Decide` may write declarative reasoning state but does not write:
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

## 4.3 `TestValidateGovern_OwnsGovernanceFieldsOnly`
Assert that `Validate/Govern` may write:
- `GovernanceResult`
- `Proposal`
- `RuntimeDisposition`
- `ExecutionBoundary`
- governance block/pause reply

and does not write:
- `ModelDirective`
- `AuthorizedTools`
- `LastResult`

## 4.4 `TestPrepareModelCall_WritesModelDirectiveOnly`
Assert that `PrepareModelCall`:
- writes `ModelDirective`
- may clear stale authorization fields
- does not create proposals
- does not emit permits
- does not emit `AuthorizedTools`

## 4.5 `TestAuthorizeModelResponse_OwnsAuthorizationFields`
Assert that `AuthorizeModelResponse` is the only stage that may emit:
- `AuthorizedToolCalls`
- `AuthorizedTools`
- `ToolPermit`
- `PendingToolCall`
- `PendingToolPermit`

## 4.6 `TestObserveOutcome_WritesLastResultAndPostExecutionRationaleOnly`
Assert that `ObserveOutcome`:
- writes `LastResult`
- updates post-execution rationale fields
- does not fabricate permits
- does not fabricate approval

## 4.7 `TestPersistState_DoesNotMutateSemantics`
Assert that persistence:
- stores current envelope
- stores history
- does not change semantic field values

## 5. Governance and proposal tests

## 5.1 `TestGovernance_ValidationOrder_IsDeterministic`
Assert exact validation order:

1. permissions
2. policy
3. configuration
4. priority_alignment
5. risk_assessment
6. execution_scope_checks
7. explicit_confirmation_checks

## 5.2 `TestValidateCandidate_ProposalCreation_IsICSOwned`
Assert that candidate-level proposal creation occurs only through ICS validation.

## 5.3 `TestValidateToolAttempt_ProposalCreation_IsICSOwned`
Assert that tool-attempt proposal creation occurs only through ICS validation.

## 5.4 `TestGovernance_Rejected_FailsClosed`
Assert that rejected governance:
- blocks execution
- emits fail-closed reason
- does not authorize tools

## 5.5 `TestGovernance_NoSurvivingCapability_FailsClosed`
Assert that if no capability survives governance:
- execution boundary is emptied or blocked
- run blocks with explicit reason

## 5.6 `TestProposalPause_PreservesPendingToolState`
Assert that when authorization pauses for approval:
- proposal is attached
- pending tool call is attached
- pending permit is attached
- runtime disposition is pause

## 6. Model directive and tool contract tests

## 6.1 `TestPrepareModelCall_DoesNotWidenToolSurface`
Assert that `ModelDirective.ToolNames` is a subset of the governed execution boundary.

## 6.2 `TestPrepareModelCall_MissingExecutableContract_Blocks`
Assert that surfaced executable tools without valid `ToolContract` cause a block.

## 6.3 `TestPrepareModelCall_ZeroSafeSurfacedTools_Blocks`
Assert that if execution intent requires executable capability but runtime cannot safely surface one, the run blocks.

## 6.4 `TestModelDirectiveToolContract_ReturnsOnlyValidContracts`
Assert that invalid tool contracts are rejected rather than exposed.

## 6.5 `TestPreparedModelDirective_ToolChoice_IsBounded`
Assert that `ToolChoice` cannot point outside the allowed surfaced tool set.

## 7. Tool authorization and execution-boundary tests

## 7.1 `TestAuthorizeModelResponse_RejectsOutOfBoundToolCalls`
Assert that model-returned tool calls not in the current allowed set are rejected.

## 7.2 `TestAuthorizeModelResponse_RejectsMissingAttemptMetadata`
Assert that tool calls without resolved attempt metadata are rejected.

## 7.3 `TestAuthorizeModelResponse_RejectsMissingContract`
Assert that tool calls without authoritative executable contract are rejected.

## 7.4 `TestAuthorizeModelResponse_EmitsAuthorizedToolInvocationOnlyForApprovedCalls`
Assert that only approved calls produce:
- `AuthorizedToolCalls`
- `AuthorizedTools`
- `ToolPermit`

## 7.5 `TestAuthorizeModelResponse_ApprovalRequired_PausesInsteadOfExecuting`
Assert that authorization requiring approval:
- creates proposal-backed pause
- does not authorize execution directly

## 7.6 `TestRuntimeExecutor_RejectsToolWithoutPermit`
Assert that runtime rejects execution when:
- no permit exists
- pending permit does not match
- authorized permit does not match

## 7.7 `TestRuntimeExecutor_RejectsToolWithoutContract`
Assert that runtime rejects execution when:
- permit has no contract
- contract ID is empty
- contract tool name mismatches
- contract is semantically invalid

## 7.8 `TestRuntimeExecutor_FailsClosedOnProposalMismatch`
Assert that runtime rejects execution if:
- permit requires approved proposal ID
- runtime does not provide matching approved proposal ID

## 7.9 `TestRuntimeExecutor_DoesNotCreateProposal`
Assert that runtime execution never creates proposals.

## 8. Pause, resume, and replay tests

## 8.1 `TestResumeReplay_ApprovedProposal_ReauthorizesBeforeExecution`
Assert that approved proposal resume:
- reloads persisted state
- reconstructs pending tool
- re-enters authorization
- does not execute directly

## 8.2 `TestResumeReplay_DeclinedProposal_DoesNotExecute`
Assert that declined proposal resume:
- does not execute pending tool
- emits rejected-pre-execution outcome
- re-enters outcome supervision

## 8.3 `TestReplay_PersistedProposal_IsNotExecutionAuthority`
Assert that presence of persisted proposal alone does not authorize execution.

## 8.4 `TestReplay_PersistedAuthorizedTools_AreNotBlindlyTrusted`
Assert that persisted authorized tool state must still pass current-cycle re-entry rules.

## 8.5 `TestReplay_MissingPermitOrContract_FailsClosed`
Assert that replay with stale or missing permit/contract blocks rather than continuing.

## 8.6 `TestPausedCheckpoint_RoundTripsPendingToolState`
Assert that checkpoint round-trip preserves:
- pending proposal ID
- pending proposal reason
- pending tool call
- envelope version linkage

## 9. Outcome supervision tests

## 9.1 `TestObserveOutcome_UpdatesPlanGraph`
Assert that outcome supervision updates:
- current node status
- plan status
- checkpoint refs
- next node when success occurs

## 9.2 `TestObserveOutcome_UpdatesRecoveryCheckpoint`
Assert that outcome supervision updates:
- recovery openness
- recovery route
- pending blockers
- pending approvals
- resume conditions

## 9.3 `TestObserveOutcome_UpdatesGoalStack`
Assert that outcome supervision:
- retires completed goals
- resumes blocked goals
- preserves active goal semantics

## 9.4 `TestObserveOutcome_UpdatesPostExecutionControlState`
Assert that outcome supervision may change:
- dominant mode
- chosen action
- execution intent
- governance handoff
- focus reason
- rejection state

only according to the observed snapshot.

## 9.5 `TestObserveOutcome_DoesNotFabricateApproval`
Assert that outcome supervision does not:
- create permits
- create proposals
- mark execution as authorized after the fact

## 9.6 `TestObserveOutcome_UpdatesDecisionTrace`
Assert that outcome supervision updates:
- stage
- occurred_at
- execution outcome ref
- executed capability
- proposal ID
- recovery ref
- reflection ref

## 10. Capability-surface boundary tests

## 10.1 `TestSkillCannotSynthesizeGovernanceOutcome`
Assert that skill execution paths do not independently emit approval/rejection semantics.

## 10.2 `TestPluginCannotSynthesizeGovernanceOutcome`
Assert that plugin execution paths do not independently emit approval/rejection semantics.

## 10.3 `TestConnectorCannotSynthesizePolicyDecision`
Assert that connectors return transport/result data only, not policy/governance outcomes.

## 10.4 `TestCapabilitySurface_DoesNotCreateProposal`
Assert that skills/plugins/connectors do not create proposal-backed approval boundaries.

## 10.5 `TestCapabilitySurface_DoesNotWriteWorldModelDirectly`
Assert that capability execution paths do not directly mutate world-model authority state.

## 11. Static architecture tests

## 11.1 `TestNoDirectDecideToExecutePath`
Assert that there is no callable direct path from `Decide` to execution.

## 11.2 `TestNoCallModelToExecuteWithoutAuthorization`
Assert that model output cannot reach execution without `AuthorizeModelResponse`.

## 11.3 `TestNoProposalCreationOutsideICSValidation`
Assert that proposal persistence is reachable only through ICS-owned validation/authorization paths.

## 11.4 `TestNoPermitEmissionOutsideAuthorization`
Assert that `ToolPermit` is emitted only by authorization stages.

## 11.5 `TestNoSemanticMutationInPersistenceLayer`
Assert that persistence/replay code does not alter envelope meaning.

## 11.6 `TestNoToolSurfaceWideningAfterGovernance`
Assert that no downstream stage widens the tool set after governance/model preparation.

## 12. Minimum blocking suite

The following tests are the minimum blocking gate and must run in CI for every ICS-affecting change:

- `TestICS_EndToEndLifecycle_UsesCanonicalStagesOnly`
- `TestDecide_WritesRationaleOnly`
- `TestGovernance_ValidationOrder_IsDeterministic`
- `TestValidateToolAttempt_ProposalCreation_IsICSOwned`
- `TestPrepareModelCall_DoesNotWidenToolSurface`
- `TestPrepareModelCall_MissingExecutableContract_Blocks`
- `TestAuthorizeModelResponse_RejectsOutOfBoundToolCalls`
- `TestAuthorizeModelResponse_RejectsMissingContract`
- `TestRuntimeExecutor_RejectsToolWithoutPermit`
- `TestRuntimeExecutor_RejectsToolWithoutContract`
- `TestResumeReplay_ApprovedProposal_ReauthorizesBeforeExecution`
- `TestResumeReplay_DeclinedProposal_DoesNotExecute`
- `TestObserveOutcome_DoesNotFabricateApproval`
- `TestNoProposalCreationOutsideICSValidation`
- `TestNoPermitEmissionOutsideAuthorization`
- `TestNoToolSurfaceWideningAfterGovernance`

## 13. Audit verdict rules

A branch is:

### `Compliant`
only if all required tests exist and pass.

### `Non-compliant`
if any required test:
- is missing
- is skipped
- is weakened to stop asserting the normative rule
- fails

## 14. Reviewer checklist

A reviewer must be able to answer yes to all:

- Are lifecycle transitions fully covered?
- Is field ownership enforced by tests?
- Are proposals owned only by ICS validation/authorization?
- Are permits owned only by authorization?
- Does runtime fail closed on missing authority?
- Does replay re-enter the lifecycle correctly?
- Does outcome supervision reconcile control state without inventing approval?

If any answer is no, the branch is non-compliant.

---

[ICS docs index](inference-control-system/INDEX.md) · [ICS architectural contract](ics-architectural-contract.md) · [docs INDEX](../INDEX.md)
