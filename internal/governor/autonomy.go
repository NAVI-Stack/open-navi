package governor

import (
	"github.com/open-navi/navi/internal/navi/skill"
	"github.com/open-navi/navi/internal/schema"
)

// Hard floor reasons: when any of these apply, autonomy cannot upgrade RequiresConfirmation to Approved.
const (
	HardFloorSystemPolicy     = "system_policy"
	HardFloorOwnerSet         = "owner_set"         // skill requires_confirmation (owner-set)
	HardFloorProposalRequired = "proposal_required" // category requires proposal
	HardFloorIrreversible     = "irreversible"      // reversibility is irreversible
	HardFloorRiskOverride     = "risk_override"     // risk tier high + policy
)

// Autonomy domains used for per-domain presets. These strings are the
// canonical keys for autonomy.domain_overrides in config.
const (
	AutonomyDomainMessaging     = "messaging"
	AutonomyDomainScheduling    = "scheduling"
	AutonomyDomainDelegation    = "delegation"
	AutonomyDomainWorkflow      = "workflow"
	AutonomyDomainCoding        = "coding"
	AutonomyDomainMemory        = "memory"
	AutonomyDomainPlugins       = "plugins"
	AutonomyDomainConfiguration = "configuration"
	AutonomyDomainArtifact      = "artifact"
)

// HardFloorReason returns a non-empty reason if the skill entry triggers a hard floor.
// When non-empty, autonomy must not auto-approve (keep RequiresConfirmation or reject).
func HardFloorReason(entry *skill.SkillEntry) string {
	if entry == nil || entry.Spec == nil {
		return ""
	}
	spec := entry.Spec
	if spec.Effects.RequiresConfirmation {
		return HardFloorOwnerSet
	}
	if spec.Effects.Reversibility == "irreversible" {
		return HardFloorIrreversible
	}
	if spec.Effects.RiskTier == "high" {
		return HardFloorRiskOverride
	}
	return ""
}

// AutonomyPresetResolver is implemented by config.AutonomyConfig so the governor
// can resolve effective preset per domain without depending on config package.
type AutonomyPresetResolver interface {
	EffectivePreset(domain string) string
}

// ExecutionThresholdResolver is optional. When implemented, ApplyAutonomy uses
// EffectiveExecutionThreshold(domain) instead of EffectivePreset(domain) to
// decide whether to upgrade RequiresConfirmation to Approved. This allows
// per-dimension overrides (e.g. domain_dimension_overrides) to be respected.
type ExecutionThresholdResolver interface {
	EffectiveExecutionThreshold(domain string) string
}

// DomainForCommand returns the autonomy domain for a command type. When
// explicitDomain is non-empty it wins; otherwise a stable mapping from
// schema.CommandType → domain is applied.
func DomainForCommand(cmdType schema.CommandType, explicitDomain string) string {
	if explicitDomain != "" {
		return explicitDomain
	}
	switch cmdType {
	case schema.CommandTypeSend:
		return AutonomyDomainMessaging
	case schema.CommandTypeSchedule:
		return AutonomyDomainScheduling
	case schema.CommandTypeDelegate:
		return AutonomyDomainDelegation
	case schema.CommandTypeCompose:
		return AutonomyDomainWorkflow
	case schema.CommandTypeAcquire:
		return AutonomyDomainPlugins
	case schema.CommandTypeQuery,
		schema.CommandTypeCreate,
		schema.CommandTypeUpdate,
		schema.CommandTypeDelete,
		schema.CommandTypeInvoke:
		return AutonomyDomainCoding
	default:
		return AutonomyDomainCoding
	}
}

// ApplyAutonomy applies autonomy to a validation result: when result is RequiresConfirmation,
// no hard floor applies, and effective execution threshold is "high", returns Approved;
// otherwise returns result unchanged. Governor defines what is permitted (permissions,
// policy, budgets, hard floors); Autonomy only downgrades RequiresConfirmation → Approved
// within those bounds when the threshold is high. See docs/concepts/autonomy-dimensions.md.
func ApplyAutonomy(result ValidationResult, resolver AutonomyPresetResolver, domain string, entry *skill.SkillEntry) ValidationResult {
	if result.Outcome != ValidationRequiresConfirmation {
		return result
	}
	if resolver == nil {
		return result
	}
	if HardFloorReason(entry) != "" {
		return result
	}
	threshold := resolver.EffectivePreset(domain)
	if r, ok := resolver.(ExecutionThresholdResolver); ok {
		threshold = r.EffectiveExecutionThreshold(domain)
	}
	if threshold != "high" {
		return result
	}
	return ValidationResult{Outcome: ValidationApproved}
}

// ApplyAutonomyForAction is a higher-level helper that first resolves the
// autonomy domain for a command (using DomainForCommand) and then delegates
// to ApplyAutonomy. It is agnostic to the caller (skills, workers, gateway).
func ApplyAutonomyForAction(result ValidationResult, resolver AutonomyPresetResolver, cmdType schema.CommandType, domain string, entry *skill.SkillEntry) ValidationResult {
	resolvedDomain := DomainForCommand(cmdType, domain)
	return ApplyAutonomy(result, resolver, resolvedDomain, entry)
}

