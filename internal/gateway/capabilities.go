package gateway

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ceoai/navi/internal/capability"
	"github.com/ceoai/navi/internal/connectors"
	"github.com/ceoai/navi/internal/navi/plugin"
	"github.com/ceoai/navi/internal/navi/skill"
	navitool "github.com/ceoai/navi/internal/tool"
)

const (
	capLifecycleEnabled  = "enabled"
	capLifecycleDisabled = "disabled"

	capValidationValid       = "valid"
	capValidationInvalid     = "invalid"
	capValidationUnvalidated = "unvalidated"

	capAvailabilityAvailable   = "available"
	capAvailabilityGated       = "gated"
	capAvailabilityBlocked     = "blocked"
	capAvailabilityUnavailable = "unavailable"

	capRuntimeRunning       = "running"
	capRuntimeStopped       = "stopped"
	capRuntimeDegraded      = "degraded"
	capRuntimeNotApplicable = "not_applicable"
	capRuntimeUnknown       = "unknown"

	capAuthConfigured   = "configured"
	capAuthUnconfigured = "unconfigured"
	capAuthExpired      = "expired"
	capAuthError        = "error"
	capAuthNotRequired  = "not_required"

	capHealthHealthy = "healthy"
	capHealthWarning = "warning"
	capHealthFailing = "failing"
	capHealthUnknown = "unknown"

	capUINone               = "none"
	capUIAvailable          = "available"
	capUIInvalid            = "invalid"
	capUIUnsupportedSurface = "unsupported_surface"
	capUIBlocked            = "blocked"
)

// CapabilityGraph is the read-only inventory projection consumed by the future
// Console Capabilities page. It intentionally aggregates existing registries
// without becoming a new source of truth.
type CapabilityGraph struct {
	Plugins        []PluginNode        `json:"plugins"`
	Skills         []SkillNode         `json:"skills"`
	ToolInterfaces []ToolInterfaceNode `json:"toolInterfaces"`
	Connectors     []ConnectorNode     `json:"connectors"`
	UISurfaces     []UISurfaceNode     `json:"uiSurfaces"`
	Docs           []DocNode           `json:"docs"`
	Edges          []CapabilityEdge    `json:"edges"`
}

type CapabilityStatus struct {
	Lifecycle    string `json:"lifecycle"`
	Validation   string `json:"validation"`
	Availability string `json:"availability"`
	Runtime      string `json:"runtime"`
	Auth         string `json:"auth"`
	Health       string `json:"health"`
	UI           string `json:"ui"`
}

type CapabilityActions struct {
	CanEnable    bool `json:"canEnable"`
	CanDisable   bool `json:"canDisable"`
	CanValidate  bool `json:"canValidate"`
	CanReload    bool `json:"canReload"`
	CanConfigure bool `json:"canConfigure"`
	CanRestart   bool `json:"canRestart"`
	CanInspect   bool `json:"canInspect"`
}

type CapabilityRisk struct {
	RiskTier             string   `json:"riskTier,omitempty"`
	SideEffects          []string `json:"sideEffects,omitempty"`
	RequiresConfirmation bool     `json:"requiresConfirmation,omitempty"`
	Reversibility        string   `json:"reversibility,omitempty"`
	CommandType          string   `json:"commandType,omitempty"`
}

type CapabilityNodeBase struct {
	ID             string            `json:"id"`
	DisplayName    string            `json:"displayName"`
	Kind           string            `json:"kind"`
	Version        string            `json:"version,omitempty"`
	Description    string            `json:"description,omitempty"`
	Source         string            `json:"source,omitempty"`
	ParentPluginID string            `json:"parentPluginId,omitempty"`
	Status         CapabilityStatus  `json:"status"`
	StatusReasons  []string          `json:"statusReasons"`
	Actions        CapabilityActions `json:"actions"`
	Risk           *CapabilityRisk   `json:"risk,omitempty"`
	TrustTier      string            `json:"trustTier,omitempty"`
	Tags           []string          `json:"tags"`
	RawRef         map[string]string `json:"rawRef,omitempty"`
}

type PluginNode struct {
	CapabilityNodeBase
	PluginID     string                    `json:"pluginId"`
	Categories   []string                  `json:"categories"`
	Capabilities []string                  `json:"capabilities"`
	Components   plugin.ManifestComponents `json:"components,omitempty"`
	ConfigSchema map[string]any            `json:"configSchema,omitempty"`
}

type SkillNode struct {
	CapabilityNodeBase
	SkillID          string   `json:"skillId"`
	Interfaces       []string `json:"interfaces"`
	RequiredEnv      []string `json:"requiredEnv,omitempty"`
	RequiredBinaries []string `json:"requiredBinaries,omitempty"`
}

type ToolInterfaceNode struct {
	CapabilityNodeBase
	CanonicalInterfaceID string         `json:"canonicalInterfaceId"`
	RegisteredToolName   string         `json:"registeredToolName,omitempty"`
	SkillID              string         `json:"skillId,omitempty"`
	InterfaceName        string         `json:"interfaceName,omitempty"`
	ToolID               string         `json:"toolId,omitempty"`
	Transport            string         `json:"transport,omitempty"`
	SourceType           string         `json:"sourceType,omitempty"`
	InputSchema          map[string]any `json:"inputSchema,omitempty"`
	OutputSchema         map[string]any `json:"outputSchema,omitempty"`
}

type ConnectorNode struct {
	CapabilityNodeBase
	DriverID     string                            `json:"driverId,omitempty"`
	InstanceID   string                            `json:"instanceId,omitempty"`
	Category     string                            `json:"category,omitempty"`
	Capabilities []connectors.CapabilityDescriptor `json:"capabilities,omitempty"`
	SetupSchema  *connectors.SetupDescriptor       `json:"setupSchema,omitempty"`
}

