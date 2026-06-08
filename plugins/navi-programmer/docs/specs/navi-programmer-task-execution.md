**Status:** Evolving
**Last Updated:** 2026-05-08
**Updated By:** ChatGPT

# NAVI Programmer — Task Execution

## Purpose

This specification defines how NAVI Programmer executes programming tasks from intake through reviewable output.

It is the execution contract for programming work. It defines the required task phases, task state semantics, decomposition rules, repo binding expectations, validation behavior, failure handling, and result reporting expectations for V1.

This document is implementation-facing. It is not canonical.

---

## Scope

This spec applies to programming tasks executed through the NAVI Programmer plugin, including:

- direct user-requested coding work
- ticket-driven coding work
- bounded repo inspection and code modification tasks
- self-update tasks that target the NAVI repository
- validation and result synthesis for programming work

This spec does not define:

- the full plugin architecture
- the self-update safety design in full detail
- release/deployment lifecycle after reviewable output exists
- the internal implementation details of every skill transport

---

## Normative Language

The key words **MUST**, **MUST NOT**, **SHOULD**, **SHOULD NOT**, and **MAY** in this document indicate requirement strength.

---

## High-Level Goal

NAVI Programmer task execution MUST convert a programming request into bounded, observable, reviewable work.

That means a programming task MUST NOT be treated as “done” merely because code was generated. The execution contract exists to ensure that programming work moves through a real lifecycle:

1. intake
2. normalization
3. scope binding
4. repo inspection
5. execution
6. validation
7. result synthesis
8. reviewable output

The local bounded mutation runner supports this lifecycle by normalizing raw
run intake, recording skill-result envelopes into an evidence ledger, checking
state transitions, and synthesizing a result envelope. NAVI core remains
responsible for invoking skills and storing durable task/run state.

---

## Task Classes

NAVI Programmer V1 MUST support the following task classes.

### 1. Read-only comprehension task
Examples:
- inspect a repo area
- explain how a subsystem works
- identify likely relevant files
- summarize implementation differences

Characteristics:
- no code mutation required
- no branch/commit required
- validation may not apply
- output is informational

### 2. Bounded mutation task
Examples:
- fix a bug
- add a small feature
- refactor a module
- add or update tests
- update docs related to code changes

Characteristics:
- file mutation required
- validation required where applicable
- reviewable output required

### 3. Ticket-driven mutation task
Examples:
- execute a Linear issue
- implement a task from structured work input

Characteristics:
- same as bounded mutation task
- task normalization must include ticket context
- acceptance target must be derived from the task source

### 4. Self-update task
Examples:
- add capability to NAVI
- fix a NAVI runtime issue
- implement a new NAVI plugin surface

Characteristics:
- same as bounded mutation task
- must follow isolated candidate behavior
- must report promotion readiness separately

---

## Required Task States

A programming task MUST move through explicit states.

The internal representation MAY vary, but the observable semantics MUST support the following states.

### 1. `received`
The task input exists but has not yet been normalized.

### 2. `normalized`
The task has been interpreted into a concrete execution target.

### 3. `scope_bound`
A repo/workspace target has been resolved.

### 4. `inspecting`
Relevant repo context is being gathered.

### 5. `planned`
The task has been reduced into executable work steps or an execution shape.

### 6. `executing`
Code or repo work is actively being performed.

### 7. `validating`
Build/test/lint or equivalent validation is being run.

### 8. `synthesizing`
Results are being summarized into structured execution output.

### 9. `review_ready`
A reviewable output exists.

### 10. `blocked`
The task cannot continue without dependency resolution, confirmation, or missing context.

### 11. `failed`
The task ended unsuccessfully.

### 12. `partially_succeeded`
Some meaningful work completed, but the overall target was not achieved cleanly.

### 13. `completed`
The task achieved its execution target and produced its required result surface.

### Rule
A mutation task MUST NOT move directly from `executing` to `completed` without passing through validation or explicitly recording why validation was not run.

---

## Task Intake

Programming tasks MAY originate from:

- direct user chat
- task source such as Linear
- directive/orchestration flow
- self-generated work item under governed conditions

### Intake requirements
At intake, the system MUST capture:

- task source
- raw task content
- any provided acceptance target
- any known repo/workspace context
- whether the task appears read-only or mutative
- whether the task targets NAVI itself

### Intake rule
Raw task content MUST be preserved. Normalization may reinterpret the task, but the original task input MUST remain available for audit and review.

---

## Task Normalization

