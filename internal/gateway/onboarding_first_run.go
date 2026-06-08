package gateway

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/navi"
	"github.com/ceoai/navi/internal/onboarding"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

const postOnboardingGreetingContent = "Hi, I'm NAVI. Your local instance is ready. I can help you chat, plan, inspect system state, and start working through tasks when you are."

var postOnboardingGreetingDelay = randomPostOnboardingGreetingDelay
var schedulePostOnboardingGreeting = func(delay time.Duration, fn func()) *time.Timer {
	return time.AfterFunc(delay, fn)
}

type ownerPassportCreateRequest struct {
	OwnerName   string
	OwnerHandle string
	DeviceName  string
	OwnerSecret string
}

type ownerPassportCreateResult struct {
	Passport                OwnerPassport
	OwnerSecret             string
	OwnerSecretWasGenerated bool
	PrimaryAPIKeyID         string
	PrimaryAPIKeyLast8Hash  string
	RecoverySeedCheckHash   string
}

func (s *Server) dataDir() string {
	if s.cfg.DataDir == "" {
		return "."
	}
	return s.cfg.DataDir
}

func (s *Server) handleOnboardingPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	path := filepath.Join(s.cfg.StaticDir, "onboarding.html")
	if _, err := os.Stat(path); err != nil {
		replyError(w, http.StatusNotFound, "onboarding page not found")
		return
	}
	http.ServeFile(w, r, path)
}

func (s *Server) handleStaticOrOnboardingRedirect(fs http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Server-routable prefixes must NOT get the SPA fallback.
		// Let them fall through to the file server which will 404 naturally
		// for paths that have no registered route handler.
		p := r.URL.Path
		if strings.HasPrefix(p, "/api/") ||
			strings.HasPrefix(p, "/auth/") ||
			strings.HasPrefix(p, "/ws/") ||
			strings.HasPrefix(p, "/webhooks/") ||
			strings.HasPrefix(p, "/v1/") ||
			strings.HasPrefix(p, "/v2/") {
			fs.ServeHTTP(w, r)
			return
		}

		// If the request has a file extension it is a static asset (JS, CSS,
		// images, source maps, fonts, etc.). Serve it from the file server
		// directly so hashed bundles, favicons, and manifests still work.
		if filepath.Ext(p) != "" && !isConsoleRoutePath(p) {
			fs.ServeHTTP(w, r)
			return
		}

		// For all non-asset, non-API paths (/, /chats/abc, /projects/xyz, etc.):
		// If first-run onboarding is not complete, redirect to /onboarding.
		if status, err := onboarding.DeriveFirstRunStatus(r.Context(), s.cfg.DB, s.dataDir()); err == nil {
			if status.State != onboarding.FirstRunComplete {
				http.Redirect(w, r, "/onboarding", http.StatusFound)
				return
			}
		}

		// SPA fallback: serve index.html for every non-asset path.
		// The React client-side router reads window.location.pathname and
		// renders the correct page. This prevents 404s on browser refresh.
		http.ServeFile(w, r, filepath.Join(s.cfg.StaticDir, "index.html"))
	})
}

func isConsoleRoutePath(path string) bool {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	if len(segments) == 0 || segments[0] == "" {
		return true
	}
	switch segments[0] {
	case "artifacts", "ceremony", "chats", "debug", "dev", "docs", "overview", "plugins", "projects", "proposals", "runs", "scheduler", "settings", "usage", "workspaces":
		return true
	default:
		return false
	}
}

