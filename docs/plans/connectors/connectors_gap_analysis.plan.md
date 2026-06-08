---
name: OpenClaw Connectors Migration
overview: Identify OpenClaw chat connectors missing in navi, then outline a migration plan to implement them as Go connectors in navi's existing connector/gateway architecture.
todos: []
isProject: false
---

# OpenClaw Chat Connectors Gap and Migration Plan

## 1. Connector inventory

**Navi (current):** Two Go connectors are shipped as plugin-owned implementations: **Telegram** in `plugins/telegram/connectors/telegram` and **Slack** in `plugins/slack/connectors/slack`. They are linked through `cmd/navid/plugin_bootstrap.go`, use shared interfaces from `connectors/`, and are configured via `internal/config/config.go`.

**OpenClaw extensions** (from [.example-code/openclaw/extensions](.example-code/openclaw/extensions)) that declare `"channels": ["..."]` in `openclaw.plugin.json` — i.e. chat/channel connectors:


| OpenClaw channel                              | Navi has? |
| --------------------------------------------- | --------- |
| telegram                                      | Yes       |
| slack                                         | Yes       |
| discord                                       | **No**    |
| bluebubbles (iMessage via BlueBubbles server) | **No**    |
| feishu (Lark)                                 | **No**    |
| whatsapp                                      | **No**    |
| signal                                        | **No**    |
| msteams                                       | **No**    |
| matrix                                        | **No**    |
| irc                                           | **No**    |
| line                                          | **No**    |
| googlechat                                    | **No**    |
| mattermost                                    | **No**    |
| nextcloud-talk                                | **No**    |
| nostr                                         | **No**    |
| synology-chat                                 | **No**    |
| twitch                                        | **No**    |
| zalo                                          | **No**    |
| imessage                                      | **No**    |


So there are **17 chat connectors** in OpenClaw that navi does not have.

---

## 2. Navi connector architecture (reference for migration)

- **Interface:** [connectors/connector.go](connectors/connector.go) — `Name()`, `Start()`, `Stop()`, `Send()`, `IsRunning()`.
- **Optional capabilities:** [connectors/capabilities.go](connectors/capabilities.go) — `WebhookHandler`, `MediaSender`, `TypingCapable`, `MessageEditor`, `ReactionCapable`, `PlaceholderCapable`, `MessageLengthProvider`.
- **Base:** [connectors/base.go](connectors/base.go) — `BaseConnector` for allowlist and name/running state.
- **Registry:** [internal/connectors/registry.go](internal/connectors/registry.go) — `RegisterFactory(name, func(cfg *config.Config, b bus.Bus) (Connector, error))`, `Create(name, cfg, bus)`.
- **Config:** [internal/config/config.go](internal/config/config.go) — add a `XxxConfig` struct and `Xxx` field on `Config` per connector.
- **Wiring:** `cmd/navid/plugin_bootstrap.go` registers built-in plugin connector factories; `cmd/navid/main.go` consumes those registrations during startup.

Connectors are **gateway clients**: they register with the gateway (`POST /api/connectors`), obtain a token, then call gateway APIs (e.g. `/api/navi/sessions/.../message`, `/api/directives/.../message`) to deliver inbound user messages and receive outbound delivery via the Manager. No TypeScript or OpenClaw SDK — pure Go.

---

## 3. Migration approach

- **Do not** port OpenClaw’s TypeScript plugin SDK or plugin runtime. Reimplement behavior in Go using existing navi patterns (see Telegram/Slack).
- Use OpenClaw extensions only as **reference** for:
  - Platform APIs (Discord API, WhatsApp Business API, Signal, etc.)
  - Auth and webhook/long-poll/WebSocket flows
  - Message format and threading/reactions where relevant
- For each new connector:
  1. Add `plugins/<name>/connectors/<name>/` (e.g. `plugins/discord/connectors/discord/bot.go`) implementing `connectors.Connector` (and optional capabilities), plus `plugins/<name>/plugin.yaml` with `kind: integration`.
  2. Add config in `internal/config/config.go` and YAML schema (e.g. `discord:` section).
  3. Register factory and `startConnector` branch in `cmd/navid/main.go`.
  4. Persist connector-specific settings in the same way as Telegram/Slack (e.g. `store.SetSetting` for tokens).

---

## 4. Suggested implementation order

Prioritize by reach and similarity to existing connectors:

**Tier 1 — High reach, similar to Telegram/Slack**

- **Discord** — Bot API + Gateway WebSocket; very common; [.example-code/openclaw/extensions/discord](.example-code/openclaw/extensions/discord) for reference.
- **WhatsApp** — Business/Cloud API or third-party; high demand; reference: [.example-code/openclaw/extensions/whatsapp](.example-code/openclaw/extensions/whatsapp).
- **Signal** — Often via signal-cli or similar; reference: [.example-code/openclaw/extensions/signal](.example-code/openclaw/extensions/signal).

**Tier 2 — Enterprise / common collaboration**

- **MS Teams** — Bot Framework / Graph; reference: [.example-code/openclaw/extensions/msteams](.example-code/openclaw/extensions/msteams).
- **Matrix** — HTTP + optional WebSocket; reference: [.example-code/openclaw/extensions/matrix](.example-code/openclaw/extensions/matrix).
- **Mattermost** — REST + WebSocket; reference: [.example-code/openclaw/extensions/mattermost](.example-code/openclaw/extensions/mattermost).

