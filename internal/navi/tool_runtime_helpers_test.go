package navi

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/governor"
	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/navi/inference"
	"github.com/open-navi/navi/internal/navi/proposals"
	naviruntime "github.com/open-navi/navi/internal/runtime"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
	navitool "github.com/open-navi/navi/internal/tool"
	"github.com/open-navi/navi/internal/worldmodel"
)

func cacheInferenceEnvelope(t *testing.T, run *naviruntime.RunState, envelope inference.DecisionEnvelope) {
	t.Helper()
	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal inference envelope: %v", err)
	}
	run.ICSDecisionEnvelope = payload
	run.ICSStateVersion = firstNonEmpty(envelope.Rationale.Version, inference.ContractVersionV1)
}

func TestParseProtectedPathConfigurationValue(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "json array",
			raw:  `["**/secrets/**", "/etc/ssh/**", "**/secrets/**"]`,
			want: []string{"**/secrets/**", "/etc/ssh/**"},
		},
		{
			name: "comma and newline separated",
			raw:  "**/secrets/**,\n/etc/ssh/**,\n  /etc/ssh/**  ",
			want: []string{"**/secrets/**", "/etc/ssh/**"},
		},
		{
			name: "empty",
			raw:  "   ",
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseProtectedPathConfigurationValue(tc.raw)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseProtectedPathConfigurationValue(%q) = %#v, want %#v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestGovernanceProposalPayloadUsesWorkspaceBoundaryShape(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()
	wm := worldmodel.New(db)
	if err := store.SaveWorkspace(ctx, db, schema.Workspace{
		ID:             "ws-1",
		Name:           "Workspace",
		Kind:           schema.WorkspaceKindProject,
		Status:         schema.WorkspaceStatusActive,
		LocalRoots:     []string{"/workspace/in-scope"},
		AllowedActions: schema.AllowedActions{Read: true, Write: true, Create: true, Modify: true, RenameMove: true},
		BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		AuditEnabled:   true,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
		CreatedBy:      "owner",
	}); err != nil {
		t.Fatalf("SaveWorkspace: %v", err)
	}
	if err := wm.SetOperatingMode(ctx, string(schema.WorkspaceOperatingModeScoped)); err != nil {
		t.Fatalf("SetOperatingMode: %v", err)
	}
	if err := wm.SetActiveWorkspaceID(ctx, "ws-1"); err != nil {
		t.Fatalf("SetActiveWorkspaceID: %v", err)
	}

	loop := NewAgentLoop(LoopConfig{
		ExperienceManager: NewExperienceManager("", ExperienceModeStandard),
	})
	loop.cfg.WorldModel = wm

	_ = &navitool.Tool{
		Name: "write_file",
		Governance: navitool.ToolGovernance{
			CommandType:     schema.CommandTypeUpdate,
			WorkspaceAction: schema.WorkspaceActionWrite,
			PathRestriction: "path",
		},
	}
	call := llm.ToolCall{
		ID:   "tc-boundary",
		Name: "write_file",
		Arguments: map[string]any{
			"path": "/outside/report.md",
		},
	}
	validation := governor.ValidationResult{
		Outcome: governor.ValidationRequiresConfirmation,
		Reason:  "Action targeting path outside workspace scope: /outside/report.md",
	}

	run := naviruntime.NewRun("sess-1", string(ExperienceModeStandard))
	run.RunID = "run-1"
	handoff := inference.GovernanceHandoff{
		CommandType:         schema.CommandTypeUpdate,
		Domain:              governor.AutonomyDomainCoding,
		ActorKind:           "navi",
		TargetCapability:    "write_file",
		AllowedCapabilities: []string{"write_file"},
	}
	intent := inference.ExecutionIntent{
		ActionType:          string(schema.CommandTypeUpdate),
		Target:              "/outside/report.md",
		TargetCapability:    "write_file",
		AllowedCapabilities: []string{"write_file"},
	}
	cacheInferenceEnvelope(t, run, inference.DecisionEnvelope{
		Rationale: inference.Rationale{
			Version:         inference.ContractVersionV1,
			Governance:      handoff,
			ExecutionIntent: intent,
		},
	})
	payload := loop.governanceProposalPayload(ctx, run, &inference.ToolContract{WorkspaceAction: schema.WorkspaceActionWrite, CommandType: schema.CommandTypeUpdate}, call, validation, "/outside/report.md")
	got, ok := payload.(proposals.WorkspaceBoundaryAction)
	if !ok {
		t.Fatalf("expected WorkspaceBoundaryAction payload, got %#v", payload)
	}
	if got.Type != "workspace_boundary_crossing" {
		t.Fatalf("expected workspace boundary payload type, got %#v", got.Type)
	}
	if got.ChatID != "sess-1" || got.RunID != "run-1" {
		t.Fatalf("expected runtime context in payload, got %#v", got)
	}
	if got.TargetPath != "/outside/report.md" {
		t.Fatalf("expected target_path in payload, got %#v", got.TargetPath)
	}
	if got.WorkspaceAction != schema.WorkspaceActionWrite {
		t.Fatalf("expected workspace action write, got %#v", got.WorkspaceAction)
	}
	if len(got.ActionTypes) != 1 || got.ActionTypes[0] != string(schema.WorkspaceActionWrite) {
		t.Fatalf("expected action_types [write], got %#v", got.ActionTypes)
	}
	if got.RuleScope != "/outside/report.md" {
		t.Fatalf("expected rule_scope to default to target path, got %#v", got.RuleScope)
	}
	if !workspaceBoundaryPayloadTargetCapability(got.ICSGovernance, "write_file") {
		t.Fatalf("expected workspace-boundary proposal to retain ICS governance handoff, got %#v", got.ICSGovernance)
	}
	if !workspaceBoundaryPayloadTargetCapability(got.ICSIntent, "write_file") {
		t.Fatalf("expected workspace-boundary proposal to retain ICS execution intent, got %#v", got.ICSIntent)
	}
}

