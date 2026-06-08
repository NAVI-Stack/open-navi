package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/navi"
)

func TestSQLiteSessionStore_ChatStoreRoundTrip(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	ctx := context.Background()
	store := NewSQLiteStore(db)

	chat, err := store.CreateChat(ctx, navi.CreateChatInput{
		Title:        "Implementation Branch",
		ProjectID:    "proj-chat",
		RootChatID:   "root-chat",
		ParentChatID: "parent-chat",
		BranchReason: "implementation focus",
	})
	if err != nil {
		t.Fatalf("CreateChat failed: %v", err)
	}
	if chat.ID == "" {
		t.Fatal("expected generated chat id")
	}

	messageTime := time.Now().UTC()
	msgID, err := store.AppendChatMessage(ctx, string(chat.ID), navi.ChatMessage{
		Role:      "user",
		Content:   "make the chat model concrete",
		CreatedAt: messageTime,
	})
	if err != nil {
		t.Fatalf("AppendChatMessage failed: %v", err)
	}
	if msgID == "" {
		t.Fatal("expected generated message id")
	}

	loaded, err := store.GetChat(ctx, string(chat.ID))
	if err != nil {
		t.Fatalf("GetChat failed: %v", err)
	}
	if loaded.Title != "Implementation Branch" {
		t.Fatalf("expected title preserved, got %q", loaded.Title)
	}
	if loaded.ProjectID == nil || string(*loaded.ProjectID) != "proj-chat" {
		t.Fatalf("expected project id preserved, got %#v", loaded.ProjectID)
	}
	if loaded.RootChatID == nil || string(*loaded.RootChatID) != "root-chat" {
		t.Fatalf("expected root chat id preserved, got %#v", loaded.RootChatID)
	}
	if loaded.ParentChatID == nil || string(*loaded.ParentChatID) != "parent-chat" {
		t.Fatalf("expected parent chat id preserved, got %#v", loaded.ParentChatID)
	}
	if loaded.BranchReason != "implementation focus" {
		t.Fatalf("expected branch reason preserved, got %q", loaded.BranchReason)
	}
	if loaded.MessageCount != 1 || loaded.UserMessageCount != 1 {
		t.Fatalf("expected message counters updated, got total=%d user=%d", loaded.MessageCount, loaded.UserMessageCount)
	}
	if loaded.LastMessageAt == nil {
		t.Fatal("expected last message timestamp")
	}

	messages, err := store.ListChatMessages(ctx, string(chat.ID), 10, 0)
	if err != nil {
		t.Fatalf("ListChatMessages failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected one chat message, got %d", len(messages))
	}
	if messages[0].Content != "make the chat model concrete" {
		t.Fatalf("unexpected message content: %q", messages[0].Content)
	}
}

