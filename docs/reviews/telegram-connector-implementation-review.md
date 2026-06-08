# Telegram Connector Implementation Review

Review of the plan-based implementation for issues and gaps. Fixes applied and remaining items are noted below.

---

## Issues Fixed

### 1. **CallbackQuery nil dereference (bug)**
- **Location:** `handleUpdate` when handling `CallbackQuery`.
- **Issue:** Accessing `u.CallbackQuery.Message.Chat.ID` without checking `Message`. For inline or other callbacks where `Message` is nil, this panics.
- **Fix:** Guard with `u.CallbackQuery.Message != nil` before using `Message.Chat.ID`.

### 2. **HandleWebhook blocks HTTP response**
- **Location:** `HandleWebhook` in `plugins/telegram/connectors/telegram/bot.go`.
- **Issue:** Telegram expects a fast 200; we sent 200 then ran `handleUpdate` synchronously, so the handler could block and trigger retries or timeouts.
- **Fix:** Send `200 OK` then run `handleUpdate` in a goroutine with `context.Background()` so the handler returns immediately.

### 3. **Double Stop() panics**
- **Location:** `Stop()` closes `b.done`.
- **Issue:** Closing an already-closed channel panics; calling `Stop()` twice (e.g. shutdown logic) could crash.
- **Fix:** Use `sync.Once` (`closeDoneOnce`) so `close(b.done)` runs only once.

### 4. **refreshToken ignores empty token**
- **Location:** `refreshToken` after decoding auth response.
- **Issue:** If the gateway returns 200 but no `token` in the body, we set `b.token = ""` and later calls get 401 with no clear cause.
- **Fix:** Decode with error check and return an error if `res["token"]` is empty.

### 5. **Webhook not configurable from runtime config**
- **Location:** `internal/config` and `cmd/navid` factory.
- **Issue:** `WebhookURL` and `WebhookSecret` existed only on the connector `Config`; there was no way to set them from YAML or env, so webhook mode was only usable when building the bot programmatically.
- **Fix:** Added `WebhookURL` and `WebhookSecret` to `TelegramConfig` in `internal/config/config.go` and pass them from the navid factory into `telegram.Config`.

---

## Gaps and Recommendations

### 1. **HealthChecker not used by Manager** — resolved
- **Was:** The Telegram bot implements `HealthChecker`; health was derived only from `IsRunning()` and consecutive send errors.
- **Now:** `Manager.Health()` calls `HealthCheck(ctx)` for connectors that implement `connectors.HealthChecker` and marks the connector "down" when the probe fails.

### 2. **No answerCallbackQuery for HITL buttons** — resolved
- **Gap:** After handling approve/reject we don’t call Telegram’s `answerCallbackQuery`. The client may keep a loading state on the button.
- **Now:** After a successful approve/reject, `answerCallbackQuery` is called to clear the loading state.

### 3. **sendMessageToChat error body not used** — resolved
- **Gap:** On `resp.StatusCode >= 400` we return a generic error; response body is not read or included, which can make debugging Telegram API errors harder.
- **Now:** On 4xx/5xx, `resp.Body` is read and included in the error.

### 4. **ParseMode not validated** — resolved
- **Gap:** We pass `msg.ParseMode` through to the Telegram API. Invalid values (e.g. typo "Markdwon") cause Telegram to return an error; we don’t validate.
- **Now:** Allowed values ("", "Markdown", "MarkdownV2", "HTML") are validated and a clear error is returned before calling the API.

### 5. **Webhook URL in setup**
- **Status:** The canonical connector setup path handles the full Telegram setup payload for `POST /api/setup/connector`, including `account`, `allow_from`, `pairing_code`, `gateway_url`, `api_url`, `webhook_url`, and `webhook_secret`. These are persisted through `telegram_accounts` and passed to the connector with the same semantics as the runtime start path.
- **Current UX:** The onboarding/setup schema intentionally prompts only for bot token and chat ID. Advanced Telegram fields remain backend-supported for later configuration instead of first-run prompting.

---

## Summary

- **Fixed:** CallbackQuery nil guard, webhook handler non-blocking response, double-close on `Stop()`, empty token handling in `refreshToken`, webhook config in runtime config + navid factory, `answerCallbackQuery` for HITL interactions, `sendMessage` error body surfacing, and `ParseMode` validation.
- **Remaining:** Setup wizard UI fields for webhook_url/webhook_secret (optional UI follow-up).
