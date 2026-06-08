package backlog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseBacklogFile_FirstPending(t *testing.T) {
	path := filepath.Join("..", "..", "..", "docs", "tasks", "autonomous-agent-readiness-backlog.md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("backlog file not found (run from repo root)")
	}
	tasks, err := ParseBacklogFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) == 0 {
		t.Fatal("expected at least one task")
	}
	first := FirstPending(tasks)
	if first != nil {
		if first.Status != "PENDING" {
			t.Errorf("FirstPending returned status %q", first.Status)
		}
	}
}

func TestFirstPending_Order(t *testing.T) {
	tasks := []Task{
		{ID: "1", Priority: "HIGH", Status: "PENDING"},
		{ID: "2", Priority: "CRITICAL", Status: "PENDING"},
		{ID: "3", Priority: "MEDIUM", Status: "PENDING"},
	}
	got := FirstPending(tasks)
	if got == nil || got.ID != "2" {
		t.Errorf("expected CRITICAL task ID 2, got %v", got)
	}
}

func TestFirstPending_None(t *testing.T) {
	tasks := []Task{
		{ID: "1", Priority: "HIGH", Status: "DONE"},
	}
	got := FirstPending(tasks)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}
