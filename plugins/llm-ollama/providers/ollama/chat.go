package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/ceoai/navi/internal/llm"
)

// Chat sends a request to Ollama's native /api/chat endpoint and returns the
// full response. On model-not-found (404) it falls back to the first locally
// available model and retries once. On tool-unsupport errors it retries without
// tools.
func (p *Provider) Chat(
	ctx context.Context,
	model string,
	messages []llm.Message,
	tools []llm.ToolDefinition,
	opts llm.Options,
) (*llm.Response, error) {
	slog.Debug("ollama: chat", "model", model)

	caps, _ := p.getCapabilities(ctx, model) // proceed with zero caps on error

	// Fallback path when tools are required but model does not support them
	if (opts.ToolsRequired || opts.ToolCallingRequired) && !caps.SupportsTools {
		slog.Debug("ollama: model does not support tools but they are required; attempting fallback", "model", model)
		fallback, fallbackErr := p.firstAvailableModelWithTools(ctx, model)
		if fallbackErr == nil && fallback != "" && fallback != model {
			slog.Debug("ollama: falling back to model with tool support", "fallback", fallback)
			fallbackCaps, _ := p.getCapabilities(ctx, fallback)
			resp, err := p.doChat(ctx, fallback, messages, tools, opts, fallbackCaps)
			if err == nil {
				if len(resp.ToolCalls) == 0 && isStartOfToolTurn(messages) {
					if parsedCalls := tryParseToolCallFromText(resp.Content, tools); len(parsedCalls) > 0 {
						slog.Debug("ollama: successfully parsed tool calls from fallback model text response", "fallback", fallback, "tool_calls", parsedCalls)
						resp.ToolCalls = parsedCalls
						return resp, nil
					}
				}
				if (opts.ToolsRequired || opts.ToolCallingRequired) && len(resp.ToolCalls) == 0 && isStartOfToolTurn(messages) {
					pResp, pErr := p.chatPromptFallback(ctx, fallback, messages, tools, opts, fallbackCaps)
					if pErr == nil {
						return pResp, nil
					}
					return nil, fmt.Errorf("ollama: fallback model %q failed to call tools for tool-requiring intent (prompt fallback failed: %v)", fallback, pErr)
				}
				return resp, nil
			}
			return nil, err
		}
		resp, err := p.chatPromptFallback(ctx, model, messages, tools, opts, caps)
		if err == nil {
			return resp, nil
		}
		return nil, fmt.Errorf("ollama: model %q does not support tool calling but tools are required (fallback failed: %v, prompt fallback failed: %v)", model, fallbackErr, err)
	}

	resp, err := p.doChat(ctx, model, messages, tools, opts, caps)
	if err == nil {
		if len(resp.ToolCalls) == 0 && isStartOfToolTurn(messages) {
			if parsedCalls := tryParseToolCallFromText(resp.Content, tools); len(parsedCalls) > 0 {
				slog.Debug("ollama: successfully parsed tool calls from model text response", "model", model, "tool_calls", parsedCalls)
				resp.ToolCalls = parsedCalls
				return resp, nil
			}
		}
		if (opts.ToolsRequired || opts.ToolCallingRequired) && len(resp.ToolCalls) == 0 && isStartOfToolTurn(messages) {
			err = fmt.Errorf("ollama: model %q failed to call tools for tool-requiring intent", model)
		} else {
			return resp, nil
		}
	}

	// Fallback path when tools are required and model failed to call tools (or errored out due to tool support issues)
	if (opts.ToolsRequired || opts.ToolCallingRequired) && err != nil && (isToolSupportError(err) || strings.Contains(err.Error(), "failed to call tools")) {
		slog.Debug("ollama: model failed to call tools or does not support them; attempting fallback", "model", model, "error", err)
		fallback, fallbackErr := p.firstAvailableModelWithTools(ctx, model)
		if fallbackErr == nil && fallback != "" && fallback != model {
			slog.Debug("ollama: falling back to model with tool support", "fallback", fallback)
			fallbackCaps, _ := p.getCapabilities(ctx, fallback)
			resp, err = p.doChat(ctx, fallback, messages, tools, opts, fallbackCaps)
			if err == nil {
				if len(resp.ToolCalls) == 0 && isStartOfToolTurn(messages) {
					if parsedCalls := tryParseToolCallFromText(resp.Content, tools); len(parsedCalls) > 0 {
						slog.Debug("ollama: successfully parsed tool calls from fallback model text response", "fallback", fallback, "tool_calls", parsedCalls)
						resp.ToolCalls = parsedCalls
						return resp, nil
					}
				}
				if (opts.ToolsRequired || opts.ToolCallingRequired) && len(resp.ToolCalls) == 0 && isStartOfToolTurn(messages) {
					pResp, pErr := p.chatPromptFallback(ctx, fallback, messages, tools, opts, fallbackCaps)
					if pErr == nil {
						return pResp, nil
					}
					return nil, fmt.Errorf("ollama: fallback model %q failed to call tools for tool-requiring intent (prompt fallback failed: %v)", fallback, pErr)
				}
				return resp, nil
			}
			return nil, err
		}
		pResp, pErr := p.chatPromptFallback(ctx, model, messages, tools, opts, caps)
		if pErr == nil {
			return pResp, nil
		}
		return nil, fmt.Errorf("%w (prompt fallback failed: %v)", err, pErr)
	}

	// Retry without tools if the model doesn't support them.
	if len(tools) > 0 && isToolSupportError(err) {
		if opts.ToolsRequired || opts.ToolCallingRequired {
			return nil, fmt.Errorf("ollama: model %q does not support tools: %w", model, err)
		}
		slog.Debug("ollama: model does not support tools, retrying without", "model", model)
		return p.doChat(ctx, model, messages, nil, opts, caps)
	}

	// Retry with the first available local model on 404.
	if isModelNotFound(err) {
		slog.Debug("ollama: model not found, attempting fallback", "model", model)
		fallback, listErr := p.firstAvailableModel(ctx)
		if listErr != nil {
			return nil, fmt.Errorf("%w (model fallback failed: %v. Try running 'ollama pull %s')", err, listErr, model)
		}
		if fallback == "" || fallback == model {
			return nil, err
		}
		slog.Debug("ollama: falling back to model", "fallback", fallback)
		fallbackCaps, _ := p.getCapabilities(ctx, fallback)
		if (opts.ToolsRequired || opts.ToolCallingRequired) && !fallbackCaps.SupportsTools {
			fallbackWithTools, fbToolsErr := p.firstAvailableModelWithTools(ctx, model)
			if fbToolsErr == nil && fallbackWithTools != "" && fallbackWithTools != model {
				fallback = fallbackWithTools
				fallbackCaps, _ = p.getCapabilities(ctx, fallback)
			} else {
				pResp, pErr := p.chatPromptFallback(ctx, fallback, messages, tools, opts, fallbackCaps)
				if pErr == nil {
					return pResp, nil
				}
				return nil, fmt.Errorf("ollama: fallback model %q does not support tool calling but tools are required (prompt fallback failed: %v)", fallback, pErr)
			}
		}
		resp, err = p.doChat(ctx, fallback, messages, tools, opts, fallbackCaps)
		if err == nil {
			if len(resp.ToolCalls) == 0 && isStartOfToolTurn(messages) {
				if parsedCalls := tryParseToolCallFromText(resp.Content, tools); len(parsedCalls) > 0 {
					slog.Debug("ollama: successfully parsed tool calls from fallback model text response on 404", "fallback", fallback, "tool_calls", parsedCalls)
					resp.ToolCalls = parsedCalls
					return resp, nil
				}
			}
			if (opts.ToolsRequired || opts.ToolCallingRequired) && len(resp.ToolCalls) == 0 && isStartOfToolTurn(messages) {
				pResp, pErr := p.chatPromptFallback(ctx, fallback, messages, tools, opts, fallbackCaps)
				if pErr == nil {
					return pResp, nil
				}
				return nil, fmt.Errorf("ollama: fallback model %q failed to call tools for tool-requiring intent (prompt fallback failed: %v)", fallback, pErr)
			}
			return resp, nil
		}
		return nil, err
	}

	return nil, err
}

