package store

import (
	"context"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/navi"
)

func TestDeleteChatMessagesFrom_TruncatesAndRecomputesCounters(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	ctx := context.Background()
	store := NewSQLiteStore(db)

	chat, err := store.CreateChat(ctx, navi.CreateChatInput{Title: "Truncate"})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	id := string(chat.ID)

	base := time.Now().UTC()
	append := func(role, content string, offset time.Duration) string {
		mid, err := store.AppendChatMessage(ctx, id, navi.ChatMessage{
			Role:      role,
			Content:   content,
			CreatedAt: base.Add(offset),
		})
		if err != nil {
			t.Fatalf("AppendChatMessage(%s): %v", content, err)
		}
		return mid
	}

	_ = append("user", "first question", 0)
	firstAssistant := append("assistant", "first answer", time.Second)
	_ = append("user", "second question", 2*time.Second)
	_ = append("assistant", "second answer", 3*time.Second)

	// Truncate from the first assistant reply onward → only the first user message remains.
	if err := store.DeleteChatMessagesFrom(ctx, id, firstAssistant); err != nil {
		t.Fatalf("DeleteChatMessagesFrom: %v", err)
	}

	msgs, err := store.ListChatMessages(ctx, id, 50, 0)
	if err != nil {
		t.Fatalf("ListChatMessages: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Content != "first question" {
		t.Fatalf("expected only the first user message to remain, got %d: %+v", len(msgs), msgs)
	}

	loaded, err := store.GetChat(ctx, id)
	if err != nil {
		t.Fatalf("GetChat: %v", err)
	}
	if loaded.MessageCount != 1 || loaded.UserMessageCount != 1 || loaded.AssistantMessageCount != 0 {
		t.Fatalf("counters not recomputed: total=%d user=%d assistant=%d",
			loaded.MessageCount, loaded.UserMessageCount, loaded.AssistantMessageCount)
	}
}

func TestDeleteChatMessagesFrom_UnknownMessage(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()
	ctx := context.Background()
	store := NewSQLiteStore(db)
	chat, _ := store.CreateChat(ctx, navi.CreateChatInput{Title: "Truncate Errors"})
	if err := store.DeleteChatMessagesFrom(ctx, string(chat.ID), "nope"); err == nil {
		t.Fatal("expected error for unknown message")
	}
}
