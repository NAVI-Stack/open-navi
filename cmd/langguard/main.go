// Command langguard is NAVI's language-layer conformance guard.
//
// It mechanically enforces three rules from the Language-Layer Contract
// (docs/architecture/language-layer-contract.md §8) so violations fail CI
// instead of relying on human review:
//
//  1. Forbidden Python imports — no module under python/ may import a store/DB
//     handle, a database driver, NAVI Go internals, or a privileged connector
//     (Contract §3.1 "access raw DB/store internals", §6.3 "Python holds no DB
//     handle and no privileged connector handle").
//
//  2. Ungoverned Python reads — every governed context read must go through the
//     navi SDK's typed query_context(purpose=…, scope=…); no Python file may
//     call the gateway context endpoint directly, and no query_context call may
//     omit purpose or scope (Contract §4 "reads are governed too").
//
//  3. Hand-edited generated contracts — the governed DTOs in
//     schema/python/navi_schema/governed.py and
//     web-src/navi-console/src/types/generated/governed.ts are generated, never
//     hand-written (Contract §7). Enforced as a drift check: regenerate from the
//     canonical Go types and compare; any difference means a generated file was
//     hand-edited (or codegen was not re-run).
//
// Usage:
//
//	langguard [-root <repo>] [-skip-drift]
//
// Exit code 0 = conformant, 1 = at least one violation (each printed with a
// file:line and a one-line fix), 2 = the checker itself could not run.
//
// It builds to bin/ via `make langguard`; never `go build` it to the repo root.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// violation is one contract breach, with enough context to fix it.
type violation struct {
	rule string // short rule id, e.g. "forbidden-import"
	loc  string // file:line (or file) the breach was found at
	msg  string // what is wrong
	fix  string // how to make it conform
}

func (v violation) String() string {
	s := fmt.Sprintf("  [%s] %s\n      %s", v.rule, v.loc, v.msg)
	if v.fix != "" {
		s += "\n      fix: " + v.fix
	}
	return s
}

func main() {
	root := flag.String("root", ".", "repository root to scan")
	skipDrift := flag.String("skip-drift", "", "set to 'true' to skip the generated-contract drift check (e.g. when `go` is unavailable)")
	flag.Parse()

	absRoot, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "langguard: resolve root:", err)
		os.Exit(2)
	}

	var checks = []struct {
		name string
		run  func(string) ([]violation, error)
	}{
		{"forbidden Python imports", checkForbiddenImports},
		{"ungoverned Python reads", checkUngovernedReads},
	}
	if !strings.EqualFold(*skipDrift, "true") {
		checks = append(checks, struct {
			name string
			run  func(string) ([]violation, error)
		}{"generated-contract drift", checkGeneratedDrift})
	}

	var all []violation
	for _, c := range checks {
		vs, err := c.run(absRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "langguard: %s: %v\n", c.name, err)
			os.Exit(2)
		}
		all = append(all, vs...)
	}

	if len(all) == 0 {
		fmt.Println("langguard: OK — language-layer contract upheld (forbidden imports, governed reads, generated contracts).")
		return
	}

	sort.Slice(all, func(i, j int) bool {
		if all[i].rule != all[j].rule {
			return all[i].rule < all[j].rule
		}
		return all[i].loc < all[j].loc
	})
	fmt.Fprintf(os.Stderr, "langguard: %d language-layer violation(s):\n\n", len(all))
	for _, v := range all {
		fmt.Fprintln(os.Stderr, v.String())
		fmt.Fprintln(os.Stderr)
	}
	fmt.Fprintln(os.Stderr, "See docs/architecture/language-layer-contract.md §8 and cmd/langguard/README.md.")
	os.Exit(1)
}

// ---- Rule 1: forbidden Python imports -------------------------------------

