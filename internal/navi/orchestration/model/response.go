package model

import "github.com/ceoai/navi/internal/llm"

func normalizeProviderResponse(profile llmProviderProfile, raw *llm.Response) map[string]any {
	if raw == nil {
		return map[string]any{
			"provider": profile.Provider,
			"model":    profile.Model,
		}
	}
	return map[string]any{
		"provider":      profile.Provider,
		"model":         profile.Model,
		"content":       raw.Content,
		"finish_reason": raw.FinishReason,
		"input_tokens":  raw.InputTokens,
		"output_tokens": raw.OutputTokens,
		"tool_calls":    raw.ToolCalls,
	}
}

type llmProviderProfile struct {
	Provider string
	Model    string
}
