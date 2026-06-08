package tool

import (
	"strings"

	"github.com/open-navi/navi/internal/schema"
)

// DiscoverySessionMode captures the current discovery surface / session mode.
type DiscoverySessionMode string

const (
	DiscoverySessionModeCompanion DiscoverySessionMode = "companion"
	DiscoverySessionModeAssistant DiscoverySessionMode = "assistant"
	DiscoverySessionModeCoder     DiscoverySessionMode = "coder"
	DiscoverySessionModeDebug     DiscoverySessionMode = "debug"
	DiscoverySessionModeAdmin     DiscoverySessionMode = "admin"
)

// DiscoveryContext supplies the policy inputs that shape discovery visibility.
type DiscoveryContext struct {
	Environment      string
	SessionMode      DiscoverySessionMode
	Authority        ToolAuthority
	FeatureFlags     []string
	MaximumRiskTier  string
	MinimumTrustTier string
}

func (ctx DiscoveryContext) normalize() DiscoveryContext {
	normalized := ctx
	normalized.Environment = strings.TrimSpace(strings.ToLower(normalized.Environment))
	if normalized.Environment == "" {
		normalized.Environment = "production"
	}
	if normalized.SessionMode == "" {
		normalized.SessionMode = DiscoverySessionModeAssistant
	}
	if normalized.Authority == "" {
		normalized.Authority = ToolAuthorityUser
	}
	return normalized
}

func (ctx DiscoveryContext) canSeeUnavailableMatches() bool {
	ctx = ctx.normalize()
	return ctx.Authority == ToolAuthorityAdmin ||
		ctx.Authority == ToolAuthoritySystem ||
		ctx.SessionMode == DiscoverySessionModeDebug ||
		ctx.SessionMode == DiscoverySessionModeAdmin
}

func (ctx DiscoveryContext) canRevealHiddenExactMatches() bool {
	ctx = ctx.normalize()
	return ctx.Authority == ToolAuthorityAdmin ||
		ctx.Authority == ToolAuthoritySystem ||
		ctx.SessionMode == DiscoverySessionModeDebug ||
		ctx.SessionMode == DiscoverySessionModeAdmin
}

func evaluateDiscoveryPolicy(toolEntry *Tool, ctx DiscoveryContext) (DiscoveryAvailabilityStatus, DiscoveryReasonCode) {
	if toolEntry == nil {
		return DiscoveryAvailabilityExplicitMiss, DiscoveryReasonNoMatch
	}
	ctx = ctx.normalize()

	if toolEntry.Hidden {
		if ctx.canRevealHiddenExactMatches() {
			return DiscoveryAvailabilityHidden, DiscoveryReasonHidden
		}
		return DiscoveryAvailabilityHidden, DiscoveryReasonHiddenByPolicy
	}
	switch toolEntry.Status {
	case ToolStatusSuspended:
		if ctx.canSeeUnavailableMatches() {
			return DiscoveryAvailabilityVisibleUnavailable, DiscoveryReasonSuspended
		}
		return DiscoveryAvailabilityHidden, DiscoveryReasonHiddenByPolicy
	case ToolStatusRemoved:
		if ctx.canSeeUnavailableMatches() {
			return DiscoveryAvailabilityVisibleUnavailable, DiscoveryReasonRemoved
		}
		return DiscoveryAvailabilityHidden, DiscoveryReasonHiddenByPolicy
	case ToolStatusInvalid:
		if ctx.canSeeUnavailableMatches() {
			return DiscoveryAvailabilityVisibleUnavailable, DiscoveryReasonInvalid
		}
		return DiscoveryAvailabilityHidden, DiscoveryReasonHiddenByPolicy
	}

	if !toolVisibleInEnvironment(toolEntry, ctx.Environment) {
		if ctx.canSeeUnavailableMatches() {
			return DiscoveryAvailabilityVisibleUnavailable, DiscoveryReasonRequiresEnvironment
		}
		return DiscoveryAvailabilityHidden, DiscoveryReasonHiddenByPolicy
	}

	if !authoritySatisfies(ctx.Authority, toolEntry.RequiredAuthority) {
		if toolEntry.Category == ToolCategoryAdminCritical && ctx.Authority == ToolAuthorityUser {
			return DiscoveryAvailabilityHidden, DiscoveryReasonHiddenByPolicy
		}
		if ctx.canSeeUnavailableMatches() {
			return DiscoveryAvailabilityVisibleUnavailable, DiscoveryReasonRequiresAuthority
		}
		return DiscoveryAvailabilityHidden, DiscoveryReasonHiddenByPolicy
	}

	if !featureFlagsSatisfied(toolEntry.FeatureFlags, ctx.FeatureFlags) {
		if ctx.canSeeUnavailableMatches() {
			return DiscoveryAvailabilityVisibleUnavailable, DiscoveryReasonRequiresFeatureFlag
		}
		return DiscoveryAvailabilityHidden, DiscoveryReasonHiddenByPolicy
	}

	if !riskTierAllowed(toolEntry.RiskTier, ctx.MaximumRiskTier) {
		if ctx.canSeeUnavailableMatches() {
			return DiscoveryAvailabilityVisibleUnavailable, DiscoveryReasonDisallowedRiskTier
		}
		return DiscoveryAvailabilityHidden, DiscoveryReasonHiddenByPolicy
	}

	if !trustTierAllowed(toolEntry.Metadata.TrustTier, ctx.MinimumTrustTier) {
		if ctx.canSeeUnavailableMatches() {
			return DiscoveryAvailabilityVisibleUnavailable, DiscoveryReasonDisallowedTrustTier
		}
		return DiscoveryAvailabilityHidden, DiscoveryReasonHiddenByPolicy
	}

	if !modeAllowsTool(toolEntry, ctx) {
		if ctx.canSeeUnavailableMatches() {
			return DiscoveryAvailabilityVisibleUnavailable, DiscoveryReasonRequiresMode
		}
		return DiscoveryAvailabilityHidden, DiscoveryReasonHiddenByPolicy
	}

	return DiscoveryAvailabilityAvailable, DiscoveryReasonNone
}

