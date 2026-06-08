package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/navi"
	naviruntime "github.com/open-navi/navi/internal/runtime"
	"github.com/open-navi/navi/internal/schema"
	corestore "github.com/open-navi/navi/internal/store"
)

func (s *SQLiteStore) AcceptMessage(ctx context.Context, runtimeSessionID string, item *naviruntime.InboxItem, experienceMode string) (*naviruntime.InboxItem, error) {
	if item == nil {
		return nil, fmt.Errorf("navi: accept message: nil inbox item")
	}
	runtimeSessionID = strings.TrimSpace(runtimeSessionID)
	if runtimeSessionID == "" {
		runtimeSessionID = strings.TrimSpace(item.RuntimeSessionID)
	}
	if runtimeSessionID == "" {
		return nil, fmt.Errorf("navi: accept message: runtime session id is required")
	}
	if item.ID == "" {
		item.ID = uuid.New().String()
	}
	if item.ReceivedAt.IsZero() {
		item.ReceivedAt = time.Now().UTC()
	}
	if item.QueueAction == "" {
		item.QueueAction = "append"
	}
	if item.Status == "" {
		item.Status = naviruntime.InboxStatusPending
	}
	if item.PayloadType == "" {
		item.PayloadType = "text"
	}
	if item.ActorType == "" {
		item.ActorType = "user"
	}
	item.RuntimeSessionID = runtimeSessionID
	item.ChatID = strings.TrimSpace(item.ChatID)
	if item.CorrelationID == "" {
		item.CorrelationID = firstNonEmpty(item.ChatID, runtimeSessionID)
	}
	msgID := strings.TrimSpace(item.MessageID)
	transcriptAlreadyPersisted := msgID != ""
	if msgID == "" {
		msgID = uuid.New().String()
	}
	if item.ChatID != "" {
		item.MessageID = msgID
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("navi: accept message begin tx: %w", err)
	}
	defer tx.Rollback()

	if key := strings.TrimSpace(item.IdempotencyKey); key != "" {
		existing, err := findInboxByIdempotencyKeyTx(ctx, tx, key)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			item.QueueAction = "supersede"
			item.Status = naviruntime.InboxStatusSuperseded
			item.MergedIntoID = existing.ID
			item.ClassifiedReason = "duplicate idempotency key already accepted"
			item.Confidence = 1
			item.RuntimeSessionID = existing.RuntimeSessionID
			item.ChatID = existing.ChatID
			item.MessageID = existing.MessageID
			return item, nil
		}
	}

	if experienceMode == "" {
		experienceMode = string(navi.ExperienceModeStandard)
	}
	experienceMode = string(navi.NormalizeExperienceMode(navi.ExperienceMode(experienceMode)))
	if err := ensureRuntimeSessionWritableTx(ctx, tx, runtimeSessionID, item, navi.ExperienceMode(experienceMode)); err != nil {
		return nil, err
	}
	if item.ChatID != "" {
		if err := ensureChatWritableTx(ctx, tx, item.ChatID, navi.ExperienceMode(experienceMode)); err != nil {
			return nil, err
		}
	}
	if item.QueueAction == "merge" && item.MergedIntoID != "" {
		exists, err := pendingInboxExistsTx(ctx, tx, runtimeSessionID, item.MergedIntoID)
		if err != nil {
			return nil, err
		}
		if !exists {
			item.QueueAction = "append"
			item.Status = naviruntime.InboxStatusPending
			item.ClassifiedReason = "merge target no longer pending; fell back to append"
			item.Confidence = 0.5
			item.MergedIntoID = ""
		}
	}
	if err := insertInboxTx(ctx, tx, item); err != nil {
		if errors.Is(err, naviruntime.ErrDuplicateInboxItem) {
			return nil, naviruntime.ErrDuplicateInboxItem
		}
		return nil, err
	}
	if item.ChatID != "" && !transcriptAlreadyPersisted {
		msgRuntimeID := navi.ID(runtimeSessionID)
		msgInboxID := navi.ID(item.ID)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO navi_chat_messages (
				message_id, chat_id, role, content, created_at,
				runtime_session_id, inbox_item_id,
				source_channel, source_message_ref, origin_endpoint_id, message_kind, metadata_json
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, '{}')
		`, msgID, item.ChatID, "user", item.Content, item.ReceivedAt, string(msgRuntimeID), string(msgInboxID), item.SourceChannel, item.SourceMessageRef, nullableString(item.OriginEndpointID)); err != nil {
			return nil, fmt.Errorf("navi: insert user chat message: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE navi_chats
			SET updated_at = ?,
			    last_message_at = ?,
			    message_count = message_count + 1,
			    user_message_count = user_message_count + 1
			WHERE chat_id = ?
		`, item.ReceivedAt, item.ReceivedAt, item.ChatID); err != nil {
			return nil, fmt.Errorf("navi: update chat stamp: %w", err)
		}
	}
	if item.QueueAction == "merge" && item.MergedIntoID != "" {
		if err := mergeIntoPendingInboxTx(ctx, tx, item); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE runtime_sessions SET last_active_at = ?, status = CASE WHEN status IN ('closed', 'failed') THEN status ELSE 'active' END WHERE runtime_session_id = ?`, item.ReceivedAt, runtimeSessionID); err != nil {
		return nil, fmt.Errorf("navi: update runtime session stamp: %w", err)
	}
	correlationID := firstNonEmpty(item.ChatID, runtimeSessionID)
	received := schema.NewEvent(
		schema.FactMessageReceived,
		schema.EventKindFact,
		correlationID,
		schema.AgentNavi,
		schema.MessageReceivedPayload{
			RuntimeSessionID: runtimeSessionID,
			ChatID:           item.ChatID,
			InboxItemID:      item.ID,
			MessageID:        msgID,
			SourceChannel:    item.SourceChannel,
			SourceMessageRef: item.SourceMessageRef,
			ContentLen:       len(item.Content),
			QueueAction:      item.QueueAction,
		},
	)
	received.Visibility = runtimeVisibilityTx(ctx, tx, runtimeSessionID, item.ChatID)
	if err := corestore.AppendEventTx(ctx, tx, received); err != nil {
		return nil, fmt.Errorf("navi: append message.received: %w", err)
	}
	classified := schema.NewEvent(
		schema.FactMessageClassified,
		schema.EventKindFact,
		correlationID,
		schema.AgentNavi,
		schema.MessageClassifiedPayload{
			RuntimeSessionID: runtimeSessionID,
			ChatID:           item.ChatID,
			InboxItemID:      item.ID,
			QueueAction:      item.QueueAction,
			Reason:           item.ClassifiedReason,
			Confidence:       item.Confidence,
		},
	)
	classified.Visibility = schema.VisibilityOperator
	if err := corestore.AppendEventTx(ctx, tx, classified); err != nil {
		return nil, fmt.Errorf("navi: append message.classified: %w", err)
	}
	if err := appendQueueActionEventTx(ctx, tx, runtimeSessionID, item); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("navi: accept message commit: %w", err)
	}
	return item, nil
}

func (s *SQLiteStore) AcceptSignal(ctx context.Context, item *naviruntime.InboxItem) (*naviruntime.InboxItem, error) {
	if item == nil {
		return nil, fmt.Errorf("navi: accept signal: nil inbox item")
	}
	if item.ID == "" {
		item.ID = uuid.New().String()
	}
	if item.ReceivedAt.IsZero() {
		item.ReceivedAt = time.Now().UTC()
	}
	if item.Status == "" {
		item.Status = naviruntime.InboxStatusPending
	}
	if item.QueueAction == "" {
		item.QueueAction = "append"
	}
	runtimeSessionID := strings.TrimSpace(item.RuntimeSessionID)
	if runtimeSessionID == "" {
		return nil, fmt.Errorf("navi: accept signal: runtime session id is required")
	}
	if item.CorrelationID == "" {
		item.CorrelationID = firstNonEmpty(item.ChatID, runtimeSessionID)
	}
	if strings.TrimSpace(item.ChatID) == "" && !schema.IsInternalRuntimeSessionID(runtimeSessionID) && !strings.EqualFold(strings.TrimSpace(item.SourceChannel), "heartbeat") {
		item.ChatID = runtimeSessionID
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("navi: accept signal begin tx: %w", err)
	}
	defer tx.Rollback()
	if err := ensureRuntimeSessionWritableTx(ctx, tx, runtimeSessionID, item, navi.ExperienceModeStandard); err != nil {
		return nil, err
	}
	if err := insertInboxTx(ctx, tx, item); err != nil {
		if errors.Is(err, naviruntime.ErrDuplicateInboxItem) {
			return nil, naviruntime.ErrDuplicateInboxItem
		}
		return nil, err
	}
	received := schema.NewEvent(
		schema.FactMessageReceived,
		schema.EventKindFact,
		firstNonEmpty(item.ChatID, runtimeSessionID),
		schema.AgentNavi,
		schema.MessageReceivedPayload{
			RuntimeSessionID: runtimeSessionID,
			ChatID:           item.ChatID,
			InboxItemID:      item.ID,
			SourceChannel:    item.SourceChannel,
			SourceMessageRef: item.SourceMessageRef,
			ContentLen:       len(item.Content),
			QueueAction:      item.QueueAction,
		},
	)
	received.Visibility = schema.VisibilityOperator
	if err := corestore.AppendEventTx(ctx, tx, received); err != nil {
		return nil, fmt.Errorf("navi: append signal event: %w", err)
	}
	classified := schema.NewEvent(
		schema.FactMessageClassified,
		schema.EventKindFact,
		firstNonEmpty(item.ChatID, runtimeSessionID),
		schema.AgentNavi,
		schema.MessageClassifiedPayload{
			RuntimeSessionID: runtimeSessionID,
			ChatID:           item.ChatID,
			InboxItemID:      item.ID,
			QueueAction:      item.QueueAction,
			Reason:           item.ClassifiedReason,
			Confidence:       item.Confidence,
		},
	)
	classified.Visibility = schema.VisibilityOperator
	if err := corestore.AppendEventTx(ctx, tx, classified); err != nil {
		return nil, fmt.Errorf("navi: append signal classified event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("navi: accept signal commit: %w", err)
	}
	return item, nil
}

func insertInboxTx(ctx context.Context, tx *sql.Tx, item *naviruntime.InboxItem) error {
	var structured any
	if len(item.Structured) > 0 {
		structured = string(item.Structured)
	}
	var idempotencyKey any
	if key := strings.TrimSpace(item.IdempotencyKey); key != "" {
		idempotencyKey = key
	}
	result, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO navi_inbox (
			inbox_item_id, runtime_session_id, chat_id,
			source_channel, source_message_ref, actor_type, payload_type,
			origin_endpoint_id,
			content, structured_payload, correlation_id, idempotency_key, queue_action, status,
			message_id, run_id, received_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, item.ID, item.RuntimeSessionID, nullableString(item.ChatID), item.SourceChannel, item.SourceMessageRef, item.ActorType, item.PayloadType,
		nullableString(item.OriginEndpointID), item.Content, structured, item.CorrelationID, idempotencyKey, item.QueueAction, string(item.Status),
		item.MessageID, item.RunID, item.ReceivedAt)
	if err != nil {
		return fmt.Errorf("navi: insert inbox item: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 && strings.TrimSpace(item.IdempotencyKey) != "" {
		return naviruntime.ErrDuplicateInboxItem
	}
	return nil
}

func pendingInboxExistsTx(ctx context.Context, tx *sql.Tx, runtimeSessionID, inboxID string) (bool, error) {
	var dummy string
	err := tx.QueryRowContext(ctx, `
		SELECT inbox_item_id
		FROM navi_inbox
		WHERE inbox_item_id = ? AND runtime_session_id = ? AND status = 'pending'
	`, inboxID, runtimeSessionID).Scan(&dummy)
	switch err {
	case nil:
		return true, nil
	case sql.ErrNoRows:
		return false, nil
	default:
		return false, fmt.Errorf("navi: find pending merge target: %w", err)
	}
}

func findInboxByIdempotencyKeyTx(ctx context.Context, tx *sql.Tx, key string) (*naviruntime.InboxItem, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, nil
	}
	row := tx.QueryRowContext(ctx, `
		SELECT inbox_item_id, runtime_session_id, chat_id,
		       source_channel, source_message_ref, actor_type, payload_type,
		       origin_endpoint_id,
		       content, structured_payload, correlation_id, idempotency_key, queue_action, status,
		       message_id, run_id, received_at
		FROM navi_inbox
		WHERE idempotency_key = ?
		ORDER BY received_at ASC
		LIMIT 1
	`, key)
	item, err := scanInboxItem(row)
	if err == nil {
		return &item, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return nil, fmt.Errorf("navi: find inbox by idempotency key: %w", err)
}

func mergeIntoPendingInboxTx(ctx context.Context, tx *sql.Tx, item *naviruntime.InboxItem) error {
	if item == nil || item.MergedIntoID == "" || item.Content == "" {
		return nil
	}
	// Compare received_at timestamps so that messages are always appended in
	// chronological order regardless of which goroutine wins the DB race.
	// When the incoming item was received before the target (race: target was
	// inserted later but classified first), prepend instead of append so that
	// the merged content reads oldest-first.
	res, err := tx.ExecContext(ctx, `
		UPDATE navi_inbox
		SET content = CASE
			WHEN ? < received_at AND COALESCE(content, '') = '' THEN ?
			WHEN ? < received_at THEN ? || char(10) || content
			WHEN COALESCE(content, '') = '' THEN ?
			ELSE content || char(10) || ?
		END
		WHERE inbox_item_id = ? AND runtime_session_id = ? AND status = 'pending'
	`, item.ReceivedAt, item.Content,
		item.ReceivedAt, item.Content,
		item.Content, item.Content,
		item.MergedIntoID, item.RuntimeSessionID)
	if err != nil {
		return fmt.Errorf("navi: merge pending inbox content: %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return fmt.Errorf("navi: merge target %s is no longer pending", item.MergedIntoID)
	}
	return nil
}

func appendQueueActionEventTx(ctx context.Context, tx *sql.Tx, runtimeSessionID string, item *naviruntime.InboxItem) error {
	if item == nil {
		return nil
	}
	var eventType schema.EventType
	switch item.QueueAction {
	case "merge":
		eventType = schema.FactMessageMerged
	case "defer":
		eventType = schema.FactMessageDeferred
	case "supersede":
		eventType = schema.FactMessageSuperseded
	default:
		return nil
	}
	ev := schema.NewEvent(
		eventType,
		schema.EventKindFact,
		firstNonEmpty(item.ChatID, runtimeSessionID),
		schema.AgentNavi,
		schema.MessageQueueActionPayload{
			RuntimeSessionID:  runtimeSessionID,
			ChatID:            item.ChatID,
			InboxItemID:       item.ID,
			QueueAction:       item.QueueAction,
			Reason:            item.ClassifiedReason,
			Confidence:        item.Confidence,
			TargetInboxItemID: item.MergedIntoID,
		},
	)
	ev.Visibility = schema.VisibilityOperator
	if err := corestore.AppendEventTx(ctx, tx, ev); err != nil {
		return fmt.Errorf("navi: append %s: %w", eventType, err)
	}
	return nil
}

func (s *SQLiteStore) ListPendingRuntimeSessions(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT runtime_session_id
		FROM navi_inbox
		WHERE status IN ('pending', 'deferred')
		  AND COALESCE(TRIM(runtime_session_id), '') != ''
		GROUP BY runtime_session_id
		ORDER BY MIN(received_at) ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("navi: list pending sessions: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var runtimeSessionID string
		if err := rows.Scan(&runtimeSessionID); err != nil {
			return nil, fmt.Errorf("navi: scan pending session: %w", err)
		}
		out = append(out, runtimeSessionID)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ListPendingItems(ctx context.Context, runtimeSessionID string, limit int) ([]naviruntime.InboxItem, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT inbox_item_id, runtime_session_id, chat_id,
		       source_channel, source_message_ref, actor_type, payload_type,
		       origin_endpoint_id,
		       content, structured_payload, correlation_id, idempotency_key, queue_action, status,
		       message_id, run_id, received_at
		FROM navi_inbox
		WHERE runtime_session_id = ? AND status = 'pending'
		ORDER BY received_at ASC
		LIMIT ?
	`, runtimeSessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("navi: list pending items: %w", err)
	}
	defer rows.Close()
	var out []naviruntime.InboxItem
	for rows.Next() {
		item, err := scanInboxItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func scanInboxItem(scanner interface{ Scan(dest ...any) error }) (naviruntime.InboxItem, error) {
	var item naviruntime.InboxItem
	var chatID, sourceMessageRef, structured, idempotencyKey, messageID, runID sql.NullString
	var originEndpointID sql.NullString
	var status string
	if err := scanner.Scan(&item.ID, &item.RuntimeSessionID, &chatID, &item.SourceChannel, &sourceMessageRef, &item.ActorType, &item.PayloadType,
		&originEndpointID, &item.Content, &structured, &item.CorrelationID, &idempotencyKey, &item.QueueAction, &status,
		&messageID, &runID, &item.ReceivedAt); err != nil {
		return naviruntime.InboxItem{}, fmt.Errorf("navi: scan inbox item: %w", err)
	}
	if chatID.Valid {
		item.ChatID = chatID.String
	}
	if sourceMessageRef.Valid {
		item.SourceMessageRef = sourceMessageRef.String
	}
	if originEndpointID.Valid {
		item.OriginEndpointID = originEndpointID.String
	}
	if structured.Valid {
		item.Structured = json.RawMessage(structured.String)
	}
	if idempotencyKey.Valid {
		item.IdempotencyKey = idempotencyKey.String
	}
	if messageID.Valid {
		item.MessageID = messageID.String
	}
	if runID.Valid {
		item.RunID = runID.String
	}
	item.Status = naviruntime.InboxStatus(status)
	return item, nil
}

func (s *SQLiteStore) MarkInboxConsumed(ctx context.Context, inboxID, runID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE navi_inbox
		SET status = 'consumed', run_id = ?, consumed_at = ?
		WHERE inbox_item_id = ?
	`, runID, time.Now().UTC(), inboxID)
	if err != nil {
		return fmt.Errorf("navi: mark inbox consumed: %w", err)
	}
	return nil
}

func (s *SQLiteStore) PromoteDeferredItems(ctx context.Context, runtimeSessionID string, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE navi_inbox
		SET status = 'pending'
		WHERE inbox_item_id IN (
			SELECT inbox_item_id
			FROM navi_inbox
			WHERE runtime_session_id = ? AND status = 'deferred'
			ORDER BY received_at ASC
			LIMIT ?
		)
	`, runtimeSessionID, limit)
	if err != nil {
		return 0, fmt.Errorf("navi: promote deferred inbox items: %w", err)
	}
	rows, _ := res.RowsAffected()
	return int(rows), nil
}

