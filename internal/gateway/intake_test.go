package gateway

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

// TestIntakePolicyRoundTrip exercises the CIP P5 gateway surface: GET the
// effective policy for a connector, POST an override, and confirm the change
// round-trips through the store.
func TestIntakePolicyRoundTrip(t *testing.T) {
	srv, _, _ := testServer(t)

	// GET default policy for a connector.
	res := doReq(t, srv, "GET", "/api/intake/policy?connector=telegram:acct-1", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET policy: status %d", res.StatusCode)
	}
	var view connectorSyncView
	if err := json.NewDecoder(res.Body).Decode(&view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	res.Body.Close()
	if view.ConnectorID != "telegram:acct-1" {
		t.Errorf("connector_id: %q", view.ConnectorID)
	}
	if view.Policy.Delta.Cadence != schema.CadenceWebhookTriggered {
		t.Errorf("default delta cadence: %q", view.Policy.Delta.Cadence)
	}

	// POST an override.
	body := schema.SyncPolicy{
		ConnectorID:  "telegram:acct-1",
		PrivacyClass: schema.PrivacyClassSensitive,
		Delta:        schema.SyncModePolicy{Cadence: "90m"},
	}
	res = doReq(t, srv, "POST", "/api/intake/policy", "", "", "127.0.0.1:1234", body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST policy: status %d", res.StatusCode)
	}
	var saved connectorSyncView
	_ = json.NewDecoder(res.Body).Decode(&saved)
	res.Body.Close()
	if saved.Policy.Delta.Cadence != "90m" || saved.Policy.PrivacyClass != schema.PrivacyClassSensitive {
		t.Errorf("override not reflected: %+v", saved.Policy)
	}

	// GET again confirms persistence.
	res = doReq(t, srv, "GET", "/api/intake/policy?connector=telegram:acct-1", "", "", "127.0.0.1:1234", nil)
	_ = json.NewDecoder(res.Body).Decode(&view)
	res.Body.Close()
	if view.Policy.Delta.Cadence != "90m" {
		t.Errorf("persistence failed: %q", view.Policy.Delta.Cadence)
	}
}

func TestIntakePolicyValidationRejected(t *testing.T) {
	srv, _, _ := testServer(t)
	body := schema.SyncPolicy{ConnectorID: "telegram", PrivacyClass: "bogus"}
	res := doReq(t, srv, "POST", "/api/intake/policy", "", "", "127.0.0.1:1234", body)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid privacy_class, got %d", res.StatusCode)
	}
	res.Body.Close()
}

func TestIntakeSyncLogEndpoint(t *testing.T) {
	srv, _, db := testServer(t)

	if _, err := store.AppendIntakeSyncLog(t.Context(), db, schema.IntakeSyncLogEntry{
		ConnectorID: "telegram:acct-1", JobMode: schema.JobModeDelta,
		RecordsAdmitted: 7, TerminalStatus: schema.SyncStatusCompleted,
	}); err != nil {
		t.Fatalf("seed sync log: %v", err)
	}

	res := doReq(t, srv, "GET", "/api/intake/sync-log?connector=telegram:acct-1", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	var rows []schema.IntakeSyncLogEntry
	_ = json.NewDecoder(res.Body).Decode(&rows)
	res.Body.Close()
	if len(rows) != 1 || rows[0].RecordsAdmitted != 7 {
		t.Errorf("unexpected sync-log rows: %+v", rows)
	}
}

func TestVaultSyncLogEndpoint(t *testing.T) {
	srv, _, db := testServer(t)
	// Empty by default (Vault not yet shipping) — surface must still exist.
	res := doReq(t, srv, "GET", "/api/vault/sync-log", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	var rows []schema.VaultSyncLogEntry
	_ = json.NewDecoder(res.Body).Decode(&rows)
	res.Body.Close()
	if len(rows) != 0 {
		t.Errorf("expected empty vault log, got %d", len(rows))
	}

	// Seed one (simulating the Vault deliverable) and confirm it surfaces.
	if _, err := store.AppendVaultSyncLog(t.Context(), db, schema.VaultSyncLogEntry{
		FilePath: "people/alex.md", DiffSummary: "edit", TerminalStatus: schema.SyncStatusCompleted,
	}); err != nil {
		t.Fatalf("seed vault log: %v", err)
	}
	res = doReq(t, srv, "GET", "/api/vault/sync-log?path=people/alex.md", "", "", "127.0.0.1:1234", nil)
	_ = json.NewDecoder(res.Body).Decode(&rows)
	res.Body.Close()
	if len(rows) != 1 {
		t.Errorf("expected 1 vault row, got %d", len(rows))
	}
}

// TestBackfillConsentEndpoint verifies the backfill request raises a consent
// Proposal in the existing queue (source_process intake_consent).
func TestBackfillConsentEndpoint(t *testing.T) {
	srv, _, db := testServer(t)

	res := doReq(t, srv, "POST", "/api/intake/backfill", "", "", "127.0.0.1:1234",
		map[string]string{"connector": "telegram:acct-1"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	var out struct {
		ProposalID string `json:"proposal_id"`
		Status     string `json:"status"`
	}
	_ = json.NewDecoder(res.Body).Decode(&out)
	res.Body.Close()
	if out.ProposalID == "" || out.Status != "consent_pending" {
		t.Fatalf("unexpected backfill response: %+v", out)
	}

	prop, err := store.GetProposal(t.Context(), db, out.ProposalID)
	if err != nil {
		t.Fatalf("GetProposal: %v", err)
	}
	if prop.SourceProcess != "intake_consent" {
		t.Errorf("source_process: %q", prop.SourceProcess)
	}
}
