package navi

import (
	"fmt"
	"strings"

	"github.com/open-navi/navi/internal/llm"
	"github.com/open-navi/navi/internal/navi/filetools"
	"github.com/open-navi/navi/internal/navi/plugin"
	"github.com/open-navi/navi/internal/navi/selfmod"
	"github.com/open-navi/navi/internal/navi/skill"
	"github.com/open-navi/navi/internal/schema"
	navitool "github.com/open-navi/navi/internal/tool"
)

func buildRuntimeToolRegistry(cfg LoopConfig) (*navitool.Registry, error) {
	reg := navitool.NewRegistry()
	for _, adapter := range buildRuntimeToolRegistryAdapters(cfg) {
		if err := adapter.Register(reg); err != nil {
			return nil, err
		}
	}

	return reg, nil
}

func schemaCommandTypeForFileTool(name string) schema.CommandType {
	if name == filetools.WriteFileToolName {
		return schema.CommandTypeUpdate
	}
	return schema.CommandTypeQuery
}

func workspaceActionForFileTool(name string) schema.WorkspaceActionType {
	if name == filetools.WriteFileToolName {
		return schema.WorkspaceActionWrite
	}
	return schema.WorkspaceActionRead
}

func schemaCommandTypeForSelfModTool(name string) schema.CommandType {
	if name == selfmod.GitCommitToolName {
		return schema.CommandTypeCreate
	}
	return schema.CommandTypeQuery
}

func workspaceActionForSelfModTool(name string) schema.WorkspaceActionType {
	switch name {
	case selfmod.GitStatusToolName, selfmod.GitDiffToolName:
		return schema.WorkspaceActionRead
	default:
		return schema.WorkspaceActionExecute
	}
}

func firstSkillDomain(entry skill.SkillEntry) string {
	if entry.Spec == nil || len(entry.Spec.Capability.Domains) == 0 {
		return "misc"
	}
	return entry.Spec.Capability.Domains[0]
}

func buildSkillTools(registry *skill.SkillRegistry) ([]*navitool.Tool, error) {
	var tools []*navitool.Tool
	for _, entry := range registry.List() {
		if !entry.Activatable || entry.Spec == nil {
			continue
		}
		for _, iface := range entry.Spec.Interfaces {
			ifaceCopy := iface
			entryCopy := entry
			def := llm.ToolDefinition{
				Name:        toolNameForSkillInterface(entryCopy, ifaceCopy),
				Description: entryCopy.Skill.Description,
				Parameters:  ifaceCopy.InputSchema,
			}
			tools = append(tools, &navitool.Tool{
				ToolID:                def.Name,
				DisplayName:           displayNameForSkillTool(entryCopy, ifaceCopy),
				Description:           descriptionForSkillTool(entryCopy, ifaceCopy),
				Source:                navitool.ToolSourceSkill,
				SourceID:              entryCopy.Spec.SkillID,
				SchemaVersion:         firstNonEmpty(entryCopy.Spec.Semver, "1.0.0"),
				InputSchema:           inputSchemaOrEmpty(ifaceCopy.InputSchema),
				OutputSchema:          ifaceCopy.OutputSchema,
				Category:              categoryForSkillCommandType(skillCommandType(&entryCopy)),
				RiskTier:              firstNonEmpty(entryCopy.Spec.Effects.RiskTier, "low"),
				SideEffects:           append([]string{}, entryCopy.Spec.Effects.SideEffects...),
				Reversibility:         skillReversibility(entryCopy),
				EnvironmentVisibility: sharedToolEnvironments(),
				RequiredModes:         []schema.DirectiveMode{},
				RequiredAuthority:     navitool.ToolAuthorityUser,
				FeatureFlags:          []string{},
				ConnectorDependencies: []string{},
				Aliases:               []string{},
				CapabilityTags:        append([]string{}, entryCopy.Spec.Capability.Tags...),
				Status:                navitool.ToolStatusActive,
				Definition:            def,
				Executor:              navitool.NewSkillToolExecutor(&entryCopy, &ifaceCopy),
				Governance: navitool.ToolGovernance{
					CommandType:     skillCommandType(&entryCopy),
					WorkspaceAction: schema.WorkspaceActionForCommandType(skillCommandType(&entryCopy)),
					RequiresConfirm: entryCopy.Spec.Effects.RequiresConfirmation,
					RiskTier:        firstNonEmpty(entryCopy.Spec.Effects.RiskTier, "low"),
					Domain:          firstSkillDomain(entryCopy),
					Reversibility:   skillReversibility(entryCopy),
				},
				Metadata: navitool.ToolMetadata{
					SkillName:      entryCopy.Skill.Name,
					SkillInterface: ifaceCopy.Name,
					Component:      "skill",
					ErrorType:      "tool_execution_failed",
					Tags:           append([]string(nil), entryCopy.Spec.Capability.Tags...),
				},
			})
		}
	}
	return tools, nil
}

