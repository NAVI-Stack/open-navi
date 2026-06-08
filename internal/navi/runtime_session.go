package navi

import (
	"context"
	"time"
)

type RuntimeSession struct {
	ID          ID  `json:"id"`
	OwnerID     ID  `json:"ownerId,omitempty"`
	WorkspaceID *ID `json:"workspaceId,omitempty"`
	ProjectID   *ID `json:"projectId,omitempty"`

	Kind   RuntimeSessionKind   `json:"kind"`
	Status RuntimeSessionStatus `json:"status"`

	ExperienceMode string `json:"experienceMode,omitempty"`
	SourceChannel  string `json:"sourceChannel,omitempty"`

	StartedAt    time.Time  `json:"startedAt"`
	LastActiveAt time.Time  `json:"lastActiveAt"`
	EndedAt      *time.Time `json:"endedAt,omitempty"`

	Metadata map[string]any `json:"metadata,omitempty"`
}

type CreateRuntimeSessionInput struct {
	ID          string
	OwnerID     string
	WorkspaceID string
	ProjectID   string

	Kind   RuntimeSessionKind
	Status RuntimeSessionStatus

	ExperienceMode string
	SourceChannel  string

	Metadata map[string]any
}

type RuntimeSessionStore interface {
	CreateRuntimeSession(ctx context.Context, input CreateRuntimeSessionInput) (*RuntimeSession, error)
	GetRuntimeSession(ctx context.Context, runtimeSessionID string) (*RuntimeSession, error)
	TouchRuntimeSession(ctx context.Context, runtimeSessionID string) error
	CloseRuntimeSession(ctx context.Context, runtimeSessionID string) error
	AttachChatToRuntimeSession(ctx context.Context, runtimeSessionID, chatID string, relationship RuntimeSessionChatRelationship) error
	ListRuntimeSessionChats(ctx context.Context, runtimeSessionID string) ([]RuntimeSessionChat, error)
	FindActiveRuntimeSessionForChat(ctx context.Context, chatID string, sourceChannel string) (*RuntimeSession, error)
}

type RuntimeSessionKind string

const (
	RuntimeSessionKindUser       RuntimeSessionKind = "user"
	RuntimeSessionKindInternal   RuntimeSessionKind = "internal"
	RuntimeSessionKindBackground RuntimeSessionKind = "background"
	RuntimeSessionKindConnector  RuntimeSessionKind = "connector"
	RuntimeSessionKindDreaming   RuntimeSessionKind = "dreaming"
)

type RuntimeSessionStatus string

const (
	RuntimeSessionStatusActive RuntimeSessionStatus = "active"
	RuntimeSessionStatusIdle   RuntimeSessionStatus = "idle"
	RuntimeSessionStatusClosed RuntimeSessionStatus = "closed"
	RuntimeSessionStatusFailed RuntimeSessionStatus = "failed"
)

type RuntimeSessionChat struct {
	RuntimeSessionID ID `json:"runtimeSessionId"`
	ChatID           ID `json:"chatId"`

	Relationship RuntimeSessionChatRelationship `json:"relationship"`
	AttachedAt   time.Time                      `json:"attachedAt"`

	Metadata map[string]any `json:"metadata,omitempty"`
}

type RuntimeSessionChatRelationship string

const (
	RuntimeSessionChatPrimary           RuntimeSessionChatRelationship = "primary"
	RuntimeSessionChatReferenced        RuntimeSessionChatRelationship = "referenced"
	RuntimeSessionChatBranched          RuntimeSessionChatRelationship = "branched"
	RuntimeSessionChatBackgroundContext RuntimeSessionChatRelationship = "background_context"
)
