package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/navi"
	"github.com/google/uuid"
)

const activeChatMetaKeyPrefix = "active_chat"

func (s *SQLiteStore) CreateChat(ctx context.Context, input navi.CreateChatInput) (*navi.Chat, error) {
	chatID := strings.TrimSpace(input.ID)
	if chatID == "" {
		chatID = uuid.NewString()
	}

	now := time.Now().UTC()

	status := input.Status
	if status == "" {
		status = navi.ChatStatusActive
	}

	visibility := input.Visibility
	if visibility == "" {
		visibility = navi.ChatVisibilityPrivate
	}

	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = "New Chat"
	}

	chat := navi.Chat{
		ID:             navi.ID(chatID),
		OwnerID:        navi.ID(strings.TrimSpace(input.OwnerID)),
		WorkspaceID:    chatStringToIDPtr(input.WorkspaceID),
		ProjectID:      chatStringToIDPtr(input.ProjectID),
		OrganizationID: chatStringToIDPtr(input.OrganizationID),

		Title:       title,
		Description: input.Description,
		Icon:        input.Icon,
		Color:       input.Color,

		Status:     status,
		Visibility: visibility,
		IsPinned:   input.IsPinned,
		IsFavorite: input.IsFavorite,

		CreatedAt: now,
		UpdatedAt: now,

		RootChatID:            chatStringToIDPtr(input.RootChatID),
		ParentChatID:          chatStringToIDPtr(input.ParentChatID),
		BranchedFromMessageID: chatStringToIDPtr(input.BranchedFromMessageID),
		BranchReason:          input.BranchReason,

		AIConfig:       input.AIConfig,
		MemoryPolicy:   input.MemoryPolicy,
		ContextSources: input.ContextSources,

		MessageCount:    0,
		ModerationState: navi.ChatModerationClean,
		Metadata:        input.Metadata,
	}

	if chat.Metadata == nil {
		chat.Metadata = map[string]any{}
	}
	if strings.TrimSpace(input.ExperienceMode) != "" {
		chat.Metadata["experienceMode"] = input.ExperienceMode
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO navi_chats (
			chat_id, owner_id, workspace_id, project_id, organization_id,
			title, description, icon, color,
			status, visibility, is_pinned, is_favorite,
			created_at, updated_at, last_message_at, archived_at, deleted_at,
			root_chat_id, parent_chat_id, branched_from_message_id, branch_reason,
			ai_config_json, memory_policy_json, context_sources_json,
			summary, short_summary, topics_json, tags_json, intent, sentiment, language,
			linked_artifacts_json, linked_files_json, linked_projects_json, linked_tasks_json,
			message_count, user_message_count, assistant_message_count, token_usage_json,
			moderation_state, moderation_flags_json, metadata_json
		) VALUES (
			?, ?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?,
			?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?
		)
	`,
		string(chat.ID), string(chat.OwnerID), chatIDPtrToNullableString(chat.WorkspaceID), chatIDPtrToNullableString(chat.ProjectID), chatIDPtrToNullableString(chat.OrganizationID),
		chat.Title, chat.Description, chat.Icon, chat.Color,
		string(chat.Status), string(chat.Visibility), chatBoolToInt(chat.IsPinned), chatBoolToInt(chat.IsFavorite),
		chat.CreatedAt, chat.UpdatedAt, chatTimePtrToNullable(chat.LastMessageAt), chatTimePtrToNullable(chat.ArchivedAt), chatTimePtrToNullable(chat.DeletedAt),
		chatIDPtrToNullableString(chat.RootChatID), chatIDPtrToNullableString(chat.ParentChatID), chatIDPtrToNullableString(chat.BranchedFromMessageID), chat.BranchReason,
		chatEncodeJSON(chat.AIConfig), chatEncodeJSON(chat.MemoryPolicy), chatEncodeJSON(chat.ContextSources),
		chat.Summary, chat.ShortSummary, chatEncodeJSON(chat.Topics), chatEncodeJSON(chat.Tags), chat.Intent, string(chat.Sentiment), chat.Language,
		chatEncodeJSON(chat.LinkedArtifacts), chatEncodeJSON(chat.LinkedFiles), chatEncodeJSON(chat.LinkedProjects), chatEncodeJSON(chat.LinkedTasks),
		chat.MessageCount, chat.UserMessageCount, chat.AssistantMessageCount, chatEncodeJSON(chat.TokenUsage),
		string(chat.ModerationState), chatEncodeJSON(chat.ModerationFlags), chatEncodeJSON(chat.Metadata),
	)
	if err != nil {
		return nil, fmt.Errorf("navi: create chat: %w", err)
	}

	return &chat, nil
}

func (s *SQLiteStore) GetChat(ctx context.Context, chatID string) (*navi.Chat, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT
			chat_id, owner_id, workspace_id, project_id, organization_id,
			title, description, icon, color,
			status, visibility, is_pinned, is_favorite,
			created_at, updated_at, last_message_at, archived_at, deleted_at,
			root_chat_id, parent_chat_id, branched_from_message_id, branch_reason,
			ai_config_json, memory_policy_json, context_sources_json,
			summary, short_summary, topics_json, tags_json, intent, sentiment, language,
			linked_artifacts_json, linked_files_json, linked_projects_json, linked_tasks_json,
			message_count, user_message_count, assistant_message_count, token_usage_json,
			moderation_state, moderation_flags_json, metadata_json
		FROM navi_chats
		WHERE chat_id = ?
	`, chatID)

	chat, err := scanChat(row)
	// scanChat wraps the scan error (%w), so compare with errors.Is rather than
	// ==; otherwise a missing chat surfaces as a generic 500 instead of 404.
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("navi: get chat: not found")
	}
	if err != nil {
		return nil, err
	}

	return chat, nil
}

