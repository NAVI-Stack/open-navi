package store

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSchemaSecurity(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Test case 1: Table and column names with spaces and reserved words
	tableWithSpaces := "my table"
	columnWithSpaces := "my column"

	_, err = db.ExecContext(ctx, "CREATE TABLE "+quoteIdentifier(tableWithSpaces)+" (id TEXT PRIMARY KEY)")
	if err != nil {
		t.Fatalf("failed to create table with spaces: %v", err)
	}

	err = addColumnIfNotExists(ctx, db, tableWithSpaces, columnWithSpaces, "TEXT")
	if err != nil {
		t.Errorf("addColumnIfNotExists failed with spaces: %v", err)
	}

	exists, err := columnExists(ctx, db, tableWithSpaces, columnWithSpaces)
	if err != nil {
		t.Errorf("columnExists failed with spaces: %v", err)
	}
	if !exists {
		t.Errorf("expected column %q to exist in table %q", columnWithSpaces, tableWithSpaces)
	}

	// Test case 2: Attempted injection in column definition (semicolon)
	err = addColumnIfNotExists(ctx, db, tableWithSpaces, "another_col", "TEXT; DROP TABLE "+quoteIdentifier(tableWithSpaces))
	if err == nil {
		t.Errorf("expected error when column definition contains semicolon, but got nil")
	}

	// Verify table still exists
	var tableName string
	err = db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name=?", tableWithSpaces).Scan(&tableName)
	if err != nil {
		t.Errorf("table should still exist after attempted injection: %v", err)
	}

	// Test case 3: Column names with quotes
	quotedColumn := "column\"with\"quotes"
	err = addColumnIfNotExists(ctx, db, tableWithSpaces, quotedColumn, "TEXT")
	if err != nil {
		t.Errorf("addColumnIfNotExists failed with quotes in column name: %v", err)
	}

	exists, err = columnExists(ctx, db, tableWithSpaces, quotedColumn)
	if err != nil {
		t.Errorf("columnExists failed with quotes in column name: %v", err)
	}
	if !exists {
		t.Errorf("expected column %q to exist", quotedColumn)
	}

	// Test case 4: Rename column with tricky names
	newColumnName := "new column name"
	err = renameColumnIfNeeded(ctx, db, tableWithSpaces, quotedColumn, newColumnName)
	if err != nil {
		t.Errorf("renameColumnIfNeeded failed: %v", err)
	}

	exists, err = columnExists(ctx, db, tableWithSpaces, newColumnName)
	if err != nil || !exists {
		t.Errorf("expected new column %q to exist", newColumnName)
	}

	exists, err = columnExists(ctx, db, tableWithSpaces, quotedColumn)
	if err != nil || exists {
		t.Errorf("expected old column %q to be gone", quotedColumn)
	}
}

func TestQuoteIdentifier(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", `"simple"`},
		{"table name", `"table name"`},
		{`table"with"quotes`, `"table""with""quotes"`},
		{`O'Reilly`, `"O'Reilly"`},
	}

	for _, tc := range tests {
		actual := quoteIdentifier(tc.input)
		if actual != tc.expected {
			t.Errorf("quoteIdentifier(%q) = %q; want %q", tc.input, actual, tc.expected)
		}
	}
}
