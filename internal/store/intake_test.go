package store

import (
	"context"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

func TestSaveAndGetIntakeRecord(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()

	ft := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	r := schema.IntakeRecord{
		ID:           "rec-001",
		ConnectorID:  "telegram:acct-1",
		SourceKind:   "message",
		SourceID:     "100:1",
		Cursor:       "100:1",
		FetchedAt:    ft,
		Trust:        schema.ContentTrustExternalUntrusted,
		PrivacyClass: schema.PrivacyClassPersonal,
		Author:       "alice",
		Raw:          []byte("hello world"),
		RawMIME:      "text/plain",
		Provenance: schema.IntakeProvenance{
			ConnectorID: "telegram:acct-1",
			AccountID:   "acct-1",
			LinkBack:    "https://t.me/c/100/1",
		},
	}

	if err := SaveIntakeRecord(ctx, db, r); err != nil {
		t.Fatalf("SaveIntakeRecord: %v", err)
	}

	got, err := GetIntakeRecord(ctx, db, "rec-001")
	if err != nil {
		t.Fatalf("GetIntakeRecord: %v", err)
	}

	if got.ConnectorID != r.ConnectorID {
		t.Errorf("ConnectorID: want %q, got %q", r.ConnectorID, got.ConnectorID)
	}
	if got.SourceKind != r.SourceKind {
		t.Errorf("SourceKind: want %q, got %q", r.SourceKind, got.SourceKind)
	}
	if got.SourceID != r.SourceID {
		t.Errorf("SourceID: want %q, got %q", r.SourceID, got.SourceID)
	}
	if got.Trust != r.Trust {
		t.Errorf("Trust: want %q, got %q", r.Trust, got.Trust)
	}
	if got.PrivacyClass != r.PrivacyClass {
		t.Errorf("PrivacyClass: want %q, got %q", r.PrivacyClass, got.PrivacyClass)
	}
	if got.Author != r.Author {
		t.Errorf("Author: want %q, got %q", r.Author, got.Author)
	}
	if string(got.Raw) != string(r.Raw) {
		t.Errorf("Raw: want %q, got %q", r.Raw, got.Raw)
	}
	if got.RawMIME != r.RawMIME {
		t.Errorf("RawMIME: want %q, got %q", r.RawMIME, got.RawMIME)
	}
	if got.Provenance.ConnectorID != r.Provenance.ConnectorID {
		t.Errorf("Provenance.ConnectorID: want %q, got %q", r.Provenance.ConnectorID, got.Provenance.ConnectorID)
	}
	if got.Provenance.AccountID != r.Provenance.AccountID {
		t.Errorf("Provenance.AccountID: want %q, got %q", r.Provenance.AccountID, got.Provenance.AccountID)
	}
	if got.Provenance.LinkBack != r.Provenance.LinkBack {
		t.Errorf("Provenance.LinkBack: want %q, got %q", r.Provenance.LinkBack, got.Provenance.LinkBack)
	}
}

func TestSaveIntakeRecord_Duplicate(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()

	r := schema.IntakeRecord{
		ID:          "rec-002",
		ConnectorID: "telegram:acct-1",
		SourceKind:  "message",
		SourceID:    "100:2",
		Trust:       schema.ContentTrustOwner,
		Raw:         []byte("msg"),
		RawMIME:     "text/plain",
		FetchedAt:   time.Now().UTC(),
	}

	if err := SaveIntakeRecord(ctx, db, r); err != nil {
		t.Fatalf("first save: %v", err)
	}

	r2 := r
	r2.ID = "rec-003" // different PK, same connector+source
	err := SaveIntakeRecord(ctx, db, r2)
	if err == nil {
		t.Fatal("expected ErrIntakeDuplicate on duplicate (connector_id, source_id)")
	}
	if err != ErrIntakeDuplicate {
		t.Errorf("want ErrIntakeDuplicate, got %v", err)
	}
}

func TestIntakeRecordExists(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()

	exists, err := IntakeRecordExists(ctx, db, "telegram:acct-1", "100:3")
	if err != nil {
		t.Fatalf("IntakeRecordExists: %v", err)
	}
	if exists {
		t.Fatal("expected false for non-existent record")
	}

	r := schema.IntakeRecord{
		ConnectorID: "telegram:acct-1",
		SourceKind:  "message",
		SourceID:    "100:3",
		Trust:       schema.ContentTrustExternalUntrusted,
		Raw:         []byte("hi"),
		RawMIME:     "text/plain",
		FetchedAt:   time.Now().UTC(),
	}
	if err := SaveIntakeRecord(ctx, db, r); err != nil {
		t.Fatalf("SaveIntakeRecord: %v", err)
	}

	exists, err = IntakeRecordExists(ctx, db, "telegram:acct-1", "100:3")
	if err != nil {
		t.Fatalf("IntakeRecordExists after insert: %v", err)
	}
	if !exists {
		t.Fatal("expected true after insert")
	}
}

func TestGetIntakeRecord_NotFound(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()

	_, err := GetIntakeRecord(ctx, db, "nonexistent-id")
	if err == nil {
		t.Fatal("expected error for non-existent record")
	}
}
