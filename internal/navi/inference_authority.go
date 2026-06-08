package navi

import (
	"context"
	"strings"

	"github.com/ceoai/navi/internal/governor"
	"github.com/ceoai/navi/internal/navi/inference"
	"github.com/ceoai/navi/internal/navi/proposals"
	"github.com/ceoai/navi/internal/navi/skill"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

type inferenceControllerValidationDep struct {
	loop *AgentLoop
}

func (d *inferenceControllerValidationDep) ValidateHandoff(ctx context.Context, chatID string, handoff inference.GovernanceHandoff, skillEntry *skill.SkillEntry) governor.ValidationResult {
	return d.loop.validateGovernanceHandoff(ctx, chatID, handoff, skillEntry)
}

func (d *inferenceControllerValidationDep) CheckWorkspaceAction(ctx context.Context, chatID, targetPath string, action schema.WorkspaceActionType) governor.ValidationResult {
	res := d.loop.getWorkspaceEnforcer(ctx, chatID).CheckAction(ctx, targetPath, action)
	return governor.ValidationResult{Outcome: res.Outcome, Reason: res.Reason}
}

func (d *inferenceControllerValidationDep) SaveProposal(ctx context.Context, draft proposals.Draft) (*schema.Proposal, error) {
	return saveOrUpsertProposal(ctx, d.loop, draft)
}

func (l *AgentLoop) inferenceController() inference.DecisionController {
	if l == nil {
		return inference.NewController()
	}
	if l.cfg.InferenceController != nil {
		return l.cfg.InferenceController
	}
	return canonicalInferenceController(l)
}

func canonicalInferenceController(loop *AgentLoop) inference.DecisionController {
	if loop == nil {
		return inference.NewController()
	}
	return inference.NewControllerWithDeps(inference.ControllerDeps{
		ValidationDependency: &inferenceControllerValidationDep{loop: loop},
		GetProposal: func(ctx context.Context, proposalID string) (schema.Proposal, error) {
			return loadProposal(ctx, loop, proposalID)
		},
	})
}

func saveOrUpsertProposal(ctx context.Context, loop *AgentLoop, draft proposals.Draft) (*schema.Proposal, error) {
	if loop == nil {
		return nil, proposals.ErrPersistenceNotConfigured
	}
	findPending := func(ctx context.Context, boundaryKey string) (*schema.Proposal, error) {
		boundaryKey = strings.TrimSpace(boundaryKey)
		if boundaryKey == "" {
			return nil, nil
		}
		if loop.cfg.FindPendingProposalByBoundaryKey != nil {
			proposal, err := loop.cfg.FindPendingProposalByBoundaryKey(ctx, boundaryKey)
			if err != nil {
				return nil, err
			}
			if proposal.ProposalID != "" {
				return &proposal, nil
			}
			return nil, nil
		}
		if loop.cfg.DB == nil {
			return nil, nil
		}
		proposal, err := store.GetPendingProposalByBoundaryKey(ctx, loop.cfg.DB, boundaryKey)
		if err != nil {
			return nil, err
		}
		if proposal.ProposalID == "" {
			return nil, nil
		}
		return &proposal, nil
	}

	save := loop.cfg.SaveProposal
	if save == nil && loop.cfg.DB != nil {
		save = func(ctx context.Context, proposal schema.Proposal) error {
			return store.SaveProposal(ctx, loop.cfg.DB, proposal)
		}
	}
	return proposals.Upsert(ctx, save, findPending, draft)
}

func loadProposal(ctx context.Context, loop *AgentLoop, proposalID string) (schema.Proposal, error) {
	if loop == nil {
		return schema.Proposal{}, nil
	}
	if loop.cfg.GetProposal != nil {
		return loop.cfg.GetProposal(ctx, proposalID)
	}
	if loop.cfg.DB != nil {
		return store.GetProposal(ctx, loop.cfg.DB, proposalID)
	}
	return schema.Proposal{}, nil
}

func (l *AgentLoop) validateGovernanceHandoff(ctx context.Context, chatID string, handoff inference.GovernanceHandoff, skillEntry *skill.SkillEntry) governor.ValidationResult {
	action := handoff.ActionDescriptor(inference.ChatContext{ChatID: chatID})
	action.SkillEntry = skillEntry
	if l.cfg.ResolveOwnerID != nil {
		action.OwnerID = l.cfg.ResolveOwnerID(ctx, chatID)
	}
	return l.validateToolGovernanceAction(ctx, action, skillEntry)
}

func buildDecisionEnvelope(rationale inference.Rationale) inference.DecisionEnvelope {
	allowed := compactRuntimeStrings(append([]string(nil), rationale.ExecutionIntent.AuthorizedCapabilities()...))
	target := firstNonEmpty(strings.TrimSpace(rationale.ExecutionIntent.TargetCapability), strings.TrimSpace(rationale.Governance.TargetCapability))
	if target == "" && len(allowed) == 1 {
		target = allowed[0]
	}
	boundary := inference.ExecutionBoundary{
		AllowedCapabilities: allowed,
		TargetCapability:    target,
	}
	if target != "" && rationale.ExecutionIntent.AllowsCapabilityExecution() {
		boundary.ToolChoice = target
	}
	return inference.DecisionEnvelope{
		Rationale:          rationale,
		GovernanceResult:   rationale.Governance.ValidationResult,
		RuntimeDisposition: inference.RuntimeDispositionCallModel,
		ExecutionBoundary:  boundary,
	}
}
