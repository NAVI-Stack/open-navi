# Project State

**Status:** Active
**Last Updated:** 2026-06-06
**Phase:** 6 - NAVI Coder Migration

## Current Goal

Migrate the closed NAVI Programmer V1 prototype into the NAVI Coder product and
domain model without breaking the legacy package, skill IDs, workflow IDs, or
runtime compatibility surfaces that current tests and gateway discovery depend
on.

## What Is Done

- Plugin package lives under `plugins/navi-programmer/` in the
  NAVI repo.
- `plugin.yaml` carries the stable plugin identifier `navi.programmer`.
- `plugin.yaml` now also carries machine-visible dependency declarations and a
  typed V1 contract surface for workflow entry points, included behavior,
  excluded behavior, and isolated self-update mode (OMN-237).
- NAVI core now parses and validates the programmer manifest's declared
  workflow contracts as typed metadata instead of leaving the runtime workflow
  surface as opaque YAML-only structure, so the plugin's workflow ownership and
  contract paths are machine-visible during manifest loading (OMN-236 / NP-025).
- The OMN-234 / Programmer V1 umbrella is now closed with a direct
  end-to-end regression that chains task normalization, repo binding,
  inspection, bounded mutation, validation, review handoff, commit creation,
  and bounded-mutation result synthesis through the actual plugin skill
  entrypoints.
- Skill IDs consistently use the `navi-programmer.<folder-name>` namespace,
  matching `plugin.yaml`, each `SKILL.yaml`, workflow contracts, fixtures, and
  runner evidence.
- Plugin-owned NAVI Programmer docs migrated from NAVI core into this package.
- NAVI core docs now keep forwarding pointers instead of owning plugin docs.
- The baseline NAVI Programmer concept, design, spec, and plan corpus is now
  surfaced directly from the plugin docs entrypoints, and a dedicated Python
  regression test locks the required files, README / index cross-links, and
  first-class-plugin framing in place so future edits do not silently drift
  away from the OMN-235 contract.
- Prototype executable skill directories exist:
  - `repo-inspect`
  - `file-mutate`
  - `patch-apply`
  - `run-validation`
  - `git-lifecycle`
  - `remote-review`
  - `task-normalize`
- Workflow contracts created:
  - `bounded-mutation.yaml`
  - `ticket-driven-coding.yaml`
  - `self-update-candidate.yaml`
- Starter test/eval directories created.
- Local plugin discovery and sync path documented in
  `docs/runbooks/local-plugin-development.md` (NP-001).
- First executable skill created at `skills/repo-inspect/` with a
  `subprocess_python` `SKILL.yaml` and stdlib-only `main.py` entrypoint
  (NP-002).
- First executable validation skill created at `skills/run-validation/` with
  bounded command execution and explicit `not_run` evidence support (NP-003).
- `run-validation` now uses a NAVI internal handler backed by
  `internal/sandbox.Runner` for runtime execution, while its Python entrypoint
  remains available as a standalone local harness for plugin tests (NP-015).
- Bounded mutation workflow contract expanded in
  `workflows/bounded-mutation.yaml` with companion spec
  `docs/specs/navi-programmer-bounded-mutation-workflow.md` (NP-004).
- Executable mutation skills created at `skills/file-mutate/` and
  `skills/patch-apply/` with preconditioned writes and structured patch
  preview/apply evidence (NP-007).
- Direct Python regression tests now cover `file-mutate.create_file`,
  `file-mutate.write_file`, `patch-apply.preview_patch`, and
  `patch-apply.apply_patch` happy/error paths, bringing the mutation surface up
  to the same completion bar as the other executable programmer skills
  (OMN-240).
- Executable Git lifecycle skill created at `skills/git-lifecycle/` with local
  status, branch dry-run/create, explicit-path commit, and review handoff
  evidence (NP-008).
- Confirmation-gated remote review skill created at
  `skills/remote-review/` with real `git push` and `gh pr create` execution,
  structured blocked outcomes when confirmation is absent, and review surfaces
  that carry validation context (NP-011).
- Executable task normalization skill created at `skills/task-normalize/` with
  raw task preservation, intent classification, conservative scope binding, and
  ambiguity blocking (NP-010).
- Local bounded mutation runner harness created at
  `workflows/bounded_mutation_runner.py` with compiled contract
  `workflows/bounded-mutation.compiled.json` for transition evaluation,
  evidence checks, skill availability checks, governance checks, and validation
  gate enforcement (NP-009).
- The runner now also exposes `start_run`, `record_step`, and
  `synthesize_result` interfaces so NAVI core can later feed skill results into
  a consistent evidence ledger and programming result envelope without
  inventing a separate harness shape.
- Ticket-driven coding now has a real workflow contract and compiled runner
  contract, and NAVI core selects that contract for Linear/task-sourced
  programmer runs while preserving source metadata through synthesized results
  (OMN-247 / NP-019).
