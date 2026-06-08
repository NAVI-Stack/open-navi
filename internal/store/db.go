package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const timeFormat = "2006-01-02T15:04:05.999999999Z07:00"

func parseTime(s string) (time.Time, error) {
	return time.Parse(timeFormat, s)
}

const sqliteDriver = "sqlite"

// Keep a small pooled connection count so WAL readers are not forced to queue
// behind unrelated runtime work on a single handle. Writes still serialize via
// SQLite locking, but allowing a few connections materially improves ingress
// latency for gateway reads under connector/runtime load.
const sqliteMaxOpenConns = 4

// sqliteDSN builds a modernc sqlite driver name. Per-connection PRAGMAs in the query
// string apply to every connection in database/sql's pool; see modernc.org/sqlite driver.Open.
func sqliteDSN(path string) string {
	if path == ":memory:" {
		return path
	}
	q := url.Values{}
	q.Add("_pragma", "busy_timeout=15000")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys=ON")
	q.Add("_pragma", "synchronous=NORMAL")
	sep := "?"
	if strings.ContainsRune(path, '?') {
		sep = "&"
	}
	return path + sep + q.Encode()
}

// Open opens a SQLite database at the given path (e.g. "navid.db" or ":memory:").
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open(sqliteDriver, sqliteDSN(path))
	if err != nil {
		return nil, fmt.Errorf("store: open sqlite: %w", err)
	}
	maxOpenConns := sqliteMaxOpenConns
	if path == ":memory:" {
		// Keep in-memory SQLite on a single connection so tests see one database.
		maxOpenConns = 1
	}
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxOpenConns)

	if path == ":memory:" {
		gopragmas := []string{
			"PRAGMA journal_mode=WAL",
			"PRAGMA busy_timeout=15000",
			"PRAGMA foreign_keys=ON",
			"PRAGMA synchronous=NORMAL",
		}
		for _, pragma := range gopragmas {
			if _, err := db.Exec(pragma); err != nil {
				db.Close()
				return nil, fmt.Errorf("store: apply pragma %q: %w", pragma, err)
			}
		}
	}

	return db, nil
}

func quoteIdentifier(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func addColumnIfNotExists(ctx context.Context, db *sql.DB, table, column, def string) error {
	if strings.Contains(def, ";") {
		return fmt.Errorf("store: invalid column definition %q: contains semicolon", def)
	}
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", quoteIdentifier(table)))
	if err != nil {
		return err
	}
	defer rows.Close()
	exists := false
	for rows.Next() {
		var cid int
		var name, typeStr string
		var notnull, pk int
		var dfltVal any
		if err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltVal, &pk); err != nil {
			return err
		}
		if name == column {
			exists = true
		}
	}
	if !exists {
		query := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", quoteIdentifier(table), quoteIdentifier(column), def)
		if _, err := db.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return nil
}

func renameColumnIfExists(ctx context.Context, db *sql.DB, table, oldCol, newCol string) error {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", quoteIdentifier(table)))
	if err != nil {
		return err
	}
	defer rows.Close()
	hasOld := false
	hasNew := false
	for rows.Next() {
		var cid int
		var name, typeStr string
		var notnull, pk int
		var dfltVal any
		if err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltVal, &pk); err != nil {
			return err
		}
		if name == oldCol {
			hasOld = true
		}
		if name == newCol {
			hasNew = true
		}
	}
	if hasOld && !hasNew {
		query := fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s", quoteIdentifier(table), quoteIdentifier(oldCol), quoteIdentifier(newCol))
		if _, err := db.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return nil
}

func lookupTableSQL(ctx context.Context, db *sql.DB, table string) (string, bool, error) {
	var sqlText string
	err := db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&sqlText)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: lookup table %s: %w", table, err)
	}
	return sqlText, true, nil
}

func ensureWorkspaceTablesCurrent(ctx context.Context, db *sql.DB) error {
	workspacesSQL, workspacesExists, err := lookupTableSQL(ctx, db, "workspaces")
	if err != nil {
		return err
	}
	rulesSQL, rulesExists, err := lookupTableSQL(ctx, db, "workspace_whitelist_rules")
	if err != nil {
		return err
	}

	needsWorkspacesMigration := workspacesExists &&
		(!strings.Contains(workspacesSQL, "CHECK (kind IN ('general', 'project', 'repository', 'functional'))") ||
			!strings.Contains(workspacesSQL, "CHECK (status IN ('active', 'inactive', 'archived', 'suspended'))") ||
			!strings.Contains(workspacesSQL, "CHECK (audit_enabled IN (0, 1))"))
	needsRulesMigration := rulesExists &&
		(!strings.Contains(rulesSQL, "ON DELETE CASCADE") ||
			!strings.Contains(rulesSQL, "CHECK (status IN ('active', 'revoked'))"))

	if !needsWorkspacesMigration && !needsRulesMigration {
		return nil
	}
	if rulesExists && !workspacesExists {
		return fmt.Errorf("store: workspace_whitelist_rules exists without workspaces; cannot migrate workspace schema safely")
	}

	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		return fmt.Errorf("store: disable foreign keys for workspace migration: %w", err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), `PRAGMA foreign_keys=ON`)
	}()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin workspace migration: %w", err)
	}
	defer tx.Rollback()

	if needsRulesMigration {
		if _, err := tx.ExecContext(ctx, `
			CREATE TABLE workspace_whitelist_rules_backup (
				rule_id      TEXT PRIMARY KEY,
				workspace_id TEXT NOT NULL,
				scope        TEXT NOT NULL,
				action_types TEXT NOT NULL,
				status       TEXT NOT NULL,
				created_at   TEXT NOT NULL,
				revoked_at   TEXT,
				expires_at   TEXT,
				created_by   TEXT NOT NULL
			)`); err != nil {
			return fmt.Errorf("store: create workspace whitelist backup table: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO workspace_whitelist_rules_backup (
				rule_id, workspace_id, scope, action_types, status, created_at, revoked_at, expires_at, created_by
			)
			SELECT
				rule_id, workspace_id, scope, action_types, status, created_at, revoked_at, expires_at, created_by
			FROM workspace_whitelist_rules`); err != nil {
			return fmt.Errorf("store: backup workspace whitelist rules: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DROP TABLE workspace_whitelist_rules`); err != nil {
			return fmt.Errorf("store: drop legacy workspace whitelist rules: %w", err)
		}
	}

	if needsWorkspacesMigration {
		if _, err := tx.ExecContext(ctx, `
			CREATE TABLE workspaces_new (
				id                 TEXT PRIMARY KEY,
				name               TEXT NOT NULL,
				description        TEXT,
				kind               TEXT NOT NULL CHECK (kind IN ('general', 'project', 'repository', 'functional')),
				status             TEXT NOT NULL CHECK (status IN ('active', 'inactive', 'archived', 'suspended')),
				local_roots        TEXT NOT NULL,
				repo_roots         TEXT NOT NULL,
				protected_paths    TEXT NOT NULL,
				allowed_actions    TEXT NOT NULL,
				boundary_policy    TEXT NOT NULL,
				audit_enabled      INTEGER NOT NULL CHECK (audit_enabled IN (0, 1)),
				created_at         TEXT NOT NULL,
				updated_at         TEXT NOT NULL,
				created_by         TEXT NOT NULL,
				tags               TEXT NOT NULL DEFAULT '[]',
				related_project_id TEXT,
				notes              TEXT,
				metadata           TEXT NOT NULL DEFAULT '{}'
			)`); err != nil {
			return fmt.Errorf("store: create replacement workspaces table: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO workspaces_new (
				id, name, description, kind, status, local_roots, repo_roots, protected_paths,
				allowed_actions, boundary_policy, audit_enabled, created_at, updated_at, created_by,
				tags, related_project_id, notes, metadata
			)
			SELECT
				id, name, description, kind, status, local_roots, repo_roots, protected_paths,
				allowed_actions, boundary_policy, audit_enabled, created_at, updated_at, created_by,
				tags, related_project_id, notes, metadata
			FROM workspaces`); err != nil {
			return fmt.Errorf("store: copy workspaces into replacement table: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DROP TABLE workspaces`); err != nil {
			return fmt.Errorf("store: drop legacy workspaces table: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `ALTER TABLE workspaces_new RENAME TO workspaces`); err != nil {
			return fmt.Errorf("store: rename replacement workspaces table: %w", err)
		}
	}

	if needsRulesMigration {
		if _, err := tx.ExecContext(ctx, `
			CREATE TABLE workspace_whitelist_rules (
				rule_id      TEXT PRIMARY KEY,
				workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
				scope        TEXT NOT NULL,
				action_types TEXT NOT NULL,
				status       TEXT NOT NULL CHECK (status IN ('active', 'revoked')),
				created_at   TEXT NOT NULL,
				revoked_at   TEXT,
				expires_at   TEXT,
				created_by   TEXT NOT NULL
			)`); err != nil {
			return fmt.Errorf("store: create replacement workspace whitelist rules table: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO workspace_whitelist_rules (
				rule_id, workspace_id, scope, action_types, status, created_at, revoked_at, expires_at, created_by
			)
			SELECT
				rule_id, workspace_id, scope, action_types, status, created_at, revoked_at, expires_at, created_by
			FROM workspace_whitelist_rules_backup`); err != nil {
			return fmt.Errorf("store: restore workspace whitelist rules: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DROP TABLE workspace_whitelist_rules_backup`); err != nil {
			return fmt.Errorf("store: drop workspace whitelist backup table: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit workspace migration: %w", err)
	}
	return nil
}

func ensureUniqueActiveWorkspaceProjectBindings(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `
		SELECT related_project_id
		FROM workspaces
		WHERE status = ? AND related_project_id IS NOT NULL AND related_project_id <> ''
		GROUP BY related_project_id
		HAVING COUNT(*) > 1
		ORDER BY related_project_id ASC
	`, string("active"))
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return fmt.Errorf("store: verify unique active workspace project bindings: %w", err)
	}
	defer rows.Close()

	var projectIDs []string
	for rows.Next() {
		var projectID string
		if err := rows.Scan(&projectID); err != nil {
			return fmt.Errorf("store: scan duplicate active workspace project binding: %w", err)
		}
		projectIDs = append(projectIDs, projectID)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: iterate duplicate active workspace project bindings: %w", err)
	}
	if len(projectIDs) > 0 {
		return fmt.Errorf("store: duplicate active workspace project bindings found for project IDs: %s", strings.Join(projectIDs, ", "))
	}
	return nil
}

