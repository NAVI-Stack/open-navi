package gateway

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/navi"
	navistore "github.com/open-navi/navi/internal/navi/store"
	"github.com/open-navi/navi/internal/schema"
	"github.com/open-navi/navi/internal/store"
)

func TestProjectAPI_CRUDReadinessAndWorkspaceBinding(t *testing.T) {
	srv, _, db := testServer(t)
	passport := claimTestInstance(t, srv, "Owner", "owner-secret")
	apiKey, _ := passport["primary_api_key"].(string)

	res := doJSONReq(t, srv, "GET", "/api/projects", "", "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET /api/projects without auth: expected 401, got %d", res.StatusCode)
	}

	res = doJSONReq(t, srv, "POST", "/api/projects", apiKey, "192.168.1.1:1234", map[string]any{
		"title":        "General Planning",
		"project_kind": "general",
		"attributes":   map[string]any{"lane": "planning"},
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/projects general: expected 201, got %d", res.StatusCode)
	}
	var general map[string]any
	if err := json.NewDecoder(res.Body).Decode(&general); err != nil {
		t.Fatalf("decode general project: %v", err)
	}
	generalID, _ := general["project_id"].(string)
	if generalID == "" || general["slug"] != "general-planning" {
		t.Fatalf("unexpected general project response: %#v", general)
	}

	res = doJSONReq(t, srv, "GET", "/api/projects/"+generalID+"/readiness", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET general readiness: expected 200, got %d", res.StatusCode)
	}
	var generalReady map[string]any
	if err := json.NewDecoder(res.Body).Decode(&generalReady); err != nil {
		t.Fatalf("decode general readiness: %v", err)
	}
	if generalReady["ready_for_chat"] != true || generalReady["ready_for_planning"] != true ||
		generalReady["ready_for_coding"] != false || generalReady["workspace_binding_required"] != false {
		t.Fatalf("unexpected general readiness: %#v", generalReady)
	}

	res = doJSONReq(t, srv, "POST", "/api/projects", apiKey, "192.168.1.1:1234", map[string]any{
		"title":        "Coding Slice",
		"project_kind": "coding",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/projects coding: expected 201, got %d", res.StatusCode)
	}
	var coding map[string]any
	if err := json.NewDecoder(res.Body).Decode(&coding); err != nil {
		t.Fatalf("decode coding project: %v", err)
	}
	codingID, _ := coding["project_id"].(string)
	if codingID == "" {
		t.Fatalf("coding create missing project_id")
	}

	res = doJSONReq(t, srv, "GET", "/api/projects/"+codingID+"/readiness", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET coding readiness: expected 200, got %d", res.StatusCode)
	}
	var codingReady map[string]any
	if err := json.NewDecoder(res.Body).Decode(&codingReady); err != nil {
		t.Fatalf("decode coding readiness: %v", err)
	}
	if codingReady["ready_for_coding"] != false || codingReady["workspace_binding_required"] != true {
		t.Fatalf("expected coding project without workspace to be not coding-ready, got %#v", codingReady)
	}

	activeWorkspace := gatewayProjectTestWorkspace("ws-project-active", schema.WorkspaceStatusActive, []string{"/workspace/project"})
	inactiveWorkspace := gatewayProjectTestWorkspace("ws-project-inactive", schema.WorkspaceStatusInactive, []string{"/workspace/inactive"})
	activeSandbox := gatewayProjectTestSandboxProfile("local-docker-default", schema.SandboxProfileStatusActive)
	if err := store.SaveWorkspace(context.Background(), db, activeWorkspace); err != nil {
		t.Fatalf("SaveWorkspace(active): %v", err)
	}
	if err := store.SaveWorkspace(context.Background(), db, inactiveWorkspace); err != nil {
		t.Fatalf("SaveWorkspace(inactive): %v", err)
	}
	if err := store.SaveSandboxProfile(context.Background(), db, activeSandbox); err != nil {
		t.Fatalf("SaveSandboxProfile(active): %v", err)
	}

	res = doJSONReq(t, srv, "PUT", "/api/projects/"+codingID+"/workspace-binding", apiKey, "192.168.1.1:1234", map[string]any{
		"workspace_id": "ws-missing",
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT project workspace binding missing: expected 400, got %d", res.StatusCode)
	}
	res = doJSONReq(t, srv, "PUT", "/api/projects/"+codingID+"/workspace-binding", apiKey, "192.168.1.1:1234", map[string]any{
		"workspace_id": inactiveWorkspace.ID,
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT project workspace binding inactive: expected 400, got %d", res.StatusCode)
	}
	res = doJSONReq(t, srv, "PUT", "/api/projects/"+codingID+"/workspace-binding", apiKey, "192.168.1.1:1234", map[string]any{
		"workspace_id": activeWorkspace.ID,
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT project workspace binding active: expected 200, got %d", res.StatusCode)
	}
	var bound map[string]any
	if err := json.NewDecoder(res.Body).Decode(&bound); err != nil {
		t.Fatalf("decode bound project: %v", err)
	}
	if bound["workspace_id"] != activeWorkspace.ID {
		t.Fatalf("expected workspace binding %q, got %#v", activeWorkspace.ID, bound)
	}

	res = doJSONReq(t, srv, "GET", "/api/projects/"+codingID+"/workspace", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET project workspace: expected 200, got %d", res.StatusCode)
	}
	var resolvedWorkspace map[string]any
	if err := json.NewDecoder(res.Body).Decode(&resolvedWorkspace); err != nil {
		t.Fatalf("decode project workspace: %v", err)
	}
	if resolvedWorkspace["workspace_id"] != activeWorkspace.ID {
		t.Fatalf("expected resolved workspace %q, got %#v", activeWorkspace.ID, resolvedWorkspace)
	}

	res = doJSONReq(t, srv, "GET", "/api/projects/"+codingID+"/readiness", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET bound coding readiness: expected 200, got %d", res.StatusCode)
	}
	if err := json.NewDecoder(res.Body).Decode(&codingReady); err != nil {
		t.Fatalf("decode bound coding readiness: %v", err)
	}
	if codingReady["ready_for_coding"] != false {
		t.Fatalf("expected coding project without sandbox profile to remain degraded, got %#v", codingReady)
	}

	res = doJSONReq(t, srv, "PUT", "/api/projects/"+codingID, apiKey, "192.168.1.1:1234", map[string]any{
		"title":              "Coding Slice",
		"sandbox_profile_id": activeSandbox.ID,
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/projects/{id} sandbox profile: expected 200, got %d", res.StatusCode)
	}
	res = doJSONReq(t, srv, "GET", "/api/projects/"+codingID+"/readiness", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET sandboxed coding readiness: expected 200, got %d", res.StatusCode)
	}
	if err := json.NewDecoder(res.Body).Decode(&codingReady); err != nil {
		t.Fatalf("decode sandboxed coding readiness: %v", err)
	}
	if codingReady["ready_for_coding"] != true || codingReady["sandbox_profile_id"] != activeSandbox.ID {
		t.Fatalf("expected coding project with workspace and sandbox profile to be ready, got %#v", codingReady)
	}

	res = doJSONReq(t, srv, "DELETE", "/api/projects/"+codingID+"/workspace-binding", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("DELETE project workspace binding: expected 200, got %d", res.StatusCode)
	}

	res = doJSONReq(t, srv, "PUT", "/api/projects/"+generalID, apiKey, "192.168.1.1:1234", map[string]any{
		"title":       "General Planning Updated",
		"description": "Queued after the first coding pass",
		"status":      "on_hold",
		"health":      "at_risk",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/projects/{id}: expected 200, got %d", res.StatusCode)
	}
	var updated map[string]any
	if err := json.NewDecoder(res.Body).Decode(&updated); err != nil {
		t.Fatalf("decode updated project: %v", err)
	}
	if updated["title"] != "General Planning Updated" || updated["status"] != "on_hold" || updated["health"] != "at_risk" {
		t.Fatalf("project update did not persist: %#v", updated)
	}

	res = doJSONReq(t, srv, "POST", "/api/projects/"+generalID+"/archive", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/projects/{id}/archive: expected 200, got %d", res.StatusCode)
	}
	var archived map[string]any
	if err := json.NewDecoder(res.Body).Decode(&archived); err != nil {
		t.Fatalf("decode archived project: %v", err)
	}
	if archived["status"] != "archived" {
		t.Fatalf("expected archived status, got %#v", archived)
	}

	res = doJSONReq(t, srv, "GET", "/api/projects", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/projects: expected 200, got %d", res.StatusCode)
	}
	var list map[string]any
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatalf("decode project list: %v", err)
	}
	items, _ := list["items"].([]any)
	for _, item := range items {
		project, _ := item.(map[string]any)
		if project["project_id"] == generalID {
			t.Fatalf("archived project should be hidden from default list: %#v", list)
		}
	}
}

func TestDeleteProjectClearsRuntimeSessionProjectReference(t *testing.T) {
	srv, _, db := testServerWithNavi(t)
	passport := claimTestInstance(t, srv, "Owner", "owner-secret")
	apiKey, _ := passport["primary_api_key"].(string)
	ctx := context.Background()
	now := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)

	if err := store.SaveProject(ctx, db, schema.Project{
		ID:        "proj-delete-runtime",
		Title:     "Delete Runtime Project",
		Kind:      schema.ProjectKindGeneral,
		Status:    schema.ProjectStatusActive,
		Health:    schema.ProjectHealthUnknown,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "owner",
	}); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	runtimeStore := navistore.NewSQLiteStore(db)
	if _, err := runtimeStore.CreateRuntimeSession(ctx, navi.CreateRuntimeSessionInput{
		ID:        "runtime-delete-project",
		ProjectID: "proj-delete-runtime",
	}); err != nil {
		t.Fatalf("CreateRuntimeSession: %v", err)
	}

	res := doJSONReq(t, srv, "DELETE", "/api/projects/proj-delete-runtime", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /api/projects/{id}: expected 200, got %d", res.StatusCode)
	}
	var projectID sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT project_id FROM runtime_sessions WHERE runtime_session_id = ?`, "runtime-delete-project").Scan(&projectID); err != nil {
		t.Fatalf("query runtime session project_id: %v", err)
	}
	if projectID.Valid && projectID.String != "" {
		t.Fatalf("expected runtime session project_id to be cleared, got %q", projectID.String)
	}
}

func TestProjectTaskAPI_IntakeReadinessAndCancel(t *testing.T) {
	srv, _, db := testServer(t)
	passport := claimTestInstance(t, srv, "Task Owner", "owner-secret")
	apiKey, _ := passport["primary_api_key"].(string)
	ctx := context.Background()
	now := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	if err := store.SaveProject(ctx, db, schema.Project{
		ID:        "proj-general-task",
		Title:     "General Task Project",
		Kind:      schema.ProjectKindGeneral,
		Status:    schema.ProjectStatusActive,
		Health:    schema.ProjectHealthUnknown,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "owner-1",
	}); err != nil {
		t.Fatalf("SaveProject(general): %v", err)
	}
	if err := store.SaveProject(ctx, db, schema.Project{
		ID:        "proj-coding-task",
		Title:     "Coding Task Project",
		Kind:      schema.ProjectKindCoding,
		Status:    schema.ProjectStatusActive,
		Health:    schema.ProjectHealthUnknown,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "owner-1",
	}); err != nil {
		t.Fatalf("SaveProject(coding): %v", err)
	}

	res := doJSONReq(t, srv, "POST", "/api/projects/proj-general-task/tasks", apiKey, "192.168.1.1:1234", map[string]any{
		"title":             "Plan the slice",
		"raw_input":         "Plan project task intake.",
		"task_class":        "planning",
		"assigned_to":       "navi",
		"acceptance_target": "Plan is visible",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST general project task: expected 201, got %d", res.StatusCode)
	}
	var planning map[string]any
	if err := json.NewDecoder(res.Body).Decode(&planning); err != nil {
		t.Fatalf("decode planning task: %v", err)
	}
	taskID, _ := planning["task_id"].(string)
	if taskID == "" || planning["status"] != "pending" || planning["project_id"] != "proj-general-task" {
		t.Fatalf("unexpected planning task response: %#v", planning)
	}
	if planning["raw_input"] != "Plan project task intake." || planning["acceptance_target"] != "Plan is visible" {
		t.Fatalf("intake fields were not preserved: %#v", planning)
	}

	res = doJSONReq(t, srv, "POST", "/api/projects/proj-coding-task/tasks", apiKey, "192.168.1.1:1234", map[string]any{
		"title":             "Mutate without workspace",
		"raw_input":         "Change code without a workspace.",
		"task_class":        "mutative",
		"assigned_to":       "navi",
		"acceptance_target": "Should block",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST coding mutative task without workspace: expected 201, got %d", res.StatusCode)
	}
	var blocked map[string]any
	if err := json.NewDecoder(res.Body).Decode(&blocked); err != nil {
		t.Fatalf("decode blocked task: %v", err)
	}
	if blocked["status"] != "blocked" || blocked["block_reason"] == "" {
		t.Fatalf("expected blocked coding task without workspace, got %#v", blocked)
	}

	workspace := gatewayProjectTestWorkspace("ws-task-ready", schema.WorkspaceStatusActive, []string{"/workspace/task-ready"})
	sandboxProfile := gatewayProjectTestSandboxProfile("local-docker-task", schema.SandboxProfileStatusActive)
	if err := store.SaveWorkspace(ctx, db, workspace); err != nil {
		t.Fatalf("SaveWorkspace: %v", err)
	}
	if err := store.SaveSandboxProfile(ctx, db, sandboxProfile); err != nil {
		t.Fatalf("SaveSandboxProfile: %v", err)
	}
	if _, err := srv.cfg.WorldModel.BindProjectWorkspace(ctx, "proj-coding-task", workspace.ID); err != nil {
		t.Fatalf("BindProjectWorkspace: %v", err)
	}
	project, err := srv.cfg.WorldModel.GetProject(ctx, "proj-coding-task")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	project.SandboxProfileID = sandboxProfile.ID
	if _, err := srv.cfg.WorldModel.UpsertProject(ctx, project); err != nil {
		t.Fatalf("UpsertProject(sandbox): %v", err)
	}
	res = doJSONReq(t, srv, "POST", "/api/projects/proj-coding-task/tasks", apiKey, "192.168.1.1:1234", map[string]any{
		"title":             "Mutate with workspace",
		"raw_input":         "Change code inside the bound workspace.",
		"task_class":        "mutative",
		"assigned_to":       "navi",
		"acceptance_target": "Tests pass",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST coding mutative task with workspace: expected 201, got %d", res.StatusCode)
	}
	var pending map[string]any
	if err := json.NewDecoder(res.Body).Decode(&pending); err != nil {
		t.Fatalf("decode pending task: %v", err)
	}
	if pending["status"] != "pending" || pending["workspace_id"] != workspace.ID {
		t.Fatalf("expected pending workspace-bound task, got %#v", pending)
	}

	res = doJSONReq(t, srv, "GET", "/api/projects/proj-general-task/tasks/"+taskID, apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET project task: expected 200, got %d", res.StatusCode)
	}
	res = doJSONReq(t, srv, "GET", "/api/projects/proj-general-task/tasks", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET project tasks: expected 200, got %d", res.StatusCode)
	}
	var list map[string]any
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatalf("decode task list: %v", err)
	}
	items, _ := list["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected one general project task, got %#v", list)
	}

	res = doJSONReq(t, srv, "POST", "/api/projects/proj-general-task/tasks/"+taskID+"/cancel", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST cancel project task: expected 200, got %d", res.StatusCode)
	}
	var cancelled map[string]any
	if err := json.NewDecoder(res.Body).Decode(&cancelled); err != nil {
		t.Fatalf("decode cancelled task: %v", err)
	}
	if cancelled["status"] != "cancelled" || cancelled["lifecycle_phase"] != "cancelled" {
		t.Fatalf("expected cancelled task, got %#v", cancelled)
	}
}

func TestSandboxProfileAPI_CreateListGet(t *testing.T) {
	srv, _, _ := testServer(t)
	passport := claimTestInstance(t, srv, "Sandbox Owner", "owner-secret")
	apiKey, _ := passport["primary_api_key"].(string)

	res := doJSONReq(t, srv, "POST", "/api/sandbox-profiles", apiKey, "192.168.1.1:1234", map[string]any{
		"sandbox_profile_id":     "local-docker-api",
		"name":                   "Local Docker API",
		"runtime":                "docker",
		"status":                 "active",
		"image":                  "golang:1.22",
		"network_mode":           "none",
		"workspace_mount_target": "/workspace",
		"command_allowlist":      []string{"go", "npm"},
		"env_allowlist":          []string{"CI"},
		"default_timeout_ms":     30000,
		"max_timeout_ms":         120000,
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/sandbox-profiles: expected 201, got %d", res.StatusCode)
	}
	var created map[string]any
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatalf("decode sandbox profile: %v", err)
	}
	if created["sandbox_profile_id"] != "local-docker-api" || created["network_mode"] != "none" {
		t.Fatalf("unexpected sandbox profile response: %#v", created)
	}

	res = doJSONReq(t, srv, "GET", "/api/sandbox-profiles/local-docker-api", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/sandbox-profiles/{id}: expected 200, got %d", res.StatusCode)
	}
	res = doJSONReq(t, srv, "GET", "/api/sandbox-profiles?status=active", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/sandbox-profiles: expected 200, got %d", res.StatusCode)
	}
	var list map[string]any
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatalf("decode sandbox profile list: %v", err)
	}
	items, _ := list["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected one active sandbox profile, got %#v", list)
	}
}

func gatewayProjectTestWorkspace(id string, status schema.WorkspaceStatus, roots []string) schema.Workspace {
	now := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	return schema.Workspace{
		ID:             id,
		Name:           "Workspace " + id,
		Kind:           schema.WorkspaceKindProject,
		Status:         status,
		LocalRoots:     roots,
		RepoRoots:      nil,
		ProtectedPaths: nil,
		AllowedActions: schema.AllowedActions{
			Read: true, Write: true, Create: true, Modify: true, RenameMove: true, Delete: false, Execute: false,
		},
		BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopePrompt},
		AuditEnabled:   true,
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      "owner-1",
		Metadata:       "{}",
	}
}

func gatewayProjectTestSandboxProfile(id string, status schema.SandboxProfileStatus) schema.SandboxProfile {
	now := time.Date(2026, 4, 1, 9, 30, 0, 0, time.UTC)
	return schema.SandboxProfile{
		ID:                   id,
		Name:                 "Sandbox " + id,
		Runtime:              schema.SandboxRuntimeDocker,
		Status:               status,
		Image:                "golang:1.22",
		NetworkMode:          schema.SandboxNetworkNone,
		WorkspaceMountTarget: "/workspace",
		CommandAllowlist:     []string{"go", "npm"},
		EnvAllowlist:         []string{"CI"},
		DefaultTimeoutMS:     30000,
		MaxTimeoutMS:         120000,
		CreatedAt:            now,
		UpdatedAt:            now,
		CreatedBy:            "owner-1",
		Metadata:             "{}",
	}
}
