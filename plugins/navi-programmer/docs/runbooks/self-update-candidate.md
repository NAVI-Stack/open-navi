# Self-Update Candidate Runbook

**Status:** Active
**Last Updated:** 2026-04-30
**Related Task:** NP-006
**Related Workflow:** `workflows/self-update-candidate.yaml`
**Related Design:** `docs/design/navi-programmer-self-update-safety.md`

---

## Purpose

This runbook defines how to safely run a NAVI Programmer self-update task — that is, a programming task whose target repository is NAVI or NAVI Programmer itself.

It answers three questions in operational terms:

1. How is candidate work kept separate from the trusted running instance?
2. How is promotion readiness determined independently of task success?
3. How does the trusted control path survive a candidate failure?

For the underlying safety model and threat analysis, see
[`docs/design/navi-programmer-self-update-safety.md`](../design/navi-programmer-self-update-safety.md).

---

## When to Use This Runbook

Use this runbook when the programming task target is:

- the NAVI core repository (`projects/navi`)
- NAVI Programmer itself (`projects/navi/plugins/navi-programmer`)
- any shared NAVI Ecosystem package that the running NAVI instance depends on

Do **not** use this runbook for programming tasks targeting external or
unrelated repositories. Those run through the standard bounded mutation
workflow.

---

## Key Terms

| Term | Meaning |
| --- | --- |
| **Live instance** | The currently running trusted NAVI that will coordinate this workflow |
| **Candidate workspace** | An isolated git worktree or clone where all changes are applied |
| **Candidate build** | Compiled or assembled output produced from the candidate workspace |
| **Candidate runtime** | A separate NAVI process launched from the candidate build, in isolation |
| **Promotion** | The explicit act of treating a candidate result as ready for trusted use |
| **Control path** | The live instance's ability to observe, halt, and recover from the candidate |

---

## Prerequisites

Before starting a self-update task, confirm:

- [ ] A live NAVI instance is running and available
- [ ] The target NAVI repo root is known and accessible
- [ ] Git is available in the execution environment
- [ ] The task has been normalized with `task-normalize` and classified as
      `self_update` scope
- [ ] The task scope is unambiguous — blocked tasks must not proceed to
      candidate preparation

---

## Procedure

### Phase 1 — Normalize and Bind Scope

**Skill:** `navi-programmer.task-normalize`

1. Run `normalize_task` with the raw self-update request.
2. Confirm `task_class` is `self_update_candidate`, `mutation_intent` is
   `self_update`, and `self_update` is `true` in the output.
3. Run `bind_scope` with the NAVI repo root as the target.
4. Confirm `binding_status` or `scope_status` is `bound` before proceeding.
5. If the normalizer returns `blocked`, surface the ambiguity reason and
   **stop**. Do not enter candidate preparation with an unresolved scope.

**Evidence required to advance:** `normalized_task`, `workspace_binding`

---

### Phase 2 — Inspect the Target Repository

**Skill:** `navi-programmer.repo-inspect`

1. Run `repo_status` to confirm the repo is clean or document any uncommitted
   state.
2. Run `list_tree` scoped to the directories the task will touch.
3. Run `read_file` and `search_text` as needed to understand the area of
   change.

**Evidence required to advance:** `repo_status`, `inspected_files`,
`relevant_patterns`

---

### Phase 3 — Prepare Candidate Workspace

This phase is self-update-specific. It does not exist in standard bounded
mutation.

1. Create an isolated candidate workspace using one of:
   - **git worktree** (preferred): `git worktree add <candidate-path> -b <candidate-branch>`
   - Isolated clone: `git clone <repo-root> <candidate-path>`
2. Record the candidate workspace root and branch name as `candidate_context`
   evidence.
3. Confirm the candidate workspace is distinct from the live runtime's working
   tree. If they share a path, **stop** — this is an invariant violation.

**Evidence required to advance:** `candidate_context` including workspace
path, branch name, and isolation confirmation

---

### Phase 4 — Apply Changes in Candidate Workspace

**Skills:** `navi-programmer.file-mutate`, `navi-programmer.patch-apply`

All mutation operations must target the **candidate workspace root**, not the
live repo root.

1. Apply each planned change using `file-mutate` or `patch-apply` with
   `root` set to the candidate workspace path.
2. Collect `changed_files` and `diff_summary` evidence for each operation.
3. If any mutation fails or is out of scope, record the failure and **stop**.
   Do not attempt to continue with partial changes to the candidate.

**Evidence required to advance:** `changed_files`, `diff_summary`

---

### Phase 5 — Validate Candidate Changes

**Skill:** `navi-programmer.run-validation`

Run validation against the candidate workspace. Validation must occur before
any promotion step.

1. Run build: `run_command` with the repo build command in the candidate
   workspace.
2. Run tests: `run_command` with the test command scoped to the changed
   packages.
3. Run lint or static analysis if applicable.
4. For each command, record the verdict (`passed`, `failed`, `timed_out`,
   `not_run`).
5. If validation cannot run for a legitimate reason (no build target, docs-only
   change), record `record_not_run` with an explicit reason. A missing reason
   is not acceptable.

**Invariant:** At least one `run_command` result or one `record_not_run` with
reason must exist before advancing. Validation evidence cannot be empty.

**Evidence required to advance:** `validation_results` with at least one entry

---

### Phase 6 — Run Candidate Runtime Smoke (if applicable)

Skip this phase only for documentation-only changes where no runtime behavior
is affected.

