package runtime

import (
	"strings"

	"github.com/open-navi/navi/internal/schema"
)

const (
	telegramTimeoutRecoveryGuidance = "Try /status, wait a moment, or start a fresh session with /new."
	defaultTimeoutRecoveryGuidance  = "Try /model set ollama <model> (e.g. llama3.1:latest) or /status."
)

func TimeoutFallbackContent(surface string) string {
	return "The request took too long. " + TimeoutRecoveryGuidance(surface)
}

func TimeoutRecoveryGuidance(surface string) string {
	if allowsModelSwitchRecovery(surface) {
		return defaultTimeoutRecoveryGuidance
	}
	return telegramTimeoutRecoveryGuidance
}

func allowsModelSwitchRecovery(surface string) bool {
	switch strings.ToLower(strings.TrimSpace(surface)) {
	case "telegram":
		return false
	default:
		return true
	}
}

func timeoutLikeMessage(msg string) bool {
	lowered := strings.ToLower(strings.TrimSpace(msg))
	return strings.Contains(lowered, "context deadline exceeded") || strings.Contains(lowered, "timeout")
}

func runSurface(run *RunState) string {
	if run == nil || len(run.Scratchpad) == 0 {
		return ""
	}
	return strings.TrimSpace(run.Scratchpad["source_channel"])
}

func userFacingRunFailureMessage(msg string, outcome schema.ExecutionOutcomeOutcome, run *RunState) string {
	if outcome == schema.ExecutionOutcomeTimedOut || timeoutLikeMessage(msg) {
		return TimeoutFallbackContent(runSurface(run))
	}
	return SanitizeErrorMessage(msg)
}
