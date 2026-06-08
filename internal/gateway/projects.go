package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/navi"
	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
	"github.com/ceoai/navi/internal/worldmodel"
	"github.com/google/uuid"
)

type projectUpsertRequest struct {
	ID               string               `json:"project_id,omitempty"`
	Title            string               `json:"title"`
	Slug             *string              `json:"slug,omitempty"`
	Description      *string              `json:"description,omitempty"`
	Kind             schema.ProjectKind   `json:"project_kind,omitempty"`
	Status           schema.ProjectStatus `json:"status,omitempty"`
	Health           schema.ProjectHealth `json:"health,omitempty"`
	WorkspaceID      *string              `json:"workspace_id,omitempty"`
	SandboxProfileID *string              `json:"sandbox_profile_id,omitempty"`
	Icon             *string              `json:"icon,omitempty"`
	Color            *string              `json:"color,omitempty"`
	MemoryScope      *string              `json:"memory_scope,omitempty"`
	Attributes       json.RawMessage      `json:"attributes,omitempty"`
}

type projectWorkspaceBindingRequest struct {
	WorkspaceID string `json:"workspace_id"`
}

type projectTaskCreateRequest struct {
	TaskID           string `json:"task_id,omitempty"`
	Title            string `json:"title"`
	Description      string `json:"description,omitempty"`
	RawInput         string `json:"raw_input"`
	TaskClass        string `json:"task_class"`
	AssignedTo       string `json:"assigned_to,omitempty"`
	Risk             string `json:"risk,omitempty"`
	AcceptanceTarget string `json:"acceptance_target,omitempty"`
}

func projectToMap(p schema.Project) map[string]any {
	return map[string]any{
		"project_id":         p.ID,
		"title":              p.Title,
		"slug":               p.Slug,
		"description":        p.Description,
		"project_kind":       string(p.Kind),
		"status":             string(p.Status),
		"health":             string(p.Health),
		"workspace_id":       p.WorkspaceID,
		"sandbox_profile_id": p.SandboxProfileID,
		"icon":               p.Icon,
		"color":              p.Color,
		"memory_scope":       string(p.MemoryScope),
		"created_at":         p.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":         p.UpdatedAt.UTC().Format(time.RFC3339),
		"created_by":         p.CreatedBy,
		"attributes":         p.Attributes,
	}
}

func projectTaskToMap(t schema.Task) map[string]any {
	return map[string]any{
		"task_id":           t.ID,
		"id":                t.ID,
		"title":             t.Title,
		"description":       t.Description,
		"status":            string(t.Status),
		"risk":              string(t.Risk),
		"assigned_to":       string(t.AssignedTo),
		"directive_id":      t.DirectiveID,
		"project_id":        t.ProjectID,
		"workspace_id":      t.WorkspaceID,
		"task_class":        string(t.TaskClass),
		"raw_input":         t.RawInput,
		"acceptance_target": t.AcceptanceTarget,
		"lifecycle_phase":   t.LifecyclePhase,
		"block_reason":      t.BlockReason,
		"created_at":        t.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":        t.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func projectReadinessToMap(r schema.ProjectReadiness) map[string]any {
	reasons := r.DegradedReasons
	if reasons == nil {
		reasons = []string{}
	}
	return map[string]any{
		"project_id":                 r.ProjectID,
		"project_kind":               r.ProjectKind,
		"status":                     r.Status,
		"ready_for_chat":             r.ReadyForChat,
		"ready_for_planning":         r.ReadyForPlanning,
		"ready_for_coding":           r.ReadyForCoding,
		"workspace_binding_required": r.WorkspaceBindingRequired,
		"workspace_id":               r.WorkspaceID,
		"sandbox_profile_id":         r.SandboxProfileID,
		"degraded_reasons":           reasons,
	}
}

func replyProjectStoreValidation(w http.ResponseWriter, err error) bool {
	if !store.IsValidationError(err) {
		return false
	}
	replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", err.Error(), nil)
	return true
}

func (s *Server) projectWorldModel() *worldmodel.WorldModel {
	if s.cfg.WorldModel != nil {
		return s.cfg.WorldModel
	}
	if s.cfg.DB == nil {
		return nil
	}
	return worldmodel.New(s.cfg.DB)
}

func (s *Server) requireProjectContext(w http.ResponseWriter, r *http.Request, projectID string) (schema.Project, bool) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "project id required", nil)
		return schema.Project{}, false
	}
	wm := s.projectWorldModel()
	if wm == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return schema.Project{}, false
	}
	project, err := wm.GetProject(r.Context(), projectID)
	if err != nil {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "project not found", nil)
		return schema.Project{}, false
	}
	return project, true
}

