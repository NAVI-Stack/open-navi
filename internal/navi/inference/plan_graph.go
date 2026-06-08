package inference

import (
	"fmt"
	"strings"

	"github.com/ceoai/navi/internal/schema"
)

// DefaultPlanGraphManager builds explicit, resumable planning structure for one cycle.
type DefaultPlanGraphManager struct{}

// Build returns the current plan graph for the decision cycle, reusing the prior
// plan state when it still applies to the active goal.
func (m DefaultPlanGraphManager) Build(input InferenceInput, focus FocusFrame, mode ModeSelection, candidate CandidateSummary) *PlanGraph {
	goalID := firstNonEmpty(focus.ActiveGoalID, input.GoalStack.ActiveGoalID, candidate.CandidateID, input.Chat.ChatID)
	existing := clonePlanGraph(input.PlanState)
	if existing != nil && samePlanGoal(existing, goalID) {
		return m.resumePlan(existing, input, focus, mode, candidate)
	}

	plan := &PlanGraph{
		PlanID:          planID(input, goalID, candidate),
		GoalID:          goalID,
		PlanningStyle:   mode.PlanningStyle,
		SuccessCriteria: planSuccessCriteria(candidate, goalID),
		FailureCriteria: planFailureCriteria(candidate),
		Status:          PlanStatusActive,
	}
	plan.Nodes = m.buildNodes(input, mode, candidate)
	plan.Edges = connectSequential(plan.Nodes)
	plan.Checkpoints = buildPlanCheckpoints(input, plan.Nodes, candidate)
	plan.CurrentNodeID = firstReadyNode(plan.Nodes)
	if hasPlanBlockers(plan) {
		plan.Status = PlanStatusBlocked
	}
	return plan
}

func (m DefaultPlanGraphManager) resumePlan(plan *PlanGraph, input InferenceInput, focus FocusFrame, mode ModeSelection, candidate CandidateSummary) *PlanGraph {
	plan.GoalID = firstNonEmpty(plan.GoalID, focus.ActiveGoalID, input.GoalStack.ActiveGoalID)
	plan.PlanningStyle = mode.PlanningStyle
	plan.SuccessCriteria = coalesceStrings(plan.SuccessCriteria, planSuccessCriteria(candidate, plan.GoalID))
	plan.FailureCriteria = coalesceStrings(plan.FailureCriteria, planFailureCriteria(candidate))
	if len(plan.Nodes) == 0 {
		plan.Nodes = m.buildNodes(input, mode, candidate)
		plan.Edges = connectSequential(plan.Nodes)
	}
	plan.Checkpoints = mergePlanCheckpoints(plan.Checkpoints, buildPlanCheckpoints(input, plan.Nodes, candidate))
	plan.CurrentNodeID = firstNonEmpty(plan.CurrentNodeID, firstReadyNode(plan.Nodes))
	if hasPlanBlockers(plan) {
		plan.Status = PlanStatusBlocked
	} else if plan.Status == "" || plan.Status == PlanStatusDraft {
		plan.Status = PlanStatusActive
	}
	return plan
}

func (m DefaultPlanGraphManager) buildNodes(input InferenceInput, mode ModeSelection, candidate CandidateSummary) []PlanNode {
	switch mode.PlanningStyle {
	case PlanningStyleAdaptive:
		return adaptiveNodes(input, candidate, mode.PlanningDepth)
	case PlanningStyleProgressive:
		return progressiveNodes(input, candidate, mode.PlanningDepth)
	case PlanningStyleShallow:
		return shallowNodes(input, candidate)
	default:
		return directNodes(input, candidate)
	}
}

func directNodes(input InferenceInput, candidate CandidateSummary) []PlanNode {
	return []PlanNode{newPlanNode(input, candidate, "act_now", candidate.Description, nil, false)}
}

func shallowNodes(input InferenceInput, candidate CandidateSummary) []PlanNode {
	nodes := []PlanNode{
		newPlanNode(input, candidate, "prepare", "prepare the next governed action", nil, false),
		newPlanNode(input, candidate, "act", candidate.Description, []string{planNodeID(candidate, "prepare")}, candidate.CandidateType == CandidateTypeExecute),
	}
	return nodes
}

