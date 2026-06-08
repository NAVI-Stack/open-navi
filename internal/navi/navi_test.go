package navi

import (
	"context"
	"errors"
	"testing"

	naviruntime "github.com/open-navi/navi/internal/runtime"
)

// TestSendMessageInput_FailsWithoutRuntimeCoordinator verifies that a NAVI with
// no runtime coordinator refuses conversational input rather than panicking or
// silently dropping it.
func TestSendMessageInput_FailsWithoutRuntimeCoordinator(t *testing.T) {
	agent := &NAVI{}
	_, err := agent.SendMessageInput(context.Background(), "chat", naviruntime.MessageInput{Content: "hello"})
	if !errors.Is(err, ErrRuntimeCoordinatorUnavailable) {
		t.Fatalf("expected ErrRuntimeCoordinatorUnavailable, got %v", err)
	}
}

// TestNew_RequiresStores guards New's config validation: it must reject a config
// that lacks the required chat / runtime-session stores instead of constructing
// a half-initialized agent.
func TestNew_RequiresStores(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("expected New(Config{}) to fail when no ChatStore is configured")
	}
}
