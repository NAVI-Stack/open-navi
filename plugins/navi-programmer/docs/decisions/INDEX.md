# Decisions - Index

Lightweight decision records for NAVI Programmer.

| Decision | Status | Summary |
| --- | --- | --- |
| [0006-local-workflow-runner-harness.md](0006-local-workflow-runner-harness.md) | Accepted | Bounded mutation uses a local evidence-gate runner harness until NAVI core owns full workflow orchestration. |
| [0005-conservative-task-normalization.md](0005-conservative-task-normalization.md) | Accepted | Task normalization is conservative and blocks ambiguous scope instead of guessing. |
| [0004-local-git-lifecycle-only.md](0004-local-git-lifecycle-only.md) | Accepted | Git lifecycle is local-only; remote push and PR creation live in the separate confirmation-gated `remote-review` skill. |
| [0003-preconditioned-mutation-skills.md](0003-preconditioned-mutation-skills.md) | Accepted | Mutation skills require preconditions and produce structured diff evidence. |
| [0002-bounded-validation-runner.md](0002-bounded-validation-runner.md) | Accepted | Validation uses a bounded no-shell runner with structured evidence and core-owned sandbox hardening. |
| [0001-plugin-source-package.md](0001-plugin-source-package.md) | Accepted | NAVI Programmer lives as a plugin package under the NAVI repo's `plugins/` tree for NAVI-first iteration. |
