package inference

import (
	"fmt"
	"strings"

	"github.com/open-navi/navi/internal/schema"
)

const (
	defaultSubreasonerPinDecay = 2
	defaultPostureBaseline     = 0.5
)

type subreasonerTrigger struct {
	reasoner Subreasoner
	source   string
	reason   string
}

func resolveSubreasoning(input InferenceInput, focus FocusFrame, mode DecisionMode, submodes []DecisionMode) (PostureState, ArbitrationState, []Subreasoner) {
	base := normalizePostureState(input.Posture)
	triggers := collectSubreasonerTriggers(input, focus, mode, submodes)
	activePins, releasedPins := mergeSubreasonerPins(base, triggers)
	base.Pins = activePins
	base.PinnedSubreasoners = pinsToReasoners(activePins)
	base = adaptPosture(base, input, focus, mode, triggers)

	invoked, suppressed, notes := selectInvokedSubreasoners(input, focus, mode, submodes, base, triggers)
	arbitration := ArbitrationState{
		ActivePins:             append([]SubreasonerPin(nil), activePins...),
		ReleasedPins:           append([]SubreasonerPin(nil), releasedPins...),
		SuppressedSubreasoners: append([]Subreasoner(nil), suppressed...),
		Notes:                  arbitrationNotes(base, triggers, notes),
	}
	return base, arbitration, invoked
}

func normalizePostureState(posture PostureState) PostureState {
	posture.InitiativeBias = boundedPostureValue(posture.InitiativeBias)
	posture.ClarificationStrictness = boundedPostureValue(posture.ClarificationStrictness)
	posture.MonitoringAggressiveness = boundedPostureValue(posture.MonitoringAggressiveness)
	posture.Pins = clonePins(posture.Pins)
	posture.PinnedSubreasoners = pinsToReasoners(posture.Pins)
	return posture
}

func collectSubreasonerTriggers(input InferenceInput, focus FocusFrame, mode DecisionMode, submodes []DecisionMode) map[Subreasoner]subreasonerTrigger {
	triggers := map[Subreasoner]subreasonerTrigger{}
	add := func(reasoner Subreasoner, source, reason string) {
		if reasoner == "" {
			return
		}
		triggers[reasoner] = subreasonerTrigger{reasoner: reasoner, source: source, reason: reason}
	}

	if strings.TrimSpace(input.Chat.UserMessage) != "" {
		add(SubreasonerInterpreter, "chat", "user message present")
	}
	if hasRelevantContext(input.RelevantContext, "needs_clarification") || focus.FocusReason == FocusReasonUserRequest && input.Posture.ClarificationStrictness >= 0.7 {
		add(SubreasonerClarifier, "context", "clarification pressure present")
	}
	if mode == DecisionModePlan || mode == DecisionModeExecute || mode == DecisionModeReplan || hasMode(submodes, DecisionModePlan) {
		add(SubreasonerPlanner, "mode", "planning work is active")
	}
	if focus.FocusReason == FocusReasonUrgentConflict || mode == DecisionModeReplan || input.Governance.LastApprovalOutcome == schema.ApprovalOutcomeDenied {
		add(SubreasonerCritic, "conflict", "contradiction or denied path requires critique")
	}
	if input.Governance.ConfirmationRequired || input.Governance.PendingProposalID != "" || input.Governance.RiskHint == schema.RiskHigh || input.Governance.RiskHint == schema.RiskCritical {
		add(SubreasonerRiskAssessor, "governance", "governance or risk pressure present")
	}
	if hasRelevantContext(input.RelevantContext, "needs_clarification") || len(input.Recovery.PendingBlockers) > 0 {
		add(SubreasonerUncertainty, "uncertainty", "uncertainty or blockers remain")
	}
	if input.Recovery.Status == schema.RecoveryStatusOpen || mode == DecisionModeRecover {
		add(SubreasonerRecovery, "recovery", "open recovery state present")
	}
	if focus.FocusReason == FocusReasonReflectionFollowup || input.Recovery.Status == schema.RecoveryStatusOpen || input.Runtime.LastResult != nil && (input.Runtime.LastResult.Outcome == schema.ExecutionOutcomeFailed || input.Runtime.LastResult.Outcome == schema.ExecutionOutcomePartiallySucceeded) {
		add(SubreasonerReflection, "reflection", "material outcome requires reflection follow-up")
	}

	return triggers
}

