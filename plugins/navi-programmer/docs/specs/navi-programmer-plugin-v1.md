**Status:** Evolving  
**Last Updated:** 2026-05-08
**Updated By:** ChatGPT

# NAVI Programmer Plugin V1

## Purpose

This document defines the V1 implementation-facing specification for the NAVI Programmer plugin.

NAVI Programmer V1 is the first packaged programming capability for NAVI. It is
designed to let NAVI perform bounded, governed programming work with enough
structure to operate on the NAVI repository itself first. Later repository
support should reuse the same governed surface instead of changing the V1
identity model.

This document specifies the minimum required V1 plugin shape, components, capability families, dependencies, workflows, and acceptance criteria.

This is a mutable implementation-facing spec. It is not canonical.

---

## Scope

This spec defines:

- the V1 identity and purpose of the NAVI Programmer plugin
- the minimum plugin composition required for V1
- the minimum capability families required for V1
- the minimum connectors and execution surfaces required for V1
- the required workflow entry points for V1
- the minimum governance expectations for V1
- the V1 success criteria

This spec does not define:

- the full long-term programming capability roadmap
- all future connectors or external systems
- full release engineering and deployment design
- the exact final data model of every runtime artifact
- the complete self-update safety design beyond what V1 requires

---

## Normative Language

The key words **MUST**, **MUST NOT**, **SHOULD**, **SHOULD NOT**, and **MAY** in this document indicate requirement strength.

---

## Plugin Identity

### Working plugin name
`NAVI Programmer`

### Proposed plugin identifier
`navi.programmer`

This identifier is a working V1 proposal and may be adjusted if repository naming conventions require a different plugin identifier, but V1 implementation SHOULD use one stable identifier and SHOULD NOT scatter programming capability under multiple competing plugin names.

### Skill identifier convention
V1 skill IDs use the plugin package slug plus the skill folder slug:
`navi-programmer.<skill-folder>`, for example
`navi-programmer.repo-inspect`. `plugin.yaml` component references MUST match
the `skill_id` declared in each `SKILL.yaml`.

### Classification
NAVI Programmer V1 is a **first-class capability plugin**.

It is not:
- a role
- a single skill
- a connector
- a one-off workflow
- a temporary special-case path for self-coding

### Primary purpose
Provide NAVI with a packaged programming capability surface that supports:
- repo understanding
- bounded code mutation
- validation
- repository lifecycle actions
- task-driven execution
- safe self-update work on NAVI

---

## V1 Success Statement

NAVI Programmer V1 succeeds when NAVI can:

1. accept a bounded programming task
2. inspect the relevant repository
3. identify and modify relevant files inside allowed scope
4. run validation commands
5. summarize changes and validation outcomes
6. create a reviewable branch and commit
7. optionally push or prepare a PR under governance
8. do the above on NAVI’s own repository without relying on unsafe live in-place mutation of the running instance

If those conditions are not met, V1 is incomplete.

---

## Architectural Position

NAVI Programmer V1 MUST exist as a plugin-level capability surface within the Capability Layer.

It MUST:
- reuse the shared skill system
- reuse the shared connector framework
- reuse the shared runtime/session/orchestration systems
- reuse shared governance and proposal flows

It MUST NOT:
- create a second governance path
- create a second runtime framework
- hardcode programming behavior directly into unrelated core packages as the primary architecture
- rely on unstructured freeform execution without a defined plugin contract

---

## V1 Plugin Composition

NAVI Programmer V1 MUST include the following categories of components.

### 1. Plugin capability contract
The plugin MUST have:
- a stable plugin identity
- a declared capability surface
- a declared dependency set
- a defined V1 scope
- clear boundaries between included and excluded behavior

### 2. Programming skill bundle
The plugin MUST include or depend on a minimum set of programming-oriented skills.

### 3. Developer/information connector usage
The plugin MUST use shared connectors or shared execution surfaces for repo access, task input, and external developer operations.

### 4. Programming workflow layer
The plugin MUST define stable workflow paths for task-driven programming execution.

### 5. Validation layer
The plugin MUST treat validation as part of the programming capability rather than optional follow-up behavior.

### 6. Reporting layer
The plugin MUST produce structured evidence of what it did, what changed, and what passed or failed.

