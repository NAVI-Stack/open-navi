package inference

import "strings"

const defaultModeChainDepth = 2

// DefaultModeRouter selects one dominant mode and a bounded submode chain.
type DefaultModeRouter struct {
	MaxChainDepth int
}

// Route returns the explicit mode selection for one focus frame.
func (r DefaultModeRouter) Route(input InferenceInput, focus FocusFrame) ModeSelection {
	mode := r.routeMode(input, focus)
	submodes := r.routeSubmodes(input, focus, mode)
	resolvedPosture, arbitration, invoked := resolveSubreasoning(input, focus, mode, submodes)
	selection := ModeSelection{
		DominantMode:        mode,
		SubmodeChain:        boundedModes(submodes, r.chainDepth()),
		PlanningStyle:       routePlanningStyle(mode, focus),
		PlanningDepth:       routePlanningDepth(mode, focus, input),
		InvokedSubreasoners: invoked,
		ResolvedPosture:     resolvedPosture,
		Arbitration:         arbitration,
		Transition: ModeTransition{
			Previous: previousMode(input.PreviousFocus),
			Current:  mode,
			Changed:  previousMode(input.PreviousFocus) != "" && previousMode(input.PreviousFocus) != mode,
			Reason:   string(focus.FocusReason),
		},
	}
	return selection
}

func (r DefaultModeRouter) chainDepth() int {
	if r.MaxChainDepth > 0 {
		return r.MaxChainDepth
	}
	return defaultModeChainDepth
}

func (r DefaultModeRouter) routeMode(input InferenceInput, focus FocusFrame) DecisionMode {
	switch {
	case focus.FocusReason == FocusReasonPendingRecovery:
		return DecisionModeRecover
	case isHardBlocked(input.Governance):
		return DecisionModeReject
	case focus.FocusReason == FocusReasonPendingProposal:
		return DecisionModePropose
	case focus.FocusReason == FocusReasonScheduledTrigger:
		return DecisionModeMonitor
	case focus.FocusReason == FocusReasonUrgentConflict && focus.ActiveGoalID != "":
		return DecisionModeReplan
	case focus.FocusReason == FocusReasonUserRequest && hasRelevantContext(input.RelevantContext, "needs_clarification"):
		return DecisionModeClarify
	case shouldExecute(input, focus):
		return DecisionModeExecute
	case focus.FocusReason == FocusReasonActiveGoal:
		return DecisionModePlan
	case focus.FocusReason == FocusReasonReflectionFollowup:
		return DecisionModePlan
	case focus.FocusReason == FocusReasonUserRequest:
		return DecisionModeRespond
	default:
		return DecisionModeDefer
	}
}

func (r DefaultModeRouter) routeSubmodes(input InferenceInput, focus FocusFrame, mode DecisionMode) []DecisionMode {
	var submodes []DecisionMode
	switch mode {
	case DecisionModeExecute:
		if focus.ActiveGoalID != "" {
			submodes = append(submodes, DecisionModePlan)
		}
	case DecisionModeRecover:
		if focus.ActiveGoalID != "" {
			submodes = append(submodes, DecisionModeReplan)
		}
	case DecisionModeRespond:
		if focus.ActiveGoalID != "" {
			submodes = append(submodes, DecisionModePlan)
		}
	case DecisionModeMonitor:
		if focus.ActiveGoalID != "" {
			submodes = append(submodes, DecisionModePlan)
		}
	}
	if mode != DecisionModeClarify && hasRelevantContext(input.RelevantContext, "needs_clarification") {
		submodes = append(submodes, DecisionModeClarify)
	}
	return submodes
}

func routePlanningStyle(mode DecisionMode, focus FocusFrame) PlanningStyle {
	switch mode {
	case DecisionModeExecute:
		return PlanningStyleShallow
	case DecisionModePlan:
		if focus.ActiveGoalID != "" {
			return PlanningStyleProgressive
		}
		return PlanningStyleShallow
	case DecisionModeRecover, DecisionModeReplan:
		return PlanningStyleAdaptive
	default:
		return PlanningStyleDirect
	}
}

func routePlanningDepth(mode DecisionMode, focus FocusFrame, input InferenceInput) int {
	switch mode {
	case DecisionModeExecute:
		return 1
	case DecisionModePlan:
		if focus.ActiveGoalID != "" && len(input.GoalStack.Ready) > 1 {
			return 2
		}
		return 1
	case DecisionModeRecover, DecisionModeReplan:
		return 2
	default:
		return 0
	}
}

func previousMode(focus *FocusFrame) DecisionMode {
	if focus == nil {
		return ""
	}
	return focus.DominantMode
}

func shouldExecute(input InferenceInput, focus FocusFrame) bool {
	if isHardBlocked(input.Governance) || strings.TrimSpace(input.Governance.PendingProposalID) != "" || input.Governance.ConfirmationRequired {
		return false
	}
	if !input.NCOS.RequiredOutput.AllowToolCalls {
		return false
	}
	if focus.FocusReason != FocusReasonUserRequest && focus.FocusReason != FocusReasonActiveGoal {
		return false
	}
	for _, capability := range input.Capabilities {
		if capability.Available && capability.Governed {
			return true
		}
	}
	return false
}

func boundedModes(modes []DecisionMode, max int) []DecisionMode {
	if len(modes) == 0 {
		return nil
	}
	if max <= 0 {
		max = defaultModeChainDepth
	}
	seen := make(map[DecisionMode]struct{}, len(modes))
	out := make([]DecisionMode, 0, len(modes))
	for _, mode := range modes {
		if mode == "" {
			continue
		}
		if _, ok := seen[mode]; ok {
			continue
		}
		seen[mode] = struct{}{}
		out = append(out, mode)
		if len(out) == max {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
