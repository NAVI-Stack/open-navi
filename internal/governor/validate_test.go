package governor

import (
	"context"
	"testing"

	"github.com/open-navi/navi/internal/navi/skill"
	"github.com/open-navi/navi/internal/schema"
)

func TestPipelineRunUsesDeterministicOrder(t *testing.T) {
	called := []string{}

	p := Pipeline{
		Permissions: func(a ActionDescriptor) (ValidationOutcome, string, string) {
			called = append(called, "permissions")
			return ValidationApproved, "", ""
		},
		Policy: func(a ActionDescriptor) (ValidationOutcome, string, string) {
			called = append(called, "policy")
			return ValidationRequiresConfirmation, "needs confirmation", ""
		},
		Configuration: func(a ActionDescriptor) (ValidationOutcome, string, string) {
			called = append(called, "configuration")
			return ValidationRejected, "config rejected", ""
		},
	}

	res := p.Run(ActionDescriptor{})

	// Policy runs before Configuration, so Policy must be authoritative and
	// Configuration must never run.
	if res.Outcome != ValidationRequiresConfirmation {
		t.Fatalf("expected outcome %v, got %v", ValidationRequiresConfirmation, res.Outcome)
	}
	if res.Reason != "needs confirmation" {
		t.Fatalf("expected reason %q, got %q", "needs confirmation", res.Reason)
	}
	if len(called) != 2 || called[0] != "permissions" || called[1] != "policy" {
		t.Fatalf("unexpected call order: %#v", called)
	}
	if res.Tier != GovernanceTierSystem {
		t.Fatalf("expected Tier System from authoritative policy result, got %q", res.Tier)
	}
}

func TestPipelineRunDoesNotAllowLaterStepToOverrideEarlierNonApproved(t *testing.T) {
	p := Pipeline{
		Permissions: func(a ActionDescriptor) (ValidationOutcome, string, string) {
			return ValidationRejected, "permissions denied", ""
		},
		Policy: func(a ActionDescriptor) (ValidationOutcome, string, string) {
			return ValidationRequiresConfirmation, "policy confirm", ""
		},
	}
	res := p.Run(ActionDescriptor{})
	// Permissions runs before Policy and is already non-approved, so it wins.
	if res.Outcome != ValidationRejected {
		t.Fatalf("expected Rejected from first non-approved step, got %v", res.Outcome)
	}
	if res.Reason != "permissions denied" {
		t.Fatalf("expected reason from permissions step, got %q", res.Reason)
	}
	if res.Tier != GovernanceTierSystem {
		t.Fatalf("expected Tier System, got %q", res.Tier)
	}
}

func TestPipelineRunPriorityAlignmentCannotOverrideConfiguration(t *testing.T) {
	called := []string{}
	p := Pipeline{
		Configuration: func(a ActionDescriptor) (ValidationOutcome, string, string) {
			called = append(called, "configuration")
			return ValidationRequiresConfirmation, "config confirm", ""
		},
		PriorityAlignment: func(a ActionDescriptor) (ValidationOutcome, string, string) {
			called = append(called, "priority_alignment")
			return ValidationRejected, "priority rejected", ""
		},
	}
	res := p.Run(ActionDescriptor{})
	// Configuration runs before PriorityAlignment and short-circuits.
	if res.Outcome != ValidationRequiresConfirmation {
		t.Fatalf("expected RequiresConfirmation from configuration, got %v", res.Outcome)
	}
	if res.Reason != "config confirm" {
		t.Fatalf("expected reason from configuration step, got %q", res.Reason)
	}
	if res.Tier != GovernanceTierOwner {
		t.Fatalf("expected Tier Owner, got %q", res.Tier)
	}
	if len(called) != 1 || called[0] != "configuration" {
		t.Fatalf("expected short-circuit before priority_alignment, got calls %#v", called)
	}
}

