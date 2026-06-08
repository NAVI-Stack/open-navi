package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

func TestAppendAndByCorrelationID(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	ev := schema.NewEvent(schema.CmdDirectiveMessage, schema.EventKindCommand, "corr-1", schema.AgentNavi, schema.DirectiveMessagePayload{
		DirectiveID: "d1",
		MessageID:   "m1",
		OwnerID:     "owner",
	})
	if err := AppendEvent(ctx, db, ev); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	list, err := EventsByCorrelationID(ctx, db, "corr-1")
	if err != nil {
		t.Fatalf("EventsByCorrelationID: %v", err)
	}
	if len(list) != 1 || list[0].ID != ev.ID || list[0].CorrelationID != "corr-1" {
		t.Fatalf("got %+v", list)
	}
}

func TestEventsByCausalParent(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	parent := schema.NewEvent(schema.FactDirectiveReplied, schema.EventKindFact, "corr-2", schema.AgentNavi, schema.DirectiveRepliedPayload{
		DirectiveID: "d2",
		MessageID:   "m2",
	})
	_ = AppendEvent(ctx, db, parent)
	child := schema.NewEvent(schema.FactDirectiveReplied, schema.EventKindFact, "corr-2", schema.AgentNavi, schema.DirectiveRepliedPayload{
		DirectiveID: "d2",
		MessageID:   "m3",
	})
	child.CausalParent = parent.ID
	_ = AppendEvent(ctx, db, child)
	list, err := EventsByCausalParent(ctx, db, parent.ID)
	if err != nil {
		t.Fatalf("EventsByCausalParent: %v", err)
	}
	if len(list) != 1 || list[0].ID != child.ID {
		t.Fatalf("got %+v", list)
	}
}

func TestLatestEventByTypeAndCorrelationID(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	first := schema.NewEvent(schema.FactNaviExperienceSnapshot, schema.EventKindFact, "sess-1", schema.AgentNavi, schema.NaviExperienceSnapshotPayload{
		SnapshotID:         "snap-1",
		ChatID:             "sess-1",
		Trigger:            "session_start",
		SourceStateID:      "eps-1",
		EffectiveStateJSON: `{"state_id":"eps-1"}`,
	})
	second := schema.NewEvent(schema.FactNaviExperienceSnapshot, schema.EventKindFact, "sess-1", schema.AgentNavi, schema.NaviExperienceSnapshotPayload{
		SnapshotID:         "snap-2",
		ChatID:             "sess-1",
		Trigger:            "material_delta",
		SourceStateID:      "eps-2",
		EffectiveStateJSON: `{"state_id":"eps-2"}`,
	})
	if err := AppendEvent(ctx, db, first); err != nil {
		t.Fatalf("AppendEvent(first): %v", err)
	}
	if err := AppendEvent(ctx, db, second); err != nil {
		t.Fatalf("AppendEvent(second): %v", err)
	}

	got, err := LatestEventByTypeAndCorrelationID(ctx, db, schema.FactNaviExperienceSnapshot, "sess-1")
	if err != nil {
		t.Fatalf("LatestEventByTypeAndCorrelationID: %v", err)
	}
	if got == nil || got.ID != second.ID {
		t.Fatalf("expected latest snapshot event %s, got %+v", second.ID, got)
	}
}

func TestLatestEventsByType(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		ev := schema.NewEvent(schema.FactNaviExperienceSnapshot, schema.EventKindFact, "sess-1", schema.AgentNavi, schema.NaviExperienceSnapshotPayload{
			SnapshotID:    fmt.Sprintf("snap-%d", i),
			Trigger:       "test",
			SourceStateID: "eps-1",
		})
		_ = AppendEvent(ctx, db, ev)
		// Small sleep to ensure session-level ordering if needed
		time.Sleep(1 * time.Millisecond)
	}

	got, err := LatestEventsByType(ctx, db, schema.FactNaviExperienceSnapshot, 3)
	if err != nil {
		t.Fatalf("LatestEventsByType: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 events, got %d", len(got))
	}
	// Should be newest first (snap-5, snap-4, snap-3)
	if got[0].Payload.(map[string]any)["snapshot_id"] != "snap-5" {
		t.Fatalf("expected snap-5 first, got %v", got[0].Payload)
	}
}

