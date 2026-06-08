package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ceoai/navi/internal/navi/compaction"
	naviruntime "github.com/ceoai/navi/internal/runtime"
)

func (s *SQLiteStore) GetChatMemory(ctx context.Context, chatID string) (compaction.ChatMemory, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT schema_version, memory_version, current_epoch_id, chat_frame_json, task_frames_json, COALESCE(retrieval_spans_json, '[]'), updated_at
		FROM navi_chat_memory
		WHERE chat_id = ?
	`, chatID)
	var mem compaction.ChatMemory
	var frameJSON, tasksJSON, retrievalJSON string
	if err := row.Scan(&mem.SchemaVersion, &mem.MemoryVersion, &mem.CurrentEpochID, &frameJSON, &tasksJSON, &retrievalJSON, &mem.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return compaction.ChatMemory{}, nil
		}
		return compaction.ChatMemory{}, fmt.Errorf("navi: get chat memory: %w", err)
	}
	mem.ChatID = chatID
	if err := json.Unmarshal([]byte(frameJSON), &mem.ChatFrame); err != nil {
		return compaction.ChatMemory{}, fmt.Errorf("navi: decode chat frame: %w", err)
	}
	if err := json.Unmarshal([]byte(tasksJSON), &mem.TaskFrames); err != nil {
		return compaction.ChatMemory{}, fmt.Errorf("navi: decode task frames: %w", err)
	}
	if err := json.Unmarshal([]byte(retrievalJSON), &mem.RetrievalSpans); err != nil {
		return compaction.ChatMemory{}, fmt.Errorf("navi: decode retrieval spans: %w", err)
	}
	return mem, nil
}

func (s *SQLiteStore) PutChatMemory(ctx context.Context, memory compaction.ChatMemory) error {
	frameJSON, err := json.Marshal(memory.ChatFrame)
	if err != nil {
		return fmt.Errorf("navi: encode chat frame: %w", err)
	}
	tasksJSON, err := json.Marshal(memory.TaskFrames)
	if err != nil {
		return fmt.Errorf("navi: encode task frames: %w", err)
	}
	retrievalJSON, err := json.Marshal(memory.RetrievalSpans)
	if err != nil {
		return fmt.Errorf("navi: encode retrieval spans: %w", err)
	}
	if memory.UpdatedAt.IsZero() {
		memory.UpdatedAt = time.Now().UTC()
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO navi_chat_memory (
			chat_id, schema_version, memory_version, current_epoch_id, chat_frame_json, task_frames_json, retrieval_spans_json, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(chat_id) DO UPDATE SET
			schema_version=excluded.schema_version,
			memory_version=excluded.memory_version,
			current_epoch_id=excluded.current_epoch_id,
			chat_frame_json=excluded.chat_frame_json,
			task_frames_json=excluded.task_frames_json,
			retrieval_spans_json=excluded.retrieval_spans_json,
			updated_at=excluded.updated_at
	`, memory.ChatID, memory.SchemaVersion, memory.MemoryVersion, memory.CurrentEpochID, string(frameJSON), string(tasksJSON), string(retrievalJSON), memory.UpdatedAt)
	if err != nil {
		return fmt.Errorf("navi: put chat memory: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListCompactionRunSnapshots(ctx context.Context, chatID string) ([]compaction.RunSnapshot, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT run_id, status, current_phase, blocked_on_proposal_id, latest_checkpoint_id,
		       scratchpad, main_artifact_id, artifact_ids, updated_at
		FROM runtime_runs
		WHERE chat_id = ?
		  AND status NOT IN ('completed', 'failed', 'cancelled')
		ORDER BY updated_at DESC, run_id ASC
	`, chatID)
	if err != nil {
		return nil, fmt.Errorf("navi: list compaction run snapshots: %w", err)
	}
	defer rows.Close()
	var out []compaction.RunSnapshot
	for rows.Next() {
		var runID string
		var status string
		var phase string
		var blockedOnProposal sql.NullString
		var latestCheckpoint sql.NullString
		var scratchpadJSON sql.NullString
		var mainArtifactID sql.NullString
		var artifactIDsJSON sql.NullString
		var updatedAt time.Time
		if err := rows.Scan(&runID, &status, &phase, &blockedOnProposal, &latestCheckpoint, &scratchpadJSON, &mainArtifactID, &artifactIDsJSON, &updatedAt); err != nil {
			return nil, fmt.Errorf("navi: scan compaction run snapshot: %w", err)
		}
		snapshot := compaction.RunSnapshot{
			RunID:               runID,
			Status:              status,
			CurrentPhase:        phase,
			LatestCheckpointID:  latestCheckpoint.String,
			BlockedOnProposalID: blockedOnProposal.String,
			MainArtifactID:      mainArtifactID.String,
			UpdatedAt:           updatedAt,
		}
		if scratchpadJSON.Valid && scratchpadJSON.String != "" {
			_ = json.Unmarshal([]byte(scratchpadJSON.String), &snapshot.Scratchpad)
		}
		if artifactIDsJSON.Valid && artifactIDsJSON.String != "" {
			_ = json.Unmarshal([]byte(artifactIDsJSON.String), &snapshot.ArtifactIDs)
		}
		cp, err := s.LoadCheckpoint(ctx, runID)
		if err != nil {
			return nil, fmt.Errorf("navi: load compaction checkpoint snapshot: %w", err)
		}
		if cp != nil {
			populateRunSnapshotFromCheckpoint(&snapshot, cp)
		}
		out = append(out, snapshot)
	}
	return out, rows.Err()
}

func populateRunSnapshotFromCheckpoint(snapshot *compaction.RunSnapshot, cp *naviruntime.Checkpoint) {
	if snapshot == nil || cp == nil {
		return
	}
	snapshot.PendingProposalID = cp.PendingProposalID
	snapshot.PendingProposalReason = cp.PendingProposalReason
	snapshot.PendingToolCall = cp.PendingToolCall != nil
	snapshot.CheckpointMainArtifact = cp.MainArtifactID
	snapshot.CheckpointArtifactIDs = append([]string(nil), cp.ArtifactIDs...)
	snapshot.CheckpointCreatedAt = cp.CreatedAt
}

func (s *SQLiteStore) AppendCompactionCheckpoint(ctx context.Context, checkpoint compaction.CompactionCheckpoint) error {
	checkpointJSON, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("navi: encode checkpoint: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO navi_chat_memory_checkpoints (
			checkpoint_id, chat_id, epoch_id, trigger_class,
			compacted_message_start_id, compacted_message_end_id,
			checkpoint_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, checkpoint.CheckpointID, checkpoint.ChatID, checkpoint.EpochID, string(checkpoint.TriggerClass), checkpoint.CompactedMessageStartID, checkpoint.CompactedMessageEndID, string(checkpointJSON), checkpoint.CreatedAt)
	if err != nil {
		return fmt.Errorf("navi: append compaction checkpoint: %w", err)
	}
	return nil
}

func (s *SQLiteStore) MarkMessagesCompacted(ctx context.Context, chatID, checkpointID, epochID, startMessageID, endMessageID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE navi_chat_messages
		SET compacted_at = ?, compacted_checkpoint_id = ?, compacted_epoch_id = ?
		WHERE chat_id = ?
		  AND compacted_at IS NULL
		  AND created_at >= (SELECT created_at FROM navi_chat_messages WHERE message_id = ?)
		  AND created_at <= (SELECT created_at FROM navi_chat_messages WHERE message_id = ?)
	`, time.Now().UTC(), checkpointID, epochID, chatID, startMessageID, endMessageID)
	if err != nil {
		return fmt.Errorf("navi: mark messages compacted: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListMessagesForCompaction(ctx context.Context, chatID string) ([]compaction.Message, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT message_id, role, COALESCE(run_id, ''), content, created_at, COALESCE(message_kind, '')
		FROM navi_chat_messages
		WHERE chat_id = ?
		ORDER BY created_at ASC
	`, chatID)
	if err != nil {
		return nil, fmt.Errorf("navi: list messages for compaction: %w", err)
	}
	defer rows.Close()
	var out []compaction.Message
	for rows.Next() {
		var m compaction.Message
		if err := rows.Scan(&m.ID, &m.Role, &m.RunID, &m.Content, &m.CreatedAt, &m.MessageKind); err != nil {
			return nil, fmt.Errorf("navi: scan compaction message: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ListRecentUncompactedMessages(ctx context.Context, chatID string, limit int) ([]compaction.Message, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT message_id, role, COALESCE(run_id, ''), content, created_at, COALESCE(message_kind, '')
		FROM navi_chat_messages
		WHERE chat_id = ? AND compacted_at IS NULL
		ORDER BY created_at DESC
		LIMIT ?
	`, chatID, limit)
	if err != nil {
		return nil, fmt.Errorf("navi: list uncompacted messages: %w", err)
	}
	defer rows.Close()
	desc := []compaction.Message{}
	for rows.Next() {
		var m compaction.Message
		if err := rows.Scan(&m.ID, &m.Role, &m.RunID, &m.Content, &m.CreatedAt, &m.MessageKind); err != nil {
			return nil, fmt.Errorf("navi: scan uncompacted message: %w", err)
		}
		desc = append(desc, m)
	}
	for i, j := 0, len(desc)-1; i < j; i, j = i+1, j-1 {
		desc[i], desc[j] = desc[j], desc[i]
	}
	return desc, rows.Err()
}
