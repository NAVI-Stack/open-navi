package inference

import (
	"sort"
	"strings"

	"github.com/ceoai/navi/internal/governor"
	"github.com/ceoai/navi/internal/schema"
)

const (
	softThresholdScore         = 0.45
	hardThresholdScore         = 0.15
	maxAllowedCapabilitySubset = 2
)

// DefaultCandidateEvaluator makes candidate generation and scoring explicit.
type DefaultCandidateEvaluator struct{}

// Evaluate builds explicit candidates, scores them, and returns the selected result.
func (e DefaultCandidateEvaluator) Evaluate(input InferenceInput, focus FocusFrame, mode ModeSelection) EvaluationResult {
	candidates := e.generateCandidates(input, focus, mode)
	if len(candidates) == 0 {
		candidate := CandidateSummary{
			CandidateID:     "candidate:defer",
			CandidateType:   CandidateTypeDefer,
			Description:     "defer pending additional control input",
			Score:           0,
			SelectionReason: string(FocusReasonIdle),
		}
		thresholds := buildThresholdState(input.Governance, candidate.Score, nil, nil)
		return EvaluationResult{
			ChosenCandidate:    candidate,
			CandidateSummaries: []CandidateSummary{candidate},
			Thresholds:         thresholds,
		}
	}

	var chosen candidateEvaluation
	chosenSet := false
	for _, candidate := range candidates {
		if !chosenSet || candidate.summary.Score > chosen.summary.Score {
			chosen = candidate
			chosenSet = true
		}
	}

	thresholds := buildThresholdState(input.Governance, chosen.summary.Score, chosen.vetoes, chosen.approvals)
	chosen.summary.ApprovalNeeded = len(chosen.approvals) > 0
	chosen.summary.SelectionReason = firstNonEmpty(chosen.summary.SelectionReason, string(focus.FocusReason))
	if rejection := deriveRejectionState(input, chosen.summary, thresholds); rejection != nil {
		chosen.summary.RejectionCode = rejection.Reason
		chosen.summary.RejectionReason = rejection.Explanation
	}

	summaries := make([]CandidateSummary, 0, len(candidates))
	for _, candidate := range candidates {
		summary := candidate.summary
		summary.ApprovalNeeded = len(candidate.approvals) > 0
		if summary.CandidateID != chosen.summary.CandidateID {
			if rejection := deriveRejectionState(input, summary, candidate.thresholds(input)); rejection != nil {
				summary.RejectionCode = rejection.Reason
				summary.RejectionReason = rejection.Explanation
			}
		}
		summaries = append(summaries, summary)
	}

	return EvaluationResult{
		ChosenCandidate:    chosen.summary,
		CandidateSummaries: summaries,
		Thresholds:         thresholds,
		Rejection:          deriveRejectionState(input, chosen.summary, thresholds),
	}
}

type candidateEvaluation struct {
	summary   CandidateSummary
	vetoes    []VetoState
	approvals []ApprovalRef
}

func (c candidateEvaluation) thresholds(input InferenceInput) ThresholdState {
	return buildThresholdState(input.Governance, c.summary.Score, c.vetoes, c.approvals)
}

func (e DefaultCandidateEvaluator) generateCandidates(input InferenceInput, focus FocusFrame, mode ModeSelection) []candidateEvaluation {
	var out []candidateEvaluation

	add := func(modeType DecisionMode, selected bool) {
		candidate := e.buildCandidate(input, focus, modeType, selected)
		out = append(out, candidate)
	}

	add(mode.DominantMode, true)
	for _, submode := range mode.SubmodeChain {
		add(submode, false)
	}

	switch mode.DominantMode {
	case DecisionModeExecute:
		add(DecisionModePlan, false)
		add(DecisionModeDefer, false)
	case DecisionModePlan:
		add(DecisionModeRespond, false)
		add(DecisionModeDefer, false)
	case DecisionModeRespond:
		add(DecisionModeClarify, false)
		add(DecisionModeDefer, false)
	case DecisionModeClarify:
		add(DecisionModeRespond, false)
		add(DecisionModeDefer, false)
	default:
		add(DecisionModeDefer, false)
	}

	return dedupeCandidates(out)
}

