package embed

import (
	"context"
	"math"
	"testing"
)

func TestStubEmbedder_Deterministic(t *testing.T) {
	e := NewStubEmbedder()
	a, _ := e.Embed(context.Background(), "hello world from navi")
	b, _ := e.Embed(context.Background(), "hello world from navi")
	if len(a.Vector) != e.Dimensions() {
		t.Fatalf("dim mismatch: %d != %d", len(a.Vector), e.Dimensions())
	}
	for i := range a.Vector {
		if a.Vector[i] != b.Vector[i] {
			t.Fatalf("embedding not deterministic at %d: %v != %v", i, a.Vector[i], b.Vector[i])
		}
	}
}

func TestStubEmbedder_Normalized(t *testing.T) {
	e := NewStubEmbedder()
	emb, _ := e.Embed(context.Background(), "the quick brown fox jumps")
	var norm float64
	for _, v := range emb.Vector {
		norm += v * v
	}
	if math.Abs(math.Sqrt(norm)-1.0) > 1e-9 {
		t.Errorf("vector not L2-normalized: norm=%.6f", math.Sqrt(norm))
	}
	if emb.Model != "stub-hash-v1" {
		t.Errorf("model: want stub-hash-v1, got %s", emb.Model)
	}
}

func TestStubEmbedder_EmptyTextZeroVector(t *testing.T) {
	e := NewStubEmbedder()
	emb, _ := e.Embed(context.Background(), "   ")
	for i, v := range emb.Vector {
		if v != 0 {
			t.Fatalf("empty text should yield zero vector, got %v at %d", v, i)
		}
	}
}
