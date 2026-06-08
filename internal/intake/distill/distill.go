// Package distill implements CIP stage 5 (Distill) as a reusable, callable
// primitive. It is invoked at ingest time (P2 — shrink what we persist) and is
// designed to be invoked again at retrieval time (P4 — shrink what we send to a
// model) with a context-budget argument; the same Distill function serves both
// by accepting either a chunk batch (FromChunks) or an arbitrary
// content+provenance bundle (the Input type directly).
//
// V1 is extractive-only:
//   - near-duplicate chunk collapse (Jaccard over word shingles)
//   - quote extraction with span-level provenance
//   - deterministic entity-name span extraction (no LLM)
//   - URL normalization
//
// An abstractive summary backend may plug in via the Summarizer hook, but no
// implementation that calls an LLM ships in P2. The default (NoopSummarizer) is
// a pass-through. Distillation never discards provenance: every output chunk
// links back to its source chunk(s) and any collapsed near-duplicate siblings.
package distill

import (
	"context"
	"sort"
	"strings"
)

// Provenance is the origin metadata carried by every chunk through distillation.
type Provenance struct {
	ConnectorID string
	SourceID    string
	AccountID   string
	LinkBack    string
	Author      string
	SourceKind  string
}

// ProvenancedChunk is the dual-use input unit: a slice of content with a
// verifiable link back to its source record/chunk. Ingest-side callers build
// these from canonicalized + chunked record content; retrieval-side callers
// build them from arbitrary stored chunks plus their provenance.
type ProvenancedChunk struct {
	ChunkID     string
	RecordID    string
	Index       int
	Content     string
	StartOffset int // offset of this chunk within its source document
	EndOffset   int
	Provenance  Provenance
}

// Summarizer is the optional abstractive-summary hook. V1 ships extractive-only;
// the default NoopSummarizer is a pass-through. No LLM-backed implementation is
// included in P2 (deferred to P4 to avoid LLM dependency and privacy-tier
// routing coupling here).
type Summarizer interface {
	Summarize(ctx context.Context, text string) (string, error)
}

// NoopSummarizer returns its input unchanged (extractive-only default).
type NoopSummarizer struct{}

// Summarize implements Summarizer as a pass-through.
func (NoopSummarizer) Summarize(_ context.Context, text string) (string, error) {
	return text, nil
}

// Options tunes a distillation pass.
type Options struct {
	// Budget, when > 0, caps the total estimated tokens of the output (used at
	// retrieval time). 0 means no budget (ingest side keeps everything that
	// survives dedupe).
	Budget int
	// DedupeThreshold is the Jaccard similarity at or above which two chunks are
	// treated as near-duplicates. 0 falls back to DefaultDedupeThreshold.
	DedupeThreshold float64
	// Summarizer is the optional abstractive hook. nil means extractive-only.
	Summarizer Summarizer
}

// DefaultDedupeThreshold is the near-duplicate collapse threshold.
const DefaultDedupeThreshold = 0.85

// Input is the bundle Distill operates on.
type Input struct {
	Chunks  []ProvenancedChunk
	Options Options
}

// Quote is an extracted quote with span-level provenance (offsets relative to
// the owning chunk Content).
type Quote struct {
	Text        string
	StartOffset int
	EndOffset   int
}

// EntitySpan is a deterministically extracted entity-name span (offsets relative
// to the owning chunk Content).
type EntitySpan struct {
	Name        string
	Kind        string // proper_noun | url | email | handle | hashtag | identifier
	StartOffset int
	EndOffset   int
}

// DistilledChunk is a surviving output chunk plus its derived artifacts. It
// preserves a verifiable link to its source chunk(s): SourceChunkIDs always
// contains its own ChunkID and the ChunkIDs of any near-duplicates collapsed
// into it.
type DistilledChunk struct {
	ChunkID        string
	RecordID       string
	Index          int
	Content        string
	StartOffset    int
	EndOffset      int
	SourceChunkIDs []string
	CollapsedIDs   []string // near-duplicate siblings folded into this chunk
	Quotes         []Quote
	Entities       []EntitySpan
	Provenance     Provenance
}

// Metrics is the per-pass telemetry emitted for tuning (CIP §11).
type Metrics struct {
	ChunksIn    int
	ChunksOut   int
	Duplicates  int
	DedupeRate  float64 // duplicates / chunks_in
	InputBytes  int
	OutputBytes int
	Quotes      int
	Entities    int
	Budgeted    bool // true if the Budget cap dropped chunks
	Dropped     int  // chunks dropped to fit the budget
}

// Result is the output of a distillation pass.
type Result struct {
	Chunks  []DistilledChunk
	Metrics Metrics
}

// FromChunks adapts a chunk batch produced by the chunk stage into Distill
// input. It is a thin convenience for ingest-side callers; retrieval-side
// callers may construct Input directly from stored chunks.
func FromChunks(chunks []ProvenancedChunk, opts Options) Input {
	return Input{Chunks: chunks, Options: opts}
}

