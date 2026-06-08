package gateway

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pkgconn "github.com/open-navi/navi/connectors"
	artifactsvc "github.com/open-navi/navi/internal/artifact"
	"github.com/open-navi/navi/internal/blob"
	"github.com/open-navi/navi/internal/bus"
	"github.com/open-navi/navi/internal/cognitive"
	"github.com/open-navi/navi/internal/config"
	intconnectors "github.com/open-navi/navi/internal/connectors"
	"github.com/open-navi/navi/internal/governor"
	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/navi"
	"github.com/open-navi/navi/internal/navi/experience"
	"github.com/open-navi/navi/internal/navi/skill"
	navistore "github.com/open-navi/navi/internal/navi/store"
	"github.com/open-navi/navi/internal/presence"
	"github.com/open-navi/navi/internal/prompts"
	naviruntime "github.com/open-navi/navi/internal/runtime"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
	navitool "github.com/open-navi/navi/internal/tool"
	"github.com/open-navi/navi/internal/worldmodel"
	"nhooyr.io/websocket"
)

type typingTestConnector struct {
	name        string
	managed     bool
	typingCalls atomic.Int32
	running     atomic.Bool
}

func newTypingTestConnector(name string) *typingTestConnector {
	c := &typingTestConnector{name: name}
	c.running.Store(true)
	return c
}

func (c *typingTestConnector) Name() string                { return c.name }
func (c *typingTestConnector) Start(context.Context) error { return nil }
func (c *typingTestConnector) Stop(context.Context) error {
	c.running.Store(false)
	return nil
}
func (c *typingTestConnector) IsRunning() bool                                     { return c.running.Load() }
func (c *typingTestConnector) Send(context.Context, pkgconn.OutboundMessage) error { return nil }
func (c *typingTestConnector) StartTyping(context.Context, string) (func(), error) {
	c.typingCalls.Add(1)
	return func() {}, nil
}
func (c *typingTestConnector) ManagesInboundOrchestration() bool { return c.managed }

func testDB(t *testing.T) *sql.DB {
	db, err := store.Open(filepath.Join(t.TempDir(), "gateway-test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.CreateTables(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return db
}

func testServer(t *testing.T) (*Server, bus.Bus, *sql.DB) {
	db := testDB(t)
	b := bus.NewMemBus(db)
	blobStore, err := blob.NewFilesystemStore(filepath.Join(t.TempDir(), "artifact-blobs"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		DB:              db,
		DirectiveWriter: cognitive.StoreDirectiveWriter(db),
		Bus:             b,
		Governor:        governor.NewGovernor(governor.GovernorConfig{}, "."),
		Registry:        NewConnectorRegistry(),
		ArtifactService: artifactsvc.NewService(db, blobStore),
		WorldModel:      worldmodel.New(db),
		Addr:            ":0",
	}
	srv := NewServer(cfg)
	seedGatewayArtifactWorkspace(t, db)
	return srv, b, db
}

func testServerWithNavi(t *testing.T) (*Server, *navi.NAVI, *sql.DB) {
	db := testDB(t)
	if err := navistore.MigrateSchema(context.Background(), db); err != nil {
		t.Fatalf("migrate navi schema: %v", err)
	}
	b := bus.NewMemBus(db)
	sessionStore := navistore.NewSQLiteStore(db)
	naviAgent, err := navi.New(navi.Config{
		DB:              db,
		Bus:             b,
		Chats:           sessionStore,
		RuntimeSessions: sessionStore,
	})
	if err != nil {
		t.Fatalf("new navi: %v", err)
	}
	srv := NewServer(Config{
		DB:              db,
		DirectiveWriter: cognitive.StoreDirectiveWriter(db),
		Bus:             b,
		Governor:        governor.NewGovernor(governor.GovernorConfig{}, "."),
		Registry:        NewConnectorRegistry(),
		Navi:            naviAgent,
		WorldModel:      worldmodel.New(db),
		Addr:            ":0",
		DataDir:         t.TempDir(),
	})
	seedGatewayArtifactWorkspace(t, db)
	return srv, naviAgent, db
}

func seedGatewayArtifactWorkspace(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	if err := store.SaveWorkspace(ctx, db, schema.Workspace{
		ID:             "ws-default",
		Name:           "Gateway Default Workspace",
		Kind:           schema.WorkspaceKindProject,
		Status:         schema.WorkspaceStatusActive,
		LocalRoots:     []string{"/workspace/gateway"},
		AllowedActions: schema.AllowedActions{Read: true, Write: true, Create: true, Modify: true, RenameMove: true, Delete: false, Execute: false},
		BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		AuditEnabled:   true,
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      "owner",
		Metadata:       "{}",
	}); err != nil {
		t.Fatalf("SaveWorkspace(ws-default): %v", err)
	}
}

func mustSaveGatewayProject(t *testing.T, db *sql.DB, projectID string) {
	t.Helper()
	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	if err := store.SaveProject(context.Background(), db, schema.Project{
		ID:          projectID,
		Title:       "Project " + projectID,
		Kind:        schema.ProjectKindGeneral,
		Status:      schema.ProjectStatusActive,
		Health:      schema.ProjectHealthUnknown,
		WorkspaceID: "ws-default",
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "owner",
	}); err != nil {
		t.Fatalf("SaveProject(%s): %v", projectID, err)
	}
}

func mustSaveGatewayWorkspace(t *testing.T, db *sql.DB, workspaceID string) {
	t.Helper()
	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	if err := store.SaveWorkspace(context.Background(), db, schema.Workspace{
		ID:             workspaceID,
		Name:           "Workspace " + workspaceID,
		Kind:           schema.WorkspaceKindProject,
		Status:         schema.WorkspaceStatusActive,
		LocalRoots:     []string{"/workspace/" + workspaceID},
		AllowedActions: schema.AllowedActions{Read: true, Write: true, Create: true, Modify: true, RenameMove: true, Delete: false, Execute: false},
		BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		AuditEnabled:   true,
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      "owner",
		Metadata:       "{}",
	}); err != nil {
		t.Fatalf("SaveWorkspace(%s): %v", workspaceID, err)
	}
}

// doReq sends a test HTTP request. apiKey is sent as X-API-Key when non-empty.
// ownerSecret is sent as X-Owner-Secret when non-empty.
func doReq(t *testing.T, srv *Server, method, path string, apiKey, ownerSecret, remoteAddr string, reqBody any) *http.Response {
	t.Helper()
	req, err := newTestRequest(method, path, reqBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	if ownerSecret != "" {
		req.Header.Set("X-Owner-Secret", ownerSecret)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	} else {
		req.RemoteAddr = "192.168.1.1:1234"
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec.Result()
}

func doReqWithContext(t *testing.T, ctx context.Context, srv *Server, method, path string, apiKey, ownerSecret, remoteAddr string, reqBody any) *http.Response {
	t.Helper()
	req, err := newTestRequest(method, path, reqBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req = req.WithContext(ctx)
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	if ownerSecret != "" {
		req.Header.Set("X-Owner-Secret", ownerSecret)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	} else {
		req.RemoteAddr = "192.168.1.1:1234"
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec.Result()
}

func newTestRequest(method, path string, reqBody any) (*http.Request, error) {
	var body []byte
	if reqBody != nil {
		body, _ = json.Marshal(reqBody)
	}
	return http.NewRequest(method, path, bytes.NewBuffer(body))
}

// doJSONReq exists for backward-compat in tests that pass a generic token.
// The token is sent as X-API-Key.
func doJSONReq(t *testing.T, srv *Server, method, path, apiKey, remoteAddr string, reqBody any) *http.Response {
	t.Helper()
	return doReq(t, srv, method, path, apiKey, "", remoteAddr, reqBody)
}

func readLiveResponseFrame(t *testing.T, c *websocket.Conn) []byte {
	t.Helper()
	readCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	for {
		_, payload, err := c.Read(readCtx)
		if err != nil {
			t.Fatalf("read live frame err: %v", err)
		}
		var meta struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(payload, &meta); err != nil {
			t.Fatalf("unmarshal live frame metadata: %v", err)
		}
		if meta.Type == "res" || meta.Type == "" {
			return payload
		}
	}
}

// claimTestInstance performs first-run recovery creation and returns the owner passport.
func claimTestInstance(t *testing.T, srv *Server, ownerName, ownerSecret string) map[string]any {
	t.Helper()
	res := doReq(t, srv, "POST", "/api/onboarding/recovery", "", "", "127.0.0.1:1234", map[string]any{
		"owner_name":   ownerName,
		"owner_secret": ownerSecret,
		"save_path":    filepath.Join(t.TempDir(), "passport.txt"),
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("recovery: expected 201, got %d", res.StatusCode)
	}
	var passport map[string]any
	if err := json.NewDecoder(res.Body).Decode(&passport); err != nil {
		t.Fatalf("recovery: decode: %v", err)
	}
	return passport
}

func TestGatewayAuth(t *testing.T) {
	srv, _, _ := testServer(t)

	// 1. Unauthenticated external request → 401.
	res := doJSONReq(t, srv, "GET", "/api/governor", "", "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", res.StatusCode)
	}

	// 2. Loopback request → 200 (admin bypass).
	res = doJSONReq(t, srv, "GET", "/api/governor", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for loopback, got %d", res.StatusCode)
	}

	// 3. Claim the instance to get an API key.
	passport := claimTestInstance(t, srv, "TestOwner", "s3cr3t!")
	apiKey, _ := passport["primary_api_key"].(string)
	if apiKey == "" {
		t.Fatalf("passport missing primary_api_key")
	}

	// 4. API key auth on external request → 200.
	res = doJSONReq(t, srv, "GET", "/api/governor", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with API key, got %d", res.StatusCode)
	}

	// 5. Connector Registry still works.
	srv.cfg.Registry.Register("test-bot")
	res = doJSONReq(t, srv, "GET", "/api/connectors", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	var cons []ConnectorInfo
	json.NewDecoder(res.Body).Decode(&cons)
	res.Body.Close()
	if len(cons) != 1 || cons[0].Name != "test-bot" {
		t.Fatalf("expected 1 connector named 'test-bot', got %v", cons)
	}
}

func TestGatewayAuthAllowsLocalhostBrowserViaDockerHostGateway(t *testing.T) {
	srv, _, _ := testServer(t)

	prev := dockerHostGatewayResolver
	dockerHostGatewayResolver = func() []net.IP { return []net.IP{net.ParseIP("172.22.0.1")} }
	t.Cleanup(func() { dockerHostGatewayResolver = prev })

	req, err := http.NewRequest(http.MethodGet, "/api/governor", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.RemoteAddr = "172.22.0.1:1234"
	req.Host = "localhost:6284"

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for localhost browser via docker host gateway, got %d", rec.Code)
	}
}

func TestAuthMiddlewareSharedSecretSkipsDBLookup(t *testing.T) {
	db := testDB(t)
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("db conn: %v", err)
	}
	defer conn.Close()

	protected := AuthMiddleware(db, "shared-secret", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/navi/chats/s1/message", bytes.NewBufferString(`{"content":"hello"}`))
	req.RemoteAddr = "192.168.1.10:4555"
	req.Header.Set("X-API-Key", "shared-secret")
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		protected.ServeHTTP(rec, req)
	}()

	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("shared-secret auth path blocked; expected DB to be skipped")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

func TestNaviMessageRouteTimeoutBoundsBlockedAuthDBLookup(t *testing.T) {
	srv, naviAgent, db := testServerWithNavi(t)
	chatID, err := naviAgent.CreateChat(context.Background(), navi.ExperienceModeStandard, "")
	if err != nil {
		t.Fatalf("create chat: %v", err)
	}

	db.SetMaxOpenConns(1)
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("db conn: %v", err)
	}
	defer conn.Close()

	prev := gatewaySessionMessageTimeout
	gatewaySessionMessageTimeout = 150 * time.Millisecond
	t.Cleanup(func() { gatewaySessionMessageTimeout = prev })

	start := time.Now()
	res := doReq(t, srv, http.MethodPost, "/api/navi/chats/"+chatID+"/message", "bogus-key", "", "192.168.1.50:1234", map[string]any{"content": "hi", "source_channel": "web"})
	elapsed := time.Since(start)

	if res.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d", res.StatusCode)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("expected timeout bound under 2s in test, got %s", elapsed)
	}
}

func TestNaviMessageRouteTimeoutWhenSendMessageInputBlocks(t *testing.T) {
	srv, _, db := testServerWithNavi(t)
	chatID := "owner-chat-timeout-test"
	if err := store.SetSetting(context.Background(), db, "telegram_owner_chat_id", chatID); err != nil {
		t.Fatalf("set owner chat id: %v", err)
	}
	prev := gatewaySessionMessageTimeout
	gatewaySessionMessageTimeout = 120 * time.Millisecond
	t.Cleanup(func() { gatewaySessionMessageTimeout = prev })

	srv.sendMessageInput = func(ctx context.Context, chatID string, input naviruntime.MessageInput) (*naviruntime.InboxItem, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	start := time.Now()
	res := doReq(t, srv, http.MethodPost, "/api/navi/chats/"+chatID+"/message", "", "", "127.0.0.1:1234", map[string]any{"content": "hello", "source_channel": "web"})
	elapsed := time.Since(start)
	if res.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d", res.StatusCode)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("expected request timeout before 60s client timeout, got %s", elapsed)
	}
	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode timeout payload: %v", err)
	}
	rootErr, _ := payload["error"].(map[string]any)
	if rootErr["code"] != "GATEWAY_TIMEOUT" {
		t.Fatalf("expected GATEWAY_TIMEOUT code, got %v", rootErr["code"])
	}
}

func TestRequestTracingLogsStartAndEndOnTimeout(t *testing.T) {
	var logs bytes.Buffer
	oldLogger := slog.Default()
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(oldLogger) })

	srv, _, db := testServerWithNavi(t)
	chatID := "owner-chat-trace-timeout-test"
	if err := store.SetSetting(context.Background(), db, "telegram_owner_chat_id", chatID); err != nil {
		t.Fatalf("set owner chat id: %v", err)
	}
	prev := gatewaySessionMessageTimeout
	gatewaySessionMessageTimeout = 120 * time.Millisecond
	t.Cleanup(func() { gatewaySessionMessageTimeout = prev })

	srv.sendMessageInput = func(ctx context.Context, chatID string, input naviruntime.MessageInput) (*naviruntime.InboxItem, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	res := doReq(t, srv, http.MethodPost, "/api/navi/chats/"+chatID+"/message", "", "", "127.0.0.1:1234", map[string]any{"content": "hello", "source_channel": "web"})
	if res.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d", res.StatusCode)
	}
	out := logs.String()
	if !strings.Contains(out, "gateway request start") {
		t.Fatalf("expected request start log, got %s", out)
	}
	if !strings.Contains(out, "gateway request end") {
		t.Fatalf("expected request end log, got %s", out)
	}
	if !strings.Contains(out, "status_code=504") {
		t.Fatalf("expected timeout status code in tracing log, got %s", out)
	}
}

func TestGatewayAuthRejectsDockerHostGatewayRequestsFromNonLocalOrigin(t *testing.T) {
	srv, _, _ := testServer(t)

	prev := dockerHostGatewayResolver
	dockerHostGatewayResolver = func() []net.IP { return []net.IP{net.ParseIP("172.22.0.1")} }
	t.Cleanup(func() { dockerHostGatewayResolver = prev })

	req, err := http.NewRequest(http.MethodGet, "/api/governor", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.RemoteAddr = "172.22.0.1:1234"
	req.Host = "localhost:6284"
	req.Header.Set("Origin", "https://evil.example")

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for docker host gateway request from non-local origin, got %d", rec.Code)
	}
}

func TestGatewayOnboardingRecovery(t *testing.T) {
	srv, _, _ := testServer(t)

	// Status before claim.
	res := doJSONReq(t, srv, "GET", "/api/onboarding/status", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	var status map[string]any
	json.NewDecoder(res.Body).Decode(&status)
	if status["claimed"] != false {
		t.Fatalf("expected claimed=false before setup")
	}

	// Claim.
	passport := claimTestInstance(t, srv, "Alice", "my-owner-secret")
	if passport["owner_name"] != "Alice" {
		t.Fatalf("expected owner_name=Alice, got %v", passport["owner_name"])
	}
	apiKey, _ := passport["primary_api_key"].(string)
	if !strings.HasPrefix(apiKey, "navi_") {
		t.Fatalf("expected navi_ prefix, got %q", apiKey)
	}

	// Second claim → 409.
	res = doReq(t, srv, "POST", "/api/onboarding/recovery", "", "", "127.0.0.1:1234", map[string]any{
		"owner_name": "Bob",
	})
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("second claim: expected 409, got %d", res.StatusCode)
	}

	// Status after claim.
	res = doJSONReq(t, srv, "GET", "/api/onboarding/status", "", "127.0.0.1:1234", nil)
	json.NewDecoder(res.Body).Decode(&status)
	if status["claimed"] != true {
		t.Fatalf("expected claimed=true after setup")
	}
}

func TestRemovedPersonaEndpointsReturnNotFound(t *testing.T) {
	srv, _, _ := testServerWithNavi(t)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/api/setup/persona"},
		{method: http.MethodPut, path: "/api/navi/persona"},
	} {
		res := doReq(t, srv, tc.method, tc.path, "", "", "127.0.0.1:1234", map[string]any{"persona": "unsupported-profile"})
		if res.StatusCode != http.StatusNotFound && res.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("%s %s: expected 404 or 405, got %d", tc.method, tc.path, res.StatusCode)
		}
	}
}

func TestLegacySessionEndpointsReturnNotFound(t *testing.T) {
	srv, _, _ := testServerWithNavi(t)

	for _, tc := range []struct {
		method string
		path   string
		body   any
	}{
		{method: http.MethodGet, path: "/api/navi/sessions"},
		{method: http.MethodPost, path: "/api/navi/sessions", body: map[string]any{}},
		{method: http.MethodGet, path: "/api/navi/sessions/legacy-session"},
		{method: http.MethodPost, path: "/api/navi/sessions/legacy-session/message", body: map[string]any{"content": "hello"}},
	} {
		res := doReq(t, srv, tc.method, tc.path, "", "", "127.0.0.1:1234", tc.body)
		if res.StatusCode != http.StatusNotFound && res.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("%s %s: expected 404 or 405, got %d", tc.method, tc.path, res.StatusCode)
		}
	}
}

