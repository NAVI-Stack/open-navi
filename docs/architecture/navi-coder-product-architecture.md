# NAVI Coder Product Architecture

**Status:** Active  
**Last Updated:** 2026-06-05  
**Updated By:** ChatGPT

## Purpose

This document translates the canonical NAVI Coder definition into implementation architecture.

It defines how NAVI Coder should appear in the console, how it should exist in the backend, how it should use skills/plugins/connectors, and how code execution should be isolated without splitting Coder away from the NAVI platform identity.

Canonical source of truth: [`docs/canonical/navi-coder.md`](../canonical/navi-coder.md).

---

## Architecture Summary

NAVI Coder is a first-class product domain inside the NAVI platform.

It should be implemented as:

1. a top-level console product route
2. a backend Coder domain/control plane in `navid`
3. persistent Coder task/run/review state
4. isolated Coder worker execution for risky repository operations
5. Coder-owned capability packs that use skills, connectors, and workflows internally
6. candidate runtime containers for NAVI self-update validation

The correct shape is:

```text
NAVI Platform
├── NAVI Console
│   ├── Chat
│   └── Coder
├── navid / NAVI Core
│   ├── Chat runtime
│   ├── Coder control plane
│   ├── Governance
│   ├── World model
│   ├── Skill and connector registries
│   └── Event / telemetry store
└── Isolated execution
    ├── Coder workers
    └── Candidate NAVI runtimes
```

---

## Product Boundary

Coder is a product domain, not a plugin.

The product boundary includes:

- task intake
- repository/project selection
- run lifecycle
- plan and execution state
- code/diff review
- validation output
- branch/commit/PR handoff
- self-update candidate checks
- coding history and evaluation evidence

The current `plugins/navi-programmer` package is a prototype implementation substrate. It must not remain the product identity.

---

## Console Boundary

The console should expose Coder as a top-level surface.

Recommended route structure:

```text
/coder
/coder/projects
/coder/projects/:projectId
/coder/repos/:repoId
/coder/tasks/:taskId
/coder/runs/:runId
/coder/reviews/:reviewId
/coder/settings
```

The Coder UI should be a coding workspace, not normal chat with hidden tools.

Required surface areas:

- project/repo selector
- task brief
- task conversation
- plan view
- run timeline
- file tree
- diff viewer
- validation/log panel
- branch/commit status
- PR/review handoff
- candidate runtime panel for self-update runs

A practical Coder screen should look like:

```text
┌────────────────────────────────────────────────────────────┐
│ NAVI Coder                                                 │
├───────────────┬──────────────────────────────┬─────────────┤
│ Projects/Repos│ Task, plan, diff, validation │ Conversation│
│ Tasks/Runs    │ logs, review state           │ and controls│
└───────────────┴──────────────────────────────┴─────────────┘
```

---

## Backend Boundary

`navid` should own a Coder domain/control plane.

Recommended backend package direction:

```text
internal/coder/
├── domain/
├── store/
├── service/
├── api/
├── workers/
├── workflows/
└── evaluation/
```

The exact package structure can evolve, but Coder state and behavior should not remain only in plugin scratchpads or generic chat runtime state.

### Domain objects

Coder should introduce persistent records for:

- `CoderProject`
- `CoderRepository`
- `CoderTask`
- `CoderRun`
- `CoderRunStep`
- `CoderArtifact`
- `CoderDiff`
- `CoderValidationResult`
- `CoderReview`
- `CoderCandidateRuntime`
- `CoderPromotionDecision`

These records should support console rendering, reviewability, run resumption, evaluation, and audit.

---

## API Boundary

Coder should expose product-level API routes.

Recommended route family:

```text
GET    /api/coder/projects
POST   /api/coder/projects
GET    /api/coder/projects/:projectId
GET    /api/coder/repos
POST   /api/coder/repos
GET    /api/coder/tasks
POST   /api/coder/tasks
GET    /api/coder/tasks/:taskId
POST   /api/coder/runs
GET    /api/coder/runs/:runId
GET    /api/coder/runs/:runId/events
POST   /api/coder/runs/:runId/approve
POST   /api/coder/runs/:runId/cancel
POST   /api/coder/runs/:runId/promote
GET    /api/coder/reviews/:reviewId
```

These APIs may call existing skills and workflows internally, but the public/backend domain API should be Coder-shaped, not plugin-shaped.

---

## Control Plane Responsibilities

The Coder control plane should:

