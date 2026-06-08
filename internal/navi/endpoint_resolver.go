package navi

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type EndpointResolver struct {
	Chats     ChatStore
	Endpoints ConversationEndpointStore
}

type EndpointOrigin struct {
	ChatID     string               `json:"chat_id"`
	EndpointID string               `json:"endpoint_id"`
	Endpoint   ConversationEndpoint `json:"endpoint"`
}

type ConnectorEndpointOriginInput struct {
	ConnectorKind       string
	ConnectorInstanceID string
	ExternalChatID      string
	ExternalThreadID    string
	DisplayName         string
	OwnerID             string
	WorkspaceID         string
	ProjectID           string
}

func (r EndpointResolver) ResolveConsoleOrigin(ctx context.Context, chatID string) (*EndpointOrigin, error) {
	if r.Endpoints == nil {
		return nil, fmt.Errorf("navi: endpoint resolver requires endpoint store")
	}
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return nil, fmt.Errorf("navi: resolve console origin: chat id is required")
	}
	endpoint, err := r.Endpoints.EnsureConsoleEndpoint(ctx, chatID)
	if err != nil {
		return nil, err
	}
	return &EndpointOrigin{
		ChatID:     string(endpoint.ChatID),
		EndpointID: string(endpoint.ID),
		Endpoint:   *endpoint,
	}, nil
}

func (r EndpointResolver) ResolveConnectorOrigin(ctx context.Context, input ConnectorEndpointOriginInput) (*EndpointOrigin, error) {
	if r.Chats == nil {
		return nil, fmt.Errorf("navi: endpoint resolver requires chat store")
	}
	if r.Endpoints == nil {
		return nil, fmt.Errorf("navi: endpoint resolver requires endpoint store")
	}
	connectorKind := strings.TrimSpace(input.ConnectorKind)
	connectorInstanceID := strings.TrimSpace(input.ConnectorInstanceID)
	externalChatID := strings.TrimSpace(input.ExternalChatID)
	externalThreadID := strings.TrimSpace(input.ExternalThreadID)
	if connectorKind == "" || connectorInstanceID == "" || externalChatID == "" {
		return nil, fmt.Errorf("navi: resolve connector origin: connector kind, connector instance id, and external chat id are required")
	}

	endpoint, err := r.Endpoints.ResolveConnectorEndpoint(ctx, connectorInstanceID, externalChatID, externalThreadID)
	if err == nil {
		if endpoint.Status != EndpointStatusActive || !endpoint.ReceiveEnabled {
			return nil, ErrConversationEndpointReceiveDisabled
		}
		return &EndpointOrigin{
			ChatID:     string(endpoint.ChatID),
			EndpointID: string(endpoint.ID),
			Endpoint:   *endpoint,
		}, nil
	}
	if !errors.Is(err, ErrConversationEndpointNotFound) {
		return nil, err
	}

	chatTitle := strings.TrimSpace(input.DisplayName)
	if chatTitle == "" {
		chatTitle = strings.TrimSpace(externalChatID)
	}
	if chatTitle == "" {
		chatTitle = "Connector Chat"
	}
	chat, err := r.Chats.CreateChat(ctx, CreateChatInput{
		OwnerID:     strings.TrimSpace(input.OwnerID),
		WorkspaceID: strings.TrimSpace(input.WorkspaceID),
		ProjectID:   strings.TrimSpace(input.ProjectID),
		Title:       chatTitle,
	})
	if err != nil {
		return nil, err
	}
	if _, err := r.Endpoints.EnsureConsoleEndpoint(ctx, string(chat.ID)); err != nil {
		return nil, err
	}
	endpoint, err = r.Endpoints.UpsertConnectorEndpoint(ctx, UpsertConnectorEndpointInput{
		ChatID:              string(chat.ID),
		ConnectorKind:       connectorKind,
		ConnectorInstanceID: connectorInstanceID,
		ExternalChatID:      externalChatID,
		ExternalThreadID:    externalThreadID,
		DisplayName:         strings.TrimSpace(input.DisplayName),
	})
	if err != nil {
		return nil, err
	}
	return &EndpointOrigin{
		ChatID:     string(endpoint.ChatID),
		EndpointID: string(endpoint.ID),
		Endpoint:   *endpoint,
	}, nil
}
