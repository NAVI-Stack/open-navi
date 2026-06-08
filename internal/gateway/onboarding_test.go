package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-navi/navi/internal/connectors"
	"github.com/open-navi/navi/internal/store"
	"github.com/open-navi/navi/internal/worldmodel"
)

func TestHandleOnboardingRecoveryCreatesOwnerContact(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatalf("create tables: %v", err)
	}

	wm := worldmodel.New(db)
	dataDir := t.TempDir()
	srv := &Server{
		cfg: Config{
			DB:         db,
			DataDir:    dataDir,
			WorldModel: wm,
		},
	}

	body, _ := json.Marshal(map[string]any{
		"owner_name":   "Test Owner",
		"owner_handle": "test_owner",
		"device_name":  "Test Device",
		"save_path":    filepath.Join(dataDir, "passport.txt"),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/onboarding/recovery", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()

	srv.handleOnboardingRecovery(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("expected status 201 Created, got %d", res.StatusCode)
	}

	owner, exists, err := store.GetOwner(ctx, db)
	if err != nil || !exists {
		t.Fatalf("expected owner to exist, err=%v exists=%v", err, exists)
	}

	contact, err := wm.GetContact(ctx, owner.ID)
	if err != nil {
		t.Fatalf("GetContact: %v", err)
	}
	if contact.Name != owner.Name {
		t.Errorf("expected contact name %q, got %q", owner.Name, contact.Name)
	}
	if contact.Kind != "person" {
		t.Errorf("expected contact kind 'person', got %q", contact.Kind)
	}
}

func TestHandleOnboardingStatusIncludesRuntimeSaveHints(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatalf("create tables: %v", err)
	}

	srv := &Server{
		cfg: Config{
			DB:      db,
			DataDir: t.TempDir(),
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/onboarding/status", nil)
	w := httptest.NewRecorder()

	srv.handleOnboardingStatus(w, req.WithContext(ctx))

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", res.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	mode, _ := body["runtime_mode"].(string)
	if mode != "daemon" && mode != "container" {
		t.Fatalf("expected normalized runtime_mode, got %q", mode)
	}
	if _, ok := body["in_container"].(bool); !ok {
		t.Fatalf("expected in_container boolean, got %#v", body["in_container"])
	}
	if path, _ := body["recovery_default_save_path"].(string); !strings.Contains(path, "navi-owner-passport.txt") {
		t.Fatalf("expected recovery_default_save_path to name the passport, got %q", path)
	}
}

func TestHandleOnboardingConnectionRejectsUnsupportedConnectorTypes(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatalf("create tables: %v", err)
	}

	dataDir := t.TempDir()
	srv := &Server{
		cfg: Config{
			DB:      db,
			DataDir: dataDir,
			SetupSchema: []connectors.SetupDescriptor{
				{Type: "telegram", DisplayName: "Telegram"},
				{Type: "slack", DisplayName: "Slack"},
			},
		},
	}

	recoveryBody, _ := json.Marshal(map[string]any{
		"owner_name": "Test Owner",
		"save_path":  filepath.Join(dataDir, "passport.txt"),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/onboarding/recovery", bytes.NewReader(recoveryBody))
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	srv.handleOnboardingRecovery(w, req.WithContext(ctx))
	if w.Result().StatusCode != http.StatusCreated {
		t.Fatalf("recovery setup failed: %d", w.Result().StatusCode)
	}

	body, _ := json.Marshal(map[string]any{
		"type": "discord",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/onboarding/connection", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:1234"
	w = httptest.NewRecorder()
	srv.handleOnboardingConnection(w, req.WithContext(ctx))

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400 Bad Request, got %d", w.Result().StatusCode)
	}
}
