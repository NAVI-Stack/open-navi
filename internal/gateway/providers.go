package gateway

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/open-navi/navi/internal/llm"
)

// LLMService is the narrow interface that the gateway uses to interact with LLM
// provider controls. It is implemented by *llm.ControlPlane but tested or mocked
// via any value satisfying the interface.
type LLMService interface {
	// Status returns the active provider/model status string.
	Status() string

	// Catalog returns the current provider/model catalog.
	Catalog(ctx context.Context) (llm.LLMCatalog, error)

	// Profiles returns task-aware model profiles.
	Profiles(ctx context.Context) ([]llm.ModelProfile, error)

	// GetActive returns the currently selected provider/model.
	GetActive(ctx context.Context) (llm.Active, error)

	// SetActive validates and applies a new active provider/model selection.
	SetActive(ctx context.Context, provider, model string) (llm.Active, error)

	// GetPreferences returns persisted model routing preferences.
	GetPreferences(ctx context.Context) (llm.ModelPreferences, error)

	// PatchPreferences applies a partial update to routing preferences.
	PatchPreferences(ctx context.Context, raw map[string]any) (llm.ModelPreferences, error)

	// SetupProvider configures and hot-swaps an LLM provider.
	SetupProvider(ctx context.Context, provider, apiKey, model string) error

	// ConfigureProvider configures a provider with full request params (including Ollama endpoint).
	ConfigureProvider(ctx context.Context, req llm.ProviderConfigRequest) error

	// DisableProvider removes a provider's credentials and rebuilds the registry.
	DisableProvider(ctx context.Context, providerKey string) error

	// ListProviders returns descriptors for all known providers, including unconfigured ones.
	ListProviders(ctx context.Context) ([]llm.ProviderDescriptor, error)

	// GetProvider returns a descriptor for a single provider.
	GetProvider(ctx context.Context, providerKey string) (llm.ProviderDescriptor, error)

	// Health performs a live health check on a provider.
	Health(ctx context.Context, providerKey string) (llm.ProviderHealth, error)

	// ProviderModels lists models for a specific provider.
	ProviderModels(ctx context.Context, providerKey string) ([]llm.ModelDescriptor, error)

	// ProviderRunningModels lists currently running models for a local provider.
	ProviderRunningModels(ctx context.Context, providerKey string) ([]llm.RunningModel, error)

	// ExecuteProviderAction runs an action (pull, delete, warm, copy) on a provider.
	ExecuteProviderAction(ctx context.Context, req llm.ProviderActionRequest) (llm.ProviderOperation, error)

	// GetOperation retrieves a provider operation by ID.
	GetOperation(ctx context.Context, id string) (*llm.ProviderOperation, error)
}

// --- Provider Control Handlers ---

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	if !HasScope(r.Context(), "read") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "read scope required", nil)
		return
	}
	if s.cfg.LLM == nil {
		replyError(w, http.StatusServiceUnavailable, "LLM control plane not configured")
		return
	}
	providers, err := s.cfg.LLM.ListProviders(r.Context())
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, map[string]any{"providers": providers})
}

func (s *Server) handleGetProvider(w http.ResponseWriter, r *http.Request) {
	if !HasScope(r.Context(), "read") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "read scope required", nil)
		return
	}
	if s.cfg.LLM == nil {
		replyError(w, http.StatusServiceUnavailable, "LLM control plane not configured")
		return
	}
	providerKey := r.PathValue("id")
	desc, err := s.cfg.LLM.GetProvider(r.Context(), providerKey)
	if err != nil {
		replyError(w, http.StatusNotFound, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, desc)
}

func (s *Server) handleProviderHealth(w http.ResponseWriter, r *http.Request) {
	if !HasScope(r.Context(), "read") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "read scope required", nil)
		return
	}
	if s.cfg.LLM == nil {
		replyError(w, http.StatusServiceUnavailable, "LLM control plane not configured")
		return
	}
	providerKey := r.PathValue("id")
	health, err := s.cfg.LLM.Health(r.Context(), providerKey)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, health)
}

func (s *Server) handleProviderModels(w http.ResponseWriter, r *http.Request) {
	if !HasScope(r.Context(), "read") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "read scope required", nil)
		return
	}
	if s.cfg.LLM == nil {
		replyError(w, http.StatusServiceUnavailable, "LLM control plane not configured")
		return
	}
	providerKey := r.PathValue("id")
	models, err := s.cfg.LLM.ProviderModels(r.Context(), providerKey)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, map[string]any{"models": models})
}

