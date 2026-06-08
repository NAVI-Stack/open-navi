package runtime

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNewRun(t *testing.T) {
	run := NewRun("sess-1", "navi")
	if run.RunID == "" {
		t.Fatal("expected non-empty RunID")
	}
	if run.RuntimeSessionID != "sess-1" {
		t.Errorf("expected RuntimeSessionID 'sess-1', got %q", run.RuntimeSessionID)
	}
	if run.ExperienceMode != "navi" {
		t.Errorf("expected ExperienceMode 'navi', got %q", run.ExperienceMode)
	}
	if run.Status != "starting" {
		t.Errorf("expected Status 'starting', got %q", run.Status)
	}
	if run.StartedAt.IsZero() {
		t.Error("expected StartedAt to be set")
	}
	if run.ToolCalls != 0 {
		t.Errorf("expected 0 tool calls, got %d", run.ToolCalls)
	}
}

func TestRunState_SetStatus(t *testing.T) {
	run := NewRun("sess-1", "navi")
	before := run.UpdatedAt

	time.Sleep(time.Millisecond)
	run.SetStatus("active")

	if run.Status != "active" {
		t.Errorf("expected Status 'active', got %q", run.Status)
	}
	if !run.UpdatedAt.After(before) {
		t.Error("expected UpdatedAt to advance after SetStatus")
	}
}

func TestRunState_IncrementToolCalls(t *testing.T) {
	run := NewRun("sess-1", "navi")
	if run.ToolCalls != 0 {
		t.Fatalf("expected 0 tool calls initially")
	}
	run.IncrementToolCalls()
	run.IncrementToolCalls()
	if run.ToolCalls != 2 {
		t.Errorf("expected 2 tool calls, got %d", run.ToolCalls)
	}
}

func TestRunState_DurationMs(t *testing.T) {
	run := NewRun("sess-1", "navi")
	time.Sleep(5 * time.Millisecond)
	d := run.DurationMs()
	if d < 1 {
		t.Errorf("expected DurationMs >= 1, got %d", d)
	}
}

func TestRunStateJSONUsesExperienceModeField(t *testing.T) {
	run := NewRun("sess-1", "navi")
	b, err := json.Marshal(run)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, `"experience_mode":"navi"`) {
		t.Fatalf("expected experience_mode field in %s", got)
	}
	if strings.Contains(got, `"persona_id"`) {
		t.Fatalf("did not expect legacy persona_id field in %s", got)
	}
}

func TestCheckpointJSONUsesExperienceModeField(t *testing.T) {
	run := NewRun("sess-1", "navi")
	cp := NewCheckpoint(run)
	b, err := json.Marshal(cp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, `"experience_mode":"navi"`) {
		t.Fatalf("expected experience_mode field in %s", got)
	}
	if strings.Contains(got, `"persona_id"`) {
		t.Fatalf("did not expect legacy persona_id field in %s", got)
	}
}