func toolVisibleInEnvironment(toolEntry *Tool, environment string) bool {
	if toolEntry == nil {
		return false
	}
	environment = strings.TrimSpace(strings.ToLower(environment))
	if environment == "" {
		return true
	}
	for _, candidate := range toolEntry.EnvironmentVisibility {
		if strings.TrimSpace(strings.ToLower(candidate)) == environment {
			return true
		}
	}
	return false
}

func authoritySatisfies(current, required ToolAuthority) bool {
	return authorityRank(current) >= authorityRank(required)
}

func authorityRank(authority ToolAuthority) int {
	switch authority {
	case ToolAuthoritySystem:
		return 4
	case ToolAuthorityAdmin:
		return 3
	case ToolAuthorityOwner:
		return 2
	default:
		return 1
	}
}

func featureFlagsSatisfied(required []string, enabled []string) bool {
	if len(required) == 0 {
		return true
	}
	enabledSet := make(map[string]struct{}, len(enabled))
	for _, flag := range enabled {
		flag = strings.TrimSpace(flag)
		if flag == "" {
			continue
		}
		enabledSet[flag] = struct{}{}
	}
	for _, flag := range required {
		flag = strings.TrimSpace(flag)
		if flag == "" {
			continue
		}
		if _, ok := enabledSet[flag]; !ok {
			return false
		}
	}
	return true
}

func riskTierAllowed(toolRiskTier string, maxRiskTier string) bool {
	maxRank := riskTierRank(maxRiskTier)
	if maxRank == 0 {
		return true
	}
	toolRank := riskTierRank(toolRiskTier)
	if toolRank == 0 {
		return true
	}
	return toolRank <= maxRank
}

func riskTierRank(value string) int {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "critical", "tier4", "4":
		return 5
	case "high", "tier3", "3":
		return 4
	case "medium", "tier2", "2":
		return 3
	case "low", "tier1", "1":
		return 2
	case "safe", "informational", "tier0", "0":
		return 1
	default:
		return 0
	}
}

func trustTierAllowed(toolTrustTier string, minTrustTier string) bool {
	requiredRank := trustTierRank(minTrustTier)
	if requiredRank == 0 {
		return true
	}
	toolRank := trustTierRank(toolTrustTier)
	if toolRank == 0 {
		return false
	}
	return toolRank >= requiredRank
}

func trustTierRank(value string) int {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "instruction_capable_internal":
		return 5
	case "trusted_internal":
		return 4
	case "trusted_local":
		return 3
	case "untrusted_user_generated":
		return 2
	case "untrusted_external":
		return 1
	default:
		return 0
	}
}

func modeAllowsTool(toolEntry *Tool, ctx DiscoveryContext) bool {
	if toolEntry == nil {
		return false
	}
	switch ctx.SessionMode {
	case DiscoverySessionModeCompanion:
		if toolEntry.Category == ToolCategoryChatSafe || toolEntry.Category == ToolCategoryReadOnly {
			return toolEntry.NormalizedExposureClass() == ToolExposureUserFacing
		}
		return false
	case DiscoverySessionModeAssistant:
		if !directiveModesAllow(toolEntry.RequiredModes, schema.DirectiveModeAssist, schema.DirectiveModeChat, schema.DirectiveModeAdvise) {
			return false
		}
		return toolEntry.NormalizedExposureClass() == ToolExposureUserFacing
	case DiscoverySessionModeCoder:
		if !directiveModesAllow(toolEntry.RequiredModes, schema.DirectiveModeAct, schema.DirectiveModeAssist) {
			return false
		}
		switch toolEntry.NormalizedExposureClass() {
		case ToolExposureUserFacing, ToolExposureDevelopment:
			return true
		default:
			return toolEntry.Category == ToolCategoryDevTest
		}
	case DiscoverySessionModeDebug:
		if !directiveModesAllow(toolEntry.RequiredModes, schema.DirectiveModeAct, schema.DirectiveModeAssist, schema.DirectiveModeWatch) {
			return false
		}
		return true
	case DiscoverySessionModeAdmin:
		return true
	default:
		return false
	}
}

func directiveModesAllow(required []schema.DirectiveMode, allowed ...schema.DirectiveMode) bool {
	if len(required) == 0 {
		return true
	}
	allowedSet := make(map[schema.DirectiveMode]struct{}, len(allowed))
	for _, mode := range allowed {
		allowedSet[mode] = struct{}{}
	}
	for _, mode := range required {
		if _, ok := allowedSet[mode]; ok {
			return true
		}
	}
	return false
}
