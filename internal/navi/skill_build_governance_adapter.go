package navi

import (
	"context"
	"strings"

	"github.com/open-navi/navi/internal/governor"
	"github.com/open-navi/navi/internal/navi/inference"
	"github.com/open-navi/navi/internal/navi/orchestration"
	"github.com/open-navi/navi/internal/navi/skill"
	"github.com/open-navi/navi/internal/schema"
)

// skillBuildGovernanceAdapter routes skill/connector build governance through the
// canonical ICS Decide -> ValidateGovernedDecision seam.
type skillBuildGovernanceAdapter struct {
	n *NAVI
}

func newSkillBuildGovernanceAdapter(n *NAVI) *skillBuildGovernanceAdapter {
	return &skillBuildGovernanceAdapter{n: n}
}

func (a *skillBuildGovernanceAdapter) Govern(ctx context.Context, req skill.GovernanceRequest) (skill.GovernanceResponse, error) {
	if a == nil || a.n == nil || a.n.loop == nil {
		return skill.GovernanceResponse{Decision: skill.GovernanceDecisionBlocked, Reason: "governance adapter is not configured"}, nil
	}

	controller := a.n.loop.inferenceController()
	if controller == nil {
		return skill.GovernanceResponse{Decision: skill.GovernanceDecisionBlocked, Reason: "inference controller is not configured"}, nil
	}

	input := a.buildInferenceInput(req)
	synthesis, err := controller.Decide(ctx, input)
	if err != nil {
		return skill.GovernanceResponse{}, err
	}
	envelope, err := controller.ValidateGovernedDecision(ctx, input, synthesis)
	if err != nil {
		return skill.GovernanceResponse{}, err
	}

	return mapSkillBuildGovernanceEnvelope(envelope), nil
}

func (a *skillBuildGovernanceAdapter) buildInferenceInput(req skill.GovernanceRequest) inference.InferenceInput {
	chatID := extractGovernanceSessionID(req)
	targetCapability := firstNonEmpty(strings.TrimSpace(req.Operation), strings.TrimSpace(req.Domain)+"."+strings.TrimSpace(req.CommandType), "skill_build")
	if strings.HasSuffix(targetCapability, ".") {
		targetCapability = "skill_build"
	}

	reason := strings.TrimSpace(firstNonEmpty(req.Rationale, req.ConfirmationRationale, "self-extension governance check"))
	approvalNeeded := req.RequireConfirmation
	if approvalNeeded && strings.TrimSpace(reason) == "" {
		reason = "owner confirmation required"
	}

	return inference.InferenceInput{
		Version: inference.ContractVersionV1,
		NCOS: orchestration.CanonicalRunRequest{
			UserMessage: reason,
			RequiredOutput: orchestration.RequiredOutput{
				AllowToolCalls: true,
			},
		},
		Chat: inference.ChatContext{
			ChatID:      chatID,
			UserMessage: reason,
		},
		Governance: inference.GovernanceState{
			ConfirmationRequired: approvalNeeded,
			BlockingReason:       strings.TrimSpace(req.ConfirmationRationale),
		},
		GoalStack: inference.GoalStack{
			ActiveGoalID: targetCapability,
			Ready: []inference.GoalRef{{
				GoalID:   targetCapability,
				Summary:  reason,
				Status:   inference.GoalStatusActive,
				Priority: 1,
			}},
		},
		Capabilities: []inference.CapabilityAvailability{{
			Name:        targetCapability,
			Kind:        strings.TrimSpace(req.Domain),
			Available:   true,
			Governed:    true,
			CommandType: schema.CommandType(req.CommandType),
			Reason:      reason,
		}},
		Extensions: map[string]any{
			"skill_build_governance": true,
			"source_process":         strings.TrimSpace(req.SourceProcess),
			"source_trigger":         strings.TrimSpace(req.SourceTrigger),
			"operation":              strings.TrimSpace(req.Operation),
		},
	}
}

func extractGovernanceSessionID(req skill.GovernanceRequest) string {
	if payload := req.Payload; payload != nil {
		if chatID, _ := payload["chat_id"].(string); strings.TrimSpace(chatID) != "" {
			return strings.TrimSpace(chatID)
		}
	}
	for _, entity := range req.AffectedEntities {
		entity = strings.TrimSpace(entity)
		if strings.HasPrefix(entity, "session:") {
			return strings.TrimSpace(strings.TrimPrefix(entity, "session:"))
		}
	}
	return ""
}

func mapSkillBuildGovernanceEnvelope(envelope inference.DecisionEnvelope) skill.GovernanceResponse {
	reason := strings.TrimSpace(firstNonEmpty(envelope.ReplyMessage, governanceValidationReason(envelope.GovernanceResult), envelope.Rationale.Governance.Intent))
	if reason == "" {
		reason = "blocked by policy"
	}

	switch envelope.RuntimeDisposition {
	case inference.RuntimeDispositionPauseForProposal:
		response := skill.GovernanceResponse{
			Decision: skill.GovernanceDecisionPending,
			Reason:   reason,
		}
		if envelope.Proposal != nil {
			response.ProposalID = envelope.Proposal.ProposalID
			response.ProposalStatus = string(envelope.Proposal.Status)
		}
		return response
	case inference.RuntimeDispositionBlockWithReply:
		return skill.GovernanceResponse{Decision: skill.GovernanceDecisionBlocked, Reason: reason}
	default:
		if envelope.GovernanceResult != nil && envelope.GovernanceResult.Outcome == governor.ValidationRejected {
			return skill.GovernanceResponse{Decision: skill.GovernanceDecisionBlocked, Reason: reason}
		}
		return skill.GovernanceResponse{Decision: skill.GovernanceDecisionApproved, Reason: strings.TrimSpace(governanceValidationReason(envelope.GovernanceResult))}
	}
}
