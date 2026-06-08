package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/schema"
	"github.com/ceoai/navi/internal/store"
)

func TestWorkspaceAPI_ModeActiveCRUDWhitelistAndBoundary(t *testing.T) {
	srv, _, db := testServer(t)
	passport := claimTestInstance(t, srv, "Owner", "owner-secret")
	apiKey, _ := passport["primary_api_key"].(string)
	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	for _, projectID := range []string{"proj-alpha", "proj-bound"} {
		if err := store.SaveProject(context.Background(), db, schema.Project{
			ID:        projectID,
			Title:     "Project " + projectID,
			Kind:      schema.ProjectKindGeneral,
			Status:    schema.ProjectStatusActive,
			Health:    schema.ProjectHealthUnknown,
			CreatedAt: now,
			UpdatedAt: now,
			CreatedBy: "owner",
		}); err != nil {
			t.Fatalf("SaveProject(%s): %v", projectID, err)
		}
	}

	res := doJSONReq(t, srv, "GET", "/api/workspace-mode", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/workspace-mode: expected 200, got %d", res.StatusCode)
	}
	var modeResp map[string]any
	if err := json.NewDecoder(res.Body).Decode(&modeResp); err != nil {
		t.Fatalf("decode workspace mode: %v", err)
	}
	if modeResp["mode"] != "global" {
		t.Fatalf("expected default hybrid mode, got %v", modeResp["mode"])
	}

	res = doJSONReq(t, srv, "PUT", "/api/workspace-mode", apiKey, "192.168.1.1:1234", map[string]any{"mode": "scoped"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/workspace-mode: expected 200, got %d", res.StatusCode)
	}

	createBody := map[string]any{
		"name":               "Project Alpha",
		"workspace_kind":     "project",
		"description":        "Primary product workspace",
		"local_roots":        []string{"/workspace/alpha"},
		"repo_roots":         []string{"/repos/alpha"},
		"protected_paths":    []string{"/workspace/alpha/secrets"},
		"related_project_id": "proj-alpha",
		"allowed_actions": map[string]any{
			"read": true, "write": true, "create": true, "modify": true, "rename_move": true, "delete": false, "execute": false,
		},
		"boundary_policy": map[string]any{"out_of_scope_default": "prompt"},
		"audit_enabled":   true,
		"tags":            []string{"product", "alpha"},
	}
	res = doJSONReq(t, srv, "POST", "/api/workspaces", apiKey, "192.168.1.1:1234", createBody)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/workspaces: expected 201, got %d", res.StatusCode)
	}
	var created map[string]any
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatalf("decode workspace create: %v", err)
	}
	workspaceID, _ := created["workspace_id"].(string)
	if workspaceID == "" {
		t.Fatalf("workspace create missing id")
	}

	res = doJSONReq(t, srv, "PUT", "/api/workspaces/active", apiKey, "192.168.1.1:1234", map[string]any{"workspace_id": workspaceID})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/workspaces/active: expected 200, got %d", res.StatusCode)
	}

	res = doJSONReq(t, srv, "GET", "/api/workspaces", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/workspaces: expected 200, got %d", res.StatusCode)
	}
	var listResp map[string]any
	if err := json.NewDecoder(res.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode workspace list: %v", err)
	}
	items, ok := listResp["items"].([]any)
	if !ok || len(items) < 2 {
		t.Fatalf("expected seeded default workspace plus created workspace, got %v", listResp["items"])
	}
	foundCreated := false
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if item["workspace_id"] == workspaceID {
			foundCreated = true
			break
		}
	}
	if !foundCreated {
		t.Fatalf("expected created workspace %s to appear in list, got %v", workspaceID, listResp["items"])
	}
	if listResp["active_workspace_id"] != workspaceID {
		t.Fatalf("expected active workspace id %s, got %v", workspaceID, listResp["active_workspace_id"])
	}

	res = doJSONReq(t, srv, "PUT", "/api/workspaces/"+workspaceID, apiKey, "192.168.1.1:1234", map[string]any{
		"name":          "Project Alpha Updated",
		"notes":         "Bound to the primary project",
		"tags":          []string{"product", "alpha", "critical"},
		"audit_enabled": false,
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/workspaces/{id}: expected 200, got %d", res.StatusCode)
	}

	res = doJSONReq(t, srv, "PUT", "/api/workspaces/"+workspaceID+"/project-binding", apiKey, "192.168.1.1:1234", map[string]any{
		"project_id": "proj-bound",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/workspaces/{id}/project-binding: expected 200, got %d", res.StatusCode)
	}

	res = doJSONReq(t, srv, "GET", "/api/projects/proj-bound/workspace", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/projects/{projectID}/workspace: expected 200, got %d", res.StatusCode)
	}

	res = doJSONReq(t, srv, "PUT", "/api/workspaces/active", apiKey, "192.168.1.1:1234", map[string]any{"workspace_id": ""})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/workspaces/active clear: expected 200, got %d", res.StatusCode)
	}

	res = doJSONReq(t, srv, "GET", "/api/workspaces/active?project_id=proj-bound", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/workspaces/active project-bound: expected 200, got %d", res.StatusCode)
	}
	var activeResp map[string]any
	if err := json.NewDecoder(res.Body).Decode(&activeResp); err != nil {
		t.Fatalf("decode active workspace response: %v", err)
	}
	if activeResp["active_workspace_id"] != workspaceID || activeResp["active_workspace_selection"] != "project_bound" {
		t.Fatalf("expected project-bound active workspace %s, got %+v", workspaceID, activeResp)
	}

	res = doJSONReq(t, srv, "PUT", "/api/workspaces/"+workspaceID, apiKey, "192.168.1.1:1234", map[string]any{
		"name": "Project Alpha Final",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/workspaces/{id} preserve optional fields: expected 200, got %d", res.StatusCode)
	}

	res = doJSONReq(t, srv, "GET", "/api/workspaces/"+workspaceID, apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/workspaces/{id}: expected 200, got %d", res.StatusCode)
	}
	var fetched map[string]any
	if err := json.NewDecoder(res.Body).Decode(&fetched); err != nil {
		t.Fatalf("decode workspace fetch: %v", err)
	}
	if fetched["related_project_id"] != "proj-bound" {
		t.Fatalf("expected related_project_id to be preserved, got %v", fetched["related_project_id"])
	}
	if fetched["notes"] != "Bound to the primary project" {
		t.Fatalf("expected notes to be preserved, got %v", fetched["notes"])
	}

	res = doJSONReq(t, srv, "POST", "/api/workspaces/"+workspaceID+"/whitelist-rules", apiKey, "192.168.1.1:1234", map[string]any{
		"scope":        "/outside/reports",
		"action_types": []string{"read", "write"},
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/workspaces/{id}/whitelist-rules: expected 201, got %d", res.StatusCode)
	}
	var createdRule map[string]any
	if err := json.NewDecoder(res.Body).Decode(&createdRule); err != nil {
		t.Fatalf("decode created whitelist rule: %v", err)
	}
	ruleID, _ := createdRule["rule_id"].(string)
	if ruleID == "" {
		t.Fatalf("expected created whitelist rule id")
	}

	res = doJSONReq(t, srv, "GET", "/api/workspaces/"+workspaceID+"/whitelist-rules", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/workspaces/{id}/whitelist-rules: expected 200, got %d", res.StatusCode)
	}
	var ruleList map[string]any
	if err := json.NewDecoder(res.Body).Decode(&ruleList); err != nil {
		t.Fatalf("decode whitelist list: %v", err)
	}
	rules, ok := ruleList["items"].([]any)
	if !ok || len(rules) != 1 {
		t.Fatalf("expected one whitelist rule, got %v", ruleList["items"])
	}

	res = doJSONReq(t, srv, "POST", "/api/workspaces/boundary/resolve", apiKey, "192.168.1.1:1234", map[string]any{
		"workspace_id": workspaceID,
		"decision":     "always_allow",
		"target_path":  "/outside/logs",
		"action":       "read",
		"action_types": []string{"read"},
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/workspaces/boundary/resolve always_allow: expected 201, got %d", res.StatusCode)
	}

	res = doJSONReq(t, srv, "POST", "/api/workspaces/boundary/resolve", apiKey, "192.168.1.1:1234", map[string]any{
		"workspace_id": workspaceID,
		"decision":     "allow_once",
		"target_path":  "/outside/tmp",
		"action":       "write",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/workspaces/boundary/resolve allow_once: expected 200, got %d", res.StatusCode)
	}

	secondWorkspace := schema.Workspace{
		ID:             "ws-beta",
		Name:           "Beta",
		Kind:           schema.WorkspaceKindGeneral,
		Status:         schema.WorkspaceStatusActive,
		LocalRoots:     []string{"/workspace/beta"},
		RepoRoots:      []string{"/repos/beta"},
		AllowedActions: schema.AllowedActions{Read: true, Write: true, Create: true, Modify: true, RenameMove: true, Delete: false, Execute: false},
		BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: "prompt"},
		AuditEnabled:   true,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
		CreatedBy:      "owner",
	}
	if err := store.SaveWorkspace(context.Background(), db, secondWorkspace); err != nil {
		t.Fatalf("SaveWorkspace second: %v", err)
	}

	res = doJSONReq(t, srv, "POST", "/api/workspaces/boundary/resolve", apiKey, "192.168.1.1:1234", map[string]any{
		"decision":            "switched_workspace",
		"switch_workspace_id": secondWorkspace.ID,
		"target_path":         "/workspace/beta/report.md",
		"action":              "write",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/workspaces/boundary/resolve switched_workspace: expected 200, got %d", res.StatusCode)
	}

	res = doJSONReq(t, srv, "POST", "/api/workspaces/"+workspaceID+"/whitelist-rules/"+ruleID+"/revoke", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/workspaces/{id}/whitelist-rules/{ruleID}/revoke: expected 200, got %d", res.StatusCode)
	}

	res = doJSONReq(t, srv, "POST", "/api/workspaces/"+workspaceID+"/archive", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/workspaces/{id}/archive: expected 200, got %d", res.StatusCode)
	}
}

func TestWorkspaceAPI_RecordsLifecycleHistoryEvents(t *testing.T) {
	srv, _, db := testServer(t)
	passport := claimTestInstance(t, srv, "Owner", "owner-secret")
	apiKey, _ := passport["primary_api_key"].(string)

	createBody := map[string]any{
		"name":            "Lifecycle Workspace",
		"workspace_kind":  "project",
		"local_roots":     []string{"/workspace/lifecycle"},
		"repo_roots":      []string{"/repos/lifecycle"},
		"boundary_policy": map[string]any{"out_of_scope_default": "prompt"},
	}
	res := doJSONReq(t, srv, "POST", "/api/workspaces", apiKey, "192.168.1.1:1234", createBody)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/workspaces: expected 201, got %d", res.StatusCode)
	}
	var created map[string]any
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatalf("decode workspace create: %v", err)
	}
	workspaceID, _ := created["workspace_id"].(string)
	if workspaceID == "" {
		t.Fatal("expected created workspace id")
	}

	res = doJSONReq(t, srv, "PUT", "/api/workspaces/active", apiKey, "192.168.1.1:1234", map[string]any{"workspace_id": workspaceID})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/workspaces/active: expected 200, got %d", res.StatusCode)
	}

	res = doJSONReq(t, srv, "PUT", "/api/workspaces/"+workspaceID, apiKey, "192.168.1.1:1234", map[string]any{
		"name": "Lifecycle Workspace Updated",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/workspaces/{id}: expected 200, got %d", res.StatusCode)
	}

	res = doJSONReq(t, srv, "POST", "/api/workspaces/"+workspaceID+"/archive", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/workspaces/{id}/archive: expected 200, got %d", res.StatusCode)
	}

	outcomes, _, err := store.ListExecutionOutcomes(context.Background(), db, 20, "", nil)
	if err != nil {
		t.Fatalf("ListExecutionOutcomes: %v", err)
	}

	var (
		sawCreated   bool
		sawActivated bool
		sawUpdated   bool
		sawArchived  bool
	)
	for _, outcome := range outcomes {
		if outcome.WorkspaceID != workspaceID {
			continue
		}
		if outcome.BoundaryCrossing || outcome.ApprovalRequired || outcome.ApprovalOutcome != schema.ApprovalOutcomeNA {
			t.Fatalf("expected lifecycle outcome to be non-boundary audit metadata, got %+v", outcome)
		}
		switch {
		case outcome.CommandType == schema.CommandTypeCreate && strings.Contains(outcome.AffectedEntities, "workspace_event:created"):
			sawCreated = true
		case outcome.CommandType == schema.CommandTypeUpdate && strings.Contains(outcome.AffectedEntities, "workspace_event:activated") && strings.Contains(outcome.AffectedEntities, "configuration:active_workspace_id"):
			sawActivated = true
		case outcome.CommandType == schema.CommandTypeUpdate && strings.Contains(outcome.AffectedEntities, "workspace_event:updated"):
			sawUpdated = true
		case outcome.CommandType == schema.CommandTypeUpdate && strings.Contains(outcome.AffectedEntities, "workspace_event:archived"):
			sawArchived = true
		}
	}

	if !sawCreated || !sawActivated || !sawUpdated || !sawArchived {
		t.Fatalf("expected lifecycle history rows for create=%v activate=%v update=%v archive=%v", sawCreated, sawActivated, sawUpdated, sawArchived)
	}
}

func TestWorkspaceAPI_DeleteWorkspaceClearsActiveSelection(t *testing.T) {
	srv, _, _ := testServer(t)
	passport := claimTestInstance(t, srv, "Owner", "owner-secret")
	apiKey, _ := passport["primary_api_key"].(string)

	res := doJSONReq(t, srv, "POST", "/api/workspaces", apiKey, "192.168.1.1:1234", map[string]any{
		"name":            "Delete Workspace",
		"workspace_kind":  "general",
		"boundary_policy": map[string]any{"out_of_scope_default": "prompt"},
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/workspaces: expected 201, got %d", res.StatusCode)
	}
	var created map[string]any
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatalf("decode workspace create: %v", err)
	}
	workspaceID, _ := created["workspace_id"].(string)
	if workspaceID == "" {
		t.Fatal("expected workspace id")
	}
	res = doJSONReq(t, srv, "PUT", "/api/workspaces/active", apiKey, "192.168.1.1:1234", map[string]any{"workspace_id": workspaceID})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/workspaces/active: expected 200, got %d", res.StatusCode)
	}
	res = doJSONReq(t, srv, "DELETE", "/api/workspaces/"+workspaceID, apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE /api/workspaces/{id}: expected 204, got %d", res.StatusCode)
	}
	res = doJSONReq(t, srv, "GET", "/api/workspaces/active", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/workspaces/active: expected 200, got %d", res.StatusCode)
	}
	var active map[string]any
	if err := json.NewDecoder(res.Body).Decode(&active); err != nil {
		t.Fatalf("decode active workspace: %v", err)
	}
	if active["active_workspace_id"] != "" {
		t.Fatalf("expected active workspace to be cleared, got %#v", active["active_workspace_id"])
	}
}

func TestWorkspaceAPI_PathBrowserListsServerVisibleDirectories(t *testing.T) {
	srv, _, _ := testServer(t)
	passport := claimTestInstance(t, srv, "Owner", "owner-secret")
	apiKey, _ := passport["primary_api_key"].(string)
	root := t.TempDir()
	srv.cfg.WorkspaceDir = root
	if err := os.Mkdir(filepath.Join(root, "repo"), 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}

	res := doJSONReq(t, srv, "GET", "/api/workspaces/path-roots", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/workspaces/path-roots: expected 200, got %d", res.StatusCode)
	}
	var roots map[string]any
	if err := json.NewDecoder(res.Body).Decode(&roots); err != nil {
		t.Fatalf("decode roots: %v", err)
	}
	items, ok := roots["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("expected path roots, got %#v", roots["items"])
	}
	foundWorkspaceRoot := false
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if ok && item["path"] == root {
			foundWorkspaceRoot = true
			break
		}
	}
	if !foundWorkspaceRoot {
		t.Fatalf("expected workspace root %s in path roots, got %#v", root, items)
	}

	res = doJSONReq(t, srv, "GET", "/api/workspaces/path-children?path="+url.QueryEscape(root), apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/workspaces/path-children: expected 200, got %d", res.StatusCode)
	}
	var children map[string]any
	if err := json.NewDecoder(res.Body).Decode(&children); err != nil {
		t.Fatalf("decode children: %v", err)
	}
	childItems, ok := children["items"].([]any)
	if !ok || len(childItems) != 1 {
		t.Fatalf("expected only directory children, got %#v", children["items"])
	}
	child, ok := childItems[0].(map[string]any)
	if !ok || child["name"] != "repo" || child["path"] != filepath.Join(root, "repo") {
		t.Fatalf("expected repo directory child, got %#v", childItems[0])
	}
}

