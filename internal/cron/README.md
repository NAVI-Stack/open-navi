# internal/cron

Persistent scheduler for NAVI (OpenClaw-inspired). Jobs live in SQLite `cron_jobs` and are executed by a single dynamically re-armed timer with **min refire gap** and **max delay** guards, a small concurrent run pool, per-job timeouts, transient-error backoff on one-shots, and startup catch-up with staggered deferral for overflow.

## Schedule shapes (`schedule_kind`)

| Kind | Fields | Notes |
|------|--------|------|
| `cron` | `schedule_expr` (full 5-field + optional descriptor), optional `schedule_tz` IANA name, optional `schedule_stagger_ms` | Stagger shifts each fire deterministically by `sha256(job_id) % stagger`. |
| `at` | RFC3339/RFC3339Nano in `schedule_expr` column (also `at` field in skills) | Runs once in the future (`computeNextInstantPlain` requires strictly after `now` at creation time). |
| `every` | `schedule_every_ms`, optional `schedule_anchor_ms`, optional `schedule_tz` | Fixed-step next instant from anchor / wall clock. |

## Execution model

- **Main session**: `session_target=main` with `payload_kind=systemEvent` creates an ACT directive plus owner message with `payload_text`. `wake_mode=now` calls heartbeat `RequestWake` (coalesced) with interval-class priority; `next-heartbeat` skips the immediate wake.
- **Isolated / detached**: stubbed — returns skipped until Phase 16 wiring exists.

## Errors & backoff

- Strings are classified via regex buckets (`rate_limit`, `overloaded`, `network`, `timeout`, `5xx`) for **transient** retries.
- One-shot (`at`) transient failures apply `Retry.BackoffMs` up to `Retry.MaxAttempts`, then disable with `next_run` cleared.
- Recurring jobs push the computed next schedule later if the last run errored (backstop ≥ backoff table).

## Failure alerts

When `consecutive_errors` reaches configured `after` and `last_failure_alert_at_ms` is older than `cooldown_ms`, the service invokes `Deps.OnFailureAlert` or logs — per-job JSON overrides global `config.Cron.failure_alert`.

## Follow-ups (not implemented)

- Busy-lane skip / automatic retry of wake-now when ICS blocks the heartbeat session
- Active-hours gate separate from heartbeat quiet hours
- Phase-based heartbeat staggering per agent hash
- Delivery routing using `delivery_json` (channel/account/thread)
