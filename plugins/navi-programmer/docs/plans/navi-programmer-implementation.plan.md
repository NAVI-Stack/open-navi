**Status:** Evolving
**Last Updated:** 2026-04-12
**Updated By:** ChatGPT

# NAVI Programmer — Implementation Plan

## Purpose

This document defines the phased implementation plan for NAVI Programmer V1.

It translates the concept, architecture, and implementation-facing specs into a concrete build sequence that minimizes conflict with existing NAVI work, reduces architectural drift, and establishes the shortest viable path to a real programming capability.

The plan is intentionally incremental. It is designed to get NAVI Programmer working as a real first-class capability plugin without destabilizing the broader system.

---

## Planning Goal

The implementation goal is:

> establish NAVI Programmer as a real first-class capability plugin that can execute bounded, validated, reviewable programming work on NAVI itself first

The plan optimizes for:
- minimal conflict with existing runtime/orchestration work
- reuse of shared systems
- high observability
- safe self-update behavior
- real repo lifecycle output
- capability maturity over ticket count

## Current MVP Slice

The immediate implementation slice is tracked in [project-centered-programming-mvp.plan.md](project-centered-programming-mvp.plan.md).

That plan narrows this broader NAVI Programmer roadmap into the first usable project-centered coding pass:

- first-class Project CRUD
- optional workspace binding for general projects
- required workspace binding for coding projects before mutation
- project-scoped chats and task intake
- project-aware coding execution
- sandboxed validation before coding tasks can be called complete

---

## Non-Goals of the Initial Plan

This plan does not attempt to:
- solve all future programming capability needs at once
- build an IDE replacement
- build a full autonomous software organization layer
- implement unrestricted self-replacement of live NAVI
- optimize for theoretical elegance over practical sequencing

The priority is a working, reviewable V1 loop.

---

## Implementation Principles

### 1. Build the plugin shape before feature sprawl
Do not scatter programming behavior across the codebase before the plugin contract exists.

### 2. Reuse shared core systems
Do not fork runtime, governance, orchestration, or connector infrastructure unless a real architectural gap forces it.

### 3. Establish local truth before remote automation
Local repo comprehension, file mutation, validation, and branch/commit matter more than PR polish early on.

### 4. Validation is part of implementation, not a later add-on
Any milestone that mutates code without validation is incomplete.

### 5. Self-update safety must arrive early
Because NAVI working on NAVI is a core proving ground, unsafe self-modification behavior cannot be deferred indefinitely.

### 6. Tickets should implement docs, not replace them
This plan assumes the core documentation set exists first.

---

## Phase Structure

The plan is organized into seven phases.

1. Documentation and contract locking
2. Plugin scaffold and capability registration
3. Local repo comprehension and mutation substrate
4. Validation and repo lifecycle substrate
5. Task-driven execution loop
6. Self-update candidate safety path
7. Task source integration and hardening

Each phase builds on the previous one.

---

## Phase 0 — Documentation Baseline

### Goal
Create the minimum doc corpus that will outlast tickets and guide implementation.

### Required outputs
- `docs/concepts/navi-programmer.md`
- `docs/design/navi-programmer-plugin-architecture.md`
- `docs/design/navi-programmer-self-update-safety.md`
- `docs/specs/navi-programmer-plugin-v1.md`
- `docs/specs/navi-programmer-task-execution.md`
- `docs/plans/navi-programmer-implementation.plan.md`
- `docs/plans/navi-programmer-evaluation.plan.md`

### Completion criteria
- docs exist
- plugin identity is clear
- self-update safety constraints are documented
- task execution lifecycle is explicit
- V1 success criteria are defined

### Why first
Without these docs, implementation will drift into ad hoc runtime patches and ticket-local assumptions.

---

## Phase 1 — Plugin Scaffold

### Goal
Make NAVI Programmer real as a named first-class capability plugin before trying to make it powerful.

### Required work
- define plugin identity and registration path
- define plugin discovery/capability metadata
- define plugin boundary relative to shared runtime and shared skill system
- wire plugin to shared capability registry or equivalent mechanism
- create plugin-level structure for skills/connectors/workflows

### Expected outputs
- identifiable plugin package or registration surface
- stable plugin name / identifier
- plugin capability metadata visible to the system
- plugin-level dependency declaration

### Must not happen in this phase
- broad feature sprawl
- custom runtime fork
- giant monolithic “programmer tool” pretending to be the plugin

### Completion criteria
- NAVI Programmer exists as a discoverable first-class capability plugin
- the system can identify what it provides and what it depends on
- no major programming behavior is still purely “floating” without plugin ownership

---

## Phase 2 — Local Repo Comprehension and Mutation Substrate

### Goal
Give the plugin the minimum local engineering substrate needed to understand and change code safely.

