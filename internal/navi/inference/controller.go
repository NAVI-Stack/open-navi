package inference

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/governor"
	naviruntime "github.com/ceoai/navi/internal/runtime"
	"github.com/ceoai/navi/internal/schema"
)

var _ DecisionController = (*Controller)(nil)

// Controller is the canonical ICS decision entrypoint.
type Controller struct {
	now     func() time.Time
	arbiter FocusArbiter
	router  ModeRouter
	eval    CandidateEvaluator
	recover RecoveryManager
	planner PlanGraphManager
	deps    ControllerDeps
}

// NewController returns a deterministic ICS controller with UTC timestamps.
func NewController() *Controller {
	return &Controller{
		now:     func() time.Time { return time.Now().UTC() },
		arbiter: DefaultFocusArbiter{},
		router:  DefaultModeRouter{},
		eval:    DefaultCandidateEvaluator{},
		recover: DefaultRecoveryManager{},
		planner: DefaultPlanGraphManager{},
	}
}

// NewControllerWithDeps returns a controller that owns runtime-facing effects
// through dependency injection, without introducing an alternate control path.
func NewControllerWithDeps(deps ControllerDeps) *Controller {
	controller := NewController()
	controller.deps = deps
	return controller
}

// Decide synthesizes the canonical ICS rationale from explicit input state.
// It does not execute governance validation or emit runtime authority effects.
func (c *Controller) Decide(ctx context.Context, input InferenceInput) (DecisionSynthesis, error) {
	synthesis, err := c.decideRationale(ctx, input)
	if err != nil {
		return DecisionSynthesis{}, err
	}
	return synthesis, nil
}

func (c *Controller) decideRationale(ctx context.Context, input InferenceInput) (DecisionSynthesis, error) {
	if err := ctx.Err(); err != nil {
		return DecisionSynthesis{}, err
	}

	version, err := normalizeVersion(input.Version)
	if err != nil {
		return DecisionSynthesis{}, err
	}

	now := c.now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	arbiter := c.arbiter
	if arbiter == nil {
		arbiter = DefaultFocusArbiter{}
	}
	router := c.router
	if router == nil {
		router = DefaultModeRouter{}
	}
	evaluator := c.eval
	if evaluator == nil {
		evaluator = DefaultCandidateEvaluator{}
	}
	recoveryManager := c.recover
	if recoveryManager == nil {
		recoveryManager = DefaultRecoveryManager{}
	}
	planner := c.planner
	if planner == nil {
		planner = DefaultPlanGraphManager{}
	}

	input = normalizeInput(input)
	decisionTime := now().UTC()
	focusSelection := arbiter.Select(input, decisionTime)
	modeSelection := router.Route(input, focusSelection.Focus)
	focusSelection.Focus.DominantMode = modeSelection.DominantMode
	focusSelection.Focus.SubmodeChain = append([]DecisionMode(nil), modeSelection.SubmodeChain...)
	focusSelection.Focus.PinnedSubreasoners = append([]Subreasoner(nil), modeSelection.ResolvedPosture.PinnedSubreasoners...)

	evaluation := evaluator.Evaluate(input, focusSelection.Focus, modeSelection)
	approvals := requiredApprovals(input.Governance)
	planGraph := planner.Build(input, focusSelection.Focus, modeSelection, evaluation.ChosenCandidate)
	intent := buildExecutionIntent(input, focusSelection.Focus, evaluation.ChosenCandidate, approvals)
	governanceHandoff := buildGovernanceHandoff(input, evaluation.ChosenCandidate, intent, approvals)
	recovery := recoveryManager.Build(input, focusSelection.Focus, evaluation.ChosenCandidate, approvals)
	reflection := buildReflectionHooks(input, focusSelection.Focus, evaluation.ChosenCandidate, recovery, approvals)
	trace := buildDecisionTrace(input, focusSelection, modeSelection, evaluation.CandidateSummaries, evaluation.Thresholds, evaluation.ChosenCandidate, planGraph, recovery, reflection, decisionTime)

	rationale := Rationale{
		Version:             version,
		Focus:               focusSelection.Focus,
		SelectedGoalID:      focusSelection.Focus.ActiveGoalID,
		GoalStack:           input.GoalStack,
		Posture:             modeSelection.ResolvedPosture,
		DominantMode:        modeSelection.DominantMode,
		SubmodeChain:        append([]DecisionMode(nil), modeSelection.SubmodeChain...),
		InvokedSubreasoners: append([]Subreasoner(nil), modeSelection.InvokedSubreasoners...),
		Arbitration:         modeSelection.Arbitration,
		PlanningStyle:       modeSelection.PlanningStyle,
		PlanningDepth:       modeSelection.PlanningDepth,
		ChosenAction:        evaluation.ChosenCandidate,
		Confidence:          evaluation.ChosenCandidate.Score,
		Thresholds:          evaluation.Thresholds,
		RequiredApprovals:   approvals,
		PlanGraph:           planGraph,
		Governance:          governanceHandoff,
		ExecutionIntent:     intent,
		Rejection:           evaluation.Rejection,
		RecoveryCheckpoint:  recovery,
		ReflectionHooks:     reflection,
		CandidateSummaries:  append([]CandidateSummary(nil), evaluation.CandidateSummaries...),
		DecisionTrace:       trace,
		Extensions:          cloneAnyMap(input.Extensions),
	}

	return DecisionSynthesis{
		Focus:      focusSelection,
		Mode:       modeSelection,
		Candidate:  evaluation,
		Rationale:  rationale,
		OccurredAt: decisionTime,
	}, nil
}

