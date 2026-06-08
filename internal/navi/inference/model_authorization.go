package inference

import (
	"context"
	"fmt"
	"strings"

	"github.com/open-navi/navi/internal/governor"
	"github.com/open-navi/navi/internal/llm"
	navitool "github.com/open-navi/navi/internal/tool"
)

// PrepareModelCall asks ICS to emit the exact model-call directive for this cycle.
func (c *Controller) PrepareModelCall(ctx context.Context, prior DecisionEnvelope, input ModelCallPreparationInput) (DecisionEnvelope, error) {
	if err := ctx.Err(); err != nil {
		return DecisionEnvelope{}, err
	}
	envelope := prior
	envelope.AuthorizedToolCalls = nil
	envelope.AuthorizedTools = nil
	envelope.ToolPermit = nil
	if content := routingGuardReply(input.Compiled.Profile); content != "" {
		return authorizationBlockedEnvelope(envelope, governor.ValidationResult{
			Outcome: governor.ValidationRejected,
			Reason:  content,
		}, content), nil
	}
	if prior.Rationale.ExecutionIntent.AllowsCapabilityExecution() && !hasAuthoritativeRationaleGovernanceHandoff(prior.Rationale) {
		reason := "ICS did not provide an authoritative executable governance handoff for this execution intent"
		return authorizationBlockedEnvelope(envelope, governor.ValidationResult{
			Outcome: governor.ValidationRejected,
			Reason:  reason,
		}, reason), nil
	}
	directive, reason := BuildPreparedModelDirective(prior, input)
	if reason != "" {
		blocked := authorizationBlockedEnvelope(envelope, governor.ValidationResult{
			Outcome: governor.ValidationRejected,
			Reason:  reason,
		}, reason)
		blocked.ReplyMessage = preparedModelDirectiveUserReply()
		blocked.ExecutionBoundary.FailClosedReason = reason
		return blocked, nil
	}
	envelope.ModelDirective = directive
	return envelope, nil
}

