package navi

import (
	"testing"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

func TestNewStatusTracker_StartsIdle(t *testing.T) {
	tr := NewStatusTracker()
	snap := tr.Snapshot()
	if snap.State != schema.AgentStateIdle {
		t.Fatalf("expected idle, got %s", snap.State)
	}
	if snap.TurnsProcessed != 0 {
		t.Fatalf("expected 0 turns, got %d", snap.TurnsProcessed)
	}
}

func TestStatusTracker_SetState(t *testing.T) {
	tr := NewStatusTracker()
	tr.SetState(schema.AgentStateProcessing, "sess-1", "")

	snap := tr.Snapshot()
	if snap.State != schema.AgentStateProcessing {
		t.Fatalf("expected processing, got %s", snap.State)
	}
	if snap.ActiveRuntimeSessionID != "sess-1" {
		t.Fatalf("expected session sess-1, got %s", snap.ActiveRuntimeSessionID)
	}
}

func TestStatusTracker_ToolExecuting(t *testing.T) {
	tr := NewStatusTracker()
	tr.SetState(schema.AgentStateToolExecuting, "sess-1", "read_file")

	snap := tr.Snapshot()
	if snap.State != schema.AgentStateToolExecuting {
		t.Fatalf("expected tool_executing, got %s", snap.State)
	}
	if snap.CurrentDetail != "read_file" {
		t.Fatalf("expected detail read_file, got %s", snap.CurrentDetail)
	}
}

func TestStatusTracker_RecordTurnComplete(t *testing.T) {
	tr := NewStatusTracker()
	tr.SetState(schema.AgentStateProcessing, "sess-1", "")
	tr.RecordTurnComplete()

	snap := tr.Snapshot()
	if snap.State != schema.AgentStateIdle {
		t.Fatalf("expected idle after turn complete, got %s", snap.State)
	}
	if snap.TurnsProcessed != 1 {
		t.Fatalf("expected 1 turn, got %d", snap.TurnsProcessed)
	}
	if snap.ActiveRuntimeSessionID != "" {
		t.Fatalf("expected empty session, got %s", snap.ActiveRuntimeSessionID)
	}
}

func TestStatusTracker_StalenessDetection(t *testing.T) {
	tr := NewStatusTracker()
	// Simulate stale state by backdating the lastUpdatedAt.
	tr.SetState(schema.AgentStateProcessing, "sess-1", "")
	staleTime := time.Now().UTC().Add(-(StalenessThreshold + time.Minute))
	tr.lastUpdatedAt.Store(staleTime)

	snap := tr.Snapshot()
	if snap.State != schema.AgentStateUnresponsive {
		t.Fatalf("expected unresponsive for stale processing state, got %s", snap.State)
	}
}

func TestStatusTracker_IdleNotStale(t *testing.T) {
	tr := NewStatusTracker()
	// Idle state should never be reported as unresponsive, even if lastUpdatedAt is old.
	staleTime := time.Now().UTC().Add(-(StalenessThreshold + time.Minute))
	tr.lastUpdatedAt.Store(staleTime)

	snap := tr.Snapshot()
	if snap.State != schema.AgentStateIdle {
		t.Fatalf("expected idle (not unresponsive) for stale idle state, got %s", snap.State)
	}
}

func TestStatusTracker_Touch(t *testing.T) {
	tr := NewStatusTracker()
	tr.SetState(schema.AgentStateProcessing, "sess-1", "")
	// Backdate then touch to prevent staleness.
	staleTime := time.Now().UTC().Add(-(StalenessThreshold + time.Minute))
	tr.lastUpdatedAt.Store(staleTime)
	tr.Touch()

	snap := tr.Snapshot()
	if snap.State != schema.AgentStateProcessing {
		t.Fatalf("expected processing after touch, got %s", snap.State)
	}
}

func TestStatusTracker_MultipleTurns(t *testing.T) {
	tr := NewStatusTracker()
	for i := 0; i < 5; i++ {
		tr.SetState(schema.AgentStateProcessing, "sess-1", "")
		tr.RecordTurnComplete()
	}
	snap := tr.Snapshot()
	if snap.TurnsProcessed != 5 {
		t.Fatalf("expected 5 turns, got %d", snap.TurnsProcessed)
	}
}
