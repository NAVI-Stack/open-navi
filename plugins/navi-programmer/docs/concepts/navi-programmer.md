**Status:** Evolving  
**Last Updated:** 2026-05-08
**Updated By:** ChatGPT

# NAVI Programmer

## Purpose

NAVI Programmer is NAVI’s first-class programming capability plugin.

It is the capability surface that enables NAVI to understand software work, inspect and modify codebases, interact with developer systems, validate changes, and execute programming tasks with governed autonomy.

NAVI Programmer is currently optimized for NAVI’s own repository. Its long-term
purpose is broader programming capability across permitted codebases, but the
active package contract and validation work are NAVI-first.

---

## Why This Exists

The Coder role in NAVI defines *when* NAVI is acting as an agentic coding partner. NAVI Programmer defines *how* that coding capability is packaged, surfaced, and executed inside the Capability Layer. The role and the capability are related but not identical.

- **Coder role** = a mode of work selection and presentation in the Experience/Cognitive layers.
- **NAVI Programmer** = a packaged capability plugin in the Capability Layer that supplies the concrete programming surface used by the Coder role.

This distinction matters because NAVI must be able to behave like a coding agent without hardcoding all programming behavior into the core runtime or treating every programming action as bespoke logic. The system needs a reusable, governed, extensible programming capability, not a one-off “self-edit mode.” The conceptual architecture already separates roles from the Capability Layer and defines plugins as packaged capabilities built from skills, commands, and connectors. :contentReference[oaicite:0]{index=0}

---

## What NAVI Programmer Is

NAVI Programmer is a capability plugin that packages together:

- governed programming skills
- developer-facing connectors
- execution workflows
- safety and autonomy constraints
- evaluation and reporting behavior
- repository-aware task execution logic

Its purpose is to let NAVI perform programming work in a way that is:

- reusable across repositories
- structured rather than improvised
- testable
- reviewable
- governed
- extensible over time

NAVI Programmer is therefore not a single skill, not a connector, and not a role. It is a packaged capability plugin that uses multiple skills and connectors to provide a coherent programming surface. This matches the repo’s model where skills are modular capability units, while connectors and plugins package integration and capability surfaces at a higher level. :contentReference[oaicite:1]{index=1} :contentReference[oaicite:2]{index=2}

---

## Core Definition

**Definition:**  
NAVI Programmer is a first-class capability plugin that enables NAVI to perform programming work through governed skills, developer connectors, and execution workflows, with enough structure and safety to operate on its own codebase first.

This definition has five important implications:

1. **It is NAVI-first.**
   It is not a self-edit loophole, but the current proving ground and contract target is NAVI itself.

2. **It is packaged.**  
   It is not “the coding parts of NAVI scattered everywhere.” It should have an identifiable, coherent capability contract.

3. **It is governed.**  
   Programming work includes filesystem changes, repo mutation, process execution, remote system access, and potentially irreversible actions. That cannot sit outside the governance model.

4. **It is compositional.**  
   It should be built from multiple atomic skills and shared integration surfaces rather than a giant monolithic tool.

5. **It is extensible.**  
   It should be able to grow from NAVI repo work into broader software engineering capability without requiring core architectural reinvention.

---

## Relationship to NAVI Architecture

NAVI Programmer exists inside the Capability Layer.

It does not replace the Cognitive Layer. It does not replace the World Model. It does not redefine governance. It does not invent new primitive commands. Instead, it packages a programming capability surface that the Cognitive Layer can invoke when the active work is coding-related. The repo’s conceptual architecture already defines the Capability Layer as the place where commands, connectors, and plugins live, and explicitly describes plugins as packaged capabilities built on commands, connectors, and skills. :contentReference[oaicite:3]{index=3}

NAVI Programmer therefore depends on, but does not own:

- the Coder role
- the Conscious process
- Validate/Govern
- the world model
- runtime/session orchestration
- task decomposition infrastructure
- proposal/confirmation flows
- execution outcome recording

It should plug into those systems, not bypass them.

---

## Relationship to Skills

Skills remain the atomic governed capability units inside NAVI. NAVI Programmer is not a replacement for skills; it is a plugin that packages multiple programming-oriented skills into a coherent capability bundle. The repo’s skills canon already defines skills as versioned declarative modules with structured interfaces, transports, effects, reversibility, and trust metadata. :contentReference[oaicite:4]{index=4}

Within NAVI Programmer, skills should remain narrow and composable.

Examples of likely programming skill families:

