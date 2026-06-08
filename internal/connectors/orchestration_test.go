package connectors

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/open-navi/navi/connectors"
)

// orchMock implements Connector + TypingCapable + ReactionCapable +
// PlaceholderCapable + MessageEditor for orchestration tests.
type orchMock struct {
	name    string
	running atomic.Bool

	typingCalls   int32
	reactionCalls int32
	placeholderID string
	editedContent string

	mu   sync.Mutex
	sent []connectors.OutboundMessage
}

func newOrchMock(name string) *orchMock {
	m := &orchMock{name: name}
	m.running.Store(true)
	return m
}

func (m *orchMock) Name() string                  { return m.name }
func (m *orchMock) IsRunning() bool               { return m.running.Load() }
func (m *orchMock) Start(_ context.Context) error { return nil }
func (m *orchMock) Stop(_ context.Context) error  { m.running.Store(false); return nil }
func (m *orchMock) Send(_ context.Context, msg connectors.OutboundMessage) error {
	m.mu.Lock()
	m.sent = append(m.sent, msg)
	m.mu.Unlock()
	return nil
}

// TypingCapable
func (m *orchMock) StartTyping(_ context.Context, _ string) (func(), error) {
	atomic.AddInt32(&m.typingCalls, 1)
	stopped := false
	return func() {
		if !stopped {
			stopped = true
		}
	}, nil
}

// ReactionCapable
func (m *orchMock) ReactToMessage(_ context.Context, _, _ string) (func(), error) {
	atomic.AddInt32(&m.reactionCalls, 1)
	undone := false
	return func() {
		if !undone {
			undone = true
		}
	}, nil
}

// PlaceholderCapable
func (m *orchMock) SendPlaceholder(_ context.Context, _ string) (string, error) {
	return "placeholder-123", nil
}

// MessageEditor
func (m *orchMock) EditMessage(_ context.Context, _, _, content string) error {
	m.mu.Lock()
	m.editedContent = content
	m.mu.Unlock()
	return nil
}

func TestOnInbound_TriggersTypingAndReaction(t *testing.T) {
	reg := NewRegistry()
	mock := newOrchMock("test")
	reg.mu.Lock()
	reg.instances["test"] = mock
	reg.mu.Unlock()

	mgr := NewManager(reg, ManagerConfig{RateLimits: map[string]float64{"test": 1000}})
	ctx := context.Background()
	mgr.StartAll(ctx)
	defer mgr.StopAll(ctx, 2*time.Second)

	mgr.OnInbound(ctx, "test", "chat1", "msg1")

	if atomic.LoadInt32(&mock.typingCalls) != 1 {
		t.Errorf("expected 1 typing call, got %d", mock.typingCalls)
	}
	if atomic.LoadInt32(&mock.reactionCalls) != 1 {
		t.Errorf("expected 1 reaction call, got %d", mock.reactionCalls)
	}
}

func TestPreSend_EditsPlaceholder(t *testing.T) {
	reg := NewRegistry()
	mock := newOrchMock("test")
	reg.mu.Lock()
	reg.instances["test"] = mock
	reg.mu.Unlock()

	mgr := NewManager(reg, ManagerConfig{RateLimits: map[string]float64{"test": 1000}})
	ctx := context.Background()
	mgr.StartAll(ctx)
	defer mgr.StopAll(ctx, 2*time.Second)

	// First trigger OnInbound which stores a placeholder
	mgr.OnInbound(ctx, "test", "chat1", "msg1")

	// Then preSend should edit the placeholder and return true
	edited := mgr.preSend(ctx, "test", "chat1", "actual response", mock)
	if !edited {
		t.Fatal("preSend should return true when placeholder is edited")
	}

	mock.mu.Lock()
	content := mock.editedContent
	mock.mu.Unlock()
	if content != "actual response" {
		t.Errorf("edited content = %q, want %q", content, "actual response")
	}
}

func TestPreSend_NoOpWithoutOnInbound(t *testing.T) {
	reg := NewRegistry()
	mock := newOrchMock("test")
	reg.mu.Lock()
	reg.instances["test"] = mock
	reg.mu.Unlock()

	mgr := NewManager(reg, ManagerConfig{RateLimits: map[string]float64{"test": 1000}})
	ctx := context.Background()
	mgr.StartAll(ctx)
	defer mgr.StopAll(ctx, 2*time.Second)

	// preSend without OnInbound should return false
	edited := mgr.preSend(ctx, "test", "chat1", "hello", mock)
	if edited {
		t.Fatal("preSend should return false without prior OnInbound")
	}
}
