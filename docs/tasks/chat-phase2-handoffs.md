# Chat Phase 2 — Handoff Prompts (continue · variants · tool-invocation streaming)

Three self-contained prompts for delegating the remaining deep Phase 2 chat
features. Each is written to be pasted to a fresh agent with no prior context.

## Shared context (included in each prompt)

Repo: `C:\Users\evirg\codespace\NAVI-Ecosystem\projects\navi`, branch
`fix/console-plugins`. **Read `CLAUDE.md` first (mandatory).** Hard rules: raw SQL
only in `internal/navi/store/` (no ORM); `internal/schema/` is the source of truth
(run `make generate-python` if you change it); never fake success; write tests
(Go + vitest); prefer the existing **optional-interface** pattern for new ChatStore
capabilities (see `chatFeedbackStore`/`chatTruncateStore` in `internal/navi/navi.go`).

Phase 1 (PET-style chat surface) and three Phase 2 features (feedback, edit-resend,
regenerate) are already shipped. Spec + status: `docs/design/chat-surface-phase2-backend.md`.

Key plumbing already in place (study these as the template):
- **Store**: `internal/navi/store/chat_store.go`. Table `navi_chat_messages`
  (`message_id, chat_id, role, content, created_at, run_id, runtime_session_id,
  message_kind, metadata_json`) in `internal/navi/store/schema.go`. Existing methods:
  `AppendChatMessage`, `ListChatMessages`, `GetChatWithMessages`,
  `DeleteChatMessagesFrom`, `SetChatMessageFeedback`. `metadata_json` round-trips
  through `ChatMessage.Metadata` (`internal/navi/chat_message.go`) — use it to avoid
  migrations where possible.
- **ChatStore interface**: `internal/navi/chat_store.go`.
- **NAVI methods**: `internal/navi/navi.go` — `SendMessageInput` (the turn-execution
  entry: → `MessageIntakeService.SubmitMessage` → runtime coordinator),
  `EditAndResendMessage`, `RegenerateLastReply`, `SetMessageFeedback`. New capabilities
  use an optional interface asserted off `n.chatStore()`.
- **Runtime**: turn execution runs through the coordinator/`ExecuteRun`
  (`internal/navi/runtime_executor.go`); replies stream as live events
  `assistant.message.partial` / `assistant.message.completed` (see the consumer in
  `web-src/navi-console/src/pages/ChatPage.tsx` `handleLiveEvent`).
- **Gateway**: routes registered in `internal/gateway/server.go` `routes()` (~line 314,
  the `/api/navi/chats/...` block); handlers live in `internal/gateway/chat_feedback.go`
  and `chat_edit.go`; use `replyJSON` / `replyErrorAPI` helpers.
- **Frontend**: `web-src/navi-console`. Message UI: `src/components/chat/ChatMessage.tsx`
  (toolbar already has copy/edit/regenerate/feedback — add new controls here, no dead
  buttons). API helpers: `src/api/chats.ts`. Wiring: `src/pages/ChatPage.tsx`
  `ExistingChatView`. Types: `src/types/api.ts` (`ChatMessageSchema` is `.passthrough()`
  with `metadata`). Tests: `npx vitest run`; typecheck `npx tsc --noEmit`; Go
  `go test ./internal/navi/store/ ./internal/gateway/ -count=1`.

---

## Prompt A — "Continue" (resume an assistant reply)

```
Implement the "Continue" chat interaction in the NAVI console end-to-end. [PASTE SHARED CONTEXT ABOVE]

Goal: let the owner extend the most recent assistant reply with more output, as in
ChatGPT/Claude "Continue generating".

Design:
- Backend: add NAVI.ContinueLastReply(ctx, chatID). Unlike regenerate, this must NOT
  drop the prior reply — it resumes generation using the existing assistant text as a
  prefix. Implement by re-running the turn through the coordinator with the prior
  assistant content injected as an assistant-prefix / continuation instruction, then
  APPENDING the new tokens to the same assistant message (preferred) or as a follow-on
  assistant message clearly linked to it. Stream via the existing
  assistant.message.partial/.completed live events so the UI updates live.
  - Investigate runtime_executor.go / the coordinator to find the cleanest injection
    point for a continuation prefix. If the runtime cannot resume an existing message,
    add a minimal, well-tested capability rather than faking it.
- Gateway: POST /api/navi/chats/{id}/continue (mirror handleNaviRegenerate in chat_edit.go).
- Frontend: add a "Continue" control next to Regenerate on the last assistant message in
  ChatMessage.tsx (only when not streaming); api/chats.ts continueLastReply(chatId);
  wire in ChatPage ExistingChatView (reseed/refresh after).
- Tests: Go store/runtime test for the prefix-continuation path; vitest for the Continue
  control. Run make test (focus internal/navi, internal/gateway), npx vitest run, npx tsc.

Acceptance: clicking Continue extends the last reply with coherent additional content
that persists and reloads correctly; no duplicate/garbled history; all suites green.
```

