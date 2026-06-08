package tool

import (
	"context"

	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/schema"
)

// ToolSource identifies where a tool originated.
type ToolSource string

const (
	ToolSourceFileTools ToolSource = "file_tools"
	ToolSourceSkill     ToolSource = "skill"
	ToolSourcePlugin    ToolSource = "plugin"
	ToolSourceSelfMod   ToolSource = "selfmod"
	ToolSourceBuiltin   ToolSource = "builtin"
)

// ToolCategory classifies a tool for governance and discoverability.
type ToolCategory string

const (
	ToolCategoryChatSafe           ToolCategory = "chat_safe"
	ToolCategoryReadOnly           ToolCategory = "read_only"
	ToolCategoryWorkflowAction     ToolCategory = "workflow_action"
	ToolCategoryOrchestrationMeta  ToolCategory = "orchestration_meta"
	ToolCategoryInternalDiagnostic ToolCategory = "internal_diagnostic"
	ToolCategoryDevTest            ToolCategory = "dev_test"
	ToolCategoryAdminCritical      ToolCategory = "admin_critical"
)

// ToolAuthority describes which runtime authority tier may invoke a tool.
type ToolAuthority string

const (
	ToolAuthorityUser   ToolAuthority = "user"
	ToolAuthorityOwner  ToolAuthority = "owner"
	ToolAuthorityAdmin  ToolAuthority = "admin"
	ToolAuthoritySystem ToolAuthority = "system"
)

// ToolStatus captures the lifecycle status of a registered tool.
type ToolStatus string

const (
	ToolStatusActive     ToolStatus = "active"
	ToolStatusDeprecated ToolStatus = "deprecated"
	ToolStatusSuspended  ToolStatus = "suspended"
	ToolStatusRemoved    ToolStatus = "removed"
	ToolStatusInvalid    ToolStatus = "invalid"
)

// ToolExposureClass is the registry-backed exposure intent consumed by NCOS
// capability-surface resolution. These values intentionally mirror the Phase 2
// contract without introducing a second tool model.
type ToolExposureClass string

const (
	ToolExposureUserFacing  ToolExposureClass = "user_facing"
	ToolExposureInternal    ToolExposureClass = "internal"
	ToolExposureDevelopment ToolExposureClass = "development"
	ToolExposureTest        ToolExposureClass = "test"
)

// ToolInteractionMode describes which request classes a tool is intended to
// participate in when NCOS resolves capability surfaces.
type ToolInteractionMode string

const (
	ToolInteractionConversation ToolInteractionMode = "conversation"
	ToolInteractionAction       ToolInteractionMode = "action"
)

// ToolResult is the normalized executor result for unified tool dispatch.
// The payload stays opaque so existing executors can return their native
// structured content without being forced through a second envelope.
type ToolResult struct {
	Content any
}

// ToolExecutor runs a tool invocation.
type ToolExecutor interface {
	Execute(ctx context.Context, args map[string]any) (ToolResult, error)
}

// ToolGovernance captures the policy surface associated with a tool.
type ToolGovernance struct {
	CommandType     schema.CommandType         `json:"command_type,omitempty"`
	WorkspaceAction schema.WorkspaceActionType `json:"workspace_action,omitempty"`
	RequiresConfirm bool                       `json:"requires_confirm,omitempty"`
	RiskTier        string                     `json:"risk_tier,omitempty"`
	Domain          string                     `json:"domain,omitempty"`
	PathRestriction string                     `json:"path_restriction,omitempty"`
	Reversibility   string                     `json:"reversibility,omitempty"` // "reversible" | "irreversible"
}

// ToolMetadata captures provenance plus NCOS exposure hints for a tool.
type ToolMetadata struct {
	SkillName                string                `json:"skill_name,omitempty"`
	SkillInterface           string                `json:"skill_interface,omitempty"`
	PluginID                 string                `json:"plugin_id,omitempty"`
	Component                string                `json:"component,omitempty"`  // "skill" | "plugin" | "runtime" | "selfmod" | "llm"
	ErrorType                string                `json:"error_type,omitempty"` // default error type for this tool
	TrustTier                string                `json:"trust_tier,omitempty"`
	ExposureClass            ToolExposureClass     `json:"exposure_class,omitempty"`
	InteractionModes         []ToolInteractionMode `json:"interaction_modes,omitempty"`
	RequiresToolCapableModel *bool                 `json:"requires_tool_capable_model,omitempty"`
	Tags                     []string              `json:"tags,omitempty"`
	Notes                    map[string]string     `json:"notes,omitempty"`
}

// Tool is the first-class representation of an LLM-callable action.
type Tool struct {
	ToolID                string                 `json:"tool_id"`
	Name                  string                 `json:"name,omitempty"` // Deprecated alias for ToolID.
	DisplayName           string                 `json:"display_name"`
	Description           string                 `json:"description"`
	Source                ToolSource             `json:"source_type"`
	SourceID              string                 `json:"source_id"`
	SchemaVersion         string                 `json:"schema_version"`
	InputSchema           map[string]any         `json:"input_schema,omitempty"`
	InputSchemaRef        string                 `json:"input_schema_ref,omitempty"`
	OutputSchema          map[string]any         `json:"output_schema,omitempty"`
	OutputSchemaRef       string                 `json:"output_schema_ref,omitempty"`
	Category              ToolCategory           `json:"category"`
	RiskTier              string                 `json:"risk_tier"`
	SideEffects           []string               `json:"side_effects"`
	Reversibility         string                 `json:"reversibility"`
	EnvironmentVisibility []string               `json:"environment_visibility"`
	RequiredModes         []schema.DirectiveMode `json:"required_modes"`
	RequiredAuthority     ToolAuthority          `json:"required_authority"`
	FeatureFlags          []string               `json:"feature_flags"`
	ConnectorDependencies []string               `json:"connector_dependencies"`
	Aliases               []string               `json:"aliases"`
	CapabilityTags        []string               `json:"capability_tags"`
	Status                ToolStatus             `json:"status"`
	Hidden                bool                   `json:"hidden,omitempty"`
	VisibleOn             []string               `json:"visible_on,omitempty"`
	Definition            llm.ToolDefinition     `json:"definition"`
	Executor              ToolExecutor           `json:"-"`
	Governance            ToolGovernance         `json:"governance,omitempty"`
	Metadata              ToolMetadata           `json:"metadata,omitempty"`
}
