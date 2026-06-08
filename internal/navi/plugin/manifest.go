package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ceoai/navi/internal/capability"
	"github.com/ceoai/navi/internal/coderalias"
)

const (
	KindLLMProvider = "llm-provider"
	KindIntegration = "integration"
	KindWorkflow    = "workflow"
	KindDomain      = "domain"
	KindAgentic     = "agentic"
	KindSensory     = "sensory"
	KindSystem      = "system"
)

var validPluginKinds = map[string]struct{}{
	KindLLMProvider: {},
	KindIntegration: {},
	KindWorkflow:    {},
	KindDomain:      {},
	KindAgentic:     {},
	KindSensory:     {},
	KindSystem:      {},
}

// Manifest describes a plugin's metadata for discovery and tooling.
type Manifest struct {
	ID           string          `json:"id" yaml:"id"`
	PluginID     string          `json:"plugin_id,omitempty" yaml:"plugin_id,omitempty"`
	Name         string          `json:"name,omitempty" yaml:"name,omitempty"`
	Description  string          `json:"description,omitempty" yaml:"description,omitempty"`
	Version      string          `json:"version,omitempty" yaml:"version,omitempty"`
	Semver       string          `json:"semver,omitempty" yaml:"semver,omitempty"`
	Kind         string          `json:"kind,omitempty" yaml:"kind,omitempty"`
	Status       string          `json:"status,omitempty" yaml:"status,omitempty"`
	DateAdded    time.Time       `json:"date_added,omitempty" yaml:"date_added,omitempty"`
	Enabled      bool            `json:"enabled" yaml:"enabled"`
	Active       bool            `json:"active" yaml:"active"`
	Display      ManifestDisplay `json:"display,omitempty" yaml:"display,omitempty"`
	ConfigSchema map[string]any  `json:"config_schema,omitempty" yaml:"config_schema,omitempty"`

	// Categories classifies the plugin along the conceptual axes used in the
	// docs (e.g. "domain", "workflow", "integration", "agentic", "sensory").
	Categories []string `json:"categories,omitempty" yaml:"categories,omitempty"`

	// Capabilities is a free-form list of higher-level capabilities the plugin
	// provides (e.g. "github_pr_review", "calendar_sync").
	Capabilities []string `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`

	// Triggers describes the conditions or hook points where this plugin
	// activates (e.g. "message_received", "after_tool_call").
	Triggers []string `json:"triggers,omitempty" yaml:"triggers,omitempty"`

	// TrustTier mirrors the skill trust tier enum ("builtin", "verified",
	// "community", "local") so governance and UX can reason about the plugin's
	// provenance and default risk posture.
	TrustTier string `json:"trust_tier,omitempty" yaml:"trust_tier,omitempty"`

	// Connector describes an optional connector driver exposed by this plugin.
	// When present, the plugin loader can register the driver at runtime without
	// a core binary change.
	Connector *ConnectorManifest `json:"connector,omitempty" yaml:"connector,omitempty"`

	Components   ManifestComponents         `json:"components,omitempty" yaml:"components,omitempty"`
	Dependencies ManifestDependencies       `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
	Contract     ManifestContract           `json:"contract,omitempty" yaml:"contract,omitempty"`
	Permissions  map[string]any             `json:"permissions,omitempty" yaml:"permissions,omitempty"`
	Runtime      map[string]any             `json:"runtime,omitempty" yaml:"runtime,omitempty"`
	Lifecycle    map[string]any             `json:"lifecycle,omitempty" yaml:"lifecycle,omitempty"`
	UISurfaces   []capability.UISurfaceSpec `json:"uiSurfaces,omitempty" yaml:"ui_surfaces,omitempty"`

	RuntimeSpec ManifestRuntime `json:"-" yaml:"-"`
	RootDir     string          `json:"-" yaml:"-"`
}

type ManifestDisplay struct {
	Name        string `json:"name,omitempty" yaml:"name,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Emoji       string `json:"emoji,omitempty" yaml:"emoji,omitempty"`
}

type ManifestComponents struct {
	Skills     []ComponentRef `json:"skills,omitempty" yaml:"skills,omitempty"`
	Connectors []ComponentRef `json:"connectors,omitempty" yaml:"connectors,omitempty"`
	Providers  []ComponentRef `json:"providers,omitempty" yaml:"providers,omitempty"`
	Policies   []ComponentRef `json:"policies,omitempty" yaml:"policies,omitempty"`
}

