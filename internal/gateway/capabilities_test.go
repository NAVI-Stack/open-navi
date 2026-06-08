package gateway

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	pkgconn "github.com/open-navi/navi/connectors"
	"github.com/open-navi/navi/internal/bus"
	"github.com/open-navi/navi/internal/capability"
	"github.com/open-navi/navi/internal/cognitive"
	"github.com/open-navi/navi/internal/config"
	intconnectors "github.com/open-navi/navi/internal/connectors"
	"github.com/open-navi/navi/internal/governor"
	"github.com/open-navi/navi/internal/navi/plugin"
	"github.com/open-navi/navi/internal/navi/skill"
	"github.com/open-navi/navi/internal/schema"
	navitool "github.com/open-navi/navi/internal/tool"
)

func TestCapabilityGraphEndpointReturnsInventoryShape(t *testing.T) {
	db := testDB(t)
	skillRegistry := capabilityTestSkillRegistry(t)
	toolRegistry := navitool.NewRegistry()
	const registeredToolName = "skill.test-skill.run"
	if err := toolRegistry.Register(gatewayTestTool(registeredToolName, "Run test skill.", navitool.ToolGovernance{
		CommandType: schema.CommandTypeQuery,
		Domain:      "testing",
	})); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	connectorRegistry := intconnectors.NewRegistry()
	connectorRegistry.RegisterInstance("odd-status", newTypingTestConnector("odd-status"))

	srv := NewServer(Config{
		DB:              db,
		DirectiveWriter: cognitive.StoreDirectiveWriter(db),
		Governor:        governor.NewGovernor(governor.GovernorConfig{}, "."),
		Registry:        connectorRegistry,
		SkillRegistry:   skillRegistry,
		ToolRegistry:    toolRegistry,
		PluginManifests: func() []plugin.Manifest {
			return []plugin.Manifest{{
				ID:          "test.plugin",
				Name:        "Test Plugin",
				Description: "Owns the test skill.",
				Version:     "1.2.3",
				Kind:        plugin.KindDomain,
				Status:      "active",
				TrustTier:   "builtin",
				Capabilities: []string{
					"testing.run",
				},
				Components: plugin.ManifestComponents{
					Skills: []plugin.ComponentRef{{
						Path:    "skills/test-skill",
						SkillID: "test-skill",
					}},
				},
			}}
		},
		Addr: ":0",
	})

	res := doJSONReq(t, srv, http.MethodGet, "/api/capabilities/graph", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/capabilities/graph: expected 200, got %d", res.StatusCode)
	}
	var graph CapabilityGraph
	if err := json.NewDecoder(res.Body).Decode(&graph); err != nil {
		t.Fatalf("decode graph: %v", err)
	}

	if graph.Plugins == nil || graph.Skills == nil || graph.ToolInterfaces == nil || graph.Connectors == nil || graph.UISurfaces == nil || graph.Docs == nil || graph.Edges == nil {
		t.Fatalf("expected all top-level arrays to be present, got %#v", graph)
	}
	if !hasPluginNode(graph, "plugin:test.plugin") {
		t.Fatalf("expected test plugin node, got %#v", graph.Plugins)
	}
	skillNode, ok := findSkillNode(graph, "skill:test-skill")
	if !ok {
		t.Fatalf("expected test skill node, got %#v", graph.Skills)
	}
	if skillNode.ParentPluginID != "test.plugin" {
		t.Fatalf("expected skill parentPluginId test.plugin, got %q", skillNode.ParentPluginID)
	}
	if len(graph.UISurfaces) != 0 {
		t.Fatalf("expected V1 uiSurfaces to be empty, got %#v", graph.UISurfaces)
	}
	if len(graph.Docs) != 0 {
		t.Fatalf("expected V1 docs to be empty, got %#v", graph.Docs)
	}
	if !hasEdge(graph, "plugin:test.plugin", "skill:test-skill", "plugin_owns_skill") {
		t.Fatalf("expected plugin_owns_skill edge, got %#v", graph.Edges)
	}

	ti, ok := findToolInterfaceNode(graph, "tool_interface:test-skill.run")
	if !ok {
		t.Fatalf("expected tool interface node for test-skill.run, got %#v", graph.ToolInterfaces)
	}
	if ti.CanonicalInterfaceID != "test-skill.run" {
		t.Fatalf("canonicalInterfaceId = %q", ti.CanonicalInterfaceID)
	}
	if ti.RegisteredToolName != registeredToolName {
		t.Fatalf("registeredToolName = %q", ti.RegisteredToolName)
	}
	if ti.ToolID != registeredToolName {
		t.Fatalf("toolId = %q", ti.ToolID)
	}
	if !hasEdge(graph, "skill:test-skill", "tool_interface:test-skill.run", "skill_exposes_interface") {
		t.Fatalf("expected skill_exposes_interface edge, got %#v", graph.Edges)
	}
	if len(graph.Connectors) == 0 {
		t.Fatal("expected connector inventory to include registered connector instance")
	}
	assertGraphStatusVocabulary(t, graph)
}