func mergeSubreasonerPins(posture PostureState, triggers map[Subreasoner]subreasonerTrigger) ([]SubreasonerPin, []SubreasonerPin) {
	current := make(map[Subreasoner]SubreasonerPin, len(posture.Pins))
	for _, pin := range posture.Pins {
		if pin.Reasoner == "" {
			continue
		}
		current[pin.Reasoner] = pin
	}

	active := make([]SubreasonerPin, 0, len(current)+len(triggers))
	released := make([]SubreasonerPin, 0)
	seen := make(map[Subreasoner]struct{}, len(triggers))

	for reasoner, trigger := range triggers {
		seen[reasoner] = struct{}{}
		active = append(active, SubreasonerPin{
			Reasoner:       reasoner,
			Source:         trigger.source,
			Reason:         trigger.reason,
			DecayRemaining: defaultSubreasonerPinDecay,
		})
	}

	for reasoner, pin := range current {
		if _, ok := seen[reasoner]; ok {
			continue
		}
		pin.DecayRemaining--
		if pin.DecayRemaining > 0 {
			active = append(active, pin)
			continue
		}
		released = append(released, pin)
	}

	return compactPins(active), compactPins(released)
}

func adaptPosture(posture PostureState, input InferenceInput, focus FocusFrame, mode DecisionMode, triggers map[Subreasoner]subreasonerTrigger) PostureState {
	posture.InitiativeBias = postureBaseValue(posture.InitiativeBias)
	posture.ClarificationStrictness = postureBaseValue(posture.ClarificationStrictness)
	posture.MonitoringAggressiveness = postureBaseValue(posture.MonitoringAggressiveness)

	if input.Governance.ConfirmationRequired || input.Governance.PendingProposalID != "" || input.Governance.RiskHint == schema.RiskHigh || input.Governance.RiskHint == schema.RiskCritical {
		posture.InitiativeBias = clamp01(posture.InitiativeBias - 0.15)
		posture.ClarificationStrictness = clamp01(posture.ClarificationStrictness + 0.20)
	}
	if input.Recovery.Status == schema.RecoveryStatusOpen {
		posture.MonitoringAggressiveness = clamp01(posture.MonitoringAggressiveness + 0.15)
		posture.InitiativeBias = clamp01(posture.InitiativeBias - 0.10)
	}
	if focus.FocusReason == FocusReasonScheduledTrigger || mode == DecisionModeMonitor {
		posture.MonitoringAggressiveness = clamp01(posture.MonitoringAggressiveness + 0.20)
	}
	if mode == DecisionModeClarify || triggered(triggers, SubreasonerClarifier) {
		posture.ClarificationStrictness = clamp01(posture.ClarificationStrictness + 0.10)
	}
	if mode == DecisionModeExecute && !input.Governance.ConfirmationRequired && strings.TrimSpace(input.Governance.PendingProposalID) == "" {
		posture.InitiativeBias = clamp01(posture.InitiativeBias + 0.05)
	}

	return posture
}

