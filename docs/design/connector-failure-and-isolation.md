## Connector Failure and Isolation (Implementation Notes)

This document narrows the conceptual Failure Model and Isolation Boundaries for connectors to the behavior implemented in Go code today. It complements the canonical conceptual design in `docs/canonical/conceptual-design-overview.md` without changing its intent.

### Goals

- **Respect Failure Model semantics**: treat connector unavailability as a first-class failure (`ConnectorUnavailable`) with explicit retry and degradation behavior.
- **Prevent cascading failures**: avoid hammering failing external systems and propagating connector errors into the Cognitive Layer.
- **Surface degradation**: expose connector health and availability to the rest of the system instead of silently dropping capabilities.

### Implementation Summary

- All outbound connector traffic flows through `internal/connectors/Manager` and `connectorWorker` instances.
- Each worker applies **per-connector rate limiting**, **message splitting**, and **classified retries** based on `connectors.Err*` values.
- The Manager tracks a **per-connector circuit-breaker** that opens after repeated failures and short-circuits new sends until a cool-down window elapses.
- Health and degradation are reflected via `ConnectorHealth` and v2 `InstanceMetadata`, not by silently ignoring failures.

### Classified Errors and Retry Budget

Connector implementations classify `Send` failures using errors from `connectors/errors.go`:

- `ErrRateLimit`: transient rate-limit; safe to retry after a fixed delay.
- `ErrTemporary`: transient operational error; safe to retry with exponential backoff.
- `ErrNotRunning` / `ErrSendFailed`: permanent failures for the current instance; not retried by the worker.

The `connectorWorker.sendWithRetry` loop applies a **per-message retry budget**:

- Up to 3 retries per logical send.
- Fixed delay for `ErrRateLimit` and exponential backoff for `ErrTemporary` and unknown errors, capped at a maximum backoff.
- On success, an execution outcome with `Outcome = succeeded` is recorded when `SaveExecutionOutcome` is configured.
- On exhaustion, an execution outcome with `Outcome = failed` and `FailureReason = "ConnectorUnavailable"`-style text is recorded and diagnostic hooks fire.

This matches the conceptual requirement that **transient connector failures retry within a bounded budget**, while **permanent failures do not spin indefinitely**.

### Circuit-Breaker Semantics

The Manager maintains **per-connector circuit state** keyed by connector name:

- **Closed** (normal): all sends are allowed; failure count is 0.
- **Open**: Dispatch short-circuits immediately for the connector; new sends fail fast.
- **Half-open**: a single probe is allowed after a cool-down window to test recovery.

State transitions:

1. **Closed → Open**
   - Triggered when either:
     - A **permanent failure** is observed (`ErrNotRunning`, `ErrSendFailed`, or `ErrUnknownConnector`), or
     - The number of consecutive send failures that exhausted the per-message retry budget reaches the configured threshold (currently 5).
   - The Manager records the error message and timestamp; `InstanceMetadata.HealthState` is updated to `"down"`.
2. **Open → Half-open**
   - After the cool-down window elapses (currently 30 seconds), the next call to `Dispatch` transitions the circuit to half-open and allows a single probe message through.
3. **Half-open → Closed**
   - If the probe send **succeeds**, the Manager resets the failure count and returns the circuit to closed state.
4. **Half-open → Open**
   - If the probe send **fails**, the Manager re-opens the circuit, records the new failure, and restarts the cool-down.

When the circuit is **open**, `Dispatch` returns `connectors.ErrNotRunning` to callers. This ensures higher layers see a clear **ConnectorUnavailable**-style failure rather than transient noise and can respond according to the Failure Model (e.g., advisory or blocking degradation).

### Degradation Visibility

Connector degradation is surfaced through:

- `Manager.Health()` → `ConnectorHealth` (status `healthy` / `degraded` / `down`).
- v2 `InstanceMetadata.HealthState` via `Registry.UpdateHealth`, which is updated whenever:
  - Health probes fail for `HealthChecker` connectors.
  - Circuit-breaker transitions open and records a new failure.

Callers that need to adjust behavior based on connector availability can read these surfaces and mark dependent capabilities as **degraded** rather than silently omitting them.

### Plugin Isolation Scope (Clarification)

Plugins execute **in-process** with the agent loop but obey the following isolation constraints:

- All plugin tool invocations are executed through `command.Executor` with **panic recovery** and **structured failure classification**.
- Plugin code **cannot write directly** to the World Model; it returns results that the Cognitive Layer may choose to apply.
- A plugin panic or failure results in a structured failure outcome, not a crash of the daemon or partial World Model mutation.

This satisfies the **isolation intent** of the conceptual design for the current phase: plugin failures are isolated to structured failure results and do not corrupt the Cognitive loop or World Model.

#### Documented deviation: in-process vs out-of-process

The canonical design (Tier 3 — Isolation Boundaries) states that *"Plugins execute in isolated processes."* The current implementation uses **in-process** execution with the safeguards above. This is a **formal deviation** accepted for the current phase.

**Acceptance criteria for in-process operation (current phase):**

- Panic recovery ensures no plugin panic terminates the daemon or blocks the Conscious loop.
- All plugin results flow through structured success/failure; no direct World Model writes from plugin code.
- Failed or panicking plugins yield structured failure outcomes and can be marked degraded; the Conscious Process treats degraded plugins as unavailable until recovery.
- Connector isolation (circuit-breaker, retry budget) is implemented independently; connector failures do not require process isolation.

**Conditions under which process isolation becomes required (future phase):**

- **Untrusted or third-party plugins**: when plugins are not vetted or run arbitrary code, out-of-process or sandboxed execution is required to prevent malicious or buggy code from affecting the daemon.
- **Resource or stability requirements**: when a plugin may consume unbounded CPU/memory or block indefinitely, process isolation (with timeouts and kill boundaries) becomes necessary to protect the Conscious loop.
- **Compliance or audit**: when a deployment must demonstrate that plugin failure cannot affect core process integrity, out-of-process execution with clear process boundaries is required.

Until one or more of these conditions apply, in-process execution with panic recovery and structured failure is the accepted implementation. A fully out-of-process plugin runtime remains a future extension; no implementation work is required for conceptual parity in the current phase.

