package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/open-navi/navi/internal/navi"
	naviruntime "github.com/open-navi/navi/internal/runtime"
	"github.com/open-navi/navi/internal/schema"
)

func (s *SQLiteStore) EnsureMessageVariantGroup(ctx context.Context, chatID, messageID string) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("navi: ensure message variant group begin tx: %w", err)
	}
	defer tx.Rollback()
	groupID, _, _, err := ensureMessageVariantGroupTx(ctx, tx, chatID, messageID)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("navi: ensure message variant group commit: %w", err)
	}
	return groupID, nil
}

func (s *SQLiteStore) AppendMessageVariant(ctx context.Context, chatID, messageID, content string) (navi.MessageVariant, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return navi.MessageVariant{}, fmt.Errorf("navi: append message variant begin tx: %w", err)
	}
	defer tx.Rollback()
	groupID, _, _, err := ensureMessageVariantGroupTx(ctx, tx, chatID, messageID)
	if err != nil {
		return navi.MessageVariant{}, err
	}
	variant, err := appendSelectedMessageVariantTx(ctx, tx, chatID, messageID, groupID, content)
	if err != nil {
		return navi.MessageVariant{}, err
	}
	if err := tx.Commit(); err != nil {
		return navi.MessageVariant{}, fmt.Errorf("navi: append message variant commit: %w", err)
	}
	return variant, nil
}

func (s *SQLiteStore) ListMessageVariants(ctx context.Context, chatID, messageID string) (navi.MessageVariants, error) {
	groupID, content, err := s.messageVariantGroup(ctx, chatID, messageID)
	if err != nil {
		return navi.MessageVariants{}, err
	}
	if groupID == "" {
		return navi.MessageVariants{
			Variants:      []navi.MessageVariant{{ID: messageID, Index: 0, Content: content}},
			SelectedIndex: 0,
		}, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT variant_id, variant_index, content, selected
		FROM navi_chat_message_variants
		WHERE chat_id = ? AND message_id = ? AND variant_group_id = ?
		ORDER BY variant_index ASC
	`, chatID, messageID, groupID)
	if err != nil {
		return navi.MessageVariants{}, fmt.Errorf("navi: list message variants: %w", err)
	}
	defer rows.Close()
	out := navi.MessageVariants{SelectedIndex: 0}
	for rows.Next() {
		var v navi.MessageVariant
		var selected int
		if err := rows.Scan(&v.ID, &v.Index, &v.Content, &selected); err != nil {
			return navi.MessageVariants{}, fmt.Errorf("navi: scan message variant: %w", err)
		}
		if selected == 1 {
			out.SelectedIndex = v.Index
		}
		out.Variants = append(out.Variants, v)
	}
	if err := rows.Err(); err != nil {
		return navi.MessageVariants{}, fmt.Errorf("navi: list message variants rows: %w", err)
	}
	if len(out.Variants) == 0 {
		out.Variants = []navi.MessageVariant{{ID: messageID, Index: 0, Content: content}}
	}
	return out, nil
}

func (s *SQLiteStore) SelectMessageVariant(ctx context.Context, chatID, messageID string, index int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("navi: select message variant begin tx: %w", err)
	}
	defer tx.Rollback()
	groupID, _, _, err := ensureMessageVariantGroupTx(ctx, tx, chatID, messageID)
	if err != nil {
		return err
	}
	var content string
	err = tx.QueryRowContext(ctx, `
		SELECT content FROM navi_chat_message_variants
		WHERE chat_id = ? AND message_id = ? AND variant_group_id = ? AND variant_index = ?
	`, chatID, messageID, groupID, index).Scan(&content)
	if err == sql.ErrNoRows {
		return fmt.Errorf("navi: message variant not found")
	}
	if err != nil {
		return fmt.Errorf("navi: select message variant read: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE navi_chat_message_variants SET selected = CASE WHEN variant_index = ? THEN 1 ELSE 0 END
		WHERE chat_id = ? AND message_id = ? AND variant_group_id = ?
	`, index, chatID, messageID, groupID); err != nil {
		return fmt.Errorf("navi: select message variant flags: %w", err)
	}
	if err := updateMessageSelectedVariantTx(ctx, tx, chatID, messageID, content, groupID, index); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) appendAssistantMessageVariant(ctx context.Context, run *naviruntime.RunState, messageID, content, experienceMode, inboxItemID string) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("navi: append assistant message variant begin tx: %w", err)
	}
	defer tx.Rollback()
	chatID := strings.TrimSpace(run.ChatID)
	if chatID == "" && strings.TrimSpace(inboxItemID) != "" {
		var found sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT chat_id FROM navi_inbox WHERE inbox_item_id = ?`, inboxItemID).Scan(&found); err == nil && found.Valid {
			chatID = found.String
		}
	}
	if chatID == "" {
		return "", fmt.Errorf("navi: append assistant message variant: chat id is required")
	}
	groupID, _, _, err := ensureMessageVariantGroupTx(ctx, tx, chatID, messageID)
	if err != nil {
		return "", err
	}
	if scratchGroup := strings.TrimSpace(run.Scratchpad["chat_action_variant_group_id"]); scratchGroup != "" {
		groupID = scratchGroup
	}
	if _, err := appendSelectedMessageVariantTx(ctx, tx, chatID, messageID, groupID, content); err != nil {
		return "", err
	}
	now := time.Now().UTC()
	experienceMode = string(navi.NormalizeExperienceMode(navi.ExperienceMode(experienceMode)))
	if _, err := tx.ExecContext(ctx, `
		UPDATE navi_chat_messages
		SET runtime_session_id = ?,
		    run_id = ?,
		    inbox_item_id = ?,
		    message_kind = ?
		WHERE chat_id = ? AND message_id = ?
	`, nullableString(run.RuntimeSessionID), nullableString(run.RunID), nullableString(inboxItemID), string(schema.AssistantMessageKindReply), chatID, messageID); err != nil {
		return "", fmt.Errorf("navi: update assistant variant run refs: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE navi_chats
		SET updated_at = ?,
		    last_message_at = ?
		WHERE chat_id = ?
	`, now, now, chatID); err != nil {
		return "", fmt.Errorf("navi: update assistant variant chat stamp: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("navi: append assistant message variant commit: %w", err)
	}
	return messageID, nil
}