func normalizeVersion(version string) (string, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		return ContractVersionV1, nil
	}
	if version != ContractVersionV1 {
		return "", fmt.Errorf("inference: unsupported contract version %q", version)
	}
	return version, nil
}

func normalizeInput(input InferenceInput) InferenceInput {
	input.GoalStack = input.GoalStack.Normalize()
	if strings.TrimSpace(input.Chat.ChatID) == "" {
		input.Chat.ChatID = strings.TrimSpace(input.NCOS.Frame.ChatID)
	}
	if strings.TrimSpace(input.Chat.UserMessage) == "" {
		input.Chat.UserMessage = strings.TrimSpace(input.NCOS.UserMessage)
	}
	if input.Runtime.CurrentPhase == "" {
		switch {
		case input.Runtime.Run != nil && input.Runtime.Run.CurrentPhase != "":
			input.Runtime.CurrentPhase = input.Runtime.Run.CurrentPhase
		case input.Runtime.Checkpoint != nil && input.Runtime.Checkpoint.Phase != "":
			input.Runtime.CurrentPhase = input.Runtime.Checkpoint.Phase
		}
	}
	return input
}

func buildExecutionIntent(input InferenceInput, focus FocusFrame, candidate CandidateSummary, approvals []ApprovalRef) ExecutionIntent {
	capabilities := resolvedCandidateCapabilities(input.Capabilities, candidate)
	actionType := string(candidate.CandidateType)
	if actionType == "" {
		actionType = string(focus.DominantMode)
	}
	if executableCandidateType(candidate.CandidateType) {
		if commandType := sharedCommandType(capabilities); commandType != "" {
			actionType = string(commandType)
		}
	}
	if actionType == "" {
		if commandType := sharedCommandType(capabilities); commandType != "" {
			actionType = string(commandType)
		}
	}

	return ExecutionIntent{
		ActionType:             actionType,
		Target:                 executionIntentTarget(input, focus, candidate, capabilities),
		TargetCapability:       resolvedTargetCapability(candidate, capabilities),
		AllowedCapabilities:    candidateAllowedCapabilities(candidate, capabilities),
		RequiredInputs:         compactStrings([]string{requiredInput("ncos", "present"), requiredInput("chat", input.Chat.ChatID)}),
		ExpectedSideEffects:    combinedExpectedSideEffects(capabilities),
		Reversibility:          combinedReversibility(capabilities),
		RiskLevel:              combinedRiskLevel(capabilities, input.Governance.RiskHint),
		ApprovalRequirement:    highestApprovalRequirement(approvals),
		SuccessCondition:       "control handoff remains explicit and inspectable",
		FailureCondition:       "required governance or runtime context is missing",
		FallbackOrRollbackPath: firstNonEmpty(strings.TrimSpace(input.Recovery.RollbackPoint), checkpointID(input.Runtime.Checkpoint), "defer"),
	}
}

