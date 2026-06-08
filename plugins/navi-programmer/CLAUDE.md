# CLAUDE.md - NAVI Programmer Plugin

## What This Package Is

NAVI Programmer is the NAVI-first programming capability plugin. It packages
repository inspection, bounded code mutation, validation execution, local git
lifecycle operations, confirmation-gated remote review, task normalization, and
workflow contracts for governed programming work on the NAVI repository.

The package is designed so NAVI core can eventually discover and invoke a
coherent programming harness. It does not own NAVI core runtime orchestration,
global governance, or connector infrastructure.

## Package Layout

```text
plugin.yaml                  # Plugin metadata, capabilities, components, workflow metadata
skills/                      # Seven atomic programming skill families
  repo-inspect/              # Read-only repository inspection, search, status, and diff inspection
  file-mutate/               # Whole-file creation/replacement with SHA256 preconditions
  patch-apply/               # Structured patch operations
  run-validation/            # Bounded build/test/lint execution with allowlists
  git-lifecycle/             # Local git status, branch, commit, and review handoff evidence
  remote-review/             # Confirmation-gated push and PR creation
  task-normalize/            # Raw task to structured execution target and scope binding
workflows/
  bounded-mutation.yaml      # Primary workflow contract
  bounded_mutation_runner.py # Local runner contract/evidence harness
  bounded-mutation.compiled.json
  self-update-candidate.yaml
  ticket-driven-coding.yaml
tests/
  fixtures/                  # Input/output test fixtures
  evals/                     # Eval harness
docs/                        # Concepts, design, specs, plans, decisions, runbooks
```

Each skill directory has `SKILL.yaml`, `main.py`, and `README.md`; runtime
transport may be `subprocess_python` or NAVI `internal` depending on whether
the skill needs direct core services.

## Identity Rules

- Plugin ID: `navi.programmer`
- Skill IDs: `navi-programmer.<skill-folder>`
- Workflow IDs: `navi.programmer.<workflow_name>`

Keep `plugin.yaml`, `skills/*/SKILL.yaml`, workflow contracts, fixtures, and
docs aligned with those runtime-facing IDs. Do not reintroduce mixed dotted and
underscored skill ID variants.

## Skills

Most skills communicate via JSON on stdin/stdout through `subprocess_python`.
`run-validation` uses NAVI `internal` transport so runtime command execution
goes through core sandbox profiles. Each skill defines input/output contracts
in `SKILL.yaml`.

| Skill | Side Effects | Risk Tier |
| --- | --- | --- |
| `navi-programmer.repo-inspect` | None | Lowest |
| `navi-programmer.task-normalize` | None | Lowest |
| `navi-programmer.file-mutate` | Writes files | Medium |
| `navi-programmer.patch-apply` | Writes files | Medium |
| `navi-programmer.run-validation` | Executes allowlisted commands | Medium |
| `navi-programmer.git-lifecycle` | Local git state | Medium |
| `navi-programmer.remote-review` | Remote push/PR creation after confirmation | High |

## Bounded Mutation Workflow

The primary execution model is:

```text
received -> normalized -> scope_bound -> inspecting -> planned -> executing -> validating -> synthesizing -> review_ready
```

Blocked and failed states are terminal escape paths.

Key invariants:

- Validation cannot be skipped after mutation without an explicit `not_run`
  reason.
- Path escape protection is always active.
- Ambiguous tasks block and request clarification.
- Self-update candidate work is classified explicitly and must use candidate
  context before promotion-worthy claims.
- Remote review is separate from local git lifecycle and is confirmation-gated.

The runner is `workflows/bounded_mutation_runner.py`. It does not invoke skills
directly today. It exposes `start_run`, `record_step`, `evaluate_transition`,
`inspect_contract`, and `synthesize_result` so NAVI core can later feed skill
result envelopes into a consistent evidence ledger and result envelope.

## Making Changes

### Skills (`skills/*/main.py`)

- Each skill is a standalone Python script callable as `python main.py < input.json`.
- Use stdlib only at execution time.
- Validate inputs strictly and return structured JSON errors on failure.
- Do not add side-effect categories without updating `SKILL.yaml`,
  `plugin.yaml`, workflow contracts, and relevant docs.

### Workflows (`workflows/`)

- Edit the `.yaml` source first.
- Keep `bounded-mutation.compiled.json` aligned when the runner consumes a
  compiled contract.
- State transitions, required evidence, and skill availability checks must
  remain aligned with real skill interface IDs.

### Tests (`tests/`)

- Fixtures are JSON files in `tests/fixtures/`.
- Run evals with `python tests/evals/run_eval_fixtures.py`.
- Add a fixture or focused unit test when changing a skill behavior, runner
  interface, manifest identity, or workflow edge.

## Design Decisions

ADRs live in `docs/decisions/`. Read these before changing architecture:

- **0001**: Skills ship as a source package, not a binary
- **0002**: Bounded validation runner with allowlisted commands
- **0003**: Preconditioned mutation skills
- **0004**: Local git lifecycle is separate from remote review
- **0005**: Conservative task normalization
- **0006**: Local workflow runner harness

## Project State

Current phase: **Phase 2 - Harness Reconciliation**. The seven prototype skill
families exist, the manifest and skill IDs are reconciled, the bounded mutation
runner exposes evidence/result interfaces, and docs are aligned around a
NAVI-first harness. The plugin is discoverable by manifest path, but it has not
been live-verified in a running NaviD instance in this session.

See `PROJECT_STATE.md`, `NEXT_ACTION.md`, and `TASK_QUEUE.md` for current
status and follow-up work.

## Key Constraints

- Never hide remote push or PR creation inside `git-lifecycle`; use
  `remote-review` with explicit confirmation.
- Never allow shell expansion or glob patterns in run-validation command
  allowlists.
- Mutation skills must reject writes outside the declared scope.
- task-normalize must block, not assume, when task scope is ambiguous.
- Validation gates in the bounded mutation workflow are mandatory.
- Do not broaden this package for external product-layer concerns during
  NAVI-first hardening.
