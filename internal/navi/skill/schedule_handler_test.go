package skill

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/config"
	"github.com/open-navi/navi/internal/cron"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

type scheduleTestWriter struct {
	messages []schema.DirectiveMessage
}

func (w *scheduleTestWriter) SaveDirective(context.Context, schema.Directive) error {
	return nil
}

func (w *scheduleTestWriter) AppendMessage(_ context.Context, msg schema.DirectiveMessage) error {
	w.messages = append(w.messages, msg)
	return nil
}

func scheduleIface(name string) *Interface {
	return &Interface{Name: name, Transport: TransportSpec{Type: "internal"}}
}

func scheduleEntry() *SkillEntry {
	return &SkillEntry{Spec: &OSS27Spec{SkillID: "core-scheduler"}}
}

func testCronConfig() config.CronConfig {
	var c config.CronConfig
	c.Enabled = true
	c.MaxConcurrentRuns = 1
	c.DefaultTimeoutMs = int64(time.Second.Milliseconds())
	c.Retry.MaxAttempts = 3
	c.Retry.BackoffMs = []int64{1000}
	return c
}

func TestScheduleHandlerUpdateAndRunNow(t *testing.T) {
	ctx := context.Background()
	db := store.InitTestDB(t)
	writer := &scheduleTestWriter{}
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	svc := cron.NewService(cron.Deps{
		DB:              db,
		DirectiveWriter: writer,
		Cron:            testCronConfig(),
		Now:             func() time.Time { return now },
	})
	RegisterScheduleHandler(db, svc)

	raw, err := Execute(ctx, scheduleEntry(), scheduleIface("schedule_task"), map[string]any{
		"name":                      "weekly report",
		"prompt":                    "run report",
		"every_ms":                  float64(time.Hour.Milliseconds()),
		"delete_after_run":          true,
		"failure_alert_after":       float64(2),
		"failure_alert_cooldown_ms": float64(5000),
	})
	if err != nil {
		t.Fatalf("schedule_task: %v", err)
	}
	created := decodeExecutionResult(t, raw).Payload.(map[string]any)
	id := created["id"].(string)
	if created["delete_after_run"] != true {
		t.Fatalf("delete_after_run missing from create response: %v", created)
	}

	raw, err = Execute(ctx, scheduleEntry(), scheduleIface("update_task"), map[string]any{
		"id":       id,
		"name":     "twice daily report",
		"prompt":   "run updated report",
		"every_ms": float64((12 * time.Hour).Milliseconds()),
		"enabled":  true,
	})
	if err != nil {
		t.Fatalf("update_task: %v", err)
	}
	updated := decodeExecutionResult(t, raw).Payload.(map[string]any)
	if updated["schedule_kind"] != "every" || updated["enabled"] != true {
		t.Fatalf("unexpected update response: %v", updated)
	}

	raw, err = Execute(ctx, scheduleEntry(), scheduleIface("run_task_now"), map[string]any{"id": id})
	if err != nil {
		t.Fatalf("run_task_now: %v", err)
	}
	result := decodeExecutionResult(t, raw)
	if result.Status != "success" {
		t.Fatalf("run_task_now status: %s", result.Status)
	}
	if len(writer.messages) != 1 || writer.messages[0].Content != "run updated report" {
		t.Fatalf("run_task_now did not dispatch updated prompt: %+v", writer.messages)
	}
}

func newTestScheduleService(t *testing.T) (context.Context, *cron.Service) {
	t.Helper()
	ctx := context.Background()
	db := store.InitTestDB(t)
	writer := &scheduleTestWriter{}
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	svc := cron.NewService(cron.Deps{
		DB:              db,
		DirectiveWriter: writer,
		Cron:            testCronConfig(),
		Now:             func() time.Time { return now },
	})
	RegisterScheduleHandler(db, svc)
	return ctx, svc
}

