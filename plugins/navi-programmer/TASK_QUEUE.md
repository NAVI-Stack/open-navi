# Task Queue

**Status:** Active
**Last Updated:** 2026-06-06
**Selection Rule:** Work CRITICAL before HIGH before MEDIUM. Keep tasks small,
reviewable, and tied to the plugin contract.

## Active

No task is currently marked `IN_PROGRESS`.

## Pending

No task is currently pending.

## Done

### NP-027: Classify Programmer References for Coder Migration
**Status:** DONE
**Priority:** HIGH
**Surface:** `docs/plans/`, plugin status docs, plugin tests
**Completed:** 2026-06-06

Added `docs/plans/navi-coder-migration-inventory.md` and a regression test that
keeps the inventory indexed, records the missing Linear-referenced source docs,
prioritizes user-facing stale naming before compatibility IDs, and prevents the
new inventory from presenting Programmer as the current product identity.

Acceptance criteria:

- Migration inventory exists in repo docs.
- User-facing stale naming is identified first.
- New migration docs present NAVI Coder as canonical and treat Programmer as
  legacy, transitional, compatibility-preserving, or historical.

### NP-026: Close Programmer V1 Umbrella Proof
**Status:** DONE
**Priority:** HIGH
**Surface:** `plugins/navi-programmer/tests/`, plugin status docs
**Completed:** 2026-05-27

Closed the OMN-234 V1 umbrella by adding an end-to-end regression that uses the
actual plugin skill entrypoints to normalize a task, bind repo scope, inspect
the target repo, mutate a scoped file, run validation, prepare a review handoff,
create a commit, and synthesize the bounded-mutation result surface.

Acceptance criteria:

- The V1 definition-of-done is backed by a direct end-to-end plugin regression.
- Reviewable branch/commit metadata and validation evidence flow into the
  structured runner result.
- Plugin-local handoff docs now point at post-V1 hardening instead of V1
  re-triage.

### NP-025: Type and Validate Programmer Workflow Manifest Metadata
**Status:** DONE
**Priority:** HIGH
**Surface:** `internal/navi/plugin/`, `plugins/navi-programmer/plugin.yaml`, plugin tests/docs
**Completed:** 2026-05-26

Parsed `runtime.workflow_contracts` into a typed plugin-manifest surface and
validated that declared workflow contract and runner paths stay inside the
plugin root and exist on disk, so the programmer plugin's workflow ownership
stops being YAML-only text.

Acceptance criteria:

- The programmer plugin's workflow contracts are machine-visible through core manifest loading.
- Broken workflow contract metadata fails validation instead of loading silently.
- The scaffold ticket no longer depends on untyped runtime metadata for workflow ownership.

### NP-024: Lock the Programmer Documentation Corpus Contract
**Status:** DONE
**Priority:** HIGH
**Surface:** `plugins/navi-programmer/docs/`, plugin tests
**Completed:** 2026-05-25

Surfaced the required concept/design/plan documents directly from the plugin
docs README and added a dedicated regression test that verifies the required
corpus files exist, the README and docs index link to them, and the concept /
architecture docs keep framing NAVI Programmer as a first-class capability
plugin rather than a role or domain.

Acceptance criteria:

- All required docs exist in the correct corpus sections.
- Docs entrypoints link to the required corpus documents.
- Programmer remains framed as a first-class capability plugin, not a role or domain.

### NP-023: Expose Machine-Visible Programmer Contract Metadata
**Status:** DONE
**Priority:** HIGH
**Surface:** `internal/navi/plugin/`, `plugins/navi-programmer/plugin.yaml`, plugin tests/docs
**Completed:** 2026-05-25

Added typed manifest fields for plugin dependency declarations and V1 contract
scope, populated the NAVI Programmer manifest with required core dependencies,
task-source dependencies, workflow entry points, excluded behavior, and
isolated self-update mode, and refreshed the stale README next-step pointer.

Acceptance criteria:

