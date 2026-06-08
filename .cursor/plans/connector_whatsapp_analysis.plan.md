---
name: WhatsApp Connector Analysis
overview: Research plan to compare WhatsApp connector implementations in OpenClaw and PicoClaw and extract design principles for NAVI's internal WhatsApp connector.
todos: []
isProject: false
---

# WhatsApp Connector – Analysis Plan

---

## 1. Objectives

- **Primary goal**: Design a grounded blueprint for an internal WhatsApp connector that is robust, compliant, and maintainable, based on OpenClaw and PicoClaw implementations.
- **Outcomes**:
  - Clear understanding of WhatsApp integration options (Web bridge, native/Cloud API).
  - Catalog of patterns to adopt or avoid from both ecosystems.
  - Constraints and risks that will directly inform our development plan.

---

## 2. Reference Implementations

### 2.1 OpenClaw WhatsApp Extension

- **Location**: `.example-code/openclaw/extensions/whatsapp/`
- **Key artifacts to study**:
  - `index.ts` – plugin registration and config wiring.
  - `openclaw.plugin.json` – manifest (`channels: ["whatsapp"]`), config schema, security hints.
  - `src/channel.ts` – `ChannelPlugin` definition (capabilities, outbound delivery mode, streaming flags).
  - `src/runtime.ts` – runtime management for WhatsApp Web (browser automation / session management).
  - `src/resolve-target.test.ts` – targeting logic for contacts/chats.
- **Focus questions**:
  - How does the extension connect to WhatsApp: headless browser (WhatsApp Web), a bridge API, or Cloud API?
  - How are login, QR presentation, and session persistence handled?
  - What capabilities are exposed (direct messages, groups, media, reactions)?
  - How are message targets (phone numbers, groups) resolved from human-friendly input?
  - How is streaming handled (chunking/coalescing messages, `blockStreaming` flags)?

### 2.2 PicoClaw WhatsApp Channels

- **Location**: upstream PicoClaw repo:
  - `pkg/channels/whatsapp/` – HTTP bridge integration.
  - `pkg/channels/whatsapp_native/` – native API integration (if present).
- **Key artifacts to study**:
  - Channel structs and their `Start/Stop/Send/IsRunning` implementations.
  - Any helper modules for session management, QR login, and phone-number normalization.
  - Configuration fields for bridge URL vs native mode.
- **Focus questions**:
  - How are bridge vs native modes selected and configured?
  - How are inbound messages delivered (webhook callbacks vs polling vs WebSocket bridge)?
  - How is media upload/download handled in Go?
  - What error and retry behavior is implemented around the bridge/native APIs?

---

## 3. Architecture & Pattern Breakdown

### 3.1 OpenClaw Implementation Shape

Research tasks:

1. **Channel surface**:
   - Document relevant `ChannelPlugin` fields for WhatsApp (capabilities, delivery mode, outbound config).
   - Identify configuration knobs: account binding, multi-account support, device sessions.
2. **Runtime & lifecycle**:
   - Diagram how the plugin’s `register(api)` leads to starting the WhatsApp runtime.
   - Understand how login flows, QR codes, and session expiration are handled.
3. **Inbound flow**:
   - Trace how messages from WhatsApp (via Web bridge) are normalized into internal messages.
   - Understand contact/group identity mapping (phone numbers, names, IDs).
4. **Outbound flow**:
   - Map NAVI-style outbound fields (chat, content, media) to WhatsApp’s payloads.
   - Identify how they treat templates, interactive messages, and media caption limits.

### 3.2 PicoClaw Implementation Shape

Research tasks:

1. **Bridge vs native modes**:
   - Clarify the responsibilities of `whatsapp` vs `whatsapp_native` channels.
   - Capture differences in config (bridge URL, auth keys, phone number, device ID).
2. **Lifecycle and workers**:
   - See how each channel registers with `RegisterFactory` and how Manager starts them.
   - Understand long-running workers, reconnection behavior, and error classification.
3. **Message mapping**:
   - Document how WhatsApp messages (from the bridge or native API) are represented in the bus.
   - Identify how replies, media, and group messages are encoded.

---

## 4. Strengths, Weaknesses, and Adoptable Patterns

### 4.1 OpenClaw

Capture:

- **Strengths**:
  - Higher-level channel abstraction for features like login tools, heartbeat, and account binding.
  - Rich UX features (e.g. tools to start QR login, monitor login state).
  - Good modeling of capabilities (media support, group chats, streaming flags).
- **Weaknesses**:
  - Tight coupling to TypeScript runtime and plugin SDK.
  - Potentially heavy dependencies on browser automation.

### 4.2 PicoClaw

Capture:

- **Strengths**:
  - Clean Go channels with clear configuration for bridge vs native modes.
  - Practical patterns for error handling, retries, and rate limiting around external APIs.
  - Concrete examples of message mapping into a Go bus.
- **Weaknesses**:
  - Harder to configure multi-account setups.
  - Less explicit modeling of advanced WhatsApp-specific features (templates, commerce).

### 4.3 Design Principles for NAVI

From the above, derive a short list of principles:

- Prefer **explicit transport modes** (e.g. Cloud API vs self-hosted bridge), configured per account.
-,Keep session management and phone-number mapping **well-encapsulated**.
- Design for **recoverable connectivity** to external bridges or Cloud API with clear diagnostics.

---

## 5. Platform API Assessment (WhatsApp)

Research questions (using current Meta / WhatsApp documentation):

1. **API choices**:
   - Official WhatsApp Business Cloud API vs on-prem or third-party bridges.
   - Limits, pricing, and account requirements.
2. **Auth model**:
   - Bearer tokens / App IDs, rotation requirements, and scopes.
3. **Webhooks and messages**:
   - Webhook payload shapes, signature verification model, retry rules.
   - Message and media limits (text length, attachment sizes, media retention windows).
4. **Templates and interactive messages**:
   - How templates are created and referenced.
   - Whether they are required for outbound initiations.

---

## 6. NAVI-Facing Requirements

Translate research into requirements for our connector:

- **Functional**:
  - Start with **Cloud API webhook mode** as the primary MVP path.
  - Support 1:1 conversations and basic group conversations where supported by the chosen API.
  - Handle text and basic media (images/documents) reliably.
- **Non-functional**:
  - Strong webhook verification and authentication.
  - Robust error handling and clear observability for delivery issues.
  - Clear story for token management and rotation.

---

## 7. Deliverables

This analysis phase is done when we have:

1. A written comparison of OpenClaw and PicoClaw WhatsApp strategies (Web bridge vs native/Cloud).
2. A list of concrete patterns and anti-patterns for NAVI.
3. A decision (or at least a preferred path) for our initial transport (Cloud API vs bridge).
4. A requirement set that will be fed directly into `connector_whatsapp_development.plan.md`. 

