package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/open-navi/navi/internal/navi"
)

func (s *SQLiteStore) EnsureConsoleEndpoint(ctx context.Context, chatID string) (*navi.ConversationEndpoint, error) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return nil, fmt.Errorf("navi: ensure console endpoint: chat id is required")
	}
	existing, err := s.queryConversationEndpoint(ctx, `
		SELECT endpoint_id, chat_id, endpoint_type, connector_kind, connector_instance_id,
		       external_chat_id, external_thread_id, display_name,
		       receive_enabled, send_enabled, mirror_enabled, status,
		       created_at, updated_at, metadata_json
		FROM navi_conversation_endpoints
		WHERE chat_id = ? AND endpoint_type = 'console' AND status = 'active'
		LIMIT 1
	`, chatID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, navi.ErrConversationEndpointNotFound) {
		return nil, err
	}

	now := time.Now().UTC()
	endpointID := uuid.NewString()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO navi_conversation_endpoints (
			endpoint_id, chat_id, endpoint_type, connector_kind, connector_instance_id,
			external_chat_id, external_thread_id, display_name,
			receive_enabled, send_enabled, mirror_enabled, status,
			created_at, updated_at, metadata_json
		) VALUES (?, ?, 'console', 'console', 'console', ?, '', 'Console', 1, 1, 0, 'active', ?, ?, '{}')
	`, endpointID, chatID, chatID, now, now); err != nil {
		return nil, fmt.Errorf("navi: ensure console endpoint: %w", err)
	}
	return s.GetConversationEndpoint(ctx, endpointID)
}

func (s *SQLiteStore) UpsertConnectorEndpoint(ctx context.Context, input navi.UpsertConnectorEndpointInput) (*navi.ConversationEndpoint, error) {
	chatID := strings.TrimSpace(input.ChatID)
	connectorKind := strings.TrimSpace(input.ConnectorKind)
	connectorInstanceID := strings.TrimSpace(input.ConnectorInstanceID)
	externalChatID := strings.TrimSpace(input.ExternalChatID)
	externalThreadID := strings.TrimSpace(input.ExternalThreadID)
	if chatID == "" || connectorKind == "" || connectorInstanceID == "" || externalChatID == "" {
		return nil, fmt.Errorf("navi: upsert connector endpoint: chat id, connector kind, connector instance id, and external chat id are required")
	}

	existing, err := s.ResolveConnectorEndpoint(ctx, connectorInstanceID, externalChatID, externalThreadID)
	if err != nil && !errors.Is(err, navi.ErrConversationEndpointNotFound) {
		return nil, err
	}
	if existing != nil {
		displayName := strings.TrimSpace(input.DisplayName)
		if displayName == "" {
			displayName = existing.DisplayName
		}
		receiveEnabled := existing.ReceiveEnabled
		if input.ReceiveEnabled != nil {
			receiveEnabled = *input.ReceiveEnabled
		}
		sendEnabled := existing.SendEnabled
		if input.SendEnabled != nil {
			sendEnabled = *input.SendEnabled
		}
		mirrorEnabled := existing.MirrorEnabled
		if input.MirrorEnabled != nil {
			mirrorEnabled = *input.MirrorEnabled
		}
		status := existing.Status
		if input.Status != "" {
			status = input.Status
		}
		metadata := existing.Metadata
		if input.Metadata != nil {
			metadata = input.Metadata
		}
		now := time.Now().UTC()
		if _, err := s.db.ExecContext(ctx, `
			UPDATE navi_conversation_endpoints
			SET chat_id = ?, connector_kind = ?, display_name = ?,
			    receive_enabled = ?, send_enabled = ?, mirror_enabled = ?, status = ?,
			    updated_at = ?, metadata_json = ?
			WHERE endpoint_id = ?
		`, chatID, connectorKind, displayName, chatBoolToInt(receiveEnabled), chatBoolToInt(sendEnabled), chatBoolToInt(mirrorEnabled), string(status), now, chatEncodeJSON(metadata), string(existing.ID)); err != nil {
			return nil, fmt.Errorf("navi: update connector endpoint: %w", err)
		}
		return s.GetConversationEndpoint(ctx, string(existing.ID))
	}

	status := input.Status
	if status == "" {
		status = navi.EndpointStatusActive
	}
	receiveEnabled := true
	if input.ReceiveEnabled != nil {
		receiveEnabled = *input.ReceiveEnabled
	}
	sendEnabled := true
	if input.SendEnabled != nil {
		sendEnabled = *input.SendEnabled
	}
	mirrorEnabled := false
	if input.MirrorEnabled != nil {
		mirrorEnabled = *input.MirrorEnabled
	}
	endpointID := strings.TrimSpace(input.ID)
	if endpointID == "" {
		endpointID = uuid.NewString()
	}
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO navi_conversation_endpoints (
			endpoint_id, chat_id, endpoint_type, connector_kind, connector_instance_id,
			external_chat_id, external_thread_id, display_name,
			receive_enabled, send_enabled, mirror_enabled, status,
			created_at, updated_at, metadata_json
		) VALUES (?, ?, 'connector', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, endpointID, chatID, connectorKind, connectorInstanceID, externalChatID, externalThreadID, strings.TrimSpace(input.DisplayName),
		chatBoolToInt(receiveEnabled), chatBoolToInt(sendEnabled), chatBoolToInt(mirrorEnabled), string(status), now, now, chatEncodeJSON(input.Metadata)); err != nil {
		return nil, fmt.Errorf("navi: insert connector endpoint: %w", err)
	}
	return s.GetConversationEndpoint(ctx, endpointID)
}

func (s *SQLiteStore) ListConversationEndpoints(ctx context.Context, chatID string) ([]navi.ConversationEndpoint, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT endpoint_id, chat_id, endpoint_type, connector_kind, connector_instance_id,
		       external_chat_id, external_thread_id, display_name,
		       receive_enabled, send_enabled, mirror_enabled, status,
		       created_at, updated_at, metadata_json
		FROM navi_conversation_endpoints
		WHERE chat_id = ? AND status != 'deleted'
		ORDER BY CASE endpoint_type WHEN 'console' THEN 0 ELSE 1 END, created_at ASC
	`, strings.TrimSpace(chatID))
	if err != nil {
		return nil, fmt.Errorf("navi: list conversation endpoints: %w", err)
	}
	defer rows.Close()
	var out []navi.ConversationEndpoint
	for rows.Next() {
		endpoint, err := scanConversationEndpoint(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *endpoint)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("navi: list conversation endpoints rows: %w", err)
	}
	return out, nil
}

