# Chat Compaction Implementation Mapping
**Status:** Draft Normative Reference  
**Applies to:** current `feat/compaction-system` branch
**Related docs:** [architectural contract](session-compaction-architectural-contract.md), [ADR-009](../adr/ADR-009-session-compaction-runtime.md)

Historical naming note: this document previously used "session"; canonical terminology is now Chat for transcript/context and RuntimeSession for execution lifecycle.

## 1. Purpose

This document maps the current branch implementation to the target Chat Compaction architecture.

It exists to make drift explicit.

## 2. Current branch summary

The current branch has **partial compaction scaffolding**, not the target subsystem.

Implemented today:

- persistent chat/message storage in `internal/navi/chat.go` and `internal/navi/store/chat_store.go`
- a rolling summarizer in `internal/navi/summarizer.go`
- prompt injection of `Earlier chat summary` through `ChatSummaryBlock(...)`
- compaction service and persistence coverage in `internal/navi/compaction/*_test.go` and `internal/navi/store/compaction_store_test.go`

Missing relative to target architecture:

- Chat Frame
- Task Frames
- compaction checkpoints
- retrieval spans
- compaction markers on raw messages
- protected-boundary-aware segment selection
- rehydration-first prompt assembly

## 3. Canonical artifacts → current files

### 3.1 Raw chat messages

**Primary files:**

- `internal/navi/chat.go`
- `internal/navi/store/chat_store.go`

### Current role

These files already provide the authoritative raw chat and message persistence surfaces.

### Relevant symbols

- `type Chat struct`
- `type ChatMessage struct`
- `type ChatStore interface`
- `type ChatHistoryReader interface`
- `func (s *SQLiteChatStore) CreateChat(...)`
- `func (s *SQLiteChatStore) GetChat(...)`
- `func (s *SQLiteChatStore) AppendMessage(...)`
- `func (s *SQLiteChatStore) MessageCount(...)`
- `func (s *SQLiteChatStore) ListMessagesChronological(...)`

### Gap

The message store has no native compaction metadata columns yet.

### 3.2 Rolling chat summary bridge

**Primary file:** `internal/navi/summarizer.go`

### Current role

This file is the active bridge implementation for long-Chat Compaction pressure.

### Relevant symbols

- `type ChatSummarizer interface`
- `type LLMChatSummarizer struct`
- `type ChatSummary struct`
- `func NewChatSummarizer(...)`
- `func (s *LLMChatSummarizer) MaybeSummarize(...)`
- `func (s *LLMChatSummarizer) SummarizeChat(...)`
- `func (s *LLMChatSummarizer) persistSummary(...)`
- `func (l *AgentLoop) ChatSummaryBlock(...) string`

### Drift note

This path writes summary-derived memory and facts too close to compaction behavior and keeps prose summary text as the effective continuity object. That is explicitly below the target architecture.

### 3.3 Prompt assembly touchpoint

**Primary file:** `internal/navi/summarizer.go`

### Current role

`ChatSummaryBlock(...)` contributes a compacted prose block into prompt assembly.

### Gap

This is summary injection, not rehydration from Chat Frame plus Task Frames.

### 3.4 Current behavioral test anchor

**Primary files:** `internal/navi/compaction/*_test.go`, `internal/navi/store/compaction_store_test.go`

### Current role

Confirms that the current branch preserves compaction service behavior and persists chat-memory checkpoints against the chat store.

### Migration implication

These tests are useful as implementation checks but should still be measured against the acceptance criteria in the compliance bill.

## 4. Target artifacts → intended package/file ownership

## 4.1 Chat Frame and Task Frames

**Target package:** `internal/navi/compaction/`

Suggested file split:

- `types.go`
- `merge.go`
- `extractor.go`

### Current state

Not implemented.

## 4.2 Budget manager and selector

**Target package:** `internal/navi/compaction/`

Suggested file split:

- `budget.go`
- `selector.go`

### Current state

Not implemented.

## 4.3 Checkpoint persistence

**Target packages:**

- `internal/navi/compaction/` for checkpoint contracts
- `internal/navi/store/` for SQLite implementation

### Current state

Not implemented for Chat Compaction.

## 4.4 Message compaction markers

**Target package:** `internal/navi/store/`

### Current state

Not implemented.

## 4.5 Rehydration-based prompt assembly

**Target packages:**

- `internal/navi/compaction/` for rehydration contract
- `internal/navi/` for runtime integration

### Current state

Not implemented as a formal subsystem.

## 5. Current drift watchpoints

These are the first places future reviewers should inspect.

### 5.1 `persistSummary(...)`

Why it matters:

- persists memory and facts from compaction-adjacent behavior
- risks blurring online continuity with durable memory shaping

### 5.2 `ChatSummaryBlock(...)`

Why it matters:

- keeps prose summary injection alive as effective chat continuity
- easy place for engineers to keep adding fields instead of building structured state

### 5.3 chat store schema

Why it matters:

- current schema supports raw chats well
- target architecture cannot land cleanly without new chat-memory and checkpoint persistence

### 5.4 test surface bias

Why it matters:

- current tests prove the bridge behavior works
- they do not yet prove protected boundaries, checkpoints, or rehydration correctness

## 6. Recommended audit order

For the compaction branch, review in this order:

1. `internal/navi/summarizer.go`
2. `internal/navi/chat.go`
3. `internal/navi/store/chat_store.go`
4. prompt assembly code paths that consume `ChatSummaryBlock(...)`
5. any new `internal/navi/compaction/` package additions
6. new store migrations for chat memory and checkpoints
7. tests covering protected boundaries and rehydration

## 7. Non-compliance triggers

This mapping becomes stale or the branch becomes non-compliant if any of the following happens without doc updates:

- rolling summary behavior is treated as canonical chat truth
- compaction writes durable memory directly without the reflection boundary
- prompt assembly remains transcript replay plus prose summary injection
- checkpointing is skipped while messages are still marked compacted
- protected boundaries are not enforced in selection logic

## 8. Near-term mapping goal

The immediate target is not to delete the rolling summarizer first.

The immediate target is to **surround and replace it** by landing:

- structured state objects
- checkpoint storage
- message markers
- rehydration path
- protected-boundary tests

Once those exist, the rolling summarizer can be demoted to compatibility fallback and then retired.

---

[architectural contract](session-compaction-architectural-contract.md) · [ADR-009](../adr/ADR-009-session-compaction-runtime.md) · [docs INDEX](../INDEX.md)
