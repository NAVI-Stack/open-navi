# Chat Surface — Phase 2 Backend Spec

Phase 1 (done) adopted PET's chat look into the NAVI console: PET-style message
layout (assistant prose + Bot avatar + toolbar; right-aligned user bubble), rich
markdown rendering (`MarkdownRenderer`), code/message copy, typing indicator,
smart scroll + scroll-to-bottom FAB, streaming stop, and error/retry-as-resend.

These interactions are **frontend-only** and shipped because they need no new API.

## Status (Phase 2 implementation)
- ✅ **Feedback (§1)** — shipped (`692f3af`): metadata-backed, gateway endpoint, thumbs UI.
- ✅ **Edit & resend (§5)** — shipped (`83b738a`/`9ecd7b0`): truncate-and-rerun + inline edit UI.
- ✅ **Regenerate (§2)** — shipped (`debc7b1`): re-runs the last user turn via the truncate path.
- ⏳ **Continue (§3)** — deferred: needs runtime prefix-continuation (resume an assistant
  message with its current text as a prefix); not reducible to truncate-and-rerun.
- ⏳ **Variants (§4)** — pending: storage grouping + regenerate-appends-variant.
- ⏳ **Tool-invocation streaming (§6)** — pending: structured tool parts in messages/events.

Phase 2 covers the interactions from PET that are **blocked on backend support** —
navid's gateway currently exposes only `createChat`, `sendChatMessage` (`POST
/api/navi/chats/{id}/message`), `archive`, `rename`, the thread, and
`runtime_summary`. The features below have no endpoint yet. Build these in navid,
then wire the already-designed UI affordances.

## Conventions
- All routes are JWT-protected under `/api/navi/chats/...` (mirror existing chat routes).
- Message IDs are the persisted chat-message IDs returned in the thread.
- Reuse the runtime coordinator / `ExecuteRun` path; do not add a parallel engine.
- Honor governor limits; never fabricate success (CLAUDE.md "no fake success").

## 1. Assistant feedback (👍 / 👎)
- `POST /api/navi/chats/{chatId}/messages/{messageId}/feedback` → body `{ "rating": "up" | "down" | null }`.
- Persist on the chat message (add `feedback` column to the chat messages table in
  `internal/navi/store/`). Idempotent; `null` clears.
- Returns `{ messageId, rating }`.

## 2. Regenerate last assistant reply
- `POST /api/navi/chats/{chatId}/regenerate` (optionally `{ "messageId": "<assistant msg>" }`).
- Server: drop/supersede the target assistant message, re-run the runtime for the
  preceding user turn, stream the new reply over the existing live-events channel
  (`assistant.message.partial` / `.completed`). Store the prior reply as a variant
  (see §4) so it isn't lost.

## 3. Continue an assistant reply
- `POST /api/navi/chats/{chatId}/continue` (optionally `{ "messageId" }`).
- Server: re-enter the runtime with the existing assistant text as a prefix and
  instruction to continue; append streamed output to the same message.

## 4. Response variants
- Storage: assistant messages gain a `variants` group — either a `variant_group_id`
  + `variant_index` on the messages table, or a child `message_variants` table.
- `GET /api/navi/chats/{chatId}/messages/{messageId}/variants` → `{ variants: [{ id, index, content }], selectedIndex }`.
- `POST /api/navi/chats/{chatId}/messages/{messageId}/variants/select` → body `{ "index": n }`.
- Regenerate (§2) appends a new variant rather than discarding the old.

## 5. Edit user message and resend
- `POST /api/navi/chats/{chatId}/messages/{messageId}/edit-resend` → body `{ "content": "..." }`.
- Server: truncate the thread after the edited user message, replace its content,
  re-run the runtime. Returns the new thread tail (or streams via live events).
- This rewrites history — gate behind the owner and record an audit event.

## 6. Tool-invocation parts in the stream/thread
- Today `toUIMessage` drops `tool`/`system` roles and the thread carries only text.
- Extend persisted messages + live events to carry structured tool parts:
  `{ type: "tool-invocation", toolInvocationId, toolName, state: "call"|"result"|"partial-call", args?, result? }`.
- Emit `tool.call.started` / `tool.call.completed` live events (or embed parts in
  `assistant.message.partial`). The console already has a placeholder for rendering
  tool chips (PET's `Message.tsx` is the reference design).

## Frontend wiring (after each endpoint lands)
- Add API helpers in `web-src/navi-console/src/api/chats.ts`
  (`sendFeedback`, `regenerate`, `continueReply`, `getVariants`, `selectVariant`,
  `editAndResend`).
- Extend `ChatMessage.tsx`'s toolbar: 👍/👎 (assistant), Regenerate + Continue on the
  last assistant message, variant `‹ n/N ›` switcher, Edit (user). Render tool chips
  above assistant prose. Keep every control gated on real capability — no dead buttons.
- Add vitest coverage for each new control (mock the API), matching the Phase 1 test style.

## Acceptance
- Each control performs a real backend action and reflects persisted state on reload.
- Streaming-aware: regenerate/continue stream via the existing live-events channel.
- All new endpoints have Go tests in `internal/gateway`; all new UI has vitest tests.