// AuthorizeModelResponse routes normalized model output back through ICS before runtime acts on it.
func (c *Controller) AuthorizeModelResponse(ctx context.Context, prior DecisionEnvelope, input ModelResponseAuthorizationInput) (DecisionEnvelope, error) {
	if err := ctx.Err(); err != nil {
		return DecisionEnvelope{}, err
	}
	envelope := prior
	envelope.AuthorizedToolCalls = nil
	envelope.AuthorizedTools = nil
	envelope.ToolPermit = nil
	envelope.PendingToolCall = nil
	envelope.PendingToolPermit = nil
	envelope.Proposal = nil

	allowedNames := make([]string, 0, len(prior.ExecutionBoundary.AllowedCapabilities))
	if prior.ModelDirective != nil && len(prior.ModelDirective.ToolNames) > 0 {
		allowedNames = append(allowedNames, prior.ModelDirective.ToolNames...)
	} else {
		allowedNames = append(allowedNames, prior.ExecutionBoundary.AllowedCapabilities...)
	}
	allowedNames = compactStrings(allowedNames)

	if len(input.Response.ToolCalls) == 0 {
		// A visible executable surface means the model was allowed to call tools, not
		// that a tool call was mandatory. Natural-language completions remain valid
		// unless a future explicit required-tool contract says otherwise.
		envelope.ExecutionBoundary.FailClosedReason = ""
		envelope.ReplyMessage = ""
		return envelope, nil
	}

	if c.deps.ValidationDependency == nil {
		reason := "ICS controller is missing the deterministic validation dependencies required for executable tool authorization"
		return authorizationBlockedEnvelope(envelope, governor.ValidationResult{
			Outcome: governor.ValidationRejected,
			Reason:  reason,
		}, reason), nil
	}

	attempts := make(map[string]ToolAuthorizationInput, len(input.ToolAttempts))
	for _, attempt := range input.ToolAttempts {
		if toolCallID := strings.TrimSpace(attempt.ToolCallID); toolCallID != "" {
			attempts[toolCallID] = attempt
		}
	}
	allowed := make(map[string]struct{}, len(allowedNames))
	for _, name := range allowedNames {
		allowed[name] = struct{}{}
	}

	authorized := make([]llm.ToolCall, 0, len(input.Response.ToolCalls))
	unauthorized := make([]llm.ToolCall, 0, len(input.Response.ToolCalls))
	for _, tc := range input.Response.ToolCalls {
		name := strings.TrimSpace(tc.Name)
		if name == "" {
			unauthorized = append(unauthorized, tc)
			continue
		}
		if !toolLoadedInActiveSet(input.ActiveToolSet, input.ToolRegistry, name) {
			unauthorized = append(unauthorized, tc)
			continue
		}
		if len(allowed) > 0 {
			if _, ok := allowed[name]; !ok {
				unauthorized = append(unauthorized, tc)
				continue
			}
		}
		authorized = append(authorized, tc)
	}
	if len(unauthorized) > 0 {
		recoveries := recoverUnauthorizedToolCalls(input, allowedNames, unauthorized)
		reason := summarizeUnauthorizedToolRecoveries(recoveries)
		blocked := authorizationBlockedEnvelope(envelope, governor.ValidationResult{
			Outcome: governor.ValidationRejected,
			Reason:  reason,
		}, reason)
		recordUnauthorizedToolRecoveries(&blocked, recoveries)
		return blocked, nil
	}

	var latestValidation *governor.ValidationResult
	for _, tc := range authorized {
		toolName := strings.TrimSpace(tc.Name)
		attempt, ok := attempts[strings.TrimSpace(tc.ID)]
		if !ok {
			reason := fmt.Sprintf("ICS did not receive the resolved execution metadata needed to govern tool call %q", toolName)
			return authorizationBlockedEnvelope(envelope, governor.ValidationResult{
				Outcome: governor.ValidationRejected,
				Reason:  reason,
			}, reason), nil
		}
		attempt.ToolCallID = strings.TrimSpace(tc.ID)
		attempt.ToolName = firstNonEmpty(toolName, strings.TrimSpace(attempt.ToolName))
		if attempt.Arguments == nil {
			attempt.Arguments = tc.Arguments
		}
		if attempt.Contract == nil {
			reason := missingExecutableContractReason(toolName)
			return authorizationBlockedEnvelope(envelope, governor.ValidationResult{
				Outcome: governor.ValidationRejected,
				Reason:  reason,
			}, reason), nil
		}
		contractCopy := *attempt.Contract
		if !toolContractValid(contractCopy, toolName) {
			reason := invalidExecutableContractReason(toolName)
			return authorizationBlockedEnvelope(envelope, governor.ValidationResult{
				Outcome: governor.ValidationRejected,
				Reason:  reason,
			}, reason), nil
		}

		verifiedApprovedProposalID := ""
		if strings.TrimSpace(attempt.ApprovedProposalID) != "" {
			verifiedProposal, reason, err := c.verifyApprovedProposalForToolAttempt(ctx, prior, attempt)
			if err != nil {
				return DecisionEnvelope{}, err
			}
			if reason != "" {
				return authorizationBlockedEnvelope(envelope, governor.ValidationResult{
					Outcome: governor.ValidationRejected,
					Reason:  reason,
				}, reason), nil
			}
			verifiedApprovedProposalID = strings.TrimSpace(verifiedProposal.ProposalID)
			attempt.VerifiedProposalID = verifiedApprovedProposalID
		}

		govResult, err := ValidateToolAttempt(ctx, attempt, prior, c.deps.ValidationDependency)
		if err != nil {
			return DecisionEnvelope{}, err
		}
		if govResult.ConstraintContext != nil {
			latestValidation = maybeValidationPtr(mergeInferenceValidation(latestValidation, *govResult.ConstraintContext))
		}

		permit := ToolPermit{
			ContractID:         strings.TrimSpace(contractCopy.ID),
			Contract:           &contractCopy,
			ToolCallID:         strings.TrimSpace(tc.ID),
			ToolName:           toolName,
			ApprovedProposalID: verifiedApprovedProposalID,
			TargetPath:         strings.TrimSpace(attempt.ResolvedTargetPath),
		}

		switch govResult.Outcome {
		case governor.ValidationApproved:
			envelope.AuthorizedToolCalls = append(envelope.AuthorizedToolCalls, tc)
			envelope.AuthorizedTools = append(envelope.AuthorizedTools, AuthorizedToolInvocation{
				ToolCall: tc,
				Permit:   permit,
			})
		case governor.ValidationModified, governor.ValidationRequiresConfirmation:
			if verifiedApprovedProposalID != "" {
				permit.ApprovedProposalID = verifiedApprovedProposalID
				envelope.AuthorizedToolCalls = append(envelope.AuthorizedToolCalls, tc)
				envelope.AuthorizedTools = append(envelope.AuthorizedTools, AuthorizedToolInvocation{
					ToolCall: tc,
					Permit:   permit,
				})
				continue
			}
			tcCopy := tc
			permitCopy := permit
			if latestValidation != nil {
				envelope.GovernanceResult = maybeValidationPtr(latestValidation)
				envelope.Rationale.Governance.ValidationResult = maybeValidationPtr(latestValidation)
				envelope.Rationale.DecisionTrace.GovernanceOutcome = validationOutcomeName(latestValidation.Outcome)
			}
			envelope.RuntimeDisposition = RuntimeDispositionPauseForProposal
			envelope.ReplyMessage = firstNonEmpty(govResult.Reason, validationReason(latestValidation), "approval required")
			envelope.ExecutionBoundary.FailClosedReason = envelope.ReplyMessage
			envelope.Proposal = govResult.Proposal
			envelope.PendingToolCall = &tcCopy
			envelope.PendingToolPermit = &permitCopy
			if govResult.Proposal != nil {
				envelope.Rationale.DecisionTrace.ProposalID = govResult.ProposalID
				envelope.Rationale.ReflectionHooks.ProposalCandidate = govResult.ProposalID
			}
			return envelope, nil
		default:
			reason := govResult.Reason
			if latestValidation != nil {
				return authorizationBlockedEnvelope(envelope, *latestValidation, firstNonEmpty(reason, validationReason(latestValidation), "ICS blocked this tool call")), nil
			}
			return authorizationBlockedEnvelope(envelope, governor.ValidationResult{
				Outcome: governor.ValidationRejected,
				Reason:  firstNonEmpty(reason, "ICS blocked this tool call"),
			}, firstNonEmpty(reason, "ICS blocked this tool call")), nil
		}
	}
	if latestValidation != nil {
		envelope.GovernanceResult = maybeValidationPtr(latestValidation)
		envelope.Rationale.Governance.ValidationResult = maybeValidationPtr(latestValidation)
		envelope.Rationale.DecisionTrace.GovernanceOutcome = validationOutcomeName(latestValidation.Outcome)
	}
	if len(envelope.AuthorizedTools) > 0 {
		envelope.RuntimeDisposition = RuntimeDispositionProceedDirect
		envelope.ExecutionBoundary.FailClosedReason = ""
		envelope.ReplyMessage = ""
	}
	return envelope, nil
}

