package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/navi"
	"github.com/ceoai/navi/internal/navi/inference"
)

func (s *SQLiteStore) SaveICSState(ctx context.Context, runID string, envelope inference.DecisionEnvelope) (*navi.ICSState, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("navi: save ICS state: store not configured")
	}
	runID = stringsTrim(runID)
	if runID == "" {
		return nil, fmt.Errorf("navi: save ICS state: run id required")
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("navi: save ICS state: marshal envelope: %w", err)
	}
	now := time.Now().UTC()
	version := firstNonEmptyString(stringsTrim(envelope.Rationale.Version), inference.ContractVersionV1)

	existing, err := s.LoadICSState(ctx, runID)
	if err != nil {
		return nil, err
	}
	createdAt := now
	if existing != nil && !existing.CreatedAt.IsZero() {
		createdAt = existing.CreatedAt
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO ics_state (run_id, version, decision_envelope, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(run_id) DO UPDATE SET
			version = excluded.version,
			decision_envelope = excluded.decision_envelope,
			updated_at = excluded.updated_at
	`, runID, version, string(payload), createdAt, now); err != nil {
		return nil, fmt.Errorf("navi: save ICS state: %w", err)
	}

	state := &navi.ICSState{
		RunID:            runID,
		Version:          version,
		DecisionEnvelope: envelope,
		CreatedAt:        createdAt,
		UpdatedAt:        now,
	}
	return state, nil
}

func (s *SQLiteStore) LoadICSState(ctx context.Context, runID string) (*navi.ICSState, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("navi: load ICS state: store not configured")
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT run_id, version, decision_envelope, created_at, updated_at
		FROM ics_state
		WHERE run_id = ?
	`, stringsTrim(runID))
	var state navi.ICSState
	var payload string
	if err := row.Scan(&state.RunID, &state.Version, &payload, &state.CreatedAt, &state.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("navi: load ICS state: %w", err)
	}
	if err := json.Unmarshal([]byte(payload), &state.DecisionEnvelope); err != nil {
		return nil, fmt.Errorf("navi: load ICS state: decode envelope: %w", err)
	}
	return &state, nil
}

func (s *SQLiteStore) AppendICSHistory(ctx context.Context, runID string, envelope inference.DecisionEnvelope) (*navi.ICSHistory, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("navi: append ICS history: store not configured")
	}
	runID = stringsTrim(runID)
	if runID == "" {
		return nil, fmt.Errorf("navi: append ICS history: run id required")
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("navi: append ICS history: marshal envelope: %w", err)
	}
	now := time.Now().UTC()
	step := 0
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(step), 0) + 1 FROM ics_history WHERE run_id = ?`, runID).Scan(&step); err != nil {
		return nil, fmt.Errorf("navi: append ICS history: next step: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO ics_history (run_id, step, decision_envelope, created_at)
		VALUES (?, ?, ?, ?)
	`, runID, step, string(payload), now); err != nil {
		return nil, fmt.Errorf("navi: append ICS history: %w", err)
	}
	return &navi.ICSHistory{
		RunID:     runID,
		Step:      step,
		Envelope:  envelope,
		Timestamp: now,
	}, nil
}

func (s *SQLiteStore) LoadICSHistory(ctx context.Context, runID string, limit int) ([]navi.ICSHistory, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("navi: load ICS history: store not configured")
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT run_id, step, decision_envelope, created_at
		FROM ics_history
		WHERE run_id = ?
		ORDER BY step DESC
		LIMIT ?
	`, stringsTrim(runID), limit)
	if err != nil {
		return nil, fmt.Errorf("navi: load ICS history: %w", err)
	}
	defer rows.Close()

	var history []navi.ICSHistory
	for rows.Next() {
		var item navi.ICSHistory
		var payload string
		if err := rows.Scan(&item.RunID, &item.Step, &payload, &item.Timestamp); err != nil {
			return nil, fmt.Errorf("navi: load ICS history scan: %w", err)
		}
		if err := json.Unmarshal([]byte(payload), &item.Envelope); err != nil {
			return nil, fmt.Errorf("navi: load ICS history decode: %w", err)
		}
		history = append(history, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("navi: load ICS history rows: %w", err)
	}
	return history, nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := stringsTrim(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func stringsTrim(value string) string { return strings.TrimSpace(value) }
