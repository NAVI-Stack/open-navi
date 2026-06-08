---
name: verification-loop
description: "Run a scoped verification pass for NAVI work: pick the right build, test, diff, and safety checks for the change, then report readiness and blockers clearly."
metadata:
  navi:
    emoji: "✅"
---
# Verification Loop

Use this skill before declaring meaningful work complete.

This is a NAVI-native, repo-local port of a verification pattern. It is advisory guidance for choosing and reporting the right checks. It does not replace judgment about scope.

## When To Use

- After implementing a feature, fix, refactor, or migration
- Before handing work back to the user
- Before opening or updating a PR
- After changing skills, prompts, templates, or operator docs

## Verification Process

### Phase 1: Scope The Change

Identify what changed before you run checks:

- Go runtime code
- gateway or operator surfaces
- skills or connectors
- docs and templates only
- mixed work across several layers

Do not run the heaviest verification by reflex if the change is clearly narrower, but do not skip checks that are necessary for confidence.

### Phase 2: Build And Static Checks

For Go code, prefer the repo's standard commands:

```bash
go build ./...
go test ./... -count=1
```

If the change is intentionally scoped to a smaller package, run the targeted package tests during iteration and call out that the full suite was not run.

For docs- or skill-only work, verify references, paths, indexes, and workflow consistency instead of pretending a code build proves the change.

### Phase 3: Runtime-Oriented Validation

When relevant, verify the surfaces you actually changed:

- skills: confirm docs, manifests, and reload guidance remain accurate
- connectors: confirm config and runbook drift is addressed
- gateway/operator APIs: run the nearest targeted tests or at least validate the affected route and auth docs
- templates: confirm new workspace files or conventions are linked from the right docs

### Phase 4: Safety Review

Check for:

- accidental secret exposure
- destructive commands or risky defaults
- unintended file churn in a dirty worktree
- missing documentation for operator-facing behavior

### Phase 5: Diff Review

Review the actual changed surface:

```bash
git diff --stat
git diff -- <paths>
```

Confirm that the final diff matches the task and does not include unrelated edits.

## Output Format

Report verification in this order:

1. scope
2. checks run
3. pass/fail status per check
4. unresolved blockers or gaps
5. overall readiness

## Rules

- Do not claim "verified" if an important check was skipped.
- If the repo is already dirty, distinguish your changes from pre-existing edits.
- Match the depth of verification to the risk of the change, then say exactly what you did.