func (s *Server) setActiveProjectContext(ctx context.Context, projectID string) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil
	}
	wm := s.projectWorldModel()
	if wm == nil {
		return nil
	}
	return wm.SetActiveProjectID(ctx, projectID)
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	var status schema.ProjectStatus
	statusRaw := strings.TrimSpace(r.URL.Query().Get("status"))
	if statusRaw != "" {
		parsed, err := schema.ParseProjectStatus(statusRaw)
		if err != nil {
			replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "invalid project status", nil)
			return
		}
		status = parsed
	}
	includeArchived := parseProjectBool(r.URL.Query().Get("include_archived"))
	projects, err := s.cfg.WorldModel.ListProjects(r.Context(), status, includeArchived)
	if err != nil {
		if replyProjectStoreValidation(w, err) {
			return
		}
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	items := make([]map[string]any, 0, len(projects))
	for _, project := range projects {
		items = append(items, projectToMap(project))
	}
	replyJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	if !s.requireOwnerAuthority(w, r) {
		return
	}
	project, err := s.projectFromRequest(r, true, schema.Project{})
	if err != nil {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", err.Error(), nil)
		return
	}
	persisted, err := s.cfg.WorldModel.UpsertProject(r.Context(), project)
	if err != nil {
		if replyProjectStoreValidation(w, err) {
			return
		}
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusCreated, projectToMap(persisted))
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "project id required", nil)
		return
	}
	project, err := s.cfg.WorldModel.GetProject(r.Context(), id)
	if err != nil {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "project not found", nil)
		return
	}
	replyJSON(w, http.StatusOK, projectToMap(project))
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	if !s.requireOwnerAuthority(w, r) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "project id required", nil)
		return
	}
	current, err := s.cfg.WorldModel.GetProject(r.Context(), id)
	if err != nil {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "project not found", nil)
		return
	}
	project, err := s.projectFromRequest(r, false, current)
	if err != nil {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", err.Error(), nil)
		return
	}
	persisted, err := s.cfg.WorldModel.UpsertProject(r.Context(), project)
	if err != nil {
		if replyProjectStoreValidation(w, err) {
			return
		}
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusOK, projectToMap(persisted))
}

func (s *Server) handleArchiveProject(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	if !s.requireOwnerAuthority(w, r) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "project id required", nil)
		return
	}
	persisted, err := s.cfg.WorldModel.ArchiveProject(r.Context(), id)
	if err != nil {
		if replyProjectStoreValidation(w, err) {
			return
		}
		if strings.Contains(err.Error(), "project not found") {
			replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "project not found", nil)
			return
		}
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusOK, projectToMap(persisted))
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	if !s.requireOwnerAuthority(w, r) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "project id required", nil)
		return
	}
	err := s.cfg.WorldModel.DeleteProject(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "project not found") {
			replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "project not found", nil)
			return
		}
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusOK, map[string]any{"status": "deleted", "project_id": id})
}

func (s *Server) handleGetProjectReadiness(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "project id required", nil)
		return
	}
	readiness, err := s.cfg.WorldModel.ProjectReadiness(r.Context(), id)
	if err != nil {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "project not found", nil)
		return
	}
	replyJSON(w, http.StatusOK, projectReadinessToMap(readiness))
}

