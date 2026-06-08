---
name: Discord Connector Development
overview: Implementation plan for a first-class Discord connector in NAVI, defining MVP scope, architecture, integration points, and future enhancements.
todos: []
isProject: false
---

# Discord Connector – Development Plan

---

## 1. MVP Scope

### 1.1 Supported Scenarios (MVP)

- **Inbound**:
  - Direct messages to the bot user.
  - Guild text channel messages that:
    - Mention the bot explicitly, or
    - Are in configured “NAVI channels”.
  - Basic replies to NAVI’s messages (for follow-ups).
- **Outbound**:
  - Text messages (up to Discord’s content limit).
  - Simple embeds (optional; can be v2 if complexity is high).
  - Message replies (using `message_reference`).
- **Session semantics**:
  - Map Discord DM/channel/thread to NAVI sessions consistently.
  - Track which Discord user/channel is associated with which NAVI session or directive.

### 1.2 Explicit Non-Goals (v2+)

- Slash commands and full command menus.
- Advanced embed building, rich cards, and components beyond simple buttons.
- Multi-shard and very large guild scaling.
- Complex per-guild configuration UI.

---

## 2. File and Package Structure

All code lives under `connectors/discord/`, following the existing connector patterns.

Proposed structure:

```text
connectors/
  discord/
    bot.go           # Core connector implementation (MVP)
    config.go        # Discord-specific config helpers (optional)
    mapping.go       # Discord↔NAVI message/session mapping helpers
    types.go         # Internal types (if needed)
    bot_test.go      # Unit tests for connector behavior
```

Key decisions:

- Keep **one primary implementation file** (`bot.go`) for lifecycle and IO.
- Isolate mapping and helper functions in `mapping.go` if they become non-trivial.
- Reuse shared patterns from `connectors/telegram` and `connectors/slack` for:
  - Gateway token resolution via the NAVI gateway.
  - Registration and health checks.

---

## 3. Config Schema and Settings

### 3.1 Config Struct

In `internal/config/config.go`, add:

- `DiscordConfig` struct with at least:
  - `BotToken string` – Discord bot token (may be resolved from secure storage).
  - `GatewayURL string` – NAVI gateway base URL (reuse pattern from Telegram/Slack).
  - `GatewaySecret string` – shared secret to obtain access tokens.
  - `AllowedGuildIDs []string` – optional allowlist of guilds.
  - `AllowedChannelIDs []string` – optional allowlist for channels.
  - `AllowedUserIDs []string` – optional allowlist for users.
  - `ReasoningChannelID string` – optional channel ID for “thinking” logs (v2).
  - Any additional flags for debugging (e.g. `APIBaseURL` override for tests).

In the top-level `Config`:

- Add `Discord DiscordConfig` and YAML tag `discord`.

### 3.2 Settings Persistence

- Decide whether to persist Discord-specific settings (e.g. bot token) via `internal/store` similar to Telegram and Slack.
- MVP can read tokens from config; v2 can add UI-driven onboarding and secret storage.

---

## 4. Connector Implementation

### 4.1 Struct and Interfaces

In `connectors/discord/bot.go`:

- Define a `Bot` struct that:
  - Embeds or composes with `BaseConnector` (for name and allowlist handling).
  - Holds config, HTTP client, Gateway WebSocket client, and a `running` flag.
  - Tracks mappings needed for sessions (e.g. channel ID, thread ID).
- Implement `connectors.Connector`:
  - `Name()` – e.g. `"discord"` or `"discord-{suffix}"` if multi-account is needed in future.
  - `Start(ctx)` – connect to Discord Gateway, subscribe to events, start loops.
  - `Stop(ctx)` – close Gateway and stop workers.
  - `Send(ctx, msg OutboundMessage)` – send text/replies via Discord REST API.
  - `IsRunning()` – thread-safe running check.

Optional interfaces for MVP:

- `**MessageLengthProvider`** – return Discord’s max content length so the Manager can split long messages.
- `**TypingCapable` (v2)** – implement if we want typing indicators.
- `**ReactionCapable` (v2)** – for lightweight message acknowledgement.

### 4.2 Lifecycle and Event Loop

- On `Start(ctx)`:
  - Resolve configuration and any persisted secrets.
  - Fetch a gateway URL and start a WebSocket session with the appropriate intents.
  - Start goroutines to:
    - Read events from Discord and route them into NAVI (gateway APIs).
    - Handle reconnects with backoff on errors.
