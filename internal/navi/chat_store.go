package navi

import "context"

type CreateChatInput struct {
	ID             string
	OwnerID        string
	WorkspaceID    string
	ProjectID      string
	OrganizationID string

	Title       string
	Description string
	Icon        string
	Color       string

	Status     ChatStatus
	Visibility ChatVisibility
	IsPinned   bool
	IsFavorite bool

	ExperienceMode string
	AIConfig       ChatAIConfig

	RootChatID            string
	ParentChatID          string
	BranchedFromMessageID string
	BranchReason          string

	MemoryPolicy   *ChatMemoryPolicy
	ContextSources []ChatContextSource

	Metadata map[string]any
}

type ChatThread struct {
	Chat     Chat          `json:"chat"`
	Messages []ChatMessage `json:"messages,omitempty"`
}

type ActiveChatScope struct {
	OwnerID       string
	ProjectID     string
	WorkspaceID   string
	SourceChannel string
}

type ChatStore interface {
	CreateChat(ctx context.Context, input CreateChatInput) (*Chat, error)
	RenameChat(ctx context.Context, chatID, title string) error
	GetChat(ctx context.Context, chatID string) (*Chat, error)
	GetChatWithMessages(ctx context.Context, chatID string) (*ChatThread, error)
	ListChats(ctx context.Context, limit int) ([]Chat, error)
	ListChatsByProject(ctx context.Context, projectID string, limit int) ([]Chat, error)
	UpdateChat(ctx context.Context, chat Chat) error
	ArchiveChat(ctx context.Context, chatID string) error
	DeleteChat(ctx context.Context, chatID string) error

	AppendChatMessage(ctx context.Context, chatID string, msg ChatMessage) (string, error)
	AppendSystemAssistantMessage(ctx context.Context, chatID, content, experienceMode, sourceChannel, kind string) (string, error)
	ListChatMessages(ctx context.Context, chatID string, limit, offset int) ([]ChatMessage, error)
	MessageCount(ctx context.Context, chatID string) (int, error)

	ActiveChatID(ctx context.Context, scope ActiveChatScope) (string, error)
	SetActiveChat(ctx context.Context, scope ActiveChatScope, chatID string) error
}