// handleOnboardingRecovery POST /api/onboarding/recovery
// Creates the owner, primary API key, owner recovery secret, and recovery seed
// for the web first-run flow. Raw secrets are returned only on this response.
func (s *Server) handleOnboardingRecovery(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status, err := onboarding.DeriveFirstRunStatus(ctx, s.cfg.DB, s.dataDir())
	if err != nil {
		replyError(w, http.StatusInternalServerError, "status check failed")
		return
	}
	if status.State != onboarding.FirstRunUninitialized {
		replyJSON(w, http.StatusConflict, map[string]any{
			"error":               "Recovery has already been created. Secrets cannot be shown again from this endpoint.",
			"first_run_state":     status.State,
			"recovery_saved_path": status.RecoverySavedPath,
		})
		return
	}

	var req struct {
		OwnerName   string `json:"owner_name"`
		OwnerHandle string `json:"owner_handle"`
		DeviceName  string `json:"device_name"`
		OwnerSecret string `json:"owner_secret"`
		SavePath    string `json:"save_path"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	created, err := s.createOwnerPassport(ctx, ownerPassportCreateRequest{
		OwnerName:   req.OwnerName,
		OwnerHandle: req.OwnerHandle,
		DeviceName:  req.DeviceName,
		OwnerSecret: req.OwnerSecret,
	})
	if err != nil {
		replyError(w, http.StatusInternalServerError, onboarding.RedactSecrets(err.Error()))
		return
	}

	if err := onboarding.SetFirstRunState(ctx, s.cfg.DB, status.State, onboarding.FirstRunRecoveryCreated); err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}

	savePath := strings.TrimSpace(req.SavePath)
	if savePath == "" {
		savePath = s.defaultRecoveryPassportPath()
	}
	passportText := renderOwnerPassportText(created.Passport, created.OwnerSecret, created.PrimaryAPIKeyID)
	saved := false
	saveError := ""
	if err := onboarding.WriteRecoveryPassport(savePath, []byte(passportText)); err != nil {
		saveError = onboarding.RedactSecrets(err.Error())
	} else {
		saved = true
		_ = store.SetSetting(ctx, s.cfg.DB, onboarding.SettingKeyRecoverySavedPath, savePath)
	}

	resp := map[string]any{
		"first_run_state":          onboarding.FirstRunRecoveryCreated,
		"owner_id":                 created.Passport.OwnerID,
		"owner_name":               created.Passport.OwnerName,
		"owner_handle":             created.Passport.OwnerHandle,
		"instance_id":              created.Passport.InstanceID,
		"owner_secret_fingerprint": created.Passport.OwnerSecretFingerprint,
		"primary_api_key":          created.Passport.PrimaryAPIKey,
		"api_key":                  created.Passport.PrimaryAPIKey,
		"key_id":                   created.PrimaryAPIKeyID,
		"recovery_seed":            created.Passport.RecoverySeed,
		"created_at":               created.Passport.CreatedAt,
		"note":                     created.Passport.Note,
		"recovery_saved":           saved,
		"recovery_saved_path":      savePath,
		"recovery_save_error":      saveError,
		"passport_text":            passportText,
		"download_filename":        "navi-owner-passport.txt",
		"secrets_shown_once":       true,
	}
	if created.OwnerSecretWasGenerated {
		resp["admin_secret"] = created.OwnerSecret
	}
	replyJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleOnboardingProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status, err := onboarding.DeriveFirstRunStatus(ctx, s.cfg.DB, s.dataDir())
	if err != nil {
		replyError(w, http.StatusInternalServerError, "status check failed")
		return
	}
	if status.State == onboarding.FirstRunUninitialized {
		replyError(w, http.StatusBadRequest, "create recovery first")
		return
	}
	if status.State == onboarding.FirstRunComplete {
		replyJSON(w, http.StatusOK, map[string]any{
			"first_run_state": status.State,
			"provider":        status.ProviderName,
			"model":           status.ProviderModel,
		})
		return
	}
	if s.cfg.LLM == nil {
		replyError(w, http.StatusServiceUnavailable, "LLM control plane not configured")
		return
	}

	var req struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
		APIKey   string `json:"api_key"`
		Endpoint string `json:"endpoint"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider == "" {
		provider = s.defaultOnboardingProvider(ctx)
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = s.defaultOnboardingModel(ctx, provider)
	}
	if provider == "" {
		replyError(w, http.StatusBadRequest, "provider is required")
		return
	}
	if model == "" {
		replyError(w, http.StatusBadRequest, "model is required")
		return
	}

	if err := s.cfg.LLM.SetupProvider(ctx, provider, strings.TrimSpace(req.APIKey), model); err != nil {
		replyError(w, http.StatusBadRequest, onboarding.RedactSecrets(err.Error()))
		return
	}
	_ = store.SetSetting(ctx, s.cfg.DB, onboarding.SettingKeyProviderConfigured, "true")
	_ = store.SetSetting(ctx, s.cfg.DB, onboarding.SettingKeyProviderName, provider)
	_ = store.SetSetting(ctx, s.cfg.DB, onboarding.SettingKeyProviderModel, model)
	if strings.TrimSpace(req.Endpoint) != "" {
		_ = store.SetSetting(ctx, s.cfg.DB, onboarding.SettingKeyProviderEndpoint, strings.TrimSpace(req.Endpoint))
	}
	if err := onboarding.SetFirstRunState(ctx, s.cfg.DB, status.State, onboarding.FirstRunProviderConfigured); err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}

	replyJSON(w, http.StatusOK, map[string]any{
		"status":          "ok",
		"first_run_state": onboarding.FirstRunProviderConfigured,
		"provider":        provider,
		"model":           model,
	})
}