func (s *SQLiteStore) RenameChat(ctx context.Context, chatID, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("navi: rename chat: title is required")
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE navi_chats
		SET title = ?,
		    updated_at = ?
		WHERE chat_id = ?
		  AND status != 'deleted'
	`, title, now, strings.TrimSpace(chatID))
	if err != nil {
		return fmt.Errorf("navi: rename chat: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("navi: chat %s not found", chatID)
	}
	return nil
}

func (s *SQLiteStore) GetChatWithMessages(ctx context.Context, chatID string) (*navi.ChatThread, error) {
	chat, err := s.GetChat(ctx, chatID)
	if err != nil {
		return nil, err
	}
	offset := chat.MessageCount - 50
	if offset < 0 {
		offset = 0
	}
	messages, err := s.ListChatMessages(ctx, chatID, 50, offset)
	if err != nil {
		return nil, err
	}
	return &navi.ChatThread{Chat: *chat, Messages: messages}, nil
}

func (s *SQLiteStore) ListChats(ctx context.Context, limit int) ([]navi.Chat, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			chat_id, owner_id, workspace_id, project_id, organization_id,
			title, description, icon, color,
			status, visibility, is_pinned, is_favorite,
			created_at, updated_at, last_message_at, archived_at, deleted_at,
			root_chat_id, parent_chat_id, branched_from_message_id, branch_reason,
			ai_config_json, memory_policy_json, context_sources_json,
			summary, short_summary, topics_json, tags_json, intent, sentiment, language,
			linked_artifacts_json, linked_files_json, linked_projects_json, linked_tasks_json,
			message_count, user_message_count, assistant_message_count, token_usage_json,
			moderation_state, moderation_flags_json, metadata_json
		FROM navi_chats
		WHERE status != 'deleted'
		  AND archived_at IS NULL
		ORDER BY COALESCE(last_message_at, updated_at, created_at) DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("navi: list chats: %w", err)
	}
	defer rows.Close()

	var chats []navi.Chat
	for rows.Next() {
		chat, err := scanChat(rows)
		if err != nil {
			return nil, err
		}
		chats = append(chats, *chat)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("navi: list chats rows: %w", err)
	}

	return chats, nil
}

func (s *SQLiteStore) ListChatsByProject(ctx context.Context, projectID string, limit int) ([]navi.Chat, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return s.ListChats(ctx, limit)
	}
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			chat_id, owner_id, workspace_id, project_id, organization_id,
			title, description, icon, color,
			status, visibility, is_pinned, is_favorite,
			created_at, updated_at, last_message_at, archived_at, deleted_at,
			root_chat_id, parent_chat_id, branched_from_message_id, branch_reason,
			ai_config_json, memory_policy_json, context_sources_json,
			summary, short_summary, topics_json, tags_json, intent, sentiment, language,
			linked_artifacts_json, linked_files_json, linked_projects_json, linked_tasks_json,
			message_count, user_message_count, assistant_message_count, token_usage_json,
			moderation_state, moderation_flags_json, metadata_json
		FROM navi_chats
		WHERE status != 'deleted'
		  AND archived_at IS NULL
		  AND project_id = ?
		ORDER BY COALESCE(last_message_at, updated_at, created_at) DESC
		LIMIT ?
	`, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("navi: list project chats: %w", err)
	}
	defer rows.Close()

	var chats []navi.Chat
	for rows.Next() {
		chat, err := scanChat(rows)
		if err != nil {
			return nil, err
		}
		chats = append(chats, *chat)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("navi: list project chats rows: %w", err)
	}
	return chats, nil
}

