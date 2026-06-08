package trace

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// EnsureTraceID preserves an existing trace ID when present, otherwise derives
// a stable run-scoped trace ID or creates a new chat-turn trace ID.
func EnsureTraceID(existing, runID, chatID, mode string) string {
	if value := strings.TrimSpace(existing); value != "" {
		return value
	}
	if value := strings.TrimSpace(runID); value != "" {
		return "run:" + value
	}

	scope := strings.TrimSpace(mode)
	if scope == "" {
		scope = "trace"
	}
	if session := strings.TrimSpace(chatID); session != "" {
		return fmt.Sprintf("%s:%s:%s", scope, session, uuid.NewString())
	}
	return NewTraceID()
}

func NewTraceID() string {
	return "trace:" + uuid.NewString()
}