type UISurfaceNode struct {
	CapabilityNodeBase
	SurfaceID      string                       `json:"surfaceId,omitempty"`
	Title          string                       `json:"title,omitempty"`
	OwnerType      string                       `json:"ownerType,omitempty"`
	OwnerID        string                       `json:"ownerId,omitempty"`
	TargetSurfaces []string                     `json:"targetSurfaces"`
	RenderMode     string                       `json:"renderMode,omitempty"`
	Schema         map[string]interface{}       `json:"schema,omitempty"`
	ActionBindings []capability.UIActionBinding `json:"actionBindings"`
}

type DocNode struct {
	CapabilityNodeBase
	Path      string `json:"path,omitempty"`
	DocType   string `json:"docType,omitempty"`
	OwnerType string `json:"ownerType,omitempty"`
	OwnerID   string `json:"ownerId,omitempty"`
}

type CapabilityEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

type capabilityGraphBuilder struct {
	plugins      []plugin.Manifest
	skills       []skill.SkillEntry
	toolRegistry *navitool.Registry
	connectors   *connectors.Registry
	connectorMgr *connectors.Manager
	diagnostics  []connectors.Diagnostic
	// isPluginEnabled reports whether a plugin is enabled in the registry
	// (reflecting runtime enable/disable). When nil, manifest status is used.
	isPluginEnabled func(pluginID string) bool

	graph            CapabilityGraph
	edgeSeen         map[string]bool
	skillNodeIDs     map[string]string
	pluginNodeIDs    map[string]string
	connectorNodeIDs map[string]string
	skillOwners      map[string]string
	toolSeen         map[string]bool
}

func (s *Server) handleCapabilityGraph(w http.ResponseWriter, r *http.Request) {
	replyJSON(w, http.StatusOK, s.buildCapabilityGraph())
}

func (s *Server) handleSkillUI(w http.ResponseWriter, r *http.Request) {
	target := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("surface")))
	if target != "" && !capability.IsKnownUITarget(target) {
		replyError(w, http.StatusBadRequest, "unsupported surface")
		return
	}
	replyJSON(w, http.StatusOK, filterUISurfaces(s.buildCapabilityGraph().UISurfaces, "", target))
}

func (s *Server) handleSkillUISurfaces(w http.ResponseWriter, r *http.Request) {
	skillID := strings.TrimSpace(r.PathValue("id"))
	if skillID == "" {
		replyError(w, http.StatusBadRequest, "skill id is required")
		return
	}
	target := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("surface")))
	if target != "" && !capability.IsKnownUITarget(target) {
		replyError(w, http.StatusBadRequest, "unsupported surface")
		return
	}
	surfaces := filterUISurfaces(s.buildCapabilityGraph().UISurfaces, skillID, target)
	replyJSON(w, http.StatusOK, surfaces)
}

func (s *Server) buildCapabilityGraph() CapabilityGraph {
	var manifests []plugin.Manifest
	if s.cfg.PluginManifests != nil {
		manifests = s.cfg.PluginManifests()
	}
	var skills []skill.SkillEntry
	if s.cfg.SkillRegistry != nil {
		skills = s.cfg.SkillRegistry.List()
	}
	var diagnostics []connectors.Diagnostic
	if s.cfg.Manager != nil && s.cfg.Manager.Diag != nil {
		diagnostics = s.cfg.Manager.Diag.List()
	}
	b := capabilityGraphBuilder{
		plugins:          manifests,
		skills:           skills,
		toolRegistry:     s.toolRegistry(),
		connectors:       s.cfg.Registry,
		connectorMgr:     s.cfg.Manager,
		diagnostics:      diagnostics,
		isPluginEnabled:  s.cfg.IsPluginEnabled,
		edgeSeen:         make(map[string]bool),
		skillNodeIDs:     make(map[string]string),
		pluginNodeIDs:    make(map[string]string),
		connectorNodeIDs: make(map[string]string),
		skillOwners:      make(map[string]string),
		toolSeen:         make(map[string]bool),
	}
	return b.build()
}

func (b *capabilityGraphBuilder) build() CapabilityGraph {
	b.graph.Plugins = []PluginNode{}
	b.graph.Skills = []SkillNode{}
	b.graph.ToolInterfaces = []ToolInterfaceNode{}
	b.graph.Connectors = []ConnectorNode{}
	b.graph.UISurfaces = []UISurfaceNode{}
	b.graph.Docs = []DocNode{}
	b.graph.Edges = []CapabilityEdge{}

	b.addPlugins()
	b.addSkills()
	b.addToolInterfaces()
	b.addConnectors()
	b.addUISurfaces()
	b.addDocs()
	b.addPluginComponentEdges()
	b.sortGraph()

	return b.graph
}

