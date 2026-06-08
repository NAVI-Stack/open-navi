package ollama

import "time"

// ---------------------------------------------------------------------------
// /api/chat request
// ---------------------------------------------------------------------------

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Tools    []ollamaTool    `json:"tools,omitempty"`
	Stream   bool            `json:"stream"`
	// Think is a pointer so think=false is serialized for thinking models.
	Think   *bool          `json:"think,omitempty"`
	Options *ollamaOptions `json:"options,omitempty"`
}

// ollamaMessage is a single turn in Ollama's native chat format.
type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	// Thinking contains reasoning content emitted by thinking-capable models.
	// Present in streaming deltas and some non-streaming responses.
	Thinking  string           `json:"thinking,omitempty"`
	Images    []string         `json:"images,omitempty"`     // base64, no data-URI prefix
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"` // assistant → tool invocations
}

type ollamaOptions struct {
	NumCtx        int     `json:"num_ctx,omitempty"`
	NumPredict    int     `json:"num_predict,omitempty"` // maps to MaxTokens
	Temperature   float64 `json:"temperature,omitempty"`
	RepeatPenalty float64 `json:"repeat_penalty,omitempty"`
	RepeatLastN   int     `json:"repeat_last_n,omitempty"`
}

// ---------------------------------------------------------------------------
// Tool types
// ---------------------------------------------------------------------------

type ollamaTool struct {
	Type     string         `json:"type"` // always "function"
	Function ollamaToolFunc `json:"function"`
}

type ollamaToolFunc struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ollamaToolCall struct {
	Function ollamaToolCallFunc `json:"function"`
}

type ollamaToolCallFunc struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// ---------------------------------------------------------------------------
// /api/chat response (non-streaming)
// ---------------------------------------------------------------------------

type ollamaChatResponse struct {
	Model           string        `json:"model"`
	CreatedAt       string        `json:"created_at"`
	Message         ollamaMessage `json:"message"`
	Done            bool          `json:"done"`
	DoneReason      string        `json:"done_reason"`
	PromptEvalCount int           `json:"prompt_eval_count"`
	EvalCount       int           `json:"eval_count"`
}

// ---------------------------------------------------------------------------
// /api/chat stream chunk (NDJSON)
// ---------------------------------------------------------------------------

type ollamaStreamChunk struct {
	Model           string        `json:"model"`
	CreatedAt       string        `json:"created_at"`
	Message         ollamaMessage `json:"message"`
	Done            bool          `json:"done"`
	DoneReason      string        `json:"done_reason,omitempty"`
	PromptEvalCount int           `json:"prompt_eval_count,omitempty"`
	EvalCount       int           `json:"eval_count,omitempty"`
}

// ---------------------------------------------------------------------------
// /api/show
// ---------------------------------------------------------------------------

type ollamaShowRequest struct {
	Name string `json:"name"`
}

type ollamaShowResponse struct {
	Modelfile    string            `json:"modelfile"`
	Parameters   string            `json:"parameters"`
	Template     string            `json:"template"`
	Details      ollamaModelDetail `json:"details"`
	ModelInfo    map[string]any    `json:"model_info"`
	Capabilities []string          `json:"capabilities"` // Ollama 0.5+
	ModifiedAt   time.Time         `json:"modified_at"`
}

type ollamaModelDetail struct {
	ParentModel       string   `json:"parent_model"`
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

// ---------------------------------------------------------------------------
// /api/tags
// ---------------------------------------------------------------------------

type ollamaTagsResponse struct {
	Models []ollamaTagModel `json:"models"`
}

type ollamaTagModel struct {
	Name       string            `json:"name"`
	Model      string            `json:"model"`
	ModifiedAt time.Time         `json:"modified_at"`
	Size       int64             `json:"size"`
	Digest     string            `json:"digest"`
	Details    ollamaModelDetail `json:"details"`
}

// ---------------------------------------------------------------------------
// /api/ps
// ---------------------------------------------------------------------------

type ollamaPsResponse struct {
	Models []ollamaPsModel `json:"models"`
}

type ollamaPsModel struct {
	Name      string    `json:"name"`
	Model     string    `json:"model"`
	Size      int64     `json:"size"`
	SizeVRAM  int64     `json:"size_vram"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ---------------------------------------------------------------------------
// /api/pull progress chunk (NDJSON)
// ---------------------------------------------------------------------------

type ollamaPullProgress struct {
	Status    string `json:"status"`
	Digest    string `json:"digest"`
	Total     int64  `json:"total"`
	Completed int64  `json:"completed"`
}
