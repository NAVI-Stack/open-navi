# Telegram Connector — Upgrade Inventory

> Status: Inventory / proposal. No code changes yet.
> Source of comparison: `example-code/openclaw/extensions/telegram` (TypeScript reference).
> NAVI target: `plugins/telegram/connectors/telegram/bot.go`.

This document catalogs capabilities present in the openclaw Telegram extension that
are missing or thin in NAVI's connector, and proposes a phased synthesis order.
It is the basis for the Phase A implementation plan (drafted separately).

---

## 1. Baseline — what NAVI already does well

These are at parity (or ahead) and need **no** action:

- Long-polling **and** webhook intake (`HandleWebhook`, `setWebhook`/`deleteWebhook`).
- WebSocket live streaming (`/ws/live`) with partial → final message reconciliation
  (`renderSessionPartial`, `finalizeSessionMessage`).
- Completion-repair watchdog for dropped terminal events (`startSessionTerminalWatchdog`,
  `scheduleCompletionRepair`).
- Native typing indicator (`StartTyping` → `sendChatAction`).
- HITL + proposal approval inline buttons (`/hitl`, `proposal.waiting`, callback routing).
- Built-in commands: `/navi`, `/new`, `/use`, `/list`, `/implement`, `/status`,
  `/model`, `/hitl`, `/pair`, `/start`.
- Multi-chat → NAVI-session binding with focus recovery on restart.
- Message splitting at the 4096 limit; `parse_mode` passthrough.
- Inbound media **metadata** parsing (`parseMediaMetadata`).
- Per-error connector telemetry (`recordConnectorError`).

---

## 2. Gap inventory

Each item lists: openclaw source, the NAVI gap, and a short rationale.

### Tier 1 — High value, clean fit

#### T1-1. Group mention gating (`requireMention`)
- **openclaw:** `src/bot-access.ts`, `src/group-config-helpers.ts`, per-group
  `TelegramGroupConfig.requireMention`, wildcard `"*"` group defaults.
- **NAVI gap:** Replies to *every* message from an allowed chat; no group vs. DM
  distinction. In a group this means NAVI answers all traffic.
- **Why:** Hard blocker for group usability. A bot that responds to every line is
  unusable in a shared chat.

#### T1-2. Reply / quote + forward context
- **openclaw:** `src/bot-message-context.session.ts` (`describeReplyTarget`,
  `normalizeForwardedContext`); injects `[Quoting …]…[/Quoting]`,
  `[Replying to …]`, `[Forwarded from …]` into the agent-visible body.
- **NAVI gap:** `tgMessage.ReplyToMessage` is parsed into the struct but discarded;
  the agent never sees quoted or forwarded content.
- **Why:** Threaded replies lose their referent; the agent answers blind.

#### T1-3. Sender attribution in groups
- **openclaw:** `buildSenderLabel`, `buildSenderName`, `buildGroupLabel`,
  `formatInboundEnvelope`.
- **NAVI gap:** Sends raw text with no "who said it" envelope.
- **Why:** In a group the agent cannot distinguish speakers without attribution.

#### T1-4. Slash-command menu registration (`setMyCommands`)
- **openclaw:** `src/bot-native-command-menu.ts`, `src/bot-native-commands.ts`
  (command specs, arg menus, per-account command config).
- **NAVI gap:** Commands live in a hardcoded `switch`; never registered with
  Telegram, so no client-side autocomplete/menu.
- **Why:** Low effort, high polish; discoverability of commands.

#### T1-5. Complete media ingestion (download + deliver) — BLOCKED (platform gap)
- **openclaw:** `src/bot-handlers.media.ts`, `src/bot.media.*`,
  `getFile` → download → deliver (incl. voice transcription, stickers, fragments).
- **NAVI gap:** `parseMediaMetadata` is explicitly "Phase 1: parse but don't ingest";
  `handleUpdate` only forwards **text** via `queueSessionMessage`. Inbound media is
  effectively **dropped**.
