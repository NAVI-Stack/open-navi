package store

import (
	"context"
	"database/sql"
	"testing"
)

// InitTestDB creates an in-memory SQLite database with all tables.
// Exported for use by other packages' test files.
func InitTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := CreateTables(context.Background(), db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	return db
}
