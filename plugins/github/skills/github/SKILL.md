---
name: github
description: "Interact with GitHub repositories, issues, pull requests, and CI using the gh CLI"
metadata:
  navi:
    emoji: "🐙"
    requires:
      bins: ["gh"]
---
# GitHub

Use this skill when the user asks about GitHub repositories, issues, PRs, Actions runs, or releases.

## Prerequisites
- The `gh` CLI must be authenticated: `gh auth status`

## Common Operations

### Issues
```bash
gh issue list --repo owner/repo --state open --limit 20
gh issue view 123 --repo owner/repo
gh issue create --title "Title" --body "Body" --repo owner/repo
gh issue close 123 --repo owner/repo
```

### Pull Requests
```bash
gh pr list --repo owner/repo --state open
gh pr view 456 --repo owner/repo
gh pr create --title "Title" --body "Body" --base main --repo owner/repo
gh pr merge 456 --squash --repo owner/repo
gh pr checks 456 --repo owner/repo
```

### Actions / CI
```bash
gh run list --repo owner/repo --limit 10
gh run view <run-id> --repo owner/repo
gh run rerun <run-id> --repo owner/repo
gh workflow run <workflow-name> --repo owner/repo
```

### Releases
```bash
gh release list --repo owner/repo
gh release view v1.0.0 --repo owner/repo
gh release create v1.1.0 --notes "Release notes" --repo owner/repo
```

## Rules
- Always confirm the target repository with the user before destructive operations (close, merge, delete).
- When listing, default to `--limit 20` unless the user asks for more.
- For the current repo, omit `--repo owner/repo` — `gh` infers it from git remote.
