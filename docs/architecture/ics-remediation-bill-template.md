# ICS Remediation Bill Template
**Status:** Draft Normative Template  
**Applies to:** Converting ICS audit findings into concrete remediation work items for implementation branches.
**Related corpus:** [ICS docs index](inference-control-system/INDEX.md)

## 1. Purpose

This document defines the required structure for an ICS remediation bill.

A remediation bill is the only valid format for translating branch audit findings into implementation work.

Its purpose is to eliminate vague “fix the drift” work and replace it with:
- exact violated rule
- exact files and symbols
- exact deletion vs refactor guidance
- concrete acceptance criteria
- concrete compliance tests

This document is normative.

## 2. When to use this template

Use this template whenever:
- an ICS audit returns `Non-compliant`
- a branch contains blocker findings
- the implementation and contract diverge
- replay/resume, proposal, permit, or envelope ownership is unclear
- lifecycle stages are blurred across packages

Do not open free-form remediation work for ICS architecture drift outside this template.

## 3. Remediation bill header

Every remediation bill must begin with:

- `Branch:`
- `Audit date:`
- `Audit verdict:`
- `Reviewer:`
- `Spec baseline:`
- `Implementation baseline:`
- `Scope:`
- `Blocking finding count:`
- `Non-blocking finding count:`

### Example header

- `Branch:` `feat/inference-control-system`
- `Audit date:` `YYYY-MM-DD`
- `Audit verdict:` `Non-compliant`
- `Reviewer:` `name or role`
- `Spec baseline:` `ics-architectural-contract.md` + supporting ICS docs
- `Implementation baseline:` `current branch HEAD`
- `Scope:` `conscious-loop ICS only`
- `Blocking finding count:` `N`
- `Non-blocking finding count:` `M`

## 4. Remediation bill structure

Every remediation bill must have these sections, in this order:

1. Executive summary
2. Blocking remediation items
3. Non-blocking remediation items
4. Required deletion list
5. Required test additions or updates
6. Acceptance gate
7. Final disposition rule

## 5. Executive summary format

The executive summary must include:

- whether the branch can merge
- whether the branch must be rejected
- whether any blocker reflects alternate authority
- whether any blocker reflects stale or conflicting contract models
- whether replay/resume authority is safe
- whether tool execution remains fail-closed

### Example format

- `Merge status:` `Rejected`
- `Reason:` `Blocking architecture drift remains`
- `Primary risk:` `alternate authority path / stale replay authority / permit bypass / proposal leakage`
- `Required next step:` `complete all blocking remediation items before re-audit`

## 6. Remediation item schema

Every remediation item must use exactly this structure:

- `ID:`
- `Severity:`
- `Rule violated:`
- `Lifecycle stage:`
- `Artifact or field affected:`
- `Where found:`
- `Observed behavior:`
- `Why it is drift:`
- `Required action:`
- `Delete vs Refactor:`
- `Acceptance criteria:`
- `Required tests:`
- `Dependencies:`
- `Notes:`

## 7. Field definitions

## 7.1 `ID`
Format:
`ICS-RB-###`

Examples:
- `ICS-RB-001`
- `ICS-RB-002`

## 7.2 `Severity`
Allowed values:
- `Blocking`
- `Non-blocking`

## 7.3 `Rule violated`
Must cite the exact normative rule or invariant from the ICS docs.

Examples:
- `No direct Decide -> Execute path`
- `DecisionEnvelope is the canonical persisted runtime control artifact`
- `Runtime must not execute without matching ToolPermit and ToolContract`
- `Replay must re-enter authorization before execution`

## 7.4 `Lifecycle stage`
Allowed values:
- `AssembleInput`
- `Decide`
- `Validate/Govern`
- `PrepareModelCall`
- `CallModel`
- `AuthorizeModelResponse`
- `ExecuteAuthorizedAction`
- `ObserveOutcome`
- `PersistState`
- `ResumeReplay`

## 7.5 `Artifact or field affected`
Must name the exact artifact or field group.

