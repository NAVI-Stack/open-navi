package navi

import (
	"context"
	"log/slog"
	"strings"

	"github.com/open-navi/navi/internal/navi/compaction"
	naviruntime "github.com/open-navi/navi/internal/runtime"
)

type chatCompactionStore interface {
	GetChatMemory(ctx context.Context, chatID string) (compaction.ChatMemory, error)
	PutChatMemory(ctx context.Context, memory compaction.ChatMemory) error
	AppendCompactionCheckpoint(ctx context.Context, checkpoint compaction.CompactionCheckpoint) error
	MarkMessagesCompacted(ctx context.Context, chatID, checkpointID, epochID, startMessageID, endMessageID string) error
	ListMessagesForCompaction(ctx context.Context, chatID string) ([]compaction.Message, error)
}

type chatRecentCompactionReader interface {
	ListRecentUncompactedMessages(ctx context.Context, chatID string, limit int) ([]compaction.Message, error)
}

func (l *AgentLoop) isStructuredCompactionEnabled() bool {
	if l == nil || l.cfg.Chats == nil {
		return false
	}
	_, ok := l.cfg.Chats.(chatCompactionStore)
	return ok
}

func (l *AgentLoop) runCompactionBeforeModel(ctx context.Context, chatID, newInput string) {
	trace := naviruntime.ProgressTraceFromContext(ctx)
	if strings.TrimSpace(trace.RuntimeSessionID) == "" {
		trace.RuntimeSessionID = chatID
	}
	tracer := naviruntime.NewProgressTracer("navi.compaction", trace)
	compactionDone := tracer.StageStart(ctx, "navi.compaction.before_model")
	defer compactionDone(nil)
	store, ok := l.cfg.Chats.(chatCompactionStore)
	if !ok {
		tracer.Mark("navi.compaction.disabled")
		return
	}
	messages, err := store.ListMessagesForCompaction(ctx, chatID)
	if err != nil {
		slog.Warn("navi: compaction list failed", "chat_id", chatID, "error", err)
		return
	}
	svc := compaction.Service{
		Budget: compaction.NewBudgetManager(nil, compaction.BudgetConfig{}),
		Selector: compaction.SegmentSelector{
			DefaultProtectedRecentWindow: 10,
			MinimumProtectedRecentWindow: 4,
		},
		Rehydrator: compaction.Rehydrator{},
		Memory:     store,
		Checkpoint: store,
	}
	output, err := svc.Run(ctx, compaction.RunInput{
		ChatID:               chatID,
		Messages:             messages,
		NewInput:             newInput,
		MaxContextTokens:     8192,
		ReservedOutputTokens: 1200,
		ReservedToolHeadroom: 600,
	})
	if err != nil {
		slog.Warn("navi: compaction run failed", "chat_id", chatID, "error", err)
		return
	}
	tracer.Mark("navi.compaction.completed", "trigger", output.Triggered)
	if output.Triggered == compaction.TriggerHard || output.Triggered == compaction.TriggerEmergency {
		slog.Debug("navi: compaction completed before model", "chat_id", chatID, "trigger", output.Triggered)
	}
}

func (l *AgentLoop) chatCompactionBlock(ctx context.Context, chatID string) string {
	store, ok := l.cfg.Chats.(interface {
		GetChatMemory(ctx context.Context, chatID string) (compaction.ChatMemory, error)
	})
	if !ok {
		return ""
	}
	mem, err := store.GetChatMemory(ctx, chatID)
	if err != nil || strings.TrimSpace(mem.ChatID) == "" {
		return ""
	}
	// Structured compaction state is the authoritative continuity surface when
	// compaction is enabled.
	liveTail := []compaction.Message{}
	if reader, ok := l.cfg.Chats.(chatRecentCompactionReader); ok {
		liveTail, _ = reader.ListRecentUncompactedMessages(ctx, chatID, 8)
	}
	rebuild := compaction.Rehydrator{}.Assemble(compaction.RehydrationInput{
		ChatFrame:             mem.ChatFrame,
		TaskFrames:            mem.TaskFrames,
		ProposalRefs:          mem.ChatFrame.ActiveProposalRefs,
		FailureRefs:           mem.ChatFrame.ActiveFailureRefs,
		RetrievalSpans:        mem.RetrievalSpans,
		LiveTail:              liveTail,
		ProtectedRecentWindow: 4,
		MaxTaskFrames:         6,
		MaxSupportSpans:       4,
	})
	return strings.Join(rebuild.Sections, "\n\n")
}