- Plugin contract exists in code and aligns with `docs/specs/navi-programmer-plugin-v1.md`.
- Dependency and capability declarations are machine-visible.
- V1 scope can be introspected cleanly.

### NP-022: Enforce Runtime Repo Binding for Programmer Mutations
**Status:** DONE
**Priority:** HIGH
**Surface:** `internal/navi/`, programmer workflow bridge/runtime tests
**Completed:** 2026-05-24

Made programmer repo/workspace binding authoritative at execution time by
defaulting self-update `bind_scope` calls to the NAVI repo, injecting the
bound repo root into downstream programmer skill arguments, and blocking
mutative calls when binding is missing or the target path escapes
`allowed_scope`.

Acceptance criteria:

- Mutation tasks cannot proceed without repo binding.
- Out-of-scope mutation is rejected before execution.
- Self-update tasks resolve the NAVI repo context explicitly during scope binding.

### NP-021: Route Self-Targeted Runs Through the Self-Update Workflow
**Status:** DONE
**Priority:** HIGH
**Surface:** `internal/navi/`, `plugins/navi-programmer/workflows/`, plugin contract tests
**Completed:** 2026-05-24

Added the compiled self-update workflow contract, taught NAVI core to switch
self-targeted programmer runs onto that contract once normalization classifies
the task as `self_update_candidate`, and surfaced explicit `candidate_context`
inside the structured programmer result.

Acceptance criteria:

- Self-targeted tasks do not report through the generic bounded workflow by default.
- Candidate context is explicit in structured reporting for self-update runs.
- The self-update compiled contract is present and verified by plugin tests.

### NP-020: Add Programmer Progress and Structured Result Reporting
**Status:** DONE
**Priority:** MEDIUM
**Surface:** `internal/navi/`, `internal/runtime/`, plugin status docs
**Completed:** 2026-05-23

Moved programmer workflow reporting from scratchpad-only bookkeeping into the
real NAVI execution surface by emitting phase-level bounded-mutation runner
states as runtime progress and attaching the synthesized structured runner
result to `ExecuteResult`.

Acceptance criteria:

- Long-running tasks show meaningful execution phase changes.
- Final result includes repo target, files changed, validation, and lifecycle output.
- Results are usable by weekly evaluation.

### NP-019: Add Ticket-Driven Workflow Contract and Linear Source Preservation
**Status:** DONE
**Priority:** MEDIUM
**Surface:** `internal/navi/`, `workflows/`, `tests/`
**Completed:** 2026-05-22

Promoted ticket-driven coding from a planned placeholder into a real workflow
contract, added a compiled runner contract, taught NAVI core to select that
contract for Linear/task-sourced programmer runs, and preserved source
metadata through the synthesized result surface.

Acceptance criteria:

- Plugin can begin execution from a Linear issue.
- Normalized task and source metadata remain preserved through execution.
- Output stays reviewable and consistent with chat-origin tasks.

### NP-018: Enable Compose Runtime Validation Path
**Status:** DONE
**Priority:** HIGH
**Surface:** `internal/sandbox/`, `Dockerfile`, `compose.yml`, plugin runtime docs
**Completed:** 2026-05-16

Enabled bounded validation commands in the supported default Docker Compose
runtime by teaching the core sandbox runner to use `docker exec` against the
running `navid` container when `NAVI_SANDBOX_CONTAINER_NAME` is configured,
installing `docker-cli` in the runtime image, mounting the Docker socket in
`compose.yml`, and documenting the resulting contract.

Acceptance criteria:

- A supported Docker Compose NaviD runtime can complete at least one bounded
  validation command through `navi-programmer.run-validation`.
- The live result keeps the existing structured validation evidence shape.
- The default Compose runtime contract is documented clearly for future plugin
  work.

### NP-016: Add Candidate Runtime Startup and Smoke Validation Signals
**Status:** DONE
**Priority:** HIGH
**Surface:** `workflows/`, `tests/fixtures/`, `tests/evals/`, plugin status docs
**Completed:** 2026-05-15