func (p *Provider) chatPromptFallback(
	ctx context.Context,
	model string,
	messages []llm.Message,
	tools []llm.ToolDefinition,
	opts llm.Options,
	caps ModelCapabilities,
) (*llm.Response, error) {
	slog.Debug("ollama: attempting prompt-based tool calling fallback", "model", model)

	injectedMessages := injectPromptToolCalling(messages, tools)
	fallbackCaps := caps
	fallbackCaps.SupportsTools = false

	resp, err := p.doChat(ctx, model, injectedMessages, nil, opts, fallbackCaps)
	if err != nil {
		return nil, err
	}

	parsedCalls := tryParseToolCallFromText(resp.Content, tools)
	if len(parsedCalls) > 0 {
		slog.Debug("ollama: successfully parsed tool calls from prompt fallback text response", "model", model, "tool_calls", parsedCalls)
		resp.ToolCalls = parsedCalls
		return resp, nil
	}

	return nil, fmt.Errorf("ollama: prompt fallback on %q failed to call tools for tool-requiring intent", model)
}

func injectPromptToolCalling(messages []llm.Message, tools []llm.ToolDefinition) []llm.Message {
	if len(tools) == 0 {
		return messages
	}

	var sb strings.Builder
	sb.WriteString("\n\n[INSTRUCTION] You must select and execute exactly one of the following tools based on the user's request. You must output your tool call as a JSON block using the format:\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"name\": \"tool_name\",\n")
	sb.WriteString("  \"arguments\": {\n")
	sb.WriteString("    \"param_name\": \"value\"\n")
	sb.WriteString("  }\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n\n")
	sb.WriteString("Available Tools:\n")
	for _, t := range tools {
		paramBytes, _ := json.Marshal(t.Parameters)
		sb.WriteString(fmt.Sprintf("- name: %q\n  description: %q\n  parameters: %s\n\n", t.Name, t.Description, string(paramBytes)))
	}
	sb.WriteString("Do not explain. Output only the JSON block.")

	newMsgs := make([]llm.Message, len(messages))
	copy(newMsgs, messages)

	if len(newMsgs) > 0 {
		lastIdx := len(newMsgs) - 1
		newMsgs[lastIdx].Content = newMsgs[lastIdx].Content + sb.String()
	} else {
		newMsgs = append(newMsgs, llm.Message{
			Role:    "user",
			Content: sb.String(),
		})
	}
	return newMsgs
}

