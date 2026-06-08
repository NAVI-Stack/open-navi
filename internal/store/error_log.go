package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ErrorSeverity string

const (
	ErrorSeverityWarning ErrorSeverity = "warning"
	ErrorSeverityError   ErrorSeverity = "error"
	ErrorSeverityFatal   ErrorSeverity = "fatal"
)

type ErrorRecord struct {
	ID           string        `json:"id"`
	Timestamp    time.Time     `json:"timestamp"`
	Severity     ErrorSeverity `json:"severity"`
	Component    string        `json:"component"`
	ChatID       string        `json:"chat_id,omitempty"`
	RunID        string        `json:"run_id,omitempty"`
	ErrorType    string        `json:"error_type"`
	ErrorMessage string        `json:"error_message"`
	StackTrace   string        `json:"stack_trace,omitempty"`
	ContextJSON  string        `json:"context_json,omitempty"`
	ResolvedAt   *time.Time    `json:"resolved_at,omitempty"`
	Resolution   string        `json:"resolution,omitempty"`
}

type ListErrorsFilter struct {
	Severity  string
	Component string
	ChatID    string
	ErrorType string
	Limit     int
}

type ErrorSummaryRow struct {
	Component string `json:"component"`
	ErrorType string `json:"error_type"`
	Count     int    `json:"count"`
}

func SaveErrorRecord(ctx context.Context, db *sql.DB, rec ErrorRecord) error {
	if strings.TrimSpace(rec.ID) == "" {
		rec.ID = uuid.NewString()
	}
	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now().UTC()
	}
	if rec.Severity == "" {
		rec.Severity = ErrorSeverityError
	}
	if strings.TrimSpace(rec.Component) == "" {
		return fmt.Errorf("store: error record component required")
	}
	if strings.TrimSpace(rec.ErrorType) == "" {
		return fmt.Errorf("store: error record error_type required")
	}
	if strings.TrimSpace(rec.ErrorMessage) == "" {
		return fmt.Errorf("store: error record message required")
	}

	var resolvedAt any
	if rec.ResolvedAt != nil {
		resolvedAt = rec.ResolvedAt.UTC().Format(timeFormat)
	}
	_, err := db.ExecContext(ctx, `INSERT INTO error_log (
		id, timestamp, severity, component, session_id, run_id, error_type, error_message, stack_trace, context_json, resolved_at, resolution
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.ID,
		rec.Timestamp.UTC().Format(timeFormat),
		string(rec.Severity),
		rec.Component,
		nullIfEmpty(rec.ChatID),
		nullIfEmpty(rec.RunID),
		rec.ErrorType,
		rec.ErrorMessage,
		nullIfEmpty(rec.StackTrace),
		nullIfEmpty(rec.ContextJSON),
		resolvedAt,
		nullIfEmpty(rec.Resolution),
	)
	if err != nil {
		return fmt.Errorf("store: save error record: %w", err)
	}
	return nil
}

func ListErrorRecords(ctx context.Context, db *sql.DB, filter ListErrorsFilter) ([]ErrorRecord, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	query := `SELECT id, timestamp, severity, component, session_id, run_id, error_type, error_message, stack_trace, context_json, resolved_at, resolution
		FROM error_log WHERE 1=1`
	args := make([]any, 0, 5)
	if filter.Severity != "" {
		query += ` AND severity = ?`
		args = append(args, filter.Severity)
	}
	if filter.Component != "" {
		query += ` AND component = ?`
		args = append(args, filter.Component)
	}
	if filter.ChatID != "" {
		query += ` AND session_id = ?`
		args = append(args, filter.ChatID)
	}
	if filter.ErrorType != "" {
		query += ` AND error_type = ?`
		args = append(args, filter.ErrorType)
	}
	query += ` ORDER BY timestamp DESC LIMIT ?`
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list error records: %w", err)
	}
	defer rows.Close()

	var out []ErrorRecord
	for rows.Next() {
		var rec ErrorRecord
		var timestamp string
		var chatID, runID, stackTrace, contextJSON, resolvedAt, resolution sql.NullString
		if err := rows.Scan(
			&rec.ID,
			&timestamp,
			&rec.Severity,
			&rec.Component,
			&chatID,
			&runID,
			&rec.ErrorType,
			&rec.ErrorMessage,
			&stackTrace,
			&contextJSON,
			&resolvedAt,
			&resolution,
		); err != nil {
			return nil, fmt.Errorf("store: scan error record: %w", err)
		}
		ts, err := parseTime(timestamp)
		if err != nil {
			return nil, fmt.Errorf("store: parse error record timestamp: %w", err)
		}
		rec.Timestamp = ts
		rec.ChatID = chatID.String
		rec.RunID = runID.String
		rec.StackTrace = stackTrace.String
		rec.ContextJSON = contextJSON.String
		rec.Resolution = resolution.String
		if resolvedAt.Valid {
			parsed, err := parseTime(resolvedAt.String)
			if err != nil {
				return nil, fmt.Errorf("store: parse error record resolved_at: %w", err)
			}
			rec.ResolvedAt = &parsed
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list error records: %w", err)
	}
	return out, nil
}

func ErrorSummary(ctx context.Context, db *sql.DB, since time.Time) ([]ErrorSummaryRow, error) {
	rows, err := db.QueryContext(ctx, `SELECT component, error_type, COUNT(*)
		FROM error_log
		WHERE timestamp >= ?
		GROUP BY component, error_type
		ORDER BY COUNT(*) DESC, component ASC, error_type ASC`, since.UTC().Format(timeFormat))
	if err != nil {
		return nil, fmt.Errorf("store: error summary: %w", err)
	}
	defer rows.Close()

	var out []ErrorSummaryRow
	for rows.Next() {
		var row ErrorSummaryRow
		if err := rows.Scan(&row.Component, &row.ErrorType, &row.Count); err != nil {
			return nil, fmt.Errorf("store: scan error summary: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: error summary: %w", err)
	}
	return out, nil
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
