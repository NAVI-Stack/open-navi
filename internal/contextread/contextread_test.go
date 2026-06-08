package contextread_test

import (
	"context"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/contextread"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
	"github.com/open-navi/navi/internal/worldmodel"
)

// newTestMediator wires the mediator against real implementations (an in-memory
// store + the world-model façade), per the repo's "actual implementations in
// tests" rule. The audit sink is the real append-only event log.
func newTestMediator(t *testing.T) (*contextread.Mediator, *worldmodel.WorldModel, func(context.Context, schema.Event) error) {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.CreateTables(context.Background(), db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	wm := worldmodel.New(db)
	audit := func(ctx context.Context, ev schema.Event) error {
		return store.AppendEvent(ctx, db, ev)
	}
	return contextread.NewMediator(wm, audit), wm, audit
}

func seedRun(t *testing.T, wm *worldmodel.WorldModel) {
	t.Helper()
	now := time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)
	end := now.Add(2 * time.Second)
	eo := schema.ExecutionOutcome{
		AttemptID:          "run-1",
		CommandID:          "cmd-1",
		AttemptNumber:      1,
		CommandType:        schema.CommandTypeInvoke,
		StartTime:          now,
		EndTime:            &end,
		Outcome:            schema.ExecutionOutcomeFailed,
		FailureClass:       schema.FailureClassExecutionFailure,
		FailureReason:      "secret token leaked in this free-text reason",
		AffectedEntities:   `["contact:alice","memory:m-7"]`,
		CompensationStatus: schema.CompensationStatusNotRequired,
		RecoveryStatus:     schema.RecoveryStatusNotRequired,
		CorrelationID:      "corr-internal",
		ProposalID:         "prop-internal",
		LLMProvider:        "anthropic",
		LLMModel:           "claude-sonnet-4-20250514",
	}
	if err := store.SaveExecutionOutcome(context.Background(), wm.DB(), eo); err != nil {
		t.Fatalf("SaveExecutionOutcome: %v", err)
	}
}

func TestQueryMissingPurposeRejected(t *testing.T) {
	m, _, _ := newTestMediator(t)
	_, err := m.Query(context.Background(), contextread.Request{
		RunID: "run-1",
		Scope: contextread.ScopeCurrentRunSummary,
	})
	if !contextread.IsRejection(err) {
		t.Fatalf("expected rejection, got %v", err)
	}
	if err != contextread.ErrMissingPurpose {
		t.Fatalf("expected ErrMissingPurpose, got %v", err)
	}
}

func TestQueryMissingScopeRejected(t *testing.T) {
	m, _, _ := newTestMediator(t)
	_, err := m.Query(context.Background(), contextread.Request{
		RunID:   "run-1",
		Purpose: contextread.PurposeEvalScoring,
	})
	if err != contextread.ErrMissingScope {
		t.Fatalf("expected ErrMissingScope, got %v", err)
	}
}

func TestQueryUnknownPurposeRejected(t *testing.T) {
	m, _, _ := newTestMediator(t)
	_, err := m.Query(context.Background(), contextread.Request{
		RunID:   "run-1",
		Purpose: contextread.Purpose("exfiltrate"),
		Scope:   contextread.ScopeCurrentRunSummary,
	})
	if err != contextread.ErrUnknownPurpose {
		t.Fatalf("expected ErrUnknownPurpose, got %v", err)
	}
	if !contextread.IsRejection(err) {
		t.Fatalf("unknown purpose must be a rejection")
	}
}

func TestQueryUnknownScopeRejected(t *testing.T) {
	m, _, _ := newTestMediator(t)
	_, err := m.Query(context.Background(), contextread.Request{
		RunID:   "run-1",
		Purpose: contextread.PurposeEvalScoring,
		Scope:   contextread.Scope("everything"),
	})
	if err != contextread.ErrUnknownScope {
		t.Fatalf("expected ErrUnknownScope, got %v", err)
	}
}

func TestQueryMissingRunIDRejected(t *testing.T) {
	m, _, _ := newTestMediator(t)
	_, err := m.Query(context.Background(), contextread.Request{
		Purpose: contextread.PurposeEvalScoring,
		Scope:   contextread.ScopeCurrentRunSummary,
	})
	if err != contextread.ErrMissingRunID {
		t.Fatalf("expected ErrMissingRunID, got %v", err)
	}
}

func TestQueryRunNotFound(t *testing.T) {
	m, _, _ := newTestMediator(t)
	_, err := m.Query(context.Background(), contextread.Request{
		RunID:   "does-not-exist",
		Purpose: contextread.PurposeEvalScoring,
		Scope:   contextread.ScopeCurrentRunSummary,
	})
	if err != contextread.ErrRunNotFound {
		t.Fatalf("expected ErrRunNotFound, got %v", err)
	}
	if contextread.IsRejection(err) {
		t.Fatalf("run-not-found is a 404, not a policy rejection")
	}
}