func TestNaviCreateChatRejectsUnsupportedLegacyPersonaInput(t *testing.T) {
	srv, _, db := testServerWithNavi(t)
	passport := claimTestInstance(t, srv, "Persona Removal Test", "owner-secret")
	if apiKey, _ := passport["primary_api_key"].(string); apiKey == "" {
		t.Fatalf("claim did not return api key")
	}
	if err := store.SetSetting(context.Background(), db, "setup_complete", "true"); err != nil {
		t.Fatalf("set setup_complete: %v", err)
	}

	res := doReq(t, srv, http.MethodPost, "/api/navi/chats", "", "", "127.0.0.1:1234", map[string]any{
		"persona": "unsupported-profile",
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.StatusCode)
	}
}

func TestNaviCreateChatAcceptsEmptyBody(t *testing.T) {
	srv, _, db := testServerWithNavi(t)
	_ = claimTestInstance(t, srv, "Empty Body Test", "owner-secret")
	if err := store.SetSetting(context.Background(), db, "setup_complete", "true"); err != nil {
		t.Fatalf("set setup_complete: %v", err)
	}

	res := doReq(t, srv, http.MethodPost, "/api/navi/chats", "", "", "127.0.0.1:1234", map[string]any{})
	if res.StatusCode != http.StatusCreated {
		buf, _ := io.ReadAll(res.Body)
		t.Fatalf("expected 201 Created for empty body, got %d: %s", res.StatusCode, string(buf))
	}

	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	res.Body.Close()
	if body["chat_id"] == "" {
		t.Fatalf("chat_id missing from response")
	}
}

func TestNaviCreateChatWithInitialMessageAssignsGeneratedTitle(t *testing.T) {
	srv, _, db := testServerWithNavi(t)
	_ = claimTestInstance(t, srv, "Generated Title Test", "owner-secret")
	if err := store.SetSetting(context.Background(), db, "setup_complete", "true"); err != nil {
		t.Fatalf("set setup_complete: %v", err)
	}
	srv.cfg.LLM = &mockLLMService{chatProv: &mockProvider{
		chatFunc: func(ctx context.Context, model string, messages []llm.Message, tools []llm.ToolDefinition, opts llm.Options) (*llm.Response, error) {
			return &llm.Response{
				Content: `{"title":"Automatic Chat Titles","confidence":0.93,"reason":"The user wants new chats named automatically."}`,
			}, nil
		},
	}}

	res := doReq(t, srv, http.MethodPost, "/api/navi/chats", "", "", "127.0.0.1:1234", map[string]any{
		"initial_message": "new conversations should be assigned a conversation name/title like how all AI chat apps do",
	})
	if res.StatusCode != http.StatusCreated {
		buf, _ := io.ReadAll(res.Body)
		t.Fatalf("expected 201 Created, got %d: %s", res.StatusCode, string(buf))
	}

	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	res.Body.Close()
	if body["chat_id"] == "" {
		t.Fatalf("chat_id missing from response")
	}
	if body["title"] != "Automatic Chat Titles" {
		t.Fatalf("expected generated title in response, got %#v", body)
	}

	var storedTitle string
	if err := db.QueryRowContext(context.Background(), `SELECT title FROM navi_chats WHERE chat_id = ?`, body["chat_id"]).Scan(&storedTitle); err != nil {
		t.Fatalf("read chat title: %v", err)
	}
	if storedTitle != "Automatic Chat Titles" {
		t.Fatalf("expected stored generated title, got %q", storedTitle)
	}
}

