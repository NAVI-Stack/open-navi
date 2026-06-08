package experience

import (
	"context"
	"strings"
	"testing"
)

func TestEngineBuildProducesCompiledControl(t *testing.T) {
	t.Parallel()

	engine := NewEngine(nil)
	rendered, err := engine.Build(context.Background(), DefaultStandardProfile(), BuildRequest{
		ChatID:       "sess-1",
		Mode:            "navi",
		LastUserMessage: "Be brief and step by step.",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if rendered.State.StateID == "" || rendered.Payload.PayloadID == "" {
		t.Fatalf("expected stable ids, got state=%q payload=%q", rendered.State.StateID, rendered.Payload.PayloadID)
	}
	if !strings.Contains(rendered.Fragment, "## Experience Control") {
		t.Fatalf("expected serialized fragment, got %q", rendered.Fragment)
	}
	if rendered.Payload.OutputContract.PreferredLength != LengthShort {
		t.Fatalf("expected short output preference, got %q", rendered.Payload.OutputContract.PreferredLength)
	}
	if rendered.Payload.OutputContract.ResponseShape != ResponseStepwise {
		t.Fatalf("expected stepwise response shape, got %q", rendered.Payload.OutputContract.ResponseShape)
	}
}

func TestEngineBuildAppliesHighStakesHumorGuard(t *testing.T) {
	t.Parallel()

	engine := NewEngine(nil)
	rendered, err := engine.Build(context.Background(), DefaultStandardProfile(), BuildRequest{
		ChatID:       "sess-2",
		Mode:            "navi",
		LastUserMessage: "I need advice on a legal contract and financial risk.",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if rendered.Payload.GatesAndClamps.HumorAllowed {
		t.Fatalf("expected humor to be gated in high-stakes context, got %+v", rendered.Payload.GatesAndClamps)
	}
	if rendered.State.ResolvedTraits["humor_playfulness"].Value != 0 {
		t.Fatalf("expected humor trait to clamp to zero, got %+v", rendered.State.ResolvedTraits["humor_playfulness"])
	}
}

func TestEngineBuildRespectsLockedModules(t *testing.T) {
	t.Parallel()

	engine := NewEngine(nil)

	// Case 1: Locked owner module should NOT be overridden by request module.
	profileLocked := DefaultStandardProfile()
	profileLocked.PersonaModules = []ModuleConfig{
		{
			ModuleID:    "tone_constraint",
			OriginScope: "owner",
			IsLocked:    true,
			TraitContributions: map[string]float64{
				"directness": 0.10,
			},
		},
	}

	renderedLocked, err := engine.Build(context.Background(), profileLocked, BuildRequest{
		ChatID: "sess-locked",
		PersonaModules: []ModuleConfig{
			{
				ModuleID: "tone_constraint",
				TraitContributions: map[string]float64{
					"directness": 0.90,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Build (locked): %v", err)
	}

	// In resolvePriorityTrait (which directness uses via DomainExpression -> MergeWeightedBlend, wait)
	// Actually directness is MergeWeightedBlend, so it will be blended.
	// But normalizeModules dedupes modules by ID.
	// We want to check if the module in PersonaModules is the one from owner or request.

	foundLocked := false
	for _, m := range renderedLocked.Input.PersonaModules {
		if m.ModuleID == "tone_constraint" {
			if m.TraitContributions["directness"] == 0.10 {
				foundLocked = true
			}
		}
	}
	if !foundLocked {
		t.Errorf("expected locked owner module to be preserved, but it was overridden")
	}

	// Case 2: Unlocked owner module SHOULD be overridden by request module.
	profileUnlocked := DefaultStandardProfile()
	profileUnlocked.PersonaModules = []ModuleConfig{
		{
			ModuleID:    "tone_constraint",
			OriginScope: "owner",
			IsLocked:    false,
			TraitContributions: map[string]float64{
				"directness": 0.10,
			},
		},
	}

	renderedUnlocked, err := engine.Build(context.Background(), profileUnlocked, BuildRequest{
		ChatID: "sess-unlocked",
		PersonaModules: []ModuleConfig{
			{
				ModuleID: "tone_constraint",
				TraitContributions: map[string]float64{
					"directness": 0.90,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Build (unlocked): %v", err)
	}

	foundOverridden := false
	for _, m := range renderedUnlocked.Input.PersonaModules {
		if m.ModuleID == "tone_constraint" {
			if m.TraitContributions["directness"] == 0.90 {
				foundOverridden = true
			}
		}
	}
	if !foundOverridden {
		t.Errorf("expected unlocked owner module to be overridden, but it was preserved")
	}
}
