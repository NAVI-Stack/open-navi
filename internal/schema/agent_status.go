package schema

import "time"

// AgentActivityState represents the current activity state of an agent.
// This is a point-in-time status derived from runtime state, not LLM inference.
type AgentActivityState string

const (
	// AgentStateIdle means the agent loop is waiting for a wake signal.
	AgentStateIdle AgentActivityState = "idle"
	// AgentStateProcessing means the agent is actively processing a conversational turn.
	AgentStateProcessing AgentActivityState = "processing"
	// AgentStateToolExecuting means the agent is executing a tool call.
	AgentStateToolExecuting AgentActivityState = "tool_executing"
	// AgentStateWaitingForInput means the agent is waiting for user input (e.g. HITL proposal).
	AgentStateWaitingForInput AgentActivityState = "waiting_for_input"
	// AgentStateHeartbeat means the agent is running a heartbeat cycle.
	AgentStateHeartbeat AgentActivityState = "heartbeat"
	// AgentStateDegraded means the system is operational but one or more subsystems are unhealthy.
	AgentStateDegraded AgentActivityState = "degraded"
	// AgentStateOffline means the agent is not running.
	AgentStateOffline AgentActivityState = "offline"
	// AgentStateUnresponsive means the agent has not updated its state within the staleness window.
	AgentStateUnresponsive AgentActivityState = "unresponsive"
)

// AgentStatusSnapshot is a point-in-time view of the agent's status.
// All fields are computed from runtime state — no LLM calls are made.
type AgentStatusSnapshot struct {
	// State is the agent's current activity state.
	State AgentActivityState `json:"state"`
	// Since is when the current state began.
	Since time.Time `json:"since"`
	// LastActivityAt is the timestamp of the last completed turn or tool execution.
	LastActivityAt time.Time `json:"last_activity_at"`
	// UptimeSince is when the agent process started.
	UptimeSince time.Time `json:"uptime_since"`
	// ActiveRuntimeSessionID is the runtime session currently being processed (empty if idle).
	ActiveRuntimeSessionID string `json:"active_runtime_session_id,omitempty"`
	// CurrentDetail provides a short description of what the agent is doing (e.g. tool name).
	CurrentDetail string `json:"current_detail,omitempty"`
	// TurnsProcessed is the total number of turns processed since startup.
	TurnsProcessed int64 `json:"turns_processed"`
	// UpdatedAt is when this snapshot was generated.
	UpdatedAt time.Time `json:"updated_at"`
}