type ManifestDependencies struct {
	RequiredCore        []string `json:"required_core,omitempty" yaml:"required_core,omitempty"`
	RequiredTaskSources []string `json:"required_task_sources,omitempty" yaml:"required_task_sources,omitempty"`
	Recommended         []string `json:"recommended,omitempty" yaml:"recommended,omitempty"`
	Optional            []string `json:"optional,omitempty" yaml:"optional,omitempty"`
}

type ManifestContract struct {
	WorkflowEntryPoints []string `json:"workflow_entry_points,omitempty" yaml:"workflow_entry_points,omitempty"`
	IncludedBehavior    []string `json:"included_behavior,omitempty" yaml:"included_behavior,omitempty"`
	ExcludedBehavior    []string `json:"excluded_behavior,omitempty" yaml:"excluded_behavior,omitempty"`
	SelfUpdateMode      string   `json:"self_update_mode,omitempty" yaml:"self_update_mode,omitempty"`
}

type ManifestRuntime struct {
	WorkflowContracts []ManifestWorkflowContract `json:"workflow_contracts,omitempty" yaml:"workflow_contracts,omitempty"`
}

type ManifestWorkflowContract struct {
	WorkflowID string `json:"workflow_id,omitempty" yaml:"workflow_id,omitempty"`
	Name       string `json:"name,omitempty" yaml:"name,omitempty"`
	Path       string `json:"path,omitempty" yaml:"path,omitempty"`
	Runner     string `json:"runner,omitempty" yaml:"runner,omitempty"`
	Extends    string `json:"extends,omitempty" yaml:"extends,omitempty"`
	Status     string `json:"status,omitempty" yaml:"status,omitempty"`
}

type ComponentRef struct {
	Path        string `json:"path" yaml:"path"`
	SkillID     string `json:"skill_id,omitempty" yaml:"skill_id,omitempty"`
	ConnectorID string `json:"connector_id,omitempty" yaml:"connector_id,omitempty"`
	ProviderID  string `json:"provider_id,omitempty" yaml:"provider_id,omitempty"`
}

func (m *Manifest) Normalize(defaultID, rootDir string) {
	if strings.TrimSpace(m.ID) == "" {
		m.ID = strings.TrimSpace(m.PluginID)
	}
	if strings.TrimSpace(m.ID) == "" {
		m.ID = strings.TrimSpace(defaultID)
	}
	if strings.TrimSpace(m.PluginID) == "" {
		m.PluginID = m.ID
	}
	if strings.TrimSpace(m.Version) == "" {
		m.Version = strings.TrimSpace(m.Semver)
	}
	if strings.TrimSpace(m.Semver) == "" {
		m.Semver = strings.TrimSpace(m.Version)
	}
	if strings.TrimSpace(m.Name) == "" {
		m.Name = strings.TrimSpace(m.Display.Name)
	}
	if strings.TrimSpace(m.Description) == "" {
		m.Description = strings.TrimSpace(m.Display.Description)
	}
	if strings.TrimSpace(m.Kind) == "" {
		m.Kind = inferKind(*m)
	}
	m.RuntimeSpec = parseManifestRuntime(m.Runtime)
	m.RootDir = rootDir
}

func inferKind(m Manifest) string {
	for _, category := range m.Categories {
		category = strings.TrimSpace(strings.ToLower(category))
		if _, ok := validPluginKinds[category]; ok {
			return category
		}
	}
	if m.Connector != nil || len(m.Components.Connectors) > 0 {
		return KindIntegration
	}
	return KindSystem
}

