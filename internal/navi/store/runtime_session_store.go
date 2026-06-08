package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/open-navi/navi/internal/navi"
)

func (s *SQLiteStore) CreateRuntimeSession(ctx context.Context, input navi.CreateRuntimeSessionInput) (*navi.RuntimeSession, error) {
	runtimeSessionID := strings.TrimSpace(input.ID)
	if runtimeSessionID == "" {
		runtimeSessionID = uuid.NewString()
	}

	now := time.Now().UTC()

	kind := input.Kind
	if kind == "" {
		kind = navi.RuntimeSessionKindUser
	}

	status := input.Status
	if status == "" {
		status = navi.RuntimeSessionStatusActive
	}

	experienceMode := strings.TrimSpace(input.ExperienceMode)
	if experienceMode == "" {
		experienceMode = "navi"
	}

	runtimeSession := navi.RuntimeSession{
		ID:             navi.ID(runtimeSessionID),
		OwnerID:        navi.ID(strings.TrimSpace(input.OwnerID)),
		WorkspaceID:    chatStringToIDPtr(input.WorkspaceID),
		ProjectID:      chatStringToIDPtr(input.ProjectID),
		Kind:           kind,
		Status:         status,
		ExperienceMode: experienceMode,
		SourceChannel:  input.SourceChannel,
		StartedAt:      now,
		LastActiveAt:   now,
		Metadata:       input.Metadata,
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO runtime_sessions (
			runtime_session_id, owner_id, workspace_id, project_id,
			kind, status, experience_mode, source_channel,
			started_at, last_active_at, ended_at, metadata_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		string(runtimeSession.ID),
		string(runtimeSession.OwnerID),
		chatIDPtrToNullableString(runtimeSession.WorkspaceID),
		chatIDPtrToNullableString(runtimeSession.ProjectID),
		string(runtimeSession.Kind),
		string(runtimeSession.Status),
		runtimeSession.ExperienceMode,
		runtimeSession.SourceChannel,
		runtimeSession.StartedAt,
		runtimeSession.LastActiveAt,
		chatTimePtrToNullable(runtimeSession.EndedAt),
		chatEncodeJSON(runtimeSession.Metadata),
	)
	if err != nil {
		return nil, fmt.Errorf("navi: create runtime session: %w", err)
	}

	return &runtimeSession, nil
}

func (s *SQLiteStore) GetRuntimeSession(ctx context.Context, runtimeSessionID string) (*navi.RuntimeSession, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT runtime_session_id, owner_id, workspace_id, project_id,
		       kind, status, experience_mode, source_channel,
		       started_at, last_active_at, ended_at, metadata_json
		FROM runtime_sessions
		WHERE runtime_session_id = ?
	`, runtimeSessionID)

	runtimeSession, err := scanRuntimeSession(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("navi: get runtime session: not found")
	}
	if err != nil {
		return nil, err
	}

	return runtimeSession, nil
}

func (s *SQLiteStore) TouchRuntimeSession(ctx context.Context, runtimeSessionID string) error {
	now := time.Now().UTC()

	_, err := s.db.ExecContext(ctx, `
		UPDATE runtime_sessions
		SET last_active_at = ?,
		    status = CASE
		        WHEN status IN ('closed', 'failed') THEN status
		        ELSE 'active'
		    END
		WHERE runtime_session_id = ?
	`, now, runtimeSessionID)
	if err != nil {
		return fmt.Errorf("navi: touch runtime session: %w", err)
	}

	return nil
}

func (s *SQLiteStore) CloseRuntimeSession(ctx context.Context, runtimeSessionID string) error {
	now := time.Now().UTC()

	_, err := s.db.ExecContext(ctx, `
		UPDATE runtime_sessions
		SET status = 'closed',
		    ended_at = COALESCE(ended_at, ?),
		    last_active_at = ?
		WHERE runtime_session_id = ?
	`, now, now, runtimeSessionID)
	if err != nil {
		return fmt.Errorf("navi: close runtime session: %w", err)
	}

	return nil
}

func (s *SQLiteStore) AttachChatToRuntimeSession(ctx context.Context, runtimeSessionID, chatID string, relationship navi.RuntimeSessionChatRelationship) error {
	runtimeSessionID = strings.TrimSpace(runtimeSessionID)
	chatID = strings.TrimSpace(chatID)

	if runtimeSessionID == "" {
		return fmt.Errorf("navi: attach chat to runtime session: runtime session id is required")
	}
	if chatID == "" {
		return fmt.Errorf("navi: attach chat to runtime session: chat id is required")
	}
	if relationship == "" {
		relationship = navi.RuntimeSessionChatPrimary
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO runtime_session_chats (
			runtime_session_id, chat_id, relationship, attached_at, metadata_json
		) VALUES (?, ?, ?, ?, ?)
	`, runtimeSessionID, chatID, string(relationship), time.Now().UTC(), "{}")
	if err != nil {
		return fmt.Errorf("navi: attach chat to runtime session: %w", err)
	}

	return nil
}

