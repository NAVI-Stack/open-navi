package intake

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/open-navi/navi/internal/intake/policy"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

const workerConsumerName = "intake-worker"

type messageOutcome int

const (
	outcomeAdmitted messageOutcome = iota
	outcomeDeduped
	outcomeErrored
	// outcomeHalted marks a record dropped because the Governor tripped the
	// per-connector cost ceiling (CIP P5 §7). The pass is terminated cleanly;
	// the message is acked (not redelivered) so it cannot loop the halt.
	outcomeHalted
)

// Worker subscribes to navi.refinery.queue, runs the Admit stage on each
// record, deduplicates by (ConnectorID, SourceID), and persists to
// intake_records. Per-pass counters are logged at Info level and, when policy is
// wired (CIP P5), flushed into the intake_sync_log.
type Worker struct {
	js           nats.JetStreamContext
	db           *sql.DB
	recordAction func() error // nil = no global action-budget enforcement
	log          *slog.Logger
	pipeline     PipelineConfig

	// CIP P5: per-connector sync policy + per-connector cost-ceiling enforcement
	// + the in-flight pass accumulators that become intake_sync_log rows.
	resolver   *policy.Resolver
	recordCost func(connectorID string, usd, ceiling float64) (float64, error) // governor; nil disables cost ceiling
	resetCost  func(connectorID string)                                        // governor; nil = no-op
	passMu     sync.Mutex
	passes     map[string]*policy.Pass // connector_id → current open pass
}

// NewWorker creates a Worker. recordAction is called before processing each
// record; returning a non-nil error halts that record (governor action budget).
// Pass nil to disable budget enforcement.
func NewWorker(js nats.JetStreamContext, db *sql.DB, recordAction func() error, log *slog.Logger) *Worker {
	if log == nil {
		log = slog.Default()
	}
	return &Worker{
		js:           js,
		db:           db,
		recordAction: recordAction,
		log:          log,
		passes:       make(map[string]*policy.Pass),
	}
}

// SetPipeline configures the downstream stage tuning, including the optional P3
// Synthesis config (Score → Embed → Extract → Resolve → Synthesize → Fold). When
// cfg.Synthesis is nil the worker behaves exactly as in P2 (no World Model
// writes). cmd/navid wires this once at startup.
func (w *Worker) SetPipeline(cfg PipelineConfig) {
	w.pipeline = cfg
}

// SetPolicy wires the CIP P5 per-connector sync policy resolver and the
// Governor's per-connector cost-ceiling hooks. resolver may be nil (the worker
// then uses built-in default policies). recordCost/resetCost may be nil to
// disable cost-ceiling enforcement. The Governor remains authoritative — these
// hooks only supply the per-connector inputs.
func (w *Worker) SetPolicy(
	resolver *policy.Resolver,
	recordCost func(connectorID string, usd, ceiling float64) (float64, error),
	resetCost func(connectorID string),
) {
	w.resolver = resolver
	w.recordCost = recordCost
	w.resetCost = resetCost
}

// resolvePolicy returns the effective policy for a connector, falling back to
// the built-in default when no resolver is wired.
func (w *Worker) resolvePolicy(ctx context.Context, connectorID string) policy.SyncPolicy {
	if w.resolver != nil {
		return w.resolver.Resolve(ctx, connectorID)
	}
	return policy.DefaultPolicy(connectorID)
}

// Run subscribes to navi.refinery.queue and processes messages until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	var admitted, deduped, errored atomic.Int64

	flush := func() {
		a, d, e := admitted.Load(), deduped.Load(), errored.Load()
		if a+d+e > 0 {
			w.log.Info("intake: worker: pass counters",
				"admitted", a, "deduped", d, "errored", e)
		}
		// CIP P5: close any open passes with activity into intake_sync_log rows.
		w.FlushPasses(ctx)
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				flush()
			}
		}
	}()

	sub, err := w.js.Subscribe(
		RefinerySubject,
		func(msg *nats.Msg) {
			var r IntakeRecord
			if err := json.Unmarshal(msg.Data, &r); err != nil {
				w.log.Error("intake: worker: unmarshal failed", "err", err)
				_ = msg.Term()
				errored.Add(1)
				return
			}
			outcome, _ := w.processRecord(ctx, r)
			switch outcome {
			case outcomeAdmitted:
				admitted.Add(1)
				_ = msg.Ack()
			case outcomeDeduped:
				deduped.Add(1)
				_ = msg.Ack()
			case outcomeHalted:
				// Cost ceiling tripped — drop cleanly (acking avoids a redelivery loop).
				_ = msg.Ack()
			case outcomeErrored:
				errored.Add(1)
				_ = msg.Nak()
			}
		},
		nats.Durable(workerConsumerName),
		nats.ManualAck(),
	)
	if err != nil {
		return fmt.Errorf("intake: worker: subscribe: %w", err)
	}
	defer sub.Unsubscribe() //nolint:errcheck

	<-ctx.Done()
	flush()
	return ctx.Err()
}

