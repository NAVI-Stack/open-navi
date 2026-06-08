package intake

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/governor"
	"github.com/open-navi/navi/internal/intake/policy"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

func policyRecord(connectorID, sourceID, body string) IntakeRecord {
	return IntakeRecord{
		ConnectorID: connectorID,
		SourceKind:  "message",
		SourceID:    sourceID,
		Cursor:      sourceID,
		FetchedAt:   time.Now().UTC(),
		Trust:       TrustExternalUntrusted,
		Raw:         []byte(body),
		RawMIME:     "text/plain",
		Provenance:  Provenance{ConnectorID: connectorID},
	}
}

// TestWorker_PrivacyClassFromPolicy verifies the Admit stage sources PrivacyClass
// from the connector's sync policy (CIP §7 / P5), not the hardcoded "personal".
func TestWorker_PrivacyClassFromPolicy(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()
	resolver := policy.NewResolver(db, nil)

	// Owner sets the connector's privacy floor to "sensitive".
	if err := resolver.SaveOverride(ctx, schema.SyncPolicy{
		ConnectorID:  "telegram:acct-1",
		PrivacyClass: schema.PrivacyClassSensitive,
	}); err != nil {
		t.Fatalf("SaveOverride: %v", err)
	}

	w := NewWorker(nil, db, nil, nil)
	w.SetPolicy(resolver, nil, nil)

	if _, err := w.processRecord(ctx, policyRecord("telegram:acct-1", "1:1", "hello world")); err != nil {
		t.Fatalf("processRecord: %v", err)
	}

	rec, err := store.GetIntakeRecordBySource(ctx, db, "telegram:acct-1", "1:1")
	if err != nil {
		t.Fatalf("get record: %v", err)
	}
	if rec.PrivacyClass != schema.PrivacyClassSensitive {
		t.Errorf("privacy_class: want sensitive (policy-driven), got %q", rec.PrivacyClass)
	}
}

// TestWorker_PerPassBudgetTerminatesPass verifies that exceeding the per-pass
// record budget terminates the current pass cleanly (a budget_exceeded sync-log
// row) and segments ingestion into a fresh pass — losslessly.
func TestWorker_PerPassBudgetTerminatesPass(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()
	resolver := policy.NewResolver(db, nil)

	// Tiny per-pass budget: 2 records.
	if err := resolver.SaveOverride(ctx, schema.SyncPolicy{
		ConnectorID: "telegram:acct-1",
		Delta:       schema.SyncModePolicy{Budget: schema.SyncBudget{MaxRecordsPerPass: 2}},
	}); err != nil {
		t.Fatalf("SaveOverride: %v", err)
	}

	w := NewWorker(nil, db, nil, nil)
	w.SetPolicy(resolver, nil, nil)

	// Admit 3 records: the 3rd should not fit the first pass.
	for i := 0; i < 3; i++ {
		if _, err := w.processRecord(ctx, policyRecord("telegram:acct-1", fmt.Sprintf("1:%d", i), "msg")); err != nil {
			t.Fatalf("processRecord %d: %v", i, err)
		}
	}
	w.FlushPasses(ctx)

	logs, err := store.ListIntakeSyncLog(ctx, db, "telegram:acct-1", 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var budgetExceeded, completed *schema.IntakeSyncLogEntry
	for i := range logs {
		switch logs[i].TerminalStatus {
		case schema.SyncStatusBudgetExceeded:
			budgetExceeded = &logs[i]
		case schema.SyncStatusCompleted:
			completed = &logs[i]
		}
	}
	if budgetExceeded == nil {
		t.Fatal("expected a budget_exceeded sync-log row")
	}
	if budgetExceeded.RecordsAdmitted != 2 {
		t.Errorf("terminated pass should hold exactly the budgeted 2 records, got %d", budgetExceeded.RecordsAdmitted)
	}
	if completed == nil || completed.RecordsAdmitted != 1 {
		t.Errorf("expected a fresh pass with the 3rd record; got %+v", completed)
	}
	// All three records are durable — segmentation is lossless.
	if n, _ := store.CountIntakeRecordsByConnector(ctx, db, "telegram:acct-1"); n != 3 {
		t.Errorf("expected all 3 records persisted, got %d", n)
	}
}

// TestWorker_CostCeilingHaltsRunawayPass is the hard invariant: the Governor —
// not the policy — decides budgets. A connector cannot exceed its configured
// per-connector cost ceiling regardless of how the policy is edited.
func TestWorker_CostCeilingHaltsRunawayPass(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()
	resolver := policy.NewResolver(db, nil)

	// A vanishingly small cost ceiling — any real record trips it.
	if err := resolver.SaveOverride(ctx, schema.SyncPolicy{
		ConnectorID: "telegram:acct-1",
		Delta:       schema.SyncModePolicy{Budget: schema.SyncBudget{CostCeilingUSD: 0.0000001}},
	}); err != nil {
		t.Fatalf("SaveOverride: %v", err)
	}

	gov := governor.NewGovernor(governor.DefaultGovernorConfig(), t.TempDir())
	w := NewWorker(nil, db, nil, nil)
	w.SetPolicy(resolver, gov.RecordConnectorCost, gov.ResetConnectorCost)

	// The first sizeable record should trip the per-connector cost ceiling.
	outcome, _ := w.processRecord(ctx, policyRecord("telegram:acct-1", "1:1",
		"this is a sizeable message body that costs more than a fraction of a microdollar to process"))
	if outcome != outcomeHalted {
		t.Fatalf("expected outcomeHalted from cost ceiling, got %v", outcome)
	}

	logs, err := store.ListIntakeSyncLog(ctx, db, "telegram:acct-1", 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var halted bool
	for _, e := range logs {
		if e.TerminalStatus == schema.SyncStatusCostCeiling {
			halted = true
		}
	}
	if !halted {
		t.Error("expected a cost_ceiling sync-log row from the halted pass")
	}

	// The record must not have been persisted (the halt is real, not cosmetic).
	if exists, _ := store.IntakeRecordExists(ctx, db, "telegram:acct-1", "1:1"); exists {
		t.Error("a cost-ceiling-halted record must not be admitted")
	}
}

// TestWorker_SyncLogCapturesFullPass verifies a normal pass is captured with
// accurate admitted/distilled counters.
func TestWorker_SyncLogCapturesFullPass(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()
	w := NewWorker(nil, db, nil, nil)
	w.SetPolicy(policy.NewResolver(db, nil), nil, nil)

	for i := 0; i < 4; i++ {
		if _, err := w.processRecord(ctx, policyRecord("telegram:acct-1", fmt.Sprintf("2:%d", i), "hello pass")); err != nil {
			t.Fatalf("processRecord %d: %v", i, err)
		}
	}
	w.FlushPasses(ctx)

	last, ok, err := store.LatestIntakeSyncLog(ctx, db, "telegram:acct-1")
	if err != nil || !ok {
		t.Fatalf("latest: ok=%v err=%v", ok, err)
	}
	if last.RecordsAdmitted != 4 {
		t.Errorf("admitted: want 4, got %d", last.RecordsAdmitted)
	}
	if last.JobMode != schema.JobModeDelta {
		t.Errorf("job_mode: want delta, got %q", last.JobMode)
	}
	if last.TerminalStatus != schema.SyncStatusCompleted {
		t.Errorf("status: want completed, got %q", last.TerminalStatus)
	}
	if last.EndedAt == nil {
		t.Error("expected ended_at set on a completed pass")
	}
}
