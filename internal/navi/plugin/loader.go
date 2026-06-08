package plugin

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/ceoai/navi/connectors"
	"github.com/ceoai/navi/internal/bus"
	"github.com/ceoai/navi/internal/config"
	connreg "github.com/ceoai/navi/internal/connectors"
	"gopkg.in/yaml.v3"
)

var pluginManifestFilenames = []string{
	"plugin.yaml",
	"plugin.yml",
	"navi.plugin.json",
	"plugin.json",
	"manifest.json",
}

// Loader discovers and loads plugins from configured paths.
type Loader struct {
	WorkspaceDir string
	GlobalDir    string
	BuiltinDir   string
	diagnostics  []ManifestDiagnostic
}

// NewLoader creates a loader with the given search paths (workspace, global e.g. ~/.navi/plugins, builtin).
func NewLoader(workspaceDir, globalDir, builtinDir string) *Loader {
	if globalDir == "" {
		if h, _ := os.UserHomeDir(); h != "" {
			globalDir = filepath.Join(h, ".navi", "plugins")
		}
	}
	return &Loader{
		WorkspaceDir: workspaceDir,
		GlobalDir:    globalDir,
		BuiltinDir:   builtinDir,
	}
}

// Diagnostics returns diagnostic entries for plugin manifests that were
// skipped during the most recent LoadManifests call.
func (l *Loader) Diagnostics() []ManifestDiagnostic {
	return l.diagnostics
}

// ListPluginPaths returns paths that may contain plugins (directories to scan). Order: workspace, global, builtin.
func (l *Loader) ListPluginPaths() []string {
	var out []string
	for _, d := range []string{l.WorkspaceDir, l.GlobalDir, l.BuiltinDir} {
		if strings.TrimSpace(d) != "" {
			out = append(out, d)
		}
	}
	return out
}

// LoadManifests scans the configured plugin roots and returns discovered manifests.
// Priority order is workspace -> global -> builtin; earlier matches win on duplicate IDs.
// Invalid manifests are tracked as diagnostics rather than aborting the load.
func (l *Loader) LoadManifests() ([]Manifest, error) {
	l.diagnostics = nil
	seen := make(map[string]bool)
	var manifests []Manifest
	for _, root := range l.ListPluginPaths() {
		list, diags, err := discoverManifestsInRoot(root)
		if err != nil {
			return nil, err
		}
		l.diagnostics = append(l.diagnostics, diags...)
		for _, manifest := range list {
			if manifest.ID == "" || seen[manifest.ID] {
				continue
			}
			seen[manifest.ID] = true
			manifests = append(manifests, manifest)
		}
	}
	return manifests, nil
}

// LoadIntoRegistry discovers plugin manifests on disk and registers them with
// the shared plugin registry. Connector plugin manifests also register a
// connector driver in the connector registry when provided.
// Invalid manifests are logged as warnings but do not prevent valid plugins from loading.
func (l *Loader) LoadIntoRegistry(reg *Registry, connectorRegistry *connreg.Registry) error {
	if reg == nil {
		return nil
	}
	manifests, err := l.LoadManifests()
	if err != nil {
		return err
	}
	for _, d := range l.diagnostics {
		log.Printf("plugin: skipped manifest %s: %s", d.Path, d.Error)
	}
	for _, manifest := range manifests {
		reg.AddManifest(manifest)
		if connectorRegistry != nil && manifest.Connector != nil && manifest.IsActive() && strings.ToLower(strings.TrimSpace(manifest.Connector.Kind)) != "builtin" {
			connectorRegistry.RegisterDriver(DriverFromManifest(manifest))
		}
	}
	return nil
}

func discoverManifestsInRoot(root string) ([]Manifest, []ManifestDiagnostic, error) {
	if root == "" {
		return nil, nil, nil
	}
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("plugin loader: stat %s: %w", root, err)
	}

	var manifests []Manifest
	var diags []ManifestDiagnostic
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}

		manifestPath, ok := resolveManifestPath(path)
		if !ok {
			return nil
		}
		manifest, err := readManifest(manifestPath)
		if err != nil {
			log.Printf("plugin: skipping invalid manifest %s: %v", manifestPath, err)
			diags = append(diags, ManifestDiagnostic{
				Path:   manifestPath,
				Status: "parse_error",
				Error:  err.Error(),
			})
			if path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if manifest.DateAdded.IsZero() {
			if info, statErr := os.Stat(manifestPath); statErr == nil {
				manifest.DateAdded = info.ModTime()
			}
		}
		manifest.Normalize(filepath.Base(path), path)
		if err := manifest.Validate(); err != nil {
			log.Printf("plugin: skipping invalid manifest %s: %v", manifestPath, err)
			diags = append(diags, ManifestDiagnostic{
				Path:     manifestPath,
				PluginID: manifest.ID,
				Status:   "invalid",
				Error:    err.Error(),
			})
			if path != root {
				return filepath.SkipDir
			}
			return nil
		}
		manifests = append(manifests, manifest)
		if path != root {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, diags, err
	}
	return manifests, diags, nil
}

