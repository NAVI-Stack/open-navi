package connectors

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/ceoai/navi/connectors"
	"github.com/ceoai/navi/internal/hooks"
	"github.com/ceoai/navi/internal/schema"
	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

// defaultQueueSize is the bounded channel size per worker. When full, dispatch
// will block, propagating backpressure to the caller (e.g. bus subscriber).
const defaultQueueSize = 64

// defaultRateLimits are per-platform send-rate defaults (messages/second).
// These can be overridden via ManagerConfig.RateLimits.
var defaultRateLimits = map[string]float64{
	"telegram": 20, // Telegram allows ~30 msg/s; leave headroom
	"slack":    1,  // Slack tier-2 rate limit
	"discord":  1,  // Discord per-channel rate limit
}

// connectorWorker wraps a Connector with a bounded send queue and rate limiter.
type connectorWorker struct {
	connector   connectors.Connector
	queue       chan connectors.OutboundMessage
	done        chan struct{}
	limiter     *rate.Limiter
	name        string
	saveOutcome func(context.Context, schema.ExecutionOutcome) error
	runCtx      context.Context
	cancel      context.CancelFunc

	sendCount       int64
	errorCount      int64
	consecutiveErrs int
	lastSendAt      time.Time
	lastErrorAt     time.Time
}

// newWorker creates a worker for the given connector with the specified rate.
// saveOutcome when non-nil is called after each Send with an ExecutionOutcome (Send command).
func newWorker(name string, conn connectors.Connector, rateLimit float64, saveOutcome func(context.Context, schema.ExecutionOutcome) error) *connectorWorker {
	if rateLimit <= 0 {
		rateLimit = defaultRateLimits[name]
		if rateLimit <= 0 {
			for baseName, baseRate := range defaultRateLimits {
				if strings.HasPrefix(name, baseName+"-") {
					rateLimit = baseRate
					break
				}
			}
		}
	}
	if rateLimit <= 0 {
		rateLimit = 10
	}
	burst := int(math.Max(1, math.Ceil(rateLimit/2)))
	return &connectorWorker{
		connector:   conn,
		queue:       make(chan connectors.OutboundMessage, defaultQueueSize),
		done:        make(chan struct{}),
		limiter:     rate.NewLimiter(rate.Limit(rateLimit), burst),
		name:        name,
		saveOutcome: saveOutcome,
	}
}

// runLoop drains the queue, applying rate limiting and retry for each message.
// It returns when the queue channel is closed (graceful shutdown) or ctx is cancelled.
func (w *connectorWorker) runLoop(ctx context.Context, mgr *Manager) {
	defer close(w.done)
	for {
		select {
		case msg, ok := <-w.queue:
			if !ok {
				return // queue closed — drain complete
			}
			w.processMessage(ctx, mgr, msg)
		case <-ctx.Done():
			// Context cancelled — drain remaining messages with best effort
			for {
				select {
				case msg, ok := <-w.queue:
					if !ok {
						return
					}
					drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					w.processMessage(drainCtx, mgr, msg)
					cancel()
				default:
					return
				}
			}
		}
	}
}

// processMessage splits the message if needed, then sends each part with retry.
func (w *connectorWorker) processMessage(ctx context.Context, mgr *Manager, msg connectors.OutboundMessage) {
	maxLen := 0
	if mlp, ok := w.connector.(connectors.MessageLengthProvider); ok {
		maxLen = mlp.MaxMessageLength()
	}

	if maxLen > 0 && len([]rune(msg.Content)) > maxLen {
		for _, chunk := range SplitMessage(msg.Content, maxLen) {
			chunkMsg := msg
			chunkMsg.Content = chunk
			w.sendWithRetry(ctx, mgr, chunkMsg)
		}
	} else {
		w.sendWithRetry(ctx, mgr, msg)
	}
}