func (s *Server) handleBindProjectWorkspace(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	if !s.requireOwnerAuthority(w, r) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "project id required", nil)
		return
	}
	var req projectWorkspaceBindingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", nil)
		return
	}
	workspaceID := strings.TrimSpace(req.WorkspaceID)
	if workspaceID == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "workspace id required", nil)
		return
	}
	persisted, err := s.cfg.WorldModel.BindProjectWorkspace(r.Context(), id, workspaceID)
	if err != nil {
		if replyProjectStoreValidation(w, err) {
			return
		}
		if strings.Contains(err.Error(), "project not found") {
			replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "project not found", nil)
			return
		}
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusOK, projectToMap(persisted))
}

func (s *Server) handleUnbindProjectWorkspace(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	if !s.requireOwnerAuthority(w, r) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "project id required", nil)
		return
	}
	persisted, err := s.cfg.WorldModel.UnbindProjectWorkspace(r.Context(), id)
	if err != nil {
		if replyProjectStoreValidation(w, err) {
			return
		}
		if strings.Contains(err.Error(), "project not found") {
			replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "project not found", nil)
			return
		}
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusOK, projectToMap(persisted))
}

func (s *Server) handleGetProjectWorkspace(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorldModel == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", "project id required", nil)
		return
	}
	workspace, err := s.cfg.WorldModel.GetProjectWorkspace(r.Context(), id)
	if err != nil {
		replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "workspace binding not found", nil)
		return
	}
	replyJSON(w, http.StatusOK, workspaceToMap(workspace))
}

func (s *Server) handleListProjectChats(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Navi == nil {
		replyError(w, http.StatusServiceUnavailable, "NAVI not enabled")
		return
	}
	projectID := strings.TrimSpace(r.PathValue("id"))
	project, ok := s.requireProjectContext(w, r, projectID)
	if !ok {
		return
	}
	chats, err := s.cfg.Navi.RecentProjectChats(r.Context(), project.ID, 50)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if chats == nil {
		chats = []navi.Chat{}
	}
	replyJSON(w, http.StatusOK, map[string]any{
		"project_id": project.ID,
		"items":      chats,
	})
}

func (s *Server) handleCreateProjectChat(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Navi == nil {
		replyError(w, http.StatusServiceUnavailable, "NAVI not enabled")
		return
	}
	projectID := strings.TrimSpace(r.PathValue("id"))
	project, ok := s.requireProjectContext(w, r, projectID)
	if !ok {
		return
	}
	if err := decodeEmptyProjectChatBody(r); err != nil {
		replyError(w, http.StatusBadRequest, err.Error())
		return
	}
	chatID, err := s.cfg.Navi.CreateChat(r.Context(), "standard", project.ID)
	if err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.setActiveProjectContext(r.Context(), project.ID); err != nil {
		replyError(w, http.StatusInternalServerError, err.Error())
		return
	}
	replyJSON(w, http.StatusCreated, map[string]string{
		"id":         chatID,
		"chat_id":    chatID,
		"project_id": project.ID,
	})
}

func (s *Server) handleListProjectTasks(w http.ResponseWriter, r *http.Request) {
	wm := s.projectWorldModel()
	if wm == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	projectID := strings.TrimSpace(r.PathValue("id"))
	tasks, err := wm.ListProjectTasks(r.Context(), projectID)
	if err != nil {
		if strings.Contains(err.Error(), "project not found") {
			replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "project not found", nil)
			return
		}
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	items := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, projectTaskToMap(task))
	}
	replyJSON(w, http.StatusOK, map[string]any{
		"project_id": projectID,
		"items":      items,
	})
}

