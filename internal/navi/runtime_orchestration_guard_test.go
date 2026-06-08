package navi

import (
	"context"
	"strings"
	"testing"

	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/navi/orchestration"
	"github.com/open-navi/navi/internal/schema"
	navitool "github.com/open-navi/navi/internal/tool"
)

func TestResolveCapabilitySurface_GuardsInvalidSurface(t *testing.T) {
	loop := &AgentLoop{cfg: LoopConfig{ToolRegistry: runtimeGuardTestRegistry(t, runtimeGuardTestTool("alpha"))}}

	req, result, err := loop.resolveCapabilitySurface(context.Background(), orchestration.CanonicalRunRequest{
		Frame: orchestration.ExecutionFrame{Mode: orchestration.ExecutionModeRunExecute},
		CapabilitySurface: orchestration.CapabilitySurface{
			Surface: "mystery",
		},
		RequiredOutput: orchestration.RequiredOutput{
			AllowToolCalls: true,
		},
		Model: orchestration.ModelProfile{
			SupportsTools: true,
		},
	})
	if err != nil {
		t.Fatalf("resolveCapabilitySurface: %v", err)
	}
	if result.Guard != orchestration.SurfaceGuardInvalidSurface {
		t.Fatalf("guard = %q, want %q", result.Guard, orchestration.SurfaceGuardInvalidSurface)
	}
	if req.RequiredOutput.AllowToolCalls {
		t.Fatal("expected invalid surface guard to disable tool calls")
	}
	if len(req.CapabilitySurface.ToolNames) != 0 {
		t.Fatalf("expected invalid surface guard to clear surfaced tools, got %#v", req.CapabilitySurface.ToolNames)
	}
	if req.Model.Metadata["capability_surface_guard"] != string(orchestration.SurfaceGuardInvalidSurface) {
		t.Fatalf("expected guard metadata, got %#v", req.Model.Metadata)
	}
}

func TestResolveCapabilitySurface_GuardsUnexpectedEmptySurface(t *testing.T) {
	loop := &AgentLoop{cfg: LoopConfig{ToolRegistry: runtimeGuardTestRegistry(t, runtimeGuardTestTool("alpha"))}}

	req, result, err := loop.resolveCapabilitySurface(context.Background(), orchestration.CanonicalRunRequest{
		Frame: orchestration.ExecutionFrame{Mode: orchestration.ExecutionModeRunExecute},
		CapabilitySurface: orchestration.CapabilitySurface{
			Surface:   orchestration.CapabilitySurfaceRuntime,
			ToolNames: []string{"missing.tool"},
		},
		RequiredOutput: orchestration.RequiredOutput{
			AllowToolCalls: true,
		},
		Model: orchestration.ModelProfile{
			SupportsTools: true,
		},
	})
	if err != nil {
		t.Fatalf("resolveCapabilitySurface: %v", err)
	}
	if result.Guard != orchestration.SurfaceGuardEmptySurface {
		t.Fatalf("guard = %q, want %q", result.Guard, orchestration.SurfaceGuardEmptySurface)
	}
	if !strings.Contains(result.GuardReason, "no tools resolved") {
		t.Fatalf("expected explicit empty-surface reason, got %q", result.GuardReason)
	}
	if req.RequiredOutput.AllowToolCalls {
		t.Fatal("expected empty surface guard to disable tool calls")
	}
	if len(req.CapabilitySurface.ToolNames) != 0 {
		t.Fatalf("expected empty surface guard to clear surfaced tools, got %#v", req.CapabilitySurface.ToolNames)
	}
}

func TestResolveCapabilitySurface_GuardsToolCapableModelRequirement(t *testing.T) {
	loop := &AgentLoop{cfg: LoopConfig{ToolRegistry: runtimeGuardTestRegistry(t, runtimeGuardTestTool("alpha"))}}

	req, result, err := loop.resolveCapabilitySurface(context.Background(), orchestration.CanonicalRunRequest{
		Frame: orchestration.ExecutionFrame{Mode: orchestration.ExecutionModeRunExecute},
		CapabilitySurface: orchestration.CapabilitySurface{
			Surface: orchestration.CapabilitySurfaceRuntime,
		},
		RequiredOutput: orchestration.RequiredOutput{
			AllowToolCalls: true,
		},
		Model: orchestration.ModelProfile{
			SupportsTools: false,
		},
	})
	if err != nil {
		t.Fatalf("resolveCapabilitySurface: %v", err)
	}
	if result.Guard != orchestration.SurfaceGuardToolCapableModelRequired {
		t.Fatalf("guard = %q, want %q", result.Guard, orchestration.SurfaceGuardToolCapableModelRequired)
	}
	if req.RequiredOutput.AllowToolCalls {
		t.Fatal("expected tool-capable-model guard to disable tool calls")
	}
	if len(req.CapabilitySurface.ToolNames) != 0 {
		t.Fatalf("expected tool-capable-model guard to clear surfaced tools, got %#v", req.CapabilitySurface.ToolNames)
	}
	if req.Model.Metadata["capability_surface_guard_reason"] == "" {
		t.Fatalf("expected guard reason metadata, got %#v", req.Model.Metadata)
	}
}

func runtimeGuardTestRegistry(t *testing.T, tools ...*navitool.Tool) *navitool.Registry {
	t.Helper()
	reg := navitool.NewRegistry()
	for _, toolEntry := range tools {
		if err := reg.Register(toolEntry); err != nil {
			t.Fatalf("register %q: %v", toolEntry.Name, err)
		}
	}
	return reg
}

func runtimeGuardTestTool(name string) *navitool.Tool {
	id := "test.runtime_guard." + name
	return &navitool.Tool{
		ToolID:                id,
		DisplayName:           name,
		Description:           "Runtime guard test tool.",
		Source:                navitool.ToolSourceBuiltin,
		SourceID:              "tests.runtime_guard",
		SchemaVersion:         "1.0.0",
		InputSchema:           map[string]any{"type": "object", "properties": map[string]any{}},
		OutputSchema:          map[string]any{"type": "string"},
		Category:              navitool.ToolCategoryReadOnly,
		RiskTier:              "low",
		SideEffects:           []string{},
		Reversibility:         "reversible",
		EnvironmentVisibility: []string{"development", "production"},
		RequiredModes:         []schema.DirectiveMode{},
		RequiredAuthority:     navitool.ToolAuthorityUser,
		FeatureFlags:          []string{},
		ConnectorDependencies: []string{},
		Aliases:               []string{},
		CapabilityTags:        []string{"runtime-guard"},
		Status:                navitool.ToolStatusActive,
		Definition: llm.ToolDefinition{
			Name:        id,
			Description: "Runtime guard test tool.",
			Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		},
	}
}
