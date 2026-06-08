package orchestration

import "strings"

// RoutingSurfacePolicyInput captures the settled Phase 2 interaction between an
// already-resolved capability surface and a routing decision. Routing may
// preserve the surface, strip tools, or trigger an explicit guard. It may not
// widen tool exposure.
type RoutingSurfacePolicyInput struct {
	Surface        CapabilitySurface
	RequiredOutput RequiredOutput
	Decision       RoutingSurfaceDecision
}

// RoutingSurfaceDecision is the narrow routing contract NCOS needs for
// capability-surface policy enforcement. It intentionally does not include any
// tool-addition path.
type RoutingSurfaceDecision struct {
	StripTools bool
	Reason     string
}

// RoutingSurfacePolicyResult is the post-policy surface state. The result is
// deterministic and fail-closed: it can preserve, strip, or guard, but never
// widen beyond the already-resolved tool set.
type RoutingSurfacePolicyResult struct {
	Surface        CapabilitySurface
	RequiredOutput RequiredOutput
	Guard          SurfaceGuardOutcome
	GuardReason    string
	Policy         string
}

// ApplyRoutingSurfacePolicy enforces the settled Phase 2 routing interaction
// rules over a resolved capability surface. Routing may preserve the current
// surface, strip tools, or trigger a guard, but may never widen exposure.
func ApplyRoutingSurfacePolicy(input RoutingSurfacePolicyInput) RoutingSurfacePolicyResult {
	result := RoutingSurfacePolicyResult{
		Surface: CapabilitySurface{
			Surface:         strings.TrimSpace(input.Surface.Surface),
			ToolNames:       append([]string(nil), input.Surface.ToolNames...),
			SelectionReason: strings.TrimSpace(input.Surface.SelectionReason),
		},
		RequiredOutput: input.RequiredOutput,
		Policy:         "preserve",
	}

	if strings.EqualFold(strings.TrimSpace(input.Decision.Reason), "no_tool_capable_model") {
		result.Surface.ToolNames = nil
		result.RequiredOutput.AllowToolCalls = false
		result.Guard = SurfaceGuardToolCapableModelRequired
		result.GuardReason = "resolved surface requires a tool-capable model"
		result.Policy = "guard"
		return result
	}

	if input.Decision.StripTools {
		essential := filterEssentialTools(result.Surface.ToolNames)
		if len(essential) > 0 {
			result.Surface.ToolNames = essential
			result.RequiredOutput.AllowToolCalls = true
			result.RequiredOutput.AllowScheduledReplies = true
			result.Surface.SelectionReason = "stripped to essential tools only"
			result.Policy = "strip_partial"
		} else {
			result.Surface.ToolNames = nil
			result.RequiredOutput.AllowToolCalls = false
			result.Surface.SelectionReason = routingSurfaceFirstNonEmpty(
				"tool surface stripped by routing policy",
				result.Surface.SelectionReason,
			)
			result.Policy = "strip"
		}
	}

	return result
}

// essentialToolNames lists tools that survive a routing-policy strip because
// they provide core messaging functionality most models can use correctly.
var essentialToolNames = map[string]bool{
	"navi.messaging.send_reply": true,
}

func filterEssentialTools(names []string) []string {
	var out []string
	for _, name := range names {
		if essentialToolNames[strings.TrimSpace(name)] {
			out = append(out, name)
		}
	}
	return out
}

func routingSurfaceFirstNonEmpty(parts ...string) string {
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
