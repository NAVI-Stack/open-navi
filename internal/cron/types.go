package cron

// ScheduleKind enumerates persisted schedule kinds for cron_jobs.
type ScheduleKind string

const (
	ScheduleAt    ScheduleKind = "at"
	ScheduleEvery ScheduleKind = "every"
	ScheduleCron  ScheduleKind = "cron"
)

// WakeMode selects how a main-session job is executed.
type WakeMode string

const (
	WakeNow            WakeMode = "now"
	WakeNextHeartbeat  WakeMode = "next-heartbeat"
)

// SessionTarget selects execution plane. Any value starting with
// SessionTargetPrefix is interpreted as a literal session id, e.g.
// "session:<chatID>", and routed to the ChatAppender path.
type SessionTarget string

const (
	SessionMain         SessionTarget = "main"
	SessionIsolated     SessionTarget = "isolated"
	SessionTargetPrefix               = "session:"
)

// PayloadKind selects cron payload semantics.
type PayloadKind string

const (
	PayloadSystemEvent     PayloadKind = "systemEvent"
	PayloadAgentTurn       PayloadKind = "agentTurn"
	PayloadAssistantMessage PayloadKind = "assistantMessage"
)

// RunStatus is the lifecycle outcome for a cron tick.
type RunStatus string

const (
	RunOK      RunStatus = "ok"
	RunError   RunStatus = "error"
	RunSkipped RunStatus = "skipped"
)

// Schedule is an in-memory schedule definition.
type Schedule struct {
	Kind     ScheduleKind
	AtRFC    string // kind=at: absolute time RFC3339
	EveryMS  int64  // kind=every
	AnchorMS int64  // kind=every: optional epoch ms anchor
	CronExpr string // kind=cron
	TZ       string // IANA TZ for cron; empty = UTC
	Stagger  int64  // kind=cron optional stagger window ms
}

// FailureAlert configures repeated-error notifications (optional).
type FailureAlert struct {
	After      int   `json:"after"`
	CooldownMS int64 `json:"cooldown_ms"`
}

// Delivery is reserved for future channel routing (JSON in DB today).
type Delivery struct {
	RawJSON string
}

// Job is the runnable view of cron_jobs merged with schedule logic.
type Job struct {
	ID            string
	OwnerID       string
	Name          string
	Description   string
	Enabled       bool
	Schedule      Schedule
	SessionTarget SessionTarget
	WakeMode      WakeMode
	PayloadKind   PayloadKind
	PayloadText   string
	Delivery      Delivery
	FailureAlert  *FailureAlert
	TimeoutMS     int64 // 0 = use default from config

	DeleteAfterRun bool

	State JobState

	CreatedAtMS int64
	UpdatedAtMS int64
}

// JobState mirrors persisted execution state fields.
type JobState struct {
	NextRunAtMS        int64 // 0 = unset / disabled advance
	RunningAtMS        int64
	LastRunAtMS        int64
	LastRunStatus      RunStatus
	LastError          string
	LastDurationMS     int64
	ConsecutiveErrors  int64
	ScheduleErrorCount int64
	LastFailureAlertMS int64
}

// RunResult is returned after executing one job payload.
type RunResult struct {
	Status  RunStatus
	Error   string
	Summary string
}
