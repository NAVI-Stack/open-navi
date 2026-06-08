// Package projector renders World Model entities to owner-facing Markdown files
// (YAML frontmatter + Markdown body) and parses owner-edited files back into a
// structured Doc. It is the entity → Markdown half of the Memory Vault
// (memory-projection-v1.md §8).
//
// Two contracts the rest of the Vault relies on:
//
//   - Idempotent rendering. Render is a pure function of its Entity input, so the
//     same entity state produces byte-identical output. Reprojecting an unchanged
//     entity never changes the file (acceptance: idempotent reprojection).
//   - No editor lock-in. Output is plain YAML frontmatter + Markdown body that
//     reads in Obsidian, Nextcloud Notes, VS Code, and `cat`. No proprietary
//     blocks (spec §3, §8).
//
// Frontmatter has two regions delineated by a marker comment: projector-managed
// fields above (rewritten on every reprojection without consulting prior file
// content) and owner-editable fields below. The marker is advisory — the diff
// parser reads owner edits wherever they land; the marker just tells a human
// which fields NAVI will overwrite.
package projector

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Entity classes the Vault projects in V1 (memory-projection-v1.md §7).
const (
	TypeContact   = "contact"
	TypeKnowledge = "knowledge"
	TypeMemory    = "memory"
	TypeArtifact  = "artifact"
)

// markerComment delineates projector-managed frontmatter (above) from
// owner-editable frontmatter (below). It is a YAML comment line, ignored on parse.
const markerComment = "# fields below this line are owner-editable; fields above are projector-managed"

// Provenance is one source attribution rendered into navi_provenance.
type Provenance struct {
	Source    string
	RecordID  string
	FetchedAt time.Time
}

// Attr is one owner-editable scalar frontmatter field (rendered as navi_<Key>).
// Order is preserved as given so rendering stays deterministic.
type Attr struct {
	Key   string
	Value string
}

// Entity is the projector's neutral input — the worker assembles it from the
// concrete World Model store types (Contact, Fact, Memory, Artifact).
type Entity struct {
	Type       string
	ID         string
	Title      string // rendered as the H1 heading
	Body       string // owner-editable Markdown body
	Confidence float64
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Provenance []Provenance
	Attrs      []Attr // owner-editable scalar frontmatter fields (kind, category, ...)
}

// Doc is the parsed result of an owner-edited (or projector-written) Vault file.
type Doc struct {
	EntityID   string
	EntityType string
	Confidence float64
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Provenance []Provenance
	Forget     bool              // navi_forget: true — owner Forget gesture
	Attrs      map[string]string // owner-editable scalar fields, key without the navi_ prefix
	Title      string            // first H1
	Body       string            // body text after the H1
}

// Render returns the full file content for an entity. Pure and deterministic.
func Render(e Entity) string {
	var b strings.Builder
	b.WriteString("---\n")
	// Projector-managed region.
	writeScalar(&b, "navi_entity_id", e.ID)
	writeScalar(&b, "navi_entity_type", e.Type)
	b.WriteString("navi_confidence: " + strconv.FormatFloat(roundConf(e.Confidence), 'f', 2, 64) + "\n")
	writeScalar(&b, "navi_created_at", formatTime(e.CreatedAt))
	writeScalar(&b, "navi_updated_at", formatTime(e.UpdatedAt))
	if len(e.Provenance) == 0 {
		b.WriteString("navi_provenance: []\n")
	} else {
		b.WriteString("navi_provenance:\n")
		for _, p := range e.Provenance {
			b.WriteString("  - source: " + yamlQuote(p.Source) + "\n")
			if p.RecordID != "" {
				b.WriteString("    record_id: " + yamlQuote(p.RecordID) + "\n")
			}
			if !p.FetchedAt.IsZero() {
				b.WriteString("    fetched_at: " + yamlQuote(formatTime(p.FetchedAt)) + "\n")
			}
		}
	}
	// Marker + owner-editable region.
	b.WriteString(markerComment + "\n")
	for _, a := range e.Attrs {
		writeScalar(&b, "navi_"+a.Key, a.Value)
	}
	b.WriteString("navi_forget: false\n")
	b.WriteString("---\n\n")
	// Body.
	b.WriteString("# " + strings.TrimSpace(e.Title) + "\n")
	body := strings.TrimRight(e.Body, "\n")
	if body != "" {
		b.WriteString("\n" + body + "\n")
	}
	return b.String()
}

