package store

import (
	"context"
	"database/sql"
)

// GetSetting retrieves a setting by key.
// Returns found=false (and no error) if the key does not exist.
func GetSetting(ctx context.Context, db *sql.DB, key string) (value string, found bool, err error) {
	row := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key)
	if scanErr := row.Scan(&value); scanErr == sql.ErrNoRows {
		return "", false, nil
	} else if scanErr != nil {
		return "", false, scanErr
	}
	return value, true, nil
}

// SetSetting inserts or updates a setting key/value pair.
func SetSetting(ctx context.Context, db *sql.DB, key, value string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO settings(key, value, updated_at) VALUES(?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=CURRENT_TIMESTAMP`,
		key, value,
	)
	return err
}

// GetAllSettings returns all settings as a key→value map.
func GetAllSettings(ctx context.Context, db *sql.DB) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		m[k] = v
	}
	return m, rows.Err()
}
