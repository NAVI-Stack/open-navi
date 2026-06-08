# ICS Implementation Mapping
**Status:** Draft Normative Reference  
**Applies to:** Mapping the current `feat/inference-control-system` branch implementation to the normative ICS architecture and control-flow documents.
**Related corpus:** [ICS docs index](inference-control-system/INDEX.md)

## 1. Purpose

This document maps the normative ICS stages, artifacts, and authority boundaries to the concrete files and symbols in the repository.

It exists to make future audits and remediation work precise.

This document is branch-specific. It must be updated when the implementation changes materially.

## 2. Canonical branch mapping summary

The current branch implements the conscious-loop control path through:

- controller interface: `internal/navi/inference/interfaces.go`
- decision synthesis: `internal/navi/inference/controller.go`
- governance and authority shaping: `internal/navi/inference/validate_govern.go` and `internal/navi/inference/controller_authority.go`
- model-call shaping and model-response authorization: `internal/navi/inference/model_authorization.go`
- runtime loop and persistence/replay: `internal/navi/runtime_inference.go` and `internal/navi/runtime_executor.go`
- outcome supervision: `internal/navi/inference/outcome_supervisor.go`

## 3. Canonical artifacts → implementation files

## 3.1 `InferenceInput`
**Primary file:** `internal/navi/inference/types.go`

### Primary symbols
- `type InferenceInput struct`
- `type GovernanceState struct`
- `type RuntimeContext struct`
- `type SessionContext struct`
- `type CapabilityAvailability struct`
- `type ContextReference struct`

### Runtime assembly entrypoint
**Primary file:** `internal/navi/runtime_inference.go`

### Primary symbols
- `func (l *AgentLoop) buildRunInferenceInput(...) inference.InferenceInput`

## 3.2 `DecisionSynthesis`
**Primary file:** `internal/navi/inference/types.go`

### Primary symbols
- `type DecisionSynthesis struct`

### Producer
**Primary file:** `internal/navi/inference/controller.go`

### Primary symbols
- `func (c *Controller) Decide(...) (DecisionSynthesis, error)`
- `func (c *Controller) decideRationale(...) (DecisionSynthesis, error)`

## 3.3 `DecisionEnvelope`
**Primary file:** `internal/navi/inference/authority.go`

### Primary symbols
- `type DecisionEnvelope struct`
- `type RuntimeDisposition string`
- `type ExecutionBoundary struct`
- `type ModelDirective struct`
- `type ToolContract struct`
- `type ToolPermit struct`
- `type AuthorizedToolInvocation struct`

### Persistence/load path
**Primary file:** `internal/navi/runtime_inference.go`

### Primary symbols
- `func (l *AgentLoop) applyInferenceEnvelope(...) error`
- `func (l *AgentLoop) loadPersistedInferenceEnvelope(...) (*inference.DecisionEnvelope, error)`
- `func runtimeInferenceEnvelope(...) *inference.DecisionEnvelope`

## 3.4 `ExecutionSnapshot`
**Primary file:** `internal/navi/inference/types.go`

### Primary symbols
- `type ExecutionSnapshot struct`

### Consumers / supervisors
**Primary files:**
- `internal/navi/runtime_inference.go`
- `internal/navi/inference/outcome_supervisor.go`

### Primary symbols
- `func (l *AgentLoop) superviseInferenceOutcome(...)`
- `func (c *Controller) ObserveOutcome(...) (DecisionEnvelope, error)`

## 4. Lifecycle stage → implementation mapping

## 4.1 `AssembleInput`
**Primary file:** `internal/navi/runtime_inference.go`

### Primary symbols
- `func (l *AgentLoop) buildRunInferenceInput(...) inference.InferenceInput`
- `func buildRuntimeGoalStack(...) inference.GoalStack`
- `func governanceStateFromEnvelope(...) inference.GovernanceState`
- `func lastResultFromEnvelope(...) *inference.ExecutionSnapshot`
- `func restoreGovernanceStateFromExtensions(...) inference.GovernanceState`

