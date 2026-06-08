package governor

import (
	"context"
	"strings"

	"github.com/open-navi/navi/internal/navi/skill"
	"github.com/open-navi/navi/internal/schema"
)

func ownerDisableReason(tags map[string]string, entries []schema.ConfigurationEntry, domain string) string {
	if tags != nil {
		if v, ok := tags["owner_disabled"]; ok && v == "true" {
			return "Action disabled by owner configuration"
		}
		if domain != "" {
			if v, ok := tags["domain_"+domain+"_disabled"]; ok && v == "true" {
				return "Domain disabled by owner configuration"
			}
		}
	}
	for _, e := range entries {
		if e.Value != "true" {
			continue
		}
		if e.Key == "action_disabled" {
			return "Action disabled by owner configuration"
		}
		if domain != "" && e.Key == "domain_"+domain+"_disabled" {
			return "Domain disabled by owner configuration"
		}
	}
	return ""
}

func requiresIntentChangeConfirmation(a ActionDescriptor) bool {
	if isIntentChangeConfirmationAllowlisted(a) {
		return false
	}
	if hasIntentChangeSideEffectTag(a.Tags) {
		return true
	}
	switch a.CommandType {
	case schema.CommandTypeCreate,
		schema.CommandTypeUpdate,
		schema.CommandTypeDelete,
		schema.CommandTypeInvoke,
		schema.CommandTypeSend,
		schema.CommandTypeSchedule,
		schema.CommandTypeDelegate:
		return true
	default:
		return false
	}
}

func isIntentChangeConfirmationAllowlisted(a ActionDescriptor) bool {
	// Contractual exception: coding-domain worker delegation is already approved
	// at task assignment. Requiring an extra confirmation here would block
	// delegated coder/critic/strategist execution after owner approval.
	if a.Domain == AutonomyDomainCoding && a.CommandType == schema.CommandTypeDelegate {
		return true
	}
	// Communication targeted at the primary owner should not require
	// intent-change approval, as it is a fundamental and safe operation.
	if a.Tags != nil && a.Tags["recipient_status"] == "owner" {
		if a.Domain == AutonomyDomainMessaging && a.CommandType == schema.CommandTypeSend {
			return true
		}
		if a.CommandType == schema.CommandTypeInvoke {
			return true
		}
	}
	// Core system actors (navi, gateway, and authenticated connectors) are
	// trusted to bypass the intent-change check for read-only operations,
	// provided no explicit side-effect tags are present.
	if !hasIntentChangeSideEffectTag(a.Tags) {
		if a.ActorKind == "navi" || a.ActorKind == "gateway" || a.ActorKind == "connector" {
			switch a.CommandType {
			case schema.CommandTypeQuery, schema.CommandTypeAcquire:
				return true
			}
		}
	}
	// Contractual exception: non-owner-set knowledge lifecycle supersede/deprecate
	// remain auto-applicable. Owner-set variants are handled separately in risk
	// checks and continue to require proposal approval.
	if a.Domain == "memory" && a.Tags != nil && a.Tags["owner_set"] != "true" {
		action := strings.TrimSpace(strings.ToLower(a.Tags["lifecycle_action"]))
		return action == "supersede" || action == "deprecate"
	}
	return false
}

func hasIntentChangeSideEffectTag(tags map[string]string) bool {
	if tags == nil {
		return false
	}
	for _, key := range []string{"side_effect", "side_effects", "side_effect_class"} {
		if indicatesIntentChangingSideEffect(tags[key]) {
			return true
		}
	}
	for _, key := range []string{"mutates_state", "external_side_effect", "requires_confirmation"} {
		if strings.EqualFold(strings.TrimSpace(tags[key]), "true") {
			return true
		}
	}
	return false
}

func indicatesIntentChangingSideEffect(raw string) bool {
	value := strings.TrimSpace(strings.ToLower(raw))
	switch value {
	case "", "none", "no", "false", "[]", "read", "read_only", "readonly", "pure", "query":
		return false
	default:
		return true
	}
}

// ValidationOutcome represents the high-level result of a governance check.
type ValidationOutcome int

const (
	ValidationApproved ValidationOutcome = iota
	ValidationRequiresConfirmation
	ValidationModified
	ValidationRejected
)

// GovernanceTier indicates who can modify which constraints. System > Owner > Plugin.
type GovernanceTier string

const (
	GovernanceTierSystem GovernanceTier = "system"
	GovernanceTierOwner  GovernanceTier = "owner"
	GovernanceTierPlugin GovernanceTier = "plugin"
)