func (s *SQLiteStore) UpdateChat(ctx context.Context, chat navi.Chat) error {
	chat.UpdatedAt = time.Now().UTC()

	_, err := s.db.ExecContext(ctx, `
		UPDATE navi_chats
		SET
			owner_id = ?,
			workspace_id = ?,
			project_id = ?,
			organization_id = ?,
			title = ?,
			description = ?,
			icon = ?,
			color = ?,
			status = ?,
			visibility = ?,
			is_pinned = ?,
			is_favorite = ?,
			updated_at = ?,
			last_message_at = ?,
			archived_at = ?,
			deleted_at = ?,
			root_chat_id = ?,
			parent_chat_id = ?,
			branched_from_message_id = ?,
			branch_reason = ?,
			ai_config_json = ?,
			memory_policy_json = ?,
			context_sources_json = ?,
			summary = ?,
			short_summary = ?,
			topics_json = ?,
			tags_json = ?,
			intent = ?,
			sentiment = ?,
			language = ?,
			linked_artifacts_json = ?,
			linked_files_json = ?,
			linked_projects_json = ?,
			linked_tasks_json = ?,
			message_count = ?,
			user_message_count = ?,
			assistant_message_count = ?,
			token_usage_json = ?,
			moderation_state = ?,
			moderation_flags_json = ?,
			metadata_json = ?
		WHERE chat_id = ?
	`,
		string(chat.OwnerID),
		chatIDPtrToNullableString(chat.WorkspaceID),
		chatIDPtrToNullableString(chat.ProjectID),
		chatIDPtrToNullableString(chat.OrganizationID),
		chat.Title,
		chat.Description,
		chat.Icon,
		chat.Color,
		string(chat.Status),
		string(chat.Visibility),
		chatBoolToInt(chat.IsPinned),
		chatBoolToInt(chat.IsFavorite),
		chat.UpdatedAt,
		chatTimePtrToNullable(chat.LastMessageAt),
		chatTimePtrToNullable(chat.ArchivedAt),
		chatTimePtrToNullable(chat.DeletedAt),
		chatIDPtrToNullableString(chat.RootChatID),
		chatIDPtrToNullableString(chat.ParentChatID),
		chatIDPtrToNullableString(chat.BranchedFromMessageID),
		chat.BranchReason,
		chatEncodeJSON(chat.AIConfig),
		chatEncodeJSON(chat.MemoryPolicy),
		chatEncodeJSON(chat.ContextSources),
		chat.Summary,
		chat.ShortSummary,
		chatEncodeJSON(chat.Topics),
		chatEncodeJSON(chat.Tags),
		chat.Intent,
		string(chat.Sentiment),
		chat.Language,
		chatEncodeJSON(chat.LinkedArtifacts),
		chatEncodeJSON(chat.LinkedFiles),
		chatEncodeJSON(chat.LinkedProjects),
		chatEncodeJSON(chat.LinkedTasks),
		chat.MessageCount,
		chat.UserMessageCount,
		chat.AssistantMessageCount,
		chatEncodeJSON(chat.TokenUsage),
		string(chat.ModerationState),
		chatEncodeJSON(chat.ModerationFlags),
		chatEncodeJSON(chat.Metadata),
		string(chat.ID),
	)
	if err != nil {
		return fmt.Errorf("navi: update chat: %w", err)
	}

	return nil
}