func TestPostIssuanceProposalPayload_UsesContractNotRegistry(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()
	wm := worldmodel.New(db)
	if err := store.SaveWorkspace(ctx, db, schema.Workspace{
		ID:             "ws-contract-only",
		Name:           "Workspace",
		Kind:           schema.WorkspaceKindProject,
		Status:         schema.WorkspaceStatusActive,
		LocalRoots:     []string{"/workspace/in-scope"},
		AllowedActions: schema.AllowedActions{Read: true, Write: true, Create: true, Modify: true, RenameMove: true, Delete: true},
		BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		AuditEnabled:   true,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
		CreatedBy:      "owner",
	}); err != nil {
		t.Fatalf("SaveWorkspace: %v", err)
	}
	if err := wm.SetOperatingMode(ctx, string(schema.WorkspaceOperatingModeScoped)); err != nil {
		t.Fatalf("SetOperatingMode: %v", err)
	}
	if err := wm.SetActiveWorkspaceID(ctx, "ws-contract-only"); err != nil {
		t.Fatalf("SetActiveWorkspaceID: %v", err)
	}

	loop := NewAgentLoop(LoopConfig{
		ExperienceManager: NewExperienceManager("", ExperienceModeStandard),
		ToolRegistry:      navitool.NewRegistry(),
	})
	loop.cfg.WorldModel = wm
	if err := loop.cfg.ToolRegistry.Register(&navitool.Tool{
		ToolID:                "navi.test.write_file",
		DisplayName:           "write_file",
		Description:           "test registry write",
		Source:                navitool.ToolSourceFileTools,
		SourceID:              "tests",
		SchemaVersion:         "1.0.0",
		InputSchema:           map[string]any{"type": "object", "properties": map[string]any{}},
		OutputSchema:          map[string]any{"type": "string"},
		Category:              navitool.ToolCategoryWorkflowAction,
		RiskTier:              "high",
		SideEffects:           []string{"workspace_write"},
		Reversibility:         "irreversible",
		EnvironmentVisibility: []string{"development"},
		RequiredModes:         []schema.DirectiveMode{},
		RequiredAuthority:     navitool.ToolAuthorityUser,
		FeatureFlags:          []string{},
		ConnectorDependencies: []string{},
		Aliases:               []string{},
		CapabilityTags:        []string{},
		Status:                navitool.ToolStatusActive,
		Definition: llm.ToolDefinition{
			Name:        "navi.test.write_file",
			Description: "test registry write",
		},
		Governance: navitool.ToolGovernance{
			CommandType:     schema.CommandTypeUpdate,
			WorkspaceAction: schema.WorkspaceActionWrite,
			PathRestriction: "path",
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	run := naviruntime.NewRun("sess-contract-only", string(ExperienceModeStandard))
	run.RunID = "run-contract-only"
	cacheInferenceEnvelope(t, run, inference.DecisionEnvelope{
		Rationale: inference.Rationale{
			Version: inference.ContractVersionV1,
			Governance: inference.GovernanceHandoff{
				CommandType:         schema.CommandTypeUpdate,
				Domain:              governor.AutonomyDomainCoding,
				ActorKind:           "navi",
				TargetCapability:    "navi.test.write_file",
				AllowedCapabilities: []string{"navi.test.write_file"},
			},
			ExecutionIntent: inference.ExecutionIntent{
				ActionType:          string(schema.CommandTypeUpdate),
				Target:              "/outside/delete-me.txt",
				TargetCapability:    "navi.test.write_file",
				AllowedCapabilities: []string{"navi.test.write_file"},
			},
		},
	})

	payload := loop.governanceProposalPayload(ctx, run, &inference.ToolContract{
		ID:              "contract:navi.test.write_file",
		ToolName:        "navi.test.write_file",
		ExecutionKind:   inference.ToolExecutionKindRegistry,
		CommandType:     schema.CommandTypeDelete,
		Domain:          "files",
		ActorKind:       "navi",
		WorkspaceAction: schema.WorkspaceActionDelete,
	}, llm.ToolCall{
		ID:   "tc-contract-only",
		Name: "navi.test.write_file",
		Arguments: map[string]any{
			"path": "/outside/delete-me.txt",
		},
	}, governor.ValidationResult{
		Outcome: governor.ValidationRequiresConfirmation,
		Reason:  "Action targeting path outside workspace scope: /outside/delete-me.txt",
	}, "/outside/delete-me.txt")

	got, ok := payload.(proposals.WorkspaceBoundaryAction)
	if !ok {
		t.Fatalf("expected WorkspaceBoundaryAction payload, got %#v", payload)
	}
	if got.WorkspaceAction != schema.WorkspaceActionDelete {
		t.Fatalf("expected contract workspace action delete, got %#v", got.WorkspaceAction)
	}
	if len(got.ActionTypes) != 1 || got.ActionTypes[0] != string(schema.WorkspaceActionDelete) {
		t.Fatalf("expected action_types [delete] from contract, got %#v", got.ActionTypes)
	}
}

func TestPostIssuanceWorkspaceCheck_UsesContractNotRegistry(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()
	wm := worldmodel.New(db)
	workspaceDir := t.TempDir()
	if err := store.SaveWorkspace(ctx, db, schema.Workspace{
		ID:             "ws-post-issuance-check",
		Name:           "Workspace",
		Kind:           schema.WorkspaceKindProject,
		Status:         schema.WorkspaceStatusActive,
		LocalRoots:     []string{workspaceDir},
		AllowedActions: schema.AllowedActions{Read: true, Write: true, Create: true, Modify: true, RenameMove: true, Delete: false},
		BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
		CreatedBy:      "owner",
	}); err != nil {
		t.Fatalf("SaveWorkspace: %v", err)
	}
	if err := wm.SetOperatingMode(ctx, string(schema.WorkspaceOperatingModeScoped)); err != nil {
		t.Fatalf("SetOperatingMode: %v", err)
	}
	if err := wm.SetActiveWorkspaceID(ctx, "ws-post-issuance-check"); err != nil {
		t.Fatalf("SetActiveWorkspaceID: %v", err)
	}

	loop := NewAgentLoop(LoopConfig{
		ExperienceManager: NewExperienceManager("", ExperienceModeStandard),
		WorldModel:        wm,
		WorkspaceDir:      workspaceDir,
	})

	targetPath := filepath.Join(workspaceDir, "inside", "delete-me.txt")
	check, ok := loop.postIssuanceWorkspaceCheck(ctx, "sess-post-issuance-check", &inference.ToolPermit{
		ToolCallID: "tc-post-issuance-check",
		ToolName:   "write_file",
		TargetPath: targetPath,
		ContractID: "contract:write_file",
		Contract: &inference.ToolContract{
			ID:                  "contract:write_file",
			ToolName:            "write_file",
			ExecutionKind:       inference.ToolExecutionKindRegistry,
			CommandType:         schema.CommandTypeDelete,
			Domain:              "files",
			ActorKind:           "navi",
			WorkspaceAction:     schema.WorkspaceActionDelete,
			TargetPathArg:       "path",
			WorkspaceScopedPath: false,
		},
	}, map[string]any{"path": targetPath})
	if !ok {
		t.Fatal("expected post-issuance workspace check to resolve from permit contract")
	}
	if check.Outcome != governor.ValidationRejected {
		t.Fatalf("expected delete deny from contract workspace action, got %#v", check)
	}
	if !strings.Contains(check.Reason, "explicitly denied") {
		t.Fatalf("expected deny-mask reason from contract workspace action, got %q", check.Reason)
	}
}

func TestPostIssuanceAuditPath_DoesNotRecomputeAuthority(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()
	wm := worldmodel.New(db)
	workspaceDir := t.TempDir()
	if err := store.SaveWorkspace(ctx, db, schema.Workspace{
		ID:             "ws-post-issuance-audit",
		Name:           "Workspace",
		Kind:           schema.WorkspaceKindProject,
		Status:         schema.WorkspaceStatusActive,
		LocalRoots:     []string{workspaceDir},
		AllowedActions: schema.AllowedActions{Read: true, Write: true, Create: true, Modify: true, RenameMove: true, Delete: false},
		BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
		CreatedBy:      "owner",
	}); err != nil {
		t.Fatalf("SaveWorkspace: %v", err)
	}
	if err := wm.SetOperatingMode(ctx, string(schema.WorkspaceOperatingModeScoped)); err != nil {
		t.Fatalf("SetOperatingMode: %v", err)
	}
	if err := wm.SetActiveWorkspaceID(ctx, "ws-post-issuance-audit"); err != nil {
		t.Fatalf("SetActiveWorkspaceID: %v", err)
	}

	var outcomes []schema.ExecutionOutcome
	loop := NewAgentLoop(LoopConfig{
		ExperienceManager: NewExperienceManager("", ExperienceModeStandard),
		WorldModel:        wm,
		WorkspaceDir:      workspaceDir,
		SaveExecutionOutcome: func(ctx context.Context, eo schema.ExecutionOutcome) error {
			outcomes = append(outcomes, eo)
			return nil
		},
	})

	targetPath := filepath.Join(workspaceDir, "inside", "delete-me.txt")
	permit := &inference.ToolPermit{
		ContractID:         "contract:write_file",
		ToolCallID:         "tc-post-issuance-audit",
		ToolName:           "write_file",
		ApprovedProposalID: "prop-contract",
		TargetPath:         targetPath,
		Contract: &inference.ToolContract{
			ID:                  "contract:write_file",
			ToolName:            "write_file",
			ExecutionKind:       inference.ToolExecutionKindRegistry,
			CommandType:         schema.CommandTypeDelete,
			Domain:              "files",
			ActorKind:           "navi",
			WorkspaceAction:     schema.WorkspaceActionDelete,
			TargetPathArg:       "path",
			WorkspaceScopedPath: false,
			SkillName:           "contract-skill",
		},
	}
	loop.recordBlockedToolExecutionOutcome(ctx, "sess-post-issuance-audit", "run-post-issuance-audit", permit, llm.ToolCall{
		ID:   "tc-post-issuance-audit",
		Name: "write_file",
		Arguments: map[string]any{
			"path": targetPath,
		},
	}, governor.ValidationResult{
		Outcome: governor.ValidationRejected,
		Reason:  "blocked before execution",
	}, "")

	if len(outcomes) != 1 {
		t.Fatalf("expected one blocked execution outcome, got %#v", outcomes)
	}
	if outcomes[0].CommandType != schema.CommandTypeDelete {
		t.Fatalf("expected audit command type from contract, got %#v", outcomes[0].CommandType)
	}
	if !reflect.DeepEqual(outcomes[0].SkillIDs, []string{"contract-skill"}) {
		t.Fatalf("expected audit skill ids from contract, got %#v", outcomes[0].SkillIDs)
	}
	if outcomes[0].ProposalID != "prop-contract" {
		t.Fatalf("expected audit proposal id from permit, got %#v", outcomes[0].ProposalID)
	}
	if outcomes[0].WorkspaceID != "ws-post-issuance-audit" {
		t.Fatalf("expected audit workspace id to resolve, got %#v", outcomes[0].WorkspaceID)
	}
}

func TestPostIssuanceExecutionHelpers_DoNotCallRegistryAuthorityHelpers(t *testing.T) {
	t.Helper()
	checks := []struct {
		file      string
		function  string
		forbidden []string
	}{
		{
			file:      "tool_runtime_helpers.go",
			function:  "func (l *AgentLoop) postIssuanceWorkspaceCheck(",
			forbidden: []string{"toolCommandType(", "toolWorkspaceAction(", "skillCommandType("},
		},
		{
			file:      "tool_runtime_helpers.go",
			function:  "func (l *AgentLoop) workspaceAuditFields(",
			forbidden: []string{"toolCommandType(", "toolWorkspaceAction(", "skillCommandType("},
		},
		{
			file:      "tool_runtime_helpers.go",
			function:  "func (l *AgentLoop) recordBlockedToolExecutionOutcome(",
			forbidden: []string{"toolCommandType(", "toolWorkspaceAction(", "skillCommandType("},
		},
		{
			file:      "tool_runtime_helpers.go",
			function:  "func (l *AgentLoop) governanceProposalPayload(",
			forbidden: []string{"toolCommandType(", "toolWorkspaceAction(", "skillCommandType("},
		},
		{
			file:      "runtime_executor.go",
			function:  "func (l *AgentLoop) executeToolForRun(",
			forbidden: []string{"toolCommandType(", "toolWorkspaceAction(", "skillCommandType("},
		},
	}

	for _, check := range checks {
		body := sourceFunctionBody(t, check.file, check.function)
		for _, forbidden := range check.forbidden {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s in %s must not call %s after contract issuance", check.function, check.file, forbidden)
			}
		}
	}
}

func sourceFunctionBody(t *testing.T, file, signature string) string {
	t.Helper()
	srcBytes, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", file, err)
	}
	src := string(srcBytes)
	start := strings.Index(src, signature)
	if start < 0 {
		t.Fatalf("expected to find function signature %q in %s", signature, file)
	}
	brace := strings.Index(src[start:], "{")
	if brace < 0 {
		t.Fatalf("expected to find opening brace for %q in %s", signature, file)
	}
	brace += start
	depth := 0
	for i := brace; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[brace : i+1]
			}
		}
	}
	t.Fatalf("expected to find closing brace for %q in %s", signature, file)
	return ""
}