// ValidationResult is returned from centralized Validate/Govern checks.
type ValidationResult struct {
	Outcome        ValidationOutcome
	Reason         string
	ModifiedAction string         // non-empty when Outcome == ValidationModified (serialized action to re-evaluate)
	Tier           GovernanceTier // which tier produced this result
}

// ActionDescriptor describes an action subject to governance validation.
// It generalizes over skills, workers, connectors, and gateway commands.
type ActionDescriptor struct {
	// CommandType is the semantic primitive for this action (query, create, update, delete, invoke, send, acquire, schedule, delegate, compose).
	CommandType schema.CommandType
	// Domain is the autonomy/governance domain (e.g., "coding", "memory", "messaging").
	Domain string
	// ActorKind identifies who is initiating the action ("navi", "worker", "connector", "gateway").
	ActorKind string
	// SkillEntry is populated for skill-based actions; nil for non-skill actions.
	SkillEntry *skill.SkillEntry
	// ChatID links the action to a conversational context when applicable.
	ChatID string
	// RuntimeSessionID links the action to a runtime session context when applicable.
	RuntimeSessionID string
	// OwnerID is the owner scope for configuration/priority checks; when set and a ConfigPriorityReader is provided, Configuration and Priority Alignment steps use real data.
	OwnerID string
	// Tags carries optional, structured flags for configuration and priority alignment.
	Tags map[string]string
	// Mutation is populated for world_model_mutation effects (intake synthesis).
	// It is the sibling descriptor to the action fields above; the same Pipeline
	// consumes both because the checks operate on the descriptor's effect surface,
	// not its concrete type (see docs/design/intake-synthesis-seam.md §5). Nil for
	// ordinary (non-mutation) actions.
	Mutation *MutationDescriptor
}

// ConfigPriorityReader supplies owner-scoped configuration and priorities for governance checks.
// When nil or when ActionDescriptor.OwnerID is empty, Configuration and Priority Alignment steps use only Tags.
type ConfigPriorityReader interface {
	ListConfiguration(ctx context.Context, ownerID string, limit int) ([]schema.ConfigurationEntry, error)
	ListPriorities(ctx context.Context, ownerID string, limit int) ([]schema.Priority, error)
}

// CheckFunc is a single validation step. Returns outcome, reason, and optional modified action.
type CheckFunc func(action ActionDescriptor) (ValidationOutcome, string, string)

// Pipeline runs validation in fixed deterministic order:
// Permissions → Policy → Configuration → Priority Alignment → Risk Assessment.
// Steps are short-circuited: the first non-Approved outcome is returned and is
// authoritative. Nil checks are treated as Approved.
type Pipeline struct {
	Permissions       CheckFunc
	Policy            CheckFunc
	Configuration     CheckFunc
	PriorityAlignment CheckFunc
	RiskAssessment    CheckFunc
}

// Run executes validation checks in deterministic pipeline order and returns
// immediately on the first non-Approved result. The returned Tier records which
// governance tier produced that authoritative outcome for observability.
func (p *Pipeline) Run(action ActionDescriptor) ValidationResult {
	steps := []struct {
		name string
		fn   CheckFunc
		tier GovernanceTier
	}{
		{"permissions", p.Permissions, GovernanceTierSystem},
		{"policy", p.Policy, GovernanceTierSystem},
		{"configuration", p.Configuration, GovernanceTierOwner},
		{"priority_alignment", p.PriorityAlignment, GovernanceTierOwner},
		{"risk_assessment", p.RiskAssessment, GovernanceTierSystem},
	}
	for _, step := range steps {
		if step.fn == nil {
			continue
		}
		outcome, reason, modifiedAction := step.fn(action)
		if outcome != ValidationApproved {
			return ValidationResult{
				Outcome:        outcome,
				Reason:         reason,
				ModifiedAction: modifiedAction,
				Tier:           step.tier,
			}
		}
	}
	return ValidationResult{Outcome: ValidationApproved}
}

