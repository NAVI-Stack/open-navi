package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/open-navi/navi/internal/ai"
	"github.com/open-navi/navi/internal/llm"
)

type chatProviderService interface {
	ChatProvider() llm.Provider
}

func (s *Server) handleGenerateConversationTitle(w http.ResponseWriter, r *http.Request) {
	if !HasScope(r.Context(), "execute") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "execute scope required", nil)
		return
	}

	var req ai.TitleGeneratorInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyError(w, http.StatusBadRequest, "failed to decode request body: "+err.Error())
		return
	}

	if s.cfg.LLM == nil {
		replyError(w, http.StatusServiceUnavailable, "LLM service not configured")
		return
	}

	active, err := s.cfg.LLM.GetActive(r.Context())
	if err != nil {
		replyError(w, http.StatusInternalServerError, "failed to get active LLM: "+err.Error())
		return
	}

	providerService, ok := s.cfg.LLM.(chatProviderService)
	if !ok {
		replyError(w, http.StatusServiceUnavailable, "LLM chat provider not configured")
		return
	}
	llmProv := providerService.ChatProvider()

	resp, err := ai.GenerateConversationTitle(r.Context(), llmProv, active.Model, req)
	if err != nil {
		replyError(w, http.StatusInternalServerError, "failed to generate title: "+err.Error())
		return
	}

	replyJSON(w, http.StatusOK, resp)
}
