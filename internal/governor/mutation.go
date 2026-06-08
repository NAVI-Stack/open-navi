package governor

import (
	"encoding/json"

	"github.com/ceoai/navi/internal/schema"
)

// EffectClassWorldModelMutation is the governed effect-vocabulary entry for
// World Model writes produced by intake synthesis (CIP stage 8; synthesis seam
// §4). It flows through the existing Pipeline as a new effect class — it is not a
// parallel engine and not a second Proposal queue. Adding it extends governance's
// effect vocabulary and its set of callers; nothing else about the engine changes.
const EffectClassWorldModelMutation = "world_model_mutation"

// MutationKind enumerates the disposition-ladder rungs (synthesis seam §5/§6).
// The ladder classifies a candidate write by reversibility and blast radius; it
// is the deterministic Go input to the existing Risk CheckFunc.
type MutationKind int

const (
	MutationAppendHistory   MutationKind = iota // append-only; unconditionally safe
	MutationReinforceEntity                     // bump confidence / add attribute; reversible
	MutationCreateEntity                        // new entity from extraction; reversible
	MutationMergeEntities                       // structural identity collapse — hard-floored
	MutationDeprecateEntity                     // destructive on Knowledge — hard-floored
	MutationForgetMemory                        // existing Forget path — hard-floored
)

// String returns a stable lowercase token for a MutationKind (used in tags,
// provenance, and Proposal text).
func (k MutationKind) String() string {
	switch k {
	case MutationAppendHistory:
		return "append_history"
	case MutationReinforceEntity:
		return "reinforce_entity"
	case MutationCreateEntity:
		return "create_entity"
	case MutationMergeEntities:
		return "merge_entities"
	case MutationDeprecateEntity:
		return "deprecate_entity"
	case MutationForgetMemory:
		return "forget_memory"
	default:
		return "unknown_mutation"
	}
}

// MutationDescriptor describes a World Model write subject to governance. It is a
// sibling to ActionDescriptor consumed by the same Pipeline.Run (synthesis seam
// §5). The two differ only in inputs because writes and actions are honestly
// different effects; overloading ActionDescriptor would muddy two honest call
// sites. SourceRecords must never be empty — provenance is non-negotiable.
type MutationDescriptor struct {
	Kind          MutationKind
	TargetType    string                   // entity class/table: contact, memory, knowledge, artifact, history
	TargetEntity  string                   // canonical id, or "" for Create
	CandidateRefs []string                 // entity ids in scope of a merge/dedupe
	SourceRecords []schema.IntakeRecordRef // provenance — never empty
	Trust         schema.ContentTrust      // from Admit
	PrivacyClass  schema.PrivacyClass
	Resolution    schema.ResolutionResult // from the Python resolution matcher
	Derivation    []string                // how this candidate was produced (chunk/record refs)
	EstConfidence float64                 // 0..1, blended from source trust + extraction + resolution
	JobMode       schema.JobMode          // Delta | Backfill — drives Proposal grouping at the batch boundary

	// Source identifies the synthesis caller. Empty means the default intake
	// pipeline (external delta / backfill). "vault" marks an owner edit that
	// entered through the Memory Vault (NAVI-VAULT-V1; spec §4 "second mouth").
	// Additive provenance only — it does not alter any governance decision; the
	// disposition ladder and hard floors are identical regardless of source.
	Source string
	// VaultPath is the Vault file the owner edit originated from, when Source is
	// "vault". Additive provenance carried so the sync log can show which file a
	// mutation came from.
	VaultPath string
}

const (
	// MutationSourceVault marks a MutationDescriptor produced by an owner edit to
	// a Memory Vault file (the second mouth on the intake pipeline).
	MutationSourceVault = "vault"
)

// SoftenedMutation is the payload carried in ValidationResult.ModifiedAction for
// the Modified outcome: instead of the risky merge the resolver was unsure about,
// the synthesizer writes a low-confidence Create tagged with possible_merge_with
// (synthesis seam §7). The Modified outcome lets the seam say "I won't do the
// risky thing, but I have a safer thing I can do unilaterally" without inventing
// new machinery.
type SoftenedMutation struct {
	Kind              MutationKind `json:"kind"`
	PossibleMergeWith []string     `json:"possible_merge_with,omitempty"`
	Reason            string       `json:"reason,omitempty"`
}

// MutationHardFloorReason returns a non-empty hard-floor reason for structural
// mutations that autonomy must never auto-approve, regardless of preset
// (synthesis seam §6/§7; acceptance criterion #3). Merge, Deprecate, and Forget
// are hard to reverse (identity collapse / destruction), so they always require
// an explicit owner decision via the Proposal Queue.
func MutationHardFloorReason(kind MutationKind) string {
	switch kind {
	case MutationMergeEntities, MutationDeprecateEntity, MutationForgetMemory:
		return HardFloorProposalRequired
	default:
		return ""
	}
}

// commandTypeForMutation maps a MutationKind onto the closest schema.CommandType
// so the world_model_mutation effect presents a coherent command surface to the
// rest of governance.
func commandTypeForMutation(kind MutationKind) schema.CommandType {
	switch kind {
	case MutationAppendHistory, MutationCreateEntity:
		return schema.CommandTypeCreate
	case MutationReinforceEntity, MutationMergeEntities:
		return schema.CommandTypeUpdate
	case MutationDeprecateEntity, MutationForgetMemory:
		return schema.CommandTypeDelete
	default:
		return schema.CommandTypeUpdate
	}
}

