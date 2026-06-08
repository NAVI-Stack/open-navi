package embed

import (
	"math"
	"sort"
)

// VectorIndex is the P4 retrieval-side vector search contract. P3 shipped only
// the Embedder interface and a persisted reference; P4 commits to a concrete
// engine behind this interface.
//
// Engine choice (committed in P4): a pure-Go, in-process brute-force cosine
// index (BruteForceIndex). Rationale:
//
//   - The supported NaviD runtime (Docker Compose) and `make test` both build
//     without CGO. sqlite-vec is a C loadable extension and an embedded ANN
//     library with CGO would reintroduce the cgo/gcc dependency P3 deliberately
//     avoided. A pure-Go scan keeps every build hermetic — the same reasoning
//     that made P3 choose the deterministic stub embedder over local-Ollama.
//   - At the V1 target volume (≈10k chunks × 96-dim vectors) a linear scan is
//     ~10k·96 ≈ 1M multiply-adds per query: sub-millisecond to low-single-digit
//     milliseconds, far below the latency floor of the LLM call it feeds.
//   - The persisted intake_embeddings table IS the index source, so committing
//     to this engine requires no data migration: the index is rebuilt from the
//     stored references on demand. When personal-assistant volume eventually
//     outgrows a linear scan, an ANN engine can replace BruteForceIndex behind
//     this same interface with no caller change (the P3/P4 contract holds).
type VectorIndex interface {
	// Search returns up to k entries ranked by descending cosine similarity to
	// query. Entries with non-positive similarity are still returned (callers
	// apply their own thresholds); a zero query yields no matches.
	Search(query []float64, k int) []Match
	// Len reports how many vectors are indexed.
	Len() int
}

// IndexedVector is one (chunk id, embedding vector) pair fed to the index.
type IndexedVector struct {
	ChunkID string
	Vector  []float64
}

// Match is a single search hit: the chunk id and its cosine similarity to the
// query in [-1, 1].
type Match struct {
	ChunkID    string
	Similarity float64
}

// BruteForceIndex is the committed V1 vector engine: a linear cosine scan over
// in-memory vectors. It is deterministic and dependency-free.
type BruteForceIndex struct {
	vectors []IndexedVector
	norms   []float64 // precomputed L2 norms, parallel to vectors
}

// NewBruteForceIndex builds an index from vectors. Empty or zero-norm vectors
// are retained but will score 0 against any query.
func NewBruteForceIndex(vectors []IndexedVector) *BruteForceIndex {
	idx := &BruteForceIndex{
		vectors: append([]IndexedVector(nil), vectors...),
		norms:   make([]float64, len(vectors)),
	}
	for i, v := range idx.vectors {
		idx.norms[i] = l2norm(v.Vector)
	}
	return idx
}

// Len reports how many vectors are indexed.
func (b *BruteForceIndex) Len() int {
	if b == nil {
		return 0
	}
	return len(b.vectors)
}

// Search returns the top-k cosine matches for query.
func (b *BruteForceIndex) Search(query []float64, k int) []Match {
	if b == nil || len(b.vectors) == 0 || k <= 0 {
		return nil
	}
	qNorm := l2norm(query)
	if qNorm == 0 {
		return nil
	}
	matches := make([]Match, 0, len(b.vectors))
	for i, v := range b.vectors {
		if b.norms[i] == 0 {
			continue
		}
		sim := dot(query, v.Vector) / (qNorm * b.norms[i])
		matches = append(matches, Match{ChunkID: v.ChunkID, Similarity: sim})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Similarity == matches[j].Similarity {
			return matches[i].ChunkID < matches[j].ChunkID
		}
		return matches[i].Similarity > matches[j].Similarity
	})
	if len(matches) > k {
		matches = matches[:k]
	}
	return matches
}

func dot(a, b []float64) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var sum float64
	for i := 0; i < n; i++ {
		sum += a[i] * b[i]
	}
	return sum
}

func l2norm(v []float64) float64 {
	var sum float64
	for _, x := range v {
		sum += x * x
	}
	if sum == 0 {
		return 0
	}
	return math.Sqrt(sum)
}

var _ VectorIndex = (*BruteForceIndex)(nil)
