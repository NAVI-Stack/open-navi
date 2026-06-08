// Package canonicalize implements CIP stage 3 (Canonicalize): it turns a raw,
// provenance-stamped IntakeRecord payload (HTML / JSON / plain text) into a
// clean internal Markdown rendering with boilerplate, navigation chrome,
// signatures, and quoted replies stripped, and with URLs / identifiers
// normalized.
//
// It is dependency-light by design (stdlib only) so the P2 pipeline carries no
// LLM or third-party-parser coupling. HTML handling is a tolerant tag stripper,
// not a full DOM parser; it is adequate for the message/email/article payloads
// connectors emit and is deterministic for identical input bytes.
package canonicalize

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"

	"github.com/ceoai/navi/internal/schema"
)

// Document is the normalized output of the Canonicalize stage.
type Document struct {
	RecordID   string // source IntakeRecord.ID
	SourceMIME string // classified source: "text/html" | "application/json" | "text/plain"
	Markdown   string // clean Markdown rendering
}

// Canonicalize normalizes r.Raw into clean Markdown according to its MIME type.
// Unknown MIME types fall back to plain-text handling. The function is pure: no
// I/O, deterministic for identical input bytes.
func Canonicalize(r schema.IntakeRecord) (Document, error) {
	mime := classifyMIME(r.RawMIME)
	raw := string(r.Raw)

	var md string
	switch mime {
	case "text/html":
		md = canonicalizeHTML(raw)
	case "application/json":
		out, err := canonicalizeJSON(r.Raw)
		if err != nil {
			// Malformed JSON: fall back to plain-text rather than failing the pass.
			md = canonicalizeText(raw)
			mime = "text/plain"
		} else {
			md = out
		}
	default:
		md = canonicalizeText(raw)
	}

	md = normalizeURLsInText(md)
	md = collapseBlankLines(strings.TrimSpace(md))

	return Document{RecordID: r.ID, SourceMIME: mime, Markdown: md}, nil
}

func classifyMIME(raw string) string {
	m := strings.ToLower(strings.TrimSpace(raw))
	if i := strings.IndexByte(m, ';'); i >= 0 {
		m = strings.TrimSpace(m[:i])
	}
	switch {
	case m == "text/html" || m == "application/xhtml+xml":
		return "text/html"
	case m == "application/json" || strings.HasSuffix(m, "+json"):
		return "application/json"
	case m == "" || strings.HasPrefix(m, "text/"):
		return "text/plain"
	default:
		return "text/plain"
	}
}

// --- Plain text ---------------------------------------------------------------

var (
	// Email/IM signature delimiter: a line that is exactly "--" or "-- ".
	sigDelimRE = regexp.MustCompile(`(?m)^--\s*$`)
	// Quoted-reply attribution line, e.g. "On Mon, ... wrote:".
	quoteAttribRE = regexp.MustCompile(`(?mi)^On .+ wrote:\s*$`)
)

// canonicalizeText cleans plain-text / email payloads: strips quoted replies,
// signatures, and excess whitespace.
func canonicalizeText(raw string) string {
	raw = normalizeNewlines(raw)

	// Cut everything from a signature delimiter onward.
	if loc := sigDelimRE.FindStringIndex(raw); loc != nil {
		raw = raw[:loc[0]]
	}

	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		trimmed := strings.TrimRight(ln, " \t")
		// Drop quoted-reply blocks (lines beginning with one or more '>').
		if strings.HasPrefix(strings.TrimSpace(trimmed), ">") {
			continue
		}
		// Drop the "On ... wrote:" attribution that precedes a quote block.
		if quoteAttribRE.MatchString(trimmed) {
			continue
		}
		out = append(out, trimmed)
	}
	return strings.Join(out, "\n")
}

// --- JSON ---------------------------------------------------------------------

// canonicalizeJSON renders a JSON payload as a stable, human-readable Markdown
// key/value outline. Object keys are emitted in sorted order for determinism.
func canonicalizeJSON(raw []byte) (string, error) {
	var v any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return "", fmt.Errorf("canonicalize: json decode: %w", err)
	}
	var b strings.Builder
	renderJSON(&b, v, 0)
	return b.String(), nil
}