Examples:
- `DecisionEnvelope.RuntimeDisposition`
- `DecisionEnvelope.ModelDirective`
- `DecisionEnvelope.AuthorizedTools`
- `ExecutionBoundary`
- `ToolPermit`
- `ExecutionSnapshot`
- `Rationale.ExecutionIntent`

## 7.6 `Where found`
Must include:
- package
- file
- symbol/function
- relevant call path

### Example
- `package:` `internal/navi`
- `file:` `runtime_executor.go`
- `symbol:` `executeToolForRun`
- `call path:` `ExecuteRun -> AuthorizeModelResponse -> executeToolForRun`

## 7.7 `Observed behavior`
Describe what the branch currently does.

This section must be factual and implementation-specific.

## 7.8 `Why it is drift`
Explain:
- what contract rule it violates
- whether it creates alternate authority
- whether it widens a boundary
- whether it bypasses replay/governance/authorization
- whether it creates a second contract model in practice

## 7.9 `Required action`
Describe the exact remediation.

Good examples:
- remove alternate execution path
- move permit emission into authorization stage
- narrow model directive generation to governed execution boundary
- re-enter authorization on resume before any execution path
- split field mutation so only owning stage writes the field

Bad examples:
- “clean this up”
- “improve architecture”
- “make it align better”

## 7.10 `Delete vs Refactor`
Allowed values:
- `Delete`
- `Refactor`

### Use `Delete` when:
- alternate lifecycle exists
- legacy compatibility path remains callable
- duplicate proposal path exists
- duplicate permit or policy engine exists

### Use `Refactor` when:
- ownership exists but is blurred
- correct lifecycle exists but field mutation is in wrong stage
- correct behavior exists but is split across wrong layers

## 7.11 `Acceptance criteria`
Must be objective and binary.

Good examples:
- `runtime can no longer execute without matching permit`
- `proposal approval no longer triggers direct execution`
- `PrepareModelCall no longer widens tool scope beyond ExecutionBoundary`
- `DecisionEnvelope is the only persisted active control artifact`

## 7.12 `Required tests`
Must list:
- existing tests to update
- new tests to add
- blocking tests from the compliance test bill that this remediation satisfies

## 7.13 `Dependencies`
List prerequisite remediation items if applicable.

## 7.14 `Notes`
Use only for grounded implementation nuance. No vague commentary.

## 8. Blocking remediation item template

Use this exact template for each blocker.

### Template

- `ID:` `ICS-RB-###`
- `Severity:` `Blocking`
- `Rule violated:` `...`
- `Lifecycle stage:` `...`
- `Artifact or field affected:` `...`
- `Where found:` `package / file / symbol / call path`
- `Observed behavior:` `...`
- `Why it is drift:` `...`
- `Required action:` `...`
- `Delete vs Refactor:` `Delete|Refactor`
- `Acceptance criteria:`
  - `...`
  - `...`
- `Required tests:`
  - `...`
  - `...`
- `Dependencies:` `none | ICS-RB-###`
- `Notes:` `...`

## 9. Required deletion list section

Every remediation bill must include a dedicated deletion list.

This section must contain every path that must be removed outright rather than preserved.

### Section format

For each deletion:
- `ID:`
- `Why delete is required:`
- `Where found:`
- `What must be removed:`
- `What must remain after deletion:`

### Typical deletion candidates
- alternate conscious-loop lifecycle
- direct `Decide -> Execute` path
- proposal creation outside ICS validation/authorization
- permit emission outside authorization
- direct resume-to-execute shortcut
- tool-surface widening path after governance/model preparation

## 10. Required test additions or updates section

Every remediation bill must include a test section grouped by remediation item.

### Section format

For each remediation item:
- `Remediation item ID:`
- `Tests to add:`
- `Tests to update:`
- `Why these tests close the drift:`

### Hard rule
A blocker is not considered fixed until the associated tests exist and pass.

## 11. Acceptance gate section

Every remediation bill must end with an acceptance gate.

### Required contents
- list of all blocking items
- statement that every blocking item must be complete
- statement that required tests must pass
- statement that a fresh re-audit is required before merge

### Required wording
A branch must not merge until:
- every blocking remediation item is complete
- every required test is present and passing
- a fresh repo audit returns `Compliant`