func TestQueryHappyPathRedactedProvenanceAndAudit(t *testing.T) {
	m, wm, _ := newTestMediator(t)
	seedRun(t, wm)
	ctx := context.Background()

	resp, err := m.Query(ctx, contextread.Request{
		RunID:   "run-1",
		Purpose: contextread.PurposeEvalScoring,
		Scope:   contextread.ScopeCurrentRunSummary,
		Caller:  "owner:test",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	// Scope-bounded summary fields present.
	if resp.Context["outcome"] != string(schema.ExecutionOutcomeFailed) {
		t.Fatalf("expected outcome failed, got %v", resp.Context["outcome"])
	}
	if resp.Context["failure_class"] != string(schema.FailureClassExecutionFailure) {
		t.Fatalf("expected failure_class, got %v", resp.Context["failure_class"])
	}

	// Redaction: raw free-text and entity references must NOT appear.
	if _, ok := resp.Context["failure_reason"]; ok {
		t.Fatalf("failure_reason must be redacted, got %v", resp.Context["failure_reason"])
	}
	if resp.Context["has_failure_reason"] != true {
		t.Fatalf("expected has_failure_reason flag")
	}
	if _, ok := resp.Context["affected_entities"]; ok {
		t.Fatalf("affected_entities must be redacted")
	}
	if resp.Context["affected_entity_count"] != 2 {
		t.Fatalf("expected affected_entity_count=2, got %v", resp.Context["affected_entity_count"])
	}
	// Least-context: internal correlation plumbing must not leak.
	for _, leaked := range []string{"correlation_id", "proposal_id", "command_id", "parent_run_id", "runtime_session_id"} {
		if _, ok := resp.Context[leaked]; ok {
			t.Fatalf("least-context violation: %q leaked", leaked)
		}
	}

	// Provenance tagging.
	if resp.Provenance.Source != contextread.Source || !resp.Provenance.KernelMediated {
		t.Fatalf("missing provenance markers: %+v", resp.Provenance)
	}
	if resp.Provenance.Purpose != contextread.PurposeEvalScoring || resp.Provenance.Scope != contextread.ScopeCurrentRunSummary {
		t.Fatalf("provenance purpose/scope mismatch: %+v", resp.Provenance)
	}
	assertContains(t, resp.Provenance.Redactions, "failure_reason")
	assertContains(t, resp.Provenance.Redactions, "affected_entities")

	// Audit attribution: a context.read fact recorded for this run.
	events, err := store.EventsByCorrelationID(ctx, wm.DB(), "run-1")
	if err != nil {
		t.Fatalf("EventsByCorrelationID: %v", err)
	}
	var audited bool
	for _, ev := range events {
		if ev.Type == schema.FactContextRead {
			audited = true
		}
	}
	if !audited {
		t.Fatalf("expected a context.read audit event, got %d events", len(events))
	}
}

// TestQueryReadOnlyNoWriteOrScheduleReachable asserts that a governed read leaves
// authoritative state untouched: it adds no execution outcomes, creates no
// proposals, and the only event written is the append-only audit fact.
func TestQueryReadOnlyNoWriteOrScheduleReachable(t *testing.T) {
	m, wm, _ := newTestMediator(t)
	seedRun(t, wm)
	ctx := context.Background()
	db := wm.DB()

	runsBefore, _, err := store.ListExecutionOutcomes(ctx, db, 200, "", nil)
	if err != nil {
		t.Fatalf("ListExecutionOutcomes: %v", err)
	}
	proposalsBefore, err := store.ListPendingProposals(ctx, db, 200)
	if err != nil {
		t.Fatalf("ListPendingProposals: %v", err)
	}

	if _, err := m.Query(ctx, contextread.Request{
		RunID:   "run-1",
		Purpose: contextread.PurposeEvalScoring,
		Scope:   contextread.ScopeCurrentRunSummary,
		Caller:  "owner:test",
	}); err != nil {
		t.Fatalf("Query: %v", err)
	}

	runsAfter, _, err := store.ListExecutionOutcomes(ctx, db, 200, "", nil)
	if err != nil {
		t.Fatalf("ListExecutionOutcomes: %v", err)
	}
	if len(runsAfter) != len(runsBefore) {
		t.Fatalf("read mutated execution ledger: %d → %d", len(runsBefore), len(runsAfter))
	}
	proposalsAfter, err := store.ListPendingProposals(ctx, db, 200)
	if err != nil {
		t.Fatalf("ListPendingProposals: %v", err)
	}
	if len(proposalsAfter) != len(proposalsBefore) {
		t.Fatalf("read created a proposal/schedule: %d → %d", len(proposalsBefore), len(proposalsAfter))
	}

	// The only event produced by a read is the audit fact — no command events.
	events, _, err := store.ListEvents(ctx, db, 0, nil, 200)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	for _, ev := range events {
		if ev.Kind == schema.EventKindCommand {
			t.Fatalf("read emitted a command event %q — not read-only", ev.Type)
		}
		if ev.Type != schema.FactContextRead {
			t.Fatalf("unexpected event type from read: %q", ev.Type)
		}
	}
}

func assertContains(t *testing.T, list []string, want string) {
	t.Helper()
	for _, s := range list {
		if s == want {
			return
		}
	}
	t.Fatalf("expected %q in %v", want, list)
}
