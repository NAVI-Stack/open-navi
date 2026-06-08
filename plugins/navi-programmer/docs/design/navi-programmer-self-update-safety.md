**Status:** Evolving  
**Last Updated:** 2026-04-12  
**Updated By:** ChatGPT

# NAVI Programmer — Self-Update Safety

## Purpose

This document defines the safety architecture for NAVI Programmer when the target codebase is NAVI itself.

Self-update is not the definition of NAVI Programmer, but it is the hardest early proving ground. It combines code mutation, validation, runtime behavior, and the risk of damaging the very system performing the work. This document exists to prevent unsafe self-modification patterns from becoming normalized during implementation.

This is a design document. It defines the safety model and architectural boundaries for self-update behavior. It does not freeze those rules as canon, and it does not act as the final code-backed implementation spec.

---

## Problem Statement

A programming-capable NAVI should eventually be able to improve NAVI itself.

That immediately creates a failure mode:

- NAVI changes its own code
- NAVI refreshes or restarts into that changed code
- the new code is broken
- the active instance is damaged, stuck, or unable to recover cleanly

That failure mode is unacceptable. It breaks the trust model, the evaluation model, and the recovery model.

The self-update problem is therefore not “can NAVI write code for itself?” It is:

> how can NAVI apply its programming capability to itself without relying on unsafe in-place mutation of the live running instance?

This document answers that question at the architecture level.

---

## Scope

This document applies only to:

- NAVI Programmer working on the NAVI repository
- safety boundaries around candidate changes to NAVI itself
- validation and promotion behavior for self-authored NAVI changes
- rollback and recovery expectations for failed candidate builds or bad updates

It does not cover:

- general programming tasks on external repositories
- general runtime architecture outside the self-update path
- low-level implementation details of every command or test harness
- release engineering or deployment beyond the self-update safety path

---

## Core Principle

**The running NAVI instance must not rely on unverified in-place self-modification as a normal operating path.**

That principle has direct consequences:

- the active instance should not patch itself and continue as though nothing changed
- self-authored changes should be applied to a candidate workspace or candidate build target
- candidate behavior must be validated before promotion-worthy actions
- the system must preserve a known-good control path capable of rejecting or recovering from bad candidate output

This is the single most important rule in this document.

---

## Design Goals

The self-update model should achieve all of the following.

### 1. Safe self-improvement
NAVI should be able to improve its own codebase without risking immediate loss of service or control.

### 2. Architecture consistency
Self-update should use the same general programming capability model as other programming tasks, with stricter safety boundaries rather than a completely separate one-off architecture.

### 3. Clear trust boundaries
The system should distinguish:
- live trusted runtime
- candidate untrusted build/result
- promotion decision

### 4. Reviewability
Every self-update attempt should produce reviewable evidence:
- task input
- changed files
- validation results
- candidate runtime results
- promotion recommendation

### 5. Recovery
Failure in the candidate path must not leave the live system without a working control path.

### 6. Extensibility
The model should support stronger automation later without requiring a rewrite of the basic safety structure.

---

## Non-Goals

This design does not attempt to:

- enable instant hot-reload of arbitrary self-authored code into the live instance
- guarantee that every self-update candidate is semantically correct
- replace human review in V1 for high-trust promotion decisions
- define the full deployment strategy for every future NAVI environment
- solve all testing and release engineering questions beyond the self-update safety path

---

## Threat Model

The self-update safety model must assume the following classes of failure are realistic.

### 1. Bad code generation
NAVI writes incorrect or incomplete code.

### 2. False-positive validation
A change appears to pass shallow checks but still breaks startup or runtime behavior.

### 3. Incomplete task execution
Files are changed, but the task is only partially complete or inconsistent.

### 4. Startup failure
The candidate NAVI instance fails to boot.

### 5. Runtime regression
The candidate boots but breaks core surfaces such as chat, tool invocation, governance, or persistence behavior.

### 6. Unsafe promotion
A candidate is promoted without adequate validation or despite known failure signals.

### 7. State corruption risk
A candidate runtime accidentally touches live state, ports, or connector sessions.

### 8. Control-plane loss
The currently running NAVI instance is replaced or broken before the candidate is proven safe.

The architecture must assume these can happen even when the programming capability is working “normally.”

---

## Key Terms

