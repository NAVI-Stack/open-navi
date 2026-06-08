package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/ceoai/navi/internal/navi"
	naviruntime "github.com/ceoai/navi/internal/runtime"
	corestore "github.com/ceoai/navi/internal/store"
)

func TestMigrateSchemaCreatesConversationEndpointTables(t *testing.T) {
	ctx := context.Background()
	db, err := corestore.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := corestore.CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	if err := MigrateSchema(ctx, db); err != nil {
		t.Fatalf("MigrateSchema: %v", err)
	}

	for _, table := range []string{
		"navi_conversation_endpoints",
		"navi_chat_delivery_policy",
		"navi_message_deliveries",
	} {
		if !conversationEndpointTestTableExists(t, db, table) {
			t.Fatalf("expected table %s to exist", table)
		}
	}

	for _, check := range []struct {
		table  string
		column string
	}{
		{"navi_chat_messages", "origin_endpoint_id"},
		{"navi_inbox", "origin_endpoint_id"},
		{"runtime_runs", "origin_endpoint_id"},
	} {
		if !conversationEndpointTestColumnExists(t, db, check.table, check.column) {
			t.Fatalf("expected %s.%s to exist", check.table, check.column)
		}
	}
}

func TestMessageDeliveriesUniqueByMessageAndEndpoint(t *testing.T) {
	ctx := context.Background()
	db := prepareTestDB(t)

	_, err := db.ExecContext(ctx, `
		INSERT INTO navi_message_deliveries (
			delivery_id, message_id, chat_id, endpoint_id, status, created_at, updated_at
		) VALUES
			('d1', 'm1', 'c1', 'e1', 'queued', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
			('d2', 'm1', 'c1', 'e1', 'queued', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err == nil {
		t.Fatal("expected duplicate message/endpoint delivery insert to fail")
	}
}

func TestStorePersistsOriginEndpointIDs(t *testing.T) {
	ctx := context.Background()
	db := prepareTestDB(t)
	store := NewSQLiteStore(db)

	chat, err := store.CreateChat(ctx, navi.CreateChatInput{ID: "chat-origin", Title: "Origin Chat"})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}

	origin := navi.ID("endpoint-telegram-1")
	messageID, err := store.AppendChatMessage(ctx, string(chat.ID), navi.ChatMessage{
		Role:             "user",
		Content:          "hello",
		OriginEndpointID: &origin,
	})
	if err != nil {
		t.Fatalf("AppendChatMessage: %v", err)
	}
	messages, err := store.ListChatMessages(ctx, string(chat.ID), 10, 0)
	if err != nil {
		t.Fatalf("ListChatMessages: %v", err)
	}
	if len(messages) != 1 || messages[0].ID != navi.ID(messageID) {
		t.Fatalf("expected appended message, got %#v", messages)
	}
	if messages[0].OriginEndpointID == nil || *messages[0].OriginEndpointID != origin {
		t.Fatalf("expected origin endpoint %q, got %#v", origin, messages[0].OriginEndpointID)
	}

	item := &naviruntime.InboxItem{
		ID:               "inbox-origin-1",
		RuntimeSessionID: "session-origin-1",
		ChatID:           string(chat.ID),
		SourceChannel:    "telegram",
		Content:          "inbox hello",
		OriginEndpointID: string(origin),
	}
	accepted, err := store.AcceptMessage(ctx, item.RuntimeSessionID, item, string(navi.ExperienceModeStandard))
	if err != nil {
		t.Fatalf("AcceptMessage: %v", err)
	}
	if accepted.OriginEndpointID != string(origin) {
		t.Fatalf("accepted inbox origin endpoint = %q, want %q", accepted.OriginEndpointID, origin)
	}
	pending, err := store.ListPendingItems(ctx, item.RuntimeSessionID, 10)
	if err != nil {
		t.Fatalf("ListPendingItems: %v", err)
	}
	if len(pending) != 1 || pending[0].OriginEndpointID != string(origin) {
		t.Fatalf("pending inbox origin endpoint mismatch: %#v", pending)
	}

	run := naviruntime.NewRun(item.RuntimeSessionID, string(navi.ExperienceModeStandard))
	run.RunID = "run-origin-1"
	run.ChatID = string(chat.ID)
	run.OriginEndpointID = string(origin)
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	loaded, err := store.GetRun(ctx, run.RunID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if loaded == nil || loaded.OriginEndpointID != string(origin) {
		t.Fatalf("loaded run origin endpoint = %#v, want %q", loaded, origin)
	}
}

func conversationEndpointTestTableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var name string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
	return err == nil
}

func conversationEndpointTestColumnExists(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info("` + table + `")`)
	if err != nil {
		t.Fatalf("table_info %s: %v", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		if name == column {
			return true
		}
	}
	return false
}
