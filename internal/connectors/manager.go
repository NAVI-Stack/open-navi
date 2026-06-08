package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/open-navi/navi/connectors"
	"github.com/open-navi/navi/internal/command"
	"github.com/open-navi/navi/internal/hooks"
	"github.com/open-navi/navi/internal/schema"
)

// SaveExecutionOutcomeFunc persists an execution outcome (e.g. for Send command recording).
type SaveExecutionOutcomeFunc func(ctx context.Context, eo schema.ExecutionOutcome) error
type SaveErrorRecordFunc func(ctx context.Context, component, chatID, runID, errorType, message, contextJSON string) error
type RecordDeliveryAttemptFunc func(ctx context.Context, deliveryID, status string, attemptDelta int, lastError string) error

// ManagerConfig holds configuration for the connector Manager.
type ManagerConfig struct {
	// RateLimits overrides per-connector rate limits (msg/sec).
	RateLimits map[string]float64
	// SaveExecutionOutcome when set records each Send (connector delivery) as an execution outcome.
	SaveExecutionOutcome SaveExecutionOutcomeFunc
	// SaveErrorRecord records connector failures into the structured diagnostics store.
	SaveErrorRecord SaveErrorRecordFunc
	// RecordDeliveryAttempt updates an already-created message delivery row from
	// the connector worker's existing bounded retry loop.
	RecordDeliveryAttempt RecordDeliveryAttemptFunc
}

// Manager wraps a Registry and adds per-connector workers with rate limiting,
// retry, message splitting, and graceful shutdown. It is the single entry point
// for dispatching outbound messages to connectors.
type Manager struct {
	registry *Registry
	config   ManagerConfig
	workers  map[string]*connectorWorker
	Diag     *DiagCollector
	Hooks    *hooks.Registry

	// circuit state is tracked per connector instance to implement a simple
	// circuit-breaker: when error thresholds are exceeded, the circuit opens
	// and Dispatch short-circuits until a cool-down window allows a probe.
	circuits map[string]*circuitState

	mu             sync.RWMutex
	dispatchCancel context.CancelFunc

	// Orchestration state: keyed by "connector:chatID"
	typingStops   sync.Map // → typingEntry
	reactionUndos sync.Map // → reactionEntry
	placeholders  sync.Map // → placeholderEntry
}

// NewManager creates a Manager that wraps the given Registry.
func NewManager(registry *Registry, cfg ManagerConfig) *Manager {
	if cfg.RateLimits == nil {
		cfg.RateLimits = make(map[string]float64)
	}
	return &Manager{
		registry: registry,
		config:   cfg,
		workers:  make(map[string]*connectorWorker),
		Diag:     NewDiagCollector(),
		Hooks:    hooks.NewRegistry(),
		circuits: make(map[string]*circuitState),
	}
}

// Registry returns the underlying connector registry.
func (m *Manager) Registry() *Registry {
	return m.registry
}

