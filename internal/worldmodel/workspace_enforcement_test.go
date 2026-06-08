package worldmodel

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/governor"
	"github.com/ceoai/navi/internal/schema"
)

func testWorkspaceForEnforcement(root string) schema.Workspace {
	now := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)
	return schema.Workspace{
		ID:             "ws-enforcement",
		Name:           "Workspace Enforcement Test",
		Kind:           schema.WorkspaceKindGeneral,
		Status:         schema.WorkspaceStatusActive,
		LocalRoots:     []string{root},
		AllowedActions: schema.AllowedActions{Read: true, Write: true, Create: true, Modify: true, RenameMove: true, Delete: true, Execute: true},
		BoundaryPolicy: schema.BoundaryPolicy{OutOfScopeDefault: schema.BoundaryPolicyOutOfScopeDeny},
		AuditEnabled:   true,
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      "owner-1",
	}
}

func TestWorkspaceEnforcer_CheckActionRejectsTraversalOutsideRoot(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	root := filepath.Join(base, "workspace")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll root: %v", err)
	}

	ws := testWorkspaceForEnforcement(root)
	enforcer := NewWorkspaceEnforcer(&ws, string(schema.WorkspaceOperatingModeScoped))

	target := filepath.Join(root, "..", "outside", "notes.txt")
	result := enforcer.CheckAction(ctx, target, schema.WorkspaceActionCreate)
	if result.Outcome != governor.ValidationRejected {
		t.Fatalf("expected traversal target to be rejected, got %#v", result)
	}
	if !result.BoundaryCross {
		t.Fatal("expected traversal target to be marked as a boundary crossing")
	}
}

func TestWorkspaceEnforcer_CheckActionRejectsSymlinkEscapeThroughMissingDescendant(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	root := filepath.Join(base, "workspace")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll root: %v", err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("MkdirAll outside: %v", err)
	}

	link := filepath.Join(root, "escape-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	ws := testWorkspaceForEnforcement(root)
	enforcer := NewWorkspaceEnforcer(&ws, string(schema.WorkspaceOperatingModeScoped))

	target := filepath.Join(link, "nested", "report.txt")
	result := enforcer.CheckAction(ctx, target, schema.WorkspaceActionCreate)
	if result.Outcome != governor.ValidationRejected {
		t.Fatalf("expected symlink escape to be rejected, got %#v", result)
	}
	if !result.BoundaryCross {
		t.Fatal("expected symlink escape to be marked as a boundary crossing")
	}
}

func TestWorkspaceEnforcer_CheckActionAllowsSymlinkTargetWithinRoot(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	root := filepath.Join(base, "workspace")
	realDir := filepath.Join(root, "real")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("MkdirAll realDir: %v", err)
	}

	link := filepath.Join(root, "link-inside")
	if err := os.Symlink(realDir, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	ws := testWorkspaceForEnforcement(root)
	enforcer := NewWorkspaceEnforcer(&ws, string(schema.WorkspaceOperatingModeScoped))

	target := filepath.Join(link, "draft.txt")
	result := enforcer.CheckAction(ctx, target, schema.WorkspaceActionCreate)
	if result.Outcome != governor.ValidationApproved {
		t.Fatalf("expected in-root symlink target to be approved, got %#v", result)
	}
}

func TestWorkspaceEnforcer_CheckActionRejectsProtectedSubpath(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	root := filepath.Join(base, "workspace")
	protected := filepath.Join(root, "secrets")
	if err := os.MkdirAll(protected, 0o755); err != nil {
		t.Fatalf("MkdirAll protected: %v", err)
	}

	ws := testWorkspaceForEnforcement(root)
	ws.ProtectedPaths = []string{protected}
	enforcer := NewWorkspaceEnforcer(&ws, string(schema.WorkspaceOperatingModeScoped))

	target := filepath.Join(protected, "token.txt")
	result := enforcer.CheckAction(ctx, target, schema.WorkspaceActionRead)
	if result.Outcome != governor.ValidationRejected {
		t.Fatalf("expected protected path to be rejected, got %#v", result)
	}
}

func TestPathPatternMatchesSupportsExactPrefixesAndGlobs(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		pattern string
		want    bool
	}{
		{"exact subtree", "/workspace/secrets/token.txt", "/workspace/secrets", true},
		{"double star", "/workspace/a/secrets/token.txt", "**/secrets/**", true},
		{"single star", "/home/alice/.ssh/config", "/home/*/.ssh/**", true},
		{"windows style", `C:\Windows\System32\drivers\etc\hosts`, `C:\Windows\System32\**`, true},
		{"no match", "/workspace/docs/readme.md", "**/secrets/**", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := pathPatternMatches(tc.target, tc.pattern); got != tc.want {
				t.Fatalf("pathPatternMatches(%q, %q) = %v, want %v", tc.target, tc.pattern, got, tc.want)
			}
		})
	}
}

