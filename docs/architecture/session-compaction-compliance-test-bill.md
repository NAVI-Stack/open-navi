# Chat Compaction Compliance Test Bill
**Status:** Draft Normative  
**Applies to:** Chat Compaction lifecycle, boundary protection, storage ordering, checkpointing, and rehydration semantics on `feat/compaction-system`.
**Related docs:** [architectural contract](session-compaction-architectural-contract.md), [implementation mapping](session-compaction-implementation-mapping.md), [spec](../specs/session-compaction-system-v1.md)

Historical naming note: this document previously used "session"; canonical terminology is now Chat for transcript/context and RuntimeSession for execution lifecycle.

## 1. Purpose

This document defines the required compliance test suite for the Chat Compaction subsystem.

A branch is non-compliant if:

- a required test is missing
- a required test fails
- a required test is weakened so drift can pass silently

## 2. Test categories

The required suite is divided into:

1. threshold and trigger tests
2. segment selection and boundary protection tests
3. continuity state merge tests
4. checkpoint and storage ordering tests
5. rehydration and prompt assembly tests
6. degradation and fallback tests
7. migration guard tests
8. static architecture tests

## 3. Threshold and trigger tests

### 3.1 `TestCompaction_BudgetManager_ComputesUsableBudget`
Assert usable budget subtracts:

- reserved output tokens
- reserved tool headroom

from max context.

### 3.2 `TestCompaction_Thresholds_SoftHardEmergency_AreOrdered`
Assert:

`soft < hard < emergency`

and all thresholds are derived from usable budget, not raw model window.

### 3.3 `TestCompaction_HardThreshold_RunsBeforeModelExecution`
Assert that when budget crosses hard threshold:

- compaction is attempted before the next model call
- the model call does not proceed first

### 3.4 `TestCompaction_UnderSoftThreshold_IsNoOp`
Assert that when budget is below soft threshold:

- no compaction run is attempted
- prompt assembly still succeeds

## 4. Segment selection and boundary protection tests

### 4.1 `TestSelector_DoesNotSplitToolCallAndResult`
Assert selector never returns a span that separates an unresolved tool call from its result.

### 4.2 `TestSelector_DoesNotSplitProposalAndResolutionExchange`
Assert selector never splits a proposal from the user response that resolves it.

### 4.3 `TestSelector_DoesNotSplitFailureAndRecoveryExchange`
Assert selector never splits an open failure from its active recovery exchange.

### 4.4 `TestSelector_DoesNotCompactLatestArtifactMutationExchange`
Assert selector protects the most recent artifact creation or mutation exchange.

### 4.5 `TestSelector_RespectsProtectedRecentWindow`
Assert selector never compacts inside the protected minimum recent window.

### 4.6 `TestSelector_ChoosesOldestEligibleContiguousSpan`
Assert selector chooses the oldest valid contiguous span once protected boundaries are excluded.

## 5. Continuity state merge tests

### 5.1 `TestMerge_ChatFrame_PreservesObjectiveUnlessSharpened`
Assert objective is not replaced by weaker wording.

### 5.2 `TestMerge_ChatFrame_PreservesOpenQuestionsUntilResolved`
Assert open questions survive until explicit grounded resolution.

### 5.3 `TestMerge_TaskFrame_UsesStableTaskIDs`
Assert Task Frames update by stable task ID, not wording similarity.

### 5.4 `TestMerge_TaskFrame_DoesNotDropBlockersSilently`
Assert blockers and dependencies are not silently removed during merge.

### 5.5 `TestMerge_ProposalAndFailureRefs_AreSourcedAuthoritatively`
Assert proposal and failure references come from authoritative stores, not inferred summary text.

## 6. Checkpoint and storage ordering tests

### 6.1 `TestCheckpoint_WriteOccursBeforeMessageMarkers`
Assert message compaction markers are not written before checkpoint persistence succeeds.

### 6.2 `TestCheckpoint_Failure_DoesNotMarkMessagesCompacted`
Assert failed checkpoint persistence leaves source messages uncompacted.

### 6.3 `TestChatMemory_VersionedWrite_RoundTrips`
Assert chat memory writes and reads round-trip with version information intact.

### 6.4 `TestCheckpoint_RoundTripsProvenance`
Assert checkpoints preserve source message IDs and compacted range boundaries.