### Responsibility in current branch
- assemble session/run/checkpoint/prior-envelope context
- derive governance state and recovery state
- derive visible capabilities and relevant context
- construct `InferenceInput`

## 4.2 `Decide`
**Primary file:** `internal/navi/inference/controller.go`

### Primary symbols
- `func (c *Controller) Decide(...) (DecisionSynthesis, error)`
- `func (c *Controller) decideRationale(...) (DecisionSynthesis, error)`
- `func buildExecutionIntent(...) ExecutionIntent`
- `func buildGovernanceHandoff(...) GovernanceHandoff`
- `func buildReflectionHooks(...) ReflectionHookSet`
- `func buildDecisionTrace(...)` *(where present via referenced helper path in rationale assembly)*

### Responsibility in current branch
- focus arbitration
- mode routing
- candidate evaluation
- rationale construction
- declarative execution/governance/recovery/reflection shaping

## 4.3 `Validate/Govern`
**Primary files:**
- `internal/navi/inference/validate_govern.go`
- `internal/navi/inference/controller_authority.go`

### Primary symbols
- `func ValidateCandidate(...) (GovernResult, error)`
- `func ValidateToolAttempt(...) (GovernResult, error)`
- `func (c *Controller) ValidateGovernedDecision(...) (DecisionEnvelope, error)`
- `func (c *Controller) finalizeDecision(...) (DecisionEnvelope, error)`
- `func authorizationBlockedEnvelope(...) DecisionEnvelope`
- `func authorizationPauseEnvelope(...) DecisionEnvelope`

### Responsibility in current branch
- deterministic validation
- proposal persistence
- governed boundary shaping
- runtime block/pause/proceed disposition shaping
- fail-closed behavior when no safe governed capability remains

## 4.4 `PrepareModelCall`
**Primary file:** `internal/navi/inference/model_authorization.go`

### Primary symbols
- `func (c *Controller) PrepareModelCall(...) (DecisionEnvelope, error)`
- `func BuildPreparedModelDirective(...) (*ModelDirective, string)`
- `func ModelDirectiveToolContract(...) *ToolContract`

### Responsibility in current branch
- emit `ModelDirective`
- narrow model-visible tools
- require authoritative executable contracts
- block if surfaced executable tools are unsafe/incomplete

## 4.5 `CallModel`
**Primary file:** `internal/navi/runtime_executor.go`

### Primary symbols
- `func (l *AgentLoop) ExecuteRun(...) (*naviruntime.ExecuteResult, error)`
- `func (l *AgentLoop) compileRunModelRequest(...) (...)`
- `func (l *AgentLoop) recompileRunModelRequest(...) (...)`
- `func (l *AgentLoop) normalizeRunModelResponse(...) (...)`

### Responsibility in current branch
- compile the prepared request
- invoke model
- normalize response
- hand normalized response back into ICS authorization

## 4.6 `AuthorizeModelResponse`
**Primary file:** `internal/navi/inference/model_authorization.go`

### Primary symbols
- `func (c *Controller) AuthorizeModelResponse(...) (DecisionEnvelope, error)`

### Responsibility in current branch
- reject out-of-bound tool calls
- require resolved tool-attempt metadata
- require authoritative `ToolContract`
- validate tool attempts through governance
- emit `AuthorizedToolInvocation` and `ToolPermit`
- emit proposal-backed pause state when approval is required

## 4.7 `ExecuteAuthorizedAction`
**Primary file:** `internal/navi/runtime_executor.go`

### Primary symbols
- `func (l *AgentLoop) executeToolForRun(...) (...)`
- `func decisionToolPermit(...) *inference.ToolPermit`
- `func toolPermitMatchesCall(...) bool`
- `func buildRuntimeToolContract(...) (inference.ToolContract, bool)`