func (b *capabilityGraphBuilder) addPlugins() {
	for _, manifest := range b.plugins {
		m := manifest
		m.Normalize(m.ID, m.RootDir)
		if strings.TrimSpace(m.ID) == "" {
			continue
		}
		for _, ref := range m.Components.Skills {
			skillID := strings.TrimSpace(ref.SkillID)
			if skillID == "" {
				skillID = strings.TrimSpace(filepath.Base(filepath.Clean(filepath.FromSlash(ref.Path))))
			}
			if skillID != "" {
				b.skillOwners[skillID] = m.ID
			}
		}
		id := pluginNodeID(m.ID)
		// A plugin is effectively enabled only when its manifest is active AND it
		// has not been disabled at runtime in the registry.
		enabled := m.IsActive()
		if enabled && b.isPluginEnabled != nil {
			enabled = b.isPluginEnabled(m.ID)
		}
		status, reasons := pluginStatus(m, enabled)
		node := PluginNode{
			CapabilityNodeBase: CapabilityNodeBase{
				ID:            id,
				DisplayName:   firstNonEmptyCapability(m.Name, m.Display.Name, m.ID),
				Kind:          firstNonEmptyCapability(m.Kind, "plugin"),
				Version:       firstNonEmptyCapability(m.Version, m.Semver),
				Description:   firstNonEmptyCapability(m.Description, m.Display.Description),
				Source:        "plugin_manifest",
				Status:        status,
				StatusReasons: stringsOrEmptyCapability(reasons),
				Actions: CapabilityActions{
					CanEnable:    !enabled,
					CanDisable:   enabled,
					CanValidate:  true,
					CanReload:    true,
					CanConfigure: len(m.ConfigSchema) > 0,
					CanInspect:   true,
				},
				TrustTier: firstNonEmptyCapability(m.TrustTier, "unknown"),
				Tags:      stringsOrEmptyCapability(m.Capabilities),
				RawRef: map[string]string{
					"pluginId": m.ID,
					"rootDir":  m.RootDir,
				},
			},
			PluginID:     m.ID,
			Categories:   append([]string(nil), m.Categories...),
			Capabilities: append([]string(nil), m.Capabilities...),
			Components:   m.Components,
			ConfigSchema: m.ConfigSchema,
		}
		b.graph.Plugins = append(b.graph.Plugins, node)
		b.pluginNodeIDs[m.ID] = id
	}
}

func (b *capabilityGraphBuilder) addSkills() {
	for _, entry := range b.skills {
		skillID := skillIDForEntry(entry)
		if skillID == "" {
			continue
		}
		id := skillNodeID(skillID)
		status, reasons := skillStatus(entry)
		parentPluginID := firstNonEmptyCapability(entry.SourcePluginID, b.skillOwners[skillID])
		displayName := firstNonEmptyCapability(entry.Skill.Name, skillID)
		var version, trustTier string
		var tags, interfaces []string
		var risk *CapabilityRisk
		if entry.Spec != nil {
			version = entry.Spec.Semver
			trustTier = entry.Spec.Governance.TrustTier
			tags = append(tags, entry.Spec.Capability.Tags...)
			risk = riskFromSkill(entry)
			for _, iface := range entry.Spec.Interfaces {
				if strings.TrimSpace(iface.Name) != "" {
					interfaces = append(interfaces, iface.Name)
				}
			}
		}
		node := SkillNode{
			CapabilityNodeBase: CapabilityNodeBase{
				ID:             id,
				DisplayName:    displayName,
				Kind:           "skill",
				Version:        version,
				Description:    entry.Skill.Description,
				Source:         skillSource(entry),
				ParentPluginID: parentPluginID,
				Status:         status,
				StatusReasons:  stringsOrEmptyCapability(reasons),
				Actions: CapabilityActions{
					CanValidate: entry.Spec != nil,
					CanInspect:  true,
				},
				Risk:      risk,
				TrustTier: firstNonEmptyCapability(trustTier, "unknown"),
				Tags:      stringsOrEmptyCapability(tags),
				RawRef: map[string]string{
					"skillId":  skillID,
					"filePath": entry.Skill.FilePath,
				},
			},
			SkillID:          skillID,
			Interfaces:       interfaces,
			RequiredEnv:      append([]string(nil), entry.Metadata.RequiresEnv...),
			RequiredBinaries: append([]string(nil), entry.Metadata.RequiresBins...),
		}
		b.graph.Skills = append(b.graph.Skills, node)
		b.skillNodeIDs[skillID] = id
		if parentPluginID != "" {
			b.addEdge(pluginNodeID(parentPluginID), id, "plugin_owns_skill")
		}
	}
}

func (b *capabilityGraphBuilder) addToolInterfaces() {
	var registered map[string]*navitool.Tool
	if b.toolRegistry != nil {
		registered = make(map[string]*navitool.Tool)
		for _, tool := range b.toolRegistry.List() {
			if tool == nil {
				continue
			}
			registered[tool.CanonicalID()] = tool
		}
	}

	for _, entry := range b.skills {
		if entry.Spec == nil {
			continue
		}
		skillID := skillIDForEntry(entry)
		for _, iface := range entry.Spec.Interfaces {
			if strings.TrimSpace(iface.Name) == "" {
				continue
			}
			registeredName := registeredToolNameForSkillInterface(skillID, iface.Name)
			var toolEntry *navitool.Tool
			if registered != nil {
				toolEntry = registered[registeredName]
			}
			node := toolInterfaceNodeFromSkill(entry, iface, toolEntry, registeredName, b.skillOwners[skillID])
			b.graph.ToolInterfaces = append(b.graph.ToolInterfaces, node)
			b.toolSeen[registeredName] = true
			b.addEdge(skillNodeID(skillID), node.ID, "skill_exposes_interface")
		}
	}

	if b.toolRegistry == nil {
		return
	}
	for _, tool := range b.toolRegistry.List() {
		if tool == nil {
			continue
		}
		toolID := tool.CanonicalID()
		if b.toolSeen[toolID] {
			continue
		}
		b.graph.ToolInterfaces = append(b.graph.ToolInterfaces, toolInterfaceNodeFromTool(tool))
		b.toolSeen[toolID] = true
	}
}

