package navi

import (
	"context"
	"errors"
	"testing"
	"time"

	naviruntime "github.com/open-navi/navi/internal/runtime"
)

func TestExecuteRunStatusUsesRuntimeSessionID(t *testing.T) {
	tr := NewStatusTracker()
	chats := &blockingStatusChatStore{
		started: make(chan struct{}),
		release: make(chan struct{}),
		err:     errors.New("stop after status update"),
	}
	loop := NewAgentLoop(LoopConfig{
		Chats:         chats,
		StatusTracker: tr,
	})
	run := naviruntime.NewRun("runtime-1", string(ExperienceModeStandard))
	run.ChatID = "chat-1"

	done := make(chan error, 1)
	go func() {
		_, err := loop.ExecuteRun(context.Background(), naviruntime.ExecuteInput{Run: run})
		done <- err
	}()

	select {
	case <-chats.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ExecuteRun to reach chat load")
	}

	snap := tr.Snapshot()
	if snap.ActiveRuntimeSessionID != "runtime-1" {
		t.Fatalf("active runtime session id = %q, want runtime-1", snap.ActiveRuntimeSessionID)
	}

	close(chats.release)
	select {
	case err := <-done:
		if !errors.Is(err, chats.err) {
			t.Fatalf("ExecuteRun error = %v, want %v", err, chats.err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ExecuteRun to return")
	}
}

type blockingStatusChatStore struct {
	started chan struct{}
	release chan struct{}
	err     error
}

func (s *blockingStatusChatStore) CreateChat(context.Context, CreateChatInput) (*Chat, error) {
	return nil, errors.New("unexpected CreateChat")
}

func (s *blockingStatusChatStore) RenameChat(context.Context, string, string) error {
	return errors.New("unexpected RenameChat")
}

func (s *blockingStatusChatStore) GetChat(context.Context, string) (*Chat, error) {
	return nil, errors.New("unexpected GetChat")
}

func (s *blockingStatusChatStore) GetChatWithMessages(ctx context.Context, _ string) (*ChatThread, error) {
	close(s.started)
	select {
	case <-s.release:
		return nil, s.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *blockingStatusChatStore) ListChats(context.Context, int) ([]Chat, error) {
	return nil, errors.New("unexpected ListChats")
}

func (s *blockingStatusChatStore) ListChatsByProject(context.Context, string, int) ([]Chat, error) {
	return nil, errors.New("unexpected ListChatsByProject")
}

func (s *blockingStatusChatStore) UpdateChat(context.Context, Chat) error {
	return errors.New("unexpected UpdateChat")
}

func (s *blockingStatusChatStore) ArchiveChat(context.Context, string) error {
	return errors.New("unexpected ArchiveChat")
}

func (s *blockingStatusChatStore) DeleteChat(context.Context, string) error {
	return errors.New("unexpected DeleteChat")
}

func (s *blockingStatusChatStore) AppendChatMessage(context.Context, string, ChatMessage) (string, error) {
	return "", errors.New("unexpected AppendChatMessage")
}

func (s *blockingStatusChatStore) AppendSystemAssistantMessage(context.Context, string, string, string, string, string) (string, error) {
	return "", errors.New("unexpected AppendSystemAssistantMessage")
}

func (s *blockingStatusChatStore) ListChatMessages(context.Context, string, int, int) ([]ChatMessage, error) {
	return nil, errors.New("unexpected ListChatMessages")
}

func (s *blockingStatusChatStore) MessageCount(context.Context, string) (int, error) {
	return 0, errors.New("unexpected MessageCount")
}

func (s *blockingStatusChatStore) ActiveChatID(context.Context, ActiveChatScope) (string, error) {
	return "", errors.New("unexpected ActiveChatID")
}

func (s *blockingStatusChatStore) SetActiveChat(context.Context, ActiveChatScope, string) error {
	return errors.New("unexpected SetActiveChat")
}
