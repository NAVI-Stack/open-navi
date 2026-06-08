package navi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ceoai/navi/internal/navi/inference"
)

func TestProcessTurnWakesActiveRuntimeSessionForActiveChat(t *testing.T) {
	t.Parallel()

	store := &processTurnRuntimeStore{
		activeChatID:     "chat-1",
		runtimeSessionID: "runtime-1",
	}
	var woke string
	loop := NewAgentLoop(LoopConfig{
		Chats:           store,
		RuntimeSessions: store,
		RuntimeStore:    &workflowEventStore{},
		ICSStateStore:   processTurnICSStateStore{},
		RuntimeWake: func(runtimeSessionID string) {
			woke = runtimeSessionID
		},
	})

	if err := loop.ProcessTurn(context.Background()); err != nil {
		t.Fatalf("ProcessTurn returned error: %v", err)
	}
	if woke != "runtime-1" {
		t.Fatalf("RuntimeWake argument = %q, want runtime-1", woke)
	}
	if store.findChatID != "chat-1" {
		t.Fatalf("FindActiveRuntimeSessionForChat chat = %q, want chat-1", store.findChatID)
	}
}

func TestProcessTurnFailsClosedWhenActiveChatHasNoRuntimeSession(t *testing.T) {
	t.Parallel()

	store := &processTurnRuntimeStore{
		activeChatID: "chat-1",
		findErr:      errors.New("no active runtime session"),
	}
	wakeCount := 0
	loop := NewAgentLoop(LoopConfig{
		Chats:           store,
		RuntimeSessions: store,
		RuntimeStore:    &workflowEventStore{},
		ICSStateStore:   processTurnICSStateStore{},
		RuntimeWake: func(string) {
			wakeCount++
		},
	})

	err := loop.ProcessTurn(context.Background())
	if err == nil {
		t.Fatal("ProcessTurn returned nil error, want missing runtime-session error")
	}
	if !errors.Is(err, store.findErr) {
		t.Fatalf("ProcessTurn error = %v, want wrapped %v", err, store.findErr)
	}
	if wakeCount != 0 {
		t.Fatalf("RuntimeWake called %d times, want 0", wakeCount)
	}
}

type processTurnRuntimeStore struct {
	activeChatID     string
	runtimeSessionID string
	findChatID       string
	findErr          error
}

type processTurnICSStateStore struct{}

func (processTurnICSStateStore) SaveICSState(context.Context, string, inference.DecisionEnvelope) (*ICSState, error) {
	return nil, nil
}

func (processTurnICSStateStore) LoadICSState(context.Context, string) (*ICSState, error) {
	return nil, nil
}

func (processTurnICSStateStore) AppendICSHistory(context.Context, string, inference.DecisionEnvelope) (*ICSHistory, error) {
	return nil, nil
}

func (processTurnICSStateStore) LoadICSHistory(context.Context, string, int) ([]ICSHistory, error) {
	return nil, nil
}

func (s *processTurnRuntimeStore) CreateChat(context.Context, CreateChatInput) (*Chat, error) {
	return nil, errors.New("unexpected CreateChat")
}

func (s *processTurnRuntimeStore) RenameChat(context.Context, string, string) error {
	return errors.New("unexpected RenameChat")
}

func (s *processTurnRuntimeStore) GetChat(context.Context, string) (*Chat, error) {
	return nil, errors.New("unexpected GetChat")
}

func (s *processTurnRuntimeStore) GetChatWithMessages(context.Context, string) (*ChatThread, error) {
	return nil, errors.New("unexpected GetChatWithMessages")
}

func (s *processTurnRuntimeStore) ListChats(context.Context, int) ([]Chat, error) {
	return nil, errors.New("unexpected ListChats")
}

func (s *processTurnRuntimeStore) ListChatsByProject(context.Context, string, int) ([]Chat, error) {
	return nil, errors.New("unexpected ListChatsByProject")
}

func (s *processTurnRuntimeStore) UpdateChat(context.Context, Chat) error {
	return errors.New("unexpected UpdateChat")
}

func (s *processTurnRuntimeStore) ArchiveChat(context.Context, string) error {
	return errors.New("unexpected ArchiveChat")
}

func (s *processTurnRuntimeStore) DeleteChat(context.Context, string) error {
	return errors.New("unexpected DeleteChat")
}

func (s *processTurnRuntimeStore) AppendChatMessage(context.Context, string, ChatMessage) (string, error) {
	return "", errors.New("unexpected AppendChatMessage")
}

func (s *processTurnRuntimeStore) AppendSystemAssistantMessage(context.Context, string, string, string, string, string) (string, error) {
	return "", errors.New("unexpected AppendSystemAssistantMessage")
}

func (s *processTurnRuntimeStore) ListChatMessages(context.Context, string, int, int) ([]ChatMessage, error) {
	return nil, errors.New("unexpected ListChatMessages")
}

func (s *processTurnRuntimeStore) MessageCount(context.Context, string) (int, error) {
	return 0, errors.New("unexpected MessageCount")
}

func (s *processTurnRuntimeStore) ActiveChatID(context.Context, ActiveChatScope) (string, error) {
	return s.activeChatID, nil
}

func (s *processTurnRuntimeStore) SetActiveChat(context.Context, ActiveChatScope, string) error {
	return errors.New("unexpected SetActiveChat")
}

func (s *processTurnRuntimeStore) CreateRuntimeSession(context.Context, CreateRuntimeSessionInput) (*RuntimeSession, error) {
	return nil, errors.New("unexpected CreateRuntimeSession")
}

func (s *processTurnRuntimeStore) GetRuntimeSession(context.Context, string) (*RuntimeSession, error) {
	return nil, errors.New("unexpected GetRuntimeSession")
}

func (s *processTurnRuntimeStore) TouchRuntimeSession(context.Context, string) error {
	return errors.New("unexpected TouchRuntimeSession")
}

func (s *processTurnRuntimeStore) CloseRuntimeSession(context.Context, string) error {
	return errors.New("unexpected CloseRuntimeSession")
}

func (s *processTurnRuntimeStore) AttachChatToRuntimeSession(context.Context, string, string, RuntimeSessionChatRelationship) error {
	return errors.New("unexpected AttachChatToRuntimeSession")
}

func (s *processTurnRuntimeStore) ListRuntimeSessionChats(context.Context, string) ([]RuntimeSessionChat, error) {
	return nil, errors.New("unexpected ListRuntimeSessionChats")
}

func (s *processTurnRuntimeStore) FindActiveRuntimeSessionForChat(_ context.Context, chatID string, _ string) (*RuntimeSession, error) {
	s.findChatID = chatID
	if s.findErr != nil {
		return nil, s.findErr
	}
	return &RuntimeSession{
		ID:             ID(s.runtimeSessionID),
		Kind:           RuntimeSessionKindUser,
		Status:         RuntimeSessionStatusActive,
		ExperienceMode: string(ExperienceModeStandard),
		StartedAt:      time.Now().UTC(),
		LastActiveAt:   time.Now().UTC(),
	}, nil
}
