package governor

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultGovernorConfig()
	if cfg.MaxActionBudget <= 0 {
		t.Fatal("expected positive action budget")
	}
	if cfg.CostCeiling <= 0 {
		t.Fatal("expected positive cost ceiling")
	}
}

func TestRecordActionWithinBudget(t *testing.T) {
	g := NewGovernor(GovernorConfig{
		MaxActionBudget:    5,
		MaxRetries:         3,
		CostCeiling:        10.0,
		AutonomousDuration: 1 * time.Minute,
	}, ".")
	for i := 0; i < 5; i++ {
		if err := g.RecordAction(); err != nil {
			t.Fatalf("unexpected trip at action %d: %v", i+1, err)
		}
	}
}

func TestRecordActionExceedsBudget(t *testing.T) {
	g := NewGovernor(GovernorConfig{
		MaxActionBudget:    2,
		MaxRetries:         3,
		CostCeiling:        10.0,
		AutonomousDuration: 1 * time.Minute,
	}, ".")
	g.RecordAction()        // 1
	g.RecordAction()        // 2
	err := g.RecordAction() // 3 — over budget
	if err == nil {
		t.Fatal("expected governor to trip on action budget")
	}
	var tripped *ErrGovernorTripped
	if !errors.As(err, &tripped) {
		t.Fatalf("expected ErrGovernorTripped, got %T", err)
	}
	if tripped.Type != GovernorActionBudget {
		t.Fatalf("expected action_budget governor, got %q", tripped.Type)
	}
}

func TestRecordRetryExceedsLimit(t *testing.T) {
	g := NewGovernor(GovernorConfig{
		MaxActionBudget:    100,
		MaxRetries:         1,
		CostCeiling:        10.0,
		AutonomousDuration: 1 * time.Minute,
	}, ".")
	g.RecordRetry()        // 1
	err := g.RecordRetry() // 2 — over limit
	if err == nil {
		t.Fatal("expected governor to trip on retry limit")
	}
	var tripped *ErrGovernorTripped
	if !errors.As(err, &tripped) {
		t.Fatalf("expected ErrGovernorTripped, got %T", err)
	}
	if tripped.Type != GovernorRetryLimit {
		t.Fatalf("expected retry_limit governor, got %q", tripped.Type)
	}
}

func TestRecordCostExceedsCeiling(t *testing.T) {
	g := NewGovernor(GovernorConfig{
		MaxActionBudget:    100,
		MaxRetries:         10,
		CostCeiling:        0.50,
		AutonomousDuration: 1 * time.Minute,
	}, ".")
	if err := g.RecordCost(0.30); err != nil {
		t.Fatalf("unexpected trip: %v", err)
	}
	err := g.RecordCost(0.30) // total: 0.60 — over ceiling
	if err == nil {
		t.Fatal("expected governor to trip on cost ceiling")
	}
	var tripped *ErrGovernorTripped
	if !errors.As(err, &tripped) {
		t.Fatalf("expected ErrGovernorTripped, got %T", err)
	}
	if tripped.Type != GovernorCostCeiling {
		t.Fatalf("expected cost_ceiling governor, got %q", tripped.Type)
	}
}

func TestCheckDurationWithinLimit(t *testing.T) {
	g := NewGovernor(GovernorConfig{
		MaxActionBudget:    100,
		MaxRetries:         10,
		CostCeiling:        10.0,
		AutonomousDuration: 500 * time.Millisecond,
	}, ".")
	if err := g.CheckDuration(); err != nil {
		t.Fatalf("unexpected trip: %v", err)
	}
}