func TestCapabilityGraphConnectorStatusMissingFieldsDoNotPanic(t *testing.T) {
	connectorRegistry := intconnectors.NewRegistry()
	connectorRegistry.RegisterFactory("factory-only", func(*config.Config, bus.Bus) (pkgconn.Connector, error) {
		return newTypingTestConnector("factory-only"), nil
	})

	srv := NewServer(Config{
		DB:       testDB(t),
		Registry: connectorRegistry,
		Addr:     ":0",
	})
	res := doJSONReq(t, srv, http.MethodGet, "/api/capabilities/graph", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/capabilities/graph: expected 200, got %d", res.StatusCode)
	}
	var graph CapabilityGraph
	if err := json.NewDecoder(res.Body).Decode(&graph); err != nil {
		t.Fatalf("decode graph: %v", err)
	}
	if len(graph.Connectors) != 1 {
		t.Fatalf("expected one connector driver, got %#v", graph.Connectors)
	}
	assertGraphStatusVocabulary(t, graph)
}

func TestCapabilityGraphProjectsUISurfacesDocsAndReadOnlyUIEndpoints(t *testing.T) {
	skillRegistry := capabilityTestSkillRegistryWithYAML(t, `oss27_version: "1.0"
skill_id: "ui-skill"
semver: "0.1.0"
display:
  name: "UI Skill"
  description: "A skill with a headless UI surface."
interfaces:
  - name: "run"
    transport:
      type: "internal"
    input_schema:
      type: "object"
    output_schema:
      type: "object"
effects:
  side_effects: []
  risk_tier: "low"
  requires_confirmation: false
security:
  auth: []
  data_access:
    pii: "none"
    secrets: "forbidden"
  sandbox:
    required: false
    network_egress: []
governance:
  publisher: "navi.test"
  signed: false
  trust_tier: "local"
ui_surfaces:
  - id: "ui-skill.console"
    title: "UI Skill"
    target_surfaces: ["console"]
    render_mode: "headless"
    schema:
      type: "panel"
    actions:
      - id: "run"
        interface: "run"
`)
	pluginRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(pluginRoot, "README.md"), []byte("# UI Plugin\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	srv := NewServer(Config{
		DB:            testDB(t),
		SkillRegistry: skillRegistry,
		PluginManifests: func() []plugin.Manifest {
			return []plugin.Manifest{{
				ID:      "ui.plugin",
				Name:    "UI Plugin",
				Kind:    plugin.KindDomain,
				Status:  "active",
				RootDir: pluginRoot,
				Components: plugin.ManifestComponents{
					Skills: []plugin.ComponentRef{{
						Path:    "skills/ui-skill",
						SkillID: "ui-skill",
					}},
				},
			}, {
				ID:     "unsupported.ui",
				Name:   "Unsupported UI",
				Kind:   plugin.KindDomain,
				Status: "active",
				UISurfaces: []capability.UISurfaceSpec{{
					ID:             "unsupported.console",
					Title:          "Unsupported",
					TargetSurfaces: []string{"console"},
					RenderMode:     "react",
				}},
			}}
		},
		Addr: ":0",
	})

	res := doJSONReq(t, srv, http.MethodGet, "/api/capabilities/graph", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET graph: expected 200, got %d", res.StatusCode)
	}
	var graph CapabilityGraph
	if err := json.NewDecoder(res.Body).Decode(&graph); err != nil {
		t.Fatalf("decode graph: %v", err)
	}
	surface, ok := findUISurfaceNode(graph, "ui_surface:skill:ui-skill:ui-skill.console")
	if !ok {
		t.Fatalf("expected skill UI surface, got %#v", graph.UISurfaces)
	}
	if surface.Status.UI != capUIAvailable || surface.RenderMode != "headless" {
		t.Fatalf("unexpected skill surface status: %#v", surface)
	}
	if !hasEdge(graph, "skill:ui-skill", surface.ID, "component_provides_ui") {
		t.Fatalf("expected component_provides_ui edge, got %#v", graph.Edges)
	}
	unsupported, ok := findUISurfaceNode(graph, "ui_surface:plugin:unsupported.ui:unsupported.console")
	if !ok {
		t.Fatalf("expected unsupported plugin UI surface, got %#v", graph.UISurfaces)
	}
	if unsupported.Status.UI != capUIUnsupportedSurface {
		t.Fatalf("unsupported UI surface status = %#v", unsupported.Status)
	}
	if len(graph.Docs) == 0 || !hasEdge(graph, "plugin:ui.plugin", graph.Docs[0].ID, "plugin_declares_doc") {
		t.Fatalf("expected plugin doc node and edge, docs=%#v edges=%#v", graph.Docs, graph.Edges)
	}

	uiRes := doJSONReq(t, srv, http.MethodGet, "/api/skill-ui?surface=console", "", "127.0.0.1:1234", nil)
	if uiRes.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/skill-ui: expected 200, got %d", uiRes.StatusCode)
	}
	var allSurfaces []UISurfaceNode
	if err := json.NewDecoder(uiRes.Body).Decode(&allSurfaces); err != nil {
		t.Fatalf("decode skill-ui: %v", err)
	}
	if len(allSurfaces) != 2 {
		t.Fatalf("expected two console UI surfaces, got %#v", allSurfaces)
	}

	skillUIRes := doJSONReq(t, srv, http.MethodGet, "/api/skills/ui-skill/ui?surface=console", "", "127.0.0.1:1234", nil)
	if skillUIRes.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/skills/{id}/ui: expected 200, got %d", skillUIRes.StatusCode)
	}
	var skillSurfaces []UISurfaceNode
	if err := json.NewDecoder(skillUIRes.Body).Decode(&skillSurfaces); err != nil {
		t.Fatalf("decode skill ui: %v", err)
	}
	if len(skillSurfaces) != 1 || skillSurfaces[0].OwnerID != "ui-skill" {
		t.Fatalf("expected filtered skill UI surface, got %#v", skillSurfaces)
	}
}

