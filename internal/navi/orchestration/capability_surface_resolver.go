package orchestration

import (
	"fmt"
	"strings"

	navitool "github.com/ceoai/navi/internal/tool"
)

// CapabilitySurfaceResolver resolves a request-scoped capability surface over
// the unified tool registry. It does not mutate the registry or widen beyond
// the requested surface.
type CapabilitySurfaceResolver struct {
	registry *navitool.Registry
}

// NewCapabilitySurfaceResolver creates the authoritative NCOS capability
// surface resolver over the unified tool registry.
func NewCapabilitySurfaceResolver(registry *navitool.Registry) *CapabilitySurfaceResolver {
	return &CapabilitySurfaceResolver{registry: registry}
}

// Resolve deterministically selects which registered tools are eligible for the
// requested capability surface and records explicit exclusions for the rest.
func (r *CapabilitySurfaceResolver) Resolve(input SurfaceResolutionInput) SurfaceResolutionResult {
	result := SurfaceResolutionResult{
		ExpectedTools: capabilitySurfaceExpectsTools(input),
		Requested:     cloneCapabilitySurface(input.Surface),
		Resolved: CapabilitySurface{
			Surface:         strings.TrimSpace(input.Surface.Surface),
			SelectionReason: strings.TrimSpace(input.Surface.SelectionReason),
		},
	}

	surfaceName := strings.TrimSpace(input.Surface.Surface)
	if surfaceName == "" {
		result.Guard = SurfaceGuardInvalidSurface
		result.GuardReason = fmt.Sprintf("unknown capability surface %q", surfaceName)
		return result
	}
	if !(CapabilitySurface{Surface: surfaceName}).IsKnownSurface() {
		result.Guard = SurfaceGuardInvalidSurface
		result.GuardReason = fmt.Sprintf("unknown capability surface %q", surfaceName)
		return result
	}
	if r == nil || r.registry == nil {
		result.Guard = SurfaceGuardInvalidSurface
		result.GuardReason = "tool registry unavailable"
		return result
	}

	requestedNames, preflightExclusions, invalidRequest := normalizeRequestedToolNames(input.Surface.ToolNames)
	result.Exclusions = append(result.Exclusions, preflightExclusions...)
	if invalidRequest != "" {
		result.Guard = SurfaceGuardInvalidSurface
		result.GuardReason = invalidRequest
		return result
	}

	interactionMode := executionInteractionMode(input.ExecutionMode)
	included := make([]string, 0)
	requireToolCapableModel := false

	for _, toolEntry := range r.registry.List() {
		if toolEntry == nil {
			continue
		}

		if len(requestedNames) > 0 {
			if _, ok := requestedNames[toolEntry.Name]; !ok {
				result.Exclusions = append(result.Exclusions, SurfaceExclusion{
					ToolName: toolEntry.Name,
					Reason:   SurfaceExclusionNotRequested,
				})
				continue
			}
		}

		if toolEntry.Hidden {
			result.Exclusions = append(result.Exclusions, SurfaceExclusion{
				ToolName: toolEntry.Name,
				Reason:   SurfaceExclusionHidden,
			})
			continue
		}

		if toolEntry.HasInvalidExposureClass() || toolEntry.HasInvalidInteractionModes() {
			result.Exclusions = append(result.Exclusions, SurfaceExclusion{
				ToolName: toolEntry.Name,
				Reason:   SurfaceExclusionInvalidMetadata,
			})
			continue
		}

		switch toolEntry.NormalizedExposureClass() {
		case navitool.ToolExposureInternal:
			result.Exclusions = append(result.Exclusions, SurfaceExclusion{
				ToolName: toolEntry.Name,
				Reason:   SurfaceExclusionInternal,
			})
			continue
		case navitool.ToolExposureDevelopment:
			result.Exclusions = append(result.Exclusions, SurfaceExclusion{
				ToolName: toolEntry.Name,
				Reason:   SurfaceExclusionDevelopmentOnly,
			})
			continue
		case navitool.ToolExposureTest:
			result.Exclusions = append(result.Exclusions, SurfaceExclusion{
				ToolName: toolEntry.Name,
				Reason:   SurfaceExclusionTestOnly,
			})
			continue
		}

		if !navitool.VisibleOnSurface(toolEntry, surfaceName) {
			result.Exclusions = append(result.Exclusions, SurfaceExclusion{
				ToolName: toolEntry.Name,
				Reason:   SurfaceExclusionSurfaceMismatch,
			})
			continue
		}

		if interactionMode != "" && !toolEntry.SupportsInteractionMode(interactionMode) {
			reason := SurfaceExclusionActionOnly
			if interactionMode == navitool.ToolInteractionAction {
				reason = SurfaceExclusionConversationOnly
			}
			result.Exclusions = append(result.Exclusions, SurfaceExclusion{
				ToolName: toolEntry.Name,
				Reason:   reason,
			})
			continue
		}

		if toolEntry.RequiresToolCapableModel() {
			requireToolCapableModel = true
		}
		included = append(included, toolEntry.Name)
	}

	if len(requestedNames) > 0 {
		for _, requestedName := range orderedRequestedToolNames(input.Surface.ToolNames) {
			if _, ok := requestedNames[requestedName]; ok {
				if _, found := r.registry.Lookup(requestedName); !found {
					result.Exclusions = append(result.Exclusions, SurfaceExclusion{
						ToolName: requestedName,
						Reason:   SurfaceExclusionUnknownTool,
					})
				}
			}
		}
	}

	result.RequiresToolCapableModel = requireToolCapableModel
	result.Resolved.ToolNames = included
	return result
}

func normalizeRequestedToolNames(raw []string) (map[string]struct{}, []SurfaceExclusion, string) {
	if len(raw) == 0 {
		return nil, nil, ""
	}

	requested := make(map[string]struct{}, len(raw))
	exclusions := make([]SurfaceExclusion, 0)
	for _, candidate := range raw {
		name := strings.TrimSpace(candidate)
		if name == "" {
			return nil, exclusions, "requested tool names must be non-empty"
		}
		if _, exists := requested[name]; exists {
			exclusions = append(exclusions, SurfaceExclusion{
				ToolName: name,
				Reason:   SurfaceExclusionDuplicate,
			})
			continue
		}
		requested[name] = struct{}{}
	}
	return requested, exclusions, ""
}

func orderedRequestedToolNames(raw []string) []string {
	out := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, candidate := range raw {
		name := strings.TrimSpace(candidate)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func executionInteractionMode(mode ExecutionMode) navitool.ToolInteractionMode {
	switch mode {
	case ExecutionModeChatTurn:
		return navitool.ToolInteractionConversation
	case ExecutionModeRunExecute, ExecutionModeResume:
		return navitool.ToolInteractionAction
	default:
		return ""
	}
}

func cloneCapabilitySurface(surface CapabilitySurface) CapabilitySurface {
	return CapabilitySurface{
		Surface:         strings.TrimSpace(surface.Surface),
		ToolNames:       append([]string(nil), surface.ToolNames...),
		SelectionReason: strings.TrimSpace(surface.SelectionReason),
	}
}

func capabilitySurfaceExpectsTools(input SurfaceResolutionInput) bool {
	return input.RequiredOutput.AllowToolCalls || len(input.Surface.ToolNames) > 0
}
