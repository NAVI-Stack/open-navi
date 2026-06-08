package runtime

import (
	"context"

	"github.com/ceoai/navi/internal/schema"
)

// Store is the persistence boundary for chat-bound runtime sessions.
type Store interface {
	AcceptMessage(ctx context.Context, runtimeSessionID string, item *InboxItem, experienceMode string) (*InboxItem, error)
	AcceptSignal(ctx context.Context, item *InboxItem) (*InboxItem, error)
	ListPendingRuntimeSessions(ctx context.Context) ([]string, error)
	ListPendingItems(ctx context.Context, runtimeSessionID string, limit int) ([]InboxItem, error)
	MarkInboxConsumed(ctx context.Context, inboxID, runID string) error
	PromoteDeferredItems(ctx context.Context, runtimeSessionID string, limit int) (int, error)
	CreateRun(ctx context.Context, run *RunState) error
	UpdateRun(ctx context.Context, run *RunState) error
	GetLatestRun(ctx context.Context, runtimeSessionID string) (*RunState, error)
	GetRun(ctx context.Context, runID string) (*RunState, error)
	GetPausedRun(ctx context.Context, runtimeSessionID string) (*RunState, error)
	LookupRuntimeSessionKind(ctx context.Context, runtimeSessionID string) (schema.RuntimeSessionKind, error)
	SaveCheckpoint(ctx context.Context, cp *Checkpoint) error
	LoadCheckpoint(ctx context.Context, runID string) (*Checkpoint, error)
	CompleteRun(ctx context.Context, run *RunState, content string, experienceMode, inboxItemID string) (string, error)
	AppendAssistantMessage(ctx context.Context, run *RunState, content, experienceMode, inboxItemID string) (string, error)
	MarkRunCompleted(ctx context.Context, run *RunState, replyLen int, finalMessageID string) error
	AppendRuntimeEvent(ctx context.Context, ev schema.Event) error
	ListActiveRuns(ctx context.Context) ([]*RunState, error)
}
