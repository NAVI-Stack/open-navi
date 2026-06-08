package inference

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/open-navi/navi/internal/governor"
	"github.com/open-navi/navi/internal/navi/proposals"
	"github.com/open-navi/navi/internal/navi/skill"
	"github.com/open-navi/navi/internal/schema"
)

// GovernResult is the deterministic outcome of Validate/Govern for a selected execution candidate.
type GovernResult struct {
	Outcome              governor.ValidationOutcome
	Reason               string
	ProposalID           string
	Proposal             *schema.Proposal
	AuthorizedCapability string
	ConstraintContext    *governor.ValidationResult
}

// ValidationDependency models external inputs needed for deterministic validation.
type ValidationDependency interface {
	ValidateHandoff(ctx context.Context, chatID string, handoff GovernanceHandoff, skillEntry *skill.SkillEntry) governor.ValidationResult
	CheckWorkspaceAction(ctx context.Context, chatID, targetPath string, action schema.WorkspaceActionType) governor.ValidationResult
	SaveProposal(ctx context.Context, draft proposals.Draft) (*schema.Proposal, error)
}

// ValidateCandidate tests a selected execution contract against deterministic validation order and returns a strict govern result.
func ValidateCandidate(ctx context.Context, input InferenceInput, rationale Rationale, targetCapability string, dep ValidationDependency) (GovernResult, error) {
	handoff := rationale.Governance

	// Apply targeted handoff if specific capability details exist.
	for _, detail := range handoff.AllowedCapabilityDetails {
		if strings.TrimSpace(detail.Name) == targetCapability {
			handoff.TargetCapability = detail.Name
			handoff.AllowedCapabilities = []string{detail.Name}
			handoff.AllowedCapabilityDetails = []CapabilityAvailability{detail}
			if handoff.CommandType == "" {
				handoff.CommandType = detail.CommandType
			}
			if handoff.Domain == "" {
				handoff.Domain = detail.Kind
			}
			break
		}
	}

	handoff.ValidationOrder = []string{
		"permissions",
		"policy",
		"configuration",
		"priority_alignment",
		"risk_assessment",
	}
	validation := dep.ValidateHandoff(ctx, input.Chat.ChatID, handoff, nil)

	switch validation.Outcome {
	case governor.ValidationRejected:
		return GovernResult{
			Outcome:           governor.ValidationRejected,
			Reason:            validation.Reason,
			ConstraintContext: &validation,
		}, nil

	case governor.ValidationRequiresConfirmation, governor.ValidationModified:
		proposal, err := dep.SaveProposal(ctx, proposals.Draft{
			SourceProcess: "ics",
			SourceTrigger: validation.Reason,
			BoundaryKey:   candidateBoundaryKey(input.Chat.ChatID, targetCapability, handoff),
			Payload: map[string]any{
				"type":                   "ics_governance_boundary",
				"target_capability":      targetCapability,
				"ics_governance_handoff": handoff,
				"ics_execution_intent":   rationale.ExecutionIntent,
			},
			AffectedEntities: []string{"chat:" + input.Chat.ChatID},
			Rationale:        validation.Reason,
			Priority:         schema.ProposalPriorityBlocking,
			Status:           schema.ProposalStatusPending,
			TTL:              24 * time.Hour,
		})
		if err != nil {
			rejected := governor.ValidationResult{
				Outcome: governor.ValidationRejected,
				Reason:  fmt.Sprintf("%s (ICS could not persist the required approval proposal: %v)", validation.Reason, err),
			}
			return GovernResult{
				Outcome:           governor.ValidationRejected,
				Reason:            rejected.Reason,
				ConstraintContext: &rejected,
			}, nil
		}

		result := GovernResult{
			Outcome:           validation.Outcome,
			Reason:            validation.Reason,
			ConstraintContext: &validation,
			Proposal:          proposal,
		}
		if proposal != nil {
			result.ProposalID = proposal.ProposalID
		}
		return result, nil

	default:
		return GovernResult{
			Outcome:              governor.ValidationApproved,
			Reason:               validation.Reason,
			AuthorizedCapability: targetCapability,
			ConstraintContext:    &validation,
		}, nil
	}
}

