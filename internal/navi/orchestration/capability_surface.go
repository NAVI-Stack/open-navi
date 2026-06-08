package orchestration

import (
	"strings"

	navitool "github.com/open-navi/navi/internal/tool"
)

const (
	CapabilitySurfaceLoop    = "loop"
	CapabilitySurfaceRuntime = "runtime"
)

// CapabilitySurface is the request-level contract for tool exposure.
// It does not replace the unified ToolRegistry. It only describes which
// registry-backed surface should be resolved for a given turn/run.
type CapabilitySurface struct {
	Surface         string   `json:"surface,omitempty"`
	ToolNames       []string `json:"tool_names,omitempty"`
	SelectionReason string   `json:"selection_reason,omitempty"`
}

// SkillSurfaceRef is kept as a compatibility alias while Phase 2 migrates to
// CapabilitySurface terminology.
type SkillSurfaceRef = CapabilitySurface

// IsKnownSurface reports whether the capability surface name is part of the
// settled user-facing Phase 2 contract.
func (s CapabilitySurface) IsKnownSurface() bool {
	switch strings.TrimSpace(s.Surface) {
	case CapabilitySurfaceLoop, CapabilitySurfaceRuntime:
		return true
	default:
		return false
	}
}

// SurfaceResolutionInput is the provider-agnostic contract passed into
// capability-surface resolution. The resolver consumes the registry as an
// implementation dependency, not as part of this contract.
type SurfaceResolutionInput struct {
	Surface        CapabilitySurface `json:"surface,omitempty"`
	ExecutionMode  ExecutionMode     `json:"execution_mode,omitempty"`
	UserMessage    string            `json:"user_message,omitempty"`
	RequiredOutput RequiredOutput    `json:"required_output,omitempty"`
	Model          ModelProfile      `json:"model,omitempty"`
}

type CapabilityExposureClass string

const (
	CapabilityExposureUserFacing  CapabilityExposureClass = "user_facing"
	CapabilityExposureInternal    CapabilityExposureClass = "internal"
	CapabilityExposureDevelopment CapabilityExposureClass = "development"
	CapabilityExposureTest        CapabilityExposureClass = "test"
)

type CapabilityInteractionMode string

const (
	CapabilityInteractionConversation CapabilityInteractionMode = "conversation"
	CapabilityInteractionAction       CapabilityInteractionMode = "action"
)

type SurfaceExclusionReason string

const (
	SurfaceExclusionHidden           SurfaceExclusionReason = "hidden"
	SurfaceExclusionInternal         SurfaceExclusionReason = "internal"
	SurfaceExclusionDevelopmentOnly  SurfaceExclusionReason = "development_only"
	SurfaceExclusionTestOnly         SurfaceExclusionReason = "test_only"
	SurfaceExclusionSurfaceMismatch  SurfaceExclusionReason = "surface_mismatch"
	SurfaceExclusionNotRequested     SurfaceExclusionReason = "not_requested"
	SurfaceExclusionConversationOnly SurfaceExclusionReason = "conversation_only"
	SurfaceExclusionActionOnly       SurfaceExclusionReason = "action_only"
	SurfaceExclusionRoutingPolicy    SurfaceExclusionReason = "routing_policy"
	SurfaceExclusionDuplicate        SurfaceExclusionReason = "duplicate"
	SurfaceExclusionUnknownTool      SurfaceExclusionReason = "unknown_tool"
	SurfaceExclusionInvalidMetadata  SurfaceExclusionReason = "invalid_metadata"
)

type SurfaceExclusion struct {
	ToolName string                 `json:"tool_name,omitempty"`
	Reason   SurfaceExclusionReason `json:"reason,omitempty"`
	Detail   string                 `json:"detail,omitempty"`
}

type SurfaceGuardOutcome string

const (
	SurfaceGuardNone                     SurfaceGuardOutcome = ""
	SurfaceGuardInvalidSurface           SurfaceGuardOutcome = "invalid_surface"
	SurfaceGuardEmptySurface             SurfaceGuardOutcome = "empty_surface"
	SurfaceGuardToolCapableModelRequired SurfaceGuardOutcome = "tool_capable_model_required"
)

type SurfaceResolutionResult struct {
	Requested                CapabilitySurface       `json:"requested,omitempty"`
	Resolved                 CapabilitySurface       `json:"resolved,omitempty"`
	Exclusions               []SurfaceExclusion      `json:"exclusions,omitempty"`
	ExpectedTools            bool                    `json:"expected_tools,omitempty"`
	RequiresToolCapableModel bool                    `json:"requires_tool_capable_model,omitempty"`
	Guard                    SurfaceGuardOutcome     `json:"guard,omitempty"`
	GuardReason              string                  `json:"guard_reason,omitempty"`
	ActiveToolSet            *navitool.ActiveToolSet `json:"-"`
}

func (r SurfaceResolutionResult) IsGuarded() bool {
	return r.Guard != SurfaceGuardNone
}
