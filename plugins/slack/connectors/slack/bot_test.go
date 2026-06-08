package slack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

func TestHandleDMCreatesSessionAndEditsPlaceholderFromLiveEvents(t *testing.T) {
	t.Parallel()

	const gatewaySecret = "test-secret"
	wsReady := make(chan *websocket.Conn, 1)
	wsDone := make(chan struct{})
	sessionPosted := make(chan map[string]string, 1)

	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/navi/chats" && r.Method == http.MethodPost:
			if r.Header.Get("X-API-Key") != gatewaySecret {
				t.Fatalf("expected X-API-Key on session create")
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"chat_id": "sess-1"})
		case r.URL.Path == "/api/navi/chats/sess-1/message" && r.Method == http.MethodPost:
			var req map[string]string
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode chat message: %v", err)
			}
			sessionPosted <- req
			conn := <-wsReady
			_ = wsjson.Write(r.Context(), conn, map[string]any{
				"type": "event",
				"event": map[string]any{
					"type": "assistant.message.partial",
					"seq":  1,
					"payload": map[string]any{
						"content": "Working on it...",
					},
				},
			})
			_ = wsjson.Write(r.Context(), conn, map[string]any{
				"type": "event",
				"event": map[string]any{
					"type": "assistant.message.completed",
					"seq":  2,
					"payload": map[string]any{
						"content": "Done from Slack session",
					},
				},
			})
			close(wsDone)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"status":"queued"}`))
		case r.URL.Path == "/ws/live":
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				t.Fatalf("accept websocket: %v", err)
			}
			var connectReq map[string]any
			if err := wsjson.Read(r.Context(), conn, &connectReq); err != nil {
				t.Fatalf("read connect frame: %v", err)
			}
			params := connectReq["params"].(map[string]any)
			if params["chat_id"] != "sess-1" {
				t.Fatalf("expected chat_id sess-1, got %v", params["chat_id"])
			}
			if r.Header.Get("X-API-Key") != gatewaySecret {
				t.Fatalf("expected X-API-Key on live websocket")
			}
			_ = wsjson.Write(r.Context(), conn, map[string]any{
				"type": "res",
				"id":   connectReq["id"],
				"ok":   true,
				"result": map[string]any{
					"chat_id": "sess-1",
				},
			})
			wsReady <- conn
			<-wsDone
			_ = conn.Close(websocket.StatusNormalClosure, "")
		default:
			http.NotFound(w, r)
		}
	}))
	defer gw.Close()

	var slackMu sync.Mutex
	var posts []string
	var updates []string
	slackAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat.postMessage":
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			slackMu.Lock()
			posts = append(posts, req["text"].(string))
			slackMu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ts": "ts-placeholder"})
		case "/chat.update":
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			slackMu.Lock()
			updates = append(updates, req["text"].(string))
			slackMu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer slackAPI.Close()

	bot := NewBot(Config{
		GatewayURL:    gw.URL,
		GatewaySecret: gatewaySecret,
		SlackBotToken: "xoxb-test",
		slackAPIURL:   slackAPI.URL,
	})
	bot.token = gatewaySecret

	bot.handleDM(context.Background(), "U123", "D123", "hello from slack", "1700000000.1")

	select {
	case req := <-sessionPosted:
		if req["content"] != "hello from slack" {
			t.Fatalf("unexpected session content %q", req["content"])
		}
		if req["source_channel"] != "slack" {
			t.Fatalf("unexpected source_channel %q", req["source_channel"])
		}
		if req["source_message_ref"] != "1700000000.1" {
			t.Fatalf("unexpected source_message_ref %q", req["source_message_ref"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for chat message post")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		slackMu.Lock()
		gotPosts := append([]string(nil), posts...)
		gotUpdates := append([]string(nil), updates...)
		slackMu.Unlock()
		if len(gotPosts) >= 1 && len(gotUpdates) >= 2 {
			if gotPosts[0] != "Thinking..." {
				t.Fatalf("unexpected placeholder post %q", gotPosts[0])
			}
			if gotUpdates[0] != "Working on it..." {
				t.Fatalf("unexpected partial update %q", gotUpdates[0])
			}
			if gotUpdates[len(gotUpdates)-1] != "Done from Slack session" {
				t.Fatalf("unexpected final update %q", gotUpdates[len(gotUpdates)-1])
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for Slack placeholder lifecycle updates")
}

func TestHandleSessionEventFailureAndCancelEditPlaceholder(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var updates []string
	slackAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat.update":
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			mu.Lock()
			updates = append(updates, req["text"].(string))
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case "/chat.postMessage":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ts": "fallback"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer slackAPI.Close()

	bot := NewBot(Config{
		SlackBotToken: "xoxb-test",
		slackAPIURL:   slackAPI.URL,
	})
	bot.ensureChatState("sess-fail", "DFAIL")
	bot.setSessionPlaceholder("sess-fail", "DFAIL", "ts-fail")
	bot.handleSessionEvent(context.Background(), "sess-fail", "run.failed", map[string]any{"error": "boom"})

	bot.ensureChatState("sess-cancel", "DCANCEL")
	bot.setSessionPlaceholder("sess-cancel", "DCANCEL", "ts-cancel")
	bot.handleSessionEvent(context.Background(), "sess-cancel", "run.cancelled", map[string]any{"reason": "user stopped it"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := append([]string(nil), updates...)
		mu.Unlock()
		if len(got) >= 2 {
			if got[0] != "NAVI run failed: boom" {
				t.Fatalf("unexpected failure text %q", got[0])
			}
			if got[1] != "NAVI run cancelled: user stopped it" {
				t.Fatalf("unexpected cancel text %q", got[1])
			}
			if state := bot.sessionState["sess-fail"]; state == nil || state.PlaceholderID != "" {
				t.Fatalf("expected failed session placeholder to be cleared, got %+v", state)
			}
			if state := bot.sessionState["sess-cancel"]; state == nil || state.PlaceholderID != "" {
				t.Fatalf("expected cancelled session placeholder to be cleared, got %+v", state)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for failure/cancel edits")
}

func TestHandleSessionEventProactivePostsSeparateMessage(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var posts []string
	var updates []string
	slackAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat.postMessage":
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			mu.Lock()
			posts = append(posts, req["text"].(string))
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ts": "ts-proactive"})
		case "/chat.update":
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			mu.Lock()
			updates = append(updates, req["text"].(string))
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer slackAPI.Close()

	bot := NewBot(Config{
		SlackBotToken: "xoxb-test",
		slackAPIURL:   slackAPI.URL,
	})
	bot.ensureChatState("sess-proactive", "DPRO")

	bot.handleSessionEvent(context.Background(), "sess-proactive", "assistant.message.completed", map[string]any{
		"content":      "The background health check found one warning.",
		"message_kind": "proactive",
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		gotPosts := append([]string(nil), posts...)
		gotUpdates := append([]string(nil), updates...)
		mu.Unlock()
		if len(gotPosts) >= 1 {
			if gotPosts[0] != "Proactive update:\nThe background health check found one warning." {
				t.Fatalf("unexpected proactive post %q", gotPosts[0])
			}
			if len(gotUpdates) != 0 {
				t.Fatalf("expected no placeholder edits for proactive message, got %+v", gotUpdates)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for proactive Slack post")
}

func TestHandleSessionEventIgnoresInternalSession(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var posts []string
	var updates []string
	slackAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat.postMessage":
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			mu.Lock()
			posts = append(posts, req["text"].(string))
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ts": "ts-ignored"})
		case "/chat.update":
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			mu.Lock()
			updates = append(updates, req["text"].(string))
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer slackAPI.Close()

	bot := NewBot(Config{
		SlackBotToken: "xoxb-test",
		slackAPIURL:   slackAPI.URL,
	})
	bot.ensureChatState("heartbeat-auto", "DINTERNAL")

	bot.handleSessionEvent(context.Background(), "heartbeat-auto", "assistant.message.completed", map[string]any{
		"content": "internal",
	})
	bot.handleSessionEvent(context.Background(), "heartbeat-auto", "run.failed", map[string]any{
		"error": "internal failure",
	})

	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(posts) != 0 || len(updates) != 0 {
		t.Fatalf("expected no Slack output for internal session, got posts=%v updates=%v", posts, updates)
	}
}
