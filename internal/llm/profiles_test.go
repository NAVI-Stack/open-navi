package llm

import (
	"context"
	"testing"

	"github.com/ceoai/navi/internal/config"
	"github.com/ceoai/navi/internal/schema"
)

type inMemorySettings struct {
	values map[string]string
}

func (s *inMemorySettings) GetSetting(_ context.Context, key string) (string, bool, error) {
	v, ok := s.values[key]
	return v, ok, nil
}

func (s *inMemorySettings) SetSetting(_ context.Context, key, value string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[key] = value
	return nil
}

func TestClassifyTaskDetectsCodingAndPerTurnOverride(t *testing.T) {
	got := ClassifyTask("Review this Go file and use opus for this", []ToolDefinition{{Name: "read_file"}}, nil)
	if got.Task != TaskClassCoding {
		t.Fatalf("Task = %q, want coding", got.Task)
	}
	if got.Complexity != ComplexityHigh {
		t.Fatalf("Complexity = %q, want high", got.Complexity)
	}
	if got.PreferredModel != "opus" {
		t.Fatalf("PreferredModel = %q, want opus", got.PreferredModel)
	}
}

func TestClassifyTaskDetectsPersistentLocalPreference(t *testing.T) {
	got := ClassifyTask("Always use local models for me", nil, nil)
	if got.PreferencePatch == nil || got.PreferencePatch.PreferLocal == nil || !*got.PreferencePatch.PreferLocal {
		t.Fatalf("expected PreferLocal patch, got %+v", got.PreferencePatch)
	}
}

func TestSeedProfilesIncludesCatalogModels(t *testing.T) {
	catalog := BuildCatalog(&config.LLMConfig{
		Providers: map[string]config.ProviderConfig{
			"anthropic": {DisplayName: "Anthropic", Models: []string{"claude-sonnet-4-20250514"}},
			"ollama":    {DisplayName: "Ollama", Models: []string{"llama3.1:latest"}},
		},
	})
	profiles := SeedProfiles(catalog)
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(profiles))
	}
}

func TestModelSelectorPrefersPerTurnOverride(t *testing.T) {
	selector := ModelSelector{
		Profiles: []ModelProfile{
			seedProfile("anthropic", "Anthropic", "claude-sonnet-4-20250514"),
			seedProfile("anthropic", "Anthropic", "claude-opus-4-20250514"),
		},
	}
	got, ok := selector.Select(TaskClassification{
		Task:           TaskClassReasoning,
		Complexity:     ComplexityHigh,
		PreferredModel: "opus",
	}, false)
	if !ok {
		t.Fatal("expected selector to return a match")
	}
	if got.Model != "claude-opus-4-20250514" {
		t.Fatalf("Model = %q, want opus", got.Model)
	}
}

func TestModelSelectorSkipsNonToolModelsForAgentic(t *testing.T) {
	selector := ModelSelector{
		Profiles: []ModelProfile{
			seedProfile("ollama", "Ollama", "llama3.1:latest"),
			seedProfile("anthropic", "Anthropic", "claude-sonnet-4-20250514"),
		},
	}
	got, ok := selector.Select(TaskClassification{
		Task:       TaskClassAgentic,
		Complexity: ComplexityMedium,
	}, true)
	if !ok {
		t.Fatal("expected selector to return a match")
	}
	if got.Provider != "anthropic" {
		t.Fatalf("Provider = %q, want anthropic", got.Provider)
	}
}

func TestModelSelectorReturnsNoMatchWhenOnlyNonToolModelsExist(t *testing.T) {
	selector := ModelSelector{
		Profiles: []ModelProfile{
			seedProfile("ollama", "Ollama", "phi3:latest"),
		},
	}
	_, ok := selector.Select(TaskClassification{
		Task:       TaskClassAgentic,
		Complexity: ComplexityMedium,
	}, true)
	if ok {
		t.Fatal("expected no match when only non-tool-capable profiles exist")
	}
}

func TestModelPreferencesRoundTrip(t *testing.T) {
	store := &inMemorySettings{}
	ctx := context.Background()
	want := ModelPreferences{
		PreferLocal:   true,
		CostSensitive: true,
		DefaultCoding: "anthropic/claude-sonnet-4-20250514",
	}
	if err := SaveModelPreferences(ctx, store, want); err != nil {
		t.Fatalf("SaveModelPreferences: %v", err)
	}
	got, err := LoadModelPreferences(ctx, store)
	if err != nil {
		t.Fatalf("LoadModelPreferences: %v", err)
	}
	if got.DefaultCoding != want.DefaultCoding || got.PreferLocal != want.PreferLocal || got.CostSensitive != want.CostSensitive {
		t.Fatalf("preferences round trip mismatch: got %+v want %+v", got, want)
	}
}

func TestApplyCalibrationAdjustsScoresFromExecutionOutcomes(t *testing.T) {
	profiles := []ModelProfile{
		seedProfile("anthropic", "Anthropic", "claude-sonnet-4-20250514"),
	}
	outcomes := []schema.ExecutionOutcome{
		{LLMProvider: "anthropic", LLMModel: "claude-sonnet-4-20250514", LLMTaskClass: string(TaskClassAgentic), Outcome: schema.ExecutionOutcomeFailed},
		{LLMProvider: "anthropic", LLMModel: "claude-sonnet-4-20250514", LLMTaskClass: string(TaskClassLightweight), Outcome: schema.ExecutionOutcomeSucceeded},
	}
	got := ApplyCalibration(profiles, outcomes)
	if got[0].LearnedEvidenceCount != 2 {
		t.Fatalf("LearnedEvidenceCount = %d, want 2", got[0].LearnedEvidenceCount)
	}
	if got[0].LearnedAgenticDelta >= 0 {
		t.Fatalf("expected negative agentic delta, got %d", got[0].LearnedAgenticDelta)
	}
	if got[0].LearnedChatDelta <= 0 {
		t.Fatalf("expected positive chat delta, got %d", got[0].LearnedChatDelta)
	}
}
