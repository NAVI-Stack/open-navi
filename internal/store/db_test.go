package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenAndCreateTables(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables idempotent: %v", err)
	}
	if got := db.Stats().MaxOpenConnections; got != sqliteMaxOpenConns {
		t.Fatalf("MaxOpenConnections = %d, want %d", got, sqliteMaxOpenConns)
	}
}

func TestOpenInMemoryUsesSingleConnection(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if got := db.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("MaxOpenConnections for :memory: = %d, want 1", got)
	}
}

func TestCreateTables_CreatesRequiredLLMKBIndexes(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "store-indexes.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables second pass: %v", err)
	}
	required := map[string]bool{
		"idx_llm_profiles_provider_id":                false,
		"idx_llm_execution_records_llm_id_created_at": false,
		"idx_router_decisions_task_id_created_at":     false,
	}
	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'index'`)
	if err != nil {
		t.Fatalf("query indexes: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan index: %v", err)
		}
		if _, ok := required[name]; ok {
			required[name] = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	for name, found := range required {
		if !found {
			t.Fatalf("required index %s not found", name)
		}
	}
}

func TestCreateTables_CreatesWorkspaceSchemaAndLifecycleIndexes(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "workspace-schema.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables second pass: %v", err)
	}

	requiredIndexes := map[string]bool{
		"idx_workspaces_status":                    false,
		"idx_workspaces_kind":                      false,
		"idx_workspaces_related_project_id":        false,
		"idx_workspaces_active_related_project_id": false,
		"idx_wwr_workspace":                        false,
		"idx_wwr_status":                           false,
		"idx_wwr_workspace_status":                 false,
		"idx_wwr_expires_at":                       false,
	}

	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'index'`)
	if err != nil {
		t.Fatalf("query indexes: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan index: %v", err)
		}
		if _, ok := requiredIndexes[name]; ok {
			requiredIndexes[name] = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	for name, found := range requiredIndexes {
		if !found {
			t.Fatalf("required workspace index %s not found", name)
		}
	}

	requiredColumns := map[string]bool{
		"rule_id":      false,
		"workspace_id": false,
		"scope":        false,
		"action_types": false,
		"status":       false,
		"created_at":   false,
		"revoked_at":   false,
		"expires_at":   false,
		"created_by":   false,
	}
	columnRows, err := db.QueryContext(ctx, `PRAGMA table_info(workspace_whitelist_rules)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info(workspace_whitelist_rules): %v", err)
	}
	defer columnRows.Close()
	for columnRows.Next() {
		var (
			cid        int
			name       string
			typ        string
			notNull    int
			defaultV   any
			primaryKey int
		)
		if err := columnRows.Scan(&cid, &name, &typ, &notNull, &defaultV, &primaryKey); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		if _, ok := requiredColumns[name]; ok {
			requiredColumns[name] = true
		}
	}
	if err := columnRows.Err(); err != nil {
		t.Fatalf("column rows: %v", err)
	}
	for name, found := range requiredColumns {
		if !found {
			t.Fatalf("required workspace_whitelist_rules column %s not found", name)
		}
	}
}

func TestCreateTables_CreatesProjectSchemaAndIndexes(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "project-schema.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables second pass: %v", err)
	}

	requiredIndexes := map[string]bool{
		"idx_projects_slug":               false,
		"idx_projects_status":             false,
		"idx_projects_workspace_id":       false,
		"idx_projects_sandbox_profile_id": false,
	}
	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'index'`)
	if err != nil {
		t.Fatalf("query indexes: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan index: %v", err)
		}
		if _, ok := requiredIndexes[name]; ok {
			requiredIndexes[name] = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	for name, found := range requiredIndexes {
		if !found {
			t.Fatalf("required project index %s not found", name)
		}
	}

	requiredColumns := map[string]bool{
		"id":                 false,
		"title":              false,
		"slug":               false,
		"description":        false,
		"project_kind":       false,
		"status":             false,
		"health":             false,
		"workspace_id":       false,
		"sandbox_profile_id": false,
		"created_at":         false,
		"updated_at":         false,
		"created_by":         false,
		"attributes":         false,
	}
	columnRows, err := db.QueryContext(ctx, `PRAGMA table_info(projects)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info(projects): %v", err)
	}
	defer columnRows.Close()
	for columnRows.Next() {
		var (
			cid        int
			name       string
			typ        string
			notNull    int
			defaultV   any
			primaryKey int
		)
		if err := columnRows.Scan(&cid, &name, &typ, &notNull, &defaultV, &primaryKey); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		if _, ok := requiredColumns[name]; ok {
			requiredColumns[name] = true
		}
	}
	if err := columnRows.Err(); err != nil {
		t.Fatalf("column rows: %v", err)
	}
	for name, found := range requiredColumns {
		if !found {
			t.Fatalf("required projects column %s not found", name)
		}
	}
}

func TestCreateTables_CreatesSandboxProfileSchemaAndIndexes(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "sandbox-profile-schema.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables second pass: %v", err)
	}

	requiredIndexes := map[string]bool{
		"idx_sandbox_profiles_status":  false,
		"idx_sandbox_profiles_runtime": false,
	}
	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'index'`)
	if err != nil {
		t.Fatalf("query indexes: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan index: %v", err)
		}
		if _, ok := requiredIndexes[name]; ok {
			requiredIndexes[name] = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	for name, found := range requiredIndexes {
		if !found {
			t.Fatalf("required sandbox profile index %s not found", name)
		}
	}

	requiredColumns := map[string]bool{
		"id":                     false,
		"name":                   false,
		"description":            false,
		"runtime":                false,
		"status":                 false,
		"image":                  false,
		"network_mode":           false,
		"workspace_mount_target": false,
		"command_allowlist":      false,
		"env_allowlist":          false,
		"default_timeout_ms":     false,
		"max_timeout_ms":         false,
		"cpu_limit":              false,
		"memory_limit":           false,
		"created_at":             false,
		"updated_at":             false,
		"created_by":             false,
		"metadata":               false,
	}
	columnRows, err := db.QueryContext(ctx, `PRAGMA table_info(sandbox_profiles)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info(sandbox_profiles): %v", err)
	}
	defer columnRows.Close()
	for columnRows.Next() {
		var (
			cid        int
			name       string
			typ        string
			notNull    int
			defaultV   any
			primaryKey int
		)
		if err := columnRows.Scan(&cid, &name, &typ, &notNull, &defaultV, &primaryKey); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		if _, ok := requiredColumns[name]; ok {
			requiredColumns[name] = true
		}
	}
	if err := columnRows.Err(); err != nil {
		t.Fatalf("column rows: %v", err)
	}
	for name, found := range requiredColumns {
		if !found {
			t.Fatalf("required sandbox_profiles column %s not found", name)
		}
	}
}

func TestCreateTables_CreatesProjectTaskColumnsAndIndexes(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "project-task-schema.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables second pass: %v", err)
	}

	requiredIndexes := map[string]bool{
		"idx_tasks_project":        false,
		"idx_tasks_project_status": false,
	}
	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'index'`)
	if err != nil {
		t.Fatalf("query indexes: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan index: %v", err)
		}
		if _, ok := requiredIndexes[name]; ok {
			requiredIndexes[name] = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	for name, found := range requiredIndexes {
		if !found {
			t.Fatalf("required task index %s not found", name)
		}
	}

	requiredColumns := map[string]bool{
		"project_id":        false,
		"workspace_id":      false,
		"task_class":        false,
		"raw_input":         false,
		"acceptance_target": false,
		"lifecycle_phase":   false,
		"block_reason":      false,
	}
	columnRows, err := db.QueryContext(ctx, `PRAGMA table_info(tasks)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info(tasks): %v", err)
	}
	defer columnRows.Close()
	for columnRows.Next() {
		var (
			cid        int
			name       string
			typ        string
			notNull    int
			defaultV   any
			primaryKey int
		)
		if err := columnRows.Scan(&cid, &name, &typ, &notNull, &defaultV, &primaryKey); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		if _, ok := requiredColumns[name]; ok {
			requiredColumns[name] = true
		}
	}
	if err := columnRows.Err(); err != nil {
		t.Fatalf("column rows: %v", err)
	}
	for name, found := range requiredColumns {
		if !found {
			t.Fatalf("required tasks column %s not found", name)
		}
	}
}

func TestCreateTables_RejectsDuplicateActiveProjectBindings(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "workspace-duplicate-bindings.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE workspaces (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			kind TEXT NOT NULL CHECK (kind IN ('general', 'project', 'repository', 'functional')),
			status TEXT NOT NULL CHECK (status IN ('active', 'inactive', 'archived', 'suspended')),
			local_roots TEXT NOT NULL,
			repo_roots TEXT NOT NULL,
			protected_paths TEXT NOT NULL,
			allowed_actions TEXT NOT NULL,
			boundary_policy TEXT NOT NULL,
			audit_enabled INTEGER NOT NULL CHECK (audit_enabled IN (0, 1)),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			created_by TEXT NOT NULL,
			tags TEXT NOT NULL DEFAULT '[]',
			related_project_id TEXT,
			notes TEXT,
			metadata TEXT NOT NULL DEFAULT '{}'
		)`); err != nil {
		t.Fatalf("create workspaces: %v", err)
	}
	for _, id := range []string{"ws-1", "ws-2"} {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO workspaces (
				id, name, kind, status, local_roots, repo_roots, protected_paths,
				allowed_actions, boundary_policy, audit_enabled, created_at, updated_at, created_by,
				tags, related_project_id, metadata
			) VALUES (
				?, ?, 'project', 'active', '[]', '[]', '[]',
				'{"read":true,"write":true,"create":true,"modify":true,"rename_move":true,"delete":false,"execute":false}',
				'{"out_of_scope_default":"prompt"}', 1, '2026-03-29T12:00:00Z', '2026-03-29T12:00:00Z', 'owner',
				'[]', 'proj-dup', '{}'
			)`, id, id); err != nil {
			t.Fatalf("insert duplicate active project binding %s: %v", id, err)
		}
	}

	err = CreateTables(ctx, db)
	if err == nil {
		t.Fatal("expected duplicate active project bindings to be rejected")
	}
	if !strings.Contains(err.Error(), "duplicate active workspace project bindings") {
		t.Fatalf("expected duplicate active workspace binding error, got %v", err)
	}
}

func TestCreateTables_MigratesLegacyWorkspaceTables(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "workspace-legacy.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE workspaces (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			kind TEXT NOT NULL,
			status TEXT NOT NULL,
			local_roots TEXT NOT NULL,
			repo_roots TEXT NOT NULL,
			protected_paths TEXT NOT NULL,
			allowed_actions TEXT NOT NULL,
			boundary_policy TEXT NOT NULL,
			audit_enabled INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			created_by TEXT NOT NULL,
			tags TEXT NOT NULL DEFAULT '[]',
			related_project_id TEXT,
			notes TEXT,
			metadata TEXT NOT NULL DEFAULT '{}'
		)`); err != nil {
		t.Fatalf("create legacy workspaces: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE workspace_whitelist_rules (
			rule_id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL REFERENCES workspaces(id),
			scope TEXT NOT NULL,
			action_types TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			revoked_at TEXT,
			expires_at TEXT,
			created_by TEXT NOT NULL
		)`); err != nil {
		t.Fatalf("create legacy workspace_whitelist_rules: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO workspaces (
			id, name, description, kind, status, local_roots, repo_roots, protected_paths,
			allowed_actions, boundary_policy, audit_enabled, created_at, updated_at, created_by,
			tags, related_project_id, notes, metadata
		) VALUES (
			'ws-legacy', 'Legacy', 'Old workspace', 'project', 'active', '[]', '[]', '[]',
			'{"read":true,"write":true,"create":true,"modify":true,"rename_move":true,"delete":false,"execute":false}',
			'{"out_of_scope_default":"prompt"}', 1, '2026-03-29T12:00:00Z', '2026-03-29T12:00:00Z', 'owner',
			'[]', 'proj-legacy', 'legacy notes', '{}'
		)`); err != nil {
		t.Fatalf("insert legacy workspace: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO workspace_whitelist_rules (
			rule_id, workspace_id, scope, action_types, status, created_at, created_by
		) VALUES (
			'rule-legacy', 'ws-legacy', '/tmp/outside', '["read"]', 'active', '2026-03-29T12:00:00Z', 'owner'
		)`); err != nil {
		t.Fatalf("insert legacy workspace whitelist rule: %v", err)
	}

	if err := CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}

	workspace, err := GetWorkspace(ctx, db, "ws-legacy")
	if err != nil {
		t.Fatalf("GetWorkspace after migration: %v", err)
	}
	if workspace.RelatedProjectID != "proj-legacy" {
		t.Fatalf("expected related_project_id to survive migration, got %q", workspace.RelatedProjectID)
	}
	if len(workspace.WhitelistRules) != 1 || workspace.WhitelistRules[0].RuleID != "rule-legacy" {
		t.Fatalf("expected whitelist rule to survive migration, got %#v", workspace.WhitelistRules)
	}

	var workspacesSQL string
	if err := db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'workspaces'`).Scan(&workspacesSQL); err != nil {
		t.Fatalf("lookup migrated workspaces sql: %v", err)
	}
	if !strings.Contains(workspacesSQL, "CHECK (kind IN ('general', 'project', 'repository', 'functional'))") {
		t.Fatalf("expected migrated workspaces table to include kind check, got %s", workspacesSQL)
	}

	rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_list(workspace_whitelist_rules)`)
	if err != nil {
		t.Fatalf("PRAGMA foreign_key_list(workspace_whitelist_rules): %v", err)
	}
	defer rows.Close()
	foundCascade := false
	for rows.Next() {
		var (
			id       int
			seq      int
			table    string
			from     string
			to       string
			onUpdate string
			onDelete string
			match    string
		)
		if err := rows.Scan(&id, &seq, &table, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			t.Fatalf("scan foreign key: %v", err)
		}
		if table == "workspaces" && onDelete == "CASCADE" {
			foundCascade = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("foreign key rows: %v", err)
	}
	if !foundCascade {
		t.Fatal("expected migrated workspace_whitelist_rules to enforce ON DELETE CASCADE")
	}
}
