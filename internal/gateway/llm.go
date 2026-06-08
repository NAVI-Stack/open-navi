package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/open-navi/navi/internal/llm"
)

func (s *Server) handleLLMCatalog(w http.ResponseWriter, r *http.Request) {
	if !HasScope(r.Context(), "read") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "read scope required", nil)
		return
	}
	if s.cfg.LLM == nil {
		replyError(w, http.StatusServiceUnavailable, "LLM catalog not configured")
		return
	}
	catalog, err := s.cfg.LLM.Catalog(r.Context())
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, catalog)
}

func (s *Server) handleLLMProfiles(w http.ResponseWriter, r *http.Request) {
	if !HasScope(r.Context(), "read") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "read scope required", nil)
		return
	}
	if s.cfg.LLM == nil {
		replyError(w, http.StatusServiceUnavailable, "LLM profiles not configured")
		return
	}
	profiles, err := s.cfg.LLM.Profiles(r.Context())
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Surface the CIP §9 privacy-tier policy (P4) so operators can inspect which
	// privacy classes may route to a cloud model without reading code. Best-effort:
	// if preferences are unavailable, fall back to the permissive default view.
	mode := llm.PrivacyModeCloud
	if prefs, perr := s.cfg.LLM.GetPreferences(r.Context()); perr == nil && prefs.PrivacyMode != "" {
		mode = prefs.PrivacyMode
	}
	replyJSON(w, http.StatusOK, map[string]any{
		"profiles": profiles,
		"privacy":  llm.BuildPrivacyPolicyView(mode),
	})
}

func (s *Server) handleLLMActive(w http.ResponseWriter, r *http.Request) {
	if !HasScope(r.Context(), "read") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "read scope required", nil)
		return
	}
	if s.cfg.LLM == nil {
		replyError(w, http.StatusServiceUnavailable, "LLM selection not configured")
		return
	}
	active, err := s.cfg.LLM.GetActive(r.Context())
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, map[string]string{
		"provider": active.Provider,
		"model":    active.Model,
		"status":   active.Provider + "/" + active.Model,
	})
}

func (s *Server) handleSetLLMActive(w http.ResponseWriter, r *http.Request) {
	if !HasScope(r.Context(), "execute") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "execute scope required", nil)
		return
	}
	if s.cfg.LLM == nil {
		replyError(w, http.StatusServiceUnavailable, "LLM selection not configured")
		return
	}
	var req struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	active, err := s.cfg.LLM.SetActive(r.Context(), req.Provider, req.Model)
	if err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, map[string]string{
		"provider": active.Provider,
		"model":    active.Model,
		"status":   active.Provider + "/" + active.Model,
	})
}

func (s *Server) handleLLMPreferences(w http.ResponseWriter, r *http.Request) {
	if !HasScope(r.Context(), "read") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "read scope required", nil)
		return
	}
	if s.cfg.LLM == nil {
		replyError(w, http.StatusServiceUnavailable, "LLM preferences not configured")
		return
	}
	prefs, err := s.cfg.LLM.GetPreferences(r.Context())
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, prefs)
}

func (s *Server) handlePatchLLMPreferences(w http.ResponseWriter, r *http.Request) {
	if !HasScope(r.Context(), "execute") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "execute scope required", nil)
		return
	}
	if s.cfg.LLM == nil {
		replyError(w, http.StatusServiceUnavailable, "LLM preferences not configured")
		return
	}
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	prefs, err := s.cfg.LLM.PatchPreferences(r.Context(), req)
	if err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, prefs)
}
