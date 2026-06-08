package schema

import "time"

// CIP P5 — Sync log (CIP §7, §11; Vault §13).
//
// The sync log is the append-only record of intake activity that powers the
// Console surfaces and lets the owner answer "did NAVI see my edit?" and "what
// did the last Telegram pass do?". Two parallel logs exist: intake_sync_log
// (one row per connector pass, populated by the intake worker) and
// vault_sync_log (one row per Vault diff pass, schema only — populated by the
// parallel Vault deliverable).

// SyncTerminalStatus is the terminal disposition of a sync pass.
type SyncTerminalStatus string

const (
	// SyncStatusRunning marks a pass that has started but not yet closed.
	SyncStatusRunning SyncTerminalStatus = "running"
	// SyncStatusCompleted marks a pass that finished within budget.
	SyncStatusCompleted SyncTerminalStatus = "completed"
	// SyncStatusBudgetExceeded marks a pass terminated by the per-pass record/byte budget.
	SyncStatusBudgetExceeded SyncTerminalStatus = "budget_exceeded"
	// SyncStatusCostCeiling marks a pass halted by the Governor cost ceiling.
	SyncStatusCostCeiling SyncTerminalStatus = "cost_ceiling"
	// SyncStatusConsentRejected marks a backfill pass the owner declined.
	SyncStatusConsentRejected SyncTerminalStatus = "consent_rejected"
	// SyncStatusConsentPending marks a backfill awaiting owner approval.
	SyncStatusConsentPending SyncTerminalStatus = "consent_pending"
	// SyncStatusError marks a pass that ended on an unrecoverable error.
	SyncStatusError SyncTerminalStatus = "error"
)

// IntakeSyncLogEntry is one row of the intake sync log: a single connector pass
// (Backfill or Delta). Append-only. A backfill that spans many passes may emit a
// parent row (ParentID == "") plus child rows (ParentID set to the parent's ID);
// the intake worker currently emits one row per aggregation window.
type IntakeSyncLogEntry struct {
	ID                 string             `json:"id"`
	ConnectorID        string             `json:"connector_id"`
	ParentID           string             `json:"parent_id,omitempty"`
	JobMode            JobMode            `json:"job_mode"`
	StartedAt          time.Time          `json:"started_at"`
	EndedAt            *time.Time         `json:"ended_at,omitempty"`
	RecordsAdmitted    int                `json:"records_admitted"`
	RecordsDeduped     int                `json:"records_deduped"`
	RecordsDistilled   int                `json:"records_distilled"`
	RecordsSynthesized int                `json:"records_synthesized"`
	Errors             int                `json:"errors"`
	TerminalStatus     SyncTerminalStatus `json:"terminal_status"`
	Note               string             `json:"note,omitempty"`
	CreatedAt          time.Time          `json:"created_at"`
}

// VaultSyncLogEntry is one row of the Vault sync log: a single per-file diff
// pass (Vault §13). Append-only. P5 ships the schema and the store; the Vault
// deliverable populates these rows. The Console renders them per-file.
type VaultSyncLogEntry struct {
	ID                string             `json:"id"`
	FilePath          string             `json:"file_path"`
	StartedAt         time.Time          `json:"started_at"`
	EndedAt           *time.Time         `json:"ended_at,omitempty"`
	DiffSummary       string             `json:"diff_summary,omitempty"`
	MutationsProposed int                `json:"mutations_proposed"`
	MutationsApproved int                `json:"mutations_approved"`
	ProposalsRaised   int                `json:"proposals_raised"`
	Errors            int                `json:"errors"`
	TerminalStatus    SyncTerminalStatus `json:"terminal_status"`
	CreatedAt         time.Time          `json:"created_at"`
}