func TestNaviCreateChatDoesNotPersistMessageOrStartTurn(t *testing.T) {
	srv, _, db := testServerWithNavi(t)
	_ = claimTestInstance(t, srv, "Contract Test", "owner-secret")
	if err := store.SetSetting(context.Background(), db, "setup_complete", "true"); err != nil {
		t.Fatalf("set setup_complete: %v", err)
	}
	srv.cfg.LLM = &mockLLMService{chatProv: &mockProvider{
		chatFunc: func(ctx context.Context, model string, messages []llm.Message, tools []llm.ToolDefinition, opts llm.Options) (*llm.Response, error) {
			return &llm.Response{
				Content: `{"title":"Automatic Chat Titles","confidence":0.93,"reason":"The user wants new chats named automatically."}`,
			}, nil
		},
	}}

	res := doReq(t, srv, http.MethodPost, "/api/navi/chats", "", "", "127.0.0.1:1234", map[string]any{
		"initial_message": "hello world, this is a test message to create a chat",
	})
	if res.StatusCode != http.StatusCreated {
		buf, _ := io.ReadAll(res.Body)
		t.Fatalf("expected 201 Created, got %d: %s", res.StatusCode, string(buf))
	}

	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	res.Body.Close()
	chatID := body["chat_id"]
	if chatID == "" {
		t.Fatalf("chat_id missing from response")
	}

	// 1. Verify no message is saved in navi_chat_messages
	var msgCount int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM navi_chat_messages WHERE chat_id = ?`, chatID).Scan(&msgCount); err != nil {
		t.Fatalf("query navi_chat_messages: %v", err)
	}
	if msgCount != 0 {
		t.Errorf("expected 0 messages in DB, got %d", msgCount)
	}

	// 2. Verify no runtime session chats are attached
	var sessionChatCount int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM runtime_session_chats WHERE chat_id = ?`, chatID).Scan(&sessionChatCount); err != nil {
		t.Fatalf("query runtime_session_chats: %v", err)
	}
	if sessionChatCount != 0 {
		t.Errorf("expected 0 runtime session chat connections, got %d", sessionChatCount)
	}

	// 3. Verify no runtime sessions are created
	var sessionCount int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM runtime_sessions`).Scan(&sessionCount); err != nil {
		t.Fatalf("query runtime_sessions: %v", err)
	}
	if sessionCount != 0 {
		t.Errorf("expected 0 runtime sessions, got %d", sessionCount)
	}

	// 4. Verify no runtime runs exist
	var runCount int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM runtime_runs WHERE chat_id = ?`, chatID).Scan(&runCount); err != nil {
		t.Fatalf("query runtime_runs: %v", err)
	}
	if runCount != 0 {
		t.Errorf("expected 0 runtime runs, got %d", runCount)
	}

	// 5. Verify no inbox items are enqueued
	var inboxCount int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM navi_inbox WHERE chat_id = ?`, chatID).Scan(&inboxCount); err != nil {
		t.Fatalf("query navi_inbox: %v", err)
	}
	if inboxCount != 0 {
		t.Errorf("expected 0 inbox items, got %d", inboxCount)
	}
}

func TestNaviListChatsFreshInstallReturnsEmptyArray(t *testing.T) {
	srv, _, db := testServerWithNavi(t)
	_ = claimTestInstance(t, srv, "Fresh List Test", "owner-secret")
	if err := store.SetSetting(context.Background(), db, "setup_complete", "true"); err != nil {
		t.Fatalf("set setup_complete: %v", err)
	}

	res := doReq(t, srv, http.MethodGet, "/api/navi/chats", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/navi/chats: expected 200, got %d", res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	res.Body.Close()
	if got := strings.TrimSpace(string(body)); got != "[]" {
		t.Fatalf("expected empty JSON array, got %s", got)
	}
}

func TestNaviCreateChatAcceptsProjectIDField(t *testing.T) {
	srv, _, db := testServerWithNavi(t)
	_ = claimTestInstance(t, srv, "ProjectID Test", "owner-secret")
	if err := store.SetSetting(context.Background(), db, "setup_complete", "true"); err != nil {
		t.Fatalf("set setup_complete: %v", err)
	}
	if err := store.SaveProject(context.Background(), db, schema.Project{
		ID:        "my-workspace-project",
		Title:     "My Workspace Project",
		Kind:      schema.ProjectKindGeneral,
		Status:    schema.ProjectStatusActive,
		Health:    schema.ProjectHealthUnknown,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		CreatedBy: "owner",
	}); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}

	res := doReq(t, srv, http.MethodPost, "/api/navi/chats", "", "", "127.0.0.1:1234", map[string]any{
		"project_id": "my-workspace-project",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created for project_id field, got %d", res.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["project_id"] != "my-workspace-project" {
		t.Fatalf("expected project_id in response, got %#v", body)
	}
	if got := srv.cfg.WorldModel.GetActiveProjectID(context.Background()); got != "my-workspace-project" {
		t.Fatalf("expected active project to be set, got %q", got)
	}
}

func TestNaviCreateChatRejectsMissingProjectID(t *testing.T) {
	srv, _, db := testServerWithNavi(t)
	_ = claimTestInstance(t, srv, "Missing Project Test", "owner-secret")
	if err := store.SetSetting(context.Background(), db, "setup_complete", "true"); err != nil {
		t.Fatalf("set setup_complete: %v", err)
	}

	res := doReq(t, srv, http.MethodPost, "/api/navi/chats", "", "", "127.0.0.1:1234", map[string]any{
		"project_id": "missing-project",
	})
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing project_id, got %d", res.StatusCode)
	}
}

func TestProjectChatAPI_CreateAndList(t *testing.T) {
	srv, _, db := testServerWithNavi(t)
	_ = claimTestInstance(t, srv, "Project Session Test", "owner-secret")
	if err := store.SetSetting(context.Background(), db, "setup_complete", "true"); err != nil {
		t.Fatalf("set setup_complete: %v", err)
	}
	now := time.Now().UTC()
	if err := store.SaveProject(context.Background(), db, schema.Project{
		ID:        "proj-session",
		Title:     "Session Project",
		Kind:      schema.ProjectKindGeneral,
		Status:    schema.ProjectStatusActive,
		Health:    schema.ProjectHealthUnknown,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "owner",
	}); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	if err := store.SaveProject(context.Background(), db, schema.Project{
		ID:        "proj-other",
		Title:     "Other Project",
		Kind:      schema.ProjectKindGeneral,
		Status:    schema.ProjectStatusActive,
		Health:    schema.ProjectHealthUnknown,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "owner",
	}); err != nil {
		t.Fatalf("SaveProject(other): %v", err)
	}

	res := doReq(t, srv, http.MethodPost, "/api/projects/proj-session/chats", "", "", "127.0.0.1:1234", map[string]any{})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/projects/{id}/chats: expected 201, got %d", res.StatusCode)
	}
	var created map[string]string
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatalf("decode created chat: %v", err)
	}
	if created["chat_id"] == "" || created["project_id"] != "proj-session" {
		t.Fatalf("unexpected project chat create response: %#v", created)
	}
	if _, err := srv.cfg.Navi.CreateChat(context.Background(), navi.ExperienceModeStandard, "proj-other"); err != nil {
		t.Fatalf("CreateChat(other): %v", err)
	}

	res = doReq(t, srv, http.MethodGet, "/api/projects/proj-session/chats", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/projects/{id}/chats: expected 200, got %d", res.StatusCode)
	}
	var list map[string]any
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatalf("decode project chats: %v", err)
	}
	items, ok := list["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one project chat, got %#v", list)
	}
	first, _ := items[0].(map[string]any)
	if first["projectId"] != "proj-session" {
		t.Fatalf("expected listed chat to be scoped to proj-session, got %#v", first)
	}

	res = doReq(t, srv, http.MethodGet, "/api/navi/chats?project_id=proj-session", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/navi/chats?project_id: expected 200, got %d", res.StatusCode)
	}
	var filtered []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&filtered); err != nil {
		t.Fatalf("decode filtered navi chats: %v", err)
	}
	if len(filtered) != 1 || filtered[0]["projectId"] != "proj-session" {
		t.Fatalf("expected one filtered chat for proj-session, got %#v", filtered)
	}
}

func TestNaviGetChatRuntimeSummaryIncludesTerminalEvent(t *testing.T) {
	srv, _, db := testServerWithNavi(t)
	sessionStore := navistore.NewSQLiteStore(db)
	ctx := context.Background()

	now := time.Now().UTC()
	if err := store.SaveProject(ctx, db, schema.Project{
		ID:        "proj-runtime",
		Title:     "Runtime Project",
		Kind:      schema.ProjectKindGeneral,
		Status:    schema.ProjectStatusActive,
		Health:    schema.ProjectHealthUnknown,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "owner",
	}); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}

	chat, err := sessionStore.CreateChat(ctx, navi.CreateChatInput{
		ProjectID:      "proj-runtime",
		ExperienceMode: string(navi.ExperienceModeStandard),
		Title:          "Runtime Chat",
	})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	chatID := string(chat.ID)
	run := naviruntime.NewRun("runtime-"+chatID, string(navi.ExperienceModeStandard))
	run.ChatID = chatID
	run.ProjectID = "proj-runtime"
	run.SetStatus(schema.RunStatusActive)
	if err := sessionStore.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	msgID, err := sessionStore.CompleteRun(ctx, run, "final reply", string(navi.ExperienceModeStandard), "")
	if err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}

	res := doReq(t, srv, http.MethodGet, "/api/navi/chats/"+chatID+"/runtime_summary", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET runtime_summary: expected 200, got %d", res.StatusCode)
	}
	defer res.Body.Close()

	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode runtime summary: %v", err)
	}
	runBody, ok := body["run"].(map[string]any)
	if !ok {
		t.Fatalf("expected run summary payload, got %#v", body)
	}
	if got, _ := runBody["status"].(string); got != string(schema.RunStatusCompleted) {
		t.Fatalf("run status = %q, want %q", got, schema.RunStatusCompleted)
	}
	if got, _ := body["project_id"].(string); got != "proj-runtime" {
		t.Fatalf("expected project_id proj-runtime in summary, got %#v", body)
	}
	projectBody, ok := body["project"].(map[string]any)
	if !ok || projectBody["project_id"] != "proj-runtime" {
		t.Fatalf("expected project identity in summary, got %#v", body["project"])
	}
	terminal, ok := body["terminal_event"].(map[string]any)
	if !ok {
		t.Fatalf("expected terminal_event in summary, got %#v", body)
	}
	if got, _ := terminal["type"].(string); got != string(schema.FactRunCompleted) {
		t.Fatalf("terminal event type = %q, want %q", got, schema.FactRunCompleted)
	}
	payload, ok := terminal["payload"].(map[string]any)
	if !ok {
		t.Fatalf("expected terminal event payload, got %#v", terminal)
	}
	if got, _ := payload["final_message_id"].(string); got != msgID {
		t.Fatalf("final_message_id = %q, want %q", got, msgID)
	}
}

func TestExperienceAPIGetReturnsStoredSnapshot(t *testing.T) {
	srv, _, db := testServer(t)
	claimTestInstance(t, srv, "Experience Owner", "owner-secret")

	ctx := context.Background()
	ownerID, err := store.GetOwnerID(ctx, db)
	if err != nil {
		t.Fatalf("GetOwnerID: %v", err)
	}
	if ownerID == "" {
		t.Fatalf("expected owner id after claim")
	}
	summaryFirst := true
	if err := experience.SaveModuleConfiguration(ctx, db, experience.ConfigScopeGlobal, experience.ConfigScopeSystem, experience.ModuleConfig{
		ModuleID: "global_briefing",
		Version:  "2026.04",
		Kind:     experience.ModuleKindBundle,
		Strength: 1,
		TraitContributions: map[string]float64{
			"structure_level": 0.82,
		},
	}, string(schema.StateKindOwnerSet)); err != nil {
		t.Fatalf("SaveModuleConfiguration(global): %v", err)
	}
	if err := experience.SaveCoreIdentityConfiguration(ctx, db, ownerID, map[string]float64{"warmth": 0.91}, string(schema.StateKindOwnerSet)); err != nil {
		t.Fatalf("SaveCoreIdentityConfiguration: %v", err)
	}
	if err := experience.SaveOutputPreferencesConfiguration(ctx, db, ownerID, experience.OutputPreferences{
		PreferredLength: experience.LengthShort,
		SummaryFirst:    &summaryFirst,
	}, string(schema.StateKindOwnerSet)); err != nil {
		t.Fatalf("SaveOutputPreferencesConfiguration: %v", err)
	}
	if err := experience.SaveRelationshipProfileConfiguration(ctx, db, ownerID, experience.RelationshipProfile{
		TraitEstimates:     map[string]float64{"humor_playfulness": 0.05},
		Confidence:         0.44,
		PerTraitConfidence: map[string]float64{"humor_playfulness": 0.90},
	}, string(schema.StateKindInferred)); err != nil {
		t.Fatalf("SaveRelationshipProfileConfiguration: %v", err)
	}

	res := doReq(t, srv, http.MethodGet, "/api/experience", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	defer res.Body.Close()

	var got experienceSnapshotResponse
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.OwnerID != ownerID {
		t.Fatalf("expected owner id %q, got %q", ownerID, got.OwnerID)
	}
	if got.CoreIdentity.Traits["warmth"] != 0.91 {
		t.Fatalf("expected warmth override, got %+v", got.CoreIdentity.Traits)
	}
	if got.OutputPreferences.PreferredLength != experience.LengthShort {
		t.Fatalf("expected preferred length short, got %q", got.OutputPreferences.PreferredLength)
	}
	if got.OutputPreferences.SummaryFirst == nil || !*got.OutputPreferences.SummaryFirst {
		t.Fatalf("expected summary_first=true, got %+v", got.OutputPreferences)
	}
	if len(got.PersonaModules) != 1 || got.PersonaModules[0].ModuleID != "global_briefing" {
		t.Fatalf("expected global briefing module, got %+v", got.PersonaModules)
	}
	if got.PersonaModules[0].Version != "2026.04" || got.PersonaModules[0].Kind != experience.ModuleKindBundle {
		t.Fatalf("expected versioned bundle module metadata, got %+v", got.PersonaModules[0])
	}
	if got.RelationshipProfile.Confidence != 0.44 {
		t.Fatalf("expected relationship confidence 0.44, got %.2f", got.RelationshipProfile.Confidence)
	}
}

func TestExperienceAPIPutCoreIdentityAndOutputPreferencesPersist(t *testing.T) {
	srv, _, db := testServer(t)
	claimTestInstance(t, srv, "Experience Owner", "owner-secret")

	ctx := context.Background()
	ownerID, err := store.GetOwnerID(ctx, db)
	if err != nil {
		t.Fatalf("GetOwnerID: %v", err)
	}

	res := doReq(t, srv, http.MethodPut, "/api/experience/core-identity", "", "", "127.0.0.1:1234", map[string]any{
		"traits": map[string]float64{"warmth": 0.88, "directness": 0.33},
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("core identity put: expected 200, got %d", res.StatusCode)
	}
	res.Body.Close()

	snapshot, err := experience.LoadConfigurationSnapshot(ctx, db, ownerID)
	if err != nil {
		t.Fatalf("LoadConfigurationSnapshot(core): %v", err)
	}
	if snapshot.CoreIdentity.Traits["warmth"] != 0.88 || snapshot.CoreIdentity.Traits["directness"] != 0.33 {
		t.Fatalf("unexpected core identity snapshot: %+v", snapshot.CoreIdentity.Traits)
	}

	res = doReq(t, srv, http.MethodPut, "/api/experience/output-preferences", "", "", "127.0.0.1:1234", map[string]any{
		"preferred_length": "short",
		"preferred_format": "structured",
		"summary_first":    false,
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("output preferences put: expected 200, got %d", res.StatusCode)
	}
	res.Body.Close()

	snapshot, err = experience.LoadConfigurationSnapshot(ctx, db, ownerID)
	if err != nil {
		t.Fatalf("LoadConfigurationSnapshot(output): %v", err)
	}
	if snapshot.OutputPreferences.PreferredLength != experience.LengthShort {
		t.Fatalf("expected preferred length short, got %q", snapshot.OutputPreferences.PreferredLength)
	}
	if snapshot.OutputPreferences.PreferredFormat != experience.FormatStructured {
		t.Fatalf("expected preferred format structured, got %q", snapshot.OutputPreferences.PreferredFormat)
	}
	if snapshot.OutputPreferences.SummaryFirst == nil || *snapshot.OutputPreferences.SummaryFirst {
		t.Fatalf("expected summary_first=false, got %+v", snapshot.OutputPreferences)
	}

	res = doReq(t, srv, http.MethodPut, "/api/experience/output-preferences", "", "", "127.0.0.1:1234", map[string]any{})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("output preferences clear: expected 200, got %d", res.StatusCode)
	}
	res.Body.Close()

	snapshot, err = experience.LoadConfigurationSnapshot(ctx, db, ownerID)
	if err != nil {
		t.Fatalf("LoadConfigurationSnapshot(clear): %v", err)
	}
	if snapshot.OutputPreferences.PreferredLength != "" || snapshot.OutputPreferences.PreferredFormat != "" || snapshot.OutputPreferences.SummaryFirst != nil {
		t.Fatalf("expected cleared output preferences, got %+v", snapshot.OutputPreferences)
	}
}

func TestExperienceAPIPutModulesReplacesOwnerModuleRegistry(t *testing.T) {
	srv, _, db := testServer(t)
	claimTestInstance(t, srv, "Experience Owner", "owner-secret")

	ctx := context.Background()
	ownerID, err := store.GetOwnerID(ctx, db)
	if err != nil {
		t.Fatalf("GetOwnerID: %v", err)
	}
	if err := experience.SaveModuleConfiguration(ctx, db, experience.ConfigScopeGlobal, experience.ConfigScopeSystem, experience.ModuleConfig{
		ModuleID:           "global_briefing",
		Version:            "2026.04",
		Kind:               experience.ModuleKindBundle,
		Strength:           1,
		TraitContributions: map[string]float64{"structure_level": 0.82},
	}, string(schema.StateKindOwnerSet)); err != nil {
		t.Fatalf("SaveModuleConfiguration(global): %v", err)
	}
	if err := experience.SaveModuleConfiguration(ctx, db, experience.ConfigScopeOwner, ownerID, experience.ModuleConfig{
		ModuleID:           "old_owner_module",
		Version:            "legacy",
		Kind:               experience.ModuleKindMicro,
		Strength:           1,
		TraitContributions: map[string]float64{"warmth": 0.25},
	}, string(schema.StateKindOwnerSet)); err != nil {
		t.Fatalf("SaveModuleConfiguration(owner): %v", err)
	}

	res := doReq(t, srv, http.MethodPut, "/api/experience/modules", "", "", "127.0.0.1:1234", map[string]any{
		"persona_modules": []map[string]any{
			{
				"module_id":           "owner_brief",
				"version":             "2026.04.1",
				"kind":                "overlay",
				"scope":               "conversation",
				"strength":            0.7,
				"trait_contributions": map[string]float64{"verbosity": 0.20},
			},
			{
				"module_id":           "owner_direct",
				"kind":                "micro_module",
				"trait_contributions": map[string]float64{"directness": 0.80},
			},
		},
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("modules put: expected 200, got %d", res.StatusCode)
	}
	defer res.Body.Close()

	var got experienceSnapshotResponse
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	seen := map[string]bool{}
	for _, module := range got.PersonaModules {
		seen[module.ModuleID] = true
		if module.ModuleID == "owner_brief" && (module.Version != "2026.04.1" || module.Kind != experience.ModuleKindOverlay) {
			t.Fatalf("expected versioned overlay metadata for owner_brief, got %+v", module)
		}
		if module.ModuleID == "owner_direct" && (module.Version != "v1" || module.Kind != experience.ModuleKindMicro) {
			t.Fatalf("expected default version and micro-module metadata for owner_direct, got %+v", module)
		}
	}
	if !seen["global_briefing"] || !seen["owner_brief"] || !seen["owner_direct"] {
		t.Fatalf("expected merged global and owner modules, got %+v", got.PersonaModules)
	}
	if seen["old_owner_module"] {
		t.Fatalf("expected owner modules to be replaced, got %+v", got.PersonaModules)
	}

	entries, err := store.ListConfigurationByScope(ctx, db, experience.ConfigScopeOwner, ownerID, 20)
	if err != nil {
		t.Fatalf("ListConfigurationByScope: %v", err)
	}
	moduleCount := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Key, experience.ConfigKeyModulePrefix) {
			moduleCount++
		}
	}
	if moduleCount != 2 {
		t.Fatalf("expected exactly 2 owner module entries, got %d", moduleCount)
	}
}

func TestExperienceAPIGetModuleRegistryListsAvailableModules(t *testing.T) {
	srv, _, db := testServer(t)
	claimTestInstance(t, srv, "Experience Owner", "owner-secret")

	ctx := context.Background()
	ownerID, err := store.GetOwnerID(ctx, db)
	if err != nil {
		t.Fatalf("GetOwnerID: %v", err)
	}
	if err := experience.SaveModuleConfiguration(ctx, db, experience.ConfigScopeGlobal, experience.ConfigScopeSystem, experience.ModuleConfig{
		ModuleID:           "global_briefing",
		Version:            "2026.04",
		Kind:               experience.ModuleKindBundle,
		Strength:           1,
		TraitContributions: map[string]float64{"structure_level": 0.82},
	}, string(schema.StateKindOwnerSet)); err != nil {
		t.Fatalf("SaveModuleConfiguration(global): %v", err)
	}
	if err := experience.SaveModuleConfiguration(ctx, db, experience.ConfigScopeOwner, ownerID, experience.ModuleConfig{
		ModuleID:           "owner_direct",
		Kind:               experience.ModuleKindMicro,
		Scope:              experience.ModuleScopeConversation,
		Strength:           0.8,
		TraitContributions: map[string]float64{"directness": 0.80},
	}, string(schema.StateKindOwnerSet)); err != nil {
		t.Fatalf("SaveModuleConfiguration(owner): %v", err)
	}

	res := doReq(t, srv, http.MethodGet, "/api/experience/module-registry", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("module registry get: expected 200, got %d", res.StatusCode)
	}
	defer res.Body.Close()

	var got experienceModuleRegistryResponse
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.OwnerID != ownerID {
		t.Fatalf("expected owner id %q, got %q", ownerID, got.OwnerID)
	}
	if len(got.Modules) != 2 {
		t.Fatalf("expected 2 registry entries, got %+v", got.Modules)
	}
	if got.Modules[0].ModuleID != "owner_direct" || got.Modules[0].ScopeID != ownerID {
		t.Fatalf("expected owner conversation module first in registry, got %+v", got.Modules)
	}
	if got.Modules[0].Version != "v1" || got.Modules[0].Kind != experience.ModuleKindMicro {
		t.Fatalf("expected owner module metadata, got %+v", got.Modules[0])
	}
	if got.Modules[1].ModuleID != "global_briefing" || got.Modules[1].ScopeID != experience.ConfigScopeSystem {
		t.Fatalf("expected global module in registry, got %+v", got.Modules)
	}
}

func TestExperienceAPIPutRejectsUnsupportedValues(t *testing.T) {
	srv, _, _ := testServer(t)
	claimTestInstance(t, srv, "Experience Owner", "owner-secret")

	for _, tc := range []struct {
		name string
		path string
		body map[string]any
	}{
		{name: "unsupported trait", path: "/api/experience/core-identity", body: map[string]any{"traits": map[string]float64{"unknown_trait": 0.5}}},
		{name: "unsupported preferred format", path: "/api/experience/output-preferences", body: map[string]any{"preferred_format": "slide_deck"}},
		{name: "unsupported module scope", path: "/api/experience/modules", body: map[string]any{"persona_modules": []map[string]any{{"module_id": "bad_scope", "scope": "workspace"}}}},
		{name: "unsupported module kind", path: "/api/experience/modules", body: map[string]any{"persona_modules": []map[string]any{{"module_id": "bad_kind", "kind": "macro_pack"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := doReq(t, srv, http.MethodPut, tc.path, "", "", "127.0.0.1:1234", tc.body)
			if res.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", res.StatusCode)
			}
			res.Body.Close()
		})
	}
}

func TestOpenAICompatRejectsUnsupportedModels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		model   string
		want    navi.ExperienceMode
		wantErr bool
	}{
		{name: "default", model: "", want: navi.ExperienceModeStandard},
		{name: "standard", model: "navi", want: navi.ExperienceModeStandard},
		{name: "wizard rejected", model: "wizard", wantErr: true},
		{name: "unsupported model rejected", model: "unsupported-model", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := oaiModelToExperienceMode(tt.model)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("oaiModelToExperienceMode(%q) expected error", tt.model)
				}
				return
			}
			if err != nil {
				t.Fatalf("oaiModelToExperienceMode(%q) error = %v", tt.model, err)
			}
			if got != tt.want {
				t.Fatalf("oaiModelToExperienceMode(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}

func TestGatewayAPIKeyManagement(t *testing.T) {
	srv, _, _ := testServer(t)

	// Claim first.
	ownerSecret := "ownerS3cret!"
	passport := claimTestInstance(t, srv, "Owner", ownerSecret)
	apiKey, _ := passport["primary_api_key"].(string)

	// Create additional key (requires X-Owner-Secret).
	res := doReq(t, srv, "POST", "/api/keys", apiKey, ownerSecret, "192.168.1.1:1234", map[string]any{
		"name":   "ci-key",
		"scopes": []string{"read"},
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create key: expected 201, got %d", res.StatusCode)
	}
	var newKey map[string]any
	json.NewDecoder(res.Body).Decode(&newKey)
	keyID, _ := newKey["id"].(string)
	rawNewKey, _ := newKey["api_key"].(string)
	if keyID == "" || !strings.HasPrefix(rawNewKey, "navi_") {
		t.Fatalf("bad key response: %+v", newKey)
	}

	// List keys.
	res = doReq(t, srv, "GET", "/api/keys", apiKey, ownerSecret, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list keys: expected 200, got %d", res.StatusCode)
	}
	var listed []map[string]any
	json.NewDecoder(res.Body).Decode(&listed)
	if len(listed) < 1 {
		t.Fatalf("expected at least 1 key in list")
	}
	if _, ok := listed[0]["api_key"]; ok {
		t.Fatalf("list response must not include raw api_key")
	}

	// New key works for API access.
	res = doJSONReq(t, srv, "GET", "/api/governor", rawNewKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("new key auth: expected 200, got %d", res.StatusCode)
	}

	// Revoke the new key.
	res = doReq(t, srv, "DELETE", "/api/keys/"+keyID, apiKey, ownerSecret, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("revoke: expected 200, got %d", res.StatusCode)
	}

	// Revoked key no longer works.
	res = doJSONReq(t, srv, "GET", "/api/governor", rawNewKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked key: expected 401, got %d", res.StatusCode)
	}

	// Require owner secret for key ops; wrong secret → 401.
	res = doReq(t, srv, "GET", "/api/keys", apiKey, "wrong-secret", "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong owner secret: expected 401, got %d", res.StatusCode)
	}
}

func TestAuthMiddlewareAPIKeyContext(t *testing.T) {
	db := testDB(t)

	rawKey, hash, err := store.GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	err = store.CreateAPIKey(context.Background(), db, store.APIKey{
		ID:        "test-key-1",
		OwnerID:   "owner-context",
		KeyHash:   hash,
		Scopes:    []string{"read", "execute"},
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	var gotOwner string
	var gotScopes []string
	protected := AuthMiddleware(db, "", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOwner = OwnerIDFromCtx(r.Context())
		gotScopes = ScopesFromCtx(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/governor", nil)
	req.Header.Set("X-API-Key", rawKey)
	req.RemoteAddr = "192.168.1.10:5555"
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if gotOwner != "owner-context" {
		t.Fatalf("expected owner-context, got %q", gotOwner)
	}
	if len(gotScopes) != 2 {
		t.Fatalf("unexpected scopes: %v", gotScopes)
	}
}

func TestAuthMiddlewareInvalidAPIKey(t *testing.T) {
	db := testDB(t)
	protected := AuthMiddleware(db, "", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/governor", nil)
	req.Header.Set("X-API-Key", "navi_totally_fake")
	req.RemoteAddr = "192.168.1.10:5555"
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestContextQueryHappyPathRedactedAndAudited(t *testing.T) {
	srv, _, db := testServer(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)
	end := now.Add(time.Second)
	if err := store.SaveExecutionOutcome(ctx, db, schema.ExecutionOutcome{
		AttemptID:          "run-cq",
		CommandID:          "cmd-cq",
		AttemptNumber:      1,
		CommandType:        schema.CommandTypeInvoke,
		StartTime:          now,
		EndTime:            &end,
		Outcome:            schema.ExecutionOutcomeFailed,
		FailureClass:       schema.FailureClassExecutionFailure,
		FailureReason:      "do-not-leak this raw reason",
		AffectedEntities:   `["contact:bob"]`,
		CompensationStatus: schema.CompensationStatusNotRequired,
		RecoveryStatus:     schema.RecoveryStatusNotRequired,
	}); err != nil {
		t.Fatalf("SaveExecutionOutcome: %v", err)
	}

	body := `{"run_id":"run-cq","purpose":"eval_scoring","scope":"current_run_summary"}`
	req := httptest.NewRequest(http.MethodPost, "/api/context/query", bytes.NewBufferString(body))
	req.RemoteAddr = "127.0.0.1:5555" // loopback → owner-like access
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Context    map[string]any `json:"context"`
		Provenance struct {
			Source         string   `json:"source"`
			KernelMediated bool     `json:"kernel_mediated"`
			Redactions     []string `json:"redactions"`
		} `json:"provenance"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, leaked := resp.Context["failure_reason"]; leaked {
		t.Fatalf("failure_reason leaked through endpoint")
	}
	if !resp.Provenance.KernelMediated || resp.Provenance.Source == "" {
		t.Fatalf("missing provenance markers: %+v", resp.Provenance)
	}

	events, err := store.EventsByCorrelationID(ctx, db, "run-cq")
	if err != nil {
		t.Fatalf("EventsByCorrelationID: %v", err)
	}
	var audited bool
	for _, ev := range events {
		if ev.Type == schema.FactContextRead {
			audited = true
		}
	}
	if !audited {
		t.Fatalf("expected a context.read audit event")
	}
}

