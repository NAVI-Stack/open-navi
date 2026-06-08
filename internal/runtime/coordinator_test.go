package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/open-navi/navi/internal/schema"
)

type stubRuntimeStore struct {
	accepted       *InboxItem
	acceptedSignal *InboxItem
	pending        []InboxItem
	pausedRun      *RunState
	promotions     int
}

func (s *stubRuntimeStore) AcceptMessage(ctx context.Context, chatID string, item *InboxItem, personaID string) (*InboxItem, error) {
	s.accepted = item
	return item, nil
}

func (s *stubRuntimeStore) AcceptSignal(ctx context.Context, item *InboxItem) (*InboxItem, error) {
	s.acceptedSignal = item
	return item, nil
}

func (s *stubRuntimeStore) ListPendingRuntimeSessions(ctx context.Context) ([]string, error) {
	return nil, nil
}
func (s *stubRuntimeStore) ListPendingItems(ctx context.Context, chatID string, limit int) ([]InboxItem, error) {
	return append([]InboxItem(nil), s.pending...), nil
}
func (s *stubRuntimeStore) MarkInboxConsumed(ctx context.Context, inboxID, runID string) error {
	return nil
}
func (s *stubRuntimeStore) PromoteDeferredItems(ctx context.Context, chatID string, limit int) (int, error) {
	s.promotions++
	return 0, nil
}
func (s *stubRuntimeStore) CreateRun(ctx context.Context, run *RunState) error { return nil }
func (s *stubRuntimeStore) UpdateRun(ctx context.Context, run *RunState) error { return nil }
func (s *stubRuntimeStore) GetLatestRun(ctx context.Context, chatID string) (*RunState, error) {
	return nil, nil
}
func (s *stubRuntimeStore) GetRun(ctx context.Context, runID string) (*RunState, error) {
	return nil, nil
}
func (s *stubRuntimeStore) GetPausedRun(ctx context.Context, chatID string) (*RunState, error) {
	return s.pausedRun, nil
}
func (s *stubRuntimeStore) SaveCheckpoint(ctx context.Context, cp *Checkpoint) error { return nil }
func (s *stubRuntimeStore) LoadCheckpoint(ctx context.Context, runID string) (*Checkpoint, error) {
	return nil, nil
}
func (s *stubRuntimeStore) LookupRuntimeSessionKind(ctx context.Context, runtimeSessionID string) (schema.RuntimeSessionKind, error) {
	return schema.DefaultRuntimeSessionKindForID(runtimeSessionID), nil
}
func (s *stubRuntimeStore) CompleteRun(ctx context.Context, run *RunState, content string, personaID, inboxItemID string) (string, error) {
	return "", nil
}
func (s *stubRuntimeStore) AppendAssistantMessage(ctx context.Context, run *RunState, content, personaID, inboxItemID string) (string, error) {
	return "", nil
}
func (s *stubRuntimeStore) MarkRunCompleted(ctx context.Context, run *RunState, replyLen int, finalMessageID string) error {
	return nil
}
func (s *stubRuntimeStore) AppendRuntimeEvent(ctx context.Context, ev schema.Event) error { return nil }
func (s *stubRuntimeStore) ListActiveRuns(ctx context.Context) ([]*RunState, error)       { return nil, nil }

type scheduledMessagesStore struct {
	stubRuntimeStore
	appendContents   []string
	markCompleted    int
	markReplyLen     int
	onMarkDone       chan struct{}
	consumedInboxIDs map[string]bool
}

func (s *scheduledMessagesStore) MarkInboxConsumed(ctx context.Context, inboxID, runID string) error {
	if s.consumedInboxIDs == nil {
		s.consumedInboxIDs = make(map[string]bool)
	}
	s.consumedInboxIDs[inboxID] = true
	return nil
}
func (s *scheduledMessagesStore) ListPendingItems(ctx context.Context, chatID string, limit int) ([]InboxItem, error) {
	if s.consumedInboxIDs == nil {
		return append([]InboxItem(nil), s.pending...), nil
	}
	var filtered []InboxItem
	for _, it := range s.pending {
		if !s.consumedInboxIDs[it.ID] {
			filtered = append(filtered, it)
		}
	}
	return filtered, nil
}

func (s *scheduledMessagesStore) AppendAssistantMessage(ctx context.Context, run *RunState, content, personaID, inboxItemID string) (string, error) {
	s.appendContents = append(s.appendContents, content)
	return "msg-id", nil
}
func (s *scheduledMessagesStore) MarkRunCompleted(ctx context.Context, run *RunState, replyLen int, finalMessageID string) error {
	s.markCompleted++
	s.markReplyLen = replyLen
	if s.onMarkDone != nil {
		close(s.onMarkDone)
		s.onMarkDone = nil
	}
	return nil
}