### Required capability work
- repo scope resolution
- file read
- directory/listing awareness
- code/repo search
- file write/update
- patch-style mutation
- diff visibility

### Required behavior
- all file operations honor repo/workspace scope
- mutation operations are targeted and diffable
- repo inspection occurs before blind mutation
- changed file visibility is preserved

### Expected outputs
- working local repo comprehension path
- working bounded file mutation path
- reviewable changed-file set

### Risks in this phase
- file mutation without explicit scope binding
- writing before inspection
- hidden side effects outside reported file set

### Completion criteria
- plugin can inspect and modify a target repo locally in bounded scope
- changed files are visible
- repo context is explicit in execution output

---

## Phase 3 — Validation and Repo Lifecycle Substrate

### Goal
Turn code mutation into real programming work by attaching validation and reviewable repo lifecycle behavior.

### Required capability work
- run build commands
- run test commands
- run lint/static commands
- capture validation evidence
- inspect repo status
- create branch
- create commit

### Strongly recommended capability work
- branch naming conventions
- commit summary synthesis
- diff summary synthesis

### Required behavior
- mutative tasks do not skip validation silently
- validation evidence is structured
- branch and commit creation are reviewable
- task result distinguishes success from partial success

### Expected outputs
- working build/test/lint execution path
- branch/commit review loop
- structured validation reporting

### Completion criteria
- plugin can mutate code, validate it, and produce reviewable branch+commit output
- validation results are captured and surfaced cleanly
- a human can review the outcome without reconstructing what happened from raw logs

---

## Phase 4 — Task-Driven Execution Loop

### Goal
Move from “manually invoke repo tools” to “execute a programming task through a defined lifecycle.”

### Required capability work
- task intake
- task normalization
- repo/workspace binding
- decomposition/planning for non-trivial tasks
- phase-level progress reporting
- result synthesis

### Required workflow behavior
- task classes are distinguished
- non-trivial tasks are decomposed enough to be executable
- task phases are visible
- blocked states are explicit
- partial completion is explicit

### Required integration
- plugin reuses shared runtime/orchestration path
- plugin does not create a second execution framework

### Expected outputs
- bounded programming tasks can be executed end-to-end
- user can see progress at the phase level
- execution results are structured

### Completion criteria
- given a direct user coding task, NAVI Programmer can run the full lifecycle from task input to review-ready output
- the lifecycle is visible and auditable
- failure modes are surfaced explicitly

---

## Phase 5 — Self-Update Candidate Safety Path

### Goal
Enable NAVI Programmer to work on NAVI itself without unsafe live in-place mutation.

### Required capability work
- explicit NAVI repo binding
- isolated candidate repo/work context
- candidate validation path
- candidate runtime launch path
- minimal smoke validation
- promotion-readiness reporting

### Required behavior
- self-update does not use unsafe live patch-and-continue as the normal path
- candidate success is distinct from promotion
- trusted control path remains intact on candidate failure
- self-update results include promotion recommendation rather than vague “done” language

### Expected outputs
- NAVI can perform bounded self-update work in isolated candidate context
- candidate runtime behavior can be checked
- bad candidates do not destroy the live control path

### Completion criteria
- self-targeted tasks follow isolated candidate semantics
- candidate validation results are visible
- the system can reject/discard a bad candidate without breaking the trusted running path

### Why this phase matters
Without this phase, “NAVI working on NAVI” remains a dangerous demo trick rather than a credible capability.

---

## Phase 6 — Task Source Integration

### Goal
Allow programming work to begin from a real task source rather than only direct chat instruction.

### Required capability work
- ingest task from Linear or equivalent task source
- map issue/task fields into normalized programming task input
- preserve issue/task provenance in execution reporting

### Recommended follow-on work
- update issue status or comment progress
- attach reviewable outputs back to the task source
- sync completion evidence

### Expected outputs
- ticket-driven execution path
- issue provenance in task execution result
- reduced manual hand-copying of work into chat

### Completion criteria
- NAVI Programmer can begin a bounded programming task from a real task source
- the resulting execution path is materially the same as direct chat execution, not a separate architecture

---

## Phase 7 — Hardening and Review Readiness

### Goal
Make the V1 loop dependable enough to use regularly.

### Required hardening work
- improve failure classification
- improve blocked-state reporting
- improve validation output clarity
- improve diff/commit summary quality
- improve self-update smoke reliability
- reduce false-success cases
- close obvious retry and cleanup gaps

### Recommended hardening work
- branch push support
- PR draft support
- richer progress summaries
- better capability introspection
- better dependency degradation handling

### Completion criteria
- the plugin is usable for repeated bounded tasks
- the failure modes are understandable
- the review surface is stable enough for weekly tracking and practical use

