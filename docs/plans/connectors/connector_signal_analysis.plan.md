---
name: Signal Connector Analysis
overview: Research plan to study Signal connector implementations in OpenClaw and possible Go bridge approaches, informing NAVI's internal Signal connector design.
todos: []
isProject: false
---

# Signal Connector – Analysis Plan

---

## 1. Objectives

- **Goal**: Understand how Signal can be integrated via signal-cli / signald / other bridges and what OpenClaw’s implementation teaches us.
- **Deliverables**:
  - Clear picture of OpenClaw’s Signal channel architecture.
  - Survey of realistic Go-friendly integration options (signal-cli REST, signald, others).
  - Design principles to guide NAVI’s connector and its dependency model.

---

## 2. Reference Implementations and Options

### 2.1 OpenClaw Signal Extension

- **Location**: `.example-code/openclaw/extensions/signal/`
- **Artifacts to study**:
  - `index.ts`, `openclaw.plugin.json`.
  - `src/channel.ts` – how the channel is defined (delivery mode, capabilities).
  - `src/runtime.ts` (or equivalent) – how Signal runtime is wired.
- **Questions**:
  - Does it integrate via HTTP bridge to signal-cli, signald, or another daemon?
  - How are accounts, phone numbers, and devices configured?
  - How are inbound messages and group chats normalized?

### 2.2 PicoClaw / Other Go References

- PicoClaw has no Signal channel, so:
  - Identify any Go projects using signal-cli’s REST API or signald as references.
  - Capture high-level patterns from those projects (if used only for conceptual guidance).

### 2.3 Bridge Architecture Options

Compare:

- **signal-cli REST API**.
- **signald** (socket-based daemon).
- Potential third-party hosted Signal bridges (for future consideration).

---

## 3. Architecture & Pattern Breakdown (OpenClaw)

Research tasks:

1. **Channel abstraction**:
  - Document `ChannelPlugin` fields relevant to Signal: capabilities (DM, group, media), delivery mode, security hints.
2. **Bridge interaction**:
  - How the extension calls the Signal bridge (HTTP endpoints, RPC, sockets).
  - How it handles outgoing send, incoming messages, and receipt updates.
3. **Account & device management**:
  - How phone numbers and device links are configured and stored.
  - Any flows for QR-code pairing or re-linking.
4. **Message mapping**:
  - How conversation threads and groups are represented internally.
  - How media is uploaded/downloaded via the bridge.

---

## 4. Strengths, Weaknesses, and Design Principles

### 4.1 OpenClaw Strengths

Capture:

- Clear separation between channel semantics and the underlying Signal bridge.
- Any helper tools for registration/linking and their UX.
- How it handles multi-device or multi-number scenarios (if any).

### 4.2 Weaknesses / Gaps

Capture:

- Tight coupling to a particular bridge implementation.
- Operational complexity (e.g. requirement of sidecar processes).
- Limitations around media, reactions, or error reporting.

### 4.3 Principles for NAVI

From the above, derive:

- Preference for a **single, well-defined bridge interface** (likely signal-cli REST initially).
- Clear encapsulation of bridge-specific code to ease replacement/upgrades.
- Minimal but sufficient feature surface for MVP (DM + group text, basic media).

---

## 5. Platform and Bridge Assessment

Research questions:

1. **Official vs community support**:
  - Status of signal-cli and signald.
  - Stability and maintenance posture.
2. **Security posture**:
  - How keys and phone numbers are managed.
  - How E2EE is preserved while using a bridge.
3. **Operational model**:
  - How to run signal-cli/signald alongside NAVI (same host, container, sidecar).
  - Failure modes and recovery strategies.

---

## 6. NAVI-Facing Requirements

Translate research into:

- **Functional**:
  - MVP: 1:1 and group messaging via a bridge, text-focused.
  - Optional: media if the bridge reliably exposes it.
- **Non-functional**:
  - Clear configuration for phone number and bridge endpoint.
  - Strong logging around bridge connectivity without leaking sensitive metadata.

---

## 7. Completion Criteria

This analysis phase is complete when:

1. We have a concise description of OpenClaw’s Signal integration and its trade-offs.
2. We have evaluated at least signal-cli REST and signald as bridge options.
3. We have chosen a preferred bridge for NAVI’s MVP and justified it.
4. We have a list of explicit requirements to feed into `connector_signal_development.plan.md`.

