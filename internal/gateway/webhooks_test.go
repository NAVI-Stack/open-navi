package gateway

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ceoai/navi/internal/bus"
	"github.com/ceoai/navi/internal/cognitive"
	"github.com/ceoai/navi/internal/governor"
	"github.com/ceoai/navi/internal/navi"
	navistore "github.com/ceoai/navi/internal/navi/store"
)

func TestWebhookRegistrationLifecycleAndIngress(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	if err := navistore.MigrateSchema(ctx, db); err != nil {
		t.Fatalf("migrate navi schema: %v", err)
	}
	b := bus.NewMemBus(db)
	sessionStore := navistore.NewSQLiteStore(db)
	naviAgent, err := navi.New(navi.Config{
		DB:    db,
		Bus:   b,
		Chats: sessionStore, RuntimeSessions: sessionStore,
	})
	if err != nil {
		t.Fatalf("new navi: %v", err)
	}
	chatID, err := naviAgent.CreateChat(ctx, navi.ExperienceModeStandard, "")
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	srv := NewServer(Config{
		DB:              db,
		DirectiveWriter: cognitive.StoreDirectiveWriter(db),
		Bus:             b,
		Governor:        governor.NewGovernor(governor.GovernorConfig{}, "."),
		Registry:        NewConnectorRegistry(),
		Navi:            naviAgent,
		Addr:            ":0",
	})

	res := doReq(t, srv, http.MethodPut, "/api/webhooks/github", "", "", "127.0.0.1:1234", map[string]any{
		"chat_id": chatID,
		"enabled": true,
		"signature": map[string]any{
			"type":   "hmac-sha256",
			"secret": "topsecret",
		},
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/webhooks/github: expected 200, got %d", res.StatusCode)
	}

	res = doReq(t, srv, http.MethodGet, "/api/webhooks", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/webhooks: expected 200, got %d", res.StatusCode)
	}
	var listed map[string]any
	if err := json.NewDecoder(res.Body).Decode(&listed); err != nil {
		t.Fatalf("decode registrations: %v", err)
	}
	items, ok := listed["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one registration, got %v", listed["items"])
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("expected map registration, got %T", items[0])
	}
	signature, ok := item["signature"].(map[string]any)
	if !ok {
		t.Fatalf("expected signature map, got %T", item["signature"])
	}
	if signature["secret"] != nil {
		t.Fatalf("registration list should not expose raw secret, got %v", signature["secret"])
	}
	if signature["secret_configured"] != true {
		t.Fatalf("expected secret_configured=true, got %v", signature["secret_configured"])
	}

	payload := `{"ref":"refs/heads/main","repository":{"full_name":"ceoai/navi"}}`
	mac := hmac.New(sha256.New, []byte("topsecret"))
	mac.Write([]byte(payload))
	req, _ := http.NewRequest(http.MethodPost, "/api/webhooks/github", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-GitHub-Delivery", "delivery-1")
	req.RemoteAddr = "203.0.113.1:1234"
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	res = rec.Result()
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("POST /api/webhooks/github: expected 202, got %d", res.StatusCode)
	}

	pending, err := sessionStore.ListPendingItems(ctx, chatID, 10)
	if err != nil {
		t.Fatalf("ListPendingItems: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected one pending inbox item, got %d", len(pending))
	}
	if pending[0].ActorType != "webhook" {
		t.Fatalf("expected actor_type webhook, got %q", pending[0].ActorType)
	}
	if pending[0].PayloadType != "json" {
		t.Fatalf("expected payload_type json, got %q", pending[0].PayloadType)
	}
	if pending[0].SourceChannel != "webhook" {
		t.Fatalf("expected source_channel webhook, got %q", pending[0].SourceChannel)
	}
	if pending[0].IdempotencyKey != "delivery-1" {
		t.Fatalf("expected idempotency key delivery-1, got %q", pending[0].IdempotencyKey)
	}
	if string(pending[0].Structured) != payload {
		t.Fatalf("expected structured payload to round-trip, got %s", string(pending[0].Structured))
	}
	if !strings.Contains(pending[0].Content, "Webhook received from github.") {
		t.Fatalf("expected rendered content to mention github, got %q", pending[0].Content)
	}
	if !strings.Contains(pending[0].Content, "Event: push") {
		t.Fatalf("expected rendered content to mention event, got %q", pending[0].Content)
	}
}

func TestWebhookIngressRejectsInvalidSignature(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	if err := navistore.MigrateSchema(ctx, db); err != nil {
		t.Fatalf("migrate navi schema: %v", err)
	}
	b := bus.NewMemBus(db)
	sessionStore := navistore.NewSQLiteStore(db)
	naviAgent, err := navi.New(navi.Config{
		DB:    db,
		Bus:   b,
		Chats: sessionStore, RuntimeSessions: sessionStore,
	})
	if err != nil {
		t.Fatalf("new navi: %v", err)
	}
	chatID, err := naviAgent.CreateChat(ctx, navi.ExperienceModeStandard, "")
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	srv := NewServer(Config{
		DB:              db,
		DirectiveWriter: cognitive.StoreDirectiveWriter(db),
		Bus:             b,
		Governor:        governor.NewGovernor(governor.GovernorConfig{}, "."),
		Registry:        NewConnectorRegistry(),
		Navi:            naviAgent,
		Addr:            ":0",
	})

	if res := doReq(t, srv, http.MethodPut, "/api/webhooks/github", "", "", "127.0.0.1:1234", map[string]any{
		"chat_id": chatID,
		"enabled": true,
		"signature": map[string]any{
			"type":   "hmac-sha256",
			"secret": "topsecret",
		},
	}); res.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/webhooks/github: expected 200, got %d", res.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodPost, "/api/webhooks/github", strings.NewReader(`{"ok":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", "sha256=bad")
	req.RemoteAddr = "203.0.113.1:1234"
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	res := rec.Result()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("POST /api/webhooks/github invalid signature: expected 401, got %d", res.StatusCode)
	}

	pending, err := sessionStore.ListPendingItems(ctx, chatID, 10)
	if err != nil {
		t.Fatalf("ListPendingItems: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending items after invalid signature, got %d", len(pending))
	}
}
