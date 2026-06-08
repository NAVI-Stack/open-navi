package gateway

import (
	"context"
	"database/sql"
	"strings"

	"github.com/open-navi/navi/internal/schema"
)

// chatHiddenFromPublicAPI reports whether chatID should be excluded from external
// API responses. A chat is hidden when:
//   - chatID is empty
//   - chatID matches schema.IsInternalRuntimeSessionID (defensive legacy-data guard;
//     new code must never create a Chat whose chat_id equals an internal runtime ID)
//   - the navi_chats row has status 'deleted'
//   - the chat is linked (via runtime_session_chats) to a runtime session with kind='internal'
//
// Chats that are absent from navi_chats are not hidden here; callers validate existence.
func chatHiddenFromPublicAPI(ctx context.Context, db *sql.DB, chatID string) (bool, error) {
	if strings.TrimSpace(chatID) == "" {
		return true, nil
	}
	if schema.IsInternalRuntimeSessionID(chatID) {
		return true, nil
	}
	if db == nil {
		return false, nil
	}
	var status sql.NullString
	var runtimeKind sql.NullString
	err := db.QueryRowContext(ctx, `
		SELECT nc.status, rs.kind
		FROM navi_chats nc
		LEFT JOIN runtime_session_chats rsc ON rsc.chat_id = nc.chat_id AND rsc.relationship = 'primary'
		LEFT JOIN runtime_sessions rs ON rs.runtime_session_id = rsc.runtime_session_id
		WHERE nc.chat_id = ?
	`, chatID).Scan(&status, &runtimeKind)
	switch err {
	case nil:
		return status.String == "deleted" || runtimeKind.String == "internal", nil
	case sql.ErrNoRows:
		return false, nil // not in navi_chats → let the handler validate existence
	default:
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return false, nil
		}
		return false, err
	}
}
