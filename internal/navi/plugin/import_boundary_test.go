package plugin

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInternalPackagesDoNotImportConcretePlugins(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	internalRoot := filepath.Join(repoRoot, "internal")
	forbidden := "github.com/open-navi/navi/" + "plugins/"

	err := filepath.WalkDir(internalRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("internal package imports concrete plugin implementation: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal packages: %v", err)
	}
}
