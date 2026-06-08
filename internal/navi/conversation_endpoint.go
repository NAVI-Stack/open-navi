package navi

import (
	"context"
	"errors"
	"time"
)

type ConversationEndpointType string
type ConversationEndpointStatus string
type DeliveryPolicyMode string
type MessageDeliveryStatus string

const (
	EndpointTypeConsole   ConversationEndpointType = "console"
	EndpointTypeConnector ConversationEndpointType = "connector"

	EndpointStatusActive   ConversationEndpointStatus = "active"
	EndpointStatusDisabled ConversationEndpointStatus = "disabled"
	EndpointStatusDeleted  ConversationEndpointStatus = "deleted"

	DeliveryPolicyReplyToOrigin DeliveryPolicyMode = "reply_to_origin"
	DeliveryPolicyExplicitOnly  DeliveryPolicyMode = "explicit_only"

	MessageDeliveryQueued  MessageDeliveryStatus = "queued"
	MessageDeliverySent    MessageDeliveryStatus = "sent"
	MessageDeliveryFailed  MessageDeliveryStatus = "failed"
	MessageDeliverySkipped MessageDeliveryStatus = "skipped"
)

var (
	ErrConversationEndpointNotFound        = errors.New("navi: conversation endpoint not found")
	ErrConversationEndpointReceiveDisabled = errors.New("navi: conversation endpoint receive disabled")
	ErrConversationEndpointSendDisabled    = errors.New("navi: conversation endpoint send disabled")
	ErrConversationEndpointChatMismatch    = errors.New("navi: conversation endpoint does not belong to chat")
)

type ConversationEndpoint struct {
	ID                  ID                         `json:"endpoint_id"`
	ChatID              ID                         `json:"chat_id"`
	Type                ConversationEndpointType   `json:"endpoint_type"`
	ConnectorKind       string                     `json:"connector_kind"`
	ConnectorInstanceID string                     `json:"connector_instance_id"`
	ExternalChatID      string                     `json:"external_chat_id"`
	ExternalThreadID    string                     `json:"external_thread_id"`
	DisplayName         string                     `json:"display_name"`
	ReceiveEnabled      bool                       `json:"receive_enabled"`
	SendEnabled         bool                       `json:"send_enabled"`
	MirrorEnabled       bool                       `json:"mirror_enabled"`
	Status              ConversationEndpointStatus `json:"status"`
	CreatedAt           time.Time                  `json:"created_at"`
	UpdatedAt           time.Time                  `json:"updated_at"`
	Metadata            map[string]any             `json:"metadata,omitempty"`
}

type UpsertConnectorEndpointInput struct {
	ID                  string
	ChatID              string
	ConnectorKind       string
	ConnectorInstanceID string
	ExternalChatID      string
	ExternalThreadID    string
	DisplayName         string
	ReceiveEnabled      *bool
	SendEnabled         *bool
	MirrorEnabled       *bool
	Status              ConversationEndpointStatus
	Metadata            map[string]any
}

type UpdateConversationEndpointInput struct {
	DisplayName    *string
	ReceiveEnabled *bool
	SendEnabled    *bool
	MirrorEnabled  *bool
	Status         *ConversationEndpointStatus
	Metadata       map[string]any
}

type ChatDeliveryPolicy struct {
	ChatID    ID                 `json:"chat_id"`
	Mode      DeliveryPolicyMode `json:"default_mode"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
	Metadata  map[string]any     `json:"metadata,omitempty"`
}

type MessageDelivery struct {
	ID                  ID                    `json:"delivery_id"`
	MessageID           ID                    `json:"message_id"`
	ChatID              ID                    `json:"chat_id"`
	EndpointID          ID                    `json:"endpoint_id"`
	ConnectorInstanceID string                `json:"connector_instance_id"`
	Status              MessageDeliveryStatus `json:"status"`
	AttemptCount        int                   `json:"attempt_count"`
	LastError           string                `json:"last_error"`
	CreatedAt           time.Time             `json:"created_at"`
	UpdatedAt           time.Time             `json:"updated_at"`
	Metadata            map[string]any        `json:"metadata,omitempty"`
}

type RecordMessageDeliveryInput struct {
	ID                  string
	MessageID           string
	ChatID              string
	EndpointID          string
	ConnectorInstanceID string
	Status              MessageDeliveryStatus
	AttemptCount        int
	LastError           string
	Metadata            map[string]any
}

type UpdateMessageDeliveryInput struct {
	Status       MessageDeliveryStatus
	AttemptDelta int
	LastError    string
}

type ConversationEndpointStore interface {
	EnsureConsoleEndpoint(ctx context.Context, chatID string) (*ConversationEndpoint, error)
	UpsertConnectorEndpoint(ctx context.Context, input UpsertConnectorEndpointInput) (*ConversationEndpoint, error)
	ListConversationEndpoints(ctx context.Context, chatID string) ([]ConversationEndpoint, error)
	GetConversationEndpoint(ctx context.Context, endpointID string) (*ConversationEndpoint, error)
	ResolveConnectorEndpoint(ctx context.Context, connectorInstanceID, externalChatID, externalThreadID string) (*ConversationEndpoint, error)
	UpdateConversationEndpoint(ctx context.Context, endpointID string, input UpdateConversationEndpointInput) (*ConversationEndpoint, error)
	GetChatDeliveryPolicy(ctx context.Context, chatID string) (*ChatDeliveryPolicy, error)
	SetChatDeliveryPolicy(ctx context.Context, chatID string, mode DeliveryPolicyMode, metadata map[string]any) (*ChatDeliveryPolicy, error)
	RecordMessageDelivery(ctx context.Context, input RecordMessageDeliveryInput) (*MessageDelivery, error)
	UpdateMessageDelivery(ctx context.Context, deliveryID string, input UpdateMessageDeliveryInput) (*MessageDelivery, error)
}
