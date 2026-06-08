package store

import (
	"context"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/navi"
	"github.com/open-navi/navi/internal/navi/compaction"
)

func TestChatMemory_VersionedWrite_RoundTrips(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()
	ctx := context.Background()
	s := NewSQLiteStore(db)
	chat, err := s.CreateChat(ctx, navi.CreateChatInput{Title: "Compaction Round Trip"})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	chatID := string(chat.ID)
	mem := compaction.ChatMemory{
		ChatID:         chatID,
		SchemaVersion:  1,
		MemoryVersion:  3,
		CurrentEpochID: "e1",
		ChatFrame:      compaction.ChatFrame{SchemaVersion: 1, ChatID: chatID, FrameVersion: 2, PrimaryObjective: "ship"},
		TaskFrames:     []compaction.TaskFrame{{TaskID: "t1", Status: "active", NextStep: "n"}},
		RetrievalSpans: []compaction.RetrievalSpan{{SpanID: "s1", Excerpt: "support", Provenance: compaction.SourceSpanProvenance{StartMessageID: "m1", EndMessageID: "m2"}}},
		UpdatedAt:      time.Now().UTC(),
	}
	if err := s.PutChatMemory(ctx, mem); err != nil {
		t.Fatalf("PutChatMemory: %v", err)
	}
	got, err := s.GetChatMemory(ctx, chatID)
	if err != nil {
		t.Fatalf("GetChatMemory: %v", err)
	}
	if got.MemoryVersion != 3 || got.ChatFrame.PrimaryObjective != "ship" {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	if len(got.RetrievalSpans) != 1 || got.RetrievalSpans[0].Provenance.StartMessageID != "m1" {
		t.Fatalf("expected retrieval spans to roundtrip, got %+v", got.RetrievalSpans)
	}
}

func TestRawMessages_RemainQueryableAfterCompaction(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()
	ctx := context.Background()
	s := NewSQLiteStore(db)
	chat, err := s.CreateChat(ctx, navi.CreateChatInput{Title: "Compaction Markers"})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	chatID := string(chat.ID)
	firstID, err := s.AppendChatMessage(ctx, chatID, navi.ChatMessage{Role: "user", Content: "one", CreatedAt: time.Now().UTC().Add(-2 * time.Minute)})
	if err != nil {
		t.Fatalf("AppendChatMessage first: %v", err)
	}
	secondID, err := s.AppendChatMessage(ctx, chatID, navi.ChatMessage{Role: "assistant", Content: "two", CreatedAt: time.Now().UTC().Add(-1 * time.Minute)})
	if err != nil {
		t.Fatalf("AppendChatMessage second: %v", err)
	}
	if err := s.AppendCompactionCheckpoint(ctx, compaction.CompactionCheckpoint{
		CheckpointID:            "cp1",
		ChatID:                  chatID,
		EpochID:                 "e1",
		TriggerClass:            compaction.TriggerHard,
		CompactedMessageStartID: firstID,
		CompactedMessageEndID:   secondID,
		CreatedAt:               time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AppendCompactionCheckpoint: %v", err)
	}
	if err := s.MarkMessagesCompacted(ctx, chatID, "cp1", "e1", firstID, secondID); err != nil {
		t.Fatalf("MarkMessagesCompacted: %v", err)
	}
	thread, err := s.GetChatWithMessages(ctx, chatID)
	if err != nil {
		t.Fatalf("GetChatWithMessages: %v", err)
	}
	if len(thread.Messages) != 2 {
		t.Fatalf("expected raw messages queryable, got %d", len(thread.Messages))
	}
	if thread.Messages[0].CompactedCheckpointID == nil || string(*thread.Messages[0].CompactedCheckpointID) != "cp1" {
		t.Fatalf("expected compaction marker on message, got %#v", thread.Messages[0].CompactedCheckpointID)
	}
}
