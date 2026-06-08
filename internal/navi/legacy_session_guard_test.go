package navi_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestLegacySessionGuard is the final safety rail for the Session -> Chat +
// RuntimeSession migration. The allowlist is intentionally narrow:
//   - internal/navi/store/schema.go and its migration tests may name legacy
//     tables/columns while dropping or testing the legacy cleanup.
//   - internal/gateway/openai_compat.go may use "session" only as external
//     protocol vocabulary and must carry the marker checked below.
//   - internal/store/db.go and error_log files keep the temporary
//     error_log.session_id diagnostics island.
//   - this guard names the tokens it forbids.
//
// All active runtime/chat code must use chat_id for durable transcript identity
// and runtime_session_id for execution lifecycle identity.

type legacySessionRule struct {
	name string
	re   *regexp.Regexp
}

var legacySessionGoRules = []legacySessionRule{
	{name: "SessionEntry", re: regexp.MustCompile(`\bSessionEntry\b`)},
	{name: "SessionMessage", re: regexp.MustCompile(`\bSessionMessage\b`)},
	{name: "SessionStore", re: regexp.MustCompile(`\bSessionStore\b`)},
	{name: "ProjectSessionLister", re: regexp.MustCompile(`\bProjectSessionLister\b`)},
	{name: "SessionHistoryReader", re: regexp.MustCompile(`\bSessionHistoryReader\b`)},
	{name: "SessionSummarizer", re: regexp.MustCompile(`\bSessionSummarizer\b`)},
	{name: "CreateSession", re: regexp.MustCompile(`\bCreateSession\b`)},
	{name: "GetSession", re: regexp.MustCompile(`\bGetSession\b`)},
	{name: "RecentSessions", re: regexp.MustCompile(`\bRecentSessions\b`)},
	{name: "RecentProjectSessions", re: regexp.MustCompile(`\bRecentProjectSessions\b`)},
	{name: "RenameSession", re: regexp.MustCompile(`\bRenameSession\b`)},
	{name: "ActiveSessionID", re: regexp.MustCompile(`\bActiveSessionID\b`)},
	{name: "SetActiveSession", re: regexp.MustCompile(`\bSetActiveSession\b`)},
	{name: "CloseSession", re: regexp.MustCompile(`\bCloseSession\b`)},
	{name: "AppendSessionMessage", re: regexp.MustCompile(`\bAppendSessionMessage\b`)},
	{name: "SummarizeSession", re: regexp.MustCompile(`\bSummarizeSession\b`)},
	{name: "sessionID", re: regexp.MustCompile(`\bsessionID\b`)},
	{name: "SessionID", re: regexp.MustCompile(`\bSessionID\b`)},
	{name: "session_id", re: regexp.MustCompile(`\bsession_id\b`)},
	{name: "SessionKind", re: regexp.MustCompile(`\bSessionKind\b`)},
	{name: "HeartbeatAutoSessionID", re: regexp.MustCompile(`\bHeartbeatAutoSessionID\b`)},
	{name: "NormalizeSessionKind", re: regexp.MustCompile(`\bNormalizeSessionKind\b`)},
	{name: "DefaultSessionKindForID", re: regexp.MustCompile(`\bDefaultSessionKindForID\b`)},
	{name: "ResolveSessionKind", re: regexp.MustCompile(`\bResolveSessionKind\b`)},
	{name: "IsInternalSession", re: regexp.MustCompile(`\bIsInternalSession\b`)},
	{name: "IsInternalSessionID", re: regexp.MustCompile(`\bIsInternalSessionID\b`)},
	{name: "navi_sessions", re: regexp.MustCompile(`\bnavi_sessions\b`)},
	{name: "navi_session_memory", re: regexp.MustCompile(`\bnavi_session_memory\b`)},
	{name: "navi_session_checkpoints", re: regexp.MustCompile(`\bnavi_session_checkpoints\b`)},
	{name: "chat_session_compat", re: regexp.MustCompile(`chat_session_compat`)},
}

var legacySessionConsoleRules = []legacySessionRule{
	{name: "session_id", re: regexp.MustCompile(`\bsession_id\b`)},
	{name: "sessionId", re: regexp.MustCompile(`\bsessionId\b`)},
}

var legacySessionGoAllowlist = map[string]string{
	"internal/gateway/openai_compat.go":                           "OpenAI protocol-boundary vocabulary only; must carry marker comment",
	"internal/navi/legacy_session_guard_test.go":                  "this guard names the tokens it forbids",
	"internal/navi/store/legacy_session_column_migration_test.go": "schema cleanup test seeds legacy columns and asserts removal",
	"internal/navi/store/schema.go":                               "schema migration intentionally drops legacy Session tables and columns",
	"internal/navi/store/schema_migration_test.go":                "schema cleanup test seeds legacy tables and asserts removal",
	"internal/store/db.go":                                        "temporary diagnostics island for error_log.session_id and related migrations",
	"internal/store/error_log.go":                                 "temporary diagnostics island for error_log.session_id",
	"internal/store/error_log_test.go":                            "tests the temporary diagnostics island for error_log.session_id",
}

func TestLegacySessionGuardGo(t *testing.T) {
	root := repoRoot(t)
	validateLegacySessionAllowlistMarkers(t, root)

	var violations []string
	for _, relRoot := range []string{"cmd", "connectors", "internal", "plugins"} {
		base := filepath.Join(root, filepath.FromSlash(relRoot))
		if _, err := os.Stat(base); err != nil {
			continue
		}
		err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				switch entry.Name() {
				case ".git", "bin", "node_modules", "vendor", "workspace", "dist", ".pytest_cache":
					return filepath.SkipDir
				}
				return nil
			}
			if filepath.Ext(path) != ".go" {
				return nil
			}
			rel := slashRel(t, root, path)
			if _, ok := legacySessionGoAllowlist[rel]; ok {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, rule := range legacySessionGoRules {
				if rule.re.Match(body) {
					violations = append(violations, rel+": banned legacy token "+rule.name)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", relRoot, err)
		}
	}
	if len(violations) > 0 {
		t.Fatalf("legacy Session tokens remain outside documented allowlist:\n%s", strings.Join(violations, "\n"))
	}
}

func TestLegacySessionGuardConsole(t *testing.T) {
	root := repoRoot(t)
	base := filepath.Join(root, "web-src", "navi-console", "src")
	var violations []string
	err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(path) {
		case ".js", ".ts", ".tsx":
		default:
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel := slashRel(t, root, path)
		for _, rule := range legacySessionConsoleRules {
			if rule.re.Match(body) {
				violations = append(violations, rel+": banned legacy token "+rule.name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk console src: %v", err)
	}
	if len(violations) > 0 {
		t.Fatalf("legacy session tokens remain in console TypeScript/JavaScript:\n%s", strings.Join(violations, "\n"))
	}
}

func validateLegacySessionAllowlistMarkers(t *testing.T, root string) {
	t.Helper()
	openAICompat := filepath.Join(root, "internal", "gateway", "openai_compat.go")
	body, err := os.ReadFile(openAICompat)
	if err != nil {
		t.Fatalf("read openai compat allowlist file: %v", err)
	}
	if !strings.Contains(string(body), "external protocol vocabulary") {
		t.Fatalf("internal/gateway/openai_compat.go is allowlisted but lacks external protocol vocabulary marker")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("resolve repo root from %s: %v", wd, err)
	}
	return root
}

func slashRel(t *testing.T, root, path string) string {
	t.Helper()
	rel, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatalf("relative path for %s: %v", path, err)
	}
	return filepath.ToSlash(rel)
}