func (b *capabilityGraphBuilder) addConnectors() {
	if b.connectors == nil {
		return
	}
	healthByName := map[string]connectors.ConnectorHealth{}
	if b.connectorMgr != nil {
		for _, health := range b.connectorMgr.Health() {
			healthByName[health.Name] = health
		}
	}
	diagnosticsByName := map[string][]connectors.Diagnostic{}
	for _, diagnostic := range b.diagnostics {
		name := strings.TrimSpace(diagnostic.Connector)
		if name == "" {
			continue
		}
		diagnosticsByName[name] = append(diagnosticsByName[name], diagnostic)
	}
	driverByID := map[string]connectors.ConnectorDriver{}
	for _, driver := range b.connectors.Drivers() {
		if driver == nil || strings.TrimSpace(driver.DriverID()) == "" {
			continue
		}
		driverByID[driver.DriverID()] = driver
	}
	instanceSeen := map[string]bool{}
	for _, inst := range b.connectors.InstancesV2() {
		if inst == nil || strings.TrimSpace(inst.InstanceID()) == "" {
			continue
		}
		instanceSeen[inst.InstanceID()] = true
		driver := driverByID[inst.DriverID()]
		meta, _ := b.connectors.InstanceMetadata(inst.InstanceID())
		health, hasHealth := healthByName[inst.InstanceID()]
		node := connectorNodeFromInstance(inst, driver, meta, health, hasHealth, diagnosticsByName[inst.InstanceID()])
		b.graph.Connectors = append(b.graph.Connectors, node)
		b.connectorNodeIDs[inst.InstanceID()] = node.ID
		b.connectorNodeIDs[inst.DriverID()] = node.ID
	}
	for driverID, driver := range driverByID {
		if instanceSeen[driverID] {
			continue
		}
		node := connectorNodeFromDriver(driver)
		b.graph.Connectors = append(b.graph.Connectors, node)
		b.connectorNodeIDs[driverID] = node.ID
	}
}

func (b *capabilityGraphBuilder) addUISurfaces() {
	for _, manifest := range b.plugins {
		m := manifest
		m.Normalize(m.ID, m.RootDir)
		for _, surface := range m.UISurfaces {
			node := uiSurfaceNodeFromSpec(surface, "plugin", m.ID, m.ID, "plugin_manifest")
			b.graph.UISurfaces = append(b.graph.UISurfaces, node)
			b.addEdge(pluginNodeID(m.ID), node.ID, "component_provides_ui")
		}
	}
	for _, entry := range b.skills {
		if entry.Spec == nil {
			continue
		}
		skillID := skillIDForEntry(entry)
		parentPluginID := firstNonEmptyCapability(entry.SourcePluginID, b.skillOwners[skillID])
		for _, surface := range entry.Spec.UISurfaces {
			node := uiSurfaceNodeFromSpec(surface, "skill", skillID, parentPluginID, "skill_ui_surface")
			b.graph.UISurfaces = append(b.graph.UISurfaces, node)
			b.addEdge(skillNodeID(skillID), node.ID, "component_provides_ui")
		}
	}
}

func (b *capabilityGraphBuilder) addDocs() {
	for _, manifest := range b.plugins {
		m := manifest
		m.Normalize(m.ID, m.RootDir)
		if strings.TrimSpace(m.ID) == "" || strings.TrimSpace(m.RootDir) == "" {
			continue
		}
		for _, path := range discoverPluginDocs(m.RootDir) {
			rel, err := filepath.Rel(m.RootDir, path)
			if err != nil {
				rel = filepath.Base(path)
			}
			node := docNodeFromPath(path, filepath.ToSlash(rel), "plugin", m.ID)
			b.graph.Docs = append(b.graph.Docs, node)
			b.addEdge(pluginNodeID(m.ID), node.ID, "plugin_declares_doc")
		}
	}
}

func (b *capabilityGraphBuilder) addPluginComponentEdges() {
	for _, manifest := range b.plugins {
		m := manifest
		m.Normalize(m.ID, m.RootDir)
		if strings.TrimSpace(m.ID) == "" {
			continue
		}
		pluginID := pluginNodeID(m.ID)
		for _, ref := range m.Components.Skills {
			skillID := strings.TrimSpace(ref.SkillID)
			if skillID == "" {
				skillID = strings.TrimSpace(filepath.Base(filepath.Clean(filepath.FromSlash(ref.Path))))
			}
			if nodeID, ok := b.skillNodeIDs[skillID]; ok {
				b.addEdge(pluginID, nodeID, "plugin_owns_skill")
			}
		}
		var connectorIDs []string
		if m.Connector != nil {
			connectorIDs = append(connectorIDs, firstNonEmptyCapability(m.Connector.DriverID, m.ID))
		}
		for _, ref := range m.Components.Connectors {
			connectorID := strings.TrimSpace(ref.ConnectorID)
			if connectorID == "" {
				connectorID = strings.TrimSpace(filepath.Base(filepath.Clean(filepath.FromSlash(ref.Path))))
			}
			if connectorID != "" {
				connectorIDs = append(connectorIDs, connectorID)
			}
		}
		for _, connectorID := range connectorIDs {
			if nodeID, ok := b.connectorNodeIDs[connectorID]; ok {
				b.addEdge(pluginID, nodeID, "plugin_uses_connector")
				b.addEdge(nodeID, pluginID, "connector_supports_plugin")
			}
		}
	}
}

func (b *capabilityGraphBuilder) addEdge(from, to, kind string) {
	if from == "" || to == "" || kind == "" {
		return
	}
	key := from + "\x00" + to + "\x00" + kind
	if b.edgeSeen[key] {
		return
	}
	b.edgeSeen[key] = true
	b.graph.Edges = append(b.graph.Edges, CapabilityEdge{From: from, To: to, Kind: kind})
}

func (b *capabilityGraphBuilder) sortGraph() {
	sort.Slice(b.graph.Plugins, func(i, j int) bool { return b.graph.Plugins[i].ID < b.graph.Plugins[j].ID })
	sort.Slice(b.graph.Skills, func(i, j int) bool { return b.graph.Skills[i].ID < b.graph.Skills[j].ID })
	sort.Slice(b.graph.ToolInterfaces, func(i, j int) bool { return b.graph.ToolInterfaces[i].ID < b.graph.ToolInterfaces[j].ID })
	sort.Slice(b.graph.Connectors, func(i, j int) bool { return b.graph.Connectors[i].ID < b.graph.Connectors[j].ID })
	sort.Slice(b.graph.UISurfaces, func(i, j int) bool { return b.graph.UISurfaces[i].ID < b.graph.UISurfaces[j].ID })
	sort.Slice(b.graph.Docs, func(i, j int) bool { return b.graph.Docs[i].ID < b.graph.Docs[j].ID })
	sort.Slice(b.graph.Edges, func(i, j int) bool {
		a := b.graph.Edges[i]
		c := b.graph.Edges[j]
		if a.From != c.From {
			return a.From < c.From
		}
		if a.To != c.To {
			return a.To < c.To
		}
		return a.Kind < c.Kind
	})
}

