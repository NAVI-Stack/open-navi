package navi

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-navi/navi/internal/navi/experience"
)

func TestNormalizeExperienceModeUsesSupportedExperienceProfiles(t *testing.T) {
	t.Parallel()

	cases := map[ExperienceMode]ExperienceMode{
		"":                    ExperienceModeStandard,
		"navi":                ExperienceModeStandard,
		"unsupported-profile": ExperienceModeStandard,
		"custom":              ExperienceModeStandard,
		"wizard":              ExperienceModeWizard,
	}

	for input, want := range cases {
		if got := NormalizeExperienceMode(input); got != want {
			t.Fatalf("NormalizeExperienceMode(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestExperienceManagerIgnoresUnsupportedOverrideFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "unsupported.yaml"), []byte(`
id: unsupported-profile
system_prompt: deprecated prompt should be ignored
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wizard.yaml"), []byte(`
id: wizard
display_name: Guided Setup
persona_modules:
  - module_id: onboarding_override
    scope: global
    strength: 1
    trait_contributions:
      structure_level: 0.95
      initiative_style: 0.85
`), 0o644); err != nil {
		t.Fatal(err)
	}

	engine := NewExperienceManager(dir, ExperienceModeStandard)

	standard, ok := engine.Get(ExperienceModeStandard)
	if !ok {
		t.Fatal("expected standard experience profile")
	}
	if standard.DisplayName != "NAVI" {
		t.Fatalf("expected standard display name to stay NAVI, got %q", standard.DisplayName)
	}
	if len(standard.Profile.PersonaModules) == 0 || standard.Profile.PersonaModules[0].ModuleID != "standard_navi_interaction" {
		t.Fatalf("expected standard experience profile to remain intact, got %+v", standard.Profile.PersonaModules)
	}

	wizard, ok := engine.Get(ExperienceModeWizard)
	if !ok {
		t.Fatal("expected wizard experience profile")
	}
	if wizard.DisplayName != "Guided Setup" {
		t.Fatalf("expected wizard display name override, got %q", wizard.DisplayName)
	}
	if len(wizard.Profile.PersonaModules) != 1 || wizard.Profile.PersonaModules[0].ModuleID != "onboarding_override" {
		t.Fatalf("expected wizard override module, got %+v", wizard.Profile.PersonaModules)
	}
	if wizard.Profile.PersonaModules[0].TraitContributions["structure_level"] != 0.95 {
		t.Fatalf("expected wizard override trait contribution, got %+v", wizard.Profile.PersonaModules[0].TraitContributions)
	}
}

func TestExperienceManagerBuildReturnsExperienceControl(t *testing.T) {
	t.Parallel()

	engine := NewExperienceManager("", ExperienceModeStandard)
	rendered, err := engine.Build(context.Background(), ExperienceModeStandard, experience.BuildRequest{
		ChatID:          "sess-1",
		Mode:            "navi",
		LastUserMessage: "Please be brief and use bullets.",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !strings.Contains(rendered.Fragment, "## Experience Control") {
		t.Fatalf("expected experience control fragment, got %q", rendered.Fragment)
	}
	if !strings.Contains(rendered.Fragment, "preferred_length short") {
		t.Fatalf("expected brief turn override in fragment, got %q", rendered.Fragment)
	}
	if rendered.Payload.SourceStateID == "" || !strings.HasPrefix(rendered.Payload.PayloadID, "cpp-") {
		t.Fatalf("expected compiled payload identifiers, got %+v", rendered.Payload)
	}
}
