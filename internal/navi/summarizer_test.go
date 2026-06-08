package navi

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

func TestChatSummarizerPersistsChatScopedMemoryAndFacts(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "summarizer.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatalf("create tables: %v", err)
	}

	s := &LLMChatSummarizer{
		db: db,
		resolveOwnerID: func(context.Context, string) string {
			return "owner-1"
		},
	}
	if err := s.persistSummary(ctx, "chat-1", 1, ChatSummary{
		TopicSummary: "The chat is about cleanup.",
		FactsLearned: []schema.ExtractedFact{{
			Scope:    "chat",
			Category: "technical_context",
			Key:      "cleanup_scope",
			Value:    "Chat is the durable transcript identity.",
		}},
	}); err != nil {
		t.Fatalf("persist summary: %v", err)
	}

	memories, err := store.ListMemories(ctx, db, "chat", "chat-1", 10)
	if err != nil {
		t.Fatalf("list chat memories: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected one chat-scoped memory, got %d", len(memories))
	}
	if memories[0].Source != "chat_summarizer" {
		t.Fatalf("memory source = %q", memories[0].Source)
	}

	facts, err := store.ListFacts(ctx, db, "chat", "chat-1", false, 10, false)
	if err != nil {
		t.Fatalf("list chat facts: %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected one chat-scoped fact, got %d", len(facts))
	}
	if facts[0].Key != "cleanup_scope" {
		t.Fatalf("fact key = %q", facts[0].Key)
	}
}
