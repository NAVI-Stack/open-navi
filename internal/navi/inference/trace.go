package inference

import (
	"strings"
	"time"
)

func buildDecisionTrace(
	input InferenceInput,
	focus FocusSelection,
	mode ModeSelection,
	candidates []CandidateSummary,
	thresholds ThresholdState,
	candidate CandidateSummary,
	plan *PlanGraph,
	recovery RecoveryCheckpoint,
	reflection ReflectionHookSet,
	now time.Time,
) DecisionTrace {
	return DecisionTrace{
		TraceID:                firstNonEmpty(input.NCOS.Frame.TraceID, runtimeRunID(input.Runtime.Run), input.Chat.ChatID),
		Stage:                  "decision",
		OccurredAt:             now,
		Focus:                  focus.Focus,
		FocusCandidates:        append([]FocusCandidate(nil), focus.Candidates...),
		Mode:                   mode.Transition,
		SubmodeChain:           append([]DecisionMode(nil), mode.SubmodeChain...),
		InvokedSubreasoners:    append([]Subreasoner(nil), mode.InvokedSubreasoners...),
		ActivePins:             append([]SubreasonerPin(nil), mode.Arbitration.ActivePins...),
		SuppressedSubreasoners: append([]Subreasoner(nil), mode.Arbitration.SuppressedSubreasoners...),
		ArbitrationNotes:       append([]string(nil), mode.Arbitration.Notes...),
		CandidateIDs:           candidateIDs(candidates, candidate),
		TargetCapability:       strings.TrimSpace(candidate.TargetCapability),
		AllowedCapabilities:    append([]string(nil), candidate.AllowedCapabilities...),
		PlanID:                 planIDFromGraph(plan),
		PlanStatus:             planStatusFromGraph(plan),
		CurrentNodeID:          planCurrentNodeID(plan),
		CheckpointRefs:         planCheckpointRefs(plan),
		RecoveryRoute:          recovery.Route.Action,
		Thresholds:             thresholds,
		GovernanceOutcome:      traceGovernanceOutcome(input.Governance, thresholds),
		ExecutionOutcomeRef:    lastResultSummary(input.Runtime.LastResult),
		ExecutedCapability:     lastResultCapability(input.Runtime.LastResult),
		ExecutedCommandType:    lastResultCommandType(input.Runtime.LastResult),
		ProposalID:             firstNonEmpty(strings.TrimSpace(input.Governance.PendingProposalID), lastResultProposalID(input.Runtime.LastResult)),
		RecoveryRef:            recovery.CheckpointRef,
		ReflectionRef:          reflection.FollowupTrigger,
		ResumeFocusID:          focus.ResumeFocusID,
	}
}

func candidateIDs(candidates []CandidateSummary, chosen CandidateSummary) []string {
	if len(candidates) == 0 {
		return compactStrings([]string{chosen.CandidateID})
	}
	ids := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.CandidateID)
	}
	return compactStrings(ids)
}

func planIDFromGraph(plan *PlanGraph) string {
	if plan == nil {
		return ""
	}
	return strings.TrimSpace(plan.PlanID)
}

func planStatusFromGraph(plan *PlanGraph) PlanStatus {
	if plan == nil {
		return ""
	}
	return plan.Status
}

func planCurrentNodeID(plan *PlanGraph) string {
	if plan == nil {
		return ""
	}
	return strings.TrimSpace(plan.CurrentNodeID)
}

func planCheckpointRefs(plan *PlanGraph) []string {
	if plan == nil || len(plan.Checkpoints) == 0 {
		return nil
	}
	refs := make([]string, 0, len(plan.Checkpoints))
	for _, checkpoint := range plan.Checkpoints {
		refs = append(refs, firstNonEmpty(checkpoint.CheckpointRef, checkpoint.CheckpointID, checkpoint.NodeID))
	}
	return compactStrings(refs)
}

func traceGovernanceOutcome(governance GovernanceState, thresholds ThresholdState) string {
	switch {
	case isHardBlocked(governance):
		return "rejected"
	case firstNonEmpty(governance.PendingProposalID) != "":
		return "proposal_pending"
	case governance.ConfirmationRequired:
		return "confirmation_required"
	case thresholds.GoverningOverride:
		return "gated"
	default:
		return "clear"
	}
}

func lastResultCapability(snapshot *ExecutionSnapshot) string {
	if snapshot == nil {
		return ""
	}
	return strings.TrimSpace(snapshot.ExecutedCapability)
}

func lastResultCommandType(snapshot *ExecutionSnapshot) string {
	if snapshot == nil {
		return ""
	}
	return strings.TrimSpace(string(snapshot.CommandType))
}

func lastResultProposalID(snapshot *ExecutionSnapshot) string {
	if snapshot == nil {
		return ""
	}
	return strings.TrimSpace(snapshot.ProposalID)
}
