package navi

import (
	"context"
	"fmt"

	"github.com/ceoai/navi/internal/llm"
	"github.com/ceoai/navi/internal/navi/filetools"
	"github.com/ceoai/navi/internal/navi/selfmod"
	"github.com/ceoai/navi/internal/navi/skill"
	"github.com/ceoai/navi/internal/schema"
	navitool "github.com/ceoai/navi/internal/tool"
)

type runtimeToolRegistryAdapter interface {
	Register(*navitool.Registry) error
}

type runtimeToolListAdapter struct {
	build func() ([]*navitool.Tool, error)
}

func (a runtimeToolListAdapter) Tools() ([]*navitool.Tool, error) {
	if a.build == nil {
		return nil, nil
	}
	return a.build()
}

func (a runtimeToolListAdapter) Register(reg *navitool.Registry) error {
	if reg == nil {
		return fmt.Errorf("tool registry adapter: registry is nil")
	}
	tools, err := a.Tools()
	if err != nil {
		return err
	}
	return registerRuntimeTools(reg, tools)
}

func buildRuntimeToolRegistryAdapters(cfg LoopConfig) []runtimeToolRegistryAdapter {
	adapters := make([]runtimeToolRegistryAdapter, 0, 5)
	if cfg.WorkspaceDir != "" {
		adapters = append(adapters, newFileToolRegistryAdapter(cfg))
	}
	if cfg.Skills != nil {
		adapters = append(adapters, newSkillToolRegistryAdapter(cfg.Skills))
	}
	if cfg.PluginRegistry != nil {
		adapters = append(adapters, newPluginToolRegistryAdapter(cfg))
	}
	if cfg.SelfModExecutor != nil {
		adapters = append(adapters, newSelfModToolRegistryAdapter(cfg))
	}
	adapters = append(adapters, newBuiltinToolRegistryAdapter(cfg))
	return adapters
}

func registerRuntimeTools(reg *navitool.Registry, tools []*navitool.Tool) error {
	for _, toolEntry := range tools {
		if err := reg.Register(toolEntry); err != nil {
			return err
		}
	}
	return nil
}

func newFileToolRegistryAdapter(cfg LoopConfig) runtimeToolListAdapter {
	return runtimeToolListAdapter{
		build: func() ([]*navitool.Tool, error) {
			var checker filetools.PathChecker
			if cfg.Governor != nil {
				checker = cfg.Governor
			}
			tools := make([]*navitool.Tool, 0, len(filetools.Tools()))
			for _, def := range filetools.Tools() {
				gov := navitool.ToolGovernance{
					CommandType:     schemaCommandTypeForFileTool(def.Name),
					WorkspaceAction: workspaceActionForFileTool(def.Name),
					Domain:          "files",
					PathRestriction: "path",
					Reversibility:   "reversible",
				}
				if def.Name == filetools.WriteFileToolName {
					gov.Reversibility = "irreversible"
				}
				tools = append(tools, &navitool.Tool{
					ToolID:                def.Name,
					DisplayName:           displayNameForFileTool(def.Name),
					Description:           def.Description,
					Source:                navitool.ToolSourceFileTools,
					SourceID:              "navi.workspace.filetools",
					SchemaVersion:         "1.0.0",
					InputSchema:           inputSchemaOrEmpty(def.Parameters),
					OutputSchema:          stringOutputSchema("Workspace file tool result."),
					Category:              categoryForFileTool(def.Name),
					RiskTier:              riskTierForFileTool(def.Name),
					SideEffects:           sideEffectsForFileTool(def.Name),
					Reversibility:         gov.Reversibility,
					EnvironmentVisibility: sharedToolEnvironments(),
					RequiredModes:         []schema.DirectiveMode{},
					RequiredAuthority:     navitool.ToolAuthorityUser,
					FeatureFlags:          []string{},
					ConnectorDependencies: []string{},
					Aliases:               aliasesForFileTool(def.Name),
					CapabilityTags:        capabilityTagsForFileTool(def.Name),
					Status:                navitool.ToolStatusActive,
					Definition:            def,
					Executor:              navitool.NewFileToolExecutor(def.Name, cfg.WorkspaceDir, checker),
					Governance:            gov,
					Metadata: navitool.ToolMetadata{
						Component:        "runtime",
						ErrorType:        "file_tool_failed",
						InteractionModes: []navitool.ToolInteractionMode{navitool.ToolInteractionAction},
					},
				})
			}
			return tools, nil
		},
	}
}

func newSkillToolRegistryAdapter(registry *skill.SkillRegistry) runtimeToolListAdapter {
	return runtimeToolListAdapter{
		build: func() ([]*navitool.Tool, error) {
			return buildSkillTools(registry)
		},
	}
}

