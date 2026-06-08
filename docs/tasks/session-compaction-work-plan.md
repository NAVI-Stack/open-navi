# Chat Compaction — Engineering Work Plan

*Status: active working backlog for `feat/compaction-system`.*

Historical naming note: this document previously used "session"; canonical terminology is now Chat for transcript/context and RuntimeSession for execution lifecycle.

This is the execution document for engineering teams. It is intentionally not a duplicate of the concept or design docs.

Source documents:

- `docs/concepts/conversation-compaction.md`
- `docs/design/session-compaction.md`
- `docs/specs/session-compaction-system-v1.md`

---

## 1. Goal

Replace rolling-summary-first chat continuity with a checkpointed, structured compaction system built around:

- Chat Frame
- Task Frames
- checkpoint persistence
- raw-message compaction markers
- prompt rehydration

---

## 2. Delivery slices

### Slice A — Storage and contracts

Ship first:

- add `navi_chat_memory`
- add `navi_chat_memory_checkpoints`
- add compaction columns to `navi_chat_messages`
- define Chat Frame and Task Frame structs
- define store interfaces for chat memory and checkpoints

Done when:

- migrations apply cleanly
- chat memory can be written and read back
- checkpoints can be appended and queried
- raw messages remain intact

### Slice B — Budgeting and selection

Ship next:

- implement budget manager
- implement token estimator
- implement soft, hard, and emergency thresholds
- implement protected-boundary rules
- implement oldest-eligible segment selection

Done when:

- selection tests prove tool/result, proposal, and recovery boundaries are never split
- hard threshold reliably forces compaction before model call

### Slice C — Structured compaction path

Ship next:

- implement compactor interface
- produce Chat Frame updates
- produce Task Frame updates
- produce checkpoint payloads
- mark messages compacted only after successful merge and checkpoint write

Done when:

- Chat Frame persists meaningful objective and open state across long chats
- at least one active Task Frame survives compaction and rehydration

### Slice D — Rehydration and prompt assembly

Ship next:

- assemble prompt from state surfaces instead of transcript replay alone
- add retrieval-span placeholder path even if retrieval indexing is minimal in phase 1
- implement over-budget eviction order

Done when:

- prompt builder works from Chat Frame plus Task Frames plus Live Tail
- retrieval pressure is evicted before protected recent context

### Slice E — Migration and cleanup

Ship next:

- keep existing rolling summary behavior only as a migration fallback
- ensure new code paths do not depend on a single prose summary block
- remove duplicate placeholder docs and stale assumptions

Done when:

- rolling-summary-only behavior is no longer the primary path
- branch docs and runtime behavior point at the same target contract

---

## 3. Required implementation rules

Teams must not:

- write durable memory directly from compaction
- store canonical state as one prose blob
- compact across unresolved atomic exchanges
- paraphrase artifacts when artifact refs exist
- mark messages compacted before merge and checkpoint success

Teams must:

- keep raw history authoritative
- keep proposal and failure truth in authoritative stores
- preserve provenance from compacted state back to source message IDs
- test degradation paths, not just success paths

---

## 4. Test matrix

Minimum test groups:

- migration tests for chats with existing rolling summary data
- threshold tests
- segment-selector boundary tests
- merge tests for Chat Frame and Task Frames
- checkpoint persistence tests
- compaction marker ordering tests
- prompt rehydration order tests
- emergency over-budget tests
- failure fallback tests

---

## 5. Exit criteria for branch readiness

The branch is ready for implementation review when:

1. concept, design, spec, and task docs agree on the state model
2. storage contracts are settled
3. protected-boundary rules are implemented and tested
4. Chat Frame and Task Frames exist in code
5. checkpoints are persisted
6. prompt assembly is rehydration-based
7. rolling-summary-only behavior is no longer treated as the target design

---

## 6. Recommended sequencing

1. migrations and structs
2. stores and interfaces
3. budget manager
4. segment selector
5. compactor skeleton
6. checkpoint writer
7. prompt rehydrator
8. migration fallback cleanup
9. broad test pass

This order minimizes rework and prevents the team from coding extraction logic against unstable storage or prompt contracts.
