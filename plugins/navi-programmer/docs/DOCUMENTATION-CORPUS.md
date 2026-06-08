# Documentation Corpus

**Status:** Active
**Last Updated:** 2026-04-28

## Purpose

This document defines the NAVI Programmer docs corpus: what belongs here, how
documents are trusted, and how agents should update the corpus as the plugin
evolves.

## Directory Roles

| Directory | Role | Trust Level |
| --- | --- | --- |
| `concepts/` | Product and conceptual framing. | Stable intent, may evolve. |
| `design/` | Architecture and safety design. | Evolving design truth. |
| `specs/` | Implementation-facing contracts. | High signal, mutable. |
| `plans/` | Build sequencing and evaluation planning. | Active planning. |
| `runbooks/` | How to operate, test, or develop the plugin. | Operational truth. |
| `tasks/` | Readiness, blockers, and task support docs. | Active execution. |
| `decisions/` | Lightweight decision records. | Reviewable decision history. |

## Source-of-Truth Rules

- Plugin-owned docs live in this package.
- NAVI core docs may contain pointers to plugin docs, not duplicate plugin docs.
- Shared runtime, plugin loader, skill format, governance, and project/workspace
  specs may remain in NAVI core.
- When plugin docs depend on NAVI core contracts, link to the core docs instead
  of copying their content.

## Agent Rules

When changing code, skills, workflows, or plugin behavior:

1. Update the nearest implementation-facing spec or runbook.
2. Update [../PROJECT_STATE.md](../PROJECT_STATE.md) if project status changes.
3. Update [../TASK_QUEUE.md](../TASK_QUEUE.md) when task state changes.
4. Update [../NEXT_ACTION.md](../NEXT_ACTION.md) when the immediate task changes.
5. Add decisions to `decisions/` when a boundary or architecture choice becomes
   durable.

## Promotion Rule

Draft notes can start in `docs/tasks/` or `docs/plans/`. Promote them to
`docs/specs/` or `docs/design/` only when they become durable enough to guide
implementation.
