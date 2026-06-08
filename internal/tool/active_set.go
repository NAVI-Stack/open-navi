package tool

import (
	"fmt"
	"strings"
	"time"
)

// ActiveToolSetScope identifies the lifecycle scope of a broker-managed tool set.
type ActiveToolSetScope string

const (
	ActiveToolSetScopeTurn         ActiveToolSetScope = "turn"
	ActiveToolSetScopeSession      ActiveToolSetScope = "session"
	ActiveToolSetScopeWorkflow     ActiveToolSetScope = "workflow"
	ActiveToolSetScopeMode         ActiveToolSetScope = "mode"
	ActiveToolSetScopeProviderCall ActiveToolSetScope = "provider_call"
)

// ActiveToolSetTool records one loaded tool plus the schema version that was loaded.
type ActiveToolSetTool struct {
	ToolID        string     `json:"tool_id"`
	SchemaVersion string     `json:"schema_version"`
	LoadedAt      time.Time  `json:"loaded_at,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	LoadReason    string     `json:"load_reason,omitempty"`
}

// ActiveToolSetConstraints captures runtime constraints that later provider exposure
// and execution governance must honor.
type ActiveToolSetConstraints struct {
	MaxToolsExposed   int      `json:"max_tools_exposed,omitempty"`
	MaxParallelCalls  int      `json:"max_parallel_calls,omitempty"`
	RiskCeiling       string   `json:"risk_ceiling,omitempty"`
	ConfirmationRules []string `json:"confirmation_rules,omitempty"`
}

// ActiveToolSetProvenance captures the registry snapshot and broker context that
// produced the active set.
type ActiveToolSetProvenance struct {
	RegistrySnapshotID  string       `json:"registry_snapshot_id,omitempty"`
	BrokerIntent        BrokerIntent `json:"broker_intent,omitempty"`
	BrokerReason        string       `json:"broker_reason,omitempty"`
	PreviousActiveSetID string       `json:"previous_active_tool_set_id,omitempty"`
	DroppedToolIDs      []string     `json:"dropped_tool_ids,omitempty"`
}

// ActiveToolSet is the runtime boundary for loaded tools. It is intentionally
// distinct from provider exposure and from execution authorization.
type ActiveToolSet struct {
	ActiveToolSetID string                   `json:"active_tool_set_id"`
	Scope           ActiveToolSetScope       `json:"scope"`
	ChatID          string                   `json:"chat_id,omitempty"`
	WorkflowID      string                   `json:"workflow_id,omitempty"`
	Mode            string                   `json:"mode,omitempty"`
	Environment     string                   `json:"environment,omitempty"`
	ModelProfile    string                   `json:"model_profile,omitempty"`
	Tools           []ActiveToolSetTool      `json:"tools,omitempty"`
	LoadedAt        time.Time                `json:"loaded_at,omitempty"`
	ExpiresAt       *time.Time               `json:"expires_at,omitempty"`
	LoadReason      string                   `json:"load_reason,omitempty"`
	Constraints     ActiveToolSetConstraints `json:"constraints,omitempty"`
	Provenance      ActiveToolSetProvenance  `json:"provenance,omitempty"`
}

// ActiveToolSetSpec is the broker-owned input for creating one runtime tool set.
type ActiveToolSetSpec struct {
	Scope        ActiveToolSetScope
	ChatID       string
	WorkflowID   string
	Mode         string
	Environment  string
	ModelProfile string
	ToolIDs      []string
	LoadedAt     time.Time
	ExpiresAt    *time.Time
	LoadReason   string
	Constraints  ActiveToolSetConstraints
}

// BuildActiveToolSet creates a runtime active tool set from a broker resolution or
// from an explicitly supplied tool id subset. The broker remains the only creator
// of these objects.
func (b *ToolBroker) BuildActiveToolSet(input BrokerInput, resolution BrokerResolution, spec ActiveToolSetSpec) ActiveToolSet {
	input = input.normalize()
	if spec.Scope == "" {
		spec.Scope = ActiveToolSetScopeTurn
	}
	if spec.LoadedAt.IsZero() {
		spec.LoadedAt = time.Now().UTC()
	}
	if strings.TrimSpace(spec.Environment) == "" {
		spec.Environment = input.Environment
	}
	if strings.TrimSpace(spec.ModelProfile) == "" {
		spec.ModelProfile = strings.TrimSpace(input.ModelProfile.Name)
	}
	if strings.TrimSpace(spec.Mode) == "" {
		spec.Mode = string(input.SessionMode)
	}
	if strings.TrimSpace(spec.LoadReason) == "" {
		spec.LoadReason = firstNonEmpty(strings.TrimSpace(resolution.BrokerReason), "broker_selected_tools")
	}
	if spec.Constraints.MaxToolsExposed <= 0 {
		spec.Constraints.MaxToolsExposed = maxInt(input.ModelProfile.maxToolCount(), len(resolution.SelectedToolIDs))
	}
	if spec.Constraints.MaxParallelCalls <= 0 {
		spec.Constraints.MaxParallelCalls = 1
	}
	if spec.Constraints.ConfirmationRules == nil {
		spec.Constraints.ConfirmationRules = []string{"respect_tool_governance_requires_confirm"}
	}
	if strings.TrimSpace(spec.Constraints.RiskCeiling) == "" {
		spec.Constraints.RiskCeiling = firstNonEmpty(strings.TrimSpace(input.MaximumRiskTier), "medium")
	}

	selectedToolIDs := dedupeStrings(spec.ToolIDs)
	if len(selectedToolIDs) == 0 {
		selectedToolIDs = dedupeStrings(resolution.SelectedToolIDs)
	}

	tools := make([]ActiveToolSetTool, 0, len(selectedToolIDs))
	droppedToolIDs := make([]string, 0)
	if b != nil && b.index != nil {
		for _, toolID := range selectedToolIDs {
			doc, ok := b.index.byID[normalizeSearchText(toolID)]
			if !ok || doc.tool == nil {
				droppedToolIDs = append(droppedToolIDs, toolID)
				continue
			}
			tools = append(tools, ActiveToolSetTool{
				ToolID:        doc.tool.ToolID,
				SchemaVersion: strings.TrimSpace(doc.tool.SchemaVersion),
				LoadedAt:      spec.LoadedAt,
				ExpiresAt:     spec.ExpiresAt,
				LoadReason:    spec.LoadReason,
			})
		}
	} else {
		droppedToolIDs = append(droppedToolIDs, selectedToolIDs...)
	}

	return ActiveToolSet{
		ActiveToolSetID: newActiveToolSetID(spec.Scope),
		Scope:           spec.Scope,
		ChatID:          strings.TrimSpace(spec.ChatID),
		WorkflowID:      strings.TrimSpace(spec.WorkflowID),
		Mode:            strings.TrimSpace(spec.Mode),
		Environment:     strings.TrimSpace(spec.Environment),
		ModelProfile:    strings.TrimSpace(spec.ModelProfile),
		Tools:           tools,
		LoadedAt:        spec.LoadedAt,
		ExpiresAt:       spec.ExpiresAt,
		LoadReason:      strings.TrimSpace(spec.LoadReason),
		Constraints: ActiveToolSetConstraints{
			MaxToolsExposed:   spec.Constraints.MaxToolsExposed,
			MaxParallelCalls:  spec.Constraints.MaxParallelCalls,
			RiskCeiling:       strings.TrimSpace(spec.Constraints.RiskCeiling),
			ConfirmationRules: append([]string(nil), spec.Constraints.ConfirmationRules...),
		},
		Provenance: ActiveToolSetProvenance{
			RegistrySnapshotID:  strings.TrimSpace(firstNonEmpty(resolution.SnapshotID, snapshotIDFromIndex(b))),
			BrokerIntent:        resolution.Intent,
			BrokerReason:        strings.TrimSpace(resolution.BrokerReason),
			PreviousActiveSetID: strings.TrimSpace(input.ActiveToolSetID),
			DroppedToolIDs:      dedupeStrings(droppedToolIDs),
		},
	}
}

// ToolIDs returns the canonical tool ids loaded in this active tool set.
func (s ActiveToolSet) ToolIDs() []string {
	out := make([]string, 0, len(s.Tools))
	for _, tool := range s.Tools {
		if toolID := strings.TrimSpace(tool.ToolID); toolID != "" {
			out = append(out, toolID)
		}
	}
	return dedupeStrings(out)
}

// ContainsTool reports whether the active set currently contains the canonical tool id.
func (s ActiveToolSet) ContainsTool(toolID string) bool {
	return s.ContainsToolAt(toolID, time.Now().UTC())
}

// ContainsToolVersion reports whether the active set contains the canonical tool id
// with the exact loaded schema version.
func (s ActiveToolSet) ContainsToolVersion(toolID string, schemaVersion string) bool {
	return s.ContainsToolVersionAt(toolID, schemaVersion, time.Now().UTC())
}

// IsExpiredAt reports whether the active tool set should be treated as stale at
// the supplied time.
func (s ActiveToolSet) IsExpiredAt(at time.Time) bool {
	if s.ExpiresAt == nil {
		return false
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return !at.Before((*s.ExpiresAt).UTC())
}

// ContainsToolAt reports whether the active set contains a still-usable canonical
// tool id at the supplied time.
func (s ActiveToolSet) ContainsToolAt(toolID string, at time.Time) bool {
	toolID = strings.TrimSpace(toolID)
	if toolID == "" || s.IsExpiredAt(at) {
		return false
	}
	for _, tool := range s.Tools {
		if !activeToolSetToolUsableAt(tool, at) {
			continue
		}
		if strings.TrimSpace(tool.ToolID) == toolID {
			return true
		}
	}
	return false
}

// ContainsToolVersionAt reports whether the active set contains the canonical
// tool id with the exact loaded schema version and without expiry at the
// supplied time.
func (s ActiveToolSet) ContainsToolVersionAt(toolID string, schemaVersion string, at time.Time) bool {
	toolID = strings.TrimSpace(toolID)
	schemaVersion = strings.TrimSpace(schemaVersion)
	if toolID == "" || schemaVersion == "" || s.IsExpiredAt(at) {
		return false
	}
	for _, tool := range s.Tools {
		if !activeToolSetToolUsableAt(tool, at) {
			continue
		}
		if strings.TrimSpace(tool.ToolID) == toolID && strings.TrimSpace(tool.SchemaVersion) == schemaVersion {
			return true
		}
	}
	return false
}

func activeToolSetToolUsableAt(tool ActiveToolSetTool, at time.Time) bool {
	if tool.ExpiresAt == nil {
		return true
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return at.Before((*tool.ExpiresAt).UTC())
}

func newActiveToolSetID(scope ActiveToolSetScope) string {
	prefix := strings.TrimSpace(string(scope))
	if prefix == "" {
		prefix = "ats"
	}
	return fmt.Sprintf("ats_%s_%d", prefix, time.Now().UTC().UnixNano())
}

func snapshotIDFromIndex(b *ToolBroker) string {
	if b == nil || b.index == nil {
		return ""
	}
	return strings.TrimSpace(b.index.SnapshotID())
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}
