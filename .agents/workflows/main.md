# Agent Workflow Policy

**Authority:** This document is a binding operational policy for all autonomous agents operating in this repository.
**Changes:** Require explicit human authorization. Append-only after initial authoring.
**Version:** 1.0
**Effective:** 2026-02-28

---

## Overview

This document defines the complete lifecycle an agent must follow in every session. No step may be skipped. An agent that deviates from this protocol is operating outside its authority.

The goal is a self-correcting, auditable, continuous development loop that any agent can enter at any time — given only the instruction "continue working on the repo."

---

## Session Lifecycle (Required — Every Session, Every Agent)

### Phase 0: Verification Gate (HALT if it fails)

```bash
bash docs/agents/scripts/verify-docs.sh
```

- If exit code is non-zero: write a `BROKEN` handoff file and **HALT immediately**. Do not proceed.
- This is not optional. It is the entry gate.

### Phase 1: Orientation

Read these files in order. Do not skip any.

1. `PROJECT_STATE.md` — Current phase, what is done, what is next.
2. `docs/agents/state.md` — Detailed project context, active work areas, known blockers.
3. `NEXT_ACTION.md` — Your assigned task. This is what you will work on.
4. Latest handoff file in `docs/agents/handoff/` — Context from the previous agent session.

If `NEXT_ACTION.md` says "Awaiting new directive," stop and report to the human developer.

### Phase 2: Contract Review

1. Read `docs/agents/design.md` to identify the relevant ADRs and phase spec.
2. In `TASK_QUEUE.md`, find the task ID matching your `NEXT_ACTION.md`.
3. If a task spec file exists at `docs/agents/tasks/NAVI-XXX-YYY.md`, read it — especially the **"What you must NOT change (Frozen Design Contract)"** section.

### Phase 3: Execution

1. Implement the task.
2. Create a new log file: `docs/agents/logs/YYYYMMDDTHHMMSS-{git-short-sha}.md`
3. Record every command run, every error encountered, and every decision made in the log file.
4. Follow the TDD pattern from `AGENTS.md`: write failing tests → implement → pass tests.

### Phase 4: Validation (Required — do not skip)

```bash
gofmt -w ./...
go vet ./...
make test
```

- If `make test` fails: revert code changes via `git checkout -- <changed-files>`. Do not commit broken state.
- Write a `BLOCKED` handoff explaining the failure. Update `NEXT_ACTION.md` with the blocker.

### Phase 5: State Reconciliation

1. Move the completed task in `TASK_QUEUE.md` from `PENDING` → `DONE`.
2. Identify the next `PENDING` task (priority order: CRITICAL → HIGH → MEDIUM → LOW).
3. Overwrite `NEXT_ACTION.md` with the new task (see format below).
4. Append findings to `docs/agents/state.md` — update the `Last Verified By` timestamp.
5. Update `PROJECT_STATE.md` — update "What Is In Progress" and "What Is Next" sections.

### Phase 6: Handoff

Create `docs/agents/handoff/handoff-YYYYMMDDTHHMMSS-{git-short-sha}.md` using the template at `docs/agents/handoff/TEMPLATE.md`. Fill in every section.

---

## File Modification Rules

### Immutable — Never Modified by Agents

| File / Directory | Reason |
|---|---|
| `internal/schema/*.go` | Go types are single source of truth; Python models generated from them |
| `docs/adr/*.md` | Architecture decisions are permanent record; only new files may be added |
| `docs/DECISIONS.md` | Design decisions are append-only; never edit existing entries |
| `docs/architecture/*.md` | System architecture reference; human-authored |
| `docs/phases/*.md` | Phase specs are immutable after authoring |
| `docs/standards/*.md` | Coding standards; human-authorized changes only |
| `AGENTS.md` | Agent directive file; tooling convention |
| `AGENT_WORKFLOW.md` | This document |
| `OBJECTIVE.md` | Product principles; immutable |
| `VISION.md` | Strategic roadmap; human-authorized changes only |
| `SECURITY.md` | Threat model; human-authorized changes only |
| `CONTRIBUTING.md` | Contribution guidelines; human-authorized changes only |
| `docs/agents/handoff/TEMPLATE.md` | Frozen structural template |
| `docs/agents/tasks/TEMPLATE.md` | Frozen structural template |
| `docs/agents/logs/TEMPLATE.md` | Frozen structural template |

### Append-Only — Agents May Add, Never Delete or Reorder

| File | Rule |
|---|---|
| `docs/agents/state.md` | Prepend new `Last Verified By` timestamp; append to Completed Milestones and Blockers |
| `CHANGELOG.md` | Prepend new release entries only |
| `docs/DECISIONS.md` | Append new decisions (D-NNN) only |

### Freely Mutable — Agents Rewrite as Needed

| File | Rule |
|---|---|
| `NEXT_ACTION.md` | Completely rewritten after each task completion |
| `TASK_QUEUE.md` | Task status updated in place (PENDING → IN_PROGRESS → DONE) |
| `PROJECT_STATE.md` | Sections updated in place to reflect current state |
| `docs/development-tracker.md` | Session entries appended |

