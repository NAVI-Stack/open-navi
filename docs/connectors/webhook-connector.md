# Webhook Connector

> Inbound HTTP trigger connector — receive external events and route them into the NAVI agent loop.

**Status:** Active
**Last Updated:** 2026-06-01
**Audience:** Integration engineers, automation authors, connector authors
**See also:** [Connectors README](README.md) · [Gateway Bridge Connector](gateway-bridge.md) · [Gateway API](../specs/gateway-api.md) · [Canonical Connector Spec](../canonical/specs/connectors.md)

---

## Overview

The webhook connector exposes a named inbound HTTP endpoint on the NAVI gateway and routes every matching POST request into the NAVI agent loop as a synthetic inbound message. It is the primary integration point for external systems — CI pipelines, monitoring alerts, third-party SaaS platforms, and custom automation — that need to push events into NAVI without a persistent connection.

The connector is **event-driven and stateless**: each POST is self-contained. NAVI processes the payload, optionally runs the associated session's CEO loop, and returns an acknowledgement.

---

## Architecture

```
External System
      │
      │  POST /webhooks/{name}
      ▼
NAVI Gateway (:6284)
  ├── Signature verification (HMAC-SHA256, optional)
  ├── Payload normalisation
  └── ConnectorManager.Dispatch()
            │
            ▼
     InboundMessage
  ┌─────────────────┐
  │  source: webhook│
  │  channel: {name}│
  │  text: …        │
  │  metadata: {…}  │
  └────────┬────────┘
           ▼
      CEO / Agent Loop
```

The gateway route `POST /webhooks/{name}` is intentionally not protected by `AuthMiddleware`. The connector itself handles request authentication via an optional shared secret and signature header.

---

## Configuration

Add a `webhook` block under `connectors` in `config/runtime.yaml`:

```yaml
connectors:
  webhook:
    - name: github-ci
      secret: "your-shared-secret"      # optional; enables HMAC-SHA256 verification
      session_id: "sess_abc123"         # optional; bind to an existing session
      extract_text: "$.action"          # optional; JSONPath to use as the message text
      allow_origins: []                 # optional; restrict to specific source IPs

    - name: pagerduty-alerts
      secret: "pd-secret"
      extract_text: "$.messages[0].event.data.summary"
```

Each entry creates a distinct endpoint at `/webhooks/{name}`.

### Configuration Reference

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | ✅ | URL slug for the endpoint: `/webhooks/{name}`. Must be unique per instance. |
| `secret` | string | No | Shared secret used to verify the `X-Hub-Signature-256` header (HMAC-SHA256). If omitted, signature checking is skipped. |
| `session_id` | string | No | Pin all events from this webhook to a specific NAVI session. If absent, a new session is created per event or a default session is used. |
| `extract_text` | string | No | JSONPath expression to extract the human-readable message text from the payload body. Falls back to the full JSON body serialized as a string if unset or if extraction fails. |
| `allow_origins` | []string | No | Allowlist of source IP addresses or CIDR ranges. Requests from unlisted IPs are rejected with `403`. Empty list = no IP restriction. |

---

## Payload Schema

The connector accepts any content type. Behaviour by content type:

| Content-Type | Handling |
|---|---|
| `application/json` | Parsed as JSON; `extract_text` JSONPath applied if configured. |
| `application/x-www-form-urlencoded` | Decoded to a map; fields available in metadata. |
| `text/plain` | Body used as message text verbatim. |
| Other / none | Raw body stored in metadata as `raw_body` (base64-encoded). |

### Inbound message envelope

Every accepted POST produces an `InboundMessage` with the following fields visible to the agent loop:

```json
{
  "source": "webhook",
  "channel": "{name}",
  "text": "… extracted or serialized payload …",
  "metadata": {
    "method": "POST",
    "path": "/webhooks/{name}",
    "headers": { "X-Github-Event": "push" },
    "raw_payload": { … }
  }
}
```

---

## Signature Verification

When `secret` is set, the connector expects a `X-Hub-Signature-256` header computed as:

```
X-Hub-Signature-256: sha256=<HMAC-SHA256(secret, raw-body)>
```

This is the format used by GitHub, Stripe, PagerDuty, and most SaaS webhook implementations. Requests without a valid signature are rejected with `401`. Timing-safe comparison is used to prevent timing attacks.

---

## Response Format

Successful delivery returns `200 OK`:

```json
{ "ok": true, "message_id": "msg_..." }
```

Errors return the appropriate HTTP status with a JSON body:

| Status | Condition |
|---|---|
| `400 Bad Request` | Payload could not be parsed or required fields are missing. |
| `401 Unauthorized` | Signature verification failed. |
| `403 Forbidden` | Source IP not in `allow_origins`. |
| `429 Too Many Requests` | Rate limit exceeded (governor). |
| `500 Internal Server Error` | Dispatch to agent loop failed. |

---

## Event Routing

By default, each inbound webhook message is dispatched to the NAVI agent loop's **Heartbeat** path — treated as an autonomous trigger rather than a user-initiated directive. This means:

- The CEO loop runs in `DISCUSS` mode unless the payload explicitly requests `IMPLEMENT`.
- No HITL checkpoint is triggered unless the resulting plan has HIGH or CRITICAL risk operations.
- Responses are logged to the session event stream but not sent back to the HTTP caller.

To change the dispatch mode, include a `x-navi-mode` header in the POST request:

```
X-Navi-Mode: IMPLEMENT
```

Accepted values: `DISCUSS` (default), `IMPLEMENT`, `REVIEW`, `AUDIT`.

---

## Example: GitHub Push Hook

Configure GitHub to send push events to `https://your-navi-host:6284/webhooks/github-ci` with a shared secret, then add to `runtime.yaml`:

```yaml
connectors:
  webhook:
    - name: github-ci
      secret: "your-github-webhook-secret"
      extract_text: "$.head_commit.message"
```

NAVI will receive each push as a message like:

```
[webhook:github-ci] fix: normalise JWT expiry check
```

The CEO loop can then trigger a Scout research pass, run tests, or open a review directive depending on configured Heartbeat rules.

---

## Example: Generic POST via curl

```bash
curl -X POST https://localhost:6284/webhooks/my-hook \
  -H "Content-Type: application/json" \
  -H "X-Hub-Signature-256: sha256=$(echo -n '{"event":"test"}' | openssl dgst -sha256 -hmac 'my-secret' | cut -d' ' -f2)" \
  -d '{"event":"test","message":"hello from automation"}'
```

---

## Operational Notes

- **Idempotency**: The connector does not deduplicate requests. If the external system retries on failure, NAVI will process the event twice. Use a unique `message_id` in the payload and add deduplication logic in a skill if needed.
- **Timeouts**: The HTTP connection is held until the message is dispatched to the agent loop (not until the CEO loop completes). Typical response time is under 200 ms.
- **Rate limiting**: The gateway governor applies a default rate limit of 60 requests/minute per endpoint name. Override via the gateway `rate_limit` config block.
- **TLS**: In production, place the gateway behind a TLS-terminating reverse proxy. The gateway itself listens on plain HTTP on `:6284` by default.

---

*See also: [Gateway Bridge Connector](gateway-bridge.md) · [Telegram Connector](telegram.md) · [Slack Connector](slack.md) · [Connectors README](README.md)*
