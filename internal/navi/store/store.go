package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ceoai/navi/internal/schema"
)

// SQLiteStore is the single concrete SQLite-backed store that implements
// navi.ChatStore, navi.RuntimeSessionStore, runtime.Store, the compaction
// store interface, and ICS state storage. Method receivers live in the
// per-concern files (chat_store.go, runtime_session_store.go, runtime_store.go,
// compaction_store.go, ics_state.go).
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore constructs a new SQLiteStore backed by db.
func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

// GetMeta reads a key from the shared navi_meta table.
func (s *SQLiteStore) GetMeta(ctx context.Context, key string) (string, error) {
	var val string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM navi_meta WHERE key = ?`, key).Scan(&val)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("navi: read meta %s: %w", key, err)
	}
	return val, nil
}

// SetMeta upserts a key-value pair in the shared navi_meta table.
func (s *SQLiteStore) SetMeta(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO navi_meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	if err != nil {
		return fmt.Errorf("navi: set meta %s: %w", key, err)
	}
	return nil
}

// LookupRuntimeSessionKind returns the runtime-session kind associated with the
// given runtime_session_id, or a default heuristic if no record exists.
func (s *SQLiteStore) LookupRuntimeSessionKind(ctx context.Context, runtimeSessionID string) (schema.RuntimeSessionKind, error) {
	var kind sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT kind FROM runtime_sessions WHERE runtime_session_id = ?`,
		runtimeSessionID,
	).Scan(&kind)
	switch err {
	case nil:
		return schema.ResolveRuntimeSessionKind(kind.String, runtimeSessionID), nil
	case sql.ErrNoRows:
		return schema.DefaultRuntimeSessionKindForID(runtimeSessionID), nil
	default:
		return schema.RuntimeSessionKindUser, fmt.Errorf("navi: lookup runtime session kind: %w", err)
	}
}