- repository inspection
- file reading
- file mutation
- diff and patch operations
- build/test/lint execution
- git lifecycle actions
- issue ingestion
- pull request operations
- web/code research support
- evaluation and reporting

The plugin is the package. The skills are the governed executable units inside that package.

---

## Relationship to Connectors

NAVI Programmer depends on developer and information connectors rather than replacing them.

Typical connectors that belong in or around NAVI Programmer include:

- local filesystem or workspace connectors
- repository hosting connectors such as GitHub
- issue/task connectors such as Linear
- web/content acquisition connectors for research
- CI/build system connectors later
- optional external coding-agent connectors later

Connectors provide access to systems and transport surfaces. NAVI Programmer provides the higher-level capability bundle that uses those connectors to complete programming work. The conceptual architecture already distinguishes connectors as integration adapters and plugins as packaged capabilities built on top of those adapters. :contentReference[oaicite:5]{index=5}

---

## Relationship to Commands

NAVI Programmer does not define new primitive commands.

It composes existing primitive commands already defined by NAVI, especially:

- `Query`
- `Create`
- `Update`
- `Invoke`
- `Acquire`
- `Delegate`
- `Compose`

For example:

- reading repository state is primarily `Query` and `Acquire`
- invoking a file mutation skill is `Invoke`
- creating a branch or commit may be modeled through governed `Invoke` and internal state recording
- executing a full ticket workflow is a `Compose` of multiple lower-level actions

This matters because NAVI Programmer must fit the existing architecture rather than expanding the command model every time a new programming behavior appears. :contentReference[oaicite:6]{index=6}

---

## Long-Term Goal

The long-term goal of NAVI Programmer is:

> make NAVI capable of programming, in any language, on any codebase it is permitted to access, through a governed and extensible capability surface

That includes:

- reading and understanding repositories
- navigating project structure
- breaking large work into smaller executable units
- implementing specifications
- updating existing systems with minimal drift
- validating changes against tests, static checks, and runtime behavior
- collaborating through issue trackers and pull requests
- operating across multiple repositories and projects
- improving its own programming capability over time

This goal is intentionally broader than “let NAVI patch itself.”

---

## Near-Term Goal

The near-term goal is narrower:

> prove NAVI Programmer by enabling NAVI to work on NAVI itself safely and reliably

This proving ground matters because it exercises the hardest parts early:

- repo awareness
- governed local mutation
- build/test validation
- status reporting
- branch/commit discipline
- failure recovery
- non-destructive self-update practices

If NAVI Programmer cannot safely work on NAVI, it does not yet qualify as a serious programming capability.

---

## V1 Win Condition

NAVI Programmer V1 succeeds when NAVI can do the following on NAVI’s own repository:

1. accept a programming task from a user or task source
2. inspect the relevant repo/workspace
3. break the task into executable steps when necessary
4. read and modify files locally within allowed scope
5. run validation commands such as build, test, and lint
6. summarize what changed and what passed or failed
7. create a reviewable branch and commit
8. push or prepare a PR only within configured governance bounds
9. avoid damaging the live running instance through unsafe self-modification

Anything short of that may still be useful infrastructure, but it is not the V1 win condition.

---

## Core Design Principles

### 1. General capability first, self-coding second

Self-programming is a proving ground, not the definition.

NAVI Programmer must first make working on NAVI reliable. The execution model
should avoid bespoke shortcuts that would prevent later use on other permitted
repositories, but this pass should not generalize beyond NAVI before the NAVI
harness is solid.

### 2. Plugin packaging, not monolithic sprawl

Programming capability should not exist as scattered partial logic across runtime, ad hoc tools, and connector-specific hacks. It should be packaged as a first-class capability plugin with a recognizable contract.

### 3. Skills should stay atomic

The plugin may be broad. Individual skills should not be.

“Do programming” is not a good skill.  
“Read file,” “apply patch,” “run tests,” and “create branch” are.

### 4. Governance applies fully

Programming work can mutate files, repositories, remote systems, and execution environments. That means programming capability must remain inside NAVI’s normal governance and confirmation model rather than acting as a special unrestricted path.

### 5. Validation is part of the capability

Code generation without validation is not programming capability. It is draft generation.

A real programming capability must include validation against repo reality:
- build status
- tests
- lint/static analysis
- targeted runtime checks where applicable

### 6. Traceability is mandatory

Programming actions must be reviewable:
- what changed
- why it changed
- what was validated
- what failed
- whether recovery or confirmation is needed

### 7. Live self-mutation is unsafe by default

