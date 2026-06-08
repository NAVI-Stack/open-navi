package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

type runtimeToolAuthorizationProposalPayload struct {
	Type       string `json:"type"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`
	ChatID     string `json:"chat_id,omitempty"`
	RunID      string `json:"run_id,omitempty"`
	TargetPath string `json:"target_path,omitempty"`
}

func (c *Controller) verifyApprovedProposalForToolAttempt(ctx context.Context, prior DecisionEnvelope, attempt ToolAuthorizationInput) (*schema.Proposal, string, error) {
	proposalID := strings.TrimSpace(attempt.ApprovedProposalID)
	if proposalID == "" {
		return nil, "", nil
	}
	if c.deps.GetProposal == nil {
		return nil, "ICS could not verify the approved proposal state required for replayed tool execution", nil
	}

	proposal, err := c.deps.GetProposal(ctx, proposalID)
	if err != nil {
		return nil, fmt.Sprintf("ICS could not load approved proposal %q for replayed tool execution", proposalID), nil
	}
	if strings.TrimSpace(proposal.ProposalID) == "" {
		return nil, fmt.Sprintf("ICS could not verify approved proposal %q for replayed tool execution", proposalID), nil
	}
	if proposal.Status != schema.ProposalStatusApproved {
		return nil, fmt.Sprintf("ICS rejected replayed tool execution because proposal %q is %s, not approved", proposalID, proposal.Status), nil
	}

	now := time.Now().UTC()
	if c.now != nil {
		now = c.now().UTC()
	}
	if proposal.ExpiresAt != nil && !proposal.ExpiresAt.IsZero() && !proposal.ExpiresAt.After(now) {
		return nil, fmt.Sprintf("ICS rejected replayed tool execution because proposal %q is stale", proposalID), nil
	}

	handoff, _, _ := buildToolGovernanceContext(prior, attempt)
	expectedBoundary := toolAttemptBoundaryKey(attempt.Chat.ChatID, attempt.ToolName, handoff, attempt.ResolvedTargetPath)
	if expectedBoundary == "" || strings.TrimSpace(proposal.BoundaryKey) == "" || strings.TrimSpace(proposal.BoundaryKey) != expectedBoundary {
		return nil, fmt.Sprintf("ICS rejected replayed tool execution because proposal %q does not match the pending tool boundary", proposalID), nil
	}

	var payload runtimeToolAuthorizationProposalPayload
	if err := json.Unmarshal([]byte(proposal.ProposedAction), &payload); err != nil {
		return nil, fmt.Sprintf("ICS rejected replayed tool execution because proposal %q does not carry verifiable authorization metadata", proposalID), nil
	}
	if strings.TrimSpace(payload.Type) != "ics_runtime_tool_authorization" {
		return nil, fmt.Sprintf("ICS rejected replayed tool execution because proposal %q is not a tool-authorization proposal", proposalID), nil
	}
	if strings.TrimSpace(payload.ToolCallID) == "" || strings.TrimSpace(payload.ToolCallID) != strings.TrimSpace(attempt.ToolCallID) {
		return nil, fmt.Sprintf("ICS rejected replayed tool execution because proposal %q does not match the pending tool call", proposalID), nil
	}
	if strings.TrimSpace(payload.ToolName) != strings.TrimSpace(attempt.ToolName) {
		return nil, fmt.Sprintf("ICS rejected replayed tool execution because proposal %q does not match tool %q", proposalID, strings.TrimSpace(attempt.ToolName)), nil
	}
	if strings.TrimSpace(payload.ChatID) != strings.TrimSpace(attempt.Chat.ChatID) {
		return nil, fmt.Sprintf("ICS rejected replayed tool execution because proposal %q does not belong to this chat", proposalID), nil
	}
	if strings.TrimSpace(payload.RunID) != strings.TrimSpace(attempt.Runtime.RunID) {
		return nil, fmt.Sprintf("ICS rejected replayed tool execution because proposal %q does not belong to this run", proposalID), nil
	}
	if strings.TrimSpace(payload.TargetPath) != strings.TrimSpace(attempt.ResolvedTargetPath) {
		return nil, fmt.Sprintf("ICS rejected replayed tool execution because proposal %q does not match the pending target path", proposalID), nil
	}

	return &proposal, "", nil
}
