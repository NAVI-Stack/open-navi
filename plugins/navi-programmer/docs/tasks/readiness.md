# Readiness

**Status:** Active
**Last Updated:** 2026-05-16

## Summary

NAVI Programmer has a coherent NAVI-first prototype harness shape: seven
prototype executable skills, aligned manifest skill IDs, bounded mutation
workflow contracts, and a local runner that can inspect contracts, start a run,
record skill-result evidence, evaluate transitions, and synthesize a result
envelope. NAVI core now also has first-pass sandbox profile persistence,
project readiness gating, sandbox profile API routes, and a local Docker runner
surface for bounded command execution. `run-validation` now routes through a
NAVI internal handler backed by that core sandbox runner in a running NAVI
instance. It is still not ready for unattended end-to-end autonomous programming
tasks because full task orchestration does not yet feed all skill results
through the bounded mutation runner automatically.

## Snapshot

| Area | Status | Notes |
| --- | --- | --- |
| Plugin package | Present | Package root, manifest, docs, skills, workflows, and tests are scaffolded. |
| Plugin docs | Present | Plugin-owned docs were migrated into this package. |
| Project workflow | Present | `PROJECT_STATE.md`, `TASK_QUEUE.md`, `NEXT_ACTION.md`, and `AGENTS.md` exist. |
| Local discovery path | Live-verified | NP-001 documented the paths; Docker Compose NaviD now live-loads `navi.programmer` from the rebuilt repo image and exposes it through `/api/plugins` and `/api/skills`. |
| Executable skills | Present | `task-normalize`, `repo-inspect`, `file-mutate`, `patch-apply`, `git-lifecycle`, and `remote-review` are prototype executable `subprocess_python` skills; `run-validation` uses a NAVI internal handler backed by the core sandbox runner. |
| Skill identity | Reconciled | Manifest component `skill_id` values match each `SKILL.yaml` and use `navi-programmer.<folder-name>`. |
| Workflow contract | Present | `bounded-mutation.yaml` defines states, evidence, gates, and outcome semantics. |
| Workflow runner integration | Present | `workflows/bounded_mutation_runner.py` consumes a compiled bounded mutation contract, enforces evidence/skill/governance/validation gates, records skill-step evidence, and synthesizes a normalized result envelope. |
| Core sandbox profiles | Present | NAVI core can store active Docker sandbox profiles and expose them through `/api/sandbox-profiles`; coding project readiness requires a bound workspace and active sandbox profile. |
| Core Docker runner | Prototype | `internal/sandbox` defines `Prepare`, `Run`, and `Cleanup` with network-off Docker args, mount scoping, timeouts, command allowlists, and env allowlists. |
| Sandbox-backed validation | Live in default Compose | `navi-programmer.run-validation` resolves sandbox profiles through NAVI core, executes through `internal/sandbox.Runner`, and in the supported default `compose.yml` runtime uses `docker exec` against the running `navid` container when `NAVI_SANDBOX_CONTAINER_NAME` is configured. Structured `not_run` blockers remain intact for unavailable sandbox paths, including strict mode. |
| Evaluation fixtures | Starter | Read-only comprehension and bounded docs mutation fixtures exist with a local checker. |
| Self-update safety execution | Present | Self-update fixtures and runner evidence now capture candidate runtime smoke results, synthesize promotion readiness, and distinguish runtime-verified candidates from merely locally-valid ones. |

## Readiness Verdict

- Design readiness: strong enough to begin.
- Project organization readiness: sufficient after Phase 0 setup.
- Runtime execution readiness: partial for normalization, repo inspection,
  mutation, patching, validation, local review handoff, remote-review proposal
  blocking, workflow transition checks, local evidence/result shaping, and core
  sandbox profile readiness gates.
- Autonomous coding readiness: not yet.

## Next Readiness Gate

NAVI Programmer becomes locally coding-ready when NAVI core feeds real skill
results through the bounded mutation runner without manual stitching and eval
coverage grows beyond the starter set.