- **Gate result (investigated):** The entire inbound pipeline is text-only and
  **cannot carry media today**:
  - Gateway `POST /api/navi/chats/{id}/message` decodes only `content` /
    `source_channel` / `source_message_ref` / `idempotency_key` (`server.go` ~2536).
  - `runtime.MessageInput` and `InboxItem` (`internal/runtime/inbox.go`) are text-only.
  - `llm.Message.Content` is a plain `string` (`internal/llm/types.go:8`); **no
    multimodal content blocks and no image encoding in any provider adapter**.
  - No speech-to-text, so voice notes cannot be converted to text either.
  - (A `LinkedFile`/blob model and `internal/blob` store exist, but nothing wires
    inbound chat media into the LLM prompt.)
- **Verdict:** True media ingestion is a **cross-cutting platform initiative**
  (multimodal `llm.Message` + per-provider image encoding + attachment plumbing
  through gateway → intake → inbox + blob wiring), not a Telegram-connector change.
  **Deferred** until that lands.
- **Connector-local stopgap (in reach now):** surface media *presence* + caption /
  filename into the text envelope (e.g. `[User attached a photo — caption: "…"]`)
  so the agent acknowledges media instead of silently dropping it. Honest, cheap,
  and consistent with NAVI's "not-yet-executable" philosophy.

#### T1-6. `allowed_updates` tuning
- **openclaw:** `src/allowed-updates.ts` opts into `message_reaction`,
  `channel_post`, `my_chat_member`, `chat_member`, `chat_join_request`, etc.
- **NAVI gap:** Uses default `getUpdates`; handles only message / edited_message /
  channel_post / callback_query.
- **Why:** Foundation for reactions (T3-13) and membership events (T2-11).

### Tier 2 — Strong value, more design work

#### T2-7. Forum topic awareness + topic-scoped sessions
- **openclaw:** `src/action-threading.ts` (`resolveTelegramAutoThreadId`),
  `src/auto-topic-label.ts` (LLM-generated topic labels), `TelegramTopicConfig`.
- **NAVI gap:** Passes `MessageThreadID` through but has no topic → session routing
  and no auto-labeling.
- **Why:** Forum supergroups want one NAVI chat per topic.

#### T2-8. Multi-account support — ALREADY PRESENT (parity)
- **openclaw:** `src/accounts.ts`, `src/account-config.ts`, `src/account-selection.ts`.
- **NAVI status:** Already implemented. `config.TelegramConfig.Accounts`
  (`[]TelegramAccountConfig`) + `config.ResolveTelegramAccounts` normalize named
  accounts (dedupe names/tokens, legacy single-account fallback), and
  `TelegramConnectorName` maps each to a `telegram` / `telegram-<slug>` connector
  registered by `registerTelegramAccountFactory` in `cmd/navid/plugin_bootstrap.go`.
- **Remaining delta vs. openclaw:** token-file resolution exists, but the richer
  *account inspection* with credential status (T2-10) and per-account group config
  do not. Treat T2-8 as done; fold residual work into T2-10 and the group-config surface.

#### T2-9. Richer approval delivery
- **openclaw:** `src/approval-native.ts`, `src/approval-handler.runtime.ts`,
  `src/exec-approvals.ts` — approver allowlist, DM-to-approver, origin notification.
- **NAVI gap:** Inline approve/reject shown to whoever is in the chat; no approver
  restriction or DM routing.
- **Why:** `ACT`-mode safety — only designated approvers should authorize actions.

#### T2-10. Token sourcing + account inspection
- **openclaw:** `src/account-inspect.ts` — resolves token from env / tokenFile /
  config with `available` / `configured_unavailable` / `missing` status.
- **NAVI gap:** Takes a raw token string; no status reporting.
- **Why:** Surfaces misconfiguration during onboarding instead of at runtime.

#### T2-11. Membership / security audit
- **openclaw:** `src/audit.ts`, `src/audit-membership-runtime.ts`,
  `security-audit-contract-api.ts`.
- **NAVI gap:** None.
- **Why:** Verify the bot is actually a member of configured groups; warn on
  wildcard "unmentioned" groups. Diagnostic value.

### Tier 3 — Nice-to-have / smaller polish

