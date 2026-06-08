// Package orchestration defines the canonical, provider-agnostic NCOS contracts.
package orchestration

import (
	"time"

	"github.com/open-navi/navi/internal/llm"
)

type ExecutionMode string

const (
	ExecutionModeChatTurn   ExecutionMode = "chat_turn"
	ExecutionModeRunExecute ExecutionMode = "run_execute"
	ExecutionModeResume     ExecutionMode = "resume"
)

type InstructionLayerKey string

const (
	InstructionLayerSystemCore         InstructionLayerKey = "system_core"
	InstructionLayerRuntimeConstraints InstructionLayerKey = "runtime_constraints"
	InstructionLayerExperienceOverlay  InstructionLayerKey = "experience_overlay"
	InstructionLayerTaskFraming        InstructionLayerKey = "task_framing"
	InstructionLayerCapabilitySurface  InstructionLayerKey = "capability_surface"
)

type ContextSource string

const (
	ContextSourceHistory    ContextSource = "history"
	ContextSourceSummary    ContextSource = "summary"
	ContextSourceFacts      ContextSource = "facts"
	ContextSourceRuntime    ContextSource = "runtime"
	ContextSourceGovernance ContextSource = "governance"
	ContextSourceScratchpad ContextSource = "scratchpad"
	ContextSourceCapability ContextSource = "capability"
)

type TrustLabel string

const (
	TrustLabelAuthoritative TrustLabel = "authoritative"
	TrustLabelDerived       TrustLabel = "derived"
	TrustLabelHeuristic     TrustLabel = "heuristic"
	TrustLabelUserSupplied  TrustLabel = "user_supplied"
)

type ContextClass string

const (
	ContextClassConversation      ContextClass = "conversation"
	ContextClassChatSummary       ContextClass = "chat_summary"
	ContextClassFacts             ContextClass = "facts"
	ContextClassRuntimeState      ContextClass = "runtime_state"
	ContextClassGovernance        ContextClass = "governance"
	ContextClassProposal          ContextClass = "proposal"
	ContextClassCapabilitySurface ContextClass = "capability_surface"
	ContextClassScratchpad        ContextClass = "scratchpad"
)

type ConversationTurn struct {
	ID          string            `json:"id,omitempty"`
	Role        string            `json:"role,omitempty"`
	Content     string            `json:"content,omitempty"`
	MessageKind string            `json:"message_kind,omitempty"`
	CreatedAt   time.Time         `json:"created_at,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type InstructionFragment struct {
	Key      string `json:"key,omitempty"`
	Content  string `json:"content,omitempty"`
	Required bool   `json:"required,omitempty"`
}

type InstructionLayer struct {
	Key       InstructionLayerKey   `json:"key,omitempty"`
	Fragments []InstructionFragment `json:"fragments,omitempty"`
}

type ContextBudget struct {
	MaxItems        int  `json:"max_items,omitempty"`
	MaxChars        int  `json:"max_chars,omitempty"`
	MaxTokens       int  `json:"max_tokens,omitempty"`
	UsedItems       int  `json:"used_items,omitempty"`
	UsedChars       int  `json:"used_chars,omitempty"`
	UsedTokens      int  `json:"used_tokens,omitempty"`
	ConsideredItems int  `json:"considered_items,omitempty"`
	IncludedItems   int  `json:"included_items,omitempty"`
	ExcludedItems   int  `json:"excluded_items,omitempty"`
	Truncated       bool `json:"truncated,omitempty"`
}

type ScheduledOutput struct {
	Content string        `json:"content,omitempty"`
	Delay   time.Duration `json:"delay,omitempty"`
}

type TraceEvent struct {
	Stage      string            `json:"stage,omitempty"`
	TraceID    string            `json:"trace_id,omitempty"`
	RunID      string            `json:"run_id,omitempty"`
	ChatID     string            `json:"chat_id,omitempty"`
	OccurredAt time.Time         `json:"occurred_at,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type ExecutionFrame struct {
	TraceID          string            `json:"trace_id,omitempty"`
	ChatID           string            `json:"chat_id,omitempty"`
	RunID            string            `json:"run_id,omitempty"`
	CheckpointID     string            `json:"checkpoint_id,omitempty"`
	Phase            string            `json:"phase,omitempty"`
	Mode             ExecutionMode     `json:"mode,omitempty"`
	Scratchpad       map[string]string `json:"scratchpad,omitempty"`
	ResumeProposalID string            `json:"resume_proposal_id,omitempty"`
	ResumeReason     string            `json:"resume_reason,omitempty"`
}

type RequiredOutput struct {
	MustReply             bool `json:"must_reply,omitempty"`
	AllowToolCalls        bool `json:"allow_tool_calls,omitempty"`
	AllowScheduledReplies bool `json:"allow_scheduled_replies,omitempty"`
	ExpectStreaming       bool `json:"expect_streaming,omitempty"`
}

