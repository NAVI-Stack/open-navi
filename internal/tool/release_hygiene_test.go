package tool

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTrackedReleaseArtifactsStayOutOfTree(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoRoot := releaseHygieneRepoRoot(t)
	cmd := exec.Command("git", "ls-files", "--", ".claude", "scratch")
	cmd.Dir = repoRoot
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}

	allowlist := map[string]struct{}{
		".claude/settings.json": {},
	}

	var offenders []string
	for _, entry := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		entry = filepath.ToSlash(strings.TrimSpace(entry))
		if entry == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(entry))); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatalf("stat %s: %v", entry, err)
		}
		if _, ok := allowlist[entry]; ok {
			continue
		}
		if strings.HasPrefix(entry, "scratch/") {
			offenders = append(offenders, entry)
			continue
		}
		if strings.HasPrefix(entry, ".claude/") && strings.Contains(entry, ".local.") {
			offenders = append(offenders, entry)
		}
	}

	if len(offenders) > 0 {
		t.Fatalf("tracked non-shipping release artifacts detected: %v", offenders)
	}
}

func releaseHygieneRepoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repo root from %s", file)
		}
		dir = parent
	}
}
