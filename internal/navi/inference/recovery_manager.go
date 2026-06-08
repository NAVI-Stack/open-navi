package inference

import (
	"strings"

	"github.com/ceoai/navi/internal/schema"
)

// DefaultRecoveryManager turns open failure/recovery state into an explicit route.
type DefaultRecoveryManager struct{}

// Build returns the canonical recovery checkpoint and route for the cycle.
func (m DefaultRecoveryManager) Build(input InferenceInput, focus FocusFrame, candidate CandidateSummary, approvals []ApprovalRef) RecoveryCheckpoint {
	checkpoint := RecoveryCheckpoint{
		CurrentGoalID:     focus.ActiveGoalID,
		CurrentPhase:      firstPhase(input.Runtime),
		CurrentTask:       strings.TrimSpace(input.Recovery.CurrentTask),
		CurrentStep:       strings.TrimSpace(input.Recovery.CurrentStep),
		CheckpointRef:     checkpointID(input.Runtime.Checkpoint),
		PendingBlockers:   append([]string(nil), input.Recovery.PendingBlockers...),
		PendingApprovals:  append([]ApprovalRef(nil), approvals...),
		ResumeConditions:  append([]string(nil), input.Recovery.ResumeConditions...),
		RollbackPoint:     firstNonEmpty(strings.TrimSpace(input.Recovery.RollbackPoint), checkpointID(input.Runtime.Checkpoint)),
		ExpiryOrStaleness: input.Recovery.ExpiryOrStaleness,
		RuntimeCheckpoint: input.Runtime.Checkpoint,
	}
	checkpoint.Route = selectRecoveryRoute(input, candidate, approvals)
	checkpoint.Open = recoveryStillOpen(input, checkpoint.Route)
	return checkpoint
}

func selectRecoveryRoute(input InferenceInput, candidate CandidateSummary, approvals []ApprovalRef) RecoveryRoute {
	if len(approvals) > 0 && recoveryRelevant(input, candidate) {
		return RecoveryRoute{
			Action:           RecoveryRoutePropose,
			Reason:           firstNonEmpty(strings.TrimSpace(input.Governance.BlockingReason), "approval required to continue recovery"),
			ProposalID:       strings.TrimSpace(input.Governance.PendingProposalID),
			RequiresApproval: true,
			KeepOpen:         true,
		}
	}

	if input.Governance.LastApprovalOutcome == schema.ApprovalOutcomeDenied {
		return RecoveryRoute{
			Action:   RecoveryRouteReplan,
			Reason:   "replan after recovery approval was denied",
			KeepOpen: true,
		}
	}

	if shouldCompensate(input, candidate) {
		return RecoveryRoute{
			Action:   RecoveryRouteCompensate,
			Reason:   firstNonEmpty(strings.TrimSpace(input.Recovery.FailureReason), "compensation required after partial external effects"),
			KeepOpen: true,
		}
	}

	if shouldRetry(input) {
		return RecoveryRoute{
			Action:   RecoveryRouteRetry,
			Reason:   firstNonEmpty(strings.TrimSpace(input.Recovery.FailureReason), "retryable transient recovery condition"),
			KeepOpen: true,
		}
	}

	if shouldReplan(input) {
		return RecoveryRoute{
			Action:   RecoveryRouteReplan,
			Reason:   firstNonEmpty(strings.TrimSpace(input.Recovery.FailureReason), "constraints changed; replan recovery path"),
			KeepOpen: true,
		}
	}

	if recoveryRelevant(input, candidate) {
		if len(input.Recovery.PendingBlockers) > 0 && candidate.CandidateType == CandidateTypeDefer {
			return RecoveryRoute{
				Action:   RecoveryRouteDefer,
				Reason:   firstNonEmpty(strings.TrimSpace(input.Recovery.FailureReason), "recovery remains blocked"),
				KeepOpen: true,
			}
		}
		return RecoveryRoute{
			Action:   RecoveryRouteRecover,
			Reason:   firstNonEmpty(strings.TrimSpace(input.Recovery.FailureReason), "continue structured recovery"),
			KeepOpen: input.Recovery.Status == schema.RecoveryStatusOpen,
		}
	}

	if candidate.CandidateType == CandidateTypeDefer {
		return RecoveryRoute{
			Action: RecoveryRouteDefer,
			Reason: "defer pending additional recovery input",
		}
	}

	return RecoveryRoute{Action: RecoveryRouteNone}
}

func recoveryStillOpen(input InferenceInput, route RecoveryRoute) bool {
	if route.KeepOpen {
		return true
	}
	return input.Recovery.Status == schema.RecoveryStatusOpen
}

func recoveryRelevant(input InferenceInput, candidate CandidateSummary) bool {
	_ = candidate
	if input.Recovery.Status == schema.RecoveryStatusOpen {
		return true
	}
	switch {
	case input.Recovery.FailureClass != "":
		return true
	case strings.TrimSpace(input.Recovery.FailureReason) != "":
		return true
	case len(input.Recovery.PendingBlockers) > 0:
		return true
	case len(input.Recovery.ResumeConditions) > 0:
		return true
	case strings.TrimSpace(input.Recovery.RollbackPoint) != "":
		return true
	case input.Runtime.Run != nil && input.Runtime.Run.Status == schema.RunStatusWaitingForRecovery:
		return true
	case input.Runtime.Run != nil && strings.TrimSpace(input.Runtime.Run.InterruptReason) != "":
		return true
	}
	return false
}

func shouldRetry(input InferenceInput) bool {
	switch input.Recovery.FailureClass {
	case schema.FailureClassTimeout, schema.FailureClassConnectorUnavailable:
		return !requiresApprovalForRecovery(input)
	}
	return false
}

func shouldCompensate(input InferenceInput, candidate CandidateSummary) bool {
	if requiresApprovalForRecovery(input) {
		return false
	}
	if input.Recovery.FailureClass != schema.FailureClassPartialExecution {
		return false
	}
	capabilities := resolvedCandidateCapabilities(input.Capabilities, candidate)
	if len(capabilities) == 0 {
		capabilities = recoveryRelevantCapabilities(input.Capabilities, DecisionModeExecute)
	}
	return combinedReversibility(capabilities) == schema.ReversibilityCompensable
}

func recoveryRelevantCapabilities(capabilities []CapabilityAvailability, mode DecisionMode) []CapabilityAvailability {
	if len(capabilities) == 0 {
		return nil
	}
	out := make([]CapabilityAvailability, 0, len(capabilities))
	for _, capability := range capabilities {
		if !capability.Available {
			continue
		}
		if capability.CommandType == schema.CommandTypeCompose || strings.EqualFold(strings.TrimSpace(capability.Kind), "reply") {
			continue
		}
		if mode == DecisionModeMonitor && capability.CommandType != schema.CommandTypeQuery && capability.CommandType != schema.CommandTypeInvoke {
			continue
		}
		out = append(out, capability)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func shouldReplan(input InferenceInput) bool {
	switch input.Recovery.FailureClass {
	case schema.FailureClassVersionConflict, schema.FailureClassContradictionStaleState, schema.FailureClassPolicyBlocked:
		return true
	}
	return input.Governance.LastApprovalOutcome == schema.ApprovalOutcomeDenied
}

func requiresApprovalForRecovery(input InferenceInput) bool {
	return strings.TrimSpace(input.Governance.PendingProposalID) != "" || input.Governance.ConfirmationRequired
}
