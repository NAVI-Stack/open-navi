# Chat Compaction

*Status: evolving — implementation target for `feat/compaction-system`.*  
*Parent: [Conceptual Design Overview](../canonical/conceptual-design-overview.md)*

Historical naming note: this document previously used "session"; canonical terminology is now Chat for transcript/context and RuntimeSession for execution lifecycle.

---

## Purpose

Chat Compaction is NAVI's architecture for preserving **live chat continuity** under prompt budget pressure without collapsing the system into one rolling summary.

It is part of cognition infrastructure, not a Skill. Skills are invoked by Decide. Compaction sits in the prompt-assembly path and prepares the working set that cognition sees.

This design exists to prevent three specific failures:

1. transcript replay as the only context strategy
2. a single prose summary becoming de facto chat truth
3. execution state, approvals, and recovery state being compacted away with ordinary chat history

---

## Architectural boundary

Chat Compaction is allowed to manage chat-scoped continuity state.

Chat Compaction is **not** allowed to:

- write durable Knowledge directly
- write Memories directly
- modify Configuration or Priorities
- resolve Proposals
- own execution truth
- replace artifacts with paraphrases when artifact references exist

Compaction may produce promotion candidates for reflection. It may not promote them itself.

---

## Target runtime model

The runtime should be treated as six cooperating surfaces:

| Surface | Owner | Role in prompt assembly |
|---|---|---|
| Permanent instructions | system configuration | always included |
| Chat Frame | compaction subsystem | compact chat-wide continuity |
| Task Frames | compaction subsystem | compact active work continuity |
| Execution Ledger | authoritative runtime stores | active proposals, failures, recoveries, workflow state |
| Retrieval History | compaction subsystem plus retrieval | older supporting spans brought back on demand |
| Live Tail | chat message store | exact recent turns and active atomic exchanges |

The prompt is rebuilt from these surfaces every turn.

That means the design goal is **rehydration**, not transcript replay.

---

## Core components

| Component | Responsibility |
|---|---|
| Budget Manager | estimates prompt usage and decides whether compaction is needed |
| Segment Selector | chooses the oldest eligible raw span that may be compacted |
| State Extractors | derive Chat Frame and Task Frame updates from compacted spans |
| Checkpoint Writer | records epoch checkpoints and raw-to-compact provenance |
| Retrieval Archiver | stores compacted source spans for later rehydration |
| Rehydrator | rebuilds the next-turn working set from current state surfaces |
| Promotion Bridge | emits durable-memory candidates to reflection |

Compaction should be synchronous in phase 1 so the next turn sees the latest committed chat state.

---

## Canonical state objects

### Chat Frame

The Chat Frame carries chat-wide continuity that is still relevant but no longer needs to stay verbatim.

Required fields:

- chat_id
- frame_version
- current_epoch_id
- primary_objective
- active_topics
- open_questions
- active_constraints
- active_commitments
- social_continuity
- active_artifact_refs
- active_proposal_refs
- active_failure_refs
- source_span_refs

The Chat Frame is small and current. It is not a bucket for every historical detail.

### Task Frame

A Task Frame carries the continuity of one active task or subtask.

Required fields:

- task_id
- parent_task_id
- title
- objective
- status
- phase
- blockers
- dependencies
- current_plan
- completed_steps
- next_step
- linked_entities
- artifact_refs
- proposal_refs
- failure_refs
- source_span_refs

Task Frames are the primary antidote to losing execution continuity in long technical conversations.

### Compaction Checkpoint

A checkpoint marks the boundary between one live epoch and the next.

Required fields:

- checkpoint_id
- chat_id
- epoch_id
- compacted_message_start_id
- compacted_message_end_id
- chat_frame_version
- task_frame_versions
- created_at
- trigger_class
- estimator_snapshot

### Retrieval Span

A Retrieval Span stores compacted source material outside the main prompt path but keeps it available for later selective rehydration.

Required fields:

- span_id
- chat_id
- checkpoint_id
- kind
- source_message_ids
- excerpt
- artifact_refs
- tags
- created_at

---

## Raw message ownership

Raw messages remain authoritative and append-only.

Compaction never deletes them in phase 1. It only marks them with compaction metadata so they can be excluded from the Live Tail and found again for retrieval or audit.

Add message-level metadata for:

- compacted_at
- compacted_checkpoint_id
- compacted_epoch_id

This is a projection, not a rewrite.

---

## Trigger policy

Recommended defaults:

- soft trigger: 60 percent of usable prompt budget
- hard trigger: 75 percent
- emergency trigger: 90 percent

Usable prompt budget means:

`max_context - reserved_output - reserved_tool_headroom`

The threshold gap is intentional because token estimation will be approximate in phase 1.

---

## Protected boundaries

The Segment Selector must never compact across these boundaries:

- an unresolved tool call and its result
- a proposal and the user response that resolves it
- an open failure and its active recovery exchange
- the most recent artifact mutation exchange
- the minimum protected recent window