func (e DefaultCandidateEvaluator) buildCandidate(input InferenceInput, focus FocusFrame, mode DecisionMode, selected bool) candidateEvaluation {
	capabilityPlan := e.resolveCapabilityPlan(input, mode)
	approvals := approvalsForCandidate(input.Governance, mode)
	vetoes := vetoesForCandidate(input, mode, capabilityPlan, approvals)
	score, supporting, blocking := scoreCandidate(input, focus, mode, capabilityPlan, approvals, vetoes)
	if selected {
		score = clamp(score+0.08, 0, 1)
		supporting = compactStrings(append(supporting, "dominant_mode_selected"))
	}

	summary := CandidateSummary{
		CandidateID:         "candidate:" + string(mode),
		CandidateType:       CandidateType(mode),
		Description:         candidateDescription(mode),
		TargetCapability:    capabilityPlan.target,
		AllowedCapabilities: append([]string(nil), capabilityPlan.allowed...),
		Score:               score,
		SupportingFactors:   compactStrings(supporting),
		BlockingFactors:     compactStrings(blocking),
		SelectionReason:     string(focus.FocusReason),
	}
	if !selected {
		summary.SelectionReason = ""
	}
	return candidateEvaluation{
		summary:   summary,
		vetoes:    vetoes,
		approvals: approvals,
	}
}

func scoreCandidate(
	input InferenceInput,
	focus FocusFrame,
	mode DecisionMode,
	capabilityPlan candidateCapabilityPlan,
	approvals []ApprovalRef,
	vetoes []VetoState,
) (float64, []string, []string) {
	score := 0.0
	supporting := make([]string, 0, 6)
	blocking := make([]string, 0, 6)

	// Primary factors.
	score += focus.PriorityScore * 0.35
	if strings.TrimSpace(input.Chat.UserMessage) != "" {
		score += 0.25
		supporting = append(supporting, "user_explicitness")
	}
	if focus.ActiveGoalID != "" {
		score += 0.15
		supporting = append(supporting, "goal_continuity")
	}

	// Secondary factors.
	score += modeAffinityScore(input, focus, mode)
	if mode == DecisionModeExecute {
		if len(capabilityPlan.capabilities) > 0 {
			score += 0.15
			supporting = append(supporting, "dependency_ready")
		} else {
			score -= 0.30
			blocking = append(blocking, "capability_unavailable")
		}
	}
	switch combinedRiskLevel(capabilityPlan.capabilities, input.Governance.RiskHint) {
	case schema.RiskCritical:
		score -= 0.35
		blocking = append(blocking, "critical_risk")
	case schema.RiskHigh:
		score -= 0.20
		blocking = append(blocking, "high_risk")
	default:
		score += 0.05
		supporting = append(supporting, "risk_within_band")
	}
	switch combinedReversibility(capabilityPlan.capabilities) {
	case schema.ReversibilityIrreversible:
		score -= 0.15
		blocking = append(blocking, "irreversible")
	default:
		score += 0.05
		supporting = append(supporting, "reversible_or_compensable")
	}
	if capabilityPlan.target != "" {
		score += 0.10
		supporting = append(supporting, "exact_capability_selected")
	} else if len(capabilityPlan.allowed) > 0 {
		score += 0.04
		supporting = append(supporting, "bounded_capability_subset_selected")
	}
	if input.PreviousFocus != nil && input.PreviousFocus.FocusID == focus.FocusID {
		score += 0.05
		supporting = append(supporting, "continuity_value")
	}
	if len(approvals) > 0 {
		score -= 0.08
		blocking = append(blocking, "approval_required")
	}
	for _, veto := range vetoes {
		if veto.Hard {
			score = 0
			blocking = append(blocking, veto.Reason)
			return clamp(score, 0, 1), supporting, compactStrings(blocking)
		}
		score -= 0.12
		blocking = append(blocking, veto.Reason)
	}

	return clamp(score, 0, 1), compactStrings(supporting), compactStrings(blocking)
}

func modeAffinityScore(input InferenceInput, focus FocusFrame, mode DecisionMode) float64 {
	switch mode {
	case DecisionModePropose:
		if focus.FocusReason == FocusReasonPendingProposal {
			return 0.30
		}
	case DecisionModeReject:
		if isHardBlocked(input.Governance) {
			return 0.35
		}
	case DecisionModeRecover:
		if focus.FocusReason == FocusReasonPendingRecovery {
			return 0.30
		}
	case DecisionModeClarify:
		if hasRelevantContext(input.RelevantContext, "needs_clarification") {
			return 0.18
		}
	case DecisionModeMonitor:
		if focus.FocusReason == FocusReasonScheduledTrigger {
			return 0.18
		}
	case DecisionModeExecute:
		if input.NCOS.RequiredOutput.AllowToolCalls {
			return 0.12
		}
	case DecisionModePlan, DecisionModeReplan:
		if focus.ActiveGoalID != "" {
			return 0.10
		}
	case DecisionModeRespond:
		if focus.FocusReason == FocusReasonUserRequest {
			return 0.08
		}
	case DecisionModeDefer:
		if focus.FocusReason == FocusReasonIdle {
			return 0.12
		}
	}
	return 0
}

