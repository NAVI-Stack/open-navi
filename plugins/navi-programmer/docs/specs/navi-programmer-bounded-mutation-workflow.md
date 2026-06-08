# NAVI Programmer Bounded Mutation Workflow

**Status:** Contract
**Last Updated:** 2026-05-08

## Purpose

This spec defines the bounded mutation workflow for NAVI Programmer. It turns a
coding request into scoped, inspectable, validated, reviewable work.

The machine-readable contract lives at
`workflows/bounded-mutation.yaml`. This document explains the state semantics,
evidence rules, validation gate, and current implementation boundary.

## Scope

This workflow applies to:

- bounded code or documentation mutation tasks
- ticket-driven mutation tasks
- self-update candidate tasks that target NAVI or NAVI Programmer

It does not apply to pure read-only comprehension tasks.

## Current Implementation Boundary

Six skills are executable now:

| Skill | Interfaces | Role |
| --- | --- | --- |
| `navi-programmer.repo-inspect` | `list_tree`, `read_file`, `search_text`, `repo_status`, `inspect_diff` | Gather repository context before planning and mutation. |
| `navi-programmer.run-validation` | `run_command`, `record_not_run` | Capture validation evidence or an explicit reason validation was not run. |
| `navi-programmer.file-mutate` | `create_file`, `write_file` | Create or replace scoped text files with preconditions. |
| `navi-programmer.patch-apply` | `preview_patch`, `apply_patch` | Preview and apply structured text patch operations. |
| `navi-programmer.git-lifecycle` | `inspect_status`, `create_branch`, `create_commit`, `prepare_review` | Produce local branch, commit, and review handoff evidence. |
| `navi-programmer.remote-review` | `push_branch`, `create_pull_request` | Perform confirmation-gated remote push and PR creation with validation context in the review surface. |
| `navi-programmer.task-normalize` | `normalize_task`, `bind_scope` | Preserve raw requests, classify task intent, and bind safe repo scope. |

No V1 skill family in this workflow remains placeholder-only. The remaining gap
is orchestration: NAVI core still needs to invoke the skills in the right order,
handle retries, and enforce user confirmation for remote actions.

Until orchestration work exists, this workflow is a contract and local harness
for runtime integration, not a fully executable end-to-end coding path.

## State Map

| Workflow State | Task Execution State | Required Evidence |
| --- | --- | --- |
| `received` | `received` | Raw task input and task source. |
| `normalized` | `normalized` | Task summary, mutation intent, acceptance target, validation requirement. |
| `scope_bound` | `scope_bound` | Repo root, allowed scope, self-update classification. |
| `inspecting` | `inspecting` | Repo status, inspected files, relevant patterns. |
| `planned` | `planned` | Execution plan, intended touch set, validation plan, recovery notes. |
| `executing` | `executing` | Mutation attempts, changed files, diff summary. |
| `validating` | `validating` | Validation results from `run-validation`. |
| `synthesizing` | `synthesizing` | Outcome classification, result summary, residual risks. |
| `review_ready` | `review_ready` | Final diff, validation summary, lifecycle summary, handoff summary. |

The workflow also supports terminal outcomes that map to `completed`,
`partially_succeeded`, `blocked`, and `failed`.

## Validation Gate

Mutation and validation are linked by a hard gate.

A workflow run that has `changed_files` MUST NOT move past `validating` without
one of these evidence items:

- at least one `run-validation.run_command` result
- one explicit `run-validation.record_not_run` result with a reason

This rule exists even for documentation-only mutation. If no meaningful command
exists, the workflow records `not_run`; it does not omit validation evidence.

## Verdict Semantics

| Verdict | Workflow Meaning |
| --- | --- |
| `passed` | The workflow may become `completed` if other evidence is complete. |
| `failed` | The workflow must become `partially_succeeded` or `failed`; it cannot be clean `completed`. |
| `timed_out` | The workflow must become `partially_succeeded`, `blocked`, or `failed` depending on remaining evidence. |
| `not_run` | The workflow may continue only when the reason is explicit and acceptable for the task. |
| `ambiguous` | Reserved for future structured classifiers; treat as not cleanly passing. |

## Outcome Rules

### Completed

Use `completed` only when:

- review-ready evidence exists
- validation passed, or `not_run` has an explicit accepted reason
- no unresolved blocker remains
- the acceptance target is met

### Partially Succeeded

Use `partially_succeeded` when useful work exists but the target is not cleanly
complete. Examples:

- files changed but validation failed
- validation timed out
- lifecycle output is incomplete
- the task target was only partly achieved

### Blocked

Use `blocked` when the workflow is paused for missing context, unavailable
dependencies, required confirmation, ambiguous scope, or unavailable candidate
context for self-update work.

Blocking is not failure. It is a governed pause with current evidence and a
needed resolution.

### Failed

Use `failed` when the workflow cannot produce useful reviewable progress or
violates a required gate such as missing validation evidence after mutation.

## Required Result Surface

Every bounded mutation run should produce a structured result containing:

- raw task input
- normalized task summary
- workspace binding
- inspected files
- execution plan
- changed files
- diff summary
- validation attempts and verdicts
- lifecycle output, if available
- final outcome
- residual risks or blockers

## Runtime Integration Notes

The workflow contract intentionally separates plugin-owned behavior from NAVI
core behavior.

NAVI Programmer owns:

- skill contracts
- workflow evidence requirements
- programming-specific outcome semantics
- validation discipline

NAVI core owns:

- plugin discovery and invocation
- shared project/workspace binding APIs
- shared sandbox enforcement
- durable task/run storage
- permission and confirmation policy

The first integration milestone is not to make mutation glamorous. It is to make
mutation auditable: inspect first, change within scope, validate or record why
validation did not run, and report the result honestly.
