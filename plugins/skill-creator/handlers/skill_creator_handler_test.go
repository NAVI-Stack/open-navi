package handlers

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/open-navi/navi/internal/llm"
	coreskill "github.com/open-navi/navi/internal/navi/skill"
	"github.com/open-navi/navi/internal/store"
)

var GetInternalHandler = coreskill.GetInternalHandler

func TestSkillCreatorToolAppearsInRegistryAndIsActivatable(t *testing.T) {
	t.Parallel()

	registry := coreskill.NewRegistry(t.TempDir())
	if err := registry.Load(); err != nil {
		t.Fatalf("registry.Load: %v", err)
	}

	sourceDir := filepath.Join("..", "..", "..", "plugins", "skill-creator", "skills", "skill-creator")
	if err := registry.Install(sourceDir, false); err != nil {
		t.Fatalf("registry.Install: %v", err)
	}

	var found bool
	for _, entry := range registry.List() {
		if entry.Spec != nil && entry.Spec.SkillID == "skill-creator" {
			found = true
			if !entry.Activatable {
				t.Fatalf("expected skill-creator to be activatable, reasons=%v", entry.ReasonsUnbound)
			}
		}
	}
	if !found {
		t.Fatal("expected skill-creator in registry list")
	}

	tools := registry.Tools()
	for _, tool := range tools {
		if tool.Name == "skill-creator_create" {
			return
		}
	}
	t.Fatal("expected skill-creator_create tool in registry")
}

func TestSkillCreatorHandlerBuildsSkill(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := store.InitTestDB(t)
	workspaceDir := t.TempDir()
	registry := coreskill.NewRegistry(workspaceDir)
	if err := registry.Load(); err != nil {
		t.Fatalf("registry.Load: %v", err)
	}

	builder := coreskill.NewSkillBuilder(
		registry,
		&skillCreatorScriptedLLM{resp: &llm.Response{Content: `{
  "skill_yaml": "oss27_version: \"1.0\"\nskill_id: tool-created-skill\nsemver: \"0.1.0\"\ndisplay:\n  name: tool-created-skill\n  description: Created via the skill creator handler.\ninterfaces:\n  - name: run\n    transport:\n      type: internal\n    input_schema:\n      type: object\n      additionalProperties: false\n      properties:\n        prompt:\n          type: string\n    output_schema:\n      type: object\n      additionalProperties: true\neffects:\n  side_effects: []\n  risk_tier: low\n  idempotency: true\n  idempotency_level: idempotent\n  reversibility: reversible_internal\n  requires_confirmation: false\nsecurity:\n  auth: []\n  data_access:\n    pii: none\n    secrets: forbidden\n  sandbox:\n    required: false\n    network_egress: []\nperformance:\n  expected_p50_ms: 50\n  timeout_ms: 1000\n  rate_limit:\n    qps: 1\n    burst: 1\nobservability:\n  log_redaction: []\n  emit_metrics: []\ngovernance:\n  publisher: navi.test\n  signed: false\n  deprecates: []\n  replaces: []\n  trust_tier: local\ncapability:\n  tags: [generated]\n  domains: [skills]\n  provides: [tool-created-skill]\n  requires: []\n  command_type: invoke\nreliability:\n  expected_failure_modes: []\n  retry_policy: none\n",
  "main_py": "",
  "requirements_txt": ""
}`}},
		"test-model",
		workspaceDir,
		db,
		func(ctx context.Context, req GovernanceRequest) (GovernanceResponse, error) {
			return coreskill.GovernanceResponse{Decision: coreskill.GovernanceDecisionApproved}, nil
		},
		nil,
	)
	RegisterSkillCreatorHandler("skill-creator", builder, db)

	handler, ok := GetInternalHandler("skill-creator", "create")
	if !ok {
		t.Fatal("expected skill-creator internal handler")
	}

	payload, err := handler(ctx, nil, &Interface{Name: "create"}, map[string]any{
		"capability": "Create an email triage skill",
		"context":    "User asked for email automation",
		"chat_id":    "sess-skill-create",
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	result, ok := payload.(map[string]any)
	if !ok {
		t.Fatalf("unexpected payload type: %T", payload)
	}
	if result["skill_id"] != "tool-created-skill" {
		t.Fatalf("unexpected skill_id: %#v", result["skill_id"])
	}
	if installed, ok := result["installed"].(bool); !ok || !installed {
		t.Fatalf("expected installed=true, got %#v", result["installed"])
	}

	if _, found := registry.Lookup("tool-created-skill"); !found {
		t.Fatal("expected generated skill to be installed")
	}

	gaps, err := store.ListOpenGaps(ctx, db, 10)
	if err != nil {
		t.Fatalf("ListOpenGaps: %v", err)
	}
	if len(gaps) != 0 {
		t.Fatalf("expected synthetic gap to be closed, open gaps=%d", len(gaps))
	}
}

type skillCreatorScriptedLLM struct {
	resp *llm.Response
}

func (s *skillCreatorScriptedLLM) Chat(ctx context.Context, model string, messages []llm.Message, tools []llm.ToolDefinition, opts llm.Options) (*llm.Response, error) {
	return s.resp, nil
}

func (s *skillCreatorScriptedLLM) Name() string { return "skill-creator-scripted" }