func approvalsForCandidate(governance GovernanceState, mode DecisionMode) []ApprovalRef {
	switch mode {
	case DecisionModeReject, DecisionModeDefer, DecisionModeClarify, DecisionModeRespond, DecisionModePlan, DecisionModeRecover, DecisionModeReplan, DecisionModeMonitor:
		return nil
	default:
		return requiredApprovals(governance)
	}
}

func vetoesForCandidate(input InferenceInput, mode DecisionMode, capabilityPlan candidateCapabilityPlan, approvals []ApprovalRef) []VetoState {
	vetoes := make([]VetoState, 0, 4)
	if isHardBlocked(input.Governance) {
		vetoes = append(vetoes, VetoState{
			Source: "governance",
			Hard:   true,
			Reason: firstNonEmpty(strings.TrimSpace(input.Governance.BlockingReason), validationReason(input.Governance.LastValidation), "governance blocked"),
		})
	}
	if executableDecisionMode(mode) && len(capabilityPlan.capabilities) == 0 {
		vetoes = append(vetoes, VetoState{
			Source: "capability",
			Hard:   true,
			Reason: "capability unavailable",
		})
	}
	if executableDecisionMode(mode) && capabilityPlan.unresolved {
		vetoes = append(vetoes, VetoState{
			Source: "capability",
			Hard:   true,
			Reason: "capability selection unresolved",
		})
	}
	if mode == DecisionModeExecute && strings.TrimSpace(input.Chat.UserMessage) == "" && focusNeedsUserConstraint(input) {
		vetoes = append(vetoes, VetoState{
			Source: "clarifier",
			Hard:   false,
			Reason: "missing critical constraints",
		})
	}
	if len(approvals) > 0 {
		vetoes = append(vetoes, VetoState{
			Source: "approval",
			Hard:   false,
			Reason: approvals[0].Reason,
		})
	}
	return vetoes
}

func focusNeedsUserConstraint(input InferenceInput) bool {
	return hasRelevantContext(input.RelevantContext, "needs_clarification")
}

func deriveRejectionState(input InferenceInput, candidate CandidateSummary, thresholds ThresholdState) *RejectionState {
	if candidate.CandidateType != CandidateTypeReject && thresholds.ScoreBand != ThresholdBandBlocked && !hasHardVeto(thresholds.Vetoes) {
		return nil
	}

	reason := classifyRejectionReason(input, candidate, thresholds)
	explanation := firstNonEmpty(candidate.RejectionReason, firstBlockingReason(thresholds.Vetoes), firstBlockingString(candidate.BlockingFactors), "candidate rejected")
	return &RejectionState{
		Reason:           reason,
		Explanation:      explanation,
		AffectedGoalID:   firstNonEmpty(input.GoalStack.ActiveGoalID, candidate.CandidateID),
		Blockers:         append([]string(nil), candidate.BlockingFactors...),
		ResumeConditions: append([]string(nil), input.Recovery.ResumeConditions...),
		ProposalID:       strings.TrimSpace(input.Governance.PendingProposalID),
	}
}

func classifyRejectionReason(input InferenceInput, candidate CandidateSummary, thresholds ThresholdState) RejectionReason {
	if isHardBlocked(input.Governance) {
		if input.Governance.LastValidation != nil && input.Governance.LastValidation.Outcome == governor.ValidationRejected {
			return RejectionReasonGovernanceBlocked
		}
		return RejectionReasonUnauthorized
	}
	for _, veto := range thresholds.Vetoes {
		switch veto.Source {
		case "capability":
			return RejectionReasonImpossibleCapability
		case "clarifier":
			return RejectionReasonMissingCriticalConstraints
		case "governance":
			return RejectionReasonGovernanceBlocked
		}
	}
	if input.Recovery.Status == schema.RecoveryStatusOpen && input.Recovery.FailureClass != "" {
		return RejectionReasonSelfStateCompromised
	}
	if firstRisk(input.Governance.RiskHint) == schema.RiskCritical {
		return RejectionReasonUnsafe
	}
	if candidate.CandidateType == CandidateTypeReject {
		return RejectionReasonGovernanceBlocked
	}
	return RejectionReasonEnvironmentUntrusted
}