func newPluginToolRegistryAdapter(cfg LoopConfig) runtimeToolListAdapter {
	return runtimeToolListAdapter{
		build: func() ([]*navitool.Tool, error) {
			tools := make([]*navitool.Tool, 0, len(cfg.PluginRegistry.ToolEntries()))
			for _, entry := range cfg.PluginRegistry.ToolEntries() {
				def := entry.Definition
				tools = append(tools, &navitool.Tool{
					ToolID:                def.Name,
					DisplayName:           displayNameForPluginTool(def.Name),
					Description:           def.Description,
					Source:                navitool.ToolSourcePlugin,
					SourceID:              firstNonEmpty(entry.PluginID, "navi.plugin.inline"),
					SchemaVersion:         "1.0.0",
					InputSchema:           inputSchemaOrEmpty(def.Parameters),
					OutputSchema:          outputSchemaForPluginTool(def.Name),
					Category:              categoryForPluginTool(def.Name),
					RiskTier:              riskTierForPluginTool(def.Name),
					SideEffects:           sideEffectsForPluginTool(def.Name),
					Reversibility:         reversibilityForPluginTool(def.Name),
					EnvironmentVisibility: sharedToolEnvironments(),
					RequiredModes:         []schema.DirectiveMode{},
					RequiredAuthority:     navitool.ToolAuthorityUser,
					FeatureFlags:          []string{},
					ConnectorDependencies: connectorDependenciesForPluginTool(entry.PluginID),
					Aliases:               []string{},
					CapabilityTags:        capabilityTagsForPluginTool(def.Name),
					Status:                navitool.ToolStatusActive,
					Definition:            def,
					Executor: navitool.NewBuiltinPluginExecutor(def.Name, func(ctx context.Context) string {
						if cfg.WorldModel == nil {
							return "UTC"
						}
						if tz := cfg.WorldModel.GetOwnerTimezone(ctx); tz != "" {
							return tz
						}
						return "UTC"
					}),
					Governance: navitool.ToolGovernance{
						CommandType:     schema.CommandTypeInvoke,
						WorkspaceAction: schema.WorkspaceActionExecute,
						Domain:          "plugins",
						Reversibility:   reversibilityForPluginTool(def.Name),
					},
					Metadata: navitool.ToolMetadata{
						PluginID:  firstNonEmpty(entry.PluginID, "navi.plugin.inline"),
						Component: "plugin",
						ErrorType: "tool_execution_failed",
					},
				})
			}
			return tools, nil
		},
	}
}

func newSelfModToolRegistryAdapter(cfg LoopConfig) runtimeToolListAdapter {
	return runtimeToolListAdapter{
		build: func() ([]*navitool.Tool, error) {
			tools := make([]*navitool.Tool, 0, len(selfmod.Tools()))
			for _, def := range selfmod.Tools() {
				reversibility := reversibilityForSelfModTool(def.Name)
				tools = append(tools, &navitool.Tool{
					ToolID:                def.Name,
					DisplayName:           displayNameForSelfModTool(def.Name),
					Description:           def.Description,
					Source:                navitool.ToolSourceSelfMod,
					SourceID:              "navi.selfmod.executor",
					SchemaVersion:         "1.0.0",
					InputSchema:           inputSchemaOrEmpty(def.Parameters),
					OutputSchema:          stringOutputSchema("Self-modification tool result."),
					Category:              categoryForSelfModTool(def.Name),
					RiskTier:              riskTierForSelfModTool(def.Name),
					SideEffects:           sideEffectsForSelfModTool(def.Name),
					Reversibility:         reversibility,
					EnvironmentVisibility: sharedToolEnvironments(),
					RequiredModes:         []schema.DirectiveMode{schema.DirectiveModeAct},
					RequiredAuthority:     navitool.ToolAuthorityUser,
					FeatureFlags:          []string{},
					ConnectorDependencies: []string{},
					Aliases:               []string{},
					CapabilityTags:        capabilityTagsForSelfModTool(def.Name),
					Status:                navitool.ToolStatusActive,
					Definition:            def,
					Executor:              navitool.NewSelfModToolExecutor(def.Name, cfg.SelfModExecutor),
					Governance: navitool.ToolGovernance{
						CommandType:     schemaCommandTypeForSelfModTool(def.Name),
						WorkspaceAction: workspaceActionForSelfModTool(def.Name),
						RequiresConfirm: def.Name == selfmod.GitCommitToolName,
						Domain:          "selfmod",
						Reversibility:   reversibility,
					},
					Metadata: navitool.ToolMetadata{
						Component: "selfmod",
						ErrorType: "selfmod_tool_failed",
						Tags:      []string{"selfmod", "codebase"},
					},
				})
			}
			return tools, nil
		},
	}
}