func TestWorkspaceEnforcer_ProtectedPathsApplyInGlobalMode(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	home := filepath.Join(base, "home")
	t.Setenv("HOME", home)
	protected := filepath.Join(home, ".ssh", "id_rsa")
	if err := os.MkdirAll(filepath.Dir(protected), 0o755); err != nil {
		t.Fatalf("MkdirAll protected dir: %v", err)
	}
	if err := os.WriteFile(protected, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile protected: %v", err)
	}

	enforcer := NewWorkspaceEnforcer(nil, string(schema.WorkspaceOperatingModeGlobal))
	result := enforcer.CheckAction(ctx, protected, schema.WorkspaceActionRead)
	if result.Outcome != governor.ValidationRejected {
		t.Fatalf("expected global protected path to be rejected, got %#v", result)
	}
	if !strings.Contains(result.Reason, "system-protected") {
		t.Fatalf("expected system-protected reason, got %q", result.Reason)
	}
}

func TestWorkspaceEnforcer_GlobalProtectedPathsSupportPatterns(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	target := filepath.Join(root, "app", "secrets", "token.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("MkdirAll target dir: %v", err)
	}
	if err := os.WriteFile(target, []byte("secret"), 0o644); err != nil {
		t.Fatalf("WriteFile target: %v", err)
	}

	enforcer := NewWorkspaceEnforcer(nil, string(schema.WorkspaceOperatingModeGlobal))
	enforcer.GlobalProtectedPaths = []string{"**/secrets/**"}
	result := enforcer.CheckAction(ctx, target, schema.WorkspaceActionRead)
	if result.Outcome != governor.ValidationRejected {
		t.Fatalf("expected user-defined protected glob to reject, got %#v", result)
	}
	if !strings.Contains(result.Reason, "user-defined protected") {
		t.Fatalf("expected user-defined protected reason, got %q", result.Reason)
	}
}

func TestWorkspaceEnforcer_WorkspaceProtectedPathCanBeOverriddenByWhitelistRule(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	target := filepath.Join(root, "secrets", "vault.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("MkdirAll target dir: %v", err)
	}
	if err := os.WriteFile(target, []byte("secret"), 0o644); err != nil {
		t.Fatalf("WriteFile target: %v", err)
	}

	ws := testWorkspaceForEnforcement(root)
	ws.ProtectedPaths = []string{"**/secrets/**"}
	now := time.Now().UTC()
	ws.WhitelistRules = []schema.WhitelistRule{
		{
			RuleID:      "rule-protected-read",
			Scope:       "**/secrets/**",
			ActionTypes: []string{"read"},
			Status:      schema.WhitelistRuleStatusActive,
			CreatedAt:   now,
			CreatedBy:   "owner-1",
		},
	}

	enforcer := NewWorkspaceEnforcer(&ws, string(schema.WorkspaceOperatingModeScoped))
	result := enforcer.CheckAction(ctx, target, schema.WorkspaceActionRead)
	if result.Outcome != governor.ValidationApproved {
		t.Fatalf("expected protected path whitelist override to approve, got %#v", result)
	}
	if result.MatchingRuleID != "rule-protected-read" {
		t.Fatalf("expected matching rule id, got %#v", result)
	}
}

func TestWorkspaceEnforcer_ProtectedPathRejectsWithoutMatchingWhitelistAction(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	target := filepath.Join(root, "secrets", "vault.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("MkdirAll target dir: %v", err)
	}
	if err := os.WriteFile(target, []byte("secret"), 0o644); err != nil {
		t.Fatalf("WriteFile target: %v", err)
	}

	ws := testWorkspaceForEnforcement(root)
	ws.ProtectedPaths = []string{"**/secrets/**"}
	now := time.Now().UTC()
	ws.WhitelistRules = []schema.WhitelistRule{
		{
			RuleID:      "rule-protected-read",
			Scope:       "**/secrets/**",
			ActionTypes: []string{"read"},
			Status:      schema.WhitelistRuleStatusActive,
			CreatedAt:   now,
			CreatedBy:   "owner-1",
		},
	}

	enforcer := NewWorkspaceEnforcer(&ws, string(schema.WorkspaceOperatingModeScoped))
	result := enforcer.CheckAction(ctx, target, schema.WorkspaceActionDelete)
	if result.Outcome != governor.ValidationRejected {
		t.Fatalf("expected unmatched whitelist action to stay rejected, got %#v", result)
	}
	if result.MatchingRuleID != "" {
		t.Fatalf("expected no whitelist override, got %#v", result)
	}
}