func buildGovernanceHandoff(input InferenceInput, candidate CandidateSummary, intent ExecutionIntent, approvals []ApprovalRef) GovernanceHandoff {
	capabilities := resolvedCandidateCapabilities(input.Capabilities, candidate)
	commandType := sharedCommandType(capabilities)
	if commandType == "" {
		commandType = commandTypeForCandidate(candidate.CandidateType)
	}
	return GovernanceHandoff{
		CommandType:              commandType,
		Domain:                   firstNonEmpty(sharedCapabilityKind(capabilities), input.Chat.SourceChannel, "navi"),
		ActorKind:                "navi",
		Intent:                   candidate.Description,
		Scope:                    governanceScope(input, candidate, capabilities),
		TargetCapability:         resolvedTargetCapability(candidate, capabilities),
		AllowedCapabilities:      candidateAllowedCapabilities(candidate, capabilities),
		AllowedCapabilityDetails: append([]CapabilityAvailability(nil), capabilities...),
		ExpectedSideEffects:      append([]string(nil), intent.ExpectedSideEffects...),
		Reversibility:            intent.Reversibility,
		RiskLevel:                intent.RiskLevel,
		ApprovalRequirement:      highestApprovalRequirement(approvals),
		ConfirmationRequired:     len(approvals) > 0,
		Tags:                     governanceTags(input, candidate, intent),
		ValidationResult:         input.Governance.LastValidation,
	}
}

func buildReflectionHooks(input InferenceInput, focus FocusFrame, candidate CandidateSummary, recovery RecoveryCheckpoint, approvals []ApprovalRef) ReflectionHookSet {
	return ReflectionHookSet{
		WhatHappened:        reflectionWhatHappened(focus, candidate, recovery),
		ExpectedVsActual:    reflectionExpectedVsActual(input, recovery),
		SuccessFailureState: reflectionOutcome(input, approvals),
		Surprises:           reflectionSurprises(input, recovery),
		LessonCandidate:     reflectionLessonCandidate(input, recovery),
		MemoryCandidate:     reflectionMemoryCandidate(input, focus, recovery),
		ProposalCandidate:   firstNonEmpty(strings.TrimSpace(input.Governance.PendingProposalID), recovery.Route.ProposalID),
		FollowupTrigger:     followupTrigger(input, recovery, approvals),
	}
}

func requiredApprovals(governance GovernanceState) []ApprovalRef {
	out := make([]ApprovalRef, 0, 1)
	switch {
	case strings.TrimSpace(governance.PendingProposalID) != "":
		out = append(out, ApprovalRef{
			Requirement: ApprovalRequirementBlockingProposal,
			ProposalID:  strings.TrimSpace(governance.PendingProposalID),
			Reason:      firstNonEmpty(strings.TrimSpace(governance.BlockingReason), validationReason(governance.LastValidation), "proposal pending"),
			Status:      firstProposalStatus(governance.PendingProposalStatus, schema.ProposalStatusPending),
		})
	case governance.ConfirmationRequired:
		out = append(out, ApprovalRef{
			Requirement: ApprovalRequirementExplicitConfirmation,
			Reason:      firstNonEmpty(strings.TrimSpace(governance.BlockingReason), validationReason(governance.LastValidation), "confirmation required"),
		})
	}
	return out
}

func candidateDescription(mode DecisionMode) string {
	switch mode {
	case DecisionModeRecover:
		return "recover from explicit recovery state"
	case DecisionModeReject:
		return "reject due to explicit governance block"
	case DecisionModePropose:
		return "surface explicit approval boundary"
	case DecisionModePlan:
		return "plan around the active goal"
	case DecisionModeExecute:
		return "execute through a governed capability"
	case DecisionModeClarify:
		return "request missing critical constraints"
	case DecisionModeMonitor:
		return "monitor for the relevant trigger or condition"
	case DecisionModeReplan:
		return "replan around changed constraints or interruptions"
	case DecisionModeRespond:
		return "respond to the current user input"
	default:
		return "defer pending additional control input"
	}
}