func (s *Server) handleCreateProjectTask(w http.ResponseWriter, r *http.Request) {
	wm := s.projectWorldModel()
	if wm == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	if !s.requireOwnerAuthority(w, r) {
		return
	}
	projectID := strings.TrimSpace(r.PathValue("id"))
	task, err := projectTaskFromRequest(r)
	if err != nil {
		replyErrorAPI(w, http.StatusBadRequest, "BAD_REQUEST", err.Error(), nil)
		return
	}
	persisted, err := wm.CreateProjectTask(r.Context(), projectID, task)
	if err != nil {
		if replyProjectStoreValidation(w, err) {
			return
		}
		if strings.Contains(err.Error(), "project not found") {
			replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "project not found", nil)
			return
		}
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusCreated, projectTaskToMap(persisted))
}

func (s *Server) handleGetProjectTask(w http.ResponseWriter, r *http.Request) {
	wm := s.projectWorldModel()
	if wm == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	projectID := strings.TrimSpace(r.PathValue("id"))
	taskID := strings.TrimSpace(r.PathValue("taskID"))
	task, err := wm.GetProjectTask(r.Context(), projectID, taskID)
	if err != nil {
		if strings.Contains(err.Error(), "project not found") {
			replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "project not found", nil)
			return
		}
		if strings.Contains(err.Error(), "project task not found") {
			replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "task not found", nil)
			return
		}
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusOK, projectTaskToMap(task))
}

func (s *Server) handleCancelProjectTask(w http.ResponseWriter, r *http.Request) {
	wm := s.projectWorldModel()
	if wm == nil {
		replyErrorAPI(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "world model not configured", nil)
		return
	}
	if !s.requireOwnerAuthority(w, r) {
		return
	}
	projectID := strings.TrimSpace(r.PathValue("id"))
	taskID := strings.TrimSpace(r.PathValue("taskID"))
	task, err := wm.CancelProjectTask(r.Context(), projectID, taskID)
	if err != nil {
		if strings.Contains(err.Error(), "project not found") {
			replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "project not found", nil)
			return
		}
		if strings.Contains(err.Error(), "task not found") {
			replyErrorAPI(w, http.StatusNotFound, "NOT_FOUND", "task not found", nil)
			return
		}
		replyErrorAPI(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil)
		return
	}
	replyJSON(w, http.StatusOK, projectTaskToMap(task))
}

func (s *Server) projectFromRequest(r *http.Request, create bool, current schema.Project) (schema.Project, error) {
	var req projectUpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return schema.Project{}, err
	}
	now := time.Now().UTC()
	id := strings.TrimSpace(req.ID)
	if !create {
		id = current.ID
	} else if id == "" {
		id = uuid.NewString()
	}
	if id == "" {
		return schema.Project{}, errProjectBadRequest("project id required")
	}
	title := strings.TrimSpace(req.Title)
	if !create && title == "" {
		title = current.Title
	}
	if create && title == "" {
		return schema.Project{}, errProjectBadRequest("project title required")
	}
	project := schema.Project{
		ID:               id,
		Title:            title,
		Slug:             current.Slug,
		Description:      current.Description,
		Kind:             current.Kind,
		Status:           current.Status,
		Health:           current.Health,
		WorkspaceID:      current.WorkspaceID,
		SandboxProfileID: current.SandboxProfileID,
		Icon:             current.Icon,
		Color:            current.Color,
		MemoryScope:      current.MemoryScope,
		CreatedAt:        current.CreatedAt,
		UpdatedAt:        now,
		CreatedBy:        current.CreatedBy,
		Attributes:       current.Attributes,
	}
	if create {
		project.CreatedAt = now
		project.CreatedBy = firstNonEmptyGateway(OwnerIDFromCtx(r.Context()), "owner")
		project.Kind = schema.ProjectKindGeneral
		project.Status = schema.ProjectStatusActive
		project.Health = schema.ProjectHealthUnknown
		project.Icon = "folder"
		project.Color = "#3b82f6"
		project.MemoryScope = schema.MemoryScopeDefault
		project.Attributes = map[string]interface{}{}
	}
	if req.Slug != nil {
		project.Slug = strings.TrimSpace(*req.Slug)
	}
	if req.Description != nil {
		project.Description = strings.TrimSpace(*req.Description)
	}
	if req.Kind != "" {
		project.Kind = req.Kind
	}
	if req.Status != "" {
		project.Status = req.Status
	}
	if req.Health != "" {
		project.Health = req.Health
	}
	if req.WorkspaceID != nil {
		project.WorkspaceID = strings.TrimSpace(*req.WorkspaceID)
	}
	if req.SandboxProfileID != nil {
		project.SandboxProfileID = strings.TrimSpace(*req.SandboxProfileID)
	}
	if req.Icon != nil {
		project.Icon = strings.TrimSpace(*req.Icon)
	}
	if req.Color != nil {
		project.Color = strings.TrimSpace(*req.Color)
	}
	if req.MemoryScope != nil {
		project.MemoryScope = schema.MemoryScope(strings.TrimSpace(*req.MemoryScope))
	}
	if len(req.Attributes) > 0 {
		var attrs map[string]interface{}
		if err := json.Unmarshal(req.Attributes, &attrs); err != nil {
			return schema.Project{}, errProjectBadRequest("attributes must be a JSON object")
		}
		if attrs == nil {
			attrs = map[string]interface{}{}
		}
		project.Attributes = attrs
	}
	return project, nil
}

