package gateway

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebArtifactWorkspaceShellIncludesRequiredViewsModesAndRehydration(t *testing.T) {
	path := filepath.Join("..", "..", "web", "index-legacy.html")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read web shell: %v", err)
	}
	content := string(body)

	requiredFragments := []string{
		`id="artifact-workspace-shell"`,
		`id="artifact-navigator"`,
		`id="artifact-canvas"`,
		`id="save-state-badge"`,
		`id="save-artifact"`,
		`id="sync-state-badge"`,
		`data-view="recent"`,
		`data-view="current_conversation"`,
		`data-view="current_project"`,
		`data-view="current_workspace"`,
		`data-view="drafts"`,
		`data-view="failed"`,
		`data-view="shared_exported"`,
		`data-view="archived"`,
		`data-mode="read"`,
		`data-mode="edit"`,
		`data-mode="diff"`,
		`data-mode="history"`,
		`data-mode="inspector"`,
		`localStorage.setItem(STORAGE_KEYS.workspace`,
		`loadWorkspaceState()`,
		`artifactDrafts`,
		`artifactSaveState`,
		`preferredExportFormat(`,
		`artifactConflictState(`,
		`saveArtifactDraft(`,
		`"conflicted"`,
		`"save_failed"`,
		`data-conflict-action="latest"`,
		`data-conflict-action="branch"`,
		`data-conflict-action="rebase"`,
		`Recovery History`,
		`/api/artifact-exports/`,
		`renderContentByType`,
		`renderMarkdown(`,
		`renderCode(`,
		`renderData(`,
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(content, fragment) {
			t.Fatalf("web shell missing required fragment %q", fragment)
		}
	}
}

func TestWebWorkspaceControlShellIncludesRequiredSettingsCrudAndBoundaryControls(t *testing.T) {
	path := filepath.Join("..", "..", "web", "index-legacy.html")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read web shell: %v", err)
	}
	content := string(body)

	requiredFragments := []string{
		`id="workspace-control-shell"`,
		`id="workspace-mode-badge"`,
		`id="active-workspace-badge"`,
		`data-workspace-mode="hybrid"`,
		`data-workspace-mode="scoped"`,
		`data-workspace-mode="global"`,
		`id="workspace-list"`,
		`id="save-workspace"`,
		`id="bind-workspace-project"`,
		`id="archive-workspace"`,
		`id="set-active-workspace"`,
		`id="workspace-rules"`,
		`data-revoke-rule="`,
		`id="boundary-deny"`,
		`id="boundary-allow-once"`,
		`id="boundary-always-allow"`,
		`id="boundary-switch"`,
		`id="boundary-run-list"`,
		`requestWorkspaceConsole()`,
		`currentWorkspacePayload()`,
		`resolveBoundary("denied")`,
		`resolveBoundary("allow_once")`,
		`resolveBoundary("always_allow")`,
		`resolveBoundary("switched_workspace")`,
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(content, fragment) {
			t.Fatalf("web shell missing required fragment %q", fragment)
		}
	}
}

func TestWebLiveLogsShellIncludesRequiredDiagnosticsControlsAndOperatorStream(t *testing.T) {
	path := filepath.Join("..", "..", "web", "index-legacy.html")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read web shell: %v", err)
	}
	content := string(body)

	requiredFragments := []string{
		`id="logs-shell"`,
		`id="log-view-controls"`,
		`data-log-view="live"`,
		`data-log-view="errors"`,
		`data-log-view="summary"`,
		`id="log-severity-filters"`,
		`data-log-severity="error"`,
		`id="log-category-filters"`,
		`data-log-category="tool"`,
		`data-log-category="governance"`,
		`data-log-category="artifact"`,
		`data-log-category="error"`,
		`data-log-category="message"`,
		`id="log-search"`,
		`id="log-run-filter"`,
		`id="log-pause"`,
		`id="log-clear"`,
		`id="log-autoscroll"`,
		`id="refresh-errors"`,
		`id="log-stream"`,
		`params: { chat_id: chatID, after_seq: state.afterSeq, stream: "operator" }`,
		`appendLiveLogEntry(event);`,
		`refreshDiagnostics().catch`,
		`refreshErrorLogs()`,
		`refreshErrorSummary()`,
		`/api/errors?component=connector&limit=100`,
		`renderLogStream()`,
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(content, fragment) {
			t.Fatalf("web shell missing required fragment %q", fragment)
		}
	}
}
