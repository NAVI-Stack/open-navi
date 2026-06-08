// Legacy task decomposition tool definitions.
// NAVI's skill system (internal/navi/skill/) is the current capability surface.
package orchestrator

import "github.com/ceoai/navi/internal/llm"

// DecomposeTasksTool is the structured output mechanism for generating tasks.
var DecomposeTasksTool = llm.ToolDefinition{
	Name:        "decompose_tasks",
	Description: "Decompose a directive into a concrete list of implementation tasks. Each task targets a specific file or directory and has a clear, atomic objective.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tasks": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"title":        map[string]any{"type": "string", "description": "Short imperative title, e.g. 'Add error handling to store.GetTask'"},
						"description":  map[string]any{"type": "string", "description": "What the coder should do and why"},
						"surface_path": map[string]any{"type": "string", "description": "Relative file or directory path this task targets, e.g. 'internal/store/task.go'"},
						"agent_type":   map[string]any{"type": "string", "enum": []string{"coder", "critic", "strategist", "scout"}, "description": "Agent to assign the task to"},
						"retryable":    map[string]any{"type": "boolean"},
					},
					"required": []string{"title", "description", "surface_path", "agent_type"},
				},
			},
			"reasoning": map[string]any{"type": "string", "description": "Brief explanation of decomposition decisions"},
		},
		"required": []string{"tasks"},
	},
}