func pluginStatus(m plugin.Manifest, enabled bool) (CapabilityStatus, []string) {
	status := defaultStatus()
	status.Runtime = capRuntimeNotApplicable
	status.Auth = capAuthNotRequired
	status.UI = capUINone
	if enabled {
		status.Lifecycle = capLifecycleEnabled
		status.Availability = capAvailabilityAvailable
	} else {
		status.Lifecycle = capLifecycleDisabled
		status.Availability = capAvailabilityUnavailable
	}
	var reasons []string
	if err := m.Validate(); err != nil {
		status.Validation = capValidationInvalid
		status.Availability = capAvailabilityBlocked
		reasons = append(reasons, err.Error())
	} else {
		status.Validation = capValidationValid
	}
	if !enabled {
		reasons = append(reasons, "plugin lifecycle is disabled")
	}
	return status, reasons
}

func skillStatus(entry skill.SkillEntry) (CapabilityStatus, []string) {
	status := defaultStatus()
	status.Lifecycle = capLifecycleEnabled
	status.Runtime = capRuntimeNotApplicable
	status.Auth = capAuthNotRequired
	status.UI = capUINone
	if entry.Spec == nil {
		status.Validation = capValidationUnvalidated
	} else if err := skill.ValidateSpec(entry.Spec); err != nil {
		status.Validation = capValidationInvalid
		status.Availability = capAvailabilityBlocked
		return status, []string{err.Error()}
	} else {
		status.Validation = capValidationValid
	}
	if entry.Activatable {
		status.Availability = capAvailabilityAvailable
		return status, nil
	}
	status.Availability = capAvailabilityGated
	return status, append([]string(nil), entry.ReasonsUnbound...)
}

func toolStatus(tool *navitool.Tool, fallbackAvailable bool, reasons []string) (CapabilityStatus, []string) {
	status := defaultStatus()
	status.Lifecycle = capLifecycleEnabled
	status.Validation = capValidationUnvalidated
	status.Runtime = capRuntimeNotApplicable
	status.Auth = capAuthNotRequired
	status.Health = capHealthUnknown
	status.UI = capUINone
	if fallbackAvailable {
		status.Availability = capAvailabilityAvailable
	} else {
		status.Availability = capAvailabilityGated
	}
	if tool == nil {
		return status, reasons
	}
	switch tool.Status {
	case navitool.ToolStatusActive:
		status.Availability = capAvailabilityAvailable
	case navitool.ToolStatusDeprecated:
		status.Availability = capAvailabilityGated
		reasons = append(reasons, "registered tool is deprecated")
	case navitool.ToolStatusSuspended:
		status.Availability = capAvailabilityBlocked
		reasons = append(reasons, "registered tool is suspended")
	case navitool.ToolStatusRemoved:
		status.Lifecycle = capLifecycleDisabled
		status.Availability = capAvailabilityUnavailable
		reasons = append(reasons, "registered tool is removed")
	case navitool.ToolStatusInvalid:
		status.Validation = capValidationInvalid
		status.Availability = capAvailabilityBlocked
		reasons = append(reasons, "registered tool is invalid")
	}
	if tool.Executor == nil {
		status.Availability = capAvailabilityGated
		reasons = append(reasons, "registered tool has no executor")
	}
	if tool.Hidden {
		reasons = append(reasons, "registered tool is hidden")
	}
	return status, reasons
}

func connectorNodeFromDriver(driver connectors.ConnectorDriver) ConnectorNode {
	setup := driver.SetupDescriptor()
	var setupPtr *connectors.SetupDescriptor
	if setup.Type != "" || setup.DisplayName != "" {
		setupCopy := setup
		setupPtr = &setupCopy
	}
	return ConnectorNode{
		CapabilityNodeBase: CapabilityNodeBase{
			ID:          connectorNodeID(driver.DriverID()),
			DisplayName: firstNonEmptyCapability(driver.DisplayName(), driver.DriverID()),
			Kind:        "connector_driver",
			Source:      "connector_registry",
			Status: CapabilityStatus{
				Lifecycle:    capLifecycleEnabled,
				Validation:   capValidationUnvalidated,
				Availability: capAvailabilityAvailable,
				Runtime:      capRuntimeStopped,
				Auth:         authStatusFromSetup(setup),
				Health:       capHealthUnknown,
				UI:           capUINone,
			},
			StatusReasons: []string{},
			Actions: CapabilityActions{
				CanConfigure: setupPtr != nil,
				CanInspect:   true,
			},
			Tags: []string{},
			RawRef: map[string]string{
				"driverId": driver.DriverID(),
			},
		},
		DriverID:     driver.DriverID(),
		Capabilities: driver.Capabilities(),
		SetupSchema:  setupPtr,
	}
}

