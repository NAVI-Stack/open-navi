package navi

import "time"

type ID string

type Chat struct {
	ID             ID  `json:"id"`
	OwnerID        ID  `json:"ownerId"`
	WorkspaceID    *ID `json:"workspaceId,omitempty"`
	ProjectID      *ID `json:"projectId,omitempty"`
	OrganizationID *ID `json:"organizationId,omitempty"`

	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Color       string `json:"color,omitempty"`

	Status     ChatStatus     `json:"status"`
	Visibility ChatVisibility `json:"visibility"`
	IsPinned   bool           `json:"isPinned"`
	IsFavorite bool           `json:"isFavorite,omitempty"`

	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	LastMessageAt *time.Time `json:"lastMessageAt,omitempty"`
	ArchivedAt    *time.Time `json:"archivedAt,omitempty"`
	DeletedAt     *time.Time `json:"deletedAt,omitempty"`

	// Branching and lineage.
	RootChatID            *ID    `json:"rootChatId,omitempty"`
	ParentChatID          *ID    `json:"parentChatId,omitempty"`
	BranchedFromMessageID *ID    `json:"branchedFromMessageId,omitempty"`
	BranchReason          string `json:"branchReason,omitempty"`

	Participants []ChatParticipant `json:"participants,omitempty"`

	AIConfig ChatAIConfig `json:"aiConfig,omitempty"`

	Summary      string        `json:"summary,omitempty"`
	ShortSummary string        `json:"shortSummary,omitempty"`
	Topics       []string      `json:"topics,omitempty"`
	Tags         []string      `json:"tags,omitempty"`
	Intent       string        `json:"intent,omitempty"`
	Sentiment    ChatSentiment `json:"sentiment,omitempty"`
	Language     string        `json:"language,omitempty"`

	LinkedArtifacts []LinkedArtifact `json:"linkedArtifacts,omitempty"`
	LinkedFiles     []LinkedFile     `json:"linkedFiles,omitempty"`
	LinkedProjects  []LinkedProject  `json:"linkedProjects,omitempty"`
	LinkedTasks     []LinkedTask     `json:"linkedTasks,omitempty"`

	MemoryPolicy   *ChatMemoryPolicy   `json:"memoryPolicy,omitempty"`
	ContextSources []ChatContextSource `json:"contextSources,omitempty"`

	MessageCount          int                `json:"messageCount"`
	UserMessageCount      int                `json:"userMessageCount,omitempty"`
	AssistantMessageCount int                `json:"assistantMessageCount,omitempty"`
	TokenUsage            *TokenUsageSummary `json:"tokenUsage,omitempty"`

	ModerationState ChatModerationState `json:"moderationState,omitempty"`
	ModerationFlags []string            `json:"moderationFlags,omitempty"`

	Metadata map[string]any `json:"metadata,omitempty"`
}

type ChatStatus string

const (
	ChatStatusActive   ChatStatus = "active"
	ChatStatusArchived ChatStatus = "archived"
	ChatStatusDeleted  ChatStatus = "deleted"
)

type ChatVisibility string

const (
	ChatVisibilityPrivate ChatVisibility = "private"
	ChatVisibilityShared  ChatVisibility = "shared"
	ChatVisibilityPublic  ChatVisibility = "public"
)

type ChatSentiment string

const (
	ChatSentimentPositive ChatSentiment = "positive"
	ChatSentimentNeutral  ChatSentiment = "neutral"
	ChatSentimentNegative ChatSentiment = "negative"
	ChatSentimentMixed    ChatSentiment = "mixed"
)

type ChatModerationState string

const (
	ChatModerationClean       ChatModerationState = "clean"
	ChatModerationFlagged     ChatModerationState = "flagged"
	ChatModerationRestricted  ChatModerationState = "restricted"
	ChatModerationNeedsReview ChatModerationState = "needs_review"
)

type ChatParticipant struct {
	ID          ID                  `json:"id"`
	Type        ChatParticipantType `json:"type"`
	Role        ChatParticipantRole `json:"role"`
	DisplayName string              `json:"displayName,omitempty"`
	AvatarURL   string              `json:"avatarUrl,omitempty"`
	JoinedAt    *time.Time          `json:"joinedAt,omitempty"`
}

type ChatParticipantType string

const (
	ChatParticipantUser      ChatParticipantType = "user"
	ChatParticipantAssistant ChatParticipantType = "assistant"
	ChatParticipantSystem    ChatParticipantType = "system"
	ChatParticipantAgent     ChatParticipantType = "agent"
	ChatParticipantBot       ChatParticipantType = "bot"
)