// InvokeAction executes a capability-based connector action against a specific
// connector instance and returns a normalized ResultEnvelope. For the current
// v1 implementation, this supports a single generic messaging.send capability
// and bridges it onto the existing Dispatch path.
func (m *Manager) InvokeAction(ctx context.Context, instanceID string, capability string, input json.RawMessage) (ResultEnvelope, error) {
	start := time.Now()

	// Locate the instance in the v2 registry model.
	var inst ConnectorInstance
	for _, ci := range m.registry.InstancesV2() {
		if ci.InstanceID() == instanceID {
			inst = ci
			break
		}
	}
	if inst == nil {
		return ResultEnvelope{
			Status:              "error",
			Error:               "unknown connector instance",
			Retryable:           false,
			ConnectorInstanceID: instanceID,
			Capability:          capability,
			DurationMS:          time.Since(start).Milliseconds(),
		}, ErrUnknownConnector
	}

	// For now we only support the generic messaging.send capability and map it
	// onto the existing OutboundMessage + Dispatch path.
	if capability != "messaging.send" {
		return ResultEnvelope{
			Status:              "error",
			Error:               "unsupported capability",
			Retryable:           false,
			ConnectorInstanceID: instanceID,
			Capability:          capability,
			DurationMS:          time.Since(start).Milliseconds(),
		}, nil
	}

	var payload struct {
		Channel          string `json:"channel"`
		ChatID           string `json:"chat_id"`
		Content          string `json:"content"`
		RuntimeSessionID string `json:"runtime_session_id,omitempty"`
		RunID            string `json:"run_id,omitempty"`
		CorrelationID    string `json:"correlation_id,omitempty"`
		SourceMessageRef string `json:"source_message_ref,omitempty"`
		ReplyToMessageID int64  `json:"reply_to_message_id"`
		MessageThreadID  int64  `json:"message_thread_id"`
		ParseMode        string `json:"parse_mode"`
	}
	if err := json.Unmarshal(input, &payload); err != nil {
		return ResultEnvelope{
			Status:              "error",
			Error:               err.Error(),
			Retryable:           false,
			ConnectorInstanceID: instanceID,
			Capability:          capability,
			DurationMS:          time.Since(start).Milliseconds(),
		}, err
	}

	runtimeSessID := payload.RuntimeSessionID
	if runtimeSessID == "" {
		runtimeSessID = payload.ChatID
	}
	desc, hasDesc := command.DescriptorFromContext(ctx)
	msg := connectors.OutboundMessage{
		Channel:          payload.Channel,
		ChatID:           payload.ChatID,
		Content:          payload.Content,
		RuntimeSessionID: runtimeSessID,
		RunID:            payload.RunID,
		CorrelationID:    payload.CorrelationID,
		SourceMessageRef: payload.SourceMessageRef,
		ReplyToMessageID: payload.ReplyToMessageID,
		MessageThreadID:  payload.MessageThreadID,
		ParseMode:        payload.ParseMode,
	}
	if hasDesc {
		if msg.RuntimeSessionID == "" {
			msg.RuntimeSessionID = desc.RuntimeSessionID
		}
		if msg.RunID == "" {
			msg.RunID = desc.RunID
		}
		if msg.CorrelationID == "" {
			msg.CorrelationID = desc.CorrelationID
		}
	}

	err := m.Dispatch(ctx, instanceID, msg)
	env := ResultEnvelope{
		Status:              "ok",
		Retryable:           false,
		ConnectorInstanceID: instanceID,
		Capability:          capability,
		DurationMS:          time.Since(start).Milliseconds(),
	}

	if err != nil {
		env.Status = "error"
		env.Error = err.Error()
		// Classify retryability based on the underlying connector error type.
		switch {
		case errors.Is(err, ErrUnknownConnector), errors.Is(err, connectors.ErrNotRunning), errors.Is(err, connectors.ErrSendFailed):
			// Unknown connector or explicit permanent failures should not be retried.
			env.Retryable = false
		case errors.Is(err, connectors.ErrRateLimit), errors.Is(err, connectors.ErrTemporary):
			// Rate limit and temporary failures are retryable.
			env.Retryable = true
		default:
			// For unknown error types, default to non-retryable to avoid unsafe loops.
			env.Retryable = false
		}
	}

	// Minimal output payload for now; callers can treat absence of output as
	// success acknowledgement only.
	if env.Status == "ok" {
		env.Output = json.RawMessage(`{"status":"sent"}`)
	}

	return env, err
}

// StartAll creates a worker for each registered connector instance and starts
// both the connector and its worker goroutine.
func (m *Manager) StartAll(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.registry.mu.RLock()
	instances := make(map[string]connectors.Connector)
	for k, v := range m.registry.instances {
		instances[k] = v
	}
	m.registry.mu.RUnlock()

	for name, conn := range instances {
		m.startConnectorLocked(ctx, name, conn)
	}
}

// StartOne starts a single connector by name. Used when a connector is added
// at runtime (e.g. via first-run onboarding or gateway API).
func (m *Manager) StartOne(ctx context.Context, name string) {
	conn := m.registry.Get(name)
	if conn == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startConnectorLocked(ctx, name, conn)
}

// startConnectorLocked starts a connector and its worker. Caller must hold m.mu.
func (m *Manager) startConnectorLocked(ctx context.Context, name string, conn connectors.Connector) {
	// Don't double-start
	if _, exists := m.workers[name]; exists {
		return
	}

	rateLimit := m.config.RateLimits[name]
	w := newWorker(name, conn, rateLimit, m.config.SaveExecutionOutcome)
	w.runCtx, w.cancel = context.WithCancel(context.Background())
	m.workers[name] = w

	// Start the connector in its own goroutine
	go func() {
		if err := conn.Start(w.runCtx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("connector %s stopped: %v", name, err)
			if m.config.SaveErrorRecord != nil {
				contextJSON := fmt.Sprintf(`{"connector":"%s","phase":"runtime","transport":"manager_start"}`, name)
				_ = m.config.SaveErrorRecord(ctx, "connector", "", "", "connector_runtime_failed", err.Error(), contextJSON)
			}
			m.Diag.Push(DiagError, name, "connector runtime failed", err)
			m.Hooks.Fire(ctx, hooks.ConnectorError, &hooks.HookEvent{
				Connector: name,
				Timestamp: time.Now(),
				Data:      map[string]any{"error": err.Error(), "phase": "runtime"},
			})
		}
	}()

	// Start the worker loop
	go w.runLoop(w.runCtx, m)

	m.registry.setStartedAt(name, time.Now().UTC())
	log.Printf("connector %s started (rate=%.1f msg/s, queue=%d)", name, float64(w.limiter.Limit()), defaultQueueSize)
	m.Hooks.Fire(ctx, hooks.ConnectorStarted, &hooks.HookEvent{Connector: name, Timestamp: time.Now()})
}

