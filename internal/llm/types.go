package llm

import "context"

// Message is a single turn in a conversation.
type Message struct {
	Role       string     `json:"role"` // "system", "user", "assistant", "tool"
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall is a function invocation requested by the model.
type ToolCall struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// ToolDefinition describes a tool the model may call.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema object
}

// Response is the model's reply to a Chat call.
type Response struct {
	Content      string     `json:"content"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	FinishReason string     `json:"finish_reason"`
	InputTokens  int        `json:"input_tokens"`
	OutputTokens int        `json:"output_tokens"`
}

// Provider is the interface every LLM backend satisfies.
type Provider interface {
	Chat(ctx context.Context, model string, messages []Message, tools []ToolDefinition, opts Options) (*Response, error)
	Name() string
}

// StreamingProvider is optional. When implemented, ChatStream streams content deltas via onChunk.
// Tool calls are not streamed; streaming is used only for final text replies.
type StreamingProvider interface {
	Provider
	ChatStream(ctx context.Context, model string, messages []Message, tools []ToolDefinition, opts Options, onChunk func(delta string)) (*Response, error)
}

// ManagedProvider extends Provider with model inspection and health checking.
// Providers that support management (listing models, health) implement this.
type ManagedProvider interface {
	Provider

	HealthCheck(ctx context.Context) (ProviderHealth, error)
	ListModels(ctx context.Context) ([]ModelDescriptor, error)
	GetModel(ctx context.Context, model string) (ModelDescriptor, error)
}

// LocalLifecycleProvider extends ManagedProvider with local model lifecycle operations.
// Only providers that manage local model storage (e.g. Ollama) implement this.
type LocalLifecycleProvider interface {
	ManagedProvider

	ListRunningModels(ctx context.Context) ([]RunningModel, error)
	PullModel(ctx context.Context, model string) (ProviderOperation, error)
	DeleteModel(ctx context.Context, model string) (ProviderOperation, error)
	CopyModel(ctx context.Context, source, target string) (ProviderOperation, error)
	CreateModel(ctx context.Context, req CreateModelRequest) (ProviderOperation, error)
	WarmModel(ctx context.Context, model string) error
}

// Options are per-call tuning parameters.
type Options struct {
	MaxTokens           int     `json:"max_tokens"`
	Temperature         float64 `json:"temperature"`
	RepeatPenalty       float64 `json:"repeat_penalty,omitempty"`
	RepeatLastN         int     `json:"repeat_last_n,omitempty"`
	Think               *bool   `json:"think,omitempty"`
	ToolChoice          string  `json:"tool_choice,omitempty"`
	ToolsRequired       bool    `json:"tools_required,omitempty"`
	ToolCallingRequired bool    `json:"tool_calling_required,omitempty"`
}