Normalization converts raw task input into a concrete execution target.

### Normalization MUST produce:
- task summary
- mutation intent classification (`read_only`, `mutative`, `self_update`)
- likely acceptance target
- repo binding hint or requirement
- likely validation requirement
- whether decomposition is needed

### Normalization SHOULD produce:
- likely affected files or directories
- candidate command classes needed
- risk hints (local-only, remote mutation likely, runtime-affecting)

### Normalization MUST NOT:
- silently widen scope beyond the original task intent
- assume repo context if multiple reasonable targets exist
- suppress ambiguity

### Ambiguity rule
If normalization cannot determine a safe execution target, the task MUST transition to `blocked` or equivalent and MUST NOT proceed to mutation.

---

## Task Decomposition

Not every task requires deep planning, but every non-trivial mutation task requires an executable structure.

### Decomposition MUST answer:
- what is being changed
- where it is being changed
- what validation will test the change
- what counts as completion

### Minimal decomposition for small tasks
For small bounded tasks, the execution structure MAY be simple:
- inspect file(s)
- mutate file(s)
- validate
- summarize

### Required decomposition for larger tasks
If a task is broad, multi-file, or underspecified, decomposition SHOULD create:
- a sequence of bounded steps
- a likely file set
- a validation plan
- a stopping condition

### Rule
Decomposition MUST reduce complexity. It MUST NOT create ornamental task structure that does not improve execution safety or clarity.

---

## Repo / Workspace Binding

Every mutation task MUST bind to an explicit repo/workspace target before code changes occur.

### Binding requirements
Repo/workspace binding MUST determine:
- target repo
- target root path
- allowed scope
- whether the task is self-update
- whether the task requires isolated candidate context

### Binding sources MAY include:
- explicit user instruction
- current project/workspace context
- task-source metadata
- repo heuristics only when unambiguous

### Binding rule
If the system cannot safely determine the target repo/workspace, the task MUST block before mutation.

### Out-of-scope rule
The task MUST NOT mutate files outside the bound repo/workspace scope.

---

## Repo Inspection

Before mutation, NAVI Programmer MUST inspect enough repo context to avoid blind editing.

### Minimum repo inspection for mutation tasks
- identify relevant files
- read existing target file contents
- inspect surrounding context
- inspect current repo state as needed

### Repo inspection SHOULD include:
- related tests
- existing patterns or conventions
- nearby config/build files when relevant
- current branch and uncommitted state when lifecycle work is expected

### Rule
Blind overwrite of files without contextual inspection SHOULD NOT be treated as compliant task execution.

---

## Planning Requirements

For mutation tasks, planning MUST be sufficient to support bounded execution.

### A task plan MUST include:
- execution target
- likely file touch set or target region
- mutation strategy
- validation strategy

### A task plan MAY include:
- intermediate checkpoints
- retry policy hints
- branch naming hint
- commit summary draft

### Rule
The plan MAY remain lightweight, but the execution path MUST still be intelligible and reviewable.

---

## Code Mutation

Code mutation is the phase where repo state changes.

### Mutation requirements
During mutation, the system MUST:
- operate within bound repo scope
- preserve changed file visibility
- avoid hidden mutation outside the reported file set where practical
- produce diffable outcomes

### Mutation SHOULD be:
- targeted
- minimal relative to task intent
- aligned with existing repo patterns when applicable
- easy to inspect in diff form

### Mutation MUST NOT:
- silently rewrite unrelated parts of the repo
- treat file write success as task success
- mutate live self-update runtime state as the default self-update path

### Self-update rule
When the target is NAVI itself, mutation MUST occur in isolated candidate repo context, not as an unsafe live in-place runtime mutation path.

---

## Validation

Validation is a required execution phase for mutative programming tasks unless the task is explicitly exempt and that exemption is reported.

### Validation triggers
Validation MUST run when:
- code changes were made
- config or build-affecting files were changed
- tests were added or modified
- the task’s acceptance target implies executable correctness

### Validation MAY be omitted only when:
- the task is purely read-only
- the mutation is non-code and no meaningful validation path exists
- validation tooling is unavailable and that absence is recorded explicitly

### Minimum validation classes
NAVI Programmer SHOULD support:
- build
- test
- lint/static analysis

### Validation result capture
For each validation action, the system MUST capture:
- command
- working directory or scope
- exit status
- stdout/stderr or structured equivalent
- outcome class

