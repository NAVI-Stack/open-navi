package plugin

import "github.com/ceoai/navi/internal/llm"

// Builtin plugin tool names (executors are in internal/navi/loop.go).
const (
	CalendarTimeWindowToolName    = "navi.domain.calendar.current_time_window"
	WorkflowTaskSummaryToolName   = "navi.workflow.tasks.summary"
	IntegrationGitHubRepoToolName = "navi.integration.github.repo_info"
	AgenticCriticRequestToolName  = "navi.agentic.critic.request"
)

// RegisterBuiltin registers first-party plugins with concrete tools per category
// (Domain, Workflow, Integration, Agentic). Each tool has an executor in the agent loop.
// Plugins are registered but disabled by default so their tools are not exposed to the
// LLM during casual chat. Enable a plugin explicitly via Registry.SetPluginEnabled(id, true)
// when the context requires it (e.g. ACT mode, directive execution).
func RegisterBuiltin(reg *Registry) {
	if reg == nil {
		return
	}

	calendarM := Manifest{
		ID:           "navi.domain.calendar",
		Name:         "Calendar Domain Plugin",
		Description:  "Provides calendar domain awareness (events, availability, time windows).",
		Version:      "0.1.0",
		Categories:   []string{"domain"},
		Capabilities: []string{"calendar_model", "availability_windows"},
		Triggers:     []string{"message_received"},
		TrustTier:    "builtin",
	}
	calendarTools := []llm.ToolDefinition{{
		Name:        CalendarTimeWindowToolName,
		Description: "Returns the current time window (start and end UTC timestamps) for calendar/availability context.",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
	}}
	reg.RegisterPlugin(calendarM, calendarTools)
	reg.SetPluginEnabled(calendarM.ID, false)
	reg.SetCatalogEntry(calendarM.ID, calendarM, calendarTools)

	workflowM := Manifest{
		ID:           "navi.workflow.tasks",
		Name:         "Task Workflow Plugin",
		Description:  "Adds opinionated task/workflow helpers on top of directives and tasks.",
		Version:      "0.1.0",
		Categories:   []string{"workflow"},
		Capabilities: []string{"task_summarization", "checklist_generation"},
		Triggers:     []string{"after_tool_call", "directive_status_changed"},
		TrustTier:    "builtin",
	}
	workflowTools := []llm.ToolDefinition{{
		Name:        WorkflowTaskSummaryToolName,
		Description: "Returns a short summary of the current task/directive context for workflow awareness.",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
	}}
	reg.RegisterPlugin(workflowM, workflowTools)
	reg.SetPluginEnabled(workflowM.ID, false)
	reg.SetCatalogEntry(workflowM.ID, workflowM, workflowTools)

	// Attach the GitHub repo_info tool to the repo-owned plugin ID without registering
	// a duplicate manifest. plugins/github/plugin.yaml is the canonical manifest source.
	// Registering a second manifest here would disable the repo-owned plugin via the
	// shared disabled-state key, suppressing ActivePluginSkillPaths() for GitHub.
	reg.AddToolForPlugin("navi.integration.github", llm.ToolDefinition{
		Name:        IntegrationGitHubRepoToolName,
		Description: "Returns GitHub repo info when the GitHub skill is configured; otherwise reports integration status.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"repo": map[string]any{"type": "string", "description": "Repository in owner/repo form"},
			},
		},
	})

	criticM := Manifest{
		ID:          "navi.agentic.critic",
		Name:        "Agentic Critic Plugin",
		Description: "Surfaces CriticAgent insights as a reusable agentic plugin.",
		Version:     "0.1.0",
		Categories:  []string{"agentic"},
		Capabilities: []string{
			"code_review",
			"plan_critique",
		},
		Triggers: []string{
			"after_tool_call",
			"directive_completed",
		},
		TrustTier: "builtin",
	}
	criticTools := []llm.ToolDefinition{{
		Name:        AgenticCriticRequestToolName,
		Description: "Request CriticAgent-style review; in ACT mode full critique runs via the task pipeline.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"context": map[string]any{"type": "string", "description": "Optional context or artifact reference for the review"},
			},
		},
	}}
	reg.RegisterPlugin(criticM, criticTools)
	reg.SetPluginEnabled(criticM.ID, false)
	reg.SetCatalogEntry(criticM.ID, criticM, criticTools)
}

// IsBuiltinExecutable reports whether the named tool is a builtin plugin tool with an executor in the agent loop.
func IsBuiltinExecutable(name string) bool {
	switch name {
	case CalendarTimeWindowToolName, WorkflowTaskSummaryToolName, IntegrationGitHubRepoToolName, AgenticCriticRequestToolName:
		return true
	default:
		return false
	}
}
