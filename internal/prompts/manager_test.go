package prompts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnsureDefaultsSeedsAllKinds(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)
	if err := m.EnsureDefaults(); err != nil {
		t.Fatal(err)
	}
	for _, k := range AllKinds() {
		p := filepath.Join(dir, string(k)+".md")
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("missing seeded file %s: %v", p, err)
		}
	}
}

func TestRenderReloadLifecycle(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)
	if err := m.EnsureDefaults(); err != nil {
		t.Fatal(err)
	}
	if err := m.Reload(); err != nil {
		t.Fatal(err)
	}
	data := ChatSystemData{
		ExperienceControl:  "test experience",
		CurrentTimeRFC3339: time.Now().UTC().Format("Monday, January 2, 2006 3:04 PM"),
		ChatID:             "sess-1",
	}
	out, err := m.Render(KindChatSystem, data, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	sn, ok := m.Snapshot(KindChatSystem)
	if !ok {
		t.Fatal("expected chat system snapshot")
	}
	if strings.Contains(sn.Raw, "ExperienceControl") {
		if !strings.Contains(out, "test experience") {
			t.Fatalf("expected experience control in output, got %q", truncate(out, 200))
		}
	} else if !strings.Contains(out, "helpful AI Companion") {
		t.Fatalf("expected rendered embedded chat prompt, got %q", truncate(out, 200))
	}
	if strings.Contains(out, "{{") {
		t.Fatalf("expected template to render fully, got %q", truncate(out, 200))
	}
}

func TestRuntimeOverrideWins(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)
	if err := m.EnsureDefaults(); err != nil {
		t.Fatal(err)
	}
	_ = m.Reload()
	ov := "strictly this"
	out, err := m.Render(KindChatSystem, ChatSystemData{}, RenderOptions{RuntimeOverride: ov})
	if err != nil || out != ov {
		t.Fatalf("got %q err=%v", out, err)
	}
}

func TestUserFileOverridesEmbedded(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)
	if err := m.EnsureDefaults(); err != nil {
		t.Fatal(err)
	}
	custom := "## Custom\nHello {{.ExperienceControl}}\n"
	path := filepath.Join(dir, string(KindSummarizerSystem)+".md")
	if err := os.WriteFile(path, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Reload(); err != nil {
		t.Fatal(err)
	}
	out, err := m.Render(KindSummarizerSystem, struct{ ExperienceControl string }{ExperienceControl: "X"}, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Hello X") {
		t.Fatalf("expected custom template output, got %q", out)
	}
}

func TestMissingFileAfterSeedRegeneratesOnReload(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)
	if err := m.EnsureDefaults(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, string(KindSummarizerSystem)+".md")
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := m.Reload(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("expected file re-seeded: %v", err)
	}
	out, err := m.Render(KindSummarizerSystem, struct{}{}, RenderOptions{})
	if err != nil || !strings.Contains(out, "summarize") {
		t.Fatalf("render: %v out=%q", err, truncate(out, 120))
	}
}

