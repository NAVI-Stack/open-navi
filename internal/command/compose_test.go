package command

import (
	"context"
	"errors"
	"testing"

	"github.com/open-navi/navi/internal/schema"
)

func TestRunCompose_Success(t *testing.T) {
	var saved []schema.ExecutionOutcome
	saver := func(_ context.Context, eo schema.ExecutionOutcome) error {
		saved = append(saved, eo)
		return nil
	}
	steps := []Step{
		{CommandType: schema.CommandTypeQuery, Reversibility: schema.ReversibilityInternal, Run: func(ctx context.Context) (any, error) { return "a", nil }},
		{CommandType: schema.CommandTypeInvoke, Reversibility: schema.ReversibilityCompensable, Run: func(ctx context.Context) (any, error) { return "b", nil }},
	}
	ctx := context.Background()
	result, comp, err := RunCompose(ctx, Descriptor{RuntimeSessionID: "s1"}, steps, schema.ComposeFailureSurfacePartial, saver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if comp == nil || comp.Outcome != schema.ExecutionOutcomeSucceeded {
		t.Errorf("expected success, got outcome %v", comp)
	}
	if result != "b" {
		t.Errorf("expected result b, got %v", result)
	}
	if len(saved) != 3 {
		t.Errorf("expected 3 outcomes (2 steps + 1 composite), got %d", len(saved))
	}
}

func TestRunCompose_FailureSurfacePartial(t *testing.T) {
	var saved []schema.ExecutionOutcome
	saver := func(_ context.Context, eo schema.ExecutionOutcome) error {
		saved = append(saved, eo)
		return nil
	}
	boom := errors.New("step failed")
	steps := []Step{
		{CommandType: schema.CommandTypeQuery, Reversibility: schema.ReversibilityInternal, Run: func(ctx context.Context) (any, error) { return "a", nil }},
		{CommandType: schema.CommandTypeInvoke, Reversibility: schema.ReversibilityCompensable, Run: func(ctx context.Context) (any, error) { return nil, boom }},
	}
	ctx := context.Background()
	result, comp, err := RunCompose(ctx, Descriptor{RuntimeSessionID: "s1"}, steps, schema.ComposeFailureSurfacePartial, saver)
	if err != boom {
		t.Fatalf("expected error %v, got %v", boom, err)
	}
	if comp == nil || comp.Outcome != schema.ExecutionOutcomePartiallySucceeded {
		t.Errorf("expected partially_succeeded, got %v", comp)
	}
	if comp.RecoveryStatus != schema.RecoveryStatusOpen {
		t.Errorf("expected recovery_status open, got %s", comp.RecoveryStatus)
	}
	if result != nil {
		t.Errorf("expected nil result on failed step, got %v", result)
	}
	if len(saved) != 3 {
		t.Errorf("expected 3 outcomes, got %d", len(saved))
	}
}