func (s *SQLiteStore) ListRuntimeSessionChats(ctx context.Context, runtimeSessionID string) ([]navi.RuntimeSessionChat, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT runtime_session_id, chat_id, relationship, attached_at, metadata_json
		FROM runtime_session_chats
		WHERE runtime_session_id = ?
		ORDER BY attached_at ASC
	`, runtimeSessionID)
	if err != nil {
		return nil, fmt.Errorf("navi: list runtime session chats: %w", err)
	}
	defer rows.Close()

	var links []navi.RuntimeSessionChat
	for rows.Next() {
		link, err := scanRuntimeSessionChat(rows)
		if err != nil {
			return nil, err
		}
		links = append(links, *link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("navi: list runtime session chats rows: %w", err)
	}

	return links, nil
}

func (s *SQLiteStore) FindActiveRuntimeSessionForChat(ctx context.Context, chatID string, sourceChannel string) (*navi.RuntimeSession, error) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return nil, nil
	}
	sourceChannel = strings.TrimSpace(sourceChannel)
	query := `
		SELECT rs.runtime_session_id, rs.owner_id, rs.workspace_id, rs.project_id,
		       rs.kind, rs.status, rs.experience_mode, rs.source_channel,
		       rs.started_at, rs.last_active_at, rs.ended_at, rs.metadata_json
		FROM runtime_sessions rs
		JOIN runtime_session_chats rsc ON rsc.runtime_session_id = rs.runtime_session_id
		WHERE rsc.chat_id = ?
		  AND rsc.relationship = ?
		  AND rs.status IN ('active', 'idle')
	`
	args := []any{chatID, string(navi.RuntimeSessionChatPrimary)}
	if sourceChannel != "" {
		query += ` AND (rs.source_channel = ? OR rs.source_channel = '')`
		args = append(args, sourceChannel)
	}
	query += ` ORDER BY rs.last_active_at DESC LIMIT 1`

	runtimeSession, err := scanRuntimeSession(s.db.QueryRowContext(ctx, query, args...))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return runtimeSession, nil
}

func scanRuntimeSession(scanner interface{ Scan(dest ...any) error }) (*navi.RuntimeSession, error) {
	var runtimeSession navi.RuntimeSession
	var workspaceID sql.NullString
	var projectID sql.NullString
	var kind string
	var status string
	var endedAt sql.NullTime
	var metadataJSON string

	err := scanner.Scan(
		&runtimeSession.ID,
		&runtimeSession.OwnerID,
		&workspaceID,
		&projectID,
		&kind,
		&status,
		&runtimeSession.ExperienceMode,
		&runtimeSession.SourceChannel,
		&runtimeSession.StartedAt,
		&runtimeSession.LastActiveAt,
		&endedAt,
		&metadataJSON,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("navi: scan runtime session: %w", err)
	}

	runtimeSession.WorkspaceID = chatNullableStringToIDPtr(workspaceID)
	runtimeSession.ProjectID = chatNullableStringToIDPtr(projectID)
	runtimeSession.Kind = navi.RuntimeSessionKind(kind)
	if runtimeSession.Kind == "" {
		runtimeSession.Kind = navi.RuntimeSessionKindUser
	}
	runtimeSession.Status = navi.RuntimeSessionStatus(status)
	if runtimeSession.Status == "" {
		runtimeSession.Status = navi.RuntimeSessionStatusActive
	}
	if endedAt.Valid {
		runtimeSession.EndedAt = &endedAt.Time
	}
	_ = chatDecodeJSON(metadataJSON, &runtimeSession.Metadata)

	return &runtimeSession, nil
}

func scanRuntimeSessionChat(scanner interface{ Scan(dest ...any) error }) (*navi.RuntimeSessionChat, error) {
	var link navi.RuntimeSessionChat
	var relationship string
	var metadataJSON string

	err := scanner.Scan(&link.RuntimeSessionID, &link.ChatID, &relationship, &link.AttachedAt, &metadataJSON)
	if err != nil {
		return nil, fmt.Errorf("navi: scan runtime session chat: %w", err)
	}

	link.Relationship = navi.RuntimeSessionChatRelationship(relationship)
	if link.Relationship == "" {
		link.Relationship = navi.RuntimeSessionChatPrimary
	}
	_ = chatDecodeJSON(metadataJSON, &link.Metadata)

	return &link, nil
}
