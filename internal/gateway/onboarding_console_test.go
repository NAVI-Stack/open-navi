package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ceoai/navi/internal/onboarding"
	"github.com/ceoai/navi/internal/store"
)

// TestRootRedirectsToOnboardingWhenFirstRunIncomplete verifies that GET /
// returns a 302 redirect to /onboarding when the first-run state has not
// reached "complete".
func TestRootRedirectsToOnboardingWhenFirstRunIncomplete(t *testing.T) {
	db := testDB(t)
	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<html>TEST_CONSOLE_CONTENT</html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	srv := NewServer(Config{
		DB:        db,
		DataDir:   t.TempDir(),
		StaticDir: staticDir,
		Addr:      ":0",
	})

	res := doReq(t, srv, http.MethodGet, "/", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusFound {
		t.Fatalf("fresh root: expected 302, got %d", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	if loc != "/onboarding" {
		t.Fatalf("fresh root redirect: expected /onboarding, got %q", loc)
	}
}

// TestRootServesStaticContentWhenFirstRunComplete verifies that GET /
// serves the static index.html when first-run is complete.
func TestRootServesStaticContentWhenFirstRunComplete(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	staticDir := t.TempDir()
	content := "<html><body>TEST_CONSOLE_CONTENT</body></html>"
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte(content), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	// DeriveFirstRunStatus requires an owner to exist for the "complete" state.
	if err := store.CreateOwner(ctx, db, store.Owner{Name: "Test", Handle: "test"}); err != nil {
		t.Fatalf("create owner: %v", err)
	}
	if err := store.SetSetting(ctx, db, onboarding.SettingKeySetupComplete, "true"); err != nil {
		t.Fatalf("set setup_complete: %v", err)
	}
	if err := store.SetSetting(ctx, db, onboarding.SettingKeyFirstRunState, string(onboarding.FirstRunComplete)); err != nil {
		t.Fatalf("set first_run_state: %v", err)
	}
	if _, err := onboarding.SkipCeremonyJourney(ctx, db); err != nil {
		t.Fatalf("skip ceremony: %v", err)
	}

	srv := NewServer(Config{
		DB:        db,
		DataDir:   t.TempDir(),
		StaticDir: staticDir,
		Addr:      ":0",
	})

	res := doReq(t, srv, http.MethodGet, "/", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("completed root: expected 200, got %d", res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "TEST_CONSOLE_CONTENT") {
		t.Fatalf("completed root should serve index.html, got: %s", string(body))
	}
}

func TestDottedPluginPathServesConsoleWhenFirstRunComplete(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	staticDir := t.TempDir()
	content := "<html><body>TEST_CONSOLE_CONTENT</body></html>"
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte(content), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := store.CreateOwner(ctx, db, store.Owner{Name: "Test", Handle: "test"}); err != nil {
		t.Fatalf("create owner: %v", err)
	}
	if err := store.SetSetting(ctx, db, onboarding.SettingKeySetupComplete, "true"); err != nil {
		t.Fatalf("set setup_complete: %v", err)
	}
	if err := store.SetSetting(ctx, db, onboarding.SettingKeyFirstRunState, string(onboarding.FirstRunComplete)); err != nil {
		t.Fatalf("set first_run_state: %v", err)
	}
	if _, err := onboarding.SkipCeremonyJourney(ctx, db); err != nil {
		t.Fatalf("skip ceremony: %v", err)
	}

	srv := NewServer(Config{
		DB:        db,
		DataDir:   t.TempDir(),
		StaticDir: staticDir,
		Addr:      ":0",
	})

	res := doReq(t, srv, http.MethodGet, "/plugins/navi.connector.telegram", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("dotted plugin route: expected 200, got %d", res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "TEST_CONSOLE_CONTENT") {
		t.Fatalf("dotted plugin route should serve index.html, got: %s", string(body))
	}
}

// TestOnboardingPageIsLocalOnly verifies that GET /onboarding is blocked from
// non-loopback remote addresses.
func TestOnboardingPageIsLocalOnly(t *testing.T) {
	db := testDB(t)
	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "onboarding.html"), []byte("<html>TEST_ONBOARDING_CONTENT</html>"), 0o644); err != nil {
		t.Fatalf("write onboarding: %v", err)
	}
	srv := NewServer(Config{
		DB:        db,
		DataDir:   t.TempDir(),
		StaticDir: staticDir,
		Addr:      ":0",
	})

	// Local should succeed.
	res := doReq(t, srv, http.MethodGet, "/onboarding", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("local onboarding: expected 200, got %d", res.StatusCode)
	}

	// Remote should be rejected.
	res = doReq(t, srv, http.MethodGet, "/onboarding", "", "", "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("remote onboarding: expected 403, got %d", res.StatusCode)
	}
}

// TestOnboardingAPIIsLocalOnly verifies that onboarding API routes
// reject non-loopback requests with 403.
func TestOnboardingAPIIsLocalOnly(t *testing.T) {
	db := testDB(t)
	srv := NewServer(Config{
		DB:      db,
		DataDir: t.TempDir(),
		Addr:    ":0",
	})

	// Remote GET /api/onboarding/status should be 403.
	res := doReq(t, srv, http.MethodGet, "/api/onboarding/status", "", "", "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("remote onboarding status: expected 403, got %d", res.StatusCode)
	}

	// Local GET /api/onboarding/status should be 200.
	res = doReq(t, srv, http.MethodGet, "/api/onboarding/status", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("local onboarding status: expected 200, got %d", res.StatusCode)
	}
}

// TestAPIRoutesNotCapturedByStaticServing verifies that /api/* paths are
// handled by API handlers, not the static file server, even after first-run
// is complete.
func TestAPIRoutesNotCapturedByStaticServing(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<html>TEST_CONSOLE_CONTENT</html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	// DeriveFirstRunStatus requires an owner to exist for the "complete" state.
	if err := store.CreateOwner(ctx, db, store.Owner{Name: "Test", Handle: "test"}); err != nil {
		t.Fatalf("create owner: %v", err)
	}
	if err := store.SetSetting(ctx, db, onboarding.SettingKeySetupComplete, "true"); err != nil {
		t.Fatalf("set setup_complete: %v", err)
	}
	if err := store.SetSetting(ctx, db, onboarding.SettingKeyFirstRunState, string(onboarding.FirstRunComplete)); err != nil {
		t.Fatalf("set first_run_state: %v", err)
	}
	if _, err := onboarding.SkipCeremonyJourney(ctx, db); err != nil {
		t.Fatalf("skip ceremony: %v", err)
	}

	srv := NewServer(Config{
		DB:        db,
		DataDir:   t.TempDir(),
		StaticDir: staticDir,
		Addr:      ":0",
	})

	// /api/status should return JSON, not HTML.
	res := doReq(t, srv, http.MethodGet, "/api/status", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/status: expected 200, got %d", res.StatusCode)
	}
	ct := res.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("GET /api/status should return JSON, got Content-Type: %s", ct)
	}

	// /api/health should return JSON.
	res = doReq(t, srv, http.MethodGet, "/api/health", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/health: expected 200, got %d", res.StatusCode)
	}
	ct = res.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("GET /api/health should return JSON, got Content-Type: %s", ct)
	}

	// /api/version should return JSON.
	res = doReq(t, srv, http.MethodGet, "/api/version", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/version: expected 200, got %d", res.StatusCode)
	}
	ct = res.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("GET /api/version should return JSON, got Content-Type: %s", ct)
	}

	// An unknown /api/* path should NOT serve HTML from static dir.
	res = doReq(t, srv, http.MethodGet, "/api/nonexistent-endpoint", "", "", "127.0.0.1:1234", nil)
	body, _ := io.ReadAll(res.Body)
	if strings.Contains(string(body), "TEST_CONSOLE_CONTENT") {
		t.Fatalf("/api/nonexistent-endpoint should not serve static HTML")
	}
}

