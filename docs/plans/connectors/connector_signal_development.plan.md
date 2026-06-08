---
name: Signal Connector Development
overview: Implementation plan for a Signal connector in NAVI using a bridge-based approach (signal-cli REST or similar) for text and basic media messaging.
todos: []
isProject: false
---

# Signal Connector – Development Plan

---

## 1. MVP Scope

### 1.1 Supported Scenarios (MVP)

- **Inbound**:
  - 1:1 and group messages received via the Signal bridge (e.g. signal-cli REST).
  - Focus on text messages; media optional if bridge support is straightforward.
- **Outbound**:
  - Replies and new messages to existing conversations.
- **Session semantics**:
  - Map Signal sender (phone number / group ID) to NAVI sessions.

### 1.2 Explicit Non-Goals (v2+)

- Multi-device or multi-account advanced features.
- Complex attachment and sticker handling.
- Deep integration with Signal profiles or contact management.

---

## 2. File and Package Structure

Place Signal code under `connectors/signal/`.

Planned files:

```text
connectors/
  signal/
    bot.go           # Connector implementation, lifecycle, bridge calls
    bridge.go        # Thin client for signal-cli/signald REST/socket API
    mapping.go       # Signal↔NAVI mapping helpers
    config.go        # Signal-specific config helpers (optional)
    bot_test.go      # Unit tests (mapping, lifecycle, bridge error handling)
```

---

## 3. Config Schema and Settings

### 3.1 Config Struct

In `internal/config/config.go`, add:

- `SignalConfig`:
  - `Enabled bool`
  - `BridgeURL string` – base URL to signal-cli or other bridge.
  - `PhoneNumber string` – phone number for the Signal account.
  - `AuthToken string` – optional auth for bridge (if applicable).
  - `GatewayURL string`, `GatewaySecret string` – same pattern as other connectors.

Add `Signal SignalConfig` on `Config` with YAML tag `signal`.

### 3.2 Storage

- MVP: read bridge URL, phone number, and auth from config/env.
- v2: allow updating and persisting via `internal/store` with gateway-driven onboarding.

---

## 4. Connector Implementation

### 4.1 Struct and Interfaces

In `bot.go`:

- `Bot` struct:
  - Holds config, HTTP (or socket) client to the bridge.
  - Running flag and optional `BaseConnector` embedding.
- Implement `connectors.Connector`:
  - `Name()` – `"signal"`.
  - `Start(ctx)` – connect/verify with bridge and spawn inbound polling/stream loops.
  - `Stop(ctx)` – stop loops and mark not running.
  - `Send(ctx, msg OutboundMessage)` – call bridge send API.
  - `IsRunning()` – concurrency-safe flag.

Optional interfaces for MVP:

- `MessageLengthProvider` – max text length, if bridge/Signal enforces a limit.

### 4.2 Inbound Message Flow

- Design depending on bridge capability:
  - Webhook mode (if bridge supports callbacks), or
  - Polling/stream mode where connector regularly fetches messages from bridge.
- Normalize messages in `mapping.go`:
  - Extract sender phone, group IDs, message IDs, timestamps, and text.
  - Map to NAVI inbound messages and send them through the gateway.

### 4.3 Outbound Message Flow

- Translate `OutboundMessage` into bridge send payload:
  - Choose `recipient` (phone number or group).
  - Text only for MVP; media attachments are deferred unless trivial.
- Map bridge responses to error types:
  - Nonexistent recipients, invalid phone numbers → permanent errors.
  - Temporary network errors → transient, retryable errors.

---

## 5. Integration Points

- `internal/connectors/registry.go`:
  - Register factory: `"signal"` → `Bot` creation.
- `cmd/navid/main.go`:
  - Register and create the Signal connector when `cfg.Signal.Enabled` is true and config is valid.
- `internal/connectors/manager.go`:
  - Leverage existing retry and message-splitting systems as needed.

---

## 6. Capabilities Matrix (MVP vs v2)

| Capability          | MVP | v2 / Future |
|---------------------|-----|-------------|
| 1:1 messages        | ✅  | –           |
| Group messages      | ✅  | –           |
| Media attachments   | ⚪  | ✅          |
| Read receipts       | ❌  | ⚪          |
| Multi-account       | ❌  | ✅          |
| Rich reactions      | ❌  | ⚪          |

---

## 7. Dependencies and Bridge Choice

- **Preferred**:
  - signal-cli REST API, if stable and well-documented.
- Alternative:
  - signald or other bridges if they provide better reliability.

Plan:

- Encapsulate bridge-specific details in `bridge.go` so switching implementations is low-friction.

---

## 8. v2 / Future Enhancements

- Support for additional message types (attachments, stickers).
- More advanced group and contact management.
- Better mapping of Signal-specific features (disappearing messages, reactions).
- Hardening multi-account and multi-device workflows.

---

## 9. Acceptance Criteria

MVP is complete when:

1. NAVI can send and receive basic Signal text and group messages via the chosen bridge.
2. The connector’s lifecycle integrates with registry and Manager as expected.
3. Bridge failures and errors are handled gracefully and surfaced via logs/diagnostics.
4. Unit tests cover mapping and error handling for key bridge operations.

