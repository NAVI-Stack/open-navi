package gateway

import (
	"net/http"

	"github.com/open-navi/navi/internal/store"
)

// handleInstanceReset POST /api/instance/reset (and OPTIONS for CORS preflight).
// Owner-only (X-Owner-Secret). Wipes all SQLite state and purges JetStream
// streams so the instance returns to an uninitialized first-run state.
func (s *Server) handleInstanceReset(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		origin := r.Header.Get("Origin")
		if origin != "" && corsOriginAllowed(origin, s.cfg.OriginPatterns) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Owner-Secret")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Max-Age", "86400")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		replyError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.verifyOwnerSecret(w, r) {
		return
	}
	ctx := r.Context()

	if err := store.ResetInstance(ctx, s.cfg.DB); err != nil {
		replyError(w, http.StatusInternalServerError, "reset failed: "+err.Error())
		return
	}

	if s.cfg.Bus != nil {
		if err := s.cfg.Bus.PurgeAll(ctx); err != nil {
			replyError(w, http.StatusInternalServerError, "reset db ok but stream purge failed: "+err.Error())
			return
		}
	}

	replyJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
