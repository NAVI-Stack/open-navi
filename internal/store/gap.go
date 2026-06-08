package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// GapType represents the high-level gap class (A–F) from the
// Capability Expansion Loop canonical doc.
type GapType string

const (
	GapTypeUnknown GapType = ""
	GapTypeMissingSkill GapType = "A" // missing skill
	GapTypeMissingConnector GapType = "B"
	GapTypeMissingRuntime GapType = "C"
	GapTypeMissingAuthConfig GapType = "D"
	GapTypeMissingPolicyTrust GapType = "E"
	GapTypeMissingComposition GapType = "F"
)

// GapStatus tracks the lifecycle of a gap record.
type GapStatus string

const (
	GapStatusDetected            GapStatus = "detected"
	GapStatusClassified          GapStatus = "classified"
	GapStatusExpansionPathChosen GapStatus = "expansion_path_chosen"
	GapStatusClosed              GapStatus = "closed"
)

// GapEvidence captures where and how the gap was observed.
// It is stored as JSON in the gaps.evidence column.
type GapEvidence struct {
	MessageID   string `json:"message_id,omitempty"`
	DirectiveID string `json:"directive_id,omitempty"`
	ChatID      string `json:"chat_id,omitempty"`
	ToolError   string `json:"tool_error,omitempty"`
	RawContext  string `json:"raw_context,omitempty"`
}

// GapClassification captures the classifier's view of the gap:
// gap class and recommended expansion path.
// It is stored as JSON in the gaps.classification_result column.
type GapClassification struct {
	GapClass      GapType `json:"gap_class,omitempty"`
	ExpansionPath string  `json:"expansion_path,omitempty"` // "skill" | "connector" | "plugin" | "none"
	Reason        string  `json:"reason,omitempty"`
}

// Gap is the stored representation of a detected capability gap.
type Gap struct {
	ID                   string            `json:"id"`
	Type                 GapType           `json:"type"`
	Status               GapStatus         `json:"status"`
	Evidence             GapEvidence       `json:"evidence"`
	ClassificationResult GapClassification `json:"classification_result"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
}

// InsertGap inserts a new gap row.
func InsertGap(ctx context.Context, db *sql.DB, g Gap) error {
	now := time.Now().UTC()
	if g.CreatedAt.IsZero() {
		g.CreatedAt = now
	}
	if g.UpdatedAt.IsZero() {
		g.UpdatedAt = g.CreatedAt
	}
	evBytes, err := json.Marshal(g.Evidence)
	if err != nil {
		return fmt.Errorf("store: marshal gap evidence: %w", err)
	}
	classBytes, err := json.Marshal(g.ClassificationResult)
	if err != nil {
		return fmt.Errorf("store: marshal gap classification: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO gaps (id, type, status, evidence, classification_result, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, g.ID, string(g.Type), string(g.Status), string(evBytes), string(classBytes), g.CreatedAt.Format(timeFormat), g.UpdatedAt.Format(timeFormat))
	if err != nil {
		return fmt.Errorf("store: insert gap: %w", err)
	}
	return nil
}

// ListOpenGaps returns gaps whose status is not closed.
func ListOpenGaps(ctx context.Context, db *sql.DB, limit int) ([]Gap, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, type, status, evidence, classification_result, created_at, updated_at
		FROM gaps
		WHERE status != ?
		ORDER BY created_at DESC
		LIMIT ?
	`, string(GapStatusClosed), limit)
	if err != nil {
		return nil, fmt.Errorf("store: list gaps: %w", err)
	}
	defer rows.Close()

	var gaps []Gap
	for rows.Next() {
		var (
			id, t, status, evidenceJSON, classJSON, createdStr, updatedStr string
		)
		if err := rows.Scan(&id, &t, &status, &evidenceJSON, &classJSON, &createdStr, &updatedStr); err != nil {
			return nil, fmt.Errorf("store: scan gap: %w", err)
		}
		createdAt, err := parseTime(createdStr)
		if err != nil {
			return nil, fmt.Errorf("store: parse gap created_at: %w", err)
		}
		updatedAt, err := parseTime(updatedStr)
		if err != nil {
			return nil, fmt.Errorf("store: parse gap updated_at: %w", err)
		}
		var evidence GapEvidence
		if evidenceJSON != "" {
			_ = json.Unmarshal([]byte(evidenceJSON), &evidence)
		}
		var class GapClassification
		if classJSON != "" {
			_ = json.Unmarshal([]byte(classJSON), &class)
		}
		gaps = append(gaps, Gap{
			ID:                   id,
			Type:                 GapType(t),
			Status:               GapStatus(status),
			Evidence:             evidence,
			ClassificationResult: class,
			CreatedAt:            createdAt,
			UpdatedAt:            updatedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list gaps rows: %w", err)
	}
	return gaps, nil
}

// UpdateGapStatus updates the status (and optionally classification) of a gap.
func UpdateGapStatus(ctx context.Context, db *sql.DB, id string, status GapStatus, class *GapClassification) error {
	nowStr := time.Now().UTC().Format(timeFormat)
	if class == nil {
		_, err := db.ExecContext(ctx, `
			UPDATE gaps
			SET status = ?, updated_at = ?
			WHERE id = ?
		`, string(status), nowStr, id)
		if err != nil {
			return fmt.Errorf("store: update gap status: %w", err)
		}
		return nil
	}
	classBytes, err := json.Marshal(class)
	if err != nil {
		return fmt.Errorf("store: marshal gap classification: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		UPDATE gaps
		SET status = ?, classification_result = ?, updated_at = ?
		WHERE id = ?
	`, string(status), string(classBytes), nowStr, id)
	if err != nil {
		return fmt.Errorf("store: update gap status/classification: %w", err)
	}
	return nil
}

// GetGap retrieves a single gap by ID.
func GetGap(ctx context.Context, db *sql.DB, id string) (*Gap, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, type, status, evidence, classification_result, created_at, updated_at
		FROM gaps
		WHERE id = ?
	`, id)

	var (
		gt, status, evidenceJSON, classJSON, createdStr, updatedStr string
	)
	if err := row.Scan(&id, &gt, &status, &evidenceJSON, &classJSON, &createdStr, &updatedStr); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("store: scan gap: %w", err)
	}
	createdAt, err := parseTime(createdStr)
	if err != nil {
		return nil, fmt.Errorf("store: parse gap created_at: %w", err)
	}
	updatedAt, err := parseTime(updatedStr)
	if err != nil {
		return nil, fmt.Errorf("store: parse gap updated_at: %w", err)
	}
	var evidence GapEvidence
	if evidenceJSON != "" {
		_ = json.Unmarshal([]byte(evidenceJSON), &evidence)
	}
	var class GapClassification
	if classJSON != "" {
		_ = json.Unmarshal([]byte(classJSON), &class)
	}
	return &Gap{
		ID:                   id,
		Type:                 GapType(gt),
		Status:               GapStatus(status),
		Evidence:             evidence,
		ClassificationResult: class,
		CreatedAt:            createdAt,
		UpdatedAt:            updatedAt,
	}, nil
}
