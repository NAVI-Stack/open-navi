---
name: Signal Connector Validation
overview: Testing, security, and documentation plan to harden the Signal connector built on a bridge like signal-cli REST.
todos: []
isProject: false
---

# Signal Connector – Validation and Documentation Plan

---

## 1. Testing Strategy

### 1.1 Unit Tests

Cover:

- Mapping between bridge payloads and NAVI inbound messages.
- Construction of bridge send requests from `OutboundMessage`.
- Error classification (bridge HTTP/socket errors → connector error types).

### 1.2 Integration Tests

Use a local fake Signal bridge or a mocked signal-cli REST server to:

- Simulate inbound messages and verify that NAVI sees correct inbound events.
- Verify outbound send payloads and error-handling behavior.

### 1.3 Manual Scenarios

With a real test Signal account:

- Send DM and group messages into NAVI and verify responses.
- Validate behavior during bridge restarts and temporary outages.

---

## 2. Security and Threat Analysis

### 2.1 Secrets and Keys

- Signal identity keys and phone number are managed by the bridge, but:
  - Any tokens or credentials used to reach the bridge must be treated as secrets.
  - Ensure they are never logged.

### 2.2 Bridge Trust Model

Document:

- That the bridge process is fully trusted to handle E2EE correctly.
- Recommended deployment patterns (same host, locked-down sidecar).

Identify threats:

- Compromised bridge leading to message interception.
- Misconfigured bridge endpoints accepting unauthenticated commands.

Mitigations:

- Use local-only endpoints where possible.
- Restrict access to bridge ports (firewall, container networking).

---

## 3. Reliability and Failure Modes

### 3.1 Bridge Connectivity

- Validate behavior when:
  - The bridge is down at startup.
  - The bridge restarts while NAVI is running.
  - The network between NAVI and the bridge is flaky.

### 3.2 Rate and Load Handling

- While Signal is not primarily rate-limited like HTTP APIs, ensure:
  - Connector does not hammer the bridge with excessive polls (if polling).
  - Adequate backoff is in place on repeated failures.

---

## 4. Data Protection and Privacy

- Treat phone numbers and message content as highly sensitive.
- Limit logging of content; prefer IDs and small redacted excerpts when necessary.
- Document any retention assumptions and configurable retention options.

---

## 5. Documentation Plan

### 5.1 User Documentation

Add docs describing:

- Required steps to set up and run signal-cli (or chosen bridge).
- How to configure NAVI to talk to the bridge (URL, phone number, any auth).
- Supported messaging scenarios and limitations.

### 5.2 Technical and Operational Docs

- Describe the overall architecture (NAVI ↔ bridge ↔ Signal network).
- Provide operational guidance:
  - How to monitor bridge health.
  - How to rotate any bridge credentials.
  - How to safely restart or upgrade the bridge.

---

## 6. Validation Checklist

Before enabling Signal in production:

- [ ] Unit tests cover mapping and error-handling for common bridge operations.
- [ ] Integration tests validate behavior against a fake or test bridge.
- [ ] Secrets and bridge configuration values are never logged.
- [ ] Failure modes (bridge down, restart, network failures) are well-understood and logged.
- [ ] User and technical documentation are complete and reviewed.

