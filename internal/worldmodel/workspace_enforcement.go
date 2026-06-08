package worldmodel

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/open-navi/navi/internal/governor"
	"github.com/open-navi/navi/internal/schema"
)

// WorkspaceValidationResult extends governor validation with workspace-specific details.
type WorkspaceValidationResult struct {
	Outcome        governor.ValidationOutcome
	Reason         string
	WorkspaceID    string
	BoundaryCross  bool
	MatchingRuleID string
}

// WorkspaceEnforcer implements the machine enforcement logic for workspace boundaries.
// It handles path normalization, root scope verification, and protected path exclusions.
type WorkspaceEnforcer struct {
	ActiveWorkspace      *schema.Workspace
	GlobalProtectedPaths []string
	OperatingMode        string // "global", "scoped", "hybrid"
}

var defaultSystemProtectedPaths = []string{
	"~/.ssh/**",
	"~/.config/gcloud/**",
	"~/.aws/**",
	"~/.docker/**",
	"~/.kube/**",
	"~/.npm/**",
	"~/.pnpm-store/**",
	"~/.cache/pip/**",
	"~/.cache/pypoetry/**",
	"/etc/shadow",
	"/etc/sudoers",
	"/etc/ssh/**",
	`C:\Windows\System32\**`,
}

// NewWorkspaceEnforcer creates an enforcer with the given workspace and system mode.
func NewWorkspaceEnforcer(w *schema.Workspace, mode string) *WorkspaceEnforcer {
	if mode == "" {
		mode = string(schema.WorkspaceOperatingModeGlobal)
	}
	return &WorkspaceEnforcer{
		ActiveWorkspace: w,
		OperatingMode:   mode,
	}
}

// CheckAction verifies if an exact workspace action type is allowed on a path within the active workspace.
func (e *WorkspaceEnforcer) CheckAction(ctx context.Context, path string, actionType schema.WorkspaceActionType) WorkspaceValidationResult {
	res := WorkspaceValidationResult{
		Outcome: governor.ValidationApproved,
	}

	if e.ActiveWorkspace != nil {
		res.WorkspaceID = e.ActiveWorkspace.ID
	}

	// 1. Normalize path before any protected/scope checks.
	normalized, err := e.NormalizePath(path)
	if err != nil {
		return WorkspaceValidationResult{
			Outcome:     governor.ValidationRejected,
			Reason:      fmt.Sprintf("workspace enforcement: path normalization failed: %v", err),
			WorkspaceID: res.WorkspaceID,
		}
	}

	// 2. Protected paths are enforced in every operating mode.
	if protected := e.checkProtectedPath(normalized, actionType); protected != nil {
		protected.WorkspaceID = res.WorkspaceID
		return *protected
	}

	// 3. Global mode skips workspace scoping, but not protected paths.
	if e.OperatingMode == string(schema.WorkspaceOperatingModeGlobal) {
		return res
	}

	// 4. If no active workspace, we allow it in hybrid mode, but reject in scoped mode.
	if e.ActiveWorkspace == nil {
		if e.OperatingMode == string(schema.WorkspaceOperatingModeScoped) {
			return WorkspaceValidationResult{
				Outcome: governor.ValidationRejected,
				Reason:  "workspace enforcement: no active workspace in scoped mode; select or create a workspace before running scoped file or repo actions",
			}
		}
		return res
	}

	// 5. Scope Check.
	inScope := false
	allRoots := append([]string{}, e.ActiveWorkspace.LocalRoots...)
	allRoots = append(allRoots, e.ActiveWorkspace.RepoRoots...)

	for _, root := range allRoots {
		normRoot, err := e.NormalizePath(root)
		if err != nil {
			continue
		}
		if isPathWithin(normalized, normRoot) {
			inScope = true
			break
		}
	}

	if !inScope {
		res.BoundaryCross = true
		if ruleID, ok := e.matchingWhitelistRule(normalized, actionType); ok {
			res.MatchingRuleID = ruleID
			return res // Allowed by whitelist
		}

		// Boundary crossing policy.
		if e.ActiveWorkspace.BoundaryPolicy.OutOfScopeDefault == schema.BoundaryPolicyOutOfScopeDeny {
			return WorkspaceValidationResult{
				Outcome:       governor.ValidationRejected,
				Reason:        fmt.Sprintf("workspace enforcement: path outside scope and policy is deny: %s", path),
				WorkspaceID:   res.WorkspaceID,
				BoundaryCross: true,
			}
		}

		// Default to prompt.
		return WorkspaceValidationResult{
			Outcome:       governor.ValidationRequiresConfirmation,
			Reason:        fmt.Sprintf("Action targeting path outside workspace scope: %s", path),
			WorkspaceID:   res.WorkspaceID,
			BoundaryCross: true,
		}
	}

	// 6. Deny-mask check for in-scope actions.
	if !e.IsActionPermitted(actionType) {
		return WorkspaceValidationResult{
			Outcome:     governor.ValidationRejected,
			Reason:      fmt.Sprintf("workspace enforcement: action %q is explicitly denied in this workspace", actionType),
			WorkspaceID: res.WorkspaceID,
		}
	}

	return res
}

