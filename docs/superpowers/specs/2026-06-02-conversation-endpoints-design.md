# Durable Conversation Endpoints Design

**Status:** Approved design for implementation planning
**Date:** 2026-06-02
**Scope:** First-class chat endpoints, connector delivery routing, Telegram binding repair, and future Discord readiness

---

## Problem

NAVI currently records where a message came from with fields such as `source_channel` and `source_message_ref`, and Telegram keeps in-memory mappings between Telegram chat IDs and NAVI chat IDs. Those fields are useful provenance, but they are not a durable delivery model.

This causes three product problems:

1. Transcript visibility and outbound delivery are not cleanly separated. The console can see connector-backed chats, but the system cannot explain whether that visibility implies external delivery.
2. Telegram single-chat recovery can infer a NAVI chat from the public chat list, which can bind Telegram to a console-created chat after restart or state loss.
3. NAVI cannot safely answer requests such as "send this to my Telegram" because it has no first-class list of valid connector destinations attached to a conversation.

The fix is to make conversation endpoints durable and explicit.

---

## Design Decisions

### Forward-Only Tranche

This tranche does not include compatibility migration or backfill behavior for existing databases or historical chats. Schema declarations must be safe for fresh development databases and normal forward creation paths. Existing chats may gain endpoints when touched by new endpoint-aware code, but there is no historical sweep requirement.

### Default Delivery Rule

NAVI replies only to the endpoint that produced the inbound user message.

The console remains the default operator-visible endpoint for every user-facing chat, but console visibility does not imply external delivery. External echoing, mirroring, or cross-connector sends require explicit endpoint policy or an explicit send request.

### Endpoint Definition

A conversation endpoint is a delivery surface attached to a NAVI chat.

Endpoint examples:

- Console endpoint: local/operator chat UI for the NAVI console.
- Telegram endpoint: one Telegram connector instance plus one external chat or topic.
- Discord endpoint: one Discord connector instance plus one channel, thread, or DM once Discord exists.

The endpoint model separates:

- Chat transcript: durable conversation state.
- Message origin: where a user message came from.
- Delivery target: where assistant output should be sent.
- Operator visibility: whether console can inspect the transcript.

### Console as Default Endpoint

Console is represented as the default endpoint type for user-facing chats. It is not dispatched through the connector manager in this tranche. The console reads chats and live events through existing gateway APIs.

This preserves the user's desired "console as default connector" behavior without introducing a fake connector worker.

---

## Data Model

### `navi_conversation_endpoints`

One row per chat delivery surface.

Required fields:

- `endpoint_id TEXT PRIMARY KEY`
- `chat_id TEXT NOT NULL REFERENCES navi_chats(chat_id) ON DELETE CASCADE`
- `endpoint_type TEXT NOT NULL CHECK(endpoint_type IN ('console', 'connector'))`
- `connector_kind TEXT NOT NULL DEFAULT ''`
- `connector_instance_id TEXT NOT NULL DEFAULT ''`
- `external_chat_id TEXT NOT NULL DEFAULT ''`
- `external_thread_id TEXT NOT NULL DEFAULT ''`
- `display_name TEXT NOT NULL DEFAULT ''`
- `receive_enabled INTEGER NOT NULL DEFAULT 1 CHECK(receive_enabled IN (0, 1))`
- `send_enabled INTEGER NOT NULL DEFAULT 1 CHECK(send_enabled IN (0, 1))`
- `mirror_enabled INTEGER NOT NULL DEFAULT 0 CHECK(mirror_enabled IN (0, 1))`
- `status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'disabled', 'deleted'))`
- `created_at DATETIME NOT NULL`
- `updated_at DATETIME NOT NULL`
- `metadata_json TEXT NOT NULL DEFAULT '{}'`

Indexes and uniqueness:

- One active console endpoint per chat.
- One active connector endpoint per `(connector_instance_id, external_chat_id, external_thread_id)`.
- Lookup by `chat_id`.

Console endpoint rows use:

