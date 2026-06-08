package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/bus"
	"github.com/open-navi/navi/internal/governor"
)

func testLoopConfig(t *testing.T) LoopConfig {
	t.Helper()
	mbus := bus.NewMemBus(nil)
	return LoopConfig{
		Bus: mbus,
		LLM: NewStubAdapter(),
		Governor: governor.NewGovernor(governor.GovernorConfig{
			MaxActionBudget:    10,
			MaxRetries:         3,
			CostCeiling:        1.0,
			AutonomousDuration: time.Minute,
		}, "."),
		TickInterval: 10 * time.Millisecond,
	}
}

func TestRunOnceEmptyStateNoOp(t *testing.T) {
	cfg := testLoopConfig(t)
	ctx := context.Background()

	// No tasks, no agents — stub returns empty decision.
	if err := RunOnce(ctx, cfg); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	stats := cfg.Governor.Stats()
	if stats.Actions != 1 {
		t.Fatalf("expected 1 action (the tick itself), got %d", stats.Actions)
	}
}

type idleAwareNoOpAdapter struct{}

func (idleAwareNoOpAdapter) HasWork(ctx context.Context) (bool, error) {
	return false, nil
}

func (idleAwareNoOpAdapter) Decide(ctx context.Context) (Decision, error) {
	return Decision{}, nil
}

func TestRunOnceIdleAwareNoOpSkipsGovernorAction(t *testing.T) {
	cfg := testLoopConfig(t)
	cfg.LLM = idleAwareNoOpAdapter{}

	if err := RunOnce(context.Background(), cfg); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if got := cfg.Governor.Stats().Actions; got != 0 {
		t.Fatalf("expected idle-aware no-op tick to consume 0 actions, got %d", got)
	}
}

func TestGovernorActionBudgetTrips(t *testing.T) {
	cfg := testLoopConfig(t)
	cfg.Governor = governor.NewGovernor(governor.GovernorConfig{
		MaxActionBudget:    1,
		MaxRetries:         3,
		CostCeiling:        1.0,
		AutonomousDuration: time.Minute,
	}, ".")
	ctx := context.Background()

	// First two ticks OK, third should trip.
	_ = RunOnce(ctx, cfg)    // 1
	_ = RunOnce(ctx, cfg)    // 2
	err := RunOnce(ctx, cfg) // 3 — should trip
	if err == nil {
		t.Fatal("expected governor trip on action budget")
	}
}

func TestGovernorCostCeilingTrips(t *testing.T) {
	cfg := testLoopConfig(t)
	stub := NewStubAdapter()
	stub.CostPerCall = 0.60
	cfg.LLM = stub
	cfg.Governor = governor.NewGovernor(governor.GovernorConfig{
		MaxActionBudget:    100,
		MaxRetries:         10,
		CostCeiling:        0.50,
		AutonomousDuration: 5 * time.Second,
	}, ".")
	ctx := context.Background()

	// First tick costs $0.60 which exceeds $0.50 ceiling — should trip.
	err := RunOnce(ctx, cfg)
	if err == nil {
		t.Fatal("expected governor trip on cost ceiling")
	}
}

func TestLoopExitsOnContextCancel(t *testing.T) {
	cfg := testLoopConfig(t)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- RunLoop(ctx, cfg)
	}()

	// Let one tick happen, then cancel.
	time.Sleep(50 * time.Millisecond)
	cancel()

	err := <-done
	if err != nil && err != context.Canceled && !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// The remaining tests in this file focus on governor behaviour (action budget,
// cost ceiling, and loop cancellation) and do not depend on task/worker state.
