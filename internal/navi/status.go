package navi

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

// StalenessThreshold is the maximum duration a "processing" state can persist
// without an update before the tracker reports "unresponsive".
const StalenessThreshold = 5 * time.Minute

// StatusTracker tracks the NAVI agent's current activity state using atomic
// operations and a mutex-protected detail field. It is safe for concurrent use.
//
// The tracker is updated by the agent loop at key lifecycle points (turn start,
// tool execution, turn end) and read by the gateway to serve status requests.
// No LLM calls are involved.
type StatusTracker struct {
	state          atomic.Value // schema.AgentActivityState
	since          atomic.Value // time.Time
	lastActivityAt atomic.Value // time.Time
	lastUpdatedAt  atomic.Value // time.Time
	uptimeSince    time.Time
	turnsProcessed atomic.Int64

	mu                     sync.RWMutex
	activeRuntimeSessionID string
	currentDetail          string
}

// NewStatusTracker creates a tracker initialized to the idle state.
func NewStatusTracker() *StatusTracker {
	t := &StatusTracker{
		uptimeSince: time.Now().UTC(),
	}
	now := time.Now().UTC()
	t.state.Store(schema.AgentStateIdle)
	t.since.Store(now)
	t.lastActivityAt.Store(now)
	t.lastUpdatedAt.Store(now)
	return t
}

// SetState transitions the tracker to a new state with an optional detail string.
func (t *StatusTracker) SetState(state schema.AgentActivityState, runtimeSessionID, detail string) {
	now := time.Now().UTC()
	t.state.Store(state)
	t.since.Store(now)
	t.lastUpdatedAt.Store(now)

	t.mu.Lock()
	t.activeRuntimeSessionID = runtimeSessionID
	t.currentDetail = detail
	t.mu.Unlock()
}

// RecordTurnComplete marks a turn as completed and transitions to idle.
func (t *StatusTracker) RecordTurnComplete() {
	now := time.Now().UTC()
	t.turnsProcessed.Add(1)
	t.lastActivityAt.Store(now)
	t.state.Store(schema.AgentStateIdle)
	t.since.Store(now)
	t.lastUpdatedAt.Store(now)

	t.mu.Lock()
	t.activeRuntimeSessionID = ""
	t.currentDetail = ""
	t.mu.Unlock()
}

// Touch updates the last-updated timestamp without changing state.
// Use this during long-running operations to prevent staleness detection.
func (t *StatusTracker) Touch() {
	t.lastUpdatedAt.Store(time.Now().UTC())
}

// Snapshot returns a point-in-time view of the agent's status.
// If the tracker has been in a non-idle state for longer than StalenessThreshold
// without an update, the snapshot reports AgentStateUnresponsive.
func (t *StatusTracker) Snapshot() schema.AgentStatusSnapshot {
	state := t.state.Load().(schema.AgentActivityState)
	since := t.since.Load().(time.Time)
	lastActivity := t.lastActivityAt.Load().(time.Time)
	lastUpdated := t.lastUpdatedAt.Load().(time.Time)
	now := time.Now().UTC()

	// Staleness detection: if we're in a "busy" state but haven't been
	// updated within the threshold, report unresponsive.
	if isActiveState(state) && now.Sub(lastUpdated) > StalenessThreshold {
		state = schema.AgentStateUnresponsive
	}

	t.mu.RLock()
	activeRuntimeSessionID := t.activeRuntimeSessionID
	detail := t.currentDetail
	t.mu.RUnlock()

	return schema.AgentStatusSnapshot{
		State:                  state,
		Since:                  since,
		LastActivityAt:         lastActivity,
		UptimeSince:            t.uptimeSince,
		ActiveRuntimeSessionID: activeRuntimeSessionID,
		CurrentDetail:          detail,
		TurnsProcessed:         t.turnsProcessed.Load(),
		UpdatedAt:              now,
	}
}

// isActiveState returns true for states that indicate the agent should be
// making progress (and thus are subject to staleness detection).
func isActiveState(s schema.AgentActivityState) bool {
	switch s {
	case schema.AgentStateProcessing, schema.AgentStateToolExecuting, schema.AgentStateHeartbeat:
		return true
	default:
		return false
	}
}
