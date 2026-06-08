# Slack Connector

> NAVI's built-in Slack connector — how it works, how to configure it, and the message lifecycle.

**Status:** Active  
**Last Updated:** 2026-05-18  
**Audience:** Users setting up Slack access; connector authors; workspace administrators  
**See also:** [Connectors README](README.md) · [Telegram Connector](telegram.md) · [Canonical Connector Spec](../canonical/specs/connectors.md) · [Gateway API](../specs/gateway-api.md) · [NAVI Runtime Config](../specs/configuration.md)

---

## Overview

The Slack connector is a **builtin connector** compiled into `navid` via `plugins/slack/`. It uses the Slack Bot API to bridge Slack DMs (and optionally channels) with NAVI sessions. Users interact with NAVI by messaging the installed Slack app.

The connector supports two Slack transport modes:

| Mode | Token required | Description |
|---|---|---|
| **Socket Mode** | Bot Token + App Token | Persistent WebSocket connection to Slack. No public URL required. Recommended for local and private deployments. |
| **Events API (webhook)** | Bot Token only | Slack POSTs events to a public HTTPS endpoint on the gateway. Requires a reachable URL. |

---

## Setup

### 1. Create a Slack app

1. Go to [api.slack.com/apps](https://api.slack.com/apps) and click **Create New App → From scratch**.
2. Name the app and select your workspace.

### 2. Configure OAuth scopes

Under **OAuth & Permissions → Bot Token Scopes**, add at minimum:

| Scope | Purpose |
|---|---|
| `chat:write` | Send messages as the bot |
| `im:history` | Read DM history (required for context) |
| `im:read` | View DM channels |
| `im:write` | Start DMs |
| `app_mentions:read` | Receive `@bot` mentions in channels |

Add `channels:history`, `channels:read`, `groups:history`, `groups:read` if you want the bot to participate in channels.

### 3. Enable Socket Mode (recommended)

1. Under **Socket Mode**, enable it and generate an **App-Level Token** with `connections:write` scope.
2. Copy the App Token (starts with `xapp-`).

### 4. Install to workspace

Under **Install App**, click **Install to Workspace** and authorize.

Copy the **Bot User OAuth Token** (starts with `xoxb-`).

### 5. Configure `runtime.yaml`

```yaml
connectors:
  slack:
    bot_token: "xoxb-..."            # Bot User OAuth Token
    app_token: "xapp-..."            # App-Level Token (Socket Mode only; omit for webhook mode)
    allowed_user_ids: []             # empty = all workspace members; restrict with Slack user IDs
    gateway_url: ""                  # auto-detected; override only in non-standard deployments
```

---

## Configuration Reference

| Field | Type | Required | Description |
|---|---|---|---|
| `bot_token` | string | ✅ | Slack Bot User OAuth Token (`xoxb-...`) |
| `app_token` | string | Socket Mode only | App-Level Token (`xapp-...`) — enables Socket Mode |
| `allowed_user_ids` | []string | | Slack user IDs permitted to interact with NAVI; empty = all workspace members |
| `gateway_url` | string | | Internal gateway URL used by the connector for session creation |

---

## Message Lifecycle

### Inbound (Slack → NAVI)

```
User sends a DM to the NAVI bot (or @mentions in a channel)
       ↓
Slack delivers the event (Socket Mode WebSocket, or Events API POST)
       ↓
Connector validates sender against allowed_user_ids (if configured)
       ↓
Connector calls InboundHandler callback with InboundMessage{
    ChatID:        "<slack-channel-id>",    // DM channel or public channel
    Content:       "<message text>",
    SessionID:     "<resolved or new session ID>",
    SourceChannel: "slack",
}
       ↓
NAVI session inbox receives the message
       ↓
CEO loop processes the message; produces a reply
```

### Outbound (NAVI → Slack)

```
CEO loop emits reply text
       ↓
Connector.Send(ctx, OutboundMessage{
    ChatID:  "<slack-channel-id>",
    Content: "<reply text>",
    // ParseMode is not used for Slack; formatting uses Slack's mrkdwn syntax
})
       ↓
Connector calls Slack chat.postMessage API
       ↓
Slack delivers the message to the user
```

**Formatting note:** Slack uses its own `mrkdwn` syntax, not Markdown. NAVI replies are sent as plain text by default. Slack will linkify URLs and handle basic formatting automatically.

---

## Socket Mode vs. Events API

### Socket Mode (recommended)

- No public URL required — ideal for local development and private deployments.
- Slack opens a persistent WebSocket connection to NAVI.
- The `app_token` field must be set.
- Does not require changes to firewall rules or DNS.

### Events API (webhook mode)

- Requires a publicly reachable HTTPS endpoint.
- Slack POSTs events to `POST /api/webhooks/slack` (or a configured URL).
- No `app_token` required.
- Better for production deployments behind a load balancer.

To use webhook mode, configure the **Event Subscriptions** URL in your Slack app to point to your gateway's public address:

```
https://your-navi-domain.example.com/api/webhooks/slack
```

Subscribe to the `message.im` event (and optionally `app_mention`) under **Subscribe to bot events**.

---

## Allowlist Model

The `allowed_user_ids` list accepts Slack member IDs (e.g., `U012AB3CD`). If the list is **empty**, all workspace members can interact with NAVI.

```yaml
allowed_user_ids:
  - U012AB3CD    # specific user
  - U098ZYX76    # another user
```

Messages from users not on the list are silently ignored — no reply is sent.

---

## Session Management

The Slack connector maps Slack `channel_id` values (DM or channel) to NAVI sessions. The session bootstrap flow mirrors the Telegram connector:

1. **Gateway readiness probe** — connector waits for gateway health before sending session create requests.
2. **Session creation with retry/backoff** — exponential backoff on gateway startup race conditions.
3. **Recovered-session reuse** — prior sessions for the same `channel_id` are resumed from SQLite.

---

## Error Handling

| Error | Meaning | Manager behavior |
|---|---|---|
| `ErrRateLimit` | Slack returned HTTP 429 or `ratelimited` error | Retry with exponential backoff; respect `retry_after` |
| `ErrTemporary` | Slack API returned a 5xx or transient error code | Retry up to 3× |
| `ErrNotRunning` | Connector stopped or socket disconnected | No retry; reconnect on next startup |
| `ErrSendFailed` | Permanent failure (invalid token, channel archived, etc.) | Log and surface in connector diagnostics |

Socket Mode disconnections trigger automatic reconnection with backoff. The connector's `IsRunning()` state reflects whether the WebSocket is currently connected.

---

## Required Slack App Permissions Summary

For a minimal personal NAVI assistant (DMs only):

```
Bot Token Scopes:
  chat:write
  im:history
  im:read
  im:write

(Socket Mode) App Token Scopes:
  connections:write
```

For a team deployment with channel support, add:

```
  app_mentions:read
  channels:history
  channels:read
  groups:history
  groups:read
```

---

## Diagnostics

```bash
# Health status
curl -H "X-API-Key: <key>" http://localhost:6284/api/health/connectors

# Detailed diagnostics
curl -H "X-API-Key: <key>" http://localhost:6284/api/diagnostics/connectors

# Recent errors from the Slack connector
curl -H "X-API-Key: <key>" http://localhost:6284/api/errors?component=slack
```

---

*See also: [Telegram Connector](telegram.md) · [Connectors README](README.md) · [Gateway API](../specs/gateway-api.md)*
