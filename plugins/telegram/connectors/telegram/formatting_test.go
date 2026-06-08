package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestMarkdownToTelegramHTML(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "escapes html chars", in: "use <provider> & <model>", want: "use &lt;provider&gt; &amp; &lt;model&gt;"},
		{name: "bold double star", in: "this is **bold** text", want: "this is <b>bold</b> text"},
		{name: "bold double underscore", in: "this is __bold__ text", want: "this is <b>bold</b> text"},
		{name: "inline code", in: "run `go test` now", want: "run <code>go test</code> now"},
		{name: "inline code is escaped", in: "call `f<x>()`", want: "call <code>f&lt;x&gt;()</code>"},
		{name: "heading to bold", in: "# Title", want: "<b>Title</b>"},
		{name: "link", in: "see [docs](https://example.com/a)", want: `see <a href="https://example.com/a">docs</a>`},
		{name: "italic not converted", in: "a snake_case_name here", want: "a snake_case_name here"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := markdownToTelegramHTML(tc.in); got != tc.want {
				t.Fatalf("markdownToTelegramHTML(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestMarkdownToTelegramHTMLFencedCode(t *testing.T) {
	t.Parallel()

	got := markdownToTelegramHTML("before\n```go\nx := 1 < 2\n```\nafter")
	want := "before\n<pre><code class=\"language-go\">x := 1 &lt; 2</code></pre>\nafter"
	if got != want {
		t.Fatalf("fenced code = %q, want %q", got, want)
	}

	gotNoLang := markdownToTelegramHTML("```\nplain code\n```")
	if gotNoLang != "<pre>plain code</pre>" {
		t.Fatalf("plain fence = %q", gotNoLang)
	}
}

func TestMarkdownToTelegramHTMLTable(t *testing.T) {
	t.Parallel()

	in := "| Name | Role |\n| --- | --- |\n| Jane | Dev |\n| Bob | PM |"
	got := markdownToTelegramHTML(in)
	if !strings.HasPrefix(got, "<pre>") || !strings.HasSuffix(got, "</pre>") {
		t.Fatalf("table should be wrapped in <pre>: %q", got)
	}
	if !strings.Contains(got, "Name | Role") {
		t.Fatalf("table missing header row: %q", got)
	}
	if !strings.Contains(got, "Jane | Dev") {
		t.Fatalf("table missing body row: %q", got)
	}
	// Columns should be padded to equal width (Jane wider than Bob).
	if !strings.Contains(got, "Bob  | PM") {
		t.Fatalf("table columns not aligned: %q", got)
	}
}

func TestTelegramHTMLParseError(t *testing.T) {
	t.Parallel()
	if !telegramHTMLParseError(errors.New("telegram sendMessage error: 400 Bad Request: can't parse entities: ...")) {
		t.Fatal("expected parse error detection")
	}
	if telegramHTMLParseError(errors.New("telegram sendMessage error: 429 Too Many Requests")) {
		t.Fatal("rate limit must not be treated as a parse error")
	}
	if telegramHTMLParseError(nil) {
		t.Fatal("nil must not be a parse error")
	}
}

// TestSendChunkRendersHTMLWithFallback verifies the outbound chat path sends HTML
// and falls back to plain text when Telegram rejects the entities.
func TestSendChunkRendersHTMLWithFallback(t *testing.T) {
	t.Parallel()

	type sent struct {
		text      string
		parseMode string
	}
	var mu sync.Mutex
	var sends []sent
	failHTML := true

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			pm, _ := req["parse_mode"].(string)
			txt, _ := req["text"].(string)
			mu.Lock()
			sends = append(sends, sent{text: txt, parseMode: pm})
			shouldFail := failHTML && pm == "HTML"
			mu.Unlock()
			if shouldFail {
				http.Error(w, `{"ok":false,"description":"Bad Request: can't parse entities: bad"}`, http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"message_id": 5}})
			return
		}
		w.WriteHeader(200)
	}))
	defer tg.Close()

	bot := NewBot(Config{TelegramToken: "test", apiURL: tg.URL, OwnerChatID: 100})

	// First: HTML send succeeds (no forced failure).
	mu.Lock()
	failHTML = false
	mu.Unlock()
	if _, err := bot.sendChunkText(context.Background(), 100, "hello **world**", nil, 0, 0, ""); err != nil {
		t.Fatalf("sendChunkText: %v", err)
	}
	mu.Lock()
	if len(sends) != 1 || sends[0].parseMode != "HTML" || !strings.Contains(sends[0].text, "<b>world</b>") {
		mu.Unlock()
		t.Fatalf("expected one HTML send with rendered bold, got %+v", sends)
	}
	sends = nil
	failHTML = true
	mu.Unlock()

	// Second: HTML send fails with a parse error → falls back to plain text.
	if _, err := bot.sendChunkText(context.Background(), 100, "hello **world**", nil, 0, 0, ""); err != nil {
		t.Fatalf("sendChunkText fallback: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(sends) != 2 {
		t.Fatalf("expected HTML attempt then plain fallback, got %d sends: %+v", len(sends), sends)
	}
	if sends[0].parseMode != "HTML" {
		t.Fatalf("first attempt should be HTML, got %+v", sends[0])
	}
	if sends[1].parseMode != "" || sends[1].text != "hello **world**" {
		t.Fatalf("fallback should be plain original text, got %+v", sends[1])
	}
}
