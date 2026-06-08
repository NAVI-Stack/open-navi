**Status:** Draft  
**Last Updated:** 2026-04-19  
**Updated By:** compaction docs refresh

# Chat Compaction System V1

Implementation-facing specification for Chat Compaction on `feat/compaction-system`.

Historical naming note: this document previously used "session"; canonical terminology is now Chat for transcript/context and RuntimeSession for execution lifecycle.

This document defines the v1 contract coding teams should build against. Where it conflicts with older branch behavior, this spec wins.

---

## 1. Scope

V1 covers **online chat continuity** inside a single active chat.

V1 includes:

- prompt-budget estimation
- compaction triggers
- protected-boundary rules
- Chat Frame and Task Frame contracts
- checkpoint persistence
- raw-message compaction markers
- retrieval span persistence and selective support-context carry-forward
- prompt rehydration order and trim policy
- graceful degradation rules
- structured inspection metadata for compaction and rehydration decisions

V1 does **not** include:

- cross-chat memory promotion
- dreaming or deep reflection behavior
- multimodal semantic summarization
- artifact storage itself
- proposal resolution logic
- execution outcome storage ownership

---

## 2. Normative invariants

The system **MUST** satisfy all of the following.

1. Raw chat messages remain authoritative and append-first.
2. Compaction never deletes raw messages in v1.
3. Compaction never writes durable Knowledge, Memories, Configuration, or Priorities.
4. Compaction never resolves Proposals or recovery states.
5. Prompt assembly uses rehydration from structured state surfaces, not transcript replay alone.
6. A tool call and its result are an atomic boundary.
7. A proposal and the user response that resolves it are an atomic boundary.
8. An open failure and its active recovery exchange are an atomic boundary.
9. Chat Frame and Task Frames are structured objects with versioning.
10. Every checkpoint stores provenance back to source message IDs.
11. If compaction merge or checkpoint write fails, the consumed messages must not be marked compacted.
12. User-visible behavior must degrade silently; no compaction failure should surface as a user-facing error.
13. When runtime or checkpoint state provides grounded task/workflow linkage, task continuity should prefer that linkage over transcript-only inference.
14. Rehydration trim behavior must preserve protected recent continuity ahead of retrieval/support context.

---

## 3. State contracts

### 3.1 ChatFrame

`ChatFrame` is the chat-wide continuity object.

Required fields:

- `schema_version`
- `chat_id`
- `frame_version`
- `current_epoch_id`
- `primary_objective`
- `active_topics[]`
- `open_questions[]`
- `active_constraints[]`
- `active_commitments[]`
- `social_continuity`
- `active_artifact_refs[]`
- `active_proposal_refs[]`
- `active_failure_refs[]`
- `source_span_refs[]`

Recommended additional fields for operator inspection and richer continuity:

- structured provenance records
- current support-span refs or support-span summary pointers
- last update timestamps or epoch references where useful

Rules:

- `frame_version` increments on every successful merge.
- `primary_objective` must be present for any non-trivial chat.
- `source_span_refs` must reference archived compacted spans or raw message ranges.
- implementations may persist structured provenance alongside `source_span_refs` for operator inspection.

### 3.2 TaskFrame

`TaskFrame` is the continuity object for one active task or subtask.

Required fields:

- `task_id`
- `parent_task_id`
- `title`
- `objective`
- `status`
- `phase`
- `blockers[]`
- `dependencies[]`
- `current_plan[]`
- `completed_steps[]`
- `next_step`
- `linked_entities[]`
- `artifact_refs[]`
- `proposal_refs[]`
- `failure_refs[]`
- `source_span_refs[]`

Recommended additional fields for grounded carry-forward:

- `run_id`
- `checkpoint_id`
- `status_source`
- `identity_source`
- structured provenance records
- `last_updated_at`

Allowed statuses:

- `active`
- `blocked`
- `awaiting_user`
- `paused`
- `done`
- `abandoned`

Rules:

- `task_id` must be stable across compaction passes.
- `next_step` must be empty only when the task is `done` or `abandoned`.
- when runtime or checkpoint state exists, task identity and task-state carry-forward should prefer that authoritative linkage over transcript-only inference.
- proposal and failure refs should remain attached to the correct task frame when grounded linkage exists.

### 3.3 CompactionCheckpoint

Append-only record for each completed compaction event.

Required fields:

- `checkpoint_id`
- `chat_id`
- `epoch_id`
- `trigger_class`
- `compacted_message_start_id`
- `compacted_message_end_id`
- `chat_frame_version`
- `task_frame_versions`
- `created_at`
- `estimator_snapshot`
- structured selection trace and source provenance sufficient to explain why the compacted span stopped where it did

Recommended additional fields:

- selected span metadata
- task survival records
- retrieval span IDs or summaries
- trim/rehydration inspection metadata pointers when available

Allowed trigger classes:

- `soft`
- `hard`
- `emergency`
- `manual`
- `idle_boundary`
- `topic_pivot`

### 3.4 RetrievalSpan

Archived compacted source material for selective rehydration.

Required fields:

- `span_id`
- `chat_id`
- `checkpoint_id`
- `kind`
- `source_message_ids[]`
- `excerpt`
- `artifact_refs[]`
- `tags[]`
- `created_at`

Recommended additional fields:

- `support_class`
- `related_task_ids[]`
- `priority`
- structured provenance

Allowed kinds:

- `conversation`
- `tool_output`
- `decision_support`
- `social`
- `artifact_context`

Rules:

- retrieval spans are support context, not primary continuity state
- rehydration may include multiple retrieval spans when useful, but must trim them before protected recent continuity

### 3.5 RuntimeSnapshot input surface

Implementations may consume runtime snapshots as a grounded input surface for compaction.

Examples:

- active run IDs
- open proposal refs
- open failure/recovery refs
- artifact mutation refs
- workflow or task progress snapshots

Rules:

- runtime snapshots may strengthen task continuity and boundary protection
- runtime snapshots do not replace raw message provenance; they augment compaction decisions and carry-forward

### 3.6 Inspection metadata

Implementations should emit structured inspection metadata for operator debugging and evaluation.

Examples:

- trigger reason
- selected span summary
- boundary stop reasons
- dropped rehydration sections and drop reasons
- task survival records
- support spans generated

Inspection metadata is not user-facing continuity state. It exists for auditability and evaluation.

---

## 4. Persistence contract

### 4.1 Table: `navi_chat_memory`

One row per chat.

Minimum columns:

- `chat_id` text primary key
- `schema_version` integer not null
- `memory_version` integer not null
- `current_epoch_id` text not null
- `chat_frame_json` text not null
- `task_frames_json` text not null
- `retrieval_spans_json` text not null
- `updated_at` datetime not null

Implementations may also persist richer structured continuity metadata inside these JSON documents.

### 4.2 Table: `navi_chat_memory_checkpoints`

Append-only.

Minimum columns:

- `checkpoint_id` text primary key
- `chat_id` text not null
- `epoch_id` text not null
- `trigger_class` text not null
- `compacted_message_start_id` text not null
- `compacted_message_end_id` text not null
- `checkpoint_json` text not null
- `created_at` datetime not null

### 4.3 Table: `navi_chat_messages` additions

Minimum added columns:

- `compacted_at` datetime nullable
- `compacted_checkpoint_id` text nullable
- `compacted_epoch_id` text nullable

Rules:

- These fields are written only after successful merge and checkpoint persistence.
- Messages with null `compacted_at` remain eligible for future selection unless protected by the recent window.

---

## 5. Trigger contract

The implementation must support three automatic thresholds.

Default configuration:

- `soft_threshold_pct = 0.60`
- `hard_threshold_pct = 0.75`
- `emergency_threshold_pct = 0.90`
- `minimum_recent_pairs = 4`
- `default_recent_pairs = 10`

Usable budget formula:

`usable = max_context_tokens - reserved_output_tokens - reserved_tool_headroom`

Rules:

- At soft threshold, compaction may run if an eligible span exists.
- At hard threshold, compaction must run before the next model call.
- At emergency threshold, compaction must run and the rehydrator must also reduce retrieval pressure if needed.
- trigger reason should be inspectable through checkpoint or run inspection metadata.

---

## 6. Segment selection contract

The selector must choose the **oldest eligible contiguous span**.

A message is ineligible if it belongs to:

- the protected recent window
- an unresolved tool/result pair
- an unresolved proposal/confirmation exchange
- an open failure/recovery exchange
- the latest artifact mutation exchange

Selection rules:

1. Walk oldest to newest.
2. Stop before any protected boundary.
3. Prefer spans that end on a topic or workflow boundary.
4. Never compact a span shorter than one complete conversational exchange unless under emergency mode.
5. When grounded runtime or checkpoint refs exist, boundary protection should prefer those refs over transcript-only content heuristics.
6. Selection should remain deterministic for the same message set and runtime snapshot input.