type ContextItem struct {
	ID           string            `json:"id,omitempty"`
	Kind         string            `json:"kind,omitempty"`
	Label        string            `json:"label,omitempty"`
	Content      string            `json:"content,omitempty"`
	Source       ContextSource     `json:"source_type,omitempty"`
	SourceRef    string            `json:"source_ref,omitempty"`
	Trust        TrustLabel        `json:"trust_level,omitempty"`
	ContextClass ContextClass      `json:"context_class,omitempty"`
	Confidence   float64           `json:"confidence,omitempty"`
	Attributes   map[string]string `json:"attributes,omitempty"`
}

type ContextPack struct {
	Items       []ContextItem `json:"items,omitempty"`
	Budget      ContextBudget `json:"budget,omitempty"`
	Warnings    []string      `json:"warnings,omitempty"`
	AssembledAt time.Time     `json:"assembled_at,omitempty"`
}

type InstructionStack struct {
	SystemCore         InstructionLayer `json:"system_core,omitempty"`
	RuntimeConstraints InstructionLayer `json:"runtime_constraints,omitempty"`
	ExperienceOverlay  InstructionLayer `json:"experience_overlay,omitempty"`
	TaskFraming        InstructionLayer `json:"task_framing,omitempty"`
	CapabilitySurface  InstructionLayer `json:"capability_surface,omitempty"`
}

// Ordered returns the non-empty instruction layers in canonical NCOS order.
func (s InstructionStack) Ordered() []InstructionLayer {
	layers := []struct {
		key   InstructionLayerKey
		layer InstructionLayer
	}{
		{key: InstructionLayerSystemCore, layer: s.SystemCore},
		{key: InstructionLayerRuntimeConstraints, layer: s.RuntimeConstraints},
		{key: InstructionLayerExperienceOverlay, layer: s.ExperienceOverlay},
		{key: InstructionLayerTaskFraming, layer: s.TaskFraming},
		{key: InstructionLayerCapabilitySurface, layer: s.CapabilitySurface},
	}

	out := make([]InstructionLayer, 0, len(layers))
	for _, entry := range layers {
		if len(entry.layer.Fragments) == 0 {
			continue
		}
		layer := entry.layer
		layer.Key = entry.key
		out = append(out, layer)
	}
	return out
}

type ModelProfile struct {
	Provider                    string            `json:"provider,omitempty"`
	Model                       string            `json:"model,omitempty"`
	SupportsTools               bool              `json:"supports_tools,omitempty"`
	SupportsStreaming           bool              `json:"supports_streaming,omitempty"`
	SupportsSystemRole          bool              `json:"supports_system_role,omitempty"`
	SupportsMultiSystemMessages bool              `json:"supports_multi_system_messages,omitempty"`
	MaxContextTokens            int               `json:"max_context_tokens,omitempty"`
	Quirks                      []string          `json:"quirks,omitempty"`
	Metadata                    map[string]string `json:"metadata,omitempty"`
}

type CanonicalRunRequest struct {
	Frame             ExecutionFrame     `json:"frame,omitempty"`
	ExperienceMode    string             `json:"experience_mode,omitempty"`
	UserMessage       string             `json:"user_message,omitempty"`
	Conversation      []ConversationTurn `json:"conversation,omitempty"`
	RequiredOutput    RequiredOutput     `json:"required_output,omitempty"`
	CapabilitySurface CapabilitySurface  `json:"capability_surface,omitempty"`
	Model             ModelProfile       `json:"model,omitempty"`
}

type CompiledModelRequest struct {
	Profile  ModelProfile         `json:"profile,omitempty"`
	Messages []llm.Message        `json:"messages,omitempty"`
	Tools    []llm.ToolDefinition `json:"tools,omitempty"`
	Options  llm.Options          `json:"options,omitempty"`
	Metadata map[string]string    `json:"metadata,omitempty"`
}

type NormalizedModelResponse struct {
	Profile      ModelProfile   `json:"profile,omitempty"`
	Content      string         `json:"content,omitempty"`
	ToolCalls    []llm.ToolCall `json:"tool_calls,omitempty"`
	FinishReason string         `json:"finish_reason,omitempty"`
	InputTokens  int            `json:"input_tokens,omitempty"`
	OutputTokens int            `json:"output_tokens,omitempty"`
	RawPayload   map[string]any `json:"raw_payload,omitempty"`
}

type RunResult struct {
	Request         CanonicalRunRequest     `json:"request,omitempty"`
	Response        NormalizedModelResponse `json:"response,omitempty"`
	Paused          bool                    `json:"paused,omitempty"`
	CheckpointID    string                  `json:"checkpoint_id,omitempty"`
	ProposalID      string                  `json:"proposal_id,omitempty"`
	ProposalReason  string                  `json:"proposal_reason,omitempty"`
	PendingToolCall *llm.ToolCall           `json:"pending_tool_call,omitempty"`
	Scheduled       []ScheduledOutput       `json:"scheduled,omitempty"`
}
