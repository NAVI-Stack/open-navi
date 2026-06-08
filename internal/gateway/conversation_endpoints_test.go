package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	pkgconn "github.com/open-navi/navi/connectors"
	"github.com/open-navi/navi/internal/bus"
	"github.com/open-navi/navi/internal/cognitive"
	"github.com/open-navi/navi/internal/governor"
	"github.com/open-navi/navi/internal/navi"
	navistore "github.com/open-navi/navi/internal/navi/store"
	"github.com/open-navi/navi/internal/worldmodel"
)

func TestConversationEndpointGatewayFlow(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	if err := navistore.MigrateSchema(ctx, db); err != nil {
		t.Fatalf("MigrateSchema: %v", err)
	}
	endpointStore := navistore.NewSQLiteStore(db)
	chat, err := endpointStore.CreateChat(ctx, navi.CreateChatInput{ID: "chat-gateway-endpoints", Title: "Gateway Endpoints"})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	dispatcher := &gatewayRecordingDispatcher{}
	srv := NewServer(Config{
		DB:                    db,
		DirectiveWriter:       cognitive.StoreDirectiveWriter(db),
		Bus:                   bus.NewMemBus(db),
		Governor:              governor.NewGovernor(governor.GovernorConfig{}, "."),
		Registry:              NewConnectorRegistry(),
		WorldModel:            worldmodel.New(db),
		ConversationEndpoints: endpointStore,
		ConnectorDispatcher:   dispatcher,
		Addr:                  ":0",
	})

	res := doReq(t, srv, http.MethodGet, "/api/navi/chats/"+string(chat.ID)+"/endpoints", "", "", "127.0.0.1:1234", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET endpoints status = %d", res.StatusCode)
	}
	var listResp struct {
		Endpoints []navi.ConversationEndpoint `json:"endpoints"`
	}
	if err := json.NewDecoder(res.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode endpoints: %v", err)
	}
	if len(listResp.Endpoints) != 1 || listResp.Endpoints[0].Type != navi.EndpointTypeConsole {
		t.Fatalf("expected default console endpoint, got %#v", listResp.Endpoints)
	}

	res = doReq(t, srv, http.MethodPost, "/api/navi/chats/"+string(chat.ID)+"/endpoints", "", "", "127.0.0.1:1234", map[string]any{
		"connector_kind":        "telegram",
		"connector_instance_id": "telegram-primary",
		"external_chat_id":      "external-gateway",
		"display_name":          "Gateway Telegram",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST endpoint status = %d", res.StatusCode)
	}
	var createResp struct {
		Endpoint navi.ConversationEndpoint `json:"endpoint"`
	}
	if err := json.NewDecoder(res.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode created endpoint: %v", err)
	}
	if createResp.Endpoint.ID == "" || createResp.Endpoint.ConnectorInstanceID != "telegram-primary" {
		t.Fatalf("unexpected created endpoint: %#v", createResp.Endpoint)
	}

	res = doReq(t, srv, http.MethodPatch, "/api/navi/chats/"+string(chat.ID)+"/delivery-policy", "", "", "127.0.0.1:1234", map[string]any{
		"default_mode": "explicit_only",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PATCH delivery policy status = %d", res.StatusCode)
	}

	res = doReq(t, srv, http.MethodPost, "/api/navi/chats/"+string(chat.ID)+"/send", "", "", "127.0.0.1:1234", map[string]any{
		"content":      "send explicitly",
		"endpoint_ids": []string{string(createResp.Endpoint.ID), string(createResp.Endpoint.ID)},
		"source":       "console",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST explicit send status = %d", res.StatusCode)
	}
	if len(dispatcher.calls) != 1 {
		t.Fatalf("expected one deduped dispatch, got %#v", dispatcher.calls)
	}
	if dispatcher.calls[0].instanceID != "telegram-primary" || dispatcher.calls[0].msg.ChatID != "external-gateway" {
		t.Fatalf("unexpected dispatch: %#v", dispatcher.calls[0])
	}
}

func TestPatchConversationEndpointRejectsWrongChatBeforeMutating(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	if err := navistore.MigrateSchema(ctx, db); err != nil {
		t.Fatalf("MigrateSchema: %v", err)
	}
	endpointStore := navistore.NewSQLiteStore(db)
	chatA, err := endpointStore.CreateChat(ctx, navi.CreateChatInput{ID: "chat-endpoint-a", Title: "Chat A"})
	if err != nil {
		t.Fatalf("CreateChat A: %v", err)
	}
	chatB, err := endpointStore.CreateChat(ctx, navi.CreateChatInput{ID: "chat-endpoint-b", Title: "Chat B"})
	if err != nil {
		t.Fatalf("CreateChat B: %v", err)
	}
	sendEnabled := true
	endpoint, err := endpointStore.UpsertConnectorEndpoint(ctx, navi.UpsertConnectorEndpointInput{
		ChatID:              string(chatA.ID),
		ConnectorKind:       "telegram",
		ConnectorInstanceID: "telegram-primary",
		ExternalChatID:      "external-a",
		SendEnabled:         &sendEnabled,
	})
	if err != nil {
		t.Fatalf("UpsertConnectorEndpoint: %v", err)
	}
	srv := NewServer(Config{
		DB:                    db,
		DirectiveWriter:       cognitive.StoreDirectiveWriter(db),
		Bus:                   bus.NewMemBus(db),
		Governor:              governor.NewGovernor(governor.GovernorConfig{}, "."),
		Registry:              NewConnectorRegistry(),
		WorldModel:            worldmodel.New(db),
		ConversationEndpoints: endpointStore,
		Addr:                  ":0",
	})

	res := doReq(t, srv, http.MethodPatch, "/api/navi/chats/"+string(chatB.ID)+"/endpoints/"+string(endpoint.ID), "", "", "127.0.0.1:1234", map[string]any{
		"send_enabled": false,
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("PATCH wrong chat status = %d, want 400", res.StatusCode)
	}
	unchanged, err := endpointStore.GetConversationEndpoint(ctx, string(endpoint.ID))
	if err != nil {
		t.Fatalf("GetConversationEndpoint: %v", err)
	}
	if !unchanged.SendEnabled {
		t.Fatal("endpoint was mutated before chat ownership validation")
	}
}

type gatewayRecordingDispatcher struct {
	calls []gatewayDispatchCall
}

type gatewayDispatchCall struct {
	instanceID string
	msg        pkgconn.OutboundMessage
}

func (d *gatewayRecordingDispatcher) Dispatch(_ context.Context, instanceID string, msg pkgconn.OutboundMessage) error {
	d.calls = append(d.calls, gatewayDispatchCall{instanceID: instanceID, msg: msg})
	return nil
}
