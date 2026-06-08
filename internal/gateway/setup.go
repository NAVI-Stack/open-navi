package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/open-navi/navi/internal/config"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

// handleHealth GET /health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	checks := map[string]string{}
	healthy := true

	if s.cfg.DB != nil {
		if err := s.cfg.DB.PingContext(r.Context()); err != nil {
			checks["database"] = "error"
			healthy = false
		} else {
			checks["database"] = "ok"
		}
	}

	if s.cfg.Bus != nil {
		if s.cfg.Bus.Healthy() {
			checks["bus"] = "ok"
		} else {
			checks["bus"] = "disconnected"
			healthy = false
		}
	}

	if s.cfg.Navi != nil {
		state := s.cfg.Navi.Status().State
		if state == schema.AgentStateOffline {
			checks["agent"] = "offline"
		} else {
			checks["agent"] = "ok"
		}
	}

	status := "ok"
	code := http.StatusOK
	if !healthy {
		status = "unhealthy"
		code = http.StatusServiceUnavailable
	}

	replyJSON(w, code, map[string]any{
		"status":  status,
		"version": s.cfg.Version,
		"checks":  checks,
	})
}

// handleGetSetup GET /api/setup
// Returns the current setup state so the CLI wizard can decide what to ask.
func (s *Server) handleGetSetup(w http.ResponseWriter, r *http.Request) {
	settings, _ := store.GetAllSettings(r.Context(), s.cfg.DB)

	// Determine whether any LLM is ready.
	// GetLLMStatus now returns "provider/model" (e.g. "ollama/llama3:latest").
	llmStatus := ""
	if s.cfg.LLM != nil {
		llmStatus = s.cfg.LLM.Status()
		if llmStatus == "none" {
			llmStatus = ""
		}
	}
	if llmStatus == "" {
		if p := settings["llm_provider"]; p != "" {
			llmStatus = p
		}
	}

	// Connectors that are active.
	connectors := []string{}
	if settings[config.TelegramSettingKeyBotToken] != "" || settings[config.TelegramSettingKeyAccounts] != "" {
		connectors = append(connectors, "telegram")
	}
	if settings["slack_bot_token"] != "" {
		connectors = append(connectors, "slack")
	}

	replyJSON(w, http.StatusOK, map[string]any{
		"complete":   settings["setup_complete"] == "true",
		"llm_status": llmStatus,
		"llm_ready":  llmStatus != "",
		"connectors": connectors,
	})
}

// handleSetupLLM POST /api/setup/llm
// Saves LLM provider credentials and hot-swaps the provider.
func (s *Server) handleSetupLLM(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
		APIKey   string `json:"api_key"`
		Model    string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Provider == "" {
		replyError(w, http.StatusBadRequest, "provider is required")
		return
	}
	if s.cfg.LLM == nil {
		replyError(w, http.StatusServiceUnavailable, "LLM control plane not configured")
		return
	}

	if err := s.cfg.LLM.SetupProvider(r.Context(), req.Provider, req.APIKey, req.Model); err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleSetupConnector POST /api/setup/connector
// Configures and starts a connector (telegram, slack, etc.).
// For type=telegram, params may include: bot_token (required), owner_chat_id, account,
// allow_from, pairing_code, gateway_url, api_url, webhook_url, and webhook_secret.
func (s *Server) handleSetupConnector(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type   string            `json:"type"`
		Params map[string]string `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Type == "" {
		replyError(w, http.StatusBadRequest, "type is required")
		return
	}
	if s.cfg.OnSetupConnector == nil {
		replyError(w, http.StatusServiceUnavailable, "connector setup callback not configured")
		return
	}
	if err := s.cfg.OnSetupConnector(r.Context(), req.Type, req.Params); err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
