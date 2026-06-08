---
name: WhatsApp Connector Development
overview: Implementation plan for a WhatsApp connector in NAVI, starting with a Meta Cloud API webhook-based MVP and leaving room for bridge/native modes later.
todos: []
isProject: false
---

# WhatsApp Connector – Development Plan

---

## 1. MVP Scope

### 1.1 Supported Scenarios (MVP)

- **Inbound**:
  - Messages received via WhatsApp Business Cloud API webhooks.
  - Supported types: text messages and basic media (images/documents).
  - Conversation context: per phone number and optional conversation ID.
- **Outbound**:
  - Text replies to inbound conversations.
  - Media attachments where the API makes this straightforward.
- **Session semantics**:
  - Map a WhatsApp user (phone number) and conversation to a NAVI session.

### 1.2 Explicit Non-Goals (v2+)

- Template creation and advanced template management.
- Interactive messages (quick replies, list messages, buttons) beyond simple text.
- Multiple transport modes (browser-based Web bridge, native APIs) — treated as future extensions.

---

## 2. File and Package Structure

All WhatsApp code will live in `connectors/whatsapp/`.

Proposed structure:

```text
connectors/
  whatsapp/
    bot.go           # Core connector implementation (Cloud API mode)
    webhook.go       # Webhook handler and verification logic
    mapping.go       # WhatsApp↔NAVI message mapping helpers
    config.go        # WhatsApp-specific config helpers (optional)
    bot_test.go      # Unit tests (lifecycle, mapping, webhook verification)
```

Key ideas:

- Keep the connector struct and lifecycle in `bot.go`.
- Put all webhook-specific HTTP logic and signature verification in `webhook.go`.
- Centralize field mapping between WhatsApp payloads and NAVI messages in `mapping.go`.

---

## 3. Config Schema and Settings

### 3.1 Config Struct

In `internal/config/config.go`, add:

- `WhatsAppConfig` with at least:
  - `Enabled bool` – feature flag for the connector.
  - `CloudAPIBaseURL string` – base API URL (default to Meta endpoint).
  - `AccessToken string` – Cloud API auth token (or token alias for settings).
  - `PhoneNumberID string` – WhatsApp phone number ID used by the API.
  - `VerifyToken string` – shared secret for webhook verification.
  - `GatewayURL string` – NAVI gateway base URL.
  - `GatewaySecret string` – shared secret for NAVI connector registration and tokens.
  - Optional: `AllowedPhoneNumbers []string` – basic allowlist.

Add `WhatsApp WhatsAppConfig` on `Config` with YAML tag `whatsapp`.

### 3.2 Secrets and Persistence

- MVP:
  - Allow tokens to be read from config/environment.
- v2:
  - Persist tokens and other secrets in `internal/store` (similar to Telegram and Slack).
  - Add onboarding flows in the gateway for updating tokens safely.

---

## 4. Connector Implementation

### 4.1 Struct and Interfaces

In `connectors/whatsapp/bot.go`:

- Define a `Bot` struct that:
  - Stores config, HTTP client, and running state.
  - Optionally embeds `BaseConnector` for allowlist logic.
- Implement `connectors.Connector`:
  - `Name()` – `"whatsapp"`.
  - `Start(ctx)` – registers with the gateway, ensures webhook is ready, marks running.
  - `Stop(ctx)` – clean shutdown, mark not running.
  - `Send(ctx, msg OutboundMessage)` – call Cloud API `/messages` endpoint.
  - `IsRunning()` – thread-safe running flag.

Optional interfaces for MVP:

- `WebhookHandler`:
  - `WebhookPath()` – e.g. `/webhooks/whatsapp`.
  - `HandleWebhook(w, r)` – validate signature/verify token, parse payload, and forward inbound messages.
- `MessageLengthProvider`:
  - Returns maximum message length, based on WhatsApp’s documented limit.

### 4.2 Lifecycle Flow

On `Start(ctx)`:

- Validate config: ensure required fields (`AccessToken`, `PhoneNumberID`, `VerifyToken`) are present.
- Register with the NAVI gateway, following the same pattern as Telegram/Slack.
- Optionally verify webhook endpoint with WhatsApp Cloud API (if configured to do so).

On `Stop(ctx)`:

- Set running flag to false.
- Rely on Manager for worker shutdown; ensure any internal goroutines exit.