func progressiveNodes(input InferenceInput, candidate CandidateSummary, depth int) []PlanNode {
	nodes := []PlanNode{
		newPlanNode(input, candidate, "stabilize", "stabilize context and choose the next concrete step", nil, false),
	}
	prior := planNodeID(candidate, "stabilize")
	if depth > 1 {
		nodes = append(nodes, newPlanNode(input, candidate, "decompose", "decompose the goal into explicit dependent work before acting", []string{prior}, false))
		prior = planNodeID(candidate, "decompose")
	}
	nodes = append(nodes,
		newPlanNode(input, candidate, "next_step", candidate.Description, []string{prior}, candidate.CandidateType == CandidateTypeExecute),
		newPlanNode(input, candidate, "expand", "expand the plan only when the current step reveals more complexity", []string{planNodeID(candidate, "next_step")}, false),
	)
	nodes[len(nodes)-1].Status = PlanNodeStatusPending
	return nodes
}

func adaptiveNodes(input InferenceInput, candidate CandidateSummary, depth int) []PlanNode {
	nodes := []PlanNode{
		newPlanNode(input, candidate, "assess", "assess blockers and recovery constraints", nil, false),
		newPlanNode(input, candidate, "route", "route around blockers and update the actionable path", []string{planNodeID(candidate, "assess")}, false),
	}
	prior := planNodeID(candidate, "route")
	if depth > 1 {
		nodes = append(nodes, newPlanNode(input, candidate, "stage", "stage a checkpoint-safe recovery path before resuming execution", []string{prior}, false))
		prior = planNodeID(candidate, "stage")
	}
	nodes = append(nodes, newPlanNode(input, candidate, "act", candidate.Description, []string{prior}, true))
	return nodes
}

func newPlanNode(input InferenceInput, candidate CandidateSummary, suffix, intent string, deps []string, checkpointBoundary bool) PlanNode {
	capabilities := resolvedCandidateCapabilities(input.Capabilities, candidate)
	node := PlanNode{
		NodeID:             planNodeID(candidate, suffix),
		Intent:             intent,
		TargetCapability:   resolvedTargetCapability(candidate, capabilities),
		CommandType:        firstPlanNodeCommandType(capabilities, candidate.CandidateType),
		Dependencies:       append([]string(nil), deps...),
		Reversibility:      combinedReversibility(capabilities),
		RetryPolicy:        retryPolicyForCandidate(candidate),
		CheckpointBoundary: checkpointBoundary,
		SuccessCriteria:    planSuccessCriteria(candidate, candidate.CandidateID),
		FailureCriteria:    planFailureCriteria(candidate),
		Blockers:           planNodeBlockers(input, candidate, checkpointBoundary),
		Status:             PlanNodeStatusReady,
	}
	if len(node.Blockers) > 0 {
		node.Status = PlanNodeStatusBlocked
	}
	return node
}

func firstPlanNodeCommandType(capabilities []CapabilityAvailability, candidateType CandidateType) schema.CommandType {
	if commandType := sharedCommandType(capabilities); commandType != "" {
		return commandType
	}
	return commandTypeForCandidate(candidateType)
}

func buildPlanCheckpoints(input InferenceInput, nodes []PlanNode, candidate CandidateSummary) []PlanCheckpoint {
	if len(nodes) == 0 {
		return nil
	}
	var checkpoints []PlanCheckpoint
	if input.Runtime.Checkpoint != nil {
		checkpoints = append(checkpoints, PlanCheckpoint{
			CheckpointID:  planCheckpointID(candidate, "resume"),
			NodeID:        firstNonEmpty(input.PlanStateCurrentNodeID(), firstReadyNode(nodes)),
			CheckpointRef: strings.TrimSpace(input.Runtime.Checkpoint.CheckpointID),
			Reason:        "resume existing runtime checkpoint",
		})
	}
	for _, node := range nodes {
		if !node.CheckpointBoundary {
			continue
		}
		checkpoints = append(checkpoints, PlanCheckpoint{
			CheckpointID:  planCheckpointID(candidate, node.NodeID),
			NodeID:        node.NodeID,
			CheckpointRef: runtimeCheckpointRef(input),
			Reason:        "checkpoint before governed execution boundary",
		})
	}
	if len(checkpoints) == 0 {
		return nil
	}
	return checkpoints
}

func connectSequential(nodes []PlanNode) []PlanEdge {
	if len(nodes) < 2 {
		return nil
	}
	edges := make([]PlanEdge, 0, len(nodes)-1)
	for i := 1; i < len(nodes); i++ {
		edges = append(edges, PlanEdge{
			From: nodes[i-1].NodeID,
			To:   nodes[i].NodeID,
			Kind: "depends_on",
		})
	}
	return edges
}

