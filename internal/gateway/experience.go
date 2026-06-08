package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/open-navi/navi/internal/navi"
	"github.com/open-navi/navi/internal/navi/experience"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

type experienceSnapshotResponse struct {
	OwnerID             string                         `json:"owner_id"`
	CoreIdentity        experience.TraitSetConfig      `json:"core_identity"`
	OutputPreferences   experience.OutputPreferences   `json:"output_preferences"`
	RelationshipProfile experience.RelationshipProfile `json:"relationship_profile"`
	PersonaModules      []experience.ModuleConfig      `json:"persona_modules"`
}

type experienceModuleRegistryResponse struct {
	OwnerID string                           `json:"owner_id"`
	Modules []experience.ModuleRegistryEntry `json:"modules"`
}

type experienceInspectResponse struct {
	OwnerID         string                           `json:"owner_id"`
	StoredConfig    experience.ConfigurationSnapshot `json:"stored_config"`
	EffectiveState  experience.EffectivePersonaState `json:"effective_state"`
	CompiledPayload experience.CompiledPersonaPayload `json:"compiled_payload"`
	SnapshotHistory []schema.Event                  `json:"snapshot_history"`
}

type putExperienceCoreIdentityRequest struct {
	Traits map[string]float64 `json:"traits"`
}

type putExperienceOutputPreferencesRequest struct {
	PreferredLength experience.PreferredLength `json:"preferred_length"`
	PreferredFormat experience.PreferredFormat `json:"preferred_format"`
	SummaryFirst    *bool                      `json:"summary_first"`
}

type putExperienceModulesRequest struct {
	PersonaModules []experience.ModuleConfig `json:"persona_modules"`
}

func (s *Server) handleGetExperience(w http.ResponseWriter, r *http.Request) {
	if !s.requireOwnerReadAccess(w, r) {
		return
	}
	ownerID, ok := s.resolveExperienceOwnerID(w, r)
	if !ok {
		return
	}
	s.replyExperienceSnapshot(w, r, ownerID)
}

func (s *Server) handleGetExperienceModuleRegistry(w http.ResponseWriter, r *http.Request) {
	if !s.requireOwnerReadAccess(w, r) {
		return
	}
	ownerID, ok := s.resolveExperienceOwnerID(w, r)
	if !ok {
		return
	}
	modules, err := experience.ListModuleRegistry(r.Context(), s.cfg.DB, ownerID, 200)
	if err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusOK, experienceModuleRegistryResponse{
		OwnerID: ownerID,
		Modules: modules,
	})
}

func (s *Server) handleGetExperienceInspect(w http.ResponseWriter, r *http.Request) {
	if !s.requireOwnerReadAccess(w, r) {
		return
	}
	ownerID, ok := s.resolveExperienceOwnerID(w, r)
	if !ok {
		return
	}
	if s.cfg.ExperienceBuilder == nil {
		replyErrorAPI(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "experience builder not configured", nil)
		return
	}

	mode := navi.ExperienceMode(r.URL.Query().Get("mode"))
	if mode == "" {
		mode = "balanced" // Default to balanced for inspection if not specified
	}

	// Load stored config for reference
	stored, err := experience.LoadConfigurationSnapshot(r.Context(), s.cfg.DB, ownerID)
	if err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", "failed to load stored config: "+err.Error(), nil)
		return
	}

	// Build effective state from stored configuration
	req := experience.BuildRequest{
		CoreIdentity:        stored.CoreIdentity,
		PersonaModules:      stored.PersonaModules,
		RelationshipProfile: stored.RelationshipProfile,
		OutputPreferences:   stored.OutputPreferences,
	}
	rendered, err := s.cfg.ExperienceBuilder(r.Context(), string(mode), req)
	if err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", "failed to build effective state: "+err.Error(), nil)
		return
	}

	// Record an on-demand inspection snapshot
	_ = experience.RecordOwnerInspectionSnapshot(r.Context(), s.cfg.DB, rendered, ownerID, string(mode))

	// Fetch snapshot history
	history, err := store.LatestEventsByType(r.Context(), s.cfg.DB, schema.FactNaviExperienceSnapshot, 10)
	if err != nil {
		// Log but don't fail the whole request
		history = []schema.Event{}
	}

	replyJSON(w, http.StatusOK, experienceInspectResponse{
		OwnerID:         ownerID,
		StoredConfig:    stored,
		EffectiveState:  rendered.State,
		CompiledPayload: rendered.Payload,
		SnapshotHistory: history,
	})
}

func (s *Server) handlePutExperienceCoreIdentity(w http.ResponseWriter, r *http.Request) {
	if !s.requireOwnerAuthority(w, r) {
		return
	}
	ownerID, ok := s.resolveExperienceOwnerID(w, r)
	if !ok {
		return
	}
	var req putExperienceCoreIdentityRequest
	if err := decodeGatewayJSONBody(r, &req); err != nil {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", nil)
		return
	}
	traits := make(map[string]float64, len(req.Traits))
	for rawTrait, value := range req.Traits {
		trait := strings.TrimSpace(rawTrait)
		if trait == "" || !experience.IsSupportedTrait(trait) {
			replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "unsupported trait: "+strings.TrimSpace(rawTrait), nil)
			return
		}
		traits[trait] = value
	}
	var err error
	if len(traits) == 0 {
		err = experience.DeleteCoreIdentityConfiguration(r.Context(), s.cfg.DB, ownerID)
	} else {
		err = experience.SaveCoreIdentityConfiguration(r.Context(), s.cfg.DB, ownerID, traits, string(schema.StateKindOwnerSet))
	}
	if err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	s.replyExperienceSnapshot(w, r, ownerID)
}