// Distill runs the extractive distillation pipeline over in.Chunks. It is
// deterministic for identical input and never discards provenance.
func Distill(ctx context.Context, in Input) Result {
	opts := in.Options
	if opts.DedupeThreshold <= 0 {
		opts.DedupeThreshold = DefaultDedupeThreshold
	}
	summarizer := opts.Summarizer

	metrics := Metrics{ChunksIn: len(in.Chunks)}

	// Precompute shingle sets for near-duplicate detection.
	type prepared struct {
		ch       ProvenancedChunk
		shingles map[string]struct{}
		survivor int // index into survivors slice, -1 if a duplicate
	}
	prep := make([]prepared, len(in.Chunks))
	for i, c := range in.Chunks {
		prep[i] = prepared{ch: c, shingles: shingleSet(c.Content), survivor: -1}
		metrics.InputBytes += len(c.Content)
	}

	var survivors []*DistilledChunk
	for i := range prep {
		dupOf := -1
		for s := 0; s < i; s++ {
			if prep[s].survivor < 0 {
				continue // s itself was a duplicate
			}
			if jaccard(prep[i].shingles, prep[s].shingles) >= opts.DedupeThreshold {
				dupOf = prep[s].survivor
				break
			}
		}
		if dupOf >= 0 {
			// Collapse into the survivor, preserving provenance link.
			surv := survivors[dupOf]
			surv.CollapsedIDs = append(surv.CollapsedIDs, prep[i].ch.ChunkID)
			surv.SourceChunkIDs = append(surv.SourceChunkIDs, prep[i].ch.ChunkID)
			metrics.Duplicates++
			continue
		}

		c := prep[i].ch
		content := c.Content
		if summarizer != nil {
			if out, err := summarizer.Summarize(ctx, content); err == nil && out != "" {
				content = out
			}
		}
		dc := &DistilledChunk{
			ChunkID:        c.ChunkID,
			RecordID:       c.RecordID,
			Index:          len(survivors),
			Content:        content,
			StartOffset:    c.StartOffset,
			EndOffset:      c.EndOffset,
			SourceChunkIDs: []string{c.ChunkID},
			Quotes:         extractQuotes(content),
			Entities:       extractEntities(content),
			Provenance:     c.Provenance,
		}
		prep[i].survivor = len(survivors)
		survivors = append(survivors, dc)
	}

	// Apply retrieval-side budget, if any, by keeping chunks in order until the
	// token budget is exhausted (extractive truncation).
	out := make([]DistilledChunk, 0, len(survivors))
	usedTokens := 0
	for _, dc := range survivors {
		if opts.Budget > 0 {
			t := estimateTokens(dc.Content)
			if usedTokens+t > opts.Budget && len(out) > 0 {
				metrics.Budgeted = true
				metrics.Dropped++
				continue
			}
			usedTokens += t
		}
		out = append(out, *dc)
	}

	for i := range out {
		out[i].Index = i
		metrics.OutputBytes += len(out[i].Content)
		metrics.Quotes += len(out[i].Quotes)
		metrics.Entities += len(out[i].Entities)
	}
	metrics.ChunksOut = len(out)
	if metrics.ChunksIn > 0 {
		metrics.DedupeRate = float64(metrics.Duplicates) / float64(metrics.ChunksIn)
	}

	return Result{Chunks: out, Metrics: metrics}
}

// --- near-duplicate detection -------------------------------------------------

const shingleSize = 3

// shingleSet builds the set of word-level n-grams (n=shingleSize) for content,
// normalized to lowercase tokens. Short content falls back to single tokens.
func shingleSet(content string) map[string]struct{} {
	tokens := tokenize(content)
	set := make(map[string]struct{})
	if len(tokens) == 0 {
		return set
	}
	if len(tokens) < shingleSize {
		for _, t := range tokens {
			set[t] = struct{}{}
		}
		return set
	}
	for i := 0; i+shingleSize <= len(tokens); i++ {
		set[strings.Join(tokens[i:i+shingleSize], " ")] = struct{}{}
	}
	return set
}

func jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	small, large := a, b
	if len(b) < len(a) {
		small, large = b, a
	}
	for k := range small {
		if _, ok := large[k]; ok {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func tokenize(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	})
	return fields
}

func estimateTokens(s string) int {
	if s == "" {
		return 0
	}
	return (len([]rune(s)) + 3) / 4
}

// sortEntities provides a stable order (by offset) for deterministic output.
func sortEntities(es []EntitySpan) {
	sort.SliceStable(es, func(i, j int) bool {
		if es[i].StartOffset != es[j].StartOffset {
			return es[i].StartOffset < es[j].StartOffset
		}
		return es[i].Kind < es[j].Kind
	})
}
