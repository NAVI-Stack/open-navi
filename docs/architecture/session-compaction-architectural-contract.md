# Chat Compaction Architectural Contract
**Status:** Draft Normative  
**Applies to:** `feat/compaction-system` until superseded by a ratified subsystem contract.
**Related docs:** [ADR-009](../adr/ADR-009-session-compaction-runtime.md), [spec](../specs/session-compaction-system-v1.md), [design](../design/session-compaction.md)

Historical naming note: this document previously used "session"; canonical terminology is now Chat for transcript/context and RuntimeSession for execution lifecycle.

## 1. Purpose

This document locks the architectural boundaries of the Chat Compaction subsystem so implementation work does not drift back toward rolling-summary-first behavior.

If this document conflicts with older branch behavior, this contract wins.

## 2. Architectural posture

Chat Compaction is an **online runtime subsystem** for active-chat continuity.

It is not:

- durable memory promotion
- dreaming
- generic summarization
- proposal ownership
- execution outcome ownership
- a Skill

## 3. Canonical subsystem surfaces

The subsystem must operate over these architectural surfaces:

1. raw chat messages
2. Chat Frame
3. Task Frames
4. checkpoint records
5. retrieval spans
6. authoritative proposal/failure references
7. runtime snapshots and inspection metadata as supporting, non-canonical surfaces

The subsystem does not create a second source of truth for proposals, failures, or artifacts.

## 4. Ownership boundaries

### 4.1 Raw message ownership

**Owner:** chat message store

Responsibilities:

- append messages
- preserve chronology
- expose windows/ranges for compaction
- retain compacted messages with markers

Compaction may read raw messages and mark them compacted after success. It may not delete them in v1.

### 4.2 chat continuity ownership

**Owner:** compaction subsystem

Responsibilities:

- Chat Frame
- Task Frames
- checkpoint creation
- retrieval span archival
- prompt-state rehydration inputs
- inspection metadata for compaction and rehydration decisions

### 4.3 Proposal ownership

**Owner:** proposal queue and governance runtime

Compaction may reference open proposals. It may not resolve, mutate, or reinterpret proposal authority.

### 4.4 Failure and recovery ownership

**Owner:** execution outcome and recovery runtime

Compaction may reference open failures and recovery state. It may not decide recovery closure.

### 4.5 Durable memory ownership

**Owner:** reflection and memory systems

Compaction may emit promotion candidates. It may not write durable Knowledge, Memories, Configuration, or Priorities.

### 4.6 Runtime snapshot ownership

**Owner:** runtime and execution subsystems

Compaction may consume runtime snapshots to strengthen task continuity and boundary protection.

Those snapshots are supporting inputs, not replacement truth for raw message provenance.

## 5. Required package boundaries

### 5.1 `internal/navi/compaction/`

This package owns the compaction subsystem contracts and orchestration.

Expected responsibilities:

- token estimation interface
- budget management
- segment selection
- extraction contracts
- merge logic
- checkpoint orchestration
- retrieval span generation
- rehydration contracts
- inspection metadata generation

This package should not own SQLite schema or proposal execution logic.

### 5.2 `internal/navi/store/`

This package owns SQLite-backed persistence for:

- chat messages
- chat memory rows
- checkpoint rows
- message compaction markers

### 5.3 `internal/navi/`

This package remains the runtime integration seam.

It may:

- invoke compaction before model execution
- consume rehydrated prompt state
- pass authoritative proposal/failure references into compaction inputs
- pass runtime snapshots into compaction inputs

It must not absorb compaction internals back into `AgentLoop` as scattered helper logic.

## 6. Required runtime lifecycle

The runtime contract is:

1. persist new message/event
2. estimate prompt budget
3. select compactable span if threshold crossed
4. gather supporting authoritative refs and runtime snapshots when available
5. extract structured continuity state
6. merge Chat Frame and Task Frames
7. write checkpoint
8. mark messages compacted
9. rehydrate next-turn prompt state
10. proceed to model execution

Steps 7 and 8 are atomic from a logical perspective: message markers must not be written if checkpoint persistence failed.

## 7. Protected boundaries

The subsystem must preserve these atomic boundaries:

- tool call + tool result
- proposal + resolution exchange
- open failure + active recovery exchange
- latest artifact mutation exchange
- protected minimum recent window

When grounded runtime or checkpoint refs exist, boundary protection should prefer those refs over transcript-only heuristics.

Any implementation that can split these boundaries is non-compliant.

## 8. Prompt assembly contract

Compaction outputs feed prompt assembly in this order:

1. permanent instructions
2. Chat Frame
3. active Task Frames
4. authoritative proposal/failure references
5. retrieval spans and artifact excerpts
6. Live Tail
7. new input

Rehydration is required. Transcript replay alone is non-compliant.

Trim behavior must remain inspectable. Support context is lower priority than protected recent continuity.

## 9. Migration rule

The current `internal/navi/summarizer.go` path is migration scaffolding.

The branch may continue to use it temporarily, but:

- it is not the authoritative architecture
- new code must not deepen dependence on the rolling summary object
- future reviews should treat any further investment in prose-summary canonicality as drift

## 10. Compliance criteria

The branch is architecturally compliant only if:

- Chat Frame exists as explicit structured state
- Task Frames exist as explicit structured state
- checkpoints are persisted
- raw messages remain authoritative
- proposal/failure truth stays outside compaction
- prompt assembly is rehydration-based
- protected boundaries are tested and enforced
- retrieval spans remain support context rather than primary continuity state
- compaction and rehydration decisions are inspectable through structured metadata

## 11. Relationship to implementation mapping

This contract defines the subsystem boundary and compliance rules.

The companion implementation mapping document names the current files and symbols that satisfy or violate those rules.

If the implementation mapping conflicts with this contract, this contract wins.

---

[ADR-009](../adr/ADR-009-session-compaction-runtime.md) · [spec](../specs/session-compaction-system-v1.md) · [docs INDEX](../INDEX.md)
