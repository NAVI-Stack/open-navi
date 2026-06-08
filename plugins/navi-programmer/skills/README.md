# NAVI Programmer Skills

This directory defines the intended atomic skill families for NAVI Programmer.

Skill directories start as capability ownership boundaries. A directory becomes
executable when it contains a completed `SKILL.yaml` with transport metadata and
an entrypoint or handler contract.

Skill IDs use the package slug and folder slug: `navi-programmer.<skill-folder>`.
Keep that convention aligned across `plugin.yaml`, `SKILL.yaml` files,
workflow contracts, fixtures, and docs.

| Skill Family | State | Purpose |
| --- | --- | --- |
| `repo-inspect/` | Prototype executable | Read files, list directories, search code, inspect repository state, and inspect diffs. |
| `file-mutate/` | Prototype executable | Create or update files within governed workspace scope. |
| `patch-apply/` | Prototype executable | Apply bounded patch-like edits and preserve reviewable diffs. |
| `run-validation/` | Prototype executable | Run build, test, lint, or static validation commands and capture evidence. |
| `git-lifecycle/` | Prototype executable | Inspect branch/status, create branches, create commits, and prepare review output. |
| `remote-review/` | Prototype executable | Push branches and create pull requests only through explicit confirmation-gated remote actions. |
| `task-normalize/` | Prototype executable | Convert raw programming requests or tickets into scoped execution targets. |
