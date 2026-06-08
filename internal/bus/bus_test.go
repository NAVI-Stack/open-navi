package bus

import (
	"context"
	"sync"
	"testing"

	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

func testBusDB(t *testing.T) *MemBus {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	return NewMemBus(db)
}

func TestPublishSubscribeRoundTrip(t *testing.T) {
	bus := testBusDB(t)
	defer bus.Close()
	ctx := context.Background()

	var received schema.Event
	var wg sync.WaitGroup
	wg.Add(1)

	err := bus.Subscribe(ctx, schema.FactDirectiveReplied, "queue", func(_ context.Context, ev schema.Event) {
		received = ev
		wg.Done()
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	ev := schema.NewEvent(schema.FactDirectiveReplied, schema.EventKindFact, "bus-corr-1", schema.AgentNavi, schema.DirectiveRepliedPayload{
		DirectiveID: "d1",
		MessageID:   "m1",
	})
	if err := bus.Publish(ctx, ev); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	wg.Wait()
	if received.ID != ev.ID {
		t.Fatalf("expected event ID %q, got %q", ev.ID, received.ID)
	}
}

func TestPublishValidationRejectsInvalidKind(t *testing.T) {
	bus := testBusDB(t)
	defer bus.Close()
	ctx := context.Background()

	// Command event type with fact kind — must be rejected.
	ev := schema.NewEvent(schema.CmdDirectiveMessage, schema.EventKindFact, "bad-kind", schema.AgentNavi, schema.DirectiveMessagePayload{
		DirectiveID: "d1", MessageID: "m1", OwnerID: "owner",
	})
	if err := bus.Publish(ctx, ev); err == nil {
		t.Fatal("expected error for kind mismatch")
	}
}

func TestPublishValidationRejectsMissingFields(t *testing.T) {
	bus := testBusDB(t)
	defer bus.Close()
	ctx := context.Background()

	ev := schema.Event{} // missing all fields
	if err := bus.Publish(ctx, ev); err == nil {
		t.Fatal("expected error for invalid event")
	}
}

func TestMultipleSubscribers(t *testing.T) {
	bus := testBusDB(t)
	defer bus.Close()
	ctx := context.Background()

	var count int
	var mu sync.Mutex

	handler := func(_ context.Context, ev schema.Event) {
		mu.Lock()
		count++
		mu.Unlock()
	}
	_ = bus.Subscribe(ctx, schema.FactDirectiveReplied, "queue1", handler)
	_ = bus.Subscribe(ctx, schema.FactDirectiveReplied, "queue2", handler)

	ev := schema.NewEvent(schema.FactDirectiveReplied, schema.EventKindFact, "multi-corr", schema.AgentNavi, schema.DirectiveRepliedPayload{
		DirectiveID: "d1", MessageID: "m1",
	})
	if err := bus.Publish(ctx, ev); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	mu.Lock()
	if count != 2 {
		t.Fatalf("expected 2 handler calls, got %d", count)
	}
	mu.Unlock()
}

func TestUnsubscribedTypesNotInvoked(t *testing.T) {
	bus := testBusDB(t)
	defer bus.Close()
	ctx := context.Background()

	called := false
	_ = bus.Subscribe(ctx, schema.FactNaviReplied, "queue", func(_ context.Context, ev schema.Event) {
		called = true
	})

	// Publish a different event type (directive, not navi).
	ev := schema.NewEvent(schema.FactDirectiveReplied, schema.EventKindFact, "other-corr", schema.AgentNavi, schema.DirectiveRepliedPayload{
		DirectiveID: "d1", MessageID: "m1",
	})
	_ = bus.Publish(ctx, ev)

	if called {
		t.Fatal("handler should not have been called for a different event type")
	}
}

func TestDualWriteToEventLog(t *testing.T) {
	b := testBusDB(t)
	defer b.Close()
	ctx := context.Background()

	ev := schema.NewEvent(schema.FactDirectiveReplied, schema.EventKindFact, "dual-corr", schema.AgentNavi, schema.DirectiveRepliedPayload{
		DirectiveID: "d1", MessageID: "m1",
	})
	if err := b.Publish(ctx, ev); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Verify the event was persisted in SQLite.
	events, err := store.EventsByCorrelationID(ctx, b.db, "dual-corr")
	if err != nil {
		t.Fatalf("EventsByCorrelationID: %v", err)
	}
	if len(events) != 1 || events[0].ID != ev.ID {
		t.Fatalf("expected 1 event with ID %q, got %d events", ev.ID, len(events))
	}
}

func TestSubjectMapping(t *testing.T) {
	subject := SubjectFor(schema.CmdDirectiveMessage)
	wantSubj := "navi.cmd.directive.message"
	if subject != wantSubj {
		t.Fatalf("expected %q, got %q", wantSubj, subject)
	}
	et := EventTypeFromSubject(subject)
	if et != schema.CmdDirectiveMessage {
		t.Fatalf("expected cmd.directive.message, got %q", et)
	}
	// Bad prefix
	if EventTypeFromSubject("wrong.prefix") != "" {
		t.Fatal("expected empty for bad prefix")
	}
}

func TestClosePreventsFurtherPublish(t *testing.T) {
	b := testBusDB(t)
	ctx := context.Background()

	_ = b.Close()

	ev := schema.NewEvent(schema.FactDirectiveReplied, schema.EventKindFact, "closed-corr", schema.AgentNavi, schema.DirectiveRepliedPayload{
		DirectiveID: "d1", MessageID: "m1",
	})

	err := b.Publish(ctx, ev)
	if err == nil {
		t.Fatal("expected error publishing to closed bus")
	}
}
