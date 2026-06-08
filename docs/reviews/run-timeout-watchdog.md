# Run Timeout Watchdog Note

## Problem

The inner LLM loop can return a timeout fallback when its own request watchdog
fires, but that alone is not enough to protect a session if the broader
`ExecuteRun(...)` call hangs and never returns to the coordinator.

If that happens, the coordinator can keep the session marked as active, which
leads to user-facing symptoms such as:

- a placeholder that never gets finalized
- no `run.failed` event for connector surfaces to render
- later messages in the same session appearing dropped because the foreground
  run never clears

## Fix

The run coordinator now supports a coordinator-level run watchdog.

- `RunCoordinator.SetRunTimeout(...)` defines the maximum wall-clock lifetime
  for one foreground run.
- If the executor does not return before that deadline, the coordinator cancels
  the run context, marks the run as failed, emits `run.failed`, and wakes the
  session so the next pending inbox item can proceed.

## Current Wiring

`internal/navi/navi.go` now sets the coordinator watchdog to:

- `llm_call_timeout + 30s`

This keeps the coordinator timeout slightly above the inner LLM timeout so
normal timeout fallbacks still win first, while hung runs are still cleaned up
deterministically.
