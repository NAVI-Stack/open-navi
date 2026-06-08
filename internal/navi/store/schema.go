package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
)

// createNaviTables defines the SQL schema for NAVI's persistent conversational state.
var createNaviTables = []string{
	`CREATE TABLE IF NOT EXISTS navi_chats (
		chat_id                  TEXT PRIMARY KEY,
		owner_id                 TEXT NOT NULL DEFAULT '',
		workspace_id             TEXT,
		project_id               TEXT,
		organization_id          TEXT,
		title                    TEXT NOT NULL DEFAULT '',
		description              TEXT NOT NULL DEFAULT '',
		icon                     TEXT NOT NULL DEFAULT '',
		color                    TEXT NOT NULL DEFAULT '',
		status                   TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'archived', 'deleted')),
		visibility               TEXT NOT NULL DEFAULT 'private' CHECK(visibility IN ('private', 'shared', 'public')),
		is_pinned                INTEGER NOT NULL DEFAULT 0 CHECK(is_pinned IN (0, 1)),
		is_favorite              INTEGER NOT NULL DEFAULT 0 CHECK(is_favorite IN (0, 1)),
		created_at               DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at               DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_message_at          DATETIME,
		archived_at              DATETIME,
		deleted_at               DATETIME,
		root_chat_id             TEXT,
		parent_chat_id           TEXT,
		branched_from_message_id TEXT,
		branch_reason            TEXT NOT NULL DEFAULT '',
		ai_config_json           TEXT NOT NULL DEFAULT '{}',
		memory_policy_json       TEXT NOT NULL DEFAULT '{}',
		context_sources_json     TEXT NOT NULL DEFAULT '[]',
		summary                  TEXT NOT NULL DEFAULT '',
		short_summary            TEXT NOT NULL DEFAULT '',
		topics_json              TEXT NOT NULL DEFAULT '[]',
		tags_json                TEXT NOT NULL DEFAULT '[]',
		intent                   TEXT NOT NULL DEFAULT '',
		sentiment                TEXT NOT NULL DEFAULT '',
		language                 TEXT NOT NULL DEFAULT '',
		linked_artifacts_json    TEXT NOT NULL DEFAULT '[]',
		linked_files_json        TEXT NOT NULL DEFAULT '[]',
		linked_projects_json     TEXT NOT NULL DEFAULT '[]',
		linked_tasks_json        TEXT NOT NULL DEFAULT '[]',
		message_count            INTEGER NOT NULL DEFAULT 0,
		user_message_count       INTEGER NOT NULL DEFAULT 0,
		assistant_message_count  INTEGER NOT NULL DEFAULT 0,
		token_usage_json         TEXT NOT NULL DEFAULT '{}',
		moderation_state         TEXT NOT NULL DEFAULT 'clean' CHECK(moderation_state IN ('clean', 'flagged', 'restricted', 'needs_review')),
		moderation_flags_json    TEXT NOT NULL DEFAULT '[]',
		metadata_json            TEXT NOT NULL DEFAULT '{}'
	);`,
	`CREATE INDEX IF NOT EXISTS idx_navi_chats_owner_updated ON navi_chats(owner_id, updated_at DESC);`,
	`CREATE INDEX IF NOT EXISTS idx_navi_chats_project_updated ON navi_chats(project_id, updated_at DESC);`,
	`CREATE INDEX IF NOT EXISTS idx_navi_chats_status_updated ON navi_chats(status, updated_at DESC);`,
	`CREATE INDEX IF NOT EXISTS idx_navi_chats_parent ON navi_chats(parent_chat_id);`,
	`CREATE INDEX IF NOT EXISTS idx_navi_chats_root ON navi_chats(root_chat_id);`,

	`CREATE TABLE IF NOT EXISTS navi_chat_messages (
		message_id              TEXT PRIMARY KEY,
		chat_id                 TEXT NOT NULL REFERENCES navi_chats(chat_id),
		role                    TEXT NOT NULL,
		content                 TEXT NOT NULL,
		created_at              DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		runtime_session_id      TEXT,
		run_id                  TEXT,
		inbox_item_id           TEXT,
		source_channel          TEXT,
		source_message_ref      TEXT,
		origin_endpoint_id      TEXT,
		message_kind            TEXT,
		compacted_at            DATETIME,
		compacted_checkpoint_id TEXT,
		compacted_epoch_id      TEXT,
		metadata_json           TEXT NOT NULL DEFAULT '{}'
	);`,
	`CREATE INDEX IF NOT EXISTS idx_navi_chat_messages_chat_created ON navi_chat_messages(chat_id, created_at DESC);`,
	`CREATE INDEX IF NOT EXISTS idx_navi_chat_messages_run ON navi_chat_messages(run_id);`,
	`CREATE INDEX IF NOT EXISTS idx_navi_chat_messages_inbox ON navi_chat_messages(inbox_item_id);`,

	`CREATE TABLE IF NOT EXISTS navi_conversation_endpoints (
	    endpoint_id           TEXT PRIMARY KEY,
	    chat_id               TEXT NOT NULL REFERENCES navi_chats(chat_id) ON DELETE CASCADE,
	    endpoint_type         TEXT NOT NULL CHECK(endpoint_type IN ('console', 'connector')),
	    connector_kind        TEXT NOT NULL DEFAULT '',
	    connector_instance_id TEXT NOT NULL DEFAULT '',
	    external_chat_id      TEXT NOT NULL DEFAULT '',
	    external_thread_id    TEXT NOT NULL DEFAULT '',
	    display_name          TEXT NOT NULL DEFAULT '',
	    receive_enabled       INTEGER NOT NULL DEFAULT 1 CHECK(receive_enabled IN (0, 1)),
	    send_enabled          INTEGER NOT NULL DEFAULT 1 CHECK(send_enabled IN (0, 1)),
	    mirror_enabled        INTEGER NOT NULL DEFAULT 0 CHECK(mirror_enabled IN (0, 1)),
	    status                TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'disabled', 'deleted')),
	    created_at            DATETIME NOT NULL,
	    updated_at            DATETIME NOT NULL,
	    metadata_json         TEXT NOT NULL DEFAULT '{}'
	);`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_navi_conversation_endpoints_console_active
	    ON navi_conversation_endpoints(chat_id)
	    WHERE endpoint_type = 'console' AND status = 'active';`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_navi_conversation_endpoints_connector_active
	    ON navi_conversation_endpoints(connector_instance_id, external_chat_id, external_thread_id)
	    WHERE endpoint_type = 'connector' AND status = 'active';`,
	`CREATE INDEX IF NOT EXISTS idx_navi_conversation_endpoints_chat
	    ON navi_conversation_endpoints(chat_id, status, endpoint_type);`,
	`CREATE INDEX IF NOT EXISTS idx_navi_conversation_endpoints_external
	    ON navi_conversation_endpoints(connector_instance_id, external_chat_id, external_thread_id);`,

	`CREATE TABLE IF NOT EXISTS navi_chat_delivery_policy (
	    chat_id       TEXT PRIMARY KEY REFERENCES navi_chats(chat_id) ON DELETE CASCADE,
	    default_mode  TEXT NOT NULL DEFAULT 'reply_to_origin' CHECK(default_mode IN ('reply_to_origin', 'explicit_only')),
	    created_at    DATETIME NOT NULL,
	    updated_at    DATETIME NOT NULL,
	    metadata_json TEXT NOT NULL DEFAULT '{}'
	);`,

	`CREATE TABLE IF NOT EXISTS navi_message_deliveries (
	    delivery_id           TEXT PRIMARY KEY,
	    message_id            TEXT NOT NULL,
	    chat_id               TEXT NOT NULL,
	    endpoint_id           TEXT NOT NULL,
	    connector_instance_id TEXT NOT NULL DEFAULT '',
	    status                TEXT NOT NULL CHECK(status IN ('queued', 'sent', 'failed', 'skipped')),
	    attempt_count         INTEGER NOT NULL DEFAULT 0,
	    last_error            TEXT NOT NULL DEFAULT '',
	    created_at            DATETIME NOT NULL,
	    updated_at            DATETIME NOT NULL,
	    metadata_json         TEXT NOT NULL DEFAULT '{}',
	    UNIQUE(message_id, endpoint_id)
	);`,
	`CREATE INDEX IF NOT EXISTS idx_navi_message_deliveries_message
	    ON navi_message_deliveries(message_id);`,
	`CREATE INDEX IF NOT EXISTS idx_navi_message_deliveries_chat
	    ON navi_message_deliveries(chat_id, created_at DESC);`,
	`CREATE INDEX IF NOT EXISTS idx_navi_message_deliveries_endpoint
	    ON navi_message_deliveries(endpoint_id, status);`,

	`CREATE TABLE IF NOT EXISTS navi_chat_message_variants (
		variant_id       TEXT PRIMARY KEY,
		chat_id          TEXT NOT NULL REFERENCES navi_chats(chat_id),
		message_id       TEXT NOT NULL REFERENCES navi_chat_messages(message_id),
		variant_group_id TEXT NOT NULL,
		variant_index    INTEGER NOT NULL,
		content          TEXT NOT NULL,
		selected         INTEGER NOT NULL DEFAULT 0 CHECK(selected IN (0, 1)),
		created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(variant_group_id, variant_index)
	);`,
	`CREATE INDEX IF NOT EXISTS idx_navi_chat_message_variants_message
	    ON navi_chat_message_variants(chat_id, message_id, variant_index);`,
	`CREATE INDEX IF NOT EXISTS idx_navi_chat_message_variants_group
	    ON navi_chat_message_variants(variant_group_id, variant_index);`,

	`CREATE TABLE IF NOT EXISTS navi_chat_memory (
	    chat_id TEXT PRIMARY KEY REFERENCES navi_chats(chat_id),
	    schema_version INTEGER NOT NULL,
	    memory_version INTEGER NOT NULL,
	    current_epoch_id TEXT NOT NULL,
	    chat_frame_json TEXT NOT NULL,
	    task_frames_json TEXT NOT NULL,
	    retrieval_spans_json TEXT NOT NULL DEFAULT '[]',
	    updated_at DATETIME NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS navi_chat_memory_checkpoints (
	    checkpoint_id TEXT PRIMARY KEY,
	    chat_id TEXT NOT NULL REFERENCES navi_chats(chat_id),
	    epoch_id TEXT NOT NULL,
	    trigger_class TEXT NOT NULL,
	    compacted_message_start_id TEXT NOT NULL,
	    compacted_message_end_id TEXT NOT NULL,
	    checkpoint_json TEXT NOT NULL,
	    created_at DATETIME NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS idx_navi_chat_memory_checkpoints_chat
	    ON navi_chat_memory_checkpoints(chat_id, created_at DESC);`,
	`CREATE TABLE IF NOT EXISTS navi_inbox (
	    inbox_item_id      TEXT PRIMARY KEY,
	    runtime_session_id TEXT,
	    chat_id            TEXT,
	    source_channel     TEXT NOT NULL,
	    source_message_ref TEXT,
	    origin_endpoint_id TEXT,
	    actor_type         TEXT,
	    payload_type       TEXT,
	    content            TEXT,
	    structured_payload TEXT,
	    correlation_id     TEXT,
	    idempotency_key    TEXT,
	    queue_action       TEXT NOT NULL DEFAULT 'append',
	    status             TEXT NOT NULL DEFAULT 'pending',
	    message_id         TEXT,
	    run_id             TEXT,
	    received_at        DATETIME NOT NULL,
	    consumed_at        DATETIME
	);`,
	`CREATE INDEX IF NOT EXISTS idx_navi_inbox_chat_status
	    ON navi_inbox(chat_id, status, received_at);`,
	`CREATE INDEX IF NOT EXISTS idx_navi_inbox_status
	    ON navi_inbox(status, received_at);`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_navi_inbox_chat_idempotency
	    ON navi_inbox(chat_id, idempotency_key)
	    WHERE chat_id IS NOT NULL AND idempotency_key IS NOT NULL;`,
	`CREATE TABLE IF NOT EXISTS runtime_runs (
	    run_id                  TEXT PRIMARY KEY,
	    runtime_session_id      TEXT,
	    chat_id                 TEXT,
	    experience_mode         TEXT,
	    origin_endpoint_id      TEXT,
	    mode                    TEXT NOT NULL,
	    status                  TEXT NOT NULL,
	    current_phase           TEXT NOT NULL,
	    initiated_by_inbox_item_id TEXT,
	    pause_reason            TEXT,
	    blocked_on_proposal_id  TEXT,
	    interrupt_class         TEXT,
	    interrupt_reason        TEXT,
	    latest_checkpoint_id    TEXT,
	    scratchpad              TEXT, -- JSON object
	    main_artifact_id        TEXT,
	    artifact_ids            TEXT, -- JSON array
	    started_at              DATETIME NOT NULL,
	    updated_at              DATETIME NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS idx_runtime_runs_chat_status
	    ON runtime_runs(chat_id, status, updated_at DESC);`,
	`CREATE TABLE IF NOT EXISTS runtime_sessions (
	    runtime_session_id TEXT PRIMARY KEY,
	    owner_id           TEXT NOT NULL DEFAULT '',
	    workspace_id       TEXT,
	    project_id         TEXT,
	    kind               TEXT NOT NULL DEFAULT 'user' CHECK(kind IN ('user', 'internal', 'background', 'connector', 'dreaming')),
	    status             TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'idle', 'closed', 'failed')),
	    experience_mode    TEXT NOT NULL DEFAULT 'navi',
	    source_channel     TEXT NOT NULL DEFAULT '',
	    started_at         DATETIME NOT NULL,
	    last_active_at     DATETIME NOT NULL,
	    ended_at           DATETIME,
	    metadata_json      TEXT NOT NULL DEFAULT '{}'
	);`,
	`CREATE INDEX IF NOT EXISTS idx_runtime_sessions_status_active
	    ON runtime_sessions(status, last_active_at DESC);`,
	`CREATE INDEX IF NOT EXISTS idx_runtime_sessions_project_active
	    ON runtime_sessions(project_id, last_active_at DESC);`,
	`CREATE TABLE IF NOT EXISTS runtime_session_chats (
	    runtime_session_id TEXT NOT NULL REFERENCES runtime_sessions(runtime_session_id),
	    chat_id            TEXT NOT NULL REFERENCES navi_chats(chat_id),
	    relationship       TEXT NOT NULL DEFAULT 'primary' CHECK(relationship IN ('primary', 'referenced', 'branched', 'background_context')),
	    attached_at        DATETIME NOT NULL,
	    metadata_json      TEXT NOT NULL DEFAULT '{}',
	    PRIMARY KEY (runtime_session_id, chat_id, relationship)
	);`,
	`CREATE INDEX IF NOT EXISTS idx_runtime_session_chats_session
	    ON runtime_session_chats(runtime_session_id, attached_at ASC);`,
	`CREATE INDEX IF NOT EXISTS idx_runtime_session_chats_chat
	    ON runtime_session_chats(chat_id, attached_at ASC);`,
	`CREATE TABLE IF NOT EXISTS runtime_checkpoints (
	    checkpoint_id        TEXT PRIMARY KEY,
	    run_id               TEXT NOT NULL REFERENCES runtime_runs(run_id),
	    runtime_session_id   TEXT,
	    chat_id              TEXT,
	    phase                TEXT NOT NULL,
	    experience_mode      TEXT,
	    llm_messages         TEXT,
	    pending_tool_call    TEXT,
	    pending_proposal_id  TEXT,
	    pending_proposal_reason TEXT,
	    scratchpad           TEXT, -- JSON object
	    main_artifact_id     TEXT,
	    artifact_ids         TEXT, -- JSON array
	    created_at           DATETIME NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS idx_runtime_checkpoints_run
	    ON runtime_checkpoints(run_id, created_at DESC);`,
	`CREATE TABLE IF NOT EXISTS ics_state (
	    run_id             TEXT PRIMARY KEY REFERENCES runtime_runs(run_id),
	    version            TEXT NOT NULL,
	    decision_envelope  TEXT NOT NULL,
	    created_at         DATETIME NOT NULL,
	    updated_at         DATETIME NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS ics_history (
	    run_id             TEXT NOT NULL REFERENCES runtime_runs(run_id),
	    step               INTEGER NOT NULL,
	    decision_envelope  TEXT NOT NULL,
	    created_at         DATETIME NOT NULL,
	    PRIMARY KEY (run_id, step)
	);`,
	`CREATE INDEX IF NOT EXISTS idx_ics_history_run_step
	    ON ics_history(run_id, step DESC);`,
	`CREATE TABLE IF NOT EXISTS navi_meta (
	    key   TEXT PRIMARY KEY,
	    value TEXT NOT NULL
	);`,
}

type queryExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// MigrateSchema applies the required SQL declarations to the database if not present.
// It is idempotent and safe to be called multiple times.
func MigrateSchema(ctx context.Context, db *sql.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("navi: store schema migrate: get conn: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("navi: store schema migrate: disable foreign keys: %w", err)
	}
	defer conn.ExecContext(ctx, "PRAGMA foreign_keys = ON")

	for _, query := range createNaviTables {
		_, err := conn.ExecContext(ctx, query)
		if err != nil {
			return fmt.Errorf("navi: store schema migration failed: %w", err)
		}
	}
	for _, rename := range []struct {
		table   string
		legacy  string
		current string
	}{
		{"runtime_runs", "persona_id", "experience_mode"},
		{"runtime_checkpoints", "persona_id", "experience_mode"},
	} {
		if err := renameColumnIfNeeded(ctx, conn, rename.table, rename.legacy, rename.current); err != nil {
			return fmt.Errorf("navi: rename %s.%s to %s: %w", rename.table, rename.legacy, rename.current, err)
		}
	}
	for _, stmt := range []struct {
		table  string
		column string
		def    string
	}{
		{"navi_inbox", "chat_id", "TEXT"},
		{"navi_inbox", "runtime_session_id", "TEXT"},
		{"navi_inbox", "origin_endpoint_id", "TEXT"},
		{"navi_chat_messages", "origin_endpoint_id", "TEXT"},
		{"runtime_runs", "interrupt_class", "TEXT"},
		{"runtime_runs", "interrupt_reason", "TEXT"},
		{"runtime_runs", "scratchpad", "TEXT"},
		{"runtime_runs", "chat_id", "TEXT"},
		{"runtime_runs", "runtime_session_id", "TEXT"},
		{"runtime_runs", "origin_endpoint_id", "TEXT"},
		{"runtime_runs", "main_artifact_id", "TEXT"},
		{"runtime_runs", "artifact_ids", "TEXT"},
		{"runtime_checkpoints", "scratchpad", "TEXT"},
		{"runtime_checkpoints", "main_artifact_id", "TEXT"},
		{"runtime_checkpoints", "artifact_ids", "TEXT"},
		{"runtime_checkpoints", "chat_id", "TEXT"},
		{"runtime_checkpoints", "runtime_session_id", "TEXT"},
	} {
		if err := addColumnIfNotExists(ctx, conn, stmt.table, stmt.column, stmt.def); err != nil {
			return fmt.Errorf("navi: migrate %s.%s: %w", stmt.table, stmt.column, err)
		}
	}
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS idx_navi_inbox_runtime_status
			ON navi_inbox(runtime_session_id, status, received_at);`,
		`CREATE INDEX IF NOT EXISTS idx_navi_inbox_chat_status
			ON navi_inbox(chat_id, status, received_at);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_navi_inbox_runtime_idempotency
			ON navi_inbox(runtime_session_id, idempotency_key)
			WHERE idempotency_key IS NOT NULL AND runtime_session_id IS NOT NULL;`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_navi_inbox_chat_idempotency
			ON navi_inbox(chat_id, idempotency_key)
			WHERE chat_id IS NOT NULL AND idempotency_key IS NOT NULL;`,
		`CREATE INDEX IF NOT EXISTS idx_navi_chat_messages_origin_endpoint
			ON navi_chat_messages(origin_endpoint_id);`,
		`CREATE INDEX IF NOT EXISTS idx_runtime_runs_runtime_status
			ON runtime_runs(runtime_session_id, status, updated_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_runtime_runs_chat_status
			ON runtime_runs(chat_id, status, updated_at DESC);`,
	} {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("navi: create chat/runtime migration index: %w", err)
		}
	}
	for _, stmt := range []struct {
		table  string
		column string
	}{
		{"runtime_runs", "experience_mode"},
		{"runtime_checkpoints", "experience_mode"},
	} {
		table := quoteIdentifier(stmt.table)
		column := quoteIdentifier(stmt.column)
		query := fmt.Sprintf(`UPDATE %s SET %s = 'navi' WHERE LOWER(TRIM(COALESCE(%s, ''))) NOT IN ('navi', 'wizard')`, table, column, column)
		if _, err := conn.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("navi: normalize %s.%s experience modes: %w", stmt.table, stmt.column, err)
		}
	}
	// Legacy cleanup — Session domain deleted. This is a clean breaking change:
	// legacy Session data is intentionally NOT preserved. If you have data in
	// navi_sessions you needed to keep, you should have exported it before
	// running this version.
	if err := dropLegacySessionSchema(ctx, conn); err != nil {
		return fmt.Errorf("navi: drop legacy session schema: %w", err)
	}
	// Drop ambiguous session_id columns from canonical tables and migrate
	// existing values into chat_id before removing. Idempotent.
	if err := dropLegacySessionIDColumns(ctx, conn); err != nil {
		return fmt.Errorf("navi: drop legacy session_id columns: %w", err)
	}
	return nil
}

// dropLegacySessionSchema removes the legacy Session-domain tables, triggers,
// and indexes if any survive from earlier versions. Idempotent. Destructive by
// design — see the Session B core-deletion tranche for context.
func dropLegacySessionSchema(ctx context.Context, db queryExecer) error {
	exists, err := tableExists(ctx, db, "navi_sessions")
	if err != nil {
		return fmt.Errorf("check navi_sessions: %w", err)
	}
	if exists {
		var rowCount int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM navi_sessions`).Scan(&rowCount); err != nil {
			return fmt.Errorf("count navi_sessions: %w", err)
		}
		if rowCount > 0 {
			slog.Warn(
				"navi: dropping legacy navi_sessions table; legacy Session data is being discarded as part of the breaking cleanup",
				"row_count", rowCount,
			)
		}
	}

	for _, stmt := range []string{
		`DROP TRIGGER IF EXISTS trg_navi_sessions_insert_chat;`,
		`DROP TRIGGER IF EXISTS trg_navi_sessions_update_chat;`,
		`DROP TRIGGER IF EXISTS trg_navi_messages_insert_chat_message;`,
		`DROP TABLE IF EXISTS navi_session_checkpoints;`,
		`DROP TABLE IF EXISTS navi_session_memory;`,
		`DROP TABLE IF EXISTS navi_session_chats;`,
		`DROP TABLE IF EXISTS navi_session_participants;`,
		`DROP TABLE IF EXISTS navi_session_events;`,
		`DROP TABLE IF EXISTS navi_messages;`,
		`DROP TABLE IF EXISTS navi_sessions;`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("drop legacy schema %q: %w", stmt, err)
		}
	}
	return nil
}

