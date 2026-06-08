package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/schema"
)

// AppendEvent validates the event envelope and kind, then appends to the log.
func AppendEvent(ctx context.Context, db *sql.DB, ev schema.Event) error {
	if err := ev.Validate(); err != nil {
		return fmt.Errorf("store: event validate: %w", err)
	}
	if err := schema.ValidateEventKind(ev); err != nil {
		return fmt.Errorf("store: event kind: %w", err)
	}
	return appendEvent(ctx, db, nil, ev)
}

// AppendEventTx appends an event as part of an existing transaction.
func AppendEventTx(ctx context.Context, tx *sql.Tx, ev schema.Event) error {
	if err := ev.Validate(); err != nil {
		return fmt.Errorf("store: event validate: %w", err)
	}
	if err := schema.ValidateEventKind(ev); err != nil {
		return fmt.Errorf("store: event kind: %w", err)
	}
	return appendEvent(ctx, nil, tx, ev)
}

func appendEvent(ctx context.Context, db *sql.DB, tx *sql.Tx, ev schema.Event) error {
	payloadJSON, err := json.Marshal(ev.Payload)
	if err != nil {
		return fmt.Errorf("store: marshal payload: %w", err)
	}
	ownedTx := tx == nil
	if ownedTx {
		tx, err = db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("store: begin tx: %w", err)
		}
		defer tx.Rollback()
	}

	var dummy string
	err = tx.QueryRowContext(ctx, `SELECT id FROM events WHERE id = ?`, ev.ID).Scan(&dummy)
	if err == nil {
		return nil // idempotent success
	}

	var seq int64
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq), 0) + 1 FROM events`).Scan(&seq)
	if err != nil {
		return fmt.Errorf("store: next seq: %w", err)
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO events (id, type, kind, correlation_id, causal_parent, source_agent, target_agent, timestamp, payload, schema_version, seq, run_id, visibility)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.ID, string(ev.Type), string(ev.Kind), ev.CorrelationID, ev.CausalParent, string(ev.SourceAgent), string(ev.TargetAgent),
		ev.Timestamp.UTC().Format(timeFormat), string(payloadJSON), ev.SchemaVersion, seq, ev.RunID, string(ev.Visibility))
	if err != nil {
		return fmt.Errorf("store: insert event: %w", err)
	}

	if !ownedTx {
		return nil
	}
	return tx.Commit()
}

// EventsByCorrelationID returns events with the given correlation ID, ordered by seq.
func EventsByCorrelationID(ctx context.Context, db *sql.DB, correlationID string) ([]schema.Event, error) {
	return queryEvents(ctx, db, `SELECT id, type, kind, correlation_id, causal_parent, source_agent, target_agent, timestamp, payload, schema_version, seq, run_id, visibility FROM events WHERE correlation_id = ? ORDER BY seq`, correlationID)
}

// EventsByCausalParent returns events whose causal_parent is the given event ID, ordered by seq.
func EventsByCausalParent(ctx context.Context, db *sql.DB, causalParentID string) ([]schema.Event, error) {
	return queryEvents(ctx, db, `SELECT id, type, kind, correlation_id, causal_parent, source_agent, target_agent, timestamp, payload, schema_version, seq, run_id, visibility FROM events WHERE causal_parent = ? ORDER BY seq`, causalParentID)
}

// LatestEventByTypeAndCorrelationID returns the most recent event of a given
// type for a correlation ID, or nil when none exists.
func LatestEventByTypeAndCorrelationID(ctx context.Context, db *sql.DB, eventType schema.EventType, correlationID string) (*schema.Event, error) {
	events, err := queryEvents(ctx, db, `
		SELECT id, type, kind, correlation_id, causal_parent, source_agent, target_agent, timestamp, payload, schema_version, seq, run_id, visibility
		FROM events
		WHERE type = ? AND correlation_id = ?
		ORDER BY seq DESC
		LIMIT 1
	`, string(eventType), correlationID)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, nil
	}
	return &events[0], nil
}

// LatestEventsByType returns the N most recent events of a given type,
// ordered newest-first. Used by the inspection surface for snapshot history.
func LatestEventsByType(ctx context.Context, db *sql.DB, eventType schema.EventType, limit int) ([]schema.Event, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return queryEvents(ctx, db, `
		SELECT id, type, kind, correlation_id, causal_parent, source_agent, target_agent, timestamp, payload, schema_version, seq, run_id, visibility
		FROM events
		WHERE type = ?
		ORDER BY seq DESC
		LIMIT ?
	`, string(eventType), limit)
}

func queryEvents(ctx context.Context, db *sql.DB, query string, args ...any) ([]schema.Event, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: query events: %w", err)
	}
	defer rows.Close()
	var out []schema.Event
	for rows.Next() {
		var ev schema.Event
		var runID, visibility sql.NullString
		var ts, payload string
		if err := rows.Scan(&ev.ID, (*string)(&ev.Type), (*string)(&ev.Kind), &ev.CorrelationID, &ev.CausalParent, (*string)(&ev.SourceAgent), (*string)(&ev.TargetAgent), &ts, &payload, &ev.SchemaVersion, &ev.Seq, &runID, &visibility); err != nil {
			return nil, fmt.Errorf("store: scan event: %w", err)
		}
		ev.Timestamp, err = parseTime(ts)
		if err != nil {
			return nil, fmt.Errorf("store: parse timestamp: %w", err)
		}
		if err := json.Unmarshal([]byte(payload), &ev.Payload); err != nil {
			return nil, fmt.Errorf("store: unmarshal payload: %w", err)
		}
		if runID.Valid {
			ev.RunID = runID.String
		}
		if visibility.Valid {
			ev.Visibility = schema.EventVisibility(visibility.String)
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

// EventsSince returns events with seq > afterSeq, ordered by seq, up to limit.
func EventsSince(ctx context.Context, db *sql.DB, afterSeq int64, limit int) ([]schema.Event, error) {
	return queryEvents(ctx, db, `SELECT id, type, kind, correlation_id, causal_parent, source_agent, target_agent, timestamp, payload, schema_version, seq, run_id, visibility FROM events WHERE seq > ? ORDER BY seq LIMIT ?`, afterSeq, limit)
}

// SessionEventsSince returns events for a single chat (correlation_id), filtered by visibility, ordered by seq.
func SessionEventsSince(ctx context.Context, db *sql.DB, chatID string, afterSeq int64, allowed []schema.EventVisibility, limit int) ([]schema.Event, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	query := `SELECT id, type, kind, correlation_id, causal_parent, source_agent, target_agent, timestamp, payload, schema_version, seq, run_id, visibility
		FROM events
		WHERE correlation_id = ? AND seq > ?`
	args := []any{chatID, afterSeq}
	if len(allowed) > 0 {
		query += ` AND visibility IN (` + placeholders(len(allowed)) + `)`
		for _, v := range allowed {
			args = append(args, string(v))
		}
	}
	query += ` ORDER BY seq LIMIT ?`
	args = append(args, limit)
	return queryEvents(ctx, db, query, args...)
}

// ListEvents returns events with seq > afterSeq, optionally filtered by timestamp >= since, ordered by seq, up to limit.
// Used by the activity API for cursor-based pagination and time filtering. nextCursor is the seq of the last returned event (or 0).
func ListEvents(ctx context.Context, db *sql.DB, afterSeq int64, since *time.Time, limit int) ([]schema.Event, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	query := `SELECT id, type, kind, correlation_id, causal_parent, source_agent, target_agent, timestamp, payload, schema_version, seq, run_id, visibility FROM events WHERE seq > ?`
	args := []any{afterSeq}
	if since != nil {
		query += ` AND timestamp >= ?`
		args = append(args, since.UTC().Format(timeFormat))
	}
	query += ` ORDER BY seq LIMIT ?`
	args = append(args, limit+1)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list events: %w", err)
	}
	defer rows.Close()
	var out []schema.Event
	for rows.Next() {
		var ev schema.Event
		var runID, visibility sql.NullString
		var ts, payload string
		if err := rows.Scan(&ev.ID, (*string)(&ev.Type), (*string)(&ev.Kind), &ev.CorrelationID, &ev.CausalParent, (*string)(&ev.SourceAgent), (*string)(&ev.TargetAgent), &ts, &payload, &ev.SchemaVersion, &ev.Seq, &runID, &visibility); err != nil {
			return nil, 0, fmt.Errorf("store: scan event: %w", err)
		}
		var parseErr error
		ev.Timestamp, parseErr = parseTime(ts)
		if parseErr != nil {
			return nil, 0, fmt.Errorf("store: parse timestamp: %w", parseErr)
		}
		if err := json.Unmarshal([]byte(payload), &ev.Payload); err != nil {
			return nil, 0, fmt.Errorf("store: unmarshal payload: %w", err)
		}
		if runID.Valid {
			ev.RunID = runID.String
		}
		if visibility.Valid {
			ev.Visibility = schema.EventVisibility(visibility.String)
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("store: list events: %w", err)
	}
	nextCursor := int64(0)
	if len(out) > limit {
		nextCursor = out[limit-1].Seq
		out = out[:limit]
	}
	return out, nextCursor, nil
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	buf := make([]byte, 0, n*2-1)
	for i := 0; i < n; i++ {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, '?')
	}
	return string(buf)
}

// DeleteEventsOlderThan removes events whose timestamp is before the given time.
// Returns the number of rows deleted. Used by the event retention janitor.
func DeleteEventsOlderThan(ctx context.Context, db *sql.DB, before time.Time) (int64, error) {
	result, err := db.ExecContext(ctx,
		`DELETE FROM events WHERE timestamp < ?`,
		before.UTC().Format(timeFormat))
	if err != nil {
		return 0, fmt.Errorf("store: delete old events: %w", err)
	}
	return result.RowsAffected()
}

// DeleteEventsByRetentionPolicy removes events according to a default retention
// window plus exact event-type overrides. Per-type entries override the default;
// a per-type value of 0 disables pruning for that event type entirely.
func DeleteEventsByRetentionPolicy(ctx context.Context, db *sql.DB, now time.Time, defaultDays int, byType map[string]int) (int64, error) {
	if defaultDays <= 0 && len(byType) == 0 {
		return 0, nil
	}

	overrideTypes := make([]string, 0, len(byType))
	for eventType := range byType {
		overrideTypes = append(overrideTypes, eventType)
	}
	sort.Strings(overrideTypes)

	var (
		clauses []string
		args    []any
	)

	if defaultDays > 0 {
		defaultClause := `timestamp < ?`
		args = append(args, now.UTC().AddDate(0, 0, -defaultDays).Format(timeFormat))
		if len(overrideTypes) > 0 {
			defaultClause += ` AND type NOT IN (` + placeholders(len(overrideTypes)) + `)`
			for _, eventType := range overrideTypes {
				args = append(args, eventType)
			}
		}
		clauses = append(clauses, `(`+defaultClause+`)`)
	}

	for _, eventType := range overrideTypes {
		days := byType[eventType]
		if days <= 0 {
			continue
		}
		clauses = append(clauses, `(type = ? AND timestamp < ?)`)
		args = append(args, eventType, now.UTC().AddDate(0, 0, -days).Format(timeFormat))
	}

	if len(clauses) == 0 {
		return 0, nil
	}

	result, err := db.ExecContext(ctx,
		`DELETE FROM events WHERE `+strings.Join(clauses, ` OR `),
		args...)
	if err != nil {
		return 0, fmt.Errorf("store: delete events by retention policy: %w", err)
	}
	return result.RowsAffected()
}
