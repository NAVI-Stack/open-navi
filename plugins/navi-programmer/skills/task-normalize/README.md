# task-normalize

Conservative task normalization and scope binding for NAVI Programmer.

This skill turns raw programming requests into structured execution targets. It
is intentionally heuristic in V1: it preserves the original request, classifies
common request shapes, identifies obvious ambiguity, and blocks instead of
guessing when repository scope is missing or unsafe.

## Interfaces

| Interface | Purpose |
| --- | --- |
| `normalize_task` | Preserve raw task input and produce task class, mutation intent, acceptance target, repo binding requirement, likely affected areas, validation hints, risk hints, and ambiguity notes. |
| `bind_scope` | Select and validate an explicit repository/workspace binding from caller-provided repo hints, current repo, explicit repo, or candidate repos. |

## Normalization Rules

- Raw task content is always preserved in the output.
- Mutative tasks require repo context before execution.
- Self-update candidates are distinguished from ordinary mutation when the task
  appears to target NAVI, NAVI Programmer, or this plugin package.
- Broad requests are marked as requiring decomposition.
- Remote or release-oriented requests are marked as requiring confirmation or
  future workflow support.
- Ambiguous requests return `state: blocked` with `blocked_reasons`.

## Scope Binding Rules

- `explicit_repo` wins when provided.
- `current_repo` is accepted as an explicit current context.
- `repo_hint` may select exactly one candidate repo.
- Multiple candidate matches block.
- Multiple candidates without a hint block unless
  `allow_single_candidate_default` is true and exactly one candidate exists.
- `allowed_scope` paths must resolve inside the selected repo.
- Self-update bindings can require an explicit `candidate_context`.