// BuildPreparedModelDirective materializes the authoritative model-call boundary
// for one cycle, including executable contracts for every surfaced tool.
func BuildPreparedModelDirective(prior DecisionEnvelope, input ModelCallPreparationInput) (*ModelDirective, string) {
	directive := &ModelDirective{
		AllowToolCalls: prior.Rationale.ExecutionIntent.AllowsCapabilityExecution(),
		ToolChoice:     strings.TrimSpace(prior.ExecutionBoundary.ToolChoice),
	}
	if !directive.AllowToolCalls {
		return directive, ""
	}

	allowed := make(map[string]struct{}, len(prior.ExecutionBoundary.AllowedCapabilities))
	for _, capability := range prior.ExecutionBoundary.AllowedCapabilities {
		if name := strings.TrimSpace(capability); name != "" {
			allowed[name] = struct{}{}
		}
	}

	executable := make(map[string]struct{}, len(input.ExecutableToolNames)+len(input.ToolContracts))
	for _, name := range input.ExecutableToolNames {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			executable[trimmed] = struct{}{}
		}
	}
	for name := range input.ToolContracts {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			executable[trimmed] = struct{}{}
		}
	}

	directive.ToolContracts = make(map[string]ToolContract)
	for _, tool := range input.Compiled.Tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		if !toolLoadedInActiveSet(input.ActiveToolSet, input.ToolRegistry, name) {
			continue
		}
		if len(allowed) > 0 {
			if _, ok := allowed[name]; !ok {
				continue
			}
		}
		if _, ok := executable[name]; ok {
			contract, ok := normalizedPreparedToolContract(input.ToolContracts, name)
			if !ok {
				return nil, missingExecutableContractReason(name)
			}
			directive.ToolContracts[name] = contract
		}
		directive.ToolNames = append(directive.ToolNames, name)
	}
	directive.ToolNames = compactStrings(directive.ToolNames)
	if len(directive.ToolNames) == 0 {
		reason := strings.TrimSpace(prior.ExecutionBoundary.FailClosedReason)
		if reason == "" {
			targetCapability := firstNonEmpty(strings.TrimSpace(directive.ToolChoice), strings.TrimSpace(prior.ExecutionBoundary.TargetCapability), strings.TrimSpace(prior.Rationale.ExecutionIntent.TargetCapability))
			allowedCapabilities := compactStrings(append([]string(nil), prior.ExecutionBoundary.AllowedCapabilities...))
			switch {
			case targetCapability != "":
				reason = fmt.Sprintf("ExecutionIntent selected capability %q but runtime could not safely surface it for execution", targetCapability)
			case len(allowedCapabilities) > 0:
				reason = fmt.Sprintf("ExecutionIntent authorized capabilities [%s] but runtime could not safely surface any of them for execution", strings.Join(allowedCapabilities, ", "))
			default:
				reason = "ExecutionIntent did not leave a safe executable tool surface for this run"
			}
		}
		return nil, reason
	}
	if directive.ToolChoice != "" {
		if _, executableTool := executable[directive.ToolChoice]; !executableTool {
			if len(directive.ToolContracts) == 0 {
				directive.ToolContracts = nil
			}
			return directive, ""
		}
		if _, ok := directive.ToolContracts[directive.ToolChoice]; !ok {
			return nil, missingExecutableContractReason(directive.ToolChoice)
		}
	}
	if len(directive.ToolContracts) == 0 {
		directive.ToolContracts = nil
	}
	return directive, ""
}

