package store

import (
	"context"
	"database/sql"
	"testing"
)

func testTableHasColumn(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatalf("pragma table_info(%s): %v", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan pragma row: %v", err)
		}
		if name == column {
			return true
		}
	}
	return false
}

func testIndexExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var got string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='index' AND name=?", name).Scan(&got)
	return err == nil
}

// TestLegacySessionColumnMigration_FreshDB verifies that a brand-new DB created
// by MigrateSchema has no session_id column on the three canonical tables.
func TestLegacySessionColumnMigration_FreshDB(t *testing.T) {
	// prepareTestDB already calls MigrateSchema; just validate the result.
	db := prepareTestDB(t)
	for _, table := range []string{"navi_inbox", "runtime_runs", "runtime_checkpoints"} {
		if testTableHasColumn(t, db, table, "session_id") {
			t.Errorf("fresh DB: table %s still has session_id column", table)
		}
	}
	for _, tc := range []struct {
		table  string
		column string
	}{
		{"navi_inbox", "chat_id"},
		{"navi_inbox", "runtime_session_id"},
		{"runtime_runs", "chat_id"},
		{"runtime_runs", "runtime_session_id"},
		{"runtime_checkpoints", "chat_id"},
		{"runtime_checkpoints", "runtime_session_id"},
	} {
		if !testTableHasColumn(t, db, tc.table, tc.column) {
			t.Errorf("fresh DB: table %s missing %s column", tc.table, tc.column)
		}
	}
	for _, idx := range []string{
		"idx_navi_inbox_chat_status",
		"idx_navi_inbox_chat_idempotency",
		"idx_navi_inbox_runtime_status",
		"idx_navi_inbox_runtime_idempotency",
		"idx_runtime_runs_chat_status",
		"idx_runtime_runs_runtime_status",
		"idx_runtime_session_chats_session",
		"idx_runtime_session_chats_chat",
	} {
		if !testIndexExists(t, db, idx) {
			t.Errorf("fresh DB: expected index %s to exist", idx)
		}
	}
	for _, idx := range []string{
		"idx_navi_inbox_session_status",
		"idx_navi_inbox_idempotency",
		"idx_runtime_runs_session_status",
	} {
		if testIndexExists(t, db, idx) {
			t.Errorf("fresh DB: legacy index %s should not exist", idx)
		}
	}
}

// TestLegacySessionColumnMigration_LegacyDB verifies that an existing DB that
// still has session_id columns (simulated by adding them back after a fresh
// migration) is migrated correctly: values are copied to chat_id, column dropped.
func TestLegacySessionColumnMigration_LegacyDB(t *testing.T) {
	ctx := context.Background()
	// Start from a proper full schema so all MigrateSchema invariants hold.
	db := prepareTestDB(t)

	// Simulate a pre-migration state: add back session_id columns and the old
	// session_id-keyed indexes that dropLegacySessionIDColumns is meant to remove.
	restore := []string{
		`ALTER TABLE navi_inbox ADD COLUMN session_id TEXT`,
		`CREATE INDEX idx_navi_inbox_session_status ON navi_inbox(session_id, status, received_at)`,
		`CREATE UNIQUE INDEX idx_navi_inbox_idempotency ON navi_inbox(session_id, inbox_item_id)`,
		`ALTER TABLE runtime_runs ADD COLUMN session_id TEXT`,
		`CREATE INDEX idx_runtime_runs_session_status ON runtime_runs(session_id, status, updated_at DESC)`,
		`ALTER TABLE runtime_checkpoints ADD COLUMN session_id TEXT`,
	}
	for _, s := range restore {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("restore legacy column: %v", err)
		}
	}

	// Insert rows with session_id set and chat_id NULL.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO navi_inbox (inbox_item_id, session_id, runtime_session_id, source_channel, received_at)
		 VALUES ('itm-1','chat-abc','rs-1','test',CURRENT_TIMESTAMP)`,
	); err != nil {
		t.Fatalf("insert inbox row: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO runtime_runs (run_id, session_id, runtime_session_id, experience_mode, mode, status, current_phase, started_at, updated_at)
		 VALUES ('run-1','chat-abc','rs-1','navi','chat','active','idle',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
	); err != nil {
		t.Fatalf("insert run row: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO runtime_checkpoints (checkpoint_id, run_id, session_id, runtime_session_id, phase, experience_mode, created_at)
		 VALUES ('cp-1','run-1','chat-abc','rs-1','idle','navi',CURRENT_TIMESTAMP)`,
	); err != nil {
		t.Fatalf("insert checkpoint row: %v", err)
	}

	// Run migration — dropLegacySessionIDColumns should fire.
	if err := MigrateSchema(ctx, db); err != nil {
		t.Fatalf("MigrateSchema: %v", err)
	}

	// session_id must be gone from all three tables.
	for _, table := range []string{"navi_inbox", "runtime_runs", "runtime_checkpoints"} {
		if testTableHasColumn(t, db, table, "session_id") {
			t.Errorf("after migration: %s still has session_id column", table)
		}
	}

	// chat_id must have been populated from the former session_id value.
	for _, tc := range []struct {
		query string
		label string
	}{
		{`SELECT chat_id FROM navi_inbox WHERE inbox_item_id = 'itm-1'`, "navi_inbox"},
		{`SELECT chat_id FROM runtime_runs WHERE run_id = 'run-1'`, "runtime_runs"},
		{`SELECT chat_id FROM runtime_checkpoints WHERE checkpoint_id = 'cp-1'`, "runtime_checkpoints"},
	} {
		var chatID string
		if err := db.QueryRowContext(ctx, tc.query).Scan(&chatID); err != nil {
			t.Fatalf("read chat_id from %s: %v", tc.label, err)
		}
		if chatID != "chat-abc" {
			t.Errorf("%s: chat_id = %q; want chat-abc", tc.label, chatID)
		}
	}

	// Replacement indexes must exist.
	for _, idx := range []string{"idx_navi_inbox_chat_status", "idx_navi_inbox_chat_idempotency", "idx_runtime_runs_chat_status"} {
		if !testIndexExists(t, db, idx) {
			t.Errorf("after migration: expected index %s", idx)
		}
	}
}

// TestLegacySessionColumnMigration_Idempotent verifies that running MigrateSchema
// twice on an already-migrated DB is a no-op.
func TestLegacySessionColumnMigration_Idempotent(t *testing.T) {
	ctx := context.Background()
	db := prepareTestDB(t)
	if err := MigrateSchema(ctx, db); err != nil {
		t.Fatalf("second MigrateSchema: %v", err)
	}
}
