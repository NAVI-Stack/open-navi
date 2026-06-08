package heartbeat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func seedHeartbeatTasks(t *testing.T, svc *HeartbeatService) {
	t.Helper()
	p := filepath.Join(svc.workspaceDir, heartbeatFile)
	if err := os.WriteFile(p, []byte("- [ ] test heartbeat item\n"), 0o644); err != nil {
		t.Fatalf("seed heartbeat md: %v", err)
	}
}

func TestRequestWake_priorityMergeAndCoalesce(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir, time.Hour)
	seedHeartbeatTasks(t, svc)

	var lastNotes string
	svc.SetHandler(func(_ context.Context, _ string, prompt string) error {
		idx := strings.Index(prompt, "## Out-of-cycle wake notes")
		if idx >= 0 {
			lastNotes = strings.TrimSpace(prompt[idx+len("## Out-of-cycle wake notes"):])
		}
		return nil
	})

	svc.wakeCoalesce = 20 * time.Millisecond
	svc.Start(context.Background())

	svc.RequestWake("low", int(WakePriorityDefault), "")
	svc.RequestWake("retry-only", int(WakePriorityRetry), "")
	svc.RequestWake("ignored-lower-pri", int(WakePriorityDefault), "")
	time.Sleep(60 * time.Millisecond)

	lastNotes = strings.TrimSpace(lastNotes)
	if lastNotes != "retry-only" {
		t.Fatalf("got notes %q", lastNotes)
	}
	svc.Stop()
}

func TestRequestWake_suppressedQuietHoursSkipsWake(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := New(dir, time.Hour)
	seedHeartbeatTasks(t, svc)
	svc.now = func() time.Time {
		return time.Date(2030, 1, 1, 23, 0, 0, 0, time.UTC)
	}
	svc.SetTimezone("UTC")
	svc.SetQuietHours("22:00", "08:00")
	var invoked bool
	svc.SetHandler(func(context.Context, string, string) error {
		invoked = true
		return nil
	})
	svc.wakeCoalesce = 15 * time.Millisecond
	svc.Start(context.Background())

	svc.RequestWakeNow("cron-test")
	time.Sleep(50 * time.Millisecond)
	if invoked {
		t.Fatal("wake should have been suppressed by quiet hours")
	}
	svc.Stop()
}
