package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ceoai/navi/internal/llm"
)

// Chat sends a non-streaming request to the Anthropic Messages API.
func (p *Provider) Chat(
	ctx context.Context,
	model string,
	messages []llm.Message,
	tools []llm.ToolDefinition,
	opts llm.Options,
) (*llm.Response, error) {
	reqBody := p.buildRequest(model, messages, tools, opts, false)

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/messages", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("anthropic: create request: %w", err)
	}
	p.setHeaders(req, model)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, llm.ClassifyError(fmt.Errorf("anthropic: request: %w", err), "anthropic", model, 0)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, llm.ClassifyError(
			fmt.Errorf("status %d: %s", resp.StatusCode, string(body)),
			"anthropic", model, resp.StatusCode,
		)
	}

	return parseResponse(body)
}

// ChatStream streams text deltas via onChunk and returns the full response.
// Tool calls and thinking blocks are accumulated silently and returned in the
// response (consistent with StreamingProvider contract).
func (p *Provider) ChatStream(
	ctx context.Context,
	model string,
	messages []llm.Message,
	tools []llm.ToolDefinition,
	opts llm.Options,
	onChunk func(delta string),
) (*llm.Response, error) {
	reqBody := p.buildRequest(model, messages, tools, opts, true)

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal stream: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/messages", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("anthropic: create stream request: %w", err)
	}
	p.setHeaders(req, model)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, llm.ClassifyError(fmt.Errorf("anthropic: stream request: %w", err), "anthropic", model, 0)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, llm.ClassifyError(
			fmt.Errorf("status %d: %s", resp.StatusCode, string(body)),
			"anthropic", model, resp.StatusCode,
		)
	}

	return parseSSEStream(resp.Body, onChunk)
}

// ---------------------------------------------------------------------------
// internal helpers
// ---------------------------------------------------------------------------

// buildRequest constructs an anthropicRequest from the given parameters.
func (p *Provider) buildRequest(
	model string,
	messages []llm.Message,
	tools []llm.ToolDefinition,
	opts llm.Options,
	stream bool,
) *anthropicRequest {
	system, nonSystemMsgs := convertMessages(messages)

	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	r := &anthropicRequest{
		Model:     model,
		MaxTokens: maxTokens,
		Messages:  nonSystemMsgs,
		Stream:    stream,
	}

	if system != "" {
		r.System = system
	}

	if len(tools) > 0 {
		r.Tools = convertTools(tools)
		if tc := convertToolChoice(opts.ToolChoice); tc != nil {
			r.ToolChoice = tc
		}
	}

	// Enable extended thinking for models that support it.
	if isThinkingModel(model) {
		r.Thinking = &antThinking{
			Type:         "enabled",
			BudgetTokens: thinkingBudget(maxTokens),
		}
	}

	return r
}

// setHeaders applies Anthropic-required headers.
func (p *Provider) setHeaders(req *http.Request, model string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	if isThinkingModel(model) {
		req.Header.Set("anthropic-beta", "interleaved-thinking-2025-05-14")
	}
}

// parseResponse decodes a non-streaming Anthropic response body.
func parseResponse(body []byte) (*llm.Response, error) {
	var antResp anthropicResponse
	if err := json.Unmarshal(body, &antResp); err != nil {
		return nil, fmt.Errorf("anthropic: parse response: %w", err)
	}

	resp := &llm.Response{
		FinishReason: antResp.StopReason,
		InputTokens:  antResp.Usage.InputTokens,
		OutputTokens: antResp.Usage.OutputTokens,
	}

	var thinking strings.Builder

	for _, block := range antResp.Content {
		switch block.Type {
		case "text":
			if resp.Content != "" {
				resp.Content += "\n"
			}
			resp.Content += block.Text
		case "tool_use":
			args, _ := normalizeInput(block.Input)
			resp.ToolCalls = append(resp.ToolCalls, llm.ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: args,
			})
		case "thinking":
			thinking.WriteString(block.Thinking)
		}
	}

	_ = thinking.String() // future: populate llm.Response.Thinking

	return resp, nil
}

// normalizeInput converts the polymorphic "input" field of a tool_use block
// into map[string]any. It handles both pre-decoded maps and raw JSON.
func normalizeInput(v any) (map[string]any, error) {
	if v == nil {
		return map[string]any{}, nil
	}
	if m, ok := v.(map[string]any); ok {
		return m, nil
	}
	// Fallback: re-marshal and unmarshal.
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	err = json.Unmarshal(data, &m)
	return m, err
}

// isThinkingModel returns true for models that support extended thinking.
func isThinkingModel(model string) bool {
	m := strings.ToLower(model)
	return strings.Contains(m, "opus")
}

// thinkingBudget returns a reasonable thinking budget based on max_tokens.
// Anthropic requires budget_tokens < max_tokens.
func thinkingBudget(maxTokens int) int {
	budget := maxTokens / 2
	if budget < 1024 {
		budget = 1024
	}
	if budget >= maxTokens {
		budget = maxTokens - 1
	}
	return budget
}