// forbiddenImports maps a denied top-level Python module to why it is denied.
// The key is matched against the *first* dotted segment of an imported module
// (so "store.foo" and "store" both trip the "store" rule). Relative imports
// (from . import x) are never denied — they stay inside the package.
var forbiddenImports = map[string]string{
	// Raw store / DB internals (Contract §3.1 "access raw DB/store internals").
	"store": "imports a store/DB handle — Python holds no store internals (Contract §3.1, §6.3)",
	// Database drivers (Contract §6.3 "Python holds no DB handle").
	"sqlite3":    "imports a database driver — Python holds no DB handle (Contract §6.3)",
	"aiosqlite":  "imports a database driver — Python holds no DB handle (Contract §6.3)",
	"psycopg":    "imports a database driver — Python holds no DB handle (Contract §6.3)",
	"psycopg2":   "imports a database driver — Python holds no DB handle (Contract §6.3)",
	"asyncpg":    "imports a database driver — Python holds no DB handle (Contract §6.3)",
	"pymysql":    "imports a database driver — Python holds no DB handle (Contract §6.3)",
	"mysql":      "imports a database driver — Python holds no DB handle (Contract §6.3)",
	"MySQLdb":    "imports a database driver — Python holds no DB handle (Contract §6.3)",
	"pymongo":    "imports a database driver — Python holds no DB handle (Contract §6.3)",
	"redis":      "imports a datastore handle — Python holds no DB handle (Contract §6.3)",
	"sqlalchemy": "imports an ORM/DB layer — Python holds no DB handle (Contract §6.3)",
	// Privileged connectors (Contract §3.1 "call privileged connectors directly").
	"connector":  "imports a privileged connector — Python may not call connectors directly (Contract §3.1, §6.3)",
	"connectors": "imports a privileged connector — Python may not call connectors directly (Contract §3.1, §6.3)",
	// NAVI Go internals reach-through.
	"internal": "imports NAVI Go internals — Python depends on the kernel only via governed surfaces (Contract §6.2)",
	"navid":    "imports the NaviD server — Python may not embed the kernel (Contract §6.2)",
}

func checkForbiddenImports(root string) ([]violation, error) {
	var out []violation
	err := walkPython(root, func(path string, lines []string) {
		rel := relPath(root, path)
		for i, line := range lines {
			mod, ok := importedTopModule(line)
			if !ok {
				continue
			}
			if reason, denied := forbiddenImports[mod]; denied {
				out = append(out, violation{
					rule: "forbidden-import",
					loc:  fmt.Sprintf("%s:%d", rel, i+1),
					msg:  fmt.Sprintf("%q %s", strings.TrimSpace(line), reason),
					fix:  "remove the import; reach the kernel through the navi SDK's governed surfaces, not raw store/DB/connector handles.",
				})
			}
		}
	})
	return out, err
}

// importedTopModule returns the first dotted segment of the module named by a
// Python import statement, and whether the line is an import at all. Relative
// imports (leading dot) return ok=false: they never cross the package boundary.
//
//	"import sqlite3"          -> "sqlite3", true
//	"import a.b as c"         -> "a", true
//	"from store.x import y"   -> "store", true
//	"from . import z"         -> "", false
//	"    x = 1  # import"     -> "", false
func importedTopModule(line string) (string, bool) {
	s := strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(s, "import "):
		s = strings.TrimSpace(s[len("import "):])
	case strings.HasPrefix(s, "from "):
		s = strings.TrimSpace(s[len("from "):])
		// stop at the " import" keyword
		if idx := strings.Index(s, " import"); idx >= 0 {
			s = strings.TrimSpace(s[:idx])
		}
	default:
		return "", false
	}
	if s == "" || strings.HasPrefix(s, ".") {
		return "", false // relative import — in-package, allowed
	}
	// First module in a comma list ("import a, b"), first dotted segment, drop alias.
	if idx := strings.IndexByte(s, ','); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimSpace(s)
	if idx := strings.IndexByte(s, ' '); idx >= 0 { // "a as b"
		s = s[:idx]
	}
	if idx := strings.IndexByte(s, '.'); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimSpace(s), true
}

// ---- Rule 2: ungoverned Python reads --------------------------------------