- accept coding tasks
- resolve project/repo/workspace binding
- create Coder task and run records
- select models/workflows/tools for coding work
- schedule isolated worker execution
- enforce policy before risky steps
- receive worker evidence
- persist run steps and artifacts
- stream progress to the console
- pause for approval when needed
- coordinate branch/commit/PR review state
- coordinate candidate runtime checks for self-update work
- classify outcomes and evaluation evidence

The Coder control plane is not the worker. It is the coordinator and authority.

---

## Worker Boundary

Risky work belongs in Coder workers or equivalent isolated execution contexts.

Coder workers should handle:

- repo clone/worktree preparation
- file reads and writes
- patch application
- build/test/lint commands
- git branch/commit operations
- review artifact generation
- candidate runtime launch
- evidence return

Workers must run under explicit scopes and policies. They must not become independent task authorities.

Recommended worker shape:

```text
coder-worker
├── assigned CoderRun
├── mounted approved repo/workspace
├── limited network policy
├── command allowlist
├── timeouts
├── artifact directory
└── evidence output channel
```

---

## Relationship to Skills and Plugins

Skills, plugins, and connectors remain the execution substrate.

They should be renamed/repositioned under Coder where appropriate.

Recommended target direction:

```text
internal/coder/
plugins/coder-core/
├── skills/
│   ├── repo-inspect/
│   ├── file-mutate/
│   ├── patch-apply/
│   ├── run-validation/
│   ├── git-lifecycle/
│   ├── remote-review/
│   └── task-normalize/
├── workflows/
└── handlers/
```

The current `plugins/navi-programmer` package should be treated as a migration source.

Short-term compatibility may keep aliases or compatibility wrappers, but new product docs, UI labels, APIs, and task names should use Coder.

---

## Self-Update Architecture

NAVI self-update is a Coder task targeting the NAVI repository.

It is not a separate self-coding product and not a special privileged subsystem.

Required self-update flow:

```text
trusted navid
  -> creates CoderTask targeting NAVI repo
  -> creates isolated CoderRun
  -> schedules coder-worker
      -> prepares candidate repo/worktree
      -> applies changes
      -> runs validation
      -> builds candidate NAVI
      -> launches candidate-navi container
      -> runs smoke checks
      -> returns promotion evidence
  -> navid records result
  -> owner/governance decides promotion
```

Rules:

- no normal live in-place self-mutation
- candidate runtime uses isolated config/state/ports
- promotion is distinct from candidate success
- NAVI Core remains trusted control path
- failed candidates are discarded or archived as evidence

---

## Deployment Boundary

Do not split Coder into a separate authority too early.

Preferred V1/V2 deployment:

```text
navid          # platform control plane and Coder domain authority
console        # includes top-level Coder UI
coder-worker   # optional/on-demand isolated worker
candidate-navi # temporary self-update validation runtime
```

`coder-worker` may be launched on demand by `navid` or run as a managed worker service. The key boundary is isolation, not product fragmentation.

---

## Migration from navi.programmer

Current reality:

- `plugins/navi-programmer` contains useful prototype assets
- its plugin identity is now stale as product architecture
- its skills and workflows should be retained, renamed, and reorganized
- runtime bridge work should be moved toward the Coder domain/control plane

Migration principle:

> Preserve useful implementation. Replace the product identity.

Migration phases:

1. update docs and task language to NAVI Coder
2. add product/domain architecture docs
3. introduce Coder backend domain objects and APIs
4. move/alias current `navi.programmer` skills into Coder core capability pack
5. add top-level console route
6. move workflow state from scratchpad/evidence-only model into persistent Coder records
7. retire or archive stale Programmer docs and names

---

## Readiness Criteria

NAVI Coder is not product-ready until:

- the console exposes Coder as a top-level surface
- Coder has backend domain state, not only plugin evidence
- Coder can create and track tasks/runs
- Coder can execute repository work through isolated workers
- validation results are persisted and visible
- branch/commit/review handoff is visible
- self-update runs use candidate runtime containers
- stale Programmer product language is removed or marked transitional

---

## Design Guardrails

Do not:

- present Coder as a plugin to users
- make ordinary chat the only Coder interface
- let workers become independent authorities
- hardwire self-coding as a special system separate from Coder
- auto-promote self-update candidates into trusted runtime
- scatter Coder domain state across generic scratchpads only

Do:

- keep Coder under NAVI identity
- use isolated workers for risky execution
- keep NAVI Core authoritative
- preserve governance and audit
- route product UX through a dedicated Coder surface
- treat current `navi.programmer` implementation as reusable migration substrate