// dropLegacySessionIDColumns removes the ambiguous `session_id` column from
// navi_inbox, runtime_runs, and runtime_checkpoints on existing DBs. For each
// table it: (1) copies non-null session_id values into chat_id where chat_id is
// still NULL, (2) drops the old session_id-keyed indexes, (3) drops the column,
// (4) creates the replacement chat_id-keyed indexes. Idempotent — guarded by
// columnExists. This is the Session C tranche of the Chat/RuntimeSession rename.
func dropLegacySessionIDColumns(ctx context.Context, db queryExecer) error {
	// navi_inbox
	if ok, err := columnExists(ctx, db, "navi_inbox", "session_id"); err != nil {
		return fmt.Errorf("check navi_inbox.session_id: %w", err)
	} else if ok {
		stmts := []string{
			`DELETE FROM navi_inbox WHERE idempotency_key IS NOT NULL AND COALESCE(chat_id, session_id) IS NOT NULL AND inbox_item_id NOT IN (SELECT MIN(inbox_item_id) FROM navi_inbox WHERE idempotency_key IS NOT NULL AND COALESCE(chat_id, session_id) IS NOT NULL GROUP BY COALESCE(chat_id, session_id), idempotency_key)`,
			`UPDATE navi_inbox SET chat_id = session_id WHERE chat_id IS NULL AND session_id IS NOT NULL`,
			`DROP INDEX IF EXISTS idx_navi_inbox_idempotency`,
			`DROP INDEX IF EXISTS idx_navi_inbox_session_status`,
			`ALTER TABLE navi_inbox DROP COLUMN session_id`,
			`CREATE INDEX IF NOT EXISTS idx_navi_inbox_chat_status ON navi_inbox(chat_id, status, received_at)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_navi_inbox_chat_idempotency ON navi_inbox(chat_id, idempotency_key) WHERE chat_id IS NOT NULL AND idempotency_key IS NOT NULL`,
		}
		for _, s := range stmts {
			if _, err := db.ExecContext(ctx, s); err != nil {
				return fmt.Errorf("navi_inbox session_id drop %q: %w", s, err)
			}
		}
	}

	// runtime_runs
	if ok, err := columnExists(ctx, db, "runtime_runs", "session_id"); err != nil {
		return fmt.Errorf("check runtime_runs.session_id: %w", err)
	} else if ok {
		stmts := []string{
			`UPDATE runtime_runs SET chat_id = session_id WHERE chat_id IS NULL AND session_id IS NOT NULL`,
			`DROP INDEX IF EXISTS idx_runtime_runs_session_status`,
			`ALTER TABLE runtime_runs DROP COLUMN session_id`,
			`CREATE INDEX IF NOT EXISTS idx_runtime_runs_chat_status ON runtime_runs(chat_id, status, updated_at DESC)`,
		}
		for _, s := range stmts {
			if _, err := db.ExecContext(ctx, s); err != nil {
				return fmt.Errorf("runtime_runs session_id drop %q: %w", s, err)
			}
		}
	}

	// runtime_checkpoints
	if ok, err := columnExists(ctx, db, "runtime_checkpoints", "session_id"); err != nil {
		return fmt.Errorf("check runtime_checkpoints.session_id: %w", err)
	} else if ok {
		stmts := []string{
			`UPDATE runtime_checkpoints SET chat_id = session_id WHERE chat_id IS NULL AND session_id IS NOT NULL`,
			`ALTER TABLE runtime_checkpoints DROP COLUMN session_id`,
		}
		for _, s := range stmts {
			if _, err := db.ExecContext(ctx, s); err != nil {
				return fmt.Errorf("runtime_checkpoints session_id drop %q: %w", s, err)
			}
		}
	}

	return nil
}

