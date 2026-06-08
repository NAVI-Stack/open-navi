package store

import (
	"context"
	"testing"

	"github.com/open-navi/navi/internal/schema"
)

func TestVaultState_Roundtrip(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()

	if err := UpsertVaultState(ctx, db, VaultState{
		FilePath: "contacts/alex-rivera-01.md", EntityType: "contact", EntityID: "c1", ProjectedHash: "h1",
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, ok, err := GetVaultState(ctx, db, "contacts/alex-rivera-01.md")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if got.EntityID != "c1" || got.ProjectedHash != "h1" {
		t.Errorf("roundtrip mismatch: %+v", got)
	}

	// Lookup by entity.
	byEntity, ok, err := GetVaultStateByEntity(ctx, db, "contact", "c1")
	if err != nil || !ok || byEntity.FilePath != "contacts/alex-rivera-01.md" {
		t.Fatalf("by entity: ok=%v err=%v path=%q", ok, err, byEntity.FilePath)
	}

	// Update hash.
	if err := UpsertVaultState(ctx, db, VaultState{
		FilePath: "contacts/alex-rivera-01.md", EntityType: "contact", EntityID: "c1", ProjectedHash: "h2",
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _, _ = GetVaultState(ctx, db, "contacts/alex-rivera-01.md")
	if got.ProjectedHash != "h2" {
		t.Errorf("hash not updated: %q", got.ProjectedHash)
	}

	all, err := ListVaultState(ctx, db)
	if err != nil || len(all) != 1 {
		t.Fatalf("list: n=%d err=%v", len(all), err)
	}

	if err := DeleteVaultState(ctx, db, "contacts/alex-rivera-01.md"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok, _ := GetVaultState(ctx, db, "contacts/alex-rivera-01.md"); ok {
		t.Error("state not deleted")
	}
}

func TestVaultSources_ListAll(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()

	if err := SaveFact(ctx, db, Fact{ID: "f1", Scope: "global", Category: "tech", Key: "k", Value: "v"}); err != nil {
		t.Fatalf("save fact: %v", err)
	}
	if _, err := SaveMemory(ctx, db, schema.Memory{Scope: "owner", ScopeID: "o1", Summary: "s", Details: "d", Source: "test"}); err != nil {
		t.Fatalf("save memory: %v", err)
	}

	facts, err := ListAllFacts(ctx, db, false, 0)
	if err != nil || len(facts) != 1 {
		t.Fatalf("list all facts: n=%d err=%v", len(facts), err)
	}
	mems, err := ListAllMemories(ctx, db, 0)
	if err != nil || len(mems) != 1 {
		t.Fatalf("list all memories: n=%d err=%v", len(mems), err)
	}
	arts, err := ListAllArtifacts(ctx, db, 0)
	if err != nil {
		t.Fatalf("list all artifacts: %v", err)
	}
	_ = arts
}
