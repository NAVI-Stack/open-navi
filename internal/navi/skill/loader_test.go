package skill

import (
	"os"
	"path/filepath"
	"testing"
)

// makeSkill writes a SKILL.md in dir/name/ and returns the skill dir path.
func makeSkill(t *testing.T, baseDir, name, content string) string {
	t.Helper()
	skillDir := filepath.Join(baseDir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return skillDir
}

const simpleSkillMD = `---
name: my-skill
description: "Does something useful"
metadata:
  navi:
    emoji: "🔧"
---
# My Skill

Instructions go here.
`

// ---------------------------------------------------------------------------
// LoadFromDir
// ---------------------------------------------------------------------------

func TestLoadFromDir_Empty(t *testing.T) {
	dir := t.TempDir()
	entries, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestLoadFromDir_MissingDir(t *testing.T) {
	entries, err := LoadFromDir("/nonexistent/path/skills")
	if err != nil {
		t.Fatalf("missing dir should return nil,nil; got err: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for missing dir, got %d", len(entries))
	}
}

func TestLoadPluginSkills_NaviCoderRepo(t *testing.T) {
	pluginSkillPath := filepath.Join("..", "..", "..", "plugins", "navi-coder", "skills", "repo")
	entries, err := LoadPluginSkills(map[string][]string{"navi-coder": []string{pluginSkillPath}})
	if err != nil {
		t.Fatalf("LoadPluginSkills: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one navi-coder skill, got %d", len(entries))
	}
	entry := entries[0]
	if entry.SourcePluginID != "navi-coder" || entry.Skill.ID != "navi.coder.repo" {
		t.Fatalf("unexpected plugin skill: source=%q id=%q", entry.SourcePluginID, entry.Skill.ID)
	}
	if entry.Spec == nil || len(entry.Spec.Interfaces) != 4 {
		t.Fatalf("expected four navi.coder.repo interfaces, got %#v", entry.Spec)
	}
}

func TestLoadFromDir_SingleSkill(t *testing.T) {
	dir := t.TempDir()
	makeSkill(t, dir, "my-skill", simpleSkillMD)

	entries, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Skill.Name != "my-skill" {
		t.Errorf("expected name %q, got %q", "my-skill", e.Skill.Name)
	}
	if e.Skill.Description != "Does something useful" {
		t.Errorf("unexpected description: %q", e.Skill.Description)
	}
	if e.Metadata.Emoji != "🔧" {
		t.Errorf("expected emoji 🔧, got %q", e.Metadata.Emoji)
	}
	if e.Skill.Body == "" {
		t.Error("body should not be empty")
	}
}

func TestLoadFromDir_RejectsSubprocessPythonWithoutRuntime(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "py-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	spec := `oss27_version: "1.0"
skill_id: "navi.test.python"
semver: "1.0.0"
display:
  name: "python-skill"
  description: "Needs runtime metadata"
interfaces:
  - name: "run"
    transport:
      type: "subprocess_python"
      subprocess_python:
        entrypoint: "main.py"
    input_schema:
      type: "object"
    output_schema:
      type: "object"
effects:
  side_effects: []
  risk_tier: "low"
  requires_confirmation: false
security:
  auth: []
  data_access:
    pii: "none"
    secrets: "forbidden"
  sandbox:
    required: false
    network_egress: []
governance:
  publisher: "navi.test"
  signed: false
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.yaml"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected invalid subprocess_python skill to be skipped, got %d entries", len(entries))
	}
}

