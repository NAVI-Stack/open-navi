package navi

import (
	"context"
	"testing"

	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/navi/orchestration"
	"github.com/ceoai/navi/internal/schema"
	navitool "github.com/ceoai/navi/internal/tool"
)

type noopToolExecutor struct{}

func (noopToolExecutor) Execute(ctx context.Context, args map[string]any) (navitool.ToolResult, error) {
	return navitool.ToolResult{Content: "ok"}, nil
}

func TestBuildInferenceCapabilities_UsesRuntimeToolMetadata(t *testing.T) {
	reg := navitool.NewRegistry()
	if err := reg.Register(&navitool.Tool{
		ToolID:                "test.runtime.write_file",
		DisplayName:           "write_file",
		Description:           "write file",
		Source:                navitool.ToolSourceFileTools,
		SourceID:              "tests.runtime",
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
		Aliases:               []string{"write_file"},
		CapabilityTags:        []string{"state_mutation"},
		Status:                navitool.ToolStatusActive,
		Definition: llm.ToolDefinition{
			Name:        "test.runtime.write_file",
			Description: "write file",
		},
		Executor: noopToolExecutor{},
		Governance: navitool.ToolGovernance{
			CommandType:     schema.CommandTypeUpdate,
			WorkspaceAction: schema.WorkspaceActionWrite,
			RequiresConfirm: true,
			RiskTier:        "high",
			Domain:          "files",
			PathRestriction: "path",
			Reversibility:   "irreversible",
		},
		Metadata: navitool.ToolMetadata{
			SkillName: "fs-skill",
			PluginID:  "plugin-x",
			Tags:      []string{"state_mutation"},
			Notes: map[string]string{
				"connector_id": "telegram",
			},
		},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	loop := NewAgentLoop(LoopConfig{ToolRegistry: reg})
	caps := loop.buildInferenceCapabilities(orchestration.CanonicalRunRequest{
		RequiredOutput: orchestration.RequiredOutput{AllowToolCalls: true},
		CapabilitySurface: orchestration.CapabilitySurface{
			Surface:   orchestration.CapabilitySurfaceRuntime,
			ToolNames: []string{"test.runtime.write_file"},
		},
	})
	if len(caps) != 1 {
		t.Fatalf("expected exactly one capability, got %d", len(caps))
	}
	got := caps[0]
	if got.SourceType != "file_tool" || got.CommandType != schema.CommandTypeUpdate || got.Domain != "files" {
		t.Fatalf("unexpected core metadata: %+v", got)
	}
	if !got.RequiresConfirmation || got.WorkspaceAction != schema.WorkspaceActionWrite || got.TargetPathArg != "path" || !got.WorkspaceScopedPath {
		t.Fatalf("expected workspace/confirmation metadata from contract, got %+v", got)
	}
	if got.RiskHint != schema.RiskHigh || got.Reversibility != schema.ReversibilityIrreversible {
		t.Fatalf("expected risk metadata from governance, got %+v", got)
	}
	if got.SkillName != "fs-skill" || got.PluginName != "plugin-x" || got.ConnectorID != "telegram" {
		t.Fatalf("expected provenance metadata, got %+v", got)
	}
}
