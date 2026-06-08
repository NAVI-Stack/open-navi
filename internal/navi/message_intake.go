package navi

import (
	"context"
	"fmt"
	"strings"
	"time"

	naviruntime "github.com/open-navi/navi/internal/runtime"
)

type RuntimeMessageSubmitter interface {
	SubmitMessage(ctx context.Context, runtimeSessionID, experienceMode string, input naviruntime.MessageInput) (*naviruntime.InboxItem, error)
}

type MessageIntakeService struct {
	Chats            ChatStore
	RuntimeSessions  RuntimeSessionStore
	Runtime          RuntimeMessageSubmitter
	EndpointResolver *EndpointResolver
}

type SubmitMessageRequest struct {
	ChatID         string
	Content        string
	ExperienceMode string

	OwnerID     string
	ProjectID   string
	WorkspaceID string

	SourceChannel    string
	SourceMessageRef string
	OriginEndpointID string
	IdempotencyKey   string
	Scope            ActiveChatScope
}

type SubmitMessageResult struct {
	Chat             Chat
	RuntimeSession   RuntimeSession
	MessageID        string
	InboxItem        *naviruntime.InboxItem
	OriginEndpointID string
}

func (s MessageIntakeService) SubmitMessage(ctx context.Context, req SubmitMessageRequest) (*SubmitMessageResult, error) {
	if s.Chats == nil {
		return nil, fmt.Errorf("navi: message intake requires chat store")
	}
	if s.RuntimeSessions == nil {
		return nil, fmt.Errorf("navi: message intake requires runtime session store")
	}
	if s.Runtime == nil {
		return nil, fmt.Errorf("navi: message intake requires runtime submitter")
	}

	content := strings.TrimSpace(req.Content)
	if content == "" {
		return nil, fmt.Errorf("navi: message content is required")
	}
	experienceMode := string(NormalizeExperienceMode(ExperienceMode(req.ExperienceMode)))
	sourceChannel := strings.TrimSpace(req.SourceChannel)
	if sourceChannel == "" {
		sourceChannel = "app"
	}

	scope := req.Scope
	if scope.OwnerID == "" {
		scope.OwnerID = req.OwnerID
	}
	if scope.ProjectID == "" {
		scope.ProjectID = req.ProjectID
	}
	if scope.WorkspaceID == "" {
		scope.WorkspaceID = req.WorkspaceID
	}
	if scope.SourceChannel == "" {
		scope.SourceChannel = sourceChannel
	}

	chat, err := s.resolveChat(ctx, req, scope, experienceMode)
	if err != nil {
		return nil, err
	}
	originEndpointID, err := s.resolveOriginEndpoint(ctx, chat, req, sourceChannel)
	if err != nil {
		return nil, err
	}

	runtimeSession, err := s.resolveRuntimeSession(ctx, chat, req, sourceChannel, experienceMode)
	if err != nil {
		return nil, err
	}

	runtimeID := ID(runtimeSession.ID)
	msgID, err := s.Chats.AppendChatMessage(ctx, string(chat.ID), ChatMessage{
		ChatID:           chat.ID,
		Role:             "user",
		Content:          content,
		CreatedAt:        time.Now().UTC(),
		RuntimeSessionID: &runtimeID,
		SourceChannel:    sourceChannel,
		SourceMessageRef: req.SourceMessageRef,
		OriginEndpointID: chatStringToIDPtr(originEndpointID),
	})
	if err != nil {
		return nil, err
	}

	_ = s.Chats.SetActiveChat(ctx, scope, string(chat.ID))

	inboxItem, err := s.Runtime.SubmitMessage(ctx, string(runtimeSession.ID), experienceMode, naviruntime.MessageInput{
		Content:          content,
		ChatID:           string(chat.ID),
		RuntimeSessionID: string(runtimeSession.ID),
		MessageID:        msgID,
		SourceChannel:    sourceChannel,
		SourceMessageRef: req.SourceMessageRef,
		OriginEndpointID: originEndpointID,
		IdempotencyKey:   req.IdempotencyKey,
	})
	if err != nil {
		return nil, err
	}

	return &SubmitMessageResult{
		Chat:             *chat,
		RuntimeSession:   *runtimeSession,
		MessageID:        msgID,
		InboxItem:        inboxItem,
		OriginEndpointID: originEndpointID,
	}, nil
}

