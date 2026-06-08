# Gateway Bridge Connector

> NAVI → Helm gateway forwarding — how NAVI AI delegates directives to a Helm instance and receives task results.

**Status:** Active
**Last Updated:** 2026-06-01
**Audience:** Platform engineers, multi-instance operators, NAVI AI developers
**See also:** [Connectors README](README.md) · [Webhook Connector](webhook-connector.md) · [Helm Public API](../../../../helm/docs/api/public-api.md) · [Integration Guide](../../../../docs/architecture/integration-guide.md) · [ADR-007 (Dual-Plane LLM)](../adr/ADR-007-dual-plane-llm.md)

---

## Overview

The gateway bridge connector enables a **NAVI AI instance** to forward directives to a remote **Helm** instance and receive task events back over a persistent WebSocket stream. It is the primary integration path between the Cognitive Layer (NAVI AI) and the Engineering Layer (Helm) in a deployed NAVI Ecosystem stack.

Without this connector, NAVI AI operates in standalone mode: it can reason, plan, and use built-in skills, but it cannot delegate implementation work to Helm's specialized Worker Agents (Coder, Critic, Strategist, Scout, Auditor, Migrator, Operator).

---

## Architecture

```
NAVI AI (:6284)                         Helm (:7700)
  │                                          │
  │  ConnectorManager                        │
  │  └── GatewayBridgeConnector              │
  │        │                                 │
  │        │  POST /api/directives           │
  │        │  (HTTP, JWT auth)               │
  │        │ ──────────────────────────────► │
  │        │                                 │  CEO Loop
  │        │                                 │  Task decomposition
  │        │                                 │  Worker dispatch
  │        │                                 │
  │        │  WS /api/events/stream          │
  │        │ ◄────────────────────────────── │
  │        │  (task.started, task.done,       │
  │        │   hitl.checkpoint, ceo.reply)    │
  │        │                                 │
  │  InboundMessage                          │
  │  (source: helm-bridge)                   │
  ▼                                          │
  CEO / Agent Loop                           │
```

NAVI AI sends directives over HTTP and listens for task events over a persistent WebSocket. The connector translates Helm event envelopes into NAVI `InboundMessage` structs so the NAVI CEO loop can react to engineering progress, approve HITL checkpoints, and incorporate task results into memory.

---

## Configuration

Add a `helm_bridge` block under `connectors` in `config/runtime.yaml`:

```yaml
connectors:
  helm_bridge:
    enabled: true
    helm_url: "http://localhost:7700"      # Helm gateway base URL
    api_key: "helm_sk_..."                 # Helm API key (X-API-Key)
    workspace_id: "ws_abc123"             # optional; scope directives to a Helm workspace
    reconnect_interval: 5s                # WebSocket reconnect backoff base
    max_reconnect_interval: 60s
    event_buffer: 256                      # in-memory event queue depth before backpressure
    forward_modes:                         # which NAVI directive modes to auto-forward
      - IMPLEMENT
      - REVIEW
```

### Configuration Reference

| Field | Type | Required | Description |
|---|---|---|---|
| `enabled` | bool | ✅ | Master switch. Set `false` to disable without removing config. |
| `helm_url` | string | ✅ | Base URL of the Helm gateway (`http[s]://host:port`). |
| `api_key` | string | ✅ | Helm API key sent as `X-API-Key` on every request. |
| `workspace_id` | string | No | Helm workspace ID to scope all forwarded directives. If absent, Helm uses the default workspace. |
| `reconnect_interval` | duration | No | Initial WebSocket reconnect wait. Defaults to `5s`. Uses exponential backoff up to `max_reconnect_interval`. |
| `max_reconnect_interval` | duration | No | Maximum reconnect backoff. Defaults to `60s`. |
| `event_buffer` | int | No | Depth of the in-memory channel between the WebSocket reader goroutine and the connector dispatch loop. Defaults to `256`. |
| `forward_modes` | []string | No | NAVI directive modes that are automatically forwarded to Helm. Defaults to `[IMPLEMENT]`. Accepted values: `DISCUSS`, `IMPLEMENT`, `REVIEW`, `AUDIT`. |

---

## Session and Auth Passthrough

NAVI AI acts as an authenticated client of the Helm gateway. All outbound requests carry:

```
X-API-Key: <helm_api_key>
X-Navi-Session: <navi_session_id>   # forwarded for Helm audit trail
```

The `X-Navi-Session` header is informational — Helm does not enforce it but logs it for provenance. Helm JWT auth (`Authorization: Bearer ...`) is also accepted if the Helm instance is configured for bearer tokens instead of API keys.

---

## Directive Forwarding

