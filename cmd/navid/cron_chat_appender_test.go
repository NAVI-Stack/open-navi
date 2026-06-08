package main

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	cpkg "github.com/open-navi/navi/internal/cron"
	"github.com/open-navi/navi/internal/navi"
	navistore "github.com/open-navi/navi/internal/navi/store"
	corestore "github.com/open-navi/navi/internal/store"
	_ "modernc.org/sqlite"
)

func testCronChatAppenderStore(t *testing.T) (*sql.DB, *navistore.SQLiteStore) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	if err := corestore.CreateTables(context.Background(), db); err != nil {
		db.Close()
		t.Fatalf("create core tables: %v", err)
	}
	if err := navistore.MigrateSchema(context.Background(), db); err != nil {
		db.Close()
		t.Fatalf("migrate navi schema: %v", err)
	}
	return db, navistore.NewSQLiteStore(db)
}

func TestCronChatAppenderWritesOnlyExistingChatTranscript(t *testing.T) {
	db, store := testCronChatAppenderStore(t)
	defer db.Close()

	ctx := context.Background()
	chat, err := store.CreateChat(ctx, navi.CreateChatInput{Title: "Scheduled reply"})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	appender := cronChatAppender{store: store}

	if err := appender.AppendAssistantMessage(ctx, string(chat.ID), "hello later"); err != nil {
		t.Fatalf("AppendAssistantMessage existing chat: %v", err)
	}

	var content, sourceChannel, messageKind string
	if err := db.QueryRowContext(ctx, `
		SELECT content, source_channel, message_kind
		FROM navi_chat_messages
		WHERE chat_id = ?
	`, string(chat.ID)).Scan(&content, &sourceChannel, &messageKind); err != nil {
		t.Fatalf("read appended chat message: %v", err)
	}
	if content != "hello later" || sourceChannel != "scheduler" || messageKind != "reply" {
		t.Fatalf("unexpected cron chat message content=%q source=%q kind=%q", content, sourceChannel, messageKind)
	}
	if tableExistsForNavidTest(t, db, "navi_messages") {
		t.Fatal("legacy navi_messages table should not exist or receive cron writes")
	}
}

func TestCronChatAppenderRejectsMissingChatWithoutFabricatingTranscript(t *testing.T) {
	db, store := testCronChatAppenderStore(t)
	defer db.Close()

	ctx := context.Background()
	appender := cronChatAppender{store: store}

	err := appender.AppendAssistantMessage(ctx, "missing-chat", "do not write")
	if !errors.Is(err, cpkg.ErrNoChatTarget) {
		t.Fatalf("expected ErrNoChatTarget for missing chat, got %v", err)
	}

	var chatCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM navi_chats WHERE chat_id = 'missing-chat'`).Scan(&chatCount); err != nil {
		t.Fatalf("count fabricated chats: %v", err)
	}
	if chatCount != 0 {
		t.Fatalf("missing chat target fabricated %d navi_chats rows", chatCount)
	}
	var messageCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM navi_chat_messages WHERE chat_id = 'missing-chat'`).Scan(&messageCount); err != nil {
		t.Fatalf("count fabricated chat messages: %v", err)
	}
	if messageCount != 0 {
		t.Fatalf("missing chat target wrote %d navi_chat_messages rows", messageCount)
	}
}

func tableExistsForNavidTest(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var got string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name = ?`, name).Scan(&got)
	switch err {
	case nil:
		return true
	case sql.ErrNoRows:
		return false
	default:
		t.Fatalf("query sqlite_master for table %s: %v", name, err)
		return false
	}
}