func workspaceBoundaryPayloadTargetCapability(value any, want string) bool {
	switch typed := value.(type) {
	case map[string]any:
		return typed["target_capability"] == want
	case *inference.GovernanceHandoff:
		return typed != nil && typed.TargetCapability == want
	case *inference.ExecutionIntent:
		return typed != nil && typed.TargetCapability == want
	default:
		return false
	}
}

func TestInferenceGovernanceActionDescriptorUsesAllowedSubsetWhenNoExactTargetExists(t *testing.T) {
	run := naviruntime.NewRun("sess-ics-governance-subset", string(ExperienceModeStandard))
	handoff := inference.GovernanceHandoff{
		CommandType:         schema.CommandTypeInvoke,
		Domain:              governor.AutonomyDomainCoding,
		ActorKind:           "navi",
		AllowedCapabilities: []string{"code_write", "code_patch"},
		AllowedCapabilityDetails: []inference.CapabilityAvailability{
			{Name: "code_write", Kind: governor.AutonomyDomainCoding, CommandType: schema.CommandTypeInvoke},
			{Name: "code_patch", Kind: governor.AutonomyDomainCoding, CommandType: schema.CommandTypeInvoke},
		},
		Tags: map[string]string{"candidate_type": "execute"},
	}
	cacheInferenceEnvelope(t, run, inference.DecisionEnvelope{
		Rationale: inference.Rationale{
			Version:    inference.ContractVersionV1,
			Governance: handoff,
		},
	})

	action, ok := inferenceGovernanceActionDescriptor(run, "code_patch")
	if !ok {
		t.Fatal("expected allowed capability to resolve through ICS governance handoff")
	}
	if action.Domain != governor.AutonomyDomainCoding || action.CommandType != schema.CommandTypeInvoke {
		t.Fatalf("expected ICS governance descriptor fields, got %#v", action)
	}
	if action.Tags["target_capability"] != "code_patch" {
		t.Fatalf("expected runtime action descriptor to bind the attempted allowed capability, got %#v", action.Tags)
	}
	if _, ok := inferenceGovernanceActionDescriptor(run, "calendar_lookup"); ok {
		t.Fatal("expected capability outside the ICS subset to be rejected")
	}
}