func (s *SQLiteStore) GetConversationEndpoint(ctx context.Context, endpointID string) (*navi.ConversationEndpoint, error) {
	return s.queryConversationEndpoint(ctx, `
		SELECT endpoint_id, chat_id, endpoint_type, connector_kind, connector_instance_id,
		       external_chat_id, external_thread_id, display_name,
		       receive_enabled, send_enabled, mirror_enabled, status,
		       created_at, updated_at, metadata_json
		FROM navi_conversation_endpoints
		WHERE endpoint_id = ?
	`, strings.TrimSpace(endpointID))
}

func (s *SQLiteStore) ResolveConnectorEndpoint(ctx context.Context, connectorInstanceID, externalChatID, externalThreadID string) (*navi.ConversationEndpoint, error) {
	return s.queryConversationEndpoint(ctx, `
		SELECT endpoint_id, chat_id, endpoint_type, connector_kind, connector_instance_id,
		       external_chat_id, external_thread_id, display_name,
		       receive_enabled, send_enabled, mirror_enabled, status,
		       created_at, updated_at, metadata_json
		FROM navi_conversation_endpoints
		WHERE endpoint_type = 'connector'
		  AND connector_instance_id = ?
		  AND external_chat_id = ?
		  AND external_thread_id = ?
		  AND status != 'deleted'
		ORDER BY CASE status WHEN 'active' THEN 0 WHEN 'disabled' THEN 1 ELSE 2 END, updated_at DESC
		LIMIT 1
	`, strings.TrimSpace(connectorInstanceID), strings.TrimSpace(externalChatID), strings.TrimSpace(externalThreadID))
}

