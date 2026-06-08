package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/llmkb"
)

func TestSQLiteLLMKBRepo_GenerateRoutingProposals_DedupesPendingItems(t *testing.T) {
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
	if err := repo.SaveProfile(ctx, llmkb.LLMProfile{
		LLMID:            "provider-a.chat-pro",
		CanonicalName:    "Chat Pro",
		Aliases:          []string{"chat-pro"},
		ProviderID:       "provider-a",
		OperationalState: llmkb.OperationalState{AvailabilityState: llmkb.AvailabilityStateAvailable},
		CreatedAt:        time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	for i := 0; i < 10; i++ {
		if err := repo.AppendExecutionRecord(ctx, llmkb.LLMExecutionRecord{
			RecordID:  "exec-coding-" + string(rune('a'+i)),
			LLMID:     "provider-a.chat-pro",
			TaskClass: llmkb.TaskClassCoding,
			Outcome:   llmkb.ExecutionOutcomeSuccess,
			CreatedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("AppendExecutionRecord(%d): %v", i, err)
		}
	}

	first, err := repo.GenerateRoutingProposals(ctx, 0.8)
	if err != nil {
		t.Fatalf("GenerateRoutingProposals first: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("expected 1 first proposal, got %+v", first)
	}

	second, err := repo.GenerateRoutingProposals(ctx, 0.8)
	if err != nil {
		t.Fatalf("GenerateRoutingProposals second: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("expected pending proposal dedupe on second pass, got %+v", second)
	}

	list, err := repo.ListRoutingProposals(ctx)
	if err != nil {
		t.Fatalf("ListRoutingProposals: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected a single persisted pending proposal, got %+v", list)
	}
}

func TestSQLiteLLMKBRepo_GetProfileByAlias_ReturnsAmbiguityError(t *testing.T) {
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
	for _, profile := range []llmkb.LLMProfile{
		{
			LLMID:            "provider-a.chat-pro",
			CanonicalName:    "Chat Pro A",
			Aliases:          []string{"chat-pro"},
			ProviderID:       "provider-a",
			OperationalState: llmkb.OperationalState{AvailabilityState: llmkb.AvailabilityStateAvailable},
			CreatedAt:        time.Now().UTC(),
		},
		{
			LLMID:            "provider-b.chat-pro",
			CanonicalName:    "Chat Pro B",
			Aliases:          []string{"chat-pro"},
			ProviderID:       "provider-b",
			OperationalState: llmkb.OperationalState{AvailabilityState: llmkb.AvailabilityStateAvailable},
			CreatedAt:        time.Now().UTC(),
		},
	} {
		if err := repo.SaveProfile(ctx, profile); err != nil {
			t.Fatalf("SaveProfile(%s): %v", profile.LLMID, err)
		}
	}

	_, err = repo.GetProfileByAlias(ctx, "chat-pro")
	if err == nil {
		t.Fatal("expected alias ambiguity error")
	}
	var ambiguity *LLMKBProfileAliasAmbiguityError
	if !errors.As(err, &ambiguity) {
		t.Fatalf("expected alias ambiguity error, got %T: %v", err, err)
	}
	if len(ambiguity.Matches) != 2 {
		t.Fatalf("expected both matching profile IDs, got %+v", ambiguity.Matches)
	}
}