func commandTypeForCandidate(candidateType CandidateType) schema.CommandType {
	switch candidateType {
	case CandidateTypeExecute:
		return schema.CommandTypeInvoke
	case CandidateTypeMonitor:
		return schema.CommandTypeQuery
	case CandidateTypePropose:
		return schema.CommandTypeAcquire
	case CandidateTypePlan, CandidateTypeRespond, CandidateTypeClarify, CandidateTypeReplan:
		return schema.CommandTypeCompose
	default:
		return schema.CommandTypeQuery
	}
}

func governanceTags(input InferenceInput, candidate CandidateSummary, intent ExecutionIntent) map[string]string {
	tags := map[string]string{
		"candidate_type": string(candidate.CandidateType),
		"target":         intent.Target,
	}
	if strings.TrimSpace(intent.TargetCapability) != "" {
		tags["target_capability"] = strings.TrimSpace(intent.TargetCapability)
	}
	if len(intent.AllowedCapabilities) > 0 {
		tags["allowed_capabilities"] = strings.Join(intent.AllowedCapabilities, ",")
	}
	if strings.TrimSpace(input.Governance.PendingProposalID) != "" {
		tags["proposal_id"] = strings.TrimSpace(input.Governance.PendingProposalID)
	}
	if input.Governance.RiskHint != "" {
		tags["risk_hint"] = string(input.Governance.RiskHint)
	}
	if input.GoalStack.ActiveGoalID != "" {
		tags["goal_id"] = input.GoalStack.ActiveGoalID
	}
	return tags
}

func reflectionOutcome(input InferenceInput, approvals []ApprovalRef) OutcomeState {
	switch {
	case isHardBlocked(input.Governance):
		return OutcomeStateBlocked
	case input.Runtime.LastResult != nil && input.Runtime.LastResult.Outcome == schema.ExecutionOutcomeFailed:
		return OutcomeStateFailure
	case input.Runtime.LastResult != nil && input.Runtime.LastResult.Outcome == schema.ExecutionOutcomePartiallySucceeded:
		return OutcomeStatePartial
	case input.Runtime.LastResult != nil && input.Runtime.LastResult.Outcome == schema.ExecutionOutcomeSucceeded:
		return OutcomeStateSuccess
	case len(approvals) > 0:
		return OutcomeStateBlocked
	default:
		return OutcomeStatePending
	}
}

func followupTrigger(input InferenceInput, recovery RecoveryCheckpoint, approvals []ApprovalRef) string {
	switch {
	case len(approvals) > 0:
		return "await approval resolution"
	case recovery.Open && recovery.Route.Action == RecoveryRouteRetry:
		return "retry recovery when blockers clear"
	case recovery.Open && recovery.Route.Action == RecoveryRouteCompensate:
		return "run compensation before resuming"
	case recovery.Open && recovery.Route.Action == RecoveryRouteReplan:
		return "re-enter recovery flow through replanning"
	case recovery.Open && recovery.Route.Action == RecoveryRouteDefer:
		return "resume when recovery blockers clear"
	case recovery.Open:
		return "re-enter recovery flow"
	default:
		return ""
	}
}

func reflectionWhatHappened(focus FocusFrame, candidate CandidateSummary, recovery RecoveryCheckpoint) string {
	if recovery.Route.Action != "" && recovery.Route.Action != RecoveryRouteNone {
		return fmt.Sprintf("ICS selected %s mode and routed recovery via %s", focus.DominantMode, recovery.Route.Action)
	}
	return fmt.Sprintf("ICS rationale assembled for %s", focus.DominantMode)
}

func reflectionExpectedVsActual(input InferenceInput, recovery RecoveryCheckpoint) string {
	return firstNonEmpty(
		lastResultSummary(input.Runtime.LastResult),
		strings.TrimSpace(input.Recovery.FailureReason),
		recovery.Route.Reason,
		"execution not yet supervised by ICS",
	)
}

func reflectionSurprises(input InferenceInput, recovery RecoveryCheckpoint) []string {
	return compactStrings(append(
		append([]string(nil), input.Recovery.PendingBlockers...),
		strings.TrimSpace(input.Recovery.FailureReason),
		recovery.Route.Reason,
	))
}