func TestCheckDurationExceedsLimit(t *testing.T) {
	g := NewGovernor(GovernorConfig{
		MaxActionBudget:    100,
		MaxRetries:         10,
		CostCeiling:        10.0,
		AutonomousDuration: 10 * time.Millisecond,
	}, ".")
	time.Sleep(20 * time.Millisecond)
	err := g.CheckDuration()
	if err == nil {
		t.Fatal("expected governor to trip on autonomous duration")
	}
	var tripped *ErrGovernorTripped
	if !errors.As(err, &tripped) {
		t.Fatalf("expected ErrGovernorTripped, got %T", err)
	}
	if tripped.Type != GovernorAutonomousDuration {
		t.Fatalf("expected autonomous_duration governor, got %q", tripped.Type)
	}
}

func TestStatsAccumulate(t *testing.T) {
	g := NewGovernor(GovernorConfig{
		MaxActionBudget:    100,
		MaxRetries:         100,
		CostCeiling:        10.0,
		AutonomousDuration: 1 * time.Minute,
	}, ".")
	g.RecordAction()
	g.RecordAction()
	g.RecordRetry()
	g.RecordCost(0.25)
	g.RecordCost(0.10)

	stats := g.Stats()
	if stats.Actions != 2 {
		t.Fatalf("expected 2 actions, got %d", stats.Actions)
	}
	if stats.Retries != 1 {
		t.Fatalf("expected 1 retry, got %d", stats.Retries)
	}
	// Use range check to avoid float64 precision issues.
	if stats.TotalUSD < 0.349 || stats.TotalUSD > 0.351 {
		t.Fatalf("expected ~$0.35, got $%.10f", stats.TotalUSD)
	}
}

func TestRecordRuntimeSessionActionWithinBudget(t *testing.T) {
	g := NewGovernor(GovernorConfig{
		MaxActionBudget:    100,
		MaxRetries:         10,
		CostCeiling:        10.0,
		AutonomousDuration: 1 * time.Minute,
	}, ".")
	for i := 0; i < 3; i++ {
		count, err := g.RecordRuntimeSessionAction("session-1", 3)
		if err != nil {
			t.Fatalf("unexpected trip at agent action %d: %v", i+1, err)
		}
		if count != i+1 {
			t.Fatalf("expected count %d, got %d", i+1, count)
		}
	}
}

func TestRecordRuntimeSessionActionExceedsBudget(t *testing.T) {
	g := NewGovernor(GovernorConfig{
		MaxActionBudget:    100,
		MaxRetries:         10,
		CostCeiling:        10.0,
		AutonomousDuration: 1 * time.Minute,
	}, ".")
	// Allow 2 actions per session
	_, _ = g.RecordRuntimeSessionAction("session-1", 2)        // 1
	_, _ = g.RecordRuntimeSessionAction("session-1", 2)        // 2
	count, err := g.RecordRuntimeSessionAction("session-1", 2) // 3 — over budget
	if err == nil {
		t.Fatal("expected agent budget exceeded error")
	}
	if count != 3 {
		t.Fatalf("expected count 3, got %d", count)
	}
	// Verify a different session is not affected
	_, err2 := g.RecordRuntimeSessionAction("session-2", 2)
	if err2 != nil {
		t.Fatalf("session-2 should not be affected by session-1's budget: %v", err2)
	}
}

func TestResetRuntimeSessionBudget(t *testing.T) {
	g := NewGovernor(GovernorConfig{
		MaxActionBudget:    100,
		MaxRetries:         10,
		CostCeiling:        10.0,
		AutonomousDuration: 1 * time.Minute,
	}, ".")
	_, _ = g.RecordRuntimeSessionAction("session-1", 2) // 1
	_, _ = g.RecordRuntimeSessionAction("session-1", 2) // 2
	// Reset
	g.ResetRuntimeSessionBudget("session-1")
	// Should be fresh again
	count, err := g.RecordRuntimeSessionAction("session-1", 2)
	if err != nil {
		t.Fatalf("expected no error after reset: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected count 1 after reset, got %d", count)
	}
}

