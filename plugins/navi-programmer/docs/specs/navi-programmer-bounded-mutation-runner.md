# NAVI Programmer Bounded Mutation Runner

**Status:** Draft executable contract
**Last Updated:** 2026-05-08

## Purpose

The bounded mutation runner is the first local harness surface for NAVI
Programmer. It does not invoke skills itself yet. Instead, it consumes the
compiled workflow contract and the evidence produced by skills, records skill
results into a consistent evidence ledger, decides whether a state transition is
valid, blocked, failed, or eligible to continue, and can synthesize a normalized
programming result envelope.

This gives NAVI core a concrete contract to call while the larger programming
orchestration layer is still being built.

## Runtime Surface

The local runner lives at:

- `workflows/bounded_mutation_runner.py`

It consumes:

- `workflows/bounded-mutation.compiled.json`

The runner exposes two interfaces over stdin/stdout JSON:

| Interface | Purpose |
| --- | --- |
| `start_run` | Create the initial run envelope and raw-task evidence. |
| `record_step` | Merge one skill result envelope into accumulated workflow evidence. |
| `inspect_contract` | Return state, evidence, skill, governance, and validation gate requirements. |
| `evaluate_transition` | Validate one workflow transition against current evidence. |
| `synthesize_result` | Produce the normalized programming result surface from accumulated evidence. |

The output envelope follows the same broad pattern as executable skills:

```json
{
  "status": "success",
  "duration_ms": 4,
  "metadata": {
    "runner_id": "navi-programmer.bounded-mutation-runner",
    "interface": "evaluate_transition"
  },
  "output": {}
}
```

## `evaluate_transition` Input

```json
{
  "interface": "evaluate_transition",
  "arguments": {
    "current_state": "validating",
    "target_state": "synthesizing",
    "task_class": "bounded_mutation",
    "available_skills": [
      "navi-programmer.task-normalize",
      "navi-programmer.repo-inspect",
      "navi-programmer.file-mutate",
      "navi-programmer.patch-apply",
      "navi-programmer.run-validation",
      "navi-programmer.git-lifecycle",
      "navi-programmer.remote-review"
    ],
    "evidence": {}
  }
}
```

`available_skills` is intentionally explicit. If the transition path needs a
skill and the caller has not declared it available, the runner blocks the
transition with `required_skill_missing`.

## Evidence Rules

For each transition, the runner checks cumulative evidence through the current
state. For example, leaving `validating` requires evidence from intake,
normalization, scope binding, inspection, planning, execution, and validation.

The runner accepts top-level evidence keys from the workflow contract and common
nested aliases from the existing executable skills. For example:

- `normalized_task.mutation_intent` satisfies `mutation_intent`.
- `workspace_binding.allowed_scope` satisfies `allowed_scope`.
- `execution_plan.validation_plan` satisfies `validation_plan`.
- `changed_files` satisfies `final_changed_files` when reviewing completion.

`record_step` is the local bridge between skill envelopes and workflow
evidence. It unwraps the standard skill result shape, maps known
`navi-programmer.*` skill/interface pairs to the workflow evidence ledger, and
preserves per-step status in `step_results`.

The runner still treats the caller as the executor. It does not read files,
write files, run commands, create branches, push, or open PRs.

## Validation Gate

Validation is not optional for mutation tasks. If changed files are present, or
the caller declares a validation-required condition, leaving validation or
claiming completion requires `validation_results`.

Verdict handling:

| Verdict | Effect |
| --- | --- |
| `passed` | Eligible for completion if all other evidence is present. |
| `failed` | Must route to `partially_succeeded` or `failed`; cannot complete cleanly. |
| `timed_out` | Must route to `partially_succeeded` or `blocked`; cannot complete cleanly. |
| `not_run` | Eligible only when an explicit reason is present. |

If changed files exist and validation evidence is absent, the runner returns
`failure_class: validation_missing` and outcome `failed`.

## Current Limit

This runner is a local harness, evidence ledger, transition evaluator, and
result synthesizer. It is not the final autonomous executor. NAVI core still
needs to own skill invocation, sandboxing, multi-agent coordination, retries,
durable run storage, and user confirmation for remote actions.
