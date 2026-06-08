package navi_test

import (
	"context"
	"testing"

	"github.com/open-navi/navi/internal/navi"
	naviruntime "github.com/open-navi/navi/internal/runtime"
)

func TestAssistantDeliveryObserverRoutesRunOriginThroughDeliveryService(t *testing.T) {
	ctx := context.Background()
	store := newEndpointResolverTestStore(t)
	chat, err := store.CreateChat(ctx, navi.CreateChatInput{ID: "chat-observer-delivery", Title: "Observer Delivery"})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	if _, err := store.UpsertConnectorEndpoint(ctx, navi.UpsertConnectorEndpointInput{
		ID:                  "endpoint-observer-delivery",
		ChatID:              string(chat.ID),
		ConnectorKind:       "telegram",
		ConnectorInstanceID: "telegram-primary",
		ExternalChatID:      "external-observer-delivery",
	}); err != nil {
		t.Fatalf("UpsertConnectorEndpoint: %v", err)
	}
	dispatcher := &recordingConnectorDispatcher{}
	service := &navi.DeliveryService{Endpoints: store, Dispatcher: dispatcher}
	observer := navi.NewAssistantDeliveryObserver(service)

	err = observer(ctx, naviruntime.AssistantMessageEvent{
		Run: &naviruntime.RunState{
			RuntimeSessionID:       "session-observer-delivery",
			RunID:                  "run-observer-delivery",
			ChatID:                 string(chat.ID),
			OriginEndpointID:       "endpoint-observer-delivery",
			ExperienceMode:         "navi",
			InitiatedByInboxItemID: "inbox-observer-delivery",
		},
		MessageID:   "message-observer-delivery",
		Content:     "reply through origin",
		InboxItemID: "inbox-observer-delivery",
	})
	if err != nil {
		t.Fatalf("observer: %v", err)
	}
	if len(dispatcher.calls) != 1 {
		t.Fatalf("expected one connector dispatch, got %#v", dispatcher.calls)
	}
	if dispatcher.calls[0].msg.RunID != "run-observer-delivery" ||
		dispatcher.calls[0].msg.RuntimeSessionID != "session-observer-delivery" ||
		dispatcher.calls[0].msg.EndpointID != "endpoint-observer-delivery" {
		t.Fatalf("unexpected dispatch context: %#v", dispatcher.calls[0])
	}
}
