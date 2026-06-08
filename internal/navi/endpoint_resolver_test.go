package navi_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ceoai/navi/internal/navi"
	navistore "github.com/ceoai/navi/internal/navi/store"
	corestore "github.com/ceoai/navi/internal/store"
)

func TestEndpointResolverConsoleOriginEnsuresConsoleEndpoint(t *testing.T) {
	ctx := context.Background()
	store := newEndpointResolverTestStore(t)
	chat, err := store.CreateChat(ctx, navi.CreateChatInput{ID: "chat-console-origin", Title: "Console Origin"})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}

	resolver := navi.EndpointResolver{Chats: store, Endpoints: store}
	origin, err := resolver.ResolveConsoleOrigin(ctx, string(chat.ID))
	if err != nil {
		t.Fatalf("ResolveConsoleOrigin: %v", err)
	}
	if origin.ChatID != string(chat.ID) || origin.EndpointID == "" {
		t.Fatalf("unexpected console origin: %#v", origin)
	}
	if origin.Endpoint.Type != navi.EndpointTypeConsole {
		t.Fatalf("origin endpoint type = %q, want console", origin.Endpoint.Type)
	}
}

func TestEndpointResolverConnectorOriginCreatesChatAndEndpoints(t *testing.T) {
	ctx := context.Background()
	store := newEndpointResolverTestStore(t)
	resolver := navi.EndpointResolver{Chats: store, Endpoints: store}

	origin, err := resolver.ResolveConnectorOrigin(ctx, navi.ConnectorEndpointOriginInput{
		ConnectorKind:       "telegram",
		ConnectorInstanceID: "telegram-primary",
		ExternalChatID:      "external-chat-2",
		ExternalThreadID:    "thread-2",
		DisplayName:         "Telegram DM",
		OwnerID:             "owner-1",
	})
	if err != nil {
		t.Fatalf("ResolveConnectorOrigin: %v", err)
	}
	if origin.ChatID == "" || origin.EndpointID == "" {
		t.Fatalf("expected chat and endpoint ids, got %#v", origin)
	}
	if origin.Endpoint.Type != navi.EndpointTypeConnector ||
		origin.Endpoint.ConnectorKind != "telegram" ||
		origin.Endpoint.ConnectorInstanceID != "telegram-primary" ||
		origin.Endpoint.ExternalChatID != "external-chat-2" {
		t.Fatalf("unexpected connector endpoint: %#v", origin.Endpoint)
	}
	endpoints, err := store.ListConversationEndpoints(ctx, origin.ChatID)
	if err != nil {
		t.Fatalf("ListConversationEndpoints: %v", err)
	}
	if len(endpoints) != 2 {
		t.Fatalf("expected console and connector endpoints, got %#v", endpoints)
	}
}

func TestEndpointResolverDisabledConnectorOriginDoesNotFallback(t *testing.T) {
	ctx := context.Background()
	store := newEndpointResolverTestStore(t)
	chat, err := store.CreateChat(ctx, navi.CreateChatInput{ID: "chat-disabled-origin", Title: "Disabled"})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	created, err := store.UpsertConnectorEndpoint(ctx, navi.UpsertConnectorEndpointInput{
		ChatID:              string(chat.ID),
		ConnectorKind:       "telegram",
		ConnectorInstanceID: "telegram-primary",
		ExternalChatID:      "external-disabled",
	})
	if err != nil {
		t.Fatalf("UpsertConnectorEndpoint: %v", err)
	}
	receiveOff := false
	status := navi.EndpointStatusDisabled
	if _, err := store.UpdateConversationEndpoint(ctx, string(created.ID), navi.UpdateConversationEndpointInput{
		ReceiveEnabled: &receiveOff,
		Status:         &status,
	}); err != nil {
		t.Fatalf("UpdateConversationEndpoint: %v", err)
	}

	resolver := navi.EndpointResolver{Chats: store, Endpoints: store}
	_, err = resolver.ResolveConnectorOrigin(ctx, navi.ConnectorEndpointOriginInput{
		ConnectorKind:       "telegram",
		ConnectorInstanceID: "telegram-primary",
		ExternalChatID:      "external-disabled",
	})
	if !errors.Is(err, navi.ErrConversationEndpointReceiveDisabled) {
		t.Fatalf("ResolveConnectorOrigin error = %v, want ErrConversationEndpointReceiveDisabled", err)
	}

	endpoints, err := store.ListConversationEndpoints(ctx, string(chat.ID))
	if err != nil {
		t.Fatalf("ListConversationEndpoints: %v", err)
	}
	if len(endpoints) != 1 || endpoints[0].ID != created.ID {
		t.Fatalf("disabled endpoint should not be bypassed or replaced, got %#v", endpoints)
	}
}

func newEndpointResolverTestStore(t *testing.T) *navistore.SQLiteStore {
	t.Helper()
	ctx := context.Background()
	db, err := corestore.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := corestore.CreateTables(ctx, db); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	if err := navistore.MigrateSchema(ctx, db); err != nil {
		t.Fatalf("MigrateSchema: %v", err)
	}
	return navistore.NewSQLiteStore(db)
}
