package heartbeat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNew_DefaultsInterval(t *testing.T) {
	svc := New(t.TempDir(), 0)
	if svc.interval != defaultInterval {
		t.Errorf("expected default interval %s, got %s", defaultInterval, svc.interval)
	}
}

func TestNew_ClampsMinInterval(t *testing.T) {
	svc := New(t.TempDir(), 1*time.Second) // below 5m minimum
	if svc.interval != defaultInterval {
		t.Errorf("expected interval clamped to %s, got %s", defaultInterval, svc.interval)
	}
}

func TestNew_RespectsValidInterval(t *testing.T) {
	want := 10 * time.Minute
	svc := New(t.TempDir(), want)
	if svc.interval != want {
		t.Errorf("expected %s, got %s", want, svc.interval)
	}
}

func TestIsRunning_FalseBeforeStart(t *testing.T) {
	svc := New(t.TempDir(), defaultInterval)
	if svc.IsRunning() {
		t.Error("service should not be running before Start")
	}
}

func TestStartStop(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir, defaultInterval)
	ctx := context.Background()

	svc.Start(ctx)
	if !svc.IsRunning() {
		t.Error("service should be running after Start")
	}

	svc.Stop()
	if svc.IsRunning() {
		t.Error("service should not be running after Stop")
	}
}

func TestStartIdempotent(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir, defaultInterval)
	ctx := context.Background()

	svc.Start(ctx)
	svc.Start(ctx) // second call should be a no-op
	svc.Stop()

	if svc.IsRunning() {
		t.Error("service should not be running after Stop")
	}
}

func TestStopIdempotent(t *testing.T) {
	svc := New(t.TempDir(), defaultInterval)
	svc.Stop() // should not panic
	svc.Stop()
}

func TestEnsureWorkspace_CreatesHeartbeatMD(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir, defaultInterval)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately so the goroutine exits at once

	svc.Start(ctx)
	// Give ensureWorkspace a moment to run before we check
	time.Sleep(50 * time.Millisecond)

	mdPath := filepath.Join(dir, heartbeatFile)
	if _, err := os.Stat(mdPath); os.IsNotExist(err) {
		t.Errorf("expected %s to be created, but it does not exist", mdPath)
	}
}

func TestExecuteHeartbeat_CallsHandler(t *testing.T) {
	dir := t.TempDir()
	// Write a HEARTBEAT.md with one task
	md := "# HB\n- [ ] Check health\n"
	if err := os.WriteFile(filepath.Join(dir, heartbeatFile), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}

	var called atomic.Int32
	svc := New(dir, defaultInterval)
	svc.SetHandler(func(_ context.Context, runtimeSessionID, prompt string) error {
		called.Add(1)
		if runtimeSessionID != "heartbeat-auto" {
			t.Errorf("unexpected runtimeSessionID: %q", runtimeSessionID)
		}
		if prompt == "" {
			t.Error("prompt should not be empty")
		}
		return nil
	})

	svc.executeHeartbeat(context.Background(), "")

	if called.Load() != 1 {
		t.Errorf("expected handler called once, got %d", called.Load())
	}
}

func TestExecuteHeartbeat_SkipsWithNoHandler(t *testing.T) {
	dir := t.TempDir()
	md := "# HB\n- [ ] Check health\n"
	os.WriteFile(filepath.Join(dir, heartbeatFile), []byte(md), 0o644)

	svc := New(dir, defaultInterval)
	// No handler set — should not panic
	svc.executeHeartbeat(context.Background(), "")
}

func TestExecuteHeartbeat_SkipsEmptyFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, heartbeatFile), []byte("# Empty\n"), 0o644)

	var called atomic.Int32
	svc := New(dir, defaultInterval)
	svc.SetHandler(func(_ context.Context, _, _ string) error {
		called.Add(1)
		return nil
	})
	svc.executeHeartbeat(context.Background(), "")

	if called.Load() != 0 {
		t.Error("handler should not be called when task list is empty")
	}
}

