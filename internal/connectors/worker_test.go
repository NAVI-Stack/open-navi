package connectors

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/open-navi/navi/connectors"
)

// mockConnector implements connectors.Connector for testing.
type mockConnector struct {
	name      string
	running   atomic.Bool
	startFunc func(ctx context.Context) error
	stopFunc  func(ctx context.Context) error
	sendFunc  func(ctx context.Context, msg connectors.OutboundMessage) error
	mu        sync.Mutex
	sent      []connectors.OutboundMessage
}

func newMockConnector(name string) *mockConnector {
	m := &mockConnector{name: name}
	m.running.Store(true)
	return m
}

func (m *mockConnector) Name() string    { return m.name }
func (m *mockConnector) IsRunning() bool { return m.running.Load() }
func (m *mockConnector) Start(ctx context.Context) error {
	if m.startFunc != nil {
		return m.startFunc(ctx)
	}
	return nil
}
func (m *mockConnector) Stop(ctx context.Context) error {
	m.running.Store(false)
	if m.stopFunc != nil {
		return m.stopFunc(ctx)
	}
	return nil
}
func (m *mockConnector) Send(ctx context.Context, msg connectors.OutboundMessage) error {
	if m.sendFunc != nil {
		return m.sendFunc(ctx, msg)
	}
	m.mu.Lock()
	m.sent = append(m.sent, msg)
	m.mu.Unlock()
	return nil
}

func (m *mockConnector) getSent() []connectors.OutboundMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]connectors.OutboundMessage, len(m.sent))
	copy(out, m.sent)
	return out
}

// --- Worker tests ---

