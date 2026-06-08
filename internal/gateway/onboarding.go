package gateway

// onboarding.go - owner passport shape and API key management.
//
// Auth model:
//   - ONE owner secret: set once, break-glass only after setup
//   - API keys: all normal authentication post-onboarding
//   - X-Owner-Secret header: owner-only operations
//   - X-API-Key header: standard client auth

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/open-navi/navi/internal/onboarding"
	"github.com/open-navi/navi/internal/store"
)

// OwnerPassport is the one-time ownership and recovery artifact returned by
// first-run recovery creation.
type OwnerPassport struct {
	OwnerID                string    `json:"owner_id"`
	OwnerName              string    `json:"owner_name"`
	OwnerHandle            string    `json:"owner_handle"`
	InstanceID             string    `json:"instance_id"`
	OwnerSecretFingerprint string    `json:"owner_secret_fingerprint"`
	PrimaryAPIKey          string    `json:"primary_api_key"`
	RecoverySeed           string    `json:"recovery_seed"`
	CreatedAt              time.Time `json:"created_at"`
	Note                   string    `json:"note"`
}

// handleOnboardingStatus GET /api/onboarding/status
// Always public. Returns the safe first-run status view.
func (s *Server) handleOnboardingStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status, err := onboarding.DeriveFirstRunStatus(ctx, s.cfg.DB, s.dataDir())
	if err != nil {
		replyError(w, http.StatusInternalServerError, "status check failed")
		return
	}
	replyJSON(w, http.StatusOK, s.onboardingStatusPayload(ctx, status))
}

// handleCreateAPIKey POST /api/keys - owner secret required.
func (s *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !s.verifyOwnerSecret(w, r) {
		return
	}

	var req struct {
		Name   string   `json:"name"`
		Scopes []string `json:"scopes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" {
		req.Name = "unnamed"
	}
	if len(req.Scopes) == 0 {
		req.Scopes = []string{"read", "execute"}
	}

	owner, exists, err := store.GetOwner(ctx, s.cfg.DB)
	if err != nil || !exists {
		replyError(w, http.StatusForbidden, "no owner established")
		return
	}

	rawKey, hash, err := store.GenerateAPIKey()
	if err != nil {
		replyError(w, http.StatusInternalServerError, "failed to generate key")
		return
	}
	k, err := store.CreateAPIKeySimple(ctx, s.cfg.DB, owner.ID, hash, req.Name, req.Scopes)
	if err != nil {
		replyError(w, http.StatusInternalServerError, "failed to create key")
		return
	}

	replyJSON(w, http.StatusCreated, map[string]any{
		"id":         k.ID,
		"name":       k.Name,
		"scopes":     k.Scopes,
		"api_key":    rawKey,
		"created_at": k.CreatedAt,
		"note":       "Store this API key securely. It will not be shown again.",
	})
}

// handleListAPIKeys GET /api/keys - owner secret required.
func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !s.verifyOwnerSecret(w, r) {
		return
	}

	owner, exists, err := store.GetOwner(ctx, s.cfg.DB)
	if err != nil || !exists {
		replyError(w, http.StatusForbidden, "no owner established")
		return
	}

	keys, err := store.ListAPIKeysByOwner(ctx, s.cfg.DB, owner.ID)
	if err != nil {
		replyError(w, http.StatusInternalServerError, "failed to list keys")
		return
	}

	type keyView struct {
		ID         string   `json:"id"`
		Name       string   `json:"name"`
		Scopes     []string `json:"scopes"`
		CreatedAt  string   `json:"created_at"`
		LastUsedAt *string  `json:"last_used_at,omitempty"`
	}
	out := make([]keyView, 0, len(keys))
	for _, k := range keys {
		kv := keyView{
			ID:        k.ID,
			Name:      k.Name,
			Scopes:    k.Scopes,
			CreatedAt: k.CreatedAt.Format(time.RFC3339),
		}
		if k.LastUsedAt != nil {
			s := k.LastUsedAt.Format(time.RFC3339)
			kv.LastUsedAt = &s
		}
		out = append(out, kv)
	}
	replyJSON(w, http.StatusOK, out)
}

// handleRevokeAPIKey DELETE /api/keys/{id} - owner secret required.
func (s *Server) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !s.verifyOwnerSecret(w, r) {
		return
	}

	owner, exists, err := store.GetOwner(ctx, s.cfg.DB)
	if err != nil || !exists {
		replyError(w, http.StatusForbidden, "no owner established")
		return
	}

	id := r.PathValue("id")
	if err := store.RevokeAPIKeyByID(ctx, s.cfg.DB, id, owner.ID); err != nil {
		replyError(w, http.StatusNotFound, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// verifyOwnerSecret checks the X-Owner-Secret header. Writes 401 and returns
// false if invalid.
func (s *Server) verifyOwnerSecret(w http.ResponseWriter, r *http.Request) bool {
	secret := r.Header.Get("X-Owner-Secret")
	if secret == "" {
		replyError(w, http.StatusUnauthorized, "X-Owner-Secret header required")
		return false
	}
	ok, err := store.VerifyOwnerSecret(r.Context(), s.cfg.DB, secret)
	if err != nil {
		replyError(w, http.StatusInternalServerError, "auth check failed")
		return false
	}
	if !ok {
		replyError(w, http.StatusUnauthorized, "invalid owner secret")
		return false
	}
	return true
}

// handleAuthMe GET /api/auth/me - returns whether the current API key is the
// owner's primary key. Requires X-API-Key or Authorization: Bearer.
func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			apiKey = strings.TrimPrefix(auth, "Bearer ")
		}
	}
	if apiKey == "" {
		replyError(w, http.StatusUnauthorized, "X-API-Key or Authorization required")
		return
	}
	key, found, err := store.LookupAPIKeyByRaw(ctx, s.cfg.DB, apiKey)
	if err != nil {
		replyError(w, http.StatusInternalServerError, "auth check failed")
		return
	}
	if !found {
		replyError(w, http.StatusUnauthorized, "invalid API key")
		return
	}
	owner, exists, err := store.GetOwner(ctx, s.cfg.DB)
	if err != nil {
		replyError(w, http.StatusInternalServerError, "owner lookup failed")
		return
	}
	if !exists {
		replyError(w, http.StatusNotFound, "no owner")
		return
	}
	isOwner := key.Name == owner.Handle+"-primary"
	replyJSON(w, http.StatusOK, map[string]bool{"is_owner": isOwner})
}
