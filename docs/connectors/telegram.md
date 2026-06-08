# Telegram Connector

> NAVI's built-in Telegram connector — how it works, how to configure it, and how the message lifecycle flows.

**Status:** Active  
**Last Updated:** 2026-05-18  
**Audience:** Users setting up Telegram access; connector authors; integration engineers  
**See also:** [Connectors README](README.md) · [Canonical Connector Spec](../canonical/specs/connectors.md) · [Gateway API](../specs/gateway-api.md) · [NAVI Runtime Config](../specs/configuration.md)

---

## Overview

The Telegram connector is a **builtin connector** compiled into `navid` via `plugins/telegram/`. It uses the Telegram Bot API (long-poll by default, or webhook) to bridge Telegram chats and NAVI sessions. Users interact with NAVI through a private Telegram chat; NAVI responds as the bot.

The connector is **identity-aware**: only the configured `owner_chat_id` (and any additional `allow_from` IDs) can initiate sessions. All other senders are silently dropped.

---

## Setup

### 1. Create a Telegram bot

1. Open Telegram and start a chat with **@BotFather**.
2. Send `/newbot` and follow the prompts to choose a name and username.
3. Copy the **bot token** (format: `123456789:ABCdef...`).

### 2. Get your chat ID

1. Start a chat with **@userinfobot** (or send any message to your new bot and check the Telegram API response).
2. Note your numeric **chat ID** (e.g., `987654321`).

### 3. Configure `runtime.yaml`

**Single-account setup (most users):**

```yaml
connectors:
  telegram:
    bot_token: "123456789:ABCdef..."   # or use token_file for secrets management
    owner_chat_id: 987654321
    allow_from: []                      # empty = only owner; add extra IDs for multi-user
```

**Multi-account setup (advanced):**

```yaml
connectors:
  telegram:
    accounts:
      - name: personal
        bot_token: "111:AAA..."
        owner_chat_id: 111111111
      - name: work
        bot_token: "222:BBB..."
        owner_chat_id: 222222222
        allow_from: [333333333, 444444444]
```

Each account creates a separately named connector instance (`telegram`, `telegram-work`, etc.) that appears in connector health and diagnostics.

**Using a token file (recommended for production):**

```yaml
connectors:
  telegram:
    token_file: /run/secrets/telegram-token
    owner_chat_id: 987654321
```

The `token_file` path is read at startup. The file must contain only the token string with no trailing whitespace.

---

## Configuration Reference

| Field | Type | Required | Description |
|---|---|---|---|
| `bot_token` | string | ✅ (or `token_file`) | Telegram Bot API token from @BotFather |
| `token_file` | string | ✅ (or `bot_token`) | Path to a file containing the bot token |
| `owner_chat_id` | int64 | ✅ | Numeric Telegram chat ID of the owner; all sessions are scoped to this identity |
| `allow_from` | []int64 | | Additional numeric chat IDs permitted to send messages |
| `pairing_code` | string | | One-time pairing code for a new instance bootstrap flow |
| `gateway_url` | string | | Internal gateway URL used by the connector to create sessions (default: auto-detected) |
| `api_url` | string | | Telegram API base URL (default: `https://api.telegram.org`; override for local testing) |
| `webhook_url` | string | | If set, registers this URL as a Telegram webhook instead of using long-polling |
| `webhook_secret` | string | | Shared secret sent in the `X-Telegram-Bot-Api-Secret-Token` header for webhook validation |

---

## Message Lifecycle

### Inbound (Telegram → NAVI)

```
User sends message in Telegram chat
       ↓
Telegram Bot API delivers update (long-poll or webhook POST /webhooks/telegram)
       ↓
Connector validates sender against owner_chat_id / allow_from
       ↓
Connector calls InboundHandler callback with InboundMessage{
    ChatID:    "<telegram-chat-id>",
    Content:   "<message text>",
    SessionID: "<resolved or new session ID>",
    SourceChannel: "telegram",
}
       ↓
NAVI session inbox receives the message
       ↓
CEO loop processes the message; produces a reply
```

### Outbound (NAVI → Telegram)

