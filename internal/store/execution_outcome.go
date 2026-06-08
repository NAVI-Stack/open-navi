package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/schema"
)

func jsonSlice(s []string) string {
	if len(s) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(s)
	return string(b)
}

// SaveExecutionOutcome inserts a new execution outcome record.
// Callers are responsible for generating stable command/attempt identifiers.
func SaveExecutionOutcome(ctx context.Context, db *sql.DB, eo schema.ExecutionOutcome) error {
	// start_time is required; end_time may be nil.
	start := eo.StartTime.UTC().Format(timeFormat)
	var end any
	if eo.EndTime != nil {
		end = eo.EndTime.UTC().Format(timeFormat)
	}

	_, err := db.ExecContext(ctx, `
		INSERT INTO execution_outcomes (
			attempt_id,
			command_id,
			attempt_number,
			retry_of,
			command_type,
			start_time,
			end_time,
			outcome,
			failure_class,
			failure_reason,
			affected_entities,
			retryable,
			compensation_required,
			compensation_status,
			recovery_status,
			proposal_id,
			run_id,
			runtime_session_id,
			correlation_id,
			parent_run_id,
			skill_ids,
			connector_ids,
			llm_provider,
			llm_model,
			llm_task_class,
			llm_complexity,
			workspace_id,
			boundary_crossing,
			approval_required,
			approval_outcome,
			artifact_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, eo.AttemptID, eo.CommandID, eo.AttemptNumber, eo.RetryOf,
		string(eo.CommandType), start, end,
		string(eo.Outcome), string(eo.FailureClass), eo.FailureReason,
		eo.AffectedEntities,
		boolToInt(eo.Retryable),
		boolToInt(eo.CompensationRequired),
		string(eo.CompensationStatus),
		string(eo.RecoveryStatus),
		eo.ProposalID,
		eo.RunID,
		eo.RuntimeSessionID, eo.CorrelationID, eo.ParentRunID,
		jsonSlice(eo.SkillIDs), jsonSlice(eo.ConnectorIDs),
		eo.LLMProvider, eo.LLMModel, eo.LLMTaskClass, eo.LLMComplexity,
		eo.WorkspaceID, boolToInt(eo.BoundaryCrossing), boolToInt(eo.ApprovalRequired), string(eo.ApprovalOutcome),
		eo.ArtifactID,
	)
	if err != nil {
		return fmt.Errorf("store: save execution outcome: %w", err)
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Compensator runs compensating logic for an outcome (e.g. inverse steps for a Compose).
// When nil, RunCompensation only updates compensation_status to not_possible.
type Compensator interface {
	Compensate(ctx context.Context, eo schema.ExecutionOutcome) error
}

// NoOpCompensator runs no compensating actions; used when no inverse/undo logic is defined.
type NoOpCompensator struct{}

func (NoOpCompensator) Compensate(ctx context.Context, eo schema.ExecutionOutcome) error {
	return nil
}

// DefaultCompensator returns a compensator for the given outcome. Use a CompensatorRegistry
// when real undo is available; DefaultCompensator remains the NoOp fallback.
func DefaultCompensator(eo schema.ExecutionOutcome) Compensator {
	return NoOpCompensator{}
}

// CompensatorRegistry maps command types to compensators. CompensatorFor returns the
// registered compensator or NoOpCompensator when none is registered.
type CompensatorRegistry struct {
	byType map[schema.CommandType]Compensator
}

// NewCompensatorRegistry returns an empty registry. Register compensators for command types
// that support undo (e.g. Create → archive created entities).
func NewCompensatorRegistry() *CompensatorRegistry {
	return &CompensatorRegistry{byType: make(map[schema.CommandType]Compensator)}
}

// Register adds a compensator for the given command type.
func (r *CompensatorRegistry) Register(ct schema.CommandType, c Compensator) {
	r.byType[ct] = c
}

// CompensatorFor returns the compensator for eo.CommandType, or NoOpCompensator if none registered.
func (r *CompensatorRegistry) CompensatorFor(eo schema.ExecutionOutcome) Compensator {
	if c, ok := r.byType[eo.CommandType]; ok {
		return c
	}
	return NoOpCompensator{}
}

// CreateCompensator undoes a Create command by archiving each affected entity (soft-delete).
// AffectedEntities must be a JSON array of "entityType:entityID" strings.
type CreateCompensator struct {
	DB *sql.DB
}

func (c *CreateCompensator) Compensate(ctx context.Context, eo schema.ExecutionOutcome) error {
	if eo.AffectedEntities == "" {
		return nil
	}
	var refs []string
	if err := json.Unmarshal([]byte(eo.AffectedEntities), &refs); err != nil {
		return fmt.Errorf("store: create compensator parse affected_entities: %w", err)
	}
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		idx := strings.Index(ref, ":")
		if idx <= 0 || idx == len(ref)-1 {
			continue
		}
		entityType := ref[:idx]
		entityID := ref[idx+1:]
		if err := ArchiveEntity(ctx, c.DB, entityType, entityID); err != nil {
			return fmt.Errorf("store: create compensator archive %s: %w", ref, err)
		}
	}
	return nil
}

// UpdateExecutionOutcomeCompensationStatus sets compensation_status for an attempt.
func UpdateExecutionOutcomeCompensationStatus(ctx context.Context, db *sql.DB, attemptID string, status schema.CompensationStatus) error {
	_, err := db.ExecContext(ctx, `UPDATE execution_outcomes SET compensation_status = ? WHERE attempt_id = ?`, string(status), attemptID)
	if err != nil {
		return fmt.Errorf("store: update execution outcome compensation status: %w", err)
	}
	return nil
}

// UpdateExecutionOutcomeArtifactID sets the artifact_id for a given attempt.
func UpdateExecutionOutcomeArtifactID(ctx context.Context, db *sql.DB, attemptID string, artifactID string) error {
	_, err := db.ExecContext(ctx, `UPDATE execution_outcomes SET artifact_id = ? WHERE attempt_id = ?`, artifactID, attemptID)
	if err != nil {
		return fmt.Errorf("store: update execution outcome artifact id: %w", err)
	}
	return nil
}

// UpdateExecutionOutcomesApprovalByProposalID updates approval metadata for all
// execution outcomes linked to the given proposal.
func UpdateExecutionOutcomesApprovalByProposalID(ctx context.Context, db *sql.DB, proposalID string, outcome schema.ApprovalOutcome, approvalRequired bool) error {
	proposalID = strings.TrimSpace(proposalID)
	if proposalID == "" {
		return fmt.Errorf("store: proposal id is required")
	}
	_, err := db.ExecContext(ctx, `
		UPDATE execution_outcomes
		SET approval_required = ?, approval_outcome = ?
		WHERE proposal_id = ?
	`, boolToInt(approvalRequired), string(outcome), proposalID)
	if err != nil {
		return fmt.Errorf("store: update execution outcomes approval by proposal id: %w", err)
	}
	return nil
}

// RunCompensation runs compensating commands for a failed Compose (or compensable) outcome.
// When compensator is non-nil and CompensationRequired is true, it runs compensator.Compensate
// and sets compensation_status to completed or failed; otherwise sets not_possible.
func RunCompensation(ctx context.Context, db *sql.DB, eo schema.ExecutionOutcome, compensator Compensator) error {
	if !eo.CompensationRequired {
		return nil
	}
	if compensator == nil {
		return UpdateExecutionOutcomeCompensationStatus(ctx, db, eo.AttemptID, schema.CompensationStatusNotPossible)
	}
	if err := compensator.Compensate(ctx, eo); err != nil {
		_ = UpdateExecutionOutcomeCompensationStatus(ctx, db, eo.AttemptID, schema.CompensationStatusFailed)
		return err
	}
	return UpdateExecutionOutcomeCompensationStatus(ctx, db, eo.AttemptID, schema.CompensationStatusCompleted)
}

// DeleteExecutionOutcomesOlderThan removes execution_outcomes whose start_time is before the given time.
// Returns the number of rows deleted and any error.
func DeleteExecutionOutcomesOlderThan(ctx context.Context, db *sql.DB, before time.Time) (int64, error) {
	beforeStr := before.UTC().Format(timeFormat)
	res, err := db.ExecContext(ctx, `DELETE FROM execution_outcomes WHERE start_time < ?`, beforeStr)
	if err != nil {
		return 0, fmt.Errorf("store: delete execution outcomes older than %s: %w", beforeStr, err)
	}
	return res.RowsAffected()
}

// GetExecutionOutcomeByAttemptID returns a single execution outcome by attempt_id, or nil when not found.
func GetExecutionOutcomeByAttemptID(ctx context.Context, db *sql.DB, attemptID string) (*schema.ExecutionOutcome, error) {
	eo, err := scanExecutionOutcome(ctx, db, `SELECT attempt_id, command_id, attempt_number, retry_of, command_type,
		start_time, end_time, outcome, failure_class, failure_reason, affected_entities,
		retryable, compensation_required, compensation_status, recovery_status, proposal_id, run_id,
		runtime_session_id, correlation_id, parent_run_id, skill_ids, connector_ids, llm_provider, llm_model, llm_task_class, llm_complexity,
		workspace_id, boundary_crossing, approval_required, approval_outcome, artifact_id
		FROM execution_outcomes WHERE attempt_id = ?`, attemptID)
	if err != nil || eo == nil {
		return nil, err
	}
	return eo, nil
}

// ListExecutionOutcomes returns runs (execution outcomes) ordered by start_time DESC, with cursor pagination and optional since filter.
// limit defaults to 50; cursor is the attempt_id of the last item from the previous page; since filters by start_time >= since.
// Returns items and next_cursor (empty when no more pages).
func ListExecutionOutcomes(ctx context.Context, db *sql.DB, limit int, cursor string, since *time.Time) ([]schema.ExecutionOutcome, string, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	args := []any{}
	where := "1=1"
	if since != nil {
		where += " AND start_time >= ?"
		args = append(args, since.UTC().Format(timeFormat))
	}
	if cursor != "" {
		var cursorStart string
		err := db.QueryRowContext(ctx, `SELECT start_time FROM execution_outcomes WHERE attempt_id = ?`, cursor).Scan(&cursorStart)
		if err == nil {
			where += " AND (start_time < ? OR (start_time = ? AND attempt_id < ?))"
			args = append(args, cursorStart, cursorStart, cursor)
		}
	}
	args = append(args, limit+1)
	query := fmt.Sprintf(`
		SELECT attempt_id, command_id, attempt_number, retry_of, command_type,
			start_time, end_time, outcome, failure_class, failure_reason, affected_entities,
			retryable, compensation_required, compensation_status, recovery_status, proposal_id, run_id,
			runtime_session_id, correlation_id, parent_run_id, skill_ids, connector_ids, llm_provider, llm_model, llm_task_class, llm_complexity,
			workspace_id, boundary_crossing, approval_required, approval_outcome, artifact_id
		FROM execution_outcomes WHERE %s ORDER BY start_time DESC, attempt_id DESC LIMIT ?`, where)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("store: list execution outcomes: %w", err)
	}
	defer rows.Close()
	var list []schema.ExecutionOutcome
	for rows.Next() {
		eo, err := scanExecutionOutcomeRow(rows)
		if err != nil {
			return nil, "", err
		}
		list = append(list, *eo)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("store: list execution outcomes: %w", err)
	}
	nextCursor := ""
	if len(list) > limit {
		nextCursor = list[limit-1].AttemptID
		list = list[:limit]
	}
	return list, nextCursor, nil
}

// ListExecutionOutcomesByArtifactID returns recent execution outcomes associated with an artifact.
func ListExecutionOutcomesByArtifactID(ctx context.Context, db *sql.DB, artifactID string, limit int) ([]schema.ExecutionOutcome, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.QueryContext(ctx, `SELECT attempt_id, command_id, attempt_number, retry_of, command_type,
		start_time, end_time, outcome, failure_class, failure_reason, affected_entities,
		retryable, compensation_required, compensation_status, recovery_status, proposal_id, run_id,
		runtime_session_id, correlation_id, parent_run_id, skill_ids, connector_ids, llm_provider, llm_model, llm_task_class, llm_complexity,
		workspace_id, boundary_crossing, approval_required, approval_outcome, artifact_id
		FROM execution_outcomes
		WHERE artifact_id = ?
		ORDER BY start_time DESC
		LIMIT ?`, artifactID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list execution outcomes by artifact id: %w", err)
	}
	defer rows.Close()

	var out []schema.ExecutionOutcome
	for rows.Next() {
		item, err := scanExecutionOutcomeRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

// ListCalibratedLLMExecutionOutcomes returns recent execution outcomes that
// include route metadata, newest first.
func ListCalibratedLLMExecutionOutcomes(ctx context.Context, db *sql.DB, limit int) ([]schema.ExecutionOutcome, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, err := db.QueryContext(ctx, `
		SELECT attempt_id, command_id, attempt_number, retry_of, command_type,
			start_time, end_time, outcome, failure_class, failure_reason, affected_entities,
			retryable, compensation_required, compensation_status, recovery_status, proposal_id, run_id,
			runtime_session_id, correlation_id, parent_run_id, skill_ids, connector_ids, llm_provider, llm_model, llm_task_class, llm_complexity,
			workspace_id, boundary_crossing, approval_required, approval_outcome, artifact_id
		FROM execution_outcomes
		WHERE llm_provider IS NOT NULL AND llm_provider != '' AND llm_model IS NOT NULL AND llm_model != ''
		ORDER BY start_time DESC, attempt_id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list calibrated llm execution outcomes: %w", err)
	}
	defer rows.Close()
	var list []schema.ExecutionOutcome
	for rows.Next() {
		eo, err := scanExecutionOutcomeRow(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *eo)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list calibrated llm execution outcomes: %w", err)
	}
	return list, nil
}