func selectInvokedSubreasoners(input InferenceInput, focus FocusFrame, mode DecisionMode, submodes []DecisionMode, posture PostureState, triggers map[Subreasoner]subreasonerTrigger) ([]Subreasoner, []Subreasoner, []string) {
	invoked := make([]Subreasoner, 0, len(posture.Pins)+4)
	suppressed := make([]Subreasoner, 0, 2)
	notes := make([]string, 0, 8)
	add := func(reasoner Subreasoner, why string) {
		for _, existing := range invoked {
			if existing == reasoner {
				return
			}
		}
		invoked = append(invoked, reasoner)
		if why != "" {
			notes = append(notes, why)
		}
	}

	for _, reasoner := range posture.PinnedSubreasoners {
		add(reasoner, fmt.Sprintf("pinned:%s", reasoner))
	}

	if strings.TrimSpace(input.Chat.UserMessage) != "" {
		add(SubreasonerInterpreter, "chat:interpreter")
	}

	switch mode {
	case DecisionModeClarify:
		add(SubreasonerClarifier, "mode:clarify")
	case DecisionModePlan, DecisionModeReplan:
		add(SubreasonerPlanner, "mode:planner")
	case DecisionModeRecover:
		add(SubreasonerRecovery, "mode:recovery")
	}

	if triggered(triggers, SubreasonerCritic) {
		add(SubreasonerCritic, "trigger:critic")
	}
	if triggered(triggers, SubreasonerRiskAssessor) {
		add(SubreasonerRiskAssessor, "trigger:risk")
	}
	if triggered(triggers, SubreasonerUncertainty) && (mode == DecisionModeClarify || mode == DecisionModeDefer || len(input.Recovery.PendingBlockers) > 0) {
		add(SubreasonerUncertainty, "trigger:uncertainty")
	}
	if triggered(triggers, SubreasonerReflection) {
		add(SubreasonerReflection, "trigger:reflection")
	}

	if mode == DecisionModeExecute && !triggered(triggers, SubreasonerPlanner) && focus.ActiveGoalID == "" {
		suppressed = append(suppressed, SubreasonerPlanner)
		notes = append(notes, "suppressed:planner_no_goal")
	}
	if input.Governance.RiskHint != schema.RiskHigh && input.Governance.RiskHint != schema.RiskCritical && !input.Governance.ConfirmationRequired && strings.TrimSpace(input.Governance.PendingProposalID) == "" && !triggered(triggers, SubreasonerRiskAssessor) {
		suppressed = append(suppressed, SubreasonerRiskAssessor)
	}
	if !hasMode(submodes, DecisionModeClarify) && !triggered(triggers, SubreasonerClarifier) && mode != DecisionModeClarify && posture.ClarificationStrictness < 0.7 {
		suppressed = append(suppressed, SubreasonerClarifier)
	}

	return compactSubreasoners(invoked), compactSubreasoners(suppressed), compactStrings(notes)
}

func arbitrationNotes(posture PostureState, triggers map[Subreasoner]subreasonerTrigger, existing []string) []string {
	notes := append([]string(nil), existing...)
	for _, pin := range posture.Pins {
		if pin.DecayRemaining < defaultSubreasonerPinDecay {
			notes = append(notes, fmt.Sprintf("pin_decay:%s:%d", pin.Reasoner, pin.DecayRemaining))
		}
	}
	if triggered(triggers, SubreasonerRiskAssessor) {
		notes = append(notes, "posture:risk_bounded")
	}
	if triggered(triggers, SubreasonerRecovery) {
		notes = append(notes, "posture:recovery_bounded")
	}
	return compactStrings(notes)
}

func pinsToReasoners(pins []SubreasonerPin) []Subreasoner {
	if len(pins) == 0 {
		return nil
	}
	out := make([]Subreasoner, 0, len(pins))
	seen := make(map[Subreasoner]struct{}, len(pins))
	for _, pin := range pins {
		if pin.Reasoner == "" {
			continue
		}
		if _, ok := seen[pin.Reasoner]; ok {
			continue
		}
		seen[pin.Reasoner] = struct{}{}
		out = append(out, pin.Reasoner)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func clonePins(pins []SubreasonerPin) []SubreasonerPin {
	if len(pins) == 0 {
		return nil
	}
	return append([]SubreasonerPin(nil), pins...)
}

func compactPins(pins []SubreasonerPin) []SubreasonerPin {
	if len(pins) == 0 {
		return nil
	}
	out := make([]SubreasonerPin, 0, len(pins))
	seen := make(map[Subreasoner]struct{}, len(pins))
	for _, pin := range pins {
		if pin.Reasoner == "" {
			continue
		}
		if _, ok := seen[pin.Reasoner]; ok {
			continue
		}
		seen[pin.Reasoner] = struct{}{}
		out = append(out, pin)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func compactSubreasoners(values []Subreasoner) []Subreasoner {
	if len(values) == 0 {
		return nil
	}
	out := make([]Subreasoner, 0, len(values))
	seen := make(map[Subreasoner]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func hasMode(values []DecisionMode, target DecisionMode) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func triggered(triggers map[Subreasoner]subreasonerTrigger, reasoner Subreasoner) bool {
	_, ok := triggers[reasoner]
	return ok
}

func postureBaseValue(value float64) float64 {
	if value == 0 {
		return defaultPostureBaseline
	}
	return boundedPostureValue(value)
}

func boundedPostureValue(value float64) float64 {
	return clamp01(value)
}

func clamp01(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return value
	}
}
