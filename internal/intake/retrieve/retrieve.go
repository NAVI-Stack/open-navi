// Package retrieve implements CIP stage 10 (Retrieve) — the query-time hybrid
// retrieval surface that feeds the Conscious loop's Contextualize step (CIP §10,
// the load-bearing section for P4). It is READ-ONLY: it never writes the World
// Model. Synthesis (P3) remains the only intake path that writes entity tables.
//
// Retrieval is hybrid, combining three signals into one documented weighted score:
//
//   - vector similarity over intake_chunks embeddings (the committed brute-force
//     cosine engine in internal/intake/embed),
//   - entity-graph traversal (query terms → matched World Model entities → the
//     chunks those entities were synthesized from, via intake_provenance),
//   - a recency boost.
//
// Trust/privacy filtering is applied as the LAST pass before results return: a
// chunk whose PrivacyClass exceeds the caller's ceiling is dropped, and every
// returned chunk keeps its ContentTrust label intact so downstream prompt
// assembly can quote-wrap external_untrusted content (content-trust model).
//
// Un-promoted chunks (chunks that never synthesized into entities) remain in the
// candidate pool — synthesis is promotion, not gating (synthesis seam §14).
package retrieve

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/ceoai/navi/internal/intake/embed"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

// Weights tune the hybrid ranking. They are kept configurable per the task
// contract ("keep the weights configurable"); the defaults are a simple,
// documented weighted combination (not learned reranking, which is out of scope).
type Weights struct {
	Similarity  float64
	EntityGraph float64
	Recency     float64
}

// DefaultWeights is the V1 weighted combination: similarity dominates, an
// entity-graph hit is a strong boost, recency is a gentle tie-breaker.
var DefaultWeights = Weights{Similarity: 1.0, EntityGraph: 0.5, Recency: 0.3}

// Options tunes a retrieval pass.
type Options struct {
	// Limit caps how many results are returned after ranking + filtering.
	Limit int
	// CandidatePool bounds how many chunks the vector scan considers before the
	// entity-graph union and filtering. 0 defaults to max(Limit*4, 50).
	CandidatePool int
	// Weights tunes the hybrid ranking. Zero falls back to DefaultWeights.
	Weights Weights
	// MaxPrivacyClass is the privacy ceiling for this query: chunks classified
	// above it are filtered out. Empty means no privacy ceiling (owner context
	// sees everything) — preserves pre-P4 behavior for callers that don't set it.
	MaxPrivacyClass schema.PrivacyClass
	// RecencyHalfLifeDays controls the recency decay. 0 defaults to 30 days.
	RecencyHalfLifeDays float64
	// MaxChunkScan bounds how many chunks are loaded as the candidate universe.
	// 0 defaults to the store default (personal-assistant scale).
	MaxChunkScan int
	// Now overrides the clock for deterministic recency scoring in tests.
	Now time.Time
}

// RetrievalResult is one provenance-bearing retrieved chunk. It carries enough
// for the Conscious loop to assemble context AND trace every span back to its
// source (CIP §10; provenance is non-negotiable).
type RetrievalResult struct {
	ChunkID     string
	Content     string
	RecordID    string
	ConnectorID string
	SourceID    string
	Cursor      string
	FetchedAt   time.Time
	LinkBack    string
	Author      string
	SourceKind  string

	Trust        schema.ContentTrust
	PrivacyClass schema.PrivacyClass

	// EntityLinks are the World Model entities this chunk was synthesized into
	// (empty for un-promoted chunks, which are still retrievable).
	EntityLinks []EntityLink
	// Confidence is the blended synthesis confidence when the chunk produced an
	// entity; 0 when un-promoted.
	Confidence float64

	// Scoring breakdown (documented weighted combination).
	Score           float64
	SimilarityScore float64
	EntityScore     float64
	RecencyScore    float64
}

// EntityLink is a synthesized World Model entity a chunk links to.
type EntityLink struct {
	EntityType string
	EntityID   string
	Confidence float64
}

