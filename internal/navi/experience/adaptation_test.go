package experience

import (
	"context"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

func TestDetectPreferenceSignals_CapturesInferredEvidenceClasses(t *testing.T) {
	repeated := DetectPreferenceSignals("Please be brief again.", "owner-1", "sess-1")
	if !hasSignal(repeated, "verbosity", schema.PreferenceEvidenceRepeatedBehavior) {
		t.Fatalf("expected repeated behavior signal, got %+v", repeated)
	}

	anomaly := DetectPreferenceSignals("That was too long.", "owner-1", "sess-1")
	if !hasSignal(anomaly, "verbosity", schema.PreferenceEvidenceAnomaly) {
		t.Fatalf("expected anomaly signal, got %+v", anomaly)
	}

	silent := DetectPreferenceSignals("That concise format works.", "owner-1", "sess-1")
	if !hasSignal(silent, "verbosity", schema.PreferenceEvidenceSilentWin) {
		t.Fatalf("expected silent-win signal, got %+v", silent)
	}
}

func TestBuildSessionPreferenceOverrides_UsesLatestImmediateSignals(t *testing.T) {
	now := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)
	input, trace, prefs := BuildSessionPreferenceOverrides([]schema.PreferenceSignal{
		{
			SignalID:       "signal-1",
			Trait:          "verbosity",
			TargetValue:    0.22,
			EvidenceClass:  schema.PreferenceEvidenceExplicitCorrection,
			SignalStrength: 1.0,
			Immediate:      true,
			CreatedAt:      now,
		},
		{
			SignalID:       "signal-2",
			Trait:          "verbosity",
			TargetValue:    0.78,
			EvidenceClass:  schema.PreferenceEvidenceExplicitCorrection,
			SignalStrength: 1.0,
			Immediate:      true,
			CreatedAt:      now.Add(time.Minute),
		},
	})
	if got := input.TraitOverrides["verbosity"]; got != 0.78 {
		t.Fatalf("expected latest session override to win, got %.2f", got)
	}
	if prefs.PreferredLength != LengthLong {
		t.Fatalf("expected long preference from latest explicit signal, got %q", prefs.PreferredLength)
	}
	if trace.MustBeBrief {
		t.Fatalf("did not expect stale brief trace to survive later override: %+v", trace)
	}
}

func TestEngineBuild_AppliesSessionPreferenceOverrides(t *testing.T) {
	now := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)
	overrides, trace, prefs := BuildSessionPreferenceOverrides([]schema.PreferenceSignal{{
		SignalID:       "signal-1",
		Trait:          "verbosity",
		TargetValue:    0.22,
		EvidenceClass:  schema.PreferenceEvidenceExplicitCorrection,
		SignalStrength: 1.0,
		Immediate:      true,
		CreatedAt:      now,
	}})
	engine := NewEngine(nil)
	rendered, err := engine.Build(context.Background(), DefaultStandardProfile(), BuildRequest{
		ChatID:                "sess-1",
		Mode:                     "navi",
		LastUserMessage:          "Thanks.",
		ExplicitSessionOverrides: overrides,
		SessionOverrideTrace:     trace,
		SessionOutputPreferences: prefs,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := rendered.Payload.OutputContract.PreferredLength; got != LengthShort {
		t.Fatalf("expected session override preferred length short, got %q", got)
	}
	if got := rendered.State.ResolvedTraits["verbosity"].Value; got != 0.22 {
		t.Fatalf("expected session override verbosity 0.22, got %.2f", got)
	}
}

func TestUpdateRelationshipProfile_DoesNotMutateStableTraits(t *testing.T) {
	now := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)
	signals := make([]schema.PreferenceSignal, 0, 5)
	for i := 0; i < 5; i++ {
		signals = append(signals, schema.PreferenceSignal{
			Trait:          "directness",
			TargetValue:    0.20,
			EvidenceClass:  schema.PreferenceEvidenceExplicitCorrection,
			SignalStrength: 1.0,
			CreatedAt:      now,
		})
	}
	profile, deltas := UpdateRelationshipProfile(RelationshipProfile{}, signals, now)
	if _, ok := profile.TraitEstimates["directness"]; ok {
		t.Fatalf("expected stable trait estimate to remain unset, got %+v", profile.TraitEstimates)
	}
	if len(deltas) != 1 {
		t.Fatalf("expected one stable-trait delta, got %+v", deltas)
	}
	if !deltas[0].ProposalWorthy || !deltas[0].Persisted {
		t.Fatalf("expected stable-trait delta to escalate after five signals, got %+v", deltas[0])
	}
}

func TestUpdateRelationshipProfile_DecaysUnreinforcedAdaptiveTraits(t *testing.T) {
	now := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)
	profile, deltas := UpdateRelationshipProfile(RelationshipProfile{
		TraitEstimates:       map[string]float64{"verbosity": 0.10},
		Confidence:           0.50,
		PerTraitConfidence:   map[string]float64{"verbosity": 0.80},
		TotalSignalCount:     5,
		ExplicitSignalCount:  5,
		TraitSignalCount:     map[string]int{"verbosity": 5},
		TraitAverageStrength: map[string]float64{"verbosity": 1.0},
		TraitLastSignalAt:    map[string]string{"verbosity": now.Add(-30 * 24 * time.Hour).Format(time.RFC3339)},
	}, nil, now)
	if len(deltas) != 0 {
		t.Fatalf("expected decay-only update to return no new-signal deltas, got %+v", deltas)
	}
	got := profile.TraitEstimates["verbosity"]
	if got <= 0.10 || got >= traitDefinitions["verbosity"].DefaultValue {
		t.Fatalf("expected verbosity to decay toward default, got %.2f", got)
	}
}

func hasSignal(signals []schema.PreferenceSignal, trait string, class schema.PreferenceEvidenceClass) bool {
	for _, signal := range signals {
		if signal.Trait == trait && signal.EvidenceClass == class {
			return true
		}
	}
	return false
}