func TestInvalidTemplateReloadDoesNotSwap(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)
	if err := m.EnsureDefaults(); err != nil {
		t.Fatal(err)
	}
	if err := m.Reload(); err != nil {
		t.Fatal(err)
	}
	okOut, err := m.Render(KindSummarizerSystem, struct{}{}, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	badPath := filepath.Join(dir, string(KindSummarizerSystem)+".md")
	if err := os.WriteFile(badPath, []byte("bad {{.Missing.Field"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Reload(); err == nil {
		t.Fatal("expected Reload error for invalid template")
	}
	out2, err := m.Render(KindSummarizerSystem, struct{}{}, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if out2 != okOut {
		t.Fatalf("template changed after failed reload")
	}
}

func TestReloadEmbeddedAllowsLaterDiskRecovery(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)
	if err := m.EnsureDefaults(); err != nil {
		t.Fatal(err)
	}
	if err := m.Reload(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, string(KindSummarizerSystem)+".md")
	if err := os.WriteFile(p, []byte("bad {{.Missing.Field"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Reload(); err == nil {
		t.Fatal("expected Reload error for invalid template")
	}
	if err := m.ReloadEmbedded(); err != nil {
		t.Fatalf("ReloadEmbedded: %v", err)
	}
	out, err := m.Render(KindSummarizerSystem, struct{}{}, RenderOptions{})
	if err != nil || !strings.Contains(out, "summarize") {
		t.Fatalf("embedded fallback render: %v out=%q", err, truncate(out, 120))
	}
	if err := os.WriteFile(p, []byte("## Fixed\nHello {{.ExperienceControl}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Reload(); err != nil {
		t.Fatalf("Reload after fixing disk template: %v", err)
	}
	out, err = m.Render(KindSummarizerSystem, struct{ ExperienceControl string }{ExperienceControl: "again"}, RenderOptions{})
	if err != nil || !strings.Contains(out, "Hello again") {
		t.Fatalf("recovered disk render: %v out=%q", err, truncate(out, 120))
	}
}

func TestWhitespaceOnlyFileUsesEmbedded(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)
	if err := m.EnsureDefaults(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, string(KindSummarizerSystem)+".md")
	if err := os.WriteFile(p, []byte("  \n\t  "), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Reload(); err != nil {
		t.Fatal(err)
	}
	out, err := m.Render(KindSummarizerSystem, struct{}{}, RenderOptions{})
	if err != nil || !strings.Contains(out, "summarize") {
		t.Fatalf("expected embedded fallback body, err=%v out=%q", err, truncate(out, 80))
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "summarize") {
		t.Fatalf("expected whitespace-only file rewritten from embed on disk, got %q", truncate(string(b), 80))
	}
}

func TestSnapshotDiskAndEmbed(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)
	if err := m.EnsureDefaults(); err != nil {
		t.Fatal(err)
	}
	sn, ok := m.Snapshot(KindSummarizerSystem)
	if !ok || sn.Source != "disk" || !strings.Contains(sn.Raw, "summarize") {
		t.Fatalf("Snapshot: ok=%v %+v", ok, sn)
	}
	em := New("")
	sn2, ok2 := em.Snapshot(KindSummarizerSystem)
	if !ok2 || sn2.Source != "embed" {
		t.Fatalf("embed Snapshot: ok=%v %+v", ok2, sn2)
	}
	p := filepath.Join(dir, string(KindSummarizerSystem)+".md")
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	sn3, ok3 := m.Snapshot(KindSummarizerSystem)
	if !ok3 || sn3.Source != "missing_fallback" {
		t.Fatalf("missing Snapshot: ok=%v %+v", ok3, sn3)
	}
	if err := os.WriteFile(p, []byte(strings.Repeat("x", maxPromptFileSize+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	sn4, ok4 := m.Snapshot(KindSummarizerSystem)
	if !ok4 || sn4.Source != "oversized_fallback" {
		t.Fatalf("oversized Snapshot: ok=%v %+v", ok4, sn4)
	}
	_, ok5 := m.Snapshot(Kind("unknown/kind"))
	if ok5 {
		t.Fatal("expected unknown kind false")
	}
}

func TestEmbeddedManager(t *testing.T) {
	m, err := EmbeddedManager()
	if err != nil {
		t.Fatal(err)
	}
	out, err := m.Render(KindSummarizerSystem, struct{}{}, RenderOptions{})
	if err != nil || !strings.Contains(out, "summarize") {
		t.Fatalf("embedded: %v %q", err, truncate(out, 80))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func TestRenderLines(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)
	if err := m.EnsureDefaults(); err != nil {
		t.Fatal(err)
	}

	// NCOS identity rules should produce multiple lines
	lines, err := m.RenderLines(KindNCOSIdentityRules, NCOSRulesData{CurrentTime: "Monday, May 5, 2025 2:00 PM UTC"}, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) < 5 {
		t.Fatalf("expected at least 5 identity rules, got %d", len(lines))
	}
	found := false
	for _, l := range lines {
		if strings.Contains(l, "Monday, May 5, 2025 2:00 PM UTC") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected CurrentTime to be rendered in identity rules")
	}
}

func TestRenderLinesSkipsComments(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)
	if err := m.EnsureDefaults(); err != nil {
		t.Fatal(err)
	}
	// Write a custom file with comments
	custom := "# This is a comment\nRule one\n\n# Another comment\nRule two\n"
	p := filepath.Join(dir, string(KindNCOSSecurityRules)+".md")
	if err := os.WriteFile(p, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Reload(); err != nil {
		t.Fatal(err)
	}
	lines, err := m.RenderLines(KindNCOSSecurityRules, nil, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || lines[0] != "Rule one" || lines[1] != "Rule two" {
		t.Fatalf("expected [Rule one, Rule two], got %v", lines)
	}
}

func TestRenderMap(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)
	if err := m.EnsureDefaults(); err != nil {
		t.Fatal(err)
	}
	modes, err := m.RenderMap(KindDirectiveModes, nil, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := modes["chat"]; !ok {
		t.Fatal("expected 'chat' key in directive modes")
	}
	if _, ok := modes["act"]; !ok {
		t.Fatal("expected 'act' key in directive modes")
	}
	if _, ok := modes["default"]; !ok {
		t.Fatal("expected 'default' key in directive modes")
	}
}

func TestRenderLinesOverride(t *testing.T) {
	dir := t.TempDir()
	m := New(dir)
	if err := m.EnsureDefaults(); err != nil {
		t.Fatal(err)
	}
	// Override security rules with custom content
	custom := "Custom security rule A\nCustom security rule B\n"
	p := filepath.Join(dir, string(KindNCOSSecurityRules)+".md")
	if err := os.WriteFile(p, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Reload(); err != nil {
		t.Fatal(err)
	}
	lines, err := m.RenderLines(KindNCOSSecurityRules, nil, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || lines[0] != "Custom security rule A" {
		t.Fatalf("expected custom override, got %v", lines)
	}
}
