# Blockers

**Status:** Active
**Last Updated:** 2026-05-08

## Open Blockers

### End-to-end orchestration is not fully automated

Impact:

- The local runner enforces workflow evidence gates and can normalize step
  evidence/results, but NAVI core still needs to invoke skills in sequence,
  collect evidence, handle retries, and feed the runner without manual
  stitching.

Tracked by:

- Future NAVI core orchestration work

## Recently Resolved

### Manifest and skill IDs drifted

Resolved on 2026-05-08. `plugin.yaml` now declares the same
`navi-programmer.<folder-name>` skill IDs that each `SKILL.yaml` declares, and
the local contract test checks this.

### Workflow runner was only an evidence gate

Resolved on 2026-05-08 for the local harness layer. The runner still does not
invoke skills itself, but it now exposes `start_run`, `record_step`, and
`synthesize_result` so NAVI core can use a consistent task/result envelope
instead of hand-building evidence shape around every skill call.

### Evaluation fixtures did not exist yet

Resolved on 2026-04-29. `tests/fixtures/` and `tests/evals/` now include a
starter read-only comprehension case, a bounded docs mutation case, and
`tests/evals/run_eval_fixtures.py` for local evidence-shape and runner-gate
checks.

### Workflow contract was not wired to a runner

Resolved on 2026-04-29. `workflows/bounded_mutation_runner.py` now consumes
`workflows/bounded-mutation.compiled.json`, evaluates state transitions, checks
cumulative evidence and skill availability, enforces governance rules, and
blocks or fails honestly when validation evidence is missing.

### Task normalization skill was a placeholder

Resolved on 2026-04-28. `skills/task-normalize/` now contains a concrete
`SKILL.yaml` and stdlib-only `main.py` entrypoint for raw task preservation,
intent classification, scope binding, validation hints, and ambiguity blocking.

### Git lifecycle skill was a placeholder

Resolved on 2026-04-28. `skills/git-lifecycle/` now contains a concrete
`SKILL.yaml` and stdlib-only `main.py` entrypoint for status inspection, local
branch creation, explicit-path commit creation, and review handoff preparation.

### Mutation skills were placeholders

Resolved on 2026-04-28. `skills/file-mutate/` and `skills/patch-apply/` now
contain concrete `SKILL.yaml` contracts and stdlib-only `main.py` entrypoints
for preconditioned file writes and structured patch preview/application.

### Bounded mutation workflow was only a placeholder

Resolved on 2026-04-28. `workflows/bounded-mutation.yaml` now defines state
mapping, evidence requirements, validation gates, and terminal outcome
semantics, with details in
[`navi-programmer-bounded-mutation-workflow.md`](../specs/navi-programmer-bounded-mutation-workflow.md).

### Validation was not executable

Resolved on 2026-04-28. `skills/run-validation/` now contains a concrete
`SKILL.yaml` and stdlib-only `main.py` entrypoint with bounded command
execution and explicit `not_run` evidence.

### Repo inspection was not executable

Resolved on 2026-04-28 and reconciled on 2026-05-08.
`skills/repo-inspect/` now contains a concrete `SKILL.yaml` and stdlib-only
`main.py` entrypoint with list, read, search, Git status, and diff inspection
interfaces.

### Local plugin discovery path was not finalized

Resolved on 2026-04-28. [local-plugin-development.md](../runbooks/local-plugin-development.md)
now documents native global, workspace-local, and Docker Compose discovery
paths.

### Plugin source package did not exist

Resolved on 2026-04-28. `plugins/navi-programmer/` now contains
the plugin package, manifest, migrated docs, skill contracts, workflow
contracts, and project workflow files.