When NAVI's CEO loop resolves a directive whose mode is in `forward_modes`, the bridge connector serialises the directive and POSTs it to Helm:

```http
POST /api/directives HTTP/1.1
Host: localhost:7700
X-API-Key: helm_sk_...
Content-Type: application/json

{
  "text": "Refactor the authentication module to use the new JWT library",
  "mode": "IMPLEMENT",
  "workspace_id": "ws_abc123",
  "origin": {
    "source": "navi-bridge",
    "session_id": "sess_...",
    "user_id": "user_..."
  }
}
```

Helm creates a Directive record, runs CEO decomposition, and starts dispatching Tasks. The bridge receives all resulting events over the open WebSocket stream.

---

## Event Stream

The connector opens a persistent WebSocket to `ws://{helm_url}/api/events/stream` and listens for Helm event envelopes:

| Helm Event Type | NAVI InboundMessage text | Disposition |
|---|---|---|
| `ceo.reply` | CEO reply text | Surfaced to user session |
| `task.started` | `[helm] Task {id} started: {title}` | Logged to session, not surfaced |
| `task.done` | `[helm] Task {id} complete` | Logged; updates NAVI task context |
| `task.failed` | `[helm] Task {id} failed: {reason}` | Triggers NAVI error-handling loop |
| `hitl.checkpoint` | `[helm] Approval required: {description}` | Surfaced to user as HITL prompt |
| `plan.complete` | `[helm] Plan complete for directive {id}` | Logged; closes forwarded directive |

NAVI re-broadcasts `hitl.checkpoint` events to the active user session so the user can approve or reject from within the NAVI interface. The approval response is relayed back to Helm via `POST /api/hitl/{id}/approve` or `/reject`.

---

## HITL Relay

Helm may pause on a HITL checkpoint during execution of a forwarded directive. The bridge surfaces this to the NAVI user session:

```
[helm] Approval required: Delete 3 source files as part of refactor (HIGH risk)
Reply APPROVE or REJECT to continue.
```

The user's reply is intercepted by NAVI's CEO loop and relayed to Helm:

```http
POST /api/hitl/{checkpoint_id}/approve HTTP/1.1
Host: localhost:7700
X-API-Key: helm_sk_...
```

This keeps the human approval UX inside NAVI (e.g., Telegram, PET) without requiring the user to access the Helm console directly.

---

## Connection Lifecycle

```
NAVI startup
  └── GatewayBridgeConnector.Start()
        ├── Dial WS → /api/events/stream
        ├── On success: enter read loop
        └── On disconnect: exponential backoff → redial
```

The WebSocket connection is maintained for the lifetime of the `navid` process. If the Helm instance is unreachable at startup, the connector retries silently in the background. Directive forwarding fails fast with an error if the WebSocket is not yet connected — the NAVI CEO loop will surface this to the user as a capability-unavailable message.

Health is reported via `GET /api/health` on the NAVI gateway:

```json
{
  "connectors": {
    "helm_bridge": {
      "status": "connected",
      "helm_url": "http://localhost:7700",
      "last_event_at": "2026-06-01T10:04:32Z"
    }
  }
}
```

---

## Standalone vs. Bridge Mode

| Mode | Behaviour |
|---|---|
| **Standalone** (no bridge) | NAVI handles all directives itself. Engineering tasks use built-in skills and tools only. No Helm Worker Agents. |
| **Bridge mode** (`helm_bridge.enabled: true`) | `IMPLEMENT` and `REVIEW` directives are forwarded to Helm. NAVI handles `DISCUSS` and `AUDIT` locally. Results stream back and are incorporated into NAVI memory. |

Bridge mode is the recommended deployment for any production use case involving code modification, since Helm provides Surface Claims, Governor constraints, and specialized Worker Agents that are not replicated in NAVI AI.

---

## Operational Notes

- **Single Helm target**: The bridge currently supports exactly one Helm target per NAVI instance. Multi-Helm routing is tracked under the NAVI Net Phase 1 identity model.
- **Event ordering**: Events arrive in the order Helm emits them. There is no replay or re-ordering guarantee if the WebSocket reconnects mid-execution. In-flight directive state is reconciled on reconnect via `GET /api/directives/{id}`.
- **Cost propagation**: Helm task token costs are included in `task.done` event metadata and written to NAVI's `runs` table for cost attribution.
- **Version compatibility**: The bridge serialises to the Helm gateway API version negotiated at startup via `GET /api/version`. Minimum Helm version required: Phase 14.

---

*See also: [Webhook Connector](webhook-connector.md) · [Helm Public API](../../../../helm/docs/api/public-api.md) · [Integration Guide](../../../../docs/architecture/integration-guide.md) · [Connectors README](README.md)*
