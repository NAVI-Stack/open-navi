package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/open-navi/navi/internal/llm"
)

// streamState accumulates partial results across NDJSON lines.
type streamState struct {
	content      strings.Builder
	thinking     strings.Builder // accumulated but NOT forwarded via onChunk
	toolCalls    []ollamaToolCall
	inputTokens  int
	outputTokens int
	finishReason string
}

// parseNDJSONStream reads the Ollama streaming response body line-by-line.
// Each line is a complete JSON object (ollamaStreamChunk).
//
// Protocol differences from OpenAI SSE:
//   - No "data: " prefix
//   - No "[DONE]" sentinel
//   - Stream ends when a chunk with "done": true is received
//
// Text deltas are forwarded to onChunk. Thinking deltas are accumulated
// internally (not forwarded — future: populate llm.Response.Thinking).
// Tool calls arrive as complete objects in the final chunk.
func parseNDJSONStream(
	ctx context.Context,
	body io.Reader,
	onChunk func(delta string),
) (*llm.Response, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var state streamState

	for scanner.Scan() {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var chunk ollamaStreamChunk
		if err := json.Unmarshal(line, &chunk); err != nil {
			// Skip malformed lines rather than aborting the stream.
			continue
		}

		// Text delta.
		if chunk.Message.Content != "" {
			state.content.WriteString(chunk.Message.Content)
			if onChunk != nil {
				onChunk(chunk.Message.Content)
			}
		}

		// Thinking delta — accumulated only; not emitted via onChunk.
		if chunk.Message.Thinking != "" {
			state.thinking.WriteString(chunk.Message.Thinking)
			// Emit an empty ping so upstream watchdogs don't time out.
			if onChunk != nil {
				onChunk("")
			}
		}

		// Tool calls are delivered as complete objects (not incrementally).
		if len(chunk.Message.ToolCalls) > 0 {
			state.toolCalls = append(state.toolCalls, chunk.Message.ToolCalls...)
		}

		if chunk.Done {
			state.finishReason = chunk.DoneReason
			state.inputTokens = chunk.PromptEvalCount
			state.outputTokens = chunk.EvalCount
			break
		}
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("ollama: stream read: %w", err)
	}

	content := state.content.String()

	// Some models embed thinking inline as <think>...</think> in the text.
	// If we received a dedicated thinking field, the text is already clean.
	var thinkingStr string
	if state.thinking.Len() > 0 {
		thinkingStr = state.thinking.String()
	} else if strings.Contains(content, "<think>") {
		content, thinkingStr = extractThinking(content)
	}
	_ = thinkingStr // future: populate llm.Response.Thinking when field is added

	return &llm.Response{
		Content:      content,
		ToolCalls:    convertToolCalls(state.toolCalls),
		FinishReason: state.finishReason,
		InputTokens:  state.inputTokens,
		OutputTokens: state.outputTokens,
	}, nil
}
