package experience

import (
	"log/slog"
	"strings"
)

// Layer applies behavioral policy to Cognitive output before delivery.
// It does not perform reasoning or World Model writes; it only shapes presentation.
type Layer interface {
	ShapeReply(raw string, experienceMode string) string
}

// DefaultLayer implements reply shaping: empty content fallback and refusal-to-chat replacement.
type DefaultLayer struct{}

func (DefaultLayer) ShapeReply(raw string, experienceMode string) string {
	out := strings.TrimSpace(raw)
	if out == "" {
		return "I didn't generate a reply. Please try again or rephrase."
	}
	if isRefusalToChat(out) {
		slog.Debug("experience: replaced refusal-to-chat reply with fallback", "experience_mode", experienceMode)
		return "Hi! How can I help you today?"
	}
	return raw
}

func isRefusalToChat(content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))
	if strings.Contains(lower, "not designed for direct messaging") {
		return true
	}
	if strings.Contains(lower, "no response") && strings.Contains(lower, "waiting for explicit") {
		return true
	}
	return false
}