func toolNameForSkillInterface(entry skill.SkillEntry, iface skill.Interface) string {
	base := entry.Skill.Name
	if entry.Spec != nil && entry.Spec.SkillID != "" {
		base = entry.Spec.SkillID
	}
	return fmt.Sprintf("skill.%s.%s", sanitizeToolName(base), sanitizeToolName(iface.Name))
}

func sanitizeToolName(raw string) string {
	cleanName := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' {
			cleanName = append(cleanName, c)
		} else {
			cleanName = append(cleanName, '_')
		}
	}
	return string(cleanName)
}

func sharedToolEnvironments() []string {
	return []string{"development", "staging", "production"}
}

func emptyObjectSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func inputSchemaOrEmpty(schema map[string]any) map[string]any {
	if len(schema) == 0 {
		return emptyObjectSchema()
	}
	return schema
}

func stringOutputSchema(description string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
	}
}

func objectOutputSchema(properties map[string]any) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": true,
	}
	if properties != nil {
		schema["properties"] = properties
	}
	return schema
}

func displayNameForFileTool(name string) string {
	switch name {
	case filetools.ReadFileToolName:
		return "Read File"
	case filetools.ListDirToolName:
		return "List Directory"
	case filetools.WriteFileToolName:
		return "Write File"
	default:
		return name
	}
}

func categoryForFileTool(name string) navitool.ToolCategory {
	if name == filetools.WriteFileToolName {
		return navitool.ToolCategoryWorkflowAction
	}
	return navitool.ToolCategoryReadOnly
}

func riskTierForFileTool(name string) string {
	if name == filetools.WriteFileToolName {
		return "medium"
	}
	return "low"
}

func sideEffectsForFileTool(name string) []string {
	if name == filetools.WriteFileToolName {
		return []string{"workspace_write"}
	}
	return []string{"workspace_read"}
}

func aliasesForFileTool(name string) []string {
	switch name {
	case filetools.ReadFileToolName:
		return []string{"read file", "open file"}
	case filetools.ListDirToolName:
		return []string{"list directory", "list files"}
	case filetools.WriteFileToolName:
		return []string{"write file", "save file"}
	default:
		return []string{}
	}
}

func capabilityTagsForFileTool(name string) []string {
	switch name {
	case filetools.ReadFileToolName:
		return []string{"files", "workspace", "read"}
	case filetools.ListDirToolName:
		return []string{"files", "workspace", "list"}
	case filetools.WriteFileToolName:
		return []string{"files", "workspace", "write"}
	default:
		return []string{"files"}
	}
}

func displayNameForPluginTool(name string) string {
	switch name {
	case plugin.CalendarTimeWindowToolName:
		return "Calendar Current Time Window"
	case plugin.WorkflowTaskSummaryToolName:
		return "Workflow Task Summary"
	case plugin.IntegrationGitHubRepoToolName:
		return "GitHub Repository Info"
	case plugin.AgenticCriticRequestToolName:
		return "Agentic Critic Request"
	default:
		return name
	}
}

func categoryForPluginTool(name string) navitool.ToolCategory {
	switch name {
	case plugin.CalendarTimeWindowToolName, plugin.IntegrationGitHubRepoToolName:
		return navitool.ToolCategoryReadOnly
	default:
		return navitool.ToolCategoryWorkflowAction
	}
}

func riskTierForPluginTool(name string) string {
	switch name {
	case plugin.CalendarTimeWindowToolName, plugin.IntegrationGitHubRepoToolName:
		return "low"
	default:
		return "medium"
	}
}

func sideEffectsForPluginTool(name string) []string {
	switch name {
	case plugin.CalendarTimeWindowToolName, plugin.IntegrationGitHubRepoToolName:
		return []string{}
	default:
		return []string{"workflow_state_read"}
	}
}

func reversibilityForPluginTool(name string) string {
	switch name {
	case plugin.CalendarTimeWindowToolName, plugin.IntegrationGitHubRepoToolName:
		return "reversible"
	default:
		return "irreversible"
	}
}

func connectorDependenciesForPluginTool(pluginID string) []string {
	if pluginID == "" {
		return []string{}
	}
	return []string{pluginID}
}

func capabilityTagsForPluginTool(name string) []string {
	switch name {
	case plugin.CalendarTimeWindowToolName:
		return []string{"plugin", "calendar", "time"}
	case plugin.WorkflowTaskSummaryToolName:
		return []string{"plugin", "workflow", "tasks"}
	case plugin.IntegrationGitHubRepoToolName:
		return []string{"plugin", "github", "integration"}
	case plugin.AgenticCriticRequestToolName:
		return []string{"plugin", "critic", "agentic"}
	default:
		return []string{"plugin"}
	}
}

