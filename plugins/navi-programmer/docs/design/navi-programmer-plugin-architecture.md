**Status:** Evolving  
**Last Updated:** 2026-05-08
**Updated By:** ChatGPT

# NAVI Programmer Plugin Architecture

## Purpose

This document defines the architectural shape of NAVI Programmer as a first-class capability plugin.

It explains how NAVI Programmer should be structured inside the existing NAVI architecture, what responsibilities belong inside the plugin, what dependencies it has on shared NAVI systems, and what boundaries must remain outside the plugin.

This is a design document, not a canonical definition and not a code-backed implementation spec. It is meant to guide the V1 build and prevent architectural drift while the capability is being introduced.

---

## Design Goal

The design goal is to package programming capability into a reusable plugin architecture that:

- fits the existing Capability Layer model
- composes governed skills instead of bypassing them
- uses shared connectors rather than reinventing them
- reuses shared runtime/orchestration systems rather than forking them
- supports NAVI self-work first and can later extend the same governed model to
  other repositories
- can expand over time without requiring a new programming architecture every time scope grows

The repo’s conceptual architecture already defines plugins as packaged capabilities built from commands, skills, and connectors, while roles remain in the Experience/Cognitive layers. This design follows that model directly. :contentReference[oaicite:7]{index=7}

---

## Architectural Position

NAVI Programmer sits in the Capability Layer as a capability plugin.

It should be understood as a packaging and execution surface below the Cognitive Layer and above concrete skills, connectors, and transport mechanisms.

### It is invoked by:
- the Conscious process
- the Coder role path
- task or directive execution flows that select programming work

### It uses:
- programming-oriented skills
- developer and information connectors
- shared runtime/session execution
- shared governance
- shared execution outcome and proposal flows
- shared workspace/project context

### It does not own:
- global reasoning
- role selection
- the primitive command model
- the global connector framework
- core runtime lifecycle
- world model authority
- general governance logic

This keeps the plugin in the proper place in the stack.

---

## Architectural Model

NAVI Programmer should be treated as a bundle with five layers.

### 1. Capability Contract Layer
This is the plugin’s declared surface.

It defines:
- what NAVI Programmer provides
- what prerequisites it requires
- what capability families it includes
- what kinds of tasks it is meant to perform
- what governance expectations apply

This is the plugin-level identity layer.

### 2. Skill Layer
This contains the atomic executable skill interfaces used by the plugin.

Examples:
- file read
- file write
- patch apply
- repo status
- diff
- branch create
- commit
- push
- task normalize
- validation run
- issue ingest

The skills canon already requires skills to remain declarative, versioned, and governed, with structured interfaces and explicit side-effect metadata. NAVI Programmer should rely on that model rather than inventing opaque programming super-tools. :contentReference[oaicite:8]{index=8}

### 3. Connector Integration Layer
This contains the external system dependencies needed by the plugin.

Examples:
- local repo/workspace access
- GitHub
- Linear
- web/content acquisition
- optional CI systems later
- optional external model/coding systems later

The plugin should use the shared connector framework rather than embedding connector logic directly into each programming workflow.

### 4. Workflow Layer
This coordinates multi-step programming behavior.

Examples:
- ticket → plan → execute
- repo inspection → patch → validate
- self-update → candidate build → test → promote
- issue → branch → PR summary

This is the layer where programming work becomes coherent and task-oriented rather than just a bag of low-level skills.

### 5. Safety and Governance Layer
This is the plugin-specific application of shared governance and execution safety.

Examples:
- local reversible edit rules
- remote mutation confirmation rules
- self-update isolation rules
- retry/stop/review boundaries
- promotion gates for self-changes

This layer does not replace core governance. It specializes it for programming work.

---

## Why a Plugin Instead of “Just Skills”

A pure skill-only model is too flat for programming capability.

Programming is not one action. It is a structured family of actions that requires:

- capability discovery
- dependency grouping
- common safety rules
- shared task execution patterns
- reusable status and evaluation behavior
- connector coordination across repo, issue tracker, validation commands, and remote hosting surfaces