func tableExists(ctx context.Context, db queryExecer, table string) (bool, error) {
	var name string
	err := db.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='table' AND name = ?`,
		table,
	).Scan(&name)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func quoteIdentifier(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func addColumnIfNotExists(ctx context.Context, db queryExecer, table, column, def string) error {
	if strings.Contains(def, ";") {
		return fmt.Errorf("navi: store: invalid column definition %q: contains semicolon", def)
	}
	_, err := db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", quoteIdentifier(table), quoteIdentifier(column), def))
	if err != nil && !strings.Contains(err.Error(), "duplicate column") {
		return err
	}
	return nil
}

func renameColumnIfNeeded(ctx context.Context, db queryExecer, table, legacyColumn, currentColumn string) error {
	hasLegacy, err := columnExists(ctx, db, table, legacyColumn)
	if err != nil {
		return err
	}
	if !hasLegacy {
		return nil
	}
	hasCurrent, err := columnExists(ctx, db, table, currentColumn)
	if err != nil {
		return err
	}
	if hasCurrent {
		return nil
	}
	_, err = db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s", quoteIdentifier(table), quoteIdentifier(legacyColumn), quoteIdentifier(currentColumn)))
	return err
}

func columnExists(ctx context.Context, db queryExecer, table, column string) (bool, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", quoteIdentifier(table)))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if strings.EqualFold(name, column) {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}