---

## Dependency Order

Implementation SHOULD follow this dependency order:

1. docs baseline
2. plugin scaffold
3. local repo comprehension
4. local mutation
5. validation
6. branch/commit lifecycle
7. task-driven lifecycle
8. self-update candidate safety
9. task source ingestion
10. hardening

### Rule
Do not move remote automation or fancy integrations ahead of local substrate and validation. That creates fragile demos instead of a reliable capability.

---

## Recommended Milestone Breakdown

### Milestone A — Plugin Exists
Includes:
- Phase 0
- Phase 1

Definition of done:
- NAVI Programmer is a visible first-class capability plugin with a documented contract

### Milestone B — Local Engineering Loop Exists
Includes:
- Phase 2
- Phase 3

Definition of done:
- NAVI can inspect, mutate, validate, and commit bounded local repo work

### Milestone C — Task Lifecycle Exists
Includes:
- Phase 4

Definition of done:
- NAVI can take a programming task and run it through a visible execution lifecycle

### Milestone D — Safe Self-Work Exists
Includes:
- Phase 5

Definition of done:
- NAVI can work on NAVI through isolated candidate semantics

### Milestone E — Ticket-Driven Workflow Exists
Includes:
- Phase 6
- Phase 7

Definition of done:
- NAVI can pull real work from a task source and execute it with review-ready output and acceptable reliability

---

## Suggested Work Sequencing Inside the Repo

The exact file/package choices depend on implementation details, but work SHOULD generally flow in this order:

### Early repo touch areas
- plugin registration / capability exposure surface
- skill definitions / plugin-owned skill grouping
- repo/workspace binding path
- process execution path for build/test/lint
- repo lifecycle path for branch/commit

### Mid-phase touch areas
- task normalization and execution state reporting
- result synthesis/reporting
- self-update candidate orchestration

### Later touch areas
- Linear integration
- PR/push support
- review polish
- hardening and cleanup

### Rule
Avoid deep invasive edits across unrelated core packages all at once. Keep changes phased and attributable to the plugin’s actual needs.

---

## Risks

### 1. Plugin identity drift
Risk:
Programming behavior gets implemented without a coherent plugin boundary.

Mitigation:
Finish plugin scaffold before feature sprawl.

### 2. Runtime leakage
Risk:
Plugin-specific behavior gets stuffed into shared core runtime code.

Mitigation:
Keep plugin-owned workflow logic inside the plugin boundary unless a shared primitive is truly needed.

### 3. Validation bypass
Risk:
Mutation gets built out faster than validation.

Mitigation:
Do not treat mutation-only milestones as complete.

### 4. Unsafe self-modification
Risk:
Early self-coding shortcuts mutate the live instance in place.

Mitigation:
Self-update candidate path is a required phase, not a “later nice-to-have.”

### 5. Connector-led architecture
Risk:
GitHub or Linear integration starts defining the programming model.

Mitigation:
Build the programming lifecycle first, then attach task sources and remote review surfaces.

### 6. Ticket-local architecture
Risk:
Implementation decisions get made implicitly inside ticket work.

Mitigation:
Keep docs current and require tickets to map back to the documented plan.

---

## Minimum Exit Criteria for V1

NAVI Programmer V1 is implementation-complete only when all of the following are true:

1. the plugin exists as a first-class capability plugin
2. bounded local repo comprehension works
3. bounded local code mutation works
4. validation works and is surfaced explicitly
5. branch+commit reviewable output exists
6. direct user programming tasks run through the lifecycle
7. self-targeted NAVI tasks use isolated candidate semantics
8. task source ingestion exists or is immediately tractable on top of the same lifecycle
9. the system no longer depends on hand-wavy “it sort of coded something” success criteria

---

## Weekly Review Guidance

Weekly review SHOULD track capability maturity against this plan rather than ticket volume alone.

Each weekly review should ask:

- Which phase(s) moved materially this week?
- Which required capability families are now real?
- Which execution phases are still weak or implicit?
- What self-update safety gaps remain?
- What validation or review gaps remain?
- What still blocks regular use?

### Suggested weekly maturity view
Track each as:
- not started
- partial
- usable
- reliable

Across:
- plugin scaffold
- repo comprehension
- mutation
- validation
- branch/commit
- task lifecycle
- self-update candidate path
- task-source integration
- hardening

---

## Summary

This implementation plan is the shortest credible path to a real NAVI Programmer V1.

The order matters:

- define the docs
- establish the plugin
- build local repo substrate
- attach validation and repo lifecycle
- build the task lifecycle
- add safe self-update candidate behavior
- add task-source integration
- harden until it is actually usable

That sequence gives NAVI Programmer a real foundation instead of a pile of scattered coding features.