func (s *SQLiteStore) ArchiveChat(ctx context.Context, chatID string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE navi_chats
		SET status = 'archived',
		    archived_at = COALESCE(archived_at, ?),
		    updated_at = ?
		WHERE chat_id = ?
	`, now, now, chatID)
	if err != nil {
		return fmt.Errorf("navi: archive chat: %w", err)
	}
	return nil
}

func (s *SQLiteStore) DeleteChat(ctx context.Context, chatID string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE navi_chats
		SET status = 'deleted',
		    deleted_at = COALESCE(deleted_at, ?),
		    updated_at = ?
		WHERE chat_id = ?
	`, now, now, chatID)
	if err != nil {
		return fmt.Errorf("navi: delete chat: %w", err)
	}
	return nil
}

func (s *SQLiteStore) AppendChatMessage(ctx context.Context, chatID string, msg navi.ChatMessage) (string, error) {
	msgID := strings.TrimSpace(string(msg.ID))
	if msgID == "" {
		msgID = uuid.NewString()
	}

	now := msg.CreatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}

	role := strings.TrimSpace(msg.Role)
	if role == "" {
		role = "user"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("navi: append chat message begin tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO navi_chat_messages (
			message_id, chat_id, role, content, created_at,
			runtime_session_id, run_id, inbox_item_id,
			source_channel, source_message_ref, origin_endpoint_id, message_kind,
			compacted_at, compacted_checkpoint_id, compacted_epoch_id,
			metadata_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		msgID,
		chatID,
		role,
		msg.Content,
		now,
		chatIDPtrToNullableString(msg.RuntimeSessionID),
		chatIDPtrToNullableString(msg.RunID),
		chatIDPtrToNullableString(msg.InboxItemID),
		msg.SourceChannel,
		msg.SourceMessageRef,
		chatIDPtrToNullableString(msg.OriginEndpointID),
		msg.MessageKind,
		chatTimePtrToNullable(msg.CompactedAt),
		chatIDPtrToNullableString(msg.CompactedCheckpointID),
		chatIDPtrToNullableString(msg.CompactedEpochID),
		chatEncodeJSON(msg.Metadata),
	)
	if err != nil {
		return "", fmt.Errorf("navi: append chat message insert: %w", err)
	}

	userDelta := 0
	assistantDelta := 0
	switch role {
	case "user":
		userDelta = 1
	case "assistant", "navi":
		assistantDelta = 1
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE navi_chats
		SET updated_at = ?,
		    last_message_at = ?,
		    message_count = message_count + 1,
		    user_message_count = user_message_count + ?,
		    assistant_message_count = assistant_message_count + ?
		WHERE chat_id = ?
	`, now, now, userDelta, assistantDelta, chatID)
	if err != nil {
		return "", fmt.Errorf("navi: append chat message update chat: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("navi: append chat message commit: %w", err)
	}

	return msgID, nil
}

// SetChatMessageFeedback records owner feedback on a single message by merging a
// "feedback" key into its metadata_json (no schema migration needed). rating must
// be "up", "down", or "" (clear).
func (s *SQLiteStore) SetChatMessageFeedback(ctx context.Context, chatID, messageID, rating string) error {
	rating = strings.TrimSpace(strings.ToLower(rating))
	switch rating {
	case "", "up", "down":
	default:
		return fmt.Errorf("navi: invalid feedback rating %q", rating)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("navi: set feedback begin tx: %w", err)
	}
	defer tx.Rollback()

	var metaRaw string
	err = tx.QueryRowContext(ctx, `
		SELECT metadata_json FROM navi_chat_messages WHERE message_id = ? AND chat_id = ?
	`, messageID, chatID).Scan(&metaRaw)
	if err == sql.ErrNoRows {
		return fmt.Errorf("navi: chat message not found: %w", err)
	}
	if err != nil {
		return fmt.Errorf("navi: set feedback read: %w", err)
	}

	meta := map[string]any{}
	if decodeErr := chatDecodeJSON(metaRaw, &meta); decodeErr != nil || meta == nil {
		meta = map[string]any{}
	}
	if rating == "" {
		delete(meta, "feedback")
	} else {
		meta["feedback"] = rating
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE navi_chat_messages SET metadata_json = ? WHERE message_id = ? AND chat_id = ?
	`, chatEncodeJSON(meta), messageID, chatID); err != nil {
		return fmt.Errorf("navi: set feedback update: %w", err)
	}

	return tx.Commit()
}