### 7. Self-update-safe path
For NAVI self-work, the plugin MUST support isolated candidate execution rather than assuming live in-place mutation of the active runtime.

---

## Required Capability Families

NAVI Programmer V1 MUST include the following capability families.

### A. Repository Comprehension
Purpose:
- inspect repo layout
- find relevant files
- understand code relationships enough to support bounded work

Minimum V1 expectations:
- list relevant files/directories
- search for symbols, strings, or patterns
- read file content
- summarize repo-relevant task context

### B. Code Mutation
Purpose:
- modify code and related repository files in bounded scope

Minimum V1 expectations:
- write new file content
- replace/update existing file content
- apply bounded patch-like edits
- preserve reviewable diff structure

### C. Validation
Purpose:
- test candidate changes against repo reality

Minimum V1 expectations:
- run build commands
- run test commands
- run lint/static validation commands where applicable
- capture stdout/stderr/exit status
- distinguish passed, failed, and ambiguous results

### D. Repository Lifecycle
Purpose:
- move work into reviewable repo state

Minimum V1 expectations:
- inspect repo status
- inspect diff
- create branch
- create commit
- summarize change set

Push and PR support are strongly recommended for V1, but branch+commit reviewability is the minimum irreversible lifecycle threshold.

### E. Task-Driven Execution
Purpose:
- take programming work as structured execution rather than one-off ad hoc mutation

Minimum V1 expectations:
- accept task input from chat
- normalize task into executable work
- identify likely relevant files
- execute bounded work steps
- report progress and outcome

### F. Self-Update Safety
Purpose:
- allow the same plugin to operate on NAVI itself safely

Minimum V1 expectations:
- isolate self-update candidate work
- avoid live in-place mutation as the normal path
- validate candidate changes before promotion-worthy actions
- summarize promotion readiness separately from mere code generation

---

## Required V1 Skill Surface

The plugin MUST expose or depend on a minimum atomic skill surface.

The exact skill identifiers may vary, but V1 MUST provide equivalent capability to the following.

### Repository inspection skills
- read file
- list directory / repo subtree
- search code / repo text
- inspect repository status
- inspect repository diff

### Code mutation skills
- write file
- replace/update file content
- apply patch or patch-like structured mutation

### Validation skills
- run build command
- run test command
- run lint/static command
- capture command results in structured form

### Repository lifecycle skills
- create branch
- create commit
- inspect current branch/state

### Task execution skills
- normalize task/spec into execution target
- summarize execution status
- summarize changed files and outcomes

### Optional but highly desirable V1 skills
- push branch
- create PR
- ingest issue from task source
- fetch web/doc context for programming task support

### Skill design rule
No V1 skill SHOULD try to implement the entire programming workflow by itself.

V1 skills MUST remain narrow enough to:
- be governed cleanly
- be retried selectively
- produce structured outputs
- support compositional workflows

---

## Required V1 Execution Surfaces

NAVI Programmer V1 MUST have access to the following execution surfaces.

### 1. Repository-scoped local file access
The plugin MUST be able to read and mutate files within an explicitly resolved repo/workspace scope.

### 2. Process execution surface
The plugin MUST be able to run bounded commands for:
- build
- test
- lint
- other task-relevant validation commands

### 3. Repository state surface
The plugin MUST be able to inspect repo status and produce reviewable diffs.

### 4. Task-input surface
The plugin MUST accept programming work from direct user instruction.
V1 SHOULD also support task ingestion from Linear or an equivalent task source.

### 5. Reviewable output surface
The plugin MUST be able to produce branch/commit-grade output that a human can inspect and continue from.

---

## Required V1 Connector / Dependency Set

The plugin MUST declare and rely on the following categories of dependencies.

### Required core dependencies
- shared skill runtime
- shared governance path
- shared execution outcome reporting
- shared workspace/project/repo context resolution
- local filesystem or equivalent repo mutation access
- process execution surface
- repository lifecycle surface

### Required task source dependency
At minimum:
- direct user task input through chat or directive path

### Strongly recommended V1 dependencies
- GitHub or equivalent repository host surface
- Linear or equivalent issue/task surface