type ChatParticipantRole string

const (
	ChatRoleOwner     ChatParticipantRole = "owner"
	ChatRoleMember    ChatParticipantRole = "member"
	ChatRoleViewer    ChatParticipantRole = "viewer"
	ChatRoleAssistant ChatParticipantRole = "assistant"
	ChatRoleSystem    ChatParticipantRole = "system"
)

type ChatAIConfig struct {
	Provider            AIProvider `json:"provider,omitempty"`
	Model               string     `json:"model,omitempty"`
	SystemPromptID      *ID        `json:"systemPromptId,omitempty"`
	SystemPromptVersion string     `json:"systemPromptVersion,omitempty"`
	PersonaID           *ID        `json:"personaId,omitempty"`

	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   *int     `json:"maxTokens,omitempty"`

	ToolsEnabled []string       `json:"toolsEnabled,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type AIProvider string

const (
	AIProviderOpenAI    AIProvider = "openai"
	AIProviderAnthropic AIProvider = "anthropic"
	AIProviderGoogle    AIProvider = "google"
	AIProviderLocal     AIProvider = "local"
	AIProviderCustom    AIProvider = "custom"
)

type LinkedArtifact struct {
	ID        ID                 `json:"id"`
	Type      LinkedArtifactType `json:"type"`
	Title     string             `json:"title,omitempty"`
	URL       string             `json:"url,omitempty"`
	CreatedAt *time.Time         `json:"createdAt,omitempty"`
}

type LinkedArtifactType string

const (
	LinkedArtifactDocument    LinkedArtifactType = "document"
	LinkedArtifactCode        LinkedArtifactType = "code"
	LinkedArtifactImage       LinkedArtifactType = "image"
	LinkedArtifactSpreadsheet LinkedArtifactType = "spreadsheet"
	LinkedArtifactSlide       LinkedArtifactType = "slide"
	LinkedArtifactCanvas      LinkedArtifactType = "canvas"
	LinkedArtifactWorkflow    LinkedArtifactType = "workflow"
)

type LinkedFile struct {
	ID         ID     `json:"id"`
	Name       string `json:"name"`
	MimeType   string `json:"mimeType,omitempty"`
	URL        string `json:"url,omitempty"`
	StorageKey string `json:"storageKey,omitempty"`
	SizeBytes  int64  `json:"sizeBytes,omitempty"`
	UploadedBy *ID    `json:"uploadedBy,omitempty"`
}

type LinkedProject struct {
	ID   ID                `json:"id"`
	Name string            `json:"name"`
	Role LinkedProjectRole `json:"role,omitempty"`
}

type LinkedProjectRole string

const (
	LinkedProjectPrimary LinkedProjectRole = "primary"
	LinkedProjectRelated LinkedProjectRole = "related"
)

type LinkedTask struct {
	ID       ID     `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status,omitempty"`
	URL      string `json:"url,omitempty"`
	System   string `json:"system,omitempty"`
	Assignee *ID    `json:"assignee,omitempty"`
}

type ChatMemoryPolicy struct {
	AllowMemoryWrite bool        `json:"allowMemoryWrite"`
	AllowMemoryRead  bool        `json:"allowMemoryRead"`
	MemoryScope      MemoryScope `json:"memoryScope"`
}

type MemoryScope string

const (
	MemoryScopeNone      MemoryScope = "none"
	MemoryScopeChat      MemoryScope = "chat"
	MemoryScopeUser      MemoryScope = "user"
	MemoryScopeProject   MemoryScope = "project"
	MemoryScopeWorkspace MemoryScope = "workspace"
)

type ChatContextSource struct {
	Type    ChatContextSourceType `json:"type"`
	ID      *ID                   `json:"id,omitempty"`
	Name    string                `json:"name,omitempty"`
	Enabled bool                  `json:"enabled"`
}

type ChatContextSourceType string

const (
	ContextSourceFile      ChatContextSourceType = "file"
	ContextSourceProject   ChatContextSourceType = "project"
	ContextSourceWorkspace ChatContextSourceType = "workspace"
	ContextSourceWeb       ChatContextSourceType = "web"
	ContextSourceDatabase  ChatContextSourceType = "database"
	ContextSourceTool      ChatContextSourceType = "tool"
)

type TokenUsageSummary struct {
	InputTokens      int     `json:"inputTokens,omitempty"`
	OutputTokens     int     `json:"outputTokens,omitempty"`
	TotalTokens      int     `json:"totalTokens,omitempty"`
	EstimatedCostUSD float64 `json:"estimatedCostUsd,omitempty"`
}