func TestCapabilityGraphNAVIContactsFixtureIncludesUISurface(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	pluginRoot := filepath.Join(repoRoot, "plugins", "navi-contacts")
	reg := skill.NewRegistry(t.TempDir())
	reg.SetPluginSkillSource(staticPluginSkillSource{paths: map[string][]string{
		"navi.contacts": {filepath.Join(pluginRoot, "skills", "navi-contacts")},
	}})
	if err := reg.Load(); err != nil {
		t.Fatalf("load NAVI Contacts skill fixture: %v", err)
	}
	srv := NewServer(Config{
		DB:            testDB(t),
		SkillRegistry: reg,
		PluginManifests: func() []plugin.Manifest {
			return []plugin.Manifest{{
				ID:        "navi.contacts",
				Name:      "NAVI Contacts",
				Kind:      plugin.KindDomain,
				Status:    "active",
				TrustTier: "builtin",
				RootDir:   pluginRoot,
				Components: plugin.ManifestComponents{
					Skills: []plugin.ComponentRef{{
						Path:    "skills/navi-contacts",
						SkillID: "navi-contacts",
					}},
				},
			}}
		},
		Addr: ":0",
	})
	res := doJSONReq(t, srv, http.MethodGet, "/api/capabilities/graph", "", "127.0.0.1:1234", nil)
	var graph CapabilityGraph
	if err := json.NewDecoder(res.Body).Decode(&graph); err != nil {
		t.Fatalf("decode graph: %v", err)
	}
	if !hasPluginNode(graph, "plugin:navi.contacts") || !hasSkillNode(graph, "skill:navi-contacts") {
		t.Fatalf("expected NAVI Contacts plugin and skill nodes, got plugins=%#v skills=%#v", graph.Plugins, graph.Skills)
	}
	if _, ok := findUISurfaceNode(graph, "ui_surface:skill:navi-contacts:contacts.console"); !ok {
		t.Fatalf("expected NAVI Contacts UI surface, got %#v", graph.UISurfaces)
	}
}

