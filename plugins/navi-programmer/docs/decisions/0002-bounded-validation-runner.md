# 0002 - Bounded Validation Runner

**Status:** Accepted
**Date:** 2026-04-28
**Updated:** 2026-05-16

## Decision

NAVI Programmer validation starts with a plugin-owned `subprocess_python`
runner in `skills/run-validation/`.

The runner executes local validation commands with these constraints:

- no shell expansion
- allowlisted validation executables
- rejected shell, remote, and inline-eval command shapes
- minimal environment by default
- explicit non-secret environment allowlists
- scoped working directory inside the bound root
- structured `passed`, `failed`, `timed_out`, and `not_run` evidence

Full OS-level sandbox enforcement remains owned by NAVI core. As of
2026-05-12, the runtime `run-validation` skill uses an internal NAVI handler
backed by `internal/sandbox.Runner`; the Python entrypoint remains a
standalone local harness for plugin tests and fixture work.

As of 2026-05-16, the supported default `compose.yml` runtime executes bounded
validation commands through that same core runner by using `docker exec`
against the running `navid` container when `NAVI_SANDBOX_CONTAINER_NAME` is
configured and the Docker socket is mounted.

## Rationale

Validation is required for mutative programming tasks, but unrestricted command
execution would turn NAVI Programmer into an open shell. The first validation
skill needs to be useful enough to prove the task lifecycle while still narrow
enough for governed automation.

## Consequences

- Build, test, lint, typecheck, static, and smoke checks can produce structured
  evidence immediately.
- Validation commands that require shell pipelines or arbitrary inline code must
  be represented through safer project scripts or future policy extensions.
- Missing or inapplicable validation must be reported through `record_not_run`
  instead of being omitted silently.
- NAVI core sandbox work hardened the same skill contract without changing the
  evidence shape.