type scheduledMessagesExecutor struct{}

func (scheduledMessagesExecutor) ExecuteRun(ctx context.Context, input ExecuteInput) (*ExecuteResult, error) {
	return &ExecuteResult{
		Run:       input.Run,
		Completed: true,
		ScheduledMessages: []ScheduledMessage{
			{Content: "first", Delay: 0},
			{Content: "second", Delay: 0},
		},
		ExperienceMode: "navi",
	}, nil
}

type stubExecutor struct{}

func (stubExecutor) ExecuteRun(ctx context.Context, input ExecuteInput) (*ExecuteResult, error) {
	return &ExecuteResult{Run: input.Run, Completed: true, FinalContent: "ok"}, nil
}

type completedFailureExecutor struct {
	outcome schema.ExecutionOutcomeOutcome
	summary string
}

func (e completedFailureExecutor) ExecuteRun(ctx context.Context, input ExecuteInput) (*ExecuteResult, error) {
	return &ExecuteResult{
		Run:            input.Run,
		Completed:      true,
		FinalContent:   "fallback reply",
		ExperienceMode: "navi",
		Outcome:        e.outcome,
		OutcomeSummary: e.summary,
	}, nil
}

func TestNewInboxItem(t *testing.T) {
	item := NewInboxItem("sess-1", "hello", "web")
	if item.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if item.RuntimeSessionID != "sess-1" {
		t.Errorf("expected RuntimeSessionID 'sess-1', got %q", item.RuntimeSessionID)
	}
	if item.Content != "hello" {
		t.Errorf("expected Content 'hello', got %q", item.Content)
	}
	if item.SourceChannel != "web" {
		t.Errorf("expected SourceChannel 'web', got %q", item.SourceChannel)
	}
	if item.QueueAction != "append" {
		t.Errorf("expected QueueAction 'append', got %q", item.QueueAction)
	}
	if item.Status != InboxStatusPending {
		t.Errorf("expected Status 'pending', got %q", item.Status)
	}
}

func TestInboxItemConsume(t *testing.T) {
	item := NewInboxItem("sess-1", "hello", "cli")
	item.Consume()
	if item.Status != InboxStatusConsumed {
		t.Errorf("expected Status 'consumed', got %q", item.Status)
	}
}

func TestRunCoordinatorSubmitMessage(t *testing.T) {
	store := &stubRuntimeStore{}
	coord := NewRunCoordinator(store, stubExecutor{}, nil)

	item, err := coord.SubmitMessage(context.Background(), "sess-1", "", MessageInput{
		Content:       "hello",
		SourceChannel: "web",
	})
	if err != nil {
		t.Fatalf("SubmitMessage: %v", err)
	}
	if item == nil || item.ID == "" {
		t.Fatal("expected accepted inbox item")
	}
	if store.accepted == nil {
		t.Fatal("expected store.AcceptMessage to be called")
	}
	if store.accepted.SourceChannel != "web" {
		t.Fatalf("expected source channel web, got %q", store.accepted.SourceChannel)
	}
	if store.accepted.Status != InboxStatusPending {
		t.Fatalf("expected pending inbox status, got %q", store.accepted.Status)
	}
	if store.accepted.ClassifiedReason == "" {
		t.Fatal("expected classified reason to be populated")
	}
}

func TestRunCoordinatorSubmitMessageDefersWhilePaused(t *testing.T) {
	store := &stubRuntimeStore{
		pausedRun: &RunState{RunID: "run-1", RuntimeSessionID: "sess-1"},
	}
	coord := NewRunCoordinator(store, stubExecutor{}, nil)

	item, err := coord.SubmitMessage(context.Background(), "sess-1", "", MessageInput{
		Content:       "hello",
		SourceChannel: "web",
	})
	if err != nil {
		t.Fatalf("SubmitMessage: %v", err)
	}
	if item.QueueAction != "defer" {
		t.Fatalf("expected defer queue action, got %q", item.QueueAction)
	}
	if item.Status != InboxStatusDeferred {
		t.Fatalf("expected deferred status, got %q", item.Status)
	}
}

