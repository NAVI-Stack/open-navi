package runtime

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type InboxItem struct {
	ID               string          `json:"id"`
	RuntimeSessionID string          `json:"runtime_session_id,omitempty"`
	ChatID           string          `json:"chat_id,omitempty"`
	SourceChannel    string          `json:"source_channel"` // "cli" | "app" | "web" | "telegram" | "system"
	SourceMessageRef string          `json:"source_message_ref,omitempty"`
	OriginEndpointID string          `json:"origin_endpoint_id,omitempty"`
	ActorType        string          `json:"actor_type,omitempty"`
	PayloadType      string          `json:"payload_type,omitempty"`
	QueueAction      string          `json:"queue_action"` // "append" (default for now)
	Status           InboxStatus     `json:"status"`
	Content          string          `json:"content,omitempty"`
	Structured       json.RawMessage `json:"structured,omitempty"`
	CorrelationID    string          `json:"correlation_id,omitempty"`
	IdempotencyKey   string          `json:"idempotency_key,omitempty"`
	MessageID        string          `json:"message_id,omitempty"`
	RunID            string          `json:"run_id,omitempty"`
	ClassifiedReason string          `json:"classified_reason,omitempty"`
	Confidence       float64         `json:"confidence,omitempty"`
	MergedIntoID     string          `json:"merged_into_id,omitempty"`
	ReceivedAt       time.Time       `json:"received_at"`
}

type MessageInput struct {
	Content          string `json:"content"`
	ChatID           string `json:"chat_id,omitempty"`
	RuntimeSessionID string `json:"runtime_session_id,omitempty"`
	MessageID        string `json:"message_id,omitempty"`
	SourceChannel    string `json:"source_channel,omitempty"`
	SourceMessageRef string `json:"source_message_ref,omitempty"`
	OriginEndpointID string `json:"origin_endpoint_id,omitempty"`
	IdempotencyKey   string `json:"idempotency_key,omitempty"`
}

// NewInboxItem creates a pending InboxItem with a generated ID.
// The runtime session is the dispatch key and acts as the default chat_id when
// callers do not provide an explicit transcript identity.
func NewInboxItem(runtimeSessionID, content, sourceChannel string) *InboxItem {
	action := "append"
	return &InboxItem{
		ID:               uuid.New().String(),
		RuntimeSessionID: runtimeSessionID,
		ChatID:           runtimeSessionID,
		SourceChannel:    sourceChannel,
		ActorType:        "user",
		PayloadType:      "text",
		QueueAction:      action,
		Status:           InboxStatusPending,
		Content:          content,
		ReceivedAt:       time.Now().UTC(),
	}
}

// Consume marks the inbox item as consumed by the run coordinator.
func (i *InboxItem) Consume() {
	i.Status = InboxStatusConsumed
}
