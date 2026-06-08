package store

import (
	"context"
	"testing"

	"github.com/ceoai/navi/internal/navi"
)

func TestSetChatMessageFeedback_PersistsAndClears(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	ctx := context.Background()
	store := NewSQLiteStore(db)

	chat, err := store.CreateChat(ctx, navi.CreateChatInput{Title: "Feedback"})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	msgID, err := store.AppendChatMessage(ctx, string(chat.ID), navi.ChatMessage{
		Role:    "assistant",
		Content: "here is your answer",
	})
	if err != nil {
		t.Fatalf("AppendChatMessage: %v", err)
	}

	feedbackOf := func() any {
		thread, err := store.GetChatWithMessages(ctx, string(chat.ID))
		if err != nil {
			t.Fatalf("GetChatWithMessages: %v", err)
		}
		for _, m := range thread.Messages {
			if string(m.ID) == msgID {
				if m.Metadata == nil {
					return nil
				}
				return m.Metadata["feedback"]
			}
		}
		t.Fatalf("message %s not found in thread", msgID)
		return nil
	}

	// Set thumbs up.
	if err := store.SetChatMessageFeedback(ctx, string(chat.ID), msgID, "up"); err != nil {
		t.Fatalf("SetChatMessageFeedback up: %v", err)
	}
	if got := feedbackOf(); got != "up" {
		t.Fatalf("expected feedback 'up', got %v", got)
	}

	// Flip to thumbs down.
	if err := store.SetChatMessageFeedback(ctx, string(chat.ID), msgID, "down"); err != nil {
		t.Fatalf("SetChatMessageFeedback down: %v", err)
	}
	if got := feedbackOf(); got != "down" {
		t.Fatalf("expected feedback 'down', got %v", got)
	}

	// Clear.
	if err := store.SetChatMessageFeedback(ctx, string(chat.ID), msgID, ""); err != nil {
		t.Fatalf("SetChatMessageFeedback clear: %v", err)
	}
	if got := feedbackOf(); got != nil {
		t.Fatalf("expected feedback cleared, got %v", got)
	}
}

func TestSetChatMessageFeedback_Errors(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	ctx := context.Background()
	store := NewSQLiteStore(db)
	chat, err := store.CreateChat(ctx, navi.CreateChatInput{Title: "Feedback Errors"})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}

	if err := store.SetChatMessageFeedback(ctx, string(chat.ID), "does-not-exist", "up"); err == nil {
		t.Fatal("expected error for unknown message")
	}

	msgID, _ := store.AppendChatMessage(ctx, string(chat.ID), navi.ChatMessage{Role: "assistant", Content: "x"})
	if err := store.SetChatMessageFeedback(ctx, string(chat.ID), msgID, "sideways"); err == nil {
		t.Fatal("expected error for invalid rating")
	}
}