func TestPipelineRunRiskAssessmentCannotOverrideEarlierOwnerDecision(t *testing.T) {
	called := []string{}
	p := Pipeline{
		Permissions: func(a ActionDescriptor) (ValidationOutcome, string, string) {
			called = append(called, "permissions")
			return ValidationApproved, "", ""
		},
		Policy: func(a ActionDescriptor) (ValidationOutcome, string, string) {
			called = append(called, "policy")
			return ValidationApproved, "", ""
		},
		Configuration: func(a ActionDescriptor) (ValidationOutcome, string, string) {
			called = append(called, "configuration")
			return ValidationModified, "owner modified", "{\"safe\":true}"
		},
		RiskAssessment: func(a ActionDescriptor) (ValidationOutcome, string, string) {
			called = append(called, "risk_assessment")
			return ValidationRejected, "risk rejected", ""
		},
	}
	res := p.Run(ActionDescriptor{})
	if res.Outcome != ValidationModified {
		t.Fatalf("expected ValidationModified from configuration, got %v", res.Outcome)
	}
	if res.Reason != "owner modified" {
		t.Fatalf("expected reason from configuration, got %q", res.Reason)
	}
	if res.ModifiedAction != "{\"safe\":true}" {
		t.Fatalf("expected modified action from configuration, got %q", res.ModifiedAction)
	}
	if res.Tier != GovernanceTierOwner {
		t.Fatalf("expected Tier Owner, got %q", res.Tier)
	}
	if len(called) != 3 || called[0] != "permissions" || called[1] != "policy" || called[2] != "configuration" {
		t.Fatalf("expected short-circuit before risk_assessment, got calls %#v", called)
	}
}

func TestValidateActionDelegatesToPolicyEngine(t *testing.T) {
	entry := &skill.SkillEntry{
		Spec: &skill.OSS27Spec{
			Display: skill.DisplayMetadata{Name: "test-skill"},
			Effects: skill.EffectMetadata{RiskTier: "high"},
		},
	}
	pe := skill.NewPolicyEngine()
	action := ActionDescriptor{SkillEntry: entry}
	res := ValidateAction(pe, action)
	if res.Outcome != ValidationRequiresConfirmation {
		t.Fatalf("expected ValidationRequiresConfirmation, got %v", res.Outcome)
	}
	if res.Reason == "" {
		t.Fatalf("expected non-empty reason")
	}
}

func TestValidateSkillExecutionWrapperUsesValidateAction(t *testing.T) {
	// PolicyDeny is returned when Python skill requests network access without sandbox network egress.
	entry := &skill.SkillEntry{
		Spec: &skill.OSS27Spec{
			Display:       skill.DisplayMetadata{Name: "wrapper-skill"},
			PythonRuntime: &skill.PythonRuntimeSpec{NetworkAccess: true},
			Security:      skill.SecuritySpec{Sandbox: skill.SandboxSpec{NetworkEgress: []string{}}},
		},
	}
	pe := skill.NewPolicyEngine()
	res := ValidateSkillExecution(pe, entry)
	if res.Outcome != ValidationRejected {
		t.Fatalf("expected ValidationRejected, got %v", res.Outcome)
	}
	if res.Reason == "" {
		t.Fatalf("expected non-empty reason")
	}
}

func TestValidateActionLifecycleTombstoneRequiresConfirmation(t *testing.T) {
	act := ActionDescriptorForLifecycleTombstone("reflection", "memory", "mem-1")
	res := ValidateAction(nil, act)
	if res.Outcome != ValidationRequiresConfirmation {
		t.Fatalf("expected ValidationRequiresConfirmation for tombstone, got %v", res.Outcome)
	}
	if res.Reason != "Tombstone requires proposal approval" {
		t.Fatalf("unexpected reason: %q", res.Reason)
	}
}

func TestValidateActionLifecycleSupersedeOwnerSetRequiresConfirmation(t *testing.T) {
	act := ActionDescriptorForLifecycleSupersede("gateway", "fact-1", "fact-2", true)
	res := ValidateAction(nil, act)
	if res.Outcome != ValidationRequiresConfirmation {
		t.Fatalf("expected ValidationRequiresConfirmation for owner-set supersede, got %v", res.Outcome)
	}
	if res.Reason != "Owner-set knowledge supersede requires proposal approval" {
		t.Fatalf("unexpected reason: %q", res.Reason)
	}
}

