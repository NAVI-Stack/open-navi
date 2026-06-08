package presence

import (
	"time"
)

// Version is the current presence protocol version.
const Version = "v1"

// Subject types
const (
	SubjectUser = "user"
	SubjectNavi = "navi"
)

// Authorities
const (
	AuthorityPet  = "pet"
	AuthorityNavi = "navi"
)

// Visibilities
const (
	VisibilityPublic  = "public"
	VisibilityPrivate = "private"
	VisibilityMixed   = "mixed"
)

// User statuses
const (
	UserStatusAvailable    = "available"
	UserStatusBusy         = "busy"
	UserStatusDoNotDisturb = "do_not_disturb"
	UserStatusAway         = "away"
	UserStatusOffline      = "offline"
)

// Navi public statuses
const (
	NaviStatusActive         = "active"
	NaviStatusIdle           = "idle"
	NaviStatusDreaming       = "dreaming"
	NaviStatusWorking        = "working"
	NaviStatusBusy           = "busy"
	NaviStatusOffline        = "offline"
	NaviStatusNeedsAttention = "needs_attention"
	NaviStatusWantsAttention = "wants_attention"
)

// Navi internal statuses
const (
	NaviInternalIdle            = "idle"
	NaviInternalProcessing      = "processing"
	NaviInternalToolExecuting   = "tool_executing"
	NaviInternalWaitingForInput = "waiting_for_input"
	NaviInternalHeartbeat       = "heartbeat"
	NaviInternalDegraded        = "degraded"
	NaviInternalOffline         = "offline"
	NaviInternalUnresponsive    = "unresponsive"
)

// PresenceEnvelope is the canonical wrapper for authoritative presence state.
type PresenceEnvelope struct {
	Version             string    `json:"version"`
	Source              string    `json:"source"`
	SubjectType         string    `json:"subject_type"`
	SubjectID           string    `json:"subject_id"`
	Authority           string    `json:"authority"`
	TransportObservedAt time.Time `json:"transport_observed_at"`
	StateUpdatedAt      time.Time `json:"state_updated_at"`
	StateRevision       int64     `json:"state_revision"`
	Visibility          string    `json:"visibility"`
	Payload             any       `json:"payload"`
}

// UserPresencePayload is the payload for subject_type = user.
type UserPresencePayload struct {
	PublicStatus  string `json:"public_status"`
	PrivateStatus string `json:"private_status,omitempty"`
	StatusText    string `json:"status_text,omitempty"`
	Subtext       string `json:"subtext,omitempty"`
}

// NaviPresencePayload is the payload for subject_type = navi.
type NaviPresencePayload struct {
	PublicStatus    string            `json:"public_status"`
	InternalStatus  string            `json:"internal_status"`
	StatusText      string            `json:"status_text,omitempty"`
	Subtext         string            `json:"subtext,omitempty"`
	ActiveRuntimeSessionID string            `json:"active_runtime_session_id,omitempty"`
	CurrentDetail   string            `json:"current_detail,omitempty"`
	Attention       PresenceAttention `json:"attention"`
	Health          PresenceHealth    `json:"health"`
}

// PresenceAttention describes the attention state of NAVI.
type PresenceAttention struct {
	Level      string  `json:"level"` // none, wants_attention, needs_attention
	ReasonCode *string `json:"reason_code,omitempty"`
	ProposalID *string `json:"proposal_id,omitempty"`
	Blocking   bool    `json:"blocking"`
}

// PresenceHealth describes the health state of NAVI.
type PresenceHealth struct {
	State          string    `json:"state"` // healthy, degraded, unresponsive, offline
	LastActivityAt time.Time `json:"last_activity_at,omitempty"`
	StaleAfterMS   int       `json:"stale_after_ms"`
}

// PresenceSnapshot is a combined view of current presence.
type PresenceSnapshot struct {
	Version          string            `json:"version"`
	SnapshotRevision int64             `json:"snapshot_revision"`
	GeneratedAt      time.Time         `json:"generated_at"`
	User             *PresenceEnvelope `json:"user,omitempty"`
	Navi             *PresenceEnvelope `json:"navi,omitempty"`
	Transport        TransportMetadata `json:"transport"`
}

// TransportMetadata describes the observed transport state.
type TransportMetadata struct {
	State           string    `json:"state"` // connected, reconnecting, stale, offline
	LastWSMessageAt time.Time `json:"last_ws_message_at,omitempty"`
	StaleAfterMS    int       `json:"stale_after_ms"`
}
