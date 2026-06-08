package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestCronJobsInsertListEnabledNext(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "cron.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	nowMs := time.Now().UTC().UnixMilli()
	rec := CronJobRecord{
		ID:                 "j1",
		OwnerID:            "o",
		Name:               "n",
		Description:        "",
		Enabled:            true,
		ScheduleKind:       "cron",
		ScheduleExpr:       sql.NullString{String: "@hourly", Valid: true},
		SessionTarget:      "main",
		WakeMode:           "next-heartbeat",
		PayloadKind:        "systemEvent",
		PayloadText:        "hello",
		NextRunAtMS:        sql.NullInt64{Int64: nowMs, Valid: true},
		CreatedAtMS:        nowMs,
		UpdatedAtMS:        nowMs,
		ConsecutiveErrors:  0,
		ScheduleErrorCount: 0,
	}
	if err := InsertCronJob(ctx, db, rec); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := ListEnabledCronJobsWithNext(ctx, db)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 job, got %d", len(got))
	}
	if got[0].ID != "j1" || got[0].ScheduleExpr.String != "@hourly" {
		t.Fatalf("unexpected row %+v", got[0])
	}
}

func TestGetRunnableCronJobsFiltersDueEnabledJobs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "cron.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	nowMs := time.Now().UTC().UnixMilli()
	base := CronJobRecord{
		OwnerID:            "o",
		Name:               "n",
		Description:        "",
		Enabled:            true,
		ScheduleKind:       "cron",
		ScheduleExpr:       sql.NullString{String: "@hourly", Valid: true},
		SessionTarget:      "main",
		WakeMode:           "next-heartbeat",
		PayloadKind:        "systemEvent",
		PayloadText:        "hello",
		CreatedAtMS:        nowMs,
		UpdatedAtMS:        nowMs,
		ConsecutiveErrors:  0,
		ScheduleErrorCount: 0,
	}
	rows := []CronJobRecord{
		func() CronJobRecord {
			r := base
			r.ID = "due-later"
			r.NextRunAtMS = sql.NullInt64{Int64: nowMs - 1000, Valid: true}
			return r
		}(),
		func() CronJobRecord {
			r := base
			r.ID = "due-earlier"
			r.NextRunAtMS = sql.NullInt64{Int64: nowMs - 2000, Valid: true}
			return r
		}(),
		func() CronJobRecord {
			r := base
			r.ID = "future"
			r.NextRunAtMS = sql.NullInt64{Int64: nowMs + 1000, Valid: true}
			return r
		}(),
		func() CronJobRecord {
			r := base
			r.ID = "disabled"
			r.Enabled = false
			r.NextRunAtMS = sql.NullInt64{Int64: nowMs - 3000, Valid: true}
			return r
		}(),
	}
	for _, row := range rows {
		if err := InsertCronJob(ctx, db, row); err != nil {
			t.Fatalf("insert %s: %v", row.ID, err)
		}
	}

	got, err := GetRunnableCronJobs(ctx, db, nowMs)
	if err != nil {
		t.Fatalf("get runnable: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 runnable jobs, got %d: %+v", len(got), got)
	}
	if got[0].ID != "due-earlier" || got[1].ID != "due-later" {
		t.Fatalf("jobs not sorted by next_run_at_ms: %+v", got)
	}
}
