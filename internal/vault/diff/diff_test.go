package diff

import (
	"testing"

	"github.com/open-navi/navi/internal/governor"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/vault/projector"
)

func current() Current {
	return Current{
		EntityType:   projector.TypeContact,
		EntityID:     "c1",
		Title:        "Alex Rivera",
		Body:         "Works on firmware.",
		Attrs:        map[string]string{"kind": "person", "trust_level": "high"},
		PrivacyClass: schema.PrivacyClassPersonal,
	}
}

func TestPlan_NoChange(t *testing.T) {
	doc := projector.Doc{EntityID: "c1", EntityType: "contact", Title: "Alex Rivera", Body: "Works on firmware.", Attrs: map[string]string{"kind": "person", "trust_level": "high"}}
	res := Plan(doc, current(), PlanOptions{RelPath: "contacts/alex.md"})
	if !res.NoOp {
		t.Fatalf("expected no-op, got %+v", res)
	}
}

func TestPlan_RoutineEdit_Reinforce(t *testing.T) {
	doc := projector.Doc{EntityID: "c1", EntityType: "contact", Title: "Alexandra Rivera", Body: "Works on RF systems.", Attrs: map[string]string{"kind": "person", "trust_level": "high"}}
	res := Plan(doc, current(), PlanOptions{RelPath: "contacts/alex.md"})
	if res.NoOp || res.Structural {
		t.Fatalf("expected routine edit, got %+v", res)
	}
	if len(res.Mutations) != 1 || res.Mutations[0].Kind != governor.MutationReinforceEntity {
		t.Fatalf("expected one reinforce, got %+v", res.Mutations)
	}
	m := res.Mutations[0]
	if m.Trust != schema.ContentTrustOwner {
		t.Errorf("trust: got %q want owner", m.Trust)
	}
	if m.Source != governor.MutationSourceVault {
		t.Errorf("source: got %q want vault", m.Source)
	}
	if m.VaultPath != "contacts/alex.md" {
		t.Errorf("vault path provenance lost: %q", m.VaultPath)
	}
	if len(res.Changes) != 2 {
		t.Errorf("expected 2 changes (title, body), got %+v", res.Changes)
	}
}

func TestPlan_Forget_HardFloored(t *testing.T) {
	doc := projector.Doc{EntityID: "c1", EntityType: "contact", Title: "Alex Rivera", Forget: true}
	res := Plan(doc, current(), PlanOptions{RelPath: "contacts/alex.md"})
	if !res.Structural {
		t.Fatal("forget must be structural")
	}
	if res.Mutations[0].Kind != governor.MutationForgetMemory {
		t.Fatalf("expected forget, got %v", res.Mutations[0].Kind)
	}
	// Hard floor must hold regardless of autonomy.
	if governor.MutationHardFloorReason(res.Mutations[0].Kind) == "" {
		t.Error("forget mutation lost its hard floor")
	}
}

func TestPlan_EntityIDSwap_Merge(t *testing.T) {
	doc := projector.Doc{EntityID: "c2", EntityType: "contact", Title: "Alex Rivera"}
	res := Plan(doc, current(), PlanOptions{RelPath: "contacts/alex.md"})
	if !res.Structural || res.Mutations[0].Kind != governor.MutationMergeEntities {
		t.Fatalf("entity_id swap must be a merge request, got %+v", res)
	}
	refs := res.Mutations[0].CandidateRefs
	if len(refs) != 2 || refs[0] != "c1" || refs[1] != "c2" {
		t.Errorf("merge candidates wrong: %+v", refs)
	}
}

func TestForgetOnDelete(t *testing.T) {
	res := ForgetOnDelete(current(), "contacts/alex.md")
	if len(res.Mutations) != 1 || res.Mutations[0].Kind != governor.MutationForgetMemory {
		t.Fatalf("delete must produce a forget request, got %+v", res)
	}
	if res.Mutations[0].Source != governor.MutationSourceVault {
		t.Error("delete forget not tagged vault source")
	}
}