// sendWithRetry sends a single message with rate limiting and classified-error retry.
func (w *connectorWorker) sendWithRetry(ctx context.Context, mgr *Manager, msg connectors.OutboundMessage) {
	const maxRetries = 3
	baseBackoff := 500 * time.Millisecond
	maxBackoff := 8 * time.Second
	rateLimitDelay := 1 * time.Second

	startTime := time.Now().UTC()
	commandID := uuid.New().String()
	recordOutcome := func(outcome schema.ExecutionOutcomeOutcome, failureReason string) {
		if w.saveOutcome == nil {
			return
		}
		endTime := time.Now().UTC()
		eo := schema.ExecutionOutcome{
			AttemptID:            commandID + ":1",
			CommandID:            commandID,
			AttemptNumber:        1,
			CommandType:          schema.CommandTypeSend,
			StartTime:            startTime,
			EndTime:              &endTime,
			Outcome:              outcome,
			FailureReason:        failureReason,
			AffectedEntities:     fmt.Sprintf(`["connector:%s","chat:%s"]`, w.name, msg.ChatID),
			Retryable:            false,
			CompensationRequired: false,
			CompensationStatus:   schema.CompensationStatusNotRequired,
			RecoveryStatus:       schema.RecoveryStatusNotRequired,
		}
		_ = w.saveOutcome(ctx, eo)
	}

	// 1. Rate limit
	if err := w.limiter.Wait(ctx); err != nil {
		mgr.recordDeliveryAttempt(ctx, msg, "failed", 0, err.Error())
		return
	}

	// 2. Pre-send orchestration (stop typing, undo reaction, edit placeholder)
	if mgr.preSend(ctx, w.name, msg.ChatID, msg.Content, w.connector) {
		w.sendCount++
		w.consecutiveErrs = 0
		w.lastSendAt = time.Now()
		recordOutcome(schema.ExecutionOutcomeSucceeded, "")
		mgr.recordDeliveryAttempt(ctx, msg, "sent", 1, "")
		return
	}

	// 3. Fire message_sending hook (mutable — can modify content or cancel)
	sendingEvent := &hooks.MessageSendingEvent{
		HookEvent: hooks.HookEvent{Connector: w.name, Timestamp: time.Now()},
		ChatID:    msg.ChatID,
		Content:   msg.Content,
	}
	mgr.Hooks.Fire(ctx, hooks.MessageSending, sendingEvent)
	if sendingEvent.Cancel {
		mgr.recordDeliveryAttempt(ctx, msg, "skipped", 0, "message_sending hook cancelled delivery")
		return // hook cancelled delivery
	}
	msg.Content = sendingEvent.Content // apply mutations

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		lastErr = w.connector.Send(ctx, msg)
		if lastErr == nil {
			w.sendCount++
			w.consecutiveErrs = 0
			w.lastSendAt = time.Now()
			recordOutcome(schema.ExecutionOutcomeSucceeded, "")
			mgr.recordDeliveryAttempt(ctx, msg, "sent", 1, "")
			// Inform the manager so circuit state can be reset on success.
			mgr.onSendResult(w.name, nil)
			mgr.Hooks.Fire(ctx, hooks.MessageSent, &hooks.HookEvent{
				Connector: w.name,
				Timestamp: time.Now(),
				Data:      map[string]any{"chat_id": msg.ChatID},
			})
			return
		}

		w.errorCount++
		w.consecutiveErrs++
		w.lastErrorAt = time.Now()

		// Permanent errors — stop immediately
		if errors.Is(lastErr, connectors.ErrNotRunning) || errors.Is(lastErr, connectors.ErrSendFailed) {
			mgr.recordDeliveryAttempt(ctx, msg, "failed", 1, lastErr.Error())
			break
		}

		// Last attempt — don't sleep
		if attempt == maxRetries {
			mgr.recordDeliveryAttempt(ctx, msg, "failed", 1, lastErr.Error())
			break
		}
		mgr.recordDeliveryAttempt(ctx, msg, "queued", 1, lastErr.Error())

		// Rate limited — fixed delay
		if errors.Is(lastErr, connectors.ErrRateLimit) {
			select {
			case <-time.After(rateLimitDelay):
				continue
			case <-ctx.Done():
				return
			}
		}

		// Temporary / unknown — exponential backoff
		backoff := time.Duration(float64(baseBackoff) * math.Pow(2, float64(attempt)))
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}
	}

	log.Printf("connector %s: send exhausted retries: %v (chat=%s)", w.name, lastErr, msg.ChatID)
	recordOutcome(schema.ExecutionOutcomeFailed, lastErr.Error())
	// Inform the manager so circuit state can trip after repeated failures.
	mgr.onSendResult(w.name, lastErr)
	if mgr.config.SaveErrorRecord != nil {
		contextJSON := fmt.Sprintf(
			`{"connector":"%s","chat_id":"%s","channel":"%s","correlation_id":"%s","source_message_ref":"%s"}`,
			w.name,
			msg.ChatID,
			msg.Channel,
			msg.CorrelationID,
			msg.SourceMessageRef,
		)
		_ = mgr.config.SaveErrorRecord(ctx, "connector", msg.RuntimeSessionID, msg.RunID, "connector_send_failed", lastErr.Error(), contextJSON)
	}
	mgr.Diag.Push(DiagError, w.name, "send exhausted retries", lastErr)
	mgr.Hooks.Fire(ctx, hooks.MessageSendFailed, &hooks.HookEvent{
		Connector: w.name,
		Timestamp: time.Now(),
		Data:      map[string]any{"chat_id": msg.ChatID, "error": lastErr.Error()},
	})
}

func (m *Manager) recordDeliveryAttempt(ctx context.Context, msg connectors.OutboundMessage, status string, attemptDelta int, lastError string) {
	if m == nil || m.config.RecordDeliveryAttempt == nil || strings.TrimSpace(msg.DeliveryID) == "" {
		return
	}
	if err := m.config.RecordDeliveryAttempt(ctx, msg.DeliveryID, status, attemptDelta, lastError); err != nil {
		log.Printf("connector delivery update failed: delivery=%s status=%s err=%v", msg.DeliveryID, status, err)
	}
}

// Enqueue adds a message to the worker's queue. Returns an error if the queue
// is full (backpressure) or the context is cancelled.
func (w *connectorWorker) Enqueue(ctx context.Context, msg connectors.OutboundMessage) error {
	select {
	case w.queue <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("connector %s: queue full (size=%d)", w.name, defaultQueueSize)
	}
}
