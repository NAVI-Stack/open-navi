package ollama

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ceoai/navi/internal/llm"
)

// ---------------------------------------------------------------------------
// Message conversion
// ---------------------------------------------------------------------------

// convertMessages converts llm.Message slice to Ollama's native format.
// When supportsVision is true, image content is extracted from user messages
// and passed as the images[] array.
func convertMessages(messages []llm.Message, supportsVision bool) []ollamaMessage {
	out := make([]ollamaMessage, 0, len(messages))
	for _, m := range messages {
		om := ollamaMessage{Role: m.Role}

		if m.Role == "user" && supportsVision {
			text, images := extractImages(m.Content)
			om.Content = text
			om.Images = images
		} else {
			om.Content = m.Content
		}

		// Outbound tool calls from assistant messages.
		if len(m.ToolCalls) > 0 {
			om.ToolCalls = make([]ollamaToolCall, len(m.ToolCalls))
			for i, tc := range m.ToolCalls {
				om.ToolCalls[i] = ollamaToolCall{
					Function: ollamaToolCallFunc{
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
				}
			}
		}

		out = append(out, om)
	}
	return out
}

// ---------------------------------------------------------------------------
// Tool conversion
// ---------------------------------------------------------------------------

// convertTools converts llm.ToolDefinition slice to Ollama's native format.
// Ollama's tool schema mirrors OpenAI exactly, so conversion is 1:1.
func convertTools(tools []llm.ToolDefinition) []ollamaTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]ollamaTool, len(tools))
	for i, t := range tools {
		out[i] = ollamaTool{
			Type: "function",
			Function: ollamaToolFunc{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		}
	}
	return out
}

// convertToolCalls converts Ollama tool call objects back to llm.ToolCall.
// Ollama doesn't assign IDs so we generate deterministic positional ones.
func convertToolCalls(calls []ollamaToolCall) []llm.ToolCall {
	out := make([]llm.ToolCall, len(calls))
	for i, c := range calls {
		out[i] = llm.ToolCall{
			ID:        fmt.Sprintf("call_%d", i),
			Name:      c.Function.Name,
			Arguments: c.Function.Arguments,
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Image extraction
// ---------------------------------------------------------------------------

// extractImages parses image data out of message content.
// It handles three cases:
//  1. JSON array of content blocks (OpenAI-style structured content):
//     [{"type":"text","text":"..."}, {"type":"image_url","image_url":{"url":"data:image/png;base64,..."}}]
//  2. A single data URI: "data:image/png;base64,<b64data>"
//  3. Plain text — returned as-is with no images.
//
// Returns (textContent, base64Images). Images are raw base64 without the
// "data:...;base64," prefix, as required by Ollama's /api/chat format.
func extractImages(content string) (string, []string) {
	content = strings.TrimSpace(content)

	// Fast path: no image data present.
	if !strings.Contains(content, "base64,") && !strings.HasPrefix(content, "[") {
		return content, nil
	}

	// Try JSON content-block array (OpenAI-compatible structured content).
	if strings.HasPrefix(content, "[") {
		var blocks []struct {
			Type     string `json:"type"`
			Text     string `json:"text,omitempty"`
			ImageURL *struct {
				URL string `json:"url"`
			} `json:"image_url,omitempty"`
		}
		if err := json.Unmarshal([]byte(content), &blocks); err == nil {
			var sb strings.Builder
			var images []string
			for _, b := range blocks {
				switch b.Type {
				case "text":
					sb.WriteString(b.Text)
				case "image_url":
					if b.ImageURL != nil {
						if img := stripDataURIPrefix(b.ImageURL.URL); img != "" {
							images = append(images, img)
						}
					}
				}
			}
			return sb.String(), images
		}
	}

	// Entire content is a single data URI.
	if img := stripDataURIPrefix(content); img != "" {
		return "", []string{img}
	}

	return content, nil
}

// stripDataURIPrefix strips "data:<mime>;base64," from a data URI, returning
// only the raw base64 payload. Returns "" if the string isn't a data URI.
func stripDataURIPrefix(s string) string {
	idx := strings.Index(s, "base64,")
	if idx == -1 {
		return ""
	}
	return s[idx+7:]
}

// ---------------------------------------------------------------------------
// Thinking extraction
// ---------------------------------------------------------------------------

// extractThinking strips the outermost <think>...</think> block from content,
// returning (textWithoutThinking, thinkingContent).
// Used for models that embed reasoning inline rather than in a dedicated field.
func extractThinking(content string) (text, thinking string) {
	const openTag, closeTag = "<think>", "</think>"
	start := strings.Index(content, openTag)
	if start == -1 {
		return content, ""
	}
	end := strings.Index(content, closeTag)
	if end == -1 || end < start {
		return content, ""
	}
	thinking = content[start+len(openTag) : end]
	text = strings.TrimSpace(content[:start] + content[end+len(closeTag):])
	return text, thinking
}
