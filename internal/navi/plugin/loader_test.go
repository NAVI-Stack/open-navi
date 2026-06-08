package plugin

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	connreg "github.com/open-navi/navi/internal/connectors"
)

func TestLoaderLoadManifestsPrefersWorkspace(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	builtin := t.TempDir()

	writePluginManifest(t, filepath.Join(workspace, "plugins", "demo"), `{
  "id": "plugin.demo",
  "name": "Workspace Demo",
  "version": "1.0.0"
}`)
	writePluginManifest(t, filepath.Join(builtin, "demo"), `{
  "id": "plugin.demo",
  "name": "Builtin Demo",
  "version": "0.1.0"
}`)

	loader := NewLoader(filepath.Join(workspace, "plugins"), "", builtin)
	manifests, err := loader.LoadManifests()
	if err != nil {
		t.Fatalf("LoadManifests: %v", err)
	}
	if len(manifests) != 1 {
		t.Fatalf("expected 1 manifest, got %d", len(manifests))
	}
	if manifests[0].Name != "Workspace Demo" {
		t.Fatalf("expected workspace manifest to win, got %q", manifests[0].Name)
	}
}

func TestLoaderLoadIntoRegistryRegistersConnectorDriver(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	writePluginManifest(t, filepath.Join(workspace, "bridge"), `{
  "id": "connector.bridge.telegram",
  "name": "Telegram Bridge Plugin",
  "categories": ["connector"],
  "connector": {
    "driver_id": "bridge.telegram",
    "display_name": "Telegram Bridge",
    "kind": "remote_bridge",
    "capabilities": [
      {
        "name": "messaging.send",
        "input_schema": {
          "type": "object",
          "properties": {
            "chat_id": { "type": "string" },
            "content": { "type": "string" }
          }
        },
        "side_effect_class": "send",
        "idempotency_class": "non_idempotent",
        "reversibility": "irreversible",
        "risk_hint": "low"
      }
    ]
  }
}`)

	reg := NewRegistry(nil)
	connectorRegistry := connreg.NewRegistry()
	loader := NewLoader(workspace, "", "")

	if err := loader.LoadIntoRegistry(reg, connectorRegistry); err != nil {
		t.Fatalf("LoadIntoRegistry: %v", err)
	}

	if len(reg.Manifests()) != 1 {
		t.Fatalf("expected 1 manifest in plugin registry, got %d", len(reg.Manifests()))
	}

	drivers := connectorRegistry.Drivers()
	if len(drivers) != 1 {
		t.Fatalf("expected 1 connector driver, got %d", len(drivers))
	}
	if drivers[0].DriverID() != "bridge.telegram" {
		t.Fatalf("unexpected driver id: %s", drivers[0].DriverID())
	}
	caps := drivers[0].Capabilities()
	if len(caps) != 1 || caps[0].Name != "messaging.send" {
		t.Fatalf("unexpected capabilities: %+v", caps)
	}
}

func TestLoaderLoadsCanonicalYAMLManifest(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pluginDir := filepath.Join(root, "yaml-demo")
	writePluginSkill(t, filepath.Join(pluginDir, "skills", "hello"))
	writePluginManifestFile(t, pluginDir, "plugin.yaml", `
id: navi.test.yaml
name: YAML Demo
version: 1.0.0
kind: workflow
status: active
components:
  skills:
    - path: skills/hello
      skill_id: hello
`)

	loader := NewLoader(root, "", "")
	manifests, err := loader.LoadManifests()
	if err != nil {
		t.Fatalf("LoadManifests: %v", err)
	}
	if len(manifests) != 1 {
		t.Fatalf("expected 1 manifest, got %d", len(manifests))
	}
	if manifests[0].ID != "navi.test.yaml" || manifests[0].Kind != KindWorkflow {
		t.Fatalf("unexpected manifest: %+v", manifests[0])
	}
	if len(manifests[0].Components.Skills) != 1 || manifests[0].Components.Skills[0].Path != "skills/hello" {
		t.Fatalf("unexpected skill components: %+v", manifests[0].Components.Skills)
	}
}