func TestContextQueryRejectsMissingPurpose(t *testing.T) {
	srv, _, _ := testServer(t)
	body := `{"run_id":"run-cq","scope":"current_run_summary"}`
	req := httptest.NewRequest(http.MethodPost, "/api/context/query", bytes.NewBufferString(body))
	req.RemoteAddr = "127.0.0.1:5555"
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing purpose, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestContextQueryRejectsUnknownScope(t *testing.T) {
	srv, _, _ := testServer(t)
	body := `{"run_id":"run-cq","purpose":"eval_scoring","scope":"everything"}`
	req := httptest.NewRequest(http.MethodPost, "/api/context/query", bytes.NewBufferString(body))
	req.RemoteAddr = "127.0.0.1:5555"
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown scope, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCORSMiddlewareAllowsConfiguredOrigin(t *testing.T) {
	protected := corsMiddleware([]string{"https://pet.example"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Header.Set("Origin", "https://pet.example")
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://pet.example" {
		t.Fatalf("expected allowed origin header, got %q", got)
	}
}

func TestCORSMiddlewareRejectsUnconfiguredOrigin(t *testing.T) {
	protected := corsMiddleware([]string{"https://pet.example"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Header.Set("Origin", "https://attacker.example")
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no allow-origin header for rejected origin, got %q", got)
	}
}

func TestCORSPreflightRejectsUnconfiguredOrigin(t *testing.T) {
	handler := corsPreflightHandler([]string{"https://pet.example"})

	req := httptest.NewRequest(http.MethodOptions, "/api/status", nil)
	req.Header.Set("Origin", "https://attacker.example")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no allow-origin header for rejected preflight, got %q", got)
	}
}

func TestOperatorOverviewIncludesCockpitSections(t *testing.T) {
	srv, _, db := testServer(t)
	now := time.Now().UTC()
	if err := store.SaveProposal(context.Background(), db, schema.Proposal{
		ProposalID:       "proposal-overview",
		SourceProcess:    "validate_govern",
		SourceTrigger:    "workspace boundary",
		ProposedAction:   "read outside workspace",
		AffectedEntities: `["session:overview"]`,
		Rationale:        "requires owner approval",
		Priority:         schema.ProposalPriorityBlocking,
		Status:           schema.ProposalStatusPending,
		CreatedAt:        now,
	}); err != nil {
		t.Fatalf("SaveProposal: %v", err)
	}

	res := doJSONReq(t, srv, http.MethodGet, "/api/operator/overview", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode overview: %v", err)
	}
	for _, key := range []string{"status", "agent", "proposals", "runs", "activity", "skills"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("expected overview section %q", key)
		}
	}
	proposals, ok := body["proposals"].(map[string]any)
	if !ok {
		t.Fatalf("expected proposals object, got %#v", body["proposals"])
	}
	if got := int(proposals["pending_count"].(float64)); got != 1 {
		t.Fatalf("expected pending_count 1, got %d", got)
	}
}

func TestGatewayDirectives(t *testing.T) {
	srv, b, _ := testServer(t)

	res := doJSONReq(t, srv, "POST", "/api/directives", "", "127.0.0.1:1234", map[string]string{"title": "Test Directive", "mode": "CHAT"})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", res.StatusCode)
	}
	var d schema.Directive
	json.NewDecoder(res.Body).Decode(&d)
	if d.Title != "Test Directive" || d.Mode != "CHAT" {
		t.Fatalf("bad directive created")
	}

	res = doJSONReq(t, srv, "GET", "/api/directives", "", "127.0.0.1:1234", nil)
	var dl []schema.Directive
	json.NewDecoder(res.Body).Decode(&dl)
	if len(dl) != 1 || dl[0].DirectiveID != d.DirectiveID {
		t.Fatalf("bad directive list")
	}

	evCh := make(chan schema.Event, 1)
	err := b.Subscribe(context.Background(), schema.CmdDirectiveMessage, "test-queue", func(ctx context.Context, ev schema.Event) {
		evCh <- ev
	})
	if err != nil {
		t.Fatalf("subscribe err: %v", err)
	}

	res = doJSONReq(t, srv, "POST", "/api/directives/"+d.DirectiveID+"/message", "", "127.0.0.1:1234", map[string]string{"content": "hello there!"})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", res.StatusCode)
	}

	select {
	case ev := <-evCh:
		var p schema.DirectiveMessagePayload
		tmp, _ := json.Marshal(ev.Payload)
		json.Unmarshal(tmp, &p)
		if p.DirectiveID != d.DirectiveID || p.OwnerID != "local" {
			t.Fatalf("bad event payload: %+v", p)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for CmdDirectiveMessage")
	}
}

func TestGatewayLiveWS(t *testing.T) {
	srv, b, _ := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws/live"
	c, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("dial err: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	initMsg := map[string]any{
		"type":   "req",
		"id":     "connect",
		"method": "connect",
		"params": map[string]any{"chat_id": "corr-ws", "after_seq": 0, "stream": "user"},
	}
	initBytes, _ := json.Marshal(initMsg)
	err = c.Write(context.Background(), websocket.MessageText, initBytes)
	if err != nil {
		t.Fatalf("write init err: %v", err)
	}

	if _, _, err := c.Read(context.Background()); err != nil {
		t.Fatalf("read connect ack err: %v", err)
	}

	ev := schema.NewEvent(schema.FactDirectiveReplied, schema.EventKindFact, "corr-ws", schema.AgentNavi, schema.DirectiveRepliedPayload{
		DirectiveID: "dir-1",
		MessageID:   "msg-1",
	})
	ev.Visibility = schema.VisibilityUser
	if err := b.Publish(context.Background(), ev); err != nil {
		t.Fatalf("publish err: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, respBytes, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read err: %v", err)
	}

	var frame struct {
		Type  string       `json:"type"`
		Event schema.Event `json:"event"`
	}
	json.Unmarshal(respBytes, &frame)
	parsed := frame.Event
	if parsed.ID != ev.ID {
		t.Fatalf("expected ev ID %s, got %s", ev.ID, parsed.ID)
	}
}

func TestGatewayLiveWSRequiresAuthForNonLoopbackRequests(t *testing.T) {
	srv, _, _ := testServer(t)

	res := doReq(t, srv, http.MethodGet, "/ws/live", "", "", "203.0.113.10:1234", nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for non-loopback unauthenticated /ws/live, got %d", res.StatusCode)
	}
}

func TestGatewayLiveWSOperatorStreamReplaysOperatorVisibleEvent(t *testing.T) {
	srv, b, _ := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	operatorEvent := schema.NewRunEvent(
		schema.FactRunPhaseChanged,
		schema.EventKindFact,
		"corr-operator-stream",
		schema.AgentNavi,
		"run-operator-stream",
		schema.VisibilityOperator,
		schema.RunPhaseChangedPayload{
			RunID:            "run-operator-stream",
			RuntimeSessionID: "corr-operator-stream",
			Phase:            "execute",
			Previous:         "contextualize",
		},
	)
	if err := b.Publish(context.Background(), operatorEvent); err != nil {
		t.Fatalf("publish operator-visible event err: %v", err)
	}

	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws/live"
	c, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("dial err: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	initMsg := map[string]any{
		"type":   "req",
		"id":     "connect",
		"method": "connect",
		"params": map[string]any{"chat_id": "corr-operator-stream", "after_seq": 0, "stream": "operator"},
	}
	initBytes, _ := json.Marshal(initMsg)
	if err := c.Write(context.Background(), websocket.MessageText, initBytes); err != nil {
		t.Fatalf("write init err: %v", err)
	}

	if _, _, err := c.Read(context.Background()); err != nil {
		t.Fatalf("read connect ack err: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, respBytes, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read replayed operator event err: %v", err)
	}

	var frame struct {
		Type  string       `json:"type"`
		Event schema.Event `json:"event"`
	}
	if err := json.Unmarshal(respBytes, &frame); err != nil {
		t.Fatalf("unmarshal replay frame err: %v", err)
	}
	if frame.Type != "event" {
		t.Fatalf("expected event frame, got %q", frame.Type)
	}
	if frame.Event.ID != operatorEvent.ID {
		t.Fatalf("expected operator event %s, got %s", operatorEvent.ID, frame.Event.ID)
	}
	if frame.Event.Type != schema.FactRunPhaseChanged {
		t.Fatalf("expected %q, got %q", schema.FactRunPhaseChanged, frame.Event.Type)
	}
}

func TestGatewayLiveWSConnectReplayFalseSkipsBacklog(t *testing.T) {
	srv, b, _ := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	backlog := schema.NewEvent(schema.FactDirectiveReplied, schema.EventKindFact, "corr-no-replay", schema.AgentNavi, schema.DirectiveRepliedPayload{
		DirectiveID: "dir-backlog",
		MessageID:   "msg-backlog",
	})
	backlog.Visibility = schema.VisibilityUser
	if err := b.Publish(context.Background(), backlog); err != nil {
		t.Fatalf("publish backlog err: %v", err)
	}

	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws/live"
	c, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("dial err: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	initMsg := map[string]any{
		"type":   "req",
		"id":     "connect",
		"method": "connect",
		"params": map[string]any{"chat_id": "corr-no-replay", "after_seq": 0, "replay": false, "stream": "user"},
	}
	initBytes, _ := json.Marshal(initMsg)
	if err := c.Write(context.Background(), websocket.MessageText, initBytes); err != nil {
		t.Fatalf("write init err: %v", err)
	}

	_, ackBytes, err := c.Read(context.Background())
	if err != nil {
		t.Fatalf("read connect ack err: %v", err)
	}
	var ack struct {
		Type   string `json:"type"`
		ID     string `json:"id"`
		OK     bool   `json:"ok"`
		Result struct {
			AfterSeq  int64 `json:"after_seq"`
			CursorSeq int64 `json:"cursor_seq"`
		} `json:"result"`
	}
	if err := json.Unmarshal(ackBytes, &ack); err != nil {
		t.Fatalf("unmarshal connect ack err: %v", err)
	}
	if !ack.OK || ack.Type != "res" || ack.ID != "connect" {
		t.Fatalf("unexpected connect ack: %s", string(ackBytes))
	}
	if ack.Result.AfterSeq != 0 {
		t.Fatalf("after_seq = %d, want 0", ack.Result.AfterSeq)
	}
	if ack.Result.CursorSeq <= 0 {
		t.Fatalf("cursor_seq = %d, want > 0 for replay:false cursor resolution", ack.Result.CursorSeq)
	}

	live := schema.NewEvent(schema.FactDirectiveReplied, schema.EventKindFact, "corr-no-replay", schema.AgentNavi, schema.DirectiveRepliedPayload{
		DirectiveID: "dir-live",
		MessageID:   "msg-live",
	})
	live.Visibility = schema.VisibilityUser
	if err := b.Publish(context.Background(), live); err != nil {
		t.Fatalf("publish live err: %v", err)
	}

	ctx, cancelLive := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelLive()
	_, respBytes, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read live event err: %v", err)
	}
	var frame struct {
		Type  string       `json:"type"`
		Event schema.Event `json:"event"`
	}
	if err := json.Unmarshal(respBytes, &frame); err != nil {
		t.Fatalf("unmarshal frame err: %v", err)
	}
	if frame.Event.ID != live.ID {
		t.Fatalf("expected live event %s, got %s", live.ID, frame.Event.ID)
	}
}

func TestGatewayLiveWSReplaysTerminalRunFailedEvent(t *testing.T) {
	srv, b, _ := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	failed := schema.NewRunEvent(
		schema.FactRunFailed,
		schema.EventKindFact,
		"corr-terminal-replay",
		schema.AgentNavi,
		"run-terminal-replay",
		schema.VisibilityUser,
		schema.RunFailedPayload{
			RunID:            "run-terminal-replay",
			RuntimeSessionID: "corr-terminal-replay",
			Error:            "The request took too long. Try /status, wait a moment, or start a fresh session with /new.",
			DurationMs:       1234,
		},
	)
	if err := b.Publish(context.Background(), failed); err != nil {
		t.Fatalf("publish run.failed backlog err: %v", err)
	}

	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws/live"
	c, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("dial err: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	initMsg := map[string]any{
		"type":   "req",
		"id":     "connect",
		"method": "connect",
		"params": map[string]any{"chat_id": "corr-terminal-replay", "after_seq": 0, "stream": "user"},
	}
	initBytes, _ := json.Marshal(initMsg)
	if err := c.Write(context.Background(), websocket.MessageText, initBytes); err != nil {
		t.Fatalf("write init err: %v", err)
	}

	if _, _, err := c.Read(context.Background()); err != nil {
		t.Fatalf("read connect ack err: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, respBytes, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read replayed run.failed err: %v", err)
	}

	var frame struct {
		Type  string       `json:"type"`
		Event schema.Event `json:"event"`
	}
	if err := json.Unmarshal(respBytes, &frame); err != nil {
		t.Fatalf("unmarshal replay frame err: %v", err)
	}
	if frame.Event.Type != schema.FactRunFailed {
		t.Fatalf("expected run.failed replay, got %#v", frame.Event)
	}
	if frame.Event.ID != failed.ID {
		t.Fatalf("expected replayed run.failed %s, got %s", failed.ID, frame.Event.ID)
	}
}

func TestGatewayLiveWSRejectsInternalSessionOnUserStream(t *testing.T) {
	srv, _, _ := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws/live"
	c, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("dial err: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	connectMsg, _ := json.Marshal(map[string]any{
		"type":   "req",
		"id":     "connect",
		"method": "connect",
		"params": map[string]any{"chat_id": "heartbeat-auto", "after_seq": 0, "stream": "user"},
	})
	if err := c.Write(context.Background(), websocket.MessageText, connectMsg); err != nil {
		t.Fatalf("write connect err: %v", err)
	}

	_, _, err = c.Read(context.Background())
	if err == nil {
		t.Fatal("expected websocket close for internal session on user stream")
	}
}