// ensureColumnsBeforeDDL adds columns that the DDL array's CREATE INDEX
// statements depend on.  CREATE TABLE IF NOT EXISTS does not modify existing
// tables, so older databases may be missing columns added after the initial
// schema.  Running these migrations first prevents "no such column" errors
// when the DDL loop creates indexes on those columns.
//
// On a fresh database the tables don't exist yet — ALTER TABLE will return
// "no such table" which we silently skip; the DDL loop will create them with
// the complete schema.
func ensureColumnsBeforeDDL(ctx context.Context, db *sql.DB) error {
	migrations := []struct{ table, column, def string }{
		// artifacts columns required by idx_artifacts_workspace / idx_artifacts_project / idx_artifacts_owner_id
		{"artifacts", "workspace_id", "TEXT NOT NULL DEFAULT ''"},
		{"artifacts", "project_id", "TEXT"},
		{"artifacts", "owner_id", "TEXT NOT NULL DEFAULT ''"},
		{"artifacts", "canonical_title", "TEXT NOT NULL DEFAULT ''"},
		{"artifacts", "display_title", "TEXT NOT NULL DEFAULT ''"},
		{"artifacts", "schema_version", "TEXT NOT NULL DEFAULT ''"},
		{"artifacts", "content_format", "TEXT NOT NULL DEFAULT ''"},
		{"artifacts", "lifecycle_state", "TEXT NOT NULL DEFAULT ''"},
		{"artifacts", "current_branch_id", "TEXT NOT NULL DEFAULT ''"},
		{"artifacts", "current_version_id", "TEXT NOT NULL DEFAULT ''"},
		{"artifacts", "head_version_number", "INTEGER NOT NULL DEFAULT 0"},
		{"artifacts", "created_by_actor_type", "TEXT NOT NULL DEFAULT ''"},
		{"artifacts", "created_by_actor_id", "TEXT"},
		{"artifacts", "provenance_root_id", "TEXT NOT NULL DEFAULT ''"},
		{"artifacts", "attributes", "TEXT NOT NULL DEFAULT '{}'"},
		{"artifacts", "archived_at", "TEXT"},
		// execution_outcomes.artifact_id (added by OMN-118)
		{"execution_outcomes", "artifact_id", "TEXT"},
		// proposals.boundary_key required by idx_proposals_boundary_key
		{"proposals", "boundary_key", "TEXT"},
		// Project task intake fields required by project task indexes.
		{"tasks", "project_id", "TEXT"},
		{"tasks", "workspace_id", "TEXT"},
		{"tasks", "task_class", "TEXT NOT NULL DEFAULT ''"},
		{"tasks", "raw_input", "TEXT NOT NULL DEFAULT ''"},
		{"tasks", "acceptance_target", "TEXT NOT NULL DEFAULT ''"},
		{"tasks", "lifecycle_phase", "TEXT NOT NULL DEFAULT ''"},
		{"tasks", "block_reason", "TEXT NOT NULL DEFAULT ''"},
		// Project execution sandbox binding.
		{"projects", "sandbox_profile_id", "TEXT"},
		// contacts.owner_type required by idx_contacts_owner_type
		{"contacts", "owner_type", "TEXT NOT NULL DEFAULT 'navi'"},
		// contacts.trust_level required by idx_contacts_trust_level (added for merged contact taxonomy)
		{"contacts", "trust_level", "TEXT NOT NULL DEFAULT 'unknown'"},
	}
	for _, m := range migrations {
		query := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", quoteIdentifier(m.table), quoteIdentifier(m.column), m.def)
		_, err := db.ExecContext(ctx, query)
		if err != nil &&
			!strings.Contains(err.Error(), "duplicate column") &&
			!strings.Contains(err.Error(), "no such table") {
			return fmt.Errorf("store: pre-ddl migrate %s.%s: %w", m.table, m.column, err)
		}
	}
	return nil
}