func (s *SQLiteStore) UpdateConversationEndpoint(ctx context.Context, endpointID string, input navi.UpdateConversationEndpointInput) (*navi.ConversationEndpoint, error) {
	endpoint, err := s.GetConversationEndpoint(ctx, endpointID)
	if err != nil {
		return nil, err
	}
	displayName := endpoint.DisplayName
	if input.DisplayName != nil {
		displayName = strings.TrimSpace(*input.DisplayName)
	}
	receiveEnabled := endpoint.ReceiveEnabled
	if input.ReceiveEnabled != nil {
		receiveEnabled = *input.ReceiveEnabled
	}
	sendEnabled := endpoint.SendEnabled
	if input.SendEnabled != nil {
		sendEnabled = *input.SendEnabled
	}
	mirrorEnabled := endpoint.MirrorEnabled
	if input.MirrorEnabled != nil {
		mirrorEnabled = *input.MirrorEnabled
	}
	status := endpoint.Status
	if input.Status != nil {
		status = *input.Status
	}
	metadata := endpoint.Metadata
	if input.Metadata != nil {
		metadata = input.Metadata
	}
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		UPDATE navi_conversation_endpoints
		SET display_name = ?, receive_enabled = ?, send_enabled = ?, mirror_enabled = ?,
		    status = ?, updated_at = ?, metadata_json = ?
		WHERE endpoint_id = ?
	`, displayName, chatBoolToInt(receiveEnabled), chatBoolToInt(sendEnabled), chatBoolToInt(mirrorEnabled), string(status), now, chatEncodeJSON(metadata), strings.TrimSpace(endpointID)); err != nil {
		return nil, fmt.Errorf("navi: update conversation endpoint: %w", err)
	}
	return s.GetConversationEndpoint(ctx, endpointID)
}

func (s *SQLiteStore) GetChatDeliveryPolicy(ctx context.Context, chatID string) (*navi.ChatDeliveryPolicy, error) {
	policy, err := s.queryChatDeliveryPolicy(ctx, strings.TrimSpace(chatID))
	if errors.Is(err, navi.ErrConversationEndpointNotFound) {
		return &navi.ChatDeliveryPolicy{
			ChatID: navi.ID(strings.TrimSpace(chatID)),
			Mode:   navi.DeliveryPolicyReplyToOrigin,
		}, nil
	}
	return policy, err
}

func (s *SQLiteStore) SetChatDeliveryPolicy(ctx context.Context, chatID string, mode navi.DeliveryPolicyMode, metadata map[string]any) (*navi.ChatDeliveryPolicy, error) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return nil, fmt.Errorf("navi: set chat delivery policy: chat id is required")
	}
	if mode == "" {
		mode = navi.DeliveryPolicyReplyToOrigin
	}
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO navi_chat_delivery_policy (
			chat_id, default_mode, created_at, updated_at, metadata_json
		) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(chat_id) DO UPDATE SET
			default_mode = excluded.default_mode,
			updated_at = excluded.updated_at,
			metadata_json = excluded.metadata_json
	`, chatID, string(mode), now, now, chatEncodeJSON(metadata)); err != nil {
		return nil, fmt.Errorf("navi: set chat delivery policy: %w", err)
	}
	return s.queryChatDeliveryPolicy(ctx, chatID)
}