- `endpoint_type = 'console'`
- `connector_kind = 'console'`
- `connector_instance_id = 'console'`
- `external_chat_id = chat_id`
- `external_thread_id = ''`

Connector endpoint rows use:

- `endpoint_type = 'connector'`
- `connector_kind = 'telegram'`, `discord`, or another connector kind.
- `connector_instance_id` matching the connector manager instance ID.
- `external_chat_id` from the external platform.
- `external_thread_id` for Telegram topics, Discord threads, or an empty string.

`connector_instance_id` must be the stable configured connector-manager instance ID. Single-instance deployments may use values such as `telegram`, but multi-instance deployments must use distinct stable IDs. `connector_kind` identifies the connector type; `connector_instance_id` identifies the configured connector instance.

### `navi_chat_delivery_policy`

One row per chat, with an implicit default if the row is absent.

Required fields:

- `chat_id TEXT PRIMARY KEY REFERENCES navi_chats(chat_id) ON DELETE CASCADE`
- `default_mode TEXT NOT NULL DEFAULT 'reply_to_origin' CHECK(default_mode IN ('reply_to_origin', 'explicit_only'))`
- `created_at DATETIME NOT NULL`
- `updated_at DATETIME NOT NULL`
- `metadata_json TEXT NOT NULL DEFAULT '{}'`

The first implementation uses `reply_to_origin` as the default mode. Mirroring is represented by `mirror_enabled` on endpoints instead of adding a third global chat mode.

In `explicit_only` mode, assistant responses are written to the chat transcript and console live-event stream, but no connector endpoint dispatch occurs unless an explicit endpoint send request names one or more target endpoint IDs. Console transcript visibility remains unaffected.

### Message and Inbox Origin Columns

Add nullable origin endpoint columns:

- `navi_chat_messages.origin_endpoint_id TEXT`
- `navi_inbox.origin_endpoint_id TEXT`
- `runtime_runs.origin_endpoint_id TEXT`

These columns preserve the endpoint that caused the current runtime turn. Existing `source_channel` and `source_message_ref` remain as provenance fields and compatibility data.

### `navi_message_deliveries`

One row per outbound delivery attempt.

Required fields:

- `delivery_id TEXT PRIMARY KEY`
- `message_id TEXT NOT NULL`
- `chat_id TEXT NOT NULL`
- `endpoint_id TEXT NOT NULL`
- `connector_instance_id TEXT NOT NULL DEFAULT ''`
- `status TEXT NOT NULL CHECK(status IN ('queued', 'sent', 'failed', 'skipped'))`
- `attempt_count INTEGER NOT NULL DEFAULT 0`
- `last_error TEXT NOT NULL DEFAULT ''`
- `created_at DATETIME NOT NULL`
- `updated_at DATETIME NOT NULL`
- `metadata_json TEXT NOT NULL DEFAULT '{}'`

Uniqueness:

- `UNIQUE(message_id, endpoint_id)`

This table is an audit and dedupe spine for connector delivery. It does not replace existing connector execution outcome records.

---

## Services

### Endpoint Store

The store provides raw-SQL operations for:

- Create default console endpoint for a chat.
- Upsert connector endpoint by external address.
- List endpoints for a chat.
- Get endpoint by ID.
- Resolve endpoint by `(connector_instance_id, external_chat_id, external_thread_id)`.
- Update endpoint flags and status.
- Read and update chat delivery policy.

### Endpoint Resolver

The resolver owns endpoint selection.

For inbound connector messages:

1. Resolve the connector endpoint by external address.
2. If found, use its `chat_id` and `endpoint_id`.
3. If missing, create a NAVI chat, create its console endpoint, then create the connector endpoint.
4. Return both IDs to message intake.

If a matching endpoint exists with `receive_enabled = 0` or non-active status, inbound connector messages must not bind to a different chat and must not auto-create a replacement endpoint for the same external address. Connector intake should reject, ignore, or record the inbound event as skipped according to connector policy.

