package openai

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/open-navi/navi/internal/llm"
)

// toolCallAccumulator assembles incremental tool call deltas into complete
// tool calls across multiple SSE chunks.
type toolCallAccumulator struct {
	calls []struct {
		id   string
		name string
		args strings.Builder
	}
}

func (a *toolCallAccumulator) addDelta(delta streamDeltaTC) {
	for delta.Index >= len(a.calls) {
		a.calls = append(a.calls, struct {
			id   string
			name string
			args strings.Builder
		}{})
	}
	tc := &a.calls[delta.Index]
	if delta.ID != "" {
		tc.id = delta.ID
	}
	if delta.Function.Name != "" {
		tc.name = delta.Function.Name
	}
	if delta.Function.Arguments != "" {
		tc.args.WriteString(delta.Function.Arguments)
	}
}

func (a *toolCallAccumulator) toolCalls() []llm.ToolCall {
	if len(a.calls) == 0 {
		return nil
	}
	out := make([]llm.ToolCall, 0, len(a.calls))
	for _, tc := range a.calls {
		if tc.name == "" {
			continue
		}
		var args map[string]any
		raw := tc.args.String()
		if raw != "" {
			if err := json.Unmarshal([]byte(raw), &args); err != nil {
				args = map[string]any{"_raw": raw}
			}
		}
		out = append(out, llm.ToolCall{
			ID:        tc.id,
			Name:      tc.name,
			Arguments: args,
		})
	}
	return out
}

// parseSSEStream reads an OpenAI SSE event stream from body, forwarding
// text deltas to onChunk. Tool call arguments are accumulated silently.
//
// OpenAI SSE format:
//
//	data: <json>
//	data: [DONE]
func parseSSEStream(
	ctx context.Context,
	body io.Reader,
	onChunk func(string),
) (*llm.Response, error) {
	var (
		fullContent  strings.Builder
		tcAccum      toolCallAccumulator
		inputTokens  int
		outputTokens int
		finishReason = "stop"
	)

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}
		dataStr := strings.TrimPrefix(line, "data: ")
		if dataStr == "[DONE]" {
			break
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
			continue
		}

		if len(chunk.Choices) > 0 {
			choice := chunk.Choices[0]
			sawActivity := false

			if choice.Delta.Content != "" {
				fullContent.WriteString(choice.Delta.Content)
				if onChunk != nil {
					onChunk(choice.Delta.Content)
				}
				sawActivity = true
			}

			for _, tcDelta := range choice.Delta.ToolCalls {
				tcAccum.addDelta(tcDelta)
				sawActivity = true
			}

			// Signal stream activity even for tool-call-only deltas so
			// upstream watchdogs don't time out.
			if sawActivity && onChunk != nil && choice.Delta.Content == "" {
				onChunk("")
			}

			if choice.FinishReason != nil {
				finishReason = *choice.FinishReason
			}
		}

		if chunk.Usage != nil {
			inputTokens = chunk.Usage.PromptTokens
			outputTokens = chunk.Usage.CompletionTokens
		}
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("openai: stream read: %w", err)
	}

	return &llm.Response{
		Content:      fullContent.String(),
		ToolCalls:    tcAccum.toolCalls(),
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		FinishReason: finishReason,
	}, nil
}