// ValidateToolAttempt tests a specific runtime tool invocation against governance rules.
func ValidateToolAttempt(ctx context.Context, input ToolAuthorizationInput, prior DecisionEnvelope, dep ValidationDependency) (GovernResult, error) {
	toolName := strings.TrimSpace(input.ToolName)
	if toolName == "" {
		validation := governor.ValidationResult{
			Outcome: governor.ValidationRejected,
			Reason:  "ICS rejected an empty tool call",
		}
		return GovernResult{
			Outcome:           governor.ValidationRejected,
			Reason:            validation.Reason,
			ConstraintContext: &validation,
		}, nil
	}

	// Boundary check
	allowed := false
	if target := strings.TrimSpace(prior.ExecutionBoundary.TargetCapability); target == toolName {
		allowed = true
	} else {
		for _, allow := range prior.ExecutionBoundary.AllowedCapabilities {
			if strings.TrimSpace(allow) == toolName {
				allowed = true
				break
			}
		}
	}

	if !allowed {
		validation := governor.ValidationResult{
			Outcome: governor.ValidationRejected,
			Reason:  fmt.Sprintf("ICS rejected tool call %q because it is outside the current authorized tool surface", toolName),
		}
		return GovernResult{
			Outcome:           governor.ValidationRejected,
			Reason:            validation.Reason,
			ConstraintContext: &validation,
		}, nil
	}

	handoff, skillEntry, requiresConfirmation := buildToolGovernanceContext(prior, input)

	handoff.ValidationOrder = []string{
		"permissions",
		"policy",
		"configuration",
		"priority_alignment",
		"risk_assessment",
	}
	validation := dep.ValidateHandoff(ctx, input.Chat.ChatID, handoff, skillEntry)

	contract := input.Contract
	targetPath := strings.TrimSpace(input.ResolvedTargetPath)
	workspaceAction := schema.WorkspaceActionType("")
	if contract != nil {
		workspaceAction = contract.WorkspaceAction
	}
	if validation.Outcome == governor.ValidationApproved && strings.TrimSpace(targetPath) != "" {
		res := dep.CheckWorkspaceAction(ctx, input.Chat.ChatID, targetPath, workspaceAction)
		validation = governor.ValidationResult{Outcome: res.Outcome, Reason: res.Reason}
	}

	if validation.Outcome == governor.ValidationApproved && requiresConfirmation && strings.TrimSpace(input.VerifiedProposalID) == "" {
		validation = governor.ValidationResult{
			Outcome: governor.ValidationRequiresConfirmation,
			Reason:  fmt.Sprintf("Tool %q requires explicit approval", toolName),
		}
	}

	switch validation.Outcome {
	case governor.ValidationApproved:
		return GovernResult{
			Outcome:              governor.ValidationApproved,
			Reason:               validation.Reason,
			AuthorizedCapability: toolName,
			ConstraintContext:    &validation,
		}, nil
	case governor.ValidationModified, governor.ValidationRequiresConfirmation:
		governancePayload := handoff
		intentPayload := prior.Rationale.ExecutionIntent

		payload := map[string]any{
			"type":                   "ics_runtime_tool_authorization",
			"tool_call_id":           input.ToolCallID,
			"tool_name":              toolName,
			"arguments":              input.Arguments,
			"chat_id":                input.Chat.ChatID,
			"run_id":                 input.Runtime.RunID,
			"ics_governance_handoff": governancePayload,
			"ics_execution_intent":   intentPayload,
		}
		if strings.TrimSpace(targetPath) != "" {
			payload["target_path"] = targetPath
			payload["workspace_action"] = workspaceAction
		}

		proposal, err := dep.SaveProposal(ctx, proposals.Draft{
			SourceProcess:    "ics",
			SourceTrigger:    validation.Reason,
			BoundaryKey:      toolAttemptBoundaryKey(input.Chat.ChatID, toolName, handoff, targetPath),
			Payload:          payload,
			AffectedEntities: []string{"chat:" + input.Chat.ChatID},
			Rationale:        validation.Reason,
			Priority:         schema.ProposalPriorityBlocking,
			Status:           schema.ProposalStatusPending,
			TTL:              24 * time.Hour,
		})
		if err != nil {
			rejected := governor.ValidationResult{
				Outcome: governor.ValidationRejected,
				Reason:  fmt.Sprintf("%s (ICS could not persist the required approval proposal: %v)", validation.Reason, err),
			}
			return GovernResult{
				Outcome:           governor.ValidationRejected,
				Reason:            rejected.Reason,
				ConstraintContext: &rejected,
			}, nil
		}

		result := GovernResult{
			Outcome:           validation.Outcome,
			Reason:            validation.Reason,
			ConstraintContext: &validation,
			Proposal:          proposal,
		}
		if proposal != nil {
			result.ProposalID = proposal.ProposalID
		}
		return result, nil
	default:
		return GovernResult{
			Outcome:           governor.ValidationRejected,
			Reason:            validation.Reason,
			ConstraintContext: &validation,
		}, nil
	}
}

