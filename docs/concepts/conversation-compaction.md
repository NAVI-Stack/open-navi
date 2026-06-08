**Status:** Draft  
**Last Updated:** 2026-04-19  
**Updated By:** compaction design pass

# NAVI AI — Conversation Compaction

Conversation compaction is NAVI's **online continuity system** for long-running sessions.

Its job is not to make a nice summary for the user. Its job is to keep the live conversation coherent, executable, and cheap enough to continue while the raw transcript grows without bound.

Compaction sits between raw session history and prompt assembly. It reduces prompt pressure by converting older, no-longer-live conversation into structured session state and retrievable history while preserving the exact parts of the interaction that must remain verbatim.

This is **not** the same thing as:

- dreaming
- durable memory formation
- reflection tier promotion
- artifact storage
- a single rolling prose summary

Dreaming and reflection decide what becomes durable Knowledge, Memories, Configuration candidates, and long-horizon understanding. Compaction only manages **active-session continuity**.

---

## Core problem

A live agent session accumulates several different kinds of state at once:

- recent chat turns
- active tasks and sub-tasks
- tool calls and tool outputs
- proposals awaiting approval
- partial failures and recovery state
- referenced artifacts such as files, docs, and diffs
- social continuity cues

Treating all of that as one long chat log is a bad design. It raises token cost, increases latency, pollutes attention, and eventually forces the system to compress away the exact details that execution continuity depends on.

A serious compaction system therefore does **state separation first, summarization second**.

---

## The six state surfaces

NAVI should reason over six state surfaces, not one transcript blob.

| Surface | Purpose | Compaction behavior |
|---|---|---|
| Live Tail | Recent verbatim user and NAVI turns, active code/error snippets, active tool/result pairs | Kept raw and protected |
| Session Frame | Current session-wide objective, active topics, unresolved questions, commitments, short social continuity | Structured and incrementally updated |
| Task Frames | One frame per active task or subtask: objective, phase, blockers, next step, linked entities/artifacts | Structured and incrementally updated |
| Execution Ledger | Authoritative execution, proposal, retry, and recovery state | Not owned by compaction; referenced from authoritative stores |
| Retrieval History | Older transcript spans, older tool outputs, dormant reasoning, archived supporting context | Stored for selective rehydration |
| Durable Memory | Cross-session memory and world understanding | Out of scope for compaction; handled by reflection and memory systems |

The key point is blunt: **the prompt is rebuilt each turn from these surfaces**. NAVI should not depend on replaying the full transcript to know what is going on.

---

## What compaction must preserve verbatim

Some things should stay exact until they are no longer live:

- the current user ask
- the most recent turns
- exact user constraints and approvals
- active proposal language
- active failure and recovery exchanges
- exact file paths, IDs, URLs, artifact references, and proposal IDs
- active code, diffs, stack traces, logs, and shell output under discussion
- the most recent artifact mutation exchange

If these are compacted too early, NAVI will not merely lose nuance. It will lose execution correctness.

---

## What compaction should compress

Older content may be compressed when it is no longer in the Live Tail and no longer part of an unresolved atomic exchange.

Typical candidates:

- repeated explanation
- dead branches of planning
- stale assistant narration
- old tool payloads after the decision-relevant portion has been extracted
- older social banter after any still-relevant interpersonal context has been preserved
- older turns whose main value is now captured in the Session Frame, Task Frames, or Retrieval History

Compression should produce structured state plus retrievable source references, not a single prose paragraph.

---

## Compaction lifecycle

Compaction should follow this runtime pattern:

1. Persist the new event to the authoritative session history.
2. Classify the event by kind such as social, planning, coding, execution, approval, or recovery.
3. Update any authoritative execution or proposal state.
4. Estimate prompt budget across the active state surfaces.
5. If thresholds are crossed, compact the oldest eligible span.
6. Merge extracted state into the Session Frame and Task Frames.
7. Archive compacted raw spans into Retrieval History with provenance.
8. Emit promotion candidates to reflection if the span suggests durable memory candidates.
9. Rehydrate the next prompt from the rebuilt working set.

The important architectural move is step 9: **rehydration**, not transcript replay.

---

## Thresholds and protection rules

Compaction should trigger before overflow, not after.

Recommended bands:

- soft trigger around 55 to 65 percent of usable budget
- hard trigger around 70 to 80 percent
- emergency trigger around 85 to 92 percent

Protected boundaries:

- never split a tool call from its result
- never split a proposal from the user response that resolves it
- never split an open failure from the recovery exchange that follows
- never compact the currently active code or artifact mutation exchange
- never compact below the minimum recent window

Oldest-first compaction is the default, but topic pivots and atomic boundaries matter more than simple message count.

---

## Checkpoints and epochs

A session should not be treated as one undifferentiated stream. It should be divided into **epochs** separated by checkpoints.

A checkpoint records the compacted range, the resulting structured session state, and the provenance needed to reconstruct how NAVI got there.

Good checkpoint moments include:

- topic pivots
- completion of a planning branch
- proposal creation or resolution
- workflow phase changes
- tool failure or recovery transitions
- hard compaction events
- long idle gaps

Checkpoints make live continuity inspectable and make later debugging possible without replaying the entire transcript mentally.

---

## Relationship to artifacts

Artifacts reduce compaction pressure.

If a file, diff, design note, or generated document already exists as an artifact, compaction should store a reference to that artifact instead of paraphrasing or embedding its full contents into session state. Artifact references are usually higher fidelity and lower drift than repeated textual compression.

---

## Relationship to reflection and dreaming

Compaction may detect material that looks durable, such as a stable preference or a repeated behavioral pattern, but it must not write durable truth directly.

Instead it produces **promotion candidates** for the reflection pipeline.

That keeps the boundary clean:

- compaction manages active-session continuity
- reflection decides whether observations should become durable world understanding
- dreaming performs deeper reorganization and long-horizon synthesis

This boundary matters because otherwise compaction becomes an accidental memory writer.

---

## Anti-patterns

These are the failure modes to avoid:

### Single rolling summary blob

A single session summary is easy to build and easy to outgrow. It collapses tasks, approvals, failures, and topic continuity into one lossy narrative object.

### Dreaming-only continuity

If live continuity depends on a later offline process, the session will fall apart before that process runs.

### Treating transcript history as working memory

This is expensive and eventually brittle. Transcript replay is not a real state model.

### Treating tool outputs as chat text

Tool outputs are evidence and state, not just conversation.

### Compaction as user-facing behavior

The user should not have to experience the system pausing to explain that it is summarizing itself.

---

## Target mental model for NAVI

The right mental model is:

raw session history on disk,
structured session state for continuity,
authoritative execution and proposal state outside compaction,
retrieval for older supporting context,
and reflection for anything durable.

In other words, compaction is not a nicer summary system. It is part of NAVI's cognitive runtime.

---

## Companion documents

- [../design/session-compaction.md](../design/session-compaction.md) — architecture and design choices
- [../specs/session-compaction-system-v1.md](../specs/session-compaction-system-v1.md) — implementation-facing contracts
- [../tasks/session-compaction-work-plan.md](../tasks/session-compaction-work-plan.md) — engineering rollout plan
