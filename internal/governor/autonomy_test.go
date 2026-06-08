package governor

import (
	"testing"

	"github.com/open-navi/navi/internal/navi/skill"
	"github.com/open-navi/navi/internal/schema"
)

type mockAutonomyResolver struct {
	presets map[string]string
}

func (m *mockAutonomyResolver) EffectivePreset(domain string) string {
	if m == nil {
		return ""
	}
	if p, ok := m.presets[domain]; ok {
		return p
	}
	return ""
}

// mockExecutionThresholdResolver implements both AutonomyPresetResolver and ExecutionThresholdResolver.
type mockExecutionThresholdResolver struct {
	preset    string
	threshold string
}

func (m *mockExecutionThresholdResolver) EffectivePreset(domain string) string       { return m.preset }
func (m *mockExecutionThresholdResolver) EffectiveExecutionThreshold(domain string) string { return m.threshold }

func TestApplyAutonomy_UsesExecutionThresholdWhenResolverImplementsIt(t *testing.T) {
	baseResult := ValidationResult{Outcome: ValidationRequiresConfirmation}
	// Preset is "balanced" but threshold is "high" — ApplyAutonomy should use threshold and approve.
	resolver := &mockExecutionThresholdResolver{preset: "balanced", threshold: "high"}
	res := ApplyAutonomy(baseResult, resolver, "coding", nil)
	if res.Outcome != ValidationApproved {
		t.Fatalf("expected Approved when ExecutionThresholdResolver returns high, got %v", res.Outcome)
	}
	// Threshold "low" → unchanged.
	resolver.threshold = "low"
	res = ApplyAutonomy(baseResult, resolver, "coding", nil)
	if res.Outcome != ValidationRequiresConfirmation {
		t.Fatalf("expected RequiresConfirmation when threshold low, got %v", res.Outcome)
	}
}

func TestDomainForCommand_ExplicitDomainWins(t *testing.T) {
	d := DomainForCommand(schema.CommandTypeSend, AutonomyDomainMemory)
	if d != AutonomyDomainMemory {
		t.Fatalf("expected explicit domain %q, got %q", AutonomyDomainMemory, d)
	}
}

func TestDomainForCommand_DefaultMapping(t *testing.T) {
	cases := []struct {
		cmdType schema.CommandType
		want    string
	}{
		{schema.CommandTypeSend, AutonomyDomainMessaging},
		{schema.CommandTypeSchedule, AutonomyDomainScheduling},
		{schema.CommandTypeDelegate, AutonomyDomainDelegation},
		{schema.CommandTypeCompose, AutonomyDomainWorkflow},
		{schema.CommandTypeAcquire, AutonomyDomainPlugins},
		{schema.CommandTypeQuery, AutonomyDomainCoding},
		{schema.CommandTypeCreate, AutonomyDomainCoding},
		{schema.CommandTypeUpdate, AutonomyDomainCoding},
		{schema.CommandTypeDelete, AutonomyDomainCoding},
		{schema.CommandTypeInvoke, AutonomyDomainCoding},
	}

	for _, tc := range cases {
		got := DomainForCommand(tc.cmdType, "")
		if got != tc.want {
			t.Errorf("DomainForCommand(%q) = %q, want %q", tc.cmdType, got, tc.want)
		}
	}
}

func TestApplyAutonomyForAction_RespectsPresetAndHardFloors(t *testing.T) {
	baseResult := ValidationResult{Outcome: ValidationRequiresConfirmation}

	resolver := &mockAutonomyResolver{
		presets: map[string]string{
			AutonomyDomainMessaging: "high",
			AutonomyDomainCoding:    "balanced",
		},
	}

	// High preset domain with no hard floor → auto-approved.
	res := ApplyAutonomyForAction(baseResult, resolver, schema.CommandTypeSend, "", nil)
	if res.Outcome != ValidationApproved {
		t.Fatalf("expected messaging domain to auto-approve, got outcome=%v", res.Outcome)
	}

	// Non-high preset domain → unchanged.
	res = ApplyAutonomyForAction(baseResult, resolver, schema.CommandTypeUpdate, "", nil)
	if res.Outcome != ValidationRequiresConfirmation {
		t.Fatalf("expected coding domain to remain RequiresConfirmation, got outcome=%v", res.Outcome)
	}

	// Hard floor on skill entry → unchanged even with high preset.
	res = ApplyAutonomyForAction(baseResult, resolver, schema.CommandTypeSend, "", &skill.SkillEntry{
		Spec: &skill.OSS27Spec{
			Effects: skill.EffectMetadata{
				RequiresConfirmation: true,
			},
		},
	})
	if res.Outcome != ValidationRequiresConfirmation {
		t.Fatalf("expected hard floor to prevent auto-approval, got outcome=%v", res.Outcome)
	}
}


