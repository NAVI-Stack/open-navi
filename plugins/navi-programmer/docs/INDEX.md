# NAVI Programmer Docs

This directory is the source of truth for NAVI Programmer plugin design and
implementation planning.

## Corpus

| Document | Purpose |
| --- | --- |
| [README.md](README.md) | Docs entry point and orientation. |
| [DOCUMENTATION-CORPUS.md](DOCUMENTATION-CORPUS.md) | Docs structure, trust levels, and lifecycle rules. |

## Concept

| Document | Purpose |
| --- | --- |
| [concepts/navi-programmer.md](concepts/navi-programmer.md) | Product and architecture concept for NAVI Programmer as a first-class plugin. |

## Design

| Document | Purpose |
| --- | --- |
| [design/navi-programmer-plugin-architecture.md](design/navi-programmer-plugin-architecture.md) | Plugin architecture, layers, boundaries, and shared runtime dependencies. |
| [design/navi-programmer-self-update-safety.md](design/navi-programmer-self-update-safety.md) | Safety model for NAVI Programmer working on NAVI itself. |

## Specs

| Document | Purpose |
| --- | --- |
| [specs/INDEX.md](specs/INDEX.md) | Specs index. |
| [specs/navi-programmer-plugin-v1.md](specs/navi-programmer-plugin-v1.md) | V1 plugin shape, required capability families, and acceptance criteria. |
| [specs/navi-programmer-task-execution.md](specs/navi-programmer-task-execution.md) | Task execution lifecycle from intake through review-ready output. |
| [specs/navi-programmer-bounded-mutation-workflow.md](specs/navi-programmer-bounded-mutation-workflow.md) | Workflow contract for scoped mutation, validation evidence, and review-ready outcomes. |
| [specs/navi-programmer-bounded-mutation-runner.md](specs/navi-programmer-bounded-mutation-runner.md) | Local runner surface for bounded mutation transition and evidence gate checks. |

## Plans

| Document | Purpose |
| --- | --- |
| [plans/navi-programmer-implementation.plan.md](plans/navi-programmer-implementation.plan.md) | Phased implementation plan for NAVI Programmer V1. |
| [plans/project-centered-programming-mvp.plan.md](plans/project-centered-programming-mvp.plan.md) | MVP slice for project-centered coding execution and sandboxed validation. |
| [plans/navi-programmer-evaluation.plan.md](plans/navi-programmer-evaluation.plan.md) | Evaluation plan for proving NAVI Programmer is real. |

## Runbooks

| Document | Purpose |
| --- | --- |
| [runbooks/INDEX.md](runbooks/INDEX.md) | Runbook index. |
| [runbooks/local-plugin-development.md](runbooks/local-plugin-development.md) | How to use this source package with NAVI core before NAVI Store exists. |

## Tasks

| Document | Purpose |
| --- | --- |
| [tasks/INDEX.md](tasks/INDEX.md) | Task, blocker, and readiness index. |
| [tasks/readiness.md](tasks/readiness.md) | Current readiness snapshot. |
| [tasks/blockers.md](tasks/blockers.md) | Known blockers and risks. |

## Decisions

| Document | Purpose |
| --- | --- |
| [decisions/INDEX.md](decisions/INDEX.md) | Decision log index. |