### Responsibility in current branch
- consume authorized invocation
- enforce permit match
- enforce contract match
- execute fail-closed
- emit execution snapshot

## 4.8 `ObserveOutcome`
**Primary file:** `internal/navi/inference/outcome_supervisor.go`

### Primary symbols
- `func (c *Controller) ObserveOutcome(...) (DecisionEnvelope, error)`
- `func supervisePlanGraph(...) *PlanGraph`
- `func superviseRecoveryCheckpoint(...) RecoveryCheckpoint`
- `func superviseGoalStack(...) GoalStack`
- `func superviseOutcomeControlState(...) Rationale`
- `func superviseReflectionHooks(...) ReflectionHookSet`
- `func superviseDecisionTrace(...) DecisionTrace`

### Responsibility in current branch
- reconcile execution snapshot into post-execution control state
- update plan/recovery/goal/focus/trace state
- preserve execution facts while updating control interpretation

## 4.9 `PersistState`
**Primary file:** `internal/navi/runtime_inference.go`

### Primary symbols
- `func (l *AgentLoop) applyInferenceEnvelope(...) error`
- `func (l *AgentLoop) loadPersistedInferenceEnvelope(...) (*inference.DecisionEnvelope, error)`

### Responsibility in current branch
- persist current envelope
- append history
- reload current envelope
- attach persisted state to run/checkpoint state

## 5. Controller interface → implementation mapping

**Primary file:** `internal/navi/inference/interfaces.go`

### Interface
- `type DecisionController interface`

### Current lifecycle methods
- `Decide`
- `ValidateGovernedDecision`
- `ObserveOutcome`
- `PrepareModelCall`
- `AuthorizeModelResponse`
- `AuthorizeToolCall`

### Concrete implementation
**Primary file:** `internal/navi/inference/controller.go`

### Primary symbol
- `type Controller struct`

## 6. Runtime entrypoint → implementation mapping

**Primary file:** `internal/navi/runtime_executor.go`

### Primary symbol
- `func (l *AgentLoop) ExecuteRun(...)`

### ICS entry handoff inside runtime
**Primary file:** `internal/navi/runtime_inference.go`

### Primary symbol
- `func (l *AgentLoop) decideRunInference(...) (inference.DecisionEnvelope, error)`

### Current runtime flow in practice
- load prior envelope
- build `InferenceInput`
- call `Decide`
- call `ValidateGovernedDecision`
- persist envelope
- call `PrepareModelCall`
- compile and call model
- normalize response
- call `AuthorizeModelResponse`
- execute authorized tools only
- supervise outcome
- persist supervised envelope

## 7. Proposal ownership → implementation mapping

**Primary file:** `internal/navi/inference/validate_govern.go`

### Primary symbols
- `type ValidationDependency interface`
- `func ValidateCandidate(...) (GovernResult, error)`
- `func ValidateToolAttempt(...) (GovernResult, error)`

### Proposal persistence entrypoint
- `SaveProposal(...)`

### Current branch role
- candidate-level proposal creation
- tool-attempt proposal creation
- approval-boundary pause generation

### Related runtime pause handling
**Primary file:** `internal/navi/runtime_executor.go`

### Primary symbols
- pause handling branches inside `ExecuteRun(...)`
- checkpoint persistence of pending proposal/pending tool state

## 8. Permit and contract ownership → implementation mapping

**Primary file:** `internal/navi/inference/authority.go`

### Canonical authoritative types
- `ToolContract`
- `ToolPermit`
- `AuthorizedToolInvocation`

**Primary file:** `internal/navi/inference/model_authorization.go`

### Permit emission path
- `AuthorizeModelResponse(...)`

**Primary file:** `internal/navi/runtime_executor.go`

### Permit consumption path
- `decisionToolPermit(...)`
- `toolPermitMatchesCall(...)`
- `executeToolForRun(...)`

## 9. Replay and resume mapping

