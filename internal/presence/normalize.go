package presence

import (
	"context"
	"time"

	"github.com/ceoai/navi/internal/schema"
)

// DefaultStaleAfterMs mirrors the current StatusTracker staleness window.
// Keep this local to presence so future health composition can replace it without
// coupling presence to the navi package.
const DefaultStaleAfterMs int64 = int64((5 * time.Minute) / time.Millisecond)

// NoOpAttentionSource always returns no attention.
type NoOpAttentionSource struct{}

func (n *NoOpAttentionSource) Attention(ctx context.Context, chatID string) PresenceAttention {
	return PresenceAttention{
		Level:    "none",
		Blocking: false,
	}
}

// DefaultClassificationSource applies conservative rules to map runtime state to public status.
// It intentionally under-classifies: heartbeat does not become dreaming, and tool execution
// becomes working rather than busy until a richer work-classification source exists.
type DefaultClassificationSource struct{}

func (d *DefaultClassificationSource) Classify(ctx context.Context, snapshot schema.AgentStatusSnapshot) (string, string, string) {
	publicStatus := NaviStatusIdle
	statusText := ""
	subtext := ""

	switch snapshot.State {
	case schema.AgentStateIdle:
		publicStatus = NaviStatusIdle
	case schema.AgentStateProcessing:
		publicStatus = NaviStatusWorking
		statusText = "Processing"
	case schema.AgentStateToolExecuting:
		publicStatus = NaviStatusWorking
		statusText = snapshot.CurrentDetail
		if statusText == "" {
			statusText = "Working"
		}
	case schema.AgentStateWaitingForInput:
		publicStatus = NaviStatusWantsAttention
		statusText = "Awaiting input"
	case schema.AgentStateHeartbeat:
		publicStatus = NaviStatusActive
		statusText = "Heartbeat"
	case schema.AgentStateDegraded:
		publicStatus = NaviStatusActive
		statusText = "Degraded"
	case schema.AgentStateOffline:
		publicStatus = NaviStatusOffline
		statusText = "Offline"
	case schema.AgentStateUnresponsive:
		publicStatus = NaviStatusOffline
		statusText = "Unresponsive"
	}

	return publicStatus, statusText, subtext
}

// DefaultHealthSource derives health from the status tracker snapshot.
type DefaultHealthSource struct {
	statusSource StatusSource
}

func NewDefaultHealthSource(status StatusSource) *DefaultHealthSource {
	return &DefaultHealthSource{statusSource: status}
}

func (h *DefaultHealthSource) Health(ctx context.Context) PresenceHealth {
	if h == nil || h.statusSource == nil {
		now := time.Now().UTC()
		return PresenceHealth{
			State:          "offline",
			LastActivityAt: now,
			StaleAfterMS:   int(DefaultStaleAfterMs),
		}
	}

	snap := h.statusSource.Snapshot()
	state := "healthy"

	if snap.State == schema.AgentStateUnresponsive {
		state = "unresponsive"
	} else if snap.State == schema.AgentStateOffline {
		state = "offline"
	} else if snap.State == schema.AgentStateDegraded {
		state = "degraded"
	}

	return PresenceHealth{
		State:          state,
		LastActivityAt: snap.LastActivityAt,
		StaleAfterMS:   int(DefaultStaleAfterMs),
	}
}
