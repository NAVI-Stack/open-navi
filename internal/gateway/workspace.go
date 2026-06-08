package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
	"github.com/google/uuid"
)

type workspaceUpsertRequest struct {
	ID               string                 `json:"workspace_id,omitempty"`
	Name             string                 `json:"name"`
	Description      string                 `json:"description,omitempty"`
	Kind             schema.WorkspaceKind   `json:"workspace_kind"`
	Status           schema.WorkspaceStatus `json:"status,omitempty"`
	LocalRoots       []string               `json:"local_roots,omitempty"`
	RepoRoots        []string               `json:"repo_roots,omitempty"`
	ProtectedPaths   []string               `json:"protected_paths,omitempty"`
	AllowedActions   *schema.AllowedActions `json:"allowed_actions,omitempty"`
	BoundaryPolicy   *schema.BoundaryPolicy `json:"boundary_policy,omitempty"`
	AuditEnabled     *bool                  `json:"audit_enabled,omitempty"`
	Tags             []string               `json:"tags,omitempty"`
	RelatedProjectID *string                `json:"related_project_id,omitempty"`
	Notes            *string                `json:"notes,omitempty"`
	Metadata         json.RawMessage        `json:"metadata,omitempty"`
}

type workspaceModeRequest struct {
	Mode string `json:"mode"`
}

type activeWorkspaceRequest struct {
	WorkspaceID string `json:"workspace_id"`
}

type whitelistRuleRequest struct {
	RuleID      string   `json:"rule_id,omitempty"`
	Scope       string   `json:"scope"`
	ActionTypes []string `json:"action_types,omitempty"`
	ExpiresAt   string   `json:"expires_at,omitempty"`
}

type boundaryResolveRequest struct {
	WorkspaceID       string   `json:"workspace_id,omitempty"`
	Decision          string   `json:"decision"`
	TargetPath        string   `json:"target_path,omitempty"`
	Action            string   `json:"action,omitempty"`
	ActionTypes       []string `json:"action_types,omitempty"`
	SwitchWorkspaceID string   `json:"switch_workspace_id,omitempty"`
	ExpiresAt         string   `json:"expires_at,omitempty"`
	RuleScope         string   `json:"rule_scope,omitempty"`
}

type workspacePathEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func workspaceToMap(w schema.Workspace) map[string]any {
	return map[string]any{
		"workspace_id":       w.ID,
		"name":               w.Name,
		"description":        w.Description,
		"workspace_kind":     string(w.Kind),
		"status":             string(w.Status),
		"local_roots":        w.LocalRoots,
		"repo_roots":         w.RepoRoots,
		"protected_paths":    w.ProtectedPaths,
		"allowed_actions":    w.AllowedActions,
		"boundary_policy":    w.BoundaryPolicy,
		"audit_enabled":      w.AuditEnabled,
		"created_at":         w.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":         w.UpdatedAt.UTC().Format(time.RFC3339),
		"created_by":         w.CreatedBy,
		"tags":               w.Tags,
		"related_project_id": w.RelatedProjectID,
		"notes":              w.Notes,
		"whitelist_rules":    whitelistRulesToMap(w.WhitelistRules),
		"metadata":           decodeFlexibleJSON(w.Metadata),
	}
}

func whitelistRulesToMap(rules []schema.WhitelistRule) []map[string]any {
	out := make([]map[string]any, 0, len(rules))
	for _, rule := range rules {
		out = append(out, whitelistRuleToMap(rule))
	}
	return out
}

func whitelistRuleToMap(rule schema.WhitelistRule) map[string]any {
	return map[string]any{
		"rule_id":      rule.RuleID,
		"scope":        rule.Scope,
		"action_types": rule.ActionTypes,
		"status":       rule.Status,
		"created_at":   rule.CreatedAt.UTC().Format(time.RFC3339),
		"revoked_at":   formatOptionalTime(rule.RevokedAt),
		"expires_at":   formatOptionalTime(rule.ExpiresAt),
		"created_by":   rule.CreatedBy,
	}
}

