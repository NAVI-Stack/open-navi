package orchestration

import "fmt"

// ApplySurfaceGuardPolicy enforces the settled fail-closed guard rules over a
// completed capability-surface resolution result. It does not widen or retry;
// it only converts invalid or unsafe resolution states into explicit guards.
func ApplySurfaceGuardPolicy(input SurfaceResolutionInput, result SurfaceResolutionResult) SurfaceResolutionResult {
	if result.IsGuarded() {
		return result
	}

	if len(result.Resolved.ToolNames) == 0 {
		if result.ExpectedTools {
			result.Guard = SurfaceGuardEmptySurface
			result.GuardReason = fmt.Sprintf("no tools resolved for surface %q", result.Resolved.Surface)
		}
		return result
	}

	if result.ExpectedTools && result.RequiresToolCapableModel && !input.Model.SupportsTools {
		result.Resolved.ToolNames = nil
		result.Guard = SurfaceGuardToolCapableModelRequired
		result.GuardReason = "resolved surface requires a tool-capable model"
	}

	return result
}
