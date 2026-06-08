package bus

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"strings"
	"sync"

	"github.com/nats-io/nats.go"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

// Handler is a callback invoked when a subscribed event is received.
type Handler func(ctx context.Context, ev schema.Event)

// Bus is the event transport interface. Implementations must validate events
// before publishing and dual-write to the SQLite event log.
type Bus interface {
	// Publish validates the event envelope and kind, publishes to the transport,
	// then appends to the SQLite event log. Returns an error if either step fails.
	Publish(ctx context.Context, ev schema.Event) error

	// Subscribe registers a handler for the given event type. The handler is
	// invoked in a separate goroutine for each received event. The queue parameter
	// acts as the durable consumer group name.
	Subscribe(ctx context.Context, eventType schema.EventType, queue string, handler Handler) error

	// PurgeAll removes all messages from all streams (JetStream) or is a no-op (MemBus).
	// Used for instance reset so the instance is logically empty.
	PurgeAll(ctx context.Context) error

	// Healthy reports whether the bus transport is connected and operational.
	Healthy() bool

	// Close shuts down the bus and releases resources.
	Close() error
}

// subjectPrefix is the NATS subject namespace.
const subjectPrefix = "navi."

// natsSubject converts an EventType to a NATS subject.
// Example: "cmd.task.create" → "navi.cmd.task.create"
func natsSubject(et schema.EventType) string {
	return subjectPrefix + string(et)
}

// ---------------------------------------------------------------------------
// JetStreamBus — production implementation backed by NATS JetStream
// ---------------------------------------------------------------------------

// JetStreamBus publishes and subscribes to events over a NATS JetStream connection.
type JetStreamBus struct {
	conn *nats.Conn
	JS   nats.JetStreamContext
	db   *sql.DB
	subs []*nats.Subscription
	mu   sync.Mutex
}

// NewJetStreamBus wraps an existing NATS connection, uses its JetStream context, and an SQLite handle.
func NewJetStreamBus(conn *nats.Conn, js nats.JetStreamContext, db *sql.DB) *JetStreamBus {
	return &JetStreamBus{conn: conn, JS: js, db: db}
}

// Publish validates, sends via JetStream, then appends to the event log.
// If NATS publishes successfully but the SQLite audit write fails, the event
// IS delivered to subscribers — we log the gap and return nil so callers don't
// treat a delivered event as a failure.
func (b *JetStreamBus) Publish(ctx context.Context, ev schema.Event) error {
	if err := ev.Validate(); err != nil {
		return fmt.Errorf("bus: %w", err)
	}
	if err := schema.ValidateEventKind(ev); err != nil {
		return fmt.Errorf("bus: %w", err)
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("bus: marshal event: %w", err)
	}
	if _, err := b.JS.Publish(natsSubject(ev.Type), data); err != nil {
		return fmt.Errorf("bus: jetstream publish: %w", err)
	}
	if err := store.AppendEvent(ctx, b.db, ev); err != nil {
		slog.Warn("bus: NATS delivered but SQLite append failed — event log gap",
			"event_id", ev.ID,
			"type", string(ev.Type),
			"error", err,
		)
	}
	return nil
}

// Subscribe registers a handler for a specific event type on JetStream using a durable consumer.
func (b *JetStreamBus) Subscribe(ctx context.Context, eventType schema.EventType, queue string, handler Handler) error {
	subj := natsSubject(eventType)
	sub, err := b.JS.Subscribe(subj, func(msg *nats.Msg) {
		var ev schema.Event
		if err := json.Unmarshal(msg.Data, &ev); err != nil {
			msg.Nak()
			return
		}

		if !schema.AcceptableSchemaVersions()[ev.SchemaVersion] {
			log.Printf("bus: schema version mismatch: got %q, subject %q", ev.SchemaVersion, subj)
			msg.Nak()
			return
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("bus: handler panic", "subject", subj, "panic", r)
					msg.Nak()
				} else {
					msg.Ack()
				}
			}()
			handler(ctx, ev)
		}()
	}, nats.Durable(queue), nats.ManualAck())

	if err != nil {
		return fmt.Errorf("bus: jetstream subscribe: %w", err)
	}

	b.mu.Lock()
	b.subs = append(b.subs, sub)
	b.mu.Unlock()
	return nil
}