### Live Instance
The currently running trusted NAVI instance that remains responsible for the active control path.

### Candidate Workspace
An isolated copy, clone, or worktree of the NAVI repository where self-authored changes are applied.

### Candidate Build
The compiled, assembled, or otherwise runnable output generated from the candidate workspace.

### Candidate Runtime
A separate runnable NAVI instance launched from the candidate build under isolated conditions.

### Promotion
The act of treating a candidate result as ready for trusted use beyond the candidate path.

### Rollback
Returning to a known-good trusted state after a failed or rejected candidate attempt.

### Control Path
The stable operational path that remains capable of reporting status, halting unsafe actions, and preserving operator control.

---

## Safety Invariants

These invariants must hold for self-update behavior.

### Invariant 1 — No live in-place mutation as the normal path
The running instance must not depend on mutating its own live executable or live runtime environment and immediately continuing on the mutated version.

### Invariant 2 — Candidate work happens in isolated repo context
All self-authored code changes must be applied to a candidate workspace, not to the trusted live runtime artifact.

### Invariant 3 — Candidate runtime must be isolated
Any self-update execution test that involves running NAVI from candidate code must occur in an isolated runtime context.

### Invariant 4 — Live state must remain protected
Candidate execution must not share live persistent state, live network bindings, or privileged connector sessions unless explicitly designed and gated for that use case.

### Invariant 5 — Validation is mandatory before promotion
A self-update candidate is not promotion-ready because code changed. It becomes promotion-ready only after required validation steps complete and their outcomes are recorded.

### Invariant 6 — Failure does not equal silent fallback to success
If the candidate fails, times out, partially succeeds, or produces ambiguous results, the system must surface that explicitly and must not treat the update as successfully completed.

### Invariant 7 — A known-good recovery path must remain
At least one trusted control path must remain available to inspect or reject candidate results.

### Invariant 8 — Promotion is a distinct action
Creating a candidate and promoting a candidate are separate phases. They must not collapse into one implicit action.

---

## Architectural Model

The self-update safety model uses a three-layer structure.

### 1. Trusted Live Control Layer
This is the known-good running NAVI path.

Responsibilities:
- receive task input
- coordinate self-update workflow
- preserve operator visibility
- record outcomes
- approve, reject, or defer promotion according to governance
- remain available even if candidate work fails

This layer is trusted and stable relative to the candidate path.

### 2. Candidate Work Layer
This is where the actual self-authored repo work occurs.

Responsibilities:
- work in an isolated repo clone or worktree
- apply code changes
- run build/test/lint
- prepare candidate artifacts
- collect outputs and evidence

This layer is mutable and disposable.

### 3. Candidate Runtime Layer
This is where runtime behavior is exercised.

Responsibilities:
- boot candidate NAVI code in isolation
- run startup checks
- run smoke flows
- verify basic behavior
- emit structured health and failure signals

This layer is also disposable.

The trusted live layer should be able to kill, reject, or discard candidate layers without damaging itself.

---

## Why Isolation Is Required

Self-update without isolation causes multiple kinds of risk at once:

- the active instance may reload into broken code
- the candidate may corrupt live state
- validation may become indistinguishable from production mutation
- failures may remove the only working control path
- operator visibility may disappear exactly when needed most

Isolation is therefore not a nice-to-have. It is the minimum credible safety boundary.

---

## Candidate Workspace Design

Self-update work should happen in a candidate repo context.

Acceptable candidate repo strategies include:

- git worktree
- cloned temp workspace
- isolated branch checkout in a separate path
- container-mounted isolated repo view

### Required properties
A candidate workspace must:
- be distinct from the currently trusted working tree used by the live runtime
- have explicit repo root identity
- support diff and artifact inspection
- be disposable on failure
- allow branch/commit generation without mutating the trusted live runtime artifact

### Preferred V1 approach
A git worktree or isolated clone is preferred because it:
- preserves repo fidelity
- supports normal diff/branch workflows
- keeps candidate state easy to inspect
- minimizes weird special-case tooling

---

## Candidate Runtime Design

A successful self-update candidate requires more than passing unit-style checks. It must be able to run as NAVI.

The candidate runtime should therefore be launched as a separate instance using:

- separate process identity
- separate ports
- separate temp/config directory where needed
- separate state path
- non-live connector mode or stubbed connector configuration
- explicit lifetime controls