func (s *SQLiteStore) RecordMessageDelivery(ctx context.Context, input navi.RecordMessageDeliveryInput) (*navi.MessageDelivery, error) {
	deliveryID := strings.TrimSpace(input.ID)
	if deliveryID == "" {
		deliveryID = uuid.NewString()
	}
	status := input.Status
	if status == "" {
		status = navi.MessageDeliveryQueued
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO navi_message_deliveries (
			delivery_id, message_id, chat_id, endpoint_id, connector_instance_id,
			status, attempt_count, last_error, created_at, updated_at, metadata_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, deliveryID, strings.TrimSpace(input.MessageID), strings.TrimSpace(input.ChatID), strings.TrimSpace(input.EndpointID), strings.TrimSpace(input.ConnectorInstanceID),
		string(status), input.AttemptCount, strings.TrimSpace(input.LastError), now, now, chatEncodeJSON(input.Metadata))
	if err != nil {
		return nil, fmt.Errorf("navi: record message delivery: %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return s.queryMessageDeliveryByTarget(ctx, input.MessageID, input.EndpointID)
	}
	return s.GetMessageDelivery(ctx, deliveryID)
}

func (s *SQLiteStore) UpdateMessageDelivery(ctx context.Context, deliveryID string, input navi.UpdateMessageDeliveryInput) (*navi.MessageDelivery, error) {
	delivery, err := s.GetMessageDelivery(ctx, deliveryID)
	if err != nil {
		return nil, err
	}
	status := input.Status
	if status == "" {
		status = delivery.Status
	}
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		UPDATE navi_message_deliveries
		SET status = ?,
		    attempt_count = attempt_count + ?,
		    last_error = ?,
		    updated_at = ?
		WHERE delivery_id = ?
	`, string(status), input.AttemptDelta, strings.TrimSpace(input.LastError), now, strings.TrimSpace(deliveryID)); err != nil {
		return nil, fmt.Errorf("navi: update message delivery: %w", err)
	}
	return s.GetMessageDelivery(ctx, deliveryID)
}

func (s *SQLiteStore) GetMessageDelivery(ctx context.Context, deliveryID string) (*navi.MessageDelivery, error) {
	return s.queryMessageDelivery(ctx, `
		SELECT delivery_id, message_id, chat_id, endpoint_id, connector_instance_id,
		       status, attempt_count, last_error, created_at, updated_at, metadata_json
		FROM navi_message_deliveries
		WHERE delivery_id = ?
	`, strings.TrimSpace(deliveryID))
}

func (s *SQLiteStore) queryConversationEndpoint(ctx context.Context, query string, args ...any) (*navi.ConversationEndpoint, error) {
	endpoint, err := scanConversationEndpoint(s.db.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, navi.ErrConversationEndpointNotFound
	}
	return endpoint, err
}

func scanConversationEndpoint(scanner interface{ Scan(dest ...any) error }) (*navi.ConversationEndpoint, error) {
	var endpoint navi.ConversationEndpoint
	var endpointType, status string
	var metadataJSON string
	var receiveEnabled, sendEnabled, mirrorEnabled int
	if err := scanner.Scan(
		&endpoint.ID,
		&endpoint.ChatID,
		&endpointType,
		&endpoint.ConnectorKind,
		&endpoint.ConnectorInstanceID,
		&endpoint.ExternalChatID,
		&endpoint.ExternalThreadID,
		&endpoint.DisplayName,
		&receiveEnabled,
		&sendEnabled,
		&mirrorEnabled,
		&status,
		&endpoint.CreatedAt,
		&endpoint.UpdatedAt,
		&metadataJSON,
	); err != nil {
		return nil, err
	}
	endpoint.Type = navi.ConversationEndpointType(endpointType)
	endpoint.ReceiveEnabled = receiveEnabled == 1
	endpoint.SendEnabled = sendEnabled == 1
	endpoint.MirrorEnabled = mirrorEnabled == 1
	endpoint.Status = navi.ConversationEndpointStatus(status)
	_ = chatDecodeJSON(metadataJSON, &endpoint.Metadata)
	if endpoint.Metadata == nil {
		endpoint.Metadata = map[string]any{}
	}
	return &endpoint, nil
}

func (s *SQLiteStore) queryChatDeliveryPolicy(ctx context.Context, chatID string) (*navi.ChatDeliveryPolicy, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT chat_id, default_mode, created_at, updated_at, metadata_json
		FROM navi_chat_delivery_policy
		WHERE chat_id = ?
	`, chatID)
	var policy navi.ChatDeliveryPolicy
	var mode string
	var metadataJSON string
	if err := row.Scan(&policy.ChatID, &mode, &policy.CreatedAt, &policy.UpdatedAt, &metadataJSON); errors.Is(err, sql.ErrNoRows) {
		return nil, navi.ErrConversationEndpointNotFound
	} else if err != nil {
		return nil, fmt.Errorf("navi: get chat delivery policy: %w", err)
	}
	policy.Mode = navi.DeliveryPolicyMode(mode)
	_ = chatDecodeJSON(metadataJSON, &policy.Metadata)
	if policy.Metadata == nil {
		policy.Metadata = map[string]any{}
	}
	return &policy, nil
}

func (s *SQLiteStore) queryMessageDeliveryByTarget(ctx context.Context, messageID, endpointID string) (*navi.MessageDelivery, error) {
	return s.queryMessageDelivery(ctx, `
		SELECT delivery_id, message_id, chat_id, endpoint_id, connector_instance_id,
		       status, attempt_count, last_error, created_at, updated_at, metadata_json
		FROM navi_message_deliveries
		WHERE message_id = ? AND endpoint_id = ?
	`, strings.TrimSpace(messageID), strings.TrimSpace(endpointID))
}

func (s *SQLiteStore) queryMessageDelivery(ctx context.Context, query string, args ...any) (*navi.MessageDelivery, error) {
	var delivery navi.MessageDelivery
	var status string
	var metadataJSON string
	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&delivery.ID,
		&delivery.MessageID,
		&delivery.ChatID,
		&delivery.EndpointID,
		&delivery.ConnectorInstanceID,
		&status,
		&delivery.AttemptCount,
		&delivery.LastError,
		&delivery.CreatedAt,
		&delivery.UpdatedAt,
		&metadataJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, navi.ErrConversationEndpointNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("navi: get message delivery: %w", err)
	}
	delivery.Status = navi.MessageDeliveryStatus(status)
	_ = chatDecodeJSON(metadataJSON, &delivery.Metadata)
	if delivery.Metadata == nil {
		delivery.Metadata = map[string]any{}
	}
	return &delivery, nil
}