func (s *SQLiteStore) CreateRun(ctx context.Context, run *naviruntime.RunState) error {
	run.ExperienceMode = string(navi.NormalizeExperienceMode(navi.ExperienceMode(run.ExperienceMode)))
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO runtime_runs (
			run_id, runtime_session_id, chat_id, experience_mode, origin_endpoint_id, mode, status, current_phase,
			initiated_by_inbox_item_id, pause_reason, blocked_on_proposal_id, interrupt_class, interrupt_reason, latest_checkpoint_id, scratchpad,
			main_artifact_id, artifact_ids,
			started_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, run.RunID, run.RuntimeSessionID, nullableString(run.ChatID), run.ExperienceMode, nullableString(run.OriginEndpointID), string(run.Mode), string(run.Status), string(run.CurrentPhase),
		run.InitiatedByInboxItemID, run.PauseReason, run.BlockedOnProposalID, string(run.InterruptClass), run.InterruptReason, run.LatestCheckpointID, jsonObject(run.Scratchpad),
		run.MainArtifactID, jsonArray(run.ArtifactIDs),
		run.StartedAt, run.UpdatedAt)
	if err != nil {
		return fmt.Errorf("navi: create run: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdateRun(ctx context.Context, run *naviruntime.RunState) error {
	run.UpdatedAt = time.Now().UTC()
	run.ExperienceMode = string(navi.NormalizeExperienceMode(navi.ExperienceMode(run.ExperienceMode)))
	_, err := s.db.ExecContext(ctx, `
		UPDATE runtime_runs
		SET runtime_session_id = ?, chat_id = ?, experience_mode = ?, origin_endpoint_id = ?, mode = ?, status = ?, current_phase = ?, initiated_by_inbox_item_id = ?,
		    pause_reason = ?, blocked_on_proposal_id = ?, interrupt_class = ?, interrupt_reason = ?, latest_checkpoint_id = ?,
		    scratchpad = ?, main_artifact_id = ?, artifact_ids = ?, updated_at = ?
		WHERE run_id = ?
	`, run.RuntimeSessionID, nullableString(run.ChatID), run.ExperienceMode, nullableString(run.OriginEndpointID), string(run.Mode), string(run.Status), string(run.CurrentPhase), run.InitiatedByInboxItemID,
		run.PauseReason, run.BlockedOnProposalID, string(run.InterruptClass), run.InterruptReason, run.LatestCheckpointID,
		jsonObject(run.Scratchpad), run.MainArtifactID, jsonArray(run.ArtifactIDs), run.UpdatedAt, run.RunID)
	if err != nil {
		return fmt.Errorf("navi: update run: %w", err)
	}
	if isTerminalRunStatus(run.Status) {
		if _, err := s.db.ExecContext(ctx, `UPDATE runtime_checkpoints SET scratchpad = '{}' WHERE run_id = ?`, run.RunID); err != nil {
			return fmt.Errorf("navi: clear checkpoint scratchpad: %w", err)
		}
	}
	return nil
}

func (s *SQLiteStore) GetLatestRun(ctx context.Context, runtimeSessionID string) (*naviruntime.RunState, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT run_id, runtime_session_id, chat_id, experience_mode, origin_endpoint_id, mode, status, current_phase, initiated_by_inbox_item_id,
		       pause_reason, blocked_on_proposal_id, interrupt_class, interrupt_reason, latest_checkpoint_id, scratchpad,
		       main_artifact_id, artifact_ids, started_at, updated_at
		FROM runtime_runs
		WHERE runtime_session_id = ?
		ORDER BY updated_at DESC
		LIMIT 1
	`, runtimeSessionID)
	return s.scanRunWithICSState(ctx, row)
}

func (s *SQLiteStore) GetRun(ctx context.Context, runID string) (*naviruntime.RunState, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT run_id, runtime_session_id, chat_id, experience_mode, origin_endpoint_id, mode, status, current_phase, initiated_by_inbox_item_id,
		       pause_reason, blocked_on_proposal_id, interrupt_class, interrupt_reason, latest_checkpoint_id, scratchpad,
		       main_artifact_id, artifact_ids, started_at, updated_at
		FROM runtime_runs
		WHERE run_id = ?
	`, runID)
	return s.scanRunWithICSState(ctx, row)
}

func (s *SQLiteStore) GetPausedRun(ctx context.Context, runtimeSessionID string) (*naviruntime.RunState, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT run_id, runtime_session_id, chat_id, experience_mode, origin_endpoint_id, mode, status, current_phase, initiated_by_inbox_item_id,
		       pause_reason, blocked_on_proposal_id, interrupt_class, interrupt_reason, latest_checkpoint_id, scratchpad,
		       main_artifact_id, artifact_ids, started_at, updated_at
		FROM runtime_runs
		WHERE runtime_session_id = ? AND status IN (?, ?)
		ORDER BY updated_at DESC
		LIMIT 1
	`, runtimeSessionID, string(schema.RunStatusPaused), string(schema.RunStatusWaitingForProposal))
	return s.scanRunWithICSState(ctx, row)
}

func (s *SQLiteStore) ListActiveRuns(ctx context.Context) ([]*naviruntime.RunState, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT run_id, runtime_session_id, chat_id, experience_mode, origin_endpoint_id, mode, status, current_phase, initiated_by_inbox_item_id,
		       pause_reason, blocked_on_proposal_id, interrupt_class, interrupt_reason, latest_checkpoint_id, scratchpad,
		       main_artifact_id, artifact_ids, started_at, updated_at
		FROM runtime_runs
		WHERE status = ?
	`, string(schema.RunStatusActive))
	if err != nil {
		return nil, fmt.Errorf("navi: list active runs: %w", err)
	}
	var result []*naviruntime.RunState
	for rows.Next() {
		run, err := s.scanRun(rows)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		if run != nil {
			result = append(result, run)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, run := range result {
		if err := s.loadRunICSState(ctx, run); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (s *SQLiteStore) scanRunWithICSState(ctx context.Context, scanner interface{ Scan(dest ...any) error }) (*naviruntime.RunState, error) {
	run, err := s.scanRun(scanner)
	if err != nil || run == nil {
		return run, err
	}
	if err := s.loadRunICSState(ctx, run); err != nil {
		return nil, err
	}
	return run, nil
}

func (s *SQLiteStore) scanRun(scanner interface{ Scan(dest ...any) error }) (*naviruntime.RunState, error) {
	var run naviruntime.RunState
	var mode, status, phase string
	var initiatedBy sql.NullString
	var pauseReason sql.NullString
	var blockedOnProposal sql.NullString
	var interruptClass sql.NullString
	var interruptReason sql.NullString
	var latestCheckpoint sql.NullString
	var scratchpad sql.NullString
	var mainArtifactID sql.NullString
	var artifactIDs sql.NullString
	var runtimeSessionID sql.NullString
	var chatID sql.NullString
	var originEndpointID sql.NullString
	err := scanner.Scan(&run.RunID, &runtimeSessionID, &chatID, &run.ExperienceMode, &originEndpointID, &mode, &status, &phase, &initiatedBy,
		&pauseReason, &blockedOnProposal, &interruptClass, &interruptReason, &latestCheckpoint, &scratchpad,
		&mainArtifactID, &artifactIDs, &run.StartedAt, &run.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("navi: scan run: %w", err)
	}
	if runtimeSessionID.Valid {
		run.RuntimeSessionID = runtimeSessionID.String
	}
	if chatID.Valid {
		run.ChatID = chatID.String
	}
	if originEndpointID.Valid {
		run.OriginEndpointID = originEndpointID.String
	}
	run.ExperienceMode = string(navi.NormalizeExperienceMode(navi.ExperienceMode(run.ExperienceMode)))
	run.Mode = naviruntime.RunMode(mode)
	run.Status = schema.RunStatus(status)
	run.CurrentPhase = naviruntime.RunPhase(phase)
	if initiatedBy.Valid {
		run.InitiatedByInboxItemID = initiatedBy.String
	}
	if pauseReason.Valid {
		run.PauseReason = pauseReason.String
	}
	if blockedOnProposal.Valid {
		run.BlockedOnProposalID = blockedOnProposal.String
	}
	if interruptClass.Valid {
		run.InterruptClass = naviruntime.InterruptClass(interruptClass.String)
	}
	if interruptReason.Valid {
		run.InterruptReason = interruptReason.String
	}
	if latestCheckpoint.Valid {
		run.LatestCheckpointID = latestCheckpoint.String
	}
	if scratchpad.Valid && scratchpad.String != "" {
		_ = json.Unmarshal([]byte(scratchpad.String), &run.Scratchpad)
	}
	if mainArtifactID.Valid {
		run.MainArtifactID = mainArtifactID.String
	}
	if artifactIDs.Valid && artifactIDs.String != "" {
		_ = json.Unmarshal([]byte(artifactIDs.String), &run.ArtifactIDs)
	}
	return &run, nil
}

func (s *SQLiteStore) loadRunICSState(ctx context.Context, run *naviruntime.RunState) error {
	if s == nil || run == nil {
		return nil
	}
	state, err := s.LoadICSState(ctx, run.RunID)
	if err != nil {
		return err
	}
	if state != nil {
		run.ICSStateVersion = state.Version
		if payload, err := json.Marshal(state.DecisionEnvelope); err == nil {
			run.ICSDecisionEnvelope = payload
		}
	}
	return nil
}

func (s *SQLiteStore) SaveCheckpoint(ctx context.Context, cp *naviruntime.Checkpoint) error {
	cp.ExperienceMode = string(navi.NormalizeExperienceMode(navi.ExperienceMode(cp.ExperienceMode)))
	msgsJSON, _ := json.Marshal(cp.LLMMessages)
	var toolJSON any
	if cp.PendingToolCall != nil {
		b, _ := json.Marshal(cp.PendingToolCall)
		toolJSON = string(b)
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO runtime_checkpoints (
			checkpoint_id, run_id, runtime_session_id, chat_id, phase, experience_mode, llm_messages,
			pending_tool_call, pending_proposal_id, pending_proposal_reason, scratchpad,
			main_artifact_id, artifact_ids, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, cp.CheckpointID, cp.RunID, cp.RuntimeSessionID, nullableString(cp.ChatID), string(cp.Phase), cp.ExperienceMode, string(msgsJSON),
		toolJSON, cp.PendingProposalID, cp.PendingProposalReason, jsonObject(cp.Scratchpad),
		cp.MainArtifactID, jsonArray(cp.ArtifactIDs), cp.CreatedAt); err != nil {
		return fmt.Errorf("navi: save checkpoint: %w", err)
	}
	_, err := s.db.ExecContext(ctx, `UPDATE runtime_runs SET latest_checkpoint_id = ?, updated_at = ? WHERE run_id = ?`,
		cp.CheckpointID, time.Now().UTC(), cp.RunID)
	if err != nil {
		return fmt.Errorf("navi: update run checkpoint: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadCheckpoint(ctx context.Context, runID string) (*naviruntime.Checkpoint, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT checkpoint_id, run_id, runtime_session_id, chat_id, phase, experience_mode, llm_messages,
		       pending_tool_call, pending_proposal_id, pending_proposal_reason, scratchpad,
		       main_artifact_id, artifact_ids, created_at
		FROM runtime_checkpoints
		WHERE run_id = ?
		ORDER BY created_at DESC
		LIMIT 1
	`, runID)
	var cp naviruntime.Checkpoint
	var phase string
	var msgsJSON sql.NullString
	var toolJSON sql.NullString
	var scratchpad sql.NullString
	var mainArtifactID sql.NullString
	var artifactIDs sql.NullString
	var runtimeSessionID sql.NullString
	var chatID sql.NullString
	err := row.Scan(&cp.CheckpointID, &cp.RunID, &runtimeSessionID, &chatID, &phase, &cp.ExperienceMode, &msgsJSON,
		&toolJSON, &cp.PendingProposalID, &cp.PendingProposalReason, &scratchpad,
		&mainArtifactID, &artifactIDs, &cp.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("navi: load checkpoint: %w", err)
	}
	if runtimeSessionID.Valid {
		cp.RuntimeSessionID = runtimeSessionID.String
	}
	if chatID.Valid {
		cp.ChatID = chatID.String
	}
	cp.ExperienceMode = string(navi.NormalizeExperienceMode(navi.ExperienceMode(cp.ExperienceMode)))
	cp.Phase = naviruntime.RunPhase(phase)
	if msgsJSON.Valid && msgsJSON.String != "" {
		_ = json.Unmarshal([]byte(msgsJSON.String), &cp.LLMMessages)
	}
	if toolJSON.Valid && toolJSON.String != "" {
		var call llm.ToolCall
		if err := json.Unmarshal([]byte(toolJSON.String), &call); err == nil {
			cp.PendingToolCall = &call
		}
	}
	if scratchpad.Valid && scratchpad.String != "" {
		_ = json.Unmarshal([]byte(scratchpad.String), &cp.Scratchpad)
	}
	if mainArtifactID.Valid {
		cp.MainArtifactID = mainArtifactID.String
	}
	if artifactIDs.Valid && artifactIDs.String != "" {
		_ = json.Unmarshal([]byte(artifactIDs.String), &cp.ArtifactIDs)
	}
	state, err := s.LoadICSState(ctx, cp.RunID)
	if err != nil {
		return nil, err
	}
	if state != nil {
		cp.ICSStateVersion = state.Version
		if payload, err := json.Marshal(state.DecisionEnvelope); err == nil {
			cp.ICSDecisionEnvelope = payload
		}
	}
	return &cp, nil
}

func (s *SQLiteStore) AppendAssistantMessage(ctx context.Context, run *naviruntime.RunState, content, experienceMode, inboxItemID string) (string, error) {
	if run == nil {
		return "", fmt.Errorf("navi: append assistant message: nil run")
	}
	if targetMessageID := strings.TrimSpace(run.Scratchpad["chat_action_target_message_id"]); targetMessageID != "" &&
		strings.TrimSpace(run.Scratchpad["chat_action"]) == "chat_regenerate_variant" {
		return s.appendAssistantMessageVariant(ctx, run, targetMessageID, content, experienceMode, inboxItemID)
	}
	return s.appendAssistantMessageWithKind(ctx, run.RuntimeSessionID, run.ChatID, run.RunID, content, experienceMode, inboxItemID, "", string(schema.AssistantMessageKindReply), assistantMessageMetadata(run))
}

func (s *SQLiteStore) AppendSystemAssistantMessage(ctx context.Context, chatID, content, experienceMode, sourceChannel string, kind string) (string, error) {
	if sourceChannel == "" {
		sourceChannel = "system"
	}
	if kind == "" {
		kind = string(schema.AssistantMessageKindReply)
	}
	return s.appendAssistantMessageWithKind(ctx, "", chatID, "", content, experienceMode, "", sourceChannel, kind, "{}")
}

// assistantToolPartsMetadata renders run tool parts into a metadata_json object
// ("{}" when there are none) so the chat UI can show tool chips after reload.
func assistantToolPartsMetadata(parts []naviruntime.ToolInvocationPart) string {
	if len(parts) == 0 {
		return "{}"
	}
	b, err := json.Marshal(map[string]any{"toolParts": parts})
	if err != nil {
		return "{}"
	}
	return string(b)
}

// assistantMessageMetadata builds the assistant message metadata_json, carrying
// tool chips (toolParts) and, when present, the data-driven render payload the
// render layer produced for this run. The render payload is stored as
// "renderPayload" so the console can render the OpenUI/data-view lane.
func assistantMessageMetadata(run *naviruntime.RunState) string {
	if run == nil {
		return "{}"
	}
	meta := map[string]any{}
	if len(run.ToolParts) > 0 {
		meta["toolParts"] = run.ToolParts
	}
	if len(run.RenderPayload) > 0 {
		var payload any
		if err := json.Unmarshal(run.RenderPayload, &payload); err == nil && payload != nil {
			meta["renderPayload"] = payload
		}
	}
	if strings.TrimSpace(run.MainArtifactID) != "" {
		meta["mainArtifactId"] = strings.TrimSpace(run.MainArtifactID)
	}
	if len(run.ArtifactIDs) > 0 {
		meta["artifactIds"] = append([]string(nil), run.ArtifactIDs...)
	}
	if len(meta) == 0 {
		return "{}"
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func (s *SQLiteStore) appendAssistantMessageWithKind(ctx context.Context, runtimeSessionID, chatID, runID, content, experienceMode, inboxItemID, sourceChannel, kind, metadataJSON string) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("navi: append assistant message begin tx: %w", err)
	}
	defer tx.Rollback()
	msgID := uuid.New().String()
	now := time.Now().UTC()
	kindText := strings.TrimSpace(kind)
	if kindText == "" {
		kindText = string(schema.AssistantMessageKindReply)
	}
	metadataJSON = strings.TrimSpace(metadataJSON)
	if metadataJSON == "" {
		metadataJSON = "{}"
	}
	experienceMode = string(navi.NormalizeExperienceMode(navi.ExperienceMode(experienceMode)))
	if chatID == "" && strings.TrimSpace(inboxItemID) != "" {
		var found sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT chat_id FROM navi_inbox WHERE inbox_item_id = ?`, inboxItemID).Scan(&found); err == nil && found.Valid {
			chatID = found.String
		}
	}
	if runtimeSessionID == "" && strings.TrimSpace(inboxItemID) != "" {
		var found sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT runtime_session_id FROM navi_inbox WHERE inbox_item_id = ?`, inboxItemID).Scan(&found); err == nil && found.Valid {
			runtimeSessionID = found.String
		}
	}
	if chatID != "" {
		if err := ensureChatWritableTx(ctx, tx, chatID, navi.ExperienceMode(experienceMode)); err != nil {
			return "", err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO navi_chat_messages (
				message_id, chat_id, role, content, created_at,
				runtime_session_id, run_id, inbox_item_id,
				source_channel, message_kind, metadata_json
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, msgID, chatID, "assistant", content, now, nullableString(runtimeSessionID), nullableString(runID), nullableString(inboxItemID), sourceChannel, kindText, metadataJSON); err != nil {
			return "", fmt.Errorf("navi: insert assistant chat message: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE navi_chats
			SET updated_at = ?,
			    last_message_at = ?,
			    message_count = message_count + 1,
			    assistant_message_count = assistant_message_count + 1
			WHERE chat_id = ?
		`, now, now, chatID); err != nil {
			return "", fmt.Errorf("navi: update assistant chat stamp: %w", err)
		}
	}
	if runtimeSessionID != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE runtime_sessions SET last_active_at = ? WHERE runtime_session_id = ?`, now, runtimeSessionID); err != nil {
			return "", fmt.Errorf("navi: update runtime session assistant stamp: %w", err)
		}
	}
	correlationID := firstNonEmpty(chatID, runtimeSessionID)
	if correlationID == "" {
		correlationID = msgID
	}
	completed := schema.NewRunEvent(
		schema.FactAssistantMessageCompleted,
		schema.EventKindFact,
		correlationID,
		schema.AgentNavi,
		runID,
		runtimeVisibilityTx(ctx, tx, runtimeSessionID, chatID),
		schema.AssistantMessageCompletedPayload{
			RunID:            runID,
			RuntimeSessionID: runtimeSessionID,
			ChatID:           chatID,
			MessageID:        msgID,
			Content:          content,
			ExperienceMode:   experienceMode,
			InboxItemID:      inboxItemID,
			MessageKind:      kindText,
		},
	)
	if runID != "" {
		slog.Debug("runtime: emitting assistant.message.completed",
			"runtime_session_id", runtimeSessionID,
			"chat_id", chatID,
			"run_id", runID,
			"message_kind", kindText,
			"content_len", len(content),
		)
	}
	if err := corestore.AppendEventTx(ctx, tx, completed); err != nil {
		return "", fmt.Errorf("navi: append assistant.message.completed: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("navi: append assistant message commit: %w", err)
	}
	return msgID, nil
}

func ensureRuntimeSessionWritableTx(ctx context.Context, tx *sql.Tx, runtimeSessionID string, item *naviruntime.InboxItem, experienceMode navi.ExperienceMode) error {
	runtimeSessionID = strings.TrimSpace(runtimeSessionID)
	if runtimeSessionID == "" {
		return fmt.Errorf("navi: runtime session id is required")
	}
	var status sql.NullString
	err := tx.QueryRowContext(ctx, `
		SELECT status
		FROM runtime_sessions
		WHERE runtime_session_id = ?
	`, runtimeSessionID).Scan(&status)
	switch err {
	case nil:
		switch navi.RuntimeSessionStatus(status.String) {
		case navi.RuntimeSessionStatusClosed, navi.RuntimeSessionStatusFailed:
			return fmt.Errorf("navi: runtime session %s is %s", runtimeSessionID, status.String)
		default:
			return nil
		}
	case sql.ErrNoRows:
		now := time.Now().UTC()
		kind := navi.RuntimeSessionKindUser
		if schema.IsInternalRuntimeSessionID(runtimeSessionID) || (item != nil && strings.EqualFold(strings.TrimSpace(item.SourceChannel), "heartbeat")) {
			kind = navi.RuntimeSessionKindInternal
		}
		sourceChannel := ""
		if item != nil {
			sourceChannel = item.SourceChannel
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO runtime_sessions (
				runtime_session_id, kind, status, experience_mode, source_channel,
				started_at, last_active_at, metadata_json
			) VALUES (?, ?, 'active', ?, ?, ?, ?, '{}')
		`, runtimeSessionID, string(kind), string(navi.NormalizeExperienceMode(experienceMode)), sourceChannel, now, now); err != nil {
			return fmt.Errorf("navi: auto-create runtime session: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("navi: inspect runtime session writability: %w", err)
	}
}

func ensureChatWritableTx(ctx context.Context, tx *sql.Tx, chatID string, experienceMode navi.ExperienceMode) error {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return nil
	}
	var status sql.NullString
	var archivedAt, deletedAt sql.NullTime
	err := tx.QueryRowContext(ctx, `
		SELECT status, archived_at, deleted_at
		FROM navi_chats
		WHERE chat_id = ?
	`, chatID).Scan(&status, &archivedAt, &deletedAt)
	switch err {
	case nil:
		if deletedAt.Valid || status.String == string(navi.ChatStatusDeleted) {
			return fmt.Errorf("navi: chat %s is deleted", chatID)
		}
		if archivedAt.Valid || status.String == string(navi.ChatStatusArchived) {
			return fmt.Errorf("navi: chat %s is archived", chatID)
		}
		return nil
	case sql.ErrNoRows:
		now := time.Now().UTC()
		metadata := chatEncodeJSON(map[string]any{"experienceMode": string(navi.NormalizeExperienceMode(experienceMode))})
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO navi_chats (
				chat_id, title, status, visibility, created_at, updated_at, metadata_json
			) VALUES (?, 'New Chat', 'active', 'private', ?, ?, ?)
		`, chatID, now, now, metadata); err != nil {
			return fmt.Errorf("navi: auto-create chat: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("navi: inspect chat writability: %w", err)
	}
}

func runtimeVisibilityTx(ctx context.Context, tx *sql.Tx, runtimeSessionID, chatID string) schema.EventVisibility {
	if strings.TrimSpace(chatID) != "" {
		return schema.VisibilityUser
	}
	var kind sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT kind FROM runtime_sessions WHERE runtime_session_id = ?`, runtimeSessionID).Scan(&kind)
	if err == nil && kind.String == string(navi.RuntimeSessionKindInternal) {
		return schema.VisibilityOperator
	}
	if schema.IsInternalRuntimeSessionID(runtimeSessionID) {
		return schema.VisibilityOperator
	}
	return schema.VisibilityAudit
}

func assistantEventVisibility(kind schema.RuntimeSessionKind, runtimeSessionID string) schema.EventVisibility {
	if schema.IsInternalRuntimeSession(kind, runtimeSessionID) {
		return schema.VisibilityOperator
	}
	return schema.VisibilityUser
}

func (s *SQLiteStore) MarkRunCompleted(ctx context.Context, run *naviruntime.RunState, replyLen int, finalMessageID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("navi: mark run completed begin tx: %w", err)
	}
	defer tx.Rollback()
	run.SetStatus(schema.RunStatusCompleted)
	run.ClearScratchpad()
	now := time.Now().UTC()
	runtimeSessionID := run.RuntimeSessionID
	chatID := strings.TrimSpace(run.ChatID)
	if chatID == "" && strings.TrimSpace(run.InitiatedByInboxItemID) != "" {
		var found sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT chat_id FROM navi_inbox WHERE inbox_item_id = ?`, run.InitiatedByInboxItemID).Scan(&found); err == nil && found.Valid {
			chatID = found.String
			run.ChatID = chatID
		}
	}
	runDone := schema.NewRunEvent(
		schema.FactRunCompleted,
		schema.EventKindFact,
		firstNonEmpty(chatID, runtimeSessionID),
		schema.AgentNavi,
		run.RunID,
		runtimeVisibilityTx(ctx, tx, runtimeSessionID, chatID),
		schema.RunCompletedPayload{
			RunID:            run.RunID,
			RuntimeSessionID: runtimeSessionID,
			ChatID:           chatID,
			ReplyLen:         replyLen,
			ToolCalls:        run.ToolCalls,
			DurationMs:       run.DurationMs(),
			FinalMessageID:   strings.TrimSpace(finalMessageID),
		},
	)
	if run.RunID != "" {
		slog.Debug("runtime: emitting run.completed",
			"runtime_session_id", runtimeSessionID,
			"chat_id", chatID,
			"run_id", run.RunID,
			"reply_len", replyLen,
			"final_message_id", strings.TrimSpace(finalMessageID),
		)
	}
	if err := corestore.AppendEventTx(ctx, tx, runDone); err != nil {
		return fmt.Errorf("navi: append run.completed: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE runtime_runs
		SET status = ?, current_phase = ?, scratchpad = ?, main_artifact_id = ?, artifact_ids = ?, updated_at = ?
		WHERE run_id = ?
	`, string(run.Status), string(run.CurrentPhase), "{}", run.MainArtifactID, jsonArray(run.ArtifactIDs), now, run.RunID); err != nil {
		return fmt.Errorf("navi: update run status: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE runtime_checkpoints SET scratchpad = '{}' WHERE run_id = ?`, run.RunID); err != nil {
		return fmt.Errorf("navi: clear checkpoint scratchpad: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("navi: mark run completed commit: %w", err)
	}
	return nil
}

func (s *SQLiteStore) CompleteRun(ctx context.Context, run *naviruntime.RunState, content string, experienceMode, inboxItemID string) (string, error) {
	msgID, err := s.AppendAssistantMessage(ctx, run, content, experienceMode, inboxItemID)
	if err != nil {
		return "", err
	}
	if err := s.MarkRunCompleted(ctx, run, len(content), msgID); err != nil {
		return msgID, err
	}
	return msgID, nil
}

func runEventVisibility(kind schema.RuntimeSessionKind, runtimeSessionID string) schema.EventVisibility {
	return assistantEventVisibility(kind, runtimeSessionID)
}

func runtimeSessionEventVisibility(kind schema.RuntimeSessionKind, runtimeSessionID string) schema.EventVisibility {
	return assistantEventVisibility(kind, runtimeSessionID)
}

func (s *SQLiteStore) AppendRuntimeEvent(ctx context.Context, ev schema.Event) error {
	return corestore.AppendEvent(ctx, s.db, ev)
}

func jsonArray(v []string) string {
	if v == nil {
		return "[]"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func jsonObject(v map[string]string) string {
	if len(v) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func nullableString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func isTerminalRunStatus(status schema.RunStatus) bool {
	return status == schema.RunStatusCompleted || status == schema.RunStatusFailed || status == schema.RunStatusCancelled
}
