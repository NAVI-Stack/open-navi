package hooks

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestFire_PriorityOrder(t *testing.T) {
	r := NewRegistry()
	var order []string

	r.Register(MessageSent, "b", func(_ context.Context, _ any) error {
		order = append(order, "b")
		return nil
	}, 200)

	r.Register(MessageSent, "a", func(_ context.Context, _ any) error {
		order = append(order, "a")
		return nil
	}, 100)

	r.Register(MessageSent, "c", func(_ context.Context, _ any) error {
		order = append(order, "c")
		return nil
	}, 300)

	r.Fire(context.Background(), MessageSent, &HookEvent{Connector: "test", Timestamp: time.Now()})

	if len(order) != 3 || order[0] != "a" || order[1] != "b" || order[2] != "c" {
		t.Errorf("expected [a b c], got %v", order)
	}
}

func TestFire_MessageSending_Cancel(t *testing.T) {
	r := NewRegistry()
	called := false

	r.Register(MessageSending, "canceller", func(_ context.Context, event any) error {
		e := event.(*MessageSendingEvent)
		e.Cancel = true
		return nil
	}, 100)

	r.Register(MessageSending, "should-not-fire", func(_ context.Context, _ any) error {
		called = true
		return nil
	}, 200)

	ev := &MessageSendingEvent{
		HookEvent: HookEvent{Connector: "test", Timestamp: time.Now()},
		ChatID:    "c1",
		Content:   "hello",
	}
	r.Fire(context.Background(), MessageSending, ev)

	if !ev.Cancel {
		t.Error("expected Cancel=true")
	}
	if called {
		t.Error("second handler should not have been called after cancel")
	}
}

func TestFire_MessageSending_MutateContent(t *testing.T) {
	r := NewRegistry()

	r.Register(MessageSending, "redactor", func(_ context.Context, event any) error {
		e := event.(*MessageSendingEvent)
		e.Content = "[REDACTED]"
		return nil
	}, 100)

	ev := &MessageSendingEvent{
		HookEvent: HookEvent{Connector: "test", Timestamp: time.Now()},
		Content:   "secret data",
	}
	r.Fire(context.Background(), MessageSending, ev)

	if ev.Content != "[REDACTED]" {
		t.Errorf("content = %q, want [REDACTED]", ev.Content)
	}
}

func TestFire_ReadOnlyHook_ErrorsLogged_ContinueFiring(t *testing.T) {
	r := NewRegistry()
	secondCalled := false

	r.Register(MessageSent, "fails", func(_ context.Context, _ any) error {
		return fmt.Errorf("oops")
	}, 100)

	r.Register(MessageSent, "still-fires", func(_ context.Context, _ any) error {
		secondCalled = true
		return nil
	}, 200)

	r.Fire(context.Background(), MessageSent, &HookEvent{Connector: "test", Timestamp: time.Now()})

	if !secondCalled {
		t.Error("second handler should fire even after first handler error on read-only hook")
	}
}

func TestFire_EmptyRegistry(t *testing.T) {
	r := NewRegistry()
	err := r.Fire(context.Background(), MessageSent, &HookEvent{})
	if err != nil {
		t.Errorf("expected nil error for empty registry, got %v", err)
	}
}

func TestCount(t *testing.T) {
	r := NewRegistry()
	if r.Count(MessageSent) != 0 {
		t.Error("expected 0 count initially")
	}
	r.Register(MessageSent, "a", func(_ context.Context, _ any) error { return nil }, 100)
	if r.Count(MessageSent) != 1 {
		t.Error("expected 1 count after register")
	}
}
