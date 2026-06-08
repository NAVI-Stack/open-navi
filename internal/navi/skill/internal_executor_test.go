package skill

import (
	"context"
	"errors"
	"testing"
)

func TestExecuteInternal(t *testing.T) {
	// 1. Success case
	RegisterInternalHandler("test.success", "run", func(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (any, error) {
		return map[string]any{"ok": true, "input": args["val"]}, nil
	})

	entry := &SkillEntry{
		Spec: &OSS27Spec{
			SkillID: "test.success",
		},
	}
	iface := &Interface{
		Name: "run",
		Transport: TransportSpec{
			Type: "internal",
		},
	}

	raw, err := Execute(context.Background(), entry, iface, map[string]any{"val": "hello"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	result := decodeExecutionResult(t, raw)
	if result.Status != "success" {
		t.Fatalf("expected success, got %s", result.Status)
	}
	payload := result.Payload.(map[string]any)
	if payload["input"] != "hello" {
		t.Fatalf("unexpected payload: %v", result.Payload)
	}

	// 2. Error case
	RegisterInternalHandler("test.error", "run", func(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (any, error) {
		return nil, errors.New("something went wrong")
	})
	entry.Spec.SkillID = "test.error"
	raw, err = Execute(context.Background(), entry, iface, nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	result = decodeExecutionResult(t, raw)
	if result.Status != "error" {
		t.Fatalf("expected error, got %s", result.Status)
	}
	if result.Error == nil || result.Error.Message != "something went wrong" {
		t.Fatalf("unexpected error: %v", result.Error)
	}

	// 3. Unknown interface
	entry.Spec.SkillID = "test.unknown"
	raw, err = Execute(context.Background(), entry, iface, nil)
	if !errors.Is(err, ErrNotExecutable) {
		t.Fatalf("expected ErrNotExecutable, got %v", err)
	}
	if err == nil || err.Error() != "skill transport not executable: internal handler not registered for test.unknown/run" {
		t.Fatalf("expected exact missing-handler error, got %v", err)
	}

	// 4. Prefix match (Onboarding)
	entry.Spec.SkillID = "connector.custom"
	iface.Name = "setup"
	raw, err = Execute(context.Background(), entry, iface, map[string]any{"redirect": "cli"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	result = decodeExecutionResult(t, raw)
	if result.Status != "success" {
		t.Fatalf("expected success, got %s", result.Status)
	}
	payload = result.Payload.(map[string]any)
	if payload["status"] != "ok" {
		t.Fatalf("unexpected payload: %v", result.Payload)
	}
}