func TestWorkspaceAPI_RejectsInvalidWorkspacePayload(t *testing.T) {
	srv, _, _ := testServer(t)
	passport := claimTestInstance(t, srv, "Owner", "owner-secret")
	apiKey, _ := passport["primary_api_key"].(string)

	res := doJSONReq(t, srv, "POST", "/api/workspaces", apiKey, "192.168.1.1:1234", map[string]any{
		"name":            "Broken Workspace",
		"workspace_kind":  "global",
		"boundary_policy": map[string]any{"out_of_scope_default": "prompt"},
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /api/workspaces with invalid workspace_kind: expected 400, got %d", res.StatusCode)
	}
}

func TestWorkspaceAPI_RejectsUnknownRelatedProject(t *testing.T) {
	srv, _, _ := testServer(t)
	passport := claimTestInstance(t, srv, "Owner", "owner-secret")
	apiKey, _ := passport["primary_api_key"].(string)

	res := doJSONReq(t, srv, "POST", "/api/workspaces", apiKey, "192.168.1.1:1234", map[string]any{
		"name":               "Broken Project Workspace",
		"workspace_kind":     "project",
		"related_project_id": "proj-missing",
		"boundary_policy":    map[string]any{"out_of_scope_default": "prompt"},
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /api/workspaces with missing related project: expected 400, got %d", res.StatusCode)
	}
}

func TestWorkspaceAPI_ModeChangeRequiresOwnerAuthority(t *testing.T) {
	srv, _, db := testServer(t)
	claimTestInstance(t, srv, "Owner", "owner-secret")

	rawKey, hash, err := store.GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	if err := store.CreateAPIKey(context.Background(), db, store.APIKey{
		ID:        "foreign-key",
		OwnerID:   "someone-else",
		KeyHash:   hash,
		Name:      "foreign",
		Scopes:    []string{"admin"},
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	res := doJSONReq(t, srv, "PUT", "/api/workspace-mode", rawKey, "192.168.1.1:1234", map[string]any{"mode": "scoped"})
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("PUT /api/workspace-mode with non-owner key: expected 403, got %d", res.StatusCode)
	}
}

func TestRunsEndpointExposesWorkspaceAuditFields(t *testing.T) {
	srv, _, db := testServer(t)
	passport := claimTestInstance(t, srv, "Owner", "owner-secret")
	apiKey, _ := passport["primary_api_key"].(string)

	err := store.SaveExecutionOutcome(context.Background(), db, schema.ExecutionOutcome{
		AttemptID:          "attempt-workspace-1",
		CommandID:          "cmd-workspace-1",
		AttemptNumber:      1,
		CommandType:        schema.CommandTypeUpdate,
		StartTime:          time.Now().UTC(),
		Outcome:            schema.ExecutionOutcomeRejectedPreExecution,
		FailureReason:      "Action targeting path outside workspace scope: /outside/path.txt",
		AffectedEntities:   `["path:/outside/path.txt"]`,
		WorkspaceID:        "ws-alpha",
		BoundaryCrossing:   true,
		ApprovalRequired:   true,
		ApprovalOutcome:    schema.ApprovalOutcomeDenied,
		CompensationStatus: schema.CompensationStatusNotRequired,
		RecoveryStatus:     schema.RecoveryStatusNotRequired,
	})
	if err != nil {
		t.Fatalf("SaveExecutionOutcome: %v", err)
	}

	res := doJSONReq(t, srv, "GET", "/api/runs/attempt-workspace-1", apiKey, "192.168.1.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/runs/{id}: expected 200, got %d", res.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode run payload: %v", err)
	}
	if payload["workspace_id"] != "ws-alpha" || payload["boundary_crossing"] != true || payload["approval_required"] != true || payload["approval_outcome"] != "denied" {
		t.Fatalf("expected workspace audit fields in run payload, got %+v", payload)
	}
}