For console messages:

1. Resolve or create the chat's console endpoint.
2. Mark it as the origin endpoint.
3. Do not select external delivery targets by default.

### Delivery Service

The delivery service is the only runtime component that turns assistant messages into connector sends.

Inputs:

- `chat_id`
- assistant `message_id`
- assistant content
- `origin_endpoint_id`
- run/session metadata

Resolution rules:

1. If the chat delivery policy is `explicit_only` and this is not an explicit endpoint send request, do not dispatch to connector endpoints.
2. If the origin endpoint is an active connector endpoint and `send_enabled = 1`, include that endpoint.
3. If the origin endpoint is console, send to console only through the normal transcript/live-event path.
4. Add any active connector endpoints with `mirror_enabled = 1`.
5. Add explicitly selected endpoint IDs for explicit send requests.
6. Dedupe the combined target list by `endpoint_id`.
7. Never call `DispatchAll`.
8. Record each target in `navi_message_deliveries`.

Connector delivery uses the existing manager `messaging.send` / `Dispatch` path with the endpoint's connector instance ID and external chat/thread IDs.

First tranche supports bounded retry using the existing connector retry policy. The Delivery Service records each outbound delivery target in `navi_message_deliveries`, increments `attempt_count` across retry attempts, updates `status` to `sent`, `failed`, or `skipped`, and stores the final connector error in `last_error` when exhausted. The Delivery Service must not invent a separate retry system in this tranche.

Delivery target resolution dedupes targets after combining the reply-to-origin endpoint, mirror-enabled endpoints, and explicitly selected endpoint IDs. A single assistant message must never be delivered twice to the same endpoint.

---

## Gateway APIs

### Chat Endpoint APIs

Owner-authenticated routes:

- `GET /api/navi/chats/{id}/endpoints`
- `POST /api/navi/chats/{id}/endpoints`
- `PATCH /api/navi/chats/{id}/endpoints/{endpoint_id}`
- `GET /api/navi/chats/{id}/delivery-policy`
- `PATCH /api/navi/chats/{id}/delivery-policy`

`GET /api/navi/chats/{id}/endpoints` returns console and connector endpoints in one list so the console and future tools can show valid destinations.

### Explicit Send API

Owner-authenticated route:

- `POST /api/navi/chats/{id}/send`

Request shape:

```json
{
  "content": "Send this text",
  "endpoint_ids": ["endpoint-1", "endpoint-2"],
  "source": "console"
}
```

Rules:

- `endpoint_ids` must belong to the chat.
- Connector endpoints must be active and send-enabled.
- Console endpoint sends are represented by appending to the chat transcript and live stream, not by connector dispatch.
- The route records delivery attempts in `navi_message_deliveries`.

### Connector Endpoint Resolve API

Connector-authenticated internal route:

- `POST /api/connectors/endpoints/resolve`

Request shape:

```json
{
  "connector_instance_id": "telegram",
  "connector_kind": "telegram",
  "external_chat_id": "12345",
  "external_thread_id": "",
  "display_name": "Telegram DM"
}
```

Response shape:

```json
{
  "chat_id": "chat-id",
  "endpoint_id": "endpoint-id",
  "created_chat": true,
  "created_endpoint": true
}
```

Telegram uses this route instead of recovering from the first public chat.

---

## Runtime and Tool Surface

`navi.messaging.send_reply` remains a current-chat reply tool. It does not become a generic connector sender.

Endpoint-aware tools are separate and governed:

- `navi.messaging.list_endpoints`
- `navi.messaging.send_to_endpoint`

These tools use the endpoint list and delivery service. The LLM cannot invent connector instance IDs or external chat IDs; it can only select attached, send-enabled endpoint IDs returned by the endpoint list.

The implementation plan should introduce endpoint tools only after the endpoint store, resolver, delivery service, and gateway API tests pass.

---

## Telegram Changes

Telegram inbound should use the endpoint resolver for chat binding.

Required behavioral changes:

