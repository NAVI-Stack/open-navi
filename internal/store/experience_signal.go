package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/schema"
	"github.com/google/uuid"
)

func SavePreferenceSignal(ctx context.Context, db *sql.DB, signal schema.PreferenceSignal) error {
	if db == nil {
		return fmt.Errorf("store: save preference signal: nil db")
	}
	now := time.Now().UTC()
	if strings.TrimSpace(signal.SignalID) == "" {
		signal.SignalID = uuid.New().String()
	}
	if signal.CreatedAt.IsZero() {
		signal.CreatedAt = now
	}
	if signal.UpdatedAt.IsZero() {
		signal.UpdatedAt = signal.CreatedAt
	}
	if signal.Status == "" {
		signal.Status = schema.PreferenceSignalStatusCaptured
	}
	_, err := db.ExecContext(ctx, `
        INSERT INTO experience_preference_signals (
            signal_id, scope, scope_id, chat_id, trait, target_value, evidence_class,
            signal_strength, immediate, summary, status, created_at, updated_at
        )
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(signal_id) DO UPDATE SET
            target_value = excluded.target_value,
            evidence_class = excluded.evidence_class,
            signal_strength = excluded.signal_strength,
            immediate = excluded.immediate,
            summary = excluded.summary,
            status = excluded.status,
            updated_at = excluded.updated_at
    `,
		signal.SignalID,
		signal.Scope,
		signal.ScopeID,
		signal.ChatID,
		signal.Trait,
		signal.TargetValue,
		string(signal.EvidenceClass),
		signal.SignalStrength,
		boolToInt(signal.Immediate),
		signal.Summary,
		string(signal.Status),
		signal.CreatedAt.Format(timeFormat),
		signal.UpdatedAt.Format(timeFormat),
	)
	if err != nil {
		return fmt.Errorf("store: save preference signal: %w", err)
	}
	return nil
}

func ListPreferenceSignalsByScope(ctx context.Context, db *sql.DB, scope, scopeID string, statuses []schema.PreferenceSignalStatus, limit int) ([]schema.PreferenceSignal, error) {
	if db == nil {
		return nil, fmt.Errorf("store: list preference signals: nil db")
	}
	if limit <= 0 {
		limit = 100
	}
	query := `
        SELECT signal_id, scope, scope_id, chat_id, trait, target_value, evidence_class,
               signal_strength, immediate, summary, status, created_at, updated_at
        FROM experience_preference_signals
        WHERE scope = ? AND scope_id = ?`
	args := []any{scope, scopeID}
	if len(statuses) > 0 {
		placeholders := make([]string, 0, len(statuses))
		for _, status := range statuses {
			placeholders = append(placeholders, "?")
			args = append(args, string(status))
		}
		query += " AND status IN (" + strings.Join(placeholders, ",") + ")"
	}
	query += " ORDER BY created_at ASC LIMIT ?"
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list preference signals: %w", err)
	}
	defer rows.Close()
	return scanPreferenceSignals(rows)
}

func ListImmediatePreferenceSignalsBySession(ctx context.Context, db *sql.DB, chatID string, limit int) ([]schema.PreferenceSignal, error) {
	if db == nil {
		return nil, fmt.Errorf("store: list immediate preference signals: nil db")
	}
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.QueryContext(ctx, `
        SELECT signal_id, scope, scope_id, chat_id, trait, target_value, evidence_class,
               signal_strength, immediate, summary, status, created_at, updated_at
        FROM experience_preference_signals
        WHERE chat_id = ? AND immediate = 1
        ORDER BY created_at ASC
        LIMIT ?
    `, chatID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list immediate preference signals: %w", err)
	}
	defer rows.Close()
	return scanPreferenceSignals(rows)
}

func scanPreferenceSignals(rows *sql.Rows) ([]schema.PreferenceSignal, error) {
	out := []schema.PreferenceSignal{}
	for rows.Next() {
		var (
			signal        schema.PreferenceSignal
			evidenceClass string
			status        string
			immediate     int
			createdRaw    string
			updatedRaw    string
		)
		if err := rows.Scan(
			&signal.SignalID,
			&signal.Scope,
			&signal.ScopeID,
			&signal.ChatID,
			&signal.Trait,
			&signal.TargetValue,
			&evidenceClass,
			&signal.SignalStrength,
			&immediate,
			&signal.Summary,
			&status,
			&createdRaw,
			&updatedRaw,
		); err != nil {
			return nil, fmt.Errorf("store: scan preference signal: %w", err)
		}
		signal.EvidenceClass = schema.PreferenceEvidenceClass(evidenceClass)
		signal.Status = schema.PreferenceSignalStatus(status)
		signal.Immediate = immediate != 0
		signal.CreatedAt, _ = parseTime(createdRaw)
		signal.UpdatedAt, _ = parseTime(updatedRaw)
		out = append(out, signal)
	}
	return out, rows.Err()
}

func UpdatePreferenceSignalStatus(ctx context.Context, db *sql.DB, signalIDs []string, status schema.PreferenceSignalStatus) error {
	if db == nil {
		return fmt.Errorf("store: update preference signal status: nil db")
	}
	if len(signalIDs) == 0 {
		return nil
	}
	placeholders := make([]string, 0, len(signalIDs))
	args := make([]any, 0, len(signalIDs)+2)
	args = append(args, string(status), time.Now().UTC().Format(timeFormat))
	for _, id := range signalIDs {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	query := `UPDATE experience_preference_signals SET status = ?, updated_at = ? WHERE signal_id IN (` + strings.Join(placeholders, ",") + `)`
	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("store: update preference signal status: %w", err)
	}
	return nil
}
