package inference

import (
	"testing"

	"github.com/ceoai/navi/internal/navi/orchestration"
	"github.com/ceoai/navi/internal/schema"
)

func TestModeRouterRoutesExplicitClarifyAndBoundsSubmodes(t *testing.T) {
	router := DefaultModeRouter{MaxChainDepth: 1}

	selection := router.Route(
		InferenceInput{
			RelevantContext: []ContextReference{{Kind: "needs_clarification"}},
			GoalStack: GoalStack{
				ActiveGoalID: "goal-1",
				Ready:        []GoalRef{{GoalID: "goal-1", Priority: 0.5}},
			},
		},
		FocusFrame{
			FocusID:      "sess-1",
			ActiveGoalID: "goal-1",
			FocusReason:  FocusReasonUserRequest,
		},
	)

	if selection.DominantMode != DecisionModeClarify {
		t.Fatalf("expected clarify dominant mode, got %q", selection.DominantMode)
	}
	if len(selection.SubmodeChain) != 0 {
		t.Fatalf("expected bounded empty submode chain for clarify, got %#v", selection.SubmodeChain)
	}
	if selection.PlanningStyle != PlanningStyleDirect {
		t.Fatalf("expected direct clarify planning style, got %q", selection.PlanningStyle)
	}
}

func TestModeRouterRoutesExecuteWithShallowPlanning(t *testing.T) {
	router := DefaultModeRouter{}

	selection := router.Route(
		InferenceInput{
			NCOS: orchestration.CanonicalRunRequest{
				RequiredOutput: orchestration.RequiredOutput{AllowToolCalls: true},
			},
			Capabilities: []CapabilityAvailability{{Name: "write_file", Available: true, Governed: true}},
			GoalStack: GoalStack{
				ActiveGoalID: "goal-2",
				Ready:        []GoalRef{{GoalID: "goal-2", Priority: 0.8}},
			},
		},
		FocusFrame{
			FocusID:      "focus-2",
			ActiveGoalID: "goal-2",
			FocusReason:  FocusReasonUserRequest,
		},
	)

	if selection.DominantMode != DecisionModeExecute {
		t.Fatalf("expected execute dominant mode, got %q", selection.DominantMode)
	}
	if selection.PlanningStyle != PlanningStyleShallow || selection.PlanningDepth != 1 {
		t.Fatalf("expected shallow execute planning, got %q depth %d", selection.PlanningStyle, selection.PlanningDepth)
	}
}

func TestModeRouterBuildsSelectiveSubreasonerArbitration(t *testing.T) {
	router := DefaultModeRouter{}

	selection := router.Route(
		InferenceInput{
			Chat: ChatContext{UserMessage: "please think this through"},
			Governance: GovernanceState{
				RiskHint:             schema.RiskHigh,
				ConfirmationRequired: true,
			},
			RelevantContext: []ContextReference{{Kind: "needs_clarification"}},
		},
		FocusFrame{
			FocusID:      "focus-3",
			ActiveGoalID: "goal-3",
			FocusReason:  FocusReasonUserRequest,
		},
	)

	if len(selection.InvokedSubreasoners) == 0 {
		t.Fatal("expected selective subreasoner invocation")
	}
	if !containsSubreasoner(selection.InvokedSubreasoners, SubreasonerInterpreter) {
		t.Fatalf("expected interpreter invocation, got %#v", selection.InvokedSubreasoners)
	}
	if !containsSubreasoner(selection.InvokedSubreasoners, SubreasonerClarifier) {
		t.Fatalf("expected clarifier invocation, got %#v", selection.InvokedSubreasoners)
	}
	if !containsSubreasoner(selection.InvokedSubreasoners, SubreasonerRiskAssessor) {
		t.Fatalf("expected risk assessor invocation, got %#v", selection.InvokedSubreasoners)
	}
	if len(selection.Arbitration.ActivePins) == 0 || len(selection.Arbitration.Notes) == 0 {
		t.Fatalf("expected explicit arbitration state, got %#v", selection.Arbitration)
	}
}

func TestModeRouterDecaysPinsWhenTriggersDisappear(t *testing.T) {
	router := DefaultModeRouter{}

	selection := router.Route(
		InferenceInput{
			Posture: PostureState{
				Pins: []SubreasonerPin{{
					Reasoner:       SubreasonerRiskAssessor,
					Source:         "legacy",
					Reason:         "prior risk pressure",
					DecayRemaining: 1,
				}},
			},
		},
		FocusFrame{
			FocusID:     "focus-4",
			FocusReason: FocusReasonIdle,
		},
	)

	if containsSubreasoner(selection.ResolvedPosture.PinnedSubreasoners, SubreasonerRiskAssessor) {
		t.Fatalf("expected expired pin to be released, got %#v", selection.ResolvedPosture)
	}
	if len(selection.Arbitration.ReleasedPins) != 1 {
		t.Fatalf("expected released pin to be explicit, got %#v", selection.Arbitration)
	}
}

func TestModeRouterBoundsPostureAdaptation(t *testing.T) {
	router := DefaultModeRouter{}

	selection := router.Route(
		InferenceInput{
			Posture: PostureState{
				InitiativeBias:           0.95,
				ClarificationStrictness:  0.95,
				MonitoringAggressiveness: 0.95,
			},
			Recovery: RecoveryState{Status: schema.RecoveryStatusOpen},
			Governance: GovernanceState{
				RiskHint: schema.RiskCritical,
			},
		},
		FocusFrame{
			FocusID:     "focus-5",
			FocusReason: FocusReasonScheduledTrigger,
		},
	)

	if selection.ResolvedPosture.InitiativeBias < 0 || selection.ResolvedPosture.InitiativeBias > 1 {
		t.Fatalf("expected bounded initiative bias, got %#v", selection.ResolvedPosture)
	}
	if selection.ResolvedPosture.ClarificationStrictness < 0 || selection.ResolvedPosture.ClarificationStrictness > 1 {
		t.Fatalf("expected bounded clarification strictness, got %#v", selection.ResolvedPosture)
	}
	if selection.ResolvedPosture.MonitoringAggressiveness < 0 || selection.ResolvedPosture.MonitoringAggressiveness > 1 {
		t.Fatalf("expected bounded monitoring aggressiveness, got %#v", selection.ResolvedPosture)
	}
}

func TestModeRouterDoesNotIntroduceAlwaysOnSubreasonersByDefault(t *testing.T) {
	router := DefaultModeRouter{}

	selection := router.Route(
		InferenceInput{},
		FocusFrame{
			FocusID:     "focus-6",
			FocusReason: FocusReasonIdle,
		},
	)

	if len(selection.InvokedSubreasoners) != 0 {
		t.Fatalf("expected no always-on default subreasoners, got %#v", selection.InvokedSubreasoners)
	}
	if len(selection.ResolvedPosture.PinnedSubreasoners) != 0 {
		t.Fatalf("expected no default pins, got %#v", selection.ResolvedPosture)
	}
}

func containsSubreasoner(values []Subreasoner, target Subreasoner) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