// DeleteChatMessagesFrom removes the given message and every message after it
// (by created_at, rowid order) from the chat, then recomputes the chat counters.
// Used by edit-and-resend to truncate the thread before re-running the turn.
func (s *SQLiteStore) DeleteChatMessagesFrom(ctx context.Context, chatID, messageID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("navi: truncate begin tx: %w", err)
	}
	defer tx.Rollback()

	var createdAt time.Time
	var rowID int64
	err = tx.QueryRowContext(ctx, `
		SELECT created_at, rowid FROM navi_chat_messages WHERE message_id = ? AND chat_id = ?
	`, messageID, chatID).Scan(&createdAt, &rowID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("navi: chat message not found: %w", err)
	}
	if err != nil {
		return fmt.Errorf("navi: truncate read target: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM navi_chat_messages
		WHERE chat_id = ? AND (created_at, rowid) >= (?, ?)
	`, chatID, createdAt, rowID); err != nil {
		return fmt.Errorf("navi: truncate delete: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE navi_chats SET
			message_count = (SELECT COUNT(*) FROM navi_chat_messages WHERE chat_id = ?),
			user_message_count = (SELECT COUNT(*) FROM navi_chat_messages WHERE chat_id = ? AND role = 'user'),
			assistant_message_count = (SELECT COUNT(*) FROM navi_chat_messages WHERE chat_id = ? AND role IN ('assistant','navi')),
			last_message_at = (SELECT MAX(created_at) FROM navi_chat_messages WHERE chat_id = ?),
			updated_at = ?
		WHERE chat_id = ?
	`, chatID, chatID, chatID, chatID, time.Now().UTC(), chatID); err != nil {
		return fmt.Errorf("navi: truncate recompute counters: %w", err)
	}

	return tx.Commit()
}

func (s *SQLiteStore) ListChatMessages(ctx context.Context, chatID string, limit, offset int) ([]navi.ChatMessage, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			message_id, chat_id, role, content, created_at,
			runtime_session_id, run_id, inbox_item_id,
			source_channel, source_message_ref, origin_endpoint_id, message_kind,
			compacted_at, compacted_checkpoint_id, compacted_epoch_id,
			metadata_json
		FROM navi_chat_messages
		WHERE chat_id = ?
		ORDER BY created_at ASC, rowid ASC
		LIMIT ? OFFSET ?
	`, chatID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("navi: list chat messages: %w", err)
	}
	defer rows.Close()

	var messages []navi.ChatMessage
	for rows.Next() {
		msg, err := scanChatMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, *msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("navi: list chat messages rows: %w", err)
	}

	return messages, nil
}

func (s *SQLiteStore) MessageCount(ctx context.Context, chatID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM navi_chat_messages
		WHERE chat_id = ?
	`, chatID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("navi: chat message count: %w", err)
	}
	return count, nil
}

func (s *SQLiteStore) ActiveChatID(ctx context.Context, scope navi.ActiveChatScope) (string, error) {
	return s.GetMeta(ctx, activeChatMetaKey(scope))
}

func (s *SQLiteStore) SetActiveChat(ctx context.Context, scope navi.ActiveChatScope, chatID string) error {
	return s.SetMeta(ctx, activeChatMetaKey(scope), chatID)
}