- NAVI core now emits programmer workflow progress using the bounded mutation
  runner's real phase states and attaches the synthesized structured programmer
  result to `ExecuteResult`, so branch/commit, changed-file, validation, and
  blocked/failure context stop living only in scratchpad JSON (OMN-244 /
  NP-020).
- Starter fixture/eval set created under `tests/fixtures/` and `tests/evals/`
  with a local checker for read-only comprehension and bounded documentation
  mutation evidence paths (NP-005).
- Structured evaluation harness now lives at
  `tests/evals/eval_harness.py` with a 25-case starter suite manifest at
  `tests/evals/starter-evaluation-suite.json`.
- Evaluated runs now produce consistent records with task class, complexity
  band, outcome class, capability scores, lifecycle evidence, and self-update
  safety notes where applicable.
- Weekly maturity reporting now aggregates actual starter-suite evidence across
  plugin scaffold, repo comprehension, mutation, validation, repo lifecycle,
  task execution, self-update candidate path, ticket-driven work, and
  reliability (NP-013 / OMN-277).
- NAVI core now has sandbox profile persistence, project readiness gating,
  sandbox profile API routes, and a Docker runner surface; coding project
  readiness requires both an active workspace and active sandbox profile
  (NP-014).
- Self-update candidate runbook written at
  `docs/runbooks/self-update-candidate.md` and
  `workflows/self-update-candidate.yaml` fleshed out with full states,
  transitions, invariants, evidence gates, and failure classes (NP-006).
- Self-update candidate evidence now captures structured runtime smoke results,
  synthesizes promotion readiness separately from local validation success, and
  blocks clean completion when runtime verification is still missing
  (OMN-246 / NP-016).
- NAVI core now switches self-targeted programmer runs onto the dedicated
  `self-update-candidate` compiled workflow as soon as normalization classifies
  the task as `self_update_candidate`, and the structured programmer result now
  carries explicit `candidate_context` for self-update reporting (OMN-245 /
  NP-021).
- NAVI core now enforces programmer repo/workspace binding at runtime by
  defaulting self-update `bind_scope` calls to the NAVI repo, injecting the
  bound repo root into downstream programmer skill execution, and blocking
  mutative tool calls that either lack binding evidence or target paths outside
  the allowed scope (OMN-238 / NP-022).
- The Coder migration now has an explicit inventory at
  `docs/plans/navi-coder-migration-inventory.md` that classifies stale
  `Programmer` references into user-facing labels, staged compatibility IDs,
  fixture compatibility, and migration history so follow-on rename work can be
  deliberate (OMN-280 / NP-027).

## What Is In Progress

- NAVI Coder migration. The legacy package remains `plugins/navi-programmer/`
  for compatibility, but user-facing product language should move to NAVI
  Coder in staged follow-up tasks.

## What Is Next

1. Re-triage the OMN-279 children after OMN-280.
2. Prefer the next task that removes user-facing stale product language or adds
   first-class Coder shape without breaking legacy `navi-programmer.*`
   compatibility.
3. Keep runtime IDs staged until alias support or an explicit move plan exists.

## Current Risks

- All V1 skill families in the bounded mutation workflow now have prototype
  executable contracts.
- Remote review actions now exist as a distinct governed surface instead of
  being folded into the local-only git lifecycle skill.
- Bounded mutation now has a local runner harness, sandbox-backed validation,
  real skill-result evidence stitching, and structured result attachment in
  NAVI core, but deeper retry/orchestration behavior still belongs to future
  runtime work.
- Runtime-side repo binding is now authoritative for mutative programmer tool
  calls, but broader multi-step eval coverage still needs to prove the model
  keeps choosing the bind-before-mutate path reliably.
- Live gateway verification now proves the built-in plugin manifest is
  discoverable, the seven manifest skill IDs match the live skill registry, and
  `repo-inspect` plus both `run-validation` interfaces can be invoked through
  NaviD.
- The supported default Compose runtime now provides sandbox-backed validation
  by installing `docker-cli`, mounting `/var/run/docker.sock`, and setting
  `NAVI_SANDBOX_CONTAINER_NAME=navid`; the sandbox runner uses `docker exec`
  against the running container so bounded validation commands can execute
  against the repo already baked into the image.
- `compose.strict.yml` intentionally does not expose that Docker socket path, so
  strict mode can still return structured sandbox blockers for validation until
  a stricter execution design exists.
- The plugin depends on NAVI core project/workspace/sandbox behavior that is
  still maturing.

## Ownership Boundary

The legacy `plugins/navi-programmer/` package owns the current executable
coding capability substrate. NAVI Coder is the product/domain identity that
should own user-facing language and future first-class backend/console shape.
NAVI core owns runtime, loading, governance, and shared execution APIs.
