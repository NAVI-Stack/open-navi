package store

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"testing"

	corestore "github.com/open-navi/navi/internal/store"
	_ "modernc.org/sqlite"
)

func tableExistsForTest(t *testing.T, db *sql.DB, name string) bool {
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

func triggerExistsForTest(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var got string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='trigger' AND name = ?`, name).Scan(&got)
	switch err {
	case nil:
		return true
	case sql.ErrNoRows:
		return false
	default:
		t.Fatalf("query sqlite_master for trigger %s: %v", name, err)
		return false
	}
}

// TestMigrateSchema_FreshDB confirms that running MigrateSchema on an empty
// database produces only the new canonical chat/runtime schema. No legacy
// Session tables or mirror triggers should appear.
func TestMigrateSchema_FreshDB(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := corestore.CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	if err := MigrateSchema(ctx, db); err != nil {
		t.Fatalf("MigrateSchema: %v", err)
	}

	for _, table := range []string{
		"navi_chats",
		"navi_chat_messages",
		"navi_chat_memory",
		"navi_chat_memory_checkpoints",
		"runtime_sessions",
		"runtime_session_chats",
		"runtime_runs",
		"runtime_checkpoints",
		"navi_inbox",
	} {
		if !tableExistsForTest(t, db, table) {
			t.Errorf("expected canonical table %q to exist", table)
		}
	}

	for _, table := range []string{
		"navi_sessions",
		"navi_messages",
		"navi_session_memory",
		"navi_session_checkpoints",
	} {
		if tableExistsForTest(t, db, table) {
			t.Errorf("legacy Session table %q must not exist on a fresh DB", table)
		}
	}

	for _, trigger := range []string{
		"trg_navi_sessions_insert_chat",
		"trg_navi_sessions_update_chat",
		"trg_navi_messages_insert_chat_message",
	} {
		if triggerExistsForTest(t, db, trigger) {
			t.Errorf("legacy mirror trigger %q must not exist on a fresh DB", trigger)
		}
	}
}

func seedLegacySessionSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS navi_sessions (
			session_id TEXT PRIMARY KEY,
			title TEXT,
			experience_mode TEXT,
			session_kind TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			closed_at DATETIME,
			archived_at DATETIME,
			project_id TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS navi_messages (
			message_id TEXT PRIMARY KEY,
			session_id TEXT,
			role TEXT,
			content TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS navi_session_memory (
			session_id TEXT PRIMARY KEY,
			schema_version INTEGER NOT NULL DEFAULT 1,
			memory_version INTEGER NOT NULL DEFAULT 0,
			current_epoch_id TEXT NOT NULL DEFAULT '',
			session_frame_json TEXT NOT NULL DEFAULT '{}',
			task_frames_json TEXT NOT NULL DEFAULT '[]',
			retrieval_spans_json TEXT NOT NULL DEFAULT '[]',
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS navi_session_checkpoints (
			checkpoint_id TEXT PRIMARY KEY,
			session_id TEXT,
			epoch_id TEXT,
			trigger_class TEXT,
			compacted_message_start_id TEXT,
			compacted_message_end_id TEXT,
			checkpoint_json TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TRIGGER IF NOT EXISTS trg_navi_sessions_insert_chat
			AFTER INSERT ON navi_sessions BEGIN SELECT 1; END;`,
		`CREATE TRIGGER IF NOT EXISTS trg_navi_sessions_update_chat
			AFTER UPDATE ON navi_sessions BEGIN SELECT 1; END;`,
		`CREATE TRIGGER IF NOT EXISTS trg_navi_messages_insert_chat_message
			AFTER INSERT ON navi_messages BEGIN SELECT 1; END;`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed legacy schema stmt %q: %v", stmt[:40], err)
		}
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO navi_sessions (session_id, experience_mode, session_kind) VALUES ('legacy-1', 'navi', 'user'), ('legacy-2', 'navi', 'user')`,
	); err != nil {
		t.Fatalf("seed legacy session rows: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO navi_messages (message_id, session_id, role, content) VALUES ('m1', 'legacy-1', 'user', 'hello')`,
	); err != nil {
		t.Fatalf("seed legacy message rows: %v", err)
	}
}

// TestMigrateSchema_LegacyDirtyDB confirms that running MigrateSchema on a DB
// containing the legacy Session schema drops the legacy tables and triggers
// without preserving any rows.
func TestMigrateSchema_LegacyDirtyDB(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := corestore.CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	seedLegacySessionSchema(t, db)

	if err := MigrateSchema(ctx, db); err != nil {
		t.Fatalf("MigrateSchema on dirty DB: %v", err)
	}

	for _, table := range []string{
		"navi_sessions",
		"navi_messages",
		"navi_session_memory",
		"navi_session_checkpoints",
	} {
		if tableExistsForTest(t, db, table) {
			t.Errorf("legacy Session table %q must be dropped after migration", table)
		}
	}
	for _, trigger := range []string{
		"trg_navi_sessions_insert_chat",
		"trg_navi_sessions_update_chat",
		"trg_navi_messages_insert_chat_message",
	} {
		if triggerExistsForTest(t, db, trigger) {
			t.Errorf("legacy mirror trigger %q must be dropped after migration", trigger)
		}
	}
}

// TestMigrateSchema_LegacyDirtyDB_LogsWarning confirms that when legacy
// navi_sessions rows are present, a one-time warning is emitted but the
// migration still proceeds and completes.
func TestMigrateSchema_LegacyDirtyDB_LogsWarning(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := corestore.CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	seedLegacySessionSchema(t, db)

	var buf bytes.Buffer
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(original) })

	if err := MigrateSchema(ctx, db); err != nil {
		t.Fatalf("MigrateSchema with warning: %v", err)
	}

	if !strings.Contains(buf.String(), "legacy Session data is being discarded") {
		t.Errorf("expected discard warning in log output, got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), `"row_count":2`) {
		t.Errorf("expected row_count=2 in warning output, got: %s", buf.String())
	}
	if tableExistsForTest(t, db, "navi_sessions") {
		t.Error("legacy navi_sessions table must still be dropped after warning")
	}
}

func TestMigrateSchema_AddsConversationEndpointColumnsBeforeIndexesOnExistingDB(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := corestore.CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	for _, stmt := range []string{
		`CREATE TABLE navi_chats (
			chat_id TEXT PRIMARY KEY,
			owner_id TEXT NOT NULL DEFAULT '',
			project_id TEXT,
			title TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			parent_chat_id TEXT,
			root_chat_id TEXT,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE navi_chat_messages (
			message_id TEXT PRIMARY KEY,
			chat_id TEXT NOT NULL REFERENCES navi_chats(chat_id),
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			run_id TEXT,
			inbox_item_id TEXT
		);`,
		`INSERT INTO navi_chats (chat_id, title) VALUES ('chat-existing', 'Existing Chat');`,
		`INSERT INTO navi_chat_messages (message_id, chat_id, role, content) VALUES ('msg-existing', 'chat-existing', 'user', 'hello');`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed existing chat schema: %v", err)
		}
	}

	if err := MigrateSchema(ctx, db); err != nil {
		t.Fatalf("MigrateSchema existing DB: %v", err)
	}
	if !testTableHasColumn(t, db, "navi_chat_messages", "origin_endpoint_id") {
		t.Fatal("navi_chat_messages.origin_endpoint_id was not added")
	}
	if !testIndexExists(t, db, "idx_navi_chat_messages_origin_endpoint") {
		t.Fatal("idx_navi_chat_messages_origin_endpoint was not created")
	}
}
