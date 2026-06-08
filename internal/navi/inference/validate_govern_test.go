package inference

import (
	"context"
	"errors"
	"testing"

	"github.com/ceoai/navi/internal/governor"
	"github.com/ceoai/navi/internal/navi/proposals"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/navi/skill"
)

type mockValidationDependency struct {
	handoffResult   governor.ValidationResult
	workspaceResult governor.ValidationResult
	saveProposalErr error
	savedProposal   *schema.Proposal

	lastHandoff GovernanceHandoff
	lastSkill   *skill.SkillEntry
}

func (m *mockValidationDependency) ValidateHandoff(ctx context.Context, runtimeSessionID string, handoff GovernanceHandoff, skillEntry *skill.SkillEntry) governor.ValidationResult {
	m.lastHandoff = handoff
	m.lastSkill = skillEntry
	return m.handoffResult
}

func (m *mockValidationDependency) CheckWorkspaceAction(ctx context.Context, runtimeSessionID, targetPath string, action schema.WorkspaceActionType) governor.ValidationResult {
	return m.workspaceResult
}

func (m *mockValidationDependency) SaveProposal(ctx context.Context, draft proposals.Draft) (*schema.Proposal, error) {
	if m.saveProposalErr != nil {
		return nil, m.saveProposalErr
	}
	p := m.savedProposal
	if p == nil {
		p = &schema.Proposal{ProposalID: "mock-prop-1"}
	}
	return p, nil
}

func TestValidateGovern_DeterministicValidationOrder(t *testing.T) {
	ctx := context.Background()
	dep := &mockValidationDependency{
		handoffResult: governor.ValidationResult{
			Outcome: governor.ValidationRequiresConfirmation,
			Reason:  "mock confirmation required",
		},
	}

	result, err := ValidateCandidate(ctx, InferenceInput{
		Chat: ChatContext{ChatID: "sess-1"},
	}, Rationale{}, "test_tool", dep)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != governor.ValidationRequiresConfirmation {
		t.Fatalf("expected requires confirmation, got %v", result.Outcome)
	}

	if len(dep.lastHandoff.ValidationOrder) != 5 {
		t.Fatalf("expected deterministic validation order to be explicitly attached by Validate/Govern module, got: %#v", dep.lastHandoff.ValidationOrder)
	}
	expected := []string{"permissions", "policy", "configuration", "priority_alignment", "risk_assessment"}
	for i, v := range expected {
		if dep.lastHandoff.ValidationOrder[i] != v {
			t.Fatalf("expected validation order %q at index %d, got %q", v, i, dep.lastHandoff.ValidationOrder[i])
		}
	}
}

func TestValidateToolAttempt_PassesSkillContext(t *testing.T) {
	ctx := context.Background()
	dep := &mockValidationDependency{}

	_, err := ValidateToolAttempt(ctx, ToolAuthorizationInput{
		ToolName: "allowed_tool",
		Contract: &ToolContract{
			ID:              "contract:allowed_tool",
			ToolName:        "allowed_tool",
			ExecutionKind:   ToolExecutionKindRegistry,
			CommandType:     schema.CommandTypeInvoke,
			Domain:          "testing",
			ActorKind:       "navi",
			WorkspaceAction: schema.WorkspaceActionExecute,
			SkillName:       "dummy_skill",
		},
	}, DecisionEnvelope{
		ExecutionBoundary: ExecutionBoundary{
			AllowedCapabilities: []string{"allowed_tool"},
		},
	}, dep)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if dep.lastSkill == nil {
		t.Fatalf("expected validate tool attempt to propagate the real SkillEntry down to governance validation")
	}
}

func TestRejectedAction_ReturnsConstraintContextToDecide(t *testing.T) {
	ctx := context.Background()

	dep := &mockValidationDependency{
		handoffResult: governor.ValidationResult{
			Outcome: governor.ValidationRejected,
			Reason:  "hard blocked",
		},
	}

	result, err := ValidateCandidate(ctx, InferenceInput{}, Rationale{}, "test_tool", dep)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != governor.ValidationRejected {
		t.Fatalf("expected rejected outcome, got %v", result.Outcome)
	}
	if result.ConstraintContext == nil || result.ConstraintContext.Reason != "hard blocked" {
		t.Fatalf("expected constraint context reason 'hard blocked', got %+v", result.ConstraintContext)
	}
}

func TestValidateCandidate_ProposalSaveFailureReturnsRejected(t *testing.T) {
	ctx := context.Background()

	dep := &mockValidationDependency{
		handoffResult: governor.ValidationResult{
			Outcome: governor.ValidationRequiresConfirmation,
			Reason:  "needs confirmation",
		},
		saveProposalErr: errors.New("db error"),
	}

	result, err := ValidateCandidate(ctx, InferenceInput{}, Rationale{}, "test_tool", dep)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != governor.ValidationRejected {
		t.Fatalf("expected rejected outcome when save fails, got %v", result.Outcome)
	}
	if result.ProposalID != "" {
		t.Fatalf("expected empty proposal ID, got %q", result.ProposalID)
	}
}

func TestValidateToolAttempt_OutOfBoundsRejection(t *testing.T) {
	ctx := context.Background()
	dep := &mockValidationDependency{}

	result, err := ValidateToolAttempt(ctx, ToolAuthorizationInput{
		ToolName: "disallowed_tool",
	}, DecisionEnvelope{
		ExecutionBoundary: ExecutionBoundary{
			AllowedCapabilities: []string{"allowed_tool"},
		},
	}, dep)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != governor.ValidationRejected {
		t.Fatalf("expected rejected outcome for out of bounds tool, got %v", result.Outcome)
	}
}
