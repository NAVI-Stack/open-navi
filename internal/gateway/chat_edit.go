package gateway

import (
	"encoding/json"
	"net/http"
	"strings"
)

// handleNaviEditResend edits a prior user message and re-runs the turn. Body:
// {"content": "..."}. The thread is truncated from that message onward, then the
// edited content is resubmitted through the normal runtime path.
func (s *Server) handleNaviEditResend(w http.ResponseWriter, r *http.Request) {
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
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "request body must be JSON with a content field", nil)
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "content is required", nil)
		return
	}

	item, err := s.cfg.Navi.EditAndResendMessage(r.Context(), chatID, messageID, req.Content)
	if err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "not found"):
			replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "chat message not found", nil)
		case strings.Contains(msg, "only user messages"):
			replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "only user messages can be edited and resent", nil)
		case strings.Contains(msg, "does not support"):
			replyErrorAPI(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "edit-and-resend not supported", nil)
		default:
			replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", msg, nil)
		}
		return
	}

	resp := map[string]any{"chatId": chatID, "status": "resent"}
	if item != nil {
		resp["inboxItemId"] = item.ID
	}
	replyJSON(w, http.StatusOK, resp)
}

// handleNaviRegenerate re-runs the most recent user turn for a fresh reply.
func (s *Server) handleNaviRegenerate(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Navi == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "NAVI not enabled", nil)
		return
	}
	chatID := strings.TrimSpace(r.PathValue("id"))
	if chatID == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "chat id is required", nil)
		return
	}

	item, err := s.cfg.Navi.RegenerateLastReply(r.Context(), chatID)
	if err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "no user message"):
			replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "nothing to regenerate", nil)
		case strings.Contains(msg, "no assistant message"):
			replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "nothing to regenerate", nil)
		case strings.Contains(msg, "does not support"):
			replyErrorAPI(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "regenerate not supported", nil)
		default:
			replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", msg, nil)
		}
		return
	}

	resp := map[string]any{"chatId": chatID, "status": "regenerating"}
	if item != nil {
		resp["inboxItemId"] = item.ID
	}
	replyJSON(w, http.StatusOK, resp)
}

// handleNaviContinue resumes the most recent assistant reply without truncating
// transcript history.
func (s *Server) handleNaviContinue(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Navi == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "NAVI not enabled", nil)
		return
	}
	chatID := strings.TrimSpace(r.PathValue("id"))
	if chatID == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "chat id is required", nil)
		return
	}

	item, err := s.cfg.Navi.ContinueLastReply(r.Context(), chatID)
	if err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "no assistant message"):
			replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "nothing to continue", nil)
		default:
			replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", msg, nil)
		}
		return
	}

	resp := map[string]any{"chatId": chatID, "status": "continuing"}
	if item != nil {
		resp["inboxItemId"] = item.ID
	}
	replyJSON(w, http.StatusOK, resp)
}

func (s *Server) handleNaviMessageVariants(w http.ResponseWriter, r *http.Request) {
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

	variants, err := s.cfg.Navi.ListMessageVariants(r.Context(), chatID, messageID)
	if err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "not found"):
			replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "chat message not found", nil)
		case strings.Contains(msg, "does not support"):
			replyErrorAPI(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "variants not supported", nil)
		default:
			replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", msg, nil)
		}
		return
	}
	replyJSON(w, http.StatusOK, variants)
}

func (s *Server) handleNaviSelectMessageVariant(w http.ResponseWriter, r *http.Request) {
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
		Index int `json:"index"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "request body must be JSON with an index field", nil)
		return
	}
	if req.Index < 0 {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "index must be non-negative", nil)
		return
	}
	if err := s.cfg.Navi.SelectMessageVariant(r.Context(), chatID, messageID, req.Index); err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "not found"):
			replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "message variant not found", nil)
		case strings.Contains(msg, "does not support"):
			replyErrorAPI(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "variants not supported", nil)
		default:
			replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", msg, nil)
		}
		return
	}
	replyJSON(w, http.StatusOK, map[string]any{"chatId": chatID, "messageId": messageID, "selectedIndex": req.Index})
}