func (s *Server) handlePutExperienceOutputPreferences(w http.ResponseWriter, r *http.Request) {
	if !s.requireOwnerAuthority(w, r) {
		return
	}
	ownerID, ok := s.resolveExperienceOwnerID(w, r)
	if !ok {
		return
	}
	var req putExperienceOutputPreferencesRequest
	if err := decodeGatewayJSONBody(r, &req); err != nil {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", nil)
		return
	}
	if req.PreferredLength != "" && !experience.IsSupportedPreferredLength(req.PreferredLength) {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "unsupported preferred_length", nil)
		return
	}
	if req.PreferredFormat != "" && !experience.IsSupportedPreferredFormat(req.PreferredFormat) {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "unsupported preferred_format", nil)
		return
	}
	prefs := experience.OutputPreferences{
		PreferredLength: req.PreferredLength,
		PreferredFormat: req.PreferredFormat,
		SummaryFirst:    req.SummaryFirst,
	}
	var err error
	if prefs.PreferredLength == "" && prefs.PreferredFormat == "" && prefs.SummaryFirst == nil {
		err = experience.DeleteOutputPreferencesConfiguration(r.Context(), s.cfg.DB, ownerID)
	} else {
		err = experience.SaveOutputPreferencesConfiguration(r.Context(), s.cfg.DB, ownerID, prefs, string(schema.StateKindOwnerSet))
	}
	if err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	s.replyExperienceSnapshot(w, r, ownerID)
}

func (s *Server) handlePutExperienceModules(w http.ResponseWriter, r *http.Request) {
	if !s.requireOwnerAuthority(w, r) {
		return
	}
	ownerID, ok := s.resolveExperienceOwnerID(w, r)
	if !ok {
		return
	}
	var req putExperienceModulesRequest
	if err := decodeGatewayJSONBody(r, &req); err != nil {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", nil)
		return
	}
	modules := make([]experience.ModuleConfig, 0, len(req.PersonaModules))
	seen := make(map[string]struct{}, len(req.PersonaModules))
	for _, module := range req.PersonaModules {
		moduleID := strings.TrimSpace(module.ModuleID)
		if moduleID == "" {
			replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "module_id required", nil)
			return
		}
		if _, exists := seen[moduleID]; exists {
			replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "duplicate module_id: "+moduleID, nil)
			return
		}
		seen[moduleID] = struct{}{}
		module.Version = strings.TrimSpace(module.Version)
		if module.Version == "" {
			module.Version = "v1"
		}
		if module.Kind != "" && !experience.IsSupportedModuleKind(module.Kind) {
			replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "unsupported module kind", nil)
			return
		}
		if module.Scope != "" && !experience.IsSupportedModuleScope(module.Scope) {
			replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "unsupported module scope", nil)
			return
		}
		cleanTraits := make(map[string]float64, len(module.TraitContributions))
		for rawTrait, value := range module.TraitContributions {
			trait := strings.TrimSpace(rawTrait)
			if trait == "" || !experience.IsSupportedTrait(trait) {
				replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "unsupported module trait: "+strings.TrimSpace(rawTrait), nil)
				return
			}
			cleanTraits[trait] = value
		}
		module.ModuleID = moduleID
		module.TraitContributions = cleanTraits
		modules = append(modules, module)
	}
	if err := experience.ReplaceModuleConfigurations(r.Context(), s.cfg.DB, experience.ConfigScopeOwner, ownerID, modules, string(schema.StateKindOwnerSet)); err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	s.replyExperienceSnapshot(w, r, ownerID)
}

func (s *Server) requireOwnerReadAccess(w http.ResponseWriter, r *http.Request) bool {
	if OwnerIDFromCtx(r.Context()) == "local" {
		return true
	}
	owner, exists, err := store.GetOwner(r.Context(), s.cfg.DB)
	if err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", "owner lookup failed", nil)
		return false
	}
	if !exists {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "owner access required", nil)
		return false
	}
	if OwnerIDFromCtx(r.Context()) != owner.ID {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "owner access required", nil)
		return false
	}
	if !HasScope(r.Context(), "read") && !HasScope(r.Context(), "admin") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "read scope required", nil)
		return false
	}
	return true
}

func (s *Server) resolveExperienceOwnerID(w http.ResponseWriter, r *http.Request) (string, bool) {
	if s.cfg.DB == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "database not configured", nil)
		return "", false
	}
	ownerID := strings.TrimSpace(OwnerIDFromCtx(r.Context()))
	if ownerID != "" && ownerID != "local" {
		return ownerID, true
	}
	resolved, err := store.GetOwnerID(r.Context(), s.cfg.DB)
	if err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", "owner lookup failed", nil)
		return "", false
	}
	resolved = strings.TrimSpace(resolved)
	if resolved == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "owner id unavailable", nil)
		return "", false
	}
	return resolved, true
}

func (s *Server) replyExperienceSnapshot(w http.ResponseWriter, r *http.Request, ownerID string) {
	snapshot, err := experience.LoadConfigurationSnapshot(r.Context(), s.cfg.DB, ownerID)
	if err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusOK, experienceSnapshotResponse{
		OwnerID:             ownerID,
		CoreIdentity:        snapshot.CoreIdentity,
		OutputPreferences:   snapshot.OutputPreferences,
		RelationshipProfile: snapshot.RelationshipProfile,
		PersonaModules:      snapshot.PersonaModules,
	})
}

func decodeGatewayJSONBody(r *http.Request, out any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil && err != io.EOF {
		return err
	}
	return nil
}
