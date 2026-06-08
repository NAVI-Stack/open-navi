package intake

import (
	"testing"
	"time"
)

func TestAdmitRecord_HappyPath(t *testing.T) {
	r := IntakeRecord{
		ConnectorID: "telegram:acct-1",
		SourceKind:  "message",
		SourceID:    "123:456",
		Trust:       TrustExternalUntrusted,
		Raw:         []byte("hello"),
	}
	got, err := AdmitRecord(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.PrivacyClass != PrivacyPersonal {
		t.Errorf("PrivacyClass: want %q, got %q", PrivacyPersonal, got.PrivacyClass)
	}
	if got.RawMIME != "text/plain" {
		t.Errorf("RawMIME: want %q, got %q", "text/plain", got.RawMIME)
	}
	if got.FetchedAt.IsZero() {
		t.Error("FetchedAt should be stamped to now")
	}
	if got.Provenance.ConnectorID != "telegram:acct-1" {
		t.Errorf("Provenance.ConnectorID: want %q, got %q", "telegram:acct-1", got.Provenance.ConnectorID)
	}
}

func TestAdmitRecord_OwnerTrust(t *testing.T) {
	r := IntakeRecord{
		ConnectorID: "telegram:acct-1",
		SourceKind:  "message",
		SourceID:    "123:457",
		Trust:       TrustOwner,
		Raw:         []byte("owner msg"),
	}
	got, err := AdmitRecord(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Trust != TrustOwner {
		t.Errorf("Trust: want %q, got %q", TrustOwner, got.Trust)
	}
}

func TestAdmitRecord_ExistingValuesPreserved(t *testing.T) {
	ft := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	r := IntakeRecord{
		ConnectorID:  "telegram:acct-1",
		SourceKind:   "message",
		SourceID:     "123:458",
		Trust:        TrustExternalUntrusted,
		PrivacyClass: PrivacySensitive,
		RawMIME:      "text/html",
		FetchedAt:    ft,
		Provenance:   Provenance{ConnectorID: "custom", AccountID: "acct"},
	}
	got, err := AdmitRecord(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.PrivacyClass != PrivacySensitive {
		t.Errorf("PrivacyClass should be preserved; got %q", got.PrivacyClass)
	}
	if got.RawMIME != "text/html" {
		t.Errorf("RawMIME should be preserved; got %q", got.RawMIME)
	}
	if !got.FetchedAt.Equal(ft) {
		t.Errorf("FetchedAt should be preserved; got %v", got.FetchedAt)
	}
	if got.Provenance.ConnectorID != "custom" {
		t.Errorf("Provenance.ConnectorID should be preserved; got %q", got.Provenance.ConnectorID)
	}
}

func TestAdmitRecord_MissingConnectorID(t *testing.T) {
	r := IntakeRecord{SourceID: "x", Trust: TrustOwner}
	_, err := AdmitRecord(r)
	if err == nil {
		t.Fatal("expected error for missing ConnectorID")
	}
}

func TestAdmitRecord_MissingSourceID(t *testing.T) {
	r := IntakeRecord{ConnectorID: "x", Trust: TrustOwner}
	_, err := AdmitRecord(r)
	if err == nil {
		t.Fatal("expected error for missing SourceID")
	}
}

func TestAdmitRecord_UnknownTrust(t *testing.T) {
	r := IntakeRecord{
		ConnectorID: "telegram:acct-1",
		SourceID:    "123:1",
		Trust:       "banana",
	}
	_, err := AdmitRecord(r)
	if err == nil {
		t.Fatal("expected error for unknown trust value")
	}
}

func TestAdmitRecord_EmptyTrust(t *testing.T) {
	r := IntakeRecord{
		ConnectorID: "telegram:acct-1",
		SourceID:    "123:1",
		Trust:       "",
	}
	_, err := AdmitRecord(r)
	if err == nil {
		t.Fatal("expected error for empty trust value")
	}
}