func projectTaskFromRequest(r *http.Request) (schema.Task, error) {
	var req projectTaskCreateRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return schema.Task{}, err
	}
	title := strings.TrimSpace(req.Title)
	rawInput := strings.TrimSpace(req.RawInput)
	if title == "" {
		return schema.Task{}, errProjectBadRequest("task title required")
	}
	if rawInput == "" {
		return schema.Task{}, errProjectBadRequest("raw_input required")
	}
	taskClass := schema.TaskClassPlanning
	if strings.TrimSpace(req.TaskClass) != "" {
		parsed, err := schema.ParseTaskClass(req.TaskClass)
		if err != nil {
			return schema.Task{}, err
		}
		taskClass = parsed
	}
	risk := schema.RiskLow
	if strings.TrimSpace(req.Risk) != "" {
		parsed, err := parseProjectTaskRisk(req.Risk)
		if err != nil {
			return schema.Task{}, err
		}
		risk = parsed
	}
	assignedTo := schema.AgentNavi
	if strings.TrimSpace(req.AssignedTo) != "" {
		assignedTo = schema.AgentType(strings.TrimSpace(req.AssignedTo))
	}
	task := schema.NewTask(title, strings.TrimSpace(req.Description), risk, assignedTo)
	if strings.TrimSpace(req.TaskID) != "" {
		task.ID = strings.TrimSpace(req.TaskID)
	}
	task.TaskClass = taskClass
	task.RawInput = rawInput
	task.AcceptanceTarget = strings.TrimSpace(req.AcceptanceTarget)
	if task.Description == "" {
		task.Description = rawInput
	}
	return task, nil
}

func parseProjectTaskRisk(raw string) (schema.RiskLevel, error) {
	switch schema.RiskLevel(strings.ToLower(strings.TrimSpace(raw))) {
	case schema.RiskLow:
		return schema.RiskLow, nil
	case schema.RiskMedium:
		return schema.RiskMedium, nil
	case schema.RiskHigh:
		return schema.RiskHigh, nil
	case schema.RiskCritical:
		return schema.RiskCritical, nil
	default:
		return "", errProjectBadRequest("risk must be low, medium, high, or critical")
	}
}

type errProjectBadRequest string

func (e errProjectBadRequest) Error() string {
	return string(e)
}

func decodeEmptyProjectChatBody(r *http.Request) error {
	var req map[string]any
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		if err == io.EOF {
			return nil
		}
		return err
	}
	if len(req) > 0 {
		return errProjectBadRequest("project chat request body must be empty")
	}
	return nil
}

func parseProjectBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
