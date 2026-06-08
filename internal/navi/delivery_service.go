package navi

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/open-navi/navi/connectors"
	naviruntime "github.com/open-navi/navi/internal/runtime"
)

type ConnectorDispatcher interface {
	Dispatch(ctx context.Context, connectorInstanceID string, msg connectors.OutboundMessage) error
}

type DeliveryService struct {
	Endpoints  ConversationEndpointStore
	Dispatcher ConnectorDispatcher
}

type AssistantDeliveryRequest struct {
	ChatID              string
	MessageID           string
	Content             string
	OriginEndpointID    string
	ExplicitEndpointIDs []string
	RuntimeSessionID    string
	RunID               string
	CorrelationID       string
	SourceMessageRef    string
}

func NewAssistantDeliveryObserver(service *DeliveryService) naviruntime.AssistantMessageObserver {
	if service == nil {
		return nil
	}
	return func(ctx context.Context, event naviruntime.AssistantMessageEvent) error {
		if event.Run == nil {
			return nil
		}
		_, err := service.DeliverAssistantMessage(ctx, AssistantDeliveryRequest{
			ChatID:           event.Run.ChatID,
			MessageID:        event.MessageID,
			Content:          event.Content,
			OriginEndpointID: event.Run.OriginEndpointID,
			RuntimeSessionID: event.Run.RuntimeSessionID,
			RunID:            event.Run.RunID,
			CorrelationID:    firstNonEmptyString(event.Run.ChatID, event.Run.RuntimeSessionID),
		})
		return err
	}
}

func (s DeliveryService) DeliverAssistantMessage(ctx context.Context, req AssistantDeliveryRequest) ([]MessageDelivery, error) {
	if s.Endpoints == nil {
		return nil, fmt.Errorf("navi: delivery service requires endpoint store")
	}
	chatID := strings.TrimSpace(req.ChatID)
	messageID := strings.TrimSpace(req.MessageID)
	content := req.Content
	if chatID == "" || messageID == "" {
		return nil, fmt.Errorf("navi: deliver assistant message requires chat id and message id")
	}

	policy, err := s.Endpoints.GetChatDeliveryPolicy(ctx, chatID)
	if err != nil {
		return nil, err
	}
	explicitIDs := normalizeEndpointIDs(req.ExplicitEndpointIDs)
	if policy.Mode == DeliveryPolicyExplicitOnly && len(explicitIDs) == 0 {
		return nil, nil
	}

	targets, err := s.resolveTargets(ctx, chatID, strings.TrimSpace(req.OriginEndpointID), explicitIDs)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, nil
	}
	if s.Dispatcher == nil {
		return nil, fmt.Errorf("navi: delivery service requires connector dispatcher")
	}

	deliveries := make([]MessageDelivery, 0, len(targets))
	for _, target := range targets {
		delivery, err := s.Endpoints.RecordMessageDelivery(ctx, RecordMessageDeliveryInput{
			MessageID:           messageID,
			ChatID:              chatID,
			EndpointID:          string(target.ID),
			ConnectorInstanceID: target.ConnectorInstanceID,
			Status:              MessageDeliveryQueued,
		})
		if err != nil {
			return nil, err
		}
		msg := connectors.OutboundMessage{
			Channel:             target.ConnectorKind,
			ChatID:              target.ExternalChatID,
			Content:             content,
			MessageID:           messageID,
			EndpointID:          string(target.ID),
			DeliveryID:          string(delivery.ID),
			ConnectorInstanceID: target.ConnectorInstanceID,
			ExternalThreadID:    target.ExternalThreadID,
			RuntimeSessionID:    req.RuntimeSessionID,
			RunID:               req.RunID,
			CorrelationID:       firstNonEmptyString(req.CorrelationID, chatID),
			SourceMessageRef:    req.SourceMessageRef,
			MessageThreadID:     endpointThreadID(target.ExternalThreadID),
		}
		if err := s.Dispatcher.Dispatch(ctx, target.ConnectorInstanceID, msg); err != nil {
			updated, updateErr := s.Endpoints.UpdateMessageDelivery(ctx, string(delivery.ID), UpdateMessageDeliveryInput{
				Status:       MessageDeliveryFailed,
				AttemptDelta: 1,
				LastError:    err.Error(),
			})
			if updateErr != nil {
				return nil, updateErr
			}
			deliveries = append(deliveries, *updated)
			continue
		}
		deliveries = append(deliveries, *delivery)
	}
	return deliveries, nil
}

func (s DeliveryService) resolveTargets(ctx context.Context, chatID, originEndpointID string, explicitIDs []string) ([]ConversationEndpoint, error) {
	seen := map[string]struct{}{}
	var targets []ConversationEndpoint
	add := func(endpoint *ConversationEndpoint) error {
		if endpoint == nil {
			return nil
		}
		if string(endpoint.ChatID) != chatID {
			return ErrConversationEndpointChatMismatch
		}
		if !isDispatchableConnectorEndpoint(*endpoint) {
			return nil
		}
		id := string(endpoint.ID)
		if _, ok := seen[id]; ok {
			return nil
		}
		seen[id] = struct{}{}
		targets = append(targets, *endpoint)
		return nil
	}

	if originEndpointID != "" {
		endpoint, err := s.Endpoints.GetConversationEndpoint(ctx, originEndpointID)
		if err != nil {
			return nil, err
		}
		if err := add(endpoint); err != nil {
			return nil, err
		}
	}

	endpoints, err := s.Endpoints.ListConversationEndpoints(ctx, chatID)
	if err != nil {
		return nil, err
	}
	for i := range endpoints {
		if endpoints[i].MirrorEnabled {
			if err := add(&endpoints[i]); err != nil {
				return nil, err
			}
		}
	}

	for _, endpointID := range explicitIDs {
		endpoint, err := s.Endpoints.GetConversationEndpoint(ctx, endpointID)
		if err != nil {
			return nil, err
		}
		if err := add(endpoint); err != nil {
			return nil, err
		}
	}

	return targets, nil
}

func isDispatchableConnectorEndpoint(endpoint ConversationEndpoint) bool {
	return endpoint.Type == EndpointTypeConnector &&
		endpoint.Status == EndpointStatusActive &&
		endpoint.SendEnabled
}

func normalizeEndpointIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := map[string]struct{}{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func endpointThreadID(externalThreadID string) int64 {
	externalThreadID = strings.TrimSpace(externalThreadID)
	if externalThreadID == "" {
		return 0
	}
	id, err := strconv.ParseInt(externalThreadID, 10, 64)
	if err != nil {
		return 0
	}
	return id
}
