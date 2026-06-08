/**
 * Single source of truth for how long the console waits for NAVI to deliver a
 * reply before giving up on a turn.
 *
 * IMPORTANT — keep this strictly GREATER than the backend run budget. The chat
 * reply path is asynchronous: POST /api/navi/chats/{id}/message returns
 * `queued` immediately and the actual reply arrives later as runtime events.
 * The backend caps a run at `coordinatorRunTimeout(LLMCallTimeout)` (see
 * internal/navi/timeouts.go — default LLMCallTimeout 3m + 30s headroom = 210s),
 * then needs up to one 300ms poll + a 5s interrupt drain to emit and deliver the
 * terminal event.
 *
 * If this budget is shorter than that — as it historically was (180s here vs a
 * 330s backend) — the console declares "NAVI did not respond in time" and rolls
 * the chat back for runs that are still legitimately in flight, losing the
 * user's message. We set it well above the default backend ceiling so a
 * slow-but-valid run still finalizes when its terminal event lands. The draft
 * handler also no longer discards the (already-persisted) message when this
 * elapses — see ChatPage's draft timeout — so this acts as a soft "taking
 * longer than usual" boundary rather than a hard data-losing cutoff.
 */
export const FRONTEND_CHAT_WAIT_BUDGET_MS = 300_000; // 5 minutes
