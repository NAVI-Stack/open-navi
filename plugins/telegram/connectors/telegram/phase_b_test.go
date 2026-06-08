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

func TestDescribeAttachments(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		attachments []InboundAttachment
		want        string
	}{
		{name: "none", attachments: nil, want: ""},
		{name: "photo", attachments: []InboundAttachment{{MediaKind: "photo"}}, want: "[User attached a photo]"},
		{name: "voice", attachments: []InboundAttachment{{MediaKind: "voice"}}, want: "[User sent a voice message]"},
		{name: "document with name", attachments: []InboundAttachment{{MediaKind: "document", Filename: "report.pdf"}}, want: "[User attached a document: report.pdf]"},
		{name: "document no name", attachments: []InboundAttachment{{MediaKind: "document"}}, want: "[User attached a document]"},
		{name: "sticker", attachments: []InboundAttachment{{MediaKind: "sticker"}}, want: "[User sent a sticker]"},
		{name: "unknown", attachments: []InboundAttachment{{MediaKind: "contact"}}, want: "[User attached media]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := describeAttachments(tc.attachments); got != tc.want {
				t.Fatalf("describeAttachments = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTelegramCommandMenu(t *testing.T) {
	t.Parallel()

	t.Run("pairing disabled omits /pair", func(t *testing.T) {
		bot := NewBot(Config{})
		for _, c := range bot.telegramCommandMenu() {
			if c.Command == "pair" {
				t.Fatalf("/pair should be absent when pairing is disabled")
			}
			if strings.HasPrefix(c.Command, "/") {
				t.Fatalf("command name must not include leading slash: %q", c.Command)
			}
		}
	})

	t.Run("pairing enabled includes /pair", func(t *testing.T) {
		bot := NewBot(Config{PairingCode: "secret"})
		found := false
		for _, c := range bot.telegramCommandMenu() {
			if c.Command == "pair" {
				found = true
			}
		}
		if !found {
			t.Fatalf("/pair should be present when pairing is enabled")
		}
	})
}

func TestRegisterCommandMenuCallsSetMyCommands(t *testing.T) {
	t.Parallel()

	gotCommands := make(chan []string, 1)
	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/setMyCommands") {
			body, _ := io.ReadAll(r.Body)
			var req struct {
				Commands []tgBotCommand `json:"commands"`
			}
			_ = json.Unmarshal(body, &req)
			names := make([]string, 0, len(req.Commands))
			for _, c := range req.Commands {
				names = append(names, c.Command)
			}
			select {
			case gotCommands <- names:
			default:
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
			return
		}
		w.WriteHeader(200)
	}))
	defer tg.Close()

	bot := NewBot(Config{TelegramToken: "test", apiURL: tg.URL})
	bot.registerCommandMenu(context.Background())

	select {
	case names := <-gotCommands:
		want := map[string]bool{"navi": true, "new": true, "use": true, "list": true, "implement": true, "status": true, "model": true, "hitl": true}
		for _, n := range names {
			if !want[n] {
				t.Fatalf("unexpected command %q in menu %v", n, names)
			}
		}
		if len(names) != len(want) {
			t.Fatalf("command menu = %v, want %d commands", names, len(want))
		}
	case <-time.After(time.Second):
		t.Fatal("registerCommandMenu did not call setMyCommands")
	}
}

// TestMediaPresenceQueuedInDM verifies a DM photo (no caption) is queued as a media
// acknowledgment rather than dropped or queued empty.
func TestMediaPresenceQueuedInDM(t *testing.T) {
	t.Parallel()

	secret := "tok"
	var mu sync.Mutex
	var queued []string
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeMockTelegramEndpointResolve(t, w, r, map[string]string{"100": "navi-sess-1"}) {
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
		OwnerChatID:   100,
		apiURL:        tg.URL,
	})
	bot.token = secret
	bot.running = true
	bot.bindSessionToChat(100, "navi-sess-1")

	msg := &tgMessage{Photo: []tgPhotoSize{{FileID: "f1", FileSize: 1024}}}
	msg.Chat.ID = 100
	msg.Chat.Type = "private"
	bot.handleUpdate(context.Background(), tgUpdate{UpdateID: 1, Message: msg})

	mu.Lock()
	defer mu.Unlock()
	if len(queued) != 1 {
		t.Fatalf("DM photo should queue one message, got %d", len(queued))
	}
	if !strings.Contains(queued[0], "[User attached a photo]") {
		t.Fatalf("queued content should acknowledge the photo, got %q", queued[0])
	}
}
