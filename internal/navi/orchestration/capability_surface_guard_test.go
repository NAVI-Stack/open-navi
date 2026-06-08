package orchestration

import "testing"

func TestApplySurfaceGuardPolicyPreservesExplicitInvalidGuard(t *testing.T) {
	result := ApplySurfaceGuardPolicy(
		SurfaceResolutionInput{},
		SurfaceResolutionResult{
			Guard:       SurfaceGuardInvalidSurface,
			GuardReason: "unknown capability surface",
		},
	)

	if result.Guard != SurfaceGuardInvalidSurface {
		t.Fatalf("guard = %q, want %q", result.Guard, SurfaceGuardInvalidSurface)
	}
}

func TestApplySurfaceGuardPolicyAllowsLegitimateToollessResolution(t *testing.T) {
	result := ApplySurfaceGuardPolicy(
		SurfaceResolutionInput{
			Surface: CapabilitySurface{Surface: CapabilitySurfaceLoop},
			RequiredOutput: RequiredOutput{
				AllowToolCalls: false,
			},
		},
		SurfaceResolutionResult{
			Resolved:      CapabilitySurface{Surface: CapabilitySurfaceLoop},
			ExpectedTools: false,
			Exclusions:    []SurfaceExclusion{{ToolName: "alpha", Reason: SurfaceExclusionNotRequested}},
			Guard:         SurfaceGuardNone,
			GuardReason:   "",
		},
	)

	if result.IsGuarded() {
		t.Fatalf("expected legitimate toolless resolution to remain unguarded, got %+v", result)
	}
}

func TestApplySurfaceGuardPolicyGuardsUnexpectedEmptyResolution(t *testing.T) {
	result := ApplySurfaceGuardPolicy(
		SurfaceResolutionInput{
			Surface: CapabilitySurface{Surface: CapabilitySurfaceRuntime},
			RequiredOutput: RequiredOutput{
				AllowToolCalls: true,
			},
		},
		SurfaceResolutionResult{
			Resolved:      CapabilitySurface{Surface: CapabilitySurfaceRuntime},
			ExpectedTools: true,
		},
	)

	if result.Guard != SurfaceGuardEmptySurface {
		t.Fatalf("guard = %q, want %q", result.Guard, SurfaceGuardEmptySurface)
	}
}

func TestApplySurfaceGuardPolicyGuardsToolCapableModelRequirement(t *testing.T) {
	result := ApplySurfaceGuardPolicy(
		SurfaceResolutionInput{
			Surface: CapabilitySurface{Surface: CapabilitySurfaceRuntime},
			Model:   ModelProfile{SupportsTools: false},
			RequiredOutput: RequiredOutput{
				AllowToolCalls: true,
			},
		},
		SurfaceResolutionResult{
			Resolved:                 CapabilitySurface{Surface: CapabilitySurfaceRuntime, ToolNames: []string{"alpha"}},
			ExpectedTools:            true,
			RequiresToolCapableModel: true,
		},
	)

	if result.Guard != SurfaceGuardToolCapableModelRequired {
		t.Fatalf("guard = %q, want %q", result.Guard, SurfaceGuardToolCapableModelRequired)
	}
	if len(result.Resolved.ToolNames) != 0 {
		t.Fatalf("guarded result should not expose tools, got %#v", result.Resolved.ToolNames)
	}
}