- On `Stop(ctx)`:
  - Signal all goroutines to exit.
  - Close WebSocket cleanly.

### 4.3 Inbound Message Handling

Steps for inbound flow:

1. Receive raw Discord events via Gateway (e.g. `MESSAGE_CREATE`).
2. Filter to events relevant for NAVI:
  - DMs to the bot.
  - Channel messages in allowed guilds/channels that match mention rules.
3. Normalize into NAVI’s inbound message format:
  - Extract user ID, username, channel ID, guild ID, thread ID.
  - Derive session context (DM vs channel vs thread).
4. Deliver inbound messages to the NAVI gateway:
  - Use the same HTTP endpoints and patterns as Telegram/Slack (`/api/navi/.../message`, etc.).

### 4.4 Outbound Message Handling

When Manager calls `Send(ctx, OutboundMessage)`:

- Map `msg.Channel`, `msg.ChatID`, and any reply/thread fields to Discord parameters:
  - Target channel or DM.
  - `message_reference` for replies.
  - Thread ID (if we choose to support threads in MVP).
- Enforce message length limits via `MessageLengthProvider` and existing split logic.
- Issue REST API calls to Discord with proper rate-limit handling:
  - Respect the `Retry-After` header (v2 may centralize this).

---

## 5. Integration Points

### 5.1 Registry and Manager

- In `internal/connectors/registry.go`:
  - Register a factory: `RegisterFactory("discord", func(cfg *config.Config, b bus.Bus) (connectors.Connector, error) { ... })`.
- Ensure `internal/connectors/manager.go` will:
  - Recognize `MessageLengthProvider` for Discord.
  - Apply standard retry and backoff behavior.

### 5.2 Main Wiring

- In `cmd/navid/main.go`:
  - Register the Discord factory when `cfg.Discord.BotToken` is set (and/or other enabling config).
  - Include Discord in `startConnector` if there is a dynamic setup/onboarding path.
  - Ensure lifecycle hooks (`StartAll`, `StopAll`) apply to Discord as they do to Telegram/Slack.

---

## 6. Capabilities Matrix (MVP vs v2)


| Capability             | MVP | v2 / Future |
| ---------------------- | --- | ----------- |
| DMs                    | ✅   | –           |
| Guild text channels    | ✅   | –           |
| Threads                | ⚪   | ✅           |
| Message replies        | ✅   | –           |
| Message edits          | ⚪   | ✅           |
| Reactions              | ⚪   | ✅           |
| Typing indicators      | ⚪   | ✅           |
| Media attachments      | ⚪   | ✅           |
| Rich embeds/components | ⚪   | ✅           |
| Slash commands         | ❌   | ✅           |
| Multi-shard support    | ❌   | ✅           |


Legend: ✅ = in-scope, ⚪ = optional / nice-to-have, ❌ = explicitly out-of-scope.

---

## 7. Dependencies and Libraries

- **Preferred approach**:
  - Use a mature Go library for Discord (e.g. a well-maintained Gateway + REST client) if it reduces boilerplate and handles rate limits robustly.
  - Otherwise, implement minimal Gateway/REST logic with `net/http` and `gorilla/websocket` (or similar), to keep control over behavior.
- **Constraints**:
  - Avoid heavy, unmaintained, or highly opinionated frameworks.
  - Ensure the chosen library works well with context cancellation and does not spawn unmanaged goroutines.

---

## 8. v2 / Future Enhancements

Capture future work explicitly rather than bloating the MVP:

- Full **slash command** support with rich command menus and options.
- **Thread-aware sessions**, mapping Discord threads 1:1 to long-lived NAVI sessions.
- **Rich embeds and components** for better UX (buttons, dropdowns).
- **Typing indicators**, reactions, and placeholder messages, wired into the Manager’s orchestration.
- **Multi-shard scaling** and horizontal scaling patterns.
- **Guild-specific configuration** driven by persisted settings and UI (per-guild allowlists, default channels, admin roles).

---

## 9. Acceptance Criteria

The Discord connector MVP is considered implemented when:

1. NAVI can be configured with a Discord bot token and start the connector successfully.
2. DMs and allowed channel messages can reach NAVI and get responses.
3. Outbound responses are delivered reliably, with basic retry handling on transient failures.
4. The connector passes unit tests for message mapping and basic lifecycle.
5. The connector participates in the standard Manager lifecycle (`StartAll`, `StopAll`) without special-casing in the core.