// PurgeAll purges all NAVI JetStream streams so no messages remain.
func (b *JetStreamBus) PurgeAll(ctx context.Context) error {
	return PurgeStreams(b.JS)
}

// Healthy reports whether the underlying NATS connection is still active.
func (b *JetStreamBus) Healthy() bool {
	return b.conn != nil && b.conn.IsConnected()
}

// Close drains all subscriptions and closes the NATS connection.
func (b *JetStreamBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, sub := range b.subs {
		_ = sub.Unsubscribe()
	}
	b.subs = nil
	b.conn.Close()
	return nil
}

// ---------------------------------------------------------------------------
// NATSBus — production implementation backed by a NATS connection
// ---------------------------------------------------------------------------

// MemBus is a channel-based in-memory Bus that requires no external services.
type MemBus struct {
	db       *sql.DB
	mu       sync.RWMutex
	handlers map[string][]Handler // key: EventType
	closed   bool
}

// NewMemBus creates an in-memory bus backed by the given SQLite handle for
// dual-write event log persistence.
func NewMemBus(db *sql.DB) *MemBus {
	return &MemBus{
		db:       db,
		handlers: make(map[string][]Handler),
	}
}

// Publish validates the event, appends to the event log, then fans out to
// all registered handlers for the event type.
func (b *MemBus) Publish(ctx context.Context, ev schema.Event) error {
	if err := ev.Validate(); err != nil {
		return fmt.Errorf("bus: %w", err)
	}
	if err := schema.ValidateEventKind(ev); err != nil {
		return fmt.Errorf("bus: %w", err)
	}
	if !schema.AcceptableSchemaVersions()[ev.SchemaVersion] {
		return fmt.Errorf("bus: schema version mismatch: got %q", ev.SchemaVersion)
	}

	// Acquire the read lock before the DB write so that a concurrent Close()
	// cannot set b.closed and nil out b.handlers between the store append and
	// the fan-out, which would cause a false-negative error after a successful
	// durable write.
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return fmt.Errorf("bus: closed")
	}
	if err := store.AppendEvent(ctx, b.db, ev); err != nil {
		return fmt.Errorf("bus: store append: %w", err)
	}
	key := string(ev.Type)
	for _, h := range b.handlers[key] {
		h(ctx, ev)
	}
	return nil
}

// Subscribe registers a handler for the given event type.
func (b *MemBus) Subscribe(_ context.Context, eventType schema.EventType, queue string, handler Handler) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return fmt.Errorf("bus: closed")
	}
	key := string(eventType)
	b.handlers[key] = append(b.handlers[key], handler)
	return nil
}

// PurgeAll is a no-op for in-memory bus.
func (b *MemBus) PurgeAll(_ context.Context) error {
	return nil
}

// Healthy reports whether the in-memory bus is still open.
func (b *MemBus) Healthy() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return !b.closed
}

// Close marks the bus as closed. Subsequent publishes return an error.
func (b *MemBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	b.handlers = nil
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// SubjectFor returns the NATS subject string for an EventType. Exported for
// tests and external tooling that need subject-level access.
func SubjectFor(et schema.EventType) string {
	return natsSubject(et)
}

// EventTypeFromSubject extracts the EventType from a NATS subject by stripping
// the prefix. Returns empty string if the subject doesn't start with the prefix.
func EventTypeFromSubject(subject string) schema.EventType {
	if !strings.HasPrefix(subject, subjectPrefix) {
		return ""
	}
	return schema.EventType(strings.TrimPrefix(subject, subjectPrefix))
}
