# run-validation

Bounded validation command runner for NAVI Programmer.

This skill captures validation evidence for mutative programming work. It can
run build, test, lint, typecheck, static, or smoke commands inside the
project's configured NAVI core sandbox profile, and it can also record an
explicit `not_run` result when validation is blocked or not applicable.

## Interfaces

| Interface | Purpose |
| --- | --- |
| `run_command` | Execute an allowlisted validation command without shell expansion and capture stdout, stderr, exit code, duration, and verdict. |
| `record_not_run` | Produce structured validation evidence when a validation path is intentionally skipped or blocked. |

## Command Policy

- Commands run with `shell: false`; shell operators such as `|`, `&&`, `;`, and
  redirection tokens are rejected.
- The executable must be a named validation tool on the allowlist.
- Shells, Git, SSH, and `npx` are rejected by default.
- Python `-c` and Node inline evaluation flags are rejected.
- Python must use `-m` with an allowlisted validation module such as
  `pytest`, `unittest`, `compileall`, `mypy`, or `ruff`.
- Raw `node` execution is limited to the built-in `--test` runner.
- `go` is limited to validation-oriented actions such as `build`, `test`, and
  `vet`; `cargo` is limited to `build`, `check`, `clippy`, `fmt`, and `test`.
- Package-manager install, publish, update, audit, and dependency mutation
  actions are rejected.
- `make` requires an explicit validation target such as `test`, `lint`,
  `build`, `check`, or `validate`.
- Environment variables are minimal by default. Callers may pass explicit
  non-secret variables or allowlist non-secret host variables by name, but the
  selected sandbox profile must also allow each variable.

## Scope Rules

- `root` defaults to `NAVI_WORKSPACE_DIR`, then the configured NAVI workspace,
  then the skill process cwd.
- `cwd` defaults to `root`.
- Absolute `cwd` values are allowed only when they resolve inside `root`.
- In a running NAVI instance, `run_command` uses an internal handler backed by
  `internal/sandbox.Runner`; the Python entrypoint remains as a standalone
  local harness for plugin tests and direct fixture work.
- A missing, inactive, or unavailable sandbox profile produces structured
  `not_run` validation evidence instead of pretending the command ran.

## Verdicts

| Verdict | Meaning |
| --- | --- |
| `passed` | Command exited with an expected exit code and evidence was captured cleanly. |
| `failed` | Command completed with an unexpected exit code. |
| `timed_out` | Command exceeded `timeout_ms`. |
| `not_run` | Validation was intentionally skipped, blocked, or dry-run only. |
| `ambiguous` | Command exited as expected but captured output was truncated, so the evidence is incomplete. |
