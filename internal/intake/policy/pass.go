package policy

import (
	"time"

	"github.com/open-navi/navi/internal/schema"
)

// CostPerKBUSD is the V1 synthetic per-kilobyte intake cost estimate (distill +
// embed spend). CIP §8 explicitly declines to claim a fixed ratio — we measure
// ours; this is a deliberately small starting estimate used only to feed the
// Governor's per-connector cost ceiling. It is tunable, not a contract.
const CostPerKBUSD = 0.0002

// EstimateRecordCostUSD estimates the processing cost of admitting a record of n
// bytes. Used to feed the per-connector cost ceiling (CIP §7); the Governor is
// the authority that decides whether the ceiling is breached.
func EstimateRecordCostUSD(n int) float64 {
	if n <= 0 {
		return 0
	}
	return float64(n) / 1024.0 * CostPerKBUSD
}

// Pass accumulates per-pass counters for one connector in one job mode. It is
// the unit the per-pass record/byte budget (CIP §7) is enforced against, and the
// shape the worker flushes into an intake_sync_log row. Not safe for concurrent
// use; callers serialize access (the worker guards it with a mutex).
type Pass struct {
	ConnectorID string
	Mode        schema.JobMode
	Policy      SyncPolicy
	StartedAt   time.Time

	Admitted    int
	Deduped     int
	Distilled   int
	Synthesized int
	Errors      int
	Bytes       int64
	CostUSD     float64
	Status      schema.SyncTerminalStatus
}

// NewPass opens a fresh pass for a connector/mode under the given policy.
func NewPass(connectorID string, mode schema.JobMode, p SyncPolicy, now time.Time) *Pass {
	return &Pass{
		ConnectorID: connectorID,
		Mode:        mode,
		Policy:      p,
		StartedAt:   now,
		Status:      schema.SyncStatusRunning,
	}
}

// CanAdmit reports whether admitting one more record of n bytes stays within the
// per-pass record and byte budget. A zero limit means "unbounded" for that
// dimension. When CanAdmit returns false the caller must terminate the pass
// cleanly with SyncStatusBudgetExceeded — exceeding budget never silently
// admits (CIP §7 acceptance: exceeding it terminates the pass cleanly).
func (p *Pass) CanAdmit(n int64) bool {
	b := p.Policy.Block(p.Mode).Budget
	if b.MaxRecordsPerPass > 0 && p.Admitted+1 > b.MaxRecordsPerPass {
		return false
	}
	if b.MaxBytesPerPass > 0 && p.Bytes+n > b.MaxBytesPerPass {
		return false
	}
	return true
}

// CostCeilingUSD returns the active block's per-connector cost ceiling.
func (p *Pass) CostCeilingUSD() float64 {
	return p.Policy.Block(p.Mode).Budget.CostCeilingUSD
}

// ToLogEntry materializes the pass into an append-only sync-log entry with the
// given terminal status and end time.
func (p *Pass) ToLogEntry(status schema.SyncTerminalStatus, endedAt time.Time) schema.IntakeSyncLogEntry {
	e := schema.IntakeSyncLogEntry{
		ConnectorID:        p.ConnectorID,
		JobMode:            p.Mode,
		StartedAt:          p.StartedAt,
		EndedAt:            &endedAt,
		RecordsAdmitted:    p.Admitted,
		RecordsDeduped:     p.Deduped,
		RecordsDistilled:   p.Distilled,
		RecordsSynthesized: p.Synthesized,
		Errors:             p.Errors,
		TerminalStatus:     status,
	}
	return e
}