func connectorNodeFromInstance(inst connectors.ConnectorInstance, driver connectors.ConnectorDriver, meta connectors.InstanceMetadata, health connectors.ConnectorHealth, hasHealth bool, diagnostics []connectors.Diagnostic) ConnectorNode {
	driverID := inst.DriverID()
	displayName := driverID
	var caps []connectors.CapabilityDescriptor
	var setupPtr *connectors.SetupDescriptor
	if driver != nil {
		displayName = firstNonEmptyCapability(driver.DisplayName(), driverID)
		caps = driver.Capabilities()
		setup := driver.SetupDescriptor()
		if setup.Type != "" || setup.DisplayName != "" {
			setupCopy := setup
			setupPtr = &setupCopy
		}
	}
	runtimeRaw := firstNonEmptyCapability(meta.Status, health.Status)
	healthRaw := meta.HealthState
	if hasHealth {
		healthRaw = health.Status
	}
	status := CapabilityStatus{
		Lifecycle:    capLifecycleEnabled,
		Validation:   capValidationUnvalidated,
		Availability: capAvailabilityAvailable,
		Runtime:      normalizeRuntimeStatus(runtimeRaw),
		Auth:         normalizeAuthStatus(meta.AuthState),
		Health:       normalizeHealthStatus(healthRaw),
		UI:           capUINone,
	}
	if status.Runtime == capRuntimeUnknown && inst.Connector() != nil {
		if inst.Connector().IsRunning() {
			status.Runtime = capRuntimeRunning
		} else {
			status.Runtime = capRuntimeStopped
		}
	}
	reasons := connectorStatusReasons(meta, health, hasHealth, diagnostics)
	if len(diagnostics) > 0 {
		status.Health = capHealthWarning
		for _, diagnostic := range diagnostics {
			if diagnostic.Level == connectors.DiagError {
				status.Health = capHealthFailing
				break
			}
		}
	}
	return ConnectorNode{
		CapabilityNodeBase: CapabilityNodeBase{
			ID:            connectorNodeID(inst.InstanceID()),
			DisplayName:   displayName,
			Kind:          "connector",
			Source:        "connector_registry",
			Status:        status,
			StatusReasons: stringsOrEmptyCapability(reasons),
			Actions: CapabilityActions{
				CanConfigure: setupPtr != nil,
				CanInspect:   true,
			},
			Tags: []string{},
			RawRef: map[string]string{
				"driverId":   driverID,
				"instanceId": inst.InstanceID(),
			},
		},
		DriverID:     driverID,
		InstanceID:   inst.InstanceID(),
		Capabilities: caps,
		SetupSchema:  setupPtr,
	}
}

func uiSurfaceNodeFromSpec(surface capability.UISurfaceSpec, ownerType, ownerID, parentPluginID, source string) UISurfaceNode {
	validation := capability.ValidateUISurface(surface, "")
	status := defaultStatus()
	status.Lifecycle = capLifecycleEnabled
	status.Runtime = capRuntimeNotApplicable
	status.Auth = capAuthNotRequired
	status.Health = capHealthUnknown
	status.Validation = capValidationValid
	status.Availability = capAvailabilityAvailable
	status.UI = capUIAvailable
	if !validation.Valid {
		status.Validation = capValidationInvalid
		status.Availability = capAvailabilityBlocked
		status.UI = capUIInvalid
	} else if !validation.Supported {
		status.Availability = capAvailabilityUnavailable
		status.UI = capUIUnsupportedSurface
	}
	displayName := firstNonEmptyCapability(surface.Title, surface.ID)
	return UISurfaceNode{
		CapabilityNodeBase: CapabilityNodeBase{
			ID:             uiSurfaceNodeID(ownerType, ownerID, surface.ID),
			DisplayName:    displayName,
			Kind:           "ui_surface",
			Description:    surface.Description,
			Source:         source,
			ParentPluginID: parentPluginID,
			Status:         status,
			StatusReasons:  stringsOrEmptyCapability(validation.Reasons),
			Actions: CapabilityActions{
				CanInspect: true,
			},
			Tags: []string{},
			RawRef: map[string]string{
				"ownerType": ownerType,
				"ownerId":   ownerID,
				"surfaceId": surface.ID,
			},
		},
		SurfaceID:      surface.ID,
		Title:          surface.Title,
		OwnerType:      ownerType,
		OwnerID:        ownerID,
		TargetSurfaces: stringsOrEmptyCapability(validation.Targets),
		RenderMode:     validation.RenderMode,
		Schema:         surface.Schema,
		ActionBindings: stringsOrEmptyUIActions(surface.Actions),
	}
}

func docNodeFromPath(absPath, relPath, ownerType, ownerID string) DocNode {
	displayName := strings.TrimSuffix(filepath.Base(relPath), filepath.Ext(relPath))
	status := defaultStatus()
	status.Lifecycle = capLifecycleEnabled
	status.Validation = capValidationUnvalidated
	status.Availability = capAvailabilityAvailable
	status.Runtime = capRuntimeNotApplicable
	status.Auth = capAuthNotRequired
	status.UI = capUINone
	return DocNode{
		CapabilityNodeBase: CapabilityNodeBase{
			ID:            docNodeID(ownerType, ownerID, relPath),
			DisplayName:   firstNonEmptyCapability(displayName, relPath),
			Kind:          "doc",
			Source:        "plugin_docs",
			Status:        status,
			StatusReasons: []string{},
			Actions: CapabilityActions{
				CanInspect: true,
			},
			Tags: []string{},
			RawRef: map[string]string{
				"ownerType": ownerType,
				"ownerId":   ownerID,
				"path":      absPath,
			},
		},
		Path:      filepath.ToSlash(relPath),
		DocType:   strings.TrimPrefix(strings.ToLower(filepath.Ext(relPath)), "."),
		OwnerType: ownerType,
		OwnerID:   ownerID,
	}
}

