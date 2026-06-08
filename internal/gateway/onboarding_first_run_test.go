package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/bus"
	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/navi"
	navistore "github.com/ceoai/navi/internal/navi/store"
	"github.com/ceoai/navi/internal/onboarding"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

type onboardingFakeLLM struct {
	setupProvider string
	setupModel    string
	failWithKey   bool
}

func (f *onboardingFakeLLM) Status() string {
	if f.setupProvider == "" {
		return "none"
	}
	return f.setupProvider + "/" + f.setupModel
}

func (f *onboardingFakeLLM) Catalog(context.Context) (llm.LLMCatalog, error) {
	return llm.LLMCatalog{Providers: []llm.LLMProviderInfo{
		{Key: "ollama", DisplayName: "Ollama", Models: []llm.LLMModelInfo{{Name: "local-model:latest"}}},
		{Key: "openai", DisplayName: "OpenAI", Models: []llm.LLMModelInfo{{Name: "gpt-4o-mini"}}},
	}}, nil
}

func (f *onboardingFakeLLM) Profiles(context.Context) ([]llm.ModelProfile, error) {
	return nil, nil
}

func (f *onboardingFakeLLM) GetActive(context.Context) (llm.Active, error) {
	return llm.Active{Provider: f.setupProvider, Model: f.setupModel}, nil
}

func (f *onboardingFakeLLM) SetActive(context.Context, string, string) (llm.Active, error) {
	return llm.Active{}, nil
}

func (f *onboardingFakeLLM) GetPreferences(context.Context) (llm.ModelPreferences, error) {
	return llm.ModelPreferences{}, nil
}

func (f *onboardingFakeLLM) PatchPreferences(context.Context, map[string]any) (llm.ModelPreferences, error) {
	return llm.ModelPreferences{}, nil
}

func (f *onboardingFakeLLM) SetupProvider(_ context.Context, provider, apiKey, model string) error {
	if f.failWithKey {
		return fmt.Errorf("provider setup failed for api_key=%s", apiKey)
	}
	f.setupProvider = provider
	f.setupModel = model
	return nil
}

func (f *onboardingFakeLLM) ListProviders(context.Context) ([]llm.ProviderDescriptor, error) {
	healthy := true
	return []llm.ProviderDescriptor{{Key: "ollama", DisplayName: "Ollama", Kind: llm.ProviderKindLocal, Enabled: true, Healthy: &healthy, ModelCount: 1}}, nil
}

func (f *onboardingFakeLLM) GetProvider(context.Context, string) (llm.ProviderDescriptor, error) {
	return llm.ProviderDescriptor{}, nil
}

func (f *onboardingFakeLLM) Health(context.Context, string) (llm.ProviderHealth, error) {
	return llm.ProviderHealth{Provider: "ollama", Healthy: true, Message: "ok", ModelCount: 1}, nil
}

func (f *onboardingFakeLLM) ProviderModels(context.Context, string) ([]llm.ModelDescriptor, error) {
	return []llm.ModelDescriptor{{Name: "local-model:latest"}}, nil
}

func (f *onboardingFakeLLM) ProviderRunningModels(context.Context, string) ([]llm.RunningModel, error) {
	return nil, nil
}

func (f *onboardingFakeLLM) ExecuteProviderAction(context.Context, llm.ProviderActionRequest) (llm.ProviderOperation, error) {
	return llm.ProviderOperation{}, nil
}

func (f *onboardingFakeLLM) GetOperation(context.Context, string) (*llm.ProviderOperation, error) {
	return nil, nil
}

func (f *onboardingFakeLLM) ConfigureProvider(_ context.Context, _ llm.ProviderConfigRequest) error {
	return nil
}

func (f *onboardingFakeLLM) DisableProvider(_ context.Context, _ string) error {
	return nil
}