// processRecord applies the Admit stage (with policy-driven privacy class),
// deduplication, per-pass budget + per-connector cost-ceiling enforcement, and
// persistence to a single record. Extracted for unit-testability without a NATS
// connection.
func (w *Worker) processRecord(ctx context.Context, r IntakeRecord) (messageOutcome, error) {
	if w.recordAction != nil {
		if err := w.recordAction(); err != nil {
			w.log.Warn("intake: worker: action budget tripped", "err", err)
			return outcomeErrored, err
		}
	}

	pol := w.resolvePolicy(ctx, r.ConnectorID)

	// Admit — PrivacyClass is sourced from the connector's sync policy when the
	// record carries none (CIP §9, now policy-driven per CIP P5 §7).
	admitted, err := AdmitRecordWithDefaults(r, pol.PrivacyClass)
	if err != nil {
		w.log.Error("intake: worker: admit failed",
			"err", err, "connector_id", r.ConnectorID, "source_id", r.SourceID)
		w.passBumpError(ctx, r.ConnectorID, pol)
		return outcomeErrored, err
	}

	exists, err := store.IntakeRecordExists(ctx, w.db, admitted.ConnectorID, admitted.SourceID)
	if err != nil {
		w.log.Error("intake: worker: dedupe check failed",
			"err", err, "connector_id", admitted.ConnectorID, "source_id", admitted.SourceID)
		w.passBumpError(ctx, admitted.ConnectorID, pol)
		return outcomeErrored, err
	}
	if exists {
		w.log.Debug("intake: worker: duplicate skipped",
			"connector_id", admitted.ConnectorID, "source_id", admitted.SourceID)
		w.passBumpDeduped(ctx, admitted.ConnectorID, pol)
		// Resume-after-crash: re-run downstream stages for the stored record.
		w.resumeDownstream(ctx, admitted.ConnectorID, admitted.SourceID)
		return outcomeDeduped, nil
	}

	// CIP P5: per-pass budget (records/bytes). Exceeding it terminates the pass
	// cleanly; a fresh pass then opens so steady-state ingestion is segmented,
	// not dropped (the hard halt is the cost ceiling below).
	n := int64(len(admitted.Raw))
	if !w.passCanAdmit(ctx, admitted.ConnectorID, schema.JobModeDelta, pol, n) {
		w.closePass(ctx, admitted.ConnectorID, schema.SyncStatusBudgetExceeded)
	}

	// CIP P5: per-connector cost ceiling, enforced by the Governor (authoritative).
	if w.recordCost != nil {
		ceiling := pol.Block(schema.JobModeDelta).Budget.CostCeilingUSD
		cost := policy.EstimateRecordCostUSD(len(admitted.Raw))
		if _, err := w.recordCost(admitted.ConnectorID, cost, ceiling); err != nil {
			w.log.Warn("intake: worker: per-connector cost ceiling tripped — halting pass",
				"connector_id", admitted.ConnectorID, "err", err)
			w.closePass(ctx, admitted.ConnectorID, schema.SyncStatusCostCeiling)
			if w.resetCost != nil {
				w.resetCost(admitted.ConnectorID)
			}
			return outcomeHalted, nil
		}
	}

	if admitted.ID == "" {
		admitted.ID = uuid.New().String()
	}

	if err := store.SaveIntakeRecord(ctx, w.db, admitted); err != nil {
		if errors.Is(err, store.ErrIntakeDuplicate) {
			w.passBumpDeduped(ctx, admitted.ConnectorID, pol)
			w.resumeDownstream(ctx, admitted.ConnectorID, admitted.SourceID)
			return outcomeDeduped, nil
		}
		w.log.Error("intake: worker: save failed",
			"err", err, "connector_id", admitted.ConnectorID, "source_id", admitted.SourceID)
		w.passBumpError(ctx, admitted.ConnectorID, pol)
		return outcomeErrored, err
	}

	w.log.Info("intake: worker: admitted",
		"connector_id", admitted.ConnectorID, "source_id", admitted.SourceID,
		"trust", admitted.Trust, "privacy_class", admitted.PrivacyClass)
	w.passBumpAdmitted(ctx, admitted.ConnectorID, schema.JobModeDelta, pol, n)

	// Stages 3–5 (+ 6–9 when synthesis is enabled). Non-fatal downstream failure.
	m, err := RunStages(ctx, w.db, admitted, w.pipeline, w.log)
	if err != nil {
		w.log.Error("intake: worker: downstream stages failed",
			"err", err, "record_id", admitted.ID,
			"connector_id", admitted.ConnectorID, "source_id", admitted.SourceID)
		w.passBumpError(ctx, admitted.ConnectorID, pol)
	} else {
		w.passBumpStages(ctx, admitted.ConnectorID, pol, m.Persisted, m.Synthesized)
	}
	return outcomeAdmitted, nil
}

// resumeDownstream re-runs stages 3–5 for an already-admitted record when no
// chunks have been persisted yet (a pass that crashed between record save and
// chunk persistence). It is a no-op when chunks already exist.
func (w *Worker) resumeDownstream(ctx context.Context, connectorID, sourceID string) {
	rec, err := store.GetIntakeRecordBySource(ctx, w.db, connectorID, sourceID)
	if err != nil {
		return
	}
	n, err := store.CountIntakeChunksByRecord(ctx, w.db, rec.ID)
	if err != nil || n > 0 {
		return
	}
	if _, err := RunStages(ctx, w.db, rec, w.pipeline, w.log); err != nil {
		w.log.Error("intake: worker: resume downstream failed",
			"err", err, "record_id", rec.ID,
			"connector_id", connectorID, "source_id", sourceID)
	}
}
