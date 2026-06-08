# git-lifecycle

Local Git lifecycle skill for NAVI Programmer.

This skill prepares review-ready repository evidence without performing remote
actions. It can inspect status and diffs, create local branches, create local
commits from explicit path lists, and produce review handoff text.

## Interfaces

| Interface | Purpose |
| --- | --- |
| `inspect_status` | Report branch, head, upstream, dirty state, changed files, staged files, unstaged files, untracked files, and optional diff. |
| `create_branch` | Create and optionally check out a local branch. Dirty worktrees are blocked unless `allow_dirty: true`. |
| `create_commit` | Stage explicit paths and create a local commit. Dry run reports selected changes without touching the index. |
| `prepare_review` | Produce local handoff Markdown with status, validation summary, changed files, and optional diff. |

## Safety Rules

- Remote push and PR creation are not performed by this skill.
- `create_commit` requires explicit `paths`; it does not stage the whole repo by
  default.
- Untracked files require `allow_untracked: true` before commit creation.
- Branch names are checked with `git check-ref-format --branch`.
- Pathspecs must resolve inside the Git repo and may not target repository
  internals or dependency/build/cache folders.
- `dry_run: true` is supported for branch and commit creation.

## Review Handoff

`prepare_review` is local-only. It returns:

- branch/head/upstream
- changed file evidence
- validation summary supplied by the caller
- handoff Markdown
- remote action flags showing push and pull request creation were not performed
