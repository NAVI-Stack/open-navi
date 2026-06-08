package llm

import (
	"errors"
	"testing"

	"github.com/open-navi/navi/internal/schema"
)

func cloudProfile(provider, model string) ModelProfile {
	return ModelProfile{
		ProviderKey: provider, ModelID: model, DisplayName: provider + " " + model,
		SupportsTools: true, ToolCallReliable: true, ChatScore: 80, ReasoningScore: 85,
		AgenticScore: 80, CodingScore: 80, Tags: []string{"cloud"},
	}
}

func localProfile(model string) ModelProfile {
	return ModelProfile{
		ProviderKey: "ollama", ModelID: model, DisplayName: "ollama " + model,
		SupportsTools: false, ToolCallReliable: false, ChatScore: 70, ReasoningScore: 55,
		AgenticScore: 40, CodingScore: 50, Tags: []string{"local", "free"},
	}
}

func toolCapableLocalProfile(model string) ModelProfile {
	p := localProfile(model)
	p.SupportsTools = true
	p.ToolCallReliable = true
	return p
}

func TestCloudEligible_PolicyTable(t *testing.T) {
	cases := []struct {
		mode  PrivacyMode
		class schema.PrivacyClass
		want  bool
	}{
		{PrivacyModeLocal, schema.PrivacyClassPublic, false},
		{PrivacyModeLocal, schema.PrivacyClassSecret, false},
		{PrivacyModeHybrid, schema.PrivacyClassPublic, true},
		{PrivacyModeHybrid, schema.PrivacyClassPersonal, true},
		{PrivacyModeHybrid, schema.PrivacyClassSensitive, false},
		{PrivacyModeHybrid, schema.PrivacyClassSecret, false},
		{PrivacyModeCloud, schema.PrivacyClassSecret, true},
		{"", schema.PrivacyClassSecret, true}, // empty = permissive default
	}
	for _, c := range cases {
		if got := CloudEligible(c.mode, c.class); got != c.want {
			t.Errorf("CloudEligible(%q,%q) = %v, want %v", c.mode, c.class, got, c.want)
		}
	}
}

func TestSelectWithPrivacy_SecretInLocalRefusesCloud(t *testing.T) {
	// Only cloud models available; Local mode bars all cloud routes for secret.
	sel := ModelSelector{
		PrivacyMode: PrivacyModeLocal,
		Profiles:    []ModelProfile{cloudProfile("anthropic", "claude-opus"), cloudProfile("openai", "gpt-4o")},
	}
	_, err := sel.SelectWithPrivacy(TaskClassification{Task: TaskClassChat}, false, schema.PrivacyClassSecret)
	var refused *RouteRefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("expected *RouteRefusedError, got %v", err)
	}
	if refused.PrivacyClass != schema.PrivacyClassSecret || refused.Mode != PrivacyModeLocal {
		t.Errorf("refusal lost context: %+v", refused)
	}
}

func TestSelectWithPrivacy_SecretInLocalFallsBackToLocal(t *testing.T) {
	sel := ModelSelector{
		PrivacyMode: PrivacyModeLocal,
		Profiles:    []ModelProfile{cloudProfile("anthropic", "claude-opus"), localProfile("llama3.2")},
	}
	route, err := sel.SelectWithPrivacy(TaskClassification{Task: TaskClassChat}, false, schema.PrivacyClassSecret)
	if err != nil {
		t.Fatalf("expected local fallback, got error: %v", err)
	}
	if route.Provider != "ollama" {
		t.Errorf("secret-in-Local routed to %q, want a local provider (no cloud)", route.Provider)
	}
	if route.Reason != "privacy_local_only" {
		t.Errorf("fallback reason = %q, want privacy_local_only", route.Reason)
	}
}

func TestSelectWithPrivacy_SecretInLocalNeedsToolsButOnlyCloudToolModel(t *testing.T) {
	// Local model exists but cannot call tools; tools required → observable refusal,
	// never a silent cloud route.
	sel := ModelSelector{
		PrivacyMode: PrivacyModeLocal,
		Profiles:    []ModelProfile{cloudProfile("anthropic", "claude-opus"), localProfile("llama3.2")},
	}
	_, err := sel.SelectWithPrivacy(TaskClassification{Task: TaskClassAgentic}, true, schema.PrivacyClassSecret)
	var refused *RouteRefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("expected *RouteRefusedError when no tool-capable local model exists, got %v", err)
	}
}

func TestSelectWithPrivacy_PublicAllowsCloud(t *testing.T) {
	sel := ModelSelector{
		PrivacyMode: PrivacyModeHybrid,
		Profiles:    []ModelProfile{cloudProfile("anthropic", "claude-opus"), localProfile("llama3.2")},
	}
	route, err := sel.SelectWithPrivacy(TaskClassification{Task: TaskClassReasoning, Complexity: ComplexityHigh}, false, schema.PrivacyClassPublic)
	if err != nil {
		t.Fatalf("public class should route fine: %v", err)
	}
	if route.Provider != "anthropic" {
		t.Errorf("public/Hybrid should be free to choose the cloud model, got %q", route.Provider)
	}
}

func TestSelectWithPrivacy_ToolCapableLocalServesSecret(t *testing.T) {
	sel := ModelSelector{
		PrivacyMode: PrivacyModeLocal,
		Profiles:    []ModelProfile{cloudProfile("anthropic", "claude-opus"), toolCapableLocalProfile("qwen2.5")},
	}
	route, err := sel.SelectWithPrivacy(TaskClassification{Task: TaskClassAgentic}, true, schema.PrivacyClassSecret)
	if err != nil {
		t.Fatalf("tool-capable local model should serve secret-in-Local: %v", err)
	}
	if route.Provider != "ollama" {
		t.Errorf("routed to %q, want ollama", route.Provider)
	}
}

func TestBuildPrivacyPolicyView(t *testing.T) {
	view := BuildPrivacyPolicyView(PrivacyModeHybrid)
	if view.Mode != string(PrivacyModeHybrid) {
		t.Errorf("mode = %q, want hybrid", view.Mode)
	}
	got := map[string]bool{}
	for _, c := range view.Classes {
		got[c.Class] = c.CloudEligible
	}
	if !got[string(schema.PrivacyClassPublic)] || got[string(schema.PrivacyClassSecret)] {
		t.Errorf("hybrid policy wrong: %+v", got)
	}

	// Empty mode resolves to the permissive Cloud view.
	def := BuildPrivacyPolicyView("")
	if def.Mode != string(PrivacyModeCloud) {
		t.Errorf("empty mode resolved to %q, want cloud", def.Mode)
	}
}

func TestPreferencePatch_PrivacyMode(t *testing.T) {
	mode := PrivacyModeLocal
	out := ModelPreferences{}.Apply(PreferencePatch{PrivacyMode: &mode})
	if out.PrivacyMode != PrivacyModeLocal {
		t.Errorf("PrivacyMode patch not applied: %q", out.PrivacyMode)
	}
}