## 12. Final disposition rule

Use exactly one of:

- `Reject branch`
- `Reject until blockers fixed`
- `Accept after targeted cleanup`
- `Accept`

### Rule
If any blocking remediation item remains open, the only valid disposition is:

`Reject until blockers fixed`

## 13. Example remediation bill skeleton

Use this as the copy-ready starting point.

---

## Branch
`feat/inference-control-system`

## Audit date
`YYYY-MM-DD`

## Audit verdict
`Non-compliant`

## Reviewer
`NAME`

## Spec baseline
- `docs/architecture/ics-architectural-contract.md`
- `docs/architecture/ics-end-to-end-control-flow.md`
- `docs/architecture/ics-field-ownership-and-mutation-rules.md`
- `docs/architecture/ics-artifact-schemas.md`
- `docs/architecture/ics-compliance-test-bill.md`
- `docs/architecture/ics-repo-audit-checklist.md`

## Implementation baseline
`branch HEAD SHA`

## Scope
`conscious-loop ICS only`

## Blocking finding count
`N`

## Non-blocking finding count
`M`

# Executive summary

- `Merge status:` `Rejected`
- `Reason:` `Blocking architecture drift remains`
- `Primary risk:` `...`
- `Required next step:` `Complete all blocking remediation items before re-audit`

# Blocking remediation items

## ICS-RB-001
- `Severity:` `Blocking`
- `Rule violated:` `...`
- `Lifecycle stage:` `...`
- `Artifact or field affected:` `...`
- `Where found:` `...`
- `Observed behavior:` `...`
- `Why it is drift:` `...`
- `Required action:` `...`
- `Delete vs Refactor:` `...`
- `Acceptance criteria:`
  - `...`
  - `...`
- `Required tests:`
  - `...`
  - `...`
- `Dependencies:` `none`
- `Notes:` `...`

## ICS-RB-002
- `Severity:` `Blocking`
- `Rule violated:` `...`
- `Lifecycle stage:` `...`
- `Artifact or field affected:` `...`
- `Where found:` `...`
- `Observed behavior:` `...`
- `Why it is drift:` `...`
- `Required action:` `...`
- `Delete vs Refactor:` `...`
- `Acceptance criteria:`
  - `...`
  - `...`
- `Required tests:`
  - `...`
  - `...`
- `Dependencies:` `ICS-RB-001`
- `Notes:` `...`

# Non-blocking remediation items

## ICS-RB-101
- `Severity:` `Non-blocking`
- `Rule violated:` `...`
- `Lifecycle stage:` `...`
- `Artifact or field affected:` `...`
- `Where found:` `...`
- `Observed behavior:` `...`
- `Why it is drift:` `...`
- `Required action:` `...`
- `Delete vs Refactor:` `...`
- `Acceptance criteria:`
  - `...`
- `Required tests:`
  - `...`
- `Dependencies:` `none`
- `Notes:` `...`

# Required deletion list

## DEL-001
- `Why delete is required:` `...`
- `Where found:` `...`
- `What must be removed:` `...`
- `What must remain after deletion:` `...`

# Required test additions or updates

## For ICS-RB-001
- `Tests to add:`
  - `...`
- `Tests to update:`
  - `...`
- `Why these tests close the drift:` `...`

## For ICS-RB-002
- `Tests to add:`
  - `...`
- `Tests to update:`
  - `...`
- `Why these tests close the drift:` `...`

# Acceptance gate

A branch must not merge until:
- every blocking remediation item is complete
- every required test is present and passing
- a fresh repo audit returns `Compliant`

# Final disposition

`Reject until blockers fixed`

---

## 14. Relationship to other ICS documents

This template is subordinate to:
- `ics-architectural-contract.md`

It operationalizes:
- `ics-repo-audit-checklist.md`
- `ics-compliance-test-bill.md`

It should be used immediately after any audit that returns blocker findings.

---

[ICS docs index](inference-control-system/INDEX.md) · [ICS repo audit checklist](ics-repo-audit-checklist.md) · [docs INDEX](../INDEX.md)
