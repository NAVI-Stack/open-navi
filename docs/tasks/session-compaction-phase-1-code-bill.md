# Chat Compaction Phase 1 Code Bill
**Status:** Active build bill  
**Applies to:** `feat/compaction-system` phase 1 implementation
**Related docs:** [ADR-009](../adr/ADR-009-session-compaction-runtime.md), [architectural contract](../architecture/session-compaction-architectural-contract.md), [spec](../specs/session-compaction-system-v1.md)

Historical naming note: this document previously used "session"; canonical terminology is now Chat for transcript/context and RuntimeSession for execution lifecycle.

## 1. Purpose

This document defines the ordered phase-1 implementation bill for the Chat Compaction subsystem.

It is not a brainstorm list. It is the required build sequence.

A coding team should be able to pick this up and implement without inventing subsystem order, storage shape, or package boundaries.

## 2. Phase-1 objective

Land a working online compaction path that:

- preserves raw chat history
- creates structured continuity state
- writes checkpoints
- respects protected boundaries
- rebuilds prompt state from structured continuity plus recent verbatim context

Phase 1 does **not** need full retrieval intelligence, multimodal handling, or cross-chat memory promotion.

## 3. Required package targets

### 3.1 `internal/navi/compaction/`

Create this package.

Required files:

- `types.go`
- `budget.go`
- `selector.go`
- `merge.go`
- `checkpoint.go`
- `rehydrate.go`
- `service.go`

Required responsibilities:

- subsystem types
- budget management
- segment selection
- merge rules
- checkpoint orchestration
- prompt-state rehydration
- top-level service orchestration

### 3.2 `internal/navi/store/`

Extend this package.

Required files or modifications:

- schema migration additions for chat memory/checkpoints/message markers
- chat memory store implementation
- checkpoint store implementation
- message compaction marker update helpers

### 3.3 `internal/navi/`

Modify integration points only.

Required responsibilities:

- call compaction service before model execution when thresholds require it
- hand prompt assembly over to rehydration-aware path
- pass authoritative proposal/failure references into compaction input

Do not bury subsystem internals back into `AgentLoop`.

## 4. Ordered implementation tasks

## Task 1 — Define subsystem types

Create:

- `ChatFrame`
- `TaskFrame`
- `CompactionCheckpoint`
- `RetrievalSpan` placeholder type
- `BudgetSnapshot`
- `SelectionResult`
- `RehydrationInput`
- `RehydrationOutput`

Done when:

- types compile
- version fields exist where required
- no prose summary struct is used as canonical continuity state

## Task 2 — Add persistence schema

Add new persistence surfaces:

### `navi_chat_memory`
Minimum columns:
- `chat_id`
- `schema_version`
- `memory_version`
- `current_epoch_id`
- `chat_frame_json`
- `task_frames_json`
- `updated_at`

### `navi_chat_memory_checkpoints`
Minimum columns:
- `checkpoint_id`
- `chat_id`
- `epoch_id`
- `trigger_class`
- `compacted_message_start_id`
- `compacted_message_end_id`
- `checkpoint_json`
- `created_at`

### `navi_chat_messages` additions
- `compacted_at`
- `compacted_checkpoint_id`
- `compacted_epoch_id`

Done when:

- migrations apply cleanly on fresh and existing dev DBs
- read/write round-trip tests pass

## Task 3 — Implement stores

Implement:

- `ChatMemoryStore`
- `CheckpointStore`
- message compaction marker update helper

Store contract requirements:

- get current chat memory
- optimistic version-aware put/update
- append checkpoints
- mark message ranges compacted after checkpoint success

Done when:

- stores support deterministic round trips
- failed writes do not partially mark messages compacted

## Task 4 — Implement budget manager

Implement:

- token estimator interface
- default conservative estimator
- usable budget calculation
- soft/hard/emergency trigger evaluation

Phase-1 default is approximate estimation. Do not block phase 1 on provider-specific tokenizers.

Done when:

- threshold tests pass
- usable budget excludes reserved output and tool headroom

## Task 5 — Implement segment selector

Implement oldest-eligible contiguous span selection.

Protected boundaries:

- tool call + tool result
- proposal + resolution exchange
- open failure + active recovery exchange
- latest artifact mutation exchange
- protected recent window

Done when:

- selector unit tests prove these boundaries are never split
- hard-threshold path can always either return a valid segment or signal safe fallback

## Task 6 — Implement continuity merge logic

Implement merge rules for:

- Chat Frame
- Task Frames
- authoritative proposal/failure reference sync

Phase 1 merge rules must be deterministic and explicit. No fuzzy text matching as the primary identity mechanism.

Done when:

- merge tests prove stable IDs survive updates
- task status and next-step updates work predictably

## Task 7 — Implement checkpoint orchestration

Implement the logical transaction order:

1. select eligible span
2. produce structured continuity update
3. merge chat state
4. write checkpoint
5. mark messages compacted

Rules:

- if checkpoint write fails, messages stay uncompacted
- if merge fails, nothing downstream writes

Done when:

- transaction-order tests pass

## Task 8 — Implement rehydration path

Implement prompt-state assembly from:

1. permanent instructions
2. Chat Frame
3. active Task Frames
4. authoritative proposal/failure references
5. retrieval placeholders or artifact excerpts
6. recent uncompacted messages
7. new input

Done when:

- prompt builder no longer depends on transcript replay plus prose summary only
- over-budget eviction order is implemented

## Task 9 — Integrate into runtime

Integrate compaction service into the pre-model path.

Requirements:

- evaluate thresholds before model execution
- run compaction inline on hard threshold
- allow no-op under threshold
- preserve user-visible continuity on compaction failure

Done when:

- runtime path works with and without compaction firing

## Task 10 — Demote rolling summary bridge

Keep `internal/navi/summarizer.go` only as migration fallback.

Requirements:

- do not expand its authority
- do not treat its `ChatSummary` as canonical continuity state
- ensure new path can coexist during migration

Done when:

- structured compaction path is the primary contract
- rolling summary path is clearly compatibility-only

## 5. Explicit non-goals for phase 1

Do not spend phase-1 time on:

- cross-chat memory promotion
- advanced retrieval ranking
- multimodal semantic extraction
- async compaction execution
- artifact semantic diffing
- compaction-driven durable memory writes

## 6. Required code review checks

Every PR touching this subsystem must answer yes to all:

- does it preserve raw messages as authoritative?
- does it avoid prose-summary canonicality?
- does it respect protected boundaries?
- does it keep proposal/failure truth outside compaction?
- does it preserve logical write ordering for checkpoint then message markers?
- does it move the branch toward rehydration instead of summary injection?

If any answer is no, the PR is drift.

## 7. Exit criteria for phase 1

Phase 1 is complete only when:

1. structured chat continuity state exists in code and storage
2. checkpoints persist successfully
3. raw messages are marked compacted only after successful checkpointing
4. hard-threshold compaction runs before model execution
5. protected-boundary tests pass
6. prompt rehydration uses Chat Frame and Task Frames
7. the rolling summary bridge is no longer the primary contract

---

[task index](INDEX.md) · [architectural contract](../architecture/session-compaction-architectural-contract.md) · [compliance bill](../architecture/session-compaction-compliance-test-bill.md)