// contextEndpoint is the governed read route. Only the SDK transport may name it
// literally; any other Python file naming it is reaching through the SDK.
const contextEndpoint = "/api/context/query"

func checkUngovernedReads(root string) ([]violation, error) {
	// The navi SDK package *is* the governed read surface: it defines the
	// endpoint constant, the query_context functions, and documents the route it
	// wraps. Exempt the whole package from the raw-endpoint check — the rule
	// targets consumers that bypass the SDK, not the SDK itself.
	sdkPkg := filepath.Join(root, "python", "navi") + string(filepath.Separator)

	var out []violation
	err := walkPython(root, func(path string, lines []string) {
		rel := relPath(root, path)
		// Test files deliberately assert on the raw path and deliberately call
		// query_context with missing kwargs to prove enforcement — exempt them.
		if isPythonTest(path) {
			return
		}
		text := strings.Join(lines, "\n")

		// 2a. Raw endpoint reach-through.
		if !strings.HasPrefix(path, sdkPkg) && strings.Contains(text, contextEndpoint) {
			line := lineOf(lines, contextEndpoint)
			out = append(out, violation{
				rule: "ungoverned-read",
				loc:  fmt.Sprintf("%s:%d", rel, line),
				msg:  fmt.Sprintf("calls the gateway context endpoint %q directly instead of via the navi SDK", contextEndpoint),
				fix:  "use navi.query_context(run_id=…, purpose=…, scope=…) — the kernel-mediated, purpose-scoped read.",
			})
		}

		// 2b. query_context call sites missing purpose or scope.
		for _, call := range findCalls(text, "query_context") {
			args := call.args
			missing := []string{}
			if !strings.Contains(args, "purpose") {
				missing = append(missing, "purpose")
			}
			if !strings.Contains(args, "scope") {
				missing = append(missing, "scope")
			}
			if len(missing) > 0 {
				out = append(out, violation{
					rule: "ungoverned-read",
					loc:  fmt.Sprintf("%s:%d", rel, lineAt(text, call.start)),
					msg:  fmt.Sprintf("query_context(...) call omits required %s — reads must be purpose- and scope-bound", strings.Join(missing, " and ")),
					fix:  "pass purpose= and scope= keyword arguments (Contract §4).",
				})
			}
		}
	})
	return out, err
}

// call is one function invocation: its argument text and where it started.
type call struct {
	args  string // text inside the outermost parentheses
	start int    // byte offset of the call name in the source
}

// findCalls returns every invocation of name (e.g. "query_context") in src,
// skipping definitions (def / async def name(...)). The returned args is the
// balanced-parenthesis content of each call.
func findCalls(src, name string) []call {
	var calls []call
	needle := name + "("
	for i := 0; ; {
		idx := strings.Index(src[i:], needle)
		if idx < 0 {
			break
		}
		pos := i + idx
		i = pos + len(needle)

		// Skip a definition: "def name(" or "async def name(".
		before := strings.TrimRight(src[:pos], " \t")
		if strings.HasSuffix(before, "def") {
			continue
		}
		// Require a non-identifier char before the name so we don't match a
		// longer identifier ending in name (e.g. "my_query_context(").
		if pos > 0 && isIdentByte(src[pos-1]) {
			continue
		}

		open := pos + len(name) // index of '('
		args, ok := balancedParens(src, open)
		if !ok {
			continue
		}
		calls = append(calls, call{args: args, start: pos})
	}
	return calls
}

// balancedParens returns the text between the '(' at openIdx and its matching
// ')', honoring nesting. ok is false if the parentheses never balance.
func balancedParens(src string, openIdx int) (string, bool) {
	depth := 0
	for j := openIdx; j < len(src); j++ {
		switch src[j] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return src[openIdx+1 : j], true
			}
		}
	}
	return "", false
}

func isIdentByte(b byte) bool {
	return b == '_' ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9')
}

// ---- Rule 3: generated-contract drift -------------------------------------

