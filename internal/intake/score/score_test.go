package score

import (
	"context"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/schema"
)

func chunkAt(trust schema.ContentTrust, author string, fetched time.Time) schema.IntakeChunk {
	return schema.IntakeChunk{
		ID:    "chunk-1",
		Trust: trust,
		Provenance: schema.ChunkProvenance{
			Author:    author,
			FetchedAt: fetched,
		},
	}
}

func TestHeuristicScorer_Deterministic(t *testing.T) {
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	s := &HeuristicScorer{Now: func() time.Time { return now }}
	c := chunkAt(schema.ContentTrustOwner, "owner", now)
	a := s.Score(context.Background(), c)
	b := s.Score(context.Background(), c)
	if a != b {
		t.Fatalf("scorer not deterministic: %+v vs %+v", a, b)
	}
}

func TestHeuristicScorer_OwnerFreshPromoted(t *testing.T) {
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	s := &HeuristicScorer{Now: func() time.Time { return now }}
	res := s.Score(context.Background(), chunkAt(schema.ContentTrustOwner, "owner", now))
	if !res.PromoteCandidate {
		t.Errorf("fresh owner content should be a promotion candidate, got score=%.3f", res.Score)
	}
	if res.RetentionTier != RetentionDurable {
		t.Errorf("want durable, got %s", res.RetentionTier)
	}
}

func TestHeuristicScorer_OldExternalNotPromoted(t *testing.T) {
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	old := now.Add(-90 * 24 * time.Hour)
	s := &HeuristicScorer{Now: func() time.Time { return now }}
	res := s.Score(context.Background(), chunkAt(schema.ContentTrustExternalUntrusted, "", old))
	if res.PromoteCandidate {
		t.Errorf("old anonymous external content should not be promoted, got score=%.3f", res.Score)
	}
	if res.RetentionTier != RetentionEphemeral {
		t.Errorf("want ephemeral, got %s", res.RetentionTier)
	}
}

func TestHeuristicScorer_OwnerOutranksExternal(t *testing.T) {
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	s := &HeuristicScorer{Now: func() time.Time { return now }}
	owner := s.Score(context.Background(), chunkAt(schema.ContentTrustOwner, "owner", now))
	external := s.Score(context.Background(), chunkAt(schema.ContentTrustExternalUntrusted, "stranger", now))
	if owner.Score <= external.Score {
		t.Errorf("owner (%.3f) should outscore external (%.3f) at equal recency", owner.Score, external.Score)
	}
}