func TestLoaderParsesPluginUISurfaces(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writePluginManifestFile(t, filepath.Join(root, "ui-demo"), "plugin.yaml", `
id: navi.test.ui
name: UI Demo
version: 1.0.0
kind: workflow
status: active
ui_surfaces:
  - id: ui-demo.console
    title: UI Demo
    target_surfaces: [console]
    render_mode: headless
    actions:
      - id: inspect
        interface: run
`)

	loader := NewLoader(root, "", "")
	manifests, err := loader.LoadManifests()
	if err != nil {
		t.Fatalf("LoadManifests: %v", err)
	}
	if len(manifests) != 1 || len(manifests[0].UISurfaces) != 1 {
		t.Fatalf("expected one manifest with one UI surface, got %#v", manifests)
	}
	surface := manifests[0].UISurfaces[0]
	if surface.RenderMode != "headless" || surface.Actions[0].Interface != "run" {
		t.Fatalf("unexpected UI surface: %#v", surface)
	}
}

func TestLoaderDiagnosesUnknownManifestKind(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writePluginManifestFile(t, filepath.Join(root, "bad-kind"), "plugin.yaml", `
id: navi.test.bad_kind
name: Bad Kind
version: 1.0.0
kind: provider
`)

	loader := NewLoader(root, "", "")
	manifests, err := loader.LoadManifests()
	if err != nil {
		t.Fatalf("expected no error for invalid manifest, got %v", err)
	}
	if len(manifests) != 0 {
		t.Fatalf("expected 0 valid manifests, got %d", len(manifests))
	}
	diags := loader.Diagnostics()
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Status != "invalid" {
		t.Errorf("expected status 'invalid', got %q", diags[0].Status)
	}
	if !strings.Contains(diags[0].Error, `invalid plugin kind "provider"`) {
		t.Errorf("expected diagnostic to mention invalid kind, got %q", diags[0].Error)
	}
}

func TestInvalidManifestDoesNotAbortValidPluginLoading(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	// Valid plugin
	writePluginManifestFile(t, filepath.Join(root, "good"), "plugin.yaml", `
id: navi.test.good
name: Good Plugin
version: 1.0.0
kind: system
`)

	// Invalid plugin (bad kind)
	writePluginManifestFile(t, filepath.Join(root, "bad"), "plugin.yaml", `
id: navi.test.bad
name: Bad Plugin
version: 1.0.0
kind: nosuchkind
`)

	loader := NewLoader(root, "", "")
	manifests, err := loader.LoadManifests()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(manifests) != 1 {
		t.Fatalf("expected 1 valid manifest, got %d", len(manifests))
	}
	if manifests[0].ID != "navi.test.good" {
		t.Errorf("expected good plugin, got %q", manifests[0].ID)
	}
	diags := loader.Diagnostics()
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic for bad plugin, got %d", len(diags))
	}
	if diags[0].PluginID != "navi.test.bad" {
		t.Errorf("expected diagnostic for navi.test.bad, got %q", diags[0].PluginID)
	}
}

func TestRegisterBuiltinDoesNotIncludeTelegramOrSlack(t *testing.T) {
	t.Parallel()

	reg := NewRegistry(nil)
	RegisterBuiltin(reg)

	for _, m := range reg.Manifests() {
		if m.ID == "navi.connector.telegram" {
			t.Error("RegisterBuiltin should not register navi.connector.telegram")
		}
		if m.ID == "navi.connector.slack" {
			t.Error("RegisterBuiltin should not register navi.connector.slack")
		}
	}

	// navi.integration.github must NOT be registered by RegisterBuiltin — plugins/github/plugin.yaml owns that ID.
	for _, m := range reg.Manifests() {
		if m.ID == "navi.integration.github" {
			t.Error("RegisterBuiltin must not register navi.integration.github (collision with plugins/github/plugin.yaml)")
		}
	}

	expected := map[string]bool{
		"navi.domain.calendar": false,
		"navi.workflow.tasks":  false,
		"navi.agentic.critic":  false,
	}
	for _, m := range reg.Manifests() {
		expected[m.ID] = true
	}
	for id, found := range expected {
		if !found {
			t.Errorf("expected builtin plugin %q to be registered", id)
		}
	}
}