### Optional V1 dependencies
- web research/content acquisition for programming support
- CI provider integration
- documentation system integration

### Dependency failure behavior
If a required dependency is absent, the plugin MUST fail clearly and MUST NOT pretend the capability is available.

If a recommended dependency is absent, the plugin MAY degrade, but it MUST surface that degradation explicitly in task execution or capability introspection.

---

## Repository Scope Rules

NAVI Programmer V1 MUST operate within explicit repository/workspace boundaries.

### Requirements
- every programming task MUST resolve to a repo/workspace context before mutation
- file mutation MUST be rejected or paused when scope is ambiguous
- out-of-scope mutation MUST NOT occur silently
- self-update work on NAVI MUST resolve the NAVI repo explicitly
- repo context SHOULD be visible in progress and result reporting

### Rationale
Programming capability without explicit repo scope is how accidental cross-project mutation happens.

---

## V1 Workflow Entry Points

NAVI Programmer V1 MUST support the following workflow entry points.

### Workflow 1 — Chat task to repo work
Input:
- natural language programming request from user

Required behavior:
1. normalize task
2. resolve repo/workspace context
3. inspect relevant repo areas
4. perform bounded changes
5. run validation
6. summarize result
7. create reviewable branch/commit state

### Workflow 2 — Ticket-driven repo work
Input:
- issue/ticket/task record from a task source such as Linear

Required behavior:
1. ingest task
2. normalize acceptance target
3. decompose when needed
4. resolve repo context
5. perform bounded changes
6. run validation
7. summarize result
8. create reviewable branch/commit state

### Workflow 3 — Self-update candidate work
Input:
- programming task targeting NAVI itself

Required behavior:
1. resolve NAVI repo context
2. create isolated candidate repo/work context
3. perform bounded changes there
4. run validation
5. build or prepare candidate runtime where required
6. run minimal candidate runtime checks
7. summarize promotion readiness separately from local code success

---

## Required V1 Task Execution Semantics

V1 task execution MUST be structured.

### Required execution phases
For a non-trivial programming task, V1 MUST be able to move through these phases:

1. **Task Intake**
2. **Task Normalization**
3. **Repo Context Resolution**
4. **Repo Inspection**
5. **Change Planning**
6. **Code Mutation**
7. **Validation**
8. **Result Synthesis**
9. **Reviewable Output Creation**

The internal implementation MAY collapse some of these steps, but the observable behavior MUST preserve equivalent semantics.

### Minimum reporting expectation
The plugin MUST be able to report:
- what repo it is operating on
- what files it changed
- what commands it ran
- what passed
- what failed
- whether the result is ready for review

---

## Validation Requirements

Validation is mandatory for V1.

### Required rule
A programming task MUST NOT be treated as complete solely because files were changed.

### Required validation behavior
For every programming task that mutates code, the plugin MUST attempt at least one relevant validation path unless:
- the task is explicitly read-only
- no relevant validation path exists and that absence is surfaced explicitly

### Minimum validation evidence
The plugin MUST record:
- command invoked
- exit status
- stdout/stderr or structured equivalent
- whether validation passed, failed, or remained ambiguous

### Validation result classes
V1 MUST distinguish:
- validation passed
- validation failed
- validation not run
- validation ambiguous/incomplete

### Completion rule
A task with unrun or failed validation MAY still produce useful artifacts, but it MUST NOT be reported as fully successful without qualification.

---

## Repository Lifecycle Requirements

NAVI Programmer V1 MUST produce reviewable output.

### Minimum required lifecycle behavior
For a successful programming task, V1 MUST support:
- diff visibility
- branch creation
- commit creation

### Strongly recommended V1 lifecycle behavior
- push branch
- open PR
- include summarized validation results in review output

### Governance boundary
Push and PR actions SHOULD default to confirmation-gated behavior in V1 unless the environment and owner settings explicitly allow more autonomy.

---

## Self-Update Requirements

When the target repository is NAVI itself, the plugin MUST apply stricter safety behavior.

### Required V1 self-update rules
- self-update work MUST occur in isolated candidate repo context
- live in-place self-mutation MUST NOT be the normal path
- candidate validation MUST occur before promotion-worthy actions
- candidate runtime checks SHOULD occur for meaningful runtime-affecting changes
- promotion readiness MUST be reported separately from ordinary task success
- replacing the trusted running instance MUST NOT be automatic in V1

