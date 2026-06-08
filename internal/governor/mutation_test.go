package governor

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/schema"
)

// fixedPresetResolver returns the same preset for every domain.
type fixedPresetResolver struct{ preset string }

func (f fixedPresetResolver) EffectivePreset(string) string { return f.preset }

func baseMutation(kind MutationKind) MutationDescriptor {
	return MutationDescriptor{
		Kind:         kind,
		TargetType:   "contact",
		SourceRecords: []schema.IntakeRecordRef{{
			RecordID:    "rec-1",
			ConnectorID: "telegram:acct-1",
			SourceID:    "999:1001",
			FetchedAt:   time.Now().UTC(),
		}},
		Trust:         schema.ContentTrustExternalUntrusted,
		PrivacyClass:  schema.PrivacyClassPersonal,
		EstConfidence: 0.6,
		JobMode:       schema.JobModeDelta,
	}
}

func TestEvaluateMutation_SafeKindsApproved(t *testing.T) {
	for _, kind := range []MutationKind{MutationAppendHistory, MutationReinforceEntity, MutationCreateEntity} {
		res := EvaluateMutation(baseMutation(kind), MutationPipelineOptions{})
		if res.Outcome != ValidationApproved {
			t.Errorf("kind %s: want Approved, got %v (%s)", kind, res.Outcome, res.Reason)
		}
	}
}

func TestEvaluateMutation_StructuralKindsRequireConfirmation(t *testing.T) {
	for _, kind := range []MutationKind{MutationMergeEntities, MutationDeprecateEntity, MutationForgetMemory} {
		res := EvaluateMutation(baseMutation(kind), MutationPipelineOptions{})
		if res.Outcome != ValidationRequiresConfirmation {
			t.Errorf("kind %s: want RequiresConfirmation, got %v", kind, res.Outcome)
		}
	}
}

// TestHardFloorsUnbreakableByAnyPreset asserts that no autonomy preset — including
// the most permissive "high" — can auto-approve a Merge / Deprecate / Forget
// mutation (synthesis seam §6/§7; acceptance criterion #3).
func TestHardFloorsUnbreakableByAnyPreset(t *testing.T) {
	presets := []string{"low", "medium", "high", "", "supervised", "autonomous"}
	hardFloored := []MutationKind{MutationMergeEntities, MutationDeprecateEntity, MutationForgetMemory}
	for _, preset := range presets {
		for _, kind := range hardFloored {
			res := EvaluateMutation(baseMutation(kind), MutationPipelineOptions{
				Resolver: fixedPresetResolver{preset: preset},
			})
			if res.Outcome != ValidationRequiresConfirmation {
				t.Errorf("preset=%q kind=%s: hard floor breached — want RequiresConfirmation, got %v", preset, kind, res.Outcome)
			}
		}
	}
}

// TestAutonomyUpgradesNonFlooredCreate confirms that the autonomy gate still works
// for non-hard-floored kinds: a "high" preset upgrades an owner-vetoed Create to
// Approved, while hard-floored kinds stay put (the contrast proves the floor).
func TestAutonomyUpgradesNonFlooredKinds(t *testing.T) {
	// Owner veto pushes a Create to RequiresConfirmation; high autonomy upgrades it.
	opts := MutationPipelineOptions{
		Resolver: fixedPresetResolver{preset: "high"},
		OwnerVeto: func(d MutationDescriptor) string {
			if d.Kind == MutationCreateEntity {
				return "owner asks before creating contacts"
			}
			return ""
		},
	}
	res := EvaluateMutation(baseMutation(MutationCreateEntity), opts)
	if res.Outcome != ValidationApproved {
		t.Fatalf("high autonomy should upgrade owner-vetoed Create to Approved, got %v", res.Outcome)
	}
}

// TestModifiedOutcomeOnAmbiguousResolution exercises the Modified path: an
// ambiguous resolution becomes a low-confidence Create tagged possible_merge_with
// — never a guessed merge (acceptance criterion: Modified outcome path).
func TestModifiedOutcomeOnAmbiguousResolution(t *testing.T) {
	desc := baseMutation(MutationMergeEntities) // resolver proposed a merge…
	desc.Resolution = schema.ResolutionResult{
		CandidateName:    "Alex Rivera",
		Ambiguous:        true, // …but was not sure
		Confidence:       0.5,
		PossibleMergeIDs: []string{"contact-42", "contact-77"},
	}
	res := EvaluateMutation(desc, MutationPipelineOptions{})
	if res.Outcome != ValidationModified {
		t.Fatalf("ambiguous resolution: want Modified, got %v", res.Outcome)
	}
	var soft SoftenedMutation
	if err := json.Unmarshal([]byte(res.ModifiedAction), &soft); err != nil {
		t.Fatalf("ModifiedAction not a SoftenedMutation: %v", err)
	}
	if soft.Kind != MutationCreateEntity {
		t.Errorf("softened kind: want create_entity, got %s", soft.Kind)
	}
	if len(soft.PossibleMergeWith) != 2 {
		t.Errorf("possible_merge_with: want 2 ids, got %v", soft.PossibleMergeWith)
	}
}

// TestWriteClassDisabledRejects verifies that disabling the world_model_mutation
// effect path causes the engine to Reject, so synthesis drops rather than writes
// (hard invariant: governor disabled ⇒ drop, not write).
func TestWriteClassDisabledRejects(t *testing.T) {
	for _, kind := range []MutationKind{MutationAppendHistory, MutationCreateEntity, MutationMergeEntities} {
		res := EvaluateMutation(baseMutation(kind), MutationPipelineOptions{WriteClassDisabled: true})
		if res.Outcome != ValidationRejected {
			t.Errorf("kind %s with write-class disabled: want Rejected, got %v", kind, res.Outcome)
		}
	}
}

func TestActionDescriptorForMutationCarriesEffectClass(t *testing.T) {
	a := ActionDescriptorForMutation(baseMutation(MutationCreateEntity))
	if a.Mutation == nil {
		t.Fatal("ActionDescriptor.Mutation must be populated")
	}
	if a.Tags["effect_class"] != EffectClassWorldModelMutation {
		t.Errorf("effect_class tag: want %s, got %q", EffectClassWorldModelMutation, a.Tags["effect_class"])
	}
	if a.Domain != AutonomyDomainMemory {
		t.Errorf("domain: want memory, got %q", a.Domain)
	}
}
