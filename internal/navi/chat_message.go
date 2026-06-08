package navi

import "time"

type ChatMessage struct {
	ID        ID        `json:"id"`
	ChatID    ID        `json:"chatId"`
	Role      string    `json:"role"` // "user" | "assistant" | "system" | "tool" | "navi"
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`

	RuntimeSessionID *ID `json:"runtimeSessionId,omitempty"`
	RunID            *ID `json:"runId,omitempty"`
	InboxItemID      *ID `json:"inboxItemId,omitempty"`

	SourceChannel    string `json:"sourceChannel,omitempty"`
	SourceMessageRef string `json:"sourceMessageRef,omitempty"`
	OriginEndpointID *ID    `json:"originEndpointId,omitempty"`
	MessageKind      string `json:"messageKind,omitempty"`

	CompactedAt           *time.Time `json:"compactedAt,omitempty"`
	CompactedCheckpointID *ID        `json:"compactedCheckpointId,omitempty"`
	CompactedEpochID      *ID        `json:"compactedEpochId,omitempty"`

	Metadata map[string]any `json:"metadata,omitempty"`
}