### Validation outcome classes
At minimum:
- `passed`
- `failed`
- `not_run`
- `ambiguous`
- `timed_out`

### Completion rule
A mutative programming task MUST NOT be labeled `completed` without explicit validation outcome reporting.

---

## Repo Lifecycle Output

Mutation tasks MUST produce reviewable repo lifecycle output.

### Minimum required lifecycle outputs
For successful mutation work:
- diff visibility
- branch creation
- commit creation

### Strongly recommended lifecycle outputs
- branch push
- PR draft/creation
- validation summary attached to the review surface

### Lifecycle reporting MUST include:
- current branch or created branch
- whether a commit was created
- commit identifier if available
- whether remote actions occurred

### Rule
A mutation task that changes repo state but cannot be inspected or handed off cleanly is incomplete.

---

## Progress Reporting

NAVI Programmer task execution MUST be observable.

### Minimum progress checkpoints
The system SHOULD surface progress when entering these phases:
- task normalized
- repo bound
- inspecting
- mutating
- validating
- synthesizing
- review ready

### For longer tasks
Progress SHOULD include:
- current step
- active file or subsystem area
- validation currently running
- whether retries or replanning occurred

### Rule
The user or operator should not need to infer whether NAVI is reading, editing, testing, or stuck.

---

## Failure Handling

Programming task execution MUST treat failure as a first-class outcome.

### Failure classes
At minimum, the execution contract MUST distinguish:
- normalization failure
- repo binding failure
- dependency unavailable
- file mutation failure
- validation failure
- repo lifecycle failure
- self-update candidate failure
- confirmation blocked
- ambiguous result / insufficient evidence

### Failure behavior
On failure, the system MUST:
- preserve the failure class
- preserve changed-file evidence where applicable
- preserve validation evidence where applicable
- avoid claiming success
- report the resulting task state explicitly

### Partial completion behavior
If code was changed but the full target was not achieved cleanly, the task MUST be marked `partially_succeeded` or equivalent, not `completed`.

### Recovery behavior
The system MAY retry bounded reversible operations, but MUST NOT silently retry irreversible repo lifecycle actions without policy support.

---

## Confirmation / Blocking Semantics

Some programming actions require blocking behavior.

### Actions that SHOULD block by default in V1
- push to remote
- create PR remotely
- self-update promotion beyond candidate validation
- any explicitly owner-constrained remote mutation

### Blocking behavior MUST include:
- clear reason for block
- current task state
- what evidence already exists
- what action is awaiting confirmation

### Rule
Blocking is not failure. It is a governed pause in execution.

---

## Self-Update Task Semantics

Self-update tasks require stricter execution semantics.

### Self-update execution MUST:
- bind to NAVI repo explicitly
- use isolated candidate repo context
- separate mutation from candidate runtime validation
- separate candidate success from promotion
- report promotion readiness distinctly

### Self-update completion classes
For self-update work, the result surface SHOULD distinguish:
- local candidate mutation complete
- candidate validation passed
- candidate runtime verified
- promotion recommended
- promotion not recommended

### Rule
For self-update tasks, “code written and tests passed” is not equivalent to “safe to replace trusted running NAVI.”

---

## Required Result Surface

Every executed programming task MUST yield a structured result object or equivalent structured summary.

### Minimum required fields
- task source
- raw task input
- normalized task summary
- task class
- bound repo/workspace
- task state outcome
- files changed
- validation attempts
- validation outcomes
- branch/commit data where created
- block/failure reason where applicable
- final summary

### For self-update tasks, result surface MUST also include:
- candidate context used
- candidate runtime validation status
- promotion readiness recommendation

---

## Acceptance Criteria for Execution Compliance

A task execution implementation complies with this spec only if:

1. mutation tasks bind to explicit repo/workspace scope before changing code
2. mutation tasks inspect relevant repo context before writing
3. mutation tasks run or explicitly report validation
4. mutation tasks produce reviewable repo lifecycle outputs
5. failure and partial completion are surfaced explicitly
6. progress is observable at execution-phase granularity
7. self-update tasks follow isolated candidate semantics
8. result output is structured enough for audit, review, and weekly capability tracking

If any of these fail, the task execution path is not compliant.

---

## Summary

NAVI Programmer task execution is the contract that turns “do some coding” into bounded engineering work.

The required lifecycle is:

- intake
- normalization
- repo binding
- inspection
- mutation
- validation
- synthesis
- reviewable output

That lifecycle applies to both ordinary coding work and self-update work, with stricter safety semantics for the latter.
