# NAVI Programmer Documentation

**Status:** Active
**Last Updated:** 2026-05-26

This docs corpus is the source of truth for the NAVI Programmer plugin.

NAVI core documentation may reference this package, but plugin-owned concepts,
designs, specs, plans, runbooks, task state, and evaluations should live here.

## Start Here

| Document | Purpose |
| --- | --- |
| [../README.md](../README.md) | Plugin overview and package layout. |
| [../PROJECT_STATE.md](../PROJECT_STATE.md) | Current implementation state and risks. |
| [../NEXT_ACTION.md](../NEXT_ACTION.md) | Immediate task. |
| [../TASK_QUEUE.md](../TASK_QUEUE.md) | Prioritized project backlog. |
| [INDEX.md](INDEX.md) | Full documentation index. |
| [DOCUMENTATION-CORPUS.md](DOCUMENTATION-CORPUS.md) | Docs structure, trust levels, and lifecycle rules. |

## Core Corpus

| Area | Go To |
| --- | --- |
| Concept | [concepts/navi-programmer.md](concepts/navi-programmer.md) |
| Plugin architecture | [design/navi-programmer-plugin-architecture.md](design/navi-programmer-plugin-architecture.md) |
| Self-update safety | [design/navi-programmer-self-update-safety.md](design/navi-programmer-self-update-safety.md) |
| Implementation plan | [plans/navi-programmer-implementation.plan.md](plans/navi-programmer-implementation.plan.md) |
| Evaluation plan | [plans/navi-programmer-evaluation.plan.md](plans/navi-programmer-evaluation.plan.md) |

## Implementation-Facing Docs

| Area | Go To |
| --- | --- |
| Plugin contract | [specs/navi-programmer-plugin-v1.md](specs/navi-programmer-plugin-v1.md) |
| Task execution lifecycle | [specs/navi-programmer-task-execution.md](specs/navi-programmer-task-execution.md) |
| Bounded mutation workflow | [specs/navi-programmer-bounded-mutation-workflow.md](specs/navi-programmer-bounded-mutation-workflow.md) |
| Bounded mutation runner | [specs/navi-programmer-bounded-mutation-runner.md](specs/navi-programmer-bounded-mutation-runner.md) |
| Project-centered MVP | [plans/project-centered-programming-mvp.plan.md](plans/project-centered-programming-mvp.plan.md) |
| Local development | [runbooks/local-plugin-development.md](runbooks/local-plugin-development.md) |
| Readiness and blockers | [tasks/INDEX.md](tasks/INDEX.md) |

## Corpus Rule

If a document explains NAVI Programmer itself, put it here. If a document
explains how NAVI core loads, governs, or invokes all plugins, keep that in NAVI
core and link to it.