func TestErrGovernorTrippedMessage(t *testing.T) {
	err := &ErrGovernorTripped{
		Type:   GovernorActionBudget,
		Limit:  "10",
		Actual: "11",
	}
	msg := err.Error()
	if msg == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestCheckPathRestricted(t *testing.T) {
	g := NewGovernor(GovernorConfig{RestrictToWorkspace: true}, ".")

	if err := g.CheckPath("valid/relative/path.txt"); err != nil {
		t.Fatalf("unexpected error for valid relative path: %v", err)
	}

	absPath, _ := filepath.Abs("/etc/passwd")

	if err := g.CheckPath(absPath); err == nil {
		t.Fatal("expected error for absolute path when constrained")
	}

	if err := g.CheckPath("../../../outside.txt"); err == nil {
		t.Fatal("expected error for path escape attempt")
	}
}

func TestCheckPathUnrestricted(t *testing.T) {
	g := NewGovernor(GovernorConfig{RestrictToWorkspace: false}, ".")

	if err := g.CheckPath("valid/relative/path.txt"); err != nil {
		t.Fatalf("unexpected error for valid relative path: %v", err)
	}

	absPath, _ := filepath.Abs("/etc/passwd")

	if err := g.CheckPath(absPath); err != nil {
		t.Fatalf("unexpected error for absolute path when unconstrained: %v", err)
	}

	if err := g.CheckPath("../../../outside.txt"); err != nil {
		t.Fatalf("unexpected error for path escape log when unconstrained: %v", err)
	}
}

func TestGovernorRecordRepetition(t *testing.T) {
	g := NewGovernor(GovernorConfig{
		MaxRepetitions: 3,
	}, ".")

	// 1st time
	if err := g.RecordRepetition("run-1", "Hello"); err != nil {
		t.Fatalf("unexpected trip at 1st repetition: %v", err)
	}
	// 2nd time
	if err := g.RecordRepetition("run-1", "Hello"); err != nil {
		t.Fatalf("unexpected trip at 2nd repetition: %v", err)
	}
	// 3rd time - should trip
	err := g.RecordRepetition("run-1", "Hello")
	if err == nil {
		t.Fatal("expected governor to trip on repetition limit")
	}

	var tripped *ErrGovernorTripped
	if !errors.As(err, &tripped) {
		t.Fatalf("expected ErrGovernorTripped, got %T", err)
	}
	if tripped.Type != GovernorRepetition {
		t.Fatalf("expected repetition_limit governor, got %q", tripped.Type)
	}

	// Reset by new content
	g2 := NewGovernor(GovernorConfig{
		MaxRepetitions: 3,
	}, ".")
	g2.RecordRepetition("run-1", "Hello")
	g2.RecordRepetition("run-1", "World") // different
	if err := g2.RecordRepetition("run-1", "World"); err != nil {
		t.Fatalf("unexpected trip after reset: %v", err)
	}

	// Sessions are isolated: "run-2" should start fresh even if "run-1" has tripped
	g3 := NewGovernor(GovernorConfig{MaxRepetitions: 3}, ".")
	g3.RecordRepetition("run-1", "X")
	g3.RecordRepetition("run-1", "X")
	g3.RecordRepetition("run-1", "X") // trips run-1
	if err := g3.RecordRepetition("run-2", "X"); err != nil {
		t.Fatalf("run-2 should be unaffected by run-1 trip: %v", err)
	}
}

// TestGovernorRepetitionDisabledOnNonPositiveLimit guards against the
// misconfiguration where MaxRepetitions=0 tripped on the very first response
// (count 1 >= 0), wedging the orchestrator loop. A non-positive limit must
// disable the check entirely rather than trip instantly.
func TestGovernorRepetitionDisabledOnNonPositiveLimit(t *testing.T) {
	for _, limit := range []int{0, -1} {
		g := NewGovernor(GovernorConfig{MaxRepetitions: limit}, ".")
		for i := 0; i < 10; i++ {
			if err := g.RecordRepetition("run-1", "same"); err != nil {
				t.Fatalf("MaxRepetitions=%d must disable the check, tripped on response %d: %v", limit, i+1, err)
			}
		}
	}
}
