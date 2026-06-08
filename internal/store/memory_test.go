package store

import (
	"context"
	"testing"

	"github.com/ceoai/navi/internal/schema"
)

func TestSaveMemory_UsesConfiguredKnowledgeEmbedder(t *testing.T) {
	SetKnowledgeEmbedder(nil)
	t.Cleanup(func() { SetKnowledgeEmbedder(nil) })

	ctx := context.Background()
	db := InitTestDB(t)

	want := []float64{0.5, 0.1, 0.4}
	SetKnowledgeEmbedder(func(ctx context.Context, text string) ([]float64, error) {
		return want, nil
	})

	saved, err := SaveMemory(ctx, db, schema.Memory{
		Scope:   "owner",
		ScopeID: "owner-1",
		Summary: "Keep SQLite for local NAVI state.",
		Details: "Owner prefers single-file local storage.",
		Source:  "explicit",
	})
	if err != nil {
		t.Fatal(err)
	}

	persisted, err := GetMemory(ctx, db, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted.Embedding) != len(want) {
		t.Fatalf("expected embedding length %d, got %d", len(want), len(persisted.Embedding))
	}
	for i := range want {
		if persisted.Embedding[i] != want[i] {
			t.Fatalf("expected embedding %v, got %v", want, persisted.Embedding)
		}
	}
}