func stringValue(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func (m Manifest) Validate() error {
	if strings.TrimSpace(m.ID) == "" {
		return fmt.Errorf("missing plugin id")
	}
	if _, ok := validPluginKinds[strings.TrimSpace(m.Kind)]; !ok {
		return fmt.Errorf("invalid plugin kind %q", m.Kind)
	}
	if err := validateComponentRefs(m.RootDir, m.Components.Skills, true); err != nil {
		return fmt.Errorf("skills: %w", err)
	}
	if err := validateComponentRefs(m.RootDir, m.Components.Connectors, false); err != nil {
		return fmt.Errorf("connectors: %w", err)
	}
	if err := validateComponentRefs(m.RootDir, m.Components.Providers, false); err != nil {
		return fmt.Errorf("providers: %w", err)
	}
	if err := validateComponentRefs(m.RootDir, m.Components.Policies, false); err != nil {
		return fmt.Errorf("policies: %w", err)
	}
	if err := validateWorkflowContracts(m.RootDir, m.RuntimeSpec.WorkflowContracts); err != nil {
		return fmt.Errorf("workflow_contracts: %w", err)
	}
	for i, surface := range m.UISurfaces {
		result := capability.ValidateUISurface(surface, "")
		if !result.Valid {
			return fmt.Errorf("ui_surfaces[%d]: %s", i, strings.Join(result.Reasons, "; "))
		}
	}
	return nil
}

func (m Manifest) IsActive() bool {
	switch strings.ToLower(strings.TrimSpace(m.Status)) {
	case "disabled", "inactive", "retired", "deprecated", "draft":
		return false
	default:
		return true
	}
}

// ResolvesID reports whether this manifest is addressable by id, accounting for
// the Programmer<->Coder plugin alias (migration ticket NEW-A). It matches the
// manifest's own ID/PluginID and the Coder-facing alias of the legacy plugin id
// (e.g. a "navi.programmer" manifest resolves "navi.coder" and vice versa).
//
// Pure predicate: it has no effect on Normalize, Validate, or registration
// dedup. This is the seam OMN-283 wires into registry/lifecycle by-id lookups so
// runtime resolution accepts both namespaces during the package repositioning.
func (m Manifest) ResolvesID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	if id == strings.TrimSpace(m.ID) || id == strings.TrimSpace(m.PluginID) {
		return true
	}
	return coderalias.PluginIDsEquivalent(m.ID, id) || coderalias.PluginIDsEquivalent(m.PluginID, id)
}

func parseManifestRuntime(raw map[string]any) ManifestRuntime {
	if len(raw) == 0 {
		return ManifestRuntime{}
	}

	var spec ManifestRuntime
	workflowItems, ok := raw["workflow_contracts"].([]any)
	if !ok {
		return spec
	}
	for _, item := range workflowItems {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		spec.WorkflowContracts = append(spec.WorkflowContracts, ManifestWorkflowContract{
			WorkflowID: stringValue(entry["workflow_id"]),
			Name:       stringValue(entry["name"]),
			Path:       stringValue(entry["path"]),
			Runner:     stringValue(entry["runner"]),
			Extends:    stringValue(entry["extends"]),
			Status:     stringValue(entry["status"]),
		})
	}
	return spec
}

func validateWorkflowContracts(root string, refs []ManifestWorkflowContract) error {
	for _, ref := range refs {
		if strings.TrimSpace(ref.WorkflowID) == "" {
			return fmt.Errorf("workflow_id is required")
		}
		if err := validateRelativePluginPath(root, ref.Path, false, "workflow contract"); err != nil {
			return err
		}
		if strings.TrimSpace(ref.Runner) != "" {
			if err := validateRelativePluginPath(root, ref.Runner, false, "workflow runner"); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateRelativePluginPath(root, raw string, allowSkillFile bool, label string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("%s path is required", label)
	}
	clean := filepath.Clean(filepath.FromSlash(raw))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s path %q escapes plugin root", label, raw)
	}
	if root == "" {
		return nil
	}
	full := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s path %q escapes plugin root", label, raw)
	}
	if !allowSkillFile {
		if _, err := os.Stat(full); err != nil {
			return fmt.Errorf("%s path %q does not exist", label, raw)
		}
		return nil
	}
	info, err := os.Stat(full)
	if err != nil {
		return fmt.Errorf("%s path %q does not exist", label, raw)
	}
	if !info.IsDir() {
		base := strings.ToLower(filepath.Base(full))
		if base != "skill.yaml" && base != "skill.yml" && base != "skill.md" {
			return fmt.Errorf("skill component path %q must point to a skill directory or SKILL file", raw)
		}
		return nil
	}
	if _, err := os.Stat(filepath.Join(full, "SKILL.yaml")); err == nil {
		return nil
	}
	if _, err := os.Stat(filepath.Join(full, "SKILL.yml")); err == nil {
		return nil
	}
	if _, err := os.Stat(filepath.Join(full, "SKILL.md")); err == nil {
		return nil
	}
	return fmt.Errorf("skill component path %q does not contain SKILL.yaml or SKILL.md", raw)
}

