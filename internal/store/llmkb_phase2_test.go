package store

import (
	"context"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/llmkb"
)

func TestSQLiteLLMKBRepo_GenerateRoutingProposals_ObeysPromotionConstraints(t *testing.T) {
	ctx := context.Background()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}

	repo := NewSQLiteLLMKBRepo(db)
	
	// Create a profile with strict promotion constraints
	profile := llmkb.LLMProfile{
		LLMID:         "provider-x.strict-model",
		CanonicalName: "Strict Model",
		ProviderID:    "provider-x",
		Routing: llmkb.RoutingProfile{
			PromotionThreshold:     0.95, // Very high
			PromotionMinSamples:    20,   // Lots of samples needed
			PromotionProbationDays: 7,    // Must be old enough
		},
		UsageStats: llmkb.UsageStats{
			FirstSeenAt:  ptr(time.Now().Add(-2 * 24 * time.Hour)), // Only 2 days old
			FallbackRate: 0.1,                                   // Needed to trigger delta < 0
		},
		OperationalState: llmkb.OperationalState{AvailabilityState: llmkb.AvailabilityStateAvailable},
		CreatedAt:        time.Now().UTC(),
	}
	if err := repo.SaveProfile(ctx, profile); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	// Add 15 successful executions (under the 20 sample limit)
	for i := 0; i < 15; i++ {
		if err := repo.AppendExecutionRecord(ctx, llmkb.LLMExecutionRecord{
			RecordID:  "exec-" + string(rune('a'+i)),
			LLMID:     profile.LLMID,
			TaskClass: llmkb.TaskClassCoding,
			Outcome:   llmkb.ExecutionOutcomeSuccess,
			CreatedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("AppendExecutionRecord: %v", err)
		}
	}

	// Case 1: Under sample size limit (15 < 20)
	proposals, err := repo.GenerateRoutingProposals(ctx, 0.8)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if len(proposals) != 0 {
		t.Errorf("expected 0 proposals due to sample size, got %d", len(proposals))
	}

	// Case 2: Under probation limit (2 days < 7 days) - even after enough samples
	for i := 15; i < 25; i++ {
		repo.AppendExecutionRecord(ctx, llmkb.LLMExecutionRecord{
			RecordID:  "exec-" + string(rune('a'+i)),
			LLMID:     profile.LLMID,
			TaskClass: llmkb.TaskClassCoding,
			Outcome:   llmkb.ExecutionOutcomeSuccess,
			CreatedAt: time.Now().UTC(),
		})
	}
	proposals, _ = repo.GenerateRoutingProposals(ctx, 0.8)
	if len(proposals) != 0 {
		t.Errorf("expected 0 proposals due to probation, got %d", len(proposals))
	}

	// Case 3: Pass all limits
	profile.UsageStats.FirstSeenAt = ptr(time.Now().Add(-10 * 24 * time.Hour))
	repo.SaveProfile(ctx, profile)
	
	proposals, _ = repo.GenerateRoutingProposals(ctx, 0.8)
	if len(proposals) != 1 {
		t.Errorf("expected 1 proposal after passing all constraints, got %d", len(proposals))
	}
	
	got := proposals[0]
	if got.ProbationElapsedDays < 10 {
		t.Errorf("expected ProbationElapsedDays >= 10, got %d", got.ProbationElapsedDays)
	}
	if got.FallbackRateDelta >= 0 {
		t.Errorf("expected negative FallbackRateDelta (improvement), got %f", got.FallbackRateDelta)
	}
}

func ptr[T any](v T) *T { return &v }