func renderJSON(b *strings.Builder, v any, depth int) {
	indent := strings.Repeat("  ", depth)
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			child := t[k]
			if isScalar(child) {
				fmt.Fprintf(b, "%s- **%s**: %s\n", indent, k, scalarString(child))
			} else {
				fmt.Fprintf(b, "%s- **%s**:\n", indent, k)
				renderJSON(b, child, depth+1)
			}
		}
	case []any:
		for _, item := range t {
			if isScalar(item) {
				fmt.Fprintf(b, "%s- %s\n", indent, scalarString(item))
			} else {
				fmt.Fprintf(b, "%s-\n", indent)
				renderJSON(b, item, depth+1)
			}
		}
	default:
		fmt.Fprintf(b, "%s%s\n", indent, scalarString(v))
	}
}

func isScalar(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return false
	default:
		return true
	}
}

func scalarString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case json.Number:
		return t.String()
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", t)
	}
}

// --- HTML ---------------------------------------------------------------------

// htmlDropTags are elements whose entire content is chrome/boilerplate and is
// removed before tag stripping. RE2 has no backreferences, so each tag gets its
// own compiled pattern.
var htmlDropTags = []string{"script", "style", "head", "nav", "footer", "header", "aside", "form", "noscript", "svg", "iframe"}

var htmlDropElementREs = func() []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(htmlDropTags))
	for i, tag := range htmlDropTags {
		out[i] = regexp.MustCompile(`(?is)<` + tag + `\b[^>]*>.*?</\s*` + tag + `\s*>`)
	}
	return out
}()

var (
	htmlCommentRE     = regexp.MustCompile(`(?s)<!--.*?-->`)
	htmlBlockBreakRE  = regexp.MustCompile(`(?i)</\s*(p|div|section|article|li|tr|h[1-6]|blockquote)\s*>`)
	htmlLineBreakRE   = regexp.MustCompile(`(?i)<\s*br\s*/?\s*>`)
	htmlListItemRE    = regexp.MustCompile(`(?i)<\s*li\b[^>]*>`)
	htmlHeadingOpenRE = regexp.MustCompile(`(?i)<\s*h([1-6])\b[^>]*>`)
	htmlAnyTagRE      = regexp.MustCompile(`(?s)<[^>]+>`)
)

// canonicalizeHTML strips boilerplate elements and tags from an HTML payload,
// converting block-level structure into Markdown-ish line breaks.
func canonicalizeHTML(raw string) string {
	raw = htmlCommentRE.ReplaceAllString(raw, "")
	for _, re := range htmlDropElementREs {
		raw = re.ReplaceAllString(raw, "")
	}

	// Headings → Markdown "## " prefixes (preserve level loosely).
	raw = htmlHeadingOpenRE.ReplaceAllStringFunc(raw, func(m string) string {
		sub := htmlHeadingOpenRE.FindStringSubmatch(m)
		level := 2
		if len(sub) == 2 {
			level = int(sub[1][0]-'0') + 1
			if level > 6 {
				level = 6
			}
		}
		return "\n" + strings.Repeat("#", level) + " "
	})
	raw = htmlListItemRE.ReplaceAllString(raw, "\n- ")
	raw = htmlLineBreakRE.ReplaceAllString(raw, "\n")
	raw = htmlBlockBreakRE.ReplaceAllString(raw, "\n")

	// Drop all remaining tags.
	raw = htmlAnyTagRE.ReplaceAllString(raw, "")
	raw = html.UnescapeString(raw)

	// Tidy per-line whitespace.
	lines := strings.Split(normalizeNewlines(raw), "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		out = append(out, strings.TrimRight(strings.TrimLeft(ln, " \t"), " \t"))
	}
	return strings.Join(out, "\n")
}

// --- Shared helpers -----------------------------------------------------------

func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

var multiBlankRE = regexp.MustCompile(`\n{3,}`)

func collapseBlankLines(s string) string {
	return multiBlankRE.ReplaceAllString(s, "\n\n")
}
