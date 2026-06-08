package inference

import (
	"testing"

	"github.com/ceoai/navi/internal/governor"
	"github.com/ceoai/navi/internal/navi/orchestration"
	"github.com/ceoai/navi/internal/schema"
)

func TestCandidateEvaluatorRanksExecuteAboveFallbacks(t *testing.T) {
	evaluator := DefaultCandidateEvaluator{}

	result := evaluator.Evaluate(
		InferenceInput{
			Chat: ChatContext{
				ChatID:   "sess-1",
				UserMessage: "make the change",
			},
			NCOS: orchestration.CanonicalRunRequest{
				RequiredOutput: orchestration.RequiredOutput{AllowToolCalls: true},
			},
			Capabilities: []CapabilityAvailability{
				{
					Name:          "write_file",
					Kind:          "coding",
					Available:     true,
					Governed:      true,
					CommandType:   schema.CommandTypeInvoke,
					RiskHint:      schema.RiskLow,
					Reversibility: schema.ReversibilityInternal,
				},
			},
		},
		FocusFrame{
			FocusID:       "goal-1",
			ActiveGoalID:  "goal-1",
			FocusReason:   FocusReasonUserRequest,
			PriorityScore: 0.9,
		},
		ModeSelection{DominantMode: DecisionModeExecute, SubmodeChain: []DecisionMode{DecisionModePlan}},
	)

	if result.ChosenCandidate.CandidateType != CandidateTypeExecute {
		t.Fatalf("expected execute candidate to win, got %#v", result.ChosenCandidate)
	}
	if len(result.CandidateSummaries) < 2 {
		t.Fatalf("expected explicit candidate alternatives, got %#v", result.CandidateSummaries)
	}
	if result.Thresholds.ScoreBand != ThresholdBandPreferred {
		t.Fatalf("expected preferred threshold band, got %q", result.Thresholds.ScoreBand)
	}
	if result.ChosenCandidate.TargetCapability != "write_file" {
		t.Fatalf("expected execute candidate to carry exact target capability, got %#v", result.ChosenCandidate)
	}
}

func TestCandidateEvaluatorRepresentsHardVetoAndStructuredRejection(t *testing.T) {
	evaluator := DefaultCandidateEvaluator{}

	result := evaluator.Evaluate(
		InferenceInput{
			Chat: ChatContext{
				ChatID:   "sess-2",
				UserMessage: "apply the change",
			},
			Governance: GovernanceState{
				HardBlocked:    true,
				BlockingReason: "policy blocked",
				LastValidation: &governor.ValidationResult{
					Outcome: governor.ValidationRejected,
					Reason:  "policy blocked",
				},
			},
			NCOS: orchestration.CanonicalRunRequest{
				RequiredOutput: orchestration.RequiredOutput{AllowToolCalls: true},
			},
			Capabilities: []CapabilityAvailability{
				{Name: "write_file", Kind: "coding", Available: true, Governed: true, CommandType: schema.CommandTypeInvoke},
			},
		},
		FocusFrame{
			FocusID:       "goal-2",
			ActiveGoalID:  "goal-2",
			FocusReason:   FocusReasonUserRequest,
			PriorityScore: 0.8,
		},
		ModeSelection{DominantMode: DecisionModeExecute},
	)

	if result.Thresholds.ScoreBand != ThresholdBandBlocked {
		t.Fatalf("expected blocked threshold band, got %q", result.Thresholds.ScoreBand)
	}
	if len(result.Thresholds.Vetoes) == 0 || !result.Thresholds.Vetoes[0].Hard {
		t.Fatalf("expected hard veto state, got %#v", result.Thresholds.Vetoes)
	}
	if result.Rejection == nil || result.Rejection.Reason != RejectionReasonGovernanceBlocked {
		t.Fatalf("expected structured governance-blocked rejection, got %#v", result.Rejection)
	}
}

func TestGovernanceHandoffActionDescriptorFeedsGovernor(t *testing.T) {
	handoff := GovernanceHandoff{
		CommandType: schema.CommandTypeUpdate,
		Domain:      governor.AutonomyDomainCoding,
		ActorKind:   "navi",
		Tags: map[string]string{
			"candidate_type": "execute",
		},
	}

	result := governor.ValidateAction(nil, handoff.ActionDescriptor(ChatContext{ChatID: "sess-3"}))
	if result.Outcome != governor.ValidationRequiresConfirmation {
		t.Fatalf("expected coding update to require confirmation, got %#v", result)
	}
}

