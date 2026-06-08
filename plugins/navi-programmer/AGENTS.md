# AGENTS.md - NAVI Programmer

**Status:** Active
**Last Updated:** 2026-05-20

This file is the first stop for AI coding assistants working on NAVI Programmer.

## Project Identity

NAVI Programmer is NAVI's first-class programming capability plugin. It is a
plugin package, not NAVI core and not an external product layer.

Its job is to package programming capability through governed skills,
workflows, validation behavior, reporting, and self-update safety rules.

## Boundary Rules

- Keep NAVI Programmer source in this package.
- Do not move plugin-owned design, specs, plans, skills, workflows, or evals
  back into `projects/navi/docs/`.
- NAVI core owns plugin loading, governance, runtime orchestration, connector
  framework, world-model authority, and shared skill execution.
- NAVI Programmer owns the programming capability contract and its plugin-local
  skills, workflows, tests, docs, and release metadata.
- Do not create a second runtime or a second governance path inside the plugin.
- Do not turn NAVI Programmer into one giant "do programming" tool. Skills stay
  atomic and workflows compose them.

## Required Reading

Before making substantive changes, read:

1. [PROJECT_STATE.md](PROJECT_STATE.md)
2. [NEXT_ACTION.md](NEXT_ACTION.md)
3. [TASK_QUEUE.md](TASK_QUEUE.md)
4. [docs/README.md](docs/README.md)
5. [docs/specs/navi-programmer-plugin-v1.md](docs/specs/navi-programmer-plugin-v1.md)
6. [docs/specs/navi-programmer-task-execution.md](docs/specs/navi-programmer-task-execution.md)

For self-update behavior, also read:

- [docs/design/navi-programmer-self-update-safety.md](docs/design/navi-programmer-self-update-safety.md)

## Development Posture

Prefer this order:

1. Keep plugin, skill, workflow, and doc identities reconciled.
2. Preserve executable, narrow skill contracts instead of adding a monolithic
   programming tool.
3. Improve the bounded mutation workflow and runner evidence/result contracts.
4. Add repeatable fixture/eval tests around each new behavior.
5. Keep self-update candidate safety explicit and NAVI-first.

## Done Means

A task is not done when files exist. It is done when:

- the intended plugin contract is clear
- the relevant skill or workflow is discoverable
- behavior is validated or explicitly marked as not yet executable
- docs and task state are updated
- failure modes are visible

## Go Cache Policy

NAVI Programmer validation must use the default Go cache locations from
`go env GOCACHE` and `go env GOMODCACHE`. Do not create plugin-local or
repo-local Go caches such as `.gocache`, `.gomodcache`, `.codex-gocache`,
`.codex-gomodcache`, `.codex-go-cache`, or `.codex-go-modcache`.

## Cross-Project Dependencies

NAVI Programmer may reference NAVI core docs for:

- Project and Workspace specs
- plugin lifecycle
- skill format
- governance and proposal flows
- runtime/orchestration contracts

Those references should remain links to NAVI core, not copied forks.