Extended the bounded mutation runner and plugin-local eval surface so
self-update tasks record candidate runtime smoke evidence, synthesize a
promotion readiness summary distinct from local validation success, and block
clean self-update completion when runtime verification is still missing.

Acceptance criteria:

- Candidate runtime smoke outcomes are captured as structured evidence.
- Promotion readiness distinguishes runtime-verified candidates from merely
  locally-valid ones.
- Self-update completion blocks when runtime smoke evidence is missing.
- Plugin-local Python tests, `go build ./...`, and `go test ./... -count=1`
  pass.

### NP-017: Live-Verify Programmer Plugin Discovery
**Status:** DONE
**Priority:** HIGH
**Surface:** Docker Compose NaviD runtime, gateway plugin/skill APIs, plugin status docs
**Completed:** 2026-05-16

Rebuilt the supported Docker Compose NaviD runtime from this checkout and
verified that `navi.programmer` is discoverable through `/api/plugins`, that
the seven manifest-declared programmer skill IDs match the live `/api/skills`
registry exactly, and that `repo-inspect.read_file`,
`run-validation.record_not_run`, and `run-validation.run_command` all invoke
through the live gateway with structured results.

Acceptance criteria:

- `navi.programmer` is visible through the running NaviD plugin API.
- At least one programmer skill call produces structured live output.
- `run-validation` is registered and returns structured validation evidence or
  an explicit sandbox blocker.
- No stale manifest-only programmer skill IDs appear in the live skill
  registry.

### NP-015: Route Validation Through Core Sandbox Runner
**Status:** DONE
**Priority:** HIGH
**Surface:** `plugins/navi-programmer/handlers/`, `skills/run-validation/`, NaviD plugin bootstrap, readiness docs
**Completed:** 2026-05-12

Changed `navi-programmer.run-validation` from a standalone runtime command
surface into a NAVI internal handler backed by `internal/sandbox.Runner`.
The handler keeps validation-specific command policy, resolves the configured
sandbox profile, executes through the core sandbox runner, and returns the same
structured validation evidence shape consumed by the bounded mutation runner.

Acceptance criteria:

- `run-validation.run_command` uses the core sandbox runner in a running NAVI
  instance.
- Missing or unavailable sandbox profiles produce explicit `not_run` evidence
  with a sandbox blocker.
- Validation command policy continues to reject shells, dependency mutation,
  inline execution, path-style executables, and non-validation commands.
- `record_not_run` remains available through the internal transport.
- Handler-level Go tests and plugin-local Python tests pass.

### NP-014: Add Core Sandbox Profile and Local Docker Runner Surface
**Status:** DONE
**Priority:** HIGH
**Surface:** `internal/schema/`, `internal/store/`, `internal/worldmodel/`, `internal/gateway/`, `internal/sandbox/`, plugin readiness docs
**Completed:** 2026-05-12

Added first-pass sandbox profile persistence, project readiness gating, API
routes for sandbox profile configuration, and a core local Docker runner
surface with bounded mounts, network mode, timeouts, command allowlists, and
env allowlists. Coding projects now require both a usable workspace and active
sandbox profile before mutative task intake can proceed.

Acceptance criteria:

- Sandbox profiles can be persisted, listed, fetched, and bound to projects.
- Coding project readiness reports degraded state until workspace and sandbox
  profile requirements are met.
- Mutative coding task intake blocks when sandbox readiness is missing.
- Docker runner builds network-off, workspace-mounted command execution args
  and blocks missing Docker, unallowlisted commands, and unallowlisted env.
- Targeted Go and plugin Python tests pass.

### NP-013: Build Evaluation Harness and Weekly Maturity Reporting
**Status:** DONE
**Priority:** HIGH
**Surface:** `tests/evals/`, `tests/fixtures/`, `tests/`
**Completed:** 2026-05-09