**Primary files:**
- `internal/navi/runtime_inference.go`
- `internal/navi/runtime_executor.go`

### Primary symbols
- `loadPersistedInferenceEnvelope(...)`
- paused-checkpoint handling inside `ExecuteRun(...)`
- resume paths inside `ExecuteRun(...)`

### Current branch behavior
- reload persisted envelope/checkpoint
- reconstruct pending proposal and pending tool state
- re-enter ICS authorization before resumed execution
- do not execute directly from persisted proposal presence alone

## 10. Capability-surface mapping

### Skill-build governance routing
**Primary file:** `internal/navi/skill_build_governance_adapter.go`

### Primary symbols
- `type skillBuildGovernanceAdapter struct`
- `func (a *skillBuildGovernanceAdapter) Govern(...) (skill.GovernanceResponse, error)`

### Current branch role
- route skill/connector build governance through canonical ICS seam

### Runtime capability execution
**Primary file:** `internal/navi/runtime_executor.go`

### Primary symbols
- skill execution branches in `executeToolForRun(...)`
- plugin execution branches in `executeToolForRun(...)`
- file-tool execution branches in `executeToolForRun(...)`

## 11. Known drift watchpoints

These are the main places future audits should inspect first.

## 11.1 Declarative vs authoritative blur
**Primary files:**
- `internal/navi/inference/controller.go`
- `internal/navi/inference/types.go`

### Why
`Rationale` carries broad declarative control state, including governance-adjacent and execution-adjacent information. Audits must verify that runtime still acts only on authoritative envelope fields.

## 11.2 Split authority helpers across inference and runtime packages
**Primary files:**
- `internal/navi/inference/controller_authority.go`
- `internal/navi/runtime_inference.go`
- `internal/navi/runtime_executor.go`

### Why
Authority shaping and state reconciliation are still split across these areas. Future changes can easily blur ownership again.

## 11.3 Hidden validation stages
**Primary file:** `internal/navi/inference/validate_govern.go`

### Why
Tool-attempt validation includes follow-on checks beyond the base five-stage validation list. Audits must verify that the declared pipeline still matches implementation.

## 11.4 Replay authority drift
**Primary files:**
- `internal/navi/runtime_inference.go`
- `internal/navi/runtime_executor.go`

### Why
Replay/resume is the highest-risk place for stale authority reuse. Audits must verify that replay re-enters authorization and fails closed on stale state.

## 12. Recommended audit order

For fastest branch review, inspect in this order:

1. `internal/navi/inference/interfaces.go`
2. `internal/navi/runtime_inference.go`
3. `internal/navi/inference/controller.go`
4. `internal/navi/inference/controller_authority.go`
5. `internal/navi/inference/validate_govern.go`
6. `internal/navi/inference/model_authorization.go`
7. `internal/navi/runtime_executor.go`
8. `internal/navi/inference/outcome_supervisor.go`
9. `internal/navi/skill_build_governance_adapter.go`

## 13. Non-compliance triggers for this mapping

This mapping becomes stale and must be updated if any of the following occur:

- a new conscious-loop stage is introduced
- a new persisted control artifact replaces `DecisionEnvelope`
- permit or contract emission moves to another package
- replay/resume bypasses the current lifecycle
- outcome supervision moves outside `ObserveOutcome`
- capability surfaces gain governance or proposal ownership

## 14. Relationship to other ICS documents

This document is a reference mapping layer.

It is subordinate to:
- `ics-architectural-contract.md`

It supports:
- `ics-end-to-end-control-flow.md`
- `ics-field-ownership-and-mutation-rules.md`
- `ics-artifact-schemas.md`
- `ics-compliance-test-bill.md`
- `ics-repo-audit-checklist.md`

If this document conflicts with the architectural contract, the architectural contract wins.

---

[ICS docs index](inference-control-system/INDEX.md) · [ICS architectural contract](ics-architectural-contract.md) · [docs INDEX](../INDEX.md)
