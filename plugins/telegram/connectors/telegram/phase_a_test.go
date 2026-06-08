package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func boolPtr(b bool) *bool { return &b }

func groupMsg(chatID int64, chatType, text string) *tgMessage {
	m := &tgMessage{Text: text, From: &tgUser{FirstName: "Jane", Username: "jane"}}
	m.Chat.ID = chatID
	m.Chat.Type = chatType
	return m
}

func TestShouldProcessGroupMessage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		groups    map[string]GroupConfig
		username  string
		msg       *tgMessage
		wantAllow bool
	}{
		{
			name:      "dm always processed",
			msg:       groupMsg(100, "private", "hello"),
			wantAllow: true,
		},
		{
			name:      "group without mention skipped by default",
			username:  "navibot",
			msg:       groupMsg(-100, "supergroup", "hello there"),
			wantAllow: false,
		},
		{
			name:      "group with mention processed",
			username:  "navibot",
			msg:       groupMsg(-100, "supergroup", "hey @naviBot can you help"),
			wantAllow: true,
		},
		{
			name:     "group reply to bot processed",
			username: "navibot",
			msg: func() *tgMessage {
				m := groupMsg(-100, "group", "yes please")
				m.ReplyToMessage = &tgMessage{From: &tgUser{IsBot: true, Username: "navibot"}}
				return m
			}(),
			wantAllow: true,
		},
		{
			name:      "group require_mention disabled processes all",
			groups:    map[string]GroupConfig{"*": {RequireMention: boolPtr(false)}},
			username:  "navibot",
			msg:       groupMsg(-100, "supergroup", "no mention here"),
			wantAllow: true,
		},
		{
			name:      "disabled group skipped",
			groups:    map[string]GroupConfig{"*": {Enabled: boolPtr(false), RequireMention: boolPtr(false)}},
			username:  "navibot",
			msg:       groupMsg(-100, "supergroup", "@navibot hi"),
			wantAllow: false,
		},
		{
			name:     "specific group overrides wildcard",
			username: "navibot",
			groups: map[string]GroupConfig{
				"*":    {RequireMention: boolPtr(false)},
				"-100": {RequireMention: boolPtr(true)},
			},
			msg:       groupMsg(-100, "supergroup", "no mention"),
			wantAllow: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bot := NewBot(Config{Groups: tc.groups})
			bot.botUsername = tc.username
			if got := bot.shouldProcessGroupMessage(tc.msg); got != tc.wantAllow {
				t.Fatalf("shouldProcessGroupMessage = %v, want %v", got, tc.wantAllow)
			}
		})
	}
}

func TestBuildInboundEnvelope(t *testing.T) {
	t.Parallel()

	t.Run("dm returns raw text", func(t *testing.T) {
		msg := groupMsg(100, "private", "hello")
		if got := buildInboundEnvelope(msg, "hello", false); got != "hello" {
			t.Fatalf("dm envelope = %q, want raw text", got)
		}
	})

	t.Run("group adds sender attribution", func(t *testing.T) {
		msg := groupMsg(-100, "supergroup", "hello")
		msg.Chat.Title = "Eng Team"
		got := buildInboundEnvelope(msg, "hello", true)
		if !strings.Contains(got, "[From Jane (@jane) in \"Eng Team\"]") {
			t.Fatalf("missing sender attribution: %q", got)
		}
		if !strings.HasSuffix(got, "hello") {
			t.Fatalf("envelope should end with body: %q", got)
		}
	})

	t.Run("group includes reply context", func(t *testing.T) {
		msg := groupMsg(-100, "group", "see above")
		msg.ReplyToMessage = &tgMessage{
			From: &tgUser{FirstName: "NAVI"},
			Text: "the earlier answer",
		}
		got := buildInboundEnvelope(msg, "see above", true)
		if !strings.Contains(got, "[Replying to NAVI] \"the earlier answer\"") {
			t.Fatalf("missing reply context: %q", got)
		}
	})

	t.Run("group includes forward context", func(t *testing.T) {
		msg := groupMsg(-100, "group", "look at this")
		msg.ForwardOrigin = &tgForwardOrigin{Type: "user", SenderUser: &tgUser{FirstName: "Bob"}}
		got := buildInboundEnvelope(msg, "look at this", true)
		if !strings.Contains(got, "[Forwarded from Bob]") {
			t.Fatalf("missing forward context: %q", got)
		}
	})
}