func activeChatMetaKey(scope navi.ActiveChatScope) string {
	parts := []string{
		activeChatMetaKeyPrefix,
		chatScopePart(scope.OwnerID),
		chatScopePart(scope.ProjectID),
		chatScopePart(scope.WorkspaceID),
		chatScopePart(scope.SourceChannel),
	}
	return strings.Join(parts, ":")
}

func chatScopePart(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "_"
	}
	value = strings.ReplaceAll(value, ":", "_")
	return value
}

func scanChat(scanner interface{ Scan(dest ...any) error }) (*navi.Chat, error) {
	var chat navi.Chat

	var workspaceID sql.NullString
	var projectID sql.NullString
	var organizationID sql.NullString
	var rootChatID sql.NullString
	var parentChatID sql.NullString
	var branchedFromMessageID sql.NullString

	var status string
	var visibility string
	var sentiment string
	var moderationState string

	var isPinned int
	var isFavorite int

	var lastMessageAt sql.NullTime
	var archivedAt sql.NullTime
	var deletedAt sql.NullTime

	var aiConfigJSON string
	var memoryPolicyJSON string
	var contextSourcesJSON string
	var topicsJSON string
	var tagsJSON string
	var linkedArtifactsJSON string
	var linkedFilesJSON string
	var linkedProjectsJSON string
	var linkedTasksJSON string
	var tokenUsageJSON string
	var moderationFlagsJSON string
	var metadataJSON string

	err := scanner.Scan(
		&chat.ID,
		&chat.OwnerID,
		&workspaceID,
		&projectID,
		&organizationID,
		&chat.Title,
		&chat.Description,
		&chat.Icon,
		&chat.Color,
		&status,
		&visibility,
		&isPinned,
		&isFavorite,
		&chat.CreatedAt,
		&chat.UpdatedAt,
		&lastMessageAt,
		&archivedAt,
		&deletedAt,
		&rootChatID,
		&parentChatID,
		&branchedFromMessageID,
		&chat.BranchReason,
		&aiConfigJSON,
		&memoryPolicyJSON,
		&contextSourcesJSON,
		&chat.Summary,
		&chat.ShortSummary,
		&topicsJSON,
		&tagsJSON,
		&chat.Intent,
		&sentiment,
		&chat.Language,
		&linkedArtifactsJSON,
		&linkedFilesJSON,
		&linkedProjectsJSON,
		&linkedTasksJSON,
		&chat.MessageCount,
		&chat.UserMessageCount,
		&chat.AssistantMessageCount,
		&tokenUsageJSON,
		&moderationState,
		&moderationFlagsJSON,
		&metadataJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("navi: scan chat: %w", err)
	}

	chat.WorkspaceID = chatNullableStringToIDPtr(workspaceID)
	chat.ProjectID = chatNullableStringToIDPtr(projectID)
	chat.OrganizationID = chatNullableStringToIDPtr(organizationID)
	chat.RootChatID = chatNullableStringToIDPtr(rootChatID)
	chat.ParentChatID = chatNullableStringToIDPtr(parentChatID)
	chat.BranchedFromMessageID = chatNullableStringToIDPtr(branchedFromMessageID)

	if lastMessageAt.Valid {
		chat.LastMessageAt = &lastMessageAt.Time
	}
	if archivedAt.Valid {
		chat.ArchivedAt = &archivedAt.Time
	}
	if deletedAt.Valid {
		chat.DeletedAt = &deletedAt.Time
	}

	chat.Status = navi.ChatStatus(status)
	if chat.Status == "" {
		chat.Status = navi.ChatStatusActive
	}
	chat.Visibility = navi.ChatVisibility(visibility)
	if chat.Visibility == "" {
		chat.Visibility = navi.ChatVisibilityPrivate
	}
	chat.IsPinned = isPinned != 0
	chat.IsFavorite = isFavorite != 0
	chat.Sentiment = navi.ChatSentiment(sentiment)
	chat.ModerationState = navi.ChatModerationState(moderationState)
	if chat.ModerationState == "" {
		chat.ModerationState = navi.ChatModerationClean
	}

	_ = chatDecodeJSON(aiConfigJSON, &chat.AIConfig)
	_ = chatDecodeJSON(memoryPolicyJSON, &chat.MemoryPolicy)
	_ = chatDecodeJSON(contextSourcesJSON, &chat.ContextSources)
	_ = chatDecodeJSON(topicsJSON, &chat.Topics)
	_ = chatDecodeJSON(tagsJSON, &chat.Tags)
	_ = chatDecodeJSON(linkedArtifactsJSON, &chat.LinkedArtifacts)
	_ = chatDecodeJSON(linkedFilesJSON, &chat.LinkedFiles)
	_ = chatDecodeJSON(linkedProjectsJSON, &chat.LinkedProjects)
	_ = chatDecodeJSON(linkedTasksJSON, &chat.LinkedTasks)
	_ = chatDecodeJSON(tokenUsageJSON, &chat.TokenUsage)
	_ = chatDecodeJSON(moderationFlagsJSON, &chat.ModerationFlags)
	_ = chatDecodeJSON(metadataJSON, &chat.Metadata)

	return &chat, nil
}

