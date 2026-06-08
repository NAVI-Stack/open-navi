package tool

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/open-navi/navi/internal/schema"
)

var canonicalToolIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:[_-][a-z0-9]+)*\.[a-z0-9]+(?:[_-][a-z0-9]+)*\.[a-z0-9]+(?:[_-][a-z0-9]+)*(?:\.[a-z0-9]+(?:[_-][a-z0-9]+)*)*$`)

// CanonicalID returns the stable tool identity used by the registry.
func (t *Tool) CanonicalID() string {
	if t == nil {
		return ""
	}
	if id := strings.TrimSpace(t.ToolID); id != "" {
		return id
	}
	if id := strings.TrimSpace(t.Name); id != "" {
		return id
	}
	return strings.TrimSpace(t.Definition.Name)
}

func (t *Tool) hasInputSchema() bool {
	return len(t.InputSchema) > 0 || strings.TrimSpace(t.InputSchemaRef) != ""
}

func (t *Tool) hasOutputSchema() bool {
	return len(t.OutputSchema) > 0 || strings.TrimSpace(t.OutputSchemaRef) != ""
}

func sliceWasSet(values []string) bool {
	return values != nil
}

func modeSliceWasSet(values []schema.DirectiveMode) bool {
	return values != nil
}

func isKnownToolCategory(category ToolCategory) bool {
	switch category {
	case ToolCategoryChatSafe,
		ToolCategoryReadOnly,
		ToolCategoryWorkflowAction,
		ToolCategoryOrchestrationMeta,
		ToolCategoryInternalDiagnostic,
		ToolCategoryDevTest,
		ToolCategoryAdminCritical:
		return true
	default:
		return false
	}
}

func isKnownToolAuthority(authority ToolAuthority) bool {
	switch authority {
	case ToolAuthorityUser, ToolAuthorityOwner, ToolAuthorityAdmin, ToolAuthoritySystem:
		return true
	default:
		return false
	}
}

func isKnownToolStatus(status ToolStatus) bool {
	switch status {
	case ToolStatusActive, ToolStatusDeprecated, ToolStatusSuspended, ToolStatusRemoved, ToolStatusInvalid:
		return true
	default:
		return false
	}
}

// NormalizeAndValidate enforces the canonical ATS-1 tool contract.
func (t *Tool) NormalizeAndValidate() error {
	if t == nil {
		return fmt.Errorf("tool contract: tool is nil")
	}

	toolID := t.CanonicalID()
	if toolID == "" {
		return fmt.Errorf("tool contract: tool_id is required")
	}
	if !canonicalToolIDPattern.MatchString(toolID) {
		return fmt.Errorf("tool contract: tool_id %q must use namespace.category.name identity", toolID)
	}
	t.ToolID = toolID
	t.Name = toolID

	if strings.TrimSpace(t.DisplayName) == "" {
		return fmt.Errorf("tool contract: display_name is required for %q", toolID)
	}

	description := strings.TrimSpace(t.Description)
	if description == "" {
		description = strings.TrimSpace(t.Definition.Description)
	}
	if description == "" {
		return fmt.Errorf("tool contract: description is required for %q", toolID)
	}
	t.Description = description

	if t.Source == "" {
		return fmt.Errorf("tool contract: source_type is required for %q", toolID)
	}
	if strings.TrimSpace(t.SourceID) == "" {
		return fmt.Errorf("tool contract: source_id is required for %q", toolID)
	}
	if strings.TrimSpace(t.SchemaVersion) == "" {
		return fmt.Errorf("tool contract: schema_version is required for %q", toolID)
	}

	if len(t.InputSchema) == 0 && len(t.Definition.Parameters) > 0 {
		t.InputSchema = cloneSchema(t.Definition.Parameters)
	}
	if !t.hasInputSchema() {
		return fmt.Errorf("tool contract: input schema is required for %q", toolID)
	}
	if !t.hasOutputSchema() {
		return fmt.Errorf("tool contract: output schema is required for %q", toolID)
	}

	if !isKnownToolCategory(t.Category) {
		return fmt.Errorf("tool contract: category %q is invalid for %q", t.Category, toolID)
	}
	if strings.TrimSpace(t.RiskTier) == "" {
		return fmt.Errorf("tool contract: risk_tier is required for %q", toolID)
	}
	if strings.TrimSpace(t.Reversibility) == "" {
		return fmt.Errorf("tool contract: reversibility is required for %q", toolID)
	}
	if len(t.EnvironmentVisibility) == 0 {
		return fmt.Errorf("tool contract: environment_visibility is required for %q", toolID)
	}
	if !isKnownToolAuthority(t.RequiredAuthority) {
		return fmt.Errorf("tool contract: required_authority %q is invalid for %q", t.RequiredAuthority, toolID)
	}
	if !isKnownToolStatus(t.Status) {
		return fmt.Errorf("tool contract: status %q is invalid for %q", t.Status, toolID)
	}

	if !sliceWasSet(t.SideEffects) {
		return fmt.Errorf("tool contract: side_effects must be set for %q", toolID)
	}
	if !modeSliceWasSet(t.RequiredModes) {
		return fmt.Errorf("tool contract: required_modes must be set for %q", toolID)
	}
	if !sliceWasSet(t.FeatureFlags) {
		return fmt.Errorf("tool contract: feature_flags must be set for %q", toolID)
	}
	if !sliceWasSet(t.ConnectorDependencies) {
		return fmt.Errorf("tool contract: connector_dependencies must be set for %q", toolID)
	}
	if !sliceWasSet(t.Aliases) {
		return fmt.Errorf("tool contract: aliases must be set for %q", toolID)
	}
	if !sliceWasSet(t.CapabilityTags) {
		return fmt.Errorf("tool contract: capability_tags must be set for %q", toolID)
	}

	for _, mode := range t.RequiredModes {
		if err := mode.Validate(); err != nil {
			return fmt.Errorf("tool contract: invalid required_mode for %q: %w", toolID, err)
		}
	}

	if t.Definition.Name == "" {
		t.Definition.Name = toolID
	}
	if t.Definition.Name != toolID {
		return fmt.Errorf("tool contract: definition name %q does not match tool_id %q", t.Definition.Name, toolID)
	}
	if t.Definition.Description == "" {
		t.Definition.Description = description
	}

	return nil
}

func cloneSchema(schema map[string]any) map[string]any {
	if len(schema) == 0 {
		return nil
	}
	out := make(map[string]any, len(schema))
	for key, value := range schema {
		out[key] = value
	}
	return out
}