### 6.5 `TestRawMessages_RemainQueryableAfterCompaction`
Assert compacted raw messages are still queryable after compaction completes.

## 7. Rehydration and prompt assembly tests

### 7.1 `TestRehydration_Order_UsesCanonicalSurfaces`
Assert rehydration order is:

1. permanent instructions
2. Chat Frame
3. active Task Frames
4. authoritative proposal/failure references
5. retrieval/artifact support
6. Live Tail
7. new input

### 7.2 `TestPromptAssembly_DoesNotDependOnRollingSummaryOnly`
Assert prompt assembly can function from structured continuity state without requiring the old prose summary block.

### 7.3 `TestEviction_Order_DropsRetrievalBeforeProtectedRecentContext`
Assert over-budget handling evicts retrieval/support context before shrinking protected recent context.

### 7.4 `TestRehydration_PreservesActiveProposalVisibility`
Assert open proposal references remain visible after compaction and rehydration.

### 7.5 `TestRehydration_PreservesOpenFailureVisibility`
Assert open failure/recovery references remain visible after compaction and rehydration.

## 8. Degradation and fallback tests

### 8.1 `TestCompactionFailure_GracefullyFallsBack`
Assert compaction failure:

- logs internally
- leaves chat usable
- does not surface a compaction-specific user error

### 8.2 `TestMalformedCompactionOutput_RejectsMerge`
Assert malformed structured compaction output is rejected and does not mutate chat continuity state.

### 8.3 `TestEmergencyBudgetPressure_StillProtectsAtomicBoundaries`
Assert emergency behavior still preserves atomic boundaries even if other context must be evicted.

## 9. Migration guard tests

### 9.1 `TestMigration_RollingSummaryBridge_CanCoexistWithStructuredState`
Assert branch can run with old rolling summary data present while new structured state exists.

### 9.2 `TestMigration_RollingSummary_IsNotCanonicalState`
Assert canonical continuity state is Chat Frame plus Task Frames, not the old `ChatSummary` object.

## 10. Static architecture tests

### 10.1 `TestNoCompactionPathWritesDurableMemoryDirectly`
Assert compaction code paths do not directly write durable Knowledge, Memories, Configuration, or Priorities.

### 10.2 `TestNoPromptBuilderDependsOnTranscriptReplayOnly`
Assert the prompt builder has a structured rehydration path and is not transcript replay plus prose summary only.

### 10.3 `TestNoSelectorCanSplitProtectedBoundaries`
Static or behavioral assertion that selector implementation cannot return spans violating protected-boundary rules.

## 11. Minimum blocking suite

These tests are the minimum CI gate for compaction-affecting changes:

- `TestCompaction_HardThreshold_RunsBeforeModelExecution`
- `TestSelector_DoesNotSplitToolCallAndResult`
- `TestSelector_DoesNotSplitProposalAndResolutionExchange`
- `TestSelector_DoesNotSplitFailureAndRecoveryExchange`
- `TestMerge_TaskFrame_UsesStableTaskIDs`
- `TestCheckpoint_WriteOccursBeforeMessageMarkers`
- `TestCheckpoint_Failure_DoesNotMarkMessagesCompacted`
- `TestRawMessages_RemainQueryableAfterCompaction`
- `TestRehydration_Order_UsesCanonicalSurfaces`
- `TestEviction_Order_DropsRetrievalBeforeProtectedRecentContext`
- `TestCompactionFailure_GracefullyFallsBack`
- `TestMigration_RollingSummary_IsNotCanonicalState`
- `TestNoCompactionPathWritesDurableMemoryDirectly`

## 12. Audit verdict rules

A branch is:

### `Compliant`
only if all required tests exist and pass.

### `Non-compliant`
if any required test:

- is missing
- is skipped
- is weakened to stop asserting the normative rule
- fails

## 13. Reviewer checklist

A reviewer must be able to answer yes to all:

- are trigger thresholds covered?
- are protected boundaries enforced by tests?
- is structured continuity merge behavior covered?
- is checkpoint ordering protected by tests?
- does prompt assembly use rehydration?
- do failures degrade gracefully?
- is the rolling summary bridge prevented from becoming canonical again?

If any answer is no, the branch is non-compliant.

---

[architectural contract](session-compaction-architectural-contract.md) · [implementation mapping](session-compaction-implementation-mapping.md) · [docs INDEX](../INDEX.md)
