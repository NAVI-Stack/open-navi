---
name: Discord Connector Validation
overview: Testing, security, and documentation plan to harden the Discord connector and ensure safe, reliable operation in NAVI.
todos: []
isProject: false
---

# Discord Connector – Validation and Documentation Plan

---

## 1. Testing Strategy

### 1.1 Unit Tests

Focus areas:

- **Message mapping**:
  - Map Discord `MESSAGE_CREATE` events to NAVI inbound structures.
  - Map NAVI `OutboundMessage` to Discord REST payloads (channel/DM, replies, threads where supported).
- **Lifecycle**:
  - `Start` and `Stop` transitions for the connector.
  - Handling of invalid config or missing tokens.
- **Error handling**:
  - Classification of send failures into permanent vs transient based on HTTP status codes and error responses.
  - Behavior when Discord’s rate limits are hit (429).

Approach:

- Table-driven tests in `connectors/discord/bot_test.go`.
- Use small fixtures for Discord JSON payloads (events and responses).

### 1.2 Integration Tests

Focus areas:

- End-to-end flow from Discord into NAVI and back:
  - Simulate Discord events into the connector (via a fake Gateway or local harness).
  - Verify that NAVI gateway endpoints are called with correct payloads.
  - Verify outbound messages are sent back to Discord (or the test harness) with correct content.
- Connector lifecycle within NAVI:
  - Create the connector via the registry, start it via the Manager, then stop it cleanly.

Approach:

- Use a **test Discord harness**:
  - Option 1: Local fake Gateway/Webhook server that mimics Discord behavior.
  - Option 2: Live test bot in a dedicated Discord server (for later CI/integration environments).

### 1.3 End-to-End / Manual Scenarios

Document a small set of manual test scripts:

- Start NAVI with Discord connector enabled, send:
  - DM to the bot and verify a reply.
  - Message in a guild channel where the bot is mentioned.
  - Reply to NAVI’s message to continue a session.
- Restart NAVI and ensure reconnect behavior is correct (no duplicate messages, no missed messages in typical cases).

---

## 2. Security and Threat Analysis

### 2.1 Secrets and Credentials

- **Discord Bot Token**:
  - Never log the token or include it in error messages.
  - Store in NAVI’s settings store or secure configuration (not in plain YAML for production).
  - Support rotation with minimal downtime (document steps).
- **Gateway Secret**:
  - Shared secret used to obtain NAVI access tokens; treat as sensitive.
  - Keep separate from Discord token; ensure both are redacted in logs.

Validation tasks:

- Verify all logs redact bot tokens and sensitive headers.
- Confirm config loading never accidentally dumps full config in error paths.

### 2.2 Inbound Event Security

- Validate that only events from Discord are processed:
  - If using any webhooks for fallback, enforce signature or shared-secret validation.
  - For Gateway events, treat the WebSocket connection as a trusted channel but still validate payload structure.
- Sanitize user-supplied content:
  - Ensure no unsafe interpolation into shell commands or SQL (should not occur in connector).
  - Enforce reasonable length limits before sending onward to NAVI.

Threats to consider:

- Malicious users attempting to spam NAVI via Discord.
- Abuse of embeds or mentions to exploit downstream rendering systems.

### 2.3 Outbound Safety

- Ensure we never send responses to unintended channels or users:
  - Strictly match `ChatID`/channel IDs and thread IDs from NAVI to Discord targets.
- Handle permission errors gracefully:
  - If the bot loses permission in a channel, log a clear error and surface it to diagnostics.

---

## 3. Rate Limiting and Reliability

### 3.1 Rate Limit Handling

- Confirm how Discord expresses rate limits in responses (per-route buckets, `Retry-After` header).
- Ensure:
  - Transient 429s are treated as retryable with appropriate delay.
  - Permanent failures (e.g. 403, 404) are surfaced without infinite retry loops.

### 3.2 Reconnect and Backoff

- Define a reconnect policy for Gateway WebSocket:
  - Exponential backoff with jitter.
  - Upper bound on reconnect interval.
  - Clear logging when reconnect attempts happen.
- Ensure `Stop(ctx)` cancels all outstanding reconnect attempts.

---

## 4. Data Protection and Privacy

### 4.1 Data Handling

- Decide which fields from Discord messages are persisted or logged:
  - Prefer minimal logging of content; prioritize IDs and metadata.
  - Avoid storing full message history unless absolutely required.
- Respect Discord’s terms:
  - Avoid archiving or exporting data beyond allowed use.

### 4.2 PII Considerations

- Usernames, discriminators, and IDs are PII in many jurisdictions.
- Document:
  - Where they appear in logs.
  - How long we retain any persistent mapping between Discord IDs and NAVI identities.

---

## 5. Documentation Plan

### 5.1 User Documentation

Create or extend docs under `docs/` to cover:

- **Configuration Guide**:
  - How to create a Discord application and bot.
  - How to obtain and configure the bot token and gateway secret.
  - YAML configuration fields for `discord:` in NAVI config.
- **Usage Guide**:
  - Which channels and message types are supported.
  - How to mention or DM the bot.
  - Any limitations (no slash commands in MVP, thread behavior).

### 5.2 Technical Documentation

- **Architecture Overview**:
  - Sequence diagrams for inbound and outbound flows.
  - Description of how the connector integrates with the registry and Manager.
- **Component Details**:
  - Explanation of `connectors/discord/` files and their roles.
  - Any reusable helpers (message mapping, rate-limit handling).

### 5.3 Operational Notes

Document:

- How to enable/disable the connector in config.
- Health check behavior and what diagnostics are exposed via the gateway.
- Common errors and how to troubleshoot them:
  - Invalid tokens.
  - Insufficient permissions.
  - Rate limiting and reconnect storms.

---

## 6. Validation Checklist

Before considering the Discord connector “hardened”:

- [ ] All unit tests in `connectors/discord/bot_test.go` pass with high coverage on mapping and lifecycle paths.
- [ ] Integration tests validate end-to-end flows (DMs and guild channels) in a test environment.
- [ ] Logs are free of secrets and avoid leaking sensitive user data.
- [ ] Rate limiting and reconnect behavior are validated under synthetic failure conditions.
- [ ] User, technical, and operational documentation are written and reviewed.
- [ ] A rollback/disable procedure is documented if the connector must be turned off quickly.