// ActionDescriptorForMutation wraps a MutationDescriptor as an ActionDescriptor so
// the existing Pipeline.Run consumes it unchanged. The mutation rides on the
// sibling Mutation field; Tags carry the effect class for observability.
func ActionDescriptorForMutation(desc MutationDescriptor) ActionDescriptor {
	d := desc
	return ActionDescriptor{
		CommandType: commandTypeForMutation(desc.Kind),
		Domain:      AutonomyDomainMemory,
		ActorKind:   "navi",
		Mutation:    &d,
		Tags: map[string]string{
			"effect_class":  EffectClassWorldModelMutation,
			"mutation_kind": desc.Kind.String(),
		},
	}
}

// MutationPipelineOptions configure the world_model_mutation evaluation. They are
// additive inputs threaded into the existing Pipeline's CheckFuncs — not a new
// engine and not a refactor of the existing one.
type MutationPipelineOptions struct {
	// Resolver resolves the effective autonomy preset for the memory domain; nil
	// disables autonomy upgrades (RequiresConfirmation is preserved).
	Resolver AutonomyPresetResolver
	// WriteClassDisabled disables the world_model_mutation effect path at the
	// System (Permissions) tier. When set, every mutation is Rejected so synthesis
	// drops rather than writes — exercised by the "governor disabled ⇒ drop" test.
	WriteClassDisabled bool
	// OwnerVeto, when non-nil and returning a non-empty reason, escalates the write
	// to RequiresConfirmation at the Owner (Configuration) tier — e.g. an owner
	// rule "always ask before deprecating Knowledge". Optional.
	OwnerVeto func(MutationDescriptor) string
}

// mutationMergeCandidates returns the entity ids a Modified (ambiguous) create
// should be tagged against, in priority order.
func mutationMergeCandidates(desc *MutationDescriptor) []string {
	if desc == nil {
		return nil
	}
	if len(desc.Resolution.PossibleMergeIDs) > 0 {
		return desc.Resolution.PossibleMergeIDs
	}
	if len(desc.CandidateRefs) > 0 {
		return desc.CandidateRefs
	}
	if desc.Resolution.MatchedID != "" {
		return []string{desc.Resolution.MatchedID}
	}
	return nil
}

// mutationRiskCheck is the disposition ladder (synthesis seam §6) evaluated as the
// Risk CheckFunc inside Pipeline.Run. It is deterministic Go (control-kernel
// logic), a fixed mapping from (MutationKind, Resolution) to a ValidationOutcome.
func mutationRiskCheck(a ActionDescriptor) (ValidationOutcome, string, string) {
	desc := a.Mutation
	if desc == nil {
		return ValidationApproved, "", ""
	}
	// Ambiguous resolution collapses to Modified: write a low-confidence Create
	// tagged possible_merge_with rather than guessing a merge (seam §7/§10).
	if desc.Resolution.Ambiguous {
		soft := SoftenedMutation{
			Kind:              MutationCreateEntity,
			PossibleMergeWith: mutationMergeCandidates(desc),
			Reason:            "ambiguous resolution; created low-confidence entity pending owner review",
		}
		b, _ := json.Marshal(soft)
		return ValidationModified, "Ambiguous resolution downgraded to low-confidence Create", string(b)
	}
	switch desc.Kind {
	case MutationAppendHistory, MutationReinforceEntity, MutationCreateEntity:
		return ValidationApproved, "", ""
	case MutationMergeEntities, MutationDeprecateEntity, MutationForgetMemory:
		return ValidationRequiresConfirmation, "Structural World Model mutation requires owner approval", ""
	default:
		return ValidationRequiresConfirmation, "Unknown mutation kind requires approval", ""
	}
}

// EvaluateMutation runs the world_model_mutation effect through the existing
// governance Pipeline and applies autonomy with hard floors. It is a new caller
// of Pipeline.Run via the sibling MutationDescriptor (synthesis seam §4) — it does
// not fork the Pipeline, create a parallel authority, or change the outcome enum.
func EvaluateMutation(desc MutationDescriptor, opts MutationPipelineOptions) ValidationResult {
	permissions := func(a ActionDescriptor) (ValidationOutcome, string, string) {
		if opts.WriteClassDisabled {
			return ValidationRejected, "world_model_mutation effect path disabled", ""
		}
		return ValidationApproved, "", ""
	}
	configuration := func(a ActionDescriptor) (ValidationOutcome, string, string) {
		if opts.OwnerVeto != nil && a.Mutation != nil {
			if reason := opts.OwnerVeto(*a.Mutation); reason != "" {
				return ValidationRequiresConfirmation, reason, ""
			}
		}
		return ValidationApproved, "", ""
	}

	p := Pipeline{
		Permissions:    permissions,
		Configuration:  configuration,
		RiskAssessment: mutationRiskCheck,
	}
	result := p.Run(ActionDescriptorForMutation(desc))
	return ApplyAutonomyMutation(result, opts.Resolver, desc)
}

// ApplyAutonomyMutation applies autonomy to a world_model_mutation result. It is
// the sibling of ApplyAutonomy for the mutation effect: it upgrades
// RequiresConfirmation → Approved only when the memory-domain threshold is "high"
// AND the mutation carries no hard floor. Merge / Deprecate / Forget are hard-
// floored and therefore never auto-approved by any preset (criterion #3).
func ApplyAutonomyMutation(result ValidationResult, resolver AutonomyPresetResolver, desc MutationDescriptor) ValidationResult {
	if result.Outcome != ValidationRequiresConfirmation {
		return result
	}
	if MutationHardFloorReason(desc.Kind) != "" {
		return result
	}
	if resolver == nil {
		return result
	}
	threshold := resolver.EffectivePreset(AutonomyDomainMemory)
	if r, ok := resolver.(ExecutionThresholdResolver); ok {
		threshold = r.EffectiveExecutionThreshold(AutonomyDomainMemory)
	}
	if threshold != "high" {
		return result
	}
	return ValidationResult{Outcome: ValidationApproved}
}
