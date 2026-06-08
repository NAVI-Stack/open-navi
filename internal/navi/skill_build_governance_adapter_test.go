package navi

import (
	"context"
	"testing"

	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/navi/inference"
	"github.com/open-navi/navi/internal/navi/skill"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

type skillBuildLLM struct{ content string }

func (s *skillBuildLLM) Chat(ctx context.Context, model string, messages []llm.Message, tools []llm.ToolDefinition, opts llm.Options) (*llm.Response, error) {
	return &llm.Response{Content: s.content}, nil
}
func (s *skillBuildLLM) Name() string { return "skill-build-llm" }

type skillBuildICSStub struct {
	decideCalls   int
	validateCalls int
	envelope      inference.DecisionEnvelope
}

func (s *skillBuildICSStub) Decide(ctx context.Context, input inference.InferenceInput) (inference.DecisionSynthesis, error) {
	s.decideCalls++
	return inference.NewController().Decide(ctx, input)
}

func (s *skillBuildICSStub) ValidateGovernedDecision(ctx context.Context, input inference.InferenceInput, synthesis inference.DecisionSynthesis) (inference.DecisionEnvelope, error) {
	s.validateCalls++
	if s.envelope.RuntimeDisposition == "" {
		s.envelope.RuntimeDisposition = inference.RuntimeDispositionProceedDirect
	}
	return s.envelope, nil
}

func (s *skillBuildICSStub) ObserveOutcome(ctx context.Context, prior inference.DecisionEnvelope, snapshot inference.ExecutionSnapshot) (inference.DecisionEnvelope, error) {
	return prior, nil
}
func (s *skillBuildICSStub) PrepareModelCall(ctx context.Context, prior inference.DecisionEnvelope, input inference.ModelCallPreparationInput) (inference.DecisionEnvelope, error) {
	return prior, nil
}
func (s *skillBuildICSStub) AuthorizeModelResponse(ctx context.Context, prior inference.DecisionEnvelope, input inference.ModelResponseAuthorizationInput) (inference.DecisionEnvelope, error) {
	return prior, nil
}
func (s *skillBuildICSStub) AuthorizeToolCall(ctx context.Context, prior inference.DecisionEnvelope, input inference.ToolAuthorizationInput) (inference.DecisionEnvelope, error) {
	return prior, nil
}

func TestBuildGapAndBuildSkillUseICSGovernanceSeam(t *testing.T) {
	ctx := context.Background()
	db := store.InitTestDB(t)
	workspaceDir := t.TempDir()
	registry := skill.NewRegistry(workspaceDir)
	if err := registry.Load(); err != nil {
		t.Fatalf("registry.Load: %v", err)
	}

	gap1 := store.Gap{ID: "gap-ics-build-gap", Type: store.GapTypeMissingSkill, Status: store.GapStatusClassified, ClassificationResult: store.GapClassification{GapClass: store.GapTypeMissingSkill, ExpansionPath: "skill", Reason: "missing"}}
	gap2 := store.Gap{ID: "gap-ics-build-skill", Type: store.GapTypeMissingSkill, Status: store.GapStatusClassified, ClassificationResult: store.GapClassification{GapClass: store.GapTypeMissingSkill, ExpansionPath: "skill", Reason: "missing"}}
	if err := store.InsertGap(ctx, db, gap1); err != nil {
		t.Fatalf("InsertGap gap1: %v", err)
	}
	if err := store.InsertGap(ctx, db, gap2); err != nil {
		t.Fatalf("InsertGap gap2: %v", err)
	}

	ics := &skillBuildICSStub{envelope: inference.DecisionEnvelope{RuntimeDisposition: inference.RuntimeDispositionProceedDirect}}
	n := &NAVI{cfg: Config{DB: db}, loop: &AgentLoop{cfg: LoopConfig{InferenceController: ics}}}
	n.skillBuilder = skill.NewSkillBuilder(
		registry,
		&skillBuildLLM{content: skillBuildSpecJSON("generated-ics-gap")},
		"test-model",
		workspaceDir,
		db,
		newSkillBuildGovernanceAdapter(n).Govern,
		nil,
	)

	if _, err := n.BuildGap(ctx, gap1.ID, "build via gap", "sess-ics-gap"); err != nil {
		t.Fatalf("BuildGap: %v", err)
	}
	n.skillBuilder = skill.NewSkillBuilder(
		registry,
		&skillBuildLLM{content: skillBuildSpecJSON("generated-ics-skill")},
		"test-model",
		workspaceDir,
		db,
		newSkillBuildGovernanceAdapter(n).Govern,
		nil,
	)
	if _, err := n.BuildSkill(ctx, gap2.ID, "build via skill", "sess-ics-skill"); err != nil {
		t.Fatalf("BuildSkill: %v", err)
	}
	if ics.decideCalls < 2 || ics.validateCalls < 2 {
		t.Fatalf("expected ICS Decide/Validate to be called for both paths, got decide=%d validate=%d", ics.decideCalls, ics.validateCalls)
	}
}

func TestSkillBuilderWiringHasNoDirectProposalSavePath(t *testing.T) {
	ctx := context.Background()
	db := store.InitTestDB(t)
	workspaceDir := t.TempDir()
	registry := skill.NewRegistry(workspaceDir)
	if err := registry.Load(); err != nil {
		t.Fatalf("registry.Load: %v", err)
	}
	gap := store.Gap{ID: "gap-ics-pending", Type: store.GapTypeMissingSkill, Status: store.GapStatusClassified, ClassificationResult: store.GapClassification{GapClass: store.GapTypeMissingSkill, ExpansionPath: "skill", Reason: "missing"}}
	if err := store.InsertGap(ctx, db, gap); err != nil {
		t.Fatalf("InsertGap: %v", err)
	}

	ics := &skillBuildICSStub{envelope: inference.DecisionEnvelope{
		RuntimeDisposition: inference.RuntimeDispositionPauseForProposal,
		ReplyMessage:       "awaiting owner approval",
		Proposal: &schema.Proposal{
			ProposalID: "proposal-ics-pending",
			Status:     schema.ProposalStatusPending,
		},
	}}
	n := &NAVI{cfg: Config{DB: db}, loop: &AgentLoop{cfg: LoopConfig{InferenceController: ics}}}
	n.skillBuilder = skill.NewSkillBuilder(
		registry,
		&skillBuildLLM{content: skillBuildSpecJSON("generated-pending")},
		"test-model",
		workspaceDir,
		db,
		newSkillBuildGovernanceAdapter(n).Govern,
		nil,
	)

	result, err := n.BuildGap(ctx, gap.ID, "build pending", "sess-pending")
	if err != nil {
		t.Fatalf("BuildGap: %v", err)
	}
	if result == nil || !result.GovernedPause {
		t.Fatalf("expected governed pause from ICS response, got %+v", result)
	}
	if result.GovernedProposalID != "proposal-ics-pending" || result.GovernedProposalStatus != "pending" {
		t.Fatalf("expected proposal metadata from ICS, got %+v", result)
	}
}

func skillBuildSpecJSON(skillID string) string {
	return "{" +
		`"skill_yaml":"oss27_version: \"1.0\"\nskill_id: ` + skillID + `\nsemver: \"0.1.0\"\ndisplay:\n  name: Generated\n  description: generated\ninterfaces:\n  - name: run\n    transport:\n      type: internal\n    input_schema:\n      type: object\n      additionalProperties: false\n      properties: {}\n    output_schema:\n      type: object\n      additionalProperties: true\neffects:\n  side_effects: []\n  risk_tier: low\n  idempotency: true\n  idempotency_level: idempotent\n  reversibility: reversible_internal\n  requires_confirmation: false\nsecurity:\n  auth: []\n  data_access:\n    pii: none\n    secrets: forbidden\n  sandbox:\n    required: false\n    network_egress: []\nperformance:\n  expected_p50_ms: 10\n  timeout_ms: 1000\n  rate_limit:\n    qps: 1\n    burst: 1\nobservability:\n  log_redaction: []\n  emit_metrics: []\ngovernance:\n  publisher: navi.test\n  signed: false\n  deprecates: []\n  replaces: []\n  trust_tier: local\ncapability:\n  tags: [generated]\n  domains: [automation]\n  provides: [` + skillID + `]\n  requires: []\n  command_type: invoke\nreliability:\n  expected_failure_modes: []\n  retry_policy: none\n","main_py":"","requirements_txt":""` +
		"}"
}
