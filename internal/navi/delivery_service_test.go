package navi_test

import (
	"context"
	"testing"

	"github.com/ceoai/navi/connectors"
	"github.com/ceoai/navi/internal/navi"
)

func TestDeliveryServiceExplicitOnlySkipsImplicitConnectorDispatch(t *testing.T) {
	ctx := context.Background()
	store := newEndpointResolverTestStore(t)
	chat, err := store.CreateChat(ctx, navi.CreateChatInput{ID: "chat-explicit-only", Title: "Explicit Only"})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	if _, err := store.UpsertConnectorEndpoint(ctx, navi.UpsertConnectorEndpointInput{
		ID:                  "endpoint-origin",
		ChatID:              string(chat.ID),
		ConnectorKind:       "telegram",
		ConnectorInstanceID: "telegram-primary",
		ExternalChatID:      "external-origin",
	}); err != nil {
		t.Fatalf("UpsertConnectorEndpoint: %v", err)
	}
	if _, err := store.SetChatDeliveryPolicy(ctx, string(chat.ID), navi.DeliveryPolicyExplicitOnly, nil); err != nil {
		t.Fatalf("SetChatDeliveryPolicy: %v", err)
	}

	dispatcher := &recordingConnectorDispatcher{}
	service := navi.DeliveryService{Endpoints: store, Dispatcher: dispatcher}
	deliveries, err := service.DeliverAssistantMessage(ctx, navi.AssistantDeliveryRequest{
		ChatID:           string(chat.ID),
		MessageID:        "message-explicit-only",
		Content:          "hello",
		OriginEndpointID: "endpoint-origin",
	})
	if err != nil {
		t.Fatalf("DeliverAssistantMessage: %v", err)
	}
	if len(deliveries) != 0 {
		t.Fatalf("expected no implicit deliveries in explicit_only mode, got %#v", deliveries)
	}
	if len(dispatcher.calls) != 0 {
		t.Fatalf("expected no connector dispatches, got %#v", dispatcher.calls)
	}
}

func TestDeliveryServiceExplicitOnlyDispatchesNamedEndpoint(t *testing.T) {
	ctx := context.Background()
	store := newEndpointResolverTestStore(t)
	chat, err := store.CreateChat(ctx, navi.CreateChatInput{ID: "chat-explicit-send", Title: "Explicit Send"})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	if _, err := store.UpsertConnectorEndpoint(ctx, navi.UpsertConnectorEndpointInput{
		ID:                  "endpoint-explicit",
		ChatID:              string(chat.ID),
		ConnectorKind:       "telegram",
		ConnectorInstanceID: "telegram-primary",
		ExternalChatID:      "external-explicit",
	}); err != nil {
		t.Fatalf("UpsertConnectorEndpoint: %v", err)
	}
	if _, err := store.SetChatDeliveryPolicy(ctx, string(chat.ID), navi.DeliveryPolicyExplicitOnly, nil); err != nil {
		t.Fatalf("SetChatDeliveryPolicy: %v", err)
	}

	dispatcher := &recordingConnectorDispatcher{}
	service := navi.DeliveryService{Endpoints: store, Dispatcher: dispatcher}
	deliveries, err := service.DeliverAssistantMessage(ctx, navi.AssistantDeliveryRequest{
		ChatID:              string(chat.ID),
		MessageID:           "message-explicit-send",
		Content:             "hello explicit",
		ExplicitEndpointIDs: []string{"endpoint-explicit"},
	})
	if err != nil {
		t.Fatalf("DeliverAssistantMessage: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("expected one explicit delivery, got %#v", deliveries)
	}
	if len(dispatcher.calls) != 1 {
		t.Fatalf("expected one connector dispatch, got %#v", dispatcher.calls)
	}
	if dispatcher.calls[0].instanceID != "telegram-primary" ||
		dispatcher.calls[0].msg.ChatID != "external-explicit" ||
		dispatcher.calls[0].msg.EndpointID != "endpoint-explicit" ||
		dispatcher.calls[0].msg.DeliveryID == "" {
		t.Fatalf("unexpected dispatch call: %#v", dispatcher.calls[0])
	}
}

func TestDeliveryServiceDedupesOriginMirrorAndExplicitTargets(t *testing.T) {
	ctx := context.Background()
	store := newEndpointResolverTestStore(t)
	chat, err := store.CreateChat(ctx, navi.CreateChatInput{ID: "chat-dedupe", Title: "Dedupe"})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	mirrorOn := true
	for _, input := range []navi.UpsertConnectorEndpointInput{
		{
			ID:                  "endpoint-origin",
			ChatID:              string(chat.ID),
			ConnectorKind:       "telegram",
			ConnectorInstanceID: "telegram-primary",
			ExternalChatID:      "external-origin",
			MirrorEnabled:       &mirrorOn,
		},
		{
			ID:                  "endpoint-mirror",
			ChatID:              string(chat.ID),
			ConnectorKind:       "telegram",
			ConnectorInstanceID: "telegram-secondary",
			ExternalChatID:      "external-mirror",
			MirrorEnabled:       &mirrorOn,
		},
	} {
		if _, err := store.UpsertConnectorEndpoint(ctx, input); err != nil {
			t.Fatalf("UpsertConnectorEndpoint(%s): %v", input.ID, err)
		}
	}

	dispatcher := &recordingConnectorDispatcher{}
	service := navi.DeliveryService{Endpoints: store, Dispatcher: dispatcher}
	deliveries, err := service.DeliverAssistantMessage(ctx, navi.AssistantDeliveryRequest{
		ChatID:              string(chat.ID),
		MessageID:           "message-dedupe",
		Content:             "dedupe me",
		OriginEndpointID:    "endpoint-origin",
		ExplicitEndpointIDs: []string{"endpoint-origin", "endpoint-mirror", "endpoint-origin"},
	})
	if err != nil {
		t.Fatalf("DeliverAssistantMessage: %v", err)
	}
	if len(deliveries) != 2 {
		t.Fatalf("expected two unique deliveries, got %#v", deliveries)
	}
	if len(dispatcher.calls) != 2 {
		t.Fatalf("expected two unique dispatches, got %#v", dispatcher.calls)
	}
	seen := map[string]int{}
	for _, call := range dispatcher.calls {
		seen[call.msg.EndpointID]++
	}
	if seen["endpoint-origin"] != 1 || seen["endpoint-mirror"] != 1 {
		t.Fatalf("dispatches were not deduped by endpoint id: %#v", seen)
	}
}

type recordingConnectorDispatcher struct {
	calls []recordedConnectorDispatch
}

type recordedConnectorDispatch struct {
	instanceID string
	msg        connectors.OutboundMessage
}

func (d *recordingConnectorDispatcher) Dispatch(_ context.Context, instanceID string, msg connectors.OutboundMessage) error {
	d.calls = append(d.calls, recordedConnectorDispatch{instanceID: instanceID, msg: msg})
	return nil
}