func TestOnboardingFirstRunRecoveryCreatesPassportOnceAndStatusRedacts(t *testing.T) {
	srv, _, db := testServer(t)
	dataDir := t.TempDir()
	srv.cfg.DataDir = dataDir

	res := doReq(t, srv, http.MethodPost, "/api/onboarding/recovery", "", "", "127.0.0.1:1234", map[string]any{
		"owner_name": "First Run Owner",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST recovery: expected 201, got %d", res.StatusCode)
	}
	var recovery map[string]any
	if err := json.NewDecoder(res.Body).Decode(&recovery); err != nil {
		t.Fatalf("decode recovery: %v", err)
	}
	apiKey, _ := recovery["primary_api_key"].(string)
	seed, _ := recovery["recovery_seed"].(string)
	adminSecret, _ := recovery["admin_secret"].(string)
	passportText, _ := recovery["passport_text"].(string)
	savedPath, _ := recovery["recovery_saved_path"].(string)
	if apiKey == "" || seed == "" || adminSecret == "" || !strings.Contains(passportText, apiKey) || savedPath == "" {
		t.Fatalf("recovery response missing one-time material: %#v", recovery)
	}
	if _, err := os.Stat(savedPath); err != nil {
		t.Fatalf("expected recovery passport to be saved: %v", err)
	}
	if complete, _, _ := store.GetSetting(context.Background(), db, onboarding.SettingKeySetupComplete); complete == "true" {
		t.Fatalf("recovery step must not mark setup complete")
	}

	res = doReq(t, srv, http.MethodGet, "/api/onboarding/status", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET status: expected 200, got %d", res.StatusCode)
	}
	var status map[string]any
	if err := json.NewDecoder(res.Body).Decode(&status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status["first_run_state"] != string(onboarding.FirstRunRecoveryCreated) {
		t.Fatalf("first_run_state = %v", status["first_run_state"])
	}
	for _, secretKey := range []string{"primary_api_key", "api_key", "admin_secret", "recovery_seed", "passport_text"} {
		if _, ok := status[secretKey]; ok {
			t.Fatalf("status leaked secret field %q: %#v", secretKey, status)
		}
	}

	diag := doReq(t, srv, http.MethodGet, "/api/status", "", "", "127.0.0.1:1234", nil)
	if diag.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/status: expected 200, got %d", diag.StatusCode)
	}
	var diagBody map[string]any
	if err := json.NewDecoder(diag.Body).Decode(&diagBody); err != nil {
		t.Fatalf("decode diagnostics status: %v", err)
	}
	diagJSON, _ := json.Marshal(diagBody)
	for _, leaked := range []string{apiKey, seed, adminSecret} {
		if leaked != "" && strings.Contains(string(diagJSON), leaked) {
			t.Fatalf("diagnostics status leaked secret %q: %s", leaked, string(diagJSON))
		}
	}

	res = doReq(t, srv, http.MethodPost, "/api/onboarding/recovery", "", "", "127.0.0.1:1234", map[string]any{})
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("second recovery: expected 409, got %d", res.StatusCode)
	}
	var conflict map[string]any
	if err := json.NewDecoder(res.Body).Decode(&conflict); err != nil {
		t.Fatalf("decode conflict: %v", err)
	}
	for _, secretKey := range []string{"primary_api_key", "api_key", "admin_secret", "recovery_seed", "passport_text"} {
		if _, ok := conflict[secretKey]; ok {
			t.Fatalf("conflict response leaked secret field %q: %#v", secretKey, conflict)
		}
	}
}

func TestOnboardingProviderConnectionCompleteAndRootRedirect(t *testing.T) {
	db := testDB(t)
	dataDir := t.TempDir()
	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("NAVI home"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	fakeLLM := &onboardingFakeLLM{}
	srv := NewServer(Config{
		DB:        db,
		DataDir:   dataDir,
		StaticDir: staticDir,
		Addr:      ":0",
		LLM:       fakeLLM,
	})

	root := doReq(t, srv, http.MethodGet, "/", "", "", "127.0.0.1:1234", nil)
	if root.StatusCode != http.StatusFound || root.Header.Get("Location") != "/onboarding" {
		t.Fatalf("fresh root should redirect to /onboarding, got %d %q", root.StatusCode, root.Header.Get("Location"))
	}

	res := doReq(t, srv, http.MethodPost, "/api/onboarding/recovery", "", "", "127.0.0.1:1234", map[string]any{
		"owner_name": "Provider Owner",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST recovery: expected 201, got %d", res.StatusCode)
	}
	res = doReq(t, srv, http.MethodPost, "/api/onboarding/provider", "", "", "127.0.0.1:1234", map[string]any{})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST provider: expected 200, got %d", res.StatusCode)
	}
	if fakeLLM.setupProvider != "ollama" || fakeLLM.setupModel != "local-model:latest" {
		t.Fatalf("unexpected provider default %s/%s", fakeLLM.setupProvider, fakeLLM.setupModel)
	}
	res = doReq(t, srv, http.MethodPost, "/api/onboarding/connection", "", "", "127.0.0.1:1234", map[string]any{"skip": true})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST connection skip: expected 200, got %d", res.StatusCode)
	}
	res = doReq(t, srv, http.MethodPost, "/api/onboarding/complete", "", "", "127.0.0.1:1234", map[string]any{})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST complete: expected 200, got %d", res.StatusCode)
	}

	res = doReq(t, srv, http.MethodGet, "/api/onboarding/status", "", "", "127.0.0.1:1234", nil)
	var status map[string]any
	if err := json.NewDecoder(res.Body).Decode(&status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status["first_run_state"] != string(onboarding.FirstRunComplete) || status["complete"] != true {
		t.Fatalf("expected complete status, got %#v", status)
	}

	// After onboarding, GET / no longer redirects to /ceremony server-side.
	// The frontend fetches /api/ceremony and calls /api/ceremony/init-chat to
	// route into the Meet NAVI chat thread.
	root = doReq(t, srv, http.MethodGet, "/", "", "", "127.0.0.1:1234", nil)
	if root.StatusCode != http.StatusOK {
		t.Fatalf("completed root should serve the SPA (200), got %d", root.StatusCode)
	}
}

