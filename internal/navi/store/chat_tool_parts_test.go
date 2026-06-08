package store

import (
	"context"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/navi"
	naviruntime "github.com/ceoai/navi/internal/runtime"
	"github.com/ceoai/navi/internal/schema"
)

// completeRunWithToolParts is a small helper that drives a chat + run through
// CompleteRun and returns the persisted assistant message metadata.
func completeRunWithToolParts(t *testing.T, parts []naviruntime.ToolInvocationPart) map[string]any {
	t.Helper()
	db := prepareTestDB(t)
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	store := NewSQLiteStore(db)
	chat, err := store.CreateChat(ctx, navi.CreateChatInput{Title: "Tool Parts"})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	runtimeSessionID := string(chat.ID)

	item := &naviruntime.InboxItem{Content: "do the thing", SourceChannel: "app", ReceivedAt: time.Now().UTC()}
	accepted, err := store.AcceptMessage(ctx, runtimeSessionID, item, string(navi.ExperienceModeStandard))
	if err != nil {
		t.Fatalf("AcceptMessage: %v", err)
	}

	run := naviruntime.NewRun(runtimeSessionID, string(navi.ExperienceModeStandard))
	run.SetStatus(schema.RunStatusActive)
	run.ChatID = runtimeSessionID
	run.InitiatedByInboxItemID = accepted.ID
	run.ToolParts = parts
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	msgID, err := store.CompleteRun(ctx, run, "all done", string(navi.ExperienceModeStandard), accepted.ID)
	if err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}

	thread, err := store.GetChatWithMessages(ctx, runtimeSessionID)
	if err != nil {
		t.Fatalf("GetChatWithMessages: %v", err)
	}
	for _, m := range thread.Messages {
		if string(m.ID) == msgID {
			return m.Metadata
		}
	}
	t.Fatalf("assistant message %s not found in thread", msgID)
	return nil
}

func TestCompleteRunPersistsToolPartsMetadata(t *testing.T) {
	meta := completeRunWithToolParts(t, []naviruntime.ToolInvocationPart{
		{ToolInvocationID: "call-1", ToolName: "github.search", State: "result", Result: "found 3 repos"},
		{ToolInvocationID: "call-2", ToolName: "fs.read", State: "result", IsError: true, Result: "Error: not found"},
	})
	if meta == nil {
		t.Fatal("expected message metadata with tool parts")
	}
	raw, ok := meta["toolParts"].([]any)
	if !ok {
		t.Fatalf("expected toolParts array in metadata, got %#v", meta["toolParts"])
	}
	if len(raw) != 2 {
		t.Fatalf("expected 2 persisted tool parts, got %d", len(raw))
	}
	first, ok := raw[0].(map[string]any)
	if !ok {
		t.Fatalf("expected tool part object, got %#v", raw[0])
	}
	if first["toolName"] != "github.search" {
		t.Fatalf("toolName = %v, want github.search", first["toolName"])
	}
	if first["state"] != "result" {
		t.Fatalf("state = %v, want result", first["state"])
	}
	if first["result"] != "found 3 repos" {
		t.Fatalf("result = %v, want 'found 3 repos'", first["result"])
	}
	second, _ := raw[1].(map[string]any)
	if second["isError"] != true {
		t.Fatalf("second part isError = %v, want true", second["isError"])
	}
}

func TestCompleteRunWithoutToolPartsLeavesCleanMetadata(t *testing.T) {
	meta := completeRunWithToolParts(t, nil)
	// A run with no tool calls must not fabricate empty/placeholder chips.
	if _, exists := meta["toolParts"]; exists {
		t.Fatalf("expected no toolParts key for a tool-free run, got %#v", meta["toolParts"])
	}
}
