package canonicalize

import (
	"strings"
	"testing"

	"github.com/ceoai/navi/internal/schema"
)

func rec(mime string, body string) schema.IntakeRecord {
	return schema.IntakeRecord{
		ID: "r1", ConnectorID: "c", SourceID: "s", RawMIME: mime, Raw: []byte(body),
	}
}

func TestCanonicalize_HTML_StripsBoilerplate(t *testing.T) {
	html := `<html><head><title>T</title><style>.x{}</style></head>
	<body>
	<nav><a href="/home">Home</a></nav>
	<h1>Hello World</h1>
	<p>This is a <b>real</b> paragraph.</p>
	<script>tracker();</script>
	<footer>Copyright 2026</footer>
	</body></html>`
	doc, err := Canonicalize(rec("text/html", html))
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	if doc.SourceMIME != "text/html" {
		t.Errorf("SourceMIME: got %q", doc.SourceMIME)
	}
	if !strings.Contains(doc.Markdown, "Hello World") {
		t.Errorf("expected heading text retained, got:\n%s", doc.Markdown)
	}
	if !strings.Contains(doc.Markdown, "real paragraph") {
		t.Errorf("expected paragraph text retained, got:\n%s", doc.Markdown)
	}
	for _, junk := range []string{"tracker()", "Copyright 2026", "Home", ".x{}"} {
		if strings.Contains(doc.Markdown, junk) {
			t.Errorf("boilerplate %q not stripped:\n%s", junk, doc.Markdown)
		}
	}
}

func TestCanonicalize_Text_StripsQuotedReplyAndSignature(t *testing.T) {
	body := "Thanks for the update, looks good.\n" +
		"\n" +
		"On Mon, May 5 2026, Alice wrote:\n" +
		"> Here is the original\n" +
		"> message text\n" +
		"\n" +
		"-- \n" +
		"Bob Smith\n" +
		"CEO, Example Inc"
	doc, err := Canonicalize(rec("text/plain", body))
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	if !strings.Contains(doc.Markdown, "Thanks for the update") {
		t.Errorf("primary content lost:\n%s", doc.Markdown)
	}
	for _, junk := range []string{"original", "message text", "Alice wrote", "Bob Smith", "CEO, Example"} {
		if strings.Contains(doc.Markdown, junk) {
			t.Errorf("expected %q stripped:\n%s", junk, doc.Markdown)
		}
	}
}

func TestCanonicalize_JSON_RendersMarkdownDeterministically(t *testing.T) {
	body := `{"name":"Acme","tags":["b","a"],"meta":{"z":1,"a":2}}`
	doc, err := Canonicalize(rec("application/json", body))
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	if doc.SourceMIME != "application/json" {
		t.Errorf("SourceMIME: got %q", doc.SourceMIME)
	}
	if !strings.Contains(doc.Markdown, "**name**: Acme") {
		t.Errorf("expected name rendered:\n%s", doc.Markdown)
	}
	// Keys sorted: "meta" before "name" before "tags"; within meta "a" before "z".
	mi := strings.Index(doc.Markdown, "**meta**")
	ni := strings.Index(doc.Markdown, "**name**")
	if mi < 0 || ni < 0 || mi > ni {
		t.Errorf("keys not sorted deterministically:\n%s", doc.Markdown)
	}

	// Determinism: same input → identical output.
	doc2, _ := Canonicalize(rec("application/json", body))
	if doc.Markdown != doc2.Markdown {
		t.Error("JSON canonicalization is not deterministic")
	}
}

func TestCanonicalize_MalformedJSON_FallsBackToText(t *testing.T) {
	doc, err := Canonicalize(rec("application/json", "{not json"))
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	if doc.SourceMIME != "text/plain" {
		t.Errorf("expected fallback to text/plain, got %q", doc.SourceMIME)
	}
	if !strings.Contains(doc.Markdown, "not json") {
		t.Errorf("fallback content lost:\n%s", doc.Markdown)
	}
}

func TestCanonicalize_NormalizesURLs(t *testing.T) {
	body := "See https://Example.COM:443/path/?utm_source=news&b=2&a=1#frag for details."
	doc, err := Canonicalize(rec("text/plain", body))
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	if !strings.Contains(doc.Markdown, "https://example.com/path/?a=1&b=2") {
		t.Errorf("URL not normalized:\n%s", doc.Markdown)
	}
	if strings.Contains(doc.Markdown, "utm_source") || strings.Contains(doc.Markdown, "#frag") || strings.Contains(doc.Markdown, ":443") {
		t.Errorf("URL normalization incomplete:\n%s", doc.Markdown)
	}
	// Trailing sentence punctuation preserved outside the URL.
	if !strings.Contains(doc.Markdown, "details.") {
		t.Errorf("trailing punctuation lost:\n%s", doc.Markdown)
	}
}

func TestNormalizeURL(t *testing.T) {
	cases := map[string]string{
		"https://Example.com/":                     "https://example.com",
		"http://host:80/a":                         "http://host/a",
		"https://host:443/a?b=2&a=1":               "https://host/a?a=1&b=2",
		"https://t.co/x?utm_campaign=z&fbclid=abc": "https://t.co/x",
		"not a url":                                "not a url",
	}
	for in, want := range cases {
		if got := NormalizeURL(in); got != want {
			t.Errorf("NormalizeURL(%q) = %q, want %q", in, got, want)
		}
	}
}
