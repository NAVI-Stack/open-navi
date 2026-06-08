# NAVI Programmer

NAVI Programmer is NAVI's first-class programming capability plugin.

It packages the skills, workflow contracts, policy expectations, and evaluation
surfaces needed for NAVI to perform bounded programming work. The current
optimization target is the NAVI repository and this plugin package itself;
broader repository support should follow the same governed model later.

## V1 Goal

NAVI Programmer V1 succeeds when NAVI can:

1. accept a bounded programming task
2. bind it to a project and workspace
3. inspect the target repository
4. make scoped code or documentation changes
5. run validation
6. summarize the result and validation evidence
7. create reviewable branch and commit output when permitted
8. keep self-update work isolated from the trusted running instance

## Package Layout

| Path | Purpose |
| --- | --- |
| [plugin.yaml](plugin.yaml) | Plugin discovery metadata consumed by NAVI plugin tooling. |
| [AGENTS.md](AGENTS.md) | Instructions for AI coding assistants working on the plugin. |
| [PROJECT_STATE.md](PROJECT_STATE.md) | Current project state, risks, and next direction. |
| [TASK_QUEUE.md](TASK_QUEUE.md) | Prioritized task queue. |
| [NEXT_ACTION.md](NEXT_ACTION.md) | Immediate task pointer. |
| [docs/](docs/README.md) | Plugin docs corpus: concepts, design, specs, plans, runbooks, tasks, and decisions. |
| [skills/](skills/README.md) | Seven atomic prototype-executable programming skill contracts. |
| [workflows/](workflows/) | Workflow contracts plus the local bounded mutation evidence runner. |
| [tests/](tests/) | Fixtures, evals, and future plugin-level verification assets. |

## Boundary

NAVI core owns plugin loading, governance, runtime orchestration, shared skill
execution, connectors, and world-model authority.

NAVI Programmer owns the programming capability package: programming skill
families, task workflows, validation expectations, reporting shape, and
self-update safety rules.

## Current Next Step

The Programmer V1 baseline is closed. [NEXT_ACTION.md](NEXT_ACTION.md) now
points future work at post-V1 hardening: broader orchestration proof, richer
autonomous multi-step coverage, and follow-up backlog selection outside the
closed OMN-234 scaffold.