Built a structured plugin-local evaluation harness that records evaluated runs,
tracks task class / complexity / outcome / capability scores, expands the
starter suite to the plan minimums, and emits a weekly maturity summary that
keeps unsafe self-update failures visible.

Acceptance criteria:

- Evaluated runs can be recorded consistently.
- Weekly review can summarize progress using actual evidence.
- Self-update safety failures are visible and not hidden inside generic failure buckets.

### NP-012: Reconcile Programmer Harness Identity and Runner Contracts
**Status:** DONE
**Priority:** HIGH
**Surface:** `plugin.yaml`, `skills/`, `workflows/`, `docs/`, `tests/`
**Completed:** 2026-05-08

Reconciled manifest skill IDs with `SKILL.yaml`, refreshed workflow contracts,
added runner evidence/result interfaces, and updated plugin-local docs to stay
NAVI-first and truthful about current runtime readiness.

Acceptance criteria:

- Manifest skill IDs match each `SKILL.yaml` skill ID.
- `repo-inspect.inspect_diff` is represented in workflow contracts.
- Runner can accumulate skill evidence and synthesize a result envelope.
- Plugin-local docs no longer point at Helm or stale external package paths.

### NP-006: Define Self-Update Candidate Runbook
**Status:** DONE
**Priority:** MEDIUM
**Surface:** `docs/runbooks/`, `workflows/self-update-candidate.yaml`
**Completed:** 2026-04-30

Wrote the operational self-update candidate runbook and fleshed out the
workflow YAML with full states, transitions, invariants, evidence gates,
failure classes, and required evidence set.

Acceptance criteria:

- Candidate context is separate from trusted running instance. ✓
- Promotion readiness is distinct from task success. ✓
- Failure keeps the trusted control path intact. ✓

### NP-005: Add Plugin Evaluation Fixtures
**Status:** DONE
**Priority:** MEDIUM
**Surface:** `tests/fixtures/`, `tests/evals/`
**Completed:** 2026-04-29

Added the first repeatable fixture and eval definitions for read-only
comprehension and bounded documentation mutation, plus a local fixture checker
that validates expected evidence shape and runner gate behavior.

Acceptance criteria:

- Includes at least one read-only repo comprehension case.
- Includes at least one bounded docs mutation case.
- Includes expected output/evidence shape.

### NP-009: Wire Bounded Mutation Runner Integration
**Status:** DONE
**Priority:** HIGH
**Depends On:** NP-007, NP-008, NP-010
**Surface:** `workflows/`, runtime integration docs
**Completed:** 2026-04-29

Added a local bounded mutation runner harness that consumes a compiled workflow
contract, evaluates state transitions, checks cumulative evidence, enforces
validation gates, and blocks honestly on missing skills, scope, or evidence.

Acceptance criteria:

- Runner can consume `workflows/bounded-mutation.yaml` or an equivalent compiled
  representation.
- State transitions produce evidence shaped by the workflow contract.
- Validation gate enforcement is not optional for mutation tasks.
- Missing skills, scope, or evidence block honestly instead of pretending
  execution worked.

### NP-010: Create Task Normalize Skill Contract
**Status:** DONE
**Priority:** HIGH
**Surface:** `skills/task-normalize/`
**Completed:** 2026-04-28

Created executable contracts for turning raw programming requests into scoped
execution targets.

Acceptance criteria:

- Inputs preserve raw task content, source, repo hints, and acceptance hints.
- Outputs include task class, mutation intent, acceptance target, repo binding
  requirements, likely affected areas, validation hints, and ambiguity notes.
- Ambiguous or unsafe scopes produce blocked output rather than guessed
  execution targets.

### NP-008: Create Git Lifecycle Skill Contract
**Status:** DONE
**Priority:** HIGH
**Surface:** `skills/git-lifecycle/`
**Completed:** 2026-04-28

Created executable contracts for review-ready repository lifecycle output.

Acceptance criteria:

- Interfaces cover status inspection, branch creation, commit creation, and
  review handoff preparation.