func (s *Server) handleProviderRunning(w http.ResponseWriter, r *http.Request) {
	if !HasScope(r.Context(), "read") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "read scope required", nil)
		return
	}
	if s.cfg.LLM == nil {
		replyError(w, http.StatusServiceUnavailable, "LLM control plane not configured")
		return
	}
	providerKey := r.PathValue("id")
	running, err := s.cfg.LLM.ProviderRunningModels(r.Context(), providerKey)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, map[string]any{"running": running})
}

func (s *Server) handleProviderPull(w http.ResponseWriter, r *http.Request) {
	if !HasScope(r.Context(), "execute") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "execute scope required", nil)
		return
	}
	if s.cfg.LLM == nil {
		replyError(w, http.StatusServiceUnavailable, "LLM control plane not configured")
		return
	}
	providerKey := r.PathValue("id")
	var req struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	op, err := s.cfg.LLM.ExecuteProviderAction(r.Context(), llm.ProviderActionRequest{
		Provider: providerKey,
		Action:   "pull",
		Model:    req.Model,
	})
	if err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	replyJSON(w, http.StatusAccepted, op)
}

func (s *Server) handleProviderDelete(w http.ResponseWriter, r *http.Request) {
	if !HasScope(r.Context(), "execute") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "execute scope required", nil)
		return
	}
	if s.cfg.LLM == nil {
		replyError(w, http.StatusServiceUnavailable, "LLM control plane not configured")
		return
	}
	providerKey := r.PathValue("id")
	var req struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	op, err := s.cfg.LLM.ExecuteProviderAction(r.Context(), llm.ProviderActionRequest{
		Provider: providerKey,
		Action:   "delete",
		Model:    req.Model,
	})
	if err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, op)
}

func (s *Server) handleProviderWarm(w http.ResponseWriter, r *http.Request) {
	if !HasScope(r.Context(), "execute") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "execute scope required", nil)
		return
	}
	if s.cfg.LLM == nil {
		replyError(w, http.StatusServiceUnavailable, "LLM control plane not configured")
		return
	}
	providerKey := r.PathValue("id")
	var req struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	op, err := s.cfg.LLM.ExecuteProviderAction(r.Context(), llm.ProviderActionRequest{
		Provider: providerKey,
		Action:   "warm",
		Model:    req.Model,
	})
	if err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	replyJSON(w, http.StatusOK, op)
}

func (s *Server) handleGetOperation(w http.ResponseWriter, r *http.Request) {
	if !HasScope(r.Context(), "read") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "read scope required", nil)
		return
	}
	if s.cfg.LLM == nil {
		replyError(w, http.StatusServiceUnavailable, "LLM control plane not configured")
		return
	}
	opID := r.PathValue("id")
	op, err := s.cfg.LLM.GetOperation(r.Context(), opID)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if op == nil {
		replyError(w, http.StatusNotFound, "operation not found")
		return
	}
	replyJSON(w, http.StatusOK, op)
}

// handleConfigureProvider handles PUT /api/llm/providers/{id} — configure or update provider credentials.
func (s *Server) handleConfigureProvider(w http.ResponseWriter, r *http.Request) {
	if !HasScope(r.Context(), "execute") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "execute scope required", nil)
		return
	}
	if s.cfg.LLM == nil {
		replyError(w, http.StatusServiceUnavailable, "LLM control plane not configured")
		return
	}
	providerKey := r.PathValue("id")
	var body struct {
		APIKey   string `json:"api_key"`
		Model    string `json:"model"`
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.cfg.LLM.ConfigureProvider(r.Context(), llm.ProviderConfigRequest{
		Provider: providerKey,
		APIKey:   body.APIKey,
		Model:    body.Model,
		Endpoint: body.Endpoint,
	}); err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	desc, err := s.cfg.LLM.GetProvider(r.Context(), providerKey)
	if err != nil {
		replyJSON(w, http.StatusOK, map[string]any{"key": providerKey, "configured": true})
		return
	}
	replyJSON(w, http.StatusOK, desc)
}

// handleDisableProvider handles DELETE /api/llm/providers/{id} — disable/remove provider credentials.
func (s *Server) handleDisableProvider(w http.ResponseWriter, r *http.Request) {
	if !HasScope(r.Context(), "execute") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "execute scope required", nil)
		return
	}
	if s.cfg.LLM == nil {
		replyError(w, http.StatusServiceUnavailable, "LLM control plane not configured")
		return
	}
	providerKey := r.PathValue("id")
	if err := s.cfg.LLM.DisableProvider(r.Context(), providerKey); err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
