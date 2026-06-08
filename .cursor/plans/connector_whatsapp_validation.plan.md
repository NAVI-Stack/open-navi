---
name: WhatsApp Connector Validation
overview: Testing, security, and documentation plan to harden the WhatsApp connector and ensure safe operation with WhatsApp Business Cloud API (and future transports).
todos: []
isProject: false
---

# WhatsApp Connector – Validation and Documentation Plan

---

## 1. Testing Strategy

### 1.1 Unit Tests

Focus:

- **Webhook handling**:
  - Verification of webhook `hub.verify_token` (or equivalent) handshake.
  - Parsing of inbound message payloads, including edge cases (multiple entries, statuses).
- **Message mapping**:
  - Mapping from WhatsApp JSON payloads to NAVI inbound structures.
  - Mapping from `OutboundMessage` to Cloud API request payloads.
- **Error classification**:
  - Handling of HTTP statuses (2xx, 4xx, 5xx).
  - Classification into permanent vs transient errors.

### 1.2 Integration Tests

Goals:

- Validate end-to-end integration between NAVI and a fake or sandboxed WhatsApp API:
  - Simulate webhook requests into the connector and observe NAVI gateway calls.
  - Mock outbound HTTP requests and assert payload structure and headers.

Approach:

- Use an in-process HTTP server to mimic WhatsApp’s webhook and Cloud API endpoints.
- Use environment-controlled configuration for integration tests (test tokens, phone number IDs).

### 1.3 Manual / E2E Scenarios

Document manual test scenarios (for a real or sandbox WhatsApp Business account):

- Send a text message from a real phone to the configured business account.
- Confirm NAVI receives the inbound message and sends a reply.
- Test error paths:
  - Invalid verify token.
  - Expired access token.

---

## 2. Security and Threat Analysis

### 2.1 Secrets and Credentials

- **Access tokens**:
  - Must never be logged or exposed in plain text.
  - Should be rotated according to Meta’s recommendations.
- **Verify token**:
  - Treat as a secret; ensure it is not logged or echoed.

Validation steps:

- Scan logs (automated or manual) to confirm secrets are redacted.
- Review error-handling code paths to ensure they do not leak sensitive data.

### 2.2 Webhook Security

Depending on Cloud API configuration:

- Confirm that webhook verification logic strictly enforces:
  - Correct `verify_token`.
  - Correct handling of verification challenges.
- Plan for optional enhancements:
  - If WhatsApp offers signed webhooks, design where and how to validate signatures.

Threats:

- Spoofed webhooks (attackers sending fake HTTP requests).
- Replay attacks where valid payloads are replayed.

Mitigations:

- Require correct verification token.
- Optionally track message IDs to detect duplicates in sensitive flows.

---

## 3. Reliability and Rate Limiting

### 3.1 Rate Limit Behavior

- Understand Cloud API rate limit policies and error codes.
- Validation tasks:
  - Ensure retry behavior is conservative and does not create thundering herds.
  - Confirm we treat certain 4xx statuses as permanent failures (no retries).

### 3.2 Failure Modes

Identify and test:

- Network errors when calling Cloud API.
- Timeouts and partial responses.
- Webhook delivery delays or gaps.

For each, ensure:

- Clear logging and diagnostics.
- No data corruption or inconsistent state in NAVI.

---

## 4. Data Protection and Privacy

### 4.1 Message and Metadata Handling

- Decide what content is logged (prefer metadata over full message content).
- Ensure phone numbers, names, and other identifiers are handled as sensitive.

Validation:

- Review logging and telemetry code for potential PII leakage.
- Document retention assumptions and configurable retention points.

### 4.2 Compliance Considerations

- WhatsApp policies:
  - Do not use data beyond allowed use cases.
  - Respect opt-in and opt-out regimes.

Plan:

- Capture policy-relevant constraints in user-facing docs.
- Provide configuration options for limiting logging and data retention.

---

## 5. Documentation Plan

### 5.1 User Documentation

Add or extend docs in `docs/` to cover:

- **Setup Guide**:
  - Steps to create a WhatsApp Business Cloud API app.
  - How to configure webhook URL and verify token.
  - How to obtain and configure access tokens and phone number IDs.
- **Usage Guide**:
  - How users interact with NAVI over WhatsApp (supported message types).
  - Known limitations (e.g. no templates or interactive messages in MVP).

### 5.2 Technical Documentation

- **Architecture**:
  - Request/response diagrams showing WhatsApp → NAVI → WhatsApp flow.
- **Component Breakdown**:
  - Explanation of `connectors/whatsapp` files (bot, webhook, mapping).
  - Description of how the connector integrates with the registry and Manager.

### 5.3 Operational Notes

Cover:

- How to enable/disable the connector via config.
- How to rotate access tokens without downtime.
- How to validate that webhooks are configured correctly.
- What to check when messages are not flowing (diagnostic procedures).

---

## 6. Validation Checklist

Before declaring the WhatsApp connector hardened:

- [ ] Unit tests cover webhook verification, payload parsing, and send logic.
- [ ] Integration tests validate round-trips through a fake API/server.
- [ ] Secrets (access tokens, verify tokens) are never logged or exposed.
- [ ] Behavior under rate limits and network failures has been tested and documented.
- [ ] User and technical documentation is complete and up to date.
- [ ] A clear, low-friction procedure exists to disable the connector or rollback if needed.