func TestCapabilityGraphConnectorDiagnosticsBecomeStatusReasons(t *testing.T) {
	connectorRegistry := intconnectors.NewRegistry()
	connectorRegistry.RegisterInstance("diag-test", newTypingTestConnector("diag-test"))
	manager := intconnectors.NewManager(connectorRegistry, intconnectors.ManagerConfig{})
	manager.Diag.Push(intconnectors.DiagError, "diag-test", "connector auth failed", nil)
	srv := NewServer(Config{
		DB:       testDB(t),
		Registry: connectorRegistry,
		Manager:  manager,
		Addr:     ":0",
	})
	res := doJSONReq(t, srv, http.MethodGet, "/api/capabilities/graph", "", "127.0.0.1:1234", nil)
	var graph CapabilityGraph
	if err := json.NewDecoder(res.Body).Decode(&graph); err != nil {
		t.Fatalf("decode graph: %v", err)
	}
	connector, ok := findConnectorNode(graph, "connector:diag-test")
	if !ok {
		t.Fatalf("expected connector node, got %#v", graph.Connectors)
	}
	if connector.Status.Health != capHealthFailing {
		t.Fatalf("expected failing health from diagnostic, got %#v", connector.Status)
	}
	if len(connector.StatusReasons) == 0 {
		t.Fatalf("expected diagnostic status reasons, got %#v", connector)
	}
}

func TestCapabilityStatusNormalizationHelpers(t *testing.T) {
	runtimeCases := map[string]string{
		"connected": capRuntimeRunning,
		"down":      capRuntimeStopped,
		"degraded":  capRuntimeDegraded,
		"":          capRuntimeUnknown,
	}
	for raw, want := range runtimeCases {
		if got := normalizeRuntimeStatus(raw); got != want {
			t.Fatalf("normalizeRuntimeStatus(%q) = %q, want %q", raw, got, want)
		}
	}
	authCases := map[string]string{
		"configured":   capAuthConfigured,
		"unconfigured": capAuthUnconfigured,
		"expired":      capAuthExpired,
		"failed":       capAuthError,
		"none":         capAuthNotRequired,
		"":             capAuthUnconfigured,
	}
	for raw, want := range authCases {
		if got := normalizeAuthStatus(raw); got != want {
			t.Fatalf("normalizeAuthStatus(%q) = %q, want %q", raw, got, want)
		}
	}
	healthCases := map[string]string{
		"healthy":  capHealthHealthy,
		"degraded": capHealthWarning,
		"down":     capHealthFailing,
		"":         capHealthUnknown,
	}
	for raw, want := range healthCases {
		if got := normalizeHealthStatus(raw); got != want {
			t.Fatalf("normalizeHealthStatus(%q) = %q, want %q", raw, got, want)
		}
	}
}

type staticPluginSkillSource struct {
	paths map[string][]string
}

func (s staticPluginSkillSource) ActivePluginSkillPaths() map[string][]string {
	return s.paths
}

func capabilityTestSkillRegistry(t *testing.T) *skill.SkillRegistry {
	t.Helper()
	skillsRoot := filepath.Join(t.TempDir(), "skills")
	skillDir := filepath.Join(skillsRoot, "test-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	skillYAML := `oss27_version: "1.0"
skill_id: "test-skill"
semver: "0.1.0"
display:
  name: "Test Skill"
  description: "A test skill for capability graph coverage."
interfaces:
  - name: "run"
    description: "Run the test skill."
    transport:
      type: "internal"
    input_schema:
      type: "object"
      additionalProperties: false
    output_schema:
      type: "object"
      properties:
        ok:
          type: "boolean"
effects:
  side_effects: []
  risk_tier: "low"
  requires_confirmation: false
security:
  auth: []
  data_access:
    pii: "none"
    secrets: "forbidden"
  sandbox:
    required: false
    network_egress: []
performance:
  expected_p50_ms: 50
  timeout_ms: 1000
observability:
  log_redaction: []
  emit_metrics: []
governance:
  publisher: "navi.test"
  signed: false
  trust_tier: "local"
capability:
  tags: ["test"]
  domains: ["testing"]
  provides: ["testing.run"]
  command_type: "query"
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.yaml"), []byte(skillYAML), 0o644); err != nil {
		t.Fatalf("write skill yaml: %v", err)
	}
	reg := skill.NewRegistry(skillsRoot)
	if err := reg.Load(); err != nil {
		t.Fatalf("load skills: %v", err)
	}
	return reg
}

func capabilityTestSkillRegistryWithYAML(t *testing.T, skillYAML string) *skill.SkillRegistry {
	t.Helper()
	skillsRoot := filepath.Join(t.TempDir(), "skills")
	skillDir := filepath.Join(skillsRoot, "ui-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.yaml"), []byte(skillYAML), 0o644); err != nil {
		t.Fatalf("write skill yaml: %v", err)
	}
	reg := skill.NewRegistry(skillsRoot)
	if err := reg.Load(); err != nil {
		t.Fatalf("load skills: %v", err)
	}
	return reg
}