func TestValidateActionLifecycleSupersedeNonOwnerSetApproved(t *testing.T) {
	act := ActionDescriptorForLifecycleSupersede("reflection", "fact-1", "fact-2", false)
	res := ValidateAction(nil, act)
	if res.Outcome != ValidationApproved {
		t.Fatalf("expected ValidationApproved for non-owner-set supersede, got %v", res.Outcome)
	}
}

func TestValidateActionLifecycleDeprecateOwnerSetRequiresConfirmation(t *testing.T) {
	act := ActionDescriptorForLifecycleDeprecate("gateway", "fact-1", true)
	res := ValidateAction(nil, act)
	if res.Outcome != ValidationRequiresConfirmation {
		t.Fatalf("expected ValidationRequiresConfirmation for owner-set deprecate, got %v", res.Outcome)
	}
	if res.Reason != "Owner-set knowledge deprecate requires proposal approval" {
		t.Fatalf("unexpected reason: %q", res.Reason)
	}
}

func TestValidateActionLifecycleDeprecateNonOwnerSetApproved(t *testing.T) {
	act := ActionDescriptorForLifecycleDeprecate("reflection", "fact-1", false)
	res := ValidateAction(nil, act)
	if res.Outcome != ValidationApproved {
		t.Fatalf("expected ValidationApproved for non-owner-set deprecate, got %v", res.Outcome)
	}
}

type fakeConfigPriorityReader struct {
	config     []schema.ConfigurationEntry
	priorities []schema.Priority
}

func (f *fakeConfigPriorityReader) ListConfiguration(ctx context.Context, ownerID string, limit int) ([]schema.ConfigurationEntry, error) {
	return f.config, nil
}

func (f *fakeConfigPriorityReader) ListPriorities(ctx context.Context, ownerID string, limit int) ([]schema.Priority, error) {
	return f.priorities, nil
}

func TestValidateActionWithOwner_ConfigurationActionDisabledRejected(t *testing.T) {
	reader := &fakeConfigPriorityReader{
		config: []schema.ConfigurationEntry{{Key: "action_disabled", Value: "true"}},
	}
	act := ActionDescriptor{OwnerID: "owner-1", Domain: "messaging"}
	res := ValidateActionWithOwner(context.Background(), nil, act, reader)
	if res.Outcome != ValidationRejected {
		t.Fatalf("expected ValidationRejected when action_disabled=true, got %v", res.Outcome)
	}
	if res.Reason != "Action disabled by owner configuration" {
		t.Fatalf("unexpected reason: %q", res.Reason)
	}
}

func TestValidateActionWithOwner_ConfigurationDomainDisabledRejected(t *testing.T) {
	reader := &fakeConfigPriorityReader{
		config: []schema.ConfigurationEntry{{Key: "domain_messaging_disabled", Value: "true"}},
	}
	act := ActionDescriptor{OwnerID: "owner-1", Domain: "messaging"}
	res := ValidateActionWithOwner(context.Background(), nil, act, reader)
	if res.Outcome != ValidationRejected {
		t.Fatalf("expected ValidationRejected when domain_messaging_disabled=true, got %v", res.Outcome)
	}
}

func TestValidateActionWithOwner_NoReaderUsesTagsOnly(t *testing.T) {
	act := ActionDescriptor{OwnerID: "owner-1", Tags: map[string]string{"owner_disabled": "true"}}
	res := ValidateActionWithOwner(context.Background(), nil, act, nil)
	if res.Outcome != ValidationRejected {
		t.Fatalf("expected ValidationRejected from tags when reader is nil, got %v", res.Outcome)
	}
}

func TestValidateActionWithOwner_WorkerTaskCodingDomainDisabledRejected(t *testing.T) {
	reader := &fakeConfigPriorityReader{
		config: []schema.ConfigurationEntry{{Key: "domain_coding_disabled", Value: "true"}},
	}
	act := ActionDescriptor{
		CommandType: schema.CommandTypeDelegate,
		Domain:      AutonomyDomainCoding,
		ActorKind:   "coder-worker",
		OwnerID:     "owner-1",
	}
	res := ValidateActionWithOwner(context.Background(), nil, act, reader)
	if res.Outcome != ValidationRejected {
		t.Fatalf("expected ValidationRejected for worker task when domain_coding_disabled=true, got %v", res.Outcome)
	}
}