// DefaultPath returns the relative Vault path for an entity per the §8 layout.
// The worker reuses an already-mapped path when one exists so owner edits to a
// title do not churn filenames; DefaultPath is only used for first projection.
func DefaultPath(e Entity) string {
	slug := Slug(e.Title)
	if slug == "" {
		slug = "untitled"
	}
	short := shortID(e.ID)
	switch e.Type {
	case TypeContact:
		return "contacts/" + slug + "-" + short + ".md"
	case TypeKnowledge:
		topic := ""
		for _, a := range e.Attrs {
			if a.Key == "category" {
				topic = Slug(a.Value)
				break
			}
		}
		if topic == "" {
			topic = "general"
		}
		return "knowledge/" + topic + "/" + slug + "-" + short + ".md"
	case TypeMemory:
		t := e.CreatedAt
		if t.IsZero() {
			t = time.Now().UTC()
		}
		t = t.UTC()
		return fmt.Sprintf("memories/%04d/%02d/%s-%s.md", t.Year(), int(t.Month()), slug, short)
	case TypeArtifact:
		return "artifacts/" + slug + "-" + short + ".md"
	default:
		return "misc/" + slug + "-" + short + ".md"
	}
}

// ContentHash returns the stable hash of rendered (or read) file content. Used to
// distinguish projector writes from owner edits and to detect drift.
func ContentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

// Parse splits a Vault file into frontmatter + body and extracts the Doc fields.
// It is tolerant of owner reformatting (any valid YAML frontmatter parses); it is
// strict only about the leading frontmatter fence so a non-Vault file is rejected.
func Parse(content string) (Doc, error) {
	var doc Doc
	doc.Attrs = map[string]string{}
	rest := content
	if !strings.HasPrefix(rest, "---\n") && !strings.HasPrefix(rest, "---\r\n") {
		return doc, fmt.Errorf("projector: missing frontmatter fence")
	}
	rest = strings.TrimPrefix(rest, "---\r\n")
	rest = strings.TrimPrefix(rest, "---\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return doc, fmt.Errorf("projector: unterminated frontmatter")
	}
	fm := rest[:end]
	body := rest[end:]
	// Drop the closing fence line.
	if i := strings.Index(body, "\n"); i >= 0 {
		body = body[i+1:]
	}

	raw := map[string]any{}
	if err := yaml.Unmarshal([]byte(fm), &raw); err != nil {
		return doc, fmt.Errorf("projector: parse frontmatter: %w", err)
	}
	doc.EntityID = asString(raw["navi_entity_id"])
	doc.EntityType = asString(raw["navi_entity_type"])
	doc.Confidence = asFloat(raw["navi_confidence"])
	doc.CreatedAt = asTime(raw["navi_created_at"])
	doc.UpdatedAt = asTime(raw["navi_updated_at"])
	doc.Forget = asBool(raw["navi_forget"])
	doc.Provenance = parseProvenance(raw["navi_provenance"])
	for k, v := range raw {
		if !strings.HasPrefix(k, "navi_") {
			continue
		}
		switch k {
		case "navi_entity_id", "navi_entity_type", "navi_confidence",
			"navi_created_at", "navi_updated_at", "navi_provenance", "navi_forget":
			continue
		}
		doc.Attrs[strings.TrimPrefix(k, "navi_")] = asString(v)
	}

	title, rest2 := splitTitle(body)
	doc.Title = title
	doc.Body = strings.TrimRight(rest2, "\n")
	return doc, nil
}

// Slug converts arbitrary text to a filesystem-safe slug.
func Slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func shortID(id string) string {
	clean := strings.ReplaceAll(id, "-", "")
	if len(clean) >= 8 {
		return clean[:8]
	}
	if clean == "" {
		return "00000000"
	}
	return clean
}

func splitTitle(body string) (title, rest string) {
	lines := strings.Split(body, "\n")
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "# ") {
			title = strings.TrimSpace(strings.TrimPrefix(t, "# "))
			rest = strings.Join(lines[i+1:], "\n")
			return title, strings.TrimLeft(rest, "\n")
		}
	}
	return "", strings.TrimLeft(body, "\n")
}

func writeScalar(b *strings.Builder, key, value string) {
	b.WriteString(key + ": " + yamlQuote(value) + "\n")
}

// yamlQuote double-quotes a scalar so multi-word and special values round-trip.
func yamlQuote(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", " ")
	return "\"" + s + "\""
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func roundConf(c float64) float64 {
	if c < 0 {
		return 0
	}
	if c > 1 {
		return 1
	}
	return c
}

func asString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(x)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	default:
		return fmt.Sprintf("%v", x)
	}
}

func asFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f
	default:
		return 0
	}
}

func asBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		b, _ := strconv.ParseBool(strings.TrimSpace(x))
		return b
	default:
		return false
	}
}

func asTime(v any) time.Time {
	s := asString(v)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05Z07:00", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func parseProvenance(v any) []Provenance {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []Provenance
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, Provenance{
			Source:    asString(m["source"]),
			RecordID:  asString(m["record_id"]),
			FetchedAt: asTime(m["fetched_at"]),
		})
	}
	return out
}

// SortAttrs returns attrs sorted by key for deterministic rendering when the
// caller does not impose its own order.
func SortAttrs(attrs []Attr) []Attr {
	out := append([]Attr(nil), attrs...)
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
