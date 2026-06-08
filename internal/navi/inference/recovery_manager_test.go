package inference

import (
	"testing"

	"github.com/ceoai/navi/internal/schema"
)

func TestRecoveryManagerRoutesRetryForTimeout(t *testing.T) {
	manager := DefaultRecoveryManager{}

	checkpoint := manager.Build(
		InferenceInput{
			Recovery: RecoveryState{
				Status:       schema.RecoveryStatusOpen,
				FailureClass: schema.FailureClassTimeout,
			},
		},
		FocusFrame{ActiveGoalID: "goal-timeout"},
		CandidateSummary{CandidateID: "candidate:recover", CandidateType: CandidateTypeRecover},
		nil,
	)

	if checkpoint.Route.Action != RecoveryRouteRetry {
		t.Fatalf("expected retry route, got %#v", checkpoint.Route)
	}
	if !checkpoint.Open {
		t.Fatalf("expected retry route to keep recovery open, got %#v", checkpoint)
	}
}

func TestRecoveryManagerRoutesCompensateForPartialCompensableFailure(t *testing.T) {
	manager := DefaultRecoveryManager{}

	checkpoint := manager.Build(
		InferenceInput{
			Recovery: RecoveryState{
				Status:       schema.RecoveryStatusOpen,
				FailureClass: schema.FailureClassPartialExecution,
			},
			Capabilities: []CapabilityAvailability{{
				Name:          "external_write",
				Available:     true,
				Reversibility: schema.ReversibilityCompensable,
			}},
		},
		FocusFrame{ActiveGoalID: "goal-partial"},
		CandidateSummary{CandidateID: "candidate:recover", CandidateType: CandidateTypeRecover},
		nil,
	)

	if checkpoint.Route.Action != RecoveryRouteCompensate {
		t.Fatalf("expected compensate route, got %#v", checkpoint.Route)
	}
}

func TestRecoveryManagerRoutesCompensateForUngovernedObservedCapability(t *testing.T) {
	manager := DefaultRecoveryManager{}

	checkpoint := manager.Build(
		InferenceInput{
			Recovery: RecoveryState{
				Status:       schema.RecoveryStatusOpen,
				FailureClass: schema.FailureClassPartialExecution,
			},
			Capabilities: []CapabilityAvailability{{
				Name:          "external_write",
				Available:     true,
				Governed:      false,
				Reversibility: schema.ReversibilityCompensable,
				CommandType:   schema.CommandTypeInvoke,
			}},
		},
		FocusFrame{ActiveGoalID: "goal-partial-ungoverned"},
		CandidateSummary{CandidateID: "candidate:recover", CandidateType: CandidateTypeRecover},
		nil,
	)

	if checkpoint.Route.Action != RecoveryRouteCompensate {
		t.Fatalf("expected compensate route for observed compensable side effects, got %#v", checkpoint.Route)
	}
}

func TestRecoveryManagerRoutesProposalAwareRecovery(t *testing.T) {
	manager := DefaultRecoveryManager{}

	checkpoint := manager.Build(
		InferenceInput{
			Recovery: RecoveryState{
				Status:       schema.RecoveryStatusOpen,
				FailureClass: schema.FailureClassExecutionFailure,
			},
			Governance: GovernanceState{
				PendingProposalID:    "proposal-recovery",
				ConfirmationRequired: true,
			},
		},
		FocusFrame{ActiveGoalID: "goal-proposal"},
		CandidateSummary{CandidateID: "candidate:recover", CandidateType: CandidateTypeRecover},
		[]ApprovalRef{{Requirement: ApprovalRequirementBlockingProposal, ProposalID: "proposal-recovery"}},
	)

	if checkpoint.Route.Action != RecoveryRoutePropose {
		t.Fatalf("expected propose route, got %#v", checkpoint.Route)
	}
	if checkpoint.Route.ProposalID != "proposal-recovery" {
		t.Fatalf("expected proposal id reuse, got %#v", checkpoint.Route)
	}
}

func TestRecoveryManagerRoutesReplanAfterDeniedApproval(t *testing.T) {
	manager := DefaultRecoveryManager{}

	checkpoint := manager.Build(
		InferenceInput{
			Recovery: RecoveryState{
				Status:       schema.RecoveryStatusOpen,
				FailureClass: schema.FailureClassPolicyBlocked,
			},
			Governance: GovernanceState{
				LastApprovalOutcome: schema.ApprovalOutcomeDenied,
			},
		},
		FocusFrame{ActiveGoalID: "goal-replan"},
		CandidateSummary{CandidateID: "candidate:replan", CandidateType: CandidateTypeReplan},
		nil,
	)

	if checkpoint.Route.Action != RecoveryRouteReplan {
		t.Fatalf("expected replan route, got %#v", checkpoint.Route)
	}
}

func TestRecoveryManagerRoutesDeferWhenRecoveryIsStillBlocked(t *testing.T) {
	manager := DefaultRecoveryManager{}

	checkpoint := manager.Build(
		InferenceInput{
			Recovery: RecoveryState{
				Status:          schema.RecoveryStatusOpen,
				FailureClass:    schema.FailureClassExecutionFailure,
				PendingBlockers: []string{"await operator guidance"},
			},
		},
		FocusFrame{ActiveGoalID: "goal-defer"},
		CandidateSummary{CandidateID: "candidate:defer", CandidateType: CandidateTypeDefer},
		nil,
	)

	if checkpoint.Route.Action != RecoveryRouteDefer {
		t.Fatalf("expected defer route, got %#v", checkpoint.Route)
	}
}
