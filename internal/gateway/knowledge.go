package gateway

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/open-navi/navi/internal/store"
)

func (s *Server) handleKnowledge(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil {
		replyError(w, http.StatusNotImplemented, "knowledge search unavailable")
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		replyError(w, http.StatusBadRequest, "missing q")
		return
	}
	limit := 10
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 50 {
			limit = parsed
		}
	}
	chatID := strings.TrimSpace(r.URL.Query().Get("chat_id"))
	ownerID, _ := store.GetOwnerID(r.Context(), s.cfg.DB)
	results, err := s.cfg.WorldModel.RecallKnowledge(r.Context(), query, ownerID, chatID, limit)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, map[string]any{
		"query":   query,
		"results": results,
	})
}
