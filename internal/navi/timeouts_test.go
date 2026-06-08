package navi

import (
	"strings"
	"testing"
	"time"
)

// frontendChatWaitBudget mirrors FRONTEND_CHAT_WAIT_BUDGET_MS in
// web-src/navi-console/src/lib/chatTimeouts.ts. The chat reply path is
// asynchronous, so the console gives up on a turn after this budget. It MUST
// stay strictly greater than the backend run budget, or the console will
// "un-save" messages for runs that are still legitimately in flight. If you
// change the console constant, change this one too.
const frontendChatWaitBudget = 300 * time.Second

// TestChatTimeoutOrderingInvariant locks in:
//
//	frontend wait budget  >  coordinatorRunTimeout(LLMCallTimeout)  >  LLMCallTimeout
//
// for the default LLM call timeout. This is the ordering that keeps the
// console from declaring "NAVI did not respond in time" before the backend has
// had a chance to finish and deliver its terminal event.
func TestChatTimeoutOrderingInvariant(t *testing.T) {
	llmCall := normalizeLLMCallTimeout(0) // default foreground budget
	runBudget := coordinatorRunTimeout(0)

	if !(runBudget > llmCall) {
		t.Fatalf("coordinatorRunTimeout (%s) must exceed LLMCallTimeout (%s)", runBudget, llmCall)
	}
	if !(frontendChatWaitBudget > runBudget) {
		t.Fatalf("frontend wait budget (%s) must exceed coordinatorRunTimeout (%s); raise FRONTEND_CHAT_WAIT_BUDGET_MS or lower the backend budget", frontendChatWaitBudget, runBudget)
	}
}

func TestTimeoutFallbackContent_TelegramOmitsModelCommand(t *testing.T) {
	got := timeoutFallbackContent("telegram")
	if strings.Contains(got, "/model") {
		t.Fatalf("telegram timeout fallback should not mention /model, got %q", got)
	}
	if !strings.Contains(got, "/status") {
		t.Fatalf("telegram timeout fallback should mention /status, got %q", got)
	}
	if !strings.Contains(got, "/new") {
		t.Fatalf("telegram timeout fallback should mention /new, got %q", got)
	}
}

func TestTimeoutFallbackContent_DefaultPreservesModelRecoveryGuidance(t *testing.T) {
	got := timeoutFallbackContent("web")
	if !strings.Contains(got, "/model set ollama") {
		t.Fatalf("non-telegram timeout fallback should preserve model guidance, got %q", got)
	}
	if !strings.Contains(got, "/status") {
		t.Fatalf("non-telegram timeout fallback should mention /status, got %q", got)
	}
}