func scanExecutionOutcome(ctx context.Context, db *sql.DB, query string, args ...any) (*schema.ExecutionOutcome, error) {
	row := db.QueryRowContext(ctx, query, args...)
	var eo schema.ExecutionOutcome
	var startStr, endNull sql.NullString
	var retryable, compReq, boundaryCrossing, approvalRequired int
	var proposalID, runID, runtimeSessionID, correlationID, parentRunID, skillIDsJSON, connectorIDsJSON, llmProvider, llmModel, llmTaskClass, llmComplexity, workspaceID, approvalOutcome, artifactID sql.NullString
	err := row.Scan(
		&eo.AttemptID, &eo.CommandID, &eo.AttemptNumber, &eo.RetryOf, &eo.CommandType,
		&startStr, &endNull,
		&eo.Outcome, &eo.FailureClass, &eo.FailureReason, &eo.AffectedEntities,
		&retryable, &compReq, &eo.CompensationStatus, &eo.RecoveryStatus, &proposalID, &runID,
		&runtimeSessionID, &correlationID, &parentRunID, &skillIDsJSON, &connectorIDsJSON, &llmProvider, &llmModel, &llmTaskClass, &llmComplexity,
		&workspaceID, &boundaryCrossing, &approvalRequired, &approvalOutcome, &artifactID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan execution outcome: %w", err)
	}
	eo.Retryable = retryable != 0
	eo.CompensationRequired = compReq != 0
	if proposalID.Valid {
		eo.ProposalID = proposalID.String
	}
	if runID.Valid {
		eo.RunID = runID.String
	}
	if runtimeSessionID.Valid {
		eo.RuntimeSessionID = runtimeSessionID.String
	}
	if correlationID.Valid {
		eo.CorrelationID = correlationID.String
	}
	if parentRunID.Valid {
		eo.ParentRunID = parentRunID.String
	}
	if skillIDsJSON.Valid && skillIDsJSON.String != "" {
		_ = json.Unmarshal([]byte(skillIDsJSON.String), &eo.SkillIDs)
	}
	if connectorIDsJSON.Valid && connectorIDsJSON.String != "" {
		_ = json.Unmarshal([]byte(connectorIDsJSON.String), &eo.ConnectorIDs)
	}
	if llmProvider.Valid {
		eo.LLMProvider = llmProvider.String
	}
	if llmModel.Valid {
		eo.LLMModel = llmModel.String
	}
	if llmTaskClass.Valid {
		eo.LLMTaskClass = llmTaskClass.String
	}
	if llmComplexity.Valid {
		eo.LLMComplexity = llmComplexity.String
	}
	if workspaceID.Valid {
		eo.WorkspaceID = workspaceID.String
	}
	eo.BoundaryCrossing = boundaryCrossing != 0
	eo.ApprovalRequired = approvalRequired != 0
	if approvalOutcome.Valid {
		eo.ApprovalOutcome = schema.ApprovalOutcome(approvalOutcome.String)
	}
	if artifactID.Valid {
		eo.ArtifactID = artifactID.String
	}
	if startStr.Valid {
		eo.StartTime, _ = parseTime(startStr.String)
	}
	if endNull.Valid && endNull.String != "" {
		if t, err := parseTime(endNull.String); err == nil {
			eo.EndTime = &t
		}
	}
	return &eo, nil
}