func TestGetUpdatesSendsAllowedUpdates(t *testing.T) {
	t.Parallel()

	gotAllowed := make(chan []string, 1)
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/getUpdates") {
			body, _ := io.ReadAll(r.Body)
			var req map[string]any
			_ = json.Unmarshal(body, &req)
			raw, _ := req["allowed_updates"].([]any)
			out := make([]string, 0, len(raw))
			for _, v := range raw {
				if s, ok := v.(string); ok {
					out = append(out, s)
				}
			}
			select {
			case gotAllowed <- out:
			default:
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": []tgUpdate{}})
			return
		}
		w.WriteHeader(200)
	}))
	defer tg.Close()

	bot := NewBot(Config{TelegramToken: "test", apiURL: tg.URL})
	if _, err := bot.getUpdates(context.Background(), 0); err != nil {
		t.Fatalf("getUpdates error: %v", err)
	}

	select {
	case got := <-gotAllowed:
		want := map[string]bool{"message": true, "edited_message": true, "channel_post": true, "callback_query": true}
		if len(got) != len(want) {
			t.Fatalf("allowed_updates = %v, want keys %v", got, want)
		}
		for _, k := range got {
			if !want[k] {
				t.Fatalf("unexpected allowed_updates entry %q in %v", k, got)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("getUpdates did not send allowed_updates")
	}
}

// TestGroupMessageQueuesEnvelope verifies that a mentioned group message is queued
// with the attribution envelope, and an unmentioned one is dropped (no queue call).
func TestGroupMessageQueuesEnvelope(t *testing.T) {
	t.Parallel()

	secret := "tok"
	var mu sync.Mutex
	var queued []string
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeMockTelegramEndpointResolve(t, w, r, map[string]string{"-100": "navi-sess-1"}) {
			return
		}
		if strings.HasSuffix(r.URL.Path, "/message") && r.Method == http.MethodPost {
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			mu.Lock()
			queued = append(queued, req["content"].(string))
			mu.Unlock()
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "queued", "inbox_status": "pending"})
			return
		}
		if r.URL.Path == "/health" {
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(404)
	}))
	defer gw.Close()

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"message_id": 1}})
	}))
	defer tg.Close()

	bot := NewBot(Config{
		Name:          "telegram",
		TelegramToken: "test",
		GatewayURL:    gw.URL,
		GatewaySecret: secret,
		OwnerChatID:   -100,
		apiURL:        tg.URL,
	})
	bot.token = secret
	bot.botUsername = "navibot"
	bot.running = true
	bot.bindSessionToChat(-100, "navi-sess-1")

	send := func(text string) {
		u := tgUpdate{UpdateID: 1, Message: groupMsg(-100, "supergroup", text)}
		bot.handleUpdate(context.Background(), u)
	}

	send("no mention here")
	mu.Lock()
	if len(queued) != 0 {
		mu.Unlock()
		t.Fatalf("unmentioned group message should not be queued, got %v", queued)
	}
	mu.Unlock()

	send("hey @navibot help")
	mu.Lock()
	defer mu.Unlock()
	if len(queued) != 1 {
		t.Fatalf("mentioned group message should be queued once, got %d", len(queued))
	}
	if !strings.Contains(queued[0], "[From Jane (@jane)") {
		t.Fatalf("queued content missing attribution envelope: %q", queued[0])
	}
	if !strings.Contains(queued[0], "hey @navibot help") {
		t.Fatalf("queued content missing original text: %q", queued[0])
	}
}

func TestDisabledGroupSkipsCommands(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var sentMethods []string
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sentMethods = append(sentMethods, strings.TrimPrefix(r.URL.Path, "/bottest/"))
		mu.Unlock()
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"message_id": 1}})
	}))
	defer tg.Close()

	bot := NewBot(Config{
		TelegramToken: "test",
		OwnerChatID:   -100,
		Groups:        map[string]GroupConfig{"*": {Enabled: boolPtr(false)}},
		apiURL:        tg.URL,
	})
	bot.token = "tok"
	bot.botUsername = "navibot"
	bot.running = true

	bot.handleUpdate(context.Background(), tgUpdate{UpdateID: 1, Message: groupMsg(-100, "supergroup", "/pair")})

	mu.Lock()
	defer mu.Unlock()
	if len(sentMethods) != 0 {
		t.Fatalf("disabled group command should not send replies, got telegram calls %v", sentMethods)
	}
}
