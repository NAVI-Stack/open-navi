package anthropic

import (
	"encoding/json"
	"strings"

	"github.com/open-navi/navi/internal/llm"
)

// ---------------------------------------------------------------------------
// Message conversion
// ---------------------------------------------------------------------------

// convertMessages converts internal llm.Messages to Anthropic wire format.
// System messages are extracted and returned separately (Anthropic uses a
// top-level "system" field, not a system role in the messages array).
//
// Special handling:
//   - "system" → extracted to returned system string
//   - "tool" → converted to "user" message with tool_result content block
//   - "assistant" with ToolCalls → content array of text + tool_use blocks
//   - "user" with image data → content array of text + image blocks
//   - everything else → plain string content
func convertMessages(messages []llm.Message) (system string, out []antMessage) {
	for _, m := range messages {
		switch m.Role {
		case "system":
			if system != "" {
				system += "\n"
			}
			system += m.Content

		case "tool":
			// Anthropic requires tool results as "user" messages with
			// tool_result content blocks.
			block := antContentBlock{
				Type:      "tool_result",
				ToolUseID: m.ToolCallID,
				Content:   m.Content,
			}
			out = append(out, antMessage{
				Role:    "user",
				Content: []antContentBlock{block},
			})

		case "assistant":
			if len(m.ToolCalls) == 0 {
				out = append(out, antMessage{
					Role:    "assistant",
					Content: m.Content,
				})
				continue
			}
			// Build content array: optional text block + tool_use blocks.
			var blocks []antContentBlock
			if m.Content != "" {
				blocks = append(blocks, antContentBlock{
					Type: "text",
					Text: m.Content,
				})
			}
			for _, tc := range m.ToolCalls {
				input := any(tc.Arguments)
				if tc.Arguments == nil {
					input = map[string]any{}
				}
				blocks = append(blocks, antContentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: input,
				})
			}
			out = append(out, antMessage{
				Role:    "assistant",
				Content: blocks,
			})

		default:
			// "user" and any other roles.
			// Check for embedded images.
			if m.Role == "user" {
				if blocks := buildUserContentBlocks(m.Content); blocks != nil {
					out = append(out, antMessage{
						Role:    "user",
						Content: blocks,
					})
					continue
				}
			}
			out = append(out, antMessage{
				Role:    m.Role,
				Content: m.Content,
			})
		}
	}
	return system, out
}

// ---------------------------------------------------------------------------
// Tool conversion
// ---------------------------------------------------------------------------

// convertTools converts llm.ToolDefinition slice to Anthropic's tool format.
func convertTools(tools []llm.ToolDefinition) []antTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]antTool, len(tools))
	for i, t := range tools {
		out[i] = antTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		}
	}
	return out
}

// convertToolChoice maps the string-based llm.Options.ToolChoice to
// Anthropic's typed tool_choice object.
func convertToolChoice(tc string) any {
	switch tc {
	case "auto":
		return map[string]any{"type": "auto"}
	case "required", "any":
		return map[string]any{"type": "any"}
	case "none":
		return nil // omit tool_choice
	case "":
		return nil
	default:
		// Specific tool name.
		return map[string]any{
			"type": "tool",
			"name": tc,
		}
	}
}

// ---------------------------------------------------------------------------
// Image extraction
// ---------------------------------------------------------------------------

// buildUserContentBlocks checks if user message content contains images
// (data URIs or OpenAI-style JSON content blocks) and returns Anthropic
// content blocks. Returns nil if the content is plain text.
func buildUserContentBlocks(content string) []antContentBlock {
	content = strings.TrimSpace(content)

	// Fast path: no image data.
	if !strings.Contains(content, "base64,") && !strings.HasPrefix(content, "[") {
		return nil
	}

	// Try OpenAI-style JSON content block array.
	if strings.HasPrefix(content, "[") {
		var blocks []struct {
			Type     string `json:"type"`
			Text     string `json:"text,omitempty"`
			ImageURL *struct {
				URL string `json:"url"`
			} `json:"image_url,omitempty"`
		}
		if err := json.Unmarshal([]byte(content), &blocks); err == nil {
			var out []antContentBlock
			for _, b := range blocks {
				switch b.Type {
				case "text":
					out = append(out, antContentBlock{Type: "text", Text: b.Text})
				case "image_url":
					if b.ImageURL != nil {
						if src := parseDataURI(b.ImageURL.URL); src != nil {
							out = append(out, antContentBlock{Type: "image", Source: src})
						}
					}
				}
			}
			if len(out) > 0 {
				return out
			}
		}
	}

	// Entire content is a single data URI.
	if src := parseDataURI(content); src != nil {
		return []antContentBlock{
			{Type: "image", Source: src},
		}
	}

	return nil
}

// parseDataURI parses a "data:<media>;base64,<data>" URI into an
// antImageSource. Returns nil if the string isn't a data URI.
func parseDataURI(s string) *antImageSource {
	// Must start with "data:" and contain "base64,"
	if !strings.HasPrefix(s, "data:") {
		return nil
	}
	idx := strings.Index(s, "base64,")
	if idx == -1 {
		return nil
	}

	// Extract media type between "data:" and ";base64,"
	header := s[5:idx] // after "data:", before "base64,"
	header = strings.TrimSuffix(header, ";")
	if header == "" {
		header = "image/png" // default
	}

	return &antImageSource{
		Type:      "base64",
		MediaType: header,
		Data:      s[idx+7:], // after "base64,"
	}
}
