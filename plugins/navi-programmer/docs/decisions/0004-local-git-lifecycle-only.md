# 0004 - Local Git Lifecycle Only

**Status:** Accepted
**Date:** 2026-04-28

## Decision

NAVI Programmer Git lifecycle starts as a local-only skill in
`skills/git-lifecycle/`.

The skill may:

- inspect branch, head, status, and diffs
- create local branches
- create local commits from explicit path lists
- prepare review handoff Markdown

The skill does not push branches, create pull requests, or mutate remote hosting
systems. Remote actions are intentionally owned by a separate governed surface
instead of widening this local skill.

## Rationale

Review-ready output needs branch, commit, and handoff evidence, but remote
mutation is a higher-trust action than local repository lifecycle. Keeping V1
local-only lets NAVI Programmer produce reviewable work while preserving a clear
governance boundary.

## Consequences

- `create_commit` requires explicit `paths` and does not stage the whole repo by
  default.
- Untracked files require `allow_untracked: true`.
- Dirty worktrees block branch creation unless `allow_dirty: true`.
- PR creation and push automation live in `skills/remote-review/` behind
  explicit confirmation and hosting-provider policy.