func TestCandidateEvaluatorBuildsBoundedSubsetWhenExactTargetIsNotJustified(t *testing.T) {
	evaluator := DefaultCandidateEvaluator{}

	result := evaluator.Evaluate(
		InferenceInput{
			Chat: ChatContext{
				ChatID:   "sess-4",
				UserMessage: "use the coding tools",
			},
			NCOS: orchestration.CanonicalRunRequest{
				RequiredOutput: orchestration.RequiredOutput{AllowToolCalls: true},
			},
			Capabilities: []CapabilityAvailability{
				{Name: "code_write", Kind: "coding", Available: true, Governed: true, CommandType: schema.CommandTypeInvoke, Reversibility: schema.ReversibilityInternal},
				{Name: "code_patch", Kind: "coding", Available: true, Governed: true, CommandType: schema.CommandTypeInvoke, Reversibility: schema.ReversibilityInternal},
				{Name: "calendar_lookup", Kind: "calendar", Available: true, Governed: true, CommandType: schema.CommandTypeQuery, Reversibility: schema.ReversibilityInternal},
			},
		},
		FocusFrame{
			FocusID:       "goal-4",
			ActiveGoalID:  "goal-4",
			FocusReason:   FocusReasonUserRequest,
			PriorityScore: 0.9,
		},
		ModeSelection{DominantMode: DecisionModeExecute},
	)

	if result.ChosenCandidate.CandidateType != CandidateTypeExecute {
		t.Fatalf("expected execute candidate to win, got %#v", result.ChosenCandidate)
	}
	if result.ChosenCandidate.TargetCapability != "" {
		t.Fatalf("expected bounded subset rather than exact target, got %#v", result.ChosenCandidate)
	}
	if got := result.ChosenCandidate.AllowedCapabilities; len(got) != 2 {
		t.Fatalf("expected bounded subset of coding capabilities, got %#v", got)
	}
	if containsString(result.ChosenCandidate.AllowedCapabilities, "calendar_lookup") {
		t.Fatalf("expected non-coding capability to be excluded from bounded subset, got %#v", result.ChosenCandidate.AllowedCapabilities)
	}
}

func TestCandidateEvaluatorAllowsExplicitTwoCapabilitySetWhenBothAreViable(t *testing.T) {
	evaluator := DefaultCandidateEvaluator{}

	result := evaluator.Evaluate(
		InferenceInput{
			Chat: ChatContext{
				ChatID:   "sess-5",
				UserMessage: "write the file and patch the code",
			},
			NCOS: orchestration.CanonicalRunRequest{
				RequiredOutput: orchestration.RequiredOutput{AllowToolCalls: true},
			},
			Capabilities: []CapabilityAvailability{
				{Name: "code_write", Kind: "coding", Available: true, Governed: true, CommandType: schema.CommandTypeInvoke, Reversibility: schema.ReversibilityInternal},
				{Name: "code_patch", Kind: "coding", Available: true, Governed: true, CommandType: schema.CommandTypeInvoke, Reversibility: schema.ReversibilityInternal},
			},
		},
		FocusFrame{
			FocusID:       "goal-5",
			ActiveGoalID:  "goal-5",
			FocusReason:   FocusReasonUserRequest,
			PriorityScore: 0.9,
		},
		ModeSelection{DominantMode: DecisionModeExecute},
	)

	if result.ChosenCandidate.CandidateType != CandidateTypeExecute {
		t.Fatalf("expected execute candidate to win, got %#v", result.ChosenCandidate)
	}
	if result.ChosenCandidate.TargetCapability != "" {
		t.Fatalf("expected explicit bounded two-capability set without forced exact target, got %#v", result.ChosenCandidate)
	}
	if got := result.ChosenCandidate.AllowedCapabilities; len(got) != 2 || !containsString(got, "code_write") || !containsString(got, "code_patch") {
		t.Fatalf("expected explicit two-capability authorization set [code_write code_patch], got %#v", got)
	}
}

func TestCandidateEvaluatorExcludesUngovernedCapabilitiesFromExecutionPlan(t *testing.T) {
	evaluator := DefaultCandidateEvaluator{}

	result := evaluator.Evaluate(
		InferenceInput{
			Chat: ChatContext{
				ChatID:   "sess-6",
				UserMessage: "make the change",
			},
			NCOS: orchestration.CanonicalRunRequest{
				RequiredOutput: orchestration.RequiredOutput{AllowToolCalls: true},
			},
			Capabilities: []CapabilityAvailability{
				{Name: "write_file", Kind: "coding", Available: true, Governed: true, CommandType: schema.CommandTypeInvoke, Reversibility: schema.ReversibilityInternal},
				{Name: "shell_exec", Kind: "coding", Available: true, Governed: false, CommandType: schema.CommandTypeInvoke, Reversibility: schema.ReversibilityInternal},
			},
		},
		FocusFrame{
			FocusID:       "goal-6",
			ActiveGoalID:  "goal-6",
			FocusReason:   FocusReasonUserRequest,
			PriorityScore: 0.9,
		},
		ModeSelection{DominantMode: DecisionModeExecute},
	)

	if result.ChosenCandidate.TargetCapability != "write_file" {
		t.Fatalf("expected governed capability to remain the execution target, got %#v", result.ChosenCandidate)
	}
	if containsString(result.ChosenCandidate.AllowedCapabilities, "shell_exec") {
		t.Fatalf("expected ungoverned capability to be excluded from execution boundary, got %#v", result.ChosenCandidate.AllowedCapabilities)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
