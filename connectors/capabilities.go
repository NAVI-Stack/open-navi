package connectors

import (
	"context"
	"net/http"
)

// WebhookHandler is an optional capability for connectors that receive HTTP webhooks.
type WebhookHandler interface {
	WebhookPath() string
	HandleWebhook(w http.ResponseWriter, r *http.Request)
}

// MediaSender is an optional capability for connectors that can send media.
type MediaSender interface {
	SendMedia(ctx context.Context, msg OutboundMediaMessage) error
}

// HealthChecker is an optional capability for connectors that support health checks.
type HealthChecker interface {
	HealthCheck(ctx context.Context) error
}

// TypingCapable is an optional capability for connectors that can show a
// typing/thinking indicator. StartTyping begins the indicator and returns a
// stop function. The stop function MUST be idempotent and safe to call
// multiple times.
type TypingCapable interface {
	StartTyping(ctx context.Context, chatID string) (stop func(), err error)
}

// MessageEditor is an optional capability for connectors that can edit an
// existing message in place.
type MessageEditor interface {
	EditMessage(ctx context.Context, chatID, messageID, content string) error
}

// ReactionCapable is an optional capability for connectors that can add a
// reaction (e.g. 👀) to an inbound message. The undo function MUST be
// idempotent and safe to call multiple times.
type ReactionCapable interface {
	ReactToMessage(ctx context.Context, chatID, messageID string) (undo func(), err error)
}

// PlaceholderCapable is an optional capability for connectors that can send
// a placeholder message (e.g. "Thinking... 💭") that will later be edited
// to the actual response. The connector MUST also implement MessageEditor
// for the placeholder to be useful.
type PlaceholderCapable interface {
	SendPlaceholder(ctx context.Context, chatID string) (messageID string, err error)
}

// InboundOrchestrationManaged is an optional capability for connectors that
// fully manage their own inbound chat UX (typing, placeholder, streaming
// updates) and therefore should not receive the gateway's generic
// Manager.OnInbound orchestration for chat message API calls.
type InboundOrchestrationManaged interface {
	ManagesInboundOrchestration() bool
}

// MessageLengthProvider is an optional capability for connectors that have a
// maximum message length. The Manager uses this to automatically split
// outbound messages that exceed the limit. A value of 0 means no limit.
type MessageLengthProvider interface {
	MaxMessageLength() int
}

// Categorizable is an optional capability that exposes the connector's taxonomy category.
// Categories align with the conceptual design: Communication, Information, Service, Device.
// Identity and Access are cross-cutting (auth/scopes) and not a peer category.
const (
	CategoryCommunication = "communication"
	CategoryInformation   = "information"
	CategoryService       = "service"
	CategoryDevice        = "device"
)

// Categorizable is an optional capability that exposes the connector's taxonomy category.
type Categorizable interface {
	Category() string
}