func replyWorkspaceStoreValidation(w http.ResponseWriter, err error) bool {
	if !store.IsValidationError(err) {
		return false
	}
	replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", err.Error(), nil)
	return true
}

func (s *Server) requireOwnerAuthority(w http.ResponseWriter, r *http.Request) bool {
	if OwnerIDFromCtx(r.Context()) == "local" {
		return true
	}
	owner, exists, err := store.GetOwner(r.Context(), s.cfg.DB)
	if err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", "owner lookup failed", nil)
		return false
	}
	if !exists {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "owner authority required", nil)
		return false
	}
	if OwnerIDFromCtx(r.Context()) != owner.ID || !HasScope(r.Context(), "admin") {
		replyErrorAPI(w, http.StatusForbidden, "FORBIDDEN", "owner authority required", nil)
		return false
	}
	return true
}

func (s *Server) handleGetWorkspaceMode(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	activeID := s.cfg.WorldModel.GetExplicitActiveWorkspaceID(r.Context())
	resolved, source, err := s.cfg.WorldModel.ResolveActiveWorkspace(r.Context(), projectID)
	if err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	resolvedID := ""
	if resolved != nil {
		resolvedID = resolved.ID
	}
	replyJSON(w, http.StatusOK, map[string]any{
		"mode":                       s.cfg.WorldModel.GetOperatingMode(r.Context()),
		"selected_workspace_id":      activeID,
		"active_workspace_id":        resolvedID,
		"active_workspace_selection": source,
		"project_context_related_id": projectID,
	})
}

func (s *Server) handleSetWorkspaceMode(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	if !s.requireOwnerAuthority(w, r) {
		return
	}
	var req workspaceModeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", nil)
		return
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	switch mode {
	case "global", "scoped", "hybrid":
	default:
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "mode must be global, scoped, or hybrid", nil)
		return
	}
	if err := s.cfg.WorldModel.SetOperatingMode(r.Context(), mode); err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusOK, map[string]any{"mode": mode})
}

func (s *Server) handleGetActiveWorkspace(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	selectedID := s.cfg.WorldModel.GetExplicitActiveWorkspaceID(r.Context())
	resolved, source, err := s.cfg.WorldModel.ResolveActiveWorkspace(r.Context(), projectID)
	if err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	resp := map[string]any{
		"selected_workspace_id":      selectedID,
		"active_workspace_id":        "",
		"active_workspace_selection": source,
	}
	if resolved != nil {
		resp["active_workspace_id"] = resolved.ID
		resp["workspace"] = workspaceToMap(*resolved)
	}
	replyJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSetActiveWorkspace(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	var req activeWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", nil)
		return
	}
	if strings.TrimSpace(req.WorkspaceID) != "" {
		workspace, err := s.cfg.WorldModel.GetWorkspace(r.Context(), req.WorkspaceID)
		if err != nil {
			replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "workspace not found", nil)
			return
		}
		if workspace.Status != schema.WorkspaceStatusActive {
			replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "active workspace must have active status", nil)
			return
		}
	}
	if err := s.cfg.WorldModel.SetActiveWorkspaceID(r.Context(), req.WorkspaceID); err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	if strings.TrimSpace(req.WorkspaceID) != "" {
		s.recordWorkspaceHistory(r.Context(), schema.CommandTypeUpdate, req.WorkspaceID, "workspace:"+req.WorkspaceID, "workspace_event:activated", "configuration:active_workspace_id")
	}
	replyJSON(w, http.StatusOK, map[string]any{"active_workspace_id": req.WorkspaceID})
}