// ChatStream streams replies from Ollama's native /api/chat endpoint.
// Text deltas are forwarded to onChunk; thinking deltas are accumulated
// internally. Returns the assembled full response.
func (p *Provider) ChatStream(
	ctx context.Context,
	model string,
	messages []llm.Message,
	tools []llm.ToolDefinition,
	opts llm.Options,
	onChunk func(delta string),
) (*llm.Response, error) {
	slog.Debug("ollama: chat stream", "model", model)

	if opts.ToolsRequired || opts.ToolCallingRequired {
		return p.Chat(ctx, model, messages, tools, opts)
	}

	caps, _ := p.getCapabilities(ctx, model)

	// Fallback path when tools are required but model does not support them
	if (opts.ToolsRequired || opts.ToolCallingRequired) && !caps.SupportsTools {
		slog.Debug("ollama: model does not support tools but they are required; attempting fallback", "model", model)
		fallback, fallbackErr := p.firstAvailableModelWithTools(ctx, model)
		if fallbackErr == nil && fallback != "" && fallback != model {
			slog.Debug("ollama: falling back to model with tool support", "fallback", fallback)
			fallbackCaps, _ := p.getCapabilities(ctx, fallback)
			resp, err := p.doChatStream(ctx, fallback, messages, tools, opts, fallbackCaps, onChunk)
			if err == nil {
				if (opts.ToolsRequired || opts.ToolCallingRequired) && len(resp.ToolCalls) == 0 && isStartOfToolTurn(messages) {
					return nil, fmt.Errorf("ollama: fallback model %q failed to call tools for tool-requiring intent", fallback)
				}
				return resp, nil
			}
			return nil, err
		}
		return nil, fmt.Errorf("ollama: model %q does not support tool calling but tools are required (fallback failed: %v)", model, fallbackErr)
	}

	resp, err := p.doChatStream(ctx, model, messages, tools, opts, caps, onChunk)
	if err == nil {
		if (opts.ToolsRequired || opts.ToolCallingRequired) && len(resp.ToolCalls) == 0 && isStartOfToolTurn(messages) {
			err = fmt.Errorf("ollama: model %q failed to call tools for tool-requiring intent", model)
		} else {
			return resp, nil
		}
	}

	// Fallback path when tools are required and model failed to call tools (or errored out due to tool support issues)
	if (opts.ToolsRequired || opts.ToolCallingRequired) && err != nil && (isToolSupportError(err) || strings.Contains(err.Error(), "failed to call tools")) {
		slog.Debug("ollama: model failed to call tools or does not support them; attempting fallback", "model", model, "error", err)
		fallback, fallbackErr := p.firstAvailableModelWithTools(ctx, model)
		if fallbackErr == nil && fallback != "" && fallback != model {
			slog.Debug("ollama: falling back to model with tool support", "fallback", fallback)
			fallbackCaps, _ := p.getCapabilities(ctx, fallback)
			resp, err = p.doChatStream(ctx, fallback, messages, tools, opts, fallbackCaps, onChunk)
			if err == nil {
				if (opts.ToolsRequired || opts.ToolCallingRequired) && len(resp.ToolCalls) == 0 && isStartOfToolTurn(messages) {
					return nil, fmt.Errorf("ollama: fallback model %q failed to call tools for tool-requiring intent", fallback)
				}
				return resp, nil
			}
			return nil, err
		}
		return nil, err
	}

	if len(tools) > 0 && isToolSupportError(err) {
		if opts.ToolsRequired || opts.ToolCallingRequired {
			return nil, fmt.Errorf("ollama: model %q does not support tools: %w", model, err)
		}
		slog.Debug("ollama: model does not support tools, retrying without", "model", model)
		return p.doChatStream(ctx, model, messages, nil, opts, caps, onChunk)
	}

	if isModelNotFound(err) {
		slog.Debug("ollama: model not found, attempting fallback", "model", model)
		fallback, listErr := p.firstAvailableModel(ctx)
		if listErr != nil {
			return nil, fmt.Errorf("%w (model fallback failed: %v. Try running 'ollama pull %s')", err, listErr, model)
		}
		if fallback == "" || fallback == model {
			return nil, err
		}
		slog.Debug("ollama: falling back to model", "fallback", fallback)
		fallbackCaps, _ := p.getCapabilities(ctx, fallback)
		if (opts.ToolsRequired || opts.ToolCallingRequired) && !fallbackCaps.SupportsTools {
			fallbackWithTools, fbToolsErr := p.firstAvailableModelWithTools(ctx, model)
			if fbToolsErr == nil && fallbackWithTools != "" && fallbackWithTools != model {
				fallback = fallbackWithTools
				fallbackCaps, _ = p.getCapabilities(ctx, fallback)
			} else {
				return nil, fmt.Errorf("ollama: fallback model %q does not support tool calling but tools are required", fallback)
			}
		}
		resp, err = p.doChatStream(ctx, fallback, messages, tools, opts, fallbackCaps, onChunk)
		if err == nil {
			if (opts.ToolsRequired || opts.ToolCallingRequired) && len(resp.ToolCalls) == 0 && isStartOfToolTurn(messages) {
				return nil, fmt.Errorf("ollama: fallback model %q failed to call tools for tool-requiring intent", fallback)
			}
			return resp, nil
		}
		return nil, err
	}

	return nil, err
}

