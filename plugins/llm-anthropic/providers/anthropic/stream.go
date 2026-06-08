package anthropic

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/open-navi/navi/internal/llm"
)

// parseSSEStream reads an Anthropic SSE event stream from body, forwarding
// text deltas to onChunk. Tool call JSON and thinking content are accumulated
// silently (not forwarded via onChunk). Returns the assembled full response.
//
// Anthropic SSE format:
//
//	event: <event_type>
//	data: <json>
//
// Event types handled: message_start, content_block_start, content_block_delta,
// content_block_stop, message_delta, message_stop, error, ping.
func parseSSEStream(body io.Reader, onChunk func(string)) (*llm.Response, error) {
	var (
		fullContent  strings.Builder
		thinking     strings.Builder
		inputTokens  int
		outputTokens int
		finishReason string
		toolCalls    []llm.ToolCall

		// Track active content blocks by index for tool_use assembly.
		blockTypes    = map[int]string{} // index → "text" | "tool_use" | "thinking"
		toolCallIDs   = map[int]string{}
		toolCallNames = map[int]string{}
		toolCallJSON  = map[int]*strings.Builder{}
	)

	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()

		// SSE: "event: <type>" — we use the type field in JSON instead.
		if strings.HasPrefix(line, "event: ") {
			continue
		}

		// SSE: "data: <json>"
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		dataStr := strings.TrimPrefix(line, "data: ")

		// Quick type peek.
		var peek struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(dataStr), &peek); err != nil {
			continue
		}

		switch peek.Type {
		case "message_start":
			var ev sseMessageStart
			if err := json.Unmarshal([]byte(dataStr), &ev); err == nil {
				inputTokens = ev.Message.Usage.InputTokens
			}

		case "content_block_start":
			var ev sseContentBlockStart
			if err := json.Unmarshal([]byte(dataStr), &ev); err == nil {
				blockTypes[ev.Index] = ev.ContentBlock.Type
				if ev.ContentBlock.Type == "tool_use" {
					toolCallIDs[ev.Index] = ev.ContentBlock.ID
					toolCallNames[ev.Index] = ev.ContentBlock.Name
					toolCallJSON[ev.Index] = &strings.Builder{}
				}
			}

		case "content_block_delta":
			var ev sseContentBlockDelta
			if err := json.Unmarshal([]byte(dataStr), &ev); err != nil {
				continue
			}
			switch ev.Delta.Type {
			case "text_delta":
				fullContent.WriteString(ev.Delta.Text)
				if onChunk != nil {
					onChunk(ev.Delta.Text)
				}
			case "input_json_delta":
				// Accumulate tool JSON silently — no onChunk.
				if b, ok := toolCallJSON[ev.Index]; ok {
					b.WriteString(ev.Delta.PartialJSON)
				}
			case "thinking_delta":
				// Accumulate thinking silently — not forwarded via onChunk.
				thinking.WriteString(ev.Delta.Thinking)
			}

		case "content_block_stop":
			// Nothing to do — blocks finalized in message_stop.

		case "message_delta":
			var ev sseMessageDelta
			if err := json.Unmarshal([]byte(dataStr), &ev); err == nil {
				finishReason = ev.Delta.StopReason
				if ev.Usage.OutputTokens > 0 {
					outputTokens = ev.Usage.OutputTokens
				}
			}

		case "message_stop":
			// Stream complete.

		case "error":
			var ev sseError
			if err := json.Unmarshal([]byte(dataStr), &ev); err == nil {
				return nil, fmt.Errorf("anthropic: stream error: %s: %s", ev.Error.Type, ev.Error.Message)
			}

		case "ping":
			// Keep-alive; ignore.
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("anthropic: stream read: %w", err)
	}

	// Assemble accumulated tool calls.
	for idx, bt := range blockTypes {
		if bt != "tool_use" {
			continue
		}
		var args map[string]any
		if b, ok := toolCallJSON[idx]; ok && b.Len() > 0 {
			if err := json.Unmarshal([]byte(b.String()), &args); err != nil {
				args = map[string]any{"_raw": b.String()}
			}
		}
		toolCalls = append(toolCalls, llm.ToolCall{
			ID:        toolCallIDs[idx],
			Name:      toolCallNames[idx],
			Arguments: args,
		})
	}

	_ = thinking.String() // future: populate llm.Response.Thinking when field is added

	return &llm.Response{
		Content:      fullContent.String(),
		ToolCalls:    toolCalls,
		FinishReason: finishReason,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}, nil
}
