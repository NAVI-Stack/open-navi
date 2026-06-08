# ADR-009: Chat Compaction Runtime

**Status:** Accepted  
**Date:** 2026-04-19  
**Deciders:** Eric (Owner)  
**See also:** [ADR-003 Stateless Orchestrator](ADR-003-stateless-orchestrator.md), [Design: session-compaction](../design/session-compaction.md), [Spec: session-compaction-system-v1](../specs/session-compaction-system-v1.md)

Historical naming note: this document previously used "session"; canonical terminology is now Chat for transcript/context and RuntimeSession for execution lifecycle.

---

## Context

The current branch contains a rolling chat summarizer in `internal/navi/summarizer.go`.

That implementation:

- batches older chat messages
- asks an LLM for a `ChatSummary`
- persists summary text and extracted facts into memory/facts tables
- injects an `Earlier chat summary` block into prompt assembly

That is useful as a bridge, but it is not a safe final architecture.

Its failure modes are straightforward:

- one rolling prose summary becomes pseudo-canonical chat truth
- task continuity, approvals, and recovery state are flattened into ordinary summary text
- durable memory writes happen too close to compaction behavior
- prompt assembly remains closer to transcript replay plus summary injection than to explicit state rehydration

NAVI needs a proper live-chat continuity system that preserves execution correctness and inspectability under context pressure.

---

## Decisions

### D1. Chat Compaction is a runtime subsystem, not summarization glue

Chat Compaction is a first-class runtime subsystem responsible for online continuity within an active chat.

It is not defined as:

- a user-facing summary feature
- a reflection or dreaming feature
- a Skill
- a generic memory writer

### D2. Canonical continuity state is structured, not prose

The canonical chat continuity objects are:

- `ChatFrame`
- `TaskFrame[]`
- `CompactionCheckpoint`
- `RetrievalSpan[]`

A prose summary may still be rendered for model readability, but prose is derived from structured state. It is not the source of truth.

### D3. Prompt assembly is rehydration-based

The runtime rebuilds the next-turn working set from state surfaces:

- permanent instructions
- Chat Frame
- active Task Frames
- authoritative proposal/failure references
- selected retrieval spans and artifact excerpts
- Live Tail
- new user input

The system must not depend on replaying the entire transcript plus a rolling summary.

### D4. Execution, proposal, and recovery truth stay outside compaction

Compaction may reference active proposal and failure state, but it does not own or resolve them.

The authoritative stores for:

- proposal lifecycle
- execution outcomes
- recovery state

remain outside the compaction subsystem.

### D5. Phase 1 compaction is synchronous and inline

In phase 1, compaction runs inline before the model call when the hard threshold is crossed.

This is intentional. Correctness beats latency until the subsystem is proven stable.

### D6. Rolling chat summarizer is migration scaffolding only

The current `LLMChatSummarizer` path remains a temporary migration bridge.

It must not be deepened into the final contract. New implementation work must target structured compaction state, checkpoints, and rehydration.

### D7. Target package boundary is explicit

The target package boundary is:

- `internal/navi/compaction/` for budgeting, segment selection, extraction, merge, checkpoint orchestration, and rehydration contracts
- `internal/navi/store/` for chat message persistence and compaction-related SQLite storage
- `internal/navi/` only as the integration seam where the runtime invokes compaction before model execution

This avoids burying compaction logic inside `AgentLoop` or keeping it as incidental logic inside the summarizer path.

---

## Consequences

### Consequence 1: Current rolling summary behavior becomes explicitly non-authoritative

Existing behavior may continue temporarily, but all new branch work should treat it as fallback behavior only.

### Consequence 2: New storage is required

The branch needs explicit persistence for:

- chat memory state
- compaction checkpoints
- message compaction markers

### Consequence 3: Prompt builder responsibilities change

Prompt building becomes a state rehydration step rather than transcript-plus-summary assembly.

### Consequence 4: More tests are mandatory

This subsystem cannot be trusted with happy-path tests only. The branch must test:

- threshold behavior
- protected boundaries
- checkpoint writes
- merge correctness
- over-budget eviction
- degraded fallback when compaction fails

---

## Rejected alternatives

### A. Keep the rolling summary model and harden the prompt text

Rejected because it still treats summary text as the chat continuity primitive.

### B. Rely on dreaming for long-chat continuity

Rejected because live chats need continuity before offline processes run.

### C. Keep compaction inside durable memory writes

Rejected because it blurs online continuity with long-term memory mutation.

### D. Leave package placement undefined until coding starts

Rejected because that invites branch drift and accidental logic sprawl.

---

## Status history

| Date | Change |
|---|---|
| 2026-04-19 | Accepted — checkpointed, structured Chat Compaction replaces rolling-summary-first continuity as the target runtime architecture |