func TestValidateActionWithOwner_WorkerTaskCodingApprovedWhenNotDisabled(t *testing.T) {
	reader := &fakeConfigPriorityReader{}
	act := ActionDescriptor{
		CommandType: schema.CommandTypeDelegate,
		Domain:      AutonomyDomainCoding,
		ActorKind:   "coder-worker",
		OwnerID:     "owner-1",
	}
	res := ValidateActionWithOwner(context.Background(), nil, act, reader)
	if res.Outcome != ValidationApproved {
		t.Fatalf("expected ValidationApproved for delegated coding worker task, got %v", res.Outcome)
	}
}

func TestValidateAction_CodingUpdateStillRequiresConfirmation(t *testing.T) {
	act := ActionDescriptor{
		CommandType: schema.CommandTypeUpdate,
		Domain:      AutonomyDomainCoding,
		ActorKind:   "gateway",
	}
	res := ValidateAction(nil, act)
	if res.Outcome != ValidationRequiresConfirmation {
		t.Fatalf("expected ValidationRequiresConfirmation for direct coding update, got %v", res.Outcome)
	}
	if res.Reason != "Codebase modification requires explicit approval" {
		t.Fatalf("unexpected reason: %q", res.Reason)
	}
}

func TestValidateAction_MessagingSendApprovedForOwner(t *testing.T) {
	act := ActionDescriptor{
		CommandType: schema.CommandTypeSend,
		Domain:      AutonomyDomainMessaging,
		Tags:        map[string]string{"recipient_status": "owner"},
	}
	res := ValidateAction(nil, act)
	if res.Outcome != ValidationApproved {
		t.Fatalf("expected ValidationApproved for owner messaging send, got %v", res.Outcome)
	}
}

func TestValidateAction_MessagingSendRequiresConfirmationForNonOwner(t *testing.T) {
	// Without the semantic tag, it should require confirmation
	act := ActionDescriptor{
		CommandType: schema.CommandTypeSend,
		Domain:      AutonomyDomainMessaging,
		ActorKind:   "gateway", // Actor-based bypass was removed
	}
	res := ValidateAction(nil, act)
	if res.Outcome != ValidationRequiresConfirmation {
		t.Fatalf("expected ValidationRequiresConfirmation for non-owner messaging send, got %v", res.Outcome)
	}
}

func TestValidateAction_IntegrationInvokeRequiresConfirmation(t *testing.T) {
	act := ActionDescriptor{
		CommandType: schema.CommandTypeInvoke,
		Domain:      "integration",
		ActorKind:   "navi",
	}
	res := ValidateAction(nil, act)
	if res.Outcome != ValidationRequiresConfirmation {
		t.Fatalf("expected ValidationRequiresConfirmation for integration invoke, got %v", res.Outcome)
	}
}

func TestValidateActionWithOwner_MemoryCreateRequiresConfirmation(t *testing.T) {
	act := ActionDescriptor{
		CommandType: schema.CommandTypeCreate,
		Domain:      "memory",
		ActorKind:   "navi",
		OwnerID:     "owner-1",
	}
	res := ValidateActionWithOwner(context.Background(), nil, act, &fakeConfigPriorityReader{})
	if res.Outcome != ValidationRequiresConfirmation {
		t.Fatalf("expected ValidationRequiresConfirmation for memory create, got %v", res.Outcome)
	}
}

func TestValidateAction_QueryRequiresConfirmationWhenSideEffectTagPresent(t *testing.T) {
	act := ActionDescriptor{
		CommandType: schema.CommandTypeQuery,
		Domain:      "messaging",
		ActorKind:   "navi",
		Tags: map[string]string{
			"side_effect_class": "external_irreversible",
		},
	}
	res := ValidateAction(nil, act)
	if res.Outcome != ValidationRequiresConfirmation {
		t.Fatalf("expected ValidationRequiresConfirmation for side-effecting query, got %v", res.Outcome)
	}
}