func TestInferenceAllowedCapabilitiesFromRun_DoesNotInferAttemptedTool(t *testing.T) {
	run := naviruntime.NewRun("sess-ics-capability-empty", string(ExperienceModeStandard))
	handoff := inference.GovernanceHandoff{
		CommandType: schema.CommandTypeInvoke,
		Domain:      governor.AutonomyDomainCoding,
		ActorKind:   "navi",
	}
	cacheInferenceEnvelope(t, run, inference.DecisionEnvelope{
		Rationale: inference.Rationale{
			Version:    inference.ContractVersionV1,
			Governance: handoff,
		},
	})

	if got := inferenceAllowedCapabilitiesFromRun(run); len(got) != 0 {
		t.Fatalf("expected no inferred allowed capabilities from empty ICS handoff, got %#v", got)
	}
}

func TestInferenceAllowedCapabilitiesFromRun_DoesNotFallBackToLegacyScratchpadList(t *testing.T) {
	run := naviruntime.NewRun("sess-ics-capability-scratchpad", string(ExperienceModeStandard))
	run.Scratchpad = map[string]string{
		inferenceScratchAllowedCapabilities: `["code_write","code_patch","code_write"]`,
	}

	got := inferenceAllowedCapabilitiesFromRun(run)
	if len(got) != 0 {
		t.Fatalf("expected no legacy scratchpad fallback without authoritative governance handoff, got %#v", got)
	}
}