- Replace first-public-chat recovery with durable endpoint lookup.
- Create connector endpoints for Telegram DMs, groups, channels, and topics.
- Preserve `source_channel` and `source_message_ref` for provenance.
- Populate `origin_endpoint_id` when queueing messages into NAVI.
- Deliver assistant replies through the delivery service, not ad hoc chat ID maps.

The existing in-memory maps can remain as a cache, but the durable endpoint table is authoritative.

---

## Discord Readiness

Discord should use the same endpoint contract when the connector is built.

Discord endpoint address fields map as follows:

- `connector_kind = 'discord'`
- `connector_instance_id = configured Discord connector instance`
- `external_chat_id = Discord channel ID or DM channel ID`
- `external_thread_id = Discord thread ID when present`

Guild IDs and additional Discord metadata live in `metadata_json` unless a Discord-specific requirement proves they need indexed columns.

---

## Console UX

The console should expose endpoint state without changing the chat list model.

Required behaviors:

- Every user-facing chat has a visible console endpoint.
- Chat details can show endpoint badges such as Console, Telegram, and Discord.
- The send composer remains local by default.
- Explicit send controls can target attached connector endpoints.
- Mirroring is shown as endpoint policy, not inferred from transcript visibility.

---

## Non-Goals

This design does not implement:

- Full Discord connector behavior.
- Multi-user membership and permissions beyond endpoint send/receive flags.
- Multi-agent conversation routing.
- Full group-chat participant graphs.
- Multimodal connector payload delivery.
- External connector discovery outside attached endpoints.

These are compatible with the endpoint model but are outside this first design.

---

## Testing Contract

Implementation must follow test-first development.

Required tests:

- Store schema creation includes endpoint, policy, origin, and delivery tables/columns on fresh DBs.
- Store schema creation is idempotent for forward development setup.
- Default console endpoint is created or resolved for every newly created user-facing chat.
- Connector endpoint upsert is idempotent by external address.
- Disabled or non-active connector endpoints cannot be bypassed by resolver fallback.
- Endpoint listing returns console and connector endpoints for a chat.
- Delivery policy defaults to `reply_to_origin`.
- `explicit_only` writes to transcript/live events but skips connector dispatch unless explicit endpoints are named.
- Console-origin assistant replies do not dispatch to connector manager.
- Telegram-origin assistant replies dispatch only to the matching Telegram endpoint.
- Mirror-enabled endpoints receive explicit additional delivery.
- Combined reply-to-origin, mirror, and explicit endpoint targets are deduped by endpoint ID.
- `navi_message_deliveries` enforces `UNIQUE(message_id, endpoint_id)`.
- Delivery records increment attempts and settle to `sent`, `failed`, or `skipped` using the existing connector retry policy.
- Delivery service never calls `DispatchAll`.
- Telegram inbound resolves by durable endpoint and does not bind to the first public chat.
- Gateway rejects explicit sends to endpoints that do not belong to the chat.
- Gateway rejects sends to disabled or send-disabled connector endpoints.

Verification commands:

```bash
go test ./internal/navi/... -count=1
go test ./internal/gateway -count=1
go test ./plugins/telegram/connectors/telegram -count=1
go test ./cmd/... ./internal/... ./connectors/... ./plugins/... -count=1
```

---

## Acceptance Criteria

The implementation is complete when:

1. Conversation endpoints persist across restarts.
2. Every newly created user-facing chat has a default console endpoint.
3. Telegram external chats bind through durable endpoints instead of first-chat recovery.
4. Runtime replies use `reply_to_origin` delivery by default.
5. Console-origin messages do not leak to Telegram or future Discord endpoints.
6. Explicit endpoint sends can target one or more attached, send-enabled endpoints.
7. Endpoint lists are available through gateway APIs.
8. Delivery attempts are recorded and inspectable.
9. Delivery attempts are deduped by `(message_id, endpoint_id)`.
10. Tests cover store, gateway, delivery resolver, and Telegram binding behavior.