// ValidateAction evaluates whether a given action may execute by running the
// ordered Permissions → Policy → Configuration → Priority Alignment → Risk
// Assessment pipeline. Policy and risk checks are currently focused on skills.
func ValidateAction(pe *skill.PolicyEngine, action ActionDescriptor) ValidationResult {
	permissionsCheck := func(a ActionDescriptor) (ValidationOutcome, string, string) {
		if reason := ownerDisableReason(a.Tags, nil, a.Domain); reason != "" {
			return ValidationRejected, reason, ""
		}
		return ValidationApproved, "", ""
	}

	var policyCheck CheckFunc
	if pe != nil {
		policyCheck = func(a ActionDescriptor) (ValidationOutcome, string, string) {
			if a.SkillEntry == nil {
				return ValidationApproved, "", ""
			}
			res, msg := pe.Check(a.SkillEntry)
			switch res {
			case skill.PolicyDeny:
				return ValidationRejected, msg, ""
			case skill.PolicyConfirmRequired:
				return ValidationRequiresConfirmation, msg, ""
			default:
				return ValidationApproved, "", ""
			}
		}
	}

	configurationCheck := func(a ActionDescriptor) (ValidationOutcome, string, string) {
		return ValidationApproved, "", ""
	}

	priorityCheck := func(a ActionDescriptor) (ValidationOutcome, string, string) {
		return ValidationApproved, "", ""
	}

	riskCheck := func(a ActionDescriptor) (ValidationOutcome, string, string) {
		if a.Tags != nil {
			if a.Tags["lifecycle_action"] == "tombstone" {
				return ValidationRequiresConfirmation, "Tombstone requires proposal approval", ""
			}
			if a.Tags["lifecycle_action"] == "supersede" && a.Tags["owner_set"] == "true" {
				return ValidationRequiresConfirmation, "Owner-set knowledge supersede requires proposal approval", ""
			}
			if a.Tags["lifecycle_action"] == "deprecate" && a.Tags["owner_set"] == "true" {
				return ValidationRequiresConfirmation, "Owner-set knowledge deprecate requires proposal approval", ""
			}
		}
		if requiresIntentChangeConfirmation(a) {
			reason := "Intent-changing action requires explicit approval"
			switch a.Domain {
			case AutonomyDomainCoding:
				reason = "Codebase modification requires explicit approval"
			case AutonomyDomainMessaging:
				reason = "Communication requires explicit approval"
			}
			return ValidationRequiresConfirmation, reason, ""
		}
		return ValidationApproved, "", ""
	}

	p := Pipeline{
		Permissions:       permissionsCheck,
		Policy:            policyCheck,
		Configuration:     configurationCheck,
		PriorityAlignment: priorityCheck,
		RiskAssessment:    riskCheck,
	}
	return p.Run(action)
}

// ValidateActionWithOwner runs the same pipeline as ValidateAction but uses reader for
// Configuration and Priority Alignment when action.OwnerID is set and reader is non-nil.
// Use this when WorldModel (or equivalent) is available so owner config and priorities influence outcome.
func ValidateActionWithOwner(ctx context.Context, pe *skill.PolicyEngine, action ActionDescriptor, reader ConfigPriorityReader) ValidationResult {
	var configEntries []schema.ConfigurationEntry
	if reader != nil && action.OwnerID != "" {
		entries, err := reader.ListConfiguration(ctx, action.OwnerID, 50)
		if err == nil {
			configEntries = entries
		}
	}

	permissionsCheck := func(a ActionDescriptor) (ValidationOutcome, string, string) {
		if reason := ownerDisableReason(a.Tags, configEntries, a.Domain); reason != "" {
			return ValidationRejected, reason, ""
		}
		return ValidationApproved, "", ""
	}
	var policyCheck CheckFunc
	if pe != nil {
		policyCheck = func(a ActionDescriptor) (ValidationOutcome, string, string) {
			if a.SkillEntry == nil {
				return ValidationApproved, "", ""
			}
			res, msg := pe.Check(a.SkillEntry)
			switch res {
			case skill.PolicyDeny:
				return ValidationRejected, msg, ""
			case skill.PolicyConfirmRequired:
				return ValidationRequiresConfirmation, msg, ""
			default:
				return ValidationApproved, "", ""
			}
		}
	}

	configurationCheck := func(a ActionDescriptor) (ValidationOutcome, string, string) {
		return ValidationApproved, "", ""
	}

	priorityCheck := func(a ActionDescriptor) (ValidationOutcome, string, string) {
		if reader == nil || a.OwnerID == "" || a.Domain == "" {
			return ValidationApproved, "", ""
		}
		priorities, err := reader.ListPriorities(ctx, a.OwnerID, 20)
		if err != nil || len(priorities) == 0 {
			return ValidationApproved, "", ""
		}
		lower := strings.ToLower(a.Domain)
		for _, p := range priorities {
			text := strings.ToLower(p.Name + " " + p.Description)
			if (strings.Contains(text, "minimize external") || strings.Contains(text, "no external api")) &&
				(lower == "integration" || lower == "messaging") {
				return ValidationRequiresConfirmation, "Action may conflict with stated priority: " + p.Name, ""
			}
		}
		return ValidationApproved, "", ""
	}

	riskCheck := func(a ActionDescriptor) (ValidationOutcome, string, string) {
		if a.Tags != nil {
			if a.Tags["lifecycle_action"] == "tombstone" {
				return ValidationRequiresConfirmation, "Tombstone requires proposal approval", ""
			}
			if a.Tags["lifecycle_action"] == "supersede" && a.Tags["owner_set"] == "true" {
				return ValidationRequiresConfirmation, "Owner-set knowledge supersede requires proposal approval", ""
			}
			if a.Tags["lifecycle_action"] == "deprecate" && a.Tags["owner_set"] == "true" {
				return ValidationRequiresConfirmation, "Owner-set knowledge deprecate requires proposal approval", ""
			}
		}
		if requiresIntentChangeConfirmation(a) {
			reason := "Intent-changing action requires explicit approval"
			switch a.Domain {
			case AutonomyDomainCoding:
				reason = "Codebase modification requires explicit approval"
			case AutonomyDomainMessaging:
				reason = "Communication requires explicit approval"
			}
			return ValidationRequiresConfirmation, reason, ""
		}
		return ValidationApproved, "", ""
	}

	p := Pipeline{
		Permissions:       permissionsCheck,
		Policy:            policyCheck,
		Configuration:     configurationCheck,
		PriorityAlignment: priorityCheck,
		RiskAssessment:    riskCheck,
	}
	return p.Run(action)
}

