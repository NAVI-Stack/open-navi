package policy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ceoai/navi/internal/schema"
	"gopkg.in/yaml.v3"
)

// LoadDir reads per-connector policy YAML files from dir (e.g.
// config/connectors/). Each file is named "<driver>.yaml" and contains a
// SyncPolicy; the file overlays the Go default for that connector. A missing
// directory is not an error (defaults apply). Returns a map keyed by the
// connector_id declared in each file (falling back to the file's base name).
func LoadDir(dir string) (map[string]SyncPolicy, error) {
	out := map[string]SyncPolicy{}
	if strings.TrimSpace(dir) == "" {
		return out, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, fmt.Errorf("policy: read dir %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("policy: read %s: %w", path, err)
		}
		var p SyncPolicy
		if err := yaml.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("policy: parse %s: %w", path, err)
		}
		key := p.ConnectorID
		if key == "" {
			key = strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
			p.ConnectorID = key
		}
		// Overlay the file onto the Go default so partial files are valid.
		merged := merge(DefaultPolicy(key), p)
		if err := Validate(merged); err != nil {
			return nil, fmt.Errorf("policy: invalid %s: %w", path, err)
		}
		out[key] = merged
	}
	return out, nil
}

// fileKeyFor returns the policy map key that should apply to a connector id.
// A file may be keyed by a full instance id ("telegram:acct-1") or by the bare
// driver ("telegram"); the instance id wins when both exist.
func fileKeyFor(files map[string]SyncPolicy, connectorID string) (SyncPolicy, bool) {
	if p, ok := files[connectorID]; ok {
		return p, true
	}
	if p, ok := files[driverOf(connectorID)]; ok {
		return p, true
	}
	return schema.SyncPolicy{}, false
}
