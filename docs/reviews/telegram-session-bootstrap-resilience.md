# Telegram chat Bootstrap Resilience

OMN-63 exposed a brittle bootstrap path in the Telegram connector. The issue was not just "gateway unavailable at startup."

Historical naming note: this document previously used "session"; canonical terminology is now Chat for transcript/context and RuntimeSession for execution lifecycle.

## What was fixed

- Telegram now waits for a real gateway health response from `GET /health` before starting its gateway-dependent bootstrap work.
- The connector HTTP client timeout was raised to `60s` to better match local gateway behavior under load.
- chat creation now retries with backoff instead of failing on the first slow response.
- `recoverFocus()` now reads the correct `chat_id` field from `/api/navi/chats`, so existing chats can be reused after connector restart.
- Message handling now goes through a shared `ensureChat()` path that prefers an existing recovered chat before trying to create a new one.

## Why this mattered

If a chat creation call was slow or timed out after the server had already persisted the chat, the old code could not recover on the next message because focus recovery looked for the wrong JSON field. That kept the bot stuck in a loop of repeated `POST /api/navi/chats` attempts and repeated initialization failures.

## Remaining follow-up

- We still use a single SQLite handle configuration in `internal/store/db.go`; if API latency under heavy runtime write load remains a problem, the next step is broadening DB concurrency rather than adding more Telegram-only retries.
- Other connectors that create chats through the gateway may want the same explicit health/backoff behavior once those surfaces become active.