- Remote push and PR creation remain blocked or confirmation-gated.
- Outputs include branch, commit, changed files, and remote action flags.

### NP-011: Add Governed Remote Review Actions
**Status:** DONE
**Priority:** MEDIUM
**Surface:** `skills/remote-review/`
**Completed:** 2026-05-08

Added a separate high-risk skill for branch push and PR creation so remote
review actions stay confirmation-gated without forcing confirmation onto local
branch/commit work.

Acceptance criteria:

- Plugin can push or create PR only through a governed confirmation path.
- Missing confirmation produces a clean blocked result instead of false success.
- Review surface includes validation summary and remote action context.

### NP-007: Create Mutation Skill Contracts
**Status:** DONE
**Priority:** HIGH
**Surface:** `skills/file-mutate/`, `skills/patch-apply/`
**Completed:** 2026-04-28
**Follow-up Verification:** 2026-05-25 (`OMN-240` closeout)

Created executable contracts for scoped file mutation and patch application.
Direct Python regression tests now cover the create/write/preview/apply
interfaces plus representative precondition and anchor-failure paths so the
mutation surface is verified directly instead of only through runner fixtures.

Acceptance criteria:

- Mutation inputs require root, path or patch target, expected preconditions,
  and scope boundaries.
- Outputs include changed files, action type, diff summary, and failure class.
- Path escape and hidden mutation protections are explicit.
- Side effects, reversibility, and confirmation rules are documented.

### NP-004: Define Bounded Mutation Workflow Contract
**Status:** DONE
**Priority:** HIGH
**Surface:** `workflows/bounded-mutation.yaml`, `docs/specs/`
**Completed:** 2026-04-28

Turned the bounded mutation workflow placeholder into a contract detailed enough
for runtime integration.

Acceptance criteria:

- States map to the task execution spec.
- Required evidence is explicit.
- Blocked, partial success, and failed outcomes are represented.
- Validation cannot be skipped silently for mutation tasks.
- Current executable skills and planned skill dependencies are identified.

### NP-003: Create Validation Skill Contract
**Status:** DONE
**Priority:** HIGH
**Surface:** `skills/run-validation/`
**Completed:** 2026-04-28

Created the first concrete validation skill contract for bounded build, test,
lint, and static validation command execution.

Acceptance criteria:

- Inputs include command, working directory, timeout, environment allowlist, and
  sandbox profile hint.
- Outputs include stdout, stderr, exit code, duration, verdict, and truncation
  flags.
- Governance and sandbox expectations are explicit.
- A `record_not_run` interface records validation exemptions or blockers.

### NP-002: Create Executable Repo Inspect Skill Contract
**Status:** DONE
**Priority:** HIGH
**Surface:** `skills/repo-inspect/`
**Completed:** 2026-04-28

Created the first concrete executable skill contract for read-only repository
inspection.

Acceptance criteria:

- Skill exposes narrow interfaces for listing, reading, searching, and repo
  status inspection.
- Side effects are explicitly read-only.
- Workspace/repo scope expectations are documented.
- Handler dependency on NAVI core is limited to the standard
  `subprocess_python` `function: run` transport.

### NP-001: Define Local Plugin Discovery and Sync Path
**Status:** DONE
**Priority:** CRITICAL
**Surface:** `docs/runbooks/`, `plugin.yaml`, local development docs
**Completed:** 2026-04-28

Defined how a developer uses this source package with NAVI core before NAVI
Store exists.

Acceptance criteria:

- Document where NAVI core expects local plugins.
- Document whether development uses symlink, copy/sync, or configured plugin
  root.
- Document how to verify `navi.programmer` is visible to NAVI.
- Do not require changing NAVI core unless the current loader cannot support a
  reasonable dev path.

### NP-000: Scaffold Plugin Project
**Status:** DONE
**Priority:** CRITICAL
**Surface:** package root, docs, skills, workflows, tests

Created the initial plugin package and migrated plugin-owned NAVI Programmer docs
out of NAVI core.