func TestOnboardingCompleteSchedulesProactiveGreeting(t *testing.T) {
	db := testDB(t)
	if err := navistore.MigrateSchema(context.Background(), db); err != nil {
		t.Fatalf("migrate navi schema: %v", err)
	}
	sessionStore := navistore.NewSQLiteStore(db)
	naviAgent, err := navi.New(navi.Config{
		DB:              db,
		Bus:             bus.NewMemBus(db),
		Chats:           sessionStore,
		RuntimeSessions: sessionStore,
	})
	if err != nil {
		t.Fatalf("new navi: %v", err)
	}

	dataDir := t.TempDir()
	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("NAVI home"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	fakeLLM := &onboardingFakeLLM{}
	srv := NewServer(Config{
		DB:        db,
		DataDir:   dataDir,
		StaticDir: staticDir,
		Addr:      ":0",
		LLM:       fakeLLM,
		Navi:      naviAgent,
	})

	oldDelay := postOnboardingGreetingDelay
	oldSchedule := schedulePostOnboardingGreeting
	scheduled := make(chan time.Duration, 1)
	delivered := make(chan struct{}, 1)
	postOnboardingGreetingDelay = func() time.Duration {
		return 37 * time.Second
	}
	schedulePostOnboardingGreeting = func(delay time.Duration, fn func()) *time.Timer {
		scheduled <- delay
		fn()
		delivered <- struct{}{}
		return nil
	}
	t.Cleanup(func() {
		postOnboardingGreetingDelay = oldDelay
		schedulePostOnboardingGreeting = oldSchedule
	})

	res := doReq(t, srv, http.MethodPost, "/api/onboarding/recovery", "", "", "127.0.0.1:1234", map[string]any{
		"owner_name": "Greeting Owner",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST recovery: expected 201, got %d", res.StatusCode)
	}
	res = doReq(t, srv, http.MethodPost, "/api/onboarding/provider", "", "", "127.0.0.1:1234", map[string]any{})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST provider: expected 200, got %d", res.StatusCode)
	}
	res = doReq(t, srv, http.MethodPost, "/api/onboarding/connection", "", "", "127.0.0.1:1234", map[string]any{"skip": true})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST connection skip: expected 200, got %d", res.StatusCode)
	}
	res = doReq(t, srv, http.MethodPost, "/api/onboarding/complete", "", "", "127.0.0.1:1234", map[string]any{})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST complete: expected 200, got %d", res.StatusCode)
	}

	select {
	case got := <-scheduled:
		if got != 37*time.Second {
			t.Fatalf("scheduled delay = %s, want 37s", got)
		}
	case <-time.After(time.Second):
		t.Fatal("expected onboarding completion to schedule proactive greeting")
	}
	select {
	case <-delivered:
	case <-time.After(time.Second):
		t.Fatal("expected scheduled proactive greeting to run")
	}

	chats, err := sessionStore.ListChats(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(chats) != 1 {
		t.Fatalf("expected one post-onboarding chat, got %d", len(chats))
	}
	thread, err := sessionStore.GetChatWithMessages(context.Background(), string(chats[0].ID))
	if err != nil {
		t.Fatalf("GetChatWithMessages: %v", err)
	}
	if len(thread.Messages) != 1 {
		t.Fatalf("expected one proactive greeting message, got %d", len(thread.Messages))
	}
	msg := thread.Messages[0]
	if msg.Role != "assistant" || msg.MessageKind != string(schema.AssistantMessageKindProactive) {
		t.Fatalf("expected proactive assistant message, got %#v", msg)
	}
	if !strings.Contains(msg.Content, "NAVI") || !strings.Contains(strings.ToLower(msg.Content), "ready") {
		t.Fatalf("unexpected greeting content %q", msg.Content)
	}
}

