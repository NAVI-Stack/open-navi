package store

import (
	"context"
	"testing"

	"github.com/open-navi/navi/internal/schema"
)

func TestConnectorSyncPolicyOverride_RoundTrip(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()

	if _, ok, err := GetConnectorSyncPolicyOverride(ctx, db, "telegram"); err != nil || ok {
		t.Fatalf("expected no override initially (ok=%v err=%v)", ok, err)
	}

	p := schema.SyncPolicy{
		ConnectorID:  "telegram",
		PrivacyClass: schema.PrivacyClassSensitive,
		Visibility:   schema.SyncVisibilityLogged,
		Delta:        schema.SyncModePolicy{Cadence: "30m"},
	}
	if err := SaveConnectorSyncPolicyOverride(ctx, db, p); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, ok, err := GetConnectorSyncPolicyOverride(ctx, db, "telegram")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if got.PrivacyClass != schema.PrivacyClassSensitive || got.Delta.Cadence != "30m" {
		t.Errorf("roundtrip mismatch: %+v", got)
	}

	// Upsert replaces.
	p.Delta.Cadence = "90m"
	if err := SaveConnectorSyncPolicyOverride(ctx, db, p); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, _, _ = GetConnectorSyncPolicyOverride(ctx, db, "telegram")
	if got.Delta.Cadence != "90m" {
		t.Errorf("upsert not applied: %q", got.Delta.Cadence)
	}

	all, err := ListConnectorSyncPolicyOverrides(ctx, db)
	if err != nil || len(all) != 1 {
		t.Fatalf("list: len=%d err=%v", len(all), err)
	}

	if err := DeleteConnectorSyncPolicyOverride(ctx, db, "telegram"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok, _ := GetConnectorSyncPolicyOverride(ctx, db, "telegram"); ok {
		t.Error("expected override gone after delete")
	}
}