// Dispatch enqueues an outbound message to the named connector's worker.
// The worker handles rate limiting, retry, and splitting.
func (m *Manager) Dispatch(ctx context.Context, connectorName string, msg connectors.OutboundMessage) error {
	// Short-circuit when the connector's circuit is open. This prevents
	// hammering an unhealthy or unavailable external system; Health surfaces
	// degradation via InstanceMetadata and ConnectorHealth.
	if m.isCircuitOpen(connectorName) {
		return connectors.ErrNotRunning
	}

	m.mu.RLock()
	w, ok := m.workers[connectorName]
	m.mu.RUnlock()
	if !ok {
		return ErrUnknownConnector
	}
	return w.Enqueue(ctx, msg)
}

// DispatchAll enqueues a message to all running connector workers (broadcast).
func (m *Manager) DispatchAll(ctx context.Context, msg connectors.OutboundMessage) {
	m.mu.RLock()
	workers := make([]*connectorWorker, 0, len(m.workers))
	for _, w := range m.workers {
		workers = append(workers, w)
	}
	m.mu.RUnlock()

	for _, w := range workers {
		if w.connector.IsRunning() {
			_ = w.Enqueue(ctx, msg)
		}
	}
}

// StopOne gracefully stops one connector worker and connector instance.
func (m *Manager) StopOne(ctx context.Context, name string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	stopCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	m.mu.Lock()
	w, ok := m.workers[name]
	if ok {
		delete(m.workers, name)
	}
	m.mu.Unlock()

	if !ok {
		return ErrUnknownConnector
	}

	close(w.queue)
	select {
	case <-w.done:
		log.Printf("connector %s worker drained", name)
	case <-stopCtx.Done():
		log.Printf("connector %s worker drain timeout", name)
	}

	if w.cancel != nil {
		w.cancel()
	}
	if err := w.connector.Stop(stopCtx); err != nil {
		return err
	}
	m.Hooks.Fire(stopCtx, hooks.ConnectorStopped, &hooks.HookEvent{Connector: name, Timestamp: time.Now()})
	m.registry.mu.Lock()
	delete(m.registry.startedAt, name)
	m.registry.mu.Unlock()
	return nil
}

// StopAll gracefully shuts down all connectors:
//  1. Close all worker queues (no more messages accepted)
//  2. Wait for workers to drain remaining messages (with timeout)
//  3. Stop all connector instances
func (m *Manager) StopAll(ctx context.Context, timeout time.Duration) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	stopCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	m.mu.Lock()
	workers := make(map[string]*connectorWorker, len(m.workers))
	for k, v := range m.workers {
		workers[k] = v
	}
	m.mu.Unlock()

	// 1. Close all worker queues
	for _, w := range workers {
		close(w.queue)
	}

	// 2. Wait for all workers to finish draining
	for name, w := range workers {
		select {
		case <-w.done:
			log.Printf("connector %s worker drained", name)
		case <-stopCtx.Done():
			log.Printf("connector %s worker drain timeout", name)
		}
	}

	// 3. Stop all connector instances
	for name, w := range workers {
		if w.cancel != nil {
			w.cancel()
		}
		if err := w.connector.Stop(stopCtx); err != nil {
			log.Printf("connector %s stop error: %v", name, err)
		}
		m.Hooks.Fire(stopCtx, hooks.ConnectorStopped, &hooks.HookEvent{Connector: name, Timestamp: time.Now()})
		m.registry.mu.Lock()
		delete(m.registry.startedAt, name)
		m.registry.mu.Unlock()
	}

	m.mu.Lock()
	m.workers = make(map[string]*connectorWorker)
	m.mu.Unlock()
}

const healthCheckTimeout = 5 * time.Second