func (s *Server) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	workspaces, err := s.cfg.WorldModel.ListWorkspaces(r.Context(), status)
	if err != nil {
		if replyWorkspaceStoreValidation(w, err) {
			return
		}
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	items := make([]map[string]any, 0, len(workspaces))
	for _, workspace := range workspaces {
		items = append(items, workspaceToMap(workspace))
	}
	replyJSON(w, http.StatusOK, map[string]any{
		"items":               items,
		"mode":                s.cfg.WorldModel.GetOperatingMode(r.Context()),
		"active_workspace_id": s.cfg.WorldModel.GetExplicitActiveWorkspaceID(r.Context()),
	})
}

func (s *Server) handleWorkspacePathRoots(w http.ResponseWriter, r *http.Request) {
	if !s.requireOwnerAuthority(w, r) {
		return
	}
	replyJSON(w, http.StatusOK, map[string]any{
		"items":         s.workspacePathRoots(),
		"runtime_os":    runtime.GOOS,
		"in_container":  runningInContainer(),
		"data_dir":      cleanAbsPath(s.cfg.DataDir),
		"workspace_dir": cleanAbsPath(s.cfg.WorkspaceDir),
	})
}

func (s *Server) handleWorkspacePathChildren(w http.ResponseWriter, r *http.Request) {
	if !s.requireOwnerAuthority(w, r) {
		return
	}
	target := strings.TrimSpace(r.URL.Query().Get("path"))
	if target == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "path query parameter required", nil)
		return
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", err.Error(), nil)
		return
	}
	info, err := os.Stat(abs)
	if err != nil {
		status := http.StatusBadRequest
		if os.IsPermission(err) {
			status = http.StatusForbidden
		} else if os.IsNotExist(err) {
			status = http.StatusNotFound
		}
		replyErrorAPI(w, status, "PATH_UNAVAILABLE", err.Error(), nil)
		return
	}
	if !info.IsDir() {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "path must be a directory", nil)
		return
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		status := http.StatusInternalServerError
		if os.IsPermission(err) {
			status = http.StatusForbidden
		}
		replyErrorAPI(w, status, "PATH_UNAVAILABLE", err.Error(), nil)
		return
	}
	items := make([]workspacePathEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(abs, entry.Name())
		items = append(items, workspacePathEntry{Name: entry.Name(), Path: path})
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	parent := filepath.Dir(abs)
	if parent == abs {
		parent = ""
	}
	replyJSON(w, http.StatusOK, map[string]any{
		"path":   abs,
		"parent": parent,
		"items":  items,
	})
}

func (s *Server) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	s.handleUpsertWorkspace(w, r, true)
}

