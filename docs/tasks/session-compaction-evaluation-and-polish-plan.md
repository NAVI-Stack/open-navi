# Chat Compaction — Evaluation and Polish Plan
**Status:** Active follow-up plan  
**Applies to:** post-phase-2 compaction work on `feat/compaction-system`
**Related docs:** [evaluation rubric](../architecture/session-compaction-evaluation-rubric.md), [spec](../specs/session-compaction-system-v1.md), [architectural contract](../architecture/session-compaction-architectural-contract.md)

Historical naming note: this document previously used "session"; canonical terminology is now Chat for transcript/context and RuntimeSession for execution lifecycle.

## 1. Purpose

This document defines the next follow-up work after the main compaction architecture and phase-2 quality pass are complete.

This is not another architecture phase. It is the plan for:

- evaluation
- regression hardening
- operator polish
- doc/code alignment cleanup

## 2. Goals

1. prove the subsystem behaves well on realistic long-chat traces
2. make compaction decisions easier to inspect and debug
3. tighten rough edges without reopening design
4. refresh the docs corpus so implementation and documentation agree

## 3. Work slices

### Slice A — Docs refresh

Update docs so they match the phase-2 implementation.

Targets:

- spec refresh
- architectural contract refresh
- architecture overview links if needed
- task index updates
- any stale phase-1-only wording in compaction docs

Done when:

- docs describe runtime snapshots, richer retrieval spans, and inspection metadata
- docs no longer materially lag the implementation

### Slice B — Evaluation harness

Build or strengthen an evaluation harness around scenario-based chat traces.

Targets:

- social continuity traces
- technical planning traces
- coding/debugging traces
- proposal/approval traces
- multi-step execution/recovery traces
- mixed long-running traces

Done when:

- each scenario produces evidence artifacts listed in the evaluation rubric
- evidence is captured in a stable structured shape suitable for diff-style regression review
- regression comparisons are possible after selector/merge/rehydration changes

### Slice C — Inspection polish

Improve operator/developer visibility into compaction behavior.

Targets:

- compact human-readable summaries for checkpoint inspection
- clearer drop reasons for rehydration trimming
- easier viewing of selected span, protected boundaries, and surviving task frames
- tests for inspection metadata completeness

Done when:

- a reviewer can explain why compaction triggered, what got compacted, and what got dropped without reverse-engineering raw JSON by hand
- compact human-readable inspection helpers exist alongside the structured metadata rather than replacing it

### Slice D — Implementation polish

Tighten the current subsystem without reopening core design.

Targets:

- naming cleanup where current symbols are misleading
- comment cleanup around migration scaffolding versus authoritative path
- removal of stale or redundant bridge-oriented helpers when safe
- test readability and fixture cleanup

Done when:

- the branch is easier to maintain and less confusing to future reviewers
- polish changes do not weaken existing compliance guarantees

## 4. Non-goals

Do not use this plan to:

- redesign compaction
- merge compaction with dreaming or durable memory
- replace deterministic selection with vague inference-heavy behavior
- reopen phase-1 storage architecture
- broaden into unrelated runtime cleanup

## 5. Success criteria

This follow-up is complete when:

1. docs match the phase-2 implementation closely enough to prevent drift
2. the evaluation harness can catch regressions in continuity, boundary safety, and trimming
3. inspection metadata is easier to understand and verify
4. polish changes reduce confusion without changing subsystem authority boundaries

## 6. Suggested execution order

1. docs refresh
2. evaluation harness skeleton
3. inspection polish
4. focused cleanup / naming / comments
5. final regression pass

This order avoids polish work landing on stale documentation or unvalidated behavior.

---

[tasks index](INDEX.md) · [evaluation rubric](../architecture/session-compaction-evaluation-rubric.md)
