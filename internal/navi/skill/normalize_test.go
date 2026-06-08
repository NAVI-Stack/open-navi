package skill

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalize_RawText(t *testing.T) {
	input := "A custom prompt to echo things."
	res, err := Normalize(context.Background(), input, "echo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Source != "raw" {
		t.Errorf("expected source raw, got %s", res.Source)
	}

	if res.Entry.Skill.Name != "echo" {
		t.Errorf("expected name echo, got %s", res.Entry.Skill.Name)
	}
}

func TestNormalize_OpenClaw(t *testing.T) {
	// Create a temporary layout
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("failed to create skill dir: %v", err)
	}

	content := `---
name: search
description: Web search integration
---
This is the search prompt body.`

	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write SKILL.md: %v", err)
	}

	res, err := Normalize(context.Background(), skillDir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Source != "openclaw" {
		t.Errorf("expected source openclaw, got %s", res.Source)
	}

	if res.Entry.Skill.Name != "search" {
		t.Errorf("expected name search, got %s", res.Entry.Skill.Name)
	}

	if res.Entry.Skill.Body != "This is the search prompt body." {
		t.Errorf("expected body %q, got %q", "This is the search prompt body.", res.Entry.Skill.Body)
	}
}

func TestNormalize_OSS27YAMLDirectory(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "yaml-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("failed to create skill dir: %v", err)
	}

	content := `oss27_version: "1.0"
skill_id: "navi.test.normalize"
semver: "1.0.0"
display:
  name: "normalize-skill"
  description: "YAML normalization path"
interfaces:
  - name: "run"
    transport:
      type: "internal"
    input_schema:
      type: "object"
    output_schema:
      type: "object"
effects:
  side_effects: []
  risk_tier: "low"
  requires_confirmation: false
governance:
  publisher: "navi.test"
  signed: false
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write SKILL.yaml: %v", err)
	}

	res, err := Normalize(context.Background(), skillDir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Entry.Spec == nil {
		t.Fatal("expected OSS-27 spec to be loaded")
	}
	if res.Entry.Skill.ID != "navi.test.normalize" {
		t.Fatalf("unexpected skill id: %s", res.Entry.Skill.ID)
	}
}

func TestNormalize_URL(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("---\nname: remote-test\n---\nRemote body test."))
	}))
	defer ts.Close()

	ctx := context.WithValue(context.Background(), testTransportKey, ts.Client().Transport)
	res, err := Normalize(ctx, ts.URL, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Source != "url" {
		t.Errorf("expected source url, got %s", res.Source)
	}

	if res.Entry.Skill.Name != "remote-test" {
		t.Errorf("expected name remote-test, got %s", res.Entry.Skill.Name)
	}

	if res.Entry.Skill.Body != "Remote body test." {
		t.Errorf("expected body %q, got %q", "Remote body test.", res.Entry.Skill.Body)
	}
}