func hasPluginNode(graph CapabilityGraph, id string) bool {
	for _, node := range graph.Plugins {
		if node.ID == id {
			return true
		}
	}
	return false
}

func hasSkillNode(graph CapabilityGraph, id string) bool {
	_, ok := findSkillNode(graph, id)
	return ok
}

func findSkillNode(graph CapabilityGraph, id string) (SkillNode, bool) {
	for _, node := range graph.Skills {
		if node.ID == id {
			return node, true
		}
	}
	return SkillNode{}, false
}

func findToolInterfaceNode(graph CapabilityGraph, id string) (ToolInterfaceNode, bool) {
	for _, node := range graph.ToolInterfaces {
		if node.ID == id {
			return node, true
		}
	}
	return ToolInterfaceNode{}, false
}

func findConnectorNode(graph CapabilityGraph, id string) (ConnectorNode, bool) {
	for _, node := range graph.Connectors {
		if node.ID == id {
			return node, true
		}
	}
	return ConnectorNode{}, false
}

func findUISurfaceNode(graph CapabilityGraph, id string) (UISurfaceNode, bool) {
	for _, node := range graph.UISurfaces {
		if node.ID == id {
			return node, true
		}
	}
	return UISurfaceNode{}, false
}

func hasEdge(graph CapabilityGraph, from, to, kind string) bool {
	for _, edge := range graph.Edges {
		if edge.From == from && edge.To == to && edge.Kind == kind {
			return true
		}
	}
	return false
}

func assertGraphStatusVocabulary(t *testing.T, graph CapabilityGraph) {
	t.Helper()
	check := func(id string, status CapabilityStatus) {
		if !allowedLifecycle[status.Lifecycle] {
			t.Fatalf("%s lifecycle %q is not normalized", id, status.Lifecycle)
		}
		if !allowedValidation[status.Validation] {
			t.Fatalf("%s validation %q is not normalized", id, status.Validation)
		}
		if !allowedAvailability[status.Availability] {
			t.Fatalf("%s availability %q is not normalized", id, status.Availability)
		}
		if !allowedRuntime[status.Runtime] {
			t.Fatalf("%s runtime %q is not normalized", id, status.Runtime)
		}
		if !allowedAuth[status.Auth] {
			t.Fatalf("%s auth %q is not normalized", id, status.Auth)
		}
		if !allowedHealth[status.Health] {
			t.Fatalf("%s health %q is not normalized", id, status.Health)
		}
		if !allowedUI[status.UI] {
			t.Fatalf("%s ui %q is not normalized", id, status.UI)
		}
	}
	for _, node := range graph.Plugins {
		check(node.ID, node.Status)
	}
	for _, node := range graph.Skills {
		check(node.ID, node.Status)
	}
	for _, node := range graph.ToolInterfaces {
		check(node.ID, node.Status)
	}
	for _, node := range graph.Connectors {
		check(node.ID, node.Status)
	}
	for _, node := range graph.UISurfaces {
		check(node.ID, node.Status)
	}
	for _, node := range graph.Docs {
		check(node.ID, node.Status)
	}
}

var allowedLifecycle = map[string]bool{
	capLifecycleEnabled:  true,
	capLifecycleDisabled: true,
}

var allowedValidation = map[string]bool{
	capValidationValid:       true,
	capValidationInvalid:     true,
	capValidationUnvalidated: true,
}

var allowedAvailability = map[string]bool{
	capAvailabilityAvailable:   true,
	capAvailabilityGated:       true,
	capAvailabilityBlocked:     true,
	capAvailabilityUnavailable: true,
}

var allowedRuntime = map[string]bool{
	capRuntimeRunning:       true,
	capRuntimeStopped:       true,
	capRuntimeDegraded:      true,
	capRuntimeNotApplicable: true,
	capRuntimeUnknown:       true,
}

var allowedAuth = map[string]bool{
	capAuthConfigured:   true,
	capAuthUnconfigured: true,
	capAuthExpired:      true,
	capAuthError:        true,
	capAuthNotRequired:  true,
}

var allowedHealth = map[string]bool{
	capHealthHealthy: true,
	capHealthWarning: true,
	capHealthFailing: true,
	capHealthUnknown: true,
}

var allowedUI = map[string]bool{
	capUINone:               true,
	capUIAvailable:          true,
	capUIInvalid:            true,
	capUIUnsupportedSurface: true,
	capUIBlocked:            true,
}
