package navi_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ceoai/navi/internal/bus"
	"github.com/ceoai/navi/internal/navi"
	navistore "github.com/ceoai/navi/internal/navi/store"
	naviruntime "github.com/ceoai/navi/internal/runtime"
	corestore "github.com/ceoai/navi/internal/store"
)

func newTestNAVIForChatActions(t *testing.T) (*navi.NAVI, *navistore.SQLiteStore) {
	t.Helper()
	db, err := corestore.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := corestore.CreateTables(context.Background(), db); err != nil {
		t.Fatalf("create core tables: %v", err)
	}
	if err := navistore.MigrateSchema(context.Background(), db); err != nil {
		t.Fatalf("migrate navi schema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	store := navistore.NewSQLiteStore(db)
	n, err := navi.New(navi.Config{
		DB:              db,
		Bus:             bus.NewMemBus(db),
		Chats:           store,
		RuntimeSessions: store,
	})
	if err != nil {
		t.Fatalf("New NAVI: %v", err)
	}
	return n, store
}

func TestContinueLastReplyQueuesRuntimeSignalWithoutTranscriptUserMessage(t *testing.T) {
	ctx := context.Background()
	n, store := newTestNAVIForChatActions(t)
	chatID, err := n.CreateChat(ctx, navi.ExperienceModeStandard, "")
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	if _, err := n.SendMessageInput(ctx, chatID, newTestMessageInput("Explain the plan")); err != nil {
		t.Fatalf("SendMessageInput: %v", err)
	}
	assistantID, err := store.AppendSystemAssistantMessage(ctx, chatID, "First half of the answer.", string(navi.ExperienceModeStandard), "system", "reply")
	if err != nil {
		t.Fatalf("AppendSystemAssistantMessage: %v", err)
	}

	item, err := n.ContinueLastReply(ctx, chatID)
	if err != nil {
		t.Fatalf("ContinueLastReply: %v", err)
	}
	if item == nil || item.PayloadType != "chat_continuation" {
		t.Fatalf("expected chat_continuation signal, got %#v", item)
	}
	var payload map[string]string
	if err := json.Unmarshal(item.Structured, &payload); err != nil {
		t.Fatalf("decode structured payload: %v", err)
	}
	if payload["target_message_id"] != assistantID {
		t.Fatalf("target_message_id = %q, want %q", payload["target_message_id"], assistantID)
	}
	thread, err := store.GetChatWithMessages(ctx, chatID)
	if err != nil {
		t.Fatalf("GetChatWithMessages: %v", err)
	}
	if len(thread.Messages) != 2 {
		t.Fatalf("continue should not add a user transcript row, got %d messages", len(thread.Messages))
	}
}

func TestRegenerateLastReplyPreservesPriorAssistantAsVariant(t *testing.T) {
	ctx := context.Background()
	n, store := newTestNAVIForChatActions(t)
	chatID, err := n.CreateChat(ctx, navi.ExperienceModeStandard, "")
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	if _, err := n.SendMessageInput(ctx, chatID, newTestMessageInput("Draft a greeting")); err != nil {
		t.Fatalf("SendMessageInput: %v", err)
	}
	assistantID, err := store.AppendSystemAssistantMessage(ctx, chatID, "Hello there.", string(navi.ExperienceModeStandard), "system", "reply")
	if err != nil {
		t.Fatalf("AppendSystemAssistantMessage: %v", err)
	}

	item, err := n.RegenerateLastReply(ctx, chatID)
	if err != nil {
		t.Fatalf("RegenerateLastReply: %v", err)
	}
	if item == nil || item.PayloadType != "chat_regenerate_variant" {
		t.Fatalf("expected chat_regenerate_variant signal, got %#v", item)
	}
	variants, err := store.ListMessageVariants(ctx, chatID, assistantID)
	if err != nil {
		t.Fatalf("ListMessageVariants: %v", err)
	}
	if variants.SelectedIndex != 0 || len(variants.Variants) != 1 {
		t.Fatalf("expected original selected variant, got %#v", variants)
	}
	if variants.Variants[0].Content != "Hello there." {
		t.Fatalf("variant content = %q", variants.Variants[0].Content)
	}
	thread, err := store.GetChatWithMessages(ctx, chatID)
	if err != nil {
		t.Fatalf("GetChatWithMessages: %v", err)
	}
	if len(thread.Messages) != 2 {
		t.Fatalf("regenerate should preserve the visible assistant message, got %d messages", len(thread.Messages))
	}
}

func newTestMessageInput(content string) naviruntime.MessageInput {
	return naviruntime.MessageInput{Content: content, SourceChannel: "app"}
}