func (s *SQLiteStore) messageVariantGroup(ctx context.Context, chatID, messageID string) (string, string, error) {
	var metaRaw string
	var content string
	err := s.db.QueryRowContext(ctx, `
		SELECT content, metadata_json FROM navi_chat_messages WHERE chat_id = ? AND message_id = ?
	`, chatID, messageID).Scan(&content, &metaRaw)
	if err == sql.ErrNoRows {
		return "", "", fmt.Errorf("navi: chat message not found")
	}
	if err != nil {
		return "", "", fmt.Errorf("navi: read message variant group: %w", err)
	}
	meta := map[string]any{}
	_ = chatDecodeJSON(metaRaw, &meta)
	return stringMeta(meta, "variantGroupId"), content, nil
}

func ensureMessageVariantGroupTx(ctx context.Context, tx *sql.Tx, chatID, messageID string) (string, string, map[string]any, error) {
	var role string
	var content string
	var metaRaw string
	err := tx.QueryRowContext(ctx, `
		SELECT role, content, metadata_json FROM navi_chat_messages WHERE chat_id = ? AND message_id = ?
	`, chatID, messageID).Scan(&role, &content, &metaRaw)
	if err == sql.ErrNoRows {
		return "", "", nil, fmt.Errorf("navi: chat message not found")
	}
	if err != nil {
		return "", "", nil, fmt.Errorf("navi: ensure message variant group read: %w", err)
	}
	if role := strings.ToLower(strings.TrimSpace(role)); role != "assistant" && role != "navi" {
		return "", "", nil, fmt.Errorf("navi: only assistant messages support variants")
	}
	meta := map[string]any{}
	if err := chatDecodeJSON(metaRaw, &meta); err != nil || meta == nil {
		meta = map[string]any{}
	}
	groupID := stringMeta(meta, "variantGroupId")
	if groupID == "" {
		groupID = uuid.NewString()
	}
	meta["variantGroupId"] = groupID
	meta["selectedVariantIndex"] = 0
	if _, ok := meta["variantCount"]; !ok {
		meta["variantCount"] = 1
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO navi_chat_message_variants (
			variant_id, chat_id, message_id, variant_group_id, variant_index, content, selected, created_at
		) VALUES (?, ?, ?, ?, 0, ?, 1, ?)
	`, uuid.NewString(), chatID, messageID, groupID, content, time.Now().UTC()); err != nil {
		return "", "", nil, fmt.Errorf("navi: insert original message variant: %w", err)
	}
	if err := writeMessageMetadataTx(ctx, tx, chatID, messageID, meta); err != nil {
		return "", "", nil, err
	}
	return groupID, content, meta, nil
}

func appendSelectedMessageVariantTx(ctx context.Context, tx *sql.Tx, chatID, messageID, groupID, content string) (navi.MessageVariant, error) {
	var nextIndex int
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(variant_index), -1) + 1 FROM navi_chat_message_variants WHERE variant_group_id = ?
	`, groupID).Scan(&nextIndex); err != nil {
		return navi.MessageVariant{}, fmt.Errorf("navi: next message variant index: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE navi_chat_message_variants SET selected = 0 WHERE variant_group_id = ?
	`, groupID); err != nil {
		return navi.MessageVariant{}, fmt.Errorf("navi: clear selected message variants: %w", err)
	}
	variantID := uuid.NewString()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO navi_chat_message_variants (
			variant_id, chat_id, message_id, variant_group_id, variant_index, content, selected, created_at
		) VALUES (?, ?, ?, ?, ?, ?, 1, ?)
	`, variantID, chatID, messageID, groupID, nextIndex, content, time.Now().UTC()); err != nil {
		return navi.MessageVariant{}, fmt.Errorf("navi: insert message variant: %w", err)
	}
	if err := updateMessageSelectedVariantTx(ctx, tx, chatID, messageID, content, groupID, nextIndex); err != nil {
		return navi.MessageVariant{}, err
	}
	return navi.MessageVariant{ID: variantID, Index: nextIndex, Content: content}, nil
}

func updateMessageSelectedVariantTx(ctx context.Context, tx *sql.Tx, chatID, messageID, content, groupID string, selectedIndex int) error {
	var metaRaw string
	if err := tx.QueryRowContext(ctx, `
		SELECT metadata_json FROM navi_chat_messages WHERE chat_id = ? AND message_id = ?
	`, chatID, messageID).Scan(&metaRaw); err != nil {
		return fmt.Errorf("navi: read selected variant metadata: %w", err)
	}
	meta := map[string]any{}
	if err := chatDecodeJSON(metaRaw, &meta); err != nil || meta == nil {
		meta = map[string]any{}
	}
	var count int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM navi_chat_message_variants WHERE variant_group_id = ?
	`, groupID).Scan(&count); err != nil {
		return fmt.Errorf("navi: count message variants: %w", err)
	}
	meta["variantGroupId"] = groupID
	meta["selectedVariantIndex"] = selectedIndex
	meta["variantCount"] = count
	if _, err := tx.ExecContext(ctx, `
		UPDATE navi_chat_messages SET content = ?, metadata_json = ? WHERE chat_id = ? AND message_id = ?
	`, content, chatEncodeJSON(meta), chatID, messageID); err != nil {
		return fmt.Errorf("navi: update selected message variant: %w", err)
	}
	return nil
}

func writeMessageMetadataTx(ctx context.Context, tx *sql.Tx, chatID, messageID string, meta map[string]any) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE navi_chat_messages SET metadata_json = ? WHERE chat_id = ? AND message_id = ?
	`, chatEncodeJSON(meta), chatID, messageID); err != nil {
		return fmt.Errorf("navi: update message metadata: %w", err)
	}
	return nil
}

func stringMeta(meta map[string]any, key string) string {
	if len(meta) == 0 {
		return ""
	}
	if value, ok := meta[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}
