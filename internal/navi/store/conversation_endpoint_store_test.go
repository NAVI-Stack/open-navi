package store

import (
	"context"
	"errors"
	"testing"

	"github.com/ceoai/navi/internal/navi"
)

func TestSQLiteStoreEnsureConsoleEndpointIsStable(t *testing.T) {
	ctx := context.Background()
	db := prepareTestDB(t)
	store := NewSQLiteStore(db)

	chat, err := store.CreateChat(ctx, navi.CreateChatInput{ID: "chat-console", Title: "Console"})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}

	first, err := store.EnsureConsoleEndpoint(ctx, string(chat.ID))
	if err != nil {
		t.Fatalf("EnsureConsoleEndpoint first: %v", err)
	}
	second, err := store.EnsureConsoleEndpoint(ctx, string(chat.ID))
	if err != nil {
		t.Fatalf("EnsureConsoleEndpoint second: %v", err)
	}

	if first.ID == "" || first.ID != second.ID {
		t.Fatalf("expected stable console endpoint id, got first=%q second=%q", first.ID, second.ID)
	}
	if first.Type != navi.EndpointTypeConsole ||
		first.ConnectorKind != "console" ||
		first.ConnectorInstanceID != "console" ||
		first.ExternalChatID != string(chat.ID) ||
		!first.ReceiveEnabled ||
		!first.SendEnabled ||
		first.MirrorEnabled ||
		first.Status != navi.EndpointStatusActive {
		t.Fatalf("unexpected console endpoint: %#v", first)
	}
}

func TestSQLiteStoreUpsertAndResolveConnectorEndpointIncludesDisabled(t *testing.T) {
	ctx := context.Background()
	db := prepareTestDB(t)
	store := NewSQLiteStore(db)

	chat, err := store.CreateChat(ctx, navi.CreateChatInput{ID: "chat-telegram", Title: "Telegram"})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}

	created, err := store.UpsertConnectorEndpoint(ctx, navi.UpsertConnectorEndpointInput{
		ChatID:              string(chat.ID),
		ConnectorKind:       "telegram",
		ConnectorInstanceID: "telegram-primary",
		ExternalChatID:      "external-chat-1",
		ExternalThreadID:    "thread-1",
		DisplayName:         "NAVI Telegram",
	})
	if err != nil {
		t.Fatalf("UpsertConnectorEndpoint create: %v", err)
	}
	if created.ID == "" || created.Type != navi.EndpointTypeConnector {
		t.Fatalf("unexpected created endpoint: %#v", created)
	}

	receiveOff := false
	status := navi.EndpointStatusDisabled
	updated, err := store.UpdateConversationEndpoint(ctx, string(created.ID), navi.UpdateConversationEndpointInput{
		ReceiveEnabled: &receiveOff,
		Status:         &status,
	})
	if err != nil {
		t.Fatalf("UpdateConversationEndpoint: %v", err)
	}
	if updated.ReceiveEnabled || updated.Status != navi.EndpointStatusDisabled {
		t.Fatalf("expected disabled receive endpoint, got %#v", updated)
	}

	resolved, err := store.ResolveConnectorEndpoint(ctx, "telegram-primary", "external-chat-1", "thread-1")
	if err != nil {
		t.Fatalf("ResolveConnectorEndpoint disabled: %v", err)
	}
	if resolved.ID != created.ID || resolved.ReceiveEnabled || resolved.Status != navi.EndpointStatusDisabled {
		t.Fatalf("expected disabled endpoint to resolve for policy handling, got %#v", resolved)
	}

	reupserted, err := store.UpsertConnectorEndpoint(ctx, navi.UpsertConnectorEndpointInput{
		ChatID:              string(chat.ID),
		ConnectorKind:       "telegram",
		ConnectorInstanceID: "telegram-primary",
		ExternalChatID:      "external-chat-1",
		ExternalThreadID:    "thread-1",
		DisplayName:         "Renamed",
	})
	if err != nil {
		t.Fatalf("UpsertConnectorEndpoint update disabled: %v", err)
	}
	if reupserted.ID != created.ID || reupserted.DisplayName != "Renamed" {
		t.Fatalf("expected address upsert to update existing endpoint, got %#v", reupserted)
	}
}

