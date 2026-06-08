package gateway

import (
	"encoding/json"
	"net/http"
	"strings"
)

// handleNaviMessageFeedback persists owner feedback (👍/👎) on an assistant
// message. Body: {"rating": "up" | "down" | null}. Rating null/empty clears it.
func (s *Server) handleNaviMessageFeedback(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Navi == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "NAVI not enabled", nil)
		return
	}
	chatID := strings.TrimSpace(r.PathValue("id"))
	messageID := strings.TrimSpace(r.PathValue("messageId"))
	if chatID == "" || messageID == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "chat id and message id are required", nil)
		return
	}

	var req struct {
		Rating *string `json:"rating"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "request body must be JSON with a rating field", nil)
			return
		}
	}
	rating := ""
	if req.Rating != nil {
		rating = strings.TrimSpace(strings.ToLower(*req.Rating))
	}
	switch rating {
	case "", "up", "down":
	default:
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "rating must be 'up', 'down', or null", nil)
		return
	}

	if err := s.cfg.Navi.SetMessageFeedback(r.Context(), chatID, messageID, rating); err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "not found"):
			replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "chat message not found", nil)
		case strings.Contains(msg, "does not support"):
			replyErrorAPI(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "feedback not supported", nil)
		default:
			replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", msg, nil)
		}
		return
	}

	replyJSON(w, http.StatusOK, map[string]any{
		"chatId":    chatID,
		"messageId": messageID,
		"rating":    rating,
	})
}