// ModelDirectiveToolContract returns the authoritative contract emitted for the
// named tool, if one exists.
func ModelDirectiveToolContract(directive *ModelDirective, toolName string) *ToolContract {
	if directive == nil || len(directive.ToolContracts) == 0 {
		return nil
	}
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return nil
	}
	contract, ok := directive.ToolContracts[toolName]
	if !ok || !toolContractValid(contract, toolName) {
		return nil
	}
	copy := contract
	return &copy
}

func normalizedPreparedToolContract(contracts map[string]ToolContract, toolName string) (ToolContract, bool) {
	if len(contracts) == 0 {
		return ToolContract{}, false
	}
	contract, ok := contracts[strings.TrimSpace(toolName)]
	if !ok || !toolContractValid(contract, toolName) {
		return ToolContract{}, false
	}
	return contract, true
}

func toolContractValid(contract ToolContract, toolName string) bool {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return false
	}
	if strings.TrimSpace(contract.ID) == "" {
		return false
	}
	if strings.TrimSpace(contract.ToolName) != toolName {
		return false
	}
	if strings.TrimSpace(string(contract.ExecutionKind)) == "" {
		return false
	}
	if contract.CommandType == "" {
		return false
	}
	if strings.TrimSpace(contract.Domain) == "" {
		return false
	}
	if strings.TrimSpace(contract.ActorKind) == "" {
		return false
	}
	if !contract.WorkspaceAction.IsValid() {
		return false
	}
	if contract.WorkspaceScopedPath && strings.TrimSpace(contract.TargetPathArg) == "" {
		return false
	}
	return true
}

