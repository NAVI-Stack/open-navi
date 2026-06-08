package compaction

import "context"

type ChatMemoryStore interface {
	GetChatMemory(ctx context.Context, chatID string) (ChatMemory, error)
	PutChatMemory(ctx context.Context, memory ChatMemory) error
}

type RuntimeStateStore interface {
	ListCompactionRunSnapshots(ctx context.Context, chatID string) ([]RunSnapshot, error)
}

type CheckpointStore interface {
	AppendCompactionCheckpoint(ctx context.Context, checkpoint CompactionCheckpoint) error
	MarkMessagesCompacted(ctx context.Context, chatID, checkpointID, epochID, startMessageID, endMessageID string) error
}