func TestOnboardingProviderErrorRedactsSecrets(t *testing.T) {
	srv, _, _ := testServer(t)
	srv.cfg.DataDir = t.TempDir()
	srv.cfg.LLM = &onboardingFakeLLM{failWithKey: true}

	res := doReq(t, srv, http.MethodPost, "/api/onboarding/recovery", "", "", "127.0.0.1:1234", map[string]any{
		"owner_name": "Redaction Owner",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST recovery: expected 201, got %d", res.StatusCode)
	}

	leakyKey := "sk-leaksecret12345"
	res = doReq(t, srv, http.MethodPost, "/api/onboarding/provider", "", "", "127.0.0.1:1234", map[string]any{
		"provider": "openai",
		"model":    "gpt-4o-mini",
		"api_key":  leakyKey,
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST provider failure: expected 400, got %d", res.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if strings.Contains(body["error"], leakyKey) {
		t.Fatalf("provider error leaked API key: %q", body["error"])
	}
}

func TestOnboardingReset(t *testing.T) {
	srv, _, _ := testServer(t)
	dataDir := t.TempDir()
	srv.cfg.DataDir = dataDir
	srv.cfg.LLM = &onboardingFakeLLM{}

	// 1. Initially status is uninitialized
	res := doReq(t, srv, http.MethodGet, "/api/onboarding/status", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET status: expected 200, got %d", res.StatusCode)
	}
	var status map[string]any
	if err := json.NewDecoder(res.Body).Decode(&status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status["first_run_state"] != string(onboarding.FirstRunUninitialized) {
		t.Fatalf("expected uninitialized state, got %v", status["first_run_state"])
	}

	// 2. Try to reset when uninitialized (incomplete) -> should work (noop)
	res = doReq(t, srv, http.MethodPost, "/api/onboarding/reset", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST reset: expected 200, got %d", res.StatusCode)
	}

	// 3. Complete Step 1 (Recovery)
	res = doReq(t, srv, http.MethodPost, "/api/onboarding/recovery", "", "", "127.0.0.1:1234", map[string]any{
		"owner_name": "Reset Test Owner",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST recovery: expected 201, got %d", res.StatusCode)
	}

	// Verify setting state is recovery_created
	res = doReq(t, srv, http.MethodGet, "/api/onboarding/status", "", "", "127.0.0.1:1234", nil)
	_ = json.NewDecoder(res.Body).Decode(&status)
	if status["first_run_state"] != string(onboarding.FirstRunRecoveryCreated) {
		t.Fatalf("expected recovery_created state, got %v", status["first_run_state"])
	}

	// Create fake config.json to check if reset deletes it
	configPath := filepath.Join(dataDir, "config.json")
	if err := os.WriteFile(configPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("failed to write fake config.json: %v", err)
	}

	// 4. Perform onboarding reset
	res = doReq(t, srv, http.MethodPost, "/api/onboarding/reset", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST reset: expected 200, got %d", res.StatusCode)
	}

	// 5. Verify it returned status to uninitialized, wiped owner, and deleted config.json
	res = doReq(t, srv, http.MethodGet, "/api/onboarding/status", "", "", "127.0.0.1:1234", nil)
	_ = json.NewDecoder(res.Body).Decode(&status)
	if status["first_run_state"] != string(onboarding.FirstRunUninitialized) {
		t.Fatalf("expected uninitialized state after reset, got %v", status["first_run_state"])
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("expected config.json to be deleted, got: %v", err)
	}

	// 6. Complete all steps and mark onboarding complete
	res = doReq(t, srv, http.MethodPost, "/api/onboarding/recovery", "", "", "127.0.0.1:1234", map[string]any{
		"owner_name": "Reset Test Owner 2",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST recovery: expected 201, got %d", res.StatusCode)
	}
	res = doReq(t, srv, http.MethodPost, "/api/onboarding/provider", "", "", "127.0.0.1:1234", map[string]any{})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST provider: expected 200, got %d", res.StatusCode)
	}
	res = doReq(t, srv, http.MethodPost, "/api/onboarding/connection", "", "", "127.0.0.1:1234", map[string]any{"skip": true})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST connection skip: expected 200, got %d", res.StatusCode)
	}
	res = doReq(t, srv, http.MethodPost, "/api/onboarding/complete", "", "", "127.0.0.1:1234", map[string]any{})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST complete: expected 200, got %d", res.StatusCode)
	}

	// Verify status is complete
	res = doReq(t, srv, http.MethodGet, "/api/onboarding/status", "", "", "127.0.0.1:1234", nil)
	_ = json.NewDecoder(res.Body).Decode(&status)
	if status["complete"] != true {
		t.Fatalf("expected complete to be true, got %v", status["complete"])
	}

	// 7. Attempting reset after complete should return Bad Request (400)
	res = doReq(t, srv, http.MethodPost, "/api/onboarding/reset", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST reset after completion: expected 400, got %d", res.StatusCode)
	}
}