func (s *Server) handleOnboardingConnection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status, err := onboarding.DeriveFirstRunStatus(ctx, s.cfg.DB, s.dataDir())
	if err != nil {
		replyError(w, http.StatusInternalServerError, "status check failed")
		return
	}
	if status.State == onboarding.FirstRunUninitialized {
		replyError(w, http.StatusBadRequest, "create recovery first")
		return
	}

	var req struct {
		Skip   bool              `json:"skip"`
		Type   string            `json:"type"`
		Params map[string]string `json:"params"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	connType := strings.TrimSpace(req.Type)
	if req.Skip || connType == "" || connType == "none" {
		_ = store.SetSetting(ctx, s.cfg.DB, onboarding.SettingKeyConnectionStatus, "skipped")
		_ = store.SetSetting(ctx, s.cfg.DB, onboarding.SettingKeyConnectionType, "")
		replyJSON(w, http.StatusOK, map[string]any{
			"status":            "skipped",
			"first_run_state":   status.State,
			"connection_status": "skipped",
		})
		return
	}

	if !s.onboardingConnectionSupported(connType) {
		replyError(w, http.StatusBadRequest, "connection type is not supported")
		return
	}
	if s.cfg.OnSetupConnector == nil {
		replyError(w, http.StatusServiceUnavailable, "connector setup is not configured")
		return
	}
	if err := s.cfg.OnSetupConnector(ctx, connType, req.Params); err != nil {
		replyError(w, http.StatusBadRequest, onboarding.RedactSecrets(err.Error()))
		return
	}
	_ = store.SetSetting(ctx, s.cfg.DB, onboarding.SettingKeyConnectionStatus, "configured")
	_ = store.SetSetting(ctx, s.cfg.DB, onboarding.SettingKeyConnectionType, connType)
	replyJSON(w, http.StatusOK, map[string]any{
		"status":            "configured",
		"first_run_state":   status.State,
		"connection_status": "configured",
		"connection_type":   connType,
	})
}

func (s *Server) handleOnboardingComplete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status, err := onboarding.DeriveFirstRunStatus(ctx, s.cfg.DB, s.dataDir())
	if err != nil {
		replyError(w, http.StatusInternalServerError, "status check failed")
		return
	}
	if status.State == onboarding.FirstRunComplete {
		replyJSON(w, http.StatusOK, map[string]any{"status": "ok", "first_run_state": status.State, "redirect": s.postOnboardingRedirect(ctx)})
		return
	}
	if status.State != onboarding.FirstRunProviderConfigured {
		replyError(w, http.StatusBadRequest, "configure a provider before completing onboarding")
		return
	}
	owner, exists, err := store.GetOwner(ctx, s.cfg.DB)
	if err != nil || !exists {
		replyError(w, http.StatusBadRequest, "owner not found; create recovery first")
		return
	}
	connectionStatus := strings.TrimSpace(status.ConnectionStatus)
	if connectionStatus == "" {
		connectionStatus = "skipped"
		_ = store.SetSetting(ctx, s.cfg.DB, onboarding.SettingKeyConnectionStatus, "skipped")
	}
	snapshot := firstRunCompletionSnapshot{
		ProviderName:      status.ProviderName,
		ProviderModel:     status.ProviderModel,
		ProviderEndpoint:  status.ProviderEndpoint,
		ConnectionStatus:  connectionStatus,
		ConnectionType:    status.ConnectionType,
		RecoverySavedPath: status.RecoverySavedPath,
	}
	if err := writeGenesisAudit(ctx, s.cfg.DB, owner, snapshot); err != nil {
		replyError(w, http.StatusInternalServerError, "failed to write genesis audit")
		return
	}
	if err := writeConfigSnapshot(s.dataDir(), owner, snapshot); err != nil {
		replyError(w, http.StatusInternalServerError, "failed to write config snapshot")
		return
	}
	if err := onboarding.SetFirstRunState(ctx, s.cfg.DB, status.State, onboarding.FirstRunComplete); err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.queuePostOnboardingGreeting(ctx)
	replyJSON(w, http.StatusOK, map[string]any{"status": "ok", "first_run_state": onboarding.FirstRunComplete, "redirect": s.postOnboardingRedirect(ctx)})
}

func randomPostOnboardingGreetingDelay() time.Duration {
	n, err := rand.Int(rand.Reader, big.NewInt(46))
	if err != nil {
		return 30 * time.Second
	}
	return time.Duration(15+n.Int64()) * time.Second
}

func (s *Server) queuePostOnboardingGreeting(ctx context.Context) {
	if s == nil || s.cfg.Navi == nil {
		return
	}
	// CreateChat ensures a target exists; SendProactiveMessage will find it as
	// the most-recently active chat after the frontend redirect.
	if _, err := s.cfg.Navi.CreateChat(ctx, navi.ExperienceModeStandard, ""); err != nil {
		slog.Warn("onboarding: create greeting chat failed", "error", err)
		return
	}
	delay := postOnboardingGreetingDelay()
	if delay < 15*time.Second {
		delay = 15 * time.Second
	}
	if delay > 60*time.Second {
		delay = 60 * time.Second
	}
	t := schedulePostOnboardingGreeting(delay, func() {
		sendCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.cfg.Navi.SendProactiveMessage(sendCtx, postOnboardingGreetingContent); err != nil {
			slog.Warn("onboarding: proactive greeting failed", "error", err)
		}
	})
	if t != nil {
		s.greetingTimer.Store(t)
	}
}

// handleOnboardingReset POST /api/onboarding/reset
// Wipes all SQLite state and purges JetStream streams, but ONLY if onboarding is not yet complete.
func (s *Server) handleOnboardingReset(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status, err := onboarding.DeriveFirstRunStatus(ctx, s.cfg.DB, s.dataDir())
	if err != nil {
		replyError(w, http.StatusInternalServerError, "status check failed")
		return
	}
	if status.Complete {
		replyError(w, http.StatusBadRequest, "cannot reset completed onboarding flow via this endpoint")
		return
	}

	if err := store.ResetInstance(ctx, s.cfg.DB); err != nil {
		replyError(w, http.StatusInternalServerError, "reset database failed: "+err.Error())
		return
	}

	if s.cfg.Bus != nil {
		if err := s.cfg.Bus.PurgeAll(ctx); err != nil {
			replyError(w, http.StatusInternalServerError, "reset db ok but stream purge failed: "+err.Error())
			return
		}
	}

	configPath := filepath.Join(s.dataDir(), "config.json")
	if _, err := os.Stat(configPath); err == nil {
		_ = os.Remove(configPath)
	}

	replyJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) createOwnerPassport(ctx context.Context, req ownerPassportCreateRequest) (*ownerPassportCreateResult, error) {
	exists, err := store.OwnerExists(ctx, s.cfg.DB)
	if err != nil {
		return nil, fmt.Errorf("claim check failed: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("already claimed")
	}

	ownerName := strings.TrimSpace(req.OwnerName)
	if ownerName == "" {
		ownerName = "Owner"
	}
	ownerHandle := strings.TrimSpace(req.OwnerHandle)
	if ownerHandle == "" {
		ownerHandle = strings.ToLower(strings.ReplaceAll(ownerName, " ", "_"))
	}
	ownerHandle = strings.ToLower(strings.ReplaceAll(ownerHandle, " ", "-"))

	ownerSecret := strings.TrimSpace(req.OwnerSecret)
	secretGeneratedByServer := false
	if ownerSecret == "" {
		raw, _, err := store.GenerateAPIKey()
		if err != nil {
			return nil, fmt.Errorf("failed to generate owner secret: %w", err)
		}
		ownerSecret = strings.TrimPrefix(raw, "navi_")
		secretGeneratedByServer = true
	}

	fingerprint := store.OwnerSecretFingerprint(ownerSecret)
	owner := store.Owner{
		Name:              ownerName,
		Handle:            ownerHandle,
		DeviceName:        strings.TrimSpace(req.DeviceName),
		SecretFingerprint: fingerprint,
	}
	if err := store.CreateOwner(ctx, s.cfg.DB, owner); err != nil {
		return nil, fmt.Errorf("failed to create owner: %w", err)
	}
	owner, _, _ = store.GetOwner(ctx, s.cfg.DB)

	if s.cfg.WorldModel != nil {
		meta := map[string]any{
			"handle":      owner.Handle,
			"device_name": owner.DeviceName,
		}
		b, _ := json.Marshal(meta)
		_, _ = s.cfg.WorldModel.UpsertContact(ctx, schema.Contact{
			ID:        owner.ID,
			Name:      owner.Name,
			Kind:      "person",
			Metadata:  string(b),
			CreatedAt: owner.CreatedAt,
		})
	}

	if err := store.SetOwnerSecret(ctx, s.cfg.DB, ownerSecret); err != nil {
		return nil, fmt.Errorf("failed to set owner secret: %w", err)
	}

	rawKey, hash, err := store.GenerateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}
	k, err := store.CreateAPIKeySimple(ctx, s.cfg.DB, owner.ID, hash, owner.Handle+"-primary", []string{"admin", "read", "execute"})
	if err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}

	recoverySeedRaw, _, _ := store.GenerateAPIKey()
	recoverySeed := strings.TrimPrefix(recoverySeedRaw, "navi_")
	primaryLast8Hash := ""
	if len(rawKey) >= 8 {
		primaryLast8Hash = store.HashAPIKey(rawKey[len(rawKey)-8:])
	}
	seedCheckHash := store.HashAPIKey(recoverySeed)
	if len(recoverySeed) >= 8 {
		seedCheckHash = store.HashAPIKey(recoverySeed[:8])
	}

	passport := OwnerPassport{
		OwnerID:                owner.ID,
		OwnerName:              owner.Name,
		OwnerHandle:            owner.Handle,
		InstanceID:             owner.InstanceID,
		OwnerSecretFingerprint: fingerprint,
		PrimaryAPIKey:          rawKey,
		RecoverySeed:           recoverySeed,
		CreatedAt:              k.CreatedAt,
		Note:                   "Store your owner recovery secret and recovery seed securely. Use the API key for normal access.",
	}
	return &ownerPassportCreateResult{
		Passport:                passport,
		OwnerSecret:             ownerSecret,
		OwnerSecretWasGenerated: secretGeneratedByServer,
		PrimaryAPIKeyID:         k.ID,
		PrimaryAPIKeyLast8Hash:  primaryLast8Hash,
		RecoverySeedCheckHash:   seedCheckHash,
	}, nil
}

func renderOwnerPassportText(passport OwnerPassport, ownerSecret, keyID string) string {
	lines := []string{
		"NAVI Owner Passport",
		"====================",
		"",
		"Keep this file somewhere private. NAVI will not show these secrets again.",
		"",
		"Owner",
		"-----",
		"Name: " + passport.OwnerName,
		"Handle: " + passport.OwnerHandle,
		"Owner ID: " + passport.OwnerID,
		"Instance ID: " + passport.InstanceID,
		"Owner secret fingerprint: " + passport.OwnerSecretFingerprint,
		"",
		"Secrets",
		"-------",
		"Owner recovery secret: " + ownerSecret,
		"Primary API key: " + passport.PrimaryAPIKey,
		"Primary API key ID: " + keyID,
		"Recovery seed: " + passport.RecoverySeed,
		"",
		"Created at: " + passport.CreatedAt.UTC().Format(time.RFC3339),
		"",
		"Use the API key for normal apps and clients. Keep the owner recovery secret for privileged recovery only.",
	}
	return strings.Join(lines, "\n") + "\n"
}

func (s *Server) defaultOnboardingProvider(ctx context.Context) string {
	if s.cfg.LLM == nil {
		return "ollama"
	}
	if health, err := s.cfg.LLM.Health(ctx, "ollama"); err == nil && health.Healthy {
		return "ollama"
	}
	catalog, err := s.cfg.LLM.Catalog(ctx)
	if err == nil {
		for _, p := range catalog.Providers {
			if p.Key == "ollama" {
				return "ollama"
			}
		}
		if len(catalog.Providers) > 0 {
			return catalog.Providers[0].Key
		}
	}
	return "ollama"
}

func (s *Server) defaultOnboardingModel(ctx context.Context, provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = "ollama"
	}
	if s.cfg.LLM != nil {
		if provider == "ollama" {
			if models, err := s.cfg.LLM.ProviderModels(ctx, "ollama"); err == nil && len(models) > 0 {
				return models[0].Name
			}
		}
		if catalog, err := s.cfg.LLM.Catalog(ctx); err == nil {
			for _, p := range catalog.Providers {
				if p.Key == provider && len(p.Models) > 0 {
					return p.Models[0].Name
				}
			}
		}
	}
	if provider == "openai" {
		return "gpt-4o-mini"
	}
	if provider == "anthropic" {
		return "claude-sonnet-4-20250514"
	}
	return "llama3:latest"
}

func (s *Server) onboardingConnectionSupported(connType string) bool {
	connType = strings.TrimSpace(connType)
	if connType == "" || connType == "none" {
		return true
	}
	descriptors := s.cfg.SetupSchema
	if s.cfg.ConnectorSetupDescriptors != nil {
		descriptors = s.cfg.ConnectorSetupDescriptors()
	}
	for _, desc := range descriptors {
		if desc.Type == connType {
			return true
		}
	}
	return false
}

func (s *Server) onboardingStatusPayload(ctx context.Context, status onboarding.FirstRunStatus) map[string]any {
	detectCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	defaultProvider := s.defaultOnboardingProvider(detectCtx)
	defaultModel := s.defaultOnboardingModel(detectCtx, defaultProvider)
	inContainer := runningInContainer()
	resp := map[string]any{
		"claimed":                    status.Claimed,
		"first_run_state":            status.State,
		"complete":                   status.Complete,
		"current_step":               currentOnboardingStep(status.State),
		"recovery_saved_path":        status.RecoverySavedPath,
		"recovery_default_save_path": s.defaultRecoveryPassportPath(),
		"runtime_mode":               firstRunRuntimeMode(inContainer),
		"in_container":               inContainer,
		"provider_configured":        status.State == onboarding.FirstRunProviderConfigured || status.State == onboarding.FirstRunComplete,
		"provider":                   status.ProviderName,
		"model":                      status.ProviderModel,
		"connection_status":          status.ConnectionStatus,
		"connection_type":            status.ConnectionType,
		"default_provider":           defaultProvider,
		"default_model":              defaultModel,
		"connection_options":         s.safeConnectionOptions(),
	}
	if status.Claimed {
		if owner, _, err := store.GetOwner(ctx, s.cfg.DB); err == nil {
			resp["owner_name"] = owner.Name
			resp["instance_id"] = owner.InstanceID
			resp["owner_secret_fingerprint"] = owner.SecretFingerprint
			resp["timezone"] = owner.Timezone
		}
	}
	if s.cfg.LLM != nil {
		if health, err := s.cfg.LLM.Health(detectCtx, "ollama"); err == nil {
			resp["ollama"] = map[string]any{
				"available":   health.Healthy,
				"message":     health.Message,
				"model_count": health.ModelCount,
			}
		}
		if catalog, err := s.cfg.LLM.Catalog(detectCtx); err == nil {
			resp["provider_options"] = safeProviderOptions(catalog)
		}
	}
	return resp
}

func (s *Server) defaultRecoveryPassportPath() string {
	if runningInContainer() {
		return onboarding.DefaultRecoveryPassportPath(s.dataDir())
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, "navi-owner-passport.txt")
	}
	return onboarding.DefaultRecoveryPassportPath(s.dataDir())
}

func firstRunRuntimeMode(inContainer bool) string {
	if inContainer {
		return "container"
	}
	return "daemon"
}

type firstRunCompletionSnapshot struct {
	ProviderName      string
	ProviderModel     string
	ProviderEndpoint  string
	ConnectionStatus  string
	ConnectionType    string
	RecoverySavedPath string
}

func writeGenesisAudit(ctx context.Context, db *sql.DB, owner store.Owner, snapshot firstRunCompletionSnapshot) error {
	payload := map[string]any{
		"event":               "instance_created",
		"timestamp":           time.Now().UTC().Format(time.RFC3339),
		"owner_id":            owner.ID,
		"owner_handle":        owner.Handle,
		"instance_id":         owner.InstanceID,
		"onboarding_version":  "first_run_local_v1",
		"provider":            snapshot.ProviderName,
		"model":               snapshot.ProviderModel,
		"provider_endpoint":   snapshot.ProviderEndpoint,
		"connection_status":   snapshot.ConnectionStatus,
		"connection_type":     snapshot.ConnectionType,
		"recovery_saved_path": snapshot.RecoverySavedPath,
	}
	data, _ := json.Marshal(payload)
	return store.SetSetting(ctx, db, "genesis_audit", string(data))
}

func writeConfigSnapshot(dataDir string, owner store.Owner, snapshot firstRunCompletionSnapshot) error {
	snap := map[string]any{
		"schema_version":           "first_run_local_v1",
		"instance_id":              owner.InstanceID,
		"owner_id":                 owner.ID,
		"owner_handle":             owner.Handle,
		"owner_public_key":         "",
		"owner_secret_fingerprint": owner.SecretFingerprint,
		"provider":                 snapshot.ProviderName,
		"model":                    snapshot.ProviderModel,
		"connection_status":        snapshot.ConnectionStatus,
		"connection_type":          snapshot.ConnectionType,
		"onboarding_complete":      true,
		"created_at":               time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.MarshalIndent(snap, "", "  ")
	path := filepath.Join(dataDir, "config.json")
	return os.WriteFile(path, data, 0o600)
}

func currentOnboardingStep(state onboarding.FirstRunState) string {
	switch state {
	case onboarding.FirstRunUninitialized:
		return "recovery"
	case onboarding.FirstRunRecoveryCreated:
		return "provider"
	case onboarding.FirstRunProviderConfigured:
		return "connection"
	case onboarding.FirstRunComplete:
		return "complete"
	default:
		return "recovery"
	}
}

func (s *Server) safeConnectionOptions() []map[string]any {
	descriptors := s.cfg.SetupSchema
	if s.cfg.ConnectorSetupDescriptors != nil {
		descriptors = s.cfg.ConnectorSetupDescriptors()
	}
	out := make([]map[string]any, 0, len(descriptors))
	for _, desc := range descriptors {
		fields := make([]map[string]any, 0, len(desc.RequiredParams)+len(desc.OptionalParams))
		for _, field := range desc.RequiredParams {
			fields = append(fields, map[string]any{
				"key":         field.Key,
				"label":       field.Label,
				"description": field.Description,
				"required":    true,
				"secret":      field.Secret,
			})
		}
		for _, field := range desc.OptionalParams {
			fields = append(fields, map[string]any{
				"key":         field.Key,
				"label":       field.Label,
				"description": field.Description,
				"required":    false,
				"secret":      field.Secret,
			})
		}
		out = append(out, map[string]any{
			"type":         desc.Type,
			"display_name": desc.DisplayName,
			"description":  desc.SetupHint,
			"fields":       fields,
		})
	}
	return out
}

func safeProviderOptions(catalog llm.LLMCatalog) []map[string]any {
	out := make([]map[string]any, 0, len(catalog.Providers))
	for _, provider := range catalog.Providers {
		models := make([]string, 0, len(provider.Models))
		for _, model := range provider.Models {
			models = append(models, model.Name)
		}
		out = append(out, map[string]any{
			"key":          provider.Key,
			"display_name": provider.DisplayName,
			"models":       models,
		})
	}
	return out
}