func resolveManifestPath(dir string) (string, bool) {
	for _, candidate := range pluginManifestFilenames {
		path := filepath.Join(dir, candidate)
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}
	return "", false
}

func readManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("plugin loader: read manifest %s: %w", path, err)
	}
	var manifest Manifest
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &manifest); err != nil {
			return Manifest{}, fmt.Errorf("plugin loader: parse manifest %s: %w", path, err)
		}
	default:
		if err := json.Unmarshal(data, &manifest); err != nil {
			return Manifest{}, fmt.Errorf("plugin loader: parse manifest %s: %w", path, err)
		}
	}
	return manifest, nil
}

type manifestDriver struct {
	manifest Manifest
}

func (d manifestDriver) DriverID() string {
	if d.manifest.Connector != nil && d.manifest.Connector.DriverID != "" {
		return d.manifest.Connector.DriverID
	}
	return d.manifest.ID
}

func (d manifestDriver) Kind() string {
	if d.manifest.Connector != nil && d.manifest.Connector.Kind != "" {
		return d.manifest.Connector.Kind
	}
	return "plugin"
}

func (d manifestDriver) DisplayName() string {
	if d.manifest.Connector != nil && d.manifest.Connector.DisplayName != "" {
		return d.manifest.Connector.DisplayName
	}
	if d.manifest.Name != "" {
		return d.manifest.Name
	}
	return d.manifest.ID
}

func (d manifestDriver) SetupDescriptor() connreg.SetupDescriptor {
	if d.manifest.Connector == nil || d.manifest.Connector.Setup == nil {
		return connreg.SetupDescriptor{
			Type:        d.DriverID(),
			DisplayName: d.DisplayName(),
		}
	}
	s := d.manifest.Connector.Setup
	out := connreg.SetupDescriptor{
		Type:           d.DriverID(),
		DisplayName:    d.DisplayName(),
		RequiredParams: make([]connreg.SetupParam, len(s.RequiredParams)),
		OptionalParams: make([]connreg.SetupParam, len(s.OptionalParams)),
		SetupHint:      s.SetupHint,
	}
	for i, p := range s.RequiredParams {
		out.RequiredParams[i] = connreg.SetupParam{
			Key:         p.Key,
			Label:       p.Label,
			Description: p.Description,
			Secret:      p.Secret,
			Placeholder: p.Placeholder,
		}
	}
	for i, p := range s.OptionalParams {
		out.OptionalParams[i] = connreg.SetupParam{
			Key:         p.Key,
			Label:       p.Label,
			Description: p.Description,
			Secret:      p.Secret,
			Placeholder: p.Placeholder,
		}
	}
	return out
}

func (d manifestDriver) CreateInstance(cfg *config.Config, b bus.Bus, params map[string]string) (connectors.Connector, error) {
	if d.manifest.Connector == nil {
		return nil, fmt.Errorf("plugin %s has no connector manifest", d.manifest.ID)
	}

	switch d.Kind() {
	case "subprocess":
		if d.manifest.Connector.Command == "" {
			return nil, fmt.Errorf("subprocess connector %s missing command", d.manifest.ID)
		}
		// In a real implementation we'd probably resolve command relative to plugin dir.
		return connreg.NewSubprocessConnector(d.manifest.ID, d.manifest.Connector.Command, d.manifest.Connector.Args, nil), nil
	case "bridge":
		// GatewayBridgeConnector currently needs a callback URL and token.
		// These could be passed in params.
		bridgeCfg := connreg.BridgeConfig{
			Name:        d.manifest.ID,
			CallbackURL: params["callback_url"],
			Token:       params["token"],
		}
		return connreg.NewGatewayBridgeConnector(bridgeCfg), nil
	default:
		return nil, fmt.Errorf("plugin %s: unsupported connector kind %q", d.manifest.ID, d.Kind())
	}
}

func (d manifestDriver) Capabilities() []connreg.CapabilityDescriptor {
	if d.manifest.Connector == nil {
		return nil
	}
	out := make([]connreg.CapabilityDescriptor, 0, len(d.manifest.Connector.Capabilities))
	for _, capability := range d.manifest.Connector.Capabilities {
		out = append(out, connreg.CapabilityDescriptor{
			Name:                capability.Name,
			InputSchema:         capability.inputSchemaJSON(),
			OutputSchema:        capability.outputSchemaJSON(),
			SideEffectClass:     capability.SideEffectClass,
			IdempotencyClass:    capability.IdempotencyClass,
			Reversibility:       capability.Reversibility,
			RiskHint:            capability.RiskHint,
			RequiredPermissions: capability.RequiredPermissions,
			SupportsDryRun:      capability.SupportsDryRun,
			SupportsRetry:       capability.SupportsRetry,
		})
	}
	return out
}

func DriverFromManifest(manifest Manifest) connreg.ConnectorDriver {
	return manifestDriver{manifest: manifest}
}
