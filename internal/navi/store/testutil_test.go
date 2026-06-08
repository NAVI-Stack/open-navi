package store

import (
	"context"
	"database/sql"
	"testing"

	corestore "github.com/open-navi/navi/internal/store"
	_ "modernc.org/sqlite" // pure-Go sqlite driver for tests (no CGO)
)

func prepareTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}
	if err := corestore.CreateTables(context.Background(), db); err != nil {
		t.Fatalf("failed to create core tables: %v", err)
	}
	if err := MigrateSchema(context.Background(), db); err != nil {
		t.Fatalf("failed to migrate schema: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