// Health returns health info for all managed connectors. When a connector
// implements connectors.HealthChecker, Health() calls HealthCheck and marks
// the connector "down" if the probe fails.
func (m *Manager) Health() []ConnectorHealth {
	m.mu.RLock()
	var out []ConnectorHealth
	type probeTarget struct {
		idx int
		hc  connectors.HealthChecker
		key string
	}
	var toProbe []probeTarget
	for name, w := range m.workers {
		status := "healthy"
		if w.consecutiveErrs >= 20 {
			status = "down"
		} else if w.consecutiveErrs >= 5 {
			status = "degraded"
		}
		if !w.connector.IsRunning() {
			status = "down"
		}
		idx := len(out)
		out = append(out, ConnectorHealth{
			Name:            name,
			Status:          status,
			LastSendAt:      w.lastSendAt,
			LastErrorAt:     w.lastErrorAt,
			SendCount:       w.sendCount,
			ErrorCount:      w.errorCount,
			ConsecutiveErrs: w.consecutiveErrs,
			QueueDepth:      len(w.queue),
		})
		if hc, ok := w.connector.(connectors.HealthChecker); ok {
			toProbe = append(toProbe, probeTarget{idx: idx, hc: hc, key: name})
		}

		// Best-effort propagation into v2 InstanceMetadata.
		if m.registry != nil {
			m.registry.UpdateHealth(name, status, w.lastErrorAt)
		}
	}
	m.mu.RUnlock()

	for _, p := range toProbe {
		ctx, cancel := context.WithTimeout(context.Background(), healthCheckTimeout)
		if err := p.hc.HealthCheck(ctx); err != nil {
			out[p.idx].Status = "down"
			if m.registry != nil {
				m.registry.UpdateHealth(p.key, "down", time.Now())
			}
		}
		cancel()
	}
	return out
}

// ConnectorHealth represents the health status of a managed connector.
type ConnectorHealth struct {
	Name            string    `json:"name"`
	Status          string    `json:"status"` // "healthy", "degraded", "down"
	LastSendAt      time.Time `json:"last_send_at"`
	LastErrorAt     time.Time `json:"last_error_at,omitempty"`
	SendCount       int64     `json:"send_count"`
	ErrorCount      int64     `json:"error_count"`
	ConsecutiveErrs int       `json:"consecutive_errors"`
	QueueDepth      int       `json:"queue_depth"`
}

// ---------------------------------------------------------------------------
// Orchestration: typing, reactions, placeholders
// ---------------------------------------------------------------------------

type typingEntry struct {
	stop      func()
	createdAt time.Time
}

type reactionEntry struct {
	undo      func()
	createdAt time.Time
}

type placeholderEntry struct {
	messageID string
	createdAt time.Time
}

func orchKey(connector, chatID string) string {
	return connector + ":" + chatID
}

// OnInbound is called when a connector receives an inbound message. It
// auto-triggers typing indicators, reactions, and placeholders for connectors
// that implement the corresponding capability interfaces.
func (m *Manager) OnInbound(ctx context.Context, connectorName, chatID, messageID string) {
	m.mu.RLock()
	w, ok := m.workers[connectorName]
	m.mu.RUnlock()
	if !ok {
		return
	}
	conn := w.connector
	key := orchKey(connectorName, chatID)

	// 1. Typing indicator
	if tc, ok := conn.(connectors.TypingCapable); ok {
		stop, err := tc.StartTyping(ctx, chatID)
		if err == nil && stop != nil {
			m.typingStops.Store(key, typingEntry{stop: stop, createdAt: time.Now()})
		}
	}

	// 2. Reaction
	if rc, ok := conn.(connectors.ReactionCapable); ok {
		undo, err := rc.ReactToMessage(ctx, chatID, messageID)
		if err == nil && undo != nil {
			m.reactionUndos.Store(key, reactionEntry{undo: undo, createdAt: time.Now()})
		}
	}

	// 3. Placeholder
	if pc, ok := conn.(connectors.PlaceholderCapable); ok {
		msgID, err := pc.SendPlaceholder(ctx, chatID)
		if err == nil && msgID != "" {
			m.placeholders.Store(key, placeholderEntry{messageID: msgID, createdAt: time.Now()})
		}
	}
}

// preSend is called by the worker before each Send(). It stops typing, undoes
// reactions, and attempts to edit a placeholder. Returns true if the placeholder
// was successfully edited (meaning the caller should skip Send).
func (m *Manager) preSend(ctx context.Context, connectorName, chatID string, content string, conn connectors.Connector) bool {
	key := orchKey(connectorName, chatID)

	// 1. Stop typing
	if val, ok := m.typingStops.LoadAndDelete(key); ok {
		entry := val.(typingEntry)
		entry.stop()
	}

	// 2. Undo reaction
	if val, ok := m.reactionUndos.LoadAndDelete(key); ok {
		entry := val.(reactionEntry)
		entry.undo()
	}

	// 3. Try editing placeholder
	if val, ok := m.placeholders.LoadAndDelete(key); ok {
		entry := val.(placeholderEntry)
		if editor, ok := conn.(connectors.MessageEditor); ok {
			if err := editor.EditMessage(ctx, chatID, entry.messageID, content); err == nil {
				return true // placeholder edited — skip Send
			}
			// Edit failed — fall through to normal Send
		}
	}

	return false
}