// generatedContract pairs a committed generated file with the Go generator that
// produces it, so the checker can regenerate and compare.
type generatedContract struct {
	committed string // path (under root) of the committed generated file
	genPkg    string // Go package to `go run` to regenerate it
	label     string // human label
}

func checkGeneratedDrift(root string) ([]violation, error) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return nil, fmt.Errorf("`go` not found on PATH (needed to regenerate governed contracts); install Go or pass -skip-drift=true: %w", err)
	}

	contracts := []generatedContract{
		{
			committed: filepath.Join("schema", "python", "navi_schema", "governed.py"),
			genPkg:    "./schema/python/gen",
			label:     "Python governed contracts",
		},
		{
			committed: filepath.Join("web-src", "navi-console", "src", "types", "generated", "governed.ts"),
			genPkg:    "./schema/ts/gen",
			label:     "TypeScript governed contracts",
		},
	}

	tmpDir, err := os.MkdirTemp("", "langguard-drift-")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	var out []violation
	for _, c := range contracts {
		tmpOut := filepath.Join(tmpDir, filepath.Base(c.committed))
		cmd := exec.Command(goBin, "run", c.genPkg, "-root", ".", "-out", tmpOut)
		cmd.Dir = root
		cmd.Env = os.Environ()
		if combined, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("regenerate %s (%s): %v\n%s", c.label, c.genPkg, err, combined)
		}

		fresh, err := os.ReadFile(tmpOut)
		if err != nil {
			return nil, fmt.Errorf("read regenerated %s: %w", c.label, err)
		}
		committedPath := filepath.Join(root, c.committed)
		current, err := os.ReadFile(committedPath)
		if err != nil {
			out = append(out, violation{
				rule: "generated-drift",
				loc:  c.committed,
				msg:  fmt.Sprintf("%s missing or unreadable: %v", c.label, err),
				fix:  "run `make generate` to (re)create the generated contracts and commit them.",
			})
			continue
		}
		// Compare content, not byte-for-byte: git's autocrlf can hand back a
		// CRLF working copy of an LF-committed generated file (Windows devs),
		// which is not a governed-DTO content change. Normalize line endings so
		// the guard flags real edits, not line-ending churn.
		if normalizeEOL(fresh) != normalizeEOL(current) {
			out = append(out, violation{
				rule: "generated-drift",
				loc:  c.committed,
				msg:  fmt.Sprintf("%s is hand-edited or stale: it differs from `%s` output (governed DTOs are generated, not hand-written)", c.label, c.genPkg),
				fix:  "do not edit generated files; run `make generate`, then commit the regenerated output.",
			})
		}
	}
	return out, nil
}

// ---- shared helpers -------------------------------------------------------

// walkPython invokes fn for every *.py file under <root>/python (skipping
// __pycache__), passing the file's path and its lines.
func walkPython(root string, fn func(path string, lines []string)) error {
	pyRoot := filepath.Join(root, "python")
	if _, err := os.Stat(pyRoot); os.IsNotExist(err) {
		return nil // no python tree yet — nothing to check
	}
	return filepath.Walk(pyRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".py") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fn(path, strings.Split(string(data), "\n"))
		return nil
	})
}

// isPythonTest reports whether a path is a Python test file or lives under a
// tests/ directory. Test files legitimately exercise rejection paths.
func isPythonTest(path string) bool {
	base := filepath.Base(path)
	if strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") {
		return true
	}
	return strings.Contains(filepath.ToSlash(path), "/tests/")
}

// normalizeEOL strips carriage returns so CRLF and LF copies of the same
// content compare equal.
func normalizeEOL(b []byte) string {
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

func relPath(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

// lineOf returns the 1-based line number of the first line containing sub, or 1.
func lineOf(lines []string, sub string) int {
	for i, l := range lines {
		if strings.Contains(l, sub) {
			return i + 1
		}
	}
	return 1
}

// lineAt returns the 1-based line number of the byte offset pos in src.
func lineAt(src string, pos int) int {
	if pos > len(src) {
		pos = len(src)
	}
	return strings.Count(src[:pos], "\n") + 1
}
