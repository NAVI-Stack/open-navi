package inference

import (
	"context"
	"fmt"
	"strings"

	"github.com/open-navi/navi/internal/governor"
)

// AuthorizeToolCall routes one concrete tool attempt through the authoritative ICS seam.
func (c *Controller) AuthorizeToolCall(ctx context.Context, prior DecisionEnvelope, input ToolAuthorizationInput) (DecisionEnvelope, error) {
	if err := ctx.Err(); err != nil {
		return DecisionEnvelope{}, err
	}
	verifiedProposalID := ""
	if strings.TrimSpace(input.ApprovedProposalID) != "" {
		verifiedProposal, reason, err := c.verifyApprovedProposalForToolAttempt(ctx, prior, input)
		if err != nil {
			return DecisionEnvelope{}, err
		}
		if reason != "" {
			return authorizationBlockedEnvelope(prior, governor.ValidationResult{
				Outcome: governor.ValidationRejected,
				Reason:  reason,
			}, reason), nil
		}
		verifiedProposalID = strings.TrimSpace(verifiedProposal.ProposalID)
		input.VerifiedProposalID = verifiedProposalID
	}
	permit := lookupToolPermit(prior, strings.TrimSpace(input.ToolCallID), strings.TrimSpace(input.ToolName), verifiedProposalID)
	if permit == nil {
		reason := fmt.Sprintf("ICS did not pre-authorize tool call %q for execution", strings.TrimSpace(input.ToolName))
		return authorizationBlockedEnvelope(prior, governor.ValidationResult{
			Outcome: governor.ValidationRejected,
			Reason:  reason,
		}, reason), nil
	}
	envelope := prior
	envelope.RuntimeDisposition = RuntimeDispositionProceedDirect
	envelope.ToolPermit = permit
	envelope.ExecutionBoundary.FailClosedReason = ""
	envelope.ReplyMessage = ""
	envelope.Proposal = nil
	envelope.PendingToolCall = nil
	envelope.PendingToolPermit = nil
	return envelope, nil
}

func lookupToolPermit(envelope DecisionEnvelope, toolCallID, toolName, approvedProposalID string) *ToolPermit {
	toolCallID = strings.TrimSpace(toolCallID)
	toolName = strings.TrimSpace(toolName)
	approvedProposalID = strings.TrimSpace(approvedProposalID)
	for _, invocation := range envelope.AuthorizedTools {
		permit := invocation.Permit
		if permitMatchesAttempt(&permit, toolCallID, toolName, approvedProposalID) {
			copy := permit
			return &copy
		}
	}
	if envelope.ToolPermit != nil && permitMatchesAttempt(envelope.ToolPermit, toolCallID, toolName, approvedProposalID) {
		copy := *envelope.ToolPermit
		return &copy
	}
	return nil
}

func permitMatchesAttempt(permit *ToolPermit, toolCallID, toolName, approvedProposalID string) bool {
	if permit == nil {
		return false
	}
	if expectedToolCallID := strings.TrimSpace(permit.ToolCallID); expectedToolCallID != "" && expectedToolCallID != toolCallID {
		return false
	}
	if strings.TrimSpace(permit.ToolName) != toolName {
		return false
	}
	expectedProposalID := strings.TrimSpace(permit.ApprovedProposalID)
	if expectedProposalID == "" {
		return true
	}
	return expectedProposalID == approvedProposalID
}