func (e *WorkspaceEnforcer) checkProtectedPath(normalizedPath string, actionType schema.WorkspaceActionType) *WorkspaceValidationResult {
	for _, protected := range defaultSystemProtectedPaths {
		if !pathPatternMatches(normalizedPath, protected) {
			continue
		}
		if ruleID, ok := e.matchingWhitelistRule(normalizedPath, actionType); ok {
			return &WorkspaceValidationResult{
				Outcome:        governor.ValidationApproved,
				MatchingRuleID: ruleID,
			}
		}
		return &WorkspaceValidationResult{
			Outcome: governor.ValidationRejected,
			Reason:  fmt.Sprintf("workspace enforcement: path matches a system-protected rule: %s", normalizedPath),
		}
	}

	for _, protected := range e.GlobalProtectedPaths {
		if !pathPatternMatches(normalizedPath, protected) {
			continue
		}
		if ruleID, ok := e.matchingWhitelistRule(normalizedPath, actionType); ok {
			return &WorkspaceValidationResult{
				Outcome:        governor.ValidationApproved,
				MatchingRuleID: ruleID,
			}
		}
		return &WorkspaceValidationResult{
			Outcome: governor.ValidationRejected,
			Reason:  fmt.Sprintf("workspace enforcement: path matches a user-defined protected rule: %s", normalizedPath),
		}
	}

	if e.ActiveWorkspace == nil {
		return nil
	}
	for _, protected := range e.ActiveWorkspace.ProtectedPaths {
		if !pathPatternMatches(normalizedPath, protected) {
			continue
		}
		if ruleID, ok := e.matchingWhitelistRule(normalizedPath, actionType); ok {
			return &WorkspaceValidationResult{
				Outcome:        governor.ValidationApproved,
				MatchingRuleID: ruleID,
			}
		}
		return &WorkspaceValidationResult{
			Outcome: governor.ValidationRejected,
			Reason:  fmt.Sprintf("workspace enforcement: path matches a workspace-protected rule: %s", normalizedPath),
		}
	}
	return nil
}

func (e *WorkspaceEnforcer) matchingWhitelistRule(normalizedPath string, actionType schema.WorkspaceActionType) (string, bool) {
	if e.ActiveWorkspace == nil {
		return "", false
	}
	now := time.Now()
	for _, rule := range e.ActiveWorkspace.WhitelistRules {
		if rule.Status != schema.WhitelistRuleStatusActive {
			continue
		}
		if rule.ExpiresAt != nil && rule.ExpiresAt.Before(now) {
			continue
		}
		if !pathPatternMatches(normalizedPath, rule.Scope) {
			continue
		}
		for _, t := range rule.ActionTypes {
			if t == "all" || t == string(actionType) {
				return rule.RuleID, true
			}
		}
	}
	return "", false
}

// IsActionPermitted checks the workspace deny-mask.
func (e *WorkspaceEnforcer) IsActionPermitted(actionType schema.WorkspaceActionType) bool {
	if e.ActiveWorkspace == nil {
		return true
	}
	return e.ActiveWorkspace.AllowedActions.Permits(actionType)
}

func (e *WorkspaceEnforcer) CheckPath(targetPath string) error {
	res := e.CheckAction(context.Background(), targetPath, schema.WorkspaceActionRead)
	if res.Outcome == governor.ValidationRejected {
		return fmt.Errorf("%s", res.Reason)
	}
	// Note: We return nil even if it requires confirmation, because simple CheckPath
	// is often used in low-level I/O. Higher-level enforcement should use CheckAction.
	return nil
}

