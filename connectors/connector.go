package connectors

import "context"

// OutboundMessage is a message to be sent to a user through a connector.
type OutboundMessage struct {
	Channel             string
	ChatID              string
	Content             string
	MessageID           string
	EndpointID          string
	DeliveryID          string
	ConnectorInstanceID string
	ExternalThreadID    string
	RuntimeSessionID    string
	RunID               string
	CorrelationID       string
	SourceMessageRef    string
	ReplyToMessageID    int64  // Telegram: reply_to_message_id (0 = not set)
	MessageThreadID     int64  // Telegram: message_thread_id for topics (0 = not set)
	ParseMode           string // e.g. "Markdown", "MarkdownV2", "HTML"; empty = plain text
}

// MediaPart represents one part of a multipart media message.
type MediaPart struct {
	ContentType string
	Data        []byte
	Filename    string
}

// OutboundMediaMessage is a message containing media to be sent through a connector.
type OutboundMediaMessage struct {
	Channel string
	ChatID  string
	Parts   []MediaPart
}

// Connector is the interface that all channel connectors must implement.
// Additional capabilities (media, webhooks, typing, etc.) are expressed via
// optional interfaces in capabilities.go and discovered by type assertion.
type Connector interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	// Send delivers a message to the specified chat. Implementations should
	// return classified errors (ErrRateLimit, ErrTemporary, ErrNotRunning,
	// ErrSendFailed) so the Manager can apply the correct retry strategy.
	Send(ctx context.Context, msg OutboundMessage) error
	IsRunning() bool
}