func scanChatMessage(scanner interface{ Scan(dest ...any) error }) (*navi.ChatMessage, error) {
	var msg navi.ChatMessage

	var runtimeSessionID sql.NullString
	var runID sql.NullString
	var inboxItemID sql.NullString
	var sourceChannel sql.NullString
	var sourceMessageRef sql.NullString
	var originEndpointID sql.NullString
	var messageKind sql.NullString
	var compactedAt sql.NullTime
	var compactedCheckpointID sql.NullString
	var compactedEpochID sql.NullString
	var metadataJSON string

	err := scanner.Scan(
		&msg.ID,
		&msg.ChatID,
		&msg.Role,
		&msg.Content,
		&msg.CreatedAt,
		&runtimeSessionID,
		&runID,
		&inboxItemID,
		&sourceChannel,
		&sourceMessageRef,
		&originEndpointID,
		&messageKind,
		&compactedAt,
		&compactedCheckpointID,
		&compactedEpochID,
		&metadataJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("navi: scan chat message: %w", err)
	}

	msg.RuntimeSessionID = chatNullableStringToIDPtr(runtimeSessionID)
	msg.RunID = chatNullableStringToIDPtr(runID)
	msg.InboxItemID = chatNullableStringToIDPtr(inboxItemID)
	msg.OriginEndpointID = chatNullableStringToIDPtr(originEndpointID)
	if sourceChannel.Valid {
		msg.SourceChannel = sourceChannel.String
	}
	if sourceMessageRef.Valid {
		msg.SourceMessageRef = sourceMessageRef.String
	}
	if messageKind.Valid {
		msg.MessageKind = messageKind.String
	}
	msg.CompactedCheckpointID = chatNullableStringToIDPtr(compactedCheckpointID)
	msg.CompactedEpochID = chatNullableStringToIDPtr(compactedEpochID)

	if compactedAt.Valid {
		msg.CompactedAt = &compactedAt.Time
	}

	_ = chatDecodeJSON(metadataJSON, &msg.Metadata)

	return &msg, nil
}

func chatStringToIDPtr(value string) *navi.ID {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	id := navi.ID(value)
	return &id
}

func chatNullableStringToIDPtr(value sql.NullString) *navi.ID {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil
	}
	id := navi.ID(value.String)
	return &id
}

func chatIDPtrToNullableString(value *navi.ID) any {
	if value == nil || strings.TrimSpace(string(*value)) == "" {
		return nil
	}
	return string(*value)
}

func chatTimePtrToNullable(value *time.Time) any {
	if value == nil {
		return nil
	}
	return *value
}

func chatBoolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func chatEncodeJSON(value any) string {
	if value == nil {
		return "{}"
	}
	b, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func chatDecodeJSON(raw string, dst any) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "{}"
	}
	return json.Unmarshal([]byte(raw), dst)
}