```
CEO loop emits reply text
       ↓
Connector.Send(ctx, OutboundMessage{
    ChatID:    "<telegram-chat-id>",
    Content:   "<reply text>",
    ParseMode: "Markdown",  // or "MarkdownV2", "HTML", or "" for plain text
    ReplyToMessageID: <optional>,
    MessageThreadID:  <optional, for topics>,
})
       ↓
Connector calls Telegram sendMessage API
       ↓
Telegram delivers the message to the user
```

**ParseMode notes:**

- NAVI defaults to `Markdown` for formatted replies.
- Use `MarkdownV2` only if you need nested formatting — it requires more escaping.
- Use `HTML` for rich formatting with explicit tags.
- Use empty string for plain text with no formatting.

The connector validates `ParseMode` before sending and returns `ErrSendFailed` for unrecognized values.

---

## Session Management

The Telegram connector maps Telegram `chat_id` values to NAVI sessions. Session bootstrap follows this sequence:

1. **Gateway readiness probe** — connector waits for the gateway to be healthy before sending session create requests.
2. **Session creation with retry/backoff** — if the session create request fails (e.g., gateway still starting), the connector retries with exponential backoff.
3. **Recovered-session reuse** — if a prior session for the same `chat_id` exists in SQLite (non-expired), the connector resumes it rather than creating a new one.
4. **SQLite pool expansion** — WAL read slots are expanded to accommodate concurrent readers during high-frequency message bursts.

---

## Multi-Model Switching

Users can switch the active LLM model from within Telegram using the `/model` command:

```
/model ollama/llama3.2
/model anthropic/claude-opus-4-6
```

The connector normalizes `_set_active` arguments and forwards them to the LLM management API. Model catalog refreshes (for Ollama) are triggered automatically on model-switch commands that reference unknown model names.

---

## Error Handling

The connector returns classified errors so the connector manager can apply the correct retry strategy:

| Error | Meaning | Manager behavior |
|---|---|---|
| `ErrRateLimit` | Telegram returned HTTP 429 | Retry with exponential backoff; respect `retry_after` header |
| `ErrTemporary` | Telegram returned HTTP 5xx | Retry up to 3× with short delay |
| `ErrNotRunning` | Connector is stopped | No retry; drop outbound message |
| `ErrSendFailed` | Permanent send failure (4xx, bad token, etc.) | Log and surface in connector diagnostics |

Errors are recorded via `SaveErrorRecord` to the structured error log (`GET /api/errors`).

---

## Webhook Mode

For production deployments where long-polling is undesirable, the connector supports webhook mode:

1. Set `webhook_url` in `runtime.yaml` to a publicly reachable HTTPS URL.
2. Optionally set `webhook_secret` — the connector verifies the `X-Telegram-Bot-Api-Secret-Token` header on each POST.
3. NAVI registers the webhook with Telegram at startup via `setWebhook` API call.
4. Inbound updates arrive at `POST /webhooks/telegram` on the gateway (no auth middleware — the connector validates the secret internally).

**Note:** Webhook mode requires a valid TLS certificate. Self-signed certificates are not accepted by Telegram.

---

## Allowlist Model

The `allow_from` list accepts Telegram user IDs (integers) or `@username` strings. If the list is empty, **only the `owner_chat_id`** may interact with NAVI. This is the recommended default for personal deployments.

```yaml
allow_from:
  - 987654321    # numeric ID
  - "@myalias"   # username (resolved at runtime)
```

Messages from senders not on the allowlist are silently dropped. No reply is sent. This is intentional — responding to unsolicited senders would leak bot presence.

---

## Diagnostics

```bash
# Health status
curl -H "X-API-Key: <key>" http://localhost:6284/api/health/connectors

# Detailed diagnostics (per-connector error counts, last error, uptime)
curl -H "X-API-Key: <key>" http://localhost:6284/api/diagnostics/connectors

# Recent errors
curl -H "X-API-Key: <key>" http://localhost:6284/api/errors?component=telegram
```

---

*See also: [Slack Connector](slack.md) · [Connectors README](README.md) · [Gateway API](../specs/gateway-api.md)*