func reflectionLessonCandidate(input InferenceInput, recovery RecoveryCheckpoint) string {
	switch recovery.Route.Action {
	case RecoveryRouteRetry:
		return "transient failures should preserve an explicit retry route"
	case RecoveryRouteCompensate:
		return "partial external effects require explicit compensation planning"
	case RecoveryRouteReplan:
		return "changed constraints should shift recovery into replanning"
	case RecoveryRoutePropose:
		return "recovery across approval boundaries must remain proposal-mediated"
	case RecoveryRouteDefer:
		return "blocked recovery must preserve resume conditions instead of collapsing"
	case RecoveryRouteRecover:
		return "open recovery should remain structured and resumable"
	}
	if input.Recovery.FailureClass != "" {
		return "failure state remained explicit in ICS"
	}
	return ""
}

func reflectionMemoryCandidate(input InferenceInput, focus FocusFrame, recovery RecoveryCheckpoint) string {
	if focus.ActiveGoalID == "" && recovery.Route.Action == RecoveryRouteNone {
		return ""
	}
	return firstNonEmpty(
		strings.TrimSpace(input.Recovery.FailureReason),
		recovery.Route.Reason,
		"goal "+focus.ActiveGoalID+" required explicit recovery handling",
	)
}

func focusScope(input InferenceInput, candidate CandidateSummary) string {
	return firstNonEmpty(input.GoalStack.ActiveGoalID, strings.TrimSpace(input.Governance.PendingProposalID), candidate.TargetCapability, candidate.CandidateID)
}

func executionIntentTarget(input InferenceInput, focus FocusFrame, candidate CandidateSummary, capabilities []CapabilityAvailability) string {
	targetCapability := resolvedTargetCapability(candidate, capabilities)
	if executableCandidateType(candidate.CandidateType) {
		return firstNonEmpty(targetCapability, focus.ActiveGoalID, strings.TrimSpace(input.Governance.PendingProposalID), candidate.CandidateID)
	}
	return firstNonEmpty(focus.ActiveGoalID, strings.TrimSpace(input.Governance.PendingProposalID), runtimeRunID(input.Runtime.Run), input.Chat.ChatID)
}

func governanceScope(input InferenceInput, candidate CandidateSummary, capabilities []CapabilityAvailability) string {
	scope := focusScope(input, candidate)
	if executableCandidateType(candidate.CandidateType) {
		return firstNonEmpty(scope, resolvedTargetCapability(candidate, capabilities), sharedCapabilityKind(capabilities), candidate.CandidateID)
	}
	return scope
}

func firstPhase(runtime RuntimeContext) naviruntime.RunPhase {
	if runtime.CurrentPhase != "" {
		return runtime.CurrentPhase
	}
	if runtime.Run != nil && runtime.Run.CurrentPhase != "" {
		return runtime.Run.CurrentPhase
	}
	if runtime.Checkpoint != nil && runtime.Checkpoint.Phase != "" {
		return runtime.Checkpoint.Phase
	}
	return ""
}

func checkpointID(checkpoint *naviruntime.Checkpoint) string {
	if checkpoint == nil {
		return ""
	}
	return strings.TrimSpace(checkpoint.CheckpointID)
}

func isHardBlocked(governance GovernanceState) bool {
	if governance.HardBlocked {
		return true
	}
	return governance.LastValidation != nil && governance.LastValidation.Outcome == governor.ValidationRejected
}

func validationReason(result *governor.ValidationResult) string {
	if result == nil {
		return ""
	}
	return strings.TrimSpace(result.Reason)
}

func lastResultSummary(result *ExecutionSnapshot) string {
	if result == nil {
		return ""
	}
	return strings.TrimSpace(result.Summary)
}

func runtimeRunID(run *naviruntime.RunState) string {
	if run == nil {
		return ""
	}
	return strings.TrimSpace(run.RunID)
}

func requiredInput(name, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return name
}

func highestApprovalRequirement(approvals []ApprovalRef) ApprovalRequirement {
	for _, approval := range approvals {
		if approval.Requirement == ApprovalRequirementBlockingProposal {
			return approval.Requirement
		}
	}
	for _, approval := range approvals {
		if approval.Requirement != "" {
			return approval.Requirement
		}
	}
	return ApprovalRequirementNone
}

func firstProposalStatus(current, fallback schema.ProposalStatus) schema.ProposalStatus {
	if current != "" {
		return current
	}
	return fallback
}

func firstRisk(values ...schema.RiskLevel) schema.RiskLevel {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return schema.RiskLow
}