// ensureCronJobsTableCurrent recreates the cron_jobs table when its CHECK
// constraints predate session-targeted assistant-message delivery (i.e. when
// session_target only allows 'main'/'isolated' or payload_kind lacks
// 'assistantMessage'). SQLite cannot drop a CHECK constraint in place, so we
// rename, recreate with the relaxed CHECK, copy rows over, and drop the old.
func ensureCronJobsTableCurrent(ctx context.Context, db *sql.DB) error {
	cronSQL, exists, err := lookupTableSQL(ctx, db, "cron_jobs")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if strings.Contains(cronSQL, "'assistantMessage'") &&
		strings.Contains(cronSQL, "session_target LIKE 'session:%'") {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin cron_jobs migration: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `ALTER TABLE cron_jobs RENAME TO cron_jobs_legacy`); err != nil {
		return fmt.Errorf("store: rename legacy cron_jobs: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE cron_jobs (
			id TEXT PRIMARY KEY,
			owner_id TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			schedule_kind TEXT NOT NULL CHECK (schedule_kind IN ('at', 'every', 'cron')),
			schedule_expr TEXT,
			schedule_every_ms INTEGER,
			schedule_anchor_ms INTEGER,
			schedule_tz TEXT,
			schedule_stagger_ms INTEGER,
			session_target TEXT NOT NULL CHECK (session_target IN ('main', 'isolated') OR session_target LIKE 'session:%'),
			wake_mode TEXT NOT NULL CHECK (wake_mode IN ('now', 'next-heartbeat')),
			payload_kind TEXT NOT NULL CHECK (payload_kind IN ('systemEvent', 'agentTurn', 'assistantMessage')),
			payload_text TEXT NOT NULL DEFAULT '',
			delivery_json TEXT,
			failure_alert_json TEXT,
			timeout_ms INTEGER,
			delete_after_run INTEGER NOT NULL DEFAULT 0,
			next_run_at_ms INTEGER,
			running_at_ms INTEGER,
			last_run_at_ms INTEGER,
			last_run_status TEXT CHECK (last_run_status IS NULL OR last_run_status IN ('ok','error','skipped')),
			last_error TEXT,
			last_duration_ms INTEGER,
			consecutive_errors INTEGER NOT NULL DEFAULT 0,
			schedule_error_count INTEGER NOT NULL DEFAULT 0,
			last_failure_alert_at_ms INTEGER,
			created_at_ms INTEGER NOT NULL,
			updated_at_ms INTEGER NOT NULL
		)`); err != nil {
		return fmt.Errorf("store: create relaxed cron_jobs: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO cron_jobs SELECT * FROM cron_jobs_legacy`); err != nil {
		return fmt.Errorf("store: copy cron_jobs rows: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE cron_jobs_legacy`); err != nil {
		return fmt.Errorf("store: drop legacy cron_jobs: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit cron_jobs migration: %w", err)
	}
	return nil
}

// CreateTables creates all Phase 2 tables idempotently.
func CreateTables(ctx context.Context, db *sql.DB) error {
	if err := ensureWorkspaceTablesCurrent(ctx, db); err != nil {
		return err
	}
	if err := ensureUniqueActiveWorkspaceProjectBindings(ctx, db); err != nil {
		return err
	}
	if err := ensureCronJobsTableCurrent(ctx, db); err != nil {
		return err
	}
	// Ensure columns exist on pre-existing tables before CREATE INDEX runs.
	if err := ensureColumnsBeforeDDL(ctx, db); err != nil {
		return err
	}
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS events (
				id TEXT PRIMARY KEY,
				type TEXT NOT NULL,
				kind TEXT NOT NULL,
				correlation_id TEXT NOT NULL,
				causal_parent TEXT NOT NULL,
				source_agent TEXT NOT NULL,
				target_agent TEXT NOT NULL,
				timestamp TEXT NOT NULL,
				payload TEXT NOT NULL,
				schema_version TEXT NOT NULL,
				seq INTEGER NOT NULL,
				run_id TEXT,
				visibility TEXT
			)`,
		`CREATE INDEX IF NOT EXISTS idx_events_correlation_id ON events(correlation_id)`,
		`CREATE INDEX IF NOT EXISTS idx_events_causal_parent ON events(causal_parent)`,
		`CREATE INDEX IF NOT EXISTS idx_events_seq ON events(seq)`,
		`CREATE INDEX IF NOT EXISTS idx_events_correlation_seq ON events(correlation_id, seq)`,
		`CREATE TABLE IF NOT EXISTS agents (
			agent_id TEXT PRIMARY KEY,
			agent_type TEXT NOT NULL,
			registered_at TEXT NOT NULL,
			retired_at TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_agents_retired_at ON agents(retired_at)`,
		`CREATE TABLE IF NOT EXISTS agent_identities (
			id TEXT PRIMARY KEY,
			key_type TEXT NOT NULL,
			public_key TEXT NOT NULL,
			fingerprint TEXT NOT NULL UNIQUE,
			created_at TEXT NOT NULL,
			status TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_identities_active
			ON agent_identities(status) WHERE status = 'active'`,
		`CREATE TABLE IF NOT EXISTS directives (
			directive_id  TEXT PRIMARY KEY,
			title         TEXT NOT NULL,
			mode          TEXT NOT NULL DEFAULT 'CHAT',
			status        TEXT NOT NULL DEFAULT 'ACTIVE',
			created_via   TEXT NOT NULL DEFAULT 'local',
			created_at    TEXT NOT NULL,
			updated_at    TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS directive_messages (
			message_id   TEXT PRIMARY KEY,
			directive_id TEXT NOT NULL REFERENCES directives(directive_id),
			role         TEXT NOT NULL CHECK(role IN ('owner','navi')),
			content      TEXT NOT NULL,
			created_at   TEXT NOT NULL,
			tokens_used  INTEGER,
			model        TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_dm_directive ON directive_messages(directive_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_directives_status ON directives(status)`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			description TEXT NOT NULL,
			status TEXT NOT NULL,
			risk TEXT NOT NULL,
			assigned_to TEXT NOT NULL,
			directive_id TEXT NOT NULL,
			dependencies TEXT NOT NULL,
			surfaces TEXT NOT NULL,
			verification TEXT NOT NULL,
			cost TEXT NOT NULL,
			project_id TEXT,
			workspace_id TEXT,
			task_class TEXT NOT NULL DEFAULT '',
			raw_input TEXT NOT NULL DEFAULT '',
			acceptance_target TEXT NOT NULL DEFAULT '',
			lifecycle_phase TEXT NOT NULL DEFAULT '',
			block_reason TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_assigned_to ON tasks(assigned_to)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_risk ON tasks(risk)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_project ON tasks(project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_project_status ON tasks(project_id, status)`,
		`CREATE TABLE IF NOT EXISTS scheduled_tasks (
			id TEXT PRIMARY KEY,
			owner_id TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT NOT NULL,
			schedule_pattern TEXT NOT NULL,
			prompt TEXT NOT NULL,
			status TEXT NOT NULL,
			last_run_at TEXT,
			next_run_at TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_scheduled_tasks_next_run_at ON scheduled_tasks(next_run_at)`,
		`CREATE INDEX IF NOT EXISTS idx_scheduled_tasks_status ON scheduled_tasks(status)`,
		`CREATE TABLE IF NOT EXISTS cron_jobs (
			id TEXT PRIMARY KEY,
			owner_id TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			schedule_kind TEXT NOT NULL CHECK (schedule_kind IN ('at', 'every', 'cron')),
			schedule_expr TEXT,
			schedule_every_ms INTEGER,
			schedule_anchor_ms INTEGER,
			schedule_tz TEXT,
			schedule_stagger_ms INTEGER,
			session_target TEXT NOT NULL CHECK (session_target IN ('main', 'isolated') OR session_target LIKE 'session:%'),
			wake_mode TEXT NOT NULL CHECK (wake_mode IN ('now', 'next-heartbeat')),
			payload_kind TEXT NOT NULL CHECK (payload_kind IN ('systemEvent', 'agentTurn', 'assistantMessage')),
			payload_text TEXT NOT NULL DEFAULT '',
			delivery_json TEXT,
			failure_alert_json TEXT,
			timeout_ms INTEGER,
			delete_after_run INTEGER NOT NULL DEFAULT 0,
			next_run_at_ms INTEGER,
			running_at_ms INTEGER,
			last_run_at_ms INTEGER,
			last_run_status TEXT CHECK (last_run_status IS NULL OR last_run_status IN ('ok','error','skipped')),
			last_error TEXT,
			last_duration_ms INTEGER,
			consecutive_errors INTEGER NOT NULL DEFAULT 0,
			schedule_error_count INTEGER NOT NULL DEFAULT 0,
			last_failure_alert_at_ms INTEGER,
			created_at_ms INTEGER NOT NULL,
			updated_at_ms INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cron_jobs_enabled_next ON cron_jobs(enabled, next_run_at_ms)`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			id TEXT PRIMARY KEY,
			owner_id TEXT NOT NULL,
			key_hash TEXT NOT NULL UNIQUE,
			name TEXT,
			scopes TEXT NOT NULL,
			created_at TEXT NOT NULL,
			last_used_at TEXT,
			revoked_at TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_revoked_at ON api_keys(revoked_at)`,
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			github_handle TEXT NOT NULL UNIQUE,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key        TEXT PRIMARY KEY,
			value      TEXT NOT NULL,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		// Owners table — single owner per NAVI instance.
		`CREATE TABLE IF NOT EXISTS owners (
			id                 TEXT PRIMARY KEY,
			instance_id        TEXT NOT NULL UNIQUE,
			name               TEXT NOT NULL,
			handle             TEXT NOT NULL,
			device_name        TEXT NOT NULL DEFAULT '',
			secret_fingerprint TEXT NOT NULL,
			created_at         TEXT NOT NULL,
			timezone           TEXT NOT NULL DEFAULT 'UTC'
		)`,
		// Facts table — minimal long-term memory (directive/session/global scoped).
		`CREATE TABLE IF NOT EXISTS facts (
			id         TEXT PRIMARY KEY,
			scope      TEXT NOT NULL,
			scope_id   TEXT NOT NULL,
			category   TEXT NOT NULL,
			key        TEXT NOT NULL,
			value      TEXT NOT NULL,
			keywords   TEXT NOT NULL DEFAULT '[]',
			tags       TEXT NOT NULL DEFAULT '[]',
			embedding  TEXT NOT NULL DEFAULT '[]',
			source     TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_facts_scope_scope_id ON facts(scope, scope_id)`,
		`CREATE INDEX IF NOT EXISTS idx_facts_category ON facts(category)`,
		// Contacts — contact graph (NAVI's own contacts + owner's imported contacts).
		`CREATE TABLE IF NOT EXISTS contacts (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			kind        TEXT NOT NULL,                    -- "person", "organization", "service", "navi"
			owner_type  TEXT NOT NULL DEFAULT 'navi',     -- "navi" or "owner"
			trust_level TEXT NOT NULL DEFAULT 'unknown',  -- "unknown", "low", "medium", "high", "verified"
			metadata    TEXT NOT NULL DEFAULT '{}',       -- JSON blob for extensible attributes
			created_at  TEXT NOT NULL,
			updated_at  TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_name ON contacts(name)`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_owner_type ON contacts(owner_type)`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_owner_type_name ON contacts(owner_type, name)`,
		`CREATE INDEX IF NOT EXISTS idx_contacts_trust_level ON contacts(trust_level)`,
		// Artifacts — files, documents, and other tangible outputs NAVI manages.
		`CREATE TABLE IF NOT EXISTS artifacts (
			id                    TEXT PRIMARY KEY,
			workspace_id          TEXT NOT NULL,
			project_id            TEXT,
			owner_id              TEXT NOT NULL,
			canonical_title       TEXT NOT NULL,
			display_title         TEXT NOT NULL,
			type                  TEXT NOT NULL, -- document, code, data, presentation
			subtype               TEXT NOT NULL, -- e.g. markdown, python, csv
			schema_version        TEXT NOT NULL,
			content_format        TEXT NOT NULL,
			lifecycle_state       TEXT NOT NULL, -- draft, active, archived, etc.
			current_branch_id     TEXT NOT NULL,
			current_version_id    TEXT NOT NULL,
			head_version_number   INTEGER NOT NULL,
			created_by_actor_type TEXT NOT NULL,
			created_by_actor_id   TEXT,
			provenance_root_id    TEXT NOT NULL,
			attributes            TEXT NOT NULL DEFAULT '{}', -- JSON blob
			created_at            TEXT NOT NULL,
			updated_at            TEXT NOT NULL,
			archived_at           TEXT
		)`,
		// Artifact indexes on workspace_id / project_id / owner_id are created after migrations
		// so older DBs that predate those columns can ALTER TABLE first.
		`CREATE TABLE IF NOT EXISTS artifact_versions (
			id                         TEXT PRIMARY KEY,
			artifact_id                TEXT NOT NULL REFERENCES artifacts(id),
			branch_id                  TEXT NOT NULL,
			version_number             INTEGER NOT NULL,
			parent_version_id          TEXT,
			base_version_id            TEXT,
			author_actor_type          TEXT NOT NULL,
			author_actor_id            TEXT,
			source_message_id          TEXT,
			source_conversation_id     TEXT,
			source_operation_id        TEXT NOT NULL,
			change_type                TEXT NOT NULL,
			change_mode                TEXT NOT NULL,
			change_summary             TEXT NOT NULL,
			content_uri                TEXT NOT NULL,
			storage_key                TEXT NOT NULL,
			checksum_sha256            TEXT NOT NULL,
			rendered_uri               TEXT,
			rendered_storage_key       TEXT,
			rendered_checksum          TEXT,
			diff_uri                   TEXT,
			diff_storage_key           TEXT,
			diff_checksum              TEXT,
			size_in_bytes              INTEGER NOT NULL,
			patch_strategy             TEXT,
			patch_metadata             TEXT, -- JSON patch
			validation_state           TEXT NOT NULL,
			commit_state               TEXT NOT NULL,
			project_snapshot_id        TEXT,
			policy_snapshot_id         TEXT NOT NULL,
			source_snapshot_ids        TEXT NOT NULL DEFAULT '[]', -- JSON array
			artifact_input_snapshot_ids TEXT NOT NULL DEFAULT '[]', -- JSON array
			history_command_id         TEXT,
			history_attempt_id         TEXT,
			attribute_delta            TEXT NOT NULL DEFAULT '{}', -- JSON blob
			created_at                 TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_av_artifact ON artifact_versions(artifact_id, version_number)`,
		`CREATE TABLE IF NOT EXISTS artifact_branches (
			id                    TEXT PRIMARY KEY,
			artifact_id           TEXT NOT NULL REFERENCES artifacts(id),
			name                  TEXT NOT NULL,
			base_version_id       TEXT NOT NULL,
			head_version_id       TEXT NOT NULL,
			status                TEXT NOT NULL,
			created_by_actor_type TEXT NOT NULL,
			created_by_actor_id   TEXT,
			created_at            TEXT NOT NULL,
			updated_at            TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ab_artifact ON artifact_branches(artifact_id)`,
		`CREATE TABLE IF NOT EXISTS artifact_references (
			id                TEXT PRIMARY KEY,
			artifact_id       TEXT NOT NULL REFERENCES artifacts(id),
			source_kind       TEXT NOT NULL,
			source_id         TEXT NOT NULL,
			relationship_type TEXT NOT NULL,
			version_id        TEXT,
			branch_id         TEXT,
			metadata          TEXT NOT NULL DEFAULT '{}', -- JSON blob
			created_at        TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ar_artifact ON artifact_references(artifact_id)`,
		`CREATE INDEX IF NOT EXISTS idx_ar_source ON artifact_references(source_kind, source_id)`,
		`CREATE TABLE IF NOT EXISTS artifact_snapshots (
			id                   TEXT PRIMARY KEY,
			snapshot_type        TEXT NOT NULL,
			captured_entity_type TEXT NOT NULL,
			captured_entity_id   TEXT NOT NULL,
			captured_version     TEXT,
			content_uri          TEXT NOT NULL,
			storage_key          TEXT NOT NULL,
			checksum_sha256      TEXT NOT NULL,
			created_at           TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS artifact_exports (
			export_id             TEXT PRIMARY KEY,
			artifact_id           TEXT NOT NULL REFERENCES artifacts(id),
			version_id            TEXT NOT NULL REFERENCES artifact_versions(id),
			format                TEXT NOT NULL,
			target_kind           TEXT NOT NULL,
			target_uri            TEXT,
			content_uri           TEXT,
			storage_key           TEXT,
			checksum_sha256       TEXT,
			content_type          TEXT,
			status                TEXT NOT NULL,
			failure_class         TEXT,
			failure_reason        TEXT,
			created_by_actor_type TEXT NOT NULL,
			created_by_actor_id   TEXT,
			created_at            TEXT NOT NULL,
			completed_at          TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ae_artifact ON artifact_exports(artifact_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_ae_status ON artifact_exports(status)`,
		`CREATE TABLE IF NOT EXISTS artifact_shares (
			share_id              TEXT PRIMARY KEY,
			artifact_id           TEXT NOT NULL REFERENCES artifacts(id),
			version_id            TEXT,
			scope                 TEXT NOT NULL,
			access_level          TEXT NOT NULL,
			status                TEXT NOT NULL,
			share_url             TEXT,
			created_by_actor_type TEXT NOT NULL,
			created_by_actor_id   TEXT,
			created_at            TEXT NOT NULL,
			expires_at            TEXT,
			revoked_at            TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_as_artifact ON artifact_shares(artifact_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_as_status ON artifact_shares(status)`,
		// World-model events — scheduling and temporal awareness (distinct from bus events).
		`CREATE TABLE IF NOT EXISTS wm_events (
			id         TEXT PRIMARY KEY,
			owner_id   TEXT NOT NULL,
			title      TEXT NOT NULL,
			start_time TEXT NOT NULL,
			end_time   TEXT,
			kind       TEXT NOT NULL,
			source     TEXT NOT NULL,
			metadata   TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_wm_events_owner_start ON wm_events(owner_id, start_time)`,
		// Memories — significant experiences and consolidated session rollups.
		`CREATE TABLE IF NOT EXISTS memories (
			id           TEXT PRIMARY KEY,
			scope        TEXT NOT NULL,
			scope_id     TEXT NOT NULL,
			summary      TEXT NOT NULL,
			details      TEXT NOT NULL,
			keywords     TEXT NOT NULL DEFAULT '[]',
			tags         TEXT NOT NULL DEFAULT '[]',
			embedding    TEXT NOT NULL DEFAULT '[]',
			significance TEXT NOT NULL,
			source       TEXT NOT NULL,
			created_at   TEXT NOT NULL,
			updated_at   TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_scope_scope_id ON memories(scope, scope_id)`,
		`CREATE TABLE IF NOT EXISTS memory_links (
			left_type        TEXT NOT NULL,
			left_id          TEXT NOT NULL,
			right_type       TEXT NOT NULL,
			right_id         TEXT NOT NULL,
			similarity       REAL NOT NULL DEFAULT 0,
			source           TEXT NOT NULL DEFAULT 'semantic_similarity',
			relationship_tag TEXT,
			created_at       TEXT NOT NULL,
			updated_at       TEXT NOT NULL,
			PRIMARY KEY (left_type, left_id, right_type, right_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_links_left ON memory_links(left_type, left_id, similarity DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_links_right ON memory_links(right_type, right_id, similarity DESC)`,
		// Configuration — explicit and inferred configuration values (world-model view).
		`CREATE TABLE IF NOT EXISTS configuration (
			id         TEXT PRIMARY KEY,
			scope      TEXT NOT NULL, -- e.g. "owner", "session", "global", or domain-specific
			scope_id   TEXT NOT NULL,
			key        TEXT NOT NULL,
			value      TEXT NOT NULL,
			source     TEXT NOT NULL, -- "explicit", "owner_set", "inferred"
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_configuration_scope_scope_id ON configuration(scope, scope_id)`,
		`CREATE INDEX IF NOT EXISTS idx_configuration_key ON configuration(key)`,
		// Experience preference signals — shallow reflection captures for later consolidation.
		`CREATE TABLE IF NOT EXISTS experience_preference_signals (
			signal_id       TEXT PRIMARY KEY,
			scope           TEXT NOT NULL,
			scope_id        TEXT NOT NULL,
			chat_id         TEXT,
			trait           TEXT NOT NULL,
			target_value    REAL NOT NULL,
			evidence_class  TEXT NOT NULL,
			signal_strength REAL NOT NULL,
			immediate       INTEGER NOT NULL DEFAULT 0,
			summary         TEXT NOT NULL DEFAULT '',
			status          TEXT NOT NULL,
			created_at      TEXT NOT NULL,
			updated_at      TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_experience_preference_signals_scope_status ON experience_preference_signals(scope, scope_id, status, created_at)`,
		// Priorities — stated and observed priorities defining goals and intentions.
		`CREATE TABLE IF NOT EXISTS priorities (
			id         TEXT PRIMARY KEY,
			scope      TEXT NOT NULL, -- e.g. "owner", "directive", "session"
			scope_id   TEXT NOT NULL,
			name       TEXT NOT NULL,
			description TEXT NOT NULL,
			source     TEXT NOT NULL, -- "owner_set", "observed"
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_priorities_scope_scope_id ON priorities(scope, scope_id)`,
		// Gaps — detected capability gaps for the Capability Expansion Loop.
		`CREATE TABLE IF NOT EXISTS gaps (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,               -- A–F (gap class)
			status TEXT NOT NULL,             -- detected, classified, expansion_path_chosen, closed
			evidence TEXT NOT NULL,           -- JSON blob with message_id, directive_id, session_id, tool_error, etc.
			classification_result TEXT NOT NULL, -- JSON blob with gap_class and expansion_path
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_gaps_status ON gaps(status)`,
		// Proposals — first-class Proposal Queue entity.
		`CREATE TABLE IF NOT EXISTS proposals (
			proposal_id      TEXT PRIMARY KEY,
			source_process   TEXT NOT NULL,
			source_trigger   TEXT NOT NULL,
			boundary_key     TEXT,
			proposed_action  TEXT NOT NULL,
			affected_entities TEXT NOT NULL,
			rationale        TEXT NOT NULL,
			priority         TEXT NOT NULL, -- "blocking" or "queued"
			status           TEXT NOT NULL, -- "pending", "approved", "declined", "expired", "superseded"
			created_at       TEXT NOT NULL,
			expires_at       TEXT,
			resolved_at      TEXT,
			resolved_by      TEXT,
			resolution_type  TEXT,
			resolution_note  TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_proposals_status ON proposals(status)`,
		`CREATE INDEX IF NOT EXISTS idx_proposals_priority ON proposals(priority)`,
		`CREATE INDEX IF NOT EXISTS idx_proposals_boundary_key ON proposals(boundary_key)`,
		// Workspaces — durable execution boundaries.
		`CREATE TABLE IF NOT EXISTS workspaces (
			id                 TEXT PRIMARY KEY,
			name               TEXT NOT NULL,
			description        TEXT,
			kind               TEXT NOT NULL CHECK (kind IN ('general', 'project', 'repository', 'functional')),
			status             TEXT NOT NULL CHECK (status IN ('active', 'inactive', 'archived', 'suspended')),
			local_roots        TEXT NOT NULL, -- JSON array
			repo_roots         TEXT NOT NULL, -- JSON array
			protected_paths    TEXT NOT NULL, -- JSON array
			allowed_actions    TEXT NOT NULL, -- JSON blob
			boundary_policy    TEXT NOT NULL, -- JSON blob
			audit_enabled      INTEGER NOT NULL CHECK (audit_enabled IN (0, 1)),
			created_at         TEXT NOT NULL,
			updated_at         TEXT NOT NULL,
			created_by         TEXT NOT NULL,
			tags               TEXT NOT NULL DEFAULT '[]', -- JSON array
			related_project_id TEXT,
			notes              TEXT,
			metadata           TEXT NOT NULL DEFAULT '{}'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_workspaces_status ON workspaces(status)`,
		`CREATE INDEX IF NOT EXISTS idx_workspaces_kind ON workspaces(kind)`,
		`CREATE INDEX IF NOT EXISTS idx_workspaces_related_project_id ON workspaces(related_project_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_workspaces_active_related_project_id
			ON workspaces(related_project_id)
			WHERE status = 'active' AND related_project_id IS NOT NULL AND related_project_id <> ''`,
		// Sandbox profiles - bounded coding execution environments.
		`CREATE TABLE IF NOT EXISTS sandbox_profiles (
			id                     TEXT PRIMARY KEY,
			name                   TEXT NOT NULL,
			description            TEXT,
			runtime                TEXT NOT NULL CHECK (runtime IN ('docker')),
			status                 TEXT NOT NULL CHECK (status IN ('active', 'inactive', 'archived')),
			image                  TEXT NOT NULL,
			network_mode           TEXT NOT NULL CHECK (network_mode IN ('none', 'bridge')),
			workspace_mount_target TEXT NOT NULL,
			command_allowlist      TEXT NOT NULL DEFAULT '[]',
			env_allowlist          TEXT NOT NULL DEFAULT '[]',
			default_timeout_ms     INTEGER NOT NULL,
			max_timeout_ms         INTEGER NOT NULL,
			cpu_limit              TEXT NOT NULL DEFAULT '',
			memory_limit           TEXT NOT NULL DEFAULT '',
			created_at             TEXT NOT NULL,
			updated_at             TEXT NOT NULL,
			created_by             TEXT NOT NULL,
			metadata               TEXT NOT NULL DEFAULT '{}'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sandbox_profiles_status ON sandbox_profiles(status)`,
		`CREATE INDEX IF NOT EXISTS idx_sandbox_profiles_runtime ON sandbox_profiles(runtime)`,
		// Projects - first-class work containers and optional workspace bindings.
		`CREATE TABLE IF NOT EXISTS projects (
			id           TEXT PRIMARY KEY,
			title        TEXT NOT NULL,
			slug         TEXT NOT NULL,
			description  TEXT,
			project_kind TEXT NOT NULL CHECK (project_kind IN ('general', 'coding')),
			status       TEXT NOT NULL CHECK (status IN ('draft', 'active', 'on_hold', 'completed', 'archived')),
			health       TEXT NOT NULL CHECK (health IN ('on_track', 'at_risk', 'blocked', 'unknown')),
			workspace_id TEXT REFERENCES workspaces(id),
			sandbox_profile_id TEXT REFERENCES sandbox_profiles(id),
			icon         TEXT NOT NULL DEFAULT 'folder',
			color        TEXT NOT NULL DEFAULT '#3b82f6',
			memory_scope TEXT NOT NULL DEFAULT 'default',
			created_at   TEXT NOT NULL,
			updated_at   TEXT NOT NULL,
			created_by   TEXT NOT NULL,
			attributes   TEXT NOT NULL DEFAULT '{}'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_projects_slug ON projects(slug)`,
		`CREATE INDEX IF NOT EXISTS idx_projects_status ON projects(status)`,
		`CREATE INDEX IF NOT EXISTS idx_projects_workspace_id ON projects(workspace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_projects_sandbox_profile_id ON projects(sandbox_profile_id)`,
		// Whitelist rules - durable approvals for out-of-scope actions.
		`CREATE TABLE IF NOT EXISTS workspace_whitelist_rules (
			rule_id      TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
			scope        TEXT NOT NULL,
			action_types TEXT NOT NULL, -- JSON array
			status       TEXT NOT NULL CHECK (status IN ('active', 'revoked')),
			created_at   TEXT NOT NULL,
			revoked_at   TEXT,
			expires_at   TEXT,
			created_by   TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_wwr_workspace ON workspace_whitelist_rules(workspace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_wwr_status ON workspace_whitelist_rules(status)`,
		`CREATE INDEX IF NOT EXISTS idx_wwr_workspace_status ON workspace_whitelist_rules(workspace_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_wwr_expires_at ON workspace_whitelist_rules(expires_at)`,
		// Execution outcomes — structured command attempt history and failure model.
		`CREATE TABLE IF NOT EXISTS execution_outcomes (
				attempt_id            TEXT PRIMARY KEY,
				command_id            TEXT NOT NULL,
				attempt_number        INTEGER NOT NULL,
				retry_of              TEXT,
			command_type          TEXT NOT NULL,
			start_time            TEXT NOT NULL,
			end_time              TEXT,
			outcome               TEXT NOT NULL, -- "succeeded", "failed", "partially_succeeded", "cancelled", "timed_out", "rejected_pre_execution"
			failure_class         TEXT,
			failure_reason        TEXT,
			affected_entities     TEXT NOT NULL,
				retryable             INTEGER NOT NULL, -- 0 = false, 1 = true
				compensation_required INTEGER NOT NULL, -- 0 = false, 1 = true
				compensation_status   TEXT NOT NULL,    -- "not_required", "pending", "completed", "failed", "not_possible"
				recovery_status       TEXT NOT NULL,    -- "not_required", "open", "resolved"
				proposal_id           TEXT,
				run_id                TEXT,
				runtime_session_id    TEXT,
				correlation_id        TEXT,
				parent_run_id         TEXT,
				skill_ids             TEXT,
				connector_ids         TEXT,
				llm_provider          TEXT,
				llm_model             TEXT,
				llm_task_class        TEXT,
				llm_complexity        TEXT,
				workspace_id          TEXT,
				boundary_crossing     INTEGER DEFAULT 0,
				approval_required     INTEGER DEFAULT 0,
				approval_outcome      TEXT,
				artifact_id           TEXT
			)`,
		`CREATE INDEX IF NOT EXISTS idx_execution_outcomes_command_id ON execution_outcomes(command_id)`,
		`CREATE INDEX IF NOT EXISTS idx_execution_outcomes_outcome ON execution_outcomes(outcome)`,
		`CREATE TABLE IF NOT EXISTS error_log (
			id TEXT PRIMARY KEY,
			timestamp TEXT NOT NULL,
			severity TEXT NOT NULL,
			component TEXT NOT NULL,
			session_id TEXT,
			run_id TEXT,
			error_type TEXT NOT NULL,
			error_message TEXT NOT NULL,
			stack_trace TEXT,
			context_json TEXT,
			resolved_at TEXT,
			resolution TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_error_log_timestamp ON error_log(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_error_log_component_error_type ON error_log(component, error_type)`,
		`CREATE INDEX IF NOT EXISTS idx_error_log_session_id ON error_log(session_id)`,
		`CREATE TABLE IF NOT EXISTS llm_providers (
			provider_id TEXT PRIMARY KEY,
			display_name TEXT NOT NULL,
			hosting_mode TEXT NOT NULL,
			runtime_backend TEXT NOT NULL,
			api_base TEXT,
			auth_status TEXT,
			health_status TEXT,
			release_channel TEXT,
			provenance_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS llm_profiles (
			llm_id TEXT PRIMARY KEY,
			schema_version INTEGER NOT NULL DEFAULT 0,
			canonical_name TEXT NOT NULL,
			aliases_json TEXT,
			provider_id TEXT NOT NULL REFERENCES llm_providers(provider_id),
			display_name TEXT NOT NULL,
			availability_state TEXT NOT NULL,
			cost_estimate_json TEXT,
			provenance_json TEXT NOT NULL,
			mutation_history_json TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_llm_profiles_provider_id ON llm_profiles(provider_id)`,
		`CREATE TABLE IF NOT EXISTS llm_profile_capabilities (
			llm_id TEXT PRIMARY KEY REFERENCES llm_profiles(llm_id),
			data_json TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS llm_profile_features (
			llm_id TEXT PRIMARY KEY REFERENCES llm_profiles(llm_id),
			data_json TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS llm_profile_operational_state (
			llm_id TEXT PRIMARY KEY REFERENCES llm_profiles(llm_id),
			data_json TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS llm_profile_evaluation (
			llm_id TEXT PRIMARY KEY REFERENCES llm_profiles(llm_id),
			data_json TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS llm_profile_usage_stats (
			llm_id TEXT PRIMARY KEY REFERENCES llm_profiles(llm_id),
			data_json TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS llm_profile_routing (
			llm_id TEXT PRIMARY KEY REFERENCES llm_profiles(llm_id),
			data_json TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS llm_runtime_instances (
			instance_id TEXT PRIMARY KEY,
			llm_id TEXT NOT NULL REFERENCES llm_profiles(llm_id),
			provider_id TEXT NOT NULL REFERENCES llm_providers(provider_id),
			runtime_backend TEXT NOT NULL,
			endpoint TEXT,
			local_path TEXT,
			ollama_tag TEXT,
			auth_config_key TEXT,
			connector_id TEXT,
			loaded_status TEXT,
			health_status TEXT,
			auth_status TEXT,
			last_probe_at TEXT,
			quota_state_json TEXT,
			storage_size_bytes INTEGER,
			attributes_json TEXT,
			provenance_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS llm_execution_records (
			record_id TEXT PRIMARY KEY,
			llm_id TEXT NOT NULL REFERENCES llm_profiles(llm_id),
			instance_id TEXT,
			router_decision_id TEXT,
			task_class TEXT NOT NULL,
			task_id TEXT,
			context_size_tokens INTEGER,
			output_size_tokens INTEGER,
			latency_ms INTEGER,
			cost_usd REAL,
			outcome TEXT NOT NULL,
			failure_reason TEXT,
			tools_invoked_json TEXT,
			tool_success_rate REAL,
			schema_validated INTEGER,
			fallback_triggered INTEGER,
			fallback_model_id TEXT,
			user_rating INTEGER,
			user_override INTEGER,
			notes TEXT,
			attributes_json TEXT,
			provenance_json TEXT,
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_llm_execution_records_llm_id_created_at ON llm_execution_records(llm_id, created_at)`,
		`CREATE TABLE IF NOT EXISTS llm_evaluations (
			evaluation_id TEXT PRIMARY KEY,
			llm_id TEXT NOT NULL REFERENCES llm_profiles(llm_id),
			task_class TEXT,
			internal_eval_scores_json TEXT,
			failure_patterns_json TEXT,
			evaluated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS router_decisions (
			decision_id TEXT PRIMARY KEY,
			task_id TEXT,
			task_class TEXT NOT NULL,
			risk_tier TEXT,
			selected_llm_id TEXT,
			selected_instance_id TEXT,
			fallback_chain_json TEXT,
			candidate_set_json TEXT,
			scores_json TEXT,
			rejected_models_json TEXT,
			capability_filters_json TEXT,
			autonomy_constraints_json TEXT,
			cost_constraints_json TEXT,
			governance_constraints_json TEXT,
			confidence REAL,
			user_visible_summary TEXT,
			debug_rationale TEXT,
			surfacing_level TEXT,
			execution_record_id TEXT,
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_router_decisions_task_id_created_at ON router_decisions(task_id, created_at)`,
		`CREATE TABLE IF NOT EXISTS routing_proposal_items (
			proposal_id TEXT PRIMARY KEY,
			llm_id TEXT NOT NULL REFERENCES llm_profiles(llm_id),
			proposed_change TEXT NOT NULL,
			affected_field TEXT NOT NULL,
			current_value_json TEXT,
			proposed_value_json TEXT,
			inferred_score REAL,
			sample_size INTEGER,
			probation_elapsed_days INTEGER,
			recent_success_rate REAL,
			fallback_rate_delta REAL,
			routing_impact TEXT,
			risk_assessment TEXT,
			evidence_record_ids_json TEXT,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			resolved_at TEXT,
			resolved_by TEXT
		)`,
		// Entity relationships — typed edges between world-model entities with basic metadata.
		`CREATE TABLE IF NOT EXISTS entity_relationships (
			id               TEXT PRIMARY KEY,
			from_entity_id   TEXT NOT NULL,
			to_entity_id     TEXT NOT NULL,
			relationship_type TEXT NOT NULL,
			confidence       REAL NOT NULL,
			recency          TEXT NOT NULL,
			provenance       TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_entity_relationships_from_to ON entity_relationships(from_entity_id, to_entity_id)`,
		`CREATE INDEX IF NOT EXISTS idx_entity_relationships_type ON entity_relationships(relationship_type)`,
		// Tombstones — record lifecycle actions (archive, forget, supersede, tombstone).
		`CREATE TABLE IF NOT EXISTS tombstones (
			id           TEXT PRIMARY KEY,
			entity_type  TEXT NOT NULL,
			entity_id    TEXT NOT NULL,
			action       TEXT NOT NULL, -- "archive", "forget", "supersede", "tombstone"
			proposal_id  TEXT,
			deleted_at   TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tombstones_entity ON tombstones(entity_type, entity_id)`,
		// Fact supersession — Knowledge lifecycle: one fact supersedes another.
		`CREATE TABLE IF NOT EXISTS fact_supersessions (
			fact_id              TEXT PRIMARY KEY,
			superseded_by_fact_id TEXT NOT NULL,
			created_at           TEXT NOT NULL
		)`,
		// Entity provenance — canonical source, derivation_chain, mutation_history per entity.
		`CREATE TABLE IF NOT EXISTS entity_provenance (
			entity_type      TEXT NOT NULL,
			entity_id        TEXT NOT NULL,
			source           TEXT NOT NULL,
			timestamp        TEXT NOT NULL,
			confidence       REAL NOT NULL,
			derivation_chain TEXT,
			mutation_history TEXT,
			proposal_id      TEXT,
			PRIMARY KEY (entity_type, entity_id)
		)`,
		// Re-evaluation flags — downstream derivatives flagged when a Memory is Forgotten (design: Forget and downstream re-evaluation).
		`CREATE TABLE IF NOT EXISTS re_evaluation_flags (
			id TEXT PRIMARY KEY,
			entity_type TEXT NOT NULL,
			entity_id TEXT NOT NULL,
			reason TEXT NOT NULL,
			source_entity_type TEXT NOT NULL,
			source_entity_id TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_re_evaluation_flags_entity ON re_evaluation_flags(entity_type, entity_id)`,
		`CREATE VIEW IF NOT EXISTS history AS
			SELECT
				message_id AS id,
				'interaction' AS class,
				directive_id AS correlation_id,
				role AS source,
				created_at AS timestamp,
				content AS summary,
				'high' AS fidelity
			FROM directive_messages
			UNION ALL
			SELECT
				id AS id,
				'event' AS class,
				correlation_id AS correlation_id,
				source_agent AS source,
				timestamp AS timestamp,
				type || ': ' || substr(payload, 1, 100) AS summary,
				'medium' AS fidelity
			FROM events
			UNION ALL
			SELECT
				attempt_id AS id,
				'execution' AS class,
				command_id AS correlation_id,
				command_type AS source,
				start_time AS timestamp,
				outcome || ': ' || failure_reason AS summary,
				'high' AS fidelity
			FROM execution_outcomes`,
		// Provider health snapshots — ring buffer of health check results per provider.
		`CREATE TABLE IF NOT EXISTS provider_health_snapshots (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			provider    TEXT    NOT NULL,
			healthy     INTEGER NOT NULL DEFAULT 1,
			latency_ms  INTEGER NOT NULL DEFAULT 0,
			message     TEXT    DEFAULT '',
			model_count INTEGER DEFAULT 0,
			checked_at  TEXT    NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_health_provider ON provider_health_snapshots(provider, checked_at DESC)`,
		// Provider operations log — pull, delete, warm, copy, create actions.
		`CREATE TABLE IF NOT EXISTS provider_operations (
			id           TEXT    PRIMARY KEY,
			provider     TEXT    NOT NULL,
			action       TEXT    NOT NULL,
			model        TEXT    NOT NULL DEFAULT '',
			status       TEXT    NOT NULL DEFAULT 'pending',
			progress     REAL   NOT NULL DEFAULT 0.0,
			message      TEXT    DEFAULT '',
			error        TEXT    DEFAULT '',
			started_at   TEXT    NOT NULL DEFAULT (datetime('now')),
			completed_at TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ops_provider ON provider_operations(provider, started_at DESC)`,
		// Discovered model inventory — cached from provider ListModels.
		`CREATE TABLE IF NOT EXISTS discovered_models (
			provider         TEXT NOT NULL,
			name             TEXT NOT NULL,
			size_bytes       INTEGER DEFAULT 0,
			family           TEXT DEFAULT '',
			parameter_size   TEXT DEFAULT '',
			quantization     TEXT DEFAULT '',
			digest           TEXT DEFAULT '',
			modified_at      TEXT DEFAULT '',
			discovered_at    TEXT NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (provider, name)
		)`,
		// CIP P1: raw intake records before any World Model synthesis.
		`CREATE TABLE IF NOT EXISTS intake_records (
			id            TEXT NOT NULL PRIMARY KEY,
			connector_id  TEXT NOT NULL,
			source_kind   TEXT NOT NULL,
			source_id     TEXT NOT NULL,
			cursor        TEXT NOT NULL DEFAULT '',
			fetched_at    TEXT NOT NULL,
			trust         TEXT NOT NULL,
			privacy_class TEXT NOT NULL,
			author        TEXT NOT NULL DEFAULT '',
			raw           BLOB NOT NULL DEFAULT '',
			raw_mime      TEXT NOT NULL DEFAULT 'text/plain',
			provenance    TEXT NOT NULL DEFAULT '{}',
			created_at    TEXT NOT NULL,
			UNIQUE (connector_id, source_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_intake_records_connector_source
			ON intake_records (connector_id, source_id)`,
		// CIP P2: distilled, provenance-bearing chunks derived from intake_records
		// (Canonicalize → Chunk → Distill). No World Model writes happen here.
		`CREATE TABLE IF NOT EXISTS intake_chunks (
			id               TEXT NOT NULL PRIMARY KEY,
			intake_record_id TEXT NOT NULL REFERENCES intake_records(id),
			connector_id     TEXT NOT NULL,
			source_id        TEXT NOT NULL,
			chunk_index      INTEGER NOT NULL,
			parent_chunk_id  TEXT NOT NULL DEFAULT '',
			content          TEXT NOT NULL,
			content_mime     TEXT NOT NULL DEFAULT 'text/markdown',
			start_offset     INTEGER NOT NULL DEFAULT 0,
			end_offset       INTEGER NOT NULL DEFAULT 0,
			token_estimate   INTEGER NOT NULL DEFAULT 0,
			trust            TEXT NOT NULL DEFAULT '',
			privacy_class    TEXT NOT NULL DEFAULT '',
			provenance       TEXT NOT NULL DEFAULT '{}',
			created_at       TEXT NOT NULL,
			UNIQUE (intake_record_id, chunk_index)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_intake_chunks_record
			ON intake_chunks (intake_record_id)`,
		`CREATE INDEX IF NOT EXISTS idx_intake_chunks_connector_source
			ON intake_chunks (connector_id, source_id)`,
		// CIP P3: persisted embedding references for intake chunks (stage 7).
		// The vector index engine is deferred — this is a reference, not an index.
		`CREATE TABLE IF NOT EXISTS intake_embeddings (
			chunk_id   TEXT NOT NULL PRIMARY KEY REFERENCES intake_chunks(id),
			model      TEXT NOT NULL,
			dim        INTEGER NOT NULL DEFAULT 0,
			vector     TEXT NOT NULL DEFAULT '[]',
			created_at TEXT NOT NULL
		)`,
		// CIP P3: provenance + idempotency spine linking intake records/chunks to
		// the World Model entities synthesis produced (stage 8). derivation_key is
		// UNIQUE so a replayed synthesis pass is a no-op (no duplicate entities or
		// proposals). entity_id is empty when the synthesis raised a Proposal.
		`CREATE TABLE IF NOT EXISTS intake_provenance (
			id               TEXT NOT NULL PRIMARY KEY,
			intake_record_id TEXT NOT NULL,
			chunk_id         TEXT NOT NULL DEFAULT '',
			connector_id     TEXT NOT NULL DEFAULT '',
			source_id        TEXT NOT NULL DEFAULT '',
			cursor           TEXT NOT NULL DEFAULT '',
			fetched_at       TEXT NOT NULL,
			entity_type      TEXT NOT NULL DEFAULT '',
			entity_id        TEXT NOT NULL DEFAULT '',
			mutation_kind    TEXT NOT NULL DEFAULT '',
			outcome          TEXT NOT NULL DEFAULT '',
			derivation_key   TEXT NOT NULL UNIQUE,
			confidence       REAL NOT NULL DEFAULT 0,
			proposal_id      TEXT NOT NULL DEFAULT '',
			created_at       TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_intake_provenance_record
			ON intake_provenance (intake_record_id)`,
		`CREATE INDEX IF NOT EXISTS idx_intake_provenance_entity
			ON intake_provenance (entity_type, entity_id)`,
		// CIP P3: Fold stage summary index (stage 9). Flat by-entity-type rollup;
		// DERIVED state, never authoritative — rebuildable from provenance.
		`CREATE TABLE IF NOT EXISTS intake_summary (
			entity_type    TEXT NOT NULL PRIMARY KEY,
			entity_count   INTEGER NOT NULL DEFAULT 0,
			last_entity_id TEXT,
			last_summary   TEXT,
			updated_at     TEXT NOT NULL
		)`,
		// CIP P5: per-connector sync policy runtime OVERRIDES (CIP §7). The full
		// SyncPolicy is stored as JSON keyed by connector id; defaults live in code
		// / config YAML. This is configuration, not World Model state.
		`CREATE TABLE IF NOT EXISTS connector_sync_policy (
			connector_id TEXT NOT NULL PRIMARY KEY,
			policy       TEXT NOT NULL,
			created_at   TEXT NOT NULL,
			updated_at   TEXT NOT NULL
		)`,
		// CIP P5: append-only intake sync log — one row per connector pass
		// (CIP §7, §11). Populated by the intake worker.
		`CREATE TABLE IF NOT EXISTS intake_sync_log (
			id                  TEXT NOT NULL PRIMARY KEY,
			connector_id        TEXT NOT NULL,
			parent_id           TEXT NOT NULL DEFAULT '',
			job_mode            TEXT NOT NULL DEFAULT 'delta',
			started_at          TEXT NOT NULL,
			ended_at            TEXT,
			records_admitted    INTEGER NOT NULL DEFAULT 0,
			records_deduped     INTEGER NOT NULL DEFAULT 0,
			records_distilled   INTEGER NOT NULL DEFAULT 0,
			records_synthesized INTEGER NOT NULL DEFAULT 0,
			errors              INTEGER NOT NULL DEFAULT 0,
			terminal_status     TEXT NOT NULL DEFAULT '',
			note                TEXT NOT NULL DEFAULT '',
			created_at          TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_intake_sync_log_connector
			ON intake_sync_log (connector_id, created_at DESC)`,
		// CIP P5: append-only Vault sync log — one row per per-file diff pass
		// (Vault §13). SCHEMA ONLY in P5; the Memory Vault deliverable populates it.
		`CREATE TABLE IF NOT EXISTS vault_sync_log (
			id                 TEXT NOT NULL PRIMARY KEY,
			file_path          TEXT NOT NULL,
			started_at         TEXT NOT NULL,
			ended_at           TEXT,
			diff_summary       TEXT NOT NULL DEFAULT '',
			mutations_proposed INTEGER NOT NULL DEFAULT 0,
			mutations_approved INTEGER NOT NULL DEFAULT 0,
			proposals_raised   INTEGER NOT NULL DEFAULT 0,
			errors             INTEGER NOT NULL DEFAULT 0,
			terminal_status    TEXT NOT NULL DEFAULT '',
			created_at         TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_vault_sync_log_path
			ON vault_sync_log (file_path, created_at DESC)`,
		// NAVI-VAULT-V1: file ↔ entity mapping + last-projected content hash. This
		// is projector bookkeeping (which file renders which entity, and the hash
		// of the last projector-owned bytes written) for drift detection and to
		// distinguish owner edits from the projector's own writes. It is NOT
		// authority — entities remain truth; this table is rebuildable from the
		// World Model at any time (a periodic drift sweep does exactly that).
		`CREATE TABLE IF NOT EXISTS vault_state (
			file_path        TEXT NOT NULL PRIMARY KEY,
			entity_type      TEXT NOT NULL,
			entity_id        TEXT NOT NULL,
			projected_hash   TEXT NOT NULL DEFAULT '',
			projected_at     TEXT NOT NULL,
			created_at       TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_vault_state_entity
			ON vault_state (entity_type, entity_id)`,
	}
	for _, s := range ddl {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("store: create tables: %w", err)
		}
	}

	if err := addColumnIfNotExists(ctx, db, "agents", "last_heartbeat", "TEXT"); err != nil {
		return fmt.Errorf("store: migrate agents.last_heartbeat: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "llm_profiles", "schema_version", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("store: migrate llm_profiles.schema_version: %w", err)
	}
	// Phase 8 + 9 auto-migrations
	if err := addColumnIfNotExists(ctx, db, "users", "api_token", "TEXT"); err != nil {
		return fmt.Errorf("store: migrate users.api_token: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "entity_provenance", "reinforcement_count", "INTEGER DEFAULT 0"); err != nil {
		return fmt.Errorf("store: migrate entity_provenance.reinforcement_count: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "agent_identities", "revoked_at", "TEXT"); err != nil {
		return fmt.Errorf("store: migrate agent_identities.revoked_at: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "agent_identities", "revocation_reason", "TEXT"); err != nil {
		return fmt.Errorf("store: migrate agent_identities.revocation_reason: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "agent_identities", "supersedes_id", "TEXT"); err != nil {
		return fmt.Errorf("store: migrate agent_identities.supersedes_id: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "agent_identities", "superseded_by_id", "TEXT"); err != nil {
		return fmt.Errorf("store: migrate agent_identities.superseded_by_id: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "agent_identities", "rotation_link_sig", "TEXT"); err != nil {
		return fmt.Errorf("store: migrate agent_identities.rotation_link_sig: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "facts", "deprecated", "INTEGER DEFAULT 0"); err != nil {
		return fmt.Errorf("store: migrate facts.deprecated: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "facts", "keywords", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return fmt.Errorf("store: migrate facts.keywords: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "facts", "tags", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return fmt.Errorf("store: migrate facts.tags: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "facts", "embedding", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return fmt.Errorf("store: migrate facts.embedding: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "artifacts", "workspace_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("store: migrate artifacts.workspace_id: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "artifacts", "project_id", "TEXT"); err != nil {
		return fmt.Errorf("store: migrate artifacts.project_id: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "artifacts", "owner_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("store: migrate artifacts.owner_id: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "artifacts", "subtype", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("store: migrate artifacts.subtype: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "artifacts", "status", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("store: migrate artifacts.status: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "artifacts", "render_status", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("store: migrate artifacts.render_status: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "artifacts", "version", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("store: migrate artifacts.version: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "artifacts", "source_tool", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("store: migrate artifacts.source_tool: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "artifacts", "source_execution_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("store: migrate artifacts.source_execution_id: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "artifacts", "source_run_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("store: migrate artifacts.source_run_id: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "artifacts", "last_error", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("store: migrate artifacts.last_error: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "artifacts", "recoverable_output", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("store: migrate artifacts.recoverable_output: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "artifacts", "metadata", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("store: migrate artifacts.metadata: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "memories", "keywords", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return fmt.Errorf("store: migrate memories.keywords: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "memories", "tags", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return fmt.Errorf("store: migrate memories.tags: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "memories", "embedding", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return fmt.Errorf("store: migrate memories.embedding: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "events", "run_id", "TEXT"); err != nil {
		return fmt.Errorf("store: migrate events.run_id: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "events", "visibility", "TEXT"); err != nil {
		return fmt.Errorf("store: migrate events.visibility: %w", err)
	}
	// Runs API correlation fields on execution_outcomes
	if err := renameColumnIfExists(ctx, db, "execution_outcomes", "session_id", "runtime_session_id"); err != nil {
		return fmt.Errorf("store: rename execution_outcomes.session_id: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "execution_outcomes", "runtime_session_id", "TEXT"); err != nil {
		return fmt.Errorf("store: migrate execution_outcomes.runtime_session_id: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "execution_outcomes", "run_id", "TEXT"); err != nil {
		return fmt.Errorf("store: migrate execution_outcomes.run_id: %w", err)
	}
	// Workspace V1 Enforcement Audit Fields (OMN-99, OMN-108)
	if err := addColumnIfNotExists(ctx, db, "execution_outcomes", "workspace_id", "TEXT"); err != nil {
		return fmt.Errorf("store: migrate execution_outcomes.workspace_id: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "execution_outcomes", "boundary_crossing", "INTEGER DEFAULT 0"); err != nil {
		return fmt.Errorf("store: migrate execution_outcomes.boundary_crossing: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "execution_outcomes", "approval_required", "INTEGER DEFAULT 0"); err != nil {
		return fmt.Errorf("store: migrate execution_outcomes.approval_required: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "execution_outcomes", "approval_outcome", "TEXT"); err != nil {
		return fmt.Errorf("store: migrate execution_outcomes.approval_outcome: %w", err)
	}
	// Proposal ResolutionKind (OMN-104)
	if err := addColumnIfNotExists(ctx, db, "proposals", "resolution_type", "TEXT"); err != nil {
		return fmt.Errorf("store: migrate proposals.resolution_type: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "execution_outcomes", "correlation_id", "TEXT"); err != nil {
		return fmt.Errorf("store: migrate execution_outcomes.correlation_id: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "execution_outcomes", "parent_run_id", "TEXT"); err != nil {
		return fmt.Errorf("store: migrate execution_outcomes.parent_run_id: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "execution_outcomes", "skill_ids", "TEXT"); err != nil {
		return fmt.Errorf("store: migrate execution_outcomes.skill_ids: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "execution_outcomes", "connector_ids", "TEXT"); err != nil {
		return fmt.Errorf("store: migrate execution_outcomes.connector_ids: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "execution_outcomes", "llm_provider", "TEXT"); err != nil {
		return fmt.Errorf("store: migrate execution_outcomes.llm_provider: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "execution_outcomes", "llm_model", "TEXT"); err != nil {
		return fmt.Errorf("store: migrate execution_outcomes.llm_model: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "execution_outcomes", "llm_task_class", "TEXT"); err != nil {
		return fmt.Errorf("store: migrate execution_outcomes.llm_task_class: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "execution_outcomes", "llm_complexity", "TEXT"); err != nil {
		return fmt.Errorf("store: migrate execution_outcomes.llm_complexity: %w", err)
	}
	// NOTE: workspace_id, boundary_crossing, approval_required, approval_outcome
	// for execution_outcomes are already migrated above (lines 814-825).
	if err := addColumnIfNotExists(ctx, db, "owners", "timezone", "TEXT NOT NULL DEFAULT 'UTC'"); err != nil {
		return fmt.Errorf("store: migrate owners.timezone: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "projects", "icon", "TEXT NOT NULL DEFAULT 'folder'"); err != nil {
		return fmt.Errorf("store: migrate projects.icon: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "projects", "color", "TEXT NOT NULL DEFAULT '#3b82f6'"); err != nil {
		return fmt.Errorf("store: migrate projects.color: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "projects", "memory_scope", "TEXT NOT NULL DEFAULT 'default'"); err != nil {
		return fmt.Errorf("store: migrate projects.memory_scope: %w", err)
	}
	if err := renameColumnIfExists(ctx, db, "experience_preference_signals", "session_id", "chat_id"); err != nil {
		return fmt.Errorf("store: rename experience_preference_signals.session_id: %w", err)
	}
	if err := addColumnIfNotExists(ctx, db, "experience_preference_signals", "chat_id", "TEXT"); err != nil {
		return fmt.Errorf("store: migrate experience_preference_signals.chat_id: %w", err)
	}
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS idx_artifacts_workspace ON artifacts(workspace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_artifacts_project ON artifacts(project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_artifacts_owner_id ON artifacts(owner_id)`,
		`CREATE INDEX IF NOT EXISTS idx_experience_preference_signals_chat ON experience_preference_signals(chat_id, created_at)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("store: create artifact index %q: %w", stmt, err)
		}
	}
	return nil
}
