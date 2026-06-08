# Chat Compaction Evaluation Rubric
**Status:** Draft  
**Applies to:** evaluation and regression testing after phase 2 implementation maturity on `feat/compaction-system`.
**Related docs:** [architectural contract](session-compaction-architectural-contract.md), [spec](../specs/session-compaction-system-v1.md), [ADR-009](../adr/ADR-009-session-compaction-runtime.md)

Historical naming note: this document previously used "session"; canonical terminology is now Chat for transcript/context and RuntimeSession for execution lifecycle.

## 1. Purpose

This document defines how NAVI’s Chat Compaction subsystem should be evaluated after the architecture and implementation are in place.

The goal is not to count summaries or token savings in isolation. The goal is to verify that compaction preserves continuity, correctness, and inspectability under budget pressure.

## 2. What good looks like

A good compaction run:

- preserves active task/workflow continuity
- protects atomic boundaries
- carries forward the right support context without bloating the prompt
- keeps raw messages authoritative and traceable
- explains why it compacted what it compacted
- explains why rehydration dropped what it dropped
- keeps the chat usable under pressure instead of failing abruptly

## 3. Evaluation dimensions

### 3.1 Continuity quality

Questions:

- after compaction, does NAVI still know the active objective?
- does it still know the current active tasks and their next steps?
- does it preserve blockers, dependencies, proposal refs, and failure refs where grounded?
- does it maintain continuity across multiple compaction epochs?

### 3.2 Boundary safety

Questions:

- did compaction avoid splitting tool/result bundles?
- did compaction avoid splitting proposal/resolution bundles?
- did compaction avoid splitting open failure/recovery bundles?
- did compaction avoid splitting artifact mutation sequences when refs existed?
- is selection deterministic for the same input?

### 3.3 Support-context usefulness

Questions:

- are retrieval/support spans relevant to the active chat state?
- do support spans preserve source provenance?
- does rehydration include the most useful support spans under budget?
- are support spans trimmed before protected recent continuity?

### 3.4 Inspectability

Questions:

- can an operator explain why compaction triggered?
- can an operator explain why the selected span stopped where it did?
- can an operator explain which task frames survived and why?
- can an operator explain which sections were dropped during rehydration and why?
- can an operator trace compacted continuity back to message ranges and IDs?

### 3.5 Degradation quality

Questions:

- under hard pressure, does NAVI degrade by dropping lower-value support/detail first?
- if compaction fails, does the chat remain usable?
- if budget is still exceeded, does trimming follow the intended order?

## 4. Evaluation scenario classes

### A. Simple social continuity

Long casual chats with light topical drift.

Checks:

- recent continuity survives
- older banter drops safely
- support context does not overwhelm recent social state

### B. Technical planning

Architecture/design chats with multiple branches, open questions, and explicit next steps.

Checks:

- objective survives
- multiple active tasks survive
- decision-support spans remain useful
- old resolved branches do not dominate the working set

### C. Coding/debugging

chats with logs, file paths, stack traces, multiple hypotheses, and fixes.

Checks:

- active debugging task continuity survives
- current blocker and next step survive
- support spans carry the right prior error or diff context
- compaction does not separate critical tool/result or failure/recovery sequences

### D. Proposal/approval workflows

chats with pending proposals, user approvals/denials, and governed actions.

Checks:

- proposal refs survive
- approval/resolution boundaries remain protected
- support context does not replace authoritative proposal visibility

### E. Multi-step execution / recovery

chats with tool execution, partial failure, retries, and recovery handling.

Checks:

- failure/recovery linkage survives
- recovery bundles remain protected
- rehydration keeps the right execution state visible

### F. Mixed long-running chats

Hours-or-days chats that alternate between social, planning, execution, and recovery.

Checks:

- compaction remains stable across multiple epochs
- task identities remain stable
- support spans remain relevant and traceable
- inspection metadata remains understandable

## 5. Suggested scoring

Use a simple 0–2 scale for each criterion:

- `0` = failed / missing
- `1` = partial / fragile
- `2` = acceptable / robust enough for current phase

Recommended score groups:

- continuity quality
- boundary safety
- support-context usefulness
- inspectability
- degradation quality

A scenario should not be considered passing if boundary safety fails, even if continuity looks superficially okay.

## 6. Required evidence artifacts

For each evaluation scenario, capture:

- input transcript or synthetic chat trace
- budget/trigger conditions
- selected span summary
- resulting Chat Frame
- resulting Task Frames
- retrieval/support spans generated
- rehydration sections before and after trimming
- drop reasons
- checkpoint metadata
- any failure/fallback behavior

## 7. Regression triggers

Re-run the evaluation suite when any of these change:

- selector logic
- task extraction/merge logic
- rehydration trim logic
- checkpoint metadata shape
- runtime snapshot integration
- proposal/failure boundary handling

## 8. Common failure patterns to watch for

- summary-style continuity replacing structured continuity
- task identity churn across compaction epochs
- support spans crowding out recent protected context
- lost proposal/failure linkage
- artifact mutation bundles being split
- non-deterministic selection on identical input
- inspection metadata too thin to explain behavior

## 9. Recommendation

Treat evaluation as a standing regression harness, not a one-time audit. The compaction subsystem is too easy to degrade quietly if changes are only judged by token counts or “seems to work” conversational smoke tests.

---

[architecture README](README.md) · [tasks index](../tasks/INDEX.md)
