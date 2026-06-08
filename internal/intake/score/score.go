// Package score implements CIP stage 6 (Score): per-chunk relevance/retention
// scoring. V1 is deterministic Go using recency, source weight, and author trust
// — no LLM call per chunk, keeping the hot loop bounded (CIP §6.1). The Scorer
// interface admits a future Python backend (the intelligence layer for richer-
// than-arithmetic scoring) without any change at the call sites, mirroring P2's
// abstractive-summary hook.
package score

import (
	"context"
	"math"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

// RetentionTier classifies how durably a chunk should be retained. Ephemeral
// chunks stay retrievable in intake_chunks but are not promotion candidates;
// durable chunks feed Synthesize as candidates for World Model entities.
type RetentionTier string

const (
	RetentionEphemeral RetentionTier = "ephemeral"
	RetentionDurable   RetentionTier = "durable"
)

// Result is the per-chunk scoring output. Score is in [0,1]; PromoteCandidate is
// the hint Synthesize consumes to decide whether a chunk is worth attempting to
// promote into the World Model. Scoring is promotion candidacy only — un-promoted
// chunks are never deleted (synthesis seam §14).
type Result struct {
	Score            float64
	RetentionTier    RetentionTier
	PromoteCandidate bool
}

// Scorer assigns a retention/promotion score to an intake chunk. V1 ships the
// deterministic HeuristicScorer; a Python-backed scorer can be substituted later
// behind this same interface (CIP §6.1).
type Scorer interface {
	Score(ctx context.Context, chunk schema.IntakeChunk) Result
}

// HeuristicScorer is the deterministic V1 scorer. It blends three cheap signals:
// recency (exponential decay on fetch age), source weight (ContentTrust class),
// and author trust (presence + trust class). No network, no LLM, no randomness.
type HeuristicScorer struct {
	// Now overrides the clock for deterministic tests; nil uses time.Now.
	Now func() time.Time
	// HalfLife is the recency decay half-life; zero defaults to 7 days.
	HalfLife time.Duration
	// DurableThreshold is the score at or above which a chunk is a durable
	// promotion candidate; zero defaults to 0.5.
	DurableThreshold float64
}

// NewHeuristicScorer returns a HeuristicScorer with default tuning.
func NewHeuristicScorer() *HeuristicScorer {
	return &HeuristicScorer{}
}

func (s *HeuristicScorer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func (s *HeuristicScorer) halfLife() time.Duration {
	if s.HalfLife > 0 {
		return s.HalfLife
	}
	return 7 * 24 * time.Hour
}

func (s *HeuristicScorer) durableThreshold() float64 {
	if s.DurableThreshold > 0 {
		return s.DurableThreshold
	}
	return 0.5
}

// sourceWeight maps a ContentTrust class to a weight in [0,1]. Owner content is
// the most authoritative; external untrusted the least.
func sourceWeight(trust schema.ContentTrust) float64 {
	switch trust {
	case schema.ContentTrustOwner:
		return 1.0
	case schema.ContentTrustInternalSystem:
		return 0.8
	case schema.ContentTrustTrustedPlugin:
		return 0.6
	case schema.ContentTrustExternalUntrusted:
		return 0.3
	default:
		return 0.3
	}
}

// authorTrust derives a trust signal from author presence and the chunk's trust
// class: a named author from a trusted source is worth more than an anonymous
// external one.
func authorTrust(chunk schema.IntakeChunk) float64 {
	base := sourceWeight(chunk.Trust)
	if chunk.Provenance.Author == "" {
		// Unattributed content loses some confidence regardless of source.
		return base * 0.6
	}
	return base
}

// recency returns an exponential-decay score in (0,1] from the chunk's fetch age.
func (s *HeuristicScorer) recency(chunk schema.IntakeChunk) float64 {
	fetched := chunk.Provenance.FetchedAt
	if fetched.IsZero() {
		fetched = chunk.CreatedAt
	}
	if fetched.IsZero() {
		return 1.0
	}
	age := s.now().Sub(fetched)
	if age <= 0 {
		return 1.0
	}
	// 0.5 ** (age / halfLife)
	return math.Pow(0.5, age.Seconds()/s.halfLife().Seconds())
}

// Score blends recency, source weight, and author trust into a single score and
// derives the retention tier and promotion candidacy.
func (s *HeuristicScorer) Score(_ context.Context, chunk schema.IntakeChunk) Result {
	recency := s.recency(chunk)
	source := sourceWeight(chunk.Trust)
	author := authorTrust(chunk)

	score := 0.4*recency + 0.4*source + 0.2*author
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	promote := score >= s.durableThreshold()
	tier := RetentionEphemeral
	if promote {
		tier = RetentionDurable
	}
	return Result{Score: score, RetentionTier: tier, PromoteCandidate: promote}
}

var _ Scorer = (*HeuristicScorer)(nil)
