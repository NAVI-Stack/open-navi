package navi

import (
	"testing"
	"time"
)

func TestExtractToolEventsFromThread(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	thread := &ChatThread{
		Messages: []ChatMessage{
			{Role: "user", Content: "hi", CreatedAt: now},
			{
				Role:      "assistant",
				Content:   "done",
				CreatedAt: now.Add(time.Minute),
				Metadata: map[string]any{
					"toolParts": []any{
						map[string]any{"toolName": "read_file", "state": "result", "isError": false},
						map[string]any{"toolName": "list_dir", "state": "result", "isError": true},
					},
				},
			},
			// Assistant message with no tool parts — ignored.
			{Role: "assistant", Content: "plain", CreatedAt: now.Add(2 * time.Minute), Metadata: map[string]any{}},
		},
	}

	events := extractToolEventsFromThread(thread)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[0].ToolName != "read_file" || events[0].IsError {
		t.Errorf("event0 = %+v", events[0])
	}
	if events[1].ToolName != "list_dir" || !events[1].IsError {
		t.Errorf("event1 = %+v", events[1])
	}
	if !events[0].UsedAt.Equal(now.Add(time.Minute)) {
		t.Errorf("UsedAt = %v, want message time", events[0].UsedAt)
	}
}

func TestExtractToolEventsFromThread_EmptyAndNil(t *testing.T) {
	if events := extractToolEventsFromThread(nil); events != nil {
		t.Errorf("nil thread = %v, want nil", events)
	}
	empty := &ChatThread{Messages: []ChatMessage{{Role: "assistant", Content: "x"}}}
	if events := extractToolEventsFromThread(empty); len(events) != 0 {
		t.Errorf("no toolParts = %v, want empty (no fabrication)", events)
	}
}