func TestGovernanceProposalPayloadIncludesCanonicalInferenceArtifacts(t *testing.T) {
	ctx := context.Background()
	loop := NewAgentLoop(LoopConfig{
		ExperienceManager: NewExperienceManager("", ExperienceModeStandard),
	})

	run := naviruntime.NewRun("sess-ics-governance-proposal", string(ExperienceModeStandard))
	run.RunID = "run-ics-governance-proposal"
	handoff := inference.GovernanceHandoff{
		CommandType:         schema.CommandTypeInvoke,
		Domain:              governor.AutonomyDomainCoding,
		ActorKind:           "navi",
		TargetCapability:    "code_patch",
		AllowedCapabilities: []string{"code_write", "code_patch"},
		Tags: map[string]string{
			"candidate_type":       "execute",
			"target_capability":    "code_patch",
			"allowed_capabilities": "code_write,code_patch",
		},
	}
	intent := inference.ExecutionIntent{
		ActionType:          string(schema.CommandTypeInvoke),
		Target:              "goal-1",
		TargetCapability:    "code_patch",
		AllowedCapabilities: []string{"code_write", "code_patch"},
	}
	cacheInferenceEnvelope(t, run, inference.DecisionEnvelope{
		Rationale: inference.Rationale{
			Version:         inference.ContractVersionV1,
			Governance:      handoff,
			ExecutionIntent: intent,
		},
	})

	payload := loop.governanceProposalPayload(ctx, run, nil, llm.ToolCall{
		ID:   "tc-governance",
		Name: "code_patch",
		Arguments: map[string]any{
			"path": "src/main.go",
		},
	}, governor.ValidationResult{
		Outcome: governor.ValidationRequiresConfirmation,
		Reason:  "requires confirmation",
	}, "")

	data, ok := payload.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload with ICS artifacts, got %#v", payload)
	}
	rawHandoff, ok := data["ics_governance_handoff"].(inference.GovernanceHandoff)
	if !ok {
		t.Fatalf("expected canonical ics_governance_handoff payload, got %#v", data["ics_governance_handoff"])
	}
	if got := rawHandoff.TargetCapability; got != "code_patch" {
		t.Fatalf("expected canonical handoff target capability code_patch, got %q", got)
	}
	rawIntent, ok := data["ics_execution_intent"].(inference.ExecutionIntent)
	if !ok {
		t.Fatalf("expected canonical ics_execution_intent payload, got %#v", data["ics_execution_intent"])
	}
	if got := rawIntent.TargetCapability; got != "code_patch" {
		t.Fatalf("expected canonical execution intent target capability code_patch, got %q", got)
	}
	governanceData, ok := data["governance"].(map[string]any)
	if !ok {
		t.Fatalf("expected proposal governance details, got %#v", data["governance"])
	}
	allowed, ok := governanceData["allowed_capabilities"].([]string)
	if !ok {
		t.Fatalf("expected proposal governance allowed_capabilities []string, got %#v", governanceData["allowed_capabilities"])
	}
	if !reflect.DeepEqual(allowed, []string{"code_patch"}) {
		t.Fatalf("expected proposal governance to remain bounded to ICS target capability, got %#v", allowed)
	}
}
