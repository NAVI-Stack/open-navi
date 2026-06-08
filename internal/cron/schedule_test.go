package cron

import (
	"testing"
	"time"
)

func TestComputeJobNextInstant(t *testing.T) {
	t.Parallel()
	locEU, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Fatalf("timezone: %v", err)
	}
	t.Run("cron_kind_with_tz_daily_boundary", func(t *testing.T) {
		job := Job{
			Schedule: Schedule{Kind: ScheduleCron, CronExpr: "@daily", TZ: "Europe/Paris"},
		}
		base := time.Date(2026, time.March, 10, 4, 0, 0, 0, time.UTC)
		next, ok := ComputeJobNextInstant(job, base)
		if !ok {
			t.Fatal("expected next")
		}
		wantWall := time.Date(2026, time.March, 11, 0, 0, 0, 0, locEU).UTC()
		if !next.Equal(wantWall) {
			t.Fatalf("got %v want %v", next, wantWall)
		}
	})
	t.Run("every_respects_anchor_in_future", func(t *testing.T) {
		anchor := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC).UnixMilli()
		job := Job{Schedule: Schedule{Kind: ScheduleEvery, EveryMS: 3600_000, AnchorMS: anchor}}
		from := time.Date(2029, 6, 1, 0, 0, 0, 0, time.UTC)
		next, ok := ComputeJobNextInstant(job, from)
		if !ok {
			t.Fatal("expected next")
		}
		want := time.UnixMilli(anchor).UTC()
		if !next.Equal(want) {
			t.Fatalf("got %v want %v", next, want)
		}
	})
	t.Run("at_future_instant", func(t *testing.T) {
		at := time.Date(2035, time.July, 4, 9, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
		job := Job{Schedule: Schedule{Kind: ScheduleAt, AtRFC: at}}
		from := time.Date(2035, time.July, 1, 0, 0, 0, 0, time.UTC)
		next, ok := ComputeJobNextInstant(job, from)
		if !ok || !next.After(from.UTC()) {
			t.Fatalf("unexpected next=%v ok=%v", next, ok)
		}
	})
}
