//go:build jetstream

package bus_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/open-navi/navi/internal/bus"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

func startTestServer(t *testing.T) *natsserver.Server {
	opts := &natsserver.Options{
		JetStream: true,
		StoreDir:  t.TempDir(),
		Port:      -1,
		NoLog:     true,
		NoSigs:    true,
	}
	s, err := natsserver.NewServer(opts)
	if err != nil {
		t.Fatalf("failed to start nats server: %v", err)
	}
	go s.Start()
	if !s.ReadyForConnections(5 * time.Second) {
		t.Fatalf("nats server not ready")
	}
	t.Cleanup(func() { s.Shutdown() })
	return s
}

func setupJSBus(t *testing.T, s *server.Server) (*bus.JetStreamBus, *nats.Conn, *sql.DB) {
	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { nc.Close() })

	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("jetstream: %v", err)
	}

	if err := bus.EnsureStreams(js); err != nil {
		t.Fatalf("ensure streams: %v", err)
	}

	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatalf("create tables: %v", err)
	}

	return bus.NewJetStreamBus(nc, js, db), nc, db
}

func TestJetStreamPublishConsumeRoundTrip(t *testing.T) {
	s := startTestServer(t)
	b, _, db := setupJSBus(t, s)
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var received schema.Event
	var wg sync.WaitGroup
	wg.Add(1)

	err := b.Subscribe(ctx, schema.FactDirectiveReplied, "test-queue", func(_ context.Context, ev schema.Event) {
		received = ev
		wg.Done()
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	ev := schema.NewEvent(schema.FactDirectiveReplied, schema.EventKindFact, "js-corr-1", schema.AgentNavi, schema.DirectiveRepliedPayload{
		DirectiveID: "d1", MessageID: "m1",
	})
	if err := b.Publish(ctx, ev); err != nil {
		t.Fatalf("publish: %v", err)
	}

	wg.Wait()
	if received.ID != ev.ID {
		t.Fatalf("expected event ID %q, got %q", ev.ID, received.ID)
	}

	events, err := store.EventsByCorrelationID(ctx, db, "js-corr-1")
	if err != nil {
		t.Fatalf("store read: %v", err)
	}
	if len(events) != 1 || events[0].ID != ev.ID {
		t.Fatal("event not dual-written to SQLite on publish")
	}
}

func TestVersionMismatchRejection(t *testing.T) {
	s := startTestServer(t)
	b, nc, db := setupJSBus(t, s)
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	called := false
	err := b.Subscribe(ctx, schema.FactDirectiveReplied, "test-queue-2", func(_ context.Context, ev schema.Event) {
		called = true
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	ev := schema.NewEvent(schema.FactDirectiveReplied, schema.EventKindFact, "bad-version", schema.AgentNavi, schema.DirectiveRepliedPayload{
		DirectiveID: "d1", MessageID: "m1",
	})
	ev.SchemaVersion = "0.0.0"

	// Bypassing b.Publish to raw publish via NATS directly, otherwise Validate will catch it before the bus gets it.
	data, _ := json.Marshal(ev)
	nc.Publish(bus.SubjectFor(ev.Type), data)

	time.Sleep(100 * time.Millisecond) // Give consumer time to reject

	if called {
		t.Fatal("handler was called despite version mismatch")
	}

	events, _ := store.EventsByCorrelationID(ctx, db, "bad-version")
	if len(events) != 0 {
		t.Fatal("event was written to SQLite despite version mismatch")
	}
}

func TestStreamPersistenceAcrossReconnect(t *testing.T) {
	s := startTestServer(t)

	// Open bus and close immediately after publishing
	b1, _, _ := setupJSBus(t, s)
	ctx := context.Background()

	ev := schema.NewEvent(schema.FactDirectiveReplied, schema.EventKindFact, "persist-corr", schema.AgentNavi, schema.DirectiveRepliedPayload{
		DirectiveID: "d1", MessageID: "m1",
	})

	if err := b1.Publish(ctx, ev); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// Small sleep to ensure JetStream acknowledges and persists before closing
	time.Sleep(50 * time.Millisecond)
	b1.Close()

	// Reconnect and verify we get it based on the durable consumer
	b2, _, _ := setupJSBus(t, s)
	defer b2.Close()

	var received schema.Event
	var wg sync.WaitGroup
	wg.Add(1)

	// Will attach to existing "navi-consumer"
	err := b2.Subscribe(ctx, schema.FactDirectiveReplied, "navi-consumer", func(_ context.Context, ev schema.Event) {
		received = ev
		wg.Done()
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	wg.Wait()
	if received.ID != ev.ID {
		t.Fatalf("failed to receive persisted event on durable reconnect")
	}
}

func TestEnsureStreamsIdempotency(t *testing.T) {
	s := startTestServer(t)

	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("jetstream: %v", err)
	}

	// Call 1
	if err := bus.EnsureStreams(js); err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	// Call 2
	if err := bus.EnsureStreams(js); err != nil {
		t.Fatalf("second call failed (not idempotent): %v", err)
	}
}
