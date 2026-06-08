package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

func SaveDirective(ctx context.Context, db *sql.DB, d schema.Directive) error {
	if err := d.Validate(); err != nil {
		return fmt.Errorf("store: invalid directive: %w", err)
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO directives (directive_id, title, mode, status, created_via, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(directive_id) DO UPDATE SET
			title=excluded.title,
			mode=excluded.mode,
			status=excluded.status,
			updated_at=excluded.updated_at
	`, d.DirectiveID, d.Title, string(d.Mode), string(d.Status), d.CreatedVia,
		d.CreatedAt.Format(timeFormat), d.UpdatedAt.Format(timeFormat))
	if err != nil {
		return fmt.Errorf("store: save directive: %w", err)
	}
	return nil
}

func GetDirective(ctx context.Context, db *sql.DB, id string) (schema.Directive, bool, error) {
	row := db.QueryRowContext(ctx, `SELECT directive_id, title, mode, status, created_via, created_at, updated_at FROM directives WHERE directive_id = ?`, id)
	var d schema.Directive
	var createdAt, updatedAt, mode, status string
	err := row.Scan(&d.DirectiveID, &d.Title, &mode, &status, &d.CreatedVia, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return schema.Directive{}, false, nil
	} else if err != nil {
		return schema.Directive{}, false, fmt.Errorf("store: get directive: %w", err)
	}
	d.Mode = schema.DirectiveMode(mode)
	d.Status = schema.DirectiveStatus(status)
	d.CreatedAt, _ = parseTime(createdAt)
	d.UpdatedAt, _ = parseTime(updatedAt)
	return d, true, nil
}

func UpdateDirectiveMode(ctx context.Context, db *sql.DB, id string, mode schema.DirectiveMode) error {
	if err := mode.Validate(); err != nil {
		return fmt.Errorf("store: invalid directive mode: %w", err)
	}
	now := time.Now().UTC().Format(timeFormat)
	res, err := db.ExecContext(ctx, `UPDATE directives SET mode = ?, updated_at = ? WHERE directive_id = ?`, string(mode), now, id)
	if err != nil {
		return fmt.Errorf("store: update directive mode: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("store: directive not found")
	}
	return nil
}

func UpdateDirectiveStatus(ctx context.Context, db *sql.DB, id string, status schema.DirectiveStatus) error {
	now := time.Now().UTC().Format(timeFormat)
	res, err := db.ExecContext(ctx, `UPDATE directives SET status = ?, updated_at = ? WHERE directive_id = ?`, string(status), now, id)
	if err != nil {
		return fmt.Errorf("store: update directive status: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("store: directive not found")
	}
	return nil
}

func AppendMessage(ctx context.Context, db *sql.DB, msg schema.DirectiveMessage) error {
	if err := msg.Validate(); err != nil {
		return fmt.Errorf("store: invalid message: %w", err)
	}

	// Set tokens_used and model to sql.Null values since they are optional.
	var tokensUsed sql.NullInt64
	if msg.TokensUsed > 0 {
		tokensUsed.Int64 = int64(msg.TokensUsed)
		tokensUsed.Valid = true
	}
	var model sql.NullString
	if msg.Model != "" {
		model.String = msg.Model
		model.Valid = true
	}

	_, err := db.ExecContext(ctx, `
		INSERT INTO directive_messages (message_id, directive_id, role, content, created_at, tokens_used, model)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, msg.MessageID, msg.DirectiveID, msg.Role, msg.Content, msg.CreatedAt.Format(timeFormat), tokensUsed, model)
	if err != nil {
		return fmt.Errorf("store: append message: %w", err)
	}
	return nil
}

// DeleteDirectiveMessage removes a single message by ID (e.g. for rollback on bus publish failure).
func DeleteDirectiveMessage(ctx context.Context, db *sql.DB, messageID string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM directive_messages WHERE message_id = ?`, messageID)
	if err != nil {
		return fmt.Errorf("store: delete directive message: %w", err)
	}
	return nil
}

// GetMessages returns messages for a directive newest-first, up to limit rows.
func GetMessages(ctx context.Context, db *sql.DB, directiveID string, limit int) ([]schema.DirectiveMessage, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT message_id, directive_id, role, content, created_at, tokens_used, model
		FROM directive_messages
		WHERE directive_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, directiveID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: get messages: %w", err)
	}
	defer rows.Close()

	var msgs []schema.DirectiveMessage
	for rows.Next() {
		var m schema.DirectiveMessage
		var createdAt string
		var tokensUsed sql.NullInt64
		var model sql.NullString
		if err := rows.Scan(&m.MessageID, &m.DirectiveID, &m.Role, &m.Content, &createdAt, &tokensUsed, &model); err != nil {
			return nil, fmt.Errorf("store: scan message: %w", err)
		}
		m.CreatedAt, _ = parseTime(createdAt)
		if tokensUsed.Valid {
			m.TokensUsed = int(tokensUsed.Int64)
		}
		if model.Valid {
			m.Model = model.String
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

func GetActiveDirectives(ctx context.Context, db *sql.DB) ([]schema.Directive, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT directive_id, title, mode, status, created_via, created_at, updated_at
		FROM directives
		WHERE status IN ('ACTIVE', 'PAUSED', 'STALLED')
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("store: get active directives: %w", err)
	}
	defer rows.Close()

	var out []schema.Directive
	for rows.Next() {
		var d schema.Directive
		var createdAt, updatedAt, mode, status string
		if err := rows.Scan(&d.DirectiveID, &d.Title, &mode, &status, &d.CreatedVia, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("store: scan directive: %w", err)
		}
		d.Mode = schema.DirectiveMode(mode)
		d.Status = schema.DirectiveStatus(status)
		d.CreatedAt, _ = parseTime(createdAt)
		d.UpdatedAt, _ = parseTime(updatedAt)
		out = append(out, d)
	}
	return out, rows.Err()
}