// ---------------------------------------------------------------------------
// internal helpers
// ---------------------------------------------------------------------------

// doChat executes a single non-streaming /api/chat call.
func (p *Provider) doChat(
	ctx context.Context,
	model string,
	messages []llm.Message,
	tools []llm.ToolDefinition,
	opts llm.Options,
	caps ModelCapabilities,
) (*llm.Response, error) {
	chatReq := p.buildRequest(model, messages, tools, opts, caps, false)

	body, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: chat: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		// Decode error body for better diagnostics.
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(res.Body).Decode(&errBody)
		msg := errBody.Error
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", res.StatusCode)
		}
		return nil, fmt.Errorf("ollama: chat: %s", msg)
	}

	var chatResp ollamaChatResponse
	if err := json.NewDecoder(res.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("ollama: decode response: %w", err)
	}

	content := chatResp.Message.Content

	// Some models embed thinking inline as <think>...</think>.
	// If the response includes a dedicated thinking field, the text is clean.
	var thinking string
	if chatResp.Message.Thinking != "" {
		thinking = chatResp.Message.Thinking
	} else if strings.Contains(content, "<think>") {
		content, thinking = extractThinking(content)
	}
	_ = thinking // future: populate llm.Response.Thinking when field is added

	return &llm.Response{
		Content:      content,
		ToolCalls:    convertToolCalls(chatResp.Message.ToolCalls),
		FinishReason: chatResp.DoneReason,
		InputTokens:  chatResp.PromptEvalCount,
		OutputTokens: chatResp.EvalCount,
	}, nil
}