func newBuiltinToolRegistryAdapter(cfg LoopConfig) runtimeToolListAdapter {
	return runtimeToolListAdapter{
		build: func() ([]*navitool.Tool, error) {
			tools := make([]*navitool.Tool, 0, 6)

			sendReply := sendReplyToolDefinition()
			tools = append(tools, &navitool.Tool{
				ToolID:                sendReply.Name,
				DisplayName:           "Send Reply",
				Description:           sendReply.Description,
				Source:                navitool.ToolSourceBuiltin,
				SourceID:              "navi.runtime.executor",
				SchemaVersion:         "1.0.0",
				InputSchema:           sendReply.Parameters,
				OutputSchema:          stringOutputSchema("Queued send reply result."),
				Category:              navitool.ToolCategoryWorkflowAction,
				RiskTier:              "medium",
				SideEffects:           []string{"message_queue_write"},
				Reversibility:         "irreversible",
				EnvironmentVisibility: sharedToolEnvironments(),
				RequiredModes:         []schema.DirectiveMode{},
				RequiredAuthority:     navitool.ToolAuthorityUser,
				FeatureFlags:          []string{},
				ConnectorDependencies: []string{},
				Aliases:               []string{"send reply"},
				CapabilityTags:        []string{"builtin", "reply", "messaging"},
				Status:                navitool.ToolStatusActive,
				Definition:            sendReply,
				Executor:              newSendReplyToolExecutor(),
				VisibleOn:             []string{runtimeToolSurface},
				Governance: navitool.ToolGovernance{
					CommandType:     schema.CommandTypeSend,
					WorkspaceAction: schema.WorkspaceActionExecute,
					Domain:          "messaging",
				},
				Metadata: navitool.ToolMetadata{
					Component: "messaging",
					ErrorType: "tool_execution_failed",
					Tags:      []string{"builtin", "reply"},
					Notes: map[string]string{
						"phase": "phase_1_runtime_visible",
					},
				},
			})

			renderVisualize := renderVisualizeToolDefinition()
			tools = append(tools, &navitool.Tool{
				ToolID:                renderVisualize.Name,
				DisplayName:           "Visualize Tool Usage",
				Description:           renderVisualize.Description,
				Source:                navitool.ToolSourceBuiltin,
				SourceID:              "navi.render",
				SchemaVersion:         "1.0.0",
				InputSchema:           renderVisualize.Parameters,
				OutputSchema:          stringOutputSchema("Render confirmation summary."),
				Category:              navitool.ToolCategoryWorkflowAction,
				RiskTier:              "low",
				SideEffects:           []string{},
				Reversibility:         "reversible_internal",
				EnvironmentVisibility: sharedToolEnvironments(),
				RequiredModes:         []schema.DirectiveMode{},
				RequiredAuthority:     navitool.ToolAuthorityUser,
				FeatureFlags:          []string{},
				ConnectorDependencies: []string{},
				Aliases:               []string{"visualize tool usage", "graph tool usage"},
				CapabilityTags:        []string{"builtin", "render", "data-view", "read-only"},
				Status:                navitool.ToolStatusActive,
				Definition:            renderVisualize,
				Executor:              newRenderToolExecutor(cfg),
				VisibleOn:             []string{runtimeToolSurface},
				Governance: navitool.ToolGovernance{
					// Read-only: queries chat-scoped tool usage and renders it. No
					// privileged action, no workspace mutation.
					CommandType:     schema.CommandTypeQuery,
					WorkspaceAction: schema.WorkspaceActionRead,
					Domain:          "render",
				},
				Metadata: navitool.ToolMetadata{
					Component: "render",
					ErrorType: "render_tool_failed",
					Tags:      []string{"builtin", "render", "read-only"},
				},
			})

			routerTools := []struct {
				name        string
				executor    navitool.ToolExecutor
				commandType schema.CommandType
				domain      string
				component   string
				errorType   string
			}{
				{
					name: routerToolList,
					executor: navitool.NewRouterToolExecutor(
						routerToolList,
						cfg.LLMService,
					),
					commandType: schema.CommandTypeQuery,
					domain:      "llm",
					component:   "llm",
					errorType:   "llm_router_failed",
				},
				{
					name: routerToolGetActive,
					executor: navitool.NewRouterToolExecutor(
						routerToolGetActive,
						cfg.LLMService,
					),
					commandType: schema.CommandTypeQuery,
					domain:      "llm",
					component:   "llm",
					errorType:   "llm_router_failed",
				},
				{
					name: routerToolSetActive,
					executor: navitool.NewRouterToolExecutor(
						routerToolSetActive,
						cfg.LLMService,
					),
					commandType: schema.CommandTypeUpdate,
					domain:      "llm",
					component:   "llm",
					errorType:   "llm_router_failed",
				},
			}

			for _, builtin := range routerTools {
				tools = append(tools, &navitool.Tool{
					ToolID:                builtin.name,
					DisplayName:           displayNameForRouterTool(builtin.name),
					Description:           descriptionForRouterTool(builtin.name),
					Source:                navitool.ToolSourceBuiltin,
					SourceID:              "navi.llm.router",
					SchemaVersion:         "1.0.0",
					InputSchema:           emptyObjectSchema(),
					OutputSchema:          objectOutputSchema(nil),
					Category:              navitool.ToolCategoryOrchestrationMeta,
					RiskTier:              riskTierForRouterTool(builtin.name),
					SideEffects:           sideEffectsForRouterTool(builtin.name),
					Reversibility:         reversibilityForRouterTool(builtin.name),
					EnvironmentVisibility: sharedToolEnvironments(),
					RequiredModes:         []schema.DirectiveMode{schema.DirectiveModeAssist, schema.DirectiveModeAct},
					RequiredAuthority:     navitool.ToolAuthorityUser,
					FeatureFlags:          []string{},
					ConnectorDependencies: []string{},
					Aliases:               []string{},
					CapabilityTags:        []string{"builtin", "llm", "router"},
					Status:                navitool.ToolStatusActive,
					Hidden:                true,
					Definition:            llm.ToolDefinition{Name: builtin.name, Description: descriptionForRouterTool(builtin.name), Parameters: emptyObjectSchema()},
					Executor:              builtin.executor,
					Governance: navitool.ToolGovernance{
						CommandType:     builtin.commandType,
						WorkspaceAction: schema.WorkspaceActionForCommandType(builtin.commandType),
						Domain:          builtin.domain,
					},
					Metadata: navitool.ToolMetadata{
						Component: builtin.component,
						ErrorType: builtin.errorType,
						Tags:      []string{"builtin", "hidden"},
					},
				})
			}

			tools = append(tools, &navitool.Tool{
				ToolID:                "navi.onboarding.set_timezone",
				DisplayName:           "Set Owner Timezone",
				Description:           "Persist the owner's IANA timezone.",
				Source:                navitool.ToolSourceBuiltin,
				SourceID:              "navi.onboarding",
				SchemaVersion:         "1.0.0",
				OutputSchema:          stringOutputSchema("Timezone update result."),
				Category:              navitool.ToolCategoryWorkflowAction,
				RiskTier:              "medium",
				SideEffects:           []string{"settings_write"},
				Reversibility:         "reversible",
				EnvironmentVisibility: sharedToolEnvironments(),
				RequiredModes:         []schema.DirectiveMode{schema.DirectiveModeAssist, schema.DirectiveModeAct},
				RequiredAuthority:     navitool.ToolAuthorityUser,
				FeatureFlags:          []string{},
				ConnectorDependencies: []string{},
				Aliases:               []string{"set timezone"},
				CapabilityTags:        []string{"builtin", "onboarding", "timezone"},
				Status:                navitool.ToolStatusActive,
				Hidden:                false,
				Definition: llm.ToolDefinition{
					Name:        "navi.onboarding.set_timezone",
					Description: "Persist the owner's IANA timezone (e.g. America/Los_Angeles, America/New_York, Europe/London, UTC). Call this whenever the user states or confirms their local timezone so future time references are accurate.",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"timezone": map[string]any{
								"type":        "string",
								"description": "IANA timezone name.",
							},
						},
						"required": []any{"timezone"},
					},
				},
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"timezone": map[string]any{
							"type":        "string",
							"description": "IANA timezone name.",
						},
					},
					"required": []any{"timezone"},
				},
				Executor: navitool.NewSetTimezoneExecutor(cfg.OnSetTimezone),
				Governance: navitool.ToolGovernance{
					CommandType:     schema.CommandTypeUpdate,
					WorkspaceAction: schema.WorkspaceActionModify,
					Domain:          "onboarding",
					Reversibility:   "reversible",
				},
				Metadata: navitool.ToolMetadata{
					Component: "onboarding",
					ErrorType: "tool_execution_failed",
					Tags:      []string{"builtin", "onboarding", "timezone"},
				},
			})

			return tools, nil
		},
	}
}