func TestWorkspaceEnforcer_OverlapUsesOnlyActiveWorkspacePermissions(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	parentRoot := filepath.Join(base, "projects")
	childRoot := filepath.Join(parentRoot, "repo")
	if err := os.MkdirAll(childRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll childRoot: %v", err)
	}
	target := filepath.Join(childRoot, "README.md")
	if err := os.WriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile target: %v", err)
	}

	parentWorkspace := testWorkspaceForEnforcement(parentRoot)
	parentWorkspace.ID = "ws-parent"
	parentWorkspace.Name = "Parent Workspace"
	parentWorkspace.AllowedActions.Delete = false

	childWorkspace := testWorkspaceForEnforcement(childRoot)
	childWorkspace.ID = "ws-child"
	childWorkspace.Name = "Child Workspace"
	childWorkspace.AllowedActions.Delete = true

	parentEnforcer := NewWorkspaceEnforcer(&parentWorkspace, string(schema.WorkspaceOperatingModeScoped))
	childEnforcer := NewWorkspaceEnforcer(&childWorkspace, string(schema.WorkspaceOperatingModeScoped))

	parentResult := parentEnforcer.CheckAction(ctx, target, schema.WorkspaceActionDelete)
	if parentResult.Outcome != governor.ValidationRejected {
		t.Fatalf("expected parent workspace deny-mask to govern overlapping path, got %#v", parentResult)
	}

	childResult := childEnforcer.CheckAction(ctx, target, schema.WorkspaceActionDelete)
	if childResult.Outcome != governor.ValidationApproved {
		t.Fatalf("expected explicit child workspace to approve under its own policy, got %#v", childResult)
	}
}

func TestWorkspaceEnforcer_CheckActionHonorsAllowedActionsForEveryActionType(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("ok"), 0o644); err != nil {
		t.Fatalf("WriteFile target: %v", err)
	}

	tests := []struct {
		name   string
		action schema.WorkspaceActionType
		deny   func(*schema.AllowedActions)
	}{
		{"read", schema.WorkspaceActionRead, func(a *schema.AllowedActions) { a.Read = false }},
		{"write", schema.WorkspaceActionWrite, func(a *schema.AllowedActions) { a.Write = false }},
		{"create", schema.WorkspaceActionCreate, func(a *schema.AllowedActions) { a.Create = false }},
		{"modify", schema.WorkspaceActionModify, func(a *schema.AllowedActions) { a.Modify = false }},
		{"rename_move", schema.WorkspaceActionRenameMove, func(a *schema.AllowedActions) { a.RenameMove = false }},
		{"delete", schema.WorkspaceActionDelete, func(a *schema.AllowedActions) { a.Delete = false }},
		{"execute", schema.WorkspaceActionExecute, func(a *schema.AllowedActions) { a.Execute = false }},
	}

	for _, tc := range tests {
		t.Run(string(tc.action), func(t *testing.T) {
			allowedWorkspace := testWorkspaceForEnforcement(root)
			allowedEnforcer := NewWorkspaceEnforcer(&allowedWorkspace, string(schema.WorkspaceOperatingModeScoped))
			allowed := allowedEnforcer.CheckAction(ctx, target, tc.action)
			if allowed.Outcome != governor.ValidationApproved {
				t.Fatalf("expected %s to be allowed, got %#v", tc.name, allowed)
			}

			deniedWorkspace := testWorkspaceForEnforcement(root)
			tc.deny(&deniedWorkspace.AllowedActions)
			deniedEnforcer := NewWorkspaceEnforcer(&deniedWorkspace, string(schema.WorkspaceOperatingModeScoped))
			denied := deniedEnforcer.CheckAction(ctx, target, tc.action)
			if denied.Outcome != governor.ValidationRejected {
				t.Fatalf("expected %s to be denied, got %#v", tc.name, denied)
			}
			if !strings.Contains(denied.Reason, string(tc.action)) {
				t.Fatalf("expected deny reason to mention %q, got %q", tc.action, denied.Reason)
			}
		})
	}
}