These protections matter more than raw age.

---

## Compaction unit

Compaction should operate on a **contiguous eligible span** of raw messages.

Selection rules:

1. oldest eligible span first
2. stop at the first protected boundary
3. prefer topic-complete spans over arbitrary message counts
4. compact only spans whose important live details can be represented by Chat Frame, Task Frames, artifact refs, and retrieval spans

This avoids the failure mode where half of an execution exchange is compressed while the other half remains live.

---

## Rehydration order

The Rehydrator should assemble the next-turn working set in this order:

1. permanent instructions
2. persona and policy surfaces
3. Chat Frame
4. active Task Frames
5. active proposal and failure references from authoritative stores
6. selected retrieval spans and artifact excerpts
7. Live Tail
8. new user input

If the prompt is still over budget after compaction, eviction priority should be:

1. retrieval spans
2. older resolved Task Frame detail
3. lower-value Chat Frame detail
4. Live Tail reduction down to the protected minimum

Permanent instructions are never evicted.

---

## Merge rules

Compaction merges structured state, not prose.

### Chat Frame merge

- keep the current objective unless the new span clearly sharpens or changes it
- append new active topics; retire topics only when no active Task Frame or unresolved reference still points to them
- open questions remain until explicitly answered or closed by a later checkpoint
- commitments persist until completed, cancelled, or superseded

### Task Frame merge

- match by stable task_id, not by wording similarity
- replace current plan, completed steps, and next step with the newest grounded extraction for that task
- do not silently drop blockers or dependencies
- close a task only when the compacted span or authoritative execution state supports closure

### Execution references

Proposal and failure state are synchronized from authoritative stores, not inferred from compacted prose.

---

## Checkpoint policy

Write a checkpoint when any of the following occurs:

- hard compaction trigger fired
- major topic pivot
- workflow phase transition
- proposal creation or resolution
- failure or recovery state transition
- long idle boundary

Checkpoint cadence should favor inspectability over minimal writes.

---

## Storage model

Phase 1 should persist compacted state explicitly rather than hiding it inside generic memory rows.

Recommended additions:

### `navi_chat_memory`

One row per active chat memory state.

Fields:

- chat_id primary key
- schema_version
- memory_version
- current_epoch_id
- chat_frame_json
- task_frames_json
- updated_at

### `navi_chat_memory_checkpoints`

Append-only checkpoint records.

Fields:

- checkpoint_id primary key
- chat_id
- epoch_id
- compacted_message_start_id
- compacted_message_end_id
- checkpoint_json
- created_at

### `navi_chat_messages` additions

- compacted_at
- compacted_checkpoint_id
- compacted_epoch_id

This preserves raw history while making compaction state auditable and queryable.

---

## Failure handling

Compaction failure should degrade gracefully and remain invisible to the user.

| Failure | Behavior |
|---|---|
| compactor call fails | log, skip compaction for this turn, retry later |
| malformed structured output | log, reject merge, retry later |
| checkpoint write fails | do not mark messages compacted; keep turn live |
| estimator undershot budget | evict retrieval first, then lower-value compacted detail, then shrink Live Tail to protected minimum |

Never pretend compaction succeeded if checkpointing or merge did not complete.

---

## Migration note for this branch

The existing rolling chat summary behavior in the branch is a **migration bridge**, not the target architecture.

It may continue to exist temporarily to keep the branch functional, but coding teams should not treat a single chat summary block as the final contract. The target contract is:

- Chat Frame
- Task Frames
- checkpointed provenance
- retrieval spans
- authoritative proposal and failure references

Any implementation work that deepens the rolling-summary approach instead of moving toward these state surfaces is design drift.

---

## Phase scope

### Phase 1

Ship the architecture skeleton:

- budget manager
- segment selector
- Chat Frame
- Task Frames
- checkpoint store
- raw message compaction markers
- synchronous compaction path
- rehydration-based prompt assembly

### Phase 2

Add richer extraction and retrieval:

- retrieval span indexing
- artifact-aware extraction
- smarter topic and segment detection
- post-compaction validation hooks

### Phase 3

Bridge into reflection and longer-lived memory shaping:

- promotion candidate routing
- cross-chat promotion rules
- tighter integration with durable memory retrieval

---

## What this design explicitly rejects

- one rolling prose summary as canonical chat truth
- dreaming as the mechanism for live continuity
- transcript replay as the primary context strategy
- storing raw tool payloads inside compacted memory state
- paraphrasing artifacts that already exist as addressable objects
- treating proposal or recovery state as ordinary chat summary material

---

## Companion documents

- [../concepts/conversation-compaction.md](../concepts/conversation-compaction.md)
- [../specs/session-compaction-system-v1.md](../specs/session-compaction-system-v1.md)
- [../tasks/session-compaction-work-plan.md](../tasks/session-compaction-work-plan.md)
