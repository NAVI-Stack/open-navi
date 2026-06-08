package store

import (
	"context"
	"testing"
)

func TestSaveFact_ListFacts_FormatFactsForPrompt(t *testing.T) {
	ctx := context.Background()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}

	f := Fact{
		Scope:    "directive",
		ScopeID:  "dir-1",
		Category: "project_decision",
		Key:      "database",
		Value:    "SQLite WAL",
		Source:   "explicit",
	}
	if err := SaveFact(ctx, db, f); err != nil {
		t.Fatal(err)
	}
	f2 := Fact{
		Scope:    "global",
		ScopeID:  "",
		Category: "user_preference",
		Key:      "indentation",
		Value:    "2 spaces",
		Source:   "inferred",
	}
	if err := SaveFact(ctx, db, f2); err != nil {
		t.Fatal(err)
	}

	facts, err := ListFacts(ctx, db, "directive", "dir-1", true, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Errorf("expected 2 facts, got %d", len(facts))
	}
	if len(facts[0].Embedding) == 0 && len(facts[1].Embedding) == 0 {
		t.Fatal("expected generated embeddings on stored facts")
	}
	if len(facts[0].Keywords) == 0 && len(facts[1].Keywords) == 0 {
		t.Fatal("expected generated keywords on stored facts")
	}
	block := FormatFactsForPrompt(facts)
	if block == "" {
		t.Error("expected non-empty prompt block")
	}
	if len(block) < 20 {
		t.Errorf("expected substantial block, got %q", block)
	}
}

func TestFormatFactsForPrompt_Empty(t *testing.T) {
	out := FormatFactsForPrompt(nil)
	if out != "" {
		t.Errorf("expected empty string for nil, got %q", out)
	}
	out = FormatFactsForPrompt([]Fact{})
	if out != "" {
		t.Errorf("expected empty string for empty slice, got %q", out)
	}
}

func TestDeprecateFact(t *testing.T) {
	ctx := context.Background()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	f := Fact{
		Scope: "directive", ScopeID: "dir-1",
		Category: "project_decision", Key: "k", Value: "v", Source: "explicit",
	}
	if err := SaveFact(ctx, db, f); err != nil {
		t.Fatal(err)
	}
	list, _ := ListFacts(ctx, db, "directive", "dir-1", false, 10, false)
	if len(list) != 1 {
		t.Fatalf("expected 1 fact before deprecate, got %d", len(list))
	}
	factID := list[0].ID
	if err := DeprecateFact(ctx, db, factID); err != nil {
		t.Fatal(err)
	}
	list, _ = ListFacts(ctx, db, "directive", "dir-1", false, 10, false)
	if len(list) != 0 {
		t.Errorf("expected 0 facts when excluding deprecated, got %d", len(list))
	}
	list, _ = ListFacts(ctx, db, "directive", "dir-1", false, 10, true)
	if len(list) != 1 || !list[0].Deprecated {
		t.Errorf("expected 1 deprecated fact when including deprecated, got %d", len(list))
	}
}

func TestSaveFact_UsesConfiguredKnowledgeEmbedder(t *testing.T) {
	SetKnowledgeEmbedder(nil)
	t.Cleanup(func() { SetKnowledgeEmbedder(nil) })

	ctx := context.Background()
	db := InitTestDB(t)

	want := []float64{0.25, 0.75}
	SetKnowledgeEmbedder(func(ctx context.Context, text string) ([]float64, error) {
		return want, nil
	})

	if err := SaveFact(ctx, db, Fact{
		Scope:    "owner",
		ScopeID:  "owner-1",
		Category: "technical_context",
		Key:      "runtime",
		Value:    "go",
		Source:   "explicit",
	}); err != nil {
		t.Fatal(err)
	}

	facts, err := ListFacts(ctx, db, "owner", "owner-1", false, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
	if len(facts[0].Embedding) != len(want) {
		t.Fatalf("expected embedding length %d, got %d", len(want), len(facts[0].Embedding))
	}
	for i := range want {
		if facts[0].Embedding[i] != want[i] {
			t.Fatalf("expected embedding %v, got %v", want, facts[0].Embedding)
		}
	}
}

func TestKnowledgeEmbedding_FallsBackWhenConfiguredEmbedderFails(t *testing.T) {
	SetKnowledgeEmbedder(nil)
	t.Cleanup(func() { SetKnowledgeEmbedder(nil) })

	SetKnowledgeEmbedder(func(ctx context.Context, text string) ([]float64, error) {
		return nil, context.DeadlineExceeded
	})

	vector := KnowledgeEmbedding(context.Background(), "sqlite", "database")
	if len(vector) == 0 {
		t.Fatal("expected heuristic fallback embedding when configured embedder fails")
	}
}