// TestStructuredErrorRendersHumanMessage verifies that gateway API handlers
// return JSON error responses that are parseable by the frontend's error
// normalization logic (handling both { "error": "string" } and
// { "error": { "code": "...", "message": "..." } } shapes).
func TestStructuredErrorRendersHumanMessage(t *testing.T) {
	srv, _, _ := testServer(t)
	srv.cfg.DataDir = t.TempDir()

	// Create recovery first so we can test the provider step error shape.
	doReq(t, srv, http.MethodPost, "/api/onboarding/recovery", "", "", "127.0.0.1:1234", map[string]any{
		"owner_name": "Error Test Owner",
	})

	// Shape 1: simple string error — try to complete without provider.
	res := doReq(t, srv, http.MethodPost, "/api/onboarding/complete", "", "", "127.0.0.1:1234", map[string]any{})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	var simpleErr struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &simpleErr); err != nil {
		t.Fatalf("failed to parse simple error JSON: %v, body: %s", err, body)
	}
	if simpleErr.Error == "" {
		t.Fatalf("expected non-empty error message, got empty, body: %s", body)
	}
}

// TestOnboardingCompleteFlowRedirectsToCeremony verifies the full onboarding
// cycle hands off to the post-required-onboarding Ceremony.
func TestOnboardingCompleteFlowRedirectsToRoot(t *testing.T) {
	db := testDB(t)
	dataDir := t.TempDir()
	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<html><body>TEST_CONSOLE_CONTENT_READY</body></html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "onboarding.html"), []byte("<html>TEST_ONBOARDING_CONTENT</html>"), 0o644); err != nil {
		t.Fatalf("write onboarding: %v", err)
	}
	fakeLLM := &onboardingFakeLLM{}
	srv := NewServer(Config{
		DB:        db,
		DataDir:   dataDir,
		StaticDir: staticDir,
		Addr:      ":0",
		LLM:       fakeLLM,
	})

	// Step 1: recovery.
	res := doReq(t, srv, http.MethodPost, "/api/onboarding/recovery", "", "", "127.0.0.1:1234", map[string]any{
		"owner_name": "Flow Test Owner",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST recovery: expected 201, got %d", res.StatusCode)
	}

	// Step 2: provider.
	res = doReq(t, srv, http.MethodPost, "/api/onboarding/provider", "", "", "127.0.0.1:1234", map[string]any{})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST provider: expected 200, got %d", res.StatusCode)
	}

	// Step 3: skip connection.
	res = doReq(t, srv, http.MethodPost, "/api/onboarding/connection", "", "", "127.0.0.1:1234", map[string]any{"skip": true})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST connection skip: expected 200, got %d", res.StatusCode)
	}

	// Step 4: complete.
	res = doReq(t, srv, http.MethodPost, "/api/onboarding/complete", "", "", "127.0.0.1:1234", map[string]any{})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST complete: expected 200, got %d", res.StatusCode)
	}

	// Post-onboarding, the complete response returns "/" so the frontend
	// fetches /api/ceremony and calls /api/ceremony/init-chat to get the chat route.
	var completeResp map[string]any
	if err := json.NewDecoder(res.Body).Decode(&completeResp); err != nil {
		t.Fatalf("decode complete: %v", err)
	}
	if completeResp["redirect"] != "/" {
		t.Fatalf("expected redirect to /, got %v", completeResp["redirect"])
	}

	// The console SPA (including /chats/...) is served by the static handler.
	res = doReq(t, srv, http.MethodGet, "/chats/test", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /chats/test after complete: expected 200, got %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "TEST_CONSOLE_CONTENT_READY") {
		t.Fatalf("/chats/test should serve console content, got: %s", string(body))
	}
}