// TestScheduleTask_KindAt_Persists locks in the kind=at path added by the
// cron-service overhaul. The model is expected to use this for one-shot
// reminders beyond send_reply's 1-hour reach.
func TestScheduleTask_KindAt_Persists(t *testing.T) {
	ctx, svc := newTestScheduleService(t)

	target := time.Date(2026, 5, 16, 13, 30, 0, 0, time.UTC) // 90 min after the fixed Now()
	raw, err := Execute(ctx, scheduleEntry(), scheduleIface("schedule_task"), map[string]any{
		"name":   "remind-tea",
		"prompt": "tea is ready",
		"kind":   "at",
		"at":     target.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("schedule_task kind=at: %v", err)
	}
	created := decodeExecutionResult(t, raw).Payload.(map[string]any)
	if got := created["schedule_kind"]; got != "at" {
		t.Fatalf("expected schedule_kind=at, got %v", got)
	}
	if created["next_run_at"] == nil || created["next_run_at"] == "" {
		t.Fatalf("expected next_run_at to be populated, got %v", created["next_run_at"])
	}
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("expected job id in response, got %v", created)
	}

	job, ok, err := svc.Get(ctx, id)
	if err != nil || !ok {
		t.Fatalf("svc.Get(%q) found=%v err=%v", id, ok, err)
	}
	if job.Schedule.Kind != cron.ScheduleAt {
		t.Fatalf("persisted kind: want %q got %q", cron.ScheduleAt, job.Schedule.Kind)
	}
	if job.Schedule.AtRFC == "" {
		t.Fatalf("persisted at RFC missing: %+v", job.Schedule)
	}
}

// TestScheduleTask_KindEvery_Persists locks in the every_ms interval path.
// This is what the model should use for recurring "every N minutes" requests
// once it learns of the core-scheduler tool via the chat-behavior prompt.
func TestScheduleTask_KindEvery_Persists(t *testing.T) {
	ctx, svc := newTestScheduleService(t)

	const tenMinMS = int64(10 * 60 * 1000)
	raw, err := Execute(ctx, scheduleEntry(), scheduleIface("schedule_task"), map[string]any{
		"name":     "ping-every-10m",
		"prompt":   "ping",
		"every_ms": float64(tenMinMS),
	})
	if err != nil {
		t.Fatalf("schedule_task kind=every: %v", err)
	}
	created := decodeExecutionResult(t, raw).Payload.(map[string]any)
	if got := created["schedule_kind"]; got != "every" {
		t.Fatalf("expected schedule_kind=every, got %v", got)
	}
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("expected job id in response, got %v", created)
	}

	job, ok, err := svc.Get(ctx, id)
	if err != nil || !ok {
		t.Fatalf("svc.Get(%q) found=%v err=%v", id, ok, err)
	}
	if job.Schedule.Kind != cron.ScheduleEvery {
		t.Fatalf("persisted kind: want %q got %q", cron.ScheduleEvery, job.Schedule.Kind)
	}
	if job.Schedule.EveryMS != tenMinMS {
		t.Fatalf("persisted every_ms: want %d got %d", tenMinMS, job.Schedule.EveryMS)
	}
}

// TestScheduleTask_ConflictingKinds_Rejected ensures the handler still rejects
// ambiguous input (every_ms supplied alongside kind=at). The model needs a
// clear error so it can re-form the request rather than persist gibberish.
func TestScheduleTask_ConflictingKinds_Rejected(t *testing.T) {
	ctx, _ := newTestScheduleService(t)

	raw, err := Execute(ctx, scheduleEntry(), scheduleIface("schedule_task"), map[string]any{
		"name":     "ambiguous",
		"prompt":   "no-op",
		"kind":     "at",
		"every_ms": float64(60000),
	})
	if err != nil {
		t.Fatalf("Execute returned Go error %v; expected packaged result with status=error", err)
	}
	result := decodeExecutionResult(t, raw)
	if result.Status != "error" {
		t.Fatalf("expected status=error for conflicting kinds, got %q (payload=%v)", result.Status, result.Payload)
	}
	if result.Error == nil || !strings.Contains(result.Error.Message, "every_ms conflicts with schedule_kind") {
		t.Fatalf("expected conflict-message error, got %+v", result.Error)
	}
}