// ValidateSkillExecution evaluates whether a given skill entry may execute.
// Backward-compatible wrapper: builds an ActionDescriptor and calls ValidateAction.
func ValidateSkillExecution(pe *skill.PolicyEngine, entry *skill.SkillEntry) ValidationResult {
	if entry == nil {
		return ValidationResult{Outcome: ValidationApproved}
	}
	action := ActionDescriptor{
		SkillEntry: entry,
	}
	return ValidateAction(pe, action)
}

// ActionDescriptorForLifecycleTombstone builds an ActionDescriptor for a Tombstone
// lifecycle action. Use with ValidateAction; when outcome is RequiresConfirmation,
// create a proposal and do not execute until the owner approves.
func ActionDescriptorForLifecycleTombstone(actorKind, entityType, entityID string) ActionDescriptor {
	return ActionDescriptor{
		CommandType: schema.CommandTypeDelete,
		Domain:      "memory",
		ActorKind:   actorKind,
		Tags: map[string]string{
			"lifecycle_action": "tombstone",
			"entity_type":      entityType,
			"entity_id":        entityID,
		},
	}
}

// ActionDescriptorForLifecycleSupersede builds an ActionDescriptor for a Knowledge
// Supersede (owner-set fact update). When ownerSet is true, ValidateAction will
// return RequiresConfirmation so callers must create a proposal.
func ActionDescriptorForLifecycleSupersede(actorKind string, oldFactID, newFactID string, ownerSet bool) ActionDescriptor {
	tags := map[string]string{
		"lifecycle_action": "supersede",
		"entity_type":      "fact",
		"entity_id":        oldFactID,
		"superseded_by":    newFactID,
	}
	if ownerSet {
		tags["owner_set"] = "true"
	}
	return ActionDescriptor{
		CommandType: schema.CommandTypeUpdate,
		Domain:      "memory",
		ActorKind:   actorKind,
		Tags:        tags,
	}
}

// ActionDescriptorForLifecycleDeprecate builds an ActionDescriptor for Knowledge
// Deprecate (soft-delete without replacement). When ownerSet is true, ValidateAction
// returns RequiresConfirmation so callers must create a proposal.
func ActionDescriptorForLifecycleDeprecate(actorKind, factID string, ownerSet bool) ActionDescriptor {
	tags := map[string]string{
		"lifecycle_action": "deprecate",
		"entity_type":      "fact",
		"entity_id":        factID,
	}
	if ownerSet {
		tags["owner_set"] = "true"
	}
	return ActionDescriptor{
		CommandType: schema.CommandTypeDelete,
		Domain:      "memory",
		ActorKind:   actorKind,
		Tags:        tags,
	}
}