// Retrieve runs hybrid retrieval for query against the persisted intake corpus.
// It performs only SELECTs (read-only).
func Retrieve(ctx context.Context, db *sql.DB, embedder embed.Embedder, query string, opts Options) ([]RetrievalResult, error) {
	if db == nil {
		return nil, fmt.Errorf("retrieve: nil db")
	}
	query = strings.TrimSpace(query)
	opts = normalizeOptions(opts)

	chunks, err := store.ListAllIntakeChunks(ctx, db, opts.MaxChunkScan)
	if err != nil {
		return nil, fmt.Errorf("retrieve: load chunks: %w", err)
	}
	if len(chunks) == 0 {
		return nil, nil
	}
	byID := make(map[string]schema.IntakeChunk, len(chunks))
	for _, c := range chunks {
		byID[c.ID] = c
	}

	// (a) Vector similarity over chunk embeddings.
	similarity := map[string]float64{}
	if embedder != nil && query != "" {
		similarity, err = vectorSimilarity(ctx, db, embedder, query, opts.CandidatePool)
		if err != nil {
			return nil, err
		}
	}

	// (b) Entity-graph traversal: query terms → matched entities → their chunks.
	entityHits, err := entityGraphHits(ctx, db, query)
	if err != nil {
		return nil, err
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	// Candidate set = union of similarity matches and entity-graph hits.
	candidates := map[string]struct{}{}
	for id := range similarity {
		candidates[id] = struct{}{}
	}
	for id := range entityHits {
		if _, ok := byID[id]; ok {
			candidates[id] = struct{}{}
		}
	}

	results := make([]RetrievalResult, 0, len(candidates))
	for id := range candidates {
		c, ok := byID[id]
		if !ok {
			continue
		}
		sim := clamp01(similarity[id])
		ent := 0.0
		if entityHits[id] > 0 {
			ent = 1.0
		}
		rec := recencyScore(chunkRecencyTime(c), now, opts.RecencyHalfLifeDays)

		res := RetrievalResult{
			ChunkID:         c.ID,
			Content:         c.Content,
			RecordID:        c.IntakeRecordID,
			ConnectorID:     c.ConnectorID,
			SourceID:        c.SourceID,
			FetchedAt:       c.Provenance.FetchedAt,
			LinkBack:        c.Provenance.LinkBack,
			Author:          c.Provenance.Author,
			SourceKind:      c.Provenance.SourceKind,
			Trust:           c.Trust,
			PrivacyClass:    c.PrivacyClass,
			SimilarityScore: sim,
			EntityScore:     ent,
			RecencyScore:    rec,
			Score:           opts.Weights.Similarity*sim + opts.Weights.EntityGraph*ent + opts.Weights.Recency*rec,
		}
		results = append(results, res)
	}

	// Rank by combined score (stable, deterministic).
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].ChunkID < results[j].ChunkID
		}
		return results[i].Score > results[j].Score
	})

	// (last pass) Trust/privacy filtering before results return.
	filtered := results[:0:0]
	for _, r := range results {
		if !privacyPermitted(r.PrivacyClass, opts.MaxPrivacyClass) {
			continue
		}
		filtered = append(filtered, r)
	}

	if len(filtered) > opts.Limit {
		filtered = filtered[:opts.Limit]
	}

	// Attach synthesized entity links + blended confidence, and the source cursor
	// (provenance round-trip). Bounded to the final top-N results.
	for i := range filtered {
		attachProvenance(ctx, db, &filtered[i])
		if cursor, err := store.GetIntakeRecordCursor(ctx, db, filtered[i].RecordID); err == nil {
			filtered[i].Cursor = cursor
		}
	}
	return filtered, nil
}

func normalizeOptions(opts Options) Options {
	if opts.Limit <= 0 {
		opts.Limit = 10
	}
	if opts.CandidatePool <= 0 {
		opts.CandidatePool = opts.Limit * 4
		if opts.CandidatePool < 50 {
			opts.CandidatePool = 50
		}
	}
	if opts.Weights == (Weights{}) {
		opts.Weights = DefaultWeights
	}
	if opts.RecencyHalfLifeDays <= 0 {
		opts.RecencyHalfLifeDays = 30
	}
	return opts
}

// vectorSimilarity embeds the query and runs the committed brute-force cosine
// index over the persisted embedding references, returning chunkID → similarity
// for the top CandidatePool matches (clamped to non-negative, then normalized
// later).
func vectorSimilarity(ctx context.Context, db *sql.DB, embedder embed.Embedder, query string, pool int) (map[string]float64, error) {
	qe, err := embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("retrieve: embed query: %w", err)
	}
	embs, err := store.ListAllIntakeEmbeddings(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("retrieve: load embeddings: %w", err)
	}
	if len(embs) == 0 {
		return map[string]float64{}, nil
	}
	vectors := make([]embed.IndexedVector, 0, len(embs))
	for _, e := range embs {
		vectors = append(vectors, embed.IndexedVector{ChunkID: e.ChunkID, Vector: e.Vector})
	}
	idx := embed.NewBruteForceIndex(vectors)
	matches := idx.Search(qe.Vector, pool)
	out := make(map[string]float64, len(matches))
	for _, m := range matches {
		out[m.ChunkID] = m.Similarity
	}
	return out, nil
}

