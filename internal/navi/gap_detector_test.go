package navi

import (
	"context"
	"testing"

	"github.com/ceoai/navi/internal/store"
)

func setupGapDetectorDB(t *testing.T) (*GapDetector, *context.Context) {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	ctx := context.Background()
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewGapDetector(db), &ctx
}

func TestRecordSignalA_Detects_Inability(t *testing.T) {
	det, ctx := setupGapDetectorDB(t)

	_, err := det.RecordSignalA(*ctx, "sess-1", "I'm sorry, I don't have the capability to send emails right now.")
	if err != nil {
		t.Fatalf("RecordSignalA: %v", err)
	}

	gaps, err := store.ListOpenGaps(*ctx, det.db, 10)
	if err != nil {
		t.Fatalf("ListOpenGaps: %v", err)
	}
	if len(gaps) != 1 {
		t.Fatalf("expected 1 gap, got %d", len(gaps))
	}
	if gaps[0].Type != store.GapTypeMissingSkill {
		t.Errorf("expected type A, got %s", gaps[0].Type)
	}
	if gaps[0].Status != store.GapStatusClassified {
		t.Errorf("expected status classified, got %s", gaps[0].Status)
	}
	if gaps[0].ClassificationResult.ExpansionPath != "skill" {
		t.Errorf("expected expansion_path 'skill', got %q", gaps[0].ClassificationResult.ExpansionPath)
	}
}

func TestRecordSignalA_Ignores_Normal_Response(t *testing.T) {
	det, ctx := setupGapDetectorDB(t)

	_, err := det.RecordSignalA(*ctx, "sess-1", "Here is the weather forecast for today: sunny, 72°F.")
	if err != nil {
		t.Fatalf("RecordSignalA: %v", err)
	}

	gaps, err := store.ListOpenGaps(*ctx, det.db, 10)
	if err != nil {
		t.Fatalf("ListOpenGaps: %v", err)
	}
	if len(gaps) != 0 {
		t.Fatalf("expected 0 gaps for normal response, got %d", len(gaps))
	}
}