func TestLoadSkillEntryParsesUISurfaces(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "ui-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	spec := `oss27_version: "1.0"
skill_id: "ui-skill"
semver: "1.0.0"
display:
  name: "UI Skill"
  description: "Provides a headless UI surface."
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
security:
  auth: []
  data_access:
    pii: "none"
    secrets: "forbidden"
  sandbox:
    required: false
    network_egress: []
governance:
  publisher: "navi.test"
  signed: false
ui_surfaces:
  - id: "ui-skill.console"
    title: "UI Skill"
    target_surfaces: ["console"]
    render_mode: "headless"
    actions:
      - id: "run"
        interface: "run"
`
	path := filepath.Join(skillDir, "SKILL.yaml")
	if err := os.WriteFile(path, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	entry, err := loadSkillEntry(skillDir, path)
	if err != nil {
		t.Fatalf("loadSkillEntry: %v", err)
	}
	if entry.Spec == nil || len(entry.Spec.UISurfaces) != 1 {
		t.Fatalf("expected one UI surface, got %#v", entry.Spec)
	}
	if entry.Spec.UISurfaces[0].RenderMode != "headless" || entry.Spec.UISurfaces[0].Actions[0].Interface != "run" {
		t.Fatalf("unexpected UI surface: %#v", entry.Spec.UISurfaces[0])
	}
}

func TestLoadSkillEntry_ShippedBuiltinsRemainValid(t *testing.T) {
	paths := []string{
		filepath.Join("..", "..", "..", "plugins", "core-scheduler", "skills", "core-scheduler", "SKILL.yaml"),
		filepath.Join("..", "..", "..", "plugins", "llm-router", "skills", "llm-router", "SKILL.yaml"),
		filepath.Join("..", "..", "..", "plugins", "i2c", "skills", "i2c", "SKILL.yaml"),
		filepath.Join("..", "..", "..", "plugins", "spi", "skills", "spi", "SKILL.yaml"),
	}

	for _, manifestPath := range paths {
		skillDir := filepath.Dir(manifestPath)
		entry, err := loadSkillEntry(skillDir, manifestPath)
		if err != nil {
			t.Fatalf("loadSkillEntry(%s): %v", manifestPath, err)
		}
		if entry.Spec == nil {
			t.Fatalf("expected spec for %s", manifestPath)
		}
		if len(entry.Spec.Interfaces) == 0 {
			t.Fatalf("expected interfaces for %s", manifestPath)
		}
		for _, iface := range entry.Spec.Interfaces {
			if iface.Transport.Type == "" {
				t.Fatalf("interface %q in %s missing transport type", iface.Name, manifestPath)
			}
		}
	}
}

func TestLoadPluginSkillsUsesDeclaredPathsOnly(t *testing.T) {
	root := t.TempDir()
	declared := makeSkill(t, root, "declared", simpleSkillMD)
	makeSkill(t, root, "not-declared", `---
name: not-declared
description: "Should not load"
---
Not declared.
`)

	entries, err := LoadPluginSkills(map[string][]string{
		"navi.test.plugin": {declared},
	})
	if err != nil {
		t.Fatalf("LoadPluginSkills: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one declared plugin skill, got %d", len(entries))
	}
	if entries[0].Skill.Name != "my-skill" {
		t.Fatalf("unexpected skill name %q", entries[0].Skill.Name)
	}
	if entries[0].SourcePluginID != "navi.test.plugin" {
		t.Fatalf("SourcePluginID = %q", entries[0].SourcePluginID)
	}
}

func TestLoadFromDir_NameFallsBackToDir(t *testing.T) {
	dir := t.TempDir()
	// No frontmatter name — should fall back to directory name
	makeSkill(t, dir, "fallback-name", "# No frontmatter\nJust a body.\n")

	entries, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Skill.Name != "fallback-name" {
		t.Errorf("expected fallback name %q, got %q", "fallback-name", entries[0].Skill.Name)
	}
}

func TestLoadFromDir_SkipsSubdirWithoutSKILLMD(t *testing.T) {
	dir := t.TempDir()
	makeSkill(t, dir, "valid-skill", simpleSkillMD)
	// Create a dir without SKILL.md
	os.MkdirAll(filepath.Join(dir, "no-skill-here"), 0o755)

	entries, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry (ignoring dir without SKILL.md), got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// SkillsLoader 2-tier deduplication
// ---------------------------------------------------------------------------

func TestSkillsLoader_WorkspaceTakesPriorityOverGlobal(t *testing.T) {
	workspace := t.TempDir()
	globalDir := t.TempDir()

	// Same skill name in both tiers — workspace should win
	makeSkill(t, filepath.Join(workspace, "skills"), "shared-skill", `---
name: shared-skill
description: "Workspace version"
---
Workspace body.
`)
	makeSkill(t, globalDir, "shared-skill", `---
name: shared-skill
description: "Global version"
---
Global body.
`)

	loader := &SkillsLoader{
		workspaceSkills: filepath.Join(workspace, "skills"),
		globalSkills:    globalDir,
	}

	skills, err := loader.ListSkills()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill (deduped), got %d", len(skills))
	}
	if skills[0].Skill.Description != "Workspace version" {
		t.Errorf("expected workspace version to win, got: %q", skills[0].Skill.Description)
	}
}

func TestSkillsLoader_GlobalAppearsWhenNoWorkspaceVersion(t *testing.T) {
	workspace := t.TempDir()
	globalDir := t.TempDir()

	makeSkill(t, globalDir, "only-global", simpleSkillMD)

	loader := &SkillsLoader{
		workspaceSkills: filepath.Join(workspace, "skills"), // won't exist
		globalSkills:    globalDir,
	}

	skills, err := loader.ListSkills()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 global skill, got %d", len(skills))
	}
}

func TestNewLoader_AcceptsDirectSkillsDirectory(t *testing.T) {
	workspaceSkills := filepath.Join(t.TempDir(), "skills")
	makeSkill(t, workspaceSkills, "direct-skill", simpleSkillMD)

	loader := NewLoader(workspaceSkills)
	loader.globalSkills = t.TempDir()

	skills, err := loader.ListSkills()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill from direct skills dir, got %d", len(skills))
	}
	if skills[0].Skill.Description != "Does something useful" {
		t.Fatalf("expected direct skills directory entry to load, got %+v", skills[0].Skill)
	}
}

func TestSkillsLoader_SkipsSkillWithMissingBin(t *testing.T) {
	globalDir := t.TempDir()
	makeSkill(t, globalDir, "needs-missing-bin", `---
name: needs-missing-bin
description: "Needs a binary that doesn't exist"
metadata:
  navi:
    requires:
      bins: ["__nonexistent_binary_xyz__"]
---
Body.
`)

	loader := &SkillsLoader{
		workspaceSkills: t.TempDir(),
		globalSkills:    globalDir,
	}

	skills, err := loader.ListSkills()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected non-activatable skill to still be listed, got %d skills", len(skills))
	}
	if skills[0].Activatable {
		t.Errorf("skill with missing binary should be marked non-activatable")
	}
	if len(skills[0].ReasonsUnbound) == 0 {
		t.Errorf("expected unmet requirement reasons to be recorded")
	}
}

func TestSkillsLoader_IncludesSkillWhenBinExists(t *testing.T) {
	globalDir := t.TempDir()
	// "python" should exist because subprocess_python skills depend on it.
	makeSkill(t, globalDir, "needs-go", `---
name: needs-go
description: "Needs python interpreter"
metadata:
  navi:
    requires:
      bins: ["python"]
---
Body.
`)

	loader := &SkillsLoader{
		workspaceSkills: t.TempDir(),
		globalSkills:    globalDir,
	}

	skills, err := loader.ListSkills()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 1 {
		t.Errorf("expected skill with 'python' binary to be included, got %d skills", len(skills))
	}
}

func TestSkillsLoaderDoesNotScanRootSkills(t *testing.T) {
	loader := NewLoader("workspace")
	if loader.workspaceSkills == "" {
		t.Fatal("expected workspaceSkills to be set")
	}
	if loader.globalSkills == "" {
		t.Fatal("expected globalSkills to be set")
	}
	// The loader should only have workspace and global tiers.
	// Verify there is no builtin field by listing skills with both tiers empty.
	loader.workspaceSkills = t.TempDir()
	loader.globalSkills = t.TempDir()
	skills, err := loader.ListSkills()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("expected 0 skills from empty tiers, got %d", len(skills))
	}
}

// ---------------------------------------------------------------------------
// Frontmatter parsing
// ---------------------------------------------------------------------------

func TestParseFrontmatter_BasicFields(t *testing.T) {
	content := `---
name: test-skill
description: "A test skill"
metadata:
  navi:
    emoji: "🧪"
---
Body here.
`
	name, desc, meta, err := ParseFrontmatter(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "test-skill" {
		t.Errorf("name: got %q, want %q", name, "test-skill")
	}
	if desc != "A test skill" {
		t.Errorf("desc: got %q, want %q", desc, "A test skill")
	}
	if meta.Emoji != "🧪" {
		t.Errorf("emoji: got %q, want 🧪", meta.Emoji)
	}
}

func TestParseFrontmatter_InlineBinsList(t *testing.T) {
	content := `---
name: gh-skill
description: "Uses gh CLI"
metadata:
  navi:
    requires:
      bins: ["gh", "git"]
---
Body.
`
	_, _, meta, err := ParseFrontmatter(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(meta.RequiresBins) != 2 {
		t.Fatalf("expected 2 bins, got %d: %v", len(meta.RequiresBins), meta.RequiresBins)
	}
	if meta.RequiresBins[0] != "gh" || meta.RequiresBins[1] != "git" {
		t.Errorf("unexpected bins: %v", meta.RequiresBins)
	}
}

func TestParseFrontmatter_EnvList(t *testing.T) {
	content := `---
name: secret-skill
description: "Needs a token"
metadata:
  navi:
    requires:
      env: ["GITHUB_TOKEN"]
---
Body.
`
	_, _, meta, err := ParseFrontmatter(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(meta.RequiresEnv) != 1 || meta.RequiresEnv[0] != "GITHUB_TOKEN" {
		t.Errorf("unexpected env: %v", meta.RequiresEnv)
	}
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	_, _, _, err := ParseFrontmatter("# Just a plain markdown file\nNo frontmatter here.\n")
	if err == nil {
		t.Error("expected error for file without frontmatter")
	}
}

// ---------------------------------------------------------------------------
// StripFrontmatter
// ---------------------------------------------------------------------------

func TestStripFrontmatter_RemovesBlock(t *testing.T) {
	content := "---\nname: foo\n---\n# Body\nContent here.\n"
	body := StripFrontmatter(content)
	if body != "# Body\nContent here." {
		t.Errorf("unexpected body: %q", body)
	}
}

func TestStripFrontmatter_Noop_WhenNoFrontmatter(t *testing.T) {
	content := "# Plain body\nNo frontmatter."
	body := StripFrontmatter(content)
	if body != content {
		t.Errorf("expected body unchanged, got %q", body)
	}
}

// ---------------------------------------------------------------------------
// FormatPrompt
// ---------------------------------------------------------------------------

func TestFormatPrompt_Empty(t *testing.T) {
	prompt := FormatPrompt(nil)
	if prompt != "" {
		t.Errorf("expected empty prompt for no skills, got %q", prompt)
	}
}

func TestFormatPrompt_ListsSkills(t *testing.T) {
	entries := []SkillEntry{
		{Skill: Skill{Name: "github", Description: "GitHub CLI"}, Metadata: SkillMetadata{Emoji: "🐙"}},
		{Skill: Skill{Name: "summarize", Description: "Summarize content"}, Metadata: SkillMetadata{Emoji: "📄"}},
	}
	prompt := FormatPrompt(entries)
	for _, e := range entries {
		if !containsStr(prompt, e.Skill.Name) {
			t.Errorf("prompt missing skill %q", e.Skill.Name)
		}
		if !containsStr(prompt, e.Skill.Description) {
			t.Errorf("prompt missing description for %q", e.Skill.Name)
		}
	}
}

func containsStr(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}()
}
