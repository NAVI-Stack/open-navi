package hooks

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"
)

// HookName identifies a lifecycle hook point.
type HookName string

const (
	ConnectorStarted  HookName = "connector_started"
	ConnectorStopped  HookName = "connector_stopped"
	ConnectorError    HookName = "connector_error"
	MessageReceived   HookName = "message_received"
	MessageSending    HookName = "message_sending" // MUTABLE — can modify content or cancel
	MessageSent       HookName = "message_sent"
	MessageSendFailed HookName = "message_send_failed"
	HealthChanged     HookName = "health_changed"
)

// HookEvent is the base event type. Concrete events embed this.
type HookEvent struct {
	Connector string         `json:"connector"`
	Timestamp time.Time      `json:"timestamp"`
	Data      map[string]any `json:"data,omitempty"`
}

// MessageSendingEvent is the only mutable hook event. Handlers can modify
// Content or set Cancel=true to suppress sending.
type MessageSendingEvent struct {
	HookEvent
	ChatID  string `json:"chat_id"`
	Content string `json:"content"` // mutable
	Cancel  bool   `json:"cancel"`  // mutable — if true, message is dropped
}

// HookHandler processes a hook event. For MessageSending, the event is a
// *MessageSendingEvent and can be mutated.
type HookHandler func(ctx context.Context, event any) error

type registeredHandler struct {
	id       string
	priority int
	handler  HookHandler
}

// Registry manages lifecycle hook handlers.
type Registry struct {
	mu       sync.RWMutex
	handlers map[HookName][]registeredHandler
}

// NewRegistry creates a new hook registry.
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[HookName][]registeredHandler),
	}
}

// Register adds a handler for the given hook. Lower priority fires first.
// Default priority is 100. The id is for logging/debugging.
func (r *Registry) Register(name HookName, id string, handler HookHandler, priority int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[name] = append(r.handlers[name], registeredHandler{
		id:       id,
		priority: priority,
		handler:  handler,
	})
	sort.Slice(r.handlers[name], func(i, j int) bool {
		return r.handlers[name][i].priority < r.handlers[name][j].priority
	})
}

// Fire invokes all handlers for the given hook in priority order.
//
// For read-only hooks: all handlers fire regardless of errors (errors logged).
// For MessageSending: if Cancel is set, remaining handlers are skipped.
//
// Returns nil for read-only hooks, or the first error for mutable hooks.
func (r *Registry) Fire(ctx context.Context, name HookName, event any) error {
	r.mu.RLock()
	handlers := r.handlers[name]
	r.mu.RUnlock()

	if len(handlers) == 0 {
		return nil
	}

	isMutable := name == MessageSending

	for _, h := range handlers {
		start := time.Now()
		err := h.handler(ctx, event)
		elapsed := time.Since(start)

		if elapsed > 100*time.Millisecond {
			log.Printf("hooks: slow handler %q for %s took %v", h.id, name, elapsed)
		}

		if err != nil {
			log.Printf("hooks: handler %q for %s error: %v", h.id, name, err)
			if isMutable {
				return err
			}
			// Read-only hooks: continue firing
		}

		// Check cancel for mutable hooks
		if isMutable {
			if mse, ok := event.(*MessageSendingEvent); ok && mse.Cancel {
				return nil // cancelled — skip remaining handlers
			}
		}
	}

	return nil
}

// Count returns the number of handlers registered for a hook.
func (r *Registry) Count(name HookName) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.handlers[name])
}