// entityGraphHits traverses from query terms to matched World Model entities and
// back to the chunks those entities were synthesized from. Returns chunkID → hit
// count. Pure read.
func entityGraphHits(ctx context.Context, db *sql.DB, query string) (map[string]int, error) {
	hits := map[string]int{}
	terms := significantTerms(query)
	if len(terms) == 0 {
		return hits, nil
	}
	seenEntity := map[string]struct{}{}
	for _, term := range terms {
		contacts, err := store.SearchContacts(ctx, db, store.SearchContactsFilter{Query: term, Limit: 10})
		if err != nil {
			return nil, fmt.Errorf("retrieve: entity search: %w", err)
		}
		for _, c := range contacts {
			if _, ok := seenEntity[c.ID]; ok {
				continue
			}
			seenEntity[c.ID] = struct{}{}
			links, err := store.ListIntakeProvenanceByEntity(ctx, db, "contact", c.ID)
			if err != nil {
				return nil, fmt.Errorf("retrieve: entity provenance: %w", err)
			}
			for _, l := range links {
				if strings.TrimSpace(l.ChunkID) != "" {
					hits[l.ChunkID]++
				}
			}
		}
	}
	return hits, nil
}

// attachProvenance fills in the synthesized entity link(s) and blended
// confidence for a retrieved chunk from intake_provenance. Best-effort: a chunk
// with no synthesis links stays un-promoted (empty EntityLinks, zero Confidence).
func attachProvenance(ctx context.Context, db *sql.DB, r *RetrievalResult) {
	links, err := store.ListIntakeProvenanceByChunk(ctx, db, r.ChunkID)
	if err != nil {
		return
	}
	var maxConf float64
	for _, l := range links {
		if strings.TrimSpace(l.EntityID) == "" {
			continue
		}
		r.EntityLinks = append(r.EntityLinks, EntityLink{
			EntityType: l.EntityType,
			EntityID:   l.EntityID,
			Confidence: l.Confidence,
		})
		if l.Confidence > maxConf {
			maxConf = l.Confidence
		}
	}
	r.Confidence = maxConf
}

// --- ranking helpers ----------------------------------------------------------

func chunkRecencyTime(c schema.IntakeChunk) time.Time {
	if !c.Provenance.FetchedAt.IsZero() {
		return c.Provenance.FetchedAt
	}
	return c.CreatedAt
}

func recencyScore(t, now time.Time, halfLifeDays float64) float64 {
	if t.IsZero() || halfLifeDays <= 0 {
		return 0
	}
	ageDays := now.Sub(t).Hours() / 24
	if ageDays < 0 {
		ageDays = 0
	}
	// Smooth decay in (0,1]: 1 at age 0, 0.5 at one half-life.
	return 1.0 / (1.0 + ageDays/halfLifeDays)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// significantTerms extracts query terms worth an entity lookup (drops short
// stopword-ish tokens). Capitalization is not required — entity matching is
// case-insensitive at the store.
func significantTerms(query string) []string {
	fields := strings.FieldsFunc(query, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '@' && r != '.'
	})
	out := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if len([]rune(f)) < 3 {
			continue
		}
		key := strings.ToLower(f)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, f)
	}
	return out
}

// --- trust / privacy ----------------------------------------------------------

// privacyRank orders privacy classes from least to most restrictive.
func privacyRank(p schema.PrivacyClass) int {
	switch p {
	case schema.PrivacyClassSecret:
		return 3
	case schema.PrivacyClassSensitive:
		return 2
	case schema.PrivacyClassPersonal:
		return 1
	default: // public or unset
		return 0
	}
}

// privacyPermitted reports whether a chunk of class p may surface under ceiling.
// An empty ceiling permits everything (owner context); otherwise p must be at or
// below the ceiling.
func privacyPermitted(p, ceiling schema.PrivacyClass) bool {
	if strings.TrimSpace(string(ceiling)) == "" {
		return true
	}
	return privacyRank(p) <= privacyRank(ceiling)
}

// MaxPrivacyClass returns the most restrictive privacy class present in a result
// set. Used to compute the effective routing privacy tier for the retrieved
// context (CIP §9/§10 coupling).
func MaxPrivacyClass(results []RetrievalResult) schema.PrivacyClass {
	max := schema.PrivacyClass("")
	for _, r := range results {
		if privacyRank(r.PrivacyClass) > privacyRank(max) {
			max = r.PrivacyClass
		}
	}
	return max
}