// doChatStream executes a single streaming /api/chat call.
func (p *Provider) doChatStream(
	ctx context.Context,
	model string,
	messages []llm.Message,
	tools []llm.ToolDefinition,
	opts llm.Options,
	caps ModelCapabilities,
	onChunk func(delta string),
) (*llm.Response, error) {
	chatReq := p.buildRequest(model, messages, tools, opts, caps, true)

	body, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: chat stream: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(res.Body).Decode(&errBody)
		msg := errBody.Error
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", res.StatusCode)
		}
		return nil, fmt.Errorf("ollama: chat stream: %s", msg)
	}

	return parseNDJSONStream(ctx, res.Body, onChunk)
}

// buildRequest constructs an ollamaChatRequest, gating provider-specific
// request fields on the model's detected capabilities.
func (p *Provider) buildRequest(
	model string,
	messages []llm.Message,
	tools []llm.ToolDefinition,
	opts llm.Options,
	caps ModelCapabilities,
	stream bool,
) *ollamaChatRequest {
	chatReq := &ollamaChatRequest{
		Model:    model,
		Messages: convertMessages(messages, caps.SupportsVision),
		Stream:   stream,
	}

	// Only attach tools when the model is known to support them.
	if len(tools) > 0 && caps.SupportsTools {
		chatReq.Tools = convertTools(tools)
	}

	if caps.SupportsThinking {
		think := false
		if opts.Think != nil {
			think = *opts.Think
		}
		chatReq.Think = &think
	}

	// Build options only when there is something to configure.
	var o ollamaOptions
	hasOpts := false

	if opts.MaxTokens > 0 {
		o.NumPredict = opts.MaxTokens
		hasOpts = true
	}
	if opts.Temperature > 0 {
		o.Temperature = opts.Temperature
		hasOpts = true
	}
	if opts.RepeatPenalty > 0 {
		o.RepeatPenalty = opts.RepeatPenalty
		hasOpts = true
	}
	if opts.RepeatLastN > 0 {
		o.RepeatLastN = opts.RepeatLastN
		hasOpts = true
	}
	if hasOpts {
		chatReq.Options = &o
	}

	return chatReq
}

