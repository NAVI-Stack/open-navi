package openai

import (
	"encoding/json"
	"strings"

	"github.com/ceoai/navi/internal/llm"
)

// ---------------------------------------------------------------------------
// Message conversion
// ---------------------------------------------------------------------------

// convertMessages converts internal llm.Messages to OpenAI wire format.
// For reasoning models, "system" role is converted to "developer" role.
func convertMessages(messages []llm.Message, reasoning bool) []oaiMessage {
	out := make([]oaiMessage, 0, len(messages))
	for _, m := range messages {
		msg := oaiMessage{
			Role:       m.Role,
			ToolCallID: m.ToolCallID,
		}

		// Reasoning models don't accept "system" role — use "developer" instead.
		if reasoning && m.Role == "system" {
			msg.Role = "developer"
		}

		// Assistant messages with tool calls need structured tool_calls field.
		if len(m.ToolCalls) > 0 {
			msg.Content = m.Content
			msg.ToolCalls = make([]oaiToolCall, len(m.ToolCalls))
			for i, tc := range m.ToolCalls {
				argData, _ := json.Marshal(tc.Arguments)
				msg.ToolCalls[i] = oaiToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: oaiToolCallFunc{
						Name:      tc.Name,
						Arguments: string(argData),
					},
				}
			}
			out = append(out, msg)
			continue
		}

		// User messages: check for embedded images.
		if m.Role == "user" {
			if blocks := buildImageContent(m.Content); blocks != nil {
				msg.Content = blocks
				out = append(out, msg)
				continue
			}
		}

		msg.Content = m.Content
		out = append(out, msg)
	}
	return out
}

// ---------------------------------------------------------------------------
// Tool conversion
// ---------------------------------------------------------------------------

// convertTools converts llm.ToolDefinition slice to OpenAI's tool format.
func convertTools(tools []llm.ToolDefinition) []oaiTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]oaiTool, len(tools))
	for i, t := range tools {
		out[i] = oaiTool{
			Type: "function",
			Function: oaiToolFunc{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		}
	}
	return out
}

// convertToolChoice maps the string-based llm.Options.ToolChoice to
// the OpenAI wire format.
func convertToolChoice(tc string) any {
	switch tc {
	case "auto", "none", "required":
		return tc // OpenAI accepts these as plain strings
	case "":
		return nil
	default:
		// Specific tool name.
		return map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": tc,
			},
		}
	}
}

// ---------------------------------------------------------------------------
// Image content
// ---------------------------------------------------------------------------

// buildImageContent checks if user message content contains images (data URIs
// or OpenAI-style JSON content blocks) and returns a []oaiContentBlock.
// Returns nil if the content is plain text.
func buildImageContent(content string) []oaiContentBlock {
	content = strings.TrimSpace(content)

	// Fast path: no image data.
	if !strings.Contains(content, "base64,") && !strings.HasPrefix(content, "[") {
		return nil
	}

	// Try OpenAI-style JSON content block array (passthrough).
	if strings.HasPrefix(content, "[") {
		var blocks []struct {
			Type     string `json:"type"`
			Text     string `json:"text,omitempty"`
			ImageURL *struct {
				URL string `json:"url"`
			} `json:"image_url,omitempty"`
		}
		if err := json.Unmarshal([]byte(content), &blocks); err == nil {
			var out []oaiContentBlock
			for _, b := range blocks {
				switch b.Type {
				case "text":
					out = append(out, oaiContentBlock{Type: "text", Text: b.Text})
				case "image_url":
					if b.ImageURL != nil {
						out = append(out, oaiContentBlock{
							Type:     "image_url",
							ImageURL: &oaiImageURL{URL: b.ImageURL.URL},
						})
					}
				}
			}
			if len(out) > 0 {
				return out
			}
		}
	}

	// Entire content is a single data URI.
	if strings.HasPrefix(content, "data:") && strings.Contains(content, "base64,") {
		return []oaiContentBlock{
			{Type: "image_url", ImageURL: &oaiImageURL{URL: content}},
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// Reasoning model detection
// ---------------------------------------------------------------------------

// isReasoningModel returns true for OpenAI reasoning models that require
// different API parameters (max_completion_tokens, no temperature, developer role).
func isReasoningModel(model string) bool {
	m := strings.ToLower(model)
	// o1, o1-mini, o1-preview, o3, o3-mini, o4-mini
	return strings.HasPrefix(m, "o1") ||
		strings.HasPrefix(m, "o3") ||
		strings.HasPrefix(m, "o4-mini")
}

// ---------------------------------------------------------------------------
// Model normalization
// ---------------------------------------------------------------------------

// normalizeModel strips provider prefixes before sending to the endpoint.
// OpenRouter is the exception — it keeps the prefix.
func normalizeModel(model, baseURL string) string {
	if strings.Contains(baseURL, "openrouter") {
		return model
	}
	prefixes := []string{"openai/", "ollama/", "openrouter/"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(model, prefix) {
			return model[len(prefix):]
		}
	}
	return model
}