func connectorStatusReasons(meta connectors.InstanceMetadata, health connectors.ConnectorHealth, hasHealth bool, diagnostics []connectors.Diagnostic) []string {
	var reasons []string
	if meta.Status != "" {
		reasons = append(reasons, "connector metadata status: "+meta.Status)
	}
	if meta.AuthState != "" {
		reasons = append(reasons, "connector auth state: "+meta.AuthState)
	}
	if hasHealth && health.Status != "" {
		reasons = append(reasons, "manager health status: "+health.Status)
	}
	if hasHealth && health.ConsecutiveErrs > 0 {
		reasons = append(reasons, fmt.Sprintf("consecutive connector errors: %d", health.ConsecutiveErrs))
	}
	for _, diagnostic := range diagnostics {
		if strings.TrimSpace(diagnostic.Message) == "" {
			continue
		}
		reasons = append(reasons, fmt.Sprintf("%s diagnostic: %s", diagnostic.Level, diagnostic.Message))
	}
	return reasons
}

func filterUISurfaces(surfaces []UISurfaceNode, skillID, target string) []UISurfaceNode {
	out := make([]UISurfaceNode, 0, len(surfaces))
	for _, surface := range surfaces {
		if skillID != "" && (surface.OwnerType != "skill" || surface.OwnerID != skillID) {
			continue
		}
		if target != "" && !stringSliceContains(surface.TargetSurfaces, target) {
			continue
		}
		out = append(out, surface)
	}
	return out
}

func discoverPluginDocs(root string) []string {
	var out []string
	for _, candidate := range []string{"README.md", "README.txt"} {
		path := filepath.Join(root, candidate)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			out = append(out, path)
		}
	}
	for _, pattern := range []string{filepath.Join(root, "docs", "*.md"), filepath.Join(root, "docs", "*.txt")} {
		matches, _ := filepath.Glob(pattern)
		for _, match := range matches {
			if info, err := os.Stat(match); err == nil && !info.IsDir() {
				out = append(out, match)
			}
		}
	}
	sort.Strings(out)
	return out
}

func toolInterfaceNodeFromSkill(entry skill.SkillEntry, iface skill.Interface, toolEntry *navitool.Tool, registeredName, manifestOwner string) ToolInterfaceNode {
	skillID := skillIDForEntry(entry)
	canonicalID := canonicalInterfaceID(skillID, iface.Name)
	parentPluginID := firstNonEmptyCapability(entry.SourcePluginID, manifestOwner)
	reasons := []string{}
	if toolEntry == nil {
		reasons = append(reasons, "interface is not registered in tool registry")
	}
	status, reasons := toolStatus(toolEntry, entry.Activatable, reasons)
	if entry.Spec != nil {
		status.Validation = capValidationValid
	}
	node := ToolInterfaceNode{
		CapabilityNodeBase: CapabilityNodeBase{
			ID:             toolInterfaceNodeID(canonicalID),
			DisplayName:    firstNonEmptyCapability(iface.Name, canonicalID),
			Kind:           "tool_interface",
			Version:        entry.Spec.Semver,
			Description:    entry.Skill.Description,
			Source:         "skill_interface",
			ParentPluginID: parentPluginID,
			Status:         status,
			StatusReasons:  stringsOrEmptyCapability(reasons),
			Actions: CapabilityActions{
				CanInspect: true,
			},
			Risk:      riskFromSkill(entry),
			TrustTier: firstNonEmptyCapability(entry.Spec.Governance.TrustTier, "unknown"),
			Tags:      stringsOrEmptyCapability(entry.Spec.Capability.Tags),
			RawRef: map[string]string{
				"skillId":   skillID,
				"interface": iface.Name,
			},
		},
		CanonicalInterfaceID: canonicalID,
		RegisteredToolName:   registeredName,
		SkillID:              skillID,
		InterfaceName:        iface.Name,
		ToolID:               registeredName,
		Transport:            iface.Transport.Type,
		SourceType:           string(navitool.ToolSourceSkill),
		InputSchema:          iface.InputSchema,
		OutputSchema:         iface.OutputSchema,
	}
	if toolEntry != nil {
		node.ToolID = toolEntry.CanonicalID()
		node.DisplayName = firstNonEmptyCapability(toolEntry.DisplayName, node.DisplayName)
		node.Risk = riskFromTool(toolEntry, node.Risk)
		node.Tags = appendUniqueStrings(node.Tags, toolEntry.CapabilityTags...)
	}
	return node
}

func toolInterfaceNodeFromTool(tool *navitool.Tool) ToolInterfaceNode {
	toolID := tool.CanonicalID()
	status, reasons := toolStatus(tool, true, nil)
	return ToolInterfaceNode{
		CapabilityNodeBase: CapabilityNodeBase{
			ID:            toolInterfaceNodeID(toolID),
			DisplayName:   firstNonEmptyCapability(tool.DisplayName, toolID),
			Kind:          "tool_interface",
			Version:       tool.SchemaVersion,
			Description:   tool.Description,
			Source:        "tool_registry",
			Status:        status,
			StatusReasons: stringsOrEmptyCapability(reasons),
			Actions: CapabilityActions{
				CanInspect: true,
			},
			Risk:      riskFromTool(tool, nil),
			TrustTier: firstNonEmptyCapability(tool.Metadata.TrustTier, "unknown"),
			Tags:      stringsOrEmptyCapability(appendUniqueStrings(append([]string(nil), tool.CapabilityTags...), tool.Metadata.Tags...)),
			RawRef: map[string]string{
				"toolId":   toolID,
				"sourceId": tool.SourceID,
			},
		},
		CanonicalInterfaceID: toolID,
		RegisteredToolName:   toolID,
		ToolID:               toolID,
		SourceType:           string(tool.Source),
		InputSchema:          tool.InputSchema,
		OutputSchema:         tool.OutputSchema,
	}
}

func defaultStatus() CapabilityStatus {
	return CapabilityStatus{
		Lifecycle:    capLifecycleEnabled,
		Validation:   capValidationUnvalidated,
		Availability: capAvailabilityUnavailable,
		Runtime:      capRuntimeUnknown,
		Auth:         capAuthNotRequired,
		Health:       capHealthUnknown,
		UI:           capUINone,
	}
}

func normalizeRuntimeStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "running", "connected", "healthy", "started", "active":
		return capRuntimeRunning
	case "stopped", "disconnected", "down", "inactive", "disabled", "not_running", "configured", "created", "pending":
		return capRuntimeStopped
	case "degraded", "warning", "half_open":
		return capRuntimeDegraded
	case "not_applicable", "n/a", "none":
		return capRuntimeNotApplicable
	default:
		return capRuntimeUnknown
	}
}

func normalizeAuthStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "configured", "ok", "ready", "connected", "authenticated", "authorized", "valid":
		return capAuthConfigured
	case "unconfigured", "missing", "not_configured", "empty", "pending":
		return capAuthUnconfigured
	case "expired", "stale":
		return capAuthExpired
	case "error", "failed", "invalid", "unauthorized", "denied":
		return capAuthError
	case "not_required", "none", "n/a":
		return capAuthNotRequired
	default:
		return capAuthUnconfigured
	}
}

func normalizeHealthStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "healthy", "ok", "running", "connected":
		return capHealthHealthy
	case "warning", "degraded", "unstable":
		return capHealthWarning
	case "failing", "failed", "down", "error", "unhealthy", "critical":
		return capHealthFailing
	default:
		return capHealthUnknown
	}
}

func authStatusFromSetup(setup connectors.SetupDescriptor) string {
	if len(setup.RequiredParams) == 0 && len(setup.OptionalParams) == 0 {
		return capAuthNotRequired
	}
	return capAuthUnconfigured
}

func skillIDForEntry(entry skill.SkillEntry) string {
	if entry.Spec != nil && strings.TrimSpace(entry.Spec.SkillID) != "" {
		return strings.TrimSpace(entry.Spec.SkillID)
	}
	if strings.TrimSpace(entry.Skill.ID) != "" {
		return strings.TrimSpace(entry.Skill.ID)
	}
	return strings.TrimSpace(entry.Skill.Name)
}

func skillSource(entry skill.SkillEntry) string {
	if entry.SourcePluginID != "" {
		return "plugin_skill"
	}
	switch entry.Tier {
	case skill.TierWorkspace:
		return "workspace_skill"
	case skill.TierGlobal:
		return "global_skill"
	case skill.TierBuiltin:
		return "builtin_skill"
	default:
		return "skill_registry"
	}
}

func riskFromSkill(entry skill.SkillEntry) *CapabilityRisk {
	if entry.Spec == nil {
		return nil
	}
	return &CapabilityRisk{
		RiskTier:             entry.Spec.Effects.RiskTier,
		SideEffects:          append([]string(nil), entry.Spec.Effects.SideEffects...),
		RequiresConfirmation: entry.Spec.Effects.RequiresConfirmation,
		Reversibility:        entry.Spec.Effects.Reversibility,
		CommandType:          entry.Spec.Capability.CommandType,
	}
}

func riskFromTool(tool *navitool.Tool, fallback *CapabilityRisk) *CapabilityRisk {
	if tool == nil {
		return fallback
	}
	risk := &CapabilityRisk{
		RiskTier:             firstNonEmptyCapability(tool.RiskTier, tool.Governance.RiskTier),
		SideEffects:          append([]string(nil), tool.SideEffects...),
		RequiresConfirmation: tool.Governance.RequiresConfirm,
		Reversibility:        firstNonEmptyCapability(tool.Reversibility, tool.Governance.Reversibility),
		CommandType:          string(tool.Governance.CommandType),
	}
	if risk.RiskTier == "" && fallback != nil {
		risk.RiskTier = fallback.RiskTier
	}
	if len(risk.SideEffects) == 0 && fallback != nil {
		risk.SideEffects = append([]string(nil), fallback.SideEffects...)
	}
	if risk.Reversibility == "" && fallback != nil {
		risk.Reversibility = fallback.Reversibility
	}
	if risk.CommandType == "" && fallback != nil {
		risk.CommandType = fallback.CommandType
	}
	return risk
}

func pluginNodeID(id string) string {
	return "plugin:" + strings.TrimSpace(id)
}

func skillNodeID(id string) string {
	return "skill:" + strings.TrimSpace(id)
}

func toolInterfaceNodeID(id string) string {
	return "tool_interface:" + strings.TrimSpace(id)
}

func connectorNodeID(id string) string {
	return "connector:" + strings.TrimSpace(id)
}

func uiSurfaceNodeID(ownerType, ownerID, surfaceID string) string {
	return "ui_surface:" + strings.TrimSpace(ownerType) + ":" + strings.TrimSpace(ownerID) + ":" + strings.TrimSpace(surfaceID)
}

func docNodeID(ownerType, ownerID, path string) string {
	return "doc:" + strings.TrimSpace(ownerType) + ":" + strings.TrimSpace(ownerID) + ":" + capabilitySanitizeName(filepath.ToSlash(path))
}

func canonicalInterfaceID(skillID, iface string) string {
	return strings.TrimSpace(skillID) + "." + strings.TrimSpace(iface)
}

func registeredToolNameForSkillInterface(skillID, iface string) string {
	return fmt.Sprintf("skill.%s.%s", capabilitySanitizeName(skillID), capabilitySanitizeName(iface))
}

func capabilitySanitizeName(raw string) string {
	clean := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' {
			clean = append(clean, c)
		} else {
			clean = append(clean, '_')
		}
	}
	return string(clean)
}

func firstNonEmptyCapability(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func appendUniqueStrings(base []string, additions ...string) []string {
	seen := make(map[string]bool, len(base)+len(additions))
	out := make([]string, 0, len(base)+len(additions))
	for _, value := range append(base, additions...) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

func stringsOrEmptyCapability(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func stringsOrEmptyUIActions(values []capability.UIActionBinding) []capability.UIActionBinding {
	if values == nil {
		return []capability.UIActionBinding{}
	}
	return values
}
