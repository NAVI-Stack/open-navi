package store

import (
	"context"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

func TestExecutionOutcomeRoundTripIncludesLLMMetadata(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	now := time.Now().UTC()
	end := now.Add(time.Second)
	eo := schema.ExecutionOutcome{
		AttemptID:          "attempt-1",
		CommandID:          "command-1",
		AttemptNumber:      1,
		CommandType:        schema.CommandTypeInvoke,
		StartTime:          now,
		EndTime:            &end,
		Outcome:            schema.ExecutionOutcomeSucceeded,
		AffectedEntities:   "[]",
		CompensationStatus: schema.CompensationStatusNotRequired,
		RecoveryStatus:     schema.RecoveryStatusNotRequired,
		LLMProvider:        "anthropic",
		LLMModel:           "claude-sonnet-4-20250514",
		LLMTaskClass:       "coding",
		LLMComplexity:      "high",
	}
	if err := SaveExecutionOutcome(ctx, db, eo); err != nil {
		t.Fatalf("SaveExecutionOutcome: %v", err)
	}
	got, err := GetExecutionOutcomeByAttemptID(ctx, db, eo.AttemptID)
	if err != nil {
		t.Fatalf("GetExecutionOutcomeByAttemptID: %v", err)
	}
	if got == nil {
		t.Fatal("expected stored execution outcome")
	}
	if got.LLMProvider != eo.LLMProvider || got.LLMModel != eo.LLMModel || got.LLMTaskClass != eo.LLMTaskClass || got.LLMComplexity != eo.LLMComplexity {
		t.Fatalf("llm metadata mismatch: got %+v want %+v", got, eo)
	}
}

func TestUpdateExecutionOutcomesApprovalByProposalID(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()

	first := schema.ExecutionOutcome{
		AttemptID:          "attempt-proposal-1",
		CommandID:          "command-proposal-1",
		AttemptNumber:      1,
		CommandType:        schema.CommandTypeUpdate,
		StartTime:          now,
		Outcome:            schema.ExecutionOutcomeRejectedPreExecution,
		AffectedEntities:   "[]",
		CompensationStatus: schema.CompensationStatusNotRequired,
		RecoveryStatus:     schema.RecoveryStatusNotRequired,
		ProposalID:         "proposal-approval",
		ApprovalRequired:   true,
		ApprovalOutcome:    schema.ApprovalOutcomeNA,
	}
	second := first
	second.AttemptID = "attempt-proposal-2"
	second.CommandID = "command-proposal-2"
	second.ProposalID = "proposal-other"

	if err := SaveExecutionOutcome(ctx, db, first); err != nil {
		t.Fatalf("SaveExecutionOutcome(first): %v", err)
	}
	if err := SaveExecutionOutcome(ctx, db, second); err != nil {
		t.Fatalf("SaveExecutionOutcome(second): %v", err)
	}

	if err := UpdateExecutionOutcomesApprovalByProposalID(ctx, db, "proposal-approval", schema.ApprovalOutcomeAlwaysAllow, true); err != nil {
		t.Fatalf("UpdateExecutionOutcomesApprovalByProposalID: %v", err)
	}

	gotFirst, err := GetExecutionOutcomeByAttemptID(ctx, db, first.AttemptID)
	if err != nil {
		t.Fatalf("GetExecutionOutcomeByAttemptID(first): %v", err)
	}
	if gotFirst == nil {
		t.Fatal("expected first execution outcome")
	}
	if !gotFirst.ApprovalRequired || gotFirst.ApprovalOutcome != schema.ApprovalOutcomeAlwaysAllow {
		t.Fatalf("expected updated approval metadata, got %+v", gotFirst)
	}

	gotSecond, err := GetExecutionOutcomeByAttemptID(ctx, db, second.AttemptID)
	if err != nil {
		t.Fatalf("GetExecutionOutcomeByAttemptID(second): %v", err)
	}
	if gotSecond == nil {
		t.Fatal("expected second execution outcome")
	}
	if gotSecond.ApprovalOutcome != schema.ApprovalOutcomeNA {
		t.Fatalf("expected unrelated outcome to remain unchanged, got %+v", gotSecond)
	}
}