func buildThresholdState(governance GovernanceState, score float64, vetoes []VetoState, approvals []ApprovalRef) ThresholdState {
	if vetoes == nil {
		vetoes = make([]VetoState, 0, 2)
		if isHardBlocked(governance) {
			vetoes = append(vetoes, VetoState{
				Source: "governance",
				Hard:   true,
				Reason: firstNonEmpty(strings.TrimSpace(governance.BlockingReason), validationReason(governance.LastValidation), "hard governance block"),
			})
		}
		if len(approvals) > 0 {
			vetoes = append(vetoes, VetoState{
				Source: "approval",
				Hard:   false,
				Reason: firstNonEmpty(strings.TrimSpace(governance.BlockingReason), validationReason(governance.LastValidation), "approval required"),
			})
		}
	}

	band := ThresholdBandWeak
	switch {
	case len(vetoes) > 0 && hasHardVeto(vetoes):
		band = ThresholdBandBlocked
	case score >= 0.75:
		band = ThresholdBandPreferred
	case score >= softThresholdScore:
		band = ThresholdBandViable
	case score < hardThresholdScore:
		band = ThresholdBandBlocked
	}

	return ThresholdState{
		Score:                  score,
		ScoreBand:              band,
		SoftThresholdSatisfied: score >= softThresholdScore && !hasHardVeto(vetoes),
		HardThresholdSatisfied: score >= hardThresholdScore && !hasHardVeto(vetoes),
		GoverningOverride:      len(vetoes) > 0,
		Vetoes:                 vetoes,
	}
}

func hasHardVeto(vetoes []VetoState) bool {
	for _, veto := range vetoes {
		if veto.Hard {
			return true
		}
	}
	return false
}

