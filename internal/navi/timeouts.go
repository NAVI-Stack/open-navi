package navi

import (
	"time"

	naviruntime "github.com/open-navi/navi/internal/runtime"
)

// Chat-path timeout budgets form a strict ordering that MUST be preserved:
//
//	frontend wait budget  >  coordinatorRunTimeout(LLMCallTimeout)  >  LLMCallTimeout
//
// The console (web-src/navi-console) abandons a message and shows
// "NAVI did not respond in time" once its wait budget elapses. If that budget
// is shorter than the backend run budget — as it historically was (180s console
// vs 330s backend) — the console "un-saves" messages for runs that are still
// legitimately in flight. Keep the console budget (FRONTEND_CHAT_WAIT_BUDGET_MS
// in src/lib/chatTimeouts.ts) strictly greater than coordinatorRunTimeout below.
//
// defaultForegroundLLMCallTimeout bounds a single foreground LLM turn. It is kept
// generous enough for slow local models, but no longer so large that one stuck
// turn can hold a chat for 5+ minutes. Override via the llm_call_timeout config.
const (
	defaultForegroundLLMCallTimeout = 3 * time.Minute
	coordinatorRunTimeoutHeadroom   = 30 * time.Second
)

func normalizeLLMCallTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return defaultForegroundLLMCallTimeout
	}
	return timeout
}

func coordinatorRunTimeout(timeout time.Duration) time.Duration {
	return normalizeLLMCallTimeout(timeout) + coordinatorRunTimeoutHeadroom
}

func timeoutFallbackContent(surface string) string {
	return naviruntime.TimeoutFallbackContent(surface)
}

func timeoutRecoveryGuidance(surface string) string {
	return naviruntime.TimeoutRecoveryGuidance(surface)
}