func TestRunCoordinatorSubmitMessageSupersedesDuplicateIdempotencyKey(t *testing.T) {
	store := &stubRuntimeStore{
		pending: []InboxItem{{
			ID:               "existing",
			RuntimeSessionID: "sess-1",
			IdempotencyKey:   "dup-key",
			Status:           InboxStatusPending,
		}},
	}
	coord := NewRunCoordinator(store, stubExecutor{}, nil)

	item, err := coord.SubmitMessage(context.Background(), "sess-1", "", MessageInput{
		Content:        "hello",
		SourceChannel:  "web",
		IdempotencyKey: "dup-key",
	})
	if err != nil {
		t.Fatalf("SubmitMessage: %v", err)
	}
	if item.QueueAction != "supersede" {
		t.Fatalf("expected supersede queue action, got %q", item.QueueAction)
	}
	if item.Status != InboxStatusSuperseded {
		t.Fatalf("expected superseded status, got %q", item.Status)
	}
}

func TestRunCoordinatorSubmitMessageTracksMergeTarget(t *testing.T) {
	store := &stubRuntimeStore{
		pending: []InboxItem{{
			ID:               "pending-1",
			RuntimeSessionID: "sess-1",
			SourceChannel:    "web",
			ActorType:        "user",
			PayloadType:      "text",
			Status:           InboxStatusPending,
			ReceivedAt:       time.Now().UTC(),
		}},
	}
	coord := NewRunCoordinator(store, stubExecutor{}, nil)

	item, err := coord.SubmitMessage(context.Background(), "sess-1", "", MessageInput{
		Content:       "follow up",
		SourceChannel: "web",
	})
	if err != nil {
		t.Fatalf("SubmitMessage: %v", err)
	}
	if item.QueueAction != "merge" {
		t.Fatalf("expected merge queue action, got %q", item.QueueAction)
	}
	if item.MergedIntoID != "pending-1" {
		t.Fatalf("expected merge target pending-1, got %q", item.MergedIntoID)
	}
}

func TestRunCoordinatorScheduledMessages(t *testing.T) {
	store := &scheduledMessagesStore{
		stubRuntimeStore: stubRuntimeStore{pending: []InboxItem{}},
		onMarkDone:       make(chan struct{}),
	}
	store.pending = append(store.pending, InboxItem{
		ID: "inbox-1", RuntimeSessionID: "sess-1", ChatID: "sess-1", Content: "hi", Status: InboxStatusPending,
		ReceivedAt: time.Now().UTC(), SourceChannel: "web", ActorType: "user", PayloadType: "text",
	})
	coord := NewRunCoordinatorWithScheduler(store, scheduledMessagesExecutor{}, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = coord.Run(ctx) }()
	_, err := coord.SubmitMessage(context.Background(), "sess-1", "", MessageInput{Content: "hi", SourceChannel: "web"})
	if err != nil {
		t.Fatalf("SubmitMessage: %v", err)
	}
	select {
	case <-store.onMarkDone:
	case <-time.After(2 * time.Second):
		t.Fatal("MarkRunCompleted was not called")
	}
	if store.markCompleted != 1 {
		t.Errorf("expected MarkRunCompleted 1 time, got %d", store.markCompleted)
	}
	if len(store.appendContents) != 2 {
		t.Fatalf("expected 2 AppendAssistantMessage calls, got %d: %v", len(store.appendContents), store.appendContents)
	}
	if store.appendContents[0] != "first" || store.appendContents[1] != "second" {
		t.Errorf("expected contents [first, second], got %v", store.appendContents)
	}
	expectedLen := len("first") + len("second")
	if store.markReplyLen != expectedLen {
		t.Errorf("expected MarkRunCompleted replyLen %d, got %d", expectedLen, store.markReplyLen)
	}
}

func TestRunCoordinatorScheduledMessagesWithDelay(t *testing.T) {
	store := &scheduledMessagesStore{
		stubRuntimeStore: stubRuntimeStore{pending: []InboxItem{}},
		onMarkDone:       make(chan struct{}),
	}
	store.pending = append(store.pending, InboxItem{
		ID: "inbox-delay", RuntimeSessionID: "sess-delay", ChatID: "sess-delay", Content: "hi", Status: InboxStatusPending,
		ReceivedAt: time.Now().UTC(), SourceChannel: "web", ActorType: "user", PayloadType: "text",
	})
	sched := NewScheduler()
	defer sched.Stop()
	coord := NewRunCoordinatorWithScheduler(store, delayedScheduledExecutor{}, nil, sched)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = coord.Run(ctx) }()
	_, err := coord.SubmitMessage(context.Background(), "sess-delay", "", MessageInput{Content: "hi", SourceChannel: "web"})
	if err != nil {
		t.Fatalf("SubmitMessage: %v", err)
	}
	select {
	case <-store.onMarkDone:
	case <-time.After(5 * time.Second):
		t.Fatal("MarkRunCompleted was not called within 5s")
	}
	if store.markCompleted != 1 {
		t.Errorf("expected MarkRunCompleted 1 time, got %d", store.markCompleted)
	}
	if len(store.appendContents) != 2 {
		t.Fatalf("expected 2 AppendAssistantMessage calls, got %d: %v", len(store.appendContents), store.appendContents)
	}
	if store.appendContents[0] != "immediate" || store.appendContents[1] != "delayed" {
		t.Errorf("expected contents [immediate, delayed], got %v", store.appendContents)
	}
}