func firstReversibility(values ...schema.ReversibilityClass) schema.ReversibilityClass {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return schema.ReversibilityInternal
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func compactStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
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

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func executableCandidateType(candidateType CandidateType) bool {
	switch candidateType {
	case CandidateTypeExecute, CandidateTypeMonitor:
		return true
	default:
		return false
	}
}

func resolvedCandidateCapabilities(capabilities []CapabilityAvailability, candidate CandidateSummary) []CapabilityAvailability {
	if len(capabilities) == 0 {
		return nil
	}
	if target := strings.TrimSpace(candidate.TargetCapability); target != "" {
		for _, capability := range capabilities {
			if capability.Available && strings.TrimSpace(capability.Name) == target {
				return []CapabilityAvailability{capability}
			}
		}
	}
	allowed := compactStrings(candidate.AllowedCapabilities)
	if len(allowed) == 0 {
		return nil
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = struct{}{}
	}
	out := make([]CapabilityAvailability, 0, len(allowed))
	for _, capability := range capabilities {
		if !capability.Available {
			continue
		}
		if _, ok := allowedSet[strings.TrimSpace(capability.Name)]; ok {
			out = append(out, capability)
		}
	}
	return out
}

func candidateAllowedCapabilities(candidate CandidateSummary, capabilities []CapabilityAvailability) []string {
	if target := strings.TrimSpace(candidate.TargetCapability); target != "" {
		return []string{target}
	}
	if len(candidate.AllowedCapabilities) > 0 {
		return compactStrings(candidate.AllowedCapabilities)
	}
	if len(capabilities) == 0 {
		return nil
	}
	out := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		out = append(out, capability.Name)
	}
	return compactStrings(out)
}

func resolvedTargetCapability(candidate CandidateSummary, capabilities []CapabilityAvailability) string {
	if target := strings.TrimSpace(candidate.TargetCapability); target != "" {
		return target
	}
	allowed := candidateAllowedCapabilities(candidate, capabilities)
	if len(allowed) == 1 {
		return allowed[0]
	}
	return ""
}

func combinedExpectedSideEffects(capabilities []CapabilityAvailability) []string {
	values := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		values = append(values, capability.ExpectedSideEffects...)
	}
	return compactStrings(values)
}

func combinedRiskLevel(capabilities []CapabilityAvailability, fallback schema.RiskLevel) schema.RiskLevel {
	level := fallback
	for _, capability := range capabilities {
		switch firstRisk(capability.RiskHint, level) {
		case schema.RiskCritical:
			level = schema.RiskCritical
		case schema.RiskHigh:
			if level != schema.RiskCritical {
				level = schema.RiskHigh
			}
		case schema.RiskMedium:
			if level != schema.RiskCritical && level != schema.RiskHigh {
				level = schema.RiskMedium
			}
		case schema.RiskLow:
			if level == "" {
				level = schema.RiskLow
			}
		}
	}
	if level == "" {
		return schema.RiskLow
	}
	return level
}

func combinedReversibility(capabilities []CapabilityAvailability) schema.ReversibilityClass {
	level := schema.ReversibilityInternal
	for _, capability := range capabilities {
		switch firstReversibility(capability.Reversibility, schema.ReversibilityInternal) {
		case schema.ReversibilityIrreversible:
			return schema.ReversibilityIrreversible
		case schema.ReversibilityCompensable:
			if level != schema.ReversibilityIrreversible {
				level = schema.ReversibilityCompensable
			}
		}
	}
	return level
}

func sharedCommandType(capabilities []CapabilityAvailability) schema.CommandType {
	var commandType schema.CommandType
	for _, capability := range capabilities {
		if capability.CommandType == "" {
			continue
		}
		if commandType == "" {
			commandType = capability.CommandType
			continue
		}
		if commandType != capability.CommandType {
			return ""
		}
	}
	return commandType
}

func sharedCapabilityKind(capabilities []CapabilityAvailability) string {
	kind := ""
	for _, capability := range capabilities {
		current := strings.TrimSpace(capability.Kind)
		if current == "" {
			continue
		}
		if kind == "" {
			kind = current
			continue
		}
		if kind != current {
			return ""
		}
	}
	return kind
}
