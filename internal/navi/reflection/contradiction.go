package reflection

import (
	"context"
	"strings"

	"github.com/ceoai/navi/internal/schema"
)

// RecentMessagesFunc returns the most recent directive messages (newest first) for
// contradiction comparison with Conscious context. Provided by the host (e.g. store.GetMessages).
type RecentMessagesFunc func(ctx context.Context, directiveID string, limit int) ([]schema.DirectiveMessage, error)

// defaultContradictionChecker compares incoming fact key/value with recent Conscious
// context (directive messages). If the same key appears in recent context with a
// different value, it reports a contradiction and material harm.
type defaultContradictionChecker struct {
	getRecent   RecentMessagesFunc
	recentLimit int
}

// NewDefaultContradictionChecker returns a ContradictionChecker that compares fact
// writes with recent directive messages. When the key appears in recent context
// but the new value does not (or a different value appears), it signals contradiction
// and material harm so the worker can emit an interruption.
func NewDefaultContradictionChecker(getRecent RecentMessagesFunc, recentLimit int) ContradictionChecker {
	if recentLimit <= 0 {
		recentLimit = 10
	}
	return &defaultContradictionChecker{getRecent: getRecent, recentLimit: recentLimit}
}

// Check implements ContradictionChecker. It loads recent messages for the directive,
// concatenates content, and checks whether the key appears with a value that conflicts
// with the proposed value (key present in context but value absent or differing).
func (d *defaultContradictionChecker) Check(ctx context.Context, chatID, directiveID, key, value string) (contradiction string, materialHarm bool) {
	if directiveID == "" || key == "" || d.getRecent == nil {
		return "", false
	}
	msgs, err := d.getRecent(ctx, directiveID, d.recentLimit)
	if err != nil || len(msgs) == 0 {
		return "", false
	}
	var combined strings.Builder
	for _, m := range msgs {
		combined.WriteString(m.Content)
		combined.WriteString(" ")
	}
	text := combined.String()
	lower := strings.ToLower(text)
	k := strings.ToLower(key)
	v := strings.ToLower(value)
	if !strings.Contains(lower, k) {
		return "", false
	}
	if v != "" && strings.Contains(lower, v) {
		return "", false
	}
	return "Recent Conscious context references \"" + key + "\" with a different or conflicting value.", true
}
