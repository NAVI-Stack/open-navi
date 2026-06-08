package anthropic

// ---------------------------------------------------------------------------
// /v1/messages request
// ---------------------------------------------------------------------------

// anthropicRequest is the top-level request body for POST /v1/messages.
type anthropicRequest struct {
	Model     string       `json:"model"`
	MaxTokens int          `json:"max_tokens"`
	Messages  []antMessage `json:"messages"`
	// System is either a plain string or a []antContentBlock for multi-block
	// system prompts. Omitted when empty.
	System     any          `json:"system,omitempty"`
	Tools      []antTool    `json:"tools,omitempty"`
	ToolChoice any          `json:"tool_choice,omitempty"`
	Stream     bool         `json:"stream,omitempty"`
	Thinking   *antThinking `json:"thinking,omitempty"`
}

// antMessage is a single conversation turn on the Anthropic wire.
// Content is polymorphic: plain string for simple text, or []antContentBlock
// for multi-block messages (tool results, images, etc.).
type antMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// ---------------------------------------------------------------------------
// Content blocks
// ---------------------------------------------------------------------------

// antContentBlock is a polymorphic content element. Only the fields relevant
// to the block Type are populated; the rest are omitted from JSON.
type antContentBlock struct {
	Type string `json:"type"` // "text", "image", "tool_use", "tool_result", "thinking"

	// text block
	Text string `json:"text,omitempty"`

	// thinking block (response only)
	Thinking string `json:"thinking,omitempty"`

	// tool_use block
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Input any    `json:"input,omitempty"`

	// image block
	Source *antImageSource `json:"source,omitempty"`

	// tool_result block
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   any    `json:"content,omitempty"` // string or []antContentBlock
}

// antImageSource describes an inline image for Anthropic's image content block.
type antImageSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // "image/png", "image/jpeg", etc.
	Data      string `json:"data"`       // raw base64 payload
}

// ---------------------------------------------------------------------------
// Tool definitions
// ---------------------------------------------------------------------------

type antTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

// ---------------------------------------------------------------------------
// Extended thinking
// ---------------------------------------------------------------------------

type antThinking struct {
	Type         string `json:"type"`          // "enabled"
	BudgetTokens int    `json:"budget_tokens"` // max tokens for thinking
}

// ---------------------------------------------------------------------------
// /v1/messages response (non-streaming)
// ---------------------------------------------------------------------------

type anthropicResponse struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"` // "message"
	Content    []antContentBlock `json:"content"`
	StopReason string            `json:"stop_reason"`
	Usage      anthropicUsage    `json:"usage"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// ---------------------------------------------------------------------------
// SSE stream event types
// ---------------------------------------------------------------------------

type sseMessageStart struct {
	Type    string `json:"type"` // "message_start"
	Message struct {
		ID    string         `json:"id"`
		Usage anthropicUsage `json:"usage"`
	} `json:"message"`
}

type sseContentBlockStart struct {
	Type         string `json:"type"` // "content_block_start"
	Index        int    `json:"index"`
	ContentBlock struct {
		Type string `json:"type"` // "text", "tool_use", "thinking"
		ID   string `json:"id,omitempty"`
		Name string `json:"name,omitempty"`
	} `json:"content_block"`
}

type sseContentBlockDelta struct {
	Type  string `json:"type"` // "content_block_delta"
	Index int    `json:"index"`
	Delta struct {
		Type        string `json:"type"` // "text_delta", "input_json_delta", "thinking_delta"
		Text        string `json:"text,omitempty"`
		PartialJSON string `json:"partial_json,omitempty"`
		Thinking    string `json:"thinking,omitempty"`
	} `json:"delta"`
}

type sseMessageDelta struct {
	Type  string `json:"type"` // "message_delta"
	Delta struct {
		StopReason string `json:"stop_reason"`
	} `json:"delta"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type sseError struct {
	Type  string `json:"type"` // "error"
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}
