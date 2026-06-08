package openai

// ---------------------------------------------------------------------------
// /v1/chat/completions request
// ---------------------------------------------------------------------------

type chatRequest struct {
	Model    string       `json:"model"`
	Messages []oaiMessage `json:"messages"`
	Tools    []oaiTool    `json:"tools,omitempty"`
	Stream   bool         `json:"stream,omitempty"`

	// Standard models use max_tokens; reasoning models (o1, o3, o4-mini) use
	// max_completion_tokens. Only one should be set per request.
	MaxTokens           int `json:"max_tokens,omitempty"`
	MaxCompletionTokens int `json:"max_completion_tokens,omitempty"`

	Temperature *float64 `json:"temperature,omitempty"` // pointer so 0 is distinguishable from absent
	ToolChoice  any      `json:"tool_choice,omitempty"` // string or object

	// StreamOptions requests usage data in the final streaming chunk.
	StreamOptions *streamOptions `json:"stream_options,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// oaiMessage is a single turn on the OpenAI wire.
// Content is polymorphic: plain string for text, or []oaiContentBlock for
// multi-modal messages (images, etc.).
type oaiMessage struct {
	Role       string        `json:"role"`
	Content    any           `json:"content"` // string or []oaiContentBlock
	ToolCallID string        `json:"tool_call_id,omitempty"`
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
}

// oaiContentBlock is a typed content element for multi-modal messages.
type oaiContentBlock struct {
	Type     string       `json:"type"` // "text" or "image_url"
	Text     string       `json:"text,omitempty"`
	ImageURL *oaiImageURL `json:"image_url,omitempty"`
}

type oaiImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"` // "auto", "low", "high"
}

// ---------------------------------------------------------------------------
// Tool definitions
// ---------------------------------------------------------------------------

type oaiTool struct {
	Type     string      `json:"type"` // "function"
	Function oaiToolFunc `json:"function"`
}

type oaiToolFunc struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

// ---------------------------------------------------------------------------
// Tool calls (in messages and responses)
// ---------------------------------------------------------------------------

type oaiToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"` // "function"
	Function oaiToolCallFunc `json:"function"`
}

type oaiToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// ---------------------------------------------------------------------------
// /v1/chat/completions response (non-streaming)
// ---------------------------------------------------------------------------

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
	Usage   chatUsage    `json:"usage"`
}

type chatChoice struct {
	Message      chatChoiceMessage `json:"message"`
	FinishReason string            `json:"finish_reason"`
}

type chatChoiceMessage struct {
	Role      string        `json:"role"`
	Content   string        `json:"content"`
	ToolCalls []oaiToolCall `json:"tool_calls,omitempty"`
}

type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// ---------------------------------------------------------------------------
// SSE stream chunk
// ---------------------------------------------------------------------------

type streamChunk struct {
	Choices []streamChoice `json:"choices"`
	Usage   *chatUsage     `json:"usage,omitempty"`
}

type streamChoice struct {
	Delta        streamDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

type streamDelta struct {
	Content   string          `json:"content"`
	ToolCalls []streamDeltaTC `json:"tool_calls,omitempty"`
}

// streamDeltaTC represents an incremental tool call delta in SSE streaming.
type streamDeltaTC struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`   // present on first chunk
	Type     string `json:"type,omitempty"` // "function"
	Function struct {
		Name      string `json:"name,omitempty"`      // present on first chunk
		Arguments string `json:"arguments,omitempty"` // streamed incrementally
	} `json:"function"`
}
