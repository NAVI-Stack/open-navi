package intake

import (
	"context"
	"time"

	"github.com/ceoai/navi/internal/intake/policy"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

// CIP P5 — per-connector pass accounting. The worker accumulates counters for an
// open pass per connector and flushes them into intake_sync_log rows. A "pass" is
// the worker's aggregation window for a connector: it opens on the first record,
// accumulates admitted/deduped/distilled/synthesized/error counts, and is closed
// (written as one append-only sync-log row) when the per-pass budget is exceeded,
// the cost ceiling trips, the periodic flush fires, or the worker shuts down.
//
// This is the documented choice for "one row per pass": a steady-state delta
// connector emits one row per activity window; a budget/cost event closes the
// current window early with the corresponding terminal status. Backfill batches
// would emit a parent row per batch (the Vault/backfill puller is future work).
//
// All access is guarded by w.passMu so the periodic flush goroutine and the
// (single-threaded) message handler never race on the counters map.

// ensurePass returns the open pass for a connector, creating it under the given
// policy/mode if none exists. Caller must hold w.passMu.
func (w *Worker) ensurePassLocked(connectorID string, mode schema.JobMode, pol policy.SyncPolicy) *policy.Pass {
	p := w.passes[connectorID]
	if p == nil {
		p = policy.NewPass(connectorID, mode, pol, time.Now().UTC())
		w.passes[connectorID] = p
	}
	return p
}

// passCanAdmit reports whether admitting one more record of n bytes stays within
// the connector's per-pass budget. It ensures a pass exists as a side effect.
func (w *Worker) passCanAdmit(_ context.Context, connectorID string, mode schema.JobMode, pol policy.SyncPolicy, n int64) bool {
	w.passMu.Lock()
	defer w.passMu.Unlock()
	p := w.ensurePassLocked(connectorID, mode, pol)
	return p.CanAdmit(n)
}

// passBumpAdmitted records a successful admit (record + bytes) on the open pass.
func (w *Worker) passBumpAdmitted(_ context.Context, connectorID string, mode schema.JobMode, pol policy.SyncPolicy, n int64) {
	w.passMu.Lock()
	defer w.passMu.Unlock()
	p := w.ensurePassLocked(connectorID, mode, pol)
	p.Admitted++
	p.Bytes += n
	p.CostUSD += policy.EstimateRecordCostUSD(int(n))
}

// passBumpDeduped records a deduplicated record on the open pass.
func (w *Worker) passBumpDeduped(_ context.Context, connectorID string, pol policy.SyncPolicy) {
	w.passMu.Lock()
	defer w.passMu.Unlock()
	p := w.ensurePassLocked(connectorID, schema.JobModeDelta, pol)
	p.Deduped++
}

// passBumpError records an error on the open pass.
func (w *Worker) passBumpError(_ context.Context, connectorID string, pol policy.SyncPolicy) {
	w.passMu.Lock()
	defer w.passMu.Unlock()
	p := w.ensurePassLocked(connectorID, schema.JobModeDelta, pol)
	p.Errors++
}

// passBumpStages records downstream-stage outcomes (distilled chunks persisted,
// World Model entities synthesized) on the open pass.
func (w *Worker) passBumpStages(_ context.Context, connectorID string, pol policy.SyncPolicy, distilled, synthesized int) {
	w.passMu.Lock()
	defer w.passMu.Unlock()
	p := w.ensurePassLocked(connectorID, schema.JobModeDelta, pol)
	p.Distilled += distilled
	p.Synthesized += synthesized
}

// closePass terminates a connector's open pass with the given terminal status,
// writing one append-only intake_sync_log row when the pass had activity (or a
// non-completed terminal status, which is always worth recording). It then
// removes the pass so the next record opens a fresh one.
func (w *Worker) closePass(ctx context.Context, connectorID string, status schema.SyncTerminalStatus) {
	w.passMu.Lock()
	p := w.passes[connectorID]
	delete(w.passes, connectorID)
	w.passMu.Unlock()
	if p == nil {
		return
	}
	w.writePassRow(ctx, p, status)
}

// FlushPasses closes every open pass with activity as a completed sync-log row.
// Called by the periodic flush ticker and on shutdown; also useful in tests.
func (w *Worker) FlushPasses(ctx context.Context) {
	w.passMu.Lock()
	toClose := make([]*policy.Pass, 0, len(w.passes))
	for id, p := range w.passes {
		toClose = append(toClose, p)
		delete(w.passes, id)
	}
	w.passMu.Unlock()
	for _, p := range toClose {
		w.writePassRow(ctx, p, schema.SyncStatusCompleted)
	}
}

// writePassRow appends the pass to intake_sync_log unless the connector's policy
// visibility is "hidden" and there is nothing notable to record.
func (w *Worker) writePassRow(ctx context.Context, p *policy.Pass, status schema.SyncTerminalStatus) {
	if w.db == nil {
		return
	}
	activity := p.Admitted + p.Deduped + p.Errors
	// Always record a non-completed terminal status (budget/cost/consent events);
	// skip empty completed windows and hidden-visibility connectors.
	if status == schema.SyncStatusCompleted {
		if activity == 0 {
			return
		}
		if p.Policy.Visibility == schema.SyncVisibilityHidden {
			return
		}
	}
	entry := p.ToLogEntry(status, time.Now().UTC())
	if _, err := store.AppendIntakeSyncLog(ctx, w.db, entry); err != nil {
		w.log.Warn("intake: worker: append sync log failed", "err", err, "connector_id", p.ConnectorID)
	}
}
