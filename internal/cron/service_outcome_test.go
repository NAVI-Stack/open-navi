package cron

import (
	"log/slog"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/config"
)

func TestApplyOutcomeResetsErrorsOnSuccess(t *testing.T) {
	t.Parallel()
	s := NewService(Deps{
		Cron: func() config.CronConfig {
			var c config.CronConfig
			c.Retry.MaxAttempts = 3
			c.Retry.BackoffMs = []int64{1000}
			return c
		}(),
		Log: slog.Default(),
		Now: time.Now,
	})
	j := &Job{
		Enabled: true,
		Schedule: Schedule{
			Kind:     ScheduleCron,
			CronExpr: "@daily",
			TZ:       "UTC",
		},
		State: JobState{
			NextRunAtMS:        time.Now().Add(time.Hour).UnixMilli(),
			ConsecutiveErrors:  2,
			LastFailureAlertMS: time.Now().UnixMilli(),
			ScheduleErrorCount: 0,
		},
	}
	start := time.UnixMilli(1000).UTC()
	end := start.Add(10 * time.Millisecond)
	s.applyOutcome(j, start, end, RunResult{Status: RunOK})

	if j.State.ConsecutiveErrors != 0 || j.State.LastFailureAlertMS != 0 {
		t.Fatalf("expected consecutive + alert cleared; got %+v", j.State)
	}
	if j.State.NextRunAtMS == 0 {
		t.Fatal("expected next_run advanced")
	}
}
