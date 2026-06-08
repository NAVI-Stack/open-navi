package store

import (
	"context"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/navi"
)

func TestSQLiteStoreMessageVariantsSelectVisibleContent(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	ctx := context.Background()
	store := NewSQLiteStore(db)
	chat, err := store.CreateChat(ctx, navi.CreateChatInput{Title: "Variants"})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	msgID, err := store.AppendChatMessage(ctx, string(chat.ID), navi.ChatMessage{
		Role:      "assistant",
		Content:   "first answer",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("AppendChatMessage: %v", err)
	}

	groupID, err := store.EnsureMessageVariantGroup(ctx, string(chat.ID), msgID)
	if err != nil {
		t.Fatalf("EnsureMessageVariantGroup: %v", err)
	}
	if groupID == "" {
		t.Fatal("expected variant group id")
	}
	added, err := store.AppendMessageVariant(ctx, string(chat.ID), msgID, "second answer")
	if err != nil {
		t.Fatalf("AppendMessageVariant: %v", err)
	}
	if added.Index != 1 {
		t.Fatalf("new variant index = %d, want 1", added.Index)
	}

	variants, err := store.ListMessageVariants(ctx, string(chat.ID), msgID)
	if err != nil {
		t.Fatalf("ListMessageVariants: %v", err)
	}
	if variants.SelectedIndex != 1 || len(variants.Variants) != 2 {
		t.Fatalf("unexpected variants: %#v", variants)
	}
	if err := store.SelectMessageVariant(ctx, string(chat.ID), msgID, 0); err != nil {
		t.Fatalf("SelectMessageVariant: %v", err)
	}
	thread, err := store.GetChatWithMessages(ctx, string(chat.ID))
	if err != nil {
		t.Fatalf("GetChatWithMessages: %v", err)
	}
	if got := thread.Messages[0].Content; got != "first answer" {
		t.Fatalf("visible content = %q, want first answer", got)
	}
	if got := thread.Messages[0].Metadata["selectedVariantIndex"]; got != float64(0) {
		t.Fatalf("metadata selectedVariantIndex = %#v, want 0", got)
	}
}