func TestRunCoordinatorCopiesInboxOriginEndpointIDToRun(t *testing.T) {
	store := &queueRuntimeStore{
		stubRuntimeStore: stubRuntimeStore{pending: []InboxItem{{
			ID:               "inbox-origin",
			RuntimeSessionID: "session-origin",
			ChatID:           "chat-origin",
			SourceChannel:    "telegram",
			OriginEndpointID: "endpoint-origin",
			Status:           InboxStatusPending,
			ReceivedAt:       time.Now().UTC(),
		}}},
	}
	coordinator := NewRunCoordinator(store, stubExecutor{}, nil)

	coordinator.dispatchRuntimeSession(context.Background(), "session-origin")

	if len(store.createdRuns) != 1 {
		t.Fatalf("expected one created run, got %d", len(store.createdRuns))
	}
	if store.createdRuns[0].OriginEndpointID != "endpoint-origin" {
		t.Fatalf("run origin endpoint = %q, want endpoint-origin", store.createdRuns[0].OriginEndpointID)
	}
}

func TestRunCoordinatorObservesCompletedAssistantMessage(t *testing.T) {
	store := &queueRuntimeStore{
		stubRuntimeStore: stubRuntimeStore{pending: []InboxItem{{
			ID:               "inbox-observe",
			RuntimeSessionID: "session-observe",
			ChatID:           "chat-observe",
			SourceChannel:    "telegram",
			OriginEndpointID: "endpoint-observe",
			Status:           InboxStatusPending,
			ReceivedAt:       time.Now().UTC(),
		}}},
	}
	coordinator := NewRunCoordinator(store, stubExecutor{}, nil)
	observed := make(chan AssistantMessageEvent, 1)
	coordinator.SetAssistantMessageObserver(func(_ context.Context, event AssistantMessageEvent) error {
		observed <- event
		return nil
	})

	coordinator.dispatchRuntimeSession(context.Background(), "session-observe")

	select {
	case event := <-observed:
		if event.MessageID != "msg-id" {
			t.Fatalf("message id = %q, want msg-id", event.MessageID)
		}
		if event.Content != "ok" {
			t.Fatalf("content = %q, want ok", event.Content)
		}
		if event.Run == nil || event.Run.OriginEndpointID != "endpoint-observe" {
			t.Fatalf("run origin endpoint mismatch: %#v", event.Run)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("assistant message observer was not called")
	}
}

func TestRunCoordinatorTimeoutUnblocksNextMessage(t *testing.T) {
	store := &queueRuntimeStore{}
	exec := &timeoutThenRecoverExecutor{started: make(chan struct{}, 1)}
	coord := NewRunCoordinator(store, exec, nil)
	coord.SetRunTimeout(20 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = coord.Run(ctx) }()

	if _, err := coord.SubmitMessage(context.Background(), "sess-timeout", "", MessageInput{
		Content:       "first",
		SourceChannel: "web",
	}); err != nil {
		t.Fatalf("SubmitMessage first: %v", err)
	}

	select {
	case <-exec.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first run to start")
	}

	if _, err := coord.SubmitMessage(context.Background(), "sess-timeout", "", MessageInput{
		Content:       "second",
		SourceChannel: "web",
	}); err != nil {
		t.Fatalf("SubmitMessage second: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(store.assistantContents) == 1 && store.assistantContents[0] == "recovered reply" {
			if exec.calls != 2 {
				t.Fatalf("expected second run to execute after timeout, got %d calls", exec.calls)
			}
			if len(store.updatedStatuses) == 0 || store.updatedStatuses[0] != schema.RunStatusFailed {
				t.Fatalf("expected first run to be marked failed, got %v", store.updatedStatuses)
			}
			if len(store.events) == 0 || store.events[0].Type != schema.FactRunStarted {
				t.Fatalf("expected run.started event, got %v", store.events)
			}
			foundFailed := false
			for _, ev := range store.events {
				if ev.Type == schema.FactRunFailed {
					foundFailed = true
					break
				}
			}
			if !foundFailed {
				t.Fatalf("expected run.failed event after timeout, got %v", store.events)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("expected second message to complete after timeout, got contents=%v statuses=%v calls=%d", store.assistantContents, store.updatedStatuses, exec.calls)
}

func TestRunCoordinatorTimeoutUsesTelegramRecoveryGuidance(t *testing.T) {
	store := &queueRuntimeStore{}
	exec := &drainingTimeoutExecutor{
		started:        make(chan struct{}, 1),
		cancelObserved: make(chan struct{}, 1),
		release:        make(chan struct{}),
	}
	coord := NewRunCoordinator(store, exec, nil)
	coord.SetRunTimeout(20 * time.Millisecond)
	coord.interruptDrainTimeout = 250 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = coord.Run(ctx) }()

	if _, err := coord.SubmitMessage(context.Background(), "sess-telegram-timeout", "", MessageInput{
		Content:       "hello",
		SourceChannel: "telegram",
	}); err != nil {
		t.Fatalf("SubmitMessage: %v", err)
	}

	select {
	case <-exec.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for telegram run to start")
	}

	select {
	case <-exec.cancelObserved:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for coordinator timeout cancellation")
	}

	close(exec.release)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, ev := range store.events {
			if ev.Type != schema.FactRunFailed {
				continue
			}
			payload := mustRunFailedPayload(t, ev)
			if got, want := payload.Error, TimeoutFallbackContent("telegram"); got != want {
				t.Fatalf("run.failed error = %q, want %q", got, want)
			}
			if len(store.updatedStatuses) == 0 || store.updatedStatuses[len(store.updatedStatuses)-1] != schema.RunStatusFailed {
				t.Fatalf("expected failed run status, got %v", store.updatedStatuses)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("expected telegram timeout to emit run.failed, got events=%v", store.events)
}

type delayedScheduledExecutor struct{}

func (delayedScheduledExecutor) ExecuteRun(ctx context.Context, input ExecuteInput) (*ExecuteResult, error) {
	return &ExecuteResult{
		Run:       input.Run,
		Completed: true,
		ScheduledMessages: []ScheduledMessage{
			{Content: "immediate", Delay: 0},
			{Content: "delayed", Delay: 500 * time.Millisecond},
		},
		ExperienceMode: "navi",
	}, nil
}

type queueRuntimeStore struct {
	stubRuntimeStore
	nextInboxID       int
	consumedInboxIDs  map[string]bool
	createdRuns       []*RunState
	updatedStatuses   []schema.RunStatus
	assistantContents []string
	events            []schema.Event
}

func (s *queueRuntimeStore) AcceptMessage(ctx context.Context, chatID string, item *InboxItem, personaID string) (*InboxItem, error) {
	s.nextInboxID++
	if item.ID == "" {
		item.ID = fmt.Sprintf("inbox-%d", s.nextInboxID)
	}
	if item.Status == "" {
		item.Status = InboxStatusPending
	}
	if item.ReceivedAt.IsZero() {
		item.ReceivedAt = time.Now().UTC()
	}
	s.pending = append(s.pending, *item)
	s.accepted = item
	return item, nil
}

func (s *queueRuntimeStore) ListPendingItems(ctx context.Context, chatID string, limit int) ([]InboxItem, error) {
	var filtered []InboxItem
	for _, it := range s.pending {
		if s.consumedInboxIDs != nil && s.consumedInboxIDs[it.ID] {
			continue
		}
		filtered = append(filtered, it)
	}
	return filtered, nil
}

func (s *queueRuntimeStore) MarkInboxConsumed(ctx context.Context, inboxID, runID string) error {
	if s.consumedInboxIDs == nil {
		s.consumedInboxIDs = make(map[string]bool)
	}
	s.consumedInboxIDs[inboxID] = true
	return nil
}

func (s *queueRuntimeStore) CreateRun(ctx context.Context, run *RunState) error {
	if run != nil {
		copied := *run
		s.createdRuns = append(s.createdRuns, &copied)
	}
	return nil
}

func (s *queueRuntimeStore) UpdateRun(ctx context.Context, run *RunState) error {
	s.updatedStatuses = append(s.updatedStatuses, run.Status)
	return nil
}

func (s *queueRuntimeStore) CompleteRun(ctx context.Context, run *RunState, content string, personaID, inboxItemID string) (string, error) {
	s.assistantContents = append(s.assistantContents, content)
	return "msg-id", nil
}

func (s *queueRuntimeStore) AppendRuntimeEvent(ctx context.Context, ev schema.Event) error {
	s.events = append(s.events, ev)
	return nil
}

type timeoutThenRecoverExecutor struct {
	started chan struct{}
	calls   int
}

func (e *timeoutThenRecoverExecutor) ExecuteRun(ctx context.Context, input ExecuteInput) (*ExecuteResult, error) {
	e.calls++
	if e.calls == 1 {
		if e.started != nil {
			select {
			case e.started <- struct{}{}:
			default:
			}
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return &ExecuteResult{
		Run:            input.Run,
		Completed:      true,
		FinalContent:   "recovered reply",
		ExperienceMode: "navi",
	}, nil
}

type drainingTimeoutExecutor struct {
	started        chan struct{}
	cancelObserved chan struct{}
	release        chan struct{}
}

func (e *drainingTimeoutExecutor) ExecuteRun(ctx context.Context, input ExecuteInput) (*ExecuteResult, error) {
	if e.started != nil {
		select {
		case e.started <- struct{}{}:
		default:
		}
	}
	<-ctx.Done()
	if e.cancelObserved != nil {
		select {
		case e.cancelObserved <- struct{}{}:
		default:
		}
	}
	if e.release != nil {
		<-e.release
	}
	return nil, ErrRunCancelled
}

type failingExecutor struct {
	err error
}

func (e failingExecutor) ExecuteRun(ctx context.Context, input ExecuteInput) (*ExecuteResult, error) {
	return nil, e.err
}

type cancellableExecutor struct {
	started chan struct{}
}

func (e *cancellableExecutor) ExecuteRun(ctx context.Context, input ExecuteInput) (*ExecuteResult, error) {
	if e.started != nil {
		select {
		case e.started <- struct{}{}:
		default:
		}
	}
	<-ctx.Done()
	return nil, ErrRunCancelled
}

func TestRunCoordinatorTimeoutWaitsForExecutorInterruptDrain(t *testing.T) {
	store := &queueRuntimeStore{}
	exec := &drainingTimeoutExecutor{
		started:        make(chan struct{}, 1),
		cancelObserved: make(chan struct{}, 1),
		release:        make(chan struct{}),
	}
	var outcomes []schema.ExecutionOutcomeOutcome
	coord := NewRunCoordinator(store, exec, func(ctx context.Context, eo schema.ExecutionOutcome) error {
		outcomes = append(outcomes, eo.Outcome)
		return nil
	})
	coord.SetRunTimeout(20 * time.Millisecond)
	coord.interruptDrainTimeout = 250 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = coord.Run(ctx) }()

	if _, err := coord.SubmitMessage(context.Background(), "sess-timeout-drain", "", MessageInput{
		Content:       "first",
		SourceChannel: "web",
	}); err != nil {
		t.Fatalf("SubmitMessage: %v", err)
	}

	select {
	case <-exec.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for run to start")
	}

	select {
	case <-exec.cancelObserved:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for coordinator timeout cancellation")
	}

	time.Sleep(40 * time.Millisecond)
	for _, status := range store.updatedStatuses {
		if status == schema.RunStatusFailed {
			t.Fatalf("expected coordinator to wait for executor drain before failing the run, got statuses %v", store.updatedStatuses)
		}
	}
	for _, ev := range store.events {
		if ev.Type == schema.FactRunFailed {
			t.Fatalf("expected no run.failed event before executor drain completes, got %v", store.events)
		}
	}

	close(exec.release)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		foundFailed := false
		foundInterruptApplied := false
		for _, ev := range store.events {
			if ev.Type == schema.FactRunFailed {
				foundFailed = true
			}
			if ev.Type == schema.FactInterruptApplied {
				foundInterruptApplied = true
			}
		}
		if foundFailed && foundInterruptApplied {
			if len(outcomes) == 0 || outcomes[len(outcomes)-1] != schema.ExecutionOutcomeTimedOut {
				t.Fatalf("expected timed_out execution outcome after drained timeout, got %v", outcomes)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("expected drained timeout to finish with interrupt applied and run failed, got events=%v outcomes=%v statuses=%v", store.events, outcomes, store.updatedStatuses)
}

func TestRunCoordinatorExecutorFailureEmitsRunFailed(t *testing.T) {
	store := &queueRuntimeStore{}
	coord := NewRunCoordinator(store, failingExecutor{err: fmt.Errorf("boom")}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = coord.Run(ctx) }()

	if _, err := coord.SubmitMessage(context.Background(), "sess-fail", "", MessageInput{
		Content:       "fail please",
		SourceChannel: "web",
	}); err != nil {
		t.Fatalf("SubmitMessage: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, ev := range store.events {
			if ev.Type == schema.FactRunFailed {
				if len(store.updatedStatuses) == 0 || store.updatedStatuses[len(store.updatedStatuses)-1] != schema.RunStatusFailed {
					t.Fatalf("expected failed run status, got %v", store.updatedStatuses)
				}
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("expected run.failed event for executor failure, got events=%v statuses=%v", store.events, store.updatedStatuses)
}

func TestRunCoordinatorExecutorFailureRecordsStructuredErrorWhenConfigured(t *testing.T) {
	store := &queueRuntimeStore{}
	coord := NewRunCoordinator(store, failingExecutor{err: fmt.Errorf("boom")}, nil)

	type recordedError struct {
		component   string
		chatID      string
		runID       string
		errorType   string
		message     string
		contextJSON string
	}
	recordedCh := make(chan recordedError, 1)
	coord.SetErrorRecorder(func(ctx context.Context, component, chatID, runID, errorType, message, contextJSON string) error {
		recordedCh <- recordedError{
			component:   component,
			chatID:      chatID,
			runID:       runID,
			errorType:   errorType,
			message:     message,
			contextJSON: contextJSON,
		}
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = coord.Run(ctx) }()

	if _, err := coord.SubmitMessage(context.Background(), "sess-fail-record", "", MessageInput{
		Content:       "fail with diagnostics",
		SourceChannel: "web",
	}); err != nil {
		t.Fatalf("SubmitMessage: %v", err)
	}

	select {
	case recorded := <-recordedCh:
		if recorded.component != "runtime" {
			t.Fatalf("component = %q, want runtime", recorded.component)
		}
		if recorded.chatID != "sess-fail-record" {
			t.Fatalf("chatID = %q, want sess-fail-record", recorded.chatID)
		}
		if recorded.runID == "" {
			t.Fatal("expected runID to be recorded")
		}
		if recorded.errorType != "run_failed" {
			t.Fatalf("errorType = %q, want run_failed", recorded.errorType)
		}
		if recorded.message != "boom" {
			t.Fatalf("message = %q, want boom", recorded.message)
		}
		if recorded.contextJSON != "" {
			t.Fatalf("contextJSON = %q, want empty", recorded.contextJSON)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected runtime failure to record a structured error")
	}
}

func TestRunCoordinatorCompletedFallbackFailureRecordsStructuredErrorAndOutcome(t *testing.T) {
	store := &queueRuntimeStore{}
	var outcomes []schema.ExecutionOutcomeOutcome
	coord := NewRunCoordinator(store, completedFailureExecutor{
		outcome: schema.ExecutionOutcomeFailed,
		summary: `llm openai: stream request: Post "http://host.docker.internal:11434/v1/chat/completions": EOF`,
	}, func(ctx context.Context, eo schema.ExecutionOutcome) error {
		outcomes = append(outcomes, eo.Outcome)
		return nil
	})

	type recordedError struct {
		component   string
		chatID      string
		runID       string
		errorType   string
		message     string
		contextJSON string
	}
	recordedCh := make(chan recordedError, 1)
	coord.SetErrorRecorder(func(ctx context.Context, component, chatID, runID, errorType, message, contextJSON string) error {
		recordedCh <- recordedError{
			component:   component,
			chatID:      chatID,
			runID:       runID,
			errorType:   errorType,
			message:     message,
			contextJSON: contextJSON,
		}
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = coord.Run(ctx) }()

	if _, err := coord.SubmitMessage(context.Background(), "sess-fallback-record", "", MessageInput{
		Content:       "trigger fallback",
		SourceChannel: "web",
	}); err != nil {
		t.Fatalf("SubmitMessage: %v", err)
	}

	select {
	case recorded := <-recordedCh:
		if recorded.component != "runtime" {
			t.Fatalf("component = %q, want runtime", recorded.component)
		}
		if recorded.chatID != "sess-fallback-record" {
			t.Fatalf("chatID = %q, want sess-fallback-record", recorded.chatID)
		}
		if recorded.runID == "" {
			t.Fatal("expected runID to be recorded")
		}
		if recorded.errorType != "run_completed_with_failure" {
			t.Fatalf("errorType = %q, want run_completed_with_failure", recorded.errorType)
		}
		if !strings.Contains(recorded.message, "host.docker.internal") || !strings.Contains(recorded.message, "EOF") {
			t.Fatalf("message = %q, want raw transport failure details", recorded.message)
		}
		if !strings.Contains(recorded.contextJSON, `"completed":true`) || !strings.Contains(recorded.contextJSON, `"outcome":"failed"`) {
			t.Fatalf("contextJSON = %q, want completed failed context", recorded.contextJSON)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected completed fallback failure to record a structured error")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(outcomes) > 0 && outcomes[len(outcomes)-1] == schema.ExecutionOutcomeFailed && len(store.assistantContents) > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("expected completed fallback failure to persist failed outcome and assistant reply, got outcomes=%v assistant=%v", outcomes, store.assistantContents)
}

func TestRunCoordinatorCancelRunEmitsRunCancelled(t *testing.T) {
	store := &queueRuntimeStore{}
	exec := &cancellableExecutor{started: make(chan struct{}, 1)}
	coord := NewRunCoordinator(store, exec, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = coord.Run(ctx) }()

	if _, err := coord.SubmitMessage(context.Background(), "sess-cancel", "", MessageInput{
		Content:       "cancel please",
		SourceChannel: "web",
	}); err != nil {
		t.Fatalf("SubmitMessage: %v", err)
	}

	select {
	case <-exec.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for run to start")
	}

	if err := coord.CancelRun("sess-cancel"); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, ev := range store.events {
			if ev.Type == schema.FactRunCancelled {
				if len(store.updatedStatuses) == 0 || store.updatedStatuses[len(store.updatedStatuses)-1] != schema.RunStatusCancelled {
					t.Fatalf("expected cancelled run status, got %v", store.updatedStatuses)
				}
				payload := mustRunCancelledPayload(t, ev)
				if payload.Reason != "cancelled by user" {
					t.Fatalf("expected cancellation reason to round-trip, got %#v", payload)
				}
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("expected run.cancelled event after CancelRun, got events=%v statuses=%v", store.events, store.updatedStatuses)
}

func TestRunCoordinatorEnqueueProposalResolutionRequiresMatchingPausedRun(t *testing.T) {
	store := &stubRuntimeStore{
		pausedRun: &RunState{
			RunID:               "run-1",
			RuntimeSessionID:    "sess-1",
			BlockedOnProposalID: "proposal-1",
		},
	}
	coord := NewRunCoordinator(store, stubExecutor{}, nil)

	if _, err := coord.EnqueueProposalResolution(context.Background(), "sess-1", "proposal-2", ProposalResolutionApprove, ""); err == nil {
		t.Fatal("expected proposal mismatch error")
	}
	if store.acceptedSignal != nil {
		t.Fatal("expected mismatched proposal not to be enqueued")
	}

	if _, err := coord.EnqueueProposalResolution(context.Background(), "sess-1", "proposal-1", ProposalResolutionApprove, "ok"); err != nil {
		t.Fatalf("expected matching proposal to enqueue, got %v", err)
	}
	if store.acceptedSignal == nil {
		t.Fatal("expected matching proposal signal to be enqueued")
	}
}

func TestRuntimeSessionEventVisibilityInternalHeartbeatSession(t *testing.T) {
	if got := runtimeSessionEventVisibility(schema.RuntimeSessionKindInternal, schema.HeartbeatAutoRuntimeSessionID); got != schema.VisibilityOperator {
		t.Fatalf("runtimeSessionEventVisibility(%s) = %q, want %q", schema.HeartbeatAutoRuntimeSessionID, got, schema.VisibilityOperator)
	}
	if got := runtimeSessionEventVisibility(schema.RuntimeSessionKindUser, "sess-user"); got != schema.VisibilityUser {
		t.Fatalf("runtimeSessionEventVisibility(sess-user) = %q, want %q", got, schema.VisibilityUser)
	}
}

func mustRunFailedPayload(t *testing.T, ev schema.Event) schema.RunFailedPayload {
	t.Helper()
	var payload schema.RunFailedPayload
	raw, err := json.Marshal(ev.Payload)
	if err != nil {
		t.Fatalf("marshal run.failed payload: %v", err)
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal run.failed payload: %v", err)
	}
	return payload
}

func mustRunCancelledPayload(t *testing.T, ev schema.Event) schema.RunCancelledPayload {
	t.Helper()
	var payload schema.RunCancelledPayload
	raw, err := json.Marshal(ev.Payload)
	if err != nil {
		t.Fatalf("marshal run.cancelled payload: %v", err)
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal run.cancelled payload: %v", err)
	}
	return payload
}