func (s MessageIntakeService) resolveOriginEndpoint(ctx context.Context, chat *Chat, req SubmitMessageRequest, sourceChannel string) (string, error) {
	if explicit := strings.TrimSpace(req.OriginEndpointID); explicit != "" {
		return explicit, nil
	}
	if s.EndpointResolver == nil || chat == nil {
		return "", nil
	}
	if isConnectorIntakeChannel(sourceChannel) {
		return "", nil
	}
	origin, err := s.EndpointResolver.ResolveConsoleOrigin(ctx, string(chat.ID))
	if err != nil {
		return "", err
	}
	return origin.EndpointID, nil
}

func (s MessageIntakeService) resolveChat(ctx context.Context, req SubmitMessageRequest, scope ActiveChatScope, experienceMode string) (*Chat, error) {
	chatID := strings.TrimSpace(req.ChatID)
	if chatID == "" {
		if active, err := s.Chats.ActiveChatID(ctx, scope); err != nil {
			return nil, err
		} else if strings.TrimSpace(active) != "" {
			chatID = strings.TrimSpace(active)
		}
	}
	if chatID != "" {
		return s.Chats.GetChat(ctx, chatID)
	}
	title := contentTitle(req.Content)
	chat, err := s.Chats.CreateChat(ctx, CreateChatInput{
		OwnerID:        req.OwnerID,
		WorkspaceID:    req.WorkspaceID,
		ProjectID:      req.ProjectID,
		Title:          title,
		ExperienceMode: experienceMode,
	})
	if err != nil {
		return nil, err
	}
	return chat, nil
}

func (s MessageIntakeService) resolveRuntimeSession(ctx context.Context, chat *Chat, req SubmitMessageRequest, sourceChannel, experienceMode string) (*RuntimeSession, error) {
	if chat == nil {
		return nil, fmt.Errorf("navi: message intake requires chat")
	}
	if existing, err := s.RuntimeSessions.FindActiveRuntimeSessionForChat(ctx, string(chat.ID), sourceChannel); err != nil {
		return nil, err
	} else if existing != nil {
		if err := s.RuntimeSessions.TouchRuntimeSession(ctx, string(existing.ID)); err != nil {
			return nil, err
		}
		return existing, nil
	}

	runtimeSession, err := s.RuntimeSessions.CreateRuntimeSession(ctx, CreateRuntimeSessionInput{
		OwnerID:        firstNonEmptyString(req.OwnerID, string(chat.OwnerID)),
		WorkspaceID:    firstNonEmptyString(req.WorkspaceID, idPtrToString(chat.WorkspaceID)),
		ProjectID:      firstNonEmptyString(req.ProjectID, idPtrToString(chat.ProjectID)),
		Kind:           RuntimeSessionKindUser,
		ExperienceMode: experienceMode,
		SourceChannel:  sourceChannel,
	})
	if err != nil {
		return nil, err
	}
	if err := s.RuntimeSessions.AttachChatToRuntimeSession(ctx, string(runtimeSession.ID), string(chat.ID), RuntimeSessionChatPrimary); err != nil {
		return nil, err
	}
	return runtimeSession, nil
}

func contentTitle(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return "New Chat"
	}
	content = strings.ReplaceAll(content, "\n", " ")
	if len(content) > 60 {
		return strings.TrimSpace(content[:60])
	}
	return content
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func isConnectorIntakeChannel(source string) bool {
	source = strings.ToLower(strings.TrimSpace(source))
	switch source {
	case "telegram", "discord", "slack":
		return true
	default:
		return strings.HasPrefix(source, "telegram-") ||
			strings.HasPrefix(source, "discord-") ||
			strings.HasPrefix(source, "slack-")
	}
}

func chatStringToIDPtr(value string) *ID {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	id := ID(value)
	return &id
}