func candidateBoundaryKey(chatID, targetCapability string, handoff GovernanceHandoff) string {
	return strings.Join([]string{
		"candidate",
		strings.TrimSpace(chatID),
		strings.TrimSpace(targetCapability),
		strings.TrimSpace(string(handoff.CommandType)),
		strings.TrimSpace(handoff.Domain),
		strings.TrimSpace(handoff.Scope),
		strings.TrimSpace(string(handoff.ApprovalRequirement)),
	}, "|")
}

func toolAttemptBoundaryKey(chatID, toolName string, handoff GovernanceHandoff, targetPath string) string {
	return strings.Join([]string{
		"tool_attempt",
		strings.TrimSpace(chatID),
		strings.TrimSpace(toolName),
		strings.TrimSpace(string(handoff.CommandType)),
		strings.TrimSpace(handoff.Domain),
		strings.TrimSpace(targetPath),
		strings.TrimSpace(string(handoff.ApprovalRequirement)),
	}, "|")
}

func buildToolGovernanceContext(prior DecisionEnvelope, input ToolAuthorizationInput) (GovernanceHandoff, *skill.SkillEntry, bool) {
	toolName := strings.TrimSpace(input.ToolName)
	handoff := prior.Rationale.Governance

	// Apply targeted handoff if specific capability details exist.
	for _, detail := range handoff.AllowedCapabilityDetails {
		if strings.TrimSpace(detail.Name) == toolName {
			handoff.TargetCapability = detail.Name
			handoff.AllowedCapabilities = []string{detail.Name}
			handoff.AllowedCapabilityDetails = []CapabilityAvailability{detail}
			if handoff.CommandType == "" {
				handoff.CommandType = detail.CommandType
			}
			if handoff.Domain == "" {
				handoff.Domain = detail.Kind
			}
			break
		}
	}

	if strings.TrimSpace(handoff.TargetCapability) == "" {
		handoff.TargetCapability = toolName
		handoff.AllowedCapabilities = []string{toolName}
	}

	contract := input.Contract
	if handoff.CommandType == "" && contract != nil {
		handoff.CommandType = contract.CommandType
	}
	if handoff.Domain == "" && contract != nil {
		handoff.Domain = contract.Domain
	}
	if handoff.ActorKind == "" && contract != nil {
		handoff.ActorKind = contract.ActorKind
	}
	if handoff.ActorKind == "" {
		handoff.ActorKind = "navi"
	}

	var skillEntry *skill.SkillEntry
	if contract != nil && strings.TrimSpace(contract.SkillName) != "" {
		skillEntry = &skill.SkillEntry{Skill: skill.Skill{Name: contract.SkillName}}
	}

	requiresConfirmation := input.RequiresConfirmation
	if contract != nil && !requiresConfirmation {
		requiresConfirmation = contract.RequiresConfirmation
	}

	return handoff, skillEntry, requiresConfirmation
}
