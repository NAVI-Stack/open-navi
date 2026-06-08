package gateway

import (
	"net/http"
	"strings"

	"github.com/ceoai/navi/internal/navi/plugin"
)

// findPluginManifest returns the registered manifest matching id (by ID or PluginID).
func (s *Server) findPluginManifest(id string) (plugin.Manifest, bool) {
	if s.cfg.PluginManifests == nil {
		return plugin.Manifest{}, false
	}
	id = strings.TrimSpace(id)
	for _, m := range s.cfg.PluginManifests() {
		if strings.EqualFold(strings.TrimSpace(m.ID), id) || strings.EqualFold(strings.TrimSpace(m.PluginID), id) {
			return m, true
		}
	}
	return plugin.Manifest{}, false
}

// handleSetPluginEnabled toggles a plugin's enabled state via the SetPluginEnabled hook.
func (s *Server) handleSetPluginEnabled(enabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if id == "" {
			replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "plugin id required", nil)
			return
		}
		m, ok := s.findPluginManifest(id)
		if !ok {
			replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "plugin not found", nil)
			return
		}
		if s.cfg.SetPluginEnabled == nil {
			replyErrorAPI(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "plugin enable/disable not supported", nil)
			return
		}
		if err := s.cfg.SetPluginEnabled(m.ID, enabled); err != nil {
			replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
			return
		}
		replyJSON(w, http.StatusOK, map[string]any{
			"pluginId": m.ID,
			"enabled":  enabled,
		})
	}
}

// handleValidatePlugin recomputes manifest validation and returns the result.
func (s *Server) handleValidatePlugin(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "plugin id required", nil)
		return
	}
	m, ok := s.findPluginManifest(id)
	if !ok {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "plugin not found", nil)
		return
	}
	m.Normalize(m.ID, m.RootDir)
	reasons := []string{}
	valid := true
	if err := m.Validate(); err != nil {
		valid = false
		reasons = append(reasons, err.Error())
	}
	replyJSON(w, http.StatusOK, map[string]any{
		"pluginId": m.ID,
		"valid":    valid,
		"reasons":  reasons,
	})
}

// handleReloadPlugins re-runs the plugin loader via the ReloadPlugins hook.
func (s *Server) handleReloadPlugins(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ReloadPlugins == nil {
		replyErrorAPI(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "plugin reload not supported", nil)
		return
	}
	if err := s.cfg.ReloadPlugins(); err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	count := 0
	if s.cfg.PluginManifests != nil {
		count = len(s.cfg.PluginManifests())
	}
	replyJSON(w, http.StatusOK, map[string]any{
		"status":  "reloaded",
		"plugins": count,
	})
}