### Agent-Created (New Files Only, Existing Never Modified)

| Location | Pattern | Rule |
|---|---|---|
| `docs/agents/handoff/` | `handoff-YYYYMMDDTHHMMSS-{sha}.md` | One new file per session |
| `docs/agents/logs/` | `YYYYMMDDTHHMMSS-{sha}.md` | One new file per session |
| `docs/agents/tasks/` | `NAVI-XXX-YYY.md` | Human-authored task specs; agents read, never write |
| `docs/logs/` | Free-form | Execution traces, terminal output summaries |

---

## NEXT_ACTION.md Format

`NEXT_ACTION.md` must always contain exactly one of the following:

### Active Task

```markdown
# Next Action

**Task ID:** NAVI-013-001
**Title:** [Imperative task title]
**Agent:** [coder | critic | strategist | scout | auditor | migrator | operator]
**Priority:** [CRITICAL | HIGH | MEDIUM | LOW]
**Surface:** [primary file or directory to modify]

## Description

[One paragraph describing the task precisely. Include the specific problem to solve, not just what to do.]

## Acceptance Criteria

- [ ] [Specific, testable outcome]
- [ ] [go test ./... passes]
- [ ] [go vet ./... clean]
```

### Awaiting New Directive (No Pending Tasks)

```markdown
# Next Action

**Status:** AWAITING_DIRECTIVE

All tasks in TASK_QUEUE.md are DONE or BLOCKED.

**Last completed:** NAVI-013-011 (Update CHANGELOG.md)
**Blocked tasks:** [list IDs if any]

A human developer must provide a new directive or unblock the listed tasks.
```

---

## NEXT_ACTION.md Update Protocol

```
Agent reads NEXT_ACTION.md
        │
        ▼
Agent executes the described task
        │
        ▼
make test passes?
   Yes ──────────────────────────────────────────────────────────▶ Continue
   No  ──▶ revert changes, write BLOCKED handoff, set NEXT_ACTION
           to describe the blocker, HALT
        │
        ▼
Mark task DONE in TASK_QUEUE.md
        │
        ▼
Find next PENDING task (CRITICAL > HIGH > MEDIUM > LOW)
        │
   Found? ──────────────────────────────────────────────────────▶ Overwrite NEXT_ACTION.md
   None   ──▶ Write AWAITING_DIRECTIVE to NEXT_ACTION.md
        │
        ▼
Append to docs/agents/state.md
Update PROJECT_STATE.md
Create handoff file
```

---

## Circuit Breakers — When Agents Must HALT

Agents must halt and write a handoff rather than guess or approximate:

| Condition | Action |
|---|---|
| `verify-docs.sh` exits non-zero | Write `BROKEN` handoff; HALT |
| `go test ./...` fails **before** your changes | Write `BROKEN` handoff; HALT |
| `go test ./...` fails **after** your changes | Revert; write `BLOCKED` handoff; HALT |
| Required design decision not in `docs/agents/design.md` | Write `BLOCKED` handoff; HALT |
| Task spec says not to modify a file you need to modify | Write `BLOCKED` handoff; HALT |
| Handoff older than 72 hours detected | Write `BROKEN` handoff; HALT |
| NEXT_ACTION.md says `AWAITING_DIRECTIVE` | Report to human; HALT |

---

## PR Protocol

When a phase or major task group is complete, an agent should open a PR:

1. Ensure `make test` passes and working tree is clean.
2. Write a concise branch name: `phase-13-coder-implement-mode`.
3. PR title format: `phase: <short description>` (e.g., `phase: coder + Orchestrator implement mode`).
4. PR body must include:
   - Summary of changes (bullet points)
   - Tasks completed from TASK_QUEUE.md (by ID)
   - Test verification result
   - Updated `PROJECT_STATE.md` section
   - Link to handoff file for this session

---

## Documentation Hierarchy

```
AGENTS.md                    ← Entry point for all agents (read first, at root)
AGENT_WORKFLOW.md            ← This document (operational policy)
PROJECT_STATE.md             ← Current state summary (fast orientation)
NEXT_ACTION.md               ← Immediate task assignment (volatile)
TASK_QUEUE.md                ← Full task backlog (authoritative)
docs/agents/index.md         ← Agent SOP (step-by-step procedure)
docs/agents/state.md         ← Detailed project state with session history
docs/agents/design.md        ← Architecture decision route map
docs/agents/handoff/         ← Per-session handoff files (audit trail)
docs/agents/logs/            ← Per-session execution logs
docs/agents/tasks/           ← Task specification files
docs/phases/                 ← Phase specifications (immutable reference)
docs/adr/                    ← Architecture Decision Records (immutable)
docs/DECISIONS.md            ← Design decision log (append-only)
```

Agents read from top to bottom. Write only to files whose modification rules permit it.
