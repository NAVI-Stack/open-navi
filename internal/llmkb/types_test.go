package llmkb

import (
	"testing"
	"time"
)

func TestEnumParsersAcceptNormalizedValuesAndRejectInvalid(t *testing.T) {
	tests := []struct {
		name    string
		valid   string
		parseOK func(string) error
	}{
		{"field_class", "observed", func(v string) error { _, err := ParseFieldClass(v); return err }},
		{"agentic_class", "capable", func(v string) error { _, err := ParseAgenticClass(v); return err }},
		{"coding_class", "strong", func(v string) error { _, err := ParseCodingClass(v); return err }},
		{"reasoning_class", "exceptional", func(v string) error { _, err := ParseReasoningClass(v); return err }},
		{"class_level", "competent", func(v string) error { _, err := ParseClassLevel(v); return err }},
		{"use_case", "general_chat", func(v string) error { _, err := ParseUseCase(v); return err }},
		{"task_class", "agentic", func(v string) error { _, err := ParseTaskClass(v); return err }},
		{"interaction_mode", "tool_using", func(v string) error { _, err := ParseInteractionMode(v); return err }},
		{"availability_state", "available", func(v string) error { _, err := ParseAvailabilityState(v); return err }},
		{"runtime_backend", "anthropic", func(v string) error { _, err := ParseRuntimeBackend(v); return err }},
		{"connector_status", "connected", func(v string) error { _, err := ParseConnectorStatus(v); return err }},
		{"auth_status", "authenticated", func(v string) error { _, err := ParseAuthStatus(v); return err }},
		{"health_status", "healthy", func(v string) error { _, err := ParseHealthStatus(v); return err }},
		{"warm_state", "warm", func(v string) error { _, err := ParseWarmState(v); return err }},
		{"rate_limit_state", "open", func(v string) error { _, err := ParseRateLimitState(v); return err }},
		{"loaded_status", "loaded", func(v string) error { _, err := ParseLoadedStatus(v); return err }},
		{"release_channel", "stable", func(v string) error { _, err := ParseReleaseChannel(v); return err }},
		{"deprecation_status", "active", func(v string) error { _, err := ParseDeprecationStatus(v); return err }},
		{"hosting_mode", "cloud", func(v string) error { _, err := ParseHostingMode(v); return err }},
		{"reliability_trend", "stable", func(v string) error { _, err := ParseReliabilityTrend(v); return err }},
		{"surfacing_level", "default", func(v string) error { _, err := ParseSurfacingLevel(v); return err }},
		{"proposal_status", "pending", func(v string) error { _, err := ParseProposalStatus(v); return err }},
		{"cost_tier", "cheap", func(v string) error { _, err := ParseCostTier(v); return err }},
		{"latency_tier", "medium", func(v string) error { _, err := ParseLatencyTier(v); return err }},
		{"risk_tier", "high", func(v string) error { _, err := ParseRiskTier(v); return err }},
		{"trust_level", "high", func(v string) error { _, err := ParseTrustLevel(v); return err }},
		{"autonomy_level", "assistive", func(v string) error { _, err := ParseAutonomyLevel(v); return err }},
		{"fit_score", "ideal", func(v string) error { _, err := ParseFitScore(v); return err }},
		{"rejection_code", "over_budget", func(v string) error { _, err := ParseRejectionCode(v); return err }},
		{"failure_pattern", "timeout", func(v string) error { _, err := ParseFailurePattern(v); return err }},
		{"provenance_source", "curated_seed", func(v string) error { _, err := ParseProvenanceSource(v); return err }},
	}
	for _, tc := range tests {
		if err := tc.parseOK("  " + tc.valid + "  "); err != nil {
			t.Fatalf("%s: expected valid parse, got %v", tc.name, err)
		}
		if err := tc.parseOK("__invalid__"); err == nil {
			t.Fatalf("%s: expected invalid parse failure", tc.name)
		}
	}
}

func TestEntityValidation(t *testing.T) {
	now := time.Now().UTC()
	provenance := Provenance{
		Source:       ProvenanceSourceCuratedSeed,
		AssertedBy:   "seed://anthropic",
		SourceDetail: "data/seed/models/anthropic.claude-sonnet.yaml",
		Confidence:   0.9,
		AssertedAt:   now,
		FieldClass:   FieldClassObserved,
	}
	profile := LLMProfile{
		SchemaVersion:     1,
		LLMID:             "anthropic.claude-sonnet",
		CanonicalName:     "anthropic/claude-sonnet",
		ProviderID:        "anthropic",
		CreatedAt:         now,
		Capabilities: CapabilityProfile{
			AgenticClass:              AgenticClassCapable,
			CodingClass:               CodingClassStrong,
			ReasoningClass:            ReasoningClassStrong,
			InstructionFollowingClass: ClassLevelStrong,
			ToolDisciplineClass:       ClassLevelGood,
			SchemaReliabilityClass:    ClassLevelStrong,
			MultimodalClass:           ClassLevelFair,
			PrimaryUseCases:           []UseCase{UseCaseCodingAssistant, UseCaseGeneralChat},
			InteractionModes:          []InteractionMode{InteractionModeConversational, InteractionModeToolUsing},
		},
		OperationalState: OperationalState{
			AvailabilityState: AvailabilityStateAvailable,
			RuntimeBackend:    RuntimeBackendAnthropic,
			HealthStatus:      HealthStatusHealthy,
		},
		Routing: RoutingProfile{
			PreferredFor:       []TaskClass{TaskClassCoding, TaskClassReasoning},
			CostTier:           CostTierExpensive,
			LatencyTier:        LatencyTierMedium,
			MaxRiskTierAllowed: RiskTierMedium,
			AutonomyCeiling:    AutonomyLevelAssistive,
		},
		Provenance: provenance,
	}
	if err := profile.Validate(); err != nil {
		t.Fatalf("expected profile validation to pass, got %v", err)
	}

	decision := RouterDecision{
		DecisionID:     "decision-1",
		TaskClass:      TaskClassCoding,
		SelectedLLMID:  "anthropic.claude-sonnet",
		CandidateSet:   []string{"anthropic.claude-sonnet"},
		Scores:         map[string]float64{"anthropic.claude-sonnet": 0.9},
		RejectedModels: []RejectionRecord{{ReasonCode: RejectionCodeOverBudget, Reason: "too expensive"}},
		CreatedAt:      now,
		Provenance: Provenance{
			Source:     ProvenanceSourceExecution,
			AssertedAt: now,
		},
	}
	if err := decision.Validate(); err != nil {
		t.Fatalf("expected decision validation to pass, got %v", err)
	}
}