NAVI Programmer must never depend on in-place modification of the currently running instance as its normal operating model. Safe candidate execution and promotion must be part of the capability.

---

## Scope of the Plugin

NAVI Programmer includes all programming-specific capability concerns needed to perform repo work safely and coherently.

That includes:

- programming-oriented skill surfaces
- repo/task-aware workflows
- developer connector usage
- programming execution state transitions
- programming-specific safety and reporting expectations
- evaluation logic for programming tasks

It does **not** include ownership of:

- core runtime/session loop
- global governance model
- world model semantics
- general-purpose connector framework
- the existence of the Coder role itself
- primitive command definitions

Those remain shared NAVI systems.

---

## Typical Task Classes

NAVI Programmer should eventually support multiple classes of programming work.

### Repository comprehension
- inspect structure
- identify relevant files
- trace behavior across modules
- summarize architecture or implementation

### Code modification
- edit existing files
- add new files
- refactor targeted components
- implement bounded features
- fix bugs

### Validation work
- run build/test/lint
- inspect failure output
- retry bounded changes
- produce validation summaries

### Repository lifecycle work
- branch creation
- commit creation
- push
- PR drafting
- review response support

### Task-driven execution
- ingest issue/ticket/spec
- normalize and decompose work
- plan execution
- track progress
- emit summaries and status

### Self-improvement work
- identify capability gaps
- accept specs for new capability work
- implement and validate improvements to NAVI itself
- do so without breaking the stable running instance

---

## What NAVI Programmer Is Not

NAVI Programmer is not:

- a new role replacing the Coder role
- a single skill
- a single connector
- a freeform “agent mode”
- unrestricted shell access without governance
- a self-edit loophole outside normal policy
- a guarantee of perfect spec compliance
- an IDE replacement in V1
- an excuse to push architecture decisions into tickets without documentation

It is a structured programming capability package.

---

## Expected Growth Path

NAVI Programmer should grow in layers.

### Stage 1 — Local repo execution
Focus on:
- repo inspection
- local file work
- tests/build/lint
- branch/commit/report

### Stage 2 — Task-source integration
Focus on:
- Linear ingestion
- task normalization
- progress updates
- issue-linked execution traces

### Stage 3 — Review and collaboration
Focus on:
- PR creation
- review response support
- CI awareness
- richer evaluation

### Stage 4 — Later multi-repo and broader language support
Focus on:
- other repositories
- stronger environment handling
- richer build systems
- more external developer surfaces

### Stage 5 — Self-improving programming substrate
Focus on:
- improving the capability itself
- guarded plugin expansion
- stronger autonomous decomposition and execution

The important point is that each stage should extend the same capability plugin rather than replacing it with a different architecture every few months.

---

## Safety Expectations

Programming capability is unusually sensitive because it combines:
- read/write local state
- process execution
- external systems
- potentially irreversible actions
- self-modification pressure

For that reason, NAVI Programmer must be built with stronger safety expectations than a generic low-risk capability plugin.

At minimum, it should assume:

- repository scope must be explicit
- mutation must be bounded to allowed workspaces
- validation must occur before promotion-worthy actions
- remote mutation should have stricter confirmation than local reversible work
- self-update must use isolated candidate execution rather than hot-replacing the running instance

The details live in the design and spec docs, but the concept must acknowledge these constraints up front.

---

## Relationship to Weekly Review

NAVI Programmer should be tracked as a capability, not as a vague initiative.

Weekly review should evaluate:
- what parts of the plugin contract exist
- what parts are still missing
- what task classes pass reliably
- what safety gates are working
- what still causes drift or execution failure

In other words, weekly review should measure capability maturity, not just ticket throughput.

---

## Open Conceptual Questions

These questions are still open at the concept level and should be resolved in design/spec docs:

- how the plugin is registered and surfaced to the Coder role
- what the minimum built-in skill set is for V1
- how repo/workspace binding is represented
- how much planning/decomposition belongs inside the plugin vs shared orchestration
- how self-update candidate execution is launched and verified
- when push/PR should require confirmation by default
- how much programming autonomy should be configurable separately from general autonomy

---

## Summary

NAVI Programmer is NAVI’s first-class programming capability plugin.

It exists to package programming skills, developer connectors, execution workflows, and safety constraints into a reusable capability surface that the Coder role can invoke.

Its long-term goal is broad programming capability across languages and codebases. Its first proving ground is NAVI itself. It must therefore be designed as a general capability package with strong validation, traceability, and self-update safety rather than as a fragile one-off self-edit mode.