func TestTemplatePluginYAMLLoadsUnderRealSchema(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	templatePath := filepath.Join(repoRoot, "plugins", "_template", "plugin.yaml")

	manifest, err := readManifest(templatePath)
	if err != nil {
		t.Fatalf("readManifest(%s): %v", templatePath, err)
	}
	manifest.Normalize("_template", filepath.Join(repoRoot, "plugins", "_template"))
	if err := manifest.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if manifest.ID != "navi.example.messaging" {
		t.Errorf("expected ID navi.example.messaging, got %q", manifest.ID)
	}
	if manifest.IsActive() {
		t.Error("template plugin should not be active (status: disabled)")
	}
	if manifest.TrustTier != "local" {
		t.Errorf("expected trust_tier 'local', got %q", manifest.TrustTier)
	}
}

func TestActivePluginSkillPathsAreActivationGated(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	activeRoot := filepath.Join(root, "active")
	statusDisabledRoot := filepath.Join(root, "status-disabled")
	registryDisabledRoot := filepath.Join(root, "registry-disabled")
	invalidRoot := filepath.Join(root, "invalid")
	for _, dir := range []string{
		filepath.Join(activeRoot, "skills", "active"),
		filepath.Join(activeRoot, "connectors", "active"),
		filepath.Join(activeRoot, "providers", "active"),
		filepath.Join(statusDisabledRoot, "skills", "hidden"),
		filepath.Join(statusDisabledRoot, "connectors", "hidden"),
		filepath.Join(statusDisabledRoot, "providers", "hidden"),
		filepath.Join(registryDisabledRoot, "skills", "hidden"),
		filepath.Join(registryDisabledRoot, "connectors", "hidden"),
		filepath.Join(registryDisabledRoot, "providers", "hidden"),
		filepath.Join(invalidRoot, "skills", "bad"),
		filepath.Join(invalidRoot, "connectors", "bad"),
		filepath.Join(invalidRoot, "providers", "bad"),
	} {
		if strings.Contains(dir, string(filepath.Separator)+"skills"+string(filepath.Separator)) {
			writePluginSkill(t, dir)
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	reg := NewRegistry(nil)
	reg.AddManifest(Manifest{
		ID:      "active",
		Kind:    KindWorkflow,
		Status:  "active",
		RootDir: activeRoot,
		Components: ManifestComponents{
			Skills: []ComponentRef{
				{Path: "skills/active"},
			},
			Connectors: []ComponentRef{
				{Path: "connectors/active", ConnectorID: "active-connector"},
			},
			Providers: []ComponentRef{
				{Path: "providers/active", ProviderID: "active-provider"},
			},
		},
	})
	reg.AddManifest(Manifest{
		ID:      "status-disabled",
		Kind:    KindWorkflow,
		Status:  "disabled",
		RootDir: statusDisabledRoot,
		Components: ManifestComponents{
			Skills: []ComponentRef{
				{Path: "skills/hidden"},
			},
			Connectors: []ComponentRef{
				{Path: "connectors/hidden", ConnectorID: "hidden-connector"},
			},
			Providers: []ComponentRef{
				{Path: "providers/hidden", ProviderID: "hidden-provider"},
			},
		},
	})
	reg.AddManifest(Manifest{
		ID:      "registry-disabled",
		Kind:    KindWorkflow,
		Status:  "active",
		RootDir: registryDisabledRoot,
		Components: ManifestComponents{
			Skills: []ComponentRef{
				{Path: "skills/hidden"},
			},
			Connectors: []ComponentRef{
				{Path: "connectors/hidden", ConnectorID: "registry-hidden-connector"},
			},
			Providers: []ComponentRef{
				{Path: "providers/hidden", ProviderID: "registry-hidden-provider"},
			},
		},
	})
	reg.SetPluginEnabled("registry-disabled", false)
	reg.AddManifest(Manifest{
		ID:      "invalid",
		Kind:    "provider",
		Status:  "active",
		RootDir: invalidRoot,
		Components: ManifestComponents{
			Skills: []ComponentRef{
				{Path: "skills/bad"},
			},
			Connectors: []ComponentRef{
				{Path: "connectors/bad", ConnectorID: "bad-connector"},
			},
			Providers: []ComponentRef{
				{Path: "providers/bad", ProviderID: "bad-provider"},
			},
		},
	})

	paths := reg.ActivePluginSkillPaths()
	if len(paths) != 1 {
		t.Fatalf("expected only active valid plugin paths, got %+v", paths)
	}
	if len(paths["active"]) != 1 || paths["active"][0] != filepath.Join(activeRoot, "skills", "active") {
		t.Fatalf("unexpected active paths: %+v", paths)
	}
	if connectors := reg.ActivePluginConnectorIDs(); len(connectors) != 1 || !connectors["active-connector"] {
		t.Fatalf("expected only active connector ID, got %+v", connectors)
	}
	if providers := reg.ActivePluginProviderIDs(); len(providers) != 1 || !providers["active-provider"] {
		t.Fatalf("expected only active provider ID, got %+v", providers)
	}
}

func TestRepoOwnedPluginManifestsAreCanonicalYAML(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	pluginsRoot := filepath.Join(repoRoot, "plugins")

	loader := &Loader{WorkspaceDir: pluginsRoot}
	manifests, err := loader.LoadManifests()
	if err != nil {
		t.Fatalf("LoadManifests(%s): %v", pluginsRoot, err)
	}
	if len(manifests) == 0 {
		t.Fatal("expected repo-owned plugin manifests")
	}

	err = filepath.WalkDir(pluginsRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrPermission) {
				if d != nil && d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(filepath.Base(path), ".") && path != pluginsRoot {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Base(path) == "plugin.json" {
			t.Fatalf("repo-owned plugin.json should be replaced by plugin.yaml: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk plugins: %v", err)
	}
}

func TestRegisterBuiltinDoesNotCollideWithRepoOwnedManifests(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	pluginsRoot := filepath.Join(repoRoot, "plugins")

	loader := &Loader{WorkspaceDir: pluginsRoot}
	repoManifests, err := loader.LoadManifests()
	if err != nil {
		t.Fatalf("LoadManifests(%s): %v", pluginsRoot, err)
	}

	reg := NewRegistry(nil)
	RegisterBuiltin(reg)

	builtinIDs := make(map[string]bool, len(reg.Manifests()))
	for _, m := range reg.Manifests() {
		builtinIDs[m.ID] = true
	}

	for _, rm := range repoManifests {
		if builtinIDs[rm.ID] {
			t.Errorf("RegisterBuiltin registers manifest ID %q which is already owned by plugins/*/plugin.yaml", rm.ID)
		}
	}

	// Explicit guard for the known-collision IDs.
	for _, forbidden := range []string{"navi.connector.telegram", "navi.connector.slack", "navi.integration.github"} {
		if builtinIDs[forbidden] {
			t.Errorf("RegisterBuiltin must not register %q", forbidden)
		}
	}
}

func TestGitHubPluginActivationRegression(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	githubPluginDir := filepath.Join(repoRoot, "plugins", "github")

	manifest, err := readManifest(filepath.Join(githubPluginDir, "plugin.yaml"))
	if err != nil {
		t.Fatalf("readManifest: %v", err)
	}
	manifest.Normalize("github", githubPluginDir)
	if err := manifest.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	reg := NewRegistry(nil)
	RegisterBuiltin(reg)
	reg.AddManifest(manifest) // simulate LoadIntoRegistry

	if !reg.IsPluginEnabled("navi.integration.github") {
		t.Error("RegisterBuiltin must not disable navi.integration.github")
	}

	paths := reg.ActivePluginSkillPaths()
	githubPaths, ok := paths["navi.integration.github"]
	if !ok || len(githubPaths) == 0 {
		t.Fatalf("ActivePluginSkillPaths: expected paths for navi.integration.github, got %+v", paths)
	}

	tools := reg.Tools()
	found := false
	for _, tool := range tools {
		if tool.Name == IntegrationGitHubRepoToolName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected builtin tool %q to be active via navi.integration.github plugin", IntegrationGitHubRepoToolName)
	}
}

func TestLoaderDiagnosesInvalidWorkflowContractPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pluginDir := filepath.Join(root, "bad-workflow")
	writePluginSkill(t, filepath.Join(pluginDir, "skills", "hello"))
	writePluginManifestFile(t, pluginDir, "plugin.yaml", `
id: navi.test.bad_workflow
name: Bad Workflow Metadata
version: 1.0.0
kind: agentic
status: active
components:
  skills:
    - path: skills/hello
      skill_id: hello
runtime:
  workflow_contracts:
    - workflow_id: navi.test.workflow
      path: ../escape.yaml
`)

	loader := NewLoader(root, "", "")
	manifests, err := loader.LoadManifests()
	if err != nil {
		t.Fatalf("expected no error for invalid manifest, got %v", err)
	}
	if len(manifests) != 0 {
		t.Fatalf("expected invalid workflow metadata to skip manifest, got %d manifests", len(manifests))
	}

	diags := loader.Diagnostics()
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if !strings.Contains(diags[0].Error, "workflow_contracts") {
		t.Fatalf("expected workflow_contracts validation error, got %q", diags[0].Error)
	}
}

func TestNaviProgrammerManifestDeclaresDependenciesAndContractScope(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	pluginDir := filepath.Join(repoRoot, "plugins", "navi-programmer")

	manifest, err := readManifest(filepath.Join(pluginDir, "plugin.yaml"))
	if err != nil {
		t.Fatalf("readManifest: %v", err)
	}
	manifest.Normalize("navi-programmer", pluginDir)
	if err := manifest.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	if len(manifest.Capabilities) == 0 {
		t.Fatal("expected machine-visible capabilities")
	}
	if len(manifest.Dependencies.RequiredCore) == 0 {
		t.Fatal("expected required_core dependency declarations")
	}
	if len(manifest.Dependencies.RequiredTaskSources) == 0 {
		t.Fatal("expected required_task_sources dependency declarations")
	}
	if len(manifest.Contract.WorkflowEntryPoints) == 0 {
		t.Fatal("expected workflow entry points in plugin contract")
	}
	if len(manifest.Contract.ExcludedBehavior) == 0 {
		t.Fatal("expected excluded behavior in plugin contract")
	}
	if manifest.Contract.SelfUpdateMode != "isolated_candidate" {
		t.Fatalf("expected isolated_candidate self-update mode, got %q", manifest.Contract.SelfUpdateMode)
	}
	if len(manifest.RuntimeSpec.WorkflowContracts) < 3 {
		t.Fatalf("expected typed workflow contract metadata, got %+v", manifest.RuntimeSpec.WorkflowContracts)
	}

	var sawTicketWorkflow bool
	for _, workflow := range manifest.RuntimeSpec.WorkflowContracts {
		if workflow.WorkflowID == "" {
			t.Fatalf("expected workflow_id for %+v", workflow)
		}
		if workflow.Path == "" {
			t.Fatalf("expected path for %+v", workflow)
		}
		if workflow.WorkflowID == "navi.programmer.ticket_driven_coding" {
			sawTicketWorkflow = true
			if workflow.Runner != "workflows/bounded_mutation_runner.py" {
				t.Fatalf("expected shared runner for ticket workflow, got %q", workflow.Runner)
			}
		}
	}
	if !sawTicketWorkflow {
		t.Fatal("expected ticket-driven workflow metadata in manifest runtime spec")
	}
}

func writePluginManifest(t *testing.T, dir, content string) {
	t.Helper()
	writePluginManifestFile(t, dir, "plugin.json", content)
}

func writePluginManifestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", dir, err)
	}
}

func writePluginSkill(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.yaml"), []byte(`skill_id: hello
display:
  name: hello
  description: Test skill.
interfaces: []
`), 0o644); err != nil {
		t.Fatalf("WriteFile skill: %v", err)
	}
}