func TestEventDeduplication(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	ev := schema.NewEvent(schema.CmdDirectiveMessage, schema.EventKindCommand, "corr-1", schema.AgentNavi, schema.DirectiveMessagePayload{
		DirectiveID: "d3",
		MessageID:   "m4",
		OwnerID:     "owner",
	})

	if err := AppendEvent(ctx, db, ev); err != nil {
		t.Fatalf("first append failed: %v", err)
	}

	if err := AppendEvent(ctx, db, ev); err != nil {
		t.Fatalf("second append should be idempotent, got error: %v", err)
	}

	var count int
	_ = db.QueryRow("SELECT count(*) FROM events WHERE id = ?", ev.ID).Scan(&count)
	if count != 1 {
		t.Fatalf("expected exactly 1 event in DB, got %d", count)
	}
}

func TestEventsSince(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		ev := schema.NewEvent(schema.CmdDirectiveMessage, schema.EventKindCommand, "corr-1", schema.AgentNavi, schema.DirectiveMessagePayload{
			DirectiveID: "d4",
			MessageID:   "m5",
			OwnerID:     "owner",
		})
		_ = AppendEvent(ctx, db, ev)
	}

	// Query after seq 2, limit 2
	list, err := EventsSince(ctx, db, 2, 2)
	if err != nil {
		t.Fatalf("EventsSince: %v", err)
	}

	if len(list) != 2 {
		t.Fatalf("expected 2 events, got %d", len(list))
	}
}

func TestDeleteEventsByRetentionPolicyHonorsPerTypeOverrides(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	now := time.Now().UTC()

	events := []schema.Event{
		schema.NewEvent(schema.CmdDirectiveMessage, schema.EventKindCommand, "corr-keep-command", schema.AgentNavi, schema.DirectiveMessagePayload{
			DirectiveID: "d1",
			MessageID:   "m1",
			OwnerID:     "owner",
		}),
		schema.NewEvent(schema.CmdDirectiveMessage, schema.EventKindCommand, "corr-drop-command", schema.AgentNavi, schema.DirectiveMessagePayload{
			DirectiveID: "d2",
			MessageID:   "m2",
			OwnerID:     "owner",
		}),
		schema.NewEvent(schema.FactDirectiveReplied, schema.EventKindFact, "corr-drop-fact", schema.AgentNavi, schema.DirectiveRepliedPayload{
			DirectiveID: "d3",
			MessageID:   "m3",
		}),
		schema.NewEvent(schema.FactRunStarted, schema.EventKindFact, "corr-keep-override", schema.AgentNavi, schema.RunStartedPayload{
			RunID:            "run-1",
			RuntimeSessionID: "runtime-1",
		}),
	}

	events[0].Timestamp = now.AddDate(0, 0, -5)
	events[1].Timestamp = now.AddDate(0, 0, -40)
	events[2].Timestamp = now.AddDate(0, 0, -5)
	events[3].Timestamp = now.AddDate(0, 0, -40)

	for _, ev := range events {
		if err := AppendEvent(ctx, db, ev); err != nil {
			t.Fatalf("AppendEvent(%s): %v", ev.Type, err)
		}
	}

	deleted, err := DeleteEventsByRetentionPolicy(ctx, db, now, 30, map[string]int{
		string(schema.FactDirectiveReplied): 3,
		string(schema.FactRunStarted):       90,
	})
	if err != nil {
		t.Fatalf("DeleteEventsByRetentionPolicy: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("expected 2 deleted rows, got %d", deleted)
	}

	remaining, _, err := ListEvents(ctx, db, 0, nil, 10)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("expected 2 remaining rows, got %d", len(remaining))
	}

	seen := map[schema.EventType]bool{}
	for _, ev := range remaining {
		seen[ev.Type] = true
	}
	if !seen[schema.CmdDirectiveMessage] {
		t.Fatal("expected recent command event to remain")
	}
	if !seen[schema.FactRunStarted] {
		t.Fatal("expected long-retention override event to remain")
	}
	if seen[schema.FactDirectiveReplied] {
		t.Fatal("expected shorter-retention fact event to be pruned")
	}
}