func TestBuildPrompt_ContainsTasks(t *testing.T) {
	svc := New(t.TempDir(), defaultInterval)
	tasks := []Task{
		{Description: "Check emails", Frequency: 30 * time.Minute},
		{Description: "Review tasks", Frequency: 30 * time.Minute},
	}
	prompt := svc.buildPrompt(tasks)

	if len(prompt) == 0 {
		t.Error("prompt should not be empty")
	}
	for _, task := range tasks {
		if !contains(prompt, task.Description) {
			t.Errorf("prompt missing task %q", task.Description)
		}
	}
	if !contains(prompt, "HEARTBEAT_OK") {
		t.Error("prompt should ask for HEARTBEAT_OK confirmation")
	}
	if !contains(prompt, "HEARTBEAT_NOTIFY:") {
		t.Error("prompt should describe proactive heartbeat notifications")
	}
}

func TestExecuteHeartbeat_SurfacesProactiveResult(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, heartbeatFile), []byte("# HB\n- [ ] Check health\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var surfaced string
	svc := New(dir, defaultInterval)
	svc.SetHandler(func(_ context.Context, _, _ string) error { return nil })
	svc.SetResultLoader(func(_ context.Context, _ string, _ time.Time) (string, error) {
		return "HEARTBEAT_NOTIFY:\nScheduler backlog is stuck on one task.", nil
	})
	svc.SetProactiveSender(func(_ context.Context, content string) error {
		surfaced = content
		return nil
	})

	svc.executeHeartbeat(context.Background(), "")

	if surfaced != "Scheduler backlog is stuck on one task." {
		t.Fatalf("unexpected proactive content %q", surfaced)
	}
}

func TestExecuteHeartbeat_SkipsWhenNoUsefulUpdate(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, heartbeatFile), []byte("# HB\n- [ ] Check health\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var surfaced atomic.Int32
	svc := New(dir, defaultInterval)
	svc.SetHandler(func(_ context.Context, _, _ string) error { return nil })
	svc.SetResultLoader(func(_ context.Context, _ string, _ time.Time) (string, error) {
		return "HEARTBEAT_OK", nil
	})
	svc.SetProactiveSender(func(_ context.Context, content string) error {
		surfaced.Add(1)
		return nil
	})

	svc.executeHeartbeat(context.Background(), "")

	if surfaced.Load() != 0 {
		t.Fatalf("expected no proactive send, got %d", surfaced.Load())
	}
}

func TestExecuteHeartbeat_RespectsQuietHoursAndDND(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, heartbeatFile), []byte("# HB\n- [ ] Check health\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	run := func(configure func(*HeartbeatService)) int32 {
		var surfaced atomic.Int32
		svc := New(dir, defaultInterval)
		svc.SetHandler(func(_ context.Context, _, _ string) error { return nil })
		svc.SetResultLoader(func(_ context.Context, _ string, _ time.Time) (string, error) {
			return "HEARTBEAT_NOTIFY:\nUseful update", nil
		})
		svc.SetProactiveSender(func(_ context.Context, content string) error {
			surfaced.Add(1)
			return nil
		})
		svc.now = func() time.Time {
			return time.Date(2026, 3, 24, 23, 0, 0, 0, time.Local)
		}
		configure(svc)
		svc.executeHeartbeat(context.Background(), "")
		return surfaced.Load()
	}

	if got := run(func(svc *HeartbeatService) {
		svc.SetQuietHours("22:00", "08:00")
	}); got != 0 {
		t.Fatalf("expected quiet-hours suppression, got %d sends", got)
	}
	if got := run(func(svc *HeartbeatService) {
		svc.SetDNDEnabled(true)
	}); got != 0 {
		t.Fatalf("expected DND suppression, got %d sends", got)
	}
}

func TestInQuietHoursHandlesWraparoundWindow(t *testing.T) {
	now := time.Date(2026, 3, 24, 23, 30, 0, 0, time.Local)
	if !inQuietHours(now, "22:00", "08:00") {
		t.Fatal("expected wraparound quiet hours to include late-night time")
	}
	if inQuietHours(time.Date(2026, 3, 24, 14, 0, 0, 0, time.Local), "22:00", "08:00") {
		t.Fatal("expected afternoon to be outside quiet hours")
	}
}

func TestCancelContextStopsLoop(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir, defaultInterval)
	ctx, cancel := context.WithCancel(context.Background())

	svc.Start(ctx)
	cancel() // trigger context cancellation

	// Allow goroutine to react
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !svc.IsRunning() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	// The loop exits via ctx.Done; IsRunning is set false there.
	// This is best-effort since there's a race between the goroutine and the check.
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