Skills are necessary but insufficient. A plugin provides the packaging boundary that says:

> these skills, connectors, workflows, and constraints together form one coherent capability surface

Without that boundary, programming capability would fragment into:
- disconnected low-level skills
- duplicated workflow logic
- inconsistent governance assumptions
- connector-specific behavior leaking into the core runtime

That would rot quickly.

---

## Relationship to the Coder Role

The Coder role determines that the current work is programming-oriented.

NAVI Programmer supplies the capability surface the Coder role uses to perform that work.

This division should be preserved:

### Coder role responsibilities
- work-mode selection
- user-facing behavioral framing
- choosing programming-oriented action paths in cognition
- deciding to invoke programming capability

### NAVI Programmer responsibilities
- providing programming skills
- exposing workflow entry points
- binding connectors and repo context
- enforcing programming-specific safety constraints
- returning structured results for reflection and follow-up

The role is not the plugin. The plugin is not the role.

---

## Relationship to Shared Runtime and Orchestration

NAVI Programmer should reuse the shared runtime and orchestration systems already documented in the implementation architecture rather than introducing a parallel execution framework. The repo’s architecture document already identifies shared components for runtime coordination, session execution, directive orchestration, worker execution, and skill execution. :contentReference[oaicite:9]{index=9}

### Reused shared systems
- `internal/runtime`
- `internal/navi`
- `internal/orchestrator`
- `internal/navi/skill`
- shared store/worldmodel integration
- shared proposal and execution outcome recording

### Plugin-specific logic
- programming workflow composition
- programming task normalization
- programming capability selection
- repo-aware execution coordination
- plugin-level safety constraints
- evaluation-specific programming checks

### Architectural rule
NAVI Programmer must not create a second runtime, a second orchestration engine, or a second governance path.

It should plug into shared execution systems and specialize behavior only where programming work actually requires specialization.

---

## Plugin Boundaries

## Inside the plugin

The following belong inside NAVI Programmer:

### Capability declaration
- plugin identity
- capability metadata
- capability requirements
- capability discovery tags

### Programming skill bundle
- programming-oriented skill definitions and grouping
- execution interface definitions
- plugin-level capability mapping

### Workflow logic
- task intake normalization
- programming execution lifecycle
- validation workflow selection
- reporting/summarization for programming tasks

### Programming-specific policy application
- local vs remote mutation expectations
- self-update safety requirements
- repo/workspace scope handling
- irreversible action handling specific to programming work

### Evaluation hooks
- what validation steps the plugin expects
- what evidence of success/failure it reports
- how plugin-level maturity is measured

## Outside the plugin

The following must remain shared core NAVI concerns:

### Role model
- Character / Assistant / Coder
- presentation-layer behavior
- role selection logic

### Core governance engine
- validation order
- proposal queue mechanics
- autonomy system
- system-tier policy rules

### Core runtime lifecycle
- session wake/sleep
- run coordinator
- inbox model
- heartbeat
- persistence plumbing

### Core connector platform
- registration
- lifecycle
- transport bootstrapping
- health/degraded-state tracking

### Core skill platform
- discovery
- validation
- transport execution
- schema enforcement

### World model ownership
- entity persistence
- mutation provenance
- execution outcome records
- proposal state

This boundary is important. NAVI Programmer should extend NAVI, not colonize it.

---

## Capability Families Inside the Plugin

The plugin should be organized around capability families rather than arbitrary feature piles.

### 1. Repository Comprehension
Purpose:
- inspect repo structure
- identify relevant files
- understand implementation surfaces
- trace relationships between modules

Example sub-capabilities:
- repo inventory
- code search
- dependency path tracing
- change impact estimation

### 2. Code Mutation
Purpose:
- perform bounded file-level or patch-level changes
- create or update code artifacts
- maintain reviewable diffs

Example sub-capabilities:
- write file
- replace in file
- patch apply
- scaffold file
- refactor targeted area

