# Delayed, Scheduled, and Multi-Message Sending

**Status:** Accepted  
**Last Updated:** 2026-03-16  
**See also:** [streaming-runtime.md](streaming-runtime.md), [ADR-006](../adr/ADR-006-streaming-runtime.md)

---

## Summary

A run may emit **zero, one, or many** assistant messages. Each message can have an optional **delay** before delivery. Surfaces (CLI, connectors, live WebSocket) receive each message via the existing `assistant.message.completed` event; ordering and timing are preserved. `run.completed` is emitted exactly once per run, after the last message is delivered.

This extends the run model in [streaming-runtime](streaming-runtime.md) and [ADR-006](../adr/ADR-006-streaming-runtime.md) without changing inbox or run lifecycle ownership.

---

## Capability

- **Multi-message:** The executor (or an LLM tool) returns a list of `(content, delay)` instead of a single `FinalContent`. The coordinator (and an in-process scheduler) persist and emit each message in order; `run.completed` is emitted only after the last message.
- **Delayed send:** Delay is a non-negative duration; `0` means send immediately. Delays are **relative to run completion** (when the executor returns): e.g. `[0, 3s, 6s]` means send now, now+3s, now+6s.
- **Mechanism:** Hybrid (Option C). The executor can return `ScheduledMessages` (for programmatic use), and exposes a **tool** `send_reply(content, delay_seconds?)` so the LLM can request "send this now" or "send this in N seconds" during the turn. When the run finishes, if the tool was used, the executor returns the collected list as `ScheduledMessages`; otherwise it returns a single `FinalContent` as today.

---

## Store contract

- **AppendAssistantMessage(ctx, sessionID, runID, content, personaID, inboxItemID string) (messageID string, err error)**  
  Inserts one assistant message and emits `assistant.message.completed` only. Does not emit `run.completed` or update run status.

- **MarkRunCompleted(ctx, run) error**  
  Updates run status to completed and emits `run.completed`. Used after the last scheduled message (or when the run has no more messages to send).

- **CompleteRun(ctx, run, content, personaID, inboxItemID)**  
  Remains the single-message convenience: equivalent to `AppendAssistantMessage` + `MarkRunCompleted` for one message.

---

## Scheduler

An in-process component that accepts "at time T, run callback C". Used only for delayed messages (Delay > 0).

- Single goroutine with a priority queue (or slice) of (time.Time, callback). When the timer fires, run all callbacks that are due, then set the timer to the next due time.
- Callback for a scheduled message: call store `AppendAssistantMessage`; if that was the last message for the run, call store `MarkRunCompleted`.
- No new infra (no cron, no external queue). Patterns similar to [internal/connectors/worker.go](../internal/connectors/worker.go) or [internal/navi/heartbeat/service.go](../internal/navi/heartbeat/service.go).

---

## Events

- `assistant.message.completed` may be emitted **multiple times per run** (one per message).
- `run.completed` is emitted **exactly once per run**, after the last message (immediately if all delays are 0, or when the last scheduled callback runs).

---

## Governance and limits

- **Max scheduled messages per run:** 10. Reject tool calls that would exceed this.
- **Max delay:** 5 minutes. Reject `delay_seconds` above 300 (or equivalent).
- Enforced in the executor when handling `send_reply` tool calls.

---

## Tool: send_reply

- **Name:** `send_reply` (or `navi_send_reply`).
- **Parameters:** `content` (string, required), `delay_seconds` (number, optional, default 0).
- **Behavior:** Appends `{Content: content, Delay: delay_seconds * time.Second}` to a run-scoped list. Does not send immediately; the executor returns the list as `ScheduledMessages` when the run completes.
- **Precedence:** If the run has any `ScheduledMessages` (from one or more tool calls), the coordinator uses that list and ignores `FinalContent`. If the run has no scheduled messages, behavior is unchanged (single `FinalContent`).

---

## Open decisions (resolved for this implementation)

- **Delay reference:** Relative to run completion (when executor returns). So `[0, 3, 6]` means now, now+3s, now+6s.
- **Tool shape:** `send_reply(content, delay_seconds)`; batch form deferred.
- **Limits:** 10 messages per run, 5 minutes max delay.