// StartJanitor starts a background goroutine that evicts stale orchestration
// entries. Typing entries expire after 5 minutes, placeholders after 10 minutes.
func (m *Manager) StartJanitor(ctx context.Context) {
	const (
		typingTTL      = 5 * time.Minute
		placeholderTTL = 10 * time.Minute
		reactionTTL    = 5 * time.Minute
		interval       = 10 * time.Second
	)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				now := time.Now()
				m.typingStops.Range(func(key, val any) bool {
					if now.Sub(val.(typingEntry).createdAt) > typingTTL {
						if v, ok := m.typingStops.LoadAndDelete(key); ok {
							v.(typingEntry).stop()
						}
					}
					return true
				})
				m.reactionUndos.Range(func(key, val any) bool {
					if now.Sub(val.(reactionEntry).createdAt) > reactionTTL {
						if v, ok := m.reactionUndos.LoadAndDelete(key); ok {
							v.(reactionEntry).undo()
						}
					}
					return true
				})
				m.placeholders.Range(func(key, val any) bool {
					if now.Sub(val.(placeholderEntry).createdAt) > placeholderTTL {
						m.placeholders.Delete(key)
					}
					return true
				})
			}
		}
	}()
}

// circuitState holds per-connector circuit-breaker state.
type circuitState struct {
	State            string
	FailureCount     int
	OpenedAt         time.Time
	LastErrorMessage string
}

const (
	circuitStateClosed   = "closed"
	circuitStateOpen     = "open"
	circuitStateHalfOpen = "half_open"

	// connectorCircuitFailureThreshold is the number of consecutive failed
	// sends (after exhausting per-message retries) before the circuit opens.
	connectorCircuitFailureThreshold = 5
	// connectorCircuitCooldown is how long an open circuit remains closed to
	// new traffic before allowing a single probe (half-open).
	connectorCircuitCooldown = 30 * time.Second
)

// onSendResult updates circuit state after a send attempt finishes. Called by
// connector workers once per logical send (after retry budget is exhausted).
func (m *Manager) onSendResult(name string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cs, ok := m.circuits[name]
	if !ok {
		cs = &circuitState{State: circuitStateClosed}
		m.circuits[name] = cs
	}

	// Success resets the circuit immediately.
	if err == nil {
		cs.State = circuitStateClosed
		cs.FailureCount = 0
		cs.LastErrorMessage = ""
		return
	}

	cs.FailureCount++
	cs.LastErrorMessage = err.Error()

	// Permanent failures open the circuit immediately; repeated transient
	// failures open it after the threshold.
	isPermanent := errors.Is(err, ErrUnknownConnector) ||
		errors.Is(err, connectors.ErrNotRunning) ||
		errors.Is(err, connectors.ErrSendFailed)

	if isPermanent || cs.FailureCount >= connectorCircuitFailureThreshold {
		if cs.State != circuitStateOpen {
			cs.State = circuitStateOpen
			cs.OpenedAt = time.Now()
		}
		if m.registry != nil {
			m.registry.UpdateHealth(name, "down", time.Now())
		}
		return
	}

	// For transient/unknown errors below threshold, keep the circuit closed
	// but allow Health() to reflect degradation via consecutive error counts.
}

// isCircuitOpen returns true when the circuit for connector name is open and
// still within its cool-down window. When the cool-down has elapsed, the
// circuit transitions to half-open and the next Dispatch is allowed through
// as a probe.
func (m *Manager) isCircuitOpen(name string) bool {
	m.mu.RLock()
	cs, ok := m.circuits[name]
	m.mu.RUnlock()
	if !ok {
		return false
	}

	if cs.State == circuitStateOpen {
		if time.Since(cs.OpenedAt) >= connectorCircuitCooldown {
			// Transition to half-open to allow a single probe.
			m.mu.Lock()
			// Re-check inside write lock to avoid races.
			if cs2, ok := m.circuits[name]; ok && cs2.State == circuitStateOpen {
				cs2.State = circuitStateHalfOpen
				m.circuits[name] = cs2
			}
			m.mu.Unlock()
			return false
		}
		return true
	}

	return false
}
