package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/open-navi/navi/internal/llm"
)

// Chat sends a non-streaming request to the OpenAI Chat Completions API.
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
		return nil, fmt.Errorf("openai: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("openai: create request: %w", err)
	}
	p.setHeaders(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, llm.ClassifyError(fmt.Errorf("openai: request: %w", err), p.name, model, 0)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, llm.ClassifyError(
			fmt.Errorf("status %d: %s", resp.StatusCode, string(body)),
			p.name, model, resp.StatusCode,
		)
	}

	return parseResponse(body)
}

// ChatStream streams text deltas via onChunk and returns the full response.
// Tool calls are accumulated silently (consistent with StreamingProvider contract).
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
		return nil, fmt.Errorf("openai: marshal stream: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("openai: create stream request: %w", err)
	}
	p.setHeaders(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, llm.ClassifyError(fmt.Errorf("openai: stream request: %w", err), p.name, model, 0)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, llm.ClassifyError(
			fmt.Errorf("status %d: %s", resp.StatusCode, string(body)),
			p.name, model, resp.StatusCode,
		)
	}

	return parseSSEStream(ctx, resp.Body, onChunk)
}

// ---------------------------------------------------------------------------
// internal helpers
// ---------------------------------------------------------------------------

// buildRequest constructs a chatRequest from the given parameters.
func (p *Provider) buildRequest(
	model string,
	messages []llm.Message,
	tools []llm.ToolDefinition,
	opts llm.Options,
	stream bool,
) *chatRequest {
	model = normalizeModel(model, p.baseURL)
	reasoning := isReasoningModel(model)

	r := &chatRequest{
		Model:    model,
		Messages: convertMessages(messages, reasoning),
		Stream:   stream,
	}

	// Token limit: reasoning models use max_completion_tokens.
	if opts.MaxTokens > 0 {
		if reasoning {
			r.MaxCompletionTokens = opts.MaxTokens
		} else {
			r.MaxTokens = opts.MaxTokens
		}
	}

	// Temperature: reasoning models don't accept it.
	if opts.Temperature > 0 && !reasoning {
		temp := opts.Temperature
		r.Temperature = &temp
	}

	if len(tools) > 0 {
		r.Tools = convertTools(tools)
		if tc := convertToolChoice(opts.ToolChoice); tc != nil {
			r.ToolChoice = tc
		}
	}

	// Request usage data in the final streaming chunk.
	if stream {
		r.StreamOptions = &streamOptions{IncludeUsage: true}
	}

	return r
}

// setHeaders applies required HTTP headers.
func (p *Provider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
}

// parseResponse decodes a non-streaming Chat Completions response body.
func parseResponse(body []byte) (*llm.Response, error) {
	var oaiResp chatResponse
	if err := json.Unmarshal(body, &oaiResp); err != nil {
		return nil, fmt.Errorf("openai: parse response: %w", err)
	}

	if len(oaiResp.Choices) == 0 {
		return nil, fmt.Errorf("openai: no choices in response")
	}

	choice := oaiResp.Choices[0]
	resp := &llm.Response{
		Content:      choice.Message.Content,
		FinishReason: choice.FinishReason,
		InputTokens:  oaiResp.Usage.PromptTokens,
		OutputTokens: oaiResp.Usage.CompletionTokens,
	}

	for _, tc := range choice.Message.ToolCalls {
		var args map[string]any
		if tc.Function.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				args = map[string]any{"_raw": tc.Function.Arguments}
			}
		}
		resp.ToolCalls = append(resp.ToolCalls, llm.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}

	return resp, nil
}