### Candidate runtime should not:
- reuse live SQLite state by default
- bind to the live gateway port
- hijack live connector sessions
- replace the trusted running instance automatically
- mutate production-facing state as part of ordinary smoke testing

### Candidate runtime should:
- boot with test-safe config
- prove it can initialize critical subsystems
- emit health/readiness signals
- support bounded smoke interactions
- terminate cleanly when done

---

## Validation Layers

Self-update validation should occur in layers.

### Layer 1 — Static and structural validation
Checks:
- file existence and shape
- syntax/parse sanity where applicable
- schema/manifest consistency where applicable

Purpose:
- fail fast on obviously broken candidate changes

### Layer 2 — Build and command validation
Checks:
- compile/build
- lint/static analysis
- targeted unit or package tests
- task-specific validation commands

Purpose:
- verify code artifacts are at least technically viable

### Layer 3 — Candidate runtime startup validation
Checks:
- process starts
- config loads
- dependency initialization succeeds
- health endpoint or equivalent readiness succeeds
- critical boot path does not panic

Purpose:
- verify the candidate can exist as NAVI, not just as code

### Layer 4 — Candidate runtime smoke validation
Checks:
- basic interaction path works
- basic tool/skill invocation path works
- basic governance/proposal path still responds
- basic persistence path behaves correctly in isolated state
- no immediate critical regressions are observed

Purpose:
- verify the candidate behaves like a minimally usable NAVI runtime

### Layer 5 — Promotion recommendation synthesis
Checks:
- all required layers passed
- no unresolved critical failures remain
- known issues are summarized
- promotion recommendation is explicit

Purpose:
- turn raw validation output into a governed decision surface

---

## Minimum V1 Smoke Suite

A V1 self-update candidate should not be considered ready without at least a minimal smoke suite.

The exact implementation may vary, but the candidate should prove all of the following in isolation:

### 1. Boot
Candidate instance starts without crash or deadlock.

### 2. Health
A health/readiness check returns success.

### 3. Session/basic request path
A minimal request or prompt path can complete.

### 4. Skill/tool surface bootstrap
The system can load and expose its required capability surface without fatal failure.

### 5. Governance path viability
A basic governed action path can still be evaluated or paused correctly.

### 6. Clean shutdown
The candidate instance can exit without leaving stuck processes or corrupt temp state.

Anything less than this is too weak to justify trust in self-authored runtime changes.

---

## Promotion Model

Promotion must be explicit.

A candidate should move through these states:

1. **Drafted** — candidate changes exist
2. **Validated Locally** — static/build/test layers have run
3. **Runtime Verified** — candidate runtime boot + smoke layers passed
4. **Promotion Recommended** — synthesis says the candidate is suitable for promotion
5. **Promoted** — human or policy-approved promotion action occurs
6. **Rejected** — candidate is not accepted
7. **Rolled Back / Discarded** — candidate context is abandoned or cleaned up

### Important rule
Promotion recommendation is not promotion.

In V1, promotion beyond candidate validation should remain human-confirmed for meaningful self-updates.

---

## Promotion Targets

Promotion may mean different things depending on environment.

Possible promotion targets:
- branch push
- PR creation
- staging artifact
- local deployment candidate
- trusted replacement build
- release handoff

For V1, the safest default is:

- generate reviewable branch/commit
- optionally push after confirmation
- optionally open PR after confirmation
- do not auto-replace the trusted running instance

This gives real value without pretending fully autonomous self-replacement is already safe.

---

## Rollback and Recovery

Rollback must be cheap and obvious.

### Candidate-level rollback
The most common rollback is simply:
- reject candidate
- discard candidate workspace/runtime
- keep trusted live instance unchanged

This should be the default failure path.

### Promotion-level rollback
If a promoted artifact later proves bad, the system should support:
- revert to known-good branch/build/reference
- explicit recovery reporting
- preservation of the failure evidence and promotion history

### Recovery expectations
Recovery should always preserve:
- task input
- candidate diff
- validation results
- failure reason
- operator-visible summary

---

## State Separation

Candidate execution should not be allowed to blur into live state.

### Candidate runtime should use isolated:
- config
- ports
- temp directories
- logs where practical
- sqlite/store path
- connector mode
- cache directories where relevant