func dedupeCandidates(candidates []candidateEvaluation) []candidateEvaluation {
	if len(candidates) == 0 {
		return nil
	}
	out := make([]candidateEvaluation, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if _, ok := seen[candidate.summary.CandidateID]; ok {
			continue
		}
		seen[candidate.summary.CandidateID] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

func firstBlockingReason(vetoes []VetoState) string {
	for _, veto := range vetoes {
		if reason := strings.TrimSpace(veto.Reason); reason != "" {
			return reason
		}
	}
	return ""
}

func firstBlockingString(values []string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

type candidateCapabilityPlan struct {
	capabilities []CapabilityAvailability
	target       string
	allowed      []string
	unresolved   bool
}

func (e DefaultCandidateEvaluator) resolveCapabilityPlan(input InferenceInput, mode DecisionMode) candidateCapabilityPlan {
	if !executableDecisionMode(mode) {
		return candidateCapabilityPlan{}
	}
	available := executableCapabilities(input.Capabilities, mode)
	if len(available) == 0 {
		return candidateCapabilityPlan{}
	}
	if len(available) == 1 {
		return candidateCapabilityPlan{
			capabilities: append([]CapabilityAvailability(nil), available...),
			target:       available[0].Name,
			allowed:      []string{available[0].Name},
		}
	}

	ranked := rankExecutableCapabilities(input, available)
	if len(ranked) == 0 || ranked[0].score <= 0 {
		return candidateCapabilityPlan{unresolved: true}
	}
	if len(ranked) == 1 || ranked[0].score-ranked[1].score >= 0.2 {
		return candidateCapabilityPlan{
			capabilities: []CapabilityAvailability{ranked[0].capability},
			target:       ranked[0].capability.Name,
			allowed:      []string{ranked[0].capability.Name},
		}
	}

	subset := boundedCapabilitySubset(ranked, len(available))
	if len(subset) == 0 || len(subset) >= len(available) {
		if explicitAll := explicitBoundedCapabilitySet(available, ranked); len(explicitAll) > 0 {
			names := make([]string, 0, len(explicitAll))
			for _, capability := range explicitAll {
				names = append(names, capability.Name)
			}
			return candidateCapabilityPlan{
				capabilities: explicitAll,
				allowed:      names,
			}
		}
		return candidateCapabilityPlan{unresolved: true}
	}
	names := make([]string, 0, len(subset))
	for _, capability := range subset {
		names = append(names, capability.Name)
	}
	if len(subset) == 1 {
		return candidateCapabilityPlan{
			capabilities: subset,
			target:       subset[0].Name,
			allowed:      names,
		}
	}
	return candidateCapabilityPlan{
		capabilities: subset,
		allowed:      names,
	}
}

type rankedCapability struct {
	capability CapabilityAvailability
	score      float64
}

func executableDecisionMode(mode DecisionMode) bool {
	switch mode {
	case DecisionModeExecute, DecisionModeMonitor:
		return true
	default:
		return false
	}
}

func executableCapabilities(capabilities []CapabilityAvailability, mode DecisionMode) []CapabilityAvailability {
	if len(capabilities) == 0 {
		return nil
	}
	out := make([]CapabilityAvailability, 0, len(capabilities))
	for _, capability := range capabilities {
		if !capability.Available || !capability.Governed {
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

func rankExecutableCapabilities(input InferenceInput, capabilities []CapabilityAvailability) []rankedCapability {
	if len(capabilities) == 0 {
		return nil
	}
	corpus := selectionCorpus(input)
	ranked := make([]rankedCapability, 0, len(capabilities))
	for _, capability := range capabilities {
		score := capabilitySelectionScore(corpus, capability)
		ranked = append(ranked, rankedCapability{capability: capability, score: score})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return strings.TrimSpace(ranked[i].capability.Name) < strings.TrimSpace(ranked[j].capability.Name)
		}
		return ranked[i].score > ranked[j].score
	})
	return ranked
}

func boundedCapabilitySubset(ranked []rankedCapability, total int) []CapabilityAvailability {
	if len(ranked) == 0 {
		return nil
	}
	top := ranked[0].score
	if top <= 0 {
		return nil
	}
	out := make([]CapabilityAvailability, 0, min(len(ranked), maxAllowedCapabilitySubset))
	for _, rankedCapability := range ranked {
		if rankedCapability.score <= 0 {
			continue
		}
		if top-rankedCapability.score > 0.1 {
			break
		}
		out = append(out, rankedCapability.capability)
		if len(out) >= maxAllowedCapabilitySubset {
			break
		}
	}
	if len(out) == 0 || len(out) >= total {
		return nil
	}
	return out
}

func explicitBoundedCapabilitySet(available []CapabilityAvailability, ranked []rankedCapability) []CapabilityAvailability {
	if len(available) == 0 || len(available) > maxAllowedCapabilitySubset {
		return nil
	}
	out := make([]CapabilityAvailability, 0, len(available))
	for _, rankedCapability := range ranked {
		if rankedCapability.score <= 0 {
			continue
		}
		out = append(out, rankedCapability.capability)
	}
	if len(out) != len(available) {
		return nil
	}
	return out
}

func selectionCorpus(input InferenceInput) string {
	fragments := []string{
		input.Chat.UserMessage,
		input.NCOS.UserMessage,
		input.GoalStack.ActiveGoalID,
	}
	if goal, ok := input.GoalStack.ActiveGoal(); ok {
		fragments = append(fragments, goal.Summary)
	}
	for _, ref := range input.RelevantContext {
		fragments = append(fragments, ref.Kind, ref.Summary)
	}
	return normalizeSelectionText(strings.Join(fragments, " "))
}

func capabilitySelectionScore(corpus string, capability CapabilityAvailability) float64 {
	score := 0.0
	for _, token := range capabilitySelectionTokens(capability) {
		if token == "" {
			continue
		}
		if strings.Contains(corpus, token) {
			score += 0.35
		}
	}
	switch firstRisk(capability.RiskHint) {
	case schema.RiskCritical:
		score -= 0.3
	case schema.RiskHigh:
		score -= 0.15
	case schema.RiskMedium:
		score -= 0.05
	default:
		score += 0.05
	}
	switch firstReversibility(capability.Reversibility) {
	case schema.ReversibilityIrreversible:
		score -= 0.15
	case schema.ReversibilityCompensable:
		score += 0.03
	default:
		score += 0.05
	}
	if capability.Governed {
		score += 0.02
	}
	if capability.CommandType != "" && capability.CommandType != schema.CommandTypeCompose {
		score += 0.03
	}
	return clamp(score, 0, 1)
}

func capabilitySelectionTokens(capability CapabilityAvailability) []string {
	return compactStrings([]string{
		normalizeSelectionText(capability.Name),
		normalizeSelectionText(capability.Kind),
		normalizeSelectionText(capability.Reason),
	})
}

func normalizeSelectionText(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(raw))
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		default:
			builder.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