---

## 5. Webhook and Message Handling

### 5.1 Webhook Entry

In `webhook.go`:

- Implement:
  - `WebhookPath()` to return a static path (e.g. `/webhooks/whatsapp`).
  - `HandleWebhook(w, r)` to:
    - Validate method and any signature/verification token.
    - For verification requests, echo challenge as required by Cloud API.
    - Parse JSON payload for message events.

### 5.2 Inbound Mapping

In `mapping.go`:

- Map WhatsApp webhook payloads to NAVI inbound representation:
  - Extract sender phone number, message ID, timestamp, and text/media.
  - Derive a stable session key (e.g. phone number + conversation ID if provided).
  - Normalize media references (URLs or IDs) for later retrieval if we choose to support media in v2.
- Forward normalized messages to NAVI gateway endpoints, mirroring the patterns used in Telegram/Slack connectors.

### 5.3 Outbound Send

In `Send(ctx, msg)`:

- Convert `OutboundMessage` into Cloud API payload:
  - Determine recipient phone number from `msg.ChatID` or metadata.
  - Use the `text` or `media` payload formats as documented by the Cloud API.
- Handle responses:
  - Map HTTP status codes to connector error types (temporary vs permanent).
  - Respect any rate limit signals for future v2 handling.

---

## 6. Integration Points

### 6.1 Registry and Manager

- In `internal/connectors/registry.go`:
  - Register the WhatsApp factory: `RegisterFactory("whatsapp", func(cfg *config.Config, b bus.Bus) (connectors.Connector, error) { ... })`.
- Ensure `internal/connectors/manager.go`:
  - Detects `WebhookHandler` and wires up `WebhookPath` correctly in the gateway HTTP server.
  - Uses `MessageLengthProvider` for splitting long replies.

### 6.2 Main Wiring

- In `cmd/navid/main.go`:
  - When `cfg.WhatsApp.Enabled` is true (and necessary config is present), register the WhatsApp factory.
  - Either:
    - Create and start the WhatsApp connector eagerly at startup, or
    - Expose a setup/onboarding endpoint that calls `startConnector("whatsapp")` when configuration is complete.

---

## 7. Capabilities Matrix (MVP vs v2)

| Capability                        | MVP | v2 / Future |
|-----------------------------------|-----|-------------|
| 1:1 conversations (text)         | ✅  | –           |
| Group conversations (text)       | ⚪  | ✅          |
| Media (images/documents)         | ⚪  | ✅          |
| Templates                        | ❌  | ✅          |
| Interactive messages             | ❌  | ✅          |
| Read receipts / delivery status  | ❌  | ✅          |
| Multiple numbers / accounts      | ❌  | ✅          |

Legend: ✅ = in-scope, ⚪ = optional, ❌ = out-of-scope for MVP.

---

## 8. Dependencies and External Services

- **External**:
  - WhatsApp Business Cloud API (Meta).
- **Go-level**:
  - Prefer `net/http` for API calls, with minimal helper wrappers.
  - Consider a small internal client package in `connectors/whatsapp` for Cloud API calls to keep `bot.go` small.

Constraints:

- Avoid tight coupling to any third-party libraries that might lag WhatsApp API updates.
- Keep unit tests runnable without live API access by mocking HTTP calls.

---

## 9. v2 / Future Enhancements

Potential expansions:

- Add support for **templates** and **interactive messages** to improve UX.
- Implement more advanced **session management** (multi-number, multi-tenant scenarios).
- Add **media download** paths and attachments bridging into NAVI’s internal tools.
- Introduce alternative transport modes:
  - Integrate with existing browser-based Web bridges or self-hosted gateways.
  - Support local/native connectors similar to PicoClaw’s `whatsapp_native`.

---

## 10. Acceptance Criteria

The WhatsApp connector MVP is considered implemented when:

1. NAVI can start a WhatsApp connector with Cloud API configuration present.
2. Webhooks from WhatsApp Business Cloud API are accepted, verified, and transformed into NAVI inbound messages.
3. NAVI can send text replies back to WhatsApp reliably for basic conversations.
4. Basic unit tests cover mapping, webhook verification paths, and send error handling.
5. The connector integrates cleanly with the registry, Manager, and gateway, without special-casing in core logic.