func TestGatewayLiveWSRejectsTypedInternalSessionOnAuditStream(t *testing.T) {
	srv, _, db := testServer(t)
	if err := navistore.MigrateSchema(context.Background(), db); err != nil {
		t.Fatalf("migrate navi schema: %v", err)
	}
	now := time.Now().UTC()
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO navi_chats (chat_id, title, status, visibility, owner_id) VALUES ('ops-internal-live', 'Internal', 'active', 'private', '')
	`); err != nil {
		t.Fatalf("seed internal chat: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO runtime_sessions (runtime_session_id, kind, status, started_at, last_active_at)
		VALUES ('ops-internal-live-rt', 'internal', 'active', ?, ?)
	`, now, now); err != nil {
		t.Fatalf("seed internal runtime session: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO runtime_session_chats (runtime_session_id, chat_id, relationship, attached_at)
		VALUES ('ops-internal-live-rt', 'ops-internal-live', 'primary', ?)
	`, now); err != nil {
		t.Fatalf("seed runtime session chat link: %v", err)
	}

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws/live"
	c, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("dial err: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	connectMsg, _ := json.Marshal(map[string]any{
		"type":   "req",
		"id":     "connect",
		"method": "connect",
		"params": map[string]any{"chat_id": "ops-internal-live", "after_seq": 0, "stream": "audit"},
	})
	if err := c.Write(context.Background(), websocket.MessageText, connectMsg); err != nil {
		t.Fatalf("write connect err: %v", err)
	}

	_, _, err = c.Read(context.Background())
	if err == nil {
		t.Fatal("expected websocket close for typed internal session on audit stream")
	}
}

func TestNaviListChatsDoesNotExposeLegacyInternalSessions(t *testing.T) {
	db := testDB(t)
	if err := navistore.MigrateSchema(context.Background(), db); err != nil {
		t.Fatalf("migrate navi schema: %v", err)
	}
	nowInternal := time.Now().UTC()
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO navi_chats (chat_id, title, status, visibility, owner_id) VALUES ('ops-internal-list', 'Internal', 'active', 'private', '')
	`); err != nil {
		t.Fatalf("seed internal chat: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO runtime_sessions (runtime_session_id, kind, status, started_at, last_active_at)
		VALUES ('ops-internal-list-rt', 'internal', 'active', ?, ?)
	`, nowInternal, nowInternal); err != nil {
		t.Fatalf("seed internal runtime session: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO runtime_session_chats (runtime_session_id, chat_id, relationship, attached_at)
		VALUES ('ops-internal-list-rt', 'ops-internal-list', 'primary', ?)
	`, nowInternal); err != nil {
		t.Fatalf("seed runtime session chat link: %v", err)
	}
	b := bus.NewMemBus(db)
	sessionStore := navistore.NewSQLiteStore(db)
	naviAgent, err := navi.New(navi.Config{
		DB:              db,
		Bus:             b,
		Chats:           sessionStore,
		RuntimeSessions: sessionStore,
	})
	if err != nil {
		t.Fatalf("new navi: %v", err)
	}
	userChatID, err := naviAgent.CreateChat(context.Background(), navi.ExperienceModeStandard, "")
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

	res := doReq(t, srv, http.MethodGet, "/api/navi/chats", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/navi/chats: expected 200, got %d", res.StatusCode)
	}
	var chats []navi.Chat
	if err := json.NewDecoder(res.Body).Decode(&chats); err != nil {
		t.Fatalf("decode chats: %v", err)
	}
	if len(chats) != 1 {
		t.Fatalf("expected 1 public chat, got %d", len(chats))
	}
	if string(chats[0].ID) != userChatID {
		t.Fatalf("expected public chat %q, got %q", userChatID, chats[0].ID)
	}
}

func TestGatewayLiveWSGetPresenceAndAck(t *testing.T) {
	srv, _, db := testServer(t)
	if err := navistore.MigrateSchema(context.Background(), db); err != nil {
		t.Fatalf("migrate navi schema: %v", err)
	}
	for _, stmt := range []string{
		`INSERT INTO navi_chats (chat_id, title, status, visibility) VALUES ('sess-live', 'Test', 'active', 'private')`,
		`INSERT INTO navi_inbox (inbox_item_id, chat_id, source_channel, queue_action, status, received_at)
		 VALUES ('inbox-1', 'sess-live', 'web', 'append', 'pending', CURRENT_TIMESTAMP)`,
		`INSERT INTO runtime_runs (run_id, chat_id, experience_mode, mode, status, current_phase, started_at, updated_at)
		 VALUES ('run-live', 'sess-live', 'navi', 'foreground', 'active', 'decide', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
	} {
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("seed live presence: %v", err)
		}
	}

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws/live"
	c, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("dial err: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	connectMsg, _ := json.Marshal(map[string]any{
		"type":   "req",
		"id":     "connect",
		"method": "connect",
		"params": map[string]any{"chat_id": "sess-live", "after_seq": 0, "stream": "user"},
	})
	if err := c.Write(context.Background(), websocket.MessageText, connectMsg); err != nil {
		t.Fatalf("write connect err: %v", err)
	}
	if _, _, err := c.Read(context.Background()); err != nil {
		t.Fatalf("read connect ack err: %v", err)
	}

	for _, req := range []map[string]any{
		{"type": "req", "id": "presence", "method": "getPresence", "params": map[string]any{}},
		{"type": "req", "id": "ack", "method": "ackNotification", "params": map[string]any{"notification_id": "note-1"}},
	} {
		b, _ := json.Marshal(req)
		if err := c.Write(context.Background(), websocket.MessageText, b); err != nil {
			t.Fatalf("write req %s err: %v", req["method"], err)
		}
		_, respBytes, err := c.Read(context.Background())
		if err != nil {
			t.Fatalf("read resp %s err: %v", req["method"], err)
		}
		var frame struct {
			Type   string         `json:"type"`
			ID     string         `json:"id"`
			OK     bool           `json:"ok"`
			Result map[string]any `json:"result"`
		}
		if err := json.Unmarshal(respBytes, &frame); err != nil {
			t.Fatalf("unmarshal %s resp: %v", req["method"], err)
		}
		if !frame.OK {
			t.Fatalf("%s response not ok: %s", req["method"], string(respBytes))
		}
		if req["method"] == "getPresence" {
			if frame.Result["chat_id"] != "sess-live" {
				t.Fatalf("expected sess-live chat_id, got %v", frame.Result["chat_id"])
			}
			if frame.Result["pending_count"] != float64(1) {
				t.Fatalf("expected pending_count=1, got %v", frame.Result["pending_count"])
			}
			run, ok := frame.Result["run"].(map[string]any)
			if !ok || run["run_id"] != "run-live" {
				t.Fatalf("expected run-live in presence, got %v", frame.Result["run"])
			}
		}
		if req["method"] == "ackNotification" && frame.Result["acknowledged"] != true {
			t.Fatalf("expected acknowledged=true, got %v", frame.Result["acknowledged"])
		}
	}
}

func TestGatewayPresenceFallsBackToNaviAccessors(t *testing.T) {
	srv, _, db := testServerWithNavi(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `INSERT INTO navi_chats (chat_id, title, status, visibility) VALUES ('sess-presence-fallback', 'Test', 'active', 'private')`); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	userEnvelope := presence.PresenceEnvelope{
		Version:             presence.Version,
		Source:              presence.AuthorityPet,
		SubjectType:         presence.SubjectUser,
		SubjectID:           "owner",
		Authority:           presence.AuthorityPet,
		TransportObservedAt: time.Date(2026, 4, 17, 15, 0, 0, 0, time.UTC),
		StateUpdatedAt:      time.Date(2026, 4, 17, 14, 59, 0, 0, time.UTC),
		StateRevision:       7,
		Visibility:          presence.VisibilityMixed,
		Payload: presence.UserPresencePayload{
			PublicStatus: presence.UserStatusBusy,
			StatusText:   "In review",
		},
	}

	res := doReq(t, srv, http.MethodPost, "/api/presence/user", "", "", "127.0.0.1:1234", userEnvelope)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/presence/user: expected 200, got %d", res.StatusCode)
	}

	res = doReq(t, srv, http.MethodGet, "/api/presence/navi", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/presence/navi: expected 200, got %d", res.StatusCode)
	}
	var naviEnvelope presence.PresenceEnvelope
	if err := json.NewDecoder(res.Body).Decode(&naviEnvelope); err != nil {
		t.Fatalf("decode navi envelope: %v", err)
	}
	if naviEnvelope.SubjectType != presence.SubjectNavi {
		t.Fatalf("expected navi subject type, got %#v", naviEnvelope)
	}

	res = doReq(t, srv, http.MethodGet, "/api/presence", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/presence: expected 200, got %d", res.StatusCode)
	}
	var snapshot presence.PresenceSnapshot
	if err := json.NewDecoder(res.Body).Decode(&snapshot); err != nil {
		t.Fatalf("decode presence snapshot: %v", err)
	}
	if snapshot.User == nil {
		t.Fatalf("expected user presence in snapshot, got %#v", snapshot)
	}
	if snapshot.User.SubjectType != presence.SubjectUser || snapshot.User.StateRevision != 7 {
		t.Fatalf("expected PET envelope to round-trip via NAVI presence service, got %#v", snapshot.User)
	}
}

func TestGatewayLiveWSEmitsPresenceSnapshotViaNaviAccessorFallback(t *testing.T) {
	srv, _, db := testServerWithNavi(t)
	if err := navistore.MigrateSchema(context.Background(), db); err != nil {
		t.Fatalf("migrate navi schema: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `INSERT INTO navi_chats (chat_id, title, status, visibility) VALUES ('sess-live-presence', 'Test', 'active', 'private')`); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws/live"
	c, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("dial err: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	connectMsg, _ := json.Marshal(map[string]any{
		"type":   "req",
		"id":     "connect",
		"method": "connect",
		"params": map[string]any{"chat_id": "sess-live-presence", "after_seq": 0, "stream": "user"},
	})
	if err := c.Write(context.Background(), websocket.MessageText, connectMsg); err != nil {
		t.Fatalf("write connect err: %v", err)
	}
	if _, _, err := c.Read(context.Background()); err != nil {
		t.Fatalf("read connect ack err: %v", err)
	}

	readCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, snapshotBytes, err := c.Read(readCtx)
	if err != nil {
		t.Fatalf("read presence snapshot err: %v", err)
	}
	var frame struct {
		Type string                    `json:"type"`
		Data presence.PresenceSnapshot `json:"data"`
	}
	if err := json.Unmarshal(snapshotBytes, &frame); err != nil {
		t.Fatalf("decode presence snapshot frame: %v", err)
	}
	if frame.Type != "presence.snapshot" {
		t.Fatalf("expected presence.snapshot frame, got %q", frame.Type)
	}
	if frame.Data.Navi == nil || frame.Data.Navi.SubjectType != presence.SubjectNavi {
		t.Fatalf("expected navi presence snapshot payload, got %#v", frame.Data)
	}
}

func TestGatewayLiveWSResolveBoundaryProposal(t *testing.T) {
	srv, _, db := testServer(t)
	if err := navistore.MigrateSchema(context.Background(), db); err != nil {
		t.Fatalf("migrate navi schema: %v", err)
	}
	ctx := context.Background()
	baseTime := time.Date(2026, 4, 1, 13, 0, 0, 0, time.UTC)

	if _, err := db.ExecContext(ctx, `INSERT INTO navi_chats (chat_id, title, status, visibility) VALUES ('sess-live-boundary', 'Test', 'active', 'private')`); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if err := store.SaveWorkspace(ctx, db, schema.Workspace{
		ID:             "ws-live",
		Name:           "Live",
		Kind:           schema.WorkspaceKindProject,
		Status:         schema.WorkspaceStatusActive,
		LocalRoots:     []string{"/workspace/live"},
		AllowedActions: schema.AllowedActions{Read: true, Write: true, Create: true, Modify: true, RenameMove: true},
		BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		AuditEnabled:   true,
		CreatedAt:      baseTime,
		UpdatedAt:      baseTime,
		CreatedBy:      "owner",
	}); err != nil {
		t.Fatalf("SaveWorkspace: %v", err)
	}
	if err := srv.cfg.WorldModel.SetActiveWorkspaceID(ctx, "ws-live"); err != nil {
		t.Fatalf("SetActiveWorkspaceID: %v", err)
	}

	proposedAction, _ := json.Marshal(map[string]any{
		"type":             "workspace_boundary_crossing",
		"chat_id":          "sess-live-boundary",
		"run_id":           "run-live-boundary",
		"workspace_id":     "ws-live",
		"target_path":      "/outside/logs/app.log",
		"action_types":     []string{"read"},
		"workspace_action": "read",
		"rule_scope":       "/outside/logs/**",
	})
	if err := store.SaveProposal(ctx, db, schema.Proposal{
		ProposalID:       "proposal-live-boundary",
		SourceProcess:    "validate_govern",
		SourceTrigger:    "Action targeting path outside workspace scope: /outside/logs/app.log",
		ProposedAction:   string(proposedAction),
		AffectedEntities: `["session:sess-live-boundary"]`,
		Rationale:        "boundary crossing",
		Priority:         schema.ProposalPriorityBlocking,
		Status:           schema.ProposalStatusPending,
		CreatedAt:        baseTime,
	}); err != nil {
		t.Fatalf("SaveProposal: %v", err)
	}
	if err := store.SaveExecutionOutcome(ctx, db, schema.ExecutionOutcome{
		AttemptID:          "attempt-live-boundary",
		CommandID:          "command-live-boundary",
		AttemptNumber:      1,
		CommandType:        schema.CommandTypeUpdate,
		StartTime:          baseTime,
		Outcome:            schema.ExecutionOutcomeRejectedPreExecution,
		AffectedEntities:   `["session:sess-live-boundary"]`,
		CompensationStatus: schema.CompensationStatusNotRequired,
		RecoveryStatus:     schema.RecoveryStatusNotRequired,
		ProposalID:         "proposal-live-boundary",
		RunID:              "run-live-boundary",
		RuntimeSessionID:   "sess-live-boundary",
		ApprovalRequired:   true,
		ApprovalOutcome:    schema.ApprovalOutcomeNA,
		WorkspaceID:        "ws-live",
		BoundaryCrossing:   true,
	}); err != nil {
		t.Fatalf("SaveExecutionOutcome: %v", err)
	}

	sessionStore := navistore.NewSQLiteStore(db)
	run := &naviruntime.RunState{
		RunID:               "run-live-boundary",
		RuntimeSessionID:    "sess-live-boundary",
		ChatID:              "sess-live-boundary",
		ExperienceMode:      "navi",
		Mode:                naviruntime.RunModeForeground,
		Status:              schema.RunStatusPaused,
		CurrentPhase:        naviruntime.RunPhaseValidateGovern,
		BlockedOnProposalID: "proposal-live-boundary",
		StartedAt:           baseTime,
		UpdatedAt:           baseTime,
	}
	if err := sessionStore.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	naviAgent, err := navi.New(navi.Config{
		DB:              db,
		Chats:           sessionStore,
		RuntimeSessions: sessionStore,
		WorldModel:      srv.cfg.WorldModel,
		GetProposal: func(ctx context.Context, proposalID string) (schema.Proposal, error) {
			return store.GetProposal(ctx, db, proposalID)
		},
		ResolveProposal: func(ctx context.Context, proposalID string, status schema.ProposalStatus, resolutionType schema.ResolutionType, resolvedBy, resolutionNote string) error {
			return store.ResolveProposal(ctx, db, proposalID, status, resolutionType, resolvedBy, resolutionNote)
		},
	})
	if err != nil {
		t.Fatalf("new navi: %v", err)
	}
	srv.cfg.Navi = naviAgent

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws/live"
	c, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("dial err: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	connectMsg, _ := json.Marshal(map[string]any{
		"type":   "req",
		"id":     "connect",
		"method": "connect",
		"params": map[string]any{"chat_id": "sess-live-boundary", "after_seq": 0, "stream": "user"},
	})
	if err := c.Write(context.Background(), websocket.MessageText, connectMsg); err != nil {
		t.Fatalf("write connect err: %v", err)
	}
	if _, _, err := c.Read(context.Background()); err != nil {
		t.Fatalf("read connect ack err: %v", err)
	}

	req, _ := json.Marshal(map[string]any{
		"type":   "req",
		"id":     "resolve-boundary",
		"method": "resolveProposal",
		"params": map[string]any{
			"proposal_id": "proposal-live-boundary",
			"action":      "always_allow",
			"note":        "allow this path",
		},
	})
	if err := c.Write(context.Background(), websocket.MessageText, req); err != nil {
		t.Fatalf("write resolve err: %v", err)
	}
	respBytes := readLiveResponseFrame(t, c)
	var frame struct {
		OK     bool           `json:"ok"`
		Result map[string]any `json:"result"`
		Error  string         `json:"error"`
	}
	if err := json.Unmarshal(respBytes, &frame); err != nil {
		t.Fatalf("unmarshal resolve resp: %v", err)
	}
	if !frame.OK {
		t.Fatalf("resolveProposal response not ok: %s", string(respBytes))
	}
	if frame.Result["approval_outcome"] != "always_allow" {
		t.Fatalf("expected always_allow approval outcome, got %v", frame.Result["approval_outcome"])
	}

	proposal, err := store.GetProposal(ctx, db, "proposal-live-boundary")
	if err != nil {
		t.Fatalf("GetProposal: %v", err)
	}
	if proposal.Status != schema.ProposalStatusApproved || proposal.ResolutionType != schema.ResolutionTypeApprovedAlways {
		t.Fatalf("expected approved always proposal, got %+v", proposal)
	}
	rules, err := store.ListWhitelistRules(ctx, db, "ws-live")
	if err != nil {
		t.Fatalf("ListWhitelistRules: %v", err)
	}
	if len(rules) != 1 || rules[0].Scope != "/outside/logs/**" {
		t.Fatalf("expected persisted whitelist rule, got %+v", rules)
	}
	outcome, err := store.GetExecutionOutcomeByAttemptID(ctx, db, "attempt-live-boundary")
	if err != nil {
		t.Fatalf("GetExecutionOutcomeByAttemptID: %v", err)
	}
	if outcome == nil || outcome.ApprovalOutcome != schema.ApprovalOutcomeAlwaysAllow {
		t.Fatalf("expected always_allow execution outcome, got %+v", outcome)
	}
}

func TestNaviSendMessageTriggersConnectorThinking(t *testing.T) {
	db := testDB(t)
	if err := navistore.MigrateSchema(context.Background(), db); err != nil {
		t.Fatalf("migrate navi schema: %v", err)
	}
	b := bus.NewMemBus(db)
	sessionStore := navistore.NewSQLiteStore(db)
	naviAgent, err := navi.New(navi.Config{
		DB:              db,
		Bus:             b,
		Chats:           sessionStore,
		RuntimeSessions: sessionStore,
	})
	if err != nil {
		t.Fatalf("new navi: %v", err)
	}
	chat, err := sessionStore.CreateChat(context.Background(), navi.CreateChatInput{
		ExperienceMode: string(navi.ExperienceModeStandard),
		Title:          "Connector Chat",
	})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	chatID := string(chat.ID)

	registry := NewConnectorRegistry()
	conn := newTypingTestConnector("telegram-test")
	registry.RegisterFactory("telegram-test", func(_ *config.Config, _ bus.Bus) (pkgconn.Connector, error) {
		return conn, nil
	})
	if err := registry.Create("telegram-test", &config.Config{}, b, nil); err != nil {
		t.Fatalf("create connector: %v", err)
	}
	manager := intconnectors.NewManager(registry, intconnectors.ManagerConfig{
		RateLimits: map[string]float64{"telegram-test": 1000},
	})
	manager.StartAll(context.Background())
	defer manager.StopAll(context.Background(), 2*time.Second)

	srv := NewServer(Config{
		DB:              db,
		DirectiveWriter: cognitive.StoreDirectiveWriter(db),
		Bus:             b,
		Governor:        governor.NewGovernor(governor.GovernorConfig{}, "."),
		Registry:        registry,
		Manager:         manager,
		Navi:            naviAgent,
		Addr:            ":0",
	})

	res := doReq(t, srv, "POST", "/api/navi/chats/"+chatID+"/message", "", "", "127.0.0.1:1234", map[string]any{
		"content":            "hello from telegram",
		"source_channel":     "telegram-test",
		"source_message_ref": "123:0:456",
	})
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", res.StatusCode)
	}
	if got := conn.typingCalls.Load(); got != 0 {
		t.Fatalf("expected no typing call when governance blocks messaging, got %d", got)
	}
}

func TestNaviSendMessageReturnsDeferredQueueStateForPausedRun(t *testing.T) {
	db := testDB(t)
	if err := navistore.MigrateSchema(context.Background(), db); err != nil {
		t.Fatalf("migrate navi schema: %v", err)
	}
	b := bus.NewMemBus(db)
	sessionStore := navistore.NewSQLiteStore(db)
	naviAgent, err := navi.New(navi.Config{
		DB:              db,
		Bus:             b,
		Chats:           sessionStore,
		RuntimeSessions: sessionStore,
	})
	if err != nil {
		t.Fatalf("new navi: %v", err)
	}

	chat, err := sessionStore.CreateChat(context.Background(), navi.CreateChatInput{
		ExperienceMode: string(navi.ExperienceModeStandard),
		Title:          "Paused Chat",
	})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	chatID := string(chat.ID)
	run := naviruntime.NewRun("runtime-"+chatID, string(navi.ExperienceModeStandard))
	run.ChatID = chatID
	run.SetStatus(schema.RunStatusWaitingForProposal)
	run.BlockedOnProposalID = "prop-123"
	run.PauseReason = "approval required"
	if err := sessionStore.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	srv := NewServer(Config{
		DB:              db,
		DirectiveWriter: cognitive.StoreDirectiveWriter(db),
		Bus:             b,
		Governor:        governor.NewGovernor(governor.GovernorConfig{}, "."),
		Navi:            naviAgent,
		Addr:            ":0",
	})

	res := doReq(t, srv, http.MethodPost, "/api/navi/chats/"+chatID+"/message", "", "", "127.0.0.1:1234", map[string]any{
		"content": "hello",
	})
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", res.StatusCode)
	}
}

func TestNaviSendMessageUsesIngressTimeoutBeforeQueueing(t *testing.T) {
	srv, naviAgent, db := testServerWithNavi(t)
	chatID, err := naviAgent.CreateChat(context.Background(), navi.ExperienceModeStandard, "")
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}

	previousTimeout := naviMessageIngressTimeout
	naviMessageIngressTimeout = 25 * time.Millisecond
	defer func() { naviMessageIngressTimeout = previousTimeout }()

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	heldConn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("reserve db conn: %v", err)
	}
	defer heldConn.Close()

	started := time.Now()
	res := doReq(t, srv, http.MethodPost, "/api/navi/chats/"+chatID+"/message", "", "", "127.0.0.1:1234", map[string]any{
		"content": "hello from telegram",
	})
	elapsed := time.Since(started)
	if res.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d", res.StatusCode)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("expected ingress timeout to fail fast, took %s", elapsed)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), context.DeadlineExceeded.Error()) {
		t.Fatalf("expected deadline exceeded body, got %q", string(body))
	}
}

func TestNaviArchiveChatMarksChatArchived(t *testing.T) {
	db := testDB(t)
	if err := navistore.MigrateSchema(context.Background(), db); err != nil {
		t.Fatalf("migrate navi schema: %v", err)
	}
	b := bus.NewMemBus(db)
	sessionStore := navistore.NewSQLiteStore(db)
	naviAgent, err := navi.New(navi.Config{
		DB:              db,
		Bus:             b,
		Chats:           sessionStore,
		RuntimeSessions: sessionStore,
	})
	if err != nil {
		t.Fatalf("new navi: %v", err)
	}

	chatID, err := naviAgent.CreateChat(context.Background(), navi.ExperienceModeStandard, "")
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

	res := doReq(t, srv, "POST", "/api/navi/chats/"+chatID+"/archive", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}

	chat, err := sessionStore.GetChat(context.Background(), chatID)
	if err != nil {
		t.Fatalf("GetChat: %v", err)
	}
	if chat == nil || chat.ArchivedAt == nil || chat.Status != navi.ChatStatusArchived {
		t.Fatalf("expected chat to be archived, got %#v", chat)
	}
}

func TestNaviDeleteChatMarksChatDeleted(t *testing.T) {
	db := testDB(t)
	if err := navistore.MigrateSchema(context.Background(), db); err != nil {
		t.Fatalf("migrate navi schema: %v", err)
	}
	b := bus.NewMemBus(db)
	sessionStore := navistore.NewSQLiteStore(db)
	naviAgent, err := navi.New(navi.Config{
		DB:              db,
		Bus:             b,
		Chats:           sessionStore,
		RuntimeSessions: sessionStore,
	})
	if err != nil {
		t.Fatalf("new navi: %v", err)
	}

	chatID, err := naviAgent.CreateChat(context.Background(), navi.ExperienceModeStandard, "")
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

	res := doReq(t, srv, "DELETE", "/api/navi/chats/"+chatID, "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}

	chat, err := sessionStore.GetChat(context.Background(), chatID)
	if err != nil {
		t.Fatalf("GetChat: %v", err)
	}
	if chat == nil || chat.DeletedAt == nil || chat.Status != navi.ChatStatusDeleted {
		t.Fatalf("expected chat to be deleted, got %#v", chat)
	}
}

func TestNaviContinueRouteQueuesContinuation(t *testing.T) {
	db := testDB(t)
	if err := navistore.MigrateSchema(context.Background(), db); err != nil {
		t.Fatalf("migrate navi schema: %v", err)
	}
	b := bus.NewMemBus(db)
	sessionStore := navistore.NewSQLiteStore(db)
	naviAgent, err := navi.New(navi.Config{
		DB:              db,
		Bus:             b,
		Chats:           sessionStore,
		RuntimeSessions: sessionStore,
	})
	if err != nil {
		t.Fatalf("new navi: %v", err)
	}
	chatID, err := naviAgent.CreateChat(context.Background(), navi.ExperienceModeStandard, "")
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	if _, err := naviAgent.SendMessageInput(context.Background(), chatID, naviruntime.MessageInput{Content: "go on", SourceChannel: "app"}); err != nil {
		t.Fatalf("SendMessageInput: %v", err)
	}
	if _, err := sessionStore.AppendSystemAssistantMessage(context.Background(), chatID, "first part", string(navi.ExperienceModeStandard), "system", "reply"); err != nil {
		t.Fatalf("AppendSystemAssistantMessage: %v", err)
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

	res := doReq(t, srv, "POST", "/api/navi/chats/"+chatID+"/continue", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
}

func TestNaviMessageVariantsRoutesListAndSelect(t *testing.T) {
	db := testDB(t)
	if err := navistore.MigrateSchema(context.Background(), db); err != nil {
		t.Fatalf("migrate navi schema: %v", err)
	}
	b := bus.NewMemBus(db)
	sessionStore := navistore.NewSQLiteStore(db)
	naviAgent, err := navi.New(navi.Config{
		DB:              db,
		Bus:             b,
		Chats:           sessionStore,
		RuntimeSessions: sessionStore,
	})
	if err != nil {
		t.Fatalf("new navi: %v", err)
	}
	chatID, err := naviAgent.CreateChat(context.Background(), navi.ExperienceModeStandard, "")
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	msgID, err := sessionStore.AppendSystemAssistantMessage(context.Background(), chatID, "first variant", string(navi.ExperienceModeStandard), "system", "reply")
	if err != nil {
		t.Fatalf("AppendSystemAssistantMessage: %v", err)
	}
	if _, err := sessionStore.AppendMessageVariant(context.Background(), chatID, msgID, "second variant"); err != nil {
		t.Fatalf("AppendMessageVariant: %v", err)
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

	res := doReq(t, srv, "GET", "/api/navi/chats/"+chatID+"/messages/"+msgID+"/variants", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected list 200, got %d", res.StatusCode)
	}
	var listed navi.MessageVariants
	if err := json.NewDecoder(res.Body).Decode(&listed); err != nil {
		t.Fatalf("decode variants: %v", err)
	}
	if len(listed.Variants) != 2 || listed.SelectedIndex != 1 {
		t.Fatalf("unexpected variants response: %#v", listed)
	}

	res = doReq(t, srv, "POST", "/api/navi/chats/"+chatID+"/messages/"+msgID+"/variants/select", "", "", "127.0.0.1:1234", map[string]int{"index": 0})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected select 200, got %d", res.StatusCode)
	}
	thread, err := sessionStore.GetChatWithMessages(context.Background(), chatID)
	if err != nil {
		t.Fatalf("GetChatWithMessages: %v", err)
	}
	if got := thread.Messages[0].Content; got != "first variant" {
		t.Fatalf("visible content = %q, want first variant", got)
	}
}

func TestGapScaffoldRouteAcceptsPathIDWithoutBody(t *testing.T) {
	db := testDB(t)
	b := bus.NewMemBus(db)
	srv := NewServer(Config{
		DB:              db,
		DirectiveWriter: cognitive.StoreDirectiveWriter(db),
		Bus:             b,
		Governor:        governor.NewGovernor(governor.GovernorConfig{}, "."),
		Registry:        NewConnectorRegistry(),
		Addr:            ":0",
		Navi:            &navi.NAVI{},
	})

	res := doJSONReq(t, srv, "POST", "/api/gaps/gap-123/scaffold", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500 from empty stub NAVI, got %d", res.StatusCode)
	}
}

func TestRemovedClaimAliasReturnsNotFound(t *testing.T) {
	srv, _, _ := testServer(t)
	res := doReq(t, srv, "POST", "/api/claim", "", "", "127.0.0.1:1234", map[string]any{})
	if res.StatusCode != http.StatusNotFound && res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/claim: expected 404 or 405, got %d", res.StatusCode)
	}
}

func TestInstanceReset(t *testing.T) {
	srv, _, _ := testServer(t)
	passport := claimTestInstance(t, srv, "ResetOwner", "owner-secret-123")
	apiKey, _ := passport["api_key"].(string)
	if apiKey == "" {
		t.Fatalf("claim did not return api_key")
	}
	res := doJSONReq(t, srv, "GET", "/api/onboarding/status", apiKey, "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/onboarding/status after recovery: expected 200, got %d", res.StatusCode)
	}
	var status map[string]any
	if err := json.NewDecoder(res.Body).Decode(&status); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if status["claimed"] != true {
		t.Fatalf("expected claimed=true after claim, got %v", status["claimed"])
	}

	res = doReq(t, srv, "POST", "/api/instance/reset", apiKey, "owner-secret-123", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/instance/reset: expected 200, got %d", res.StatusCode)
	}
	var resetBody map[string]any
	if err := json.NewDecoder(res.Body).Decode(&resetBody); err != nil {
		t.Fatalf("decode reset response: %v", err)
	}
	if resetBody["ok"] != true {
		t.Fatalf("expected ok=true, got %v", resetBody["ok"])
	}

	res = doJSONReq(t, srv, "GET", "/api/onboarding/status", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/onboarding/status after reset: expected 200, got %d", res.StatusCode)
	}
	if err := json.NewDecoder(res.Body).Decode(&status); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if status["claimed"] != false {
		t.Fatalf("expected claimed=false after reset, got %v", status["claimed"])
	}
}

func TestInstanceResetRequiresOwnerSecretOnLoopback(t *testing.T) {
	srv, _, _ := testServer(t)
	passport := claimTestInstance(t, srv, "ResetOwner", "owner-secret-123")
	apiKey, _ := passport["api_key"].(string)
	if apiKey == "" {
		t.Fatalf("claim did not return api_key")
	}

	res := doReq(t, srv, "POST", "/api/instance/reset", apiKey, "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("POST /api/instance/reset without owner secret: expected 401, got %d", res.StatusCode)
	}
}

func TestAPIHealth(t *testing.T) {
	srv, _, _ := testServer(t)
	for _, path := range []string{"/health", "/api/health"} {
		res := doJSONReq(t, srv, "GET", path, "", "", nil)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: expected 200, got %d", path, res.StatusCode)
		}
		var data map[string]any
		if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if data["status"] != "ok" {
			t.Fatalf("GET %s: expected status=ok, got %v", path, data["status"])
		}
		if _, ok := data["version"]; !ok {
			t.Fatalf("GET %s: missing version", path)
		}
	}
}

func TestGatewayErrorsEndpoints(t *testing.T) {
	srv, _, db := testServer(t)
	ctx := context.Background()
	if err := store.SaveErrorRecord(ctx, db, store.ErrorRecord{
		Component:    "runtime",
		ChatID:       "sess-errors",
		ErrorType:    "llm_timeout",
		ErrorMessage: "deadline exceeded",
	}); err != nil {
		t.Fatalf("SaveErrorRecord: %v", err)
	}
	if err := store.SaveErrorRecord(ctx, db, store.ErrorRecord{
		Component:    "connector",
		ErrorType:    "connector_send_failed",
		ErrorMessage: "send failed",
	}); err != nil {
		t.Fatalf("SaveErrorRecord: %v", err)
	}

	res := doJSONReq(t, srv, "GET", "/api/errors?chat_id=sess-errors", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/errors: expected 200, got %d", res.StatusCode)
	}
	var list map[string]any
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatalf("decode errors list: %v", err)
	}
	items, ok := list["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected 1 filtered error item, got %v", list["items"])
	}

	res = doJSONReq(t, srv, "GET", "/api/errors/summary?window=24h", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/errors/summary: expected 200, got %d", res.StatusCode)
	}
	var summary map[string]any
	if err := json.NewDecoder(res.Body).Decode(&summary); err != nil {
		t.Fatalf("decode errors summary: %v", err)
	}
	summaryItems, ok := summary["items"].([]any)
	if !ok || len(summaryItems) < 2 {
		t.Fatalf("expected summary items, got %v", summary["items"])
	}
}

func TestGatewayArtifactsEndpoints(t *testing.T) {
	srv, _, db := testServer(t)
	passport := claimTestInstance(t, srv, "Owner", "owner-secret")
	apiKey, _ := passport["primary_api_key"].(string)
	ownerID, err := store.GetOwnerID(context.Background(), db)
	if err != nil {
		t.Fatalf("GetOwnerID: %v", err)
	}
	wm := worldmodel.New(db)
	materialized, err := wm.MaterializeArtifact(context.Background(), worldmodel.ArtifactMaterializationRequest{
		ArtifactID:        "artifact:test:gateway",
		OwnerID:           ownerID,
		Kind:              "document",
		Location:          "artifact://gateway",
		Description:       "Gateway artifact",
		Status:            worldmodel.ArtifactStatusInProgress,
		SourceTool:        "test-skill",
		SourceExecutionID: "artifact-exec:1",
		Snapshot:          map[string]any{"step": "draft"},
	})
	if err != nil {
		t.Fatalf("MaterializeArtifact: %v", err)
	}
	if err := store.SaveExecutionOutcome(context.Background(), db, schema.ExecutionOutcome{
		AttemptID:          "artifact-exec:1",
		CommandID:          "artifact-exec",
		AttemptNumber:      1,
		CommandType:        schema.CommandTypeCreate,
		StartTime:          time.Now().UTC(),
		Outcome:            schema.ExecutionOutcomeSucceeded,
		AffectedEntities:   `[{"kind":"artifact","id":"artifact:test:gateway"}]`,
		CompensationStatus: schema.CompensationStatusNotRequired,
		RecoveryStatus:     schema.RecoveryStatusNotRequired,
	}); err != nil {
		t.Fatalf("SaveExecutionOutcome: %v", err)
	}

	res := doJSONReq(t, srv, "GET", "/api/artifacts", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/artifacts: expected 200, got %d", res.StatusCode)
	}
	var list map[string]any
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	items, ok := list["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected 1 artifact item, got %v", list["items"])
	}

	res = doJSONReq(t, srv, "GET", "/api/artifacts/"+materialized.Artifact.ID, apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/artifacts/{id}: expected 200, got %d", res.StatusCode)
	}
	var detail map[string]any
	if err := json.NewDecoder(res.Body).Decode(&detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail["status"] != worldmodel.ArtifactStatusInProgress {
		t.Fatalf("expected in_progress status, got %v", detail["status"])
	}
	versions, ok := detail["versions"].([]any)
	if !ok || len(versions) != 1 {
		t.Fatalf("expected one artifact version, got %v", detail["versions"])
	}
	history, ok := detail["history"].(map[string]any)
	if !ok || history["run_id"] != "artifact-exec:1" {
		t.Fatalf("expected linked history record, got %v", detail["history"])
	}
	if detail["renderer"] == nil || detail["editor"] == nil {
		t.Fatalf("expected renderer/editor metadata, got %v", detail)
	}
}

func TestGatewayArtifactDetailEmitsWorkspaceLoadedEvent(t *testing.T) {
	srv, _, db := testServer(t)
	passport := claimTestInstance(t, srv, "Owner", "owner-secret")
	apiKey, _ := passport["primary_api_key"].(string)
	ownerID, err := store.GetOwnerID(context.Background(), db)
	if err != nil {
		t.Fatalf("GetOwnerID: %v", err)
	}
	mustSaveGatewayProject(t, db, "proj-gateway")

	created, err := srv.cfg.ArtifactService.CreateArtifact(context.Background(), schema.ArtifactOperationEnvelope{
		Operation:       schema.ArtifactOpCreate,
		OperationID:     "gateway-observe-1",
		WorkspaceID:     "ws-default",
		ProjectID:       "proj-gateway",
		OwnerID:         ownerID,
		Title:           "Observed Artifact",
		ArtifactType:    schema.ArtifactTypeDocument,
		ArtifactSubtype: "markdown",
		ContentFormat:   "text/markdown",
		Payload:         "# Hello\n",
		ActorType:       schema.ActorAgent,
		ActorID:         "navi",
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}

	res := doJSONReq(t, srv, "GET", "/api/artifacts/"+created.ID, apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/artifacts/{id}: expected 200, got %d", res.StatusCode)
	}

	events, err := store.EventsSince(context.Background(), db, 0, 200)
	if err != nil {
		t.Fatalf("EventsSince: %v", err)
	}
	var found bool
	for _, ev := range events {
		if ev.Type != schema.FactArtifactWorkspaceLoaded {
			continue
		}
		payload, ok := ev.Payload.(map[string]any)
		if !ok {
			continue
		}
		if payload["artifact_id"] == created.ID && payload["operation"] == "workspace_load" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected artifact.workspace.loaded event for %s, got %+v", created.ID, events)
	}
}

func TestGatewayLiveWSArtifactWorkspaceMethods(t *testing.T) {
	srv, _, db := testServer(t)
	ownerID, err := store.GetOwnerID(context.Background(), db)
	if err != nil {
		t.Fatalf("GetOwnerID: %v", err)
	}
	mustSaveGatewayProject(t, db, "proj-live")
	if srv.cfg.ArtifactService == nil {
		t.Fatal("expected artifact service on server")
	}
	created, err := srv.cfg.ArtifactService.CreateArtifact(context.Background(), schema.ArtifactOperationEnvelope{
		Operation:            schema.ArtifactOpCreate,
		OperationID:          "live-artifact-op-1",
		WorkspaceID:          "ws-default",
		ProjectID:            "proj-live",
		OwnerID:              ownerID,
		Title:                "Live Artifact",
		ArtifactType:         schema.ArtifactTypeDocument,
		ArtifactSubtype:      "markdown",
		ContentFormat:        "text/markdown",
		Payload:              "# First\n",
		ActorType:            schema.ActorAgent,
		ActorID:              "navi",
		SourceConversationID: "sess-live-artifact",
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	version, err := srv.cfg.ArtifactService.UpdateContent(context.Background(), schema.ArtifactOperationEnvelope{
		Operation:             schema.ArtifactOpPatch,
		OperationID:           "live-artifact-op-2",
		TargetArtifactID:      created.ID,
		ExpectedBaseVersionID: created.CurrentVersionID,
		Payload:               "# Second\n",
		ActorType:             schema.ActorAgent,
		ActorID:               "navi",
		SourceConversationID:  "sess-live-artifact",
	})
	if err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws/live"
	c, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("dial err: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	connectMsg, _ := json.Marshal(map[string]any{
		"type":   "req",
		"id":     "connect",
		"method": "connect",
		"params": map[string]any{"chat_id": "sess-live-artifact", "after_seq": 0, "stream": "user"},
	})
	if err := c.Write(context.Background(), websocket.MessageText, connectMsg); err != nil {
		t.Fatalf("write connect err: %v", err)
	}
	if _, _, err := c.Read(context.Background()); err != nil {
		t.Fatalf("read connect ack err: %v", err)
	}

	for _, req := range []map[string]any{
		{"type": "req", "id": "artifacts", "method": "getArtifacts", "params": map[string]any{"view": "current_conversation"}},
		{"type": "req", "id": "artifact", "method": "getArtifact", "params": map[string]any{"artifact_id": created.ID}},
		{"type": "req", "id": "content", "method": "getArtifactContent", "params": map[string]any{"artifact_id": created.ID}},
		{"type": "req", "id": "diff", "method": "getArtifactDiff", "params": map[string]any{"artifact_id": created.ID, "version_id": version.ID}},
	} {
		b, _ := json.Marshal(req)
		if err := c.Write(context.Background(), websocket.MessageText, b); err != nil {
			t.Fatalf("write req %s err: %v", req["method"], err)
		}
		_, respBytes, err := c.Read(context.Background())
		if err != nil {
			t.Fatalf("read resp %s err: %v", req["method"], err)
		}
		var frame struct {
			Type   string `json:"type"`
			ID     string `json:"id"`
			OK     bool   `json:"ok"`
			Result any    `json:"result"`
		}
		if err := json.Unmarshal(respBytes, &frame); err != nil {
			t.Fatalf("unmarshal %s resp: %v", req["method"], err)
		}
		if !frame.OK {
			t.Fatalf("%s response not ok: %s", req["method"], string(respBytes))
		}
		switch req["method"] {
		case "getArtifacts":
			items, ok := frame.Result.([]any)
			if !ok || len(items) != 1 {
				t.Fatalf("expected one artifact, got %v", frame.Result)
			}
			item, ok := items[0].(map[string]any)
			if !ok || item["sync_state"] == nil || item["last_editor"] == nil {
				t.Fatalf("expected enriched artifact list item, got %v", frame.Result)
			}
		case "getArtifact":
			item, ok := frame.Result.(map[string]any)
			if !ok || item["id"] != created.ID {
				t.Fatalf("unexpected artifact detail %v", frame.Result)
			}
		case "getArtifactContent":
			item, ok := frame.Result.(map[string]any)
			if !ok || !strings.Contains(item["content"].(string), "# Second") {
				t.Fatalf("unexpected artifact content %v", frame.Result)
			}
		case "getArtifactDiff":
			item, ok := frame.Result.(map[string]any)
			if !ok {
				t.Fatalf("unexpected artifact diff %v", frame.Result)
			}
			diff, ok := item["diff"].(map[string]any)
			if !ok || diff["kind"] != "line" {
				t.Fatalf("unexpected artifact diff payload %v", frame.Result)
			}
		}
	}
}

func TestGatewayLiveWSArtifactLibraryViews(t *testing.T) {
	srv, _, db := testServer(t)
	ownerID, err := store.GetOwnerID(context.Background(), db)
	if err != nil {
		t.Fatalf("GetOwnerID: %v", err)
	}
	mustSaveGatewayWorkspace(t, db, "ws-alpha")
	mustSaveGatewayWorkspace(t, db, "ws-beta")
	mustSaveGatewayProject(t, db, "proj-alpha")
	mustSaveGatewayProject(t, db, "proj-beta")
	if err := store.BindProjectWorkspace(context.Background(), db, "proj-alpha", "ws-alpha", time.Date(2026, 4, 2, 10, 1, 0, 0, time.UTC)); err != nil {
		t.Fatalf("BindProjectWorkspace(proj-alpha): %v", err)
	}
	if err := store.BindProjectWorkspace(context.Background(), db, "proj-beta", "ws-beta", time.Date(2026, 4, 2, 10, 2, 0, 0, time.UTC)); err != nil {
		t.Fatalf("BindProjectWorkspace(proj-beta): %v", err)
	}
	svc := srv.cfg.ArtifactService
	if svc == nil {
		t.Fatal("expected artifact service on server")
	}

	currentConversation, err := svc.CreateArtifact(context.Background(), schema.ArtifactOperationEnvelope{
		Operation:            schema.ArtifactOpCreate,
		OperationID:          "view-op-1",
		WorkspaceID:          "ws-alpha",
		ProjectID:            "proj-alpha",
		OwnerID:              ownerID,
		Title:                "Conversation Draft",
		ArtifactType:         schema.ArtifactTypeDocument,
		ArtifactSubtype:      "markdown",
		ContentFormat:        "text/markdown",
		Payload:              "# Draft\n",
		ActorType:            schema.ActorAgent,
		ActorID:              "navi",
		SourceConversationID: "sess-library",
	})
	if err != nil {
		t.Fatalf("CreateArtifact currentConversation: %v", err)
	}
	inProgress, err := svc.CreateArtifact(context.Background(), schema.ArtifactOperationEnvelope{
		Operation:       schema.ArtifactOpCreate,
		OperationID:     "view-op-2",
		WorkspaceID:     "ws-alpha",
		ProjectID:       "proj-alpha",
		OwnerID:         ownerID,
		Title:           "In Progress Draft",
		ArtifactType:    schema.ArtifactTypeCode,
		ArtifactSubtype: "go",
		ContentFormat:   "text/plain",
		Payload:         "package main\n",
		ActorType:       schema.ActorAgent,
		ActorID:         "navi",
		Attributes: map[string]any{
			"pending_execution_state": "running",
		},
	})
	if err != nil {
		t.Fatalf("CreateArtifact inProgress: %v", err)
	}
	if _, err := svc.TransitionLifecycle(context.Background(), schema.ArtifactOperationEnvelope{
		TargetArtifactID: inProgress.ID,
		ToLifecycleState: schema.ArtifactLifecycleInProgress,
	}); err != nil {
		t.Fatalf("TransitionLifecycle inProgress: %v", err)
	}
	archived, err := svc.CreateArtifact(context.Background(), schema.ArtifactOperationEnvelope{
		Operation:       schema.ArtifactOpCreate,
		OperationID:     "view-op-3",
		WorkspaceID:     "ws-alpha",
		ProjectID:       "proj-alpha",
		OwnerID:         ownerID,
		Title:           "Archived Draft",
		ArtifactType:    schema.ArtifactTypeData,
		ArtifactSubtype: "json",
		ContentFormat:   "application/json",
		Payload:         map[string]any{"ok": true},
		ActorType:       schema.ActorAgent,
		ActorID:         "navi",
	})
	if err != nil {
		t.Fatalf("CreateArtifact archived: %v", err)
	}
	if err := svc.ArchiveArtifact(context.Background(), schema.ArtifactOperationEnvelope{TargetArtifactID: archived.ID}); err != nil {
		t.Fatalf("ArchiveArtifact: %v", err)
	}
	sharedExported, err := svc.CreateArtifact(context.Background(), schema.ArtifactOperationEnvelope{
		Operation:       schema.ArtifactOpCreate,
		OperationID:     "view-op-4",
		WorkspaceID:     "ws-beta",
		ProjectID:       "proj-beta",
		OwnerID:         ownerID,
		Title:           "Shared Export",
		ArtifactType:    schema.ArtifactTypeDocument,
		ArtifactSubtype: "markdown",
		ContentFormat:   "text/markdown",
		Payload:         "# Shared\n",
		ActorType:       schema.ActorAgent,
		ActorID:         "navi",
		Attributes: map[string]any{
			"sync_state":  "exported",
			"shared_at":   time.Now().UTC().Format(time.RFC3339),
			"share_url":   "https://example.test/shared",
			"exported_at": time.Now().UTC().Format(time.RFC3339),
		},
	})
	if err != nil {
		t.Fatalf("CreateArtifact sharedExported: %v", err)
	}
	if _, err := svc.TransitionLifecycle(context.Background(), schema.ArtifactOperationEnvelope{
		TargetArtifactID: sharedExported.ID,
		ToLifecycleState: schema.ArtifactLifecyclePublished,
	}); err != nil {
		t.Fatalf("TransitionLifecycle sharedExported: %v", err)
	}

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws/live"
	c, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("dial err: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	connectMsg, _ := json.Marshal(map[string]any{
		"type":   "req",
		"id":     "connect",
		"method": "connect",
		"params": map[string]any{"chat_id": "sess-library", "after_seq": 0, "stream": "user"},
	})
	if err := c.Write(context.Background(), websocket.MessageText, connectMsg); err != nil {
		t.Fatalf("write connect err: %v", err)
	}
	if _, _, err := c.Read(context.Background()); err != nil {
		t.Fatalf("read connect ack err: %v", err)
	}

	assertViewCount := func(view string, expected int) []any {
		t.Helper()
		req, _ := json.Marshal(map[string]any{
			"type":   "req",
			"id":     "view-" + view,
			"method": "getArtifacts",
			"params": map[string]any{"view": view},
		})
		if err := c.Write(context.Background(), websocket.MessageText, req); err != nil {
			t.Fatalf("write view %s err: %v", view, err)
		}
		_, respBytes, err := c.Read(context.Background())
		if err != nil {
			t.Fatalf("read view %s err: %v", view, err)
		}
		var frame struct {
			OK     bool  `json:"ok"`
			Result []any `json:"result"`
		}
		if err := json.Unmarshal(respBytes, &frame); err != nil {
			t.Fatalf("unmarshal view %s resp: %v", view, err)
		}
		if !frame.OK || len(frame.Result) != expected {
			t.Fatalf("view %s expected %d items, got %v", view, expected, string(respBytes))
		}
		return frame.Result
	}

	assertViewCount("current_conversation", 1)
	assertViewCount("current_project", 3)
	currentWorkspaceItems := assertViewCount("current_workspace", 3)
	assertViewCount("drafts", 2)
	assertViewCount("archived", 1)
	sharedItems := assertViewCount("shared_exported", 1)

	workspaceItem, ok := currentWorkspaceItems[1].(map[string]any)
	if !ok || workspaceItem["sync_state"] == nil || workspaceItem["pending_execution_state"] == nil {
		t.Fatalf("expected enriched workspace item metadata, got %v", currentWorkspaceItems)
	}
	sharedItem, ok := sharedItems[0].(map[string]any)
	if !ok || sharedItem["shared_exported"] != true || sharedItem["sync_state"] != "exported" {
		t.Fatalf("expected shared/exported metadata, got %v", sharedItems)
	}
	if workspaceItem["project_id"] != currentConversation.ProjectID {
		t.Fatalf("expected current workspace item project to match current context, got %v", workspaceItem["project_id"])
	}
}

func TestGatewayArtifactsEndpointsExposeFirstClassArtifactMetadataAndFilters(t *testing.T) {
	srv, _, db := testServer(t)
	passport := claimTestInstance(t, srv, "Owner", "owner-secret")
	apiKey, _ := passport["primary_api_key"].(string)
	ownerID, err := store.GetOwnerID(context.Background(), db)
	if err != nil {
		t.Fatalf("GetOwnerID: %v", err)
	}
	mustSaveGatewayProject(t, db, "proj-1")

	svc := srv.cfg.ArtifactService
	if svc == nil {
		t.Fatal("expected artifact service on test server")
	}
	created, err := svc.CreateArtifact(context.Background(), schema.ArtifactOperationEnvelope{
		Operation:            schema.ArtifactOpCreate,
		OperationID:          "artifact-op-1",
		WorkspaceID:          "ws-default",
		ProjectID:            "proj-1",
		OwnerID:              ownerID,
		Title:                "Structured Output",
		ArtifactType:         schema.ArtifactTypeData,
		ArtifactSubtype:      "json",
		ContentFormat:        "application/json",
		Payload:              map[string]any{"hello": "world"},
		ActorType:            schema.ActorAgent,
		ActorID:              "navi",
		SourceConversationID: "sess-1",
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	version, err := svc.UpdateContent(context.Background(), schema.ArtifactOperationEnvelope{
		Operation:             schema.ArtifactOpPatch,
		OperationID:           "artifact-op-2",
		TargetArtifactID:      created.ID,
		ExpectedBaseVersionID: created.CurrentVersionID,
		Payload:               map[string]any{"hello": "navi"},
		ActorType:             schema.ActorAgent,
		ActorID:               "navi",
	})
	if err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}

	res := doJSONReq(t, srv, "GET", "/api/artifacts?type=data&subtype=json&q=structured&source_conversation_id=sess-1", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/artifacts filtered: expected 200, got %d", res.StatusCode)
	}
	var list map[string]any
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatalf("decode filtered list: %v", err)
	}
	items, ok := list["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected 1 filtered artifact item, got %v", list["items"])
	}
	item := items[0].(map[string]any)
	if item["type"] != "data" || item["subtype"] != "json" {
		t.Fatalf("unexpected filtered item: %v", item)
	}
	renderer, ok := item["renderer"].(map[string]any)
	if !ok || renderer["component_id"] != "JSONRenderer" {
		t.Fatalf("expected JSON renderer metadata, got %v", item["renderer"])
	}

	res = doJSONReq(t, srv, "GET", "/api/artifacts/"+created.ID, apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/artifacts/{id}: expected 200, got %d", res.StatusCode)
	}
	var detail map[string]any
	if err := json.NewDecoder(res.Body).Decode(&detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail["project_id"] != "proj-1" {
		t.Fatalf("expected project_id proj-1, got %v", detail["project_id"])
	}
	editor, ok := detail["editor"].(map[string]any)
	if !ok || editor["validate_strategy"] != "json" {
		t.Fatalf("expected json editor metadata, got %v", detail["editor"])
	}
	versions, ok := detail["versions"].([]any)
	if !ok || len(versions) == 0 {
		t.Fatalf("expected artifact versions, got %v", detail["versions"])
	}
	latest := versions[0].(map[string]any)
	if latest["patch_strategy"] != artifactsvc.PatchStrategyStructured {
		t.Fatalf("expected structured patch strategy, got %v", latest["patch_strategy"])
	}
	diffRef, ok := latest["diff_ref"].(map[string]any)
	if !ok || diffRef["storage_key"] == "" {
		t.Fatalf("expected diff_ref metadata, got %v", latest["diff_ref"])
	}
	raw, err := svc.ReadVersionDiff(context.Background(), version.ID)
	if err != nil {
		t.Fatalf("ReadVersionDiff: %v", err)
	}
	if !strings.Contains(raw, "\"kind\":\"structured\"") {
		t.Fatalf("unexpected diff payload %s", raw)
	}
	if latest["version_id"] != version.ID {
		t.Fatalf("expected latest version id %q, got %v", version.ID, latest["version_id"])
	}

	res = doJSONReq(t, srv, "GET", "/api/artifacts/"+created.ID+"/content", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/artifacts/{id}/content: expected 200, got %d", res.StatusCode)
	}
	var content map[string]any
	if err := json.NewDecoder(res.Body).Decode(&content); err != nil {
		t.Fatalf("decode content: %v", err)
	}
	if !strings.Contains(content["content"].(string), "\"hello\":\"navi\"") {
		t.Fatalf("unexpected content payload %v", content["content"])
	}

	res = doJSONReq(t, srv, "GET", "/api/artifacts/"+created.ID+"/versions/"+version.ID+"/diff", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/artifacts/{id}/versions/{versionID}/diff: expected 200, got %d", res.StatusCode)
	}
	var diffResp map[string]any
	if err := json.NewDecoder(res.Body).Decode(&diffResp); err != nil {
		t.Fatalf("decode diff: %v", err)
	}
	diffBody, ok := diffResp["diff"].(map[string]any)
	if !ok || diffBody["kind"] != "structured" {
		t.Fatalf("unexpected diff response %v", diffResp["diff"])
	}
}

func TestGatewayArtifactExportAndShareEndpoints(t *testing.T) {
	srv, _, db := testServer(t)
	passport := claimTestInstance(t, srv, "Owner", "owner-secret")
	apiKey, _ := passport["primary_api_key"].(string)
	ownerID, err := store.GetOwnerID(context.Background(), db)
	if err != nil {
		t.Fatalf("GetOwnerID: %v", err)
	}
	mustSaveGatewayProject(t, db, "proj-export")

	svc := srv.cfg.ArtifactService
	if svc == nil {
		t.Fatal("expected artifact service on test server")
	}
	created, err := svc.CreateArtifact(context.Background(), schema.ArtifactOperationEnvelope{
		Operation:       schema.ArtifactOpCreate,
		OperationID:     "artifact-export-op-1",
		WorkspaceID:     "ws-default",
		ProjectID:       "proj-export",
		OwnerID:         ownerID,
		Title:           "Exportable Doc",
		ArtifactType:    schema.ArtifactTypeDocument,
		ArtifactSubtype: "markdown",
		ContentFormat:   "text/markdown",
		Payload:         "# Hello export\n",
		ActorType:       schema.ActorAgent,
		ActorID:         "navi",
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}

	res := doJSONReq(t, srv, "POST", "/api/artifacts/"+created.ID+"/exports", apiKey, "192.168.1.1:1234", map[string]any{
		"format":      "pdf",
		"target_kind": "download",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/artifacts/{id}/exports: expected 201, got %d", res.StatusCode)
	}
	var exportResp map[string]any
	if err := json.NewDecoder(res.Body).Decode(&exportResp); err != nil {
		t.Fatalf("decode export response: %v", err)
	}
	exportID, _ := exportResp["export_id"].(string)
	if exportID == "" || exportResp["status"] != "completed" {
		t.Fatalf("unexpected export payload: %v", exportResp)
	}

	download := doReq(t, srv, "GET", "/api/artifact-exports/"+exportID+"/content", apiKey, "", "192.168.1.1:1234", nil)
	if download.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/artifact-exports/{id}/content: expected 200, got %d", download.StatusCode)
	}
	body, err := io.ReadAll(download.Body)
	if err != nil {
		t.Fatalf("read download body: %v", err)
	}
	if !bytes.HasPrefix(body, []byte("%PDF-1.4")) {
		t.Fatalf("expected PDF payload, got %q", string(body))
	}

	res = doJSONReq(t, srv, "POST", "/api/artifacts/"+created.ID+"/shares", apiKey, "192.168.1.1:1234", map[string]any{
		"scope":             "link",
		"access_level":      "read",
		"confirmation_mode": "proposal",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/artifacts/{id}/shares: expected 201, got %d", res.StatusCode)
	}
	var shareResp map[string]any
	if err := json.NewDecoder(res.Body).Decode(&shareResp); err != nil {
		t.Fatalf("decode share response: %v", err)
	}
	if shareResp["status"] != "active" || shareResp["share_url"] == "" {
		t.Fatalf("unexpected share response: %v", shareResp)
	}

	res = doJSONReq(t, srv, "POST", "/api/artifacts/"+created.ID+"/branches", apiKey, "192.168.1.1:1234", map[string]any{
		"base_version_id": created.CurrentVersionID,
		"name":            "conflict-fallback",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/artifacts/{id}/branches: expected 201, got %d", res.StatusCode)
	}

	if err := store.SaveExecutionOutcome(context.Background(), db, schema.ExecutionOutcome{
		AttemptID:          "artifact-export-failure-1",
		CommandID:          "cmd-export-failure-1",
		AttemptNumber:      1,
		CommandType:        schema.CommandTypeUpdate,
		StartTime:          time.Now().UTC(),
		Outcome:            schema.ExecutionOutcomeFailed,
		FailureClass:       schema.FailureClassExportFailure,
		FailureReason:      "export pipeline failed after renderer handoff",
		AffectedEntities:   `["artifact:` + created.ID + `"]`,
		CompensationStatus: schema.CompensationStatusNotRequired,
		RecoveryStatus:     schema.RecoveryStatusOpen,
		ArtifactID:         created.ID,
	}); err != nil {
		t.Fatalf("SaveExecutionOutcome: %v", err)
	}

	res = doJSONReq(t, srv, "GET", "/api/artifacts/"+created.ID, apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/artifacts/{id}: expected 200, got %d", res.StatusCode)
	}
	var detail map[string]any
	if err := json.NewDecoder(res.Body).Decode(&detail); err != nil {
		t.Fatalf("decode artifact detail: %v", err)
	}
	exports, ok := detail["exports"].([]any)
	if !ok || len(exports) != 1 {
		t.Fatalf("expected one export in detail, got %v", detail["exports"])
	}
	shares, ok := detail["shares"].([]any)
	if !ok || len(shares) != 1 {
		t.Fatalf("expected one share in detail, got %v", detail["shares"])
	}
	branches, ok := detail["branches"].([]any)
	if !ok || len(branches) < 2 {
		t.Fatalf("expected branch detail including main + fallback branch, got %v", detail["branches"])
	}
	recentRuns, ok := detail["recent_runs"].([]any)
	if !ok || len(recentRuns) == 0 {
		t.Fatalf("expected recent failure runs in detail, got %v", detail["recent_runs"])
	}
	recentRun := recentRuns[0].(map[string]any)
	if recentRun["failure_class"] != "export_failure" || recentRun["recovery_status"] != "open" {
		t.Fatalf("expected recovery failure run metadata, got %v", recentRun)
	}

	res = doJSONReq(t, srv, "GET", "/api/artifacts?project_id=proj-export", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/artifacts filtered: expected 200, got %d", res.StatusCode)
	}
	var list map[string]any
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatalf("decode artifact list: %v", err)
	}
	items, ok := list["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected 1 artifact item, got %v", list["items"])
	}
	item := items[0].(map[string]any)
	if item["shared_exported"] != true || item["sync_state"] != "exported" {
		t.Fatalf("expected first-class export/share metadata, got %v", item)
	}
}

func TestGatewayArtifactSaveContentAndConflictResponse(t *testing.T) {
	srv, _, db := testServer(t)
	passport := claimTestInstance(t, srv, "Owner", "owner-secret")
	apiKey, _ := passport["primary_api_key"].(string)
	ownerID, err := store.GetOwnerID(context.Background(), db)
	if err != nil {
		t.Fatalf("GetOwnerID: %v", err)
	}
	mustSaveGatewayProject(t, db, "proj-save")
	svc := srv.cfg.ArtifactService
	if svc == nil {
		t.Fatal("expected artifact service on test server")
	}
	created, err := svc.CreateArtifact(context.Background(), schema.ArtifactOperationEnvelope{
		Operation:       schema.ArtifactOpCreate,
		OperationID:     "artifact-save-op-1",
		WorkspaceID:     "ws-default",
		ProjectID:       "proj-save",
		OwnerID:         ownerID,
		Title:           "Saveable Doc",
		ArtifactType:    schema.ArtifactTypeDocument,
		ArtifactSubtype: "markdown",
		ContentFormat:   "text/markdown",
		Payload:         "# First\n",
		ActorType:       schema.ActorAgent,
		ActorID:         "navi",
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	originalVersionID := created.CurrentVersionID

	res := doJSONReq(t, srv, "POST", "/api/artifacts/"+created.ID+"/content", apiKey, "192.168.1.1:1234", map[string]any{
		"content":                  "# Saved\n",
		"expected_base_version_id": originalVersionID,
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/artifacts/{id}/content: expected 200, got %d", res.StatusCode)
	}
	var saved map[string]any
	if err := json.NewDecoder(res.Body).Decode(&saved); err != nil {
		t.Fatalf("decode save response: %v", err)
	}
	if saved["saved_version_id"] == "" || saved["current_version_id"] == originalVersionID {
		t.Fatalf("unexpected save payload: %v", saved)
	}

	res = doJSONReq(t, srv, "POST", "/api/artifacts/"+created.ID+"/content", apiKey, "192.168.1.1:1234", map[string]any{
		"content":                  "# Stale\n",
		"expected_base_version_id": originalVersionID,
	})
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("stale POST /api/artifacts/{id}/content: expected 409, got %d", res.StatusCode)
	}
	var conflict map[string]any
	if err := json.NewDecoder(res.Body).Decode(&conflict); err != nil {
		t.Fatalf("decode conflict response: %v", err)
	}
	errBody, ok := conflict["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected structured error body, got %v", conflict)
	}
	if errBody["code"] != "VERSION_CONFLICT" {
		t.Fatalf("expected VERSION_CONFLICT, got %v", errBody["code"])
	}
	details, ok := errBody["details"].(map[string]any)
	if !ok || details["current_head_version_id"] == "" {
		t.Fatalf("expected conflict details with current head version, got %v", errBody["details"])
	}
}

func TestGatewayArtifactRestoreVersion(t *testing.T) {
	srv, _, db := testServer(t)
	passport := claimTestInstance(t, srv, "Owner", "owner-secret")
	apiKey, _ := passport["primary_api_key"].(string)
	ownerID, err := store.GetOwnerID(context.Background(), db)
	if err != nil {
		t.Fatalf("GetOwnerID: %v", err)
	}
	mustSaveGatewayProject(t, db, "proj-restore")
	svc := srv.cfg.ArtifactService
	if svc == nil {
		t.Fatal("expected artifact service on test server")
	}
	created, err := svc.CreateArtifact(context.Background(), schema.ArtifactOperationEnvelope{
		Operation:       schema.ArtifactOpCreate,
		OperationID:     "artifact-restore-op-1",
		WorkspaceID:     "ws-default",
		ProjectID:       "proj-restore",
		OwnerID:         ownerID,
		Title:           "Restorable Doc",
		ArtifactType:    schema.ArtifactTypeDocument,
		ArtifactSubtype: "markdown",
		ContentFormat:   "text/markdown",
		Payload:         "# First\n",
		ActorType:       schema.ActorAgent,
		ActorID:         "navi",
	})
	if err != nil {
		t.Fatalf("CreateArtifact: %v", err)
	}
	originalVersionID := created.CurrentVersionID
	updated, err := svc.UpdateContent(context.Background(), schema.ArtifactOperationEnvelope{
		Operation:             schema.ArtifactOpReplace,
		OperationID:           "artifact-restore-op-2",
		TargetArtifactID:      created.ID,
		ExpectedBaseVersionID: created.CurrentVersionID,
		Payload:               "# Second\n",
		ActorType:             schema.ActorAgent,
		ActorID:               "navi",
	})
	if err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}

	res := doJSONReq(t, srv, "POST", "/api/artifacts/"+created.ID+"/versions/"+originalVersionID+"/restore", apiKey, "192.168.1.1:1234", map[string]any{
		"expected_base_version_id": updated.ID,
		"change_summary":           "Restore the original draft",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/artifacts/{id}/versions/{versionID}/restore: expected 200, got %d", res.StatusCode)
	}
	var restored map[string]any
	if err := json.NewDecoder(res.Body).Decode(&restored); err != nil {
		t.Fatalf("decode restore response: %v", err)
	}
	savedVersionID, _ := restored["saved_version_id"].(string)
	if savedVersionID == "" || savedVersionID == originalVersionID || savedVersionID == updated.ID {
		t.Fatalf("unexpected restore payload: %v", restored)
	}

	contentRes := doJSONReq(t, srv, "GET", "/api/artifacts/"+created.ID+"/content", apiKey, "192.168.1.1:1234", nil)
	if contentRes.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/artifacts/{id}/content: expected 200, got %d", contentRes.StatusCode)
	}
	var content map[string]any
	if err := json.NewDecoder(contentRes.Body).Decode(&content); err != nil {
		t.Fatalf("decode artifact content: %v", err)
	}
	if content["content"] != "# First\n" {
		t.Fatalf("expected restored content, got %v", content["content"])
	}
}

func TestGatewayKnowledgeSearch(t *testing.T) {
	srv, _, db := testServer(t)

	passport := claimTestInstance(t, srv, "Owner", "owner-secret")
	apiKey, _ := passport["primary_api_key"].(string)
	ownerID, err := store.GetOwnerID(context.Background(), db)
	if err != nil {
		t.Fatalf("GetOwnerID: %v", err)
	}
	if err := store.SaveFact(context.Background(), db, store.Fact{
		ID:       "fact-go",
		Scope:    "owner",
		ScopeID:  ownerID,
		Category: "technical_context",
		Key:      "preferred_language",
		Value:    "go",
		Source:   "explicit",
	}); err != nil {
		t.Fatalf("SaveFact: %v", err)
	}

	res := doJSONReq(t, srv, "GET", "/api/knowledge?q=golang+language", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	results, _ := body["results"].([]any)
	if len(results) == 0 {
		t.Fatal("expected semantic knowledge search results")
	}
	first, _ := results[0].(map[string]any)
	if first["entity_type"] != "fact" {
		t.Fatalf("expected fact hit, got %+v", first)
	}
}

func TestGatewayToolsEndpoints(t *testing.T) {
	db := testDB(t)
	reg := navitool.NewRegistry()
	const runtimeToolID = "runtime.messaging.runtime-only-tool"
	if err := reg.Register(gatewayTestTool(runtimeToolID, "runtime-only tool", navitool.ToolGovernance{
		CommandType: schema.CommandTypeSend,
		Domain:      "messaging",
	})); err != nil {
		t.Fatalf("Register runtime tool: %v", err)
	}
	hiddenTool := gatewayTestTool("runtime.internal.hidden-tool", "hidden tool", navitool.ToolGovernance{
		CommandType: schema.CommandTypeQuery,
		Domain:      "runtime",
	})
	hiddenTool.Hidden = true
	if err := reg.Register(hiddenTool); err != nil {
		t.Fatalf("Register hidden tool: %v", err)
	}

	srv := NewServer(Config{
		DB:              db,
		DirectiveWriter: cognitive.StoreDirectiveWriter(db),
		Governor:        governor.NewGovernor(governor.GovernorConfig{}, "."),
		Registry:        NewConnectorRegistry(),
		ToolRegistry:    reg,
		Addr:            ":0",
	})

	res := doJSONReq(t, srv, "GET", "/api/tools", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/tools: expected 200, got %d", res.StatusCode)
	}
	var list map[string]any
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatalf("decode tools list: %v", err)
	}
	items, ok := list["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("expected 2 tools, got %v", list["items"])
	}

	res = doJSONReq(t, srv, "GET", "/api/tools?surface=loop&include_hidden=false", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/tools filtered: expected 200, got %d", res.StatusCode)
	}
	list = map[string]any{}
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatalf("decode filtered tools list: %v", err)
	}
	items, ok = list["items"].([]any)
	if !ok || len(items) != 0 {
		t.Fatalf("expected 0 loop-visible non-hidden tools, got %v", list["items"])
	}

	res = doJSONReq(t, srv, "GET", "/api/tools/"+runtimeToolID, "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/tools/{name}: expected 200, got %d", res.StatusCode)
	}
	var detail map[string]any
	if err := json.NewDecoder(res.Body).Decode(&detail); err != nil {
		t.Fatalf("decode tool detail: %v", err)
	}
	if detail["name"] != runtimeToolID {
		t.Fatalf("expected %s detail, got %v", runtimeToolID, detail["name"])
	}
}

func TestGatewayReloadSkillsRefreshesToolRegistry(t *testing.T) {
	db := testDB(t)
	if err := navistore.MigrateSchema(context.Background(), db); err != nil {
		t.Fatalf("migrate navi schema: %v", err)
	}
	skillsRoot := filepath.Join(t.TempDir(), "skills")
	if err := os.MkdirAll(skillsRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll skills root: %v", err)
	}

	sessionStore := navistore.NewSQLiteStore(db)
	naviAgent, err := navi.New(navi.Config{
		SkillsDir:       skillsRoot,
		Chats:           sessionStore,
		RuntimeSessions: sessionStore,
	})
	if err != nil {
		t.Fatalf("new navi: %v", err)
	}

	skillDir := filepath.Join(skillsRoot, "gateway-hot-reload")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll skill dir: %v", err)
	}
	skillYAML := `oss27_version: "1.0"
skill_id: "gateway-hot-reload"
semver: "0.1.0"
display:
  name: "Gateway Hot Reload"
  description: "Test skill for gateway reload."
interfaces:
  - name: "run"
    description: "Run the gateway reload test tool."
    transport:
      type: "internal"
    input_schema:
      type: "object"
      additionalProperties: false
    output_schema:
      type: "object"
      properties: {}
effects:
  side_effects: []
  risk_tier: "low"
  requires_confirmation: false
security:
  auth: []
  data_access:
    pii: "none"
    secrets: "forbidden"
  sandbox:
    required: false
    network_egress: []
performance:
  expected_p50_ms: 50
  timeout_ms: 1000
observability:
  log_redaction: []
  emit_metrics: []
governance:
  publisher: "navi.test"
  signed: false
  trust_tier: "local"
capability:
  tags: ["test"]
  domains: ["testing"]
  provides: ["testing.gateway_reload"]
  command_type: "query"
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.yaml"), []byte(skillYAML), 0o644); err != nil {
		t.Fatalf("WriteFile SKILL.yaml: %v", err)
	}

	srv := NewServer(Config{
		DB:              db,
		DirectiveWriter: cognitive.StoreDirectiveWriter(db),
		Governor:        governor.NewGovernor(governor.GovernorConfig{}, "."),
		Registry:        NewConnectorRegistry(),
		Navi:            naviAgent,
		SkillRegistry:   naviAgent.Skills(),
		ToolRegistry:    naviAgent.ToolRegistry(),
		Addr:            ":0",
	})

	res := doJSONReq(t, srv, "POST", "/api/skills/reload", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/skills/reload: expected 200, got %d", res.StatusCode)
	}

	const reloadedToolID = "skill.gateway-hot-reload.run"
	res = doJSONReq(t, srv, "GET", "/api/tools/"+reloadedToolID, "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/tools/{name} after reload: expected 200, got %d", res.StatusCode)
	}
	var detail map[string]any
	if err := json.NewDecoder(res.Body).Decode(&detail); err != nil {
		t.Fatalf("decode reloaded tool detail: %v", err)
	}
	if detail["name"] != reloadedToolID {
		t.Fatalf("expected %s detail, got %v", reloadedToolID, detail["name"])
	}
}

func TestGatewayInvokeSkillInterface(t *testing.T) {
	workspace := t.TempDir()
	skillDir := filepath.Join(workspace, "skills", "invoke-test")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	skillYAML := `oss27_version: "1.0"
skill_id: "gateway.invoke.test"
semver: "0.1.0"
display:
  name: "Gateway Invoke Test"
  description: "Test skill for direct invocation."
interfaces:
  - name: "ping"
    transport:
      type: "internal"
    input_schema:
      type: "object"
      additionalProperties: false
    output_schema:
      type: "object"
      additionalProperties: true
effects:
  side_effects: []
  risk_tier: "low"
  requires_confirmation: false
security:
  auth: []
  data_access:
    pii: "none"
    secrets: "forbidden"
  sandbox:
    required: false
    network_egress: []
performance:
  expected_p50_ms: 50
  timeout_ms: 1000
observability:
  log_redaction: []
  emit_metrics: []
governance:
  publisher: "navi.test"
  signed: false
  trust_tier: "local"
capability:
  tags: ["test"]
  domains: ["testing"]
  provides: ["testing.gateway_invoke"]
  command_type: "query"
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.yaml"), []byte(skillYAML), 0o644); err != nil {
		t.Fatalf("write SKILL.yaml: %v", err)
	}
	reg := skill.NewRegistry(workspace)
	if err := reg.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	skill.RegisterInternalHandler("gateway.invoke.test", "ping", func(ctx context.Context, entry *skill.SkillEntry, iface *skill.Interface, args map[string]any) (any, error) {
		return map[string]any{
			"echo":      args["value"],
			"workspace": skillExecutionWorkspace(ctx),
		}, nil
	})
	srv := NewServer(Config{SkillRegistry: reg, WorkspaceDir: workspace, Addr: ":0"})

	res := doJSONReq(t, srv, http.MethodPost, "/api/skills/gateway.invoke.test/interfaces/ping/invoke", "", "127.0.0.1:1234", map[string]any{
		"arguments": map[string]any{"value": "pong"},
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST invoke: expected 200, got %d", res.StatusCode)
	}
	var result skill.SkillExecutionResult
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("status = %q, want success", result.Status)
	}
	payload, ok := result.Payload.(map[string]any)
	if !ok || payload["echo"] != "pong" || payload["workspace"] != workspace {
		t.Fatalf("unexpected payload: %#v", result.Payload)
	}

	res = doJSONReq(t, srv, http.MethodPost, "/api/skills/gateway.invoke.test/interfaces/missing/invoke", "", "127.0.0.1:1234", map[string]any{"arguments": map[string]any{}})
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("missing interface: expected 404, got %d", res.StatusCode)
	}
}

func skillExecutionWorkspace(ctx context.Context) string {
	if exec, ok := skill.ExecutionContextFromContext(ctx); ok {
		return exec.WorkspaceDir
	}
	return ""
}

func gatewayTestTool(toolID, description string, governance navitool.ToolGovernance) *navitool.Tool {
	return &navitool.Tool{
		ToolID:        toolID,
		DisplayName:   toolID,
		Description:   description,
		Source:        navitool.ToolSourceBuiltin,
		SourceID:      "gateway.tests",
		SchemaVersion: "1.0.0",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
		},
		OutputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
		Category:              navitool.ToolCategoryReadOnly,
		RiskTier:              "low",
		SideEffects:           []string{},
		Reversibility:         "reversible",
		EnvironmentVisibility: []string{"development"},
		RequiredModes:         []schema.DirectiveMode{},
		RequiredAuthority:     navitool.ToolAuthorityUser,
		FeatureFlags:          []string{},
		ConnectorDependencies: []string{},
		Aliases:               []string{},
		CapabilityTags:        []string{},
		Status:                navitool.ToolStatusActive,
		VisibleOn:             []string{"runtime"},
		Definition: llm.ToolDefinition{
			Name:        toolID,
			Description: description,
			Parameters: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
			},
		},
		Governance: governance,
	}
}

func TestGatewayReloadPrompts(t *testing.T) {
	db := testDB(t)
	if err := navistore.MigrateSchema(context.Background(), db); err != nil {
		t.Fatalf("migrate navi schema: %v", err)
	}
	promptDir := filepath.Join(t.TempDir(), "prompts")
	pm := prompts.New(promptDir)
	if err := pm.EnsureDefaults(); err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}
	sessionStore := navistore.NewSQLiteStore(db)
	naviAgent, err := navi.New(navi.Config{
		SkillsDir:       filepath.Join(t.TempDir(), "skills"),
		Chats:           sessionStore,
		RuntimeSessions: sessionStore,
		Prompts:         pm,
	})
	if err != nil {
		t.Fatalf("new navi: %v", err)
	}
	srv := NewServer(Config{
		DB:              db,
		DirectiveWriter: cognitive.StoreDirectiveWriter(db),
		Governor:        governor.NewGovernor(governor.GovernorConfig{}, "."),
		Registry:        NewConnectorRegistry(),
		Navi:            naviAgent,
		SkillRegistry:   naviAgent.Skills(),
		ToolRegistry:    naviAgent.ToolRegistry(),
		Addr:            ":0",
	})
	res := doJSONReq(t, srv, "POST", "/api/prompts/reload", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/prompts/reload: expected 200, got %d", res.StatusCode)
	}
}

func TestGatewayRecoverMiddlewareRecordsPanic(t *testing.T) {
	db := testDB(t)
	srv := NewServer(Config{
		DB: db,
		SaveErrorRecord: func(ctx context.Context, component, chatID, runID, errorType, message, contextJSON string) error {
			return store.SaveErrorRecord(ctx, db, store.ErrorRecord{
				Component:    component,
				ChatID:       chatID,
				RunID:        runID,
				ErrorType:    errorType,
				ErrorMessage: message,
				ContextJSON:  contextJSON,
			})
		},
	})

	handler := srv.recoverMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}

	items, err := store.ListErrorRecords(context.Background(), db, store.ListErrorsFilter{Component: "gateway", Limit: 10})
	if err != nil {
		t.Fatalf("ListErrorRecords: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected panic to be recorded")
	}
}

func TestGatewayStaticPathTraversal(t *testing.T) {
	srv, _, _ := testServer(t)
	tests := []struct {
		name string
		path string
	}{
		{"valid path", "/index.html"},
		{"traversal basic", "/../../../../etc/passwd"},
		{"traversal encoded", "/..%2f..%2f..%2fetc/passwd"},
		{"traversal double encoded", "/%252e%252e%252f%252e%252e%252f%252e%252e%252fetc%252fpasswd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := doJSONReq(t, srv, "GET", tt.path, "", "", nil)
			if res.StatusCode == http.StatusOK {
				t.Errorf("Path %s returned 200 OK, expected error/404/400", tt.path)
			}
		})
	}
}
