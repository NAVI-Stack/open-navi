package cron

import (
	"context"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/config"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

type recordingDirectiveWriter struct {
	directives []schema.Directive
	messages   []schema.DirectiveMessage
}

func (w *recordingDirectiveWriter) SaveDirective(_ context.Context, d schema.Directive) error {
	w.directives = append(w.directives, d)
	return nil
}

func (w *recordingDirectiveWriter) AppendMessage(_ context.Context, msg schema.DirectiveMessage) error {
	w.messages = append(w.messages, msg)
	return nil
}

func cronConfigForTest() config.CronConfig {
	var c config.CronConfig
	c.Enabled = true
	c.MaxConcurrentRuns = 1
	c.DefaultTimeoutMs = int64(time.Second.Milliseconds())
	c.Retry.MaxAttempts = 3
	c.Retry.BackoffMs = []int64{1000}
	return c
}

func TestServiceUpdateReschedulesJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := store.InitTestDB(t)
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	s := NewService(Deps{
		DB:              db,
		DirectiveWriter: &recordingDirectiveWriter{},
		Cron:            cronConfigForTest(),
		Now:             func() time.Time { return now },
	})

	created, err := s.Create(ctx, Job{
		ID:            "job-update",
		Name:          "old",
		PayloadText:   "old prompt",
		SessionTarget: SessionMain,
		WakeMode:      WakeNextHeartbeat,
		PayloadKind:   PayloadSystemEvent,
		Schedule: Schedule{
			Kind:    ScheduleEvery,
			EveryMS: int64(time.Hour.Milliseconds()),
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	created.Name = "new"
	created.PayloadText = "new prompt"
	created.DeleteAfterRun = true
	created.FailureAlert = &FailureAlert{After: 2, CooldownMS: 5000}
	created.Schedule = Schedule{
		Kind:    ScheduleEvery,
		EveryMS: int64((2 * time.Hour).Milliseconds()),
	}

	updated, err := s.Update(ctx, created)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "new" || updated.PayloadText != "new prompt" {
		t.Fatalf("updated job did not persist fields: %+v", updated)
	}
	if updated.Schedule.EveryMS != int64((2 * time.Hour).Milliseconds()) {
		t.Fatalf("schedule not updated: %+v", updated.Schedule)
	}
	wantNext, ok := ComputeJobNextInstant(updated, now.UTC())
	if !ok {
		t.Fatal("expected updated schedule to compute a next run")
	}
	if updated.State.NextRunAtMS != wantNext.UnixMilli() {
		t.Fatalf("next run not recomputed: got %d want %d", updated.State.NextRunAtMS, wantNext.UnixMilli())
	}
	if updated.FailureAlert == nil || updated.FailureAlert.After != 2 || updated.FailureAlert.CooldownMS != 5000 {
		t.Fatalf("failure alert not persisted: %+v", updated.FailureAlert)
	}
}

func TestServiceRunNowDispatchesJob(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := store.InitTestDB(t)
	writer := &recordingDirectiveWriter{}
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	s := NewService(Deps{
		DB:              db,
		DirectiveWriter: writer,
		Cron:            cronConfigForTest(),
		Now:             func() time.Time { return now },
	})
	if _, err := s.Create(ctx, Job{
		ID:            "job-run-now",
		Name:          "run now",
		PayloadText:   "do the thing",
		SessionTarget: SessionMain,
		WakeMode:      WakeNextHeartbeat,
		PayloadKind:   PayloadSystemEvent,
		Schedule: Schedule{
			Kind:    ScheduleEvery,
			EveryMS: int64(time.Hour.Milliseconds()),
		},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.RunNow(ctx, "job-run-now"); err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	if len(writer.directives) != 1 || len(writer.messages) != 1 {
		t.Fatalf("expected one directive/message, got %d/%d", len(writer.directives), len(writer.messages))
	}
	if writer.messages[0].Content != "do the thing" {
		t.Fatalf("unexpected message content %q", writer.messages[0].Content)
	}
	got, ok, err := s.Get(ctx, "job-run-now")
	if err != nil || !ok {
		t.Fatalf("Get after RunNow: ok=%v err=%v", ok, err)
	}
	if got.State.LastRunStatus != RunOK || got.State.RunningAtMS != 0 {
		t.Fatalf("unexpected post-run state: %+v", got.State)
	}
}

func TestFailureAlertCallbackFiresAtThreshold(t *testing.T) {
	t.Parallel()
	var c config.CronConfig
	c.Retry.MaxAttempts = 3
	c.Retry.BackoffMs = []int64{1000}
	c.FailureAlert.Enabled = true
	c.FailureAlert.After = 2
	c.FailureAlert.CooldownMs = int64(time.Hour.Milliseconds())
	alerts := 0
	s := NewService(Deps{
		Cron: c,
		Now:  func() time.Time { return time.Unix(100, 0).UTC() },
		OnFailureAlert: func(context.Context, Job, string) {
			alerts++
		},
	})
	j := &Job{
		ID:      "job-alert",
		Name:    "alert me",
		Enabled: true,
		Schedule: Schedule{
			Kind:     ScheduleCron,
			CronExpr: "@daily",
			TZ:       "UTC",
		},
		State: JobState{ConsecutiveErrors: 1},
	}
	s.applyOutcome(j, time.Unix(100, 0).UTC(), time.Unix(101, 0).UTC(), RunResult{Status: RunError, Error: "boom"})
	if alerts != 1 {
		t.Fatalf("expected alert callback once, got %d", alerts)
	}
	if j.State.LastFailureAlertMS == 0 {
		t.Fatal("expected last failure alert timestamp to be set")
	}
}

func TestAgentTurnPayloadIsExplicitlyUnsupported(t *testing.T) {
	t.Parallel()
	res := executeMainDispatch(context.Background(), &recordingDirectiveWriter{}, nil, Job{
		Name:          "agent turn",
		PayloadKind:   PayloadAgentTurn,
		PayloadText:   "hello",
		SessionTarget: SessionMain,
	})
	if res.Status != RunSkipped || res.Error == "" {
		t.Fatalf("expected explicit skipped result for agentTurn, got %+v", res)
	}
}

// recordingChatAppender lets us assert assistant-message delivery without
// pulling in the full navi session store.
type recordingChatAppender struct {
	calls []struct {
		chatID  string
		content string
	}
	failWith error
}

func (a *recordingChatAppender) AppendAssistantMessage(_ context.Context, chatID, content string) error {
	a.calls = append(a.calls, struct {
		chatID  string
		content string
	}{chatID, content})
	return a.failWith
}

func TestSessionTargetedDispatchAppendsAssistantMessage(t *testing.T) {
	t.Parallel()
	sa := &recordingChatAppender{}
	res := executeDispatch(context.Background(), &recordingDirectiveWriter{}, sa, nil, Job{
		Name:          "session-targeted",
		SessionTarget: SessionTarget(SessionTargetPrefix + "sess-123"),
		PayloadKind:   PayloadAssistantMessage,
		PayloadText:   "hello later",
	})
	if res.Status != RunOK {
		t.Fatalf("expected RunOK, got %+v", res)
	}
	if len(sa.calls) != 1 {
		t.Fatalf("expected 1 appender call, got %d", len(sa.calls))
	}
	if sa.calls[0].chatID != "sess-123" || sa.calls[0].content != "hello later" {
		t.Fatalf("unexpected appender args: %+v", sa.calls[0])
	}
}

func TestSessionTargetedDispatchWithoutAppenderReportsError(t *testing.T) {
	t.Parallel()
	res := executeDispatch(context.Background(), &recordingDirectiveWriter{}, nil, nil, Job{
		Name:          "no-appender",
		SessionTarget: SessionTarget(SessionTargetPrefix + "sess-1"),
		PayloadKind:   PayloadAssistantMessage,
		PayloadText:   "x",
	})
	if res.Status != RunError {
		t.Fatalf("expected RunError when ChatAppender is nil, got %+v", res)
	}
}

func TestSessionTargetedDispatchRejectsWrongPayloadKind(t *testing.T) {
	t.Parallel()
	sa := &recordingChatAppender{}
	res := executeDispatch(context.Background(), &recordingDirectiveWriter{}, sa, nil, Job{
		Name:          "wrong-kind",
		SessionTarget: SessionTarget(SessionTargetPrefix + "sess-1"),
		PayloadKind:   PayloadSystemEvent,
		PayloadText:   "x",
	})
	if res.Status != RunError {
		t.Fatalf("expected RunError for non-assistantMessage payload, got %+v", res)
	}
	if len(sa.calls) != 0 {
		t.Fatalf("appender should not be called for wrong payload kind")
	}
}

// TestOutcomeUpdateDoesNotClobberConcurrentEdit verifies the audit-flagged
// race fix: the post-execution outcome write must not overwrite a name /
// payload / schedule edit committed between the runOneJob refresh and its
// final persist call.
func TestOutcomeUpdateDoesNotClobberConcurrentEdit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := store.InitTestDB(t)
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	s := NewService(Deps{
		DB:              db,
		DirectiveWriter: &recordingDirectiveWriter{},
		Cron:            cronConfigForTest(),
		Now:             func() time.Time { return now },
	})
	created, err := s.Create(ctx, Job{
		ID:            "race-job",
		Name:          "original-name",
		PayloadText:   "original-payload",
		SessionTarget: SessionMain,
		WakeMode:      WakeNextHeartbeat,
		PayloadKind:   PayloadSystemEvent,
		Schedule: Schedule{
			Kind:    ScheduleEvery,
			EveryMS: int64(time.Hour.Milliseconds()),
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Simulate runOneJob's refresh: take a snapshot of the job pre-mutation.
	snapshot, ok, err := s.Get(ctx, created.ID)
	if err != nil || !ok {
		t.Fatalf("Get snapshot: ok=%v err=%v", ok, err)
	}

	// Concurrent edit: rename the job and rewrite its payload via the public
	// Update path while our "in-flight runOneJob" still holds the stale snapshot.
	snapshot.Name = "renamed-by-user"
	snapshot.PayloadText = "edited-by-user"
	if _, err := s.Update(ctx, snapshot); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Now have the runOneJob path apply its outcome (start/end times don't
	// matter; we only care that the targeted UPDATE leaves name/payload alone).
	snapshot.State.RunningAtMS = now.UnixMilli()
	s.applyOutcome(&snapshot, now, now.Add(time.Second), RunResult{Status: RunOK})
	outcome := store.CronJobOutcome{
		Enabled:            snapshot.Enabled,
		NextRunAtMS:        nullableInt64(snapshot.State.NextRunAtMS),
		RunningAtMS:        nullableInt64(snapshot.State.RunningAtMS),
		LastRunAtMS:        nullableInt64(snapshot.State.LastRunAtMS),
		LastRunStatus:      nullableString(string(snapshot.State.LastRunStatus)),
		LastError:          nullableString(snapshot.State.LastError),
		LastDurationMS:     nullableInt64(snapshot.State.LastDurationMS),
		ConsecutiveErrors:  snapshot.State.ConsecutiveErrors,
		ScheduleErrorCount: snapshot.State.ScheduleErrorCount,
		LastFailureAlertMS: nullableInt64(snapshot.State.LastFailureAlertMS),
		UpdatedAtMS:        now.UnixMilli(),
	}
	if err := store.UpdateCronJobOutcome(ctx, db, snapshot.ID, outcome); err != nil {
		t.Fatalf("UpdateCronJobOutcome: %v", err)
	}

	// The concurrent edit must survive the outcome write.
	final, ok, err := s.Get(ctx, created.ID)
	if err != nil || !ok {
		t.Fatalf("Get final: ok=%v err=%v", ok, err)
	}
	if final.Name != "renamed-by-user" {
		t.Fatalf("concurrent rename was clobbered by outcome write: name=%q", final.Name)
	}
	if final.PayloadText != "edited-by-user" {
		t.Fatalf("concurrent payload edit was clobbered: payload=%q", final.PayloadText)
	}
	if final.State.LastRunStatus != RunOK {
		t.Fatalf("outcome status did not persist: %+v", final.State)
	}
	if final.State.RunningAtMS != 0 {
		t.Fatalf("running marker should have been cleared by applyOutcome: %d", final.State.RunningAtMS)
	}
}
