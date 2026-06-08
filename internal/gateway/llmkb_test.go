package gateway

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/bus"
	"github.com/open-navi/navi/internal/cognitive"
	"github.com/open-navi/navi/internal/governor"
	"github.com/open-navi/navi/internal/llmkb"
	"github.com/open-navi/navi/internal/store"
)

func TestGatewayDebugLLMKBProfilesEndpoints(t *testing.T) {
	srv, db := testLLMKBServer(t)
	defer db.Close()
	repo := store.NewSQLiteLLMKBRepo(db)
	if err := repo.LoadLLMSeeds(context.Background(), filepath.Join("..", "..", "data", "seed", "models"), nil); err != nil {
		t.Fatalf("LoadLLMSeeds: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := repo.SaveRuntimeInstance(context.Background(), llmkb.LLMRuntimeInstance{
		InstanceID:        "ollama-runtime-1",
		LLMID:             "meta.llama-3-2-3b",
		ProviderID:        "ollama",
		RuntimeBackend:    llmkb.RuntimeBackendOllama,
		HealthStatus:      llmkb.HealthStatusHealthy,
		AuthStatus:        llmkb.AuthStatusAuthenticated,
		LoadedStatus:      llmkb.LoadedStatusLoaded,
		LastProbeAt:       &now,
	}); err != nil {
		t.Fatalf("SaveRuntimeInstance: %v", err)
	}

	listRes := doJSONReq(t, srv, "GET", "/api/debug/llmkb/profiles", "", "127.0.0.1:1234", nil)
	if listRes.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d", listRes.StatusCode)
	}
	var listBody struct {
		Profiles []map[string]any `json:"profiles"`
	}
	if err := json.NewDecoder(listRes.Body).Decode(&listBody); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listBody.Profiles) < 3 {
		t.Fatalf("expected at least 3 profiles, got %+v", listBody.Profiles)
	}

	detailRes := doJSONReq(t, srv, "GET", "/api/debug/llmkb/profiles/meta.llama-3-2-3b", "", "127.0.0.1:1234", nil)
	if detailRes.StatusCode != http.StatusOK {
		t.Fatalf("detail status = %d", detailRes.StatusCode)
	}
	var detailBody struct {
		Summary string `json:"summary"`
		Groups  []struct {
			Name           string         `json:"name"`
			Classification string         `json:"classification"`
			Provenance     map[string]any `json:"provenance"`
		} `json:"groups"`
		RuntimeInstances []map[string]any `json:"runtime_instances"`
	}
	if err := json.NewDecoder(detailRes.Body).Decode(&detailBody); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detailBody.Summary == "" {
		t.Fatal("expected non-empty summary")
	}
	if len(detailBody.Groups) == 0 {
		t.Fatal("expected field groups in detail response")
	}
	foundRouting := false
	foundClassification := false
	for _, group := range detailBody.Groups {
		if group.Name == "routing_profile" {
			foundRouting = true
		}
		if group.Classification != "" && group.Provenance["source"] != nil {
			foundClassification = true
		}
	}
	if !foundRouting || !foundClassification {
		t.Fatalf("expected routing group and visible classification/provenance, got %+v", detailBody.Groups)
	}
	if len(detailBody.RuntimeInstances) != 1 {
		t.Fatalf("expected runtime instance detail, got %+v", detailBody.RuntimeInstances)
	}
}

func TestGatewayDebugLLMKBProfileAliasAmbiguity(t *testing.T) {
	srv, db := testLLMKBServer(t)
	defer db.Close()
	repo := store.NewSQLiteLLMKBRepo(db)
	for _, profile := range []llmkb.LLMProfile{
		{
			LLMID:             "provider-a.chat-pro",
			CanonicalName:     "Chat Pro A",
			Aliases:           []string{"chat-pro"},
			ProviderID:        "provider-a",
			OperationalState:  llmkb.OperationalState{AvailabilityState: llmkb.AvailabilityStateAvailable},
			CreatedAt:         time.Now().UTC(),
		},
		{
			LLMID:             "provider-b.chat-pro",
			CanonicalName:     "Chat Pro B",
			Aliases:           []string{"chat-pro"},
			ProviderID:        "provider-b",
			OperationalState:  llmkb.OperationalState{AvailabilityState: llmkb.AvailabilityStateAvailable},
			CreatedAt:         time.Now().UTC(),
		},
	} {
		if err := repo.SaveProfile(context.Background(), profile); err != nil {
			t.Fatalf("SaveProfile(%s): %v", profile.LLMID, err)
		}
	}

	res := doJSONReq(t, srv, "GET", "/api/debug/llmkb/profiles/chat-pro", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for ambiguous alias, got %d", res.StatusCode)
	}
	var body struct {
		Error   string   `json:"error"`
		Alias   string   `json:"alias"`
		Matches []string `json:"matches"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode ambiguity response: %v", err)
	}
	if body.Alias != "chat-pro" || len(body.Matches) != 2 {
		t.Fatalf("unexpected ambiguity response: %+v", body)
	}
}

func testLLMKBServer(t *testing.T) (*Server, *sql.DB) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "gateway-llmkb.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateTables(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		DB:              db,
		DirectiveWriter: cognitive.StoreDirectiveWriter(db),
		Bus:             bus.NewMemBus(db),
		Governor:        governor.NewGovernor(governor.GovernorConfig{}, "."),
		Registry:        NewConnectorRegistry(),
		Addr:            ":0",
	}
	return NewServer(cfg), db
}
