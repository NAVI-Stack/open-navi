package fold

import (
	"context"
	"testing"

	"github.com/ceoai/navi/internal/store"
)

func TestFold_AccumulatesByEntityType(t *testing.T) {
	db := store.InitTestDB(t)
	ctx := context.Background()
	f := New(db, nil)

	counts, err := f.Fold(ctx, []Entry{
		{EntityType: "contact", EntityID: "c1", Summary: "Alex"},
		{EntityType: "contact", EntityID: "c2", Summary: "Sam"},
		{EntityType: "memory", EntityID: "m1", Summary: "chat"},
		{EntityType: "contact", EntityID: ""}, // proposal/drop — ignored
	})
	if err != nil {
		t.Fatalf("fold: %v", err)
	}
	if counts["contact"] != 2 || counts["memory"] != 1 {
		t.Fatalf("unexpected counts: %+v", counts)
	}

	node, ok, err := store.GetIntakeSummary(ctx, db, "contact")
	if err != nil || !ok {
		t.Fatalf("get summary: %v %v", ok, err)
	}
	if node.EntityCount != 2 {
		t.Errorf("contact summary count: want 2, got %d", node.EntityCount)
	}

	// Fold is derived/additive: a second fold accumulates.
	if _, err := f.Fold(ctx, []Entry{{EntityType: "contact", EntityID: "c3"}}); err != nil {
		t.Fatalf("second fold: %v", err)
	}
	node, _, _ = store.GetIntakeSummary(ctx, db, "contact")
	if node.EntityCount != 3 {
		t.Errorf("contact summary count after second fold: want 3, got %d", node.EntityCount)
	}
}
