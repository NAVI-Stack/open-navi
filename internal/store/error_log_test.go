package store

import (
	"context"
	"testing"
	"time"
)

func TestSaveAndListErrorRecords(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()

	err := SaveErrorRecord(ctx, db, ErrorRecord{
		Component:    "runtime",
		ChatID:       "sess-1",
		RunID:        "run-1",
		ErrorType:    "llm_timeout",
		ErrorMessage: "deadline exceeded",
		ContextJSON:  `{"model":"chat"}`,
	})
	if err != nil {
		t.Fatalf("SaveErrorRecord: %v", err)
	}

	items, err := ListErrorRecords(ctx, db, ListErrorsFilter{ChatID: "sess-1", Limit: 10})
	if err != nil {
		t.Fatalf("ListErrorRecords: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 record, got %d", len(items))
	}
	if items[0].ErrorType != "llm_timeout" {
		t.Fatalf("expected llm_timeout, got %q", items[0].ErrorType)
	}
}

func TestErrorSummary(t *testing.T) {
	db := InitTestDB(t)
	ctx := context.Background()
	for _, item := range []ErrorRecord{
		{Component: "runtime", ErrorType: "llm_timeout", ErrorMessage: "one", Timestamp: time.Now().UTC()},
		{Component: "runtime", ErrorType: "llm_timeout", ErrorMessage: "two", Timestamp: time.Now().UTC()},
		{Component: "connector", ErrorType: "connector_send_failed", ErrorMessage: "three", Timestamp: time.Now().UTC()},
	} {
		if err := SaveErrorRecord(ctx, db, item); err != nil {
			t.Fatalf("SaveErrorRecord: %v", err)
		}
	}

	rows, err := ErrorSummary(ctx, db, time.Now().UTC().Add(-time.Hour))
	if err != nil {
		t.Fatalf("ErrorSummary: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("expected multiple summary rows, got %d", len(rows))
	}
	if rows[0].Component != "runtime" || rows[0].ErrorType != "llm_timeout" || rows[0].Count != 2 {
		t.Fatalf("unexpected first summary row: %+v", rows[0])
	}
}
