package gateway

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/open-navi/navi/internal/schema"
)

type sandboxProfileUpsertRequest struct {
	ID                   string                      `json:"sandbox_profile_id,omitempty"`
	Name                 string                      `json:"name"`
	Description          string                      `json:"description,omitempty"`
	Runtime              schema.SandboxRuntime       `json:"runtime,omitempty"`
	Status               schema.SandboxProfileStatus `json:"status,omitempty"`
	Image                string                      `json:"image"`
	NetworkMode          schema.SandboxNetworkMode   `json:"network_mode,omitempty"`
	WorkspaceMountTarget string                      `json:"workspace_mount_target,omitempty"`
	CommandAllowlist     []string                    `json:"command_allowlist,omitempty"`
	EnvAllowlist         []string                    `json:"env_allowlist,omitempty"`
	DefaultTimeoutMS     int                         `json:"default_timeout_ms,omitempty"`
	MaxTimeoutMS         int                         `json:"max_timeout_ms,omitempty"`
	CPULimit             string                      `json:"cpu_limit,omitempty"`
	MemoryLimit          string                      `json:"memory_limit,omitempty"`
	Metadata             json.RawMessage             `json:"metadata,omitempty"`
}

func sandboxProfileToMap(p schema.SandboxProfile) map[string]any {
	return map[string]any{
		"sandbox_profile_id":     p.ID,
		"name":                   p.Name,
		"description":            p.Description,
		"runtime":                string(p.Runtime),
		"status":                 string(p.Status),
		"image":                  p.Image,
		"network_mode":           string(p.NetworkMode),
		"workspace_mount_target": p.WorkspaceMountTarget,
		"command_allowlist":      p.CommandAllowlist,
		"env_allowlist":          p.EnvAllowlist,
		"default_timeout_ms":     p.DefaultTimeoutMS,
		"max_timeout_ms":         p.MaxTimeoutMS,
		"cpu_limit":              p.CPULimit,
		"memory_limit":           p.MemoryLimit,
		"created_at":             p.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":             p.UpdatedAt.UTC().Format(time.RFC3339),
		"created_by":             p.CreatedBy,
		"metadata":               decodeFlexibleJSON(p.Metadata),
	}
}

func (s *Server) handleListSandboxProfiles(w http.ResponseWriter, r *http.Request) {
	wm := s.projectWorldModel()
	if wm == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	var status schema.SandboxProfileStatus
	if raw := strings.TrimSpace(r.URL.Query().Get("status")); raw != "" {
		parsed, err := schema.ParseSandboxProfileStatus(raw)
		if err != nil {
			replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "invalid sandbox profile status", nil)
			return
		}
		status = parsed
	}
	profiles, err := wm.ListSandboxProfiles(r.Context(), status)
	if err != nil {
		if replyProjectStoreValidation(w, err) {
			return
		}
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	items := make([]map[string]any, 0, len(profiles))
	for _, profile := range profiles {
		items = append(items, sandboxProfileToMap(profile))
	}
	replyJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleCreateSandboxProfile(w http.ResponseWriter, r *http.Request) {
	wm := s.projectWorldModel()
	if wm == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	if !s.requireOwnerAuthority(w, r) {
		return
	}
	profile, err := sandboxProfileFromRequest(r)
	if err != nil {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", err.Error(), nil)
		return
	}
	persisted, err := wm.UpsertSandboxProfile(r.Context(), profile)
	if err != nil {
		if replyProjectStoreValidation(w, err) {
			return
		}
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusCreated, sandboxProfileToMap(persisted))
}

func (s *Server) handleGetSandboxProfile(w http.ResponseWriter, r *http.Request) {
	wm := s.projectWorldModel()
	if wm == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	profile, err := wm.GetSandboxProfile(r.Context(), id)
	if err != nil {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "sandbox profile not found", nil)
		return
	}
	replyJSON(w, http.StatusOK, sandboxProfileToMap(profile))
}

func sandboxProfileFromRequest(r *http.Request) (schema.SandboxProfile, error) {
	var req sandboxProfileUpsertRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return schema.SandboxProfile{}, err
	}
	now := time.Now().UTC()
	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = uuid.NewString()
	}
	metadata := "{}"
	if len(req.Metadata) > 0 {
		if !json.Valid(req.Metadata) {
			return schema.SandboxProfile{}, errProjectBadRequest("metadata must be valid JSON")
		}
		metadata = string(req.Metadata)
	}
	return schema.SandboxProfile{
		ID:                   id,
		Name:                 strings.TrimSpace(req.Name),
		Description:          strings.TrimSpace(req.Description),
		Runtime:              req.Runtime,
		Status:               req.Status,
		Image:                strings.TrimSpace(req.Image),
		NetworkMode:          req.NetworkMode,
		WorkspaceMountTarget: strings.TrimSpace(req.WorkspaceMountTarget),
		CommandAllowlist:     req.CommandAllowlist,
		EnvAllowlist:         req.EnvAllowlist,
		DefaultTimeoutMS:     req.DefaultTimeoutMS,
		MaxTimeoutMS:         req.MaxTimeoutMS,
		CPULimit:             strings.TrimSpace(req.CPULimit),
		MemoryLimit:          strings.TrimSpace(req.MemoryLimit),
		CreatedAt:            now,
		UpdatedAt:            now,
		CreatedBy:            firstNonEmptyGateway(OwnerIDFromCtx(r.Context()), "owner"),
		Metadata:             metadata,
	}, nil
}