func validateComponentRefs(root string, refs []ComponentRef, allowSkillFile bool) error {
	for _, ref := range refs {
		if err := validateRelativePluginPath(root, ref.Path, allowSkillFile, "component"); err != nil {
			return err
		}
	}
	return nil
}

// ManifestDiagnostic records why a discovered plugin manifest was skipped
// during loading. Invalid manifests are tracked rather than aborting the load.
type ManifestDiagnostic struct {
	Path     string `json:"path"`
	PluginID string `json:"plugin_id,omitempty"`
	Status   string `json:"status"` // "invalid", "parse_error"
	Error    string `json:"error"`
}

// ConnectorManifest describes a connector driver contributed by a plugin.
type ConnectorManifest struct {
	DriverID     string                        `json:"driver_id,omitempty" yaml:"driver_id,omitempty"`
	DisplayName  string                        `json:"display_name,omitempty" yaml:"display_name,omitempty"`
	Kind         string                        `json:"kind,omitempty" yaml:"kind,omitempty"` // "bridge", "subprocess", "builtin"
	Capabilities []ConnectorCapabilityManifest `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	Setup        *ConnectorSetupManifest       `json:"setup,omitempty" yaml:"setup,omitempty"`
	Command      string                        `json:"command,omitempty" yaml:"command,omitempty"`
	Args         []string                      `json:"args,omitempty" yaml:"args,omitempty"`
}

// ConnectorSetupManifest describes the schema for configuring the connector.
type ConnectorSetupManifest struct {
	RequiredParams []ConnectorSetupParam `json:"required_params,omitempty" yaml:"required_params,omitempty"`
	OptionalParams []ConnectorSetupParam `json:"optional_params,omitempty" yaml:"optional_params,omitempty"`
	SetupHint      string                `json:"setup_hint,omitempty" yaml:"setup_hint,omitempty"`
}

// ConnectorSetupParam describes a single configuration parameter.
type ConnectorSetupParam struct {
	Key         string `json:"key" yaml:"key"`
	Label       string `json:"label" yaml:"label"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Secret      bool   `json:"secret,omitempty" yaml:"secret,omitempty"`
	Placeholder string `json:"placeholder,omitempty" yaml:"placeholder,omitempty"`
}

// ConnectorCapabilityManifest is the manifest form of a connector capability.
type ConnectorCapabilityManifest struct {
	Name                string         `json:"name" yaml:"name"`
	InputSchema         map[string]any `json:"input_schema,omitempty" yaml:"input_schema,omitempty"`
	OutputSchema        map[string]any `json:"output_schema,omitempty" yaml:"output_schema,omitempty"`
	SideEffectClass     string         `json:"side_effect_class,omitempty" yaml:"side_effect_class,omitempty"`
	IdempotencyClass    string         `json:"idempotency_class,omitempty" yaml:"idempotency_class,omitempty"`
	Reversibility       string         `json:"reversibility,omitempty" yaml:"reversibility,omitempty"`
	RiskHint            string         `json:"risk_hint,omitempty" yaml:"risk_hint,omitempty"`
	RequiredPermissions []string       `json:"required_permissions,omitempty" yaml:"required_permissions,omitempty"`
	SupportsDryRun      bool           `json:"supports_dry_run,omitempty" yaml:"supports_dry_run,omitempty"`
	SupportsRetry       bool           `json:"supports_retry,omitempty" yaml:"supports_retry,omitempty"`
}

func (c ConnectorCapabilityManifest) inputSchemaJSON() json.RawMessage {
	if len(c.InputSchema) == 0 {
		return nil
	}
	b, err := json.Marshal(c.InputSchema)
	if err != nil {
		return nil
	}
	return b
}

func (c ConnectorCapabilityManifest) outputSchemaJSON() json.RawMessage {
	if len(c.OutputSchema) == 0 {
		return nil
	}
	b, err := json.Marshal(c.OutputSchema)
	if err != nil {
		return nil
	}
	return b
}