func TestSQLiteStoreChatDeliveryPolicyDefaultAndUpdate(t *testing.T) {
	ctx := context.Background()
	db := prepareTestDB(t)
	store := NewSQLiteStore(db)

	if _, err := store.CreateChat(ctx, navi.CreateChatInput{ID: "chat-policy", Title: "Policy"}); err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	defaultPolicy, err := store.GetChatDeliveryPolicy(ctx, "chat-policy")
	if err != nil {
		t.Fatalf("GetChatDeliveryPolicy default: %v", err)
	}
	if defaultPolicy.Mode != navi.DeliveryPolicyReplyToOrigin {
		t.Fatalf("default mode = %q, want %q", defaultPolicy.Mode, navi.DeliveryPolicyReplyToOrigin)
	}

	updated, err := store.SetChatDeliveryPolicy(ctx, "chat-policy", navi.DeliveryPolicyExplicitOnly, nil)
	if err != nil {
		t.Fatalf("SetChatDeliveryPolicy: %v", err)
	}
	if updated.Mode != navi.DeliveryPolicyExplicitOnly {
		t.Fatalf("updated mode = %q, want %q", updated.Mode, navi.DeliveryPolicyExplicitOnly)
	}

	loaded, err := store.GetChatDeliveryPolicy(ctx, "chat-policy")
	if err != nil {
		t.Fatalf("GetChatDeliveryPolicy loaded: %v", err)
	}
	if loaded.Mode != navi.DeliveryPolicyExplicitOnly {
		t.Fatalf("loaded mode = %q, want %q", loaded.Mode, navi.DeliveryPolicyExplicitOnly)
	}
}

func TestSQLiteStoreRecordsAndUpdatesMessageDeliveries(t *testing.T) {
	ctx := context.Background()
	db := prepareTestDB(t)
	store := NewSQLiteStore(db)

	delivery, err := store.RecordMessageDelivery(ctx, navi.RecordMessageDeliveryInput{
		ID:                  "delivery-1",
		MessageID:           "message-1",
		ChatID:              "chat-1",
		EndpointID:          "endpoint-1",
		ConnectorInstanceID: "telegram-primary",
		Status:              navi.MessageDeliveryQueued,
	})
	if err != nil {
		t.Fatalf("RecordMessageDelivery: %v", err)
	}
	if delivery.AttemptCount != 0 || delivery.Status != navi.MessageDeliveryQueued {
		t.Fatalf("unexpected recorded delivery: %#v", delivery)
	}

	updated, err := store.UpdateMessageDelivery(ctx, "delivery-1", navi.UpdateMessageDeliveryInput{
		Status:       navi.MessageDeliveryFailed,
		AttemptDelta: 2,
		LastError:    "telegram timeout",
	})
	if err != nil {
		t.Fatalf("UpdateMessageDelivery: %v", err)
	}
	if updated.AttemptCount != 2 || updated.Status != navi.MessageDeliveryFailed || updated.LastError != "telegram timeout" {
		t.Fatalf("unexpected updated delivery: %#v", updated)
	}

	duplicate, err := store.RecordMessageDelivery(ctx, navi.RecordMessageDeliveryInput{
		ID:                  "delivery-duplicate",
		MessageID:           "message-1",
		ChatID:              "chat-1",
		EndpointID:          "endpoint-1",
		ConnectorInstanceID: "telegram-primary",
		Status:              navi.MessageDeliveryQueued,
	})
	if err != nil {
		t.Fatalf("RecordMessageDelivery duplicate: %v", err)
	}
	if duplicate.ID != "delivery-1" || duplicate.AttemptCount != 2 {
		t.Fatalf("duplicate should return existing delivery, got %#v", duplicate)
	}

	_, err = store.UpdateMessageDelivery(ctx, "missing", navi.UpdateMessageDeliveryInput{Status: navi.MessageDeliverySent})
	if !errors.Is(err, navi.ErrConversationEndpointNotFound) {
		t.Fatalf("missing delivery error = %v, want ErrConversationEndpointNotFound", err)
	}
}