**Tier 3 — Regional / niche**

- **Feishu (Lark)** — reference: [.example-code/openclaw/extensions/feishu](.example-code/openclaw/extensions/feishu).
- **Line** — reference: [.example-code/openclaw/extensions/line](.example-code/openclaw/extensions/line).
- **Google Chat** — reference: [.example-code/openclaw/extensions/googlechat](.example-code/openclaw/extensions/googlechat).
- **Nextcloud Talk** — reference: [.example-code/openclaw/extensions/nextcloud-talk](.example-code/openclaw/extensions/nextcloud-talk).
- **IRC** — reference: [.example-code/openclaw/extensions/irc](.example-code/openclaw/extensions/irc).
- **Nostr** — reference: [.example-code/openclaw/extensions/nostr](.example-code/openclaw/extensions/nostr).
- **Synology Chat** — reference: [.example-code/openclaw/extensions/synology-chat](.example-code/openclaw/extensions/synology-chat).
- **Twitch** — reference: [.example-code/openclaw/extensions/twitch](.example-code/openclaw/extensions/twitch).
- **Zalo** — reference: [.example-code/openclaw/extensions/zalo](.example-code/openclaw/extensions/zalo).
- **Blue Bubbles / iMessage** — reference: [.example-code/openclaw/extensions/bluebubbles](.example-code/openclaw/extensions/bluebubbles), [.example-code/openclaw/extensions/imessage](.example-code/openclaw/extensions/imessage).

---

## 5. Per-connector migration steps (template)

For each connector (e.g. Discord):

1. **Config** — In `internal/config/config.go`: add `DiscordConfig` (e.g. bot token, app id, gateway URL, allowlist) and `Discord DiscordConfig` on `Config`; load from YAML (e.g. `discord:`).
2. **Connector package** — Add `plugins/discord/connectors/discord/bot.go` (and tests in `plugins/discord/connectors/discord/bot_test.go`):
  - Struct holding config, HTTP client, gateway token, running state.
  - `Name() string` (e.g. `"discord"`).
  - `Start(ctx)` — resolve token, register with gateway (`POST /api/connectors`), obtain JWT, start Discord Gateway WebSocket (or webhook) and forward inbound messages to gateway (e.g. `POST /api/navi/sessions/.../message` or equivalent).
  - `Stop(ctx)` — close WebSocket, unregister if desired.
  - `Send(ctx, msg OutboundMessage)` — call Discord REST API to send to `msg.ChatID` (channel/DM id).
  - `IsRunning() bool`.
  - Optionally implement `WebhookHandler`, `MediaSender`, `MessageLengthProvider` from [connectors/capabilities.go](connectors/capabilities.go) if the platform supports them.
3. **Registry** — In `cmd/navid/plugin_bootstrap.go`: `connectorRegistry.RegisterFactory("discord", func(cfg *config.Config, b bus.Bus) (pkgconn.Connector, error) { ... })`.
4. **Lifecycle** — In `startConnector`, add `case "discord":` only for runtime lifecycle behavior that cannot be expressed by the existing manager path; persist tokens/settings, `connectorRegistry.Create("discord", cfg, natsBus)`, `connectorMgr.StartOne(ctx, "discord")`.
5. **Docs** — Add minimal config/runtime notes under `documentation/` (per project rules).

Repeat the same pattern for each of the 17 connectors, using OpenClaw extension code only as API/flow reference.

---

## 6. Dependency and platform notes

- **Discord** — Go SDK or REST + Gateway WebSocket; bot token + optional app id.
- **WhatsApp** — Meta Cloud API (webhook + REST) or third-party (e.g. Baileys); often requires verified business and webhook URL.
- **Signal** — No official HTTP API; use signal-cli or libraries that wrap it; may require phone number / registration.
- **MS Teams** — Bot Framework (Azure) or Graph API; app registration and webhook endpoints.
- **Matrix** — REST API + optional sync; access token or login.
- **Blue Bubbles / iMessage** — Depends on BlueBubbles server (macOS); REST to that server.

New Go dependencies (e.g. Discord SDK, Matrix client) should be added only when needed; prefer minimal REST + stdlib where sufficient.

---

## 7. Summary

- **Gap:** 17 OpenClaw chat channels (Discord, WhatsApp, Signal, MS Teams, Matrix, Feishu, Line, Google Chat, Mattermost, Nextcloud Talk, IRC, Nostr, Synology Chat, Twitch, Zalo, Blue Bubbles, iMessage) are not in navi.
- **Strategy:** Implement each as a Go connector under `plugins/<name>/connectors/<name>/`, following `plugins/telegram/connectors/telegram` and `plugins/slack/connectors/slack`; config remains in `internal/config` when global runtime config is required; built-in registration belongs in `cmd/navid/plugin_bootstrap.go`; use OpenClaw extensions only as API/flow reference.
- **Order:** Start with Discord (and optionally WhatsApp and Signal), then MS Teams/Matrix/Mattermost, then the rest.

No code or config changes are applied in this plan; it only identifies the gap and outlines the migration steps.