1. Build the candidate artifact from the candidate workspace.
2. Launch a candidate NAVI instance with:
   - Separate process
   - Separate port (not the live gateway port)
   - Separate config and state directory
   - Connectors disabled or stubbed
3. Run the minimum smoke suite:
   - Boot — instance starts without crash
   - Health — readiness check returns success
   - Basic request path — a minimal prompt completes
   - Skill surface bootstrap — capability surface loads without fatal failure
   - Governance path — a basic governed action path responds
   - Clean shutdown — instance exits cleanly
4. Record a structured `smoke_results` entry for each check.
5. Terminate the candidate runtime when smoke is complete.

**Evidence required to advance:** `smoke_results` with pass/fail for each
check above

---

### Phase 7 — Synthesize Promotion Readiness

This phase is self-update-specific.

Produce a `promotion_readiness_summary` that answers:

| Question | Required answer |
| --- | --- |
| Did all required validation layers run? | Yes, or each skipped layer has an explicit recorded reason |
| Did any validation layer fail? | If yes, promotion readiness is `not_ready` |
| Did the candidate runtime boot and pass smoke? | Yes, or the change is confirmed docs-only |
| Are there unresolved risks or known regressions? | List them explicitly |
| Is promotion recommended? | `recommended`, `not_recommended`, or `needs_review` |

**Task success is not promotion readiness.** A task that produced a correct
diff and passed all checks is `promotion_recommended`, not `promoted`. These
are different states.

---

### Phase 8 — Create Review-Ready Output

**Skill:** `navi-programmer.git-lifecycle`

1. Run `create_branch` in the candidate workspace to confirm the candidate
   branch is named and recorded.
2. Run `create_commit` with the explicit list of changed files. Do not use
   broad staging.
3. Run `prepare_review` to produce the handoff summary.

The candidate workspace branch and commit are the handoff artifact. The live
instance's working tree is unchanged.

**Evidence required:** `lifecycle_summary` with branch, commit, changed files,
and review handoff summary

---

### Phase 9 — Confirm Before Remote Actions

No remote action (push, PR creation, trusted runtime replacement) should
happen automatically.

After review-ready output exists:

1. Present the `promotion_readiness_summary` and `lifecycle_summary` to the
   operator.
2. Wait for explicit confirmation before:
   - Pushing the candidate branch to remote
   - Opening a pull request
   - Any deployment or runtime replacement action
3. If the operator rejects or defers, proceed to Failure / Cleanup below.

---

## Failure and Cleanup

If any phase fails, or if promotion is rejected:

1. **Preserve all evidence** — do not discard `candidate_context`,
   `validation_results`, `smoke_results`, or `diff_summary`.
2. **Terminate the candidate runtime** if it is still running.
3. **Discard or archive the candidate workspace.** The live repo is unchanged.
4. Record the failure class (see below) in the final outcome.
5. Surface the structured failure summary to the operator.

The live NAVI instance must remain operational after any of these steps. If
the live instance is affected by a candidate failure, that is an invariant
violation that must be escalated.

### Failure Classes

| Class | Meaning |
| --- | --- |
| `scope_resolution_failure` | Task was ambiguous or scope binding failed |
| `candidate_preparation_failure` | Workspace could not be created or isolated |
| `mutation_failure` | File changes failed or landed outside allowed scope |
| `build_validation_failure` | Build or compile step failed |
| `test_validation_failure` | Test suite failed |
| `candidate_startup_failure` | Candidate runtime failed to boot |
| `candidate_smoke_failure` | Candidate runtime failed smoke checks |
| `promotion_rejected` | Operator rejected promotion after review |
| `cleanup_failure` | Candidate artifacts could not be cleaned up cleanly |

---

## Invariants

These must hold for every self-update run. A violation is not a degraded path
— it is a stop condition.

| # | Invariant |
| --- | --- |
| 1 | Candidate workspace is distinct from the live runtime working tree |
| 2 | All mutations target the candidate workspace, never the live repo root |
| 3 | Candidate runtime does not share live ports, state, or connectors |
| 4 | Validation evidence is mandatory before any promotion step |
| 5 | Task success and promotion readiness are recorded as separate states |
| 6 | Failure keeps the trusted live control path intact and operational |
| 7 | Promotion requires explicit confirmation — it is never implicit |
| 8 | Candidate failure evidence is preserved, never silently discarded |

---

## Promotion Decision Reference

| Promotion readiness state | Meaning | Next action |
| --- | --- | --- |
| `recommended` | All layers passed, no unresolved risks | Present to operator for confirmation |
| `needs_review` | Passed with minor unresolved items or caveats | Present summary, require operator review before confirming |
| `not_recommended` | One or more layers failed or unresolved risks remain | Do not promote; document and discard candidate |

---

## Required Final Evidence Set

A complete self-update run must produce all of the following:

- `normalized_task` — structured task with self-update classification
- `workspace_binding` — confirmed repo root and allowed scope
- `candidate_context` — workspace path, branch, isolation confirmation
- `repo_status` — pre-mutation repo state
- `inspected_files` — files examined before planning
- `changed_files` — all files mutated
- `diff_summary` — patch/diff for each changed file
- `validation_results` — at least one entry (run or recorded not_run)
- `smoke_results` — boot/health/request/skill/governance/shutdown outcomes (or explicit docs-only skip)
- `promotion_readiness_summary` — structured promotion recommendation
- `lifecycle_summary` — branch, commit, and review handoff output