func firstReadyNode(nodes []PlanNode) string {
	for _, node := range nodes {
		if node.Status == PlanNodeStatusReady || node.Status == "" {
			return node.NodeID
		}
	}
	for _, node := range nodes {
		if node.NodeID != "" {
			return node.NodeID
		}
	}
	return ""
}

func hasPlanBlockers(plan *PlanGraph) bool {
	if plan == nil {
		return false
	}
	for _, node := range plan.Nodes {
		if len(node.Blockers) > 0 || node.Status == PlanNodeStatusBlocked {
			return true
		}
	}
	return false
}

func planSuccessCriteria(candidate CandidateSummary, goalID string) []string {
	goalAdvance := ""
	if strings.TrimSpace(goalID) != "" {
		goalAdvance = "advance " + goalID
	}
	return compactStrings([]string{
		goalAdvance,
		"complete the selected " + string(candidate.CandidateType) + " action cleanly",
	})
}

func planFailureCriteria(candidate CandidateSummary) []string {
	return compactStrings([]string{
		"governance blocks the selected action",
		"required capability is unavailable for " + string(candidate.CandidateType),
	})
}

func planNodeBlockers(input InferenceInput, candidate CandidateSummary, checkpointBoundary bool) []string {
	blockers := append([]string(nil), input.Recovery.PendingBlockers...)
	if candidate.ApprovalNeeded || input.Governance.PendingProposalID != "" || input.Governance.ConfirmationRequired {
		blockers = append(blockers, "approval boundary remains open")
	}
	return compactStrings(blockers)
}

func retryPolicyForCandidate(candidate CandidateSummary) string {
	switch candidate.CandidateType {
	case CandidateTypeRecover, CandidateTypeReplan:
		return "retry_with_replan"
	case CandidateTypeExecute:
		return "checkpoint_then_retry_once"
	default:
		return "no_retry"
	}
}

func planID(input InferenceInput, goalID string, candidate CandidateSummary) string {
	base := firstNonEmpty(goalID, candidate.CandidateID, input.NCOS.Frame.TraceID, input.Chat.ChatID, "plan")
	return "plan:" + strings.ReplaceAll(base, " ", "_")
}

func planNodeID(candidate CandidateSummary, suffix string) string {
	return fmt.Sprintf("%s:%s", firstNonEmpty(candidate.CandidateID, "candidate"), strings.TrimSpace(suffix))
}

func planCheckpointID(candidate CandidateSummary, suffix string) string {
	return fmt.Sprintf("checkpoint:%s:%s", firstNonEmpty(candidate.CandidateID, "candidate"), strings.TrimSpace(suffix))
}

func samePlanGoal(plan *PlanGraph, goalID string) bool {
	if plan == nil {
		return false
	}
	return strings.TrimSpace(plan.GoalID) != "" && strings.TrimSpace(plan.GoalID) == strings.TrimSpace(goalID)
}

func clonePlanGraph(plan *PlanGraph) *PlanGraph {
	if plan == nil {
		return nil
	}
	cloned := *plan
	cloned.Nodes = append([]PlanNode(nil), plan.Nodes...)
	cloned.Edges = append([]PlanEdge(nil), plan.Edges...)
	cloned.Checkpoints = append([]PlanCheckpoint(nil), plan.Checkpoints...)
	cloned.SuccessCriteria = append([]string(nil), plan.SuccessCriteria...)
	cloned.FailureCriteria = append([]string(nil), plan.FailureCriteria...)
	return &cloned
}

func mergePlanCheckpoints(current, extras []PlanCheckpoint) []PlanCheckpoint {
	if len(current) == 0 && len(extras) == 0 {
		return nil
	}
	out := append([]PlanCheckpoint(nil), current...)
	seen := make(map[string]struct{}, len(current))
	for _, checkpoint := range current {
		seen[strings.TrimSpace(checkpoint.CheckpointID)] = struct{}{}
	}
	for _, checkpoint := range extras {
		key := strings.TrimSpace(checkpoint.CheckpointID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, checkpoint)
	}
	return out
}

func coalesceStrings(current, fallback []string) []string {
	if len(current) > 0 {
		return append([]string(nil), current...)
	}
	if len(fallback) == 0 {
		return nil
	}
	return append([]string(nil), fallback...)
}

func runtimeCheckpointRef(input InferenceInput) string {
	if input.Runtime.Checkpoint == nil {
		return ""
	}
	return strings.TrimSpace(input.Runtime.Checkpoint.CheckpointID)
}

func (input InferenceInput) PlanStateCurrentNodeID() string {
	if input.PlanState == nil {
		return ""
	}
	return strings.TrimSpace(input.PlanState.CurrentNodeID)
}