func missingExecutableContractReason(toolName string) string {
	return fmt.Sprintf("ICS could not emit an authoritative executable ToolContract for surfaced tool %q", strings.TrimSpace(toolName))
}

func invalidExecutableContractReason(toolName string) string {
	return fmt.Sprintf("ICS received an invalid authoritative executable ToolContract for surfaced tool %q", strings.TrimSpace(toolName))
}

func toolLoadedInActiveSet(activeToolSet *navitool.ActiveToolSet, reg *navitool.Registry, toolName string) bool {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return false
	}
	if activeToolSet == nil {
		return true
	}
	if reg != nil {
		if exact, ok := reg.LookupExact(toolName); ok && exact.Tool != nil {
			return activeToolSet.ContainsToolVersion(exact.Tool.ToolID, exact.Tool.SchemaVersion)
		}
	}
	return activeToolSet.ContainsTool(toolName)
}

func preparedModelDirectiveUserReply() string {
	return "I couldn't safely execute that action with the tools currently available, so I stopped instead of widening tool access."
}

func recoverUnauthorizedToolCalls(input ModelResponseAuthorizationInput, allowedNames []string, toolCalls []llm.ToolCall) []navitool.ToolCallRecoveryResult {
	if len(toolCalls) == 0 {
		return nil
	}
	modelProfile := navitool.BrokerModelProfile{
		Name:             firstNonEmpty(strings.TrimSpace(input.Compiled.Profile.Model), strings.TrimSpace(input.Compiled.Profile.Provider)),
		SupportsTools:    input.Compiled.Profile.SupportsTools || len(input.Compiled.Tools) > 0,
		ToolCallReliable: input.Compiled.Profile.SupportsTools || len(input.Compiled.Tools) > 0,
	}
	results := make([]navitool.ToolCallRecoveryResult, 0, len(toolCalls))
	for _, tc := range toolCalls {
		results = append(results, navitool.RecoverToolCall(navitool.ToolCallRecoveryInput{
			Registry:      input.ToolRegistry,
			ToolCall:      tc,
			ActiveToolIDs: allowedNames,
			BrokerInput: navitool.BrokerInput{
				UserInput:     strings.TrimSpace(tc.Name),
				ActiveToolIDs: append([]string(nil), allowedNames...),
				ModelProfile:  modelProfile,
			},
			RepairAttemptsRemaining: input.RepairAttemptsRemaining,
		}))
	}
	return results
}

func summarizeUnauthorizedToolRecoveries(results []navitool.ToolCallRecoveryResult) string {
	if len(results) == 0 {
		return "the requested action referenced a tool outside the current authorized surface"
	}
	if len(results) == 1 {
		result := results[0]
		if strings.TrimSpace(result.SafeReason) != "" {
			return strings.TrimSpace(result.SafeReason)
		}
	}
	hasUnknown := false
	hasLoadableUnloaded := false
	for _, result := range results {
		switch result.Disposition {
		case navitool.ToolCallRecoveryDispositionUnknownTool:
			hasUnknown = true
		case navitool.ToolCallRecoveryDispositionUnloadedTool:
			if result.LoadAllowed {
				hasLoadableUnloaded = true
			}
		}
	}
	switch {
	case hasUnknown:
		return "the requested action referenced tools that are not available in this runtime"
	case hasLoadableUnloaded:
		return "the requested action referenced tools that are not loaded in the current active tool surface"
	default:
		return "the requested action referenced tools outside the current active tool surface"
	}
}

func recordUnauthorizedToolRecoveries(envelope *DecisionEnvelope, recoveries []navitool.ToolCallRecoveryResult) {
	if envelope == nil || len(recoveries) == 0 {
		return
	}
	if envelope.Rationale.Extensions == nil {
		envelope.Rationale.Extensions = map[string]any{}
	}
	envelope.Rationale.Extensions["tool_call_recovery"] = append([]navitool.ToolCallRecoveryResult(nil), recoveries...)
}
