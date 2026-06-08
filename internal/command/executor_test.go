package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/schema"
)

func TestExecutorExecute_Success(t *testing.T) {
	var saved []schema.ExecutionOutcome
	exec := NewExecutor(func(ctx context.Context, eo schema.ExecutionOutcome) error {
		saved = append(saved, eo)
		return nil
	})

	desc := Descriptor{
		Type:       schema.CommandTypeQuery,
		UserFacing: true,
		RuntimeSessionID:  "s1",
	}

	ctx := context.Background()
	result, err := exec.Execute(ctx, desc, func(ctx context.Context) (any, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Fatalf("unexpected result: %v", result)
	}
	if len(saved) != 1 {
		t.Fatalf("expected 1 execution outcome, got %d", len(saved))
	}
	eo := saved[0]
	if eo.CommandType != schema.CommandTypeQuery {
		t.Errorf("unexpected command type: %s", eo.CommandType)
	}
	if eo.Outcome != schema.ExecutionOutcomeSucceeded {
		t.Errorf("unexpected outcome: %s", eo.Outcome)
	}
	if eo.FailureClass != "" {
		t.Errorf("expected empty failure class, got %s", eo.FailureClass)
	}
	if eo.Retryable {
		t.Errorf("expected non-retryable for success")
	}
	if eo.StartTime.IsZero() || eo.EndTime == nil || eo.EndTime.IsZero() {
		t.Errorf("expected non-zero timestamps")
	}
}

func TestExecutorExecute_FailureRetryableByIdempotency(t *testing.T) {
	var saved []schema.ExecutionOutcome
	exec := NewExecutor(func(ctx context.Context, eo schema.ExecutionOutcome) error {
		saved = append(saved, eo)
		return nil
	})

	desc := Descriptor{
		Type:       schema.CommandTypeQuery, // idempotent
		UserFacing: true,
		RuntimeSessionID:  "s2",
	}

	ctx := context.Background()
	testErr := errors.New("boom")
	_, err := exec.Execute(ctx, desc, func(ctx context.Context) (any, error) {
		return nil, testErr
	})
	if !errors.Is(err, testErr) {
		t.Fatalf("expected error %v, got %v", testErr, err)
	}
	if len(saved) != 1 {
		t.Fatalf("expected 1 execution outcome, got %d", len(saved))
	}
	eo := saved[0]
	if eo.Outcome != schema.ExecutionOutcomeFailed {
		t.Errorf("unexpected outcome: %s", eo.Outcome)
	}
	if eo.FailureClass != schema.FailureClassExecutionFailure {
		t.Errorf("unexpected failure class: %s", eo.FailureClass)
	}
	if !eo.Retryable {
		t.Errorf("expected retryable for idempotent failure")
	}
}

func TestClassifyFailure_NilAndNonNil(t *testing.T) {
	if got := ClassifyFailure(nil); got != "" {
		t.Errorf("expected empty failure class for nil error, got %s", got)
	}
	if got := ClassifyFailure(errors.New("x")); got != schema.FailureClassExecutionFailure {
		t.Errorf("expected execution_failure, got %s", got)
	}
}

func TestClassifyFailure_ProviderAuthMapsToPermissionDenial(t *testing.T) {
	err := &llm.ProviderError{
		Reason: llm.ReasonAuth,
		Status: 401,
		Err:    errors.New("unauthorized"),
	}
	if got := ClassifyFailure(err); got != schema.FailureClassPermissionDenial {
		t.Fatalf("expected permission_denial, got %s", got)
	}
}

func TestClassifyFailure_TimeoutMapsToTimeout(t *testing.T) {
	if got := ClassifyFailure(context.DeadlineExceeded); got != schema.FailureClassTimeout {
		t.Fatalf("expected timeout, got %s", got)
	}
}

func TestIsRetryable_ByFailureClassAndIdempotency(t *testing.T) {
	if !IsRetryable(schema.CommandTypeQuery, schema.FailureClassConnectorUnavailable) {
		t.Errorf("expected retryable for connector_unavailable")
	}
	if IsRetryable(schema.CommandTypeCreate, schema.FailureClassExecutionFailure) {
		t.Errorf("expected non-retryable for non-idempotent execution failure")
	}
}

func TestExecutorExecute_NoSaverConfigured(t *testing.T) {
	exec := NewExecutor(nil)
	ctx := context.Background()
	start := time.Now()
	res, err := exec.Execute(ctx, Descriptor{Type: schema.CommandTypeQuery}, func(ctx context.Context) (any, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != "ok" {
		t.Fatalf("unexpected result: %v", res)
	}
	// No panic and no additional assertions; ensures path without saver behaves like a direct call.
	_ = start
}