## Prompt B — "Response variants"

```
Implement assistant response variants in the NAVI console end-to-end. [PASTE SHARED CONTEXT ABOVE]

Goal: when the owner regenerates an assistant reply, keep the previous reply as a
selectable variant, with a "‹ n of N ›" switcher on the message.

Design:
- Storage: group variants of the same assistant turn. Prefer adding variant_group_id +
  variant_index to navi_chat_messages (forward-only ALTER TABLE migration in
  internal/navi/store/schema.go guarded for existing DBs), or a child table — justify
  the choice. Keep raw SQL.
- Regenerate integration: change RegenerateLastReply (internal/navi/navi.go) so the
  prior assistant reply is preserved as a variant in the same group instead of being
  truncated away; the new reply becomes the selected variant.
- Store methods: ListMessageVariants(ctx, chatID, messageID) -> {variants:[{id,index,content}], selectedIndex};
  SelectMessageVariant(ctx, chatID, messageID, index). Expose via NAVI + optional interface.
- Gateway: GET /api/navi/chats/{id}/messages/{messageId}/variants and
  POST .../variants/select (body {index}).
- Frontend: api/chats.ts (getVariants, selectVariant); ChatMessage.tsx shows a
  ‹ index/total › switcher on assistant messages with >1 variant and swaps content on
  select; wire in ChatPage. The thread/GetChatWithMessages must return the selected
  variant as the visible content.
- Tests: Go store tests (regenerate appends variant; select switches); vitest for the
  switcher. Run go test ./internal/navi/... ./internal/gateway/..., npx vitest run, npx tsc.

Acceptance: regenerate stacks a variant (old reply not lost); switcher navigates and
persists selection across reload; counters/threads stay consistent; suites green.
```

## Prompt C — "Tool-invocation streaming"

```
Surface tool invocations in the NAVI console chat end-to-end. [PASTE SHARED CONTEXT ABOVE]

Goal: when NAVI calls tools during a turn, show structured tool-invocation chips in the
assistant message (tool name, running/result state), like PET's Message.tsx tool parts.

Design:
- Data model: carry structured tool parts alongside assistant text. Options: store them
  in the assistant message metadata_json (no migration) and/or emit dedicated live events
  (tool.call.started / tool.call.completed). Shape per part:
  {toolInvocationId, toolName, state: "call"|"partial-call"|"result", args?, result?}.
- Runtime: find where tool calls are executed in the turn (runtime_executor.go / tool
  registry / governor path) and emit the tool parts to the live-events stream and/or
  persist them on the message. Reuse the existing bus/live-event channel that already
  carries assistant.message.partial.
- Frontend: extend the live-events handling in ChatPage.tsx (handleLiveEvent) and/or the
  thread message shape to capture tool parts; render tool chips in ChatMessage.tsx above
  the assistant prose (PET apps/pet-web/src/components/chat/Message.tsx is the reference
  design — a bordered row with a wrench icon, tool name, and Running…/result preview).
  ChatMessageView gains an optional toolParts field.
- Tests: Go test that a tool-running turn emits/persists tool parts; vitest that
  ChatMessage renders tool chips for given parts. Run the Go + vitest + tsc suites.

Acceptance: a turn that invokes a tool shows a live "Running…" chip that resolves to a
result; renders cleanly with zero tool parts; suites green; no fake/empty chips.
```

---

## Sequencing note
Variants (B) modifies `RegenerateLastReply`, which is already shipped — do B's regenerate
change carefully to preserve current behavior + tests. Tool-streaming (C) and continue (A)
are independent of each other. All three are independent enough to run in parallel, but if
one agent takes B it should own the regenerate code path.
