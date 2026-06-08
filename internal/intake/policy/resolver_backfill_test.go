package policy

import (
	"context"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

func TestResolver_OverrideRoundTrip(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()
	r := NewResolver(db, nil)

	// Default before any override.
	base := r.Resolve(ctx, "telegram:acct-1")
	if base.Delta.Cadence != schema.CadenceWebhookTriggered {
		t.Fatalf("unexpected default cadence: %q", base.Delta.Cadence)
	}

	// Owner edits cadence + privacy class.
	edit := schema.SyncPolicy{
		ConnectorID:  "telegram:acct-1",
		PrivacyClass: schema.PrivacyClassSensitive,
		Delta:        schema.SyncModePolicy{Cadence: "60m"},
	}
	if err := r.SaveOverride(ctx, edit); err != nil {
		t.Fatalf("SaveOverride: %v", err)
	}

	got := r.Resolve(ctx, "telegram:acct-1")
	if got.Delta.Cadence != "60m" {
		t.Errorf("cadence override not applied: %q", got.Delta.Cadence)
	}
	if got.PrivacyClass != schema.PrivacyClassSensitive {
		t.Errorf("privacy override not applied: %q", got.PrivacyClass)
	}
	// Budget untouched → still the default.
	if got.Delta.Budget.MaxRecordsPerPass != base.Delta.Budget.MaxRecordsPerPass {
		t.Error("untouched budget should remain the default")
	}

	// Clearing reverts to default.
	if err := r.ClearOverride(ctx, "telegram:acct-1"); err != nil {
		t.Fatalf("ClearOverride: %v", err)
	}
	if r.Resolve(ctx, "telegram:acct-1").Delta.Cadence != schema.CadenceWebhookTriggered {
		t.Error("expected revert to default after clear")
	}
}

func TestResolver_SaveOverrideValidates(t *testing.T) {
	db := store.InitTestDB(t)
	r := NewResolver(db, nil)
	err := r.SaveOverride(context.Background(), schema.SyncPolicy{ConnectorID: ""})
	if err == nil {
		t.Error("expected validation error for empty connector_id")
	}
}

// TestBackfillConsentGate covers the acceptance contract: a backfill raises a
// Proposal, refuses to start without approval, starts after approval, and logs a
// consent_rejected sync-log row on rejection.
func TestBackfillConsentGate(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()
	pol := DefaultPolicy("telegram:acct-1")

	// Request raises a consent Proposal (cold start → not a resume).
	proposalID, resume, err := RequestBackfill(ctx, db, "telegram:acct-1", pol)
	if err != nil {
		t.Fatalf("RequestBackfill: %v", err)
	}
	if proposalID == "" {
		t.Fatal("expected a proposal id")
	}
	if resume {
		t.Error("cold start should not be a resume")
	}

	// The Proposal exists, is pending, and is tagged for consent.
	prop, err := store.GetProposal(ctx, db, proposalID)
	if err != nil {
		t.Fatalf("GetProposal: %v", err)
	}
	if prop.SourceProcess != SourceProcessConsent {
		t.Errorf("source_process: want %q, got %q", SourceProcessConsent, prop.SourceProcess)
	}
	if prop.Status != schema.ProposalStatusPending {
		t.Errorf("status: want pending, got %q", prop.Status)
	}

	// Without approval, the gate refuses to start.
	allowed, status, err := StartApprovedBackfill(ctx, db, "telegram:acct-1")
	if err != nil {
		t.Fatalf("StartApprovedBackfill: %v", err)
	}
	if allowed {
		t.Error("backfill must not start while consent is pending")
	}
	if status != schema.ProposalStatusPending {
		t.Errorf("status: want pending, got %q", status)
	}

	// A duplicate request reuses the open Proposal (no second queue, no dupe).
	id2, _, err := RequestBackfill(ctx, db, "telegram:acct-1", pol)
	if err != nil {
		t.Fatalf("RequestBackfill (dup): %v", err)
	}
	if id2 != proposalID {
		t.Errorf("expected reuse of pending proposal, got %q vs %q", id2, proposalID)
	}

	// Approve → gate allows the pass.
	if err := store.ResolveProposal(ctx, db, proposalID, schema.ProposalStatusApproved, schema.ResolutionTypeApprovedOnce, "owner", ""); err != nil {
		t.Fatalf("ResolveProposal: %v", err)
	}
	allowed, status, err = StartApprovedBackfill(ctx, db, "telegram:acct-1")
	if err != nil {
		t.Fatalf("StartApprovedBackfill (approved): %v", err)
	}
	if !allowed || status != schema.ProposalStatusApproved {
		t.Errorf("expected allowed+approved, got allowed=%v status=%q", allowed, status)
	}
}

func TestBackfillConsentRejectedLogsSyncRow(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()
	pol := DefaultPolicy("telegram:acct-1")

	proposalID, _, err := RequestBackfill(ctx, db, "telegram:acct-1", pol)
	if err != nil {
		t.Fatalf("RequestBackfill: %v", err)
	}
	if err := store.ResolveProposal(ctx, db, proposalID, schema.ProposalStatusDeclined, schema.ResolutionTypeDenied, "owner", "no thanks"); err != nil {
		t.Fatalf("ResolveProposal: %v", err)
	}

	allowed, status, err := StartApprovedBackfill(ctx, db, "telegram:acct-1")
	if err != nil {
		t.Fatalf("StartApprovedBackfill: %v", err)
	}
	if allowed {
		t.Error("declined consent must not start a backfill")
	}
	if status != schema.ProposalStatusDeclined {
		t.Errorf("status: want declined, got %q", status)
	}

	logs, err := store.ListIntakeSyncLog(ctx, db, "telegram:acct-1", 10)
	if err != nil {
		t.Fatalf("ListIntakeSyncLog: %v", err)
	}
	var found bool
	for _, e := range logs {
		if e.TerminalStatus == schema.SyncStatusConsentRejected && e.JobMode == schema.JobModeBackfill {
			found = true
		}
	}
	if !found {
		t.Error("expected a consent_rejected backfill sync-log row")
	}
}

func TestRequestBackfill_ResumeWhenHistoryExists(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()

	// Seed a historical record so the connector has prior state.
	if err := store.SaveIntakeRecord(ctx, db, schema.IntakeRecord{
		ID: "rec-1", ConnectorID: "telegram:acct-1", SourceKind: "message",
		SourceID: "1:1", Cursor: "cursor-42", FetchedAt: time.Now().UTC(),
		Trust: schema.ContentTrustOwner, PrivacyClass: schema.PrivacyClassPersonal,
		Raw: []byte("hello"), RawMIME: "text/plain",
	}); err != nil {
		t.Fatalf("seed record: %v", err)
	}

	_, resume, err := RequestBackfill(ctx, db, "telegram:acct-1", DefaultPolicy("telegram:acct-1"))
	if err != nil {
		t.Fatalf("RequestBackfill: %v", err)
	}
	if !resume {
		t.Error("expected resume=true when historical state exists")
	}
}
