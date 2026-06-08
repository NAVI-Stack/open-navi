# Webhook Ingestion Endpoint

OMN-36 adds a generic webhook ingress path alongside the existing connector-specific
`POST /webhooks/{name}` route.

## Shipped Scope

- `GET /api/webhooks`
- `PUT /api/webhooks/{source}`
- `DELETE /api/webhooks/{source}`
- `POST /api/webhooks/{source}`

Webhook registrations are persisted in the settings table under a single JSON
setting and include:

- source
- session_id
- enabled
- source_channel
- event_header
- delivery_id_header
- signature config

The gateway redacts secrets from operator responses while preserving a
`secret_configured` signal for inspection.

## Signature Modes

- `none`
- `header-value`
- `hmac-sha256`

GitHub registrations get default header conventions for:

- `X-GitHub-Event`
- `X-GitHub-Delivery`
- `X-Hub-Signature-256`
- `sha256=` prefix

## Runtime Behavior

Validated webhook payloads are translated into structured NAVI inbox signals:

- `actor_type = webhook`
- `payload_type = json`
- `source_channel = webhook` by default
- raw JSON preserved in `structured_payload`
- rendered summary text preserved in `content`

This keeps external automation ingress on the same runtime queueing path as
other session inputs instead of creating a side-channel execution path.