### 3. Validation
Purpose:
- test whether changes hold against repo reality

Example sub-capabilities:
- build
- test
- lint
- command result inspection
- bounded repair loop

### 4. Repository Lifecycle
Purpose:
- manage git and review workflow

Example sub-capabilities:
- status
- diff
- branch create
- commit
- push
- PR create
- PR summary

### 5. Task-Driven Programming
Purpose:
- convert tickets/specs into executable programming work

Example sub-capabilities:
- task ingest
- ticket normalize
- decomposition
- step planning
- execution progress reporting

### 6. Self-Improvement Execution
Purpose:
- apply the same programming capability to NAVI itself under stronger safety rules

Example sub-capabilities:
- self-repo worktree handling
- candidate build
- isolated test instance execution
- promotion readiness summary

---

## Connector Dependencies

NAVI Programmer should not depend on every connector at once. It should have a layered dependency model.

### Core V1 connector dependencies
These are the minimum credible set:

- workspace/local filesystem access
- shell/process execution surface
- repository host surface such as GitHub
- task source such as Linear
- web/content access for research and library lookup where needed

### Near-term optional dependencies
- CI provider connector
- artifact/report publishing connectors
- additional repository hosts
- documentation platform integration

### Later optional dependencies
- external coding-agent systems
- IDE/editor integrations
- broader deployment/observability systems

### Architectural rule
Connector dependence should be declared, not assumed.

The plugin should know which of its workflows require which connectors and degrade explicitly when they are unavailable.

---

## Skill Design Expectations

All skills used by NAVI Programmer should follow the existing skill model: structured, governed, explicit about effects, and transport-aware. :contentReference[oaicite:10]{index=10}

Programming-specific skill expectations:

### Atomicity
A skill should do one small thing or one tightly related family of things.

Bad:
- “implement feature from issue”

Good:
- “read file”
- “apply patch”
- “run command”
- “create branch”

### Structured I/O
Every skill should return structured outputs suitable for:
- reflection
- retry logic
- reporting
- persistence
- evaluation

### Declared side effects
Programming skills often involve:
- filesystem reads
- filesystem writes
- process execution
- remote system writes

Those effects must be declared at the skill level rather than hidden inside the implementation.

### Reversibility awareness
A local file patch and a remote push are not the same class of action.
Programming skills should make that difference machine-readable.

---

## Workflow Architecture

The plugin should expose stable workflow patterns rather than forcing cognition to improvise the whole programming sequence every turn.

At minimum, V1 should standardize these workflow families:

### Workflow A: Spec-driven repo work
1. accept spec/task
2. normalize task
3. inspect repo
4. choose target files/areas
5. mutate code
6. run validation
7. summarize result

### Workflow B: Ticket-driven repo work
1. ingest issue/ticket
2. normalize acceptance target
3. decompose when needed
4. inspect repo
5. implement
6. validate
7. branch/commit/report

### Workflow C: Self-update candidate flow
1. accept NAVI change task
2. resolve self-repo context
3. operate in isolated worktree/candidate context
4. implement
5. validate
6. build candidate instance
7. run smoke checks
8. summarize promotion readiness

These workflows should be explicit enough to keep behavior stable, but not so rigid that every repo or task type becomes impossible.

---

## Safety Architecture

Programming work is not just another low-risk plugin category. It combines:

- local mutable state
- shell/process execution
- remote repository actions
- self-modification pressure
- potentially irreversible changes

So the plugin architecture must embed safety assumptions from the start.

### Safety rule 1: repo scope must be explicit
The plugin must know what repo/workspace it is allowed to touch.

### Safety rule 2: live instance mutation is not a normal path
For NAVI self-work, the plugin must not assume it can patch the currently running instance and continue safely.

### Safety rule 3: local reversible work and remote irreversible work are different classes
They should not share the same default autonomy threshold.

### Safety rule 4: validation is part of mutation
A change is not complete because bytes changed. It is only complete when validation has been attempted and the outcome is recorded.

### Safety rule 5: partial completion must remain explicit
If code changed but validation failed, the result is partial or failed, not “done.”

