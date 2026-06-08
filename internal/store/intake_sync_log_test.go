package store

import (
	"context"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

func TestIntakeSyncLog_AppendAndList(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()

	for i := 0; i < 3; i++ {
		end := now.Add(time.Duration(i) * time.Minute)
		_, err := AppendIntakeSyncLog(ctx, db, schema.IntakeSyncLogEntry{
			ConnectorID:        "telegram:acct-1",
			JobMode:            schema.JobModeDelta,
			StartedAt:          now,
			EndedAt:            &end,
			RecordsAdmitted:    10 + i,
			RecordsDeduped:     i,
			RecordsDistilled:   5,
			RecordsSynthesized: 2,
			TerminalStatus:     schema.SyncStatusCompleted,
		})
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	// A different connector.
	if _, err := AppendIntakeSyncLog(ctx, db, schema.IntakeSyncLogEntry{
		ConnectorID: "slack:team-1", JobMode: schema.JobModeBackfill,
		TerminalStatus: schema.SyncStatusBudgetExceeded,
	}); err != nil {
		t.Fatalf("append slack: %v", err)
	}

	tg, err := ListIntakeSyncLog(ctx, db, "telegram:acct-1", 50)
	if err != nil {
		t.Fatalf("list telegram: %v", err)
	}
	if len(tg) != 3 {
		t.Fatalf("want 3 telegram rows, got %d", len(tg))
	}
	if tg[0].EndedAt == nil {
		t.Error("expected ended_at to round-trip")
	}

	all, err := ListIntakeSyncLog(ctx, db, "", 50)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("want 4 rows total, got %d", len(all))
	}

	last, ok, err := LatestIntakeSyncLog(ctx, db, "telegram:acct-1")
	if err != nil || !ok {
		t.Fatalf("latest: ok=%v err=%v", ok, err)
	}
	if last.RecordsAdmitted == 0 {
		t.Error("expected populated latest row")
	}
}

func TestVaultSyncLog_AppendAndList(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()

	if _, err := AppendVaultSyncLog(ctx, db, schema.VaultSyncLogEntry{
		FilePath:          "people/alex.md",
		DiffSummary:       "added phone number",
		MutationsProposed: 1,
		ProposalsRaised:   1,
		TerminalStatus:    schema.SyncStatusCompleted,
	}); err != nil {
		t.Fatalf("append: %v", err)
	}

	byPath, err := ListVaultSyncLog(ctx, db, "people/alex.md", 10)
	if err != nil || len(byPath) != 1 {
		t.Fatalf("by path: len=%d err=%v", len(byPath), err)
	}
	if byPath[0].DiffSummary != "added phone number" {
		t.Errorf("diff summary: %q", byPath[0].DiffSummary)
	}

	all, err := ListVaultSyncLog(ctx, db, "", 10)
	if err != nil || len(all) != 1 {
		t.Fatalf("all: len=%d err=%v", len(all), err)
	}
}