// NormalizePath performs strict normalization including symlink/junction resolution and absolute path conversion.
func (e *WorkspaceEnforcer) NormalizePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || path == "." {
		pwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		path = pwd
	}
	path = expandPathAliases(path)

	// Resolve absolute path first to handle relative inputs.
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)

	resolved, err := resolvePathWithExistingAncestor(abs)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func expandPathAliases(path string) string {
	if path == "~" {
		if home := homeDir(); home != "" {
			return home
		}
		return path
	}
	// Accept both forward-slash (~/) and OS-native separator (~\) so that
	// patterns like ~/.ssh/** work correctly on Windows where filepath.Separator is \.
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~"+string(filepath.Separator)) {
		if home := homeDir(); home != "" {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// homeDir returns the current user's home directory, preferring the HOME
// environment variable so that tests can override it with t.Setenv on any OS.
func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return h
}

func resolvePathWithExistingAncestor(absPath string) (string, error) {
	resolved, err := filepath.EvalSymlinks(absPath)
	if err == nil {
		return resolved, nil
	}
	if isPathResolutionPermissionError(err) {
		return absPath, nil
	}

	current := filepath.Clean(absPath)
	var suffix []string
	for {
		parent := filepath.Dir(current)
		if parent == current {
			return absPath, nil
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent

		info, statErr := os.Lstat(current)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			if isPathResolutionPermissionError(statErr) {
				return absPath, nil
			}
			return "", statErr
		}
		if info == nil {
			return absPath, nil
		}

		resolvedAncestor, err := filepath.EvalSymlinks(current)
		if err != nil {
			if isPathResolutionPermissionError(err) {
				resolvedAncestor = current
			} else {
				return "", err
			}
		}
		for i := len(suffix) - 1; i >= 0; i-- {
			resolvedAncestor = filepath.Join(resolvedAncestor, suffix[i])
		}
		return resolvedAncestor, nil
	}
}

func isPathResolutionPermissionError(err error) bool {
	if err == nil {
		return false
	}
	return os.IsPermission(err)
}

func isPathWithin(target, root string) bool {
	target = comparablePath(target)
	root = comparablePath(root)
	if target == root {
		return true
	}
	return strings.HasPrefix(target, root+string(filepath.Separator))
}

func comparablePath(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

func pathPatternMatches(targetPath, pattern string) bool {
	target := normalizeComparableMatcherPath(targetPath)
	pattern = normalizeComparableMatcherPattern(pattern)
	if target == "" || pattern == "" {
		return false
	}
	if !containsGlob(pattern) {
		if target == pattern {
			return true
		}
		return strings.HasPrefix(target, pattern+"/")
	}
	return matchPatternSegments(splitMatchPath(target), splitMatchPath(pattern))
}

func normalizeComparableMatcherPath(value string) string {
	value = strings.ReplaceAll(comparablePath(value), "\\", "/")
	return path.Clean(value)
}

func normalizeComparableMatcherPattern(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = expandPathAliases(value)
	value = strings.ReplaceAll(value, "\\", "/")
	cleaned := path.Clean(value)
	if strings.HasSuffix(value, "/**") && !strings.HasSuffix(cleaned, "/**") {
		cleaned += "/**"
	}
	if runtime.GOOS == "windows" {
		cleaned = strings.ToLower(cleaned)
	}
	return cleaned
}

func splitMatchPath(value string) []string {
	trimmed := strings.Trim(value, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func containsGlob(value string) bool {
	return strings.ContainsAny(value, "*?[")
}

func matchPatternSegments(targetSegments, patternSegments []string) bool {
	if len(patternSegments) == 0 {
		return len(targetSegments) == 0
	}
	if patternSegments[0] == "**" {
		if len(patternSegments) == 1 {
			return true
		}
		if matchPatternSegments(targetSegments, patternSegments[1:]) {
			return true
		}
		if len(targetSegments) == 0 {
			return false
		}
		return matchPatternSegments(targetSegments[1:], patternSegments)
	}
	if len(targetSegments) == 0 {
		return false
	}
	matched, err := path.Match(patternSegments[0], targetSegments[0])
	if err != nil || !matched {
		return false
	}
	return matchPatternSegments(targetSegments[1:], patternSegments[1:])
}
