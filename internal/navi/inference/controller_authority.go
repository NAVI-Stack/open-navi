package inference

import (
	"context"
	"strings"

	"github.com/ceoai/navi/internal/governor"
	"github.com/ceoai/navi/internal/navi/orchestration"
)

func (c *Controller) finalizeDecision(ctx context.Context, input InferenceInput, rationale Rationale) (DecisionEnvelope, error) {
	envelope := defaultDecisionEnvelope(rationale)
	if proposalID := strings.TrimSpace(input.Governance.PendingProposalID); proposalID != "" {
		envelope.RuntimeDisposition = RuntimeDispositionPauseForProposal
		envelope.ReplyMessage = firstNonEmpty(strings.TrimSpace(input.Governance.BlockingReason), "proposal resolution required")
		envelope.ExecutionBoundary.FailClosedReason = envelope.ReplyMessage
		if c.deps.GetProposal != nil {
			if proposal, err := c.deps.GetProposal(ctx, proposalID); err == nil {
				envelope.Proposal = &proposal
			}
		}
		return envelope, nil
	}
	if !rationale.ExecutionIntent.AllowsCapabilityExecution() {
		return envelope, nil
	}
	if reason := decisionGuardReason(input); reason != "" {
		return authorizationBlockedEnvelope(envelope, governor.ValidationResult{
			Outcome: governor.ValidationRejected,
			Reason:  reason,
		}, reason), nil
	}
	if !hasAuthoritativeRationaleGovernanceHandoff(rationale) {
		reason := "ICS did not provide an authoritative executable governance handoff for this execution intent"
		return authorizationBlockedEnvelope(envelope, governor.ValidationResult{
			Outcome: governor.ValidationRejected,
			Reason:  reason,
		}, reason), nil
	}
	if c.deps.ValidationDependency == nil {
		reason := "ICS controller is missing the deterministic validation dependencies required for executable governance"
		return authorizationBlockedEnvelope(envelope, governor.ValidationResult{
			Outcome: governor.ValidationRejected,
			Reason:  reason,
		}, reason), nil
	}

	allowedDetails := append([]CapabilityAvailability(nil), rationale.Governance.AllowedCapabilityDetails...)
	if len(allowedDetails) == 0 {
		allowedDetails = decisionCapabilitiesFromInput(input, rationale)
	}
	if len(allowedDetails) == 0 {
		reason := "ICS did not authorize an executable capability boundary for this run"
		return authorizationBlockedEnvelope(envelope, governor.ValidationResult{
			Outcome: governor.ValidationRejected,
			Reason:  reason,
		}, reason), nil
	}

	filtered := make([]CapabilityAvailability, 0, len(allowedDetails))
	var aggregate *governor.ValidationResult
	for _, capability := range allowedDetails {
		govResult, err := ValidateCandidate(ctx, input, rationale, capability.Name, c.deps.ValidationDependency)
		if err != nil {
			return DecisionEnvelope{}, err
		}
		if govResult.ConstraintContext != nil {
			aggregate = mergeInferenceValidation(aggregate, *govResult.ConstraintContext)
		}

		switch govResult.Outcome {
		case governor.ValidationRejected:
			if strings.TrimSpace(rationale.Governance.TargetCapability) == strings.TrimSpace(capability.Name) {
				validation := derefValidation(govResult.ConstraintContext)
				return authorizationBlockedEnvelope(defaultDecisionEnvelope(applyPreflightValidation(rationale, validation, nil)), validation, govResult.Reason), nil
			}
		case governor.ValidationModified, governor.ValidationRequiresConfirmation:
			if strings.TrimSpace(rationale.Governance.TargetCapability) == strings.TrimSpace(capability.Name) || len(allowedDetails) == 1 {
				validation := derefValidation(govResult.ConstraintContext)
				env := authorizationPauseEnvelope(defaultDecisionEnvelope(applyPreflightValidation(rationale, validation, nil)), validation, govResult.Proposal)
				if govResult.ProposalID != "" {
					env.Rationale.DecisionTrace.ProposalID = govResult.ProposalID
					env.Rationale.ReflectionHooks.ProposalCandidate = govResult.ProposalID
				}
				return env, nil
			}
		default:
			filtered = append(filtered, capability)
		}
	}

	if len(filtered) == 0 {
		validation := derefValidation(aggregate)
		if validation.Reason == "" {
			validation = governor.ValidationResult{
				Outcome: governor.ValidationRejected,
				Reason:  "no executable capability survived ICS governance",
			}
		}
		return authorizationBlockedEnvelope(defaultDecisionEnvelope(applyPreflightValidation(rationale, validation, nil)), validation, validation.Reason), nil
	}

	rationale = applyPreflightValidation(rationale, derefValidation(aggregate), filtered)
	envelope = defaultDecisionEnvelope(rationale)
	envelope.GovernanceResult = maybeValidationPtr(aggregate)
	return envelope, nil
}

// ValidateGovernedDecision applies governance validation and emits the authoritative runtime envelope.
func (c *Controller) ValidateGovernedDecision(ctx context.Context, input InferenceInput, synthesis DecisionSynthesis) (DecisionEnvelope, error) {
	return c.finalizeDecision(ctx, input, synthesis.Rationale)
}

func hasAuthoritativeRationaleGovernanceHandoff(rationale Rationale) bool {
	if strings.TrimSpace(rationale.Governance.TargetCapability) != "" {
		return true
	}
	return len(compactStrings(append([]string(nil), rationale.Governance.AllowedCapabilities...))) > 0
}