---

## 7. Merge contract

### ChatFrame merge

- `primary_objective` updates only when the newer extraction sharpens or replaces the active objective.
- `active_topics` are additive unless explicitly retired by later grounded state.
- `open_questions` persist until resolved by later grounded evidence.
- `active_commitments` persist until completed, cancelled, or superseded.

### TaskFrame merge

- Match by stable `task_id`.
- Replace task-local plan state with the newest grounded extraction for that task.
- Do not silently remove blockers or dependencies.
- Mark `done` only when the compacted span or authoritative execution state supports closure.
- when authoritative task updates exist, prefer those updates over weaker transcript-only field guesses.

### RetrievalSpan merge

- merge by stable span identity when available
- preserve highest-priority and richest provenance view
- retrieval spans remain support context and must not overwrite primary continuity state

### External references

- proposal references are synchronized from authoritative proposal state
- failure references are synchronized from authoritative execution state
- artifact references are copied by reference, not by paraphrased expansion

---

## 8. Prompt rehydration contract

The rehydrator must assemble the model working set in this order:

1. permanent instructions
2. persona and policy surfaces
3. Chat Frame
4. active Task Frames
5. active proposal and failure references
6. selected retrieval spans and artifact excerpts
7. Live Tail
8. new input

Eviction order when still over budget:

1. retrieval spans
2. lower-value resolved task detail
3. lower-value chat detail
4. Live Tail reduction down to protected minimum

Permanent instructions are never evicted.

Implementations should record structured trim metadata for dropped sections so operators can inspect why support context or lower-priority detail was removed.

---

## 9. Failure contract

| Failure | Required behavior |
|---|---|
| compactor call failure | log and continue without compaction |
| malformed compactor output | reject merge, log raw response, continue without compaction |
| checkpoint persistence failure | do not mark messages compacted |
| budget still exceeded after compaction | apply rehydration eviction order |

The user must not see a compaction-specific error message.

---

## 10. Recommended interfaces

Minimum required interfaces:

- `TokenEstimator`
- `BudgetManager`
- `SegmentSelector`
- `Compactor`
- `ChatMemoryStore`
- `CheckpointStore`
- `Rehydrator`
- `PromotionBridge`

Common phase-2 supporting interfaces or input surfaces may also include:

- runtime snapshot providers
- inspection metadata emitters or serializers

The exact package layout is implementation-defined, but these responsibilities are not optional.

---

## 11. Migration rule for current branch behavior

The branch currently includes rolling chat summary behavior.

Migration requirement:

- existing summary behavior may remain as a temporary fallback
- new implementation work must target Chat Frame, Task Frames, checkpoints, retrieval spans, and inspection metadata as the stable contract
- no new code may deepen dependence on a single prose chat summary object as canonical truth

---

## 12. Phase 1 acceptance criteria

Phase 1 is complete only when all of the following are true.

1. Chat Frame persists across turns in the active chat.
2. At least one Task Frame can persist across turns in the active chat.
3. Hard-threshold compaction runs before the next model call.
4. Protected boundaries are never split in tests.
5. Checkpoints are written for completed compaction passes.
6. Raw messages remain queryable after compaction.
7. Prompt assembly uses rehydration and can run without replaying the full transcript.
8. Open proposal and open failure references survive compaction.
9. Compaction failure leaves the chat usable.
10. Rolling-summary-only behavior is no longer the primary contract.

---

## 13. Recommended follow-on evaluation focus

After phase 1 and phase 2 implementation maturity, teams should evaluate:

- long-chat continuity quality
- task/workflow continuity carry-forward across multiple compaction epochs
- boundary safety under mixed tool/proposal/recovery traces
- support-span usefulness under budget pressure
- operator inspection completeness for debugging compaction choices

---

## 14. Required test coverage

Minimum required tests:

- threshold trigger unit tests
- protected-boundary selection tests
- merge tests for Chat Frame and Task Frames
- checkpoint persistence tests
- message marker write-after-success tests
- prompt rehydration order tests
- over-budget eviction tests
- compaction failure fallback tests
- migration-path tests for chats with existing rolling summary data
- task continuity survival tests
- retrieval span provenance tests
- structured continuity authority tests for compaction-enabled runs

---

## 15. Companion documents

- [../concepts/conversation-compaction.md](../concepts/conversation-compaction.md)
- [../design/session-compaction.md](../design/session-compaction.md)
- [../tasks/session-compaction-work-plan.md](../tasks/session-compaction-work-plan.md)
