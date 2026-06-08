package embed

import (
	"context"
	"testing"
)

func TestBruteForceIndex_RanksByCosine(t *testing.T) {
	e := NewStubEmbedder()
	ctx := context.Background()

	docs := map[string]string{
		"a": "the quarterly budget review meeting with finance",
		"b": "a completely unrelated note about gardening and tomatoes",
		"c": "budget review notes and finance figures for the quarter",
	}
	vectors := make([]IndexedVector, 0, len(docs))
	for id, text := range docs {
		emb, err := e.Embed(ctx, text)
		if err != nil {
			t.Fatalf("embed %s: %v", id, err)
		}
		vectors = append(vectors, IndexedVector{ChunkID: id, Vector: emb.Vector})
	}
	idx := NewBruteForceIndex(vectors)
	if idx.Len() != 3 {
		t.Fatalf("Len = %d, want 3", idx.Len())
	}

	q, _ := e.Embed(ctx, "budget review finance figures")
	matches := idx.Search(q.Vector, 3)
	if len(matches) == 0 {
		t.Fatal("no matches")
	}
	// The two budget/finance docs must outrank the gardening note.
	top := matches[0].ChunkID
	if top != "a" && top != "c" {
		t.Errorf("top match = %q, want a or c (budget docs)", top)
	}
	last := matches[len(matches)-1].ChunkID
	if last != "b" {
		t.Errorf("worst match = %q, want b (gardening note)", last)
	}
}

func TestBruteForceIndex_DeterministicTieBreak(t *testing.T) {
	idx := NewBruteForceIndex([]IndexedVector{
		{ChunkID: "z", Vector: []float64{1, 0}},
		{ChunkID: "a", Vector: []float64{1, 0}},
	})
	m1 := idx.Search([]float64{1, 0}, 2)
	m2 := idx.Search([]float64{1, 0}, 2)
	if len(m1) != 2 || len(m2) != 2 {
		t.Fatalf("expected 2 matches each, got %d/%d", len(m1), len(m2))
	}
	// Equal similarity → deterministic order by chunk id.
	if m1[0].ChunkID != "a" || m1[1].ChunkID != "z" {
		t.Errorf("tie-break order = %q,%q; want a,z", m1[0].ChunkID, m1[1].ChunkID)
	}
	if m1[0] != m2[0] || m1[1] != m2[1] {
		t.Error("search not deterministic across calls")
	}
}

func TestBruteForceIndex_ZeroQueryNoMatches(t *testing.T) {
	idx := NewBruteForceIndex([]IndexedVector{{ChunkID: "a", Vector: []float64{1, 1}}})
	if m := idx.Search([]float64{0, 0}, 5); len(m) != 0 {
		t.Errorf("zero query returned %d matches, want 0", len(m))
	}
	if m := idx.Search([]float64{1, 1}, 0); len(m) != 0 {
		t.Errorf("k=0 returned %d matches, want 0", len(m))
	}
}

func TestBruteForceIndex_NilSafe(t *testing.T) {
	var idx *BruteForceIndex
	if idx.Len() != 0 {
		t.Error("nil index Len != 0")
	}
	if m := idx.Search([]float64{1}, 1); m != nil {
		t.Error("nil index Search != nil")
	}
}
