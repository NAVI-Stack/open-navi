package navi

import (
	"time"

	"github.com/open-navi/navi/internal/schema"
)

// ChatRuntimeView is the runtime executor's view of a chat conversation —
// a flat, runtime-oriented projection of [Chat] + [ChatMessage] augmented with
// runtime-side context (experience mode, runtime session kind) the executor
// needs in a single read.
//
// Populated by [chatThreadToRuntimeView] from a [ChatThread] returned by the
// [ChatStore]. Not persisted directly; the persistent transcript is in
// `navi_chat_messages`.
type ChatRuntimeView struct {
	ChatID             string                    `json:"chat_id"`
	Title              string                    `json:"title,omitempty"`
	ExperienceMode     ExperienceMode            `json:"experience_mode"`
	RuntimeSessionKind schema.RuntimeSessionKind `json:"runtime_session_kind"`
	Messages           []ChatRuntimeMessage      `json:"messages,omitempty"`
	CreatedAt          time.Time                 `json:"created_at"`
	UpdatedAt          time.Time                 `json:"updated_at"`
	ArchivedAt         *time.Time                `json:"archived_at,omitempty"`
	ProjectID          string                    `json:"project_id,omitempty"`
}

// ChatRuntimeMessage is the runtime executor's view of a single chat message.
// Role is the canonical [ChatMessage] role ("user", "assistant", "system",
// "tool") — the legacy "navi" alias is no longer produced by the store layer.
type ChatRuntimeMessage struct {
	ID                    string     `json:"message_id"`
	Role                  string     `json:"role"`
	Content               string     `json:"content"`
	CreatedAt             time.Time  `json:"created_at"`
	InboxItemID           string     `json:"inbox_item_id,omitempty"`
	RunID                 string     `json:"run_id,omitempty"`
	SourceChannel         string     `json:"source_channel,omitempty"`
	SourceMessageRef      string     `json:"source_message_ref,omitempty"`
	MessageKind           string     `json:"message_kind,omitempty"`
	CompactedAt           *time.Time `json:"compacted_at,omitempty"`
	CompactedCheckpointID string     `json:"compacted_checkpoint_id,omitempty"`
	CompactedEpochID      string     `json:"compacted_epoch_id,omitempty"`
}

// HeartbeatAutoRuntimeSessionID is the canonical identifier for the internal
// heartbeat-driven runtime session.
const HeartbeatAutoRuntimeSessionID = schema.HeartbeatAutoRuntimeSessionID

func IsInternalRuntimeSessionID(id string) bool {
	return schema.IsInternalRuntimeSessionID(id)
}

func IsInternalChatRuntime(view *ChatRuntimeView) bool {
	if view == nil {
		return false
	}
	return schema.IsInternalRuntimeSession(view.RuntimeSessionKind, view.ChatID)
}

// NormalizeAssistantMessageKind defaults the message kind to "reply" for
// assistant-authored messages when none is provided.
func NormalizeAssistantMessageKind(role string, kind string) string {
	if role != "assistant" {
		return kind
	}
	if kind == "" {
		return string(schema.AssistantMessageKindReply)
	}
	return kind
}

// chatThreadToRuntimeView projects a [ChatThread] (persisted form) into a
// [ChatRuntimeView] (runtime form). The runtime session kind defaults to
// user; callers that have a richer [RuntimeSession] context may overwrite it.
// The experience mode is resolved from the chat AI configuration when present
// and falls back to ExperienceModeStandard.
func chatThreadToRuntimeView(thread *ChatThread) *ChatRuntimeView {
	if thread == nil {
		return nil
	}
	view := &ChatRuntimeView{
		ChatID:             string(thread.Chat.ID),
		Title:              thread.Chat.Title,
		ExperienceMode:     experienceModeFromChat(thread.Chat),
		RuntimeSessionKind: schema.DefaultRuntimeSessionKindForID(string(thread.Chat.ID)),
		CreatedAt:          thread.Chat.CreatedAt,
		UpdatedAt:          thread.Chat.UpdatedAt,
		ArchivedAt:         thread.Chat.ArchivedAt,
	}
	if thread.Chat.ProjectID != nil {
		view.ProjectID = string(*thread.Chat.ProjectID)
	}
	view.Messages = make([]ChatRuntimeMessage, 0, len(thread.Messages))
	for _, m := range thread.Messages {
		view.Messages = append(view.Messages, chatMessageToRuntime(m))
	}
	return view
}

func chatMessageToRuntime(m ChatMessage) ChatRuntimeMessage {
	out := ChatRuntimeMessage{
		ID:               string(m.ID),
		Role:             m.Role,
		Content:          m.Content,
		CreatedAt:        m.CreatedAt,
		SourceChannel:    m.SourceChannel,
		SourceMessageRef: m.SourceMessageRef,
		MessageKind:      m.MessageKind,
		CompactedAt:      m.CompactedAt,
	}
	if m.InboxItemID != nil {
		out.InboxItemID = string(*m.InboxItemID)
	}
	if m.RunID != nil {
		out.RunID = string(*m.RunID)
	}
	if m.CompactedCheckpointID != nil {
		out.CompactedCheckpointID = string(*m.CompactedCheckpointID)
	}
	if m.CompactedEpochID != nil {
		out.CompactedEpochID = string(*m.CompactedEpochID)
	}
	return out
}

func idPtrToString(id *ID) string {
	if id == nil {
		return ""
	}
	return string(*id)
}

func experienceModeFromChat(chat Chat) ExperienceMode {
	if raw, ok := chat.AIConfig.Metadata["experience_mode"]; ok {
		if s, ok := raw.(string); ok && s != "" {
			return NormalizeExperienceMode(ExperienceMode(s))
		}
	}
	return ExperienceModeStandard
}