func scanExecutionOutcomeRow(rows *sql.Rows) (*schema.ExecutionOutcome, error) {
	var eo schema.ExecutionOutcome
	var startStr, endNull sql.NullString
	var retryable, compReq, boundaryCrossing, approvalRequired int
	var proposalID, runID, runtimeSessionID, correlationID, parentRunID, skillIDsJSON, connectorIDsJSON, llmProvider, llmModel, llmTaskClass, llmComplexity, workspaceID, approvalOutcome, artifactID sql.NullString
	err := rows.Scan(
		&eo.AttemptID, &eo.CommandID, &eo.AttemptNumber, &eo.RetryOf, &eo.CommandType,
		&startStr, &endNull,
		&eo.Outcome, &eo.FailureClass, &eo.FailureReason, &eo.AffectedEntities,
		&retryable, &compReq, &eo.CompensationStatus, &eo.RecoveryStatus, &proposalID, &runID,
		&runtimeSessionID, &correlationID, &parentRunID, &skillIDsJSON, &connectorIDsJSON, &llmProvider, &llmModel, &llmTaskClass, &llmComplexity,
		&workspaceID, &boundaryCrossing, &approvalRequired, &approvalOutcome, &artifactID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: scan execution outcome row: %w", err)
	}
	eo.Retryable = retryable != 0
	eo.CompensationRequired = compReq != 0
	if proposalID.Valid {
		eo.ProposalID = proposalID.String
	}
	if runID.Valid {
		eo.RunID = runID.String
	}
	if runtimeSessionID.Valid {
		eo.RuntimeSessionID = runtimeSessionID.String
	}
	if correlationID.Valid {
		eo.CorrelationID = correlationID.String
	}
	if parentRunID.Valid {
		eo.ParentRunID = parentRunID.String
	}
	if skillIDsJSON.Valid && skillIDsJSON.String != "" {
		_ = json.Unmarshal([]byte(skillIDsJSON.String), &eo.SkillIDs)
	}
	if connectorIDsJSON.Valid && connectorIDsJSON.String != "" {
		_ = json.Unmarshal([]byte(connectorIDsJSON.String), &eo.ConnectorIDs)
	}
	if llmProvider.Valid {
		eo.LLMProvider = llmProvider.String
	}
	if llmModel.Valid {
		eo.LLMModel = llmModel.String
	}
	if llmTaskClass.Valid {
		eo.LLMTaskClass = llmTaskClass.String
	}
	if llmComplexity.Valid {
		eo.LLMComplexity = llmComplexity.String
	}
	if workspaceID.Valid {
		eo.WorkspaceID = workspaceID.String
	}
	eo.BoundaryCrossing = boundaryCrossing != 0
	eo.ApprovalRequired = approvalRequired != 0
	if approvalOutcome.Valid {
		eo.ApprovalOutcome = schema.ApprovalOutcome(approvalOutcome.String)
	}
	if artifactID.Valid {
		eo.ArtifactID = artifactID.String
	}
	if startStr.Valid {
		eo.StartTime, _ = parseTime(startStr.String)
	}
	if endNull.Valid && endNull.String != "" {
		if t, err := parseTime(endNull.String); err == nil {
			eo.EndTime = &t
		}
	}
	return &eo, nil
}
