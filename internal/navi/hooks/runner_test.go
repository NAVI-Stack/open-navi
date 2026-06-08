package hooks

import (
	"context"
	"errors"
	"testing"
)

func TestRunner(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()

	var order []int
	r.Register(HookBeforePromptBuild, 10, func(ctx context.Context, payload interface{}) (interface{}, error) {
		order = append(order, 10)
		return payload, nil
	})
	r.Register(HookBeforePromptBuild, 0, func(ctx context.Context, payload interface{}) (interface{}, error) {
		order = append(order, 0)
		return "modified", nil
	})
	r.Register(HookBeforePromptBuild, 5, func(ctx context.Context, payload interface{}) (interface{}, error) {
		order = append(order, 5)
		return payload, nil
	})

	out, err := r.Run(ctx, HookBeforePromptBuild, "input")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out != "modified" {
		t.Errorf("expected modified, got %v", out)
	}
	if len(order) != 3 || order[0] != 0 || order[1] != 5 || order[2] != 10 {
		t.Errorf("expected order [0,5,10], got %v", order)
	}
}

func TestRunnerShortCircuit(t *testing.T) {
	r := NewRunner()
	ctx := context.Background()
	wantErr := errors.New("stop")

	r.Register(HookSessionStart, 0, func(ctx context.Context, payload interface{}) (interface{}, error) {
		return nil, wantErr
	})
	r.Register(HookSessionStart, 1, func(ctx context.Context, payload interface{}) (interface{}, error) {
		t.Fatal("should not be called")
		return nil, nil
	})

	_, err := r.Run(ctx, HookSessionStart, nil)
	if err != wantErr {
		t.Errorf("expected %v, got %v", wantErr, err)
	}
}
