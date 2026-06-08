package projector

import (
	"strings"
	"testing"
	"time"
)

func sampleContact() Entity {
	return Entity{
		Type:       TypeContact,
		ID:         "contact-01HX2345abcdef",
		Title:      "Alex Rivera",
		Body:       "Met through the NAVI early-tester group. Works on hardware-side firmware.",
		Confidence: 0.871,
		CreatedAt:  time.Date(2026, 4, 12, 19, 4, 11, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 5, 26, 8, 31, 0, 0, time.UTC),
		Provenance: []Provenance{{
			Source:    "telegram:acct-1",
			RecordID:  "tg-msg-9912",
			FetchedAt: time.Date(2026, 4, 12, 19, 4, 11, 0, time.UTC),
		}},
		Attrs: []Attr{{Key: "kind", Value: "person"}, {Key: "trust_level", Value: "high"}},
	}
}

func TestRender_Idempotent(t *testing.T) {
	e := sampleContact()
	a := Render(e)
	b := Render(e)
	if a != b {
		t.Fatalf("Render not idempotent:\n--- a ---\n%s\n--- b ---\n%s", a, b)
	}
	if ContentHash(a) != ContentHash(b) {
		t.Fatal("ContentHash differs for identical entity state")
	}
}

func TestRender_FrontmatterFields(t *testing.T) {
	out := Render(sampleContact())
	for _, want := range []string{
		"navi_entity_id: \"contact-01HX2345abcdef\"",
		"navi_entity_type: \"contact\"",
		"navi_confidence: 0.87",
		"navi_created_at: \"2026-04-12T19:04:11Z\"",
		"navi_updated_at: \"2026-05-26T08:31:00Z\"",
		"navi_provenance:",
		"source: \"telegram:acct-1\"",
		markerComment,
		"navi_kind: \"person\"",
		"navi_forget: false",
		"# Alex Rivera",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered output missing %q\n%s", want, out)
		}
	}
}

func TestRoundTrip_Parse(t *testing.T) {
	e := sampleContact()
	out := Render(e)
	doc, err := Parse(out)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if doc.EntityID != e.ID {
		t.Errorf("entity id: got %q want %q", doc.EntityID, e.ID)
	}
	if doc.EntityType != TypeContact {
		t.Errorf("entity type: got %q", doc.EntityType)
	}
	if doc.Title != "Alex Rivera" {
		t.Errorf("title: got %q", doc.Title)
	}
	if !strings.Contains(doc.Body, "hardware-side firmware") {
		t.Errorf("body lost: %q", doc.Body)
	}
	if doc.Attrs["kind"] != "person" {
		t.Errorf("attr kind: got %q", doc.Attrs["kind"])
	}
	if doc.Forget {
		t.Error("forget should be false")
	}
	if len(doc.Provenance) != 1 || doc.Provenance[0].Source != "telegram:acct-1" {
		t.Errorf("provenance lost: %+v", doc.Provenance)
	}
}

func TestParse_OwnerEdits(t *testing.T) {
	e := sampleContact()
	out := Render(e)
	// Simulate an owner editing the H1 name and the body, and setting forget.
	edited := strings.Replace(out, "# Alex Rivera", "# Alexandra Rivera", 1)
	edited = strings.Replace(edited, "navi_forget: false", "navi_forget: true", 1)
	edited = strings.Replace(edited, "hardware-side firmware.", "embedded systems and RF.", 1)

	doc, err := Parse(edited)
	if err != nil {
		t.Fatalf("Parse edited: %v", err)
	}
	if doc.Title != "Alexandra Rivera" {
		t.Errorf("edited title not parsed: %q", doc.Title)
	}
	if !doc.Forget {
		t.Error("owner forget gesture not parsed")
	}
	if !strings.Contains(doc.Body, "embedded systems and RF") {
		t.Errorf("edited body not parsed: %q", doc.Body)
	}
}

func TestParse_RejectsNonVaultFile(t *testing.T) {
	if _, err := Parse("# Just a markdown note\n\nno frontmatter here"); err == nil {
		t.Fatal("expected error for missing frontmatter")
	}
}

func TestDefaultPath_Layout(t *testing.T) {
	cases := []struct {
		e      Entity
		prefix string
	}{
		{sampleContact(), "contacts/alex-rivera-"},
		{Entity{Type: TypeKnowledge, ID: "k1", Title: "NAVI Architecture", Attrs: []Attr{{Key: "category", Value: "Projects"}}}, "knowledge/projects/navi-architecture-"},
		{Entity{Type: TypeMemory, ID: "m1", Title: "Conversation with Alex", CreatedAt: time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)}, "memories/2026/05/conversation-with-alex-"},
		{Entity{Type: TypeArtifact, ID: "a1", Title: "Q3 Report"}, "artifacts/q3-report-"},
	}
	for _, c := range cases {
		got := DefaultPath(c.e)
		if !strings.HasPrefix(got, c.prefix) {
			t.Errorf("DefaultPath(%s) = %q, want prefix %q", c.e.Type, got, c.prefix)
		}
		if !strings.HasSuffix(got, ".md") {
			t.Errorf("DefaultPath(%s) = %q, want .md suffix", c.e.Type, got)
		}
	}
}

func TestRenderIndex_ReadOnly(t *testing.T) {
	out := RenderIndex("All contacts by recency", []IndexEntry{
		{Title: "Alex Rivera", RelPath: "contacts/alex-rivera-01.md", Subtitle: "updated 2026-05-26"},
	})
	if !strings.Contains(out, "navi_readonly: true") {
		t.Error("index page missing readonly flag")
	}
	if !strings.Contains(out, "../contacts/alex-rivera-01.md") {
		t.Errorf("index link malformed:\n%s", out)
	}
	if !IsIndexPath(ContactsByRecencyPath()) {
		t.Error("ContactsByRecencyPath not recognized as index path")
	}
	if IsIndexPath("contacts/alex.md") {
		t.Error("entity path misclassified as index")
	}
}