func applyPreflightValidation(rationale Rationale, validation governor.ValidationResult, allowed []CapabilityAvailability) Rationale {
	rationale.Governance.ValidationResult = &validation
	rationale.DecisionTrace.GovernanceOutcome = validationOutcomeName(validation.Outcome)
	if validation.Outcome == governor.ValidationRejected {
		rationale.Thresholds.ScoreBand = ThresholdBandBlocked
	}
	if allowed == nil {
		// We preserve TargetCapability for blocked/paused intents to ensure
		// recovery and tracing context remains intact across cycles.
		if validation.Outcome == governor.ValidationApproved {
			rationale.Governance.TargetCapability = ""
		}
		rationale.Governance.AllowedCapabilities = nil
		rationale.Governance.AllowedCapabilityDetails = nil
		rationale.ExecutionIntent.TargetCapability = rationale.Governance.TargetCapability
		rationale.ExecutionIntent.AllowedCapabilities = nil
		rationale.ChosenAction.TargetCapability = ""
		rationale.ChosenAction.AllowedCapabilities = nil
		rationale.DecisionTrace.TargetCapability = ""
		rationale.DecisionTrace.AllowedCapabilities = nil
		return rationale
	}

	allowed = append([]CapabilityAvailability(nil), allowed...)
	allowedNames := make([]string, 0, len(allowed))
	for _, capability := range allowed {
		allowedNames = append(allowedNames, capability.Name)
	}
	allowedNames = compactStrings(allowedNames)
	targetCapability := strings.TrimSpace(rationale.Governance.TargetCapability)
	if targetCapability != "" && !containsRuntimeString(allowedNames, targetCapability) {
		targetCapability = ""
	}
	if targetCapability == "" && len(allowedNames) == 1 {
		targetCapability = allowedNames[0]
	}
	rationale.Governance.TargetCapability = targetCapability
	rationale.Governance.AllowedCapabilities = allowedNames
	rationale.Governance.AllowedCapabilityDetails = allowed
	rationale.ExecutionIntent.TargetCapability = targetCapability
	rationale.ExecutionIntent.AllowedCapabilities = append([]string(nil), allowedNames...)
	rationale.ChosenAction.TargetCapability = targetCapability
	rationale.ChosenAction.AllowedCapabilities = append([]string(nil), allowedNames...)
	rationale.DecisionTrace.TargetCapability = targetCapability
	rationale.DecisionTrace.AllowedCapabilities = append([]string(nil), allowedNames...)
	return rationale
}

func mergeInferenceValidation(current *governor.ValidationResult, next governor.ValidationResult) *governor.ValidationResult {
	if current == nil {
		copy := next
		return &copy
	}
	if validationSeverity(next.Outcome) > validationSeverity(current.Outcome) {
		copy := next
		return &copy
	}
	return current
}

func validationSeverity(outcome governor.ValidationOutcome) int {
	switch outcome {
	case governor.ValidationRejected:
		return 4
	case governor.ValidationRequiresConfirmation:
		return 3
	case governor.ValidationModified:
		return 2
	case governor.ValidationApproved:
		return 1
	default:
		return 0
	}
}

func containsRuntimeString(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func decisionGuardReason(input InferenceInput) string {
	normalize := func(value string) string {
		value = strings.TrimSpace(value)
		if value == "" {
			return ""
		}
		lower := strings.ToLower(value)
		if strings.Contains(lower, "no tools resolved for surface") {
			return "I could not safely expose the selected tool capability, so I stopped to avoid widening tool access."
		}
		if strings.Contains(lower, "tool-capable model") || strings.Contains(lower, "tool_reliable") {
			return "This request needs reliable tool use, but the selected model cannot call tools for this surface."
		}
		return value
	}
	if input.Extensions == nil {
		return ""
	}
	if value, _ := input.Extensions["surface_guard_reason"].(string); strings.TrimSpace(value) != "" {
		return normalize(value)
	}
	if value, _ := input.Extensions["routing_guard_reason"].(string); strings.TrimSpace(value) != "" {
		return normalize(value)
	}
	return ""
}

func decisionCapabilitiesFromInput(input InferenceInput, rationale Rationale) []CapabilityAvailability {
	allowed := rationale.ExecutionIntent.AuthorizedCapabilities()
	if len(allowed) == 0 {
		allowed = append([]string(nil), rationale.Governance.AllowedCapabilities...)
	}
	if len(allowed) == 0 {
		return nil
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedSet[strings.TrimSpace(name)] = struct{}{}
	}
	out := make([]CapabilityAvailability, 0, len(allowed))
	for _, capability := range input.Capabilities {
		if _, ok := allowedSet[strings.TrimSpace(capability.Name)]; ok {
			out = append(out, capability)
		}
	}
	return out
}

func derefValidation(result *governor.ValidationResult) governor.ValidationResult {
	if result == nil {
		return governor.ValidationResult{Outcome: governor.ValidationApproved}
	}
	return *result
}

func maybeValidationPtr(result *governor.ValidationResult) *governor.ValidationResult {
	if result == nil {
		return nil
	}
	copy := *result
	return &copy
}

func routingGuardReply(profile orchestration.ModelProfile) string {
	if len(profile.Metadata) == 0 {
		return ""
	}
	switch strings.TrimSpace(profile.Metadata["routing_guard"]) {
	case string(orchestration.SurfaceGuardToolCapableModelRequired), "no_tool_capable_model":
		return "This task needs reliable tool use, but I do not have a tool-capable model available right now. Switch to a tool-capable model and try again."
	default:
		return ""
	}
}