func isStartOfToolTurn(messages []llm.Message) bool {
	if len(messages) == 0 {
		return true
	}
	lastMsg := messages[len(messages)-1]
	return lastMsg.Role != "tool"
}

func tryParseToolCallFromText(content string, tools []llm.ToolDefinition) []llm.ToolCall {
	if len(tools) == 0 {
		return nil
	}

	// Helper to check if a name matches any tool
	findTool := func(name string) (llm.ToolDefinition, bool) {
		name = strings.TrimSpace(strings.ToLower(name))
		for _, t := range tools {
			tName := strings.ToLower(t.Name)
			if tName == name || strings.HasSuffix(tName, "."+name) || strings.HasSuffix(name, "."+tName) {
				return t, true
			}
		}
		return llm.ToolDefinition{}, false
	}

	// Try extracting json blocks first
	var blocks []string
	idx := 0
	for {
		start := strings.Index(content[idx:], "```json")
		if start == -1 {
			// Try just ```
			start = strings.Index(content[idx:], "```")
			if start == -1 {
				break
			}
			start += idx
			end := strings.Index(content[start+3:], "```")
			if end == -1 {
				break
			}
			end += start + 3
			blocks = append(blocks, content[start+3:end])
			idx = end + 3
			continue
		}
		start += idx
		end := strings.Index(content[start+7:], "```")
		if end == -1 {
			break
		}
		end += start + 7
		blocks = append(blocks, content[start+7:end])
		idx = end + 3
	}

	// If no code blocks, look for the outermost '{' and '}'
	if len(blocks) == 0 {
		firstBrace := strings.Index(content, "{")
		lastBrace := strings.LastIndex(content, "}")
		if firstBrace != -1 && lastBrace != -1 && lastBrace > firstBrace {
			blocks = append(blocks, content[firstBrace:lastBrace+1])
		}
	}

	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}

		// Try to decode as a direct tool call structure
		var genericCall struct {
			Name       string         `json:"name"`
			Tool       string         `json:"tool"`
			ToolName   string         `json:"tool_name"`
			Function   string         `json:"function"`
			Action     string         `json:"action"`
			Arguments  map[string]any `json:"arguments"`
			Parameters map[string]any `json:"parameters"`
			Params     map[string]any `json:"params"`
			Args       map[string]any `json:"args"`
		}

		if err := json.Unmarshal([]byte(block), &genericCall); err == nil {
			toolName := genericCall.Name
			if toolName == "" {
				toolName = genericCall.Tool
			}
			if toolName == "" {
				toolName = genericCall.ToolName
			}
			if toolName == "" {
				toolName = genericCall.Function
			}
			if toolName == "" {
				toolName = genericCall.Action
			}
			t, ok := findTool(toolName)
			if ok {
				args := genericCall.Arguments
				if len(args) == 0 {
					args = genericCall.Parameters
				}
				if len(args) == 0 {
					args = genericCall.Params
				}
				if len(args) == 0 {
					args = genericCall.Args
				}
				return []llm.ToolCall{{
					ID:        "call_parsed",
					Name:      t.Name,
					Arguments: args,
				}}
			}
		}

		// What if the JSON is just the arguments map for a single/forced tool?
		var argsMap map[string]any
		if err := json.Unmarshal([]byte(block), &argsMap); err == nil && len(argsMap) > 0 {
			// If there's exactly one tool, we can assume it's for that tool
			if len(tools) == 1 {
				// Make sure we didn't accidentally parse a full tool call schema as arguments map.
				if _, hasName := argsMap["name"]; !hasName {
					if _, hasTool := argsMap["tool"]; !hasTool {
						return []llm.ToolCall{{
							ID:        "call_parsed",
							Name:      tools[0].Name,
							Arguments: argsMap,
						}}
					}
				}
			}
		}
	}

	return nil
}