func TestWorker_SendsMessages(t *testing.T) {
	mock := newMockConnector("test")
	mgr := NewManager(NewRegistry(), ManagerConfig{})
	w := newWorker("test", mock, 100, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.runLoop(ctx, mgr)

	msg := connectors.OutboundMessage{ChatID: "chat1", Content: "hello"}
	if err := w.Enqueue(ctx, msg); err != nil {
		t.Fatal(err)
	}
	if err := w.Enqueue(ctx, connectors.OutboundMessage{ChatID: "chat2", Content: "world"}); err != nil {
		t.Fatal(err)
	}

	// Close queue and wait for drain.
	close(w.queue)
	select {
	case <-w.done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not drain in time")
	}

	sent := mock.getSent()
	if len(sent) != 2 {
		t.Fatalf("expected 2 messages sent, got %d", len(sent))
	}
	if sent[0].Content != "hello" {
		t.Errorf("sent[0].Content = %q, want %q", sent[0].Content, "hello")
	}
	if sent[1].Content != "world" {
		t.Errorf("sent[1].Content = %q, want %q", sent[1].Content, "world")
	}
}

func TestWorker_RetryTemporaryError(t *testing.T) {
	attempts := int32(0)
	mock := newMockConnector("test")
	mock.sendFunc = func(_ context.Context, msg connectors.OutboundMessage) error {
		n := atomic.AddInt32(&attempts, 1)
		if n <= 2 {
			return connectors.ErrTemporary
		}
		mock.mu.Lock()
		mock.sent = append(mock.sent, msg)
		mock.mu.Unlock()
		return nil
	}

	mgr := NewManager(NewRegistry(), ManagerConfig{})
	w := newWorker("test", mock, 1000, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.runLoop(ctx, mgr)

	if err := w.Enqueue(ctx, connectors.OutboundMessage{ChatID: "c", Content: "retry-me"}); err != nil {
		t.Fatal(err)
	}

	close(w.queue)
	select {
	case <-w.done:
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not drain in time")
	}

	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
	sent := mock.getSent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 message after retry, got %d", len(sent))
	}
}

func TestWorker_PermanentErrorNoRetry(t *testing.T) {
	attempts := int32(0)
	mock := newMockConnector("test")
	mock.sendFunc = func(_ context.Context, _ connectors.OutboundMessage) error {
		atomic.AddInt32(&attempts, 1)
		return connectors.ErrSendFailed
	}

	mgr := NewManager(NewRegistry(), ManagerConfig{})
	w := newWorker("test", mock, 1000, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.runLoop(ctx, mgr)

	if err := w.Enqueue(ctx, connectors.OutboundMessage{ChatID: "c", Content: "fail"}); err != nil {
		t.Fatal(err)
	}

	close(w.queue)
	select {
	case <-w.done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not drain")
	}

	if n := atomic.LoadInt32(&attempts); n != 1 {
		t.Errorf("expected 1 attempt (permanent), got %d", n)
	}
}

func TestWorker_PermanentErrorRecordsStructuredConnectorFailure(t *testing.T) {
	attempts := int32(0)
	mock := newMockConnector("test")
	mock.sendFunc = func(_ context.Context, _ connectors.OutboundMessage) error {
		atomic.AddInt32(&attempts, 1)
		return connectors.ErrSendFailed
	}

	type recordedError struct {
		component   string
		chatID      string
		runID       string
		errorType   string
		message     string
		contextJSON string
	}
	recordedCh := make(chan recordedError, 1)
	mgr := NewManager(NewRegistry(), ManagerConfig{
		SaveErrorRecord: func(ctx context.Context, component, chatID, runID, errorType, message, contextJSON string) error {
			recordedCh <- recordedError{
				component:   component,
				chatID:      chatID,
				runID:       runID,
				errorType:   errorType,
				message:     message,
				contextJSON: contextJSON,
			}
			return nil
		},
	})
	w := newWorker("test", mock, 1000, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.runLoop(ctx, mgr)

	if err := w.Enqueue(ctx, connectors.OutboundMessage{
		ChatID:           "chat-123",
		Content:          "fail",
		Channel:          "telegram",
		RuntimeSessionID: "sess-connector",
		RunID:            "run-connector",
		CorrelationID:    "corr-connector",
		SourceMessageRef: "telegram:123:456",
	}); err != nil {
		t.Fatal(err)
	}

	close(w.queue)
	select {
	case <-w.done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not drain")
	}

	if n := atomic.LoadInt32(&attempts); n != 1 {
		t.Fatalf("expected 1 attempt (permanent), got %d", n)
	}

	select {
	case recorded := <-recordedCh:
		if recorded.component != "connector" {
			t.Fatalf("component = %q, want connector", recorded.component)
		}
		if recorded.chatID != "sess-connector" || recorded.runID != "run-connector" {
			t.Fatalf("expected session/run correlation, got session=%q run=%q", recorded.chatID, recorded.runID)
		}
		if recorded.errorType != "connector_send_failed" {
			t.Fatalf("errorType = %q, want connector_send_failed", recorded.errorType)
		}
		if !strings.Contains(recorded.message, connectors.ErrSendFailed.Error()) {
			t.Fatalf("message = %q, want to contain %q", recorded.message, connectors.ErrSendFailed.Error())
		}
		if !strings.Contains(recorded.contextJSON, `"chat_id":"chat-123"`) {
			t.Fatalf("contextJSON = %q, expected chat_id", recorded.contextJSON)
		}
		if !strings.Contains(recorded.contextJSON, `"channel":"telegram"`) {
			t.Fatalf("contextJSON = %q, expected channel", recorded.contextJSON)
		}
		if !strings.Contains(recorded.contextJSON, `"correlation_id":"corr-connector"`) {
			t.Fatalf("contextJSON = %q, expected correlation_id", recorded.contextJSON)
		}
		if !strings.Contains(recorded.contextJSON, `"source_message_ref":"telegram:123:456"`) {
			t.Fatalf("contextJSON = %q, expected source_message_ref", recorded.contextJSON)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected connector exhaustion to record a structured error")
	}
}

func TestWorker_QueueFull(t *testing.T) {
	mock := newMockConnector("test")
	// Don't start the worker — queue will fill up.
	w := newWorker("test", mock, 10, nil)

	for i := 0; i < defaultQueueSize; i++ {
		ctx := context.Background()
		if err := w.Enqueue(ctx, connectors.OutboundMessage{ChatID: "c", Content: "msg"}); err != nil {
			t.Fatalf("enqueue %d failed unexpectedly: %v", i, err)
		}
	}

	// Next enqueue should fail
	err := w.Enqueue(context.Background(), connectors.OutboundMessage{ChatID: "c", Content: "overflow"})
	if err == nil {
		t.Fatal("expected error from full queue")
	}
}

// --- Manager tests ---

func TestManager_DispatchAndDrain(t *testing.T) {
	reg := NewRegistry()
	mock := newMockConnector("test")
	reg.mu.Lock()
	reg.instances["test"] = mock
	reg.mu.Unlock()

	mgr := NewManager(reg, ManagerConfig{RateLimits: map[string]float64{"test": 1000}})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.StartAll(ctx)

	// Give worker a moment to start
	time.Sleep(50 * time.Millisecond)

	if err := mgr.Dispatch(ctx, "test", connectors.OutboundMessage{ChatID: "c", Content: "hello"}); err != nil {
		t.Fatal(err)
	}

	mgr.StopAll(context.Background(), 5*time.Second)

	sent := mock.getSent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 message sent, got %d", len(sent))
	}
}

func TestManager_DispatchUnknown(t *testing.T) {
	mgr := NewManager(NewRegistry(), ManagerConfig{})
	err := mgr.Dispatch(context.Background(), "nonexistent", connectors.OutboundMessage{})
	if !errors.Is(err, ErrUnknownConnector) {
		t.Fatalf("expected ErrUnknownConnector, got %v", err)
	}
}

func TestManagerRecordsDeliveryAttemptsAcrossWorkerRetry(t *testing.T) {
	reg := NewRegistry()
	mock := newMockConnector("test")
	var sends int32
	mock.sendFunc = func(_ context.Context, msg connectors.OutboundMessage) error {
		if msg.DeliveryID != "delivery-1" {
			t.Fatalf("DeliveryID = %q, want delivery-1", msg.DeliveryID)
		}
		if atomic.AddInt32(&sends, 1) == 1 {
			return connectors.ErrTemporary
		}
		mock.mu.Lock()
		mock.sent = append(mock.sent, msg)
		mock.mu.Unlock()
		return nil
	}
	reg.mu.Lock()
	reg.instances["test"] = mock
	reg.mu.Unlock()

	type deliveryAttemptRecord struct {
		deliveryID   string
		status       string
		attemptDelta int
		lastError    string
	}
	var (
		mu      sync.Mutex
		records []deliveryAttemptRecord
	)
	mgr := NewManager(reg, ManagerConfig{
		RateLimits: map[string]float64{"test": 1000},
		RecordDeliveryAttempt: func(_ context.Context, deliveryID, status string, attemptDelta int, lastError string) error {
			mu.Lock()
			defer mu.Unlock()
			records = append(records, deliveryAttemptRecord{
				deliveryID:   deliveryID,
				status:       status,
				attemptDelta: attemptDelta,
				lastError:    lastError,
			})
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.StartAll(ctx)
	if err := mgr.Dispatch(ctx, "test", connectors.OutboundMessage{
		ChatID:     "c",
		Content:    "hello",
		DeliveryID: "delivery-1",
	}); err != nil {
		t.Fatal(err)
	}
	mgr.StopAll(context.Background(), 5*time.Second)

	mu.Lock()
	defer mu.Unlock()
	if len(records) != 2 {
		t.Fatalf("records = %#v, want two attempt updates", records)
	}
	if records[0].deliveryID != "delivery-1" || records[0].status != "queued" || records[0].attemptDelta != 1 || records[0].lastError == "" {
		t.Fatalf("first record = %#v, want queued failed attempt", records[0])
	}
	if records[1].deliveryID != "delivery-1" || records[1].status != "sent" || records[1].attemptDelta != 1 || records[1].lastError != "" {
		t.Fatalf("second record = %#v, want sent attempt", records[1])
	}
}

func TestManager_StartFailureRecordsStructuredConnectorError(t *testing.T) {
	reg := NewRegistry()
	mock := newMockConnector("test")
	mock.startFunc = func(ctx context.Context) error {
		return errors.New("connector bootstrap failed")
	}
	reg.mu.Lock()
	reg.instances["test"] = mock
	reg.mu.Unlock()

	type recordedError struct {
		component   string
		errorType   string
		message     string
		contextJSON string
	}
	recordedCh := make(chan recordedError, 1)
	mgr := NewManager(reg, ManagerConfig{
		RateLimits: map[string]float64{"test": 1000},
		SaveErrorRecord: func(ctx context.Context, component, chatID, runID, errorType, message, contextJSON string) error {
			recordedCh <- recordedError{
				component:   component,
				errorType:   errorType,
				message:     message,
				contextJSON: contextJSON,
			}
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.StartAll(ctx)

	select {
	case recorded := <-recordedCh:
		if recorded.component != "connector" {
			t.Fatalf("component = %q, want connector", recorded.component)
		}
		if recorded.errorType != "connector_runtime_failed" {
			t.Fatalf("errorType = %q, want connector_runtime_failed", recorded.errorType)
		}
		if !strings.Contains(recorded.message, "connector bootstrap failed") {
			t.Fatalf("message = %q, want bootstrap failure", recorded.message)
		}
		if !strings.Contains(recorded.contextJSON, `"connector":"test"`) {
			t.Fatalf("contextJSON = %q, expected connector name", recorded.contextJSON)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected start failure to be recorded")
	}
}

func TestManager_Health(t *testing.T) {
	reg := NewRegistry()
	mock := newMockConnector("test")
	reg.mu.Lock()
	reg.instances["test"] = mock
	reg.mu.Unlock()

	mgr := NewManager(reg, ManagerConfig{RateLimits: map[string]float64{"test": 1000}})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.StartAll(ctx)
	time.Sleep(50 * time.Millisecond)

	health := mgr.Health()
	if len(health) != 1 {
		t.Fatalf("expected 1 health entry, got %d", len(health))
	}
	if health[0].Status != "healthy" {
		t.Errorf("expected status=healthy, got %q", health[0].Status)
	}

	mgr.StopAll(context.Background(), 5*time.Second)
}