func (s *Server) workspacePathRoots() []workspacePathEntry {
	seen := map[string]struct{}{}
	var roots []workspacePathEntry
	add := func(label, raw string) {
		path := cleanAbsPath(raw)
		if path == "" {
			return
		}
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		name := strings.TrimSpace(label)
		if name == "" {
			name = filepath.Base(path)
		}
		if name == "." || name == string(filepath.Separator) || name == "" {
			name = path
		}
		roots = append(roots, workspacePathEntry{Name: name, Path: path})
	}

	add("workspace", s.cfg.WorkspaceDir)
	add("data", s.cfg.DataDir)
	if cwd, err := os.Getwd(); err == nil {
		add("current process", cwd)
	}
	for _, candidate := range []string{"/navi", "/navi/data", "/workspace", "/app", "/data"} {
		add(candidate, candidate)
	}
	if runtime.GOOS == "windows" {
		for drive := 'A'; drive <= 'Z'; drive++ {
			root := string(drive) + `:\`
			add(root, root)
		}
	} else {
		add("/", "/")
	}
	return roots
}

func cleanAbsPath(raw string) string {
	path := strings.TrimSpace(raw)
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return abs
}

func runningInContainer() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	data, err := os.ReadFile("/proc/1/cgroup")
	if err != nil {
		return false
	}
	text := string(data)
	return strings.Contains(text, "docker") || strings.Contains(text, "containerd") || strings.Contains(text, "kubepods")
}

func (s *Server) handleUpdateWorkspace(w http.ResponseWriter, r *http.Request) {
	s.handleUpsertWorkspace(w, r, false)
}

func (s *Server) handleUpsertWorkspace(w http.ResponseWriter, r *http.Request, create bool) {
	if s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	var req workspaceUpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", nil)
		return
	}
	now := time.Now().UTC()
	id := strings.TrimSpace(req.ID)
	if !create {
		id = strings.TrimSpace(r.PathValue("id"))
	}
	if create {
		if id == "" {
			id = uuid.New().String()
		}
	} else if id == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "workspace id required", nil)
		return
	}

	var (
		current schema.Workspace
		err     error
	)
	if !create {
		current, err = s.cfg.WorldModel.GetWorkspace(r.Context(), id)
		if err != nil {
			replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "workspace not found", nil)
			return
		}
	}
	name := strings.TrimSpace(req.Name)
	if create && name == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "workspace name required", nil)
		return
	}

	workspace := schema.Workspace{
		ID:               id,
		Name:             firstNonEmptyGateway(name, current.Name),
		Description:      firstNonEmptyGateway(req.Description, current.Description),
		Kind:             current.Kind,
		Status:           current.Status,
		LocalRoots:       current.LocalRoots,
		RepoRoots:        current.RepoRoots,
		ProtectedPaths:   current.ProtectedPaths,
		AllowedActions:   current.AllowedActions,
		BoundaryPolicy:   current.BoundaryPolicy,
		AuditEnabled:     current.AuditEnabled,
		CreatedAt:        current.CreatedAt,
		UpdatedAt:        now,
		CreatedBy:        current.CreatedBy,
		Tags:             current.Tags,
		RelatedProjectID: current.RelatedProjectID,
		Notes:            current.Notes,
		WhitelistRules:   current.WhitelistRules,
		Metadata:         current.Metadata,
	}
	if create {
		workspace.CreatedAt = now
		workspace.CreatedBy = firstNonEmptyGateway(OwnerIDFromCtx(r.Context()), "owner")
		workspace.Status = schema.WorkspaceStatusActive
		workspace.Kind = schema.WorkspaceKindGeneral
		workspace.AllowedActions = schema.AllowedActions{
			Read: true, Write: true, Create: true, Modify: true, RenameMove: true, Delete: false, Execute: false,
		}
		workspace.BoundaryPolicy = schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt}
	}
	if req.Kind != "" {
		workspace.Kind = req.Kind
	}
	if req.Status != "" {
		workspace.Status = req.Status
	}
	if req.LocalRoots != nil {
		workspace.LocalRoots = req.LocalRoots
	}
	if req.RepoRoots != nil {
		workspace.RepoRoots = req.RepoRoots
	}
	if req.ProtectedPaths != nil {
		workspace.ProtectedPaths = req.ProtectedPaths
	}
	if req.AllowedActions != nil {
		workspace.AllowedActions = *req.AllowedActions
	}
	if req.BoundaryPolicy != nil {
		workspace.BoundaryPolicy = *req.BoundaryPolicy
	}
	if req.AuditEnabled != nil {
		workspace.AuditEnabled = *req.AuditEnabled
	}
	if req.Tags != nil {
		workspace.Tags = req.Tags
	}
	if req.RelatedProjectID != nil {
		workspace.RelatedProjectID = strings.TrimSpace(*req.RelatedProjectID)
	}
	if req.Notes != nil {
		workspace.Notes = strings.TrimSpace(*req.Notes)
	}
	if len(req.Metadata) > 0 {
		workspace.Metadata = string(req.Metadata)
	}
	if strings.TrimSpace(workspace.Name) == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "workspace name required", nil)
		return
	}
	persisted, err := s.cfg.WorldModel.UpsertWorkspace(r.Context(), workspace)
	if err != nil {
		if replyWorkspaceStoreValidation(w, err) {
			return
		}
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	code := http.StatusOK
	if create {
		code = http.StatusCreated
	}
	commandType := schema.CommandTypeUpdate
	eventRef := "workspace_event:updated"
	if create {
		commandType = schema.CommandTypeCreate
		eventRef = "workspace_event:created"
	}
	s.recordWorkspaceHistory(r.Context(), commandType, persisted.ID, "workspace:"+persisted.ID, eventRef)
	replyJSON(w, code, workspaceToMap(persisted))
}

func (s *Server) handleGetWorkspace(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "workspace id required", nil)
		return
	}
	workspace, err := s.cfg.WorldModel.GetWorkspace(r.Context(), id)
	if err != nil {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "workspace not found", nil)
		return
	}
	replyJSON(w, http.StatusOK, workspaceToMap(workspace))
}

func (s *Server) handleArchiveWorkspace(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "workspace id required", nil)
		return
	}
	persisted, err := s.cfg.WorldModel.ArchiveWorkspace(r.Context(), id)
	if err != nil {
		if replyWorkspaceStoreValidation(w, err) {
			return
		}
		if strings.Contains(err.Error(), "workspace not found") {
			replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "workspace not found", nil)
			return
		}
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	s.recordWorkspaceHistory(r.Context(), schema.CommandTypeUpdate, persisted.ID, "workspace:"+persisted.ID, "workspace_event:archived")
	replyJSON(w, http.StatusOK, workspaceToMap(persisted))
}

func (s *Server) handleDeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	if !s.requireOwnerAuthority(w, r) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "workspace id required", nil)
		return
	}
	if activeID := strings.TrimSpace(s.cfg.WorldModel.GetExplicitActiveWorkspaceID(r.Context())); activeID == id {
		if err := s.cfg.WorldModel.SetActiveWorkspaceID(r.Context(), ""); err != nil {
			replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
			return
		}
	}
	if err := s.cfg.WorldModel.DeleteWorkspace(r.Context(), id); err != nil {
		if replyWorkspaceStoreValidation(w, err) {
			return
		}
		if strings.Contains(err.Error(), "workspace not found") {
			replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "workspace not found", nil)
			return
		}
		if strings.Contains(strings.ToLower(err.Error()), "foreign key") {
			replyErrorAPI(w, http.StatusConflict, "CONFLICT", "workspace is still referenced; archive it or remove project/resource bindings before deleting", nil)
			return
		}
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	s.recordWorkspaceHistory(r.Context(), schema.CommandTypeDelete, id, "workspace:"+id, "workspace_event:deleted")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetWorkspaceByProject(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	projectID := strings.TrimSpace(r.PathValue("projectID"))
	if projectID == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "project id required", nil)
		return
	}
	workspace, err := s.cfg.WorldModel.GetWorkspaceByProjectID(r.Context(), projectID)
	if err != nil {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "workspace binding not found", nil)
		return
	}
	replyJSON(w, http.StatusOK, workspaceToMap(workspace))
}

func (s *Server) handleBindWorkspaceProject(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "workspace id required", nil)
		return
	}
	var req struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", nil)
		return
	}
	workspace, err := s.cfg.WorldModel.GetWorkspace(r.Context(), id)
	if err != nil {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "workspace not found", nil)
		return
	}
	workspace.RelatedProjectID = strings.TrimSpace(req.ProjectID)
	workspace.UpdatedAt = time.Now().UTC()
	persisted, err := s.cfg.WorldModel.UpsertWorkspace(r.Context(), workspace)
	if err != nil {
		if replyWorkspaceStoreValidation(w, err) {
			return
		}
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	affected := []string{"workspace:" + persisted.ID, "workspace_event:updated"}
	if strings.TrimSpace(req.ProjectID) != "" {
		affected = append(affected, "project:"+strings.TrimSpace(req.ProjectID))
	}
	s.recordWorkspaceHistory(r.Context(), schema.CommandTypeUpdate, persisted.ID, affected...)
	replyJSON(w, http.StatusOK, workspaceToMap(persisted))
}

func (s *Server) handleListWhitelistRules(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "workspace id required", nil)
		return
	}
	rules, err := s.cfg.WorldModel.ListWhitelistRules(r.Context(), id)
	if err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusOK, map[string]any{"items": whitelistRulesToMap(rules)})
}

func (s *Server) handleCreateWhitelistRule(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	workspaceID := strings.TrimSpace(r.PathValue("id"))
	if workspaceID == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "workspace id required", nil)
		return
	}
	if _, err := s.cfg.WorldModel.GetWorkspace(r.Context(), workspaceID); err != nil {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "workspace not found", nil)
		return
	}
	var req whitelistRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", nil)
		return
	}
	if strings.TrimSpace(req.Scope) == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "scope required", nil)
		return
	}
	now := time.Now().UTC()
	rule := schema.WhitelistRule{
		RuleID:      firstNonEmptyGateway(strings.TrimSpace(req.RuleID), uuid.New().String()),
		Scope:       strings.TrimSpace(req.Scope),
		ActionTypes: normalizeActionTypes(req.ActionTypes),
		Status:      schema.WhitelistRuleStatusActive,
		CreatedAt:   now,
		CreatedBy:   firstNonEmptyGateway(OwnerIDFromCtx(r.Context()), "owner"),
	}
	if strings.TrimSpace(req.ExpiresAt) != "" {
		expiresAt, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "expires_at must be RFC3339", nil)
			return
		}
		rule.ExpiresAt = &expiresAt
	}
	if err := s.cfg.WorldModel.UpsertWhitelistRule(r.Context(), rule, workspaceID); err != nil {
		if replyWorkspaceStoreValidation(w, err) {
			return
		}
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusCreated, whitelistRuleToMap(rule))
}

func (s *Server) handleRevokeWhitelistRule(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	workspaceID := strings.TrimSpace(r.PathValue("id"))
	ruleID := strings.TrimSpace(r.PathValue("ruleID"))
	if workspaceID == "" || ruleID == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "workspace id and rule id required", nil)
		return
	}
	rules, err := s.cfg.WorldModel.ListWhitelistRules(r.Context(), workspaceID)
	if err != nil {
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	var target *schema.WhitelistRule
	for i := range rules {
		if rules[i].RuleID == ruleID {
			target = &rules[i]
			break
		}
	}
	if target == nil {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "whitelist rule not found", nil)
		return
	}
	now := time.Now().UTC()
	target.Status = schema.WhitelistRuleStatusRevoked
	target.RevokedAt = &now
	if err := s.cfg.WorldModel.UpsertWhitelistRule(r.Context(), *target, workspaceID); err != nil {
		if replyWorkspaceStoreValidation(w, err) {
			return
		}
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusOK, whitelistRuleToMap(*target))
}

func (s *Server) handleResolveWorkspaceBoundary(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	var req boundaryResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", nil)
		return
	}
	workspaceID := strings.TrimSpace(req.WorkspaceID)
	if workspaceID == "" {
		workspaceID = s.cfg.WorldModel.GetActiveWorkspaceID(r.Context())
	}
	if workspaceID == "" && strings.TrimSpace(req.Decision) != string(schema.ApprovalOutcomeSwitchedWorkspace) {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "workspace id required", nil)
		return
	}
	decision := strings.TrimSpace(req.Decision)
	switch decision {
	case string(schema.ApprovalOutcomeDenied):
		replyJSON(w, http.StatusOK, map[string]any{
			"approval_outcome": string(schema.ApprovalOutcomeDenied),
			"workspace_id":     workspaceID,
			"target_path":      req.TargetPath,
			"action":           req.Action,
		})
		return
	case string(schema.ApprovalOutcomeAllowOnce):
		replyJSON(w, http.StatusOK, map[string]any{
			"approval_outcome": string(schema.ApprovalOutcomeAllowOnce),
			"workspace_id":     workspaceID,
			"target_path":      req.TargetPath,
			"action":           req.Action,
		})
		return
	case string(schema.ApprovalOutcomeAlwaysAllow):
		ruleReq := whitelistRuleRequest{
			Scope:       firstNonEmptyGateway(strings.TrimSpace(req.RuleScope), strings.TrimSpace(req.TargetPath)),
			ActionTypes: normalizeActionTypes(req.ActionTypes),
			ExpiresAt:   req.ExpiresAt,
		}
		if ruleReq.Scope == "" {
			replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "target_path or rule_scope required for always_allow", nil)
			return
		}
		body, _ := json.Marshal(ruleReq)
		r.Body = ioNopCloser(body)
		r.SetPathValue("id", workspaceID)
		s.handleCreateWhitelistRule(w, r)
		return
	case string(schema.ApprovalOutcomeSwitchedWorkspace):
		targetWorkspace := strings.TrimSpace(req.SwitchWorkspaceID)
		if targetWorkspace == "" {
			replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "switch_workspace_id required", nil)
			return
		}
		if _, err := s.cfg.WorldModel.GetWorkspace(r.Context(), targetWorkspace); err != nil {
			replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "switch target workspace not found", nil)
			return
		}
		if err := s.cfg.WorldModel.SetActiveWorkspaceID(r.Context(), targetWorkspace); err != nil {
			replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
			return
		}
		replyJSON(w, http.StatusOK, map[string]any{
			"approval_outcome":    string(schema.ApprovalOutcomeSwitchedWorkspace),
			"active_workspace_id": targetWorkspace,
			"target_path":         req.TargetPath,
			"action":              req.Action,
		})
		return
	default:
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "decision must be denied, allow_once, always_allow, or switched_workspace", nil)
		return
	}
}

func normalizeActionTypes(actionTypes []string) []string {
	if len(actionTypes) == 0 {
		return []string{"all"}
	}
	out := make([]string, 0, len(actionTypes))
	for _, actionType := range actionTypes {
		actionType = strings.TrimSpace(strings.ToLower(actionType))
		if actionType == "" {
			continue
		}
		out = append(out, actionType)
	}
	if len(out) == 0 {
		return []string{"all"}
	}
	return out
}

type staticReadCloser struct {
	*strings.Reader
}

func (staticReadCloser) Close() error { return nil }

func ioNopCloser(body []byte) staticReadCloser {
	return staticReadCloser{Reader: strings.NewReader(string(body))}
}

func (s *Server) recordWorkspaceHistory(ctx context.Context, commandType schema.CommandType, workspaceID string, affectedRefs ...string) {
	if s == nil || s.cfg.DB == nil {
		return
	}
	now := time.Now().UTC()
	commandID := uuid.New().String()
	affectedEntities := "[]"
	if len(affectedRefs) > 0 {
		if payload, err := json.Marshal(affectedRefs); err == nil {
			affectedEntities = string(payload)
		}
	}
	_ = store.SaveExecutionOutcome(ctx, s.cfg.DB, schema.ExecutionOutcome{
		AttemptID:          commandID + ":1",
		CommandID:          commandID,
		AttemptNumber:      1,
		CommandType:        commandType,
		StartTime:          now,
		EndTime:            &now,
		Outcome:            schema.ExecutionOutcomeSucceeded,
		AffectedEntities:   affectedEntities,
		CompensationStatus: schema.CompensationStatusNotRequired,
		RecoveryStatus:     schema.RecoveryStatusNotRequired,
		WorkspaceID:        strings.TrimSpace(workspaceID),
		ApprovalOutcome:    schema.ApprovalOutcomeNA,
	})
}