This aligns directly with the repo’s broader failure model, which already treats partial, failed, and degraded outcomes as first-class rather than hidden states. :contentReference[oaicite:11]{index=11}

---

## Registration and Discovery Model

NAVI Programmer should be discoverable as a first-class plugin in the capability registry.

At a high level, discovery should answer:

- what does this plugin provide
- what systems does it require
- what skills belong to it
- what workflows it supports
- what trust/safety bounds apply
- what level of autonomy is allowed

The plugin should advertise capability families such as:
- repository_comprehension
- bounded_code_mutation
- evidence_gated_programming_workflow
- validation_execution
- repository_lifecycle
- confirmation_gated_remote_review
- task_driven_programming
- self_update_candidate_workflow

This lets cognition choose the plugin intentionally rather than selecting random low-level skills without a coherent programming package.

---

## Dependency Model

NAVI Programmer has both hard and soft dependencies.

### Hard dependencies
These are required for the plugin to function meaningfully:
- skill runtime
- workspace/repo scope resolution
- process execution surface
- at least one repository interaction surface
- execution outcome recording

### Soft dependencies
These enhance value but are not required for the plugin to exist:
- Linear ingestion
- PR creation
- CI status integration
- external coding-agent support
- advanced evaluation/reporting surfaces

The architecture should fail clearly when hard dependencies are absent and degrade visibly when soft dependencies are absent.

---

## V1 Architectural Shape

For V1, the plugin should be deliberately narrow.

### Included in V1
- local repo comprehension
- bounded file mutation
- build/test/lint execution
- git branch/commit workflow
- task intake from chat
- task intake from Linear soon after
- self-update through isolated candidate path
- status reporting and result summarization

### Excluded from V1
- generalized multi-repo swarm orchestration
- automatic self-merge
- unrestricted autonomous remote mutation
- IDE replacement scope
- broad deployment automation
- “works on any environment instantly” assumptions

That exclusion list matters because V1 fails when it tries to become a complete software engineer platform before the foundation works.

---

## Expected Evolution

The architecture should support expansion without reclassification.

That means:
- new skills can be added without redesigning the plugin
- new connectors can be attached without changing plugin identity
- new workflows can be added without breaking V1 workflows
- self-coding and external-repo coding remain the same capability plugin with different safety/application contexts

The stable unit is the plugin identity. The internal capability surface should be allowed to grow.

---

## Anti-Patterns to Avoid

### 1. Monolithic programmer tool
Do not build a single “programmer.do_work” super-skill and call it architecture.

### 2. Core-runtime leakage
Do not stuff plugin-specific programming logic into unrelated runtime packages just because it is faster in the moment.

### 3. Connector-owned workflow logic
Do not let GitHub or Linear integration become the place where programming orchestration secretly lives.

### 4. Self-coding special-case architecture
Do not create a weird one-off self-edit path that bypasses the general programming model.

### 5. Validation as an afterthought
Do not treat test/build/lint as optional add-ons after code generation exists.

### 6. Ticket-first architecture
Do not let tickets define the architecture. The plugin contract should define the tickets.

---

## Open Design Questions

The following still need resolution in implementation-facing specs:

- exact plugin registration shape
- plugin metadata schema for capability discovery
- minimum required V1 skill list
- whether task decomposition lives mostly in plugin workflow code or shared orchestrator paths
- exact repo/workspace binding mechanism
- how self-update candidate instances are launched
- how plugin-level evaluation hooks are surfaced
- where plugin-level status reporting best attaches in the runtime/gateway flow

---

## Summary

NAVI Programmer should be built as a first-class capability plugin in the Capability Layer.

It should package:

- programming skills
- developer connectors
- programming workflows
- safety constraints
- evaluation/reporting behavior

It should reuse NAVI's shared runtime, governance, connector, and skill
infrastructure rather than creating parallel systems. Its current design center
is NAVI itself; broader repository support should come later by applying the
same contracts, not by generalizing the harness prematurely.