| ID | Capability | openclaw source | NAVI gap |
|----|-----------|-----------------|----------|
| T3-12 | `allowFrom` by username (not just numeric ID) | `bot-access.ts` (`normalizeAllowFrom`, `isSenderAllowed`) | NAVI matches numeric chat IDs only |
| T3-13 | Message reactions as input/output | `message_reaction` handling | NAVI ignores reactions |
| T3-14 | Reply-to modes + message pin | `ReplyToMode`, channelData `pin` | **Deferred** — reply-threading touches the delicate streaming/placeholder path for modest group-only gain; pin has no trigger in NAVI |
| T3-15 | Markdown rendering (code/tables/bold) | `markdown-table-runtime`, parse_mode | **Implemented** — see below |
| T3-16 | Implicit mention (reply-to-bot counts as mention) | `bot-message-context.implicit-mention` | N/A until T1-1 exists |
| T3-17 | Silent ingest (record context, no reply) | `bot-message-context.silent-ingest` | NAVI always replies |
| T3-18 | Group pending-history context window | `buildPendingHistoryContextFromMap` | NAVI sends a single message, no recent-group-history window |

---

## 3. Recommended synthesis order

- **Phase A — make groups usable:** T1-6 → T1-1 → T1-3 → T1-2. **(Implemented.)**
  Adds a `channels.telegram.groups` config map (`require_mention`, `enabled`; key
  `"*"` = default, specific chat ID overrides), `getMe`-based bot-username capture,
  mention/reply-to-bot gating in groups, explicit `allowed_updates`, and a wrapped
  inbound envelope (sender + reply + forward context) for group messages. DMs are
  unchanged. Reactions/member events remain out of scope until their tiers.
- **Phase B — richer I/O:** T1-4 **(implemented)** + T1-5 stopgap **(implemented)**;
  T3-14 / T3-15 remain. Full T1-5 ingestion **deferred** (platform multimodal work).
  - *Implemented:* `setMyCommands` registration on startup (client-side command menu;
    `/pair` listed only when pairing is enabled), and a media-presence stopgap —
    inbound photos/voice/docs/etc. now surface as a bracketed `[User attached …]`
    note in the message body (DMs and groups) instead of being silently dropped.
  - *T3-15 implemented:* outbound chat replies now render markdown → Telegram HTML
    (`plugins/telegram/connectors/telegram/formatting.go`): fenced code → `<pre>`,
    inline code → `<code>`, GitHub tables → aligned `<pre>`, `**bold**`, links, and
    ATX headings → `<b>`. `&<>` are escaped. Italic/strikethrough are intentionally
    not converted (too many false positives on `snake_case`). A parse-error fallback
    resends as plain text, so a bad render never blocks delivery. Streaming partials
    stay plain; the HTML render is applied when the reply is finalized.
  - *T3-14 deferred:* reply-threading would thread NAVI's answer to the triggering
    message but requires plumbing a message ID through the streaming/placeholder/
    watchdog machinery (regression risk, modest group-only benefit); message pin has
    no automatic trigger in NAVI. Revisit if a concrete need appears.
- **Phase C — scale & topics:** T2-7 → T2-8 → T2-9 → T2-10 / T2-11.

Tier 3 items slot in opportunistically alongside their related phase
(e.g. T3-16 / T3-18 with Phase A, T3-13 with Phase B).

---

## 4. Open questions to resolve before building

1. **Media gap (T1-5): investigated — BLOCKED.** The gateway/intake/LLM pipeline is
   text-only with no multimodal support (see T1-5 gate result). True ingestion is a
   platform initiative, deferred. Connector-local stopgap = surface media presence +
   caption/filename as text.
2. **Multi-account (T2-8): resolved — already implemented** via
   `config.ResolveTelegramAccounts` + `registerTelegramAccountFactory`. No rebuild needed.
3. **Group config surface:** Where do per-group settings (`requireMention`, allowlists,
   topic config) live in NAVI's config model? openclaw uses a nested
   `channels.telegram.groups` map; NAVI has no equivalent yet.
