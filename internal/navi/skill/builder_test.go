package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pkgconn "github.com/open-navi/navi/connectors"
	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
	"gopkg.in/yaml.v3"
)

type builderTestLLM struct {
	content string
}

func (b *builderTestLLM) Chat(ctx context.Context, model string, messages []llm.Message, tools []llm.ToolDefinition, opts llm.Options) (*llm.Response, error) {
	return &llm.Response{Content: b.content}, nil
}

func (b *builderTestLLM) Name() string { return "builder-test" }

func TestSkillBuilderBuildInstallsSkillAndClosesGap(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := store.InitTestDB(t)
	registry := NewRegistry(t.TempDir())
	if err := registry.Load(); err != nil {
		t.Fatalf("registry.Load: %v", err)
	}

	gap := store.Gap{
		ID:     "gap-build-1",
		Type:   store.GapTypeMissingSkill,
		Status: store.GapStatusClassified,
		Evidence: store.GapEvidence{
			ChatID:  "sess-1",
			ToolError:  "tool not found: generated_gap_closer_run",
			RawContext: "user asked to run a missing skill",
		},
		ClassificationResult: store.GapClassification{
			GapClass:      store.GapTypeMissingSkill,
			ExpansionPath: "skill",
			Reason:        "Missing generated gap closer skill",
		},
	}
	if err := store.InsertGap(ctx, db, gap); err != nil {
		t.Fatalf("InsertGap: %v", err)
	}

	builder := NewSkillBuilder(
		registry,
		&builderTestLLM{content: `{
  "skill_yaml": "oss27_version: \"1.0\"\nskill_id: generated-gap-closer\nsemver: \"0.1.0\"\ndisplay:\n  name: Generated Gap Closer\n  description: Closes a detected missing-skill gap.\ninterfaces:\n  - name: run\n    transport:\n      type: internal\n    input_schema:\n      type: object\n      additionalProperties: false\n      properties:\n        query:\n          type: string\n    output_schema:\n      type: object\n      additionalProperties: true\neffects:\n  side_effects: []\n  risk_tier: low\n  idempotency: true\n  idempotency_level: idempotent\n  reversibility: reversible_internal\n  requires_confirmation: false\nsecurity:\n  auth: []\n  data_access:\n    pii: none\n    secrets: forbidden\n  sandbox:\n    required: false\n    network_egress: []\nperformance:\n  expected_p50_ms: 50\n  timeout_ms: 1000\n  rate_limit:\n    qps: 1\n    burst: 1\nobservability:\n  log_redaction: []\n  emit_metrics: []\ngovernance:\n  publisher: navi.test\n  signed: false\n  deprecates: []\n  replaces: []\n  trust_tier: local\ncapability:\n  tags: [gap]\n  domains: [automation]\n  provides: [generated-gap-closer]\n  requires: []\n  command_type: invoke\nreliability:\n  expected_failure_modes: []\n  retry_policy: none\n",
  "main_py": "",
  "requirements_txt": ""
}`},
		"test-model",
		registry.workspaceDir,
		db,
		func(ctx context.Context, req GovernanceRequest) (GovernanceResponse, error) {
			return GovernanceResponse{Decision: GovernanceDecisionApproved}, nil
		},
		nil,
	)

	result, err := builder.Build(ctx, BuildRequest{
		Gap:         gap,
		UserContext: "Create the missing generated gap closer skill",
		ChatID:   "sess-1",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !result.Installed {
		t.Fatalf("expected Installed=true, got false")
	}
	if !result.GapClosed {
		t.Fatalf("expected GapClosed=true, got false")
	}
	if result.SkillID != "generated-gap-closer" {
		t.Fatalf("unexpected skill id: %s", result.SkillID)
	}
	if result.SkillDir == "" {
		t.Fatalf("expected installed skill dir")
	}

	entry, ok := builder.lookupInstalledSkill("generated-gap-closer")
	if !ok {
		t.Fatalf("generated skill not found in registry")
	}
	if !entry.Activatable {
		t.Fatalf("expected installed skill to be activatable, reasons=%v", entry.ReasonsUnbound)
	}

	updatedGap, err := store.GetGap(ctx, db, gap.ID)
	if err != nil {
		t.Fatalf("GetGap: %v", err)
	}
	if updatedGap == nil || updatedGap.Status != store.GapStatusClosed {
		t.Fatalf("expected closed gap, got %#v", updatedGap)
	}
}

func TestSkillBuilderBuildReturnsGovernedPauseWhenApprovalIsPending(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := store.InitTestDB(t)
	registry := NewRegistry(t.TempDir())
	if err := registry.Load(); err != nil {
		t.Fatalf("registry.Load: %v", err)
	}

	gap := store.Gap{
		ID:     "gap-build-2",
		Type:   store.GapTypeMissingSkill,
		Status: store.GapStatusClassified,
		ClassificationResult: store.GapClassification{
			GapClass:      store.GapTypeMissingSkill,
			ExpansionPath: "skill",
			Reason:        "Needs review before install",
		},
	}

	var govReq GovernanceRequest
	builder := NewSkillBuilder(
		registry,
		&builderTestLLM{content: `{
  "skill_yaml": "oss27_version: \"1.0\"\nskill_id: generated-review-skill\nsemver: \"0.1.0\"\ndisplay:\n  name: Generated Review Skill\n  description: Requires approval before install.\ninterfaces:\n  - name: run\n    transport:\n      type: internal\n    input_schema:\n      type: object\n      properties: {}\n    output_schema:\n      type: object\n      properties: {}\neffects:\n  side_effects: []\n  risk_tier: low\n  idempotency: true\n  idempotency_level: idempotent\n  reversibility: reversible_internal\n  requires_confirmation: false\nsecurity:\n  auth: []\n  data_access:\n    pii: none\n    secrets: forbidden\n  sandbox:\n    required: false\n    network_egress: []\nperformance:\n  expected_p50_ms: 50\n  timeout_ms: 1000\n  rate_limit:\n    qps: 1\n    burst: 1\nobservability:\n  log_redaction: []\n  emit_metrics: []\ngovernance:\n  publisher: navi.test\n  signed: false\n  deprecates: []\n  replaces: []\n  trust_tier: local\n",
  "main_py": "",
  "requirements_txt": ""
}`},
		"test-model",
		registry.workspaceDir,
		db,
		func(ctx context.Context, req GovernanceRequest) (GovernanceResponse, error) {
			govReq = req
			return GovernanceResponse{
				Decision:       GovernanceDecisionPending,
				Reason:         "needs owner confirmation",
				ProposalID:     "proposal-123",
				ProposalStatus: string(schema.ProposalStatusPending),
			}, nil
		},
		nil,
	)

	result, err := builder.Build(ctx, BuildRequest{
		Gap:       gap,
		ChatID: "sess-2",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !result.GovernedPause {
		t.Fatalf("expected governed pause")
	}
	if result.GovernedProposalID != "proposal-123" {
		t.Fatalf("expected proposal id, got %q", result.GovernedProposalID)
	}
	if result.GovernedProposalStatus != string(schema.ProposalStatusPending) {
		t.Fatalf("expected pending status, got %q", result.GovernedProposalStatus)
	}
	if govReq.Operation != "install_skill" {
		t.Fatalf("expected install_skill operation, got %q", govReq.Operation)
	}
	if govReq.SourceProcess != "gap_skill_scaffolding" {
		t.Fatalf("unexpected source process %q", govReq.SourceProcess)
	}
}

func TestSkillBuilderBuildScaffoldsConnectorAndClosesGap(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := store.InitTestDB(t)
	workspaceDir := t.TempDir()
	registry := NewRegistry(filepath.Join(workspaceDir, "skills"))
	if err := registry.Load(); err != nil {
		t.Fatalf("registry.Load: %v", err)
	}

	gap := store.Gap{
		ID:     "gap-build-connector",
		Type:   store.GapTypeMissingConnector,
		Status: store.GapStatusClassified,
		Evidence: store.GapEvidence{
			ChatID:  "sess-3",
			ToolError:  "connector not configured",
			RawContext: "user asked to send a message through a missing connector",
		},
		ClassificationResult: store.GapClassification{
			GapClass:      store.GapTypeMissingConnector,
			ExpansionPath: "connector",
			Reason:        "Messaging connector is missing",
		},
	}
	if err := store.InsertGap(ctx, db, gap); err != nil {
		t.Fatalf("InsertGap: %v", err)
	}

	builder := NewSkillBuilder(
		registry,
		&builderTestLLM{content: `{
  "connector_yaml": "name: generated-connector\ndisplay_name: Generated Connector\ntype: subprocess\ncommand: python3\nargs:\n  - main.py\ncapabilities:\n  - name: messaging.send\n",
  "main_py": "import json\nimport sys\n\nfor raw in sys.stdin:\n    msg = json.loads(raw)\n    if msg.get(\"type\") == \"stop\":\n        break\n",
  "requirements_txt": ""
}`},
		"test-model",
		workspaceDir,
		db,
		func(ctx context.Context, req GovernanceRequest) (GovernanceResponse, error) {
			if req.Domain != "connectors" {
				t.Fatalf("expected connector domain validation, got %q", req.Domain)
			}
			return GovernanceResponse{Decision: GovernanceDecisionApproved}, nil
		},
		nil,
	)

	result, err := builder.Build(ctx, BuildRequest{
		Gap:         gap,
		UserContext: "Create the missing messaging connector",
		ChatID:   "sess-3",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if result.Kind != "connector" {
		t.Fatalf("expected connector result, got %q", result.Kind)
	}
	if !result.Installed || !result.GapClosed {
		t.Fatalf("expected installed+gap closed, got %+v", result)
	}
	if result.ConnectorName != "generated-connector" {
		t.Fatalf("unexpected connector name %q", result.ConnectorName)
	}
	if result.ConnectorDir == "" {
		t.Fatalf("expected connector dir")
	}

	manifestPath := filepath.Join(result.ConnectorDir, "CONNECTOR.yaml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile(CONNECTOR.yaml): %v", err)
	}
	var manifest pkgconn.ConnectorManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if manifest.Name != "generated-connector" {
		t.Fatalf("unexpected manifest name %q", manifest.Name)
	}
	if manifest.Type != "subprocess" {
		t.Fatalf("unexpected manifest type %q", manifest.Type)
	}
	if _, err := os.Stat(filepath.Join(result.ConnectorDir, "main.py")); err != nil {
		t.Fatalf("expected main.py: %v", err)
	}

	updatedGap, err := store.GetGap(ctx, db, gap.ID)
	if err != nil {
		t.Fatalf("GetGap: %v", err)
	}
	if updatedGap == nil || updatedGap.Status != store.GapStatusClosed {
		t.Fatalf("expected closed connector gap, got %#v", updatedGap)
	}
}

func TestSkillBuilderBuildCommunityHubSkillUsesCentralGovernanceSeam(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := store.InitTestDB(t)
	registry := NewRegistry(t.TempDir())
	if err := registry.Load(); err != nil {
		t.Fatalf("registry.Load: %v", err)
	}

	gap := store.Gap{
		ID:     "gap-build-hub-community",
		Type:   store.GapTypeMissingSkill,
		Status: store.GapStatusClassified,
		ClassificationResult: store.GapClassification{
			GapClass:      store.GapTypeMissingSkill,
			ExpansionPath: "skill",
			Reason:        "Need community skill fallback",
		},
	}

	var requests []GovernanceRequest
	builder := NewSkillBuilder(
		registry,
		&builderTestLLM{content: `not-json`},
		"test-model",
		registry.workspaceDir,
		db,
		func(ctx context.Context, req GovernanceRequest) (GovernanceResponse, error) {
			requests = append(requests, req)
			return GovernanceResponse{
				Decision:       GovernanceDecisionPending,
				Reason:         "owner approval required",
				ProposalID:     "proposal-community",
				ProposalStatus: string(schema.ProposalStatusPending),
			}, nil
		},
		&stubBuilderHub{
			results: []HubIndexEntry{{
				SkillID:   "community-weather",
				TrustTier: "community",
			}},
		},
	)

	result, err := builder.Build(ctx, BuildRequest{
		Gap:       gap,
		ChatID: "sess-community",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !result.GovernedPause {
		t.Fatalf("expected governed pause for community hub skill")
	}
	if len(requests) != 1 {
		t.Fatalf("expected 1 governance request, got %d", len(requests))
	}
	req := requests[0]
	if req.Operation != "install_skill_from_hub" {
		t.Fatalf("unexpected operation %q", req.Operation)
	}
	if !req.RequireConfirmation {
		t.Fatalf("expected community install to force confirmation")
	}
	if !strings.Contains(req.ConfirmationRationale, "community") {
		t.Fatalf("unexpected confirmation rationale %q", req.ConfirmationRationale)
	}
}

type stubBuilderHub struct {
	results []HubIndexEntry
}

func (s *stubBuilderHub) Search(ctx context.Context, query string) ([]HubIndexEntry, error) {
	return s.results, nil
}

func (s *stubBuilderHub) Download(ctx context.Context, entry HubIndexEntry, destDir string) error {
	return nil
}
