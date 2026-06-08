package inference

import (
	"testing"

	"github.com/open-navi/navi/internal/schema"
)

// If recovery requires approval, retry must never be selected even for timeout.
func TestRecoveryManagerTimeoutWithApprovalRoutesPropose(t *testing.T) {
	manager := DefaultRecoveryManager{}

	checkpoint := manager.Build(
		InferenceInput{
			Recovery: RecoveryState{
				Status:       schema.RecoveryStatusOpen,
				FailureClass: schema.FailureClassTimeout,
			},
			Governance: GovernanceState{
				PendingProposalID:    "proposal-1",
				ConfirmationRequired: true,
			},
		},
		FocusFrame{ActiveGoalID: "goal-1"},
		CandidateSummary{CandidateType: CandidateTypeRecover},
		[]ApprovalRef{{Requirement: ApprovalRequirementBlockingProposal, ProposalID: "proposal-1"}},
	)

	if checkpoint.Route.Action != RecoveryRoutePropose {
		t.Fatalf("expected propose route when approval required, got %#v", checkpoint.Route)
	}
}

// Irreversible partial work must not be retried or compensated automatically.
func TestRecoveryManagerIrreversiblePartialExecutionDoesNotRetryOrCompensate(t *testing.T) {
	manager := DefaultRecoveryManager{}

	checkpoint := manager.Build(
		InferenceInput{
			Recovery: RecoveryState{
				Status:       schema.RecoveryStatusOpen,
				FailureClass: schema.FailureClassPartialExecution,
			},
			Capabilities: []CapabilityAvailability{{
				Name:          "irreversible_action",
				Available:     true,
				Reversibility: schema.ReversibilityIrreversible,
			}},
		},
		FocusFrame{ActiveGoalID: "goal-irreversible"},
		CandidateSummary{CandidateType: CandidateTypeRecover},
		nil,
	)

	if checkpoint.Route.Action == RecoveryRouteRetry || checkpoint.Route.Action == RecoveryRouteCompensate {
		t.Fatalf("irreversible partial execution must not retry or compensate, got %#v", checkpoint.Route)
	}
}