### Default rule
Assume shared state is unsafe unless explicitly proven necessary and guarded.

This reduces the chance that “testing the candidate” mutates real operational state.

---

## Connector Safety During Self-Update

Connectors are especially risky during candidate execution.

The candidate runtime should default to one of the following connector modes:

### Mode A — Disabled
No external connectors initialize.

Best for:
- basic runtime smoke validation

### Mode B — Stubbed / fake
Connector interfaces exist but talk to no real external systems.

Best for:
- capability path validation without external side effects

### Mode C — Explicit limited test mode
Only narrowly allowed connectors initialize in restricted test-safe configuration.

Best for:
- later targeted integration tests

### V1 rule
Use Disabled or Stubbed by default. Real external connectors should not be part of ordinary self-update smoke validation.

---

## Governance Expectations

Self-update should remain inside shared governance, but the safety model implies stronger defaults.

### Expected default boundaries
- local candidate edits may be autonomous inside allowed repo scope
- remote push should require confirmation in V1
- PR creation should require confirmation in V1
- trusted runtime replacement should require explicit confirmation in V1
- irreversible self-update actions must not auto-execute just because the candidate looks promising

### Why
The risk profile of self-modification is higher than ordinary reversible local file work.

---

## Failure Handling

Self-update failures should be classified clearly.

### Failure classes
- task interpretation failure
- repo scope resolution failure
- file mutation failure
- build/test/lint failure
- candidate startup failure
- candidate smoke failure
- ambiguous validation outcome
- promotion rejection
- cleanup failure

### Required behavior
For every self-update attempt:
- record the failure class
- preserve the candidate evidence
- preserve trusted live control path
- avoid implicit promotion
- avoid implicit success framing

### Never do this
- overwrite trusted runtime and then discover whether it works
- treat candidate build success as runtime success
- suppress candidate failure because some earlier phase passed
- auto-promote because “the diff looks small”

---

## Reporting Requirements

Every self-update attempt should produce a structured summary.

At minimum, the summary should include:
- task or ticket origin
- candidate workspace reference
- changed files
- validation layers run
- pass/fail outcome for each layer
- candidate runtime result
- promotion recommendation
- unresolved risks
- cleanup/discard status if rejected

This is required both for trust and for weekly capability review.

---

## Preferred V1 Flow

The preferred V1 self-update flow is:

1. receive self-update task
2. resolve NAVI repo scope
3. create isolated candidate workspace
4. apply changes there
5. run local validation
6. build candidate
7. launch isolated candidate runtime
8. run minimal smoke suite
9. synthesize result
10. create branch/commit summary
11. request confirmation for any remote or promotion-worthy action
12. discard candidate on failure, preserve trusted live instance

This is the baseline safe path.

---

## What This Design Forbids

The following should be treated as forbidden or anti-pattern behavior for V1:

### 1. Live patch-and-continue
Mutating the running instance’s active code/runtime artifact and continuing as though the candidate were already trusted.

### 2. Shared live-state candidate boot by default
Launching candidate runtime against live operational state without strong reason and explicit protection.

### 3. Implicit promotion
Treating “candidate exists” or “tests passed” as equivalent to promotion.

### 4. Auto-replacing the trusted instance
Replacing the working live runtime automatically after candidate validation in V1.

### 5. Silent failure discard
Throwing away candidate failure information without persistent record.

---

## Open Design Questions

These remain unresolved and should be answered in implementation-facing specs:

- exact mechanism for candidate workspace creation
- exact mechanism for candidate runtime launch
- whether worktree or clone is the V1 default
- exact smoke suite contents and command shapes
- exact promotion command or artifact model
- exact rollback reference model
- how candidate execution is surfaced through runtime/gateway status
- how much of the smoke suite is reusable across repositories vs NAVI-specific

---

## Summary

Self-update is the highest-risk early use case for NAVI Programmer.

The correct safety model is not “let NAVI patch itself live and hope.” The correct model is:

- trusted live control path remains intact
- candidate work happens in isolated repo context
- candidate runtime is launched separately
- validation is layered
- promotion is explicit
- rollback is cheap
- failure remains first-class

That model gives NAVI a realistic path to safe self-improvement without pretending fully autonomous live self-replacement is already trustworthy.