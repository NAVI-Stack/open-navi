package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ceoai/navi/connectors"
	"github.com/gorilla/websocket"
)

func writeMockTelegramEndpointResolve(t *testing.T, w http.ResponseWriter, r *http.Request, sessionsByExternalChat map[string]string) bool {
	t.Helper()
	if r.URL.Path != "/api/connectors/endpoints/resolve" {
		return false
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return true
	}
	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Fatalf("decode endpoint resolve request: %v", err)
	}
	externalChatID := strings.TrimSpace(req["external_chat_id"])
	naviChatID := strings.TrimSpace(sessionsByExternalChat[externalChatID])
	if naviChatID == "" {
		naviChatID = "sess-" + externalChatID
	}
	endpointID := "endpoint-" + naviChatID
	_ = json.NewEncoder(w).Encode(map[string]any{
		"chat_id":     naviChatID,
		"endpoint_id": endpointID,
		"endpoint": map[string]any{
			"endpoint_id":           endpointID,
			"chat_id":               naviChatID,
			"endpoint_type":         "connector",
			"connector_kind":        "telegram",
			"connector_instance_id": strings.TrimSpace(req["connector_instance_id"]),
			"external_chat_id":      externalChatID,
			"external_thread_id":    strings.TrimSpace(req["external_thread_id"]),
			"status":                "active",
			"receive_enabled":       true,
			"send_enabled":          true,
		},
	})
	return true
}

func TestTelegramBot(t *testing.T) {
	// Mock Gateway
	gwHit := make(map[string]int)
	gatewaySecret := "mock-jwt"
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gwHit[r.URL.Path]++
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/api/navi/chats" && r.Method == "POST" {
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(map[string]string{"chat_id": "navi-sess-1"})
			return
		}
		if r.URL.Path == "/api/directives" && r.Method == "POST" {
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(map[string]string{"directive_id": "dir-123"})
			return
		}
		if r.URL.Path == "/api/directives" && r.Method == "GET" {
			w.WriteHeader(200)
			json.NewEncoder(w).Encode([]map[string]string{{"directive_id": "dir-123", "status": "OPEN"}})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/message") {
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":        "queued",
				"queue_action":  "append",
				"inbox_status":  "pending",
				"inbox_item_id": "inbox-test-1",
			})
			return
		}
		if r.URL.Path == "/api/tasks" {
			w.WriteHeader(200)
			json.NewEncoder(w).Encode([]map[string]string{{"id": "t1"}, {"id": "t2"}})
			return
		}
		if r.URL.Path == "/api/agent/status" {
			w.WriteHeader(200)
			json.NewEncoder(w).Encode(map[string]any{
				"state":           "idle",
				"current_detail":  "2 active tasks",
				"turns_processed": 7,
			})
			return
		}
		if r.URL.Path == "/api/status" {
			w.WriteHeader(200)
			json.NewEncoder(w).Encode(map[string]any{
				"governor": map[string]any{
					"tripped":     false,
					"budget_used": 2,
					"budget_max":  1000,
				},
				"connectors": map[string]any{
					"running": 1,
					"total":   1,
				},
				"llm": map[string]any{
					"configured": true,
					"status":     "ready",
				},
				"degraded": false,
			})
			return
		}
		if r.URL.Path == "/api/hitl" {
			w.WriteHeader(200)
			json.NewEncoder(w).Encode([]map[string]any{
				{"event_id": "ev-1", "task_id": "t1", "reason": "need approval"},
			})
			return
		}
		w.WriteHeader(404)
	}))
	defer gw.Close()

	// Mock Telegram API
	tgHit := 0
	tgMsg := make(chan string, 10)
	tgUpdates := []tgUpdate{}

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/getUpdates") {
			tgHit++
			if tgHit == 1 {
				json.NewEncoder(w).Encode(map[string]any{
					"ok":     true,
					"result": tgUpdates,
				})
				return
			}
			time.Sleep(1 * time.Second)
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": []tgUpdate{}})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			var req map[string]any
			json.NewDecoder(r.Body).Decode(&req)
			tgMsg <- req["text"].(string)
			w.WriteHeader(200)
			return
		}
	}))
	defer tg.Close()

	cfg := Config{
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		OwnerChatID:   100,
		apiURL:        tg.URL,
	}
	bot := NewBot(cfg)
	bot.token = gatewaySecret

	handleMsg := func(chatID int64, text string) {
		u := tgUpdate{
			UpdateID: 1,
			Message: &tgMessage{
				Text: text,
			},
		}
		u.Message.Chat.ID = chatID
		bot.handleUpdate(context.Background(), u)
	}

	handleMsg(999, "/new foo")
	select {
	case <-tgMsg:
		t.Fatalf("should not respond to wrong chat id")
	default:
	}

	handleMsg(100, "hello bot")
	// With no active context the bot creates a session and posts the message,
	// signalling work only via Telegram's native typing action — it must NOT
	// post a "Thinking…" placeholder message. The reply arrives later via the
	// live event stream, not synchronously here.
	select {
	case m := <-tgMsg:
		if !strings.Contains(m, "Error") {
			t.Fatalf("expected no placeholder message after creating a new chat, got: %s", m)
		}
	case <-time.After(300 * time.Millisecond):
	}

	handleMsg(100, "/new foo")
	if m := <-tgMsg; !strings.Contains(m, "dir-123") {
		t.Fatalf("bad /new reply: %s", m)
	}
	if bot.activeDirectiveID != "dir-123" {
		t.Fatalf("active directive not set after /new")
	}
	if gwHit["/api/directives"] == 0 {
		t.Fatalf("gateway not called for /new")
	}

	handleMsg(100, "/status")
	if m := <-tgMsg; !strings.Contains(m, "2 active tasks") {
		t.Fatalf("bad /status reply: %s", m)
	}

	handleMsg(100, "hello bot")
	if gwHit["/api/directives/dir-123/message"] == 0 {
		t.Fatalf("gateway not called for message")
	}

	handleMsg(100, "/use dir-456")
	if m := <-tgMsg; !strings.Contains(m, "dir-456") {
		t.Fatalf("bad /use reply: %s", m)
	}
	if bot.activeDirectiveID != "dir-456" {
		t.Fatalf("active directive not set after /use")
	}

	handleMsg(100, "/list")
	if m := <-tgMsg; !strings.Contains(m, "dir-123") {
		t.Fatalf("bad /list reply: %s", m)
	}

	// HITL poller is tested via fetchAndNotifyHITL (see TestFetchAndNotifyHITL).
}

func TestSendMessageToChatRecordsStructuredConnectorError(t *testing.T) {
	t.Parallel()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bottest/sendMessage" {
			http.Error(w, `{"ok":false,"description":"transport unavailable"}`, http.StatusBadGateway)
			return
		}
		http.NotFound(w, r)
	}))
	defer api.Close()

	type recordedError struct {
		component   string
		chatID      string
		errorType   string
		message     string
		contextJSON string
	}
	recordedCh := make(chan recordedError, 1)
	bot := NewBot(Config{
		Name:          "telegram-test",
		TelegramToken: "test",
		GatewaySecret: "tok",
		apiURL:        api.URL,
		SaveErrorRecord: func(ctx context.Context, component, chatID, runID, errorType, message, contextJSON string) error {
			recordedCh <- recordedError{
				component:   component,
				chatID:      chatID,
				errorType:   errorType,
				message:     message,
				contextJSON: contextJSON,
			}
			return nil
		},
	})
	const (
		naviChatID     = "sess-telegram-error"
		telegramChatID = int64(123)
	)
	bot.naviChatToTelegramChat[naviChatID] = telegramChatID
	bot.telegramChatToNaviChat[telegramChatID] = naviChatID

	if _, err := bot.sendMessageToChat(context.Background(), "123", "hello", nil, 0, 0, ""); err == nil {
		t.Fatal("expected sendMessageToChat to fail")
	}

	select {
	case recorded := <-recordedCh:
		if recorded.component != "connector" {
			t.Fatalf("component = %q, want connector", recorded.component)
		}
		if recorded.chatID != naviChatID {
			t.Fatalf("chatID = %q, want %q", recorded.chatID, naviChatID)
		}
		if recorded.errorType != "send_message_failed" {
			t.Fatalf("errorType = %q, want send_message_failed", recorded.errorType)
		}
		if !strings.Contains(recorded.message, "telegram sendMessage error: 502") {
			t.Fatalf("message = %q, want http failure", recorded.message)
		}
		if !strings.Contains(recorded.contextJSON, `"chat_id":123`) {
			t.Fatalf("contextJSON = %q, expected chat_id", recorded.contextJSON)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected connector error to be recorded")
	}
}

func TestFetchAndNotifyHITL(t *testing.T) {
	gatewaySecret := "mock-jwt"
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/api/hitl" {
			json.NewEncoder(w).Encode([]map[string]any{
				{"event_id": "ev-hitl1", "task_id": "task-1", "reason": "confirm action"},
			})
			return
		}
		w.WriteHeader(404)
	}))
	defer gw.Close()

	tgMsg := make(chan string, 2)
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			var req map[string]any
			json.NewDecoder(r.Body).Decode(&req)
			if text, ok := req["text"].(string); ok {
				tgMsg <- text
			}
			w.WriteHeader(200)
		}
	}))
	defer tg.Close()

	bot := NewBot(Config{
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		OwnerChatID:   100,
		apiURL:        tg.URL,
	})
	bot.token = gatewaySecret
	bot.mu.Lock()
	bot.running = true
	bot.mu.Unlock()

	ctx := context.Background()
	bot.fetchAndNotifyHITL(ctx)

	select {
	case m := <-tgMsg:
		if !strings.Contains(m, "task-1") || !strings.Contains(m, "confirm action") {
			t.Errorf("expected HITL message with task and reason, got: %s", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected one Telegram sendMessage from fetchAndNotifyHITL")
	}

	// Second call should not send again (already notified)
	bot.fetchAndNotifyHITL(ctx)
	select {
	case <-tgMsg:
		t.Fatal("fetchAndNotifyHITL should not send again for same event_id")
	case <-time.After(100 * time.Millisecond):
		// expected: no second message
	}
}

func TestTelegramStatusCommandBypassesConversationalRuntime(t *testing.T) {
	gatewaySecret := "mock-jwt"
	var (
		chatMessageCalls int
		sentTexts        []string
	)

	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/agent/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"state":           "idle",
				"current_detail":  "2 active tasks",
				"turns_processed": 7,
			})
		case "/api/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"governor": map[string]any{
					"tripped":     false,
					"budget_used": 2,
					"budget_max":  1000,
				},
				"connectors": map[string]any{
					"running": 1,
					"total":   1,
				},
				"llm": map[string]any{
					"configured": true,
					"status":     "ready",
				},
				"degraded": false,
			})
		default:
			if strings.HasSuffix(r.URL.Path, "/message") {
				chatMessageCalls++
			}
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gw.Close()

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			if text, _ := req["text"].(string); text != "" {
				sentTexts = append(sentTexts, text)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": map[string]any{"message_id": 1},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer tg.Close()

	bot := NewBot(Config{
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		OwnerChatID:   100,
		apiURL:        tg.URL,
	})
	bot.token = gatewaySecret

	u := tgUpdate{UpdateID: 1, Message: &tgMessage{Text: "/status"}}
	u.Message.Chat.ID = 100
	bot.handleUpdate(context.Background(), u)

	if chatMessageCalls != 0 {
		t.Fatalf("expected /status to bypass conversational runtime, got %d message posts", chatMessageCalls)
	}
	if len(sentTexts) != 1 || !strings.Contains(sentTexts[0], "2 active tasks") {
		t.Fatalf("unexpected /status output: %#v", sentTexts)
	}
	if strings.Contains(sentTexts[0], "/model") || strings.Contains(sentTexts[0], "The request took too long") {
		t.Fatalf("expected /status output to remain direct status text, got %q", sentTexts[0])
	}
}

func TestParseModelCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want modelCommand
	}{
		{
			name: "list",
			text: "/model list",
			want: modelCommand{Handled: true, Action: "list"},
		},
		{
			name: "current",
			text: "/model current",
			want: modelCommand{Handled: true, Action: "current"},
		},
		{
			name: "set explicit",
			text: "/model set anthropic claude-sonnet-4-20250514",
			want: modelCommand{Handled: true, Action: "set", Provider: "anthropic", Model: "claude-sonnet-4-20250514"},
		},
		{
			name: "set shorthand",
			text: "/model ollama llama3.1:latest",
			want: modelCommand{Handled: true, Action: "set", Provider: "ollama", Model: "llama3.1:latest"},
		},
		{
			name: "usage",
			text: "/model",
			want: modelCommand{Handled: true, Usage: "Usage: /model list | /model current | /model set <provider> <model> | /model <provider> <model>"},
		},
		{
			name: "not model",
			text: "/status",
			want: modelCommand{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseModelCommand(tt.text)
			if got != tt.want {
				t.Fatalf("parseModelCommand(%q) = %#v, want %#v", tt.text, got, tt.want)
			}
		})
	}
}

func TestTelegramModelListBypassesConversationalRuntime(t *testing.T) {
	gatewaySecret := "mock-jwt"
	var (
		chatMessageCalls int
		sentTexts        []string
	)

	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/llm/catalog":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"providers": []map[string]any{
					{
						"key":          "ollama",
						"display_name": "Ollama",
						"models": []map[string]string{
							{"name": "llama3.1:latest"},
						},
					},
					{
						"key":          "anthropic",
						"display_name": "Anthropic",
						"models": []map[string]string{
							{"name": "claude-sonnet-4-20250514"},
						},
					},
				},
			})
		default:
			if strings.HasSuffix(r.URL.Path, "/message") {
				chatMessageCalls++
			}
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gw.Close()

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			if text, _ := req["text"].(string); text != "" {
				sentTexts = append(sentTexts, text)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": map[string]any{"message_id": 1},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer tg.Close()

	bot := NewBot(Config{
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		OwnerChatID:   100,
		apiURL:        tg.URL,
	})
	bot.token = gatewaySecret

	u := tgUpdate{UpdateID: 1, Message: &tgMessage{Text: "/model list"}}
	u.Message.Chat.ID = 100
	bot.handleUpdate(context.Background(), u)

	if chatMessageCalls != 0 {
		t.Fatalf("expected /model list to bypass conversational runtime, got %d message posts", chatMessageCalls)
	}
	if len(sentTexts) != 1 {
		t.Fatalf("expected 1 Telegram message, got %#v", sentTexts)
	}
	if !strings.Contains(sentTexts[0], "Ollama") || !strings.Contains(sentTexts[0], "Anthropic") {
		t.Fatalf("expected catalog output in Telegram, got %q", sentTexts[0])
	}
}

func TestTelegramModelSetUsesDirectGatewayPath(t *testing.T) {
	gatewaySecret := "mock-jwt"
	var (
		chatMessageCalls int
		gotProvider      string
		gotModel         string
		sentTexts        []string
	)

	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/llm/active":
			if r.Method != http.MethodPut {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var req map[string]string
			_ = json.NewDecoder(r.Body).Decode(&req)
			gotProvider = req["provider"]
			gotModel = req["model"]
			_ = json.NewEncoder(w).Encode(map[string]string{
				"provider": gotProvider,
				"model":    gotModel,
				"status":   gotProvider + "/" + gotModel,
			})
		default:
			if strings.HasSuffix(r.URL.Path, "/message") {
				chatMessageCalls++
			}
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gw.Close()

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			if text, _ := req["text"].(string); text != "" {
				sentTexts = append(sentTexts, text)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": map[string]any{"message_id": 1},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer tg.Close()

	bot := NewBot(Config{
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		OwnerChatID:   100,
		apiURL:        tg.URL,
	})
	bot.token = gatewaySecret

	u := tgUpdate{UpdateID: 1, Message: &tgMessage{Text: "/model anthropic claude-sonnet-4-20250514"}}
	u.Message.Chat.ID = 100
	bot.handleUpdate(context.Background(), u)

	if chatMessageCalls != 0 {
		t.Fatalf("expected /model set to bypass conversational runtime, got %d message posts", chatMessageCalls)
	}
	if gotProvider != "anthropic" || gotModel != "claude-sonnet-4-20250514" {
		t.Fatalf("unexpected direct model switch payload: provider=%q model=%q", gotProvider, gotModel)
	}
	if len(sentTexts) != 1 || !strings.Contains(sentTexts[0], "Switched to provider anthropic, model claude-sonnet-4-20250514.") {
		t.Fatalf("unexpected /model set output: %#v", sentTexts)
	}
}

func TestTelegramModelSetSurfacesExactGatewayError(t *testing.T) {
	gatewaySecret := "mock-jwt"
	var sentTexts []string

	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/api/llm/active" && r.Method == http.MethodPut {
			http.Error(w, "llm: unknown provider/model combination \"anthropic\" / \"missing\"", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer gw.Close()

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			if text, _ := req["text"].(string); text != "" {
				sentTexts = append(sentTexts, text)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": map[string]any{"message_id": 1},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer tg.Close()

	bot := NewBot(Config{
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		OwnerChatID:   100,
		apiURL:        tg.URL,
	})
	bot.token = gatewaySecret

	u := tgUpdate{UpdateID: 1, Message: &tgMessage{Text: "/model anthropic missing"}}
	u.Message.Chat.ID = 100
	bot.handleUpdate(context.Background(), u)

	if len(sentTexts) != 1 {
		t.Fatalf("expected exactly one Telegram reply, got %#v", sentTexts)
	}
	if !strings.Contains(sentTexts[0], "Model switch failed: gateway PUT /api/llm/active: status 400") ||
		!strings.Contains(sentTexts[0], "unknown provider/model combination") {
		t.Fatalf("expected exact gateway error in Telegram reply, got %q", sentTexts[0])
	}
}

func TestTelegramNewUsesAdviseMode(t *testing.T) {
	var gotMode string
	gatewaySecret := "secret"
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/api/directives" && r.Method == http.MethodPost {
			var req map[string]string
			_ = json.NewDecoder(r.Body).Decode(&req)
			gotMode = req["mode"]
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"directive_id": "dir-123"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer gw.Close()

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"message_id": 1,
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer tg.Close()

	bot := NewBot(Config{
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		OwnerChatID:   100,
		apiURL:        tg.URL,
	})
	bot.token = gatewaySecret

	u := tgUpdate{UpdateID: 1, Message: &tgMessage{Text: "/new roadmap"}}
	u.Message.Chat.ID = 100
	bot.handleUpdate(context.Background(), u)

	if gotMode != "ADVISE" {
		t.Fatalf("expected /new to use ADVISE mode, got %q", gotMode)
	}
}

func TestTelegramImplementUsesActMode(t *testing.T) {
	var (
		gotMode string
		gotText string
	)
	gatewaySecret := "secret"
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/api/directives/dir-123/mode" && r.Method == http.MethodPut {
			var req map[string]string
			_ = json.NewDecoder(r.Body).Decode(&req)
			gotMode = req["mode"]
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer gw.Close()

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			gotText, _ = req["text"].(string)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"message_id": 1,
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer tg.Close()

	bot := NewBot(Config{
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		OwnerChatID:   100,
		apiURL:        tg.URL,
	})
	bot.token = gatewaySecret

	u := tgUpdate{UpdateID: 1, Message: &tgMessage{Text: "/implement dir-123"}}
	u.Message.Chat.ID = 100
	bot.handleUpdate(context.Background(), u)

	if gotMode != "ACT" {
		t.Fatalf("expected /implement to use ACT mode, got %q", gotMode)
	}
	if gotText != "Mode set to ACT." {
		t.Fatalf("expected ACT confirmation text, got %q", gotText)
	}
}

func TestSendMediaRoutesByContentType(t *testing.T) {
	type mediaHit struct {
		endpoint string
		chatID   string
		field    string
	}

	hits := make(chan mediaHit, 4)
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendPhoto"):
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("parse multipart sendPhoto: %v", err)
			}
			if _, _, err := r.FormFile("photo"); err != nil {
				t.Fatalf("expected photo form field: %v", err)
			}
			hits <- mediaHit{endpoint: "sendPhoto", chatID: r.FormValue("chat_id"), field: "photo"}
			w.WriteHeader(200)
		case strings.HasSuffix(r.URL.Path, "/sendDocument"):
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("parse multipart sendDocument: %v", err)
			}
			if _, _, err := r.FormFile("document"); err != nil {
				t.Fatalf("expected document form field: %v", err)
			}
			hits <- mediaHit{endpoint: "sendDocument", chatID: r.FormValue("chat_id"), field: "document"}
			w.WriteHeader(200)
		default:
			w.WriteHeader(404)
		}
	}))
	defer tg.Close()

	bot := NewBot(Config{
		Name:          "telegram-media-test",
		TelegramToken: "test",
		OwnerChatID:   321,
		apiURL:        tg.URL,
	})
	bot.mu.Lock()
	bot.running = true
	bot.mu.Unlock()

	err := bot.SendMedia(context.Background(), connectors.OutboundMediaMessage{
		ChatID: "",
		Parts: []connectors.MediaPart{
			{ContentType: "image/png", Data: []byte("img-bytes"), Filename: "image.png"},
			{ContentType: "application/pdf", Data: []byte("pdf-bytes"), Filename: "file.pdf"},
		},
	})
	if err != nil {
		t.Fatalf("SendMedia: %v", err)
	}

	got := map[string]mediaHit{}
	for i := 0; i < 2; i++ {
		select {
		case h := <-hits:
			got[h.endpoint] = h
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for media API calls")
		}
	}
	if got["sendPhoto"].field != "photo" || got["sendPhoto"].chatID != "321" {
		t.Fatalf("bad sendPhoto hit: %+v", got["sendPhoto"])
	}
	if got["sendDocument"].field != "document" || got["sendDocument"].chatID != "321" {
		t.Fatalf("bad sendDocument hit: %+v", got["sendDocument"])
	}
}

func TestHandlePairing(t *testing.T) {
	tgMsg := make(chan string, 8)
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			if text, ok := req["text"].(string); ok {
				tgMsg <- text
			}
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(404)
	}))
	defer tg.Close()

	bot := NewBot(Config{
		TelegramToken: "test",
		OwnerChatID:   100,
		AllowFrom:     []int64{200},
		PairingCode:   "pair-code",
		apiURL:        tg.URL,
	})

	// Non-pairing message from unauthorized chat should be ignored.
	bot.handleUpdate(context.Background(), tgUpdate{
		UpdateID: 1,
		Message: &tgMessage{
			Text: "hello",
			Chat: tgChat{ID: 999},
		},
	})
	select {
	case <-tgMsg:
		t.Fatal("unexpected response for unauthorized non-pairing message")
	default:
	}

	// Wrong pairing code should fail.
	bot.handleUpdate(context.Background(), tgUpdate{
		UpdateID: 2,
		Message: &tgMessage{
			Text: "/pair wrong",
			Chat: tgChat{ID: 999},
		},
	})
	select {
	case m := <-tgMsg:
		if !strings.Contains(m, "Pairing failed") {
			t.Fatalf("expected pairing failure message, got: %s", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected pairing failure response")
	}

	// Correct pairing code authorizes the chat.
	bot.handleUpdate(context.Background(), tgUpdate{
		UpdateID: 3,
		Message: &tgMessage{
			Text: "/pair pair-code",
			Chat: tgChat{ID: 999},
		},
	})
	select {
	case m := <-tgMsg:
		if !strings.Contains(m, "Pairing successful") {
			t.Fatalf("expected pairing success message, got: %s", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected pairing success response")
	}

	if !bot.isAllowedChat(999) {
		t.Fatal("expected chat 999 to be authorized after successful pairing")
	}
	if !bot.isAllowedChat(200) {
		t.Fatal("expected allow_from chat 200 to be pre-authorized")
	}
}

func TestHITLPollerUsesInjectedTicker(t *testing.T) {
	gatewaySecret := "mock-jwt"
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/api/hitl" {
			w.WriteHeader(200)
			json.NewEncoder(w).Encode([]map[string]any{
				{"event_id": "ev-tick-1", "task_id": "task-1", "reason": "tick check"},
			})
			return
		}
		w.WriteHeader(404)
	}))
	defer gw.Close()

	tgMsg := make(chan string, 2)
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			if text, ok := req["text"].(string); ok {
				tgMsg <- text
			}
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(404)
	}))
	defer tg.Close()

	bot := NewBot(Config{
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		OwnerChatID:   100,
		apiURL:        tg.URL,
	})
	bot.token = "mock-jwt"
	bot.mu.Lock()
	bot.running = true
	bot.mu.Unlock()

	tickCh := make(chan time.Time, 1)
	stopped := make(chan struct{}, 1)
	bot.newHITLTickSource = func() (<-chan time.Time, func()) {
		return tickCh, func() { close(stopped) }
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go bot.hitlPoller(ctx)

	tickCh <- time.Now()
	select {
	case m := <-tgMsg:
		if !strings.Contains(m, "tick check") {
			t.Fatalf("expected hitl tick message, got: %s", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected hitl message after injected tick")
	}

	cancel()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("expected injected ticker stop function to be called")
	}
}

func TestTelegramStreamingRenderLifecycle(t *testing.T) {
	var sendCount int
	var editCount int
	var lastSent string
	var lastEdited string
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			sendCount++
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			lastSent, _ = req["text"].(string)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"message_id": 700 + sendCount,
				},
			})
		case strings.HasSuffix(r.URL.Path, "/editMessageText"):
			editCount++
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			lastEdited, _ = req["text"].(string)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		default:
			w.WriteHeader(404)
		}
	}))
	defer tg.Close()

	bot := NewBot(Config{
		TelegramToken: "test",
		OwnerChatID:   100,
		apiURL:        tg.URL,
	})

	ctx := context.Background()
	bot.renderSessionPartial(ctx, "sess-1", 123, "Hello")
	if sendCount != 1 {
		t.Fatalf("expected first partial to send one message, got %d", sendCount)
	}
	if lastSent != "Hello" {
		t.Fatalf("expected first send to contain partial text, got %q", lastSent)
	}

	bot.renderSessionPartial(ctx, "sess-1", 123, "Hello there")
	if editCount != 1 {
		t.Fatalf("expected second partial to edit existing message, got %d edits", editCount)
	}
	if lastEdited != "Hello there" {
		t.Fatalf("expected edit text %q, got %q", "Hello there", lastEdited)
	}

	bot.finalizeSessionMessage(ctx, "sess-1", 123, "Hello there, world")
	if editCount != 2 {
		t.Fatalf("expected completion to edit existing message, got %d edits", editCount)
	}
	if lastEdited != "Hello there, world" {
		t.Fatalf("expected final edit text %q, got %q", "Hello there, world", lastEdited)
	}
	if _, ok := bot.chatStream["sess-1"]; ok {
		t.Fatal("expected chat stream state to be cleared after completion")
	}
}

func TestTelegramStartSessionThinkingEmitsTypingOnly(t *testing.T) {
	var (
		chatActionCount int
		sendCount       int
		editCount       int
	)

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendChatAction"):
			chatActionCount++
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			sendCount++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"message_id": 900 + sendCount,
				},
			})
		case strings.HasSuffix(r.URL.Path, "/editMessageText"):
			editCount++
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		default:
			w.WriteHeader(404)
		}
	}))
	defer tg.Close()

	bot := NewBot(Config{
		Name:          "telegram-test",
		TelegramToken: "test",
		OwnerChatID:   100,
		apiURL:        tg.URL,
	})

	ctx := context.Background()
	// startSessionThinking signals work via Telegram's native typing action only.
	// It must never post or edit a "Thinking…" placeholder message.
	bot.startSessionThinking(ctx, "sess-reuse", 123)
	bot.startSessionThinking(ctx, "sess-reuse", 123)

	if sendCount != 0 {
		t.Fatalf("expected no placeholder message to be sent, got %d sends", sendCount)
	}
	if editCount != 0 {
		t.Fatalf("expected no placeholder edit, got %d edits", editCount)
	}
	if chatActionCount != 2 {
		t.Fatalf("expected the typing indicator to start once per turn, got %d calls", chatActionCount)
	}
}

func TestTelegramFinalizeSessionMessageSplitsLongReplies(t *testing.T) {
	var sentTexts []string
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			if text, ok := req["text"].(string); ok {
				sentTexts = append(sentTexts, text)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"message_id": len(sentTexts),
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer tg.Close()

	bot := NewBot(Config{
		TelegramToken: "test",
		OwnerChatID:   100,
		apiURL:        tg.URL,
	})

	content := strings.Repeat("a", 4097) + "\n\n" + strings.Repeat("b", 64)
	bot.finalizeSessionMessage(context.Background(), "sess-long", 100, content)

	if len(sentTexts) < 2 {
		t.Fatalf("expected long reply to split into multiple messages, got %d", len(sentTexts))
	}
	for i, text := range sentTexts {
		if len([]rune(text)) > bot.MaxMessageLength() {
			t.Fatalf("chunk %d exceeds telegram max length: %d", i, len([]rune(text)))
		}
	}
}

func TestTelegramRecoverFocusUsesSessionIDJSONField(t *testing.T) {
	gatewaySecret := "mock-jwt"
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/navi/chats":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"chat_id": "sess-recovered"},
			})
		case "/api/directives":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gw.Close()

	bot := NewBot(Config{
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		OwnerChatID:   100,
	})
	bot.token = gatewaySecret

	if err := bot.recoverFocus(context.Background()); err != nil {
		t.Fatalf("recoverFocus: %v", err)
	}
	if bot.activeChatID != "sess-recovered" {
		t.Fatalf("expected recovered chat id, got %q", bot.activeChatID)
	}
}

func TestTelegramRecoverFocusSkipsHeartbeatSession(t *testing.T) {
	gatewaySecret := "mock-jwt"
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/navi/chats":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"chat_id": "heartbeat-auto"},
				{"chat_id": "sess-user"},
			})
		case "/api/directives":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gw.Close()

	bot := NewBot(Config{
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		OwnerChatID:   100,
	})
	bot.token = gatewaySecret

	if err := bot.recoverFocus(context.Background()); err != nil {
		t.Fatalf("recoverFocus: %v", err)
	}
	if bot.activeChatID != "sess-user" {
		t.Fatalf("expected non-heartbeat chat id, got %q", bot.activeChatID)
	}
}

func TestTelegramCreateSessionWithRetry(t *testing.T) {
	gatewaySecret := "mock-jwt"
	var createAttempts int
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/api/navi/chats" && r.Method == http.MethodPost:
			createAttempts++
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode create session request: %v", err)
			}
			if _, ok := req["persona"]; ok {
				t.Fatalf("unexpected persona field in create session payload: %#v", req)
			}
			if createAttempts == 1 {
				w.WriteHeader(http.StatusGatewayTimeout)
				_, _ = w.Write([]byte(`timeout`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"chat_id": "sess-created"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gw.Close()

	bot := NewBot(Config{
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		OwnerChatID:   100,
	})
	bot.token = gatewaySecret
	bot.chatBackoff = func(int) time.Duration { return 0 }

	chatID, err := bot.createChatWithRetry(context.Background())
	if err != nil {
		t.Fatalf("createChatWithRetry: %v", err)
	}
	if chatID != "sess-created" {
		t.Fatalf("expected created chat id, got %q", chatID)
	}
	if createAttempts != 2 {
		t.Fatalf("expected 2 create attempts, got %d", createAttempts)
	}
}

func TestTelegramWaitForGatewayReadyUsesHealthEndpoint(t *testing.T) {
	var healthHits int
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		healthHits++
		if healthHits < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer gw.Close()

	bot := NewBot(Config{
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		OwnerChatID:   100,
	})

	if err := bot.waitForGatewayReady(context.Background(), 2*time.Second); err != nil {
		t.Fatalf("waitForGatewayReady: %v", err)
	}
	if healthHits < 2 {
		t.Fatalf("expected readiness probe retries, got %d hits", healthHits)
	}
}

func TestTelegramSendSanitizesBreakTags(t *testing.T) {
	var lastSent string
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendMessage") {
			w.WriteHeader(404)
			return
		}
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		lastSent, _ = req["text"].(string)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": map[string]any{
				"message_id": 701,
			},
		})
	}))
	defer tg.Close()

	bot := NewBot(Config{
		TelegramToken: "test",
		OwnerChatID:   100,
		apiURL:        tg.URL,
	})
	bot.mu.Lock()
	bot.running = true
	bot.mu.Unlock()

	err := bot.Send(context.Background(), connectors.OutboundMessage{
		Content: "line 1<br>line 2<BR/>line 3",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if lastSent != "line 1\nline 2\nline 3" {
		t.Fatalf("expected sanitized send text, got %q", lastSent)
	}
}

func TestTelegramStreamingSanitizesBreakTags(t *testing.T) {
	var lastSent string
	var lastEdited string
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			lastSent, _ = req["text"].(string)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"message_id": 702,
				},
			})
		case strings.HasSuffix(r.URL.Path, "/editMessageText"):
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			lastEdited, _ = req["text"].(string)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		default:
			w.WriteHeader(404)
		}
	}))
	defer tg.Close()

	bot := NewBot(Config{
		TelegramToken: "test",
		OwnerChatID:   100,
		apiURL:        tg.URL,
	})

	ctx := context.Background()
	bot.renderSessionPartial(ctx, "sess-2", 123, "first<br>partial")
	if lastSent != "first\npartial" {
		t.Fatalf("expected sanitized partial send text, got %q", lastSent)
	}

	bot.finalizeSessionMessage(ctx, "sess-2", 123, "done<br/>now")
	if lastEdited != "done\nnow" {
		t.Fatalf("expected sanitized final edit text, got %q", lastEdited)
	}
}

func TestFormatProposalWaitingTextIncludesMetadata(t *testing.T) {
	text := formatProposalWaitingText("prop-123", "skill-creator_create", "confirmation required", map[string]any{
		"proposal_summary":       `Create "disk-usage" to check disk space.`,
		"capability_description": "check disk space",
		"side_effects":           []any{"filesystem_write", "registry_update"},
		"risk_tier":              "high",
	})

	for _, want := range []string{
		`Action: Create "disk-usage" to check disk space.`,
		"Capability: check disk space",
		"Side effects: filesystem_write, registry_update",
		"Risk: high",
		"Proposal: prop-123",
		"Tool: skill-creator_create",
		"Reason: confirmation required",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected proposal text to contain %q, got %q", want, text)
		}
	}
}

func TestProposalReplyMarkupIncludesApproveRejectButtons(t *testing.T) {
	replyMarkup, ok := proposalReplyMarkup("prop-123").(map[string]any)
	if !ok {
		t.Fatalf("expected map reply markup, got %T", proposalReplyMarkup("prop-123"))
	}
	rows, ok := replyMarkup["inline_keyboard"].([][]map[string]string)
	if !ok || len(rows) != 1 || len(rows[0]) != 2 {
		t.Fatalf("unexpected inline keyboard shape: %#v", replyMarkup["inline_keyboard"])
	}
	if rows[0][0]["callback_data"] != "proposal_approve:prop-123" {
		t.Fatalf("unexpected approve callback data: %#v", rows[0][0])
	}
	if rows[0][1]["callback_data"] != "proposal_reject:prop-123" {
		t.Fatalf("unexpected reject callback data: %#v", rows[0][1])
	}
}

func TestHandleCallbackResolvesProposal(t *testing.T) {
	var resolvedPath string
	var resolvedAction string
	tgMsg := make(chan string, 1)
	gatewaySecret := "mock-jwt"
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/api/proposals/prop-123/resolve" || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		resolvedPath = r.URL.Path
		var req map[string]string
		_ = json.NewDecoder(r.Body).Decode(&req)
		resolvedAction = req["action"]
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "approved"})
	}))
	defer gw.Close()

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			if text, ok := req["text"].(string); ok {
				tgMsg <- text
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"message_id": 900}})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/answerCallbackQuery") {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer tg.Close()

	bot := NewBot(Config{
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		OwnerChatID:   100,
		apiURL:        tg.URL,
	})
	bot.token = gatewaySecret

	bot.handleCallback(context.Background(), 100, &tgCallbackQuery{
		ID:   "cb-1",
		Data: "proposal_approve:prop-123",
	})

	if resolvedPath != "/api/proposals/prop-123/resolve" {
		t.Fatalf("unexpected resolved path %q", resolvedPath)
	}
	if resolvedAction != "approve" {
		t.Fatalf("expected approve action, got %q", resolvedAction)
	}
	select {
	case got := <-tgMsg:
		if got != "Proposal approved." {
			t.Fatalf("unexpected callback confirmation %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected Telegram callback confirmation message")
	}
}

func TestRecoverFocusUsesSessionIDJSONField(t *testing.T) {
	gatewaySecret := "mock-jwt"
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/navi/chats":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"chat_id": "navi-sess-1"},
			})
		case "/api/directives":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gw.Close()

	bot := NewBot(Config{
		Name:          "telegram-test",
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
	})
	bot.token = gatewaySecret

	if err := bot.recoverFocus(context.Background()); err != nil {
		t.Fatalf("recoverFocus: %v", err)
	}
	if bot.activeChatID != "navi-sess-1" {
		t.Fatalf("activeChatID = %q, want %q", bot.activeChatID, "navi-sess-1")
	}
}

func TestCreateSessionWithRetry(t *testing.T) {
	gatewaySecret := "mock-jwt"
	postAttempts := 0
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/navi/chats":
			if r.Method == http.MethodGet {
				_ = json.NewEncoder(w).Encode([]map[string]any{})
				return
			}
			postAttempts++
			if postAttempts < 3 {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte("retry"))
				return
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"chat_id": "sess-retry"})
		case "/api/directives":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gw.Close()

	bot := NewBot(Config{
		Name:          "telegram-test",
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
	})
	bot.token = gatewaySecret
	sleepCalls := 0
	bot.sleep = func(time.Duration) { sleepCalls++ }

	chatID, err := bot.createChatWithRetry(context.Background())
	if err != nil {
		t.Fatalf("createChatWithRetry: %v", err)
	}
	if chatID != "sess-retry" {
		t.Fatalf("chatID = %q, want %q", chatID, "sess-retry")
	}
	if postAttempts != 3 {
		t.Fatalf("postAttempts = %d, want 3", postAttempts)
	}
	if sleepCalls != 2 {
		t.Fatalf("sleepCalls = %d, want 2", sleepCalls)
	}
}

func TestWaitForGatewayReadyUsesHealthEndpoint(t *testing.T) {
	gatewaySecret := "mock-jwt"
	healthHits := 0
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		healthHits++
		if healthHits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer gw.Close()

	bot := NewBot(Config{
		Name:          "telegram-test",
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
	})
	bot.token = gatewaySecret
	bot.sleep = func(time.Duration) {}

	if err := bot.waitForGatewayReady(context.Background()); err != nil {
		t.Fatalf("waitForGatewayReady: %v", err)
	}
	if healthHits != 3 {
		t.Fatalf("healthHits = %d, want 3", healthHits)
	}
}

func TestEnsureSessionUsesResolvedExistingEndpoint(t *testing.T) {
	gatewaySecret := "mock-jwt"
	legacyChatHits := 0
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if writeMockTelegramEndpointResolve(t, w, r, map[string]string{"123": "sess-existing"}) {
			return
		}
		switch r.URL.Path {
		case "/api/navi/chats":
			legacyChatHits++
			w.WriteHeader(http.StatusNotFound)
		case "/api/directives":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gw.Close()

	bot := NewBot(Config{
		Name:          "telegram-test",
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
	})
	bot.token = gatewaySecret

	chatID, err := bot.ensureChat(context.Background(), 123)
	if err != nil {
		t.Fatalf("ensureChat: %v", err)
	}
	if chatID != "sess-existing" {
		t.Fatalf("chatID = %q, want %q", chatID, "sess-existing")
	}
	if legacyChatHits != 0 {
		t.Fatalf("legacy chat route hits = %d, want 0", legacyChatHits)
	}
	if got := bot.naviChatToTelegramChat["sess-existing"]; got != 123 {
		t.Fatalf("naviChatToTelegramChat entry = %d, want 123", got)
	}
}

func TestEnsureSessionUsesDurableEndpointResolve(t *testing.T) {
	gatewaySecret := "mock-jwt"
	var resolveHits int
	var legacyChatHits int
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/connectors/endpoints/resolve":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			resolveHits++
			var req map[string]string
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode resolve request: %v", err)
			}
			if req["connector_kind"] != "telegram" {
				t.Fatalf("connector_kind = %q, want telegram", req["connector_kind"])
			}
			if req["connector_instance_id"] != "telegram-test" {
				t.Fatalf("connector_instance_id = %q, want telegram-test", req["connector_instance_id"])
			}
			if req["external_chat_id"] != "123" {
				t.Fatalf("external_chat_id = %q, want 123", req["external_chat_id"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"chat_id":     "sess-resolved",
				"endpoint_id": "endpoint-resolved",
				"endpoint": map[string]any{
					"endpoint_id":           "endpoint-resolved",
					"chat_id":               "sess-resolved",
					"endpoint_type":         "connector",
					"connector_kind":        "telegram",
					"connector_instance_id": "telegram-test",
					"external_chat_id":      "123",
					"status":                "active",
					"receive_enabled":       true,
					"send_enabled":          true,
				},
			})
		case "/api/navi/chats":
			legacyChatHits++
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gw.Close()

	bot := NewBot(Config{
		Name:          "telegram-test",
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
	})
	bot.token = gatewaySecret
	bot.sleep = func(time.Duration) {}

	chatID, err := bot.ensureChat(context.Background(), 123)
	if err != nil {
		t.Fatalf("ensureChat: %v", err)
	}
	if chatID != "sess-resolved" {
		t.Fatalf("chatID = %q, want sess-resolved", chatID)
	}
	if resolveHits != 1 {
		t.Fatalf("resolveHits = %d, want 1", resolveHits)
	}
	if legacyChatHits != 0 {
		t.Fatalf("legacy chat route hits = %d, want 0", legacyChatHits)
	}
	if got := bot.naviChatToTelegramChat["sess-resolved"]; got != 123 {
		t.Fatalf("naviChatToTelegramChat entry = %d, want 123", got)
	}
}

func TestHandleUpdatePostsDurableOriginEndpointID(t *testing.T) {
	gatewaySecret := "mock-jwt"
	var postedOriginEndpointID string
	var postedContent string
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/connectors/endpoints/resolve":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"chat_id":     "sess-resolved",
				"endpoint_id": "endpoint-resolved",
				"endpoint": map[string]any{
					"endpoint_id":           "endpoint-resolved",
					"chat_id":               "sess-resolved",
					"endpoint_type":         "connector",
					"connector_kind":        "telegram",
					"connector_instance_id": "telegram-test",
					"external_chat_id":      "123",
					"status":                "active",
					"receive_enabled":       true,
					"send_enabled":          true,
				},
			})
		case "/api/navi/chats/sess-resolved/message":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode chat message request: %v", err)
			}
			postedOriginEndpointID, _ = req["origin_endpoint_id"].(string)
			postedContent, _ = req["content"].(string)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":        "queued",
				"queue_action":  "append",
				"inbox_status":  "pending",
				"inbox_item_id": "inbox-test-1",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gw.Close()

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"message_id": 1001,
				},
			})
		case strings.HasSuffix(r.URL.Path, "/sendChatAction"), strings.HasSuffix(r.URL.Path, "/editMessageText"):
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer tg.Close()

	bot := NewBot(Config{
		Name:          "telegram-test",
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		OwnerChatID:   123,
		apiURL:        tg.URL,
	})
	bot.token = gatewaySecret
	bot.sleep = func(time.Duration) {}

	bot.handleUpdate(context.Background(), tgUpdate{
		UpdateID: 1,
		Message: &tgMessage{
			MessageID: 77,
			Chat: tgChat{
				ID:   123,
				Type: "private",
			},
			Text: "hello durable endpoint",
		},
	})

	if postedContent != "hello durable endpoint" {
		t.Fatalf("posted content = %q, want hello durable endpoint", postedContent)
	}
	if postedOriginEndpointID != "endpoint-resolved" {
		t.Fatalf("origin_endpoint_id = %q, want endpoint-resolved", postedOriginEndpointID)
	}
}

func TestEnsureSessionDoesNotReuseRecoveredSessionInMultiChatMode(t *testing.T) {
	gatewaySecret := "mock-jwt"
	legacyChatHits := 0
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if writeMockTelegramEndpointResolve(t, w, r, map[string]string{"200": "sess-chat-200"}) {
			return
		}
		switch r.URL.Path {
		case "/api/navi/chats":
			legacyChatHits++
			w.WriteHeader(http.StatusNotFound)
		case "/api/directives":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gw.Close()

	bot := NewBot(Config{
		Name:          "telegram-test",
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		OwnerChatID:   100,
		AllowFrom:     []int64{200},
	})
	bot.token = gatewaySecret

	chatID, err := bot.ensureChat(context.Background(), 200)
	if err != nil {
		t.Fatalf("ensureChat: %v", err)
	}
	if chatID != "sess-chat-200" {
		t.Fatalf("chatID = %q, want %q", chatID, "sess-chat-200")
	}
	if legacyChatHits != 0 {
		t.Fatalf("legacy chat route hits = %d, want 0", legacyChatHits)
	}
	if got := bot.naviChatToTelegramChat["sess-owner"]; got != 0 {
		t.Fatalf("recovered owner session should not be rebound to chat 200, got %d", got)
	}
}

func TestEnsureSessionIgnoresHeartbeatActiveSession(t *testing.T) {
	gatewaySecret := "mock-jwt"
	legacyChatHits := 0
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if writeMockTelegramEndpointResolve(t, w, r, map[string]string{"123": "sess-new"}) {
			return
		}
		switch r.URL.Path {
		case "/api/navi/chats":
			legacyChatHits++
			w.WriteHeader(http.StatusNotFound)
		case "/api/directives":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gw.Close()

	bot := NewBot(Config{
		Name:          "telegram-test",
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
	})
	bot.token = gatewaySecret
	bot.activeChatID = "heartbeat-auto"

	chatID, err := bot.ensureChat(context.Background(), 123)
	if err != nil {
		t.Fatalf("ensureChat: %v", err)
	}
	if chatID != "sess-new" {
		t.Fatalf("chatID = %q, want %q", chatID, "sess-new")
	}
	if legacyChatHits != 0 {
		t.Fatalf("legacy chat route hits = %d, want 0", legacyChatHits)
	}
}

// ---------------------------------------------------------------------------
// Tests for replay-mode and naviChatToTelegramChat fixes
// ---------------------------------------------------------------------------

// wsUpgrader is used by WS test servers in this file.
var wsUpgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

// TestEnsureSession_DurableEndpointDoesNotMarkFreshSession verifies that
// endpoint resolution does not guess whether the gateway created or recovered a
// chat. Fresh replay state remains explicit.
func TestEnsureSession_DurableEndpointDoesNotMarkFreshSession(t *testing.T) {
	gatewaySecret := "tok"
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeMockTelegramEndpointResolve(t, w, r, map[string]string{"111": "fresh-sess"}) {
			return
		}
		if r.URL.Path == "/api/navi/chats" && r.Method == "POST" {
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(map[string]string{"chat_id": "fresh-sess"})
			return
		}
		if r.URL.Path == "/api/navi/chats" && r.Method == "GET" {
			// No existing sessions → triggers createChatWithRetry
			w.WriteHeader(200)
			json.NewEncoder(w).Encode([]map[string]string{})
			return
		}
		if r.URL.Path == "/api/directives" {
			w.WriteHeader(200)
			json.NewEncoder(w).Encode([]map[string]string{})
			return
		}
		w.WriteHeader(404)
	}))
	defer gw.Close()

	bot := NewBot(Config{
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
	})
	bot.token = gatewaySecret

	chatID, err := bot.ensureChat(context.Background(), 111)
	if err != nil {
		t.Fatalf("ensureChat: %v", err)
	}
	if chatID != "fresh-sess" {
		t.Fatalf("chatID = %q, want fresh-sess", chatID)
	}

	bot.mu.Lock()
	isFresh := bot.freshChatIDs[chatID]
	bot.mu.Unlock()

	if isFresh {
		t.Fatal("freshChatIDs[fresh-sess] should remain false after endpoint resolution")
	}
}

func TestHandleUpdateUsesPerChatFocus(t *testing.T) {
	gatewaySecret := "mock-jwt"
	var (
		directiveMessages []string
		chatMessages      []string
	)
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if writeMockTelegramEndpointResolve(t, w, r, map[string]string{"200": "sess-chat-200"}) {
			return
		}
		switch {
		case r.URL.Path == "/api/directives" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"directive_id": "dir-chat-100"})
		case r.URL.Path == "/api/directives/dir-chat-100/message" && r.Method == http.MethodPost:
			var req map[string]string
			_ = json.NewDecoder(r.Body).Decode(&req)
			directiveMessages = append(directiveMessages, req["content"])
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/api/navi/chats" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case r.URL.Path == "/api/directives" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case r.URL.Path == "/api/navi/chats" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"chat_id": "sess-chat-200"})
		case r.URL.Path == "/api/navi/chats/sess-chat-200/message" && r.Method == http.MethodPost:
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			if content, ok := req["content"].(string); ok {
				chatMessages = append(chatMessages, content)
			}
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gw.Close()

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"message_id": 1001,
				},
			})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/sendChatAction") || strings.HasSuffix(r.URL.Path, "/editMessageText") {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer tg.Close()

	bot := NewBot(Config{
		Name:          "telegram-test",
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		OwnerChatID:   100,
		AllowFrom:     []int64{200},
		apiURL:        tg.URL,
	})
	bot.token = gatewaySecret

	makeUpdate := func(chatID int64, text string) tgUpdate {
		u := tgUpdate{
			UpdateID: 1,
			Message: &tgMessage{
				Text: text,
			},
		}
		u.Message.Chat.ID = chatID
		u.Message.MessageID = 1
		return u
	}

	bot.handleUpdate(context.Background(), makeUpdate(100, "/new roadmap"))
	bot.handleUpdate(context.Background(), makeUpdate(200, "hello from chat"))
	bot.handleUpdate(context.Background(), makeUpdate(100, "hello from directive chat"))

	if len(directiveMessages) != 1 || directiveMessages[0] != "hello from directive chat" {
		t.Fatalf("directive messages = %#v, want only chat 100 content", directiveMessages)
	}
	if len(chatMessages) != 1 || chatMessages[0] != "hello from chat" {
		t.Fatalf("chat messages = %#v, want only chat 200 content", chatMessages)
	}
}

func TestHandleUpdateNaviClearsSingleChatDirectiveFocus(t *testing.T) {
	gatewaySecret := "mock-jwt"
	var (
		directiveMessages []string
		chatMessages      []string
	)
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if writeMockTelegramEndpointResolve(t, w, r, map[string]string{"100": "sess-single"}) {
			return
		}
		switch {
		case r.URL.Path == "/api/directives" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"directive_id": "dir-single"})
		case r.URL.Path == "/api/directives/dir-single/message" && r.Method == http.MethodPost:
			var req map[string]string
			_ = json.NewDecoder(r.Body).Decode(&req)
			directiveMessages = append(directiveMessages, req["content"])
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/api/navi/chats" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case r.URL.Path == "/api/directives" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case r.URL.Path == "/api/navi/chats" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"chat_id": "sess-single"})
		case r.URL.Path == "/api/navi/chats/sess-single/message" && r.Method == http.MethodPost:
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			if content, ok := req["content"].(string); ok {
				chatMessages = append(chatMessages, content)
			}
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gw.Close()

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"message_id": 1002,
				},
			})
		case strings.HasSuffix(r.URL.Path, "/sendChatAction"), strings.HasSuffix(r.URL.Path, "/editMessageText"):
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer tg.Close()

	bot := NewBot(Config{
		Name:          "telegram-test",
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		OwnerChatID:   100,
		apiURL:        tg.URL,
	})
	bot.token = gatewaySecret

	makeUpdate := func(text string) tgUpdate {
		u := tgUpdate{
			UpdateID: 1,
			Message: &tgMessage{
				Text: text,
			},
		}
		u.Message.Chat.ID = 100
		u.Message.MessageID = 1
		return u
	}

	bot.handleUpdate(context.Background(), makeUpdate("/new roadmap"))
	bot.handleUpdate(context.Background(), makeUpdate("/navi"))
	bot.handleUpdate(context.Background(), makeUpdate("route me to session"))

	if len(directiveMessages) != 0 {
		t.Fatalf("directive messages = %#v, want none after /navi focus switch", directiveMessages)
	}
	if len(chatMessages) != 1 || chatMessages[0] != "route me to session" {
		t.Fatalf("chat messages = %#v, want chat-bound content", chatMessages)
	}
}

func TestHandleUpdateUnknownSlashCommandDoesNotPostAsChatContent(t *testing.T) {
	gatewaySecret := "mock-jwt"
	var personaGatewayHits int
	var sentTexts []string

	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/api/navi/persona" {
			personaGatewayHits++
		}
		switch {
		case r.URL.Path == "/api/navi/chats" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case r.URL.Path == "/api/directives" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gw.Close()

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			if text, _ := req["text"].(string); text != "" {
				sentTexts = append(sentTexts, text)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": map[string]any{"message_id": 3001},
			})
		case strings.HasSuffix(r.URL.Path, "/sendChatAction"), strings.HasSuffix(r.URL.Path, "/editMessageText"):
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer tg.Close()

	bot := NewBot(Config{
		Name:          "telegram-test",
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		OwnerChatID:   100,
		apiURL:        tg.URL,
	})
	bot.token = gatewaySecret

	u := tgUpdate{
		UpdateID: 1,
		Message: &tgMessage{
			Text:      "/persona",
			MessageID: 10,
		},
	}
	u.Message.Chat.ID = 100

	bot.handleUpdate(context.Background(), u)

	if personaGatewayHits != 0 {
		t.Fatalf("persona gateway hits = %d, want 0", personaGatewayHits)
	}
	if len(sentTexts) != 1 {
		t.Fatalf("expected one Telegram reply, got %d", len(sentTexts))
	}
	if !strings.Contains(sentTexts[0], "Unknown command.") {
		t.Fatalf("unexpected reply: %q", sentTexts[0])
	}
}

func TestHandleUpdateDeferredSessionMessageShowsApprovalInsteadOfThinking(t *testing.T) {
	gatewaySecret := "mock-jwt"
	var sentTexts []string
	var sentReplyMarkup []map[string]any
	var editedTexts []string
	var editedReplyMarkup []map[string]any

	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if writeMockTelegramEndpointResolve(t, w, r, map[string]string{"100": "sess-paused"}) {
			return
		}
		switch {
		case r.URL.Path == "/api/navi/chats/sess-paused/message" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":                 "queued",
				"inbox_item_id":          "inbox-paused",
				"queue_action":           "defer",
				"inbox_status":           "deferred",
				"classified_reason":      "session has a paused foreground run waiting for external input",
				"blocked_on_proposal_id": "prop-123",
				"pause_reason":           "approval required",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gw.Close()

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			if text, _ := req["text"].(string); text != "" {
				sentTexts = append(sentTexts, text)
			}
			if replyMarkup, ok := req["reply_markup"].(map[string]any); ok {
				sentReplyMarkup = append(sentReplyMarkup, replyMarkup)
			} else {
				sentReplyMarkup = append(sentReplyMarkup, nil)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"message_id": 2001,
				},
			})
		case strings.HasSuffix(r.URL.Path, "/editMessageText"):
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			if text, _ := req["text"].(string); text != "" {
				editedTexts = append(editedTexts, text)
			}
			if replyMarkup, ok := req["reply_markup"].(map[string]any); ok {
				editedReplyMarkup = append(editedReplyMarkup, replyMarkup)
			} else {
				editedReplyMarkup = append(editedReplyMarkup, nil)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		case strings.HasSuffix(r.URL.Path, "/sendChatAction"):
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer tg.Close()

	bot := NewBot(Config{
		Name:          "telegram-test",
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		OwnerChatID:   100,
		apiURL:        tg.URL,
	})
	bot.token = gatewaySecret
	bot.bindSessionToChat(100, "sess-paused")

	u := tgUpdate{
		UpdateID: 1,
		Message: &tgMessage{
			Text:      "hello while paused",
			MessageID: 10,
		},
	}
	u.Message.Chat.ID = 100

	bot.handleUpdate(context.Background(), u)

	// No "Thinking…" placeholder is posted; the deferred/approval notice is
	// delivered as a single fresh message carrying the approval reply markup.
	if len(sentTexts) != 1 {
		t.Fatalf("expected one message send, got %d", len(sentTexts))
	}
	if !strings.Contains(sentTexts[0], "Approval needed before this chat can continue.") {
		t.Fatalf("expected approval notice, got %q", sentTexts[0])
	}
	if !strings.Contains(sentTexts[0], "Proposal: prop-123") {
		t.Fatalf("expected proposal id in notice, got %q", sentTexts[0])
	}
	if len(editedTexts) != 0 {
		t.Fatalf("expected no placeholder edit, got %v", editedTexts)
	}
	if len(sentReplyMarkup) != 1 || sentReplyMarkup[0] == nil {
		t.Fatal("expected approval reply markup on the notice message")
	}
}

func TestHandleUpdateSessionMessageStartsTypingBeforeRetryableQueueResponse(t *testing.T) {
	gatewaySecret := "mock-jwt"
	const (
		naviChatID     = "sess-retry"
		telegramChatID = int64(100)
	)

	var (
		mu              sync.Mutex
		messageAttempts int
		idempotencyKeys []string
		sentTexts       []string
	)
	typingStarted := make(chan struct{})

	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if writeMockTelegramEndpointResolve(t, w, r, map[string]string{"100": naviChatID}) {
			return
		}
		switch {
		case r.URL.Path == "/api/navi/chats/"+naviChatID+"/message" && r.Method == http.MethodPost:
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode queue request: %v", err)
			}
			key, _ := req["idempotency_key"].(string)
			mu.Lock()
			messageAttempts++
			idempotencyKeys = append(idempotencyKeys, key)
			attempt := messageAttempts
			mu.Unlock()

			select {
			case <-typingStarted:
			case <-time.After(250 * time.Millisecond):
				t.Fatal("expected typing indicator before queue response")
			}

			if attempt == 1 {
				w.WriteHeader(http.StatusGatewayTimeout)
				_, _ = w.Write([]byte(`timeout`))
				return
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":        "queued",
				"queue_action":  "append",
				"inbox_status":  "pending",
				"inbox_item_id": "inbox-1",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer gw.Close()

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			text, _ := req["text"].(string)
			mu.Lock()
			sentTexts = append(sentTexts, text)
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"message_id": 9001,
				},
			})
		case strings.HasSuffix(r.URL.Path, "/sendChatAction"):
			select {
			case <-typingStarted:
			default:
				close(typingStarted)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		case strings.HasSuffix(r.URL.Path, "/editMessageText"):
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer tg.Close()

	bot := NewBot(Config{
		Name:          "telegram",
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		OwnerChatID:   telegramChatID,
		apiURL:        tg.URL,
	})
	bot.token = gatewaySecret
	bot.chatBackoff = func(int) time.Duration { return 0 }
	bot.bindSessionToChat(telegramChatID, naviChatID)

	done := make(chan struct{})
	go func() {
		defer close(done)
		u := tgUpdate{
			UpdateID: 1,
			Message: &tgMessage{
				Text:      "retry me",
				MessageID: 44,
			},
		}
		u.Message.Chat.ID = telegramChatID
		bot.handleUpdate(context.Background(), u)
	}()

	select {
	case <-typingStarted:
	case <-time.After(time.Second):
		t.Fatal("expected typing indicator before queue retries finished")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for handleUpdate")
	}

	mu.Lock()
	defer mu.Unlock()
	if messageAttempts != 2 {
		t.Fatalf("expected 2 queue attempts, got %d", messageAttempts)
	}
	if len(idempotencyKeys) != 2 {
		t.Fatalf("expected 2 idempotency keys, got %d", len(idempotencyKeys))
	}
	if idempotencyKeys[0] == "" || idempotencyKeys[0] != idempotencyKeys[1] {
		t.Fatalf("expected stable idempotency key across retries, got %v", idempotencyKeys)
	}
	if len(sentTexts) != 0 {
		t.Fatalf("expected no placeholder message to be sent, got %v", sentTexts)
	}
	if _, active := bot.chatStreamSnapshot(naviChatID); active {
		t.Fatalf("expected no placeholder stream after successful retry, got active=%v", active)
	}
}

func TestQueuedMessageStartsThinkingSkipsSupersededDuplicates(t *testing.T) {
	if queuedMessageStartsThinking(queuedSessionMessageResponse{InboxStatus: "superseded", QueueAction: "supersede"}) {
		t.Fatal("superseded duplicate messages should not start or keep a thinking placeholder")
	}
	if !queuedMessageStartsThinking(queuedSessionMessageResponse{InboxStatus: "pending", QueueAction: "append"}) {
		t.Fatal("pending messages should start a thinking placeholder")
	}
}

func TestHandleUpdateSessionMessageFailureSendsErrorReply(t *testing.T) {
	gatewaySecret := "mock-jwt"
	const (
		naviChatID     = "sess-fail"
		telegramChatID = int64(100)
	)

	var (
		mu              sync.Mutex
		messageAttempts int
		sentTexts       []string
		editedTexts     []string
	)
	typingStarted := make(chan struct{})

	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if writeMockTelegramEndpointResolve(t, w, r, map[string]string{"100": naviChatID}) {
			return
		}
		if r.URL.Path != "/api/navi/chats/"+naviChatID+"/message" || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		mu.Lock()
		messageAttempts++
		mu.Unlock()
		select {
		case <-typingStarted:
		case <-time.After(250 * time.Millisecond):
			t.Fatal("expected typing indicator before queue failure")
		}
		w.WriteHeader(http.StatusGatewayTimeout)
		_, _ = w.Write([]byte(`timeout`))
	}))
	defer gw.Close()

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			text, _ := req["text"].(string)
			mu.Lock()
			sentTexts = append(sentTexts, text)
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"message_id": 9002,
				},
			})
		case strings.HasSuffix(r.URL.Path, "/editMessageText"):
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			text, _ := req["text"].(string)
			mu.Lock()
			editedTexts = append(editedTexts, text)
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		case strings.HasSuffix(r.URL.Path, "/sendChatAction"):
			select {
			case <-typingStarted:
			default:
				close(typingStarted)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer tg.Close()

	bot := NewBot(Config{
		Name:          "telegram",
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		OwnerChatID:   telegramChatID,
		apiURL:        tg.URL,
	})
	bot.token = gatewaySecret
	bot.chatBackoff = func(int) time.Duration { return 0 }
	bot.bindSessionToChat(telegramChatID, naviChatID)

	u := tgUpdate{
		UpdateID: 1,
		Message: &tgMessage{
			Text:      "will fail",
			MessageID: 55,
		},
	}
	u.Message.Chat.ID = telegramChatID
	bot.handleUpdate(context.Background(), u)

	mu.Lock()
	defer mu.Unlock()
	if messageAttempts != 3 {
		t.Fatalf("expected 3 queue attempts, got %d", messageAttempts)
	}
	// With no placeholder, the terminal failure is delivered as a single fresh
	// message — not an edit of a "Thinking…" placeholder.
	if len(sentTexts) != 1 || !strings.Contains(sentTexts[0], "NAVI Communication Error:") {
		t.Fatalf("expected one error message send, got %v", sentTexts)
	}
	if len(editedTexts) != 0 {
		t.Fatalf("expected no placeholder edit, got %v", editedTexts)
	}
	if _, active := bot.chatStreamSnapshot(naviChatID); active {
		t.Fatal("expected no placeholder stream after terminal failure")
	}
	bot.mu.Lock()
	if stop := bot.chatTypingStop[naviChatID]; stop != nil {
		bot.mu.Unlock()
		t.Fatal("expected typing state to be cleared after terminal failure")
	}
	bot.mu.Unlock()
}

func TestQueueSessionMessageRejectsMalformedJSON(t *testing.T) {
	gatewaySecret := "mock-jwt"
	var attempts int

	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != gatewaySecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		attempts++
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{not-json`))
	}))
	defer gw.Close()

	bot := NewBot(Config{
		Name:          "telegram",
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		OwnerChatID:   100,
	})
	bot.token = gatewaySecret
	bot.chatBackoff = func(int) time.Duration { return 0 }

	_, err := bot.queueSessionMessage(context.Background(), "sess-malformed", "hello", "100:0:1", "telegram:100:0:1", "")
	if err == nil {
		t.Fatal("expected malformed queue response error")
	}
	if !strings.Contains(err.Error(), "decode queued chat message response") {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected malformed response to skip retries, got %d attempts", attempts)
	}
}

// TestRecoverFocus_PopulatesSessionChat verifies that recoverFocus sets naviChatToTelegramChat
// to OwnerChatID so that events for a recovered chat are delivered somewhere
// rather than being silently dropped.
func TestRecoverFocus_PopulatesSessionChat(t *testing.T) {
	gatewaySecret := "tok"
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/navi/chats" && r.Method == "GET" {
			w.WriteHeader(200)
			json.NewEncoder(w).Encode([]map[string]string{{"chat_id": "recovered-sess"}})
			return
		}
		w.WriteHeader(404)
	}))
	defer gw.Close()

	bot := NewBot(Config{
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		OwnerChatID:   999,
	})
	bot.token = gatewaySecret

	if err := bot.recoverFocus(context.Background()); err != nil {
		t.Fatalf("recoverFocus: %v", err)
	}

	bot.mu.Lock()
	gotChatID := bot.naviChatToTelegramChat["recovered-sess"]
	gotNaviID := bot.activeChatID
	bot.mu.Unlock()

	if gotNaviID != "recovered-sess" {
		t.Fatalf("activeChatID = %q, want recovered-sess", gotNaviID)
	}
	if gotChatID != 999 {
		t.Fatalf("naviChatToTelegramChat[recovered-sess] = %d, want 999", gotChatID)
	}
}

// TestRecoverFocus_NoOwnerChatID verifies that when OwnerChatID == 0,
// recoverFocus still sets activeChatID but does NOT set naviChatToTelegramChat (keeps the
// zero value so callers can tell there is no routing target).
func TestRecoverFocus_NoOwnerChatID(t *testing.T) {
	gatewaySecret := "tok"
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/navi/chats" && r.Method == "GET" {
			w.WriteHeader(200)
			json.NewEncoder(w).Encode([]map[string]string{{"chat_id": "recovered-sess"}})
			return
		}
		w.WriteHeader(404)
	}))
	defer gw.Close()

	bot := NewBot(Config{
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		OwnerChatID:   0, // no owner chat
	})
	bot.token = gatewaySecret

	if err := bot.recoverFocus(context.Background()); err != nil {
		t.Fatalf("recoverFocus: %v", err)
	}

	bot.mu.Lock()
	gotChatID := bot.naviChatToTelegramChat["recovered-sess"]
	bot.mu.Unlock()

	if gotChatID != 0 {
		t.Fatalf("naviChatToTelegramChat[recovered-sess] = %d, want 0 when OwnerChatID is not set", gotChatID)
	}
}

// wsConnectParams holds the connect-frame params sent by the bot to the WS server.
type wsConnectParams struct {
	ChatID    string `json:"chat_id"`
	AfterSeq  int64  `json:"after_seq"`
	ReplayRaw *bool  `json:"replay,omitempty"`
	Stream    string `json:"stream"`
}

// startWsTestServer starts a WS test server that captures the connect params from
// the first wsLivePoller connection and sends a single event then closes.
func startWsTestServer(t *testing.T, eventToSend map[string]any) (*httptest.Server, chan wsConnectParams) {
	t.Helper()
	paramsCh := make(chan wsConnectParams, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws/live" {
			w.WriteHeader(404)
			return
		}
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Read the connect frame
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var req struct {
			Params wsConnectParams `json:"params"`
		}
		if json.Unmarshal(raw, &req) != nil {
			return
		}
		paramsCh <- req.Params

		// Acknowledge
		_ = conn.WriteJSON(map[string]any{
			"type": "res",
			"id":   "connect",
			"ok":   true,
		})

		// Send one event if requested
		if eventToSend != nil {
			_ = conn.WriteJSON(map[string]any{
				"type":  "event",
				"event": eventToSend,
			})
		}

		// Keep connection open briefly so the bot can read
		time.Sleep(200 * time.Millisecond)
	}))
	return srv, paramsCh
}

// makeTestBotForWS creates a Bot wired to use srv as the gateway/WS endpoint.
func makeTestBotForWS(t *testing.T, srv *httptest.Server, ownerChatID int64) *Bot {
	t.Helper()
	gatewaySecret := "tok"
	// Minimal gateway HTTP handler: health + session creation
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(200)
			return
		}
		if r.URL.Path == "/api/connectors" {
			w.WriteHeader(200)
			return
		}
		if r.URL.Path == "/api/navi/chats" && r.Method == "GET" {
			w.WriteHeader(200)
			json.NewEncoder(w).Encode([]map[string]string{})
			return
		}
		if r.URL.Path == "/api/directives" {
			w.WriteHeader(200)
			json.NewEncoder(w).Encode([]map[string]string{})
			return
		}
		w.WriteHeader(200)
	}))
	t.Cleanup(gw.Close)

	bot := NewBot(Config{
		GatewayURL:    srv.URL, // WS live goes to srv
		GatewaySecret: gatewaySecret,
		OwnerChatID:   ownerChatID,
	})
	bot.token = gatewaySecret
	return bot
}

// TestWsLivePoller_FreshSession_OmitsReplayFalse checks that for a freshly created
// session (freshChatIDs set), the wsLivePoller sends "replay":false so
// the gateway does NOT replay historical events (prevents stale response delivery).
func TestWsLivePoller_FreshSession_OmitsReplayFalse(t *testing.T) {
	srv, paramsCh := startWsTestServer(t, nil)
	defer srv.Close()

	bot := makeTestBotForWS(t, srv, 0)
	const sess = "fresh-sess-1"
	bot.mu.Lock()
	bot.activeChatID = sess
	bot.freshChatIDs[sess] = true
	bot.naviChatToTelegramChat[sess] = 42
	bot.telegramChatToNaviChat[42] = sess
	bot.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go bot.wsLivePoller(ctx)

	select {
	case p := <-paramsCh:
		if p.ChatID != sess {
			t.Fatalf("chat_id = %q, want %q", p.ChatID, sess)
		}
		if p.ReplayRaw == nil || *p.ReplayRaw {
			t.Fatal("fresh chat: replay should be false to skip historical events")
		}
		if p.AfterSeq != 0 {
			t.Fatalf("after_seq = %d, want 0 for a fresh chat", p.AfterSeq)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for wsLivePoller to connect")
	}
}

// TestWsLivePoller_RecoveredSession_SendsReplayFalse checks that for a recovered
// session (NOT in freshChatIDs, lastSeq==0), the bot sends "replay":false to
// skip historical messages and avoid re-delivering them to Telegram.
func TestWsLivePoller_RecoveredSession_SendsReplayFalse(t *testing.T) {
	srv, paramsCh := startWsTestServer(t, nil)
	defer srv.Close()

	bot := makeTestBotForWS(t, srv, 0)
	const sess = "recovered-sess-1"
	bot.mu.Lock()
	bot.activeChatID = sess
	bot.naviChatToTelegramChat[sess] = 42
	bot.telegramChatToNaviChat[42] = sess
	bot.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go bot.wsLivePoller(ctx)

	select {
	case p := <-paramsCh:
		if p.ChatID != sess {
			t.Fatalf("chat_id = %q, want %q", p.ChatID, sess)
		}
		if p.ReplayRaw == nil || *p.ReplayRaw {
			t.Fatal("recovered chat with lastSeq==0: replay should be false to skip history")
		}
		if p.AfterSeq != 0 {
			t.Fatalf("after_seq = %d, want 0", p.AfterSeq)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for wsLivePoller to connect")
	}
}

func TestWsLivePoller_FreshSessionReconnectAfterNoEventsOmitsReplayFalse(t *testing.T) {
	paramsCh := make(chan wsConnectParams, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws/live" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var req struct {
			Params wsConnectParams `json:"params"`
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			return
		}
		paramsCh <- req.Params

		_ = conn.WriteJSON(map[string]any{
			"type": "res",
			"id":   "connect",
			"ok":   true,
			"result": map[string]any{
				"after_seq":  0,
				"cursor_seq": 0,
			},
		})
		time.Sleep(20 * time.Millisecond)
	}))
	defer srv.Close()

	bot := makeTestBotForWS(t, srv, 0)
	const sess = "fresh-reconnect-sess"
	bot.mu.Lock()
	bot.activeChatID = sess
	bot.freshChatIDs[sess] = true
	bot.naviChatToTelegramChat[sess] = 42
	bot.telegramChatToNaviChat[42] = sess
	bot.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go bot.wsLivePoller(ctx)

	first := wsConnectParams{}
	second := wsConnectParams{}
	select {
	case first = <-paramsCh:
	case <-ctx.Done():
		t.Fatal("timed out waiting for first ws connect")
	}
	select {
	case second = <-paramsCh:
	case <-ctx.Done():
		t.Fatal("timed out waiting for second ws connect")
	}

	if first.ReplayRaw == nil || *first.ReplayRaw {
		t.Fatalf("fresh first connect should send replay:false to skip historical events, got %+v", first)
	}
	if second.ReplayRaw != nil && !*second.ReplayRaw {
		t.Fatalf("fresh reconnect after zero seq (reconnect_zero_seq path) should not send replay:false, got %+v", second)
	}
	if second.AfterSeq != 0 {
		t.Fatalf("second after_seq = %d, want 0", second.AfterSeq)
	}
}

// TestWsLivePoller_DeliversFinalReply verifies that when the WS server sends an
// assistant.message.completed event, the bot calls finalizeSessionMessage, which
// results in a Telegram sendMessage call for the session's chat.
func TestWsLivePoller_DeliversFinalReply(t *testing.T) {
	const (
		sess    = "test-session"
		chatID  = int64(12345)
		content = "Hello from NAVI!"
	)

	// Track sendMessage calls to Telegram API
	tgMsgs := make(chan string, 5)

	// Fake Telegram API
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if text, ok := body["text"].(string); ok {
				tgMsgs <- text
			}
			w.WriteHeader(200)
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"message_id": 1}})
			return
		}
		w.WriteHeader(200)
	}))
	defer tg.Close()

	// WS server that sends assistant.message.completed once the bot connects
	completedEvent := map[string]any{
		"type": "assistant.message.completed",
		"seq":  int64(1),
		"payload": map[string]any{
			"content":      content,
			"message_kind": "reply",
		},
	}
	wsSrv, _ := startWsTestServer(t, completedEvent)
	defer wsSrv.Close()

	bot := NewBot(Config{
		GatewayURL:    wsSrv.URL,
		GatewaySecret: "tok",
		APIURL:        tg.URL,
		TelegramToken: "test-token",
		OwnerChatID:   chatID,
	})
	bot.token = "tok"
	bot.mu.Lock()
	bot.activeChatID = sess
	bot.freshChatIDs[sess] = true
	bot.naviChatToTelegramChat[sess] = chatID
	bot.telegramChatToNaviChat[chatID] = sess
	bot.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go bot.wsLivePoller(ctx)

	select {
	case got := <-tgMsgs:
		if got != content {
			t.Fatalf("telegram sendMessage text = %q, want %q", got, content)
		}
	case <-ctx.Done():
		t.Fatal("timed out: finalizeSessionMessage did not deliver the reply to Telegram")
	}
}

func TestWsLivePoller_AssistantCompletedTimeoutFinalizesPlaceholderWithinGracePeriod(t *testing.T) {
	const (
		sess          = "assistant-timeout-session"
		chatID        = int64(12345)
		placeholderID = int64(700)
	)

	var (
		editedText string
		editCount  int
		sendCount  int
	)

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/editMessageText"):
			editCount++
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			editedText, _ = body["text"].(string)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			sendCount++
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"message_id": 1}})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer tg.Close()

	timeoutContent := "The request took too long. Try /status, wait a moment, or start a fresh chat with /new."
	completedEvent := map[string]any{
		"type": "assistant.message.completed",
		"seq":  int64(1),
		"payload": map[string]any{
			"content":      timeoutContent,
			"message_kind": "reply",
		},
	}
	wsSrv, _ := startWsTestServer(t, completedEvent)
	defer wsSrv.Close()

	bot := NewBot(Config{
		GatewayURL:    wsSrv.URL,
		GatewaySecret: "tok",
		APIURL:        tg.URL,
		TelegramToken: "test-token",
		OwnerChatID:   chatID,
	})
	bot.token = "tok"
	bot.mu.Lock()
	bot.activeChatID = sess
	bot.freshChatIDs[sess] = true
	bot.naviChatToTelegramChat[sess] = chatID
	bot.telegramChatToNaviChat[chatID] = sess
	stopCalled := false
	bot.chatTypingStop[sess] = func() { stopCalled = true }
	bot.chatStream[sess] = streamMessage{
		ChatID:    chatID,
		MessageID: placeholderID,
		Content:   telegramThinkingPlaceholder,
	}
	bot.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go bot.wsLivePoller(ctx)

	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if editCount == 1 {
			if editedText != timeoutContent {
				t.Fatalf("unexpected terminal edit text %q", editedText)
			}
			if sendCount != 0 {
				t.Fatalf("expected placeholder edit instead of new sendMessage, got %d send calls", sendCount)
			}
			bot.mu.Lock()
			_, exists := bot.chatStream[sess]
			_, typingActive := bot.chatTypingStop[sess]
			bot.mu.Unlock()
			if exists {
				t.Fatal("expected chat stream to clear after terminal assistant.message.completed event")
			}
			if typingActive || !stopCalled {
				t.Fatal("expected assistant.message.completed to stop typing and clear typing state")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for assistant.message.completed timeout to finalize the placeholder: editCount=%d editedText=%q sendCount=%d", editCount, editedText, sendCount)
}

func TestWsLivePoller_RunCompletedRepairsPlaceholderFromFinalMessageID(t *testing.T) {
	const (
		sess           = "run-completed-repair-session"
		chatID         = int64(12345)
		placeholderID  = int64(703)
		finalMessageID = "msg-final-1"
	)

	var (
		editedText string
		editCount  int
		sendCount  int
		chatReads  int
	)

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/editMessageText"):
			editCount++
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			editedText, _ = body["text"].(string)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			sendCount++
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"message_id": 1}})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer tg.Close()

	finalContent := "Recovered final reply from chat history."
	gatewaySecret := "tok"
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/ws/live":
			conn, err := wsUpgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()

			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var req struct {
				Params wsConnectParams `json:"params"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return
			}

			_ = conn.WriteJSON(map[string]any{
				"type": "res",
				"id":   "connect",
				"ok":   true,
				"result": map[string]any{
					"after_seq":  0,
					"cursor_seq": 0,
				},
			})
			_ = conn.WriteJSON(map[string]any{
				"type": "event",
				"event": map[string]any{
					"type": "run.completed",
					"seq":  int64(1),
					"payload": map[string]any{
						"final_message_id": finalMessageID,
					},
				},
			})
			time.Sleep(100 * time.Millisecond)
		case r.URL.Path == "/api/navi/chats/"+sess:
			if r.Header.Get("X-API-Key") != gatewaySecret {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			chatReads++
			now := time.Now().UTC()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"chat_id":              sess,
				"experience_mode":      "standard",
				"runtime_session_kind": "user",
				"messages": []map[string]any{
					{
						"message_id":   finalMessageID,
						"role":         "navi",
						"content":      finalContent,
						"created_at":   now,
						"message_kind": "reply",
					},
				},
				"created_at": now,
				"updated_at": now,
			})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer gw.Close()

	bot := NewBot(Config{
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		APIURL:        tg.URL,
		TelegramToken: "test-token",
		OwnerChatID:   chatID,
	})
	bot.token = gatewaySecret
	bot.completionRepairGrace = 10 * time.Millisecond
	bot.mu.Lock()
	bot.activeChatID = sess
	bot.freshChatIDs[sess] = true
	bot.naviChatToTelegramChat[sess] = chatID
	bot.telegramChatToNaviChat[chatID] = sess
	stopCalled := false
	bot.chatTypingStop[sess] = func() { stopCalled = true }
	bot.chatStream[sess] = streamMessage{
		ChatID:    chatID,
		MessageID: placeholderID,
		Content:   telegramThinkingPlaceholder,
	}
	bot.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go bot.wsLivePoller(ctx)

	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if editCount == 1 {
			if editedText != finalContent {
				t.Fatalf("unexpected repaired terminal edit text %q", editedText)
			}
			if sendCount != 0 {
				t.Fatalf("expected placeholder edit instead of new sendMessage, got %d send calls", sendCount)
			}
			if chatReads == 0 {
				t.Fatal("expected run.completed repair to fetch chat history")
			}
			bot.mu.Lock()
			_, exists := bot.chatStream[sess]
			_, typingActive := bot.chatTypingStop[sess]
			_, repairActive := bot.chatRepair[sess]
			bot.mu.Unlock()
			if exists {
				t.Fatal("expected chat stream to clear after run.completed repair")
			}
			if typingActive || repairActive || !stopCalled {
				t.Fatal("expected run.completed repair to stop typing and clear repair state")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for run.completed repair to finalize the placeholder: editCount=%d editedText=%q chatReads=%d sendCount=%d", editCount, editedText, chatReads, sendCount)
}

func TestTelegramDiagnosticQueryCompletedDeliversReplyWithoutPlaceholder(t *testing.T) {
	const (
		sess           = "diag-query-repair-session"
		chatID         = int64(12347)
		finalMessageID = "msg-final-diagnostics"
	)

	diagnosticPrompt := "What errors have occurred recently?"
	finalContent := "Recent NAVI failures in the last 24h:\n- runtime/run_timeout: 2\n\nMost recent occurrences:\n- runtime/run_timeout (session sess-diag-runtime): context deadline exceeded"

	var (
		editedText string
		editCount  int
		sendCount  int
		chatReads  int
		sentTexts  []string
	)

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			sendCount++
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if text, _ := body["text"].(string); text != "" {
				sentTexts = append(sentTexts, text)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"message_id": 811,
				},
			})
		case strings.HasSuffix(r.URL.Path, "/editMessageText"):
			editCount++
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			editedText, _ = body["text"].(string)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer tg.Close()

	gatewaySecret := "tok"
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/connectors/endpoints/resolve" && r.Method == http.MethodPost:
			if r.Header.Get("X-API-Key") != gatewaySecret {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if writeMockTelegramEndpointResolve(t, w, r, map[string]string{"12347": sess}) {
				return
			}
		case r.URL.Path == "/ws/live":
			conn, err := wsUpgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()

			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var req struct {
				Params wsConnectParams `json:"params"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return
			}

			_ = conn.WriteJSON(map[string]any{
				"type": "res",
				"id":   "connect",
				"ok":   true,
				"result": map[string]any{
					"after_seq":  0,
					"cursor_seq": 0,
				},
			})
			_ = conn.WriteJSON(map[string]any{
				"type": "event",
				"event": map[string]any{
					"type": "assistant.message.completed",
					"seq":  int64(1),
					"payload": map[string]any{
						"message_id":   finalMessageID,
						"content":      finalContent,
						"message_kind": "reply",
					},
				},
			})
			time.Sleep(100 * time.Millisecond)
		case r.URL.Path == "/api/navi/chats" && r.Method == http.MethodPost:
			if r.Header.Get("X-API-Key") != gatewaySecret {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"chat_id": sess})
		case r.URL.Path == "/api/navi/chats/"+sess+"/message" && r.Method == http.MethodPost:
			if r.Header.Get("X-API-Key") != gatewaySecret {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":        "accepted",
				"queue_action":  "append",
				"inbox_status":  "pending",
				"inbox_item_id": "inbox-diag-1",
			})
		case r.URL.Path == "/api/navi/chats/"+sess && r.Method == http.MethodGet:
			if r.Header.Get("X-API-Key") != gatewaySecret {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			chatReads++
			now := time.Now().UTC()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"chat_id":              sess,
				"experience_mode":      "standard",
				"runtime_session_kind": "user",
				"messages": []map[string]any{
					{
						"message_id":   finalMessageID,
						"role":         "navi",
						"content":      finalContent,
						"created_at":   now,
						"message_kind": "reply",
					},
				},
				"created_at": now,
				"updated_at": now,
			})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer gw.Close()

	bot := NewBot(Config{
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		APIURL:        tg.URL,
		TelegramToken: "test-token",
		OwnerChatID:   chatID,
	})
	bot.token = gatewaySecret
	bot.completionRepairGrace = 10 * time.Millisecond

	u := tgUpdate{UpdateID: 1, Message: &tgMessage{Text: diagnosticPrompt, MessageID: 44}}
	u.Message.Chat.ID = chatID
	bot.handleUpdate(context.Background(), u)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go bot.wsLivePoller(ctx)

	// The reply content is carried inline on assistant.message.completed and
	// delivered as a single fresh message — no "Thinking…" placeholder is ever
	// posted or edited.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if sendCount >= 1 {
			if sentTexts[0] != finalContent {
				t.Fatalf("expected diagnostic reply %q, got %q", finalContent, sentTexts[0])
			}
			if editCount != 0 {
				t.Fatalf("expected no placeholder edit, got %d edits (%q)", editCount, editedText)
			}
			bot.mu.Lock()
			_, exists := bot.chatStream[sess]
			_, typingActive := bot.chatTypingStop[sess]
			_, repairActive := bot.chatRepair[sess]
			bot.mu.Unlock()
			if exists {
				t.Fatal("expected no placeholder stream after completion")
			}
			if typingActive || repairActive {
				t.Fatal("expected completion to clear typing and repair state")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for diagnostic reply delivery: editCount=%d editedText=%q chatReads=%d sendCount=%d sentTexts=%#v", editCount, editedText, chatReads, sendCount, sentTexts)
}

func TestWsLivePoller_WatchdogRepairsCompletedPlaceholderWithoutTerminalLiveEvent(t *testing.T) {
	const (
		sess           = "watchdog-completed-session"
		chatID         = int64(12346)
		placeholderID  = int64(704)
		finalMessageID = "msg-final-watchdog"
	)

	var (
		editedText string
		editCount  int
	)

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/editMessageText"):
			editCount++
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			editedText, _ = body["text"].(string)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer tg.Close()

	finalContent := "Recovered by watchdog after missing live terminal events."
	gatewaySecret := "tok"
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/ws/live":
			conn, err := wsUpgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			_, _, err = conn.ReadMessage()
			if err != nil {
				return
			}
			_ = conn.WriteJSON(map[string]any{
				"type": "res",
				"id":   "connect",
				"ok":   true,
				"result": map[string]any{
					"after_seq":  0,
					"cursor_seq": 0,
				},
			})
			time.Sleep(200 * time.Millisecond)
		case r.URL.Path == "/api/navi/chats/"+sess+"/runtime_summary":
			if r.Header.Get("X-API-Key") != gatewaySecret {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			now := time.Now().UTC()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"chat_id": sess,
				"run": map[string]any{
					"run_id":     "run-watchdog-complete",
					"status":     "completed",
					"updated_at": now,
				},
				"terminal_event": map[string]any{
					"type": "run.completed",
					"seq":  9,
					"payload": map[string]any{
						"final_message_id": finalMessageID,
					},
				},
			})
		case r.URL.Path == "/api/navi/chats/"+sess:
			if r.Header.Get("X-API-Key") != gatewaySecret {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			now := time.Now().UTC()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"chat_id":              sess,
				"experience_mode":      "standard",
				"runtime_session_kind": "user",
				"messages": []map[string]any{
					{
						"message_id":   finalMessageID,
						"role":         "navi",
						"run_id":       "run-watchdog-complete",
						"content":      finalContent,
						"created_at":   now,
						"message_kind": "reply",
					},
				},
				"created_at": now,
				"updated_at": now,
			})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer gw.Close()

	bot := NewBot(Config{
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		APIURL:        tg.URL,
		TelegramToken: "test-token",
		OwnerChatID:   chatID,
	})
	bot.token = gatewaySecret
	bot.terminalWatchdogPoll = 10 * time.Millisecond
	bot.mu.Lock()
	bot.activeChatID = sess
	bot.freshChatIDs[sess] = true
	bot.naviChatToTelegramChat[sess] = chatID
	bot.telegramChatToNaviChat[chatID] = sess
	bot.chatStream[sess] = streamMessage{
		ChatID:    chatID,
		MessageID: placeholderID,
		Content:   telegramThinkingPlaceholder,
	}
	bot.mu.Unlock()
	bot.startSessionTerminalWatchdog(sess, chatID)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go bot.wsLivePoller(ctx)

	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if editCount == 1 {
			if editedText != finalContent {
				t.Fatalf("unexpected watchdog repaired text %q", editedText)
			}
			bot.mu.Lock()
			_, exists := bot.chatStream[sess]
			bot.mu.Unlock()
			if exists {
				t.Fatal("expected chat stream to clear after watchdog completed repair")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for watchdog completion repair: editCount=%d editedText=%q", editCount, editedText)
}

func TestWsLivePoller_WatchdogRepairsFailedPlaceholderWithoutTerminalLiveEvent(t *testing.T) {
	const (
		sess          = "watchdog-failed-session"
		chatID        = int64(12347)
		placeholderID = int64(705)
	)

	var (
		editedText string
		editCount  int
	)

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/editMessageText"):
			editCount++
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			editedText, _ = body["text"].(string)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer tg.Close()

	gatewaySecret := "tok"
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/ws/live":
			conn, err := wsUpgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			_, _, err = conn.ReadMessage()
			if err != nil {
				return
			}
			_ = conn.WriteJSON(map[string]any{
				"type": "res",
				"id":   "connect",
				"ok":   true,
				"result": map[string]any{
					"after_seq":  0,
					"cursor_seq": 0,
				},
			})
			time.Sleep(200 * time.Millisecond)
		case r.URL.Path == "/api/navi/chats/"+sess+"/runtime_summary":
			if r.Header.Get("X-API-Key") != gatewaySecret {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			now := time.Now().UTC()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"chat_id": sess,
				"run": map[string]any{
					"run_id":     "run-watchdog-failed",
					"status":     "failed",
					"updated_at": now,
				},
				"terminal_event": map[string]any{
					"type": "run.failed",
					"seq":  10,
					"payload": map[string]any{
						"error": "The request took too long. Try /status, wait a moment, or start a fresh chat with /new.",
					},
				},
			})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer gw.Close()

	bot := NewBot(Config{
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		APIURL:        tg.URL,
		TelegramToken: "test-token",
		OwnerChatID:   chatID,
	})
	bot.token = gatewaySecret
	bot.terminalWatchdogPoll = 10 * time.Millisecond
	bot.mu.Lock()
	bot.activeChatID = sess
	bot.freshChatIDs[sess] = true
	bot.naviChatToTelegramChat[sess] = chatID
	bot.telegramChatToNaviChat[chatID] = sess
	bot.chatStream[sess] = streamMessage{
		ChatID:    chatID,
		MessageID: placeholderID,
		Content:   telegramThinkingPlaceholder,
	}
	bot.mu.Unlock()
	bot.startSessionTerminalWatchdog(sess, chatID)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go bot.wsLivePoller(ctx)

	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if editCount == 1 {
			want := "NAVI run failed: The request took too long. Try /status, wait a moment, or start a fresh chat with /new."
			if editedText != want {
				t.Fatalf("unexpected watchdog failed text %q", editedText)
			}
			bot.mu.Lock()
			_, exists := bot.chatStream[sess]
			bot.mu.Unlock()
			if exists {
				t.Fatal("expected chat stream to clear after watchdog failed repair")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for watchdog failed repair: editCount=%d editedText=%q", editCount, editedText)
}

func TestWsLivePoller_RunFailedFinalizesPlaceholderWithinGracePeriod(t *testing.T) {
	const (
		sess          = "timeout-session"
		chatID        = int64(12345)
		placeholderID = int64(701)
	)

	var (
		editedText string
		editCount  int
		sendCount  int
	)

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/editMessageText"):
			editCount++
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			editedText, _ = body["text"].(string)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			sendCount++
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"message_id": 1}})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer tg.Close()

	failedEvent := map[string]any{
		"type": "run.failed",
		"seq":  int64(1),
		"payload": map[string]any{
			"error": "The request took too long. Try /status, wait a moment, or start a fresh chat with /new.",
		},
	}
	wsSrv, _ := startWsTestServer(t, failedEvent)
	defer wsSrv.Close()

	bot := NewBot(Config{
		GatewayURL:    wsSrv.URL,
		GatewaySecret: "tok",
		APIURL:        tg.URL,
		TelegramToken: "test-token",
		OwnerChatID:   chatID,
	})
	bot.token = "tok"
	bot.mu.Lock()
	bot.activeChatID = sess
	bot.freshChatIDs[sess] = true
	bot.naviChatToTelegramChat[sess] = chatID
	bot.telegramChatToNaviChat[chatID] = sess
	stopCalled := false
	bot.chatTypingStop[sess] = func() { stopCalled = true }
	bot.chatStream[sess] = streamMessage{
		ChatID:    chatID,
		MessageID: placeholderID,
		Content:   telegramThinkingPlaceholder,
	}
	bot.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go bot.wsLivePoller(ctx)

	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if editCount == 1 {
			if editedText != "NAVI run failed: The request took too long. Try /status, wait a moment, or start a fresh chat with /new." {
				t.Fatalf("unexpected terminal edit text %q", editedText)
			}
			if sendCount != 0 {
				t.Fatalf("expected placeholder edit instead of new sendMessage, got %d send calls", sendCount)
			}
			bot.mu.Lock()
			_, exists := bot.chatStream[sess]
			_, typingActive := bot.chatTypingStop[sess]
			bot.mu.Unlock()
			if exists {
				t.Fatal("expected chat stream to clear after terminal run.failed event")
			}
			if typingActive || !stopCalled {
				t.Fatal("expected run.failed to stop typing and clear typing state")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for run.failed to finalize the placeholder: editCount=%d editedText=%q sendCount=%d", editCount, editedText, sendCount)
}

func TestWsLivePoller_RunCancelledFinalizesPlaceholderWithReason(t *testing.T) {
	const (
		sess          = "cancelled-session"
		chatID        = int64(12345)
		placeholderID = int64(702)
	)

	var (
		editedText string
		editCount  int
	)

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/editMessageText"):
			editCount++
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			editedText, _ = body["text"].(string)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer tg.Close()

	cancelledEvent := map[string]any{
		"type": "run.cancelled",
		"seq":  int64(1),
		"payload": map[string]any{
			"reason": "cancelled by user",
		},
	}
	wsSrv, _ := startWsTestServer(t, cancelledEvent)
	defer wsSrv.Close()

	bot := NewBot(Config{
		GatewayURL:    wsSrv.URL,
		GatewaySecret: "tok",
		APIURL:        tg.URL,
		TelegramToken: "test-token",
		OwnerChatID:   chatID,
	})
	bot.token = "tok"
	bot.mu.Lock()
	bot.activeChatID = sess
	bot.freshChatIDs[sess] = true
	bot.naviChatToTelegramChat[sess] = chatID
	bot.telegramChatToNaviChat[chatID] = sess
	stopCalled := false
	bot.chatTypingStop[sess] = func() { stopCalled = true }
	bot.chatStream[sess] = streamMessage{
		ChatID:    chatID,
		MessageID: placeholderID,
		Content:   telegramThinkingPlaceholder,
	}
	bot.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go bot.wsLivePoller(ctx)

	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if editCount == 1 {
			if editedText != "NAVI run cancelled: cancelled by user" {
				t.Fatalf("unexpected terminal cancel text %q", editedText)
			}
			bot.mu.Lock()
			_, exists := bot.chatStream[sess]
			_, typingActive := bot.chatTypingStop[sess]
			bot.mu.Unlock()
			if exists {
				t.Fatal("expected chat stream to clear after terminal run.cancelled event")
			}
			if typingActive || !stopCalled {
				t.Fatal("expected run.cancelled to stop typing and clear typing state")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for run.cancelled to finalize the placeholder: editCount=%d editedText=%q", editCount, editedText)
}

func TestWsLivePoller_RunCompletedWithoutPlaceholderClearsTerminalState(t *testing.T) {
	const (
		sess   = "completed-no-placeholder"
		chatID = int64(12348)
	)

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendMessage"), strings.HasSuffix(r.URL.Path, "/editMessageText"):
			t.Fatalf("did not expect Telegram message writes for run.completed cleanup without placeholder")
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer tg.Close()

	completedEvent := map[string]any{
		"type": "run.completed",
		"seq":  int64(1),
		"payload": map[string]any{
			"final_message_id": "msg-cleanup",
		},
	}
	wsSrv, _ := startWsTestServer(t, completedEvent)
	defer wsSrv.Close()

	bot := NewBot(Config{
		GatewayURL:    wsSrv.URL,
		GatewaySecret: "tok",
		APIURL:        tg.URL,
		TelegramToken: "test-token",
		OwnerChatID:   chatID,
	})
	bot.token = "tok"
	bot.mu.Lock()
	bot.activeChatID = sess
	bot.freshChatIDs[sess] = true
	bot.naviChatToTelegramChat[sess] = chatID
	bot.telegramChatToNaviChat[chatID] = sess
	stopCalled := false
	bot.chatTypingStop[sess] = func() { stopCalled = true }
	bot.chatRepair[sess] = 999
	bot.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go bot.wsLivePoller(ctx)

	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		bot.mu.Lock()
		_, hasStream := bot.chatStream[sess]
		_, hasTyping := bot.chatTypingStop[sess]
		_, hasRepair := bot.chatRepair[sess]
		bot.mu.Unlock()
		if !hasStream && !hasTyping && !hasRepair {
			if !stopCalled {
				t.Fatal("expected run.completed without placeholder to stop typing")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	bot.mu.Lock()
	_, hasStream := bot.chatStream[sess]
	_, hasTyping := bot.chatTypingStop[sess]
	_, hasRepair := bot.chatRepair[sess]
	bot.mu.Unlock()
	t.Fatalf("expected run.completed without placeholder to clear state: stream=%v typing=%v repair=%v stopCalled=%v", hasStream, hasTyping, hasRepair, stopCalled)
}

func TestWsLivePoller_ProposalWaiting(t *testing.T) {
	const (
		sess   = "test-session"
		chatID = int64(12345)
	)

	type sendMessageRequest struct {
		Text        string         `json:"text"`
		ReplyMarkup map[string]any `json:"reply_markup"`
	}

	tgReqs := make(chan sendMessageRequest, 5)

	// Fake Telegram API
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			var body sendMessageRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			tgReqs <- body
			w.WriteHeader(200)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"message_id": 1}})
			return
		}
		w.WriteHeader(200)
	}))
	defer tg.Close()

	// WS server that sends proposal.waiting once the bot connects
	proposalEvent := map[string]any{
		"type": "proposal.waiting",
		"seq":  int64(1),
		"payload": map[string]any{
			"proposal_id":            "prop-abc",
			"tool_name":              "my_tool",
			"reason":                 "requires confirmation",
			"proposal_summary":       "Decompose directive into tasks",
			"capability_description": "decompose directives",
			"side_effects":           []any{"create_task"},
			"risk_tier":              "low",
		},
	}
	wsSrv, _ := startWsTestServer(t, proposalEvent)
	defer wsSrv.Close()

	bot := NewBot(Config{
		GatewayURL:    wsSrv.URL,
		GatewaySecret: "tok",
		APIURL:        tg.URL,
		TelegramToken: "test-token",
		OwnerChatID:   chatID,
	})
	bot.token = "tok"
	bot.mu.Lock()
	bot.activeChatID = sess
	bot.freshChatIDs[sess] = true
	bot.naviChatToTelegramChat[sess] = chatID
	bot.telegramChatToNaviChat[chatID] = sess
	bot.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go bot.wsLivePoller(ctx)

	select {
	case req := <-tgReqs:
		if !strings.Contains(req.Text, "Approval needed.") {
			t.Errorf("expected text to contain approval header, got %q", req.Text)
		}
		if !strings.Contains(req.Text, "Proposal: prop-abc") {
			t.Errorf("expected text to contain proposal ID, got %q", req.Text)
		}
		if !strings.Contains(req.Text, "Tool: my_tool") {
			t.Errorf("expected text to contain tool name, got %q", req.Text)
		}
		if !strings.Contains(req.Text, "Reason: requires confirmation") {
			t.Errorf("expected text to contain reason, got %q", req.Text)
		}

		// Verify inline keyboard buttons
		rows, ok := req.ReplyMarkup["inline_keyboard"].([]any)
		if !ok || len(rows) != 1 {
			t.Fatalf("unexpected inline keyboard rows: %#v", req.ReplyMarkup["inline_keyboard"])
		}
		row, ok := rows[0].([]any)
		if !ok || len(row) != 2 {
			t.Fatalf("unexpected row layout: %#v", rows[0])
		}

		btn1, ok := row[0].(map[string]any)
		if !ok || btn1["callback_data"] != "proposal_approve:prop-abc" {
			t.Fatalf("unexpected approve button structure: %#v", row[0])
		}

		btn2, ok := row[1].(map[string]any)
		if !ok || btn2["callback_data"] != "proposal_reject:prop-abc" {
			t.Fatalf("unexpected reject button structure: %#v", row[1])
		}

	case <-ctx.Done():
		t.Fatal("timed out: proposal.waiting event did not deliver proposal prompt to Telegram")
	}
}
