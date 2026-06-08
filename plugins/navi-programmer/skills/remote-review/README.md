# remote-review

Confirmation-gated remote review actions for NAVI Programmer.

This skill exists separately from `git-lifecycle` so local branch/commit work
can remain autonomous while remote push and pull request creation stay on a
stricter governed path.

## Interfaces

| Interface | Purpose |
| --- | --- |
| `push_branch` | Push a local branch to a named remote after explicit confirmation. |
| `create_pull_request` | Create a draft or ready pull request with validation and changed-file context after explicit confirmation. |

## Governance Rules

- Both interfaces require explicit confirmation metadata before they execute.
- Missing or unapproved confirmation returns a structured
  `blocked_requires_confirmation` result instead of pretending the action ran.
- `push_branch` uses real `git push` subprocess execution.
- `create_pull_request` uses the `gh` CLI and fails clearly when it is missing.

## Review Surface

Both interfaces produce a review surface that includes:

- summary
- validation summary
- changed files
- remote action context
- PR body content where applicable
