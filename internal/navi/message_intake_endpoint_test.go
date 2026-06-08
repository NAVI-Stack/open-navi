package navi_test

import (
	"context"
	"testing"

	"github.com/open-navi/navi/internal/navi"
	naviruntime "github.com/open-navi/navi/internal/runtime"
)

func TestMessageIntakeUsesConsoleEndpointAsDefaultOrigin(t *testing.T) {
	ctx := context.Background()
	store := newEndpointResolverTestStore(t)
	chat, err := store.CreateChat(ctx, navi.CreateChatInput{ID: "chat-intake-console", Title: "Intake"})
	if err != nil {
		t.Fatalf("CreateChat: %v", err)
	}
	runtime := &recordingRuntimeSubmitter{}
	resolver := navi.EndpointResolver{Chats: store, Endpoints: store}
	intake := navi.MessageIntakeService{
		Chats:            store,
		RuntimeSessions:  store,
		Runtime:          runtime,
		EndpointResolver: &resolver,
	}

	result, err := intake.SubmitMessage(ctx, navi.SubmitMessageRequest{
		ChatID:        string(chat.ID),
		Content:       "hello console",
		SourceChannel: "app",
	})
	if err != nil {
		t.Fatalf("SubmitMessage: %v", err)
	}
	if result.OriginEndpointID == "" {
		t.Fatal("expected origin endpoint id")
	}
	if runtime.input.OriginEndpointID != result.OriginEndpointID {
		t.Fatalf("runtime origin endpoint = %q, want %q", runtime.input.OriginEndpointID, result.OriginEndpointID)
	}
	messages, err := store.ListChatMessages(ctx, string(chat.ID), 10, 0)
	if err != nil {
		t.Fatalf("ListChatMessages: %v", err)
	}
	if len(messages) != 1 || messages[0].OriginEndpointID == nil || string(*messages[0].OriginEndpointID) != result.OriginEndpointID {
		t.Fatalf("message origin endpoint mismatch: %#v", messages)
	}
	endpoint, err := store.GetConversationEndpoint(ctx, result.OriginEndpointID)
	if err != nil {
		t.Fatalf("GetConversationEndpoint: %v", err)
	}
	if endpoint.Type != navi.EndpointTypeConsole {
		t.Fatalf("origin endpoint type = %q, want console", endpoint.Type)
	}
}

type recordingRuntimeSubmitter struct {
	input naviruntime.MessageInput
}

func (r *recordingRuntimeSubmitter) SubmitMessage(_ context.Context, runtimeSessionID, _ string, input naviruntime.MessageInput) (*naviruntime.InboxItem, error) {
	r.input = input
	return &naviruntime.InboxItem{
		ID:               "inbox-recorded",
		RuntimeSessionID: runtimeSessionID,
		ChatID:           input.ChatID,
		MessageID:        input.MessageID,
		SourceChannel:    input.SourceChannel,
		SourceMessageRef: input.SourceMessageRef,
		OriginEndpointID: input.OriginEndpointID,
		Content:          input.Content,
		Status:           naviruntime.InboxStatusPending,
	}, nil
}
