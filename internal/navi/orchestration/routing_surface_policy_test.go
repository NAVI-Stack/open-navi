package orchestration

import "testing"

func TestApplyRoutingSurfacePolicyPreservesResolvedSurface(t *testing.T) {
	input := RoutingSurfacePolicyInput{
		Surface: CapabilitySurface{
			Surface:         CapabilitySurfaceRuntime,
			ToolNames:       []string{"tool_a", "tool_b"},
			SelectionReason: "resolved by capability policy",
		},
		RequiredOutput: RequiredOutput{AllowToolCalls: true},
		Decision:       RoutingSurfaceDecision{},
	}

	result := ApplyRoutingSurfacePolicy(input)
	if result.Policy != "preserve" {
		t.Fatalf("policy = %q, want preserve", result.Policy)
	}
	if result.Guard != SurfaceGuardNone {
		t.Fatalf("guard = %q, want none", result.Guard)
	}
	if len(result.Surface.ToolNames) != 2 || result.Surface.ToolNames[0] != "tool_a" || result.Surface.ToolNames[1] != "tool_b" {
		t.Fatalf("surface tool names = %#v, want preserved subset", result.Surface.ToolNames)
	}
}

func TestApplyRoutingSurfacePolicyStripsWithoutWidening(t *testing.T) {
	input := RoutingSurfacePolicyInput{
		Surface: CapabilitySurface{
			Surface:         CapabilitySurfaceLoop,
			ToolNames:       []string{"tool_a"},
			SelectionReason: "resolved by capability policy",
		},
		RequiredOutput: RequiredOutput{AllowToolCalls: true},
		Decision: RoutingSurfaceDecision{
			StripTools: true,
		},
	}

	result := ApplyRoutingSurfacePolicy(input)
	if result.Policy != "strip" {
		t.Fatalf("policy = %q, want strip", result.Policy)
	}
	if result.Guard != SurfaceGuardNone {
		t.Fatalf("guard = %q, want none", result.Guard)
	}
	if len(result.Surface.ToolNames) != 0 {
		t.Fatalf("surface tool names = %#v, want stripped empty set", result.Surface.ToolNames)
	}
	if result.RequiredOutput.AllowToolCalls {
		t.Fatal("expected tool calls to be disabled after strip")
	}
}

func TestApplyRoutingSurfacePolicyStripPartialPreservesEssentialTools(t *testing.T) {
	input := RoutingSurfacePolicyInput{
		Surface: CapabilitySurface{
			Surface:         CapabilitySurfaceRuntime,
			ToolNames:       []string{"tool_a", "navi.messaging.send_reply", "tool_b"},
			SelectionReason: "resolved by capability policy",
		},
		RequiredOutput: RequiredOutput{AllowToolCalls: true},
		Decision: RoutingSurfaceDecision{
			StripTools: true,
		},
	}

	result := ApplyRoutingSurfacePolicy(input)
	if result.Policy != "strip_partial" {
		t.Fatalf("policy = %q, want strip_partial", result.Policy)
	}
	if len(result.Surface.ToolNames) != 1 || result.Surface.ToolNames[0] != "navi.messaging.send_reply" {
		t.Fatalf("surface tool names = %#v, want only send_reply", result.Surface.ToolNames)
	}
	if !result.RequiredOutput.AllowToolCalls {
		t.Fatal("expected tool calls to remain enabled for essential tools")
	}
	if !result.RequiredOutput.AllowScheduledReplies {
		t.Fatal("expected scheduled replies to be enabled for essential tools")
	}
}

func TestApplyRoutingSurfacePolicyGuardsToolCapableModelRequirement(t *testing.T) {
	input := RoutingSurfacePolicyInput{
		Surface: CapabilitySurface{
			Surface:   CapabilitySurfaceRuntime,
			ToolNames: []string{"tool_a"},
		},
		RequiredOutput: RequiredOutput{AllowToolCalls: true},
		Decision: RoutingSurfaceDecision{
			Reason: "no_tool_capable_model",
		},
	}

	result := ApplyRoutingSurfacePolicy(input)
	if result.Policy != "guard" {
		t.Fatalf("policy = %q, want guard", result.Policy)
	}
	if result.Guard != SurfaceGuardToolCapableModelRequired {
		t.Fatalf("guard = %q, want %q", result.Guard, SurfaceGuardToolCapableModelRequired)
	}
	if len(result.Surface.ToolNames) != 0 {
		t.Fatalf("surface tool names = %#v, want guarded empty set", result.Surface.ToolNames)
	}
	if result.GuardReason == "" {
		t.Fatal("expected explicit guard reason")
	}
}
