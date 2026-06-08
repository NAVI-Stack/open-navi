# Connector setup (generic flow)

**Purpose:** Add and configure connectors (e.g. Telegram, Slack) using a single metadata-driven path. The server defines required/optional params and labels via **`GET /api/connectors/setup-schema`**; the CLI and `navi init` use that schema for prompts. There is no Telegram-only or Slack-only built-in flow.

**After an instance reset** (API or manual), connector config is wiped. Re-add connectors by running `navi init` and choosing a connector type when prompted, or in `navi chat` run **`/connect <type>`** (e.g. `/connect telegram`, `/connect slack`) and enter the credentials the server prompts for.

---

## How setup works

1. **Server** exposes connector setup descriptors at **`GET /api/connectors/setup-schema`** (auth required). Each descriptor includes `type`, `display_name`, `required_params`, `optional_params`, and `setup_hint`. Params have `key`, `label`, `description`, `secret`, etc.
2. **CLI** (e.g. `/connect telegram` or `navi init` connector step) fetches the schema, finds the descriptor for the chosen type, and prompts for each required/optional param using the server’s labels and hints.
3. **CLI** sends **`POST /api/setup/connector`** with `type` and `params`. The server validates (required params, format where applicable), persists config, creates the connector instance, and starts it.

So: **connector setup is server-driven.** Supported types and their params are defined on the server; the CLI does not hard-code connector-specific prompts.

---

## API

### GET /api/connectors/setup-schema

Returns a JSON array of **setup descriptors**. Example shape (per item):

- `type` — connector type (e.g. `telegram`, `slack`)
- `display_name` — human-readable name
- `required_params` — list of `{ key, label, description?, secret, placeholder?, validation_hint? }`
- `optional_params` — same shape
- `setup_hint` — one-line hint for the user

### POST /api/setup/connector

**Auth:** Required (API key or token).

**Body:** `{ "type": "<connector type>", "params": { "<key>": "<value>", ... } }`

The server rejects unsupported types, missing required params, and invalid param values. Persists settings, creates the connector via the registry, and starts it.

**Example (Telegram):**

```json
{
  "type": "telegram",
  "params": {
    "bot_token": "<from @BotFather>",
    "owner_chat_id": "<from @userinfobot>"
  }
}
```

Telegram onboarding is intentionally kept minimal in the live setup schema: it prompts for the bot token and chat ID only. Advanced fields such as `account`, `allow_from`, `pairing_code`, `gateway_url`, `api_url`, `webhook_url`, and `webhook_secret` are still supported by the backend and can be added later through follow-up configuration.

---

## Verification

1. **After setup:** `GET /api/connectors` (with auth) should list the connector (e.g. `telegram` or `telegram-<account>`) with status `connected` or `disconnected`.
2. **Schema:** `GET /api/connectors/setup-schema` returns the descriptors the CLI uses for prompts.
3. **Process log:** Look for connector start messages in navid logs.

If the connector does not appear after setup, confirm that `POST /api/setup/connector` was called with the correct `type` and required `params` for that type (as defined by the server’s setup schema).
