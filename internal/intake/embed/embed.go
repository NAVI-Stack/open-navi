// Package embed implements CIP stage 7 (Embed): computing vector embeddings for
// distilled chunks. P3 ships the embedding *interface* and a deterministic V1
// backend; it persists embedding references but deliberately does NOT select a
// vector index engine — that decision is deferred (CIP §2.2, task spec). The V1
// backend is a deterministic hashing embedder (StubEmbedder), chosen over a
// local-Ollama backend so the test suite runs hermetically with no network or
// model dependency. The Ollama path (via internal/llm's Ollama provider) can be
// added later behind this same Embedder interface without touching callers.
package embed

import (
	"context"
	"hash/fnv"
	"math"
	"strings"
	"unicode"
)

// Embedding is a computed vector plus the model that produced it. The Vector is
// what a future vector index would store; in P3 it is persisted as an opaque
// reference alongside the chunk.
type Embedding struct {
	Vector []float64 `json:"vector"`
	Model  string    `json:"model"`
	Dim    int       `json:"dim"`
}

// Embedder computes embeddings for text. V1 ships StubEmbedder; production may
// substitute a local-Ollama-backed embedder behind this interface.
type Embedder interface {
	Embed(ctx context.Context, text string) (Embedding, error)
	Dimensions() int
	Name() string
}

const defaultStubDim = 96

// StubEmbedder is the deterministic V1 backend. It maps text to a fixed-dimension
// L2-normalized vector by hashing word tokens into buckets. Identical input
// always yields an identical vector (required for idempotency and reproducible
// tests); similar texts that share tokens land near each other, which is enough
// to exercise the interface and persistence without committing to an index.
type StubEmbedder struct {
	Dim int
}

// NewStubEmbedder returns a StubEmbedder with the default dimensionality.
func NewStubEmbedder() *StubEmbedder { return &StubEmbedder{Dim: defaultStubDim} }

func (e *StubEmbedder) dim() int {
	if e.Dim > 0 {
		return e.Dim
	}
	return defaultStubDim
}

// Dimensions returns the embedding dimensionality.
func (e *StubEmbedder) Dimensions() int { return e.dim() }

// Name returns the backend identifier persisted with each embedding.
func (e *StubEmbedder) Name() string { return "stub-hash-v1" }

func tokenize(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// Embed computes a deterministic, L2-normalized embedding for text.
func (e *StubEmbedder) Embed(_ context.Context, text string) (Embedding, error) {
	dim := e.dim()
	vec := make([]float64, dim)
	for _, tok := range tokenize(text) {
		h := fnv.New32a()
		_, _ = h.Write([]byte(tok))
		sum := h.Sum32()
		idx := int(sum % uint32(dim))
		// Sign bit from a second hash position keeps buckets from only growing.
		if sum&0x80000000 != 0 {
			vec[idx] -= 1
		} else {
			vec[idx] += 1
		}
	}
	// L2 normalize so cosine similarity is well-defined; zero vector stays zero.
	var norm float64
	for _, v := range vec {
		norm += v * v
	}
	if norm > 0 {
		norm = math.Sqrt(norm)
		for i := range vec {
			vec[i] /= norm
		}
	}
	return Embedding{Vector: vec, Model: e.Name(), Dim: dim}, nil
}

var _ Embedder = (*StubEmbedder)(nil)