func TestSQLiteSessionStore_ListChatMessagesPreservesInsertionOrderOnTimestampTie(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	ctx := context.Background()
	store := NewSQLiteStore(db)

	chat, err := store.CreateChat(ctx, navi.CreateChatInput{Title: "Timestamp Tie"})
	if err != nil {
		t.Fatalf("CreateChat failed: %v", err)
	}

	createdAt := time.Now().UTC()
	if _, err := store.AppendChatMessage(ctx, string(chat.ID), navi.ChatMessage{
		Role:      "user",
		Content:   "first",
		CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("AppendChatMessage first failed: %v", err)
	}
	if _, err := store.AppendChatMessage(ctx, string(chat.ID), navi.ChatMessage{
		Role:      "assistant",
		Content:   "second",
		CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("AppendChatMessage second failed: %v", err)
	}

	messages, err := store.ListChatMessages(ctx, string(chat.ID), 10, 0)
	if err != nil {
		t.Fatalf("ListChatMessages failed: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected two chat messages, got %d", len(messages))
	}
	if messages[0].Content != "first" || messages[1].Content != "second" {
		t.Fatalf("expected insertion order on timestamp tie, got %q then %q", messages[0].Content, messages[1].Content)
	}
}

func TestSQLiteSessionStore_GetChatWithMessagesReturnsLatestWindow(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	store := NewSQLiteStore(db)
	ctx := context.Background()

	chat, err := store.CreateChat(ctx, navi.CreateChatInput{ID: "chat-latest-window", Title: "Latest Window"})
	if err != nil {
		t.Fatalf("CreateChat failed: %v", err)
	}
	for i := 0; i < 55; i++ {
		if _, err := store.AppendChatMessage(ctx, string(chat.ID), navi.ChatMessage{
			Role:    "user",
			Content: fmt.Sprintf("message-%02d", i),
		}); err != nil {
			t.Fatalf("AppendChatMessage %d failed: %v", i, err)
		}
	}

	thread, err := store.GetChatWithMessages(ctx, string(chat.ID))
	if err != nil {
		t.Fatalf("GetChatWithMessages failed: %v", err)
	}
	if len(thread.Messages) != 50 {
		t.Fatalf("message count = %d, want 50", len(thread.Messages))
	}
	if got := thread.Messages[0].Content; got != "message-05" {
		t.Fatalf("first returned message = %q, want message-05", got)
	}
	if got := thread.Messages[49].Content; got != "message-54" {
		t.Fatalf("last returned message = %q, want message-54", got)
	}
}

func TestSQLiteSessionStore_RuntimeSessionCanLinkChats(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	ctx := context.Background()
	store := NewSQLiteStore(db)

	runtimeSession, err := store.CreateRuntimeSession(ctx, navi.CreateRuntimeSessionInput{
		Kind:           navi.RuntimeSessionKindBackground,
		ExperienceMode: "navi",
		SourceChannel:  "system",
	})
	if err != nil {
		t.Fatalf("CreateRuntimeSession failed: %v", err)
	}
	if runtimeSession.ID == "" {
		t.Fatal("expected generated runtime session id")
	}

	chat, err := store.CreateChat(ctx, navi.CreateChatInput{Title: "Referenced Chat"})
	if err != nil {
		t.Fatalf("CreateChat failed: %v", err)
	}
	if err := store.AttachChatToRuntimeSession(ctx, string(runtimeSession.ID), string(chat.ID), navi.RuntimeSessionChatReferenced); err != nil {
		t.Fatalf("AttachChatToRuntimeSession failed: %v", err)
	}

	links, err := store.ListRuntimeSessionChats(ctx, string(runtimeSession.ID))
	if err != nil {
		t.Fatalf("ListRuntimeSessionChats failed: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected one runtime session chat link, got %d", len(links))
	}
	if links[0].ChatID != chat.ID || links[0].Relationship != navi.RuntimeSessionChatReferenced {
		t.Fatalf("unexpected runtime session chat link: %#v", links[0])
	}
}

func TestSQLiteSessionStore_ChatStoreConversationParity(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	ctx := context.Background()
	store := NewSQLiteStore(db)

	first, err := store.CreateChat(ctx, navi.CreateChatInput{
		Title:          "First Project Chat",
		OwnerID:        "owner-1",
		WorkspaceID:    "workspace-1",
		ProjectID:      "project-1",
		ExperienceMode: string(navi.ExperienceModeStandard),
	})
	if err != nil {
		t.Fatalf("CreateChat first failed: %v", err)
	}
	second, err := store.CreateChat(ctx, navi.CreateChatInput{
		Title:       "Second Project Chat",
		OwnerID:     "owner-1",
		WorkspaceID: "workspace-2",
		ProjectID:   "project-2",
	})
	if err != nil {
		t.Fatalf("CreateChat second failed: %v", err)
	}

	if err := store.RenameChat(ctx, string(first.ID), "Renamed Chat"); err != nil {
		t.Fatalf("RenameChat failed: %v", err)
	}
	if _, err := store.AppendChatMessage(ctx, string(first.ID), navi.ChatMessage{
		Role:      "user",
		Content:   "keep this as transcript",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AppendChatMessage user failed: %v", err)
	}
	if _, err := store.AppendSystemAssistantMessage(ctx, string(first.ID), "assistant transcript", string(navi.ExperienceModeStandard), "system", "proactive"); err != nil {
		t.Fatalf("AppendSystemAssistantMessage failed: %v", err)
	}

	thread, err := store.GetChatWithMessages(ctx, string(first.ID))
	if err != nil {
		t.Fatalf("GetChatWithMessages failed: %v", err)
	}
	if thread.Chat.Title != "Renamed Chat" {
		t.Fatalf("renamed title = %q", thread.Chat.Title)
	}
	if len(thread.Messages) != 2 {
		t.Fatalf("expected two transcript messages, got %d", len(thread.Messages))
	}
	if thread.Messages[1].Role != "assistant" || thread.Messages[1].MessageKind != "proactive" {
		t.Fatalf("assistant message not normalized/preserved: %#v", thread.Messages[1])
	}

	projectChats, err := store.ListChatsByProject(ctx, "project-1", 10)
	if err != nil {
		t.Fatalf("ListChatsByProject failed: %v", err)
	}
	if len(projectChats) != 1 || projectChats[0].ID != first.ID {
		t.Fatalf("expected only first project chat, got %#v", projectChats)
	}

	if err := store.ArchiveChat(ctx, string(first.ID)); err != nil {
		t.Fatalf("ArchiveChat failed: %v", err)
	}
	projectChats, err = store.ListChatsByProject(ctx, "project-1", 10)
	if err != nil {
		t.Fatalf("ListChatsByProject after archive failed: %v", err)
	}
	if len(projectChats) != 0 {
		t.Fatalf("archived chat should not list as active project chat: %#v", projectChats)
	}

	if err := store.DeleteChat(ctx, string(second.ID)); err != nil {
		t.Fatalf("DeleteChat failed: %v", err)
	}
	chats, err := store.ListChats(ctx, 10)
	if err != nil {
		t.Fatalf("ListChats failed: %v", err)
	}
	for _, chat := range chats {
		if chat.ID == second.ID {
			t.Fatalf("deleted chat should not appear in active list: %#v", chat)
		}
	}
}

func TestSQLiteSessionStore_ActiveChatIsScoped(t *testing.T) {
	db := prepareTestDB(t)
	defer db.Close()

	ctx := context.Background()
	store := NewSQLiteStore(db)

	ownerProjectWeb := navi.ActiveChatScope{
		OwnerID:       "owner-1",
		ProjectID:     "project-1",
		WorkspaceID:   "workspace-1",
		SourceChannel: "web",
	}
	ownerProjectTelegram := navi.ActiveChatScope{
		OwnerID:       "owner-1",
		ProjectID:     "project-1",
		WorkspaceID:   "workspace-1",
		SourceChannel: "telegram",
	}
	otherProject := navi.ActiveChatScope{
		OwnerID:       "owner-1",
		ProjectID:     "project-2",
		WorkspaceID:   "workspace-1",
		SourceChannel: "web",
	}

	if err := store.SetActiveChat(ctx, ownerProjectWeb, "chat-web"); err != nil {
		t.Fatalf("SetActiveChat web failed: %v", err)
	}
	if err := store.SetActiveChat(ctx, ownerProjectTelegram, "chat-telegram"); err != nil {
		t.Fatalf("SetActiveChat telegram failed: %v", err)
	}
	if err := store.SetActiveChat(ctx, otherProject, "chat-other-project"); err != nil {
		t.Fatalf("SetActiveChat other project failed: %v", err)
	}

	got, err := store.ActiveChatID(ctx, ownerProjectWeb)
	if err != nil {
		t.Fatalf("ActiveChatID web failed: %v", err)
	}
	if got != "chat-web" {
		t.Fatalf("web active chat = %q", got)
	}
	got, err = store.ActiveChatID(ctx, ownerProjectTelegram)
	if err != nil {
		t.Fatalf("ActiveChatID telegram failed: %v", err)
	}
	if got != "chat-telegram" {
		t.Fatalf("telegram active chat = %q", got)
	}
	got, err = store.ActiveChatID(ctx, otherProject)
	if err != nil {
		t.Fatalf("ActiveChatID other project failed: %v", err)
	}
	if got != "chat-other-project" {
		t.Fatalf("other project active chat = %q", got)
	}
}
