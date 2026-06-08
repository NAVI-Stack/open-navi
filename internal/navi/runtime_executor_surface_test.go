package navi

import (
	"strings"
	"testing"

	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/navi/inference"
	"github.com/open-navi/navi/internal/navi/orchestration"
)

func TestApplyInferenceModelDirectiveCannotWidenResolvedRuntimeSurface(t *testing.T) {
	req := orchestration.CanonicalRunRequest{
		RequiredOutput: orchestration.RequiredOutput{AllowToolCalls: true},
		CapabilitySurface: orchestration.CapabilitySurface{
			Surface:         orchestration.CapabilitySurfaceRuntime,
			ToolNames:       []string{"resolved.alpha"},
			SelectionReason: "resolver output",
		},
	}

	got := applyInferenceModelDirective(req, &inference.ModelDirective{
		AllowToolCalls: true,
		ToolNames:      []string{"resolved.alpha", "unsurfaced.beta"},
	})

	if len(got.CapabilitySurface.ToolNames) != 1 || got.CapabilitySurface.ToolNames[0] != "resolved.alpha" {
		t.Fatalf("directive widened resolved surface to %#v", got.CapabilitySurface.ToolNames)
	}
	if !got.RequiredOutput.AllowToolCalls {
		t.Fatal("expected tool calls to remain allowed when at least one directive tool was already resolved")
	}
	if !strings.Contains(got.CapabilitySurface.SelectionReason, "ics model directive narrowed tools to resolved.alpha") {
		t.Fatalf("selection reason did not describe narrowed resolved tools: %q", got.CapabilitySurface.SelectionReason)
	}
}

func TestApplyInferenceModelDirectiveDisablesToolsWhenDirectiveHasNoResolvedTool(t *testing.T) {
	req := orchestration.CanonicalRunRequest{
		RequiredOutput: orchestration.RequiredOutput{AllowToolCalls: true},
		CapabilitySurface: orchestration.CapabilitySurface{
			Surface:         orchestration.CapabilitySurfaceRuntime,
			ToolNames:       []string{"resolved.alpha"},
			SelectionReason: "resolver output",
		},
	}

	got := applyInferenceModelDirective(req, &inference.ModelDirective{
		AllowToolCalls: true,
		ToolNames:      []string{"unsurfaced.beta"},
	})

	if got.RequiredOutput.AllowToolCalls {
		t.Fatal("expected tool calls to be disabled when directive only names tools outside the resolved surface")
	}
	if len(got.CapabilitySurface.ToolNames) != 0 {
		t.Fatalf("expected no surfaced tools, got %#v", got.CapabilitySurface.ToolNames)
	}
	if !strings.Contains(got.CapabilitySurface.SelectionReason, "ics model directive requested no tools in the resolved capability surface") {
		t.Fatalf("selection reason did not explain blocked widening: %q", got.CapabilitySurface.SelectionReason)
	}
}

func TestApplyInferenceModelDirectivePreservesResolvedSurfaceWhenDirectiveToolListEmpty(t *testing.T) {
	req := orchestration.CanonicalRunRequest{
		RequiredOutput: orchestration.RequiredOutput{AllowToolCalls: true},
		CapabilitySurface: orchestration.CapabilitySurface{
			Surface:         orchestration.CapabilitySurfaceRuntime,
			ToolNames:       []string{"navi.render.visualize", "navi.messaging.send_reply"},
			SelectionReason: "resolver output",
		},
	}

	got := applyInferenceModelDirective(req, &inference.ModelDirective{
		AllowToolCalls: true,
	})

	if !got.RequiredOutput.AllowToolCalls {
		t.Fatal("expected tool calls to remain allowed when directive permits tools without narrowing")
	}
	if strings.Join(got.CapabilitySurface.ToolNames, ",") != "navi.render.visualize,navi.messaging.send_reply" {
		t.Fatalf("expected resolved tool surface to be preserved, got %#v", got.CapabilitySurface.ToolNames)
	}
	if !strings.Contains(got.CapabilitySurface.SelectionReason, "ics model directive preserved resolved tools") {
		t.Fatalf("selection reason did not describe preserved tools: %q", got.CapabilitySurface.SelectionReason)
	}
}

func TestApplyCompiledInferenceMetadataDoesNotForceUnsufacedToolChoice(t *testing.T) {
	compiled := orchestration.CompiledModelRequest{
		Tools: []llm.ToolDefinition{{Name: "resolved.alpha"}},
	}
	decision := inference.DecisionEnvelope{
		ModelDirective: &inference.ModelDirective{
			AllowToolCalls: true,
			ToolNames:      []string{"resolved.alpha"},
			ToolChoice:     "unsurfaced.beta",
		},
	}

	got := applyCompiledInferenceMetadata(compiled, decision)

	if got.Options.ToolChoice != "" {
		t.Fatalf("compiled request forced unsurfaced tool choice %q", got.Options.ToolChoice)
	}
	if got.Metadata["ics_target_capability"] == "unsurfaced.beta" {
		t.Fatalf("metadata target capability used unsurfaced tool: %#v", got.Metadata)
	}
}
