package intake

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/store"
)

func newTestWorker(t *testing.T) *Worker {
	t.Helper()
	db := store.InitTestDB(t)
	return NewWorker(nil, db, nil, nil)
}

func makeRecord(connectorID, sourceID string, trust ContentTrust) IntakeRecord {
	return IntakeRecord{
		ConnectorID: connectorID,
		SourceKind:  "message",
		SourceID:    sourceID,
		Trust:       trust,
		Raw:         []byte("test message"),
		RawMIME:     "text/plain",
		FetchedAt:   time.Now().UTC(),
	}
}

func TestWorker_ProcessRecord_HappyPath(t *testing.T) {
	w := newTestWorker(t)
	ctx := context.Background()

	r := makeRecord("telegram:acct-1", "100:1", TrustExternalUntrusted)
	outcome, err := w.processRecord(ctx, r)
	if err != nil {
		t.Fatalf("processRecord: %v", err)
	}
	if outcome != outcomeAdmitted {
		t.Errorf("outcome: want outcomeAdmitted, got %v", outcome)
	}
}

func TestWorker_ProcessRecord_Deduplication(t *testing.T) {
	w := newTestWorker(t)
	ctx := context.Background()

	r := makeRecord("telegram:acct-1", "100:2", TrustExternalUntrusted)

	o1, err := w.processRecord(ctx, r)
	if err != nil {
		t.Fatalf("first processRecord: %v", err)
	}
	if o1 != outcomeAdmitted {
		t.Errorf("first outcome: want outcomeAdmitted, got %v", o1)
	}

	// Same (connector_id, source_id) — must be deduped.
	o2, err := w.processRecord(ctx, r)
	if err != nil {
		t.Fatalf("second processRecord: %v", err)
	}
	if o2 != outcomeDeduped {
		t.Errorf("second outcome: want outcomeDeduped, got %v", o2)
	}
}

func TestWorker_ProcessRecord_AdmitValidation(t *testing.T) {
	w := newTestWorker(t)
	ctx := context.Background()

	// Record with unknown trust value — Admit must reject it.
	r := IntakeRecord{
		ConnectorID: "telegram:acct-1",
		SourceKind:  "message",
		SourceID:    "100:3",
		Trust:       "unknown",
		Raw:         []byte("bad"),
	}
	outcome, err := w.processRecord(ctx, r)
	if err == nil {
		t.Fatal("expected error for invalid trust value")
	}
	if outcome != outcomeErrored {
		t.Errorf("outcome: want outcomeErrored, got %v", outcome)
	}
}

func TestWorker_ProcessRecord_OwnerTrust(t *testing.T) {
	w := newTestWorker(t)
	ctx := context.Background()

	r := makeRecord("telegram:acct-1", "100:4", TrustOwner)
	outcome, err := w.processRecord(ctx, r)
	if err != nil {
		t.Fatalf("processRecord: %v", err)
	}
	if outcome != outcomeAdmitted {
		t.Errorf("outcome: want outcomeAdmitted, got %v", outcome)
	}

	exists, _ := store.IntakeRecordExists(ctx, w.db, "telegram:acct-1", "100:4")
	if !exists {
		t.Fatal("record not found in store after admit")
	}
}

func TestWorker_ProcessRecord_GovernorTripped(t *testing.T) {
	db := store.InitTestDB(t)
	// Action recorder that always fails immediately.
	tripped := errors.New("budget exceeded")
	recordAction := func() error { return tripped }

	w := NewWorker(nil, db, recordAction, nil)
	ctx := context.Background()

	r := makeRecord("telegram:acct-1", "100:5", TrustExternalUntrusted)
	outcome, err := w.processRecord(ctx, r)
	if err == nil {
		t.Fatal("expected budget error")
	}
	if outcome != outcomeErrored {
		t.Errorf("outcome: want outcomeErrored, got %v", outcome)
	}
}

func TestWorker_ProcessRecord_DefaultsApplied(t *testing.T) {
	w := newTestWorker(t)
	ctx := context.Background()

	// Minimal record — Admit should fill in defaults.
	r := IntakeRecord{
		ConnectorID: "telegram:acct-1",
		SourceKind:  "message",
		SourceID:    "100:6",
		Trust:       TrustExternalUntrusted,
		Raw:         []byte("minimal"),
	}
	outcome, err := w.processRecord(ctx, r)
	if err != nil {
		t.Fatalf("processRecord: %v", err)
	}
	if outcome != outcomeAdmitted {
		t.Errorf("outcome: want outcomeAdmitted, got %v", outcome)
	}
}
