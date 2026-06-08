package inference

import (
	"context"
	"strings"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

// ObserveOutcome reconciles actual runtime execution outcomes back into ICS-owned state.
func (c *Controller) ObserveOutcome(ctx context.Context, prior DecisionEnvelope, snapshot ExecutionSnapshot) (DecisionEnvelope, error) {
	if err := ctx.Err(); err != nil {
		return DecisionEnvelope{}, err
	}

	now := time.Now().UTC()
	if c != nil && c.now != nil {
		now = c.now().UTC()
	}

	updated := cloneRationale(prior.Rationale)
	updated.PlanGraph = supervisePlanGraph(updated.PlanGraph, snapshot)
	updated.RecoveryCheckpoint = superviseRecoveryCheckpoint(updated.RecoveryCheckpoint, updated.PlanGraph, snapshot)
	updated.GoalStack = superviseGoalStack(updated.GoalStack, updated.SelectedGoalID, updated.PlanGraph, snapshot)
	updated = superviseOutcomeControlState(updated, snapshot)
	updated.ReflectionHooks = superviseReflectionHooks(updated.ReflectionHooks, updated.RecoveryCheckpoint, snapshot)
	updated.DecisionTrace = superviseDecisionTrace(updated, snapshot, now)
	envelope := prior
	envelope.Rationale = updated
	if envelope.GovernanceResult != nil {
		copy := *envelope.GovernanceResult
		envelope.Rationale.Governance.ValidationResult = &copy
	}
	return envelope, nil
}

func cloneRationale(prior Rationale) Rationale {
	cloned := prior
	cloned.SubmodeChain = append([]DecisionMode(nil), prior.SubmodeChain...)
	cloned.InvokedSubreasoners = append([]Subreasoner(nil), prior.InvokedSubreasoners...)
	cloned.RequiredApprovals = append([]ApprovalRef(nil), prior.RequiredApprovals...)
	cloned.CandidateSummaries = append([]CandidateSummary(nil), prior.CandidateSummaries...)
	cloned.Extensions = cloneAnyMap(prior.Extensions)
	cloned.PlanGraph = clonePlanGraph(prior.PlanGraph)
	cloned.ReflectionHooks.Surprises = append([]string(nil), prior.ReflectionHooks.Surprises...)
	cloned.RecoveryCheckpoint.PendingBlockers = append([]string(nil), prior.RecoveryCheckpoint.PendingBlockers...)
	cloned.RecoveryCheckpoint.PendingApprovals = append([]ApprovalRef(nil), prior.RecoveryCheckpoint.PendingApprovals...)
	cloned.RecoveryCheckpoint.ResumeConditions = append([]string(nil), prior.RecoveryCheckpoint.ResumeConditions...)
	cloned.DecisionTrace.CheckpointRefs = append([]string(nil), prior.DecisionTrace.CheckpointRefs...)
	cloned.DecisionTrace.CandidateIDs = append([]string(nil), prior.DecisionTrace.CandidateIDs...)
	cloned.DecisionTrace.SubmodeChain = append([]DecisionMode(nil), prior.DecisionTrace.SubmodeChain...)
	cloned.DecisionTrace.InvokedSubreasoners = append([]Subreasoner(nil), prior.DecisionTrace.InvokedSubreasoners...)
	cloned.DecisionTrace.ActivePins = append([]SubreasonerPin(nil), prior.DecisionTrace.ActivePins...)
	cloned.DecisionTrace.SuppressedSubreasoners = append([]Subreasoner(nil), prior.DecisionTrace.SuppressedSubreasoners...)
	cloned.DecisionTrace.ArbitrationNotes = append([]string(nil), prior.DecisionTrace.ArbitrationNotes...)
	cloned.DecisionTrace.FocusCandidates = append([]FocusCandidate(nil), prior.DecisionTrace.FocusCandidates...)
	return cloned
}

func supervisePlanGraph(plan *PlanGraph, snapshot ExecutionSnapshot) *PlanGraph {
	if plan == nil {
		return nil
	}

	currentIdx := observedPlanNodeIndex(plan, currentPlanNodeIndex(plan), snapshot)
	if currentIdx < 0 {
		if plan.Status == "" {
			plan.Status = PlanStatusActive
		}
		return plan
	}

	if snapshot.ExecutedCapability != "" && strings.TrimSpace(plan.Nodes[currentIdx].TargetCapability) == "" {
		plan.Nodes[currentIdx].TargetCapability = strings.TrimSpace(snapshot.ExecutedCapability)
	}
	if snapshot.CommandType != "" && plan.Nodes[currentIdx].CommandType == "" {
		plan.Nodes[currentIdx].CommandType = snapshot.CommandType
	}
	if checkpointRef := strings.TrimSpace(snapshot.CheckpointRef); checkpointRef != "" {
		updatePlanCheckpointRef(plan, plan.Nodes[currentIdx].NodeID, checkpointRef)
	}

	switch snapshot.Outcome {
	case schema.ExecutionOutcomeSucceeded:
		plan.Nodes[currentIdx].Status = PlanNodeStatusCompleted
		plan.Nodes[currentIdx].Blockers = nil
		if nextIdx := nextIncompletePlanNodeIndex(plan, currentIdx+1); nextIdx >= 0 {
			if plan.Nodes[nextIdx].Status == "" || plan.Nodes[nextIdx].Status == PlanNodeStatusPending {
				plan.Nodes[nextIdx].Status = PlanNodeStatusReady
			}
			plan.CurrentNodeID = plan.Nodes[nextIdx].NodeID
			plan.Status = planStatusFromNodes(plan)
		} else {
			plan.CurrentNodeID = ""
			plan.Status = PlanStatusCompleted
		}
	case schema.ExecutionOutcomeRejectedPreExecution:
		plan.Nodes[currentIdx].Status = PlanNodeStatusBlocked
		switch snapshot.FailureClass {
		case schema.FailureClassApprovalPending:
			addPlanNodeBlocker(&plan.Nodes[currentIdx], firstNonEmpty(strings.TrimSpace(snapshot.Summary), "approval boundary is pending"))
		default:
			if snapshot.ApprovalOutcome == schema.ApprovalOutcomeDenied {
				addPlanNodeBlocker(&plan.Nodes[currentIdx], firstNonEmpty(strings.TrimSpace(snapshot.Summary), "approval was denied"))
			} else {
				addPlanNodeBlocker(&plan.Nodes[currentIdx], firstNonEmpty(strings.TrimSpace(snapshot.Summary), "governance blocked this action"))
			}
		}
		plan.Status = planStatusFromNodes(plan)
	case schema.ExecutionOutcomeTimedOut:
		plan.Nodes[currentIdx].Status = PlanNodeStatusBlocked
		addPlanNodeBlocker(&plan.Nodes[currentIdx], firstNonEmpty(strings.TrimSpace(snapshot.Summary), "execution timed out"))
		plan.Status = PlanStatusBlocked
	case schema.ExecutionOutcomePartiallySucceeded:
		plan.Nodes[currentIdx].Status = PlanNodeStatusBlocked
		addPlanNodeBlocker(&plan.Nodes[currentIdx], firstNonEmpty(strings.TrimSpace(snapshot.Summary), "execution requires recovery"))
		plan.Status = PlanStatusBlocked
	case schema.ExecutionOutcomeFailed, schema.ExecutionOutcomeCancelled:
		plan.Nodes[currentIdx].Status = PlanNodeStatusFailed
		addPlanNodeBlocker(&plan.Nodes[currentIdx], firstNonEmpty(strings.TrimSpace(snapshot.Summary), "execution failed"))
		plan.Status = PlanStatusFailed
	}

	if plan.Status == "" {
		plan.Status = planStatusFromNodes(plan)
	}
	return plan
}

func updatePlanCheckpointRef(plan *PlanGraph, nodeID, checkpointRef string) {
	if plan == nil || strings.TrimSpace(nodeID) == "" || strings.TrimSpace(checkpointRef) == "" {
		return
	}
	for idx := range plan.Checkpoints {
		if strings.TrimSpace(plan.Checkpoints[idx].NodeID) == strings.TrimSpace(nodeID) {
			plan.Checkpoints[idx].CheckpointRef = checkpointRef
			return
		}
	}
	plan.Checkpoints = append(plan.Checkpoints, PlanCheckpoint{
		CheckpointID:  "checkpoint:" + strings.TrimSpace(nodeID),
		NodeID:        strings.TrimSpace(nodeID),
		CheckpointRef: checkpointRef,
		Reason:        "observed runtime checkpoint",
	})
}

func currentPlanNodeIndex(plan *PlanGraph) int {
	if plan == nil || len(plan.Nodes) == 0 {
		return -1
	}
	if current := strings.TrimSpace(plan.CurrentNodeID); current != "" {
		for idx := range plan.Nodes {
			if strings.TrimSpace(plan.Nodes[idx].NodeID) == current {
				return idx
			}
		}
	}
	return nextIncompletePlanNodeIndex(plan, 0)
}

func observedPlanNodeIndex(plan *PlanGraph, currentIdx int, snapshot ExecutionSnapshot) int {
	if plan == nil {
		return -1
	}
	if currentIdx < 0 {
		return currentIdx
	}
	if strings.TrimSpace(snapshot.ExecutedCapability) == "" {
		return currentIdx
	}
	if plan.Nodes[currentIdx].CheckpointBoundary {
		return currentIdx
	}
	for idx := currentIdx; idx < len(plan.Nodes); idx++ {
		node := plan.Nodes[idx]
		if !node.CheckpointBoundary {
			continue
		}
		if sameObservedCapability(node.TargetCapability, snapshot.ExecutedCapability) {
			return idx
		}
	}
	for idx := currentIdx; idx < len(plan.Nodes); idx++ {
		if sameObservedCapability(plan.Nodes[idx].TargetCapability, snapshot.ExecutedCapability) {
			return idx
		}
	}
	return currentIdx
}

func sameObservedCapability(nodeCapability, observedCapability string) bool {
	return strings.TrimSpace(nodeCapability) != "" &&
		strings.TrimSpace(nodeCapability) == strings.TrimSpace(observedCapability)
}

func nextIncompletePlanNodeIndex(plan *PlanGraph, start int) int {
	if plan == nil {
		return -1
	}
	for idx := start; idx < len(plan.Nodes); idx++ {
		if plan.Nodes[idx].Status != PlanNodeStatusCompleted {
			return idx
		}
	}
	return -1
}

func addPlanNodeBlocker(node *PlanNode, blocker string) {
	blocker = strings.TrimSpace(blocker)
	if node == nil || blocker == "" {
		return
	}
	for _, existing := range node.Blockers {
		if strings.TrimSpace(existing) == blocker {
			return
		}
	}
	node.Blockers = append(node.Blockers, blocker)
}

func planStatusFromNodes(plan *PlanGraph) PlanStatus {
	if plan == nil || len(plan.Nodes) == 0 {
		return PlanStatusDraft
	}
	allCompleted := true
	for _, node := range plan.Nodes {
		switch node.Status {
		case PlanNodeStatusFailed:
			return PlanStatusFailed
		case PlanNodeStatusBlocked:
			return PlanStatusBlocked
		case PlanNodeStatusCompleted:
			continue
		default:
			allCompleted = false
		}
	}
	if allCompleted {
		return PlanStatusCompleted
	}
	return PlanStatusActive
}

func superviseRecoveryCheckpoint(checkpoint RecoveryCheckpoint, plan *PlanGraph, snapshot ExecutionSnapshot) RecoveryCheckpoint {
	if checkpointRef := strings.TrimSpace(snapshot.CheckpointRef); checkpointRef != "" {
		checkpoint.CheckpointRef = checkpointRef
		if strings.TrimSpace(checkpoint.RollbackPoint) == "" {
			checkpoint.RollbackPoint = checkpointRef
		}
	}
	if plan != nil {
		checkpoint.CurrentGoalID = firstNonEmpty(strings.TrimSpace(checkpoint.CurrentGoalID), planGoalID(plan))
		checkpoint.CurrentStep = firstNonEmpty(planCurrentNodeID(plan), checkpoint.CurrentStep)
	}

	switch snapshot.Outcome {
	case schema.ExecutionOutcomeSucceeded:
		if checkpoint.Open &&
			checkpoint.Route.Action != RecoveryRouteNone &&
			(plan == nil || plan.Status != PlanStatusCompleted) &&
			strings.TrimSpace(snapshot.ExecutedCapability) == "" &&
			strings.TrimSpace(snapshot.ProposalID) == "" {
			return checkpoint
		}
		checkpoint.Open = false
		checkpoint.PendingBlockers = nil
		checkpoint.PendingApprovals = nil
		checkpoint.ResumeConditions = nil
		checkpoint.Route = RecoveryRoute{Action: RecoveryRouteNone}
	case schema.ExecutionOutcomeRejectedPreExecution:
		checkpoint.Open = true
		checkpoint.CurrentStep = firstNonEmpty(planCurrentNodeID(plan), checkpoint.CurrentStep)
		switch {
		case snapshot.ApprovalOutcome == schema.ApprovalOutcomeDenied:
			checkpoint.Route = RecoveryRoute{
				Action:   RecoveryRouteReplan,
				Reason:   firstNonEmpty(strings.TrimSpace(snapshot.Summary), "approval was denied; replan required"),
				KeepOpen: true,
			}
			checkpoint.PendingApprovals = nil
			checkpoint.PendingBlockers = compactStrings(append([]string{"approval was denied"}, checkpoint.PendingBlockers...))
			checkpoint.ResumeConditions = compactStrings(append([]string{"replan around the denied approval"}, checkpoint.ResumeConditions...))
		case strings.TrimSpace(snapshot.ProposalID) != "":
			checkpoint.Route = RecoveryRoute{
				Action:           RecoveryRoutePropose,
				Reason:           firstNonEmpty(strings.TrimSpace(snapshot.Summary), "approval required before execution can continue"),
				ProposalID:       strings.TrimSpace(snapshot.ProposalID),
				RequiresApproval: true,
				KeepOpen:         true,
			}
			checkpoint.PendingBlockers = compactStrings(append([]string{"approval boundary remains open"}, checkpoint.PendingBlockers...))
			checkpoint.ResumeConditions = compactStrings(append([]string{"proposal resolution required"}, checkpoint.ResumeConditions...))
			if len(checkpoint.PendingApprovals) == 0 {
				checkpoint.PendingApprovals = []ApprovalRef{{
					Requirement: ApprovalRequirementBlockingProposal,
					ProposalID:  snapshot.ProposalID,
					Reason:      checkpoint.Route.Reason,
					Status:      schema.ProposalStatusPending,
				}}
			}
		default:
			checkpoint.Route = RecoveryRoute{
				Action:   RecoveryRouteReplan,
				Reason:   firstNonEmpty(strings.TrimSpace(snapshot.Summary), "execution was rejected before it could run"),
				KeepOpen: true,
			}
			checkpoint.PendingApprovals = nil
			checkpoint.PendingBlockers = compactStrings(append([]string{"execution was rejected before it could run"}, checkpoint.PendingBlockers...))
			checkpoint.ResumeConditions = compactStrings(append([]string{"resolve the governance rejection before resuming"}, checkpoint.ResumeConditions...))
		}
	case schema.ExecutionOutcomeTimedOut:
		checkpoint.Open = true
		checkpoint.PendingApprovals = nil
		checkpoint.Route = RecoveryRoute{
			Action:   RecoveryRouteRetry,
			Reason:   firstNonEmpty(strings.TrimSpace(snapshot.Summary), "retry after timeout"),
			KeepOpen: true,
		}
		checkpoint.PendingBlockers = compactStrings(append([]string{firstNonEmpty(strings.TrimSpace(snapshot.Summary), "execution timed out")}, checkpoint.PendingBlockers...))
		checkpoint.ResumeConditions = compactStrings(append([]string{"retry when the timeout cause is cleared"}, checkpoint.ResumeConditions...))
	case schema.ExecutionOutcomePartiallySucceeded:
		checkpoint.Open = true
		checkpoint.PendingApprovals = nil
		action := checkpoint.Route.Action
		if action == RecoveryRouteNone {
			action = RecoveryRouteRecover
		}
		if snapshot.FailureClass == schema.FailureClassPartialExecution && action == RecoveryRouteRecover {
			action = RecoveryRouteCompensate
		}
		checkpoint.Route = RecoveryRoute{
			Action:   action,
			Reason:   firstNonEmpty(strings.TrimSpace(snapshot.Summary), "partial execution still requires structured recovery"),
			KeepOpen: true,
		}
		checkpoint.PendingBlockers = compactStrings(append([]string{firstNonEmpty(strings.TrimSpace(snapshot.Summary), "execution requires compensation or recovery")}, checkpoint.PendingBlockers...))
		checkpoint.ResumeConditions = compactStrings(append([]string{"recover or compensate before advancing the plan"}, checkpoint.ResumeConditions...))
	case schema.ExecutionOutcomeFailed, schema.ExecutionOutcomeCancelled:
		checkpoint.Open = true
		checkpoint.PendingApprovals = nil
		action := checkpoint.Route.Action
		if action == RecoveryRouteNone || action == RecoveryRoutePropose {
			action = recoveryRouteForFailure(snapshot.FailureClass)
		}
		checkpoint.Route = RecoveryRoute{
			Action:   action,
			Reason:   firstNonEmpty(strings.TrimSpace(snapshot.Summary), "execution failed and requires recovery"),
			KeepOpen: true,
		}
		checkpoint.PendingBlockers = compactStrings(append([]string{firstNonEmpty(strings.TrimSpace(snapshot.Summary), "execution failed")}, checkpoint.PendingBlockers...))
		checkpoint.ResumeConditions = compactStrings(append([]string{"advance the explicit recovery route before retrying"}, checkpoint.ResumeConditions...))
	}
	return checkpoint
}

func superviseGoalStack(stack GoalStack, selectedGoalID string, plan *PlanGraph, snapshot ExecutionSnapshot) GoalStack {
	stack = stack.Normalize()
	goalID := firstNonEmpty(strings.TrimSpace(selectedGoalID), planGoalID(plan), stack.ActiveGoalID)
	if goalID == "" {
		return stack
	}

	if plan != nil && plan.Status == PlanStatusCompleted {
		if retired, _, ok := stack.Retire(goalID); ok {
			stack = retired
		}
	} else if stack.ActiveGoalID == "" {
		if promoted, _, ok := stack.Promote(goalID); ok {
			stack = promoted
		}
	}

	if stack.ActiveGoalID == "" && len(stack.Ready) > 0 {
		if promoted, _, ok := stack.Promote(stack.Ready[0].GoalID); ok {
			stack = promoted
		}
	}

	switch snapshot.Outcome {
	case schema.ExecutionOutcomeFailed, schema.ExecutionOutcomeTimedOut, schema.ExecutionOutcomePartiallySucceeded, schema.ExecutionOutcomeRejectedPreExecution, schema.ExecutionOutcomeCancelled:
		if stack.ActiveGoalID == "" && goalID != "" {
			if resumed, _, ok := stack.Resume(goalID); ok {
				stack = resumed
			}
		}
	}

	return stack.Normalize()
}

func superviseOutcomeControlState(rationale Rationale, snapshot ExecutionSnapshot) Rationale {
	nextMode, nextCandidateType := postOutcomeMode(rationale, snapshot)
	rationale.DominantMode = nextMode
	rationale.Focus.DominantMode = nextMode
	rationale.SubmodeChain = outcomeSubmodeChain(nextMode, rationale.PlanGraph, rationale.RecoveryCheckpoint)
	rationale.Focus.SubmodeChain = append([]DecisionMode(nil), rationale.SubmodeChain...)
	rationale.RequiredApprovals = append([]ApprovalRef(nil), rationale.RecoveryCheckpoint.PendingApprovals...)

	activeGoalID := firstNonEmpty(
		rationale.GoalStack.ActiveGoalID,
		planGoalID(rationale.PlanGraph),
		strings.TrimSpace(rationale.SelectedGoalID),
		strings.TrimSpace(rationale.Focus.ActiveGoalID),
	)
	rationale.SelectedGoalID = activeGoalID
	rationale.Focus.ActiveGoalID = activeGoalID

	governance, intent := outcomeGovernanceAndIntent(rationale, nextCandidateType)
	rationale.Governance = governance
	rationale.ExecutionIntent = intent
	rationale.ChosenAction = outcomeCandidateSummary(rationale, nextCandidateType, snapshot)
	rationale.Focus.FocusReason = outcomeFocusReason(rationale, snapshot, nextMode)
	rationale.Rejection = outcomeRejectionState(rationale, snapshot, nextCandidateType)

	return rationale
}

func postOutcomeMode(rationale Rationale, snapshot ExecutionSnapshot) (DecisionMode, CandidateType) {
	if rationale.PlanGraph != nil && rationale.PlanGraph.Status == PlanStatusCompleted {
		return DecisionModeRespond, CandidateTypeRespond
	}

	switch snapshot.Outcome {
	case schema.ExecutionOutcomeSucceeded:
		if node, ok := currentPlanNode(rationale.PlanGraph); ok && strings.TrimSpace(node.TargetCapability) != "" {
			return DecisionModeExecute, CandidateTypeExecute
		}
		if rationale.RecoveryCheckpoint.Open && rationale.RecoveryCheckpoint.Route.Action != RecoveryRouteNone {
			if rationale.RecoveryCheckpoint.Route.Action == RecoveryRouteReplan {
				return DecisionModeReplan, CandidateTypeReplan
			}
			return DecisionModeRecover, CandidateTypeRecover
		}
		if rationale.PlanGraph != nil && rationale.PlanGraph.CurrentNodeID != "" {
			return DecisionModePlan, CandidateTypePlan
		}
		return DecisionModeRespond, CandidateTypeRespond
	case schema.ExecutionOutcomeRejectedPreExecution:
		switch {
		case snapshot.ApprovalOutcome == schema.ApprovalOutcomeDenied:
			return DecisionModeReplan, CandidateTypeReplan
		case strings.TrimSpace(snapshot.ProposalID) != "":
			return DecisionModePropose, CandidateTypePropose
		default:
			return DecisionModeReject, CandidateTypeReject
		}
	case schema.ExecutionOutcomeTimedOut:
		if rationale.RecoveryCheckpoint.Route.Action == RecoveryRouteRetry {
			return DecisionModeRecover, CandidateTypeRecover
		}
		return DecisionModeReplan, CandidateTypeReplan
	case schema.ExecutionOutcomePartiallySucceeded:
		if rationale.RecoveryCheckpoint.Route.Action == RecoveryRouteCompensate || rationale.RecoveryCheckpoint.Route.Action == RecoveryRouteRecover || rationale.RecoveryCheckpoint.Route.Action == RecoveryRouteRetry {
			return DecisionModeRecover, CandidateTypeRecover
		}
		return DecisionModeReplan, CandidateTypeReplan
	case schema.ExecutionOutcomeFailed, schema.ExecutionOutcomeCancelled:
		switch rationale.RecoveryCheckpoint.Route.Action {
		case RecoveryRouteRetry, RecoveryRouteRecover, RecoveryRouteCompensate:
			return DecisionModeRecover, CandidateTypeRecover
		case RecoveryRoutePropose:
			return DecisionModePropose, CandidateTypePropose
		case RecoveryRouteDefer:
			return DecisionModeDefer, CandidateTypeDefer
		default:
			return DecisionModeReplan, CandidateTypeReplan
		}
	default:
		return rationale.DominantMode, rationale.ChosenAction.CandidateType
	}
}

func outcomeGovernanceAndIntent(rationale Rationale, candidateType CandidateType) (GovernanceHandoff, ExecutionIntent) {
	switch candidateType {
	case CandidateTypeExecute, CandidateTypeRecover:
		handoff, ok := outcomeGovernanceBoundary(rationale)
		if !ok {
			break
		}
		target := strings.TrimSpace(handoff.TargetCapability)
		allowed := compactStrings(append([]string(nil), handoff.AllowedCapabilities...))
		if target != "" && len(allowed) == 0 {
			allowed = []string{target}
		}
		return handoff, ExecutionIntent{
			ActionType:             string(firstOutcomeCommandType(handoff.CommandType, schema.CommandTypeInvoke)),
			Target:                 firstNonEmpty(planCurrentNodeID(rationale.PlanGraph), rationale.SelectedGoalID, rationale.Focus.ActiveGoalID),
			TargetCapability:       target,
			AllowedCapabilities:    allowed,
			ExpectedSideEffects:    compactStrings(append([]string(nil), handoff.ExpectedSideEffects...)),
			Reversibility:          firstReversibility(handoff.Reversibility),
			RiskLevel:              firstRisk(handoff.RiskLevel),
			ApprovalRequirement:    highestApprovalRequirement(rationale.RequiredApprovals),
			SuccessCondition:       firstNonEmpty(planCurrentNodeIntent(rationale.PlanGraph), "advance the current plan node under the supervised recovery path"),
			FailureCondition:       firstNonEmpty(rationale.RecoveryCheckpoint.Route.Reason, "execution deviates from the supervised recovery path"),
			FallbackOrRollbackPath: firstNonEmpty(strings.TrimSpace(rationale.RecoveryCheckpoint.RollbackPoint), strings.TrimSpace(rationale.RecoveryCheckpoint.CheckpointRef)),
		}
	case CandidateTypePropose:
		handoff, ok := outcomeGovernanceBoundary(rationale)
		if ok && len(rationale.RecoveryCheckpoint.PendingApprovals) > 0 {
			target := strings.TrimSpace(handoff.TargetCapability)
			allowed := compactStrings(append([]string(nil), handoff.AllowedCapabilities...))
			if target != "" && len(allowed) == 0 {
				allowed = []string{target}
			}
			return handoff, ExecutionIntent{
				ActionType:             string(commandTypeForCandidate(candidateType)),
				Target:                 firstNonEmpty(rationale.SelectedGoalID, rationale.Focus.ActiveGoalID, planCurrentNodeID(rationale.PlanGraph)),
				TargetCapability:       target,
				AllowedCapabilities:    allowed,
				ExpectedSideEffects:    compactStrings(append([]string(nil), handoff.ExpectedSideEffects...)),
				Reversibility:          firstReversibility(handoff.Reversibility),
				RiskLevel:              firstRisk(handoff.RiskLevel),
				ApprovalRequirement:    highestApprovalRequirement(rationale.RequiredApprovals),
				SuccessCondition:       "hold the governed action at the approval boundary until the pending proposal is resolved",
				FailureCondition:       firstNonEmpty(rationale.RecoveryCheckpoint.Route.Reason, "approval remains unresolved"),
				FallbackOrRollbackPath: firstNonEmpty(strings.TrimSpace(rationale.RecoveryCheckpoint.RollbackPoint), strings.TrimSpace(rationale.RecoveryCheckpoint.CheckpointRef)),
			}
		}
	}

	actionType := string(commandTypeForCandidate(candidateType))
	if actionType == "" {
		actionType = string(schema.CommandTypeCompose)
	}
	return clearedGovernanceHandoff(rationale.Governance), ExecutionIntent{
		ActionType:             actionType,
		Target:                 firstNonEmpty(rationale.SelectedGoalID, rationale.Focus.ActiveGoalID),
		RequiredInputs:         []string{requiredInput("goal", firstNonEmpty(rationale.SelectedGoalID, rationale.Focus.ActiveGoalID))},
		ApprovalRequirement:    highestApprovalRequirement(rationale.RequiredApprovals),
		SuccessCondition:       "surface the supervised outcome without bypassing governance",
		FailureCondition:       firstNonEmpty(rationale.RecoveryCheckpoint.Route.Reason, "post-outcome control interpretation failed"),
		FallbackOrRollbackPath: firstNonEmpty(strings.TrimSpace(rationale.RecoveryCheckpoint.RollbackPoint), strings.TrimSpace(rationale.RecoveryCheckpoint.CheckpointRef), "defer"),
	}
}

func outcomeGovernanceBoundary(rationale Rationale) (GovernanceHandoff, bool) {
	if node, ok := currentPlanNode(rationale.PlanGraph); ok && strings.TrimSpace(node.TargetCapability) != "" {
		handoff := cloneGovernanceHandoff(rationale.Governance)
		target := strings.TrimSpace(node.TargetCapability)
		handoff.TargetCapability = target
		handoff.AllowedCapabilities = []string{target}
		if detail, ok := outcomeCapabilityDetail(handoff, target); ok {
			handoff.AllowedCapabilityDetails = []CapabilityAvailability{detail}
			handoff.CommandType = firstOutcomeCommandType(handoff.CommandType, detail.CommandType, node.CommandType)
			handoff.Domain = firstNonEmpty(handoff.Domain, detail.Kind)
			handoff.Reversibility = firstReversibility(handoff.Reversibility, detail.Reversibility)
			handoff.RiskLevel = firstRisk(handoff.RiskLevel, detail.RiskHint)
			handoff.ExpectedSideEffects = compactStrings(append([]string(nil), detail.ExpectedSideEffects...))
		} else {
			handoff.AllowedCapabilityDetails = nil
			handoff.CommandType = firstOutcomeCommandType(handoff.CommandType, node.CommandType, schema.CommandTypeInvoke)
		}
		if handoff.Tags == nil {
			handoff.Tags = make(map[string]string, 2)
		}
		handoff.Tags["target_capability"] = target
		handoff.Tags["allowed_capabilities"] = target
		return handoff, true
	}
	if hasGovernedBoundary(rationale.Governance) {
		return cloneGovernanceHandoff(rationale.Governance), true
	}
	return GovernanceHandoff{}, false
}

func outcomeCandidateSummary(rationale Rationale, candidateType CandidateType, snapshot ExecutionSnapshot) CandidateSummary {
	candidate := CandidateSummary{
		CandidateID:     "candidate:" + string(candidateType),
		CandidateType:   candidateType,
		Description:     candidateDescription(DecisionMode(candidateType)),
		Score:           rationale.Confidence,
		SelectionReason: "post_execution_supervision",
	}
	candidate.TargetCapability = rationale.ExecutionIntent.TargetCapability
	candidate.AllowedCapabilities = append([]string(nil), rationale.ExecutionIntent.AllowedCapabilities...)

	switch snapshot.Outcome {
	case schema.ExecutionOutcomeSucceeded:
		candidate.SupportingFactors = compactStrings([]string{"runtime_outcome_succeeded"})
	case schema.ExecutionOutcomeRejectedPreExecution:
		candidate.BlockingFactors = compactStrings([]string{firstNonEmpty(strings.TrimSpace(snapshot.Summary), "execution stopped at the approval boundary")})
		candidate.ApprovalNeeded = strings.TrimSpace(snapshot.ProposalID) != ""
	case schema.ExecutionOutcomeTimedOut, schema.ExecutionOutcomePartiallySucceeded, schema.ExecutionOutcomeFailed, schema.ExecutionOutcomeCancelled:
		candidate.BlockingFactors = compactStrings([]string{firstNonEmpty(strings.TrimSpace(snapshot.Summary), "runtime outcome requires recovery")})
	}
	if candidate.Score == 0 {
		candidate.Score = 1
	}
	return candidate
}

func outcomeFocusReason(rationale Rationale, snapshot ExecutionSnapshot, mode DecisionMode) FocusReason {
	switch mode {
	case DecisionModePropose:
		return FocusReasonPendingProposal
	case DecisionModeRecover:
		return FocusReasonPendingRecovery
	case DecisionModeReplan:
		return FocusReasonUrgentConflict
	case DecisionModeRespond, DecisionModePlan, DecisionModeExecute:
		if rationale.Focus.FocusReason != "" {
			return rationale.Focus.FocusReason
		}
		return FocusReasonActiveGoal
	case DecisionModeReject:
		return FocusReasonUrgentConflict
	case DecisionModeDefer:
		return FocusReasonIdle
	default:
		if snapshot.Outcome == schema.ExecutionOutcomeRejectedPreExecution {
			return FocusReasonPendingProposal
		}
		return rationale.Focus.FocusReason
	}
}

func outcomeRejectionState(rationale Rationale, snapshot ExecutionSnapshot, candidateType CandidateType) *RejectionState {
	switch candidateType {
	case CandidateTypeReject:
		return &RejectionState{
			Reason:           RejectionReasonGovernanceBlocked,
			Explanation:      firstNonEmpty(strings.TrimSpace(snapshot.Summary), rationale.RecoveryCheckpoint.Route.Reason, "governance blocked execution"),
			AffectedGoalID:   firstNonEmpty(rationale.SelectedGoalID, rationale.Focus.ActiveGoalID),
			Blockers:         append([]string(nil), rationale.RecoveryCheckpoint.PendingBlockers...),
			ResumeConditions: append([]string(nil), rationale.RecoveryCheckpoint.ResumeConditions...),
			ProposalID:       strings.TrimSpace(snapshot.ProposalID),
		}
	default:
		return nil
	}
}

func outcomeSubmodeChain(mode DecisionMode, plan *PlanGraph, recovery RecoveryCheckpoint) []DecisionMode {
	switch mode {
	case DecisionModeExecute:
		if plan != nil && plan.CurrentNodeID != "" {
			return []DecisionMode{DecisionModePlan}
		}
	case DecisionModeRecover:
		if recovery.Route.Action == RecoveryRouteReplan {
			return []DecisionMode{DecisionModeReplan}
		}
	case DecisionModeRespond:
		if plan != nil && plan.Status == PlanStatusCompleted {
			return nil
		}
	}
	return nil
}

func currentPlanNode(plan *PlanGraph) (PlanNode, bool) {
	if plan == nil || strings.TrimSpace(plan.CurrentNodeID) == "" {
		return PlanNode{}, false
	}
	for _, node := range plan.Nodes {
		if strings.TrimSpace(node.NodeID) == strings.TrimSpace(plan.CurrentNodeID) {
			return node, true
		}
	}
	return PlanNode{}, false
}

func planGoalID(plan *PlanGraph) string {
	if plan == nil {
		return ""
	}
	return strings.TrimSpace(plan.GoalID)
}

func planCurrentNodeIntent(plan *PlanGraph) string {
	node, ok := currentPlanNode(plan)
	if !ok {
		return ""
	}
	return strings.TrimSpace(node.Intent)
}

func cloneGovernanceHandoff(handoff GovernanceHandoff) GovernanceHandoff {
	cloned := handoff
	cloned.AllowedCapabilities = append([]string(nil), handoff.AllowedCapabilities...)
	cloned.AllowedCapabilityDetails = append([]CapabilityAvailability(nil), handoff.AllowedCapabilityDetails...)
	cloned.ExpectedSideEffects = append([]string(nil), handoff.ExpectedSideEffects...)
	cloned.Tags = cloneStringMap(handoff.Tags)
	cloned.ValidationOrder = append([]string(nil), handoff.ValidationOrder...)
	if handoff.ValidationResult != nil {
		copy := *handoff.ValidationResult
		cloned.ValidationResult = &copy
	}
	return cloned
}

func clearedGovernanceHandoff(handoff GovernanceHandoff) GovernanceHandoff {
	cleared := cloneGovernanceHandoff(handoff)
	cleared.TargetCapability = ""
	cleared.AllowedCapabilities = nil
	cleared.AllowedCapabilityDetails = nil
	cleared.ExpectedSideEffects = nil
	cleared.ConfirmationRequired = false
	cleared.ValidationResult = nil
	return cleared
}

func outcomeCapabilityDetail(handoff GovernanceHandoff, target string) (CapabilityAvailability, bool) {
	target = strings.TrimSpace(target)
	for _, detail := range handoff.AllowedCapabilityDetails {
		if strings.TrimSpace(detail.Name) == target {
			return detail, true
		}
	}
	return CapabilityAvailability{}, false
}

func hasGovernedBoundary(handoff GovernanceHandoff) bool {
	return strings.TrimSpace(handoff.TargetCapability) != "" || len(compactStrings(handoff.AllowedCapabilities)) > 0
}

func firstOutcomeCommandType(values ...schema.CommandType) schema.CommandType {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func recoveryRouteForFailure(class schema.FailureClass) RecoveryRouteKind {
	switch class {
	case schema.FailureClassTimeout, schema.FailureClassConnectorUnavailable:
		return RecoveryRouteRetry
	case schema.FailureClassVersionConflict, schema.FailureClassContradictionStaleState, schema.FailureClassPolicyBlocked:
		return RecoveryRouteReplan
	case schema.FailureClassPartialExecution:
		return RecoveryRouteCompensate
	default:
		return RecoveryRouteRecover
	}
}

func superviseReflectionHooks(hooks ReflectionHookSet, recovery RecoveryCheckpoint, snapshot ExecutionSnapshot) ReflectionHookSet {
	hooks.WhatHappened = firstNonEmpty(strings.TrimSpace(snapshot.Summary), recovery.Route.Reason, hooks.WhatHappened)
	hooks.ExpectedVsActual = reflectionExpectedVsActualForOutcome(snapshot)
	hooks.SuccessFailureState = reflectionOutcomeForExecution(snapshot)
	hooks.FollowupTrigger = followupTriggerForOutcome(recovery, snapshot)
	hooks.LessonCandidate = lessonCandidateForOutcome(snapshot)
	hooks.MemoryCandidate = memoryCandidateForOutcome(snapshot, recovery)
	hooks.ProposalCandidate = firstNonEmpty(strings.TrimSpace(snapshot.ProposalID), strings.TrimSpace(recovery.Route.ProposalID))
	return hooks
}

func superviseDecisionTrace(rationale Rationale, snapshot ExecutionSnapshot, now time.Time) DecisionTrace {
	trace := rationale.DecisionTrace
	plan := rationale.PlanGraph
	recovery := rationale.RecoveryCheckpoint
	hooks := rationale.ReflectionHooks
	trace.Stage = "post_execution"
	trace.OccurredAt = now.UTC()
	trace.Focus = rationale.Focus
	trace.Mode = ModeTransition{
		Previous: trace.Mode.Current,
		Current:  rationale.DominantMode,
		Changed:  trace.Mode.Current != "" && trace.Mode.Current != rationale.DominantMode,
		Reason:   "post_execution_supervision",
	}
	trace.SubmodeChain = append([]DecisionMode(nil), rationale.SubmodeChain...)
	trace.PlanStatus = planStatusFromGraph(plan)
	trace.CurrentNodeID = planCurrentNodeID(plan)
	trace.CheckpointRefs = planCheckpointRefs(plan)
	trace.RecoveryRoute = recovery.Route.Action
	trace.ExecutionOutcomeRef = firstNonEmpty(strings.TrimSpace(snapshot.Summary), string(snapshot.Outcome))
	trace.TargetCapability = firstNonEmpty(strings.TrimSpace(rationale.Governance.TargetCapability), trace.TargetCapability)
	trace.AllowedCapabilities = compactStrings(append([]string(nil), rationale.Governance.AllowedCapabilities...))
	trace.ExecutedCapability = firstNonEmpty(strings.TrimSpace(snapshot.ExecutedCapability), trace.ExecutedCapability)
	trace.ExecutedCommandType = firstNonEmpty(strings.TrimSpace(string(snapshot.CommandType)), trace.ExecutedCommandType)
	trace.ProposalID = firstNonEmpty(strings.TrimSpace(snapshot.ProposalID), strings.TrimSpace(recovery.Route.ProposalID), trace.ProposalID)
	trace.RecoveryRef = firstNonEmpty(strings.TrimSpace(recovery.CheckpointRef), strings.TrimSpace(snapshot.CheckpointRef), trace.RecoveryRef)
	trace.ReflectionRef = hooks.FollowupTrigger
	return trace
}

func reflectionOutcomeForExecution(snapshot ExecutionSnapshot) OutcomeState {
	switch snapshot.Outcome {
	case schema.ExecutionOutcomeSucceeded:
		return OutcomeStateSuccess
	case schema.ExecutionOutcomeRejectedPreExecution:
		return OutcomeStateBlocked
	case schema.ExecutionOutcomePartiallySucceeded:
		return OutcomeStatePartial
	case schema.ExecutionOutcomeFailed, schema.ExecutionOutcomeTimedOut, schema.ExecutionOutcomeCancelled:
		return OutcomeStateFailure
	default:
		return OutcomeStatePending
	}
}

func reflectionExpectedVsActualForOutcome(snapshot ExecutionSnapshot) string {
	switch snapshot.Outcome {
	case schema.ExecutionOutcomeSucceeded:
		return firstNonEmpty(strings.TrimSpace(snapshot.Summary), "execution completed successfully")
	case schema.ExecutionOutcomeRejectedPreExecution:
		return firstNonEmpty(strings.TrimSpace(snapshot.Summary), "execution stopped at the approval boundary")
	case schema.ExecutionOutcomeTimedOut:
		return firstNonEmpty(strings.TrimSpace(snapshot.Summary), "execution timed out before reaching the expected outcome")
	case schema.ExecutionOutcomePartiallySucceeded:
		return firstNonEmpty(strings.TrimSpace(snapshot.Summary), "execution partially succeeded and still requires recovery")
	case schema.ExecutionOutcomeFailed, schema.ExecutionOutcomeCancelled:
		return firstNonEmpty(strings.TrimSpace(snapshot.Summary), "execution failed before reaching the expected outcome")
	default:
		return strings.TrimSpace(snapshot.Summary)
	}
}

func followupTriggerForOutcome(recovery RecoveryCheckpoint, snapshot ExecutionSnapshot) string {
	if !recovery.Open {
		return ""
	}
	if strings.TrimSpace(snapshot.ProposalID) != "" {
		return "await_proposal_resolution"
	}
	switch recovery.Route.Action {
	case RecoveryRouteRetry:
		return "retry_recovery_path"
	case RecoveryRouteCompensate:
		return "compensate_partial_execution"
	case RecoveryRouteReplan:
		return "replan_after_execution_failure"
	case RecoveryRouteRecover:
		return "continue_structured_recovery"
	case RecoveryRoutePropose:
		return "await_proposal_resolution"
	default:
		return "recovery_followup"
	}
}

func lessonCandidateForOutcome(snapshot ExecutionSnapshot) string {
	switch snapshot.Outcome {
	case schema.ExecutionOutcomeRejectedPreExecution:
		return "keep approval boundaries explicit in runtime control flow"
	case schema.ExecutionOutcomeTimedOut:
		return "time-bounded execution paths should re-enter recovery through explicit retry policy"
	case schema.ExecutionOutcomePartiallySucceeded:
		return "partial execution must keep compensation and recovery state explicit"
	case schema.ExecutionOutcomeFailed, schema.ExecutionOutcomeCancelled:
		return "failed execution should advance the explicit recovery route before the next cycle"
	default:
		return ""
	}
}

func memoryCandidateForOutcome(snapshot ExecutionSnapshot, recovery RecoveryCheckpoint) string {
	if snapshot.Outcome == schema.ExecutionOutcomeSucceeded {
		return ""
	}
	return firstNonEmpty(strings.TrimSpace(snapshot.Summary), strings.TrimSpace(recovery.Route.Reason))
}