func outputSchemaForPluginTool(name string) map[string]any {
	switch name {
	case plugin.CalendarTimeWindowToolName:
		return objectOutputSchema(map[string]any{
			"start_utc":  map[string]any{"type": "string"},
			"end_utc":    map[string]any{"type": "string"},
			"local_time": map[string]any{"type": "string"},
			"timezone":   map[string]any{"type": "string"},
		})
	default:
		return objectOutputSchema(nil)
	}
}

func displayNameForSelfModTool(name string) string {
	switch name {
	case selfmod.GitStatusToolName:
		return "Git Status"
	case selfmod.GitDiffToolName:
		return "Git Diff"
	case selfmod.GitCommitToolName:
		return "Git Commit"
	case selfmod.GoBuildToolName:
		return "Go Build"
	case selfmod.GoTestToolName:
		return "Go Test"
	default:
		return name
	}
}

func categoryForSelfModTool(name string) navitool.ToolCategory {
	switch name {
	case selfmod.GitCommitToolName:
		return navitool.ToolCategoryAdminCritical
	default:
		return navitool.ToolCategoryDevTest
	}
}

func riskTierForSelfModTool(name string) string {
	switch name {
	case selfmod.GitCommitToolName:
		return "high"
	case selfmod.GoBuildToolName, selfmod.GoTestToolName:
		return "medium"
	default:
		return "low"
	}
}

func sideEffectsForSelfModTool(name string) []string {
	switch name {
	case selfmod.GitCommitToolName:
		return []string{"git_commit"}
	case selfmod.GoBuildToolName:
		return []string{"build_execution"}
	case selfmod.GoTestToolName:
		return []string{"test_execution"}
	default:
		return []string{"workspace_read"}
	}
}

func reversibilityForSelfModTool(name string) string {
	if name == selfmod.GitCommitToolName {
		return "irreversible"
	}
	return "reversible"
}

func capabilityTagsForSelfModTool(name string) []string {
	switch name {
	case selfmod.GitStatusToolName, selfmod.GitDiffToolName, selfmod.GitCommitToolName:
		return []string{"selfmod", "git", "codebase"}
	case selfmod.GoBuildToolName, selfmod.GoTestToolName:
		return []string{"selfmod", "go", "codebase"}
	default:
		return []string{"selfmod"}
	}
}

func displayNameForRouterTool(name string) string {
	switch name {
	case routerToolList:
		return "List Router Models"
	case routerToolGetActive:
		return "Get Active Router Model"
	case routerToolSetActive:
		return "Set Active Router Model"
	default:
		return name
	}
}

func descriptionForRouterTool(name string) string {
	switch name {
	case routerToolList:
		return "List available LLM providers and models."
	case routerToolGetActive:
		return "Get the active LLM provider and model."
	case routerToolSetActive:
		return "Set the active LLM provider and model."
	default:
		return name
	}
}

func riskTierForRouterTool(name string) string {
	if name == routerToolSetActive {
		return "medium"
	}
	return "low"
}

func sideEffectsForRouterTool(name string) []string {
	if name == routerToolSetActive {
		return []string{"llm_route_update"}
	}
	return []string{}
}

func reversibilityForRouterTool(name string) string {
	if name == routerToolSetActive {
		return "reversible"
	}
	return "reversible"
}

func displayNameForSkillTool(entry skill.SkillEntry, iface skill.Interface) string {
	return fmt.Sprintf("%s %s", entry.Skill.Name, iface.Name)
}

func descriptionForSkillTool(entry skill.SkillEntry, iface skill.Interface) string {
	// Each interface of a multi-interface skill needs a distinguishable
	// description; sharing the parent skill's blurb across every
	// schedule_task / list_tasks / update_task / delete_task / run_task_now
	// tool entry leaves the model unable to pick the right one. Prefix the
	// interface name so the LLM's tool list is self-describing.
	base := strings.TrimSpace(entry.Skill.Description)
	ifaceName := strings.TrimSpace(iface.Name)
	if base != "" {
		if ifaceName != "" {
			return fmt.Sprintf("%s — %s", ifaceName, base)
		}
		return base
	}
	return fmt.Sprintf("Execute the %s interface of the %s skill.", iface.Name, entry.Skill.Name)
}

func skillReversibility(entry skill.SkillEntry) string {
	if entry.Spec != nil && entry.Spec.Effects.Reversibility != "" {
		return entry.Spec.Effects.Reversibility
	}
	return "reversible"
}

func categoryForSkillCommandType(commandType schema.CommandType) navitool.ToolCategory {
	switch commandType {
	case schema.CommandTypeQuery:
		return navitool.ToolCategoryReadOnly
	case schema.CommandTypeCreate, schema.CommandTypeUpdate, schema.CommandTypeSend, schema.CommandTypeInvoke:
		return navitool.ToolCategoryWorkflowAction
	default:
		return navitool.ToolCategoryOrchestrationMeta
	}
}