These rules are mandatory for credible self-update safety.

---

## Required Result Surface

Every V1 programming task MUST yield a structured result.

### Minimum result content
- task summary
- repo/workspace target
- files changed
- commands run
- validation outcomes
- diff/commit/branch references where available
- overall outcome classification
- next-action recommendation

### Outcome classifications
At minimum, V1 MUST distinguish:
- succeeded
- partially_succeeded
- failed
- blocked_requires_confirmation
- blocked_missing_dependency
- blocked_scope_resolution

### Why
The plugin must support reflection, review, retries, and weekly evaluation. Freeform prose alone is not enough.

---

## Governance Expectations

NAVI Programmer V1 MUST remain fully governed.

### Required governance behavior
- programming skills MUST not bypass shared validation/governance flow
- irreversible or remote-impact actions MUST be treated more strictly than reversible local edits
- confirmation-required actions MUST pause or produce proposal-equivalent behavior rather than auto-executing
- self-update promotion MUST be treated as a distinct governed action

### Suggested V1 defaults
- local read-only repo work: autonomous
- local reversible file edits in allowed scope: autonomous or bounded-autonomous
- branch creation: autonomous
- commit creation: autonomous with visible reporting
- push: confirmation by default
- PR creation: confirmation by default
- live replacement/promotion of self-update candidate: confirmation by default

---

## Excluded From V1

The following are explicitly out of scope for V1 unless later added intentionally.

- automatic self-merge of self-authored PRs
- automatic live instance replacement
- broad deployment/orchestration automation
- multi-repo swarm programming as baseline behavior
- IDE replacement scope
- unlimited autonomous shelling across arbitrary local machine scope
- “any language, any environment” completeness as a V1 requirement

V1 is about establishing the plugin and proving the loop, not pretending the full end-state already exists.

---

## Observability Requirements

NAVI Programmer V1 MUST be reviewable in operation.

### Minimum observable progress
The plugin SHOULD surface phase-level progress such as:
- resolving repo
- reading files
- mutating files
- running tests
- synthesizing result

### Minimum post-run observability
The system MUST be able to inspect:
- changed files
- commands run
- validation outputs
- branch/commit identifiers where created
- final outcome class

---

## Acceptance Criteria

NAVI Programmer Plugin V1 is acceptable only if all of the following are true.

### Capability shape
- the plugin has a stable identifiable capability surface
- it is implemented as a plugin-level package rather than scattered ad hoc logic
- it uses shared skills/connectors/runtime rather than bypassing them

### Repository work
- NAVI can inspect a target repo
- NAVI can identify and change relevant files in allowed scope
- NAVI can show what changed

### Validation
- NAVI can run and capture at least one relevant validation path
- validation outcomes are structured and surfaced explicitly

### Reviewability
- NAVI can create a branch and commit for successful bounded tasks
- the resulting work is reviewable by a human

### Task-driven execution
- NAVI can execute from a user-supplied programming task
- NAVI can normalize the task into actionable work
- task status and result are visible

### Self-update safety
- NAVI can perform candidate self-work without relying on live in-place mutation of the trusted running instance
- candidate results are clearly separated from promotion
- bad candidates do not destroy the trusted control path

If any of these fail, V1 is not done.

---

## Deferred Items

These are reasonable next-wave expansions but are not required to declare V1 complete.

- richer Linear automation
- richer GitHub PR automation
- CI-aware validation loops
- multi-repo capability bundles
- deeper language/environment coverage
- external coding-agent integrations
- stronger autonomous decomposition
- richer evaluation harnesses
- automated promotion in carefully bounded environments

---

## Summary

NAVI Programmer Plugin V1 is the first packaged programming capability for NAVI.

V1 requires:
- a real plugin-level capability bundle
- atomic programming skills
- repo-aware task execution
- mandatory validation
- reviewable lifecycle output
- governed remote actions
- safe self-update candidate behavior

V1 is complete when NAVI can use this plugin to do real, reviewable, validated
programming work on NAVI itself without depending on unsafe live self-mutation.
