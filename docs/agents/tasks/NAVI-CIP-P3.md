# Task spec — NAVI-CIP-P3

## Task ID

NAVI-CIP-P3

## Title

Implement Context Intake Pipeline Phase 3 — Score, Embed/Extract, Synthesize (governed), Fold

## Pinned context (read first, in this order)

1. [ADR-012 — Personal Continuity Layer](../../adr/ADR-012-context-intake-and-memory-projection.md) — architectural commitment.
2. [Intake Synthesis Seam design note](../../design/intake-synthesis-seam.md) — **read in full. This is the load-bearing document for P3.** It specifies the disposition ladder, the `MutationDescriptor`, the Go/Python split, the Proposal-grouping rules, and the idempotency contract.
3. [Context Intake Pipeline V1 spec](../../specs/context-intake-pipeline-v1.md) — **focus on §6 (stages 6–9), §6.1 (scoring placement), §13 (storage), §14 (P3 row), §15 (acceptance criteria #1, #2, #3, #4, #6, #8 apply to P3).**
4. [Governance tier resolution (GOV-04)](../../design/governance-tier-resolution.md) — the System > Owner > Plugin precedence and most-restrictive-within-tier rule that automatically applies to any `governor.Pipeline.Run` caller, including the synthesizer.
5. [Memory & World Model concept](../../concepts/memory.md) — the entity classes synthesis writes (Contacts, Knowledge, Memories, Artifacts, History at the entity-event level; Events; Proposals).
6. [Content Trust model](../../concepts/content-trust.md) — `ContentTrust` propagation through the synthesis seam.
7. [ADR-001 — Go orchestration, Python AI](../../adr/ADR-001-go-orchestration-python-ai.md) — ratified Go/Python split that this phase extends from skills to intake.
8. [Skill Python Runtime concept](../../concepts/skill-python-runtime.md) and [Skill Subprocess Runtime concept](../../concepts/skill-subprocess-runtime.md) — **reuse this pattern. Do not invent new IPC.** Specifically: `internal/navi/skill/subprocess_runtime.go` and `internal/navi/skill/python_runtime.go`.
9. [NAVI-CIP-P1](NAVI-CIP-P1.md) and [NAVI-CIP-P2](NAVI-CIP-P2.md) task specs — hard dependencies. Read the corresponding handoff docs to see how the pipeline actually landed.
10. [ADR-011 — Memory v2](../../adr/ADR-011-memory-v2.md) — the entity store you will write into.
11. [ADR-005 — SQLite single writer](../../adr/ADR-005-sqlite-single-writer.md) — storage constraints.
12. [CLAUDE.md](../../../CLAUDE.md) — build artifact policy, no-ORM rule, naming.
13. Existing code to read before writing: `internal/governor/validate.go` (Pipeline, ValidationOutcome, ActionDescriptor, ValidationResult); `internal/governor/autonomy.go` (ApplyAutonomy, HardFloorProposalRequired); P1+P2 as they actually landed in `internal/intake/` and `internal/store/`; the existing World Model entity stores (Contacts/Knowledge/Memories/Artifacts/Proposals).

## Description

Phase 3 of CIP, per ADR-012. This is the largest phase in the pipeline and the one where intake **first writes the World Model**. Goal: take the distilled chunks landed by P2 and run them through Score → Embed/Extract → **Synthesize (governed)** → Fold, producing provenance-bearing entity writes for the simple dispositions and **Proposals for the structural dispositions**, all flowing through the existing `internal/governor` engine with no parallel authority.

The Synthesize stage is the architectural commitment of ADR-012. It is implemented as **a new caller of `governor.Pipeline.Run`** via a sibling `MutationDescriptor`. The disposition ladder is the Risk rubric that feeds the engine; the engine's four `ValidationOutcome` values drive the actual write/Proposal/drop behavior. **Do not fork the Pipeline. Do not create a parallel authority surface.**

Two Python integrations land in this phase, both reusing the existing subprocess runtime pattern:

- **Entity extraction** — parse distilled chunks for entity candidates (proper-noun spans, identifiers, dates, etc.).
- **Resolution matching** — given an extracted candidate, decide if it matches an existing World Model entity. Deterministic blocking keys first; similarity for the fuzzy cases; ambiguity → `Modified` outcome that becomes a low-confidence Create plus a `possible_merge_with` tag, never a guessed merge.

**Scoring** in V1 stays deterministic Go (recency, source weight, author trust). Per CIP §6.1, the *intelligence layer* for scoring is Python, but trivial-arithmetic scoring is fine in Go for V1 — define a scoring interface that admits a Python backend later (same pattern as P2's abstractive-summary hook).

## What you must NOT change (Frozen Design Contract)

- **Do not fork `governor.Pipeline`.** Synthesis is an extension via a sibling descriptor, not a parallel engine. Two authorities is the failure mode this phase exists to prevent.
- **Do not invent a second Proposal queue.** Synthesis Proposals land in the one existing queue, tagged with their source.
- **Do not move scoring authority outside Go.** Python may *inform* via a backend interface; Go's governor remains the only writer of `ValidationOutcome`. (Same dataflow direction as resolution matching: Python → Go, never the reverse.)
- **Do not invent new IPC.** Reuse `internal/navi/skill/subprocess_runtime.go` / `internal/navi/skill/python_runtime.go` for both extraction and resolution. One subprocess pattern across all P3 Python work.
- **Do not let Python write the World Model.** Python produces structured envelopes (ExtractionResult, ResolutionResult); Go is the only writer. A test must verify this.
- **Do not bypass the governor.** A code path from a `MutationDescriptor` to a World Model write that does not first call `governor.Pipeline.Run` is a bug, not an optimization.
- **Do not override hard floors.** `MutationMergeEntities`, `MutationDeprecateEntity`, `MutationForgetMemory` must carry `HardFloorProposalRequired` and must never be auto-approved by any autonomy preset.
- **Do not discard un-promoted chunks.** Chunks that don't clear scoring or don't synthesize an entity remain in `intake_chunks` and remain retrievable in P4. Synthesis is promotion, not gating.
- **Do not select a vector index engine.** V1 ships the embedding **interface** and persists embedding references; the engine decision is deferred. A deterministic stub or local-Ollama backend is acceptable for V1; pick one and document the choice in the handoff.
- **No retrieval surface in P3.** That's P4.
- **No Vault code.** Parallel deliverable.
- **No privacy-tier routing enforcement.** That's P4. P3 inherits whatever routing exists today.
- **No Console surfaces.** That's P5.
- **No bare `go build`.** Binaries to `bin/` via Make targets (CLAUDE.md "Build Artifact Policy").
- **No new ORM.** Raw SQL only (ADR-005).
- **Do not modify** ADR-012, CIP V1 spec, synthesis seam design note, Vault spec, P1/P2 task specs, or this task file.
- **Do not break P1/P2.** Existing Admit, Canonicalize, Chunk, Distill must continue to function unchanged; P3 extends downstream.

## Acceptance criteria

### Governance extension

- [ ] A sibling `MutationDescriptor` is defined (illustrative shape in the synthesis seam doc §5); `governor.Pipeline.Run` (or a sibling entry that runs the same `Pipeline`) accepts it and returns a `ValidationResult` with the existing four `ValidationOutcome` values.
- [ ] A new effect class `world_model_mutation` is added to the governor's effect vocabulary. The existing tier resolution (System > Owner > Plugin) and restrictiveness ordering apply unchanged.
- [ ] `MutationMergeEntities`, `MutationDeprecateEntity`, `MutationForgetMemory` carry `HardFloorProposalRequired` and a test verifies no autonomy preset can auto-approve them.
- [ ] The `Modified` outcome path is exercised: an ambiguous-resolution candidate becomes a low-confidence `Create` with a `possible_merge_with` tag, **not** a guessed merge. Tested.

### Stages

- [ ] **Score (Go, deterministic V1):** per-chunk scorer using recency, source weight, and author trust; produces a retention tier and a promotion candidacy hint feeding Synthesize. Scoring interface admits a future Python backend without code changes elsewhere.
- [ ] **Embed (interface + V1 backend):** embedding interface defined; a V1 backend (deterministic stub OR local-Ollama via existing `internal/llm` Ollama provider — choose one and document) computes embeddings; embedding references persisted on or alongside `intake_chunks`. **No vector index engine commit in P3.**
- [ ] **Extract (Python):** Python subprocess produces `ExtractionResult` envelopes (entity-name spans, identifiers, dates with span-level provenance) from distilled chunks. Reuses `internal/navi/skill/subprocess_runtime.go` / `python_runtime.go` infrastructure.
- [ ] **Resolve (Python, same invocation surface as Extract OK):** Python produces `ResolutionResult{ MatchedID?, Confidence, Reason }` for each extraction candidate. Deterministic blocking keys (email, handle, normalized name, source-anchored id) before similarity; ambiguity is honestly reported (not guessed).
- [ ] **Synthesize (Go):** builds `MutationDescriptor`s from (extraction, resolution, score, content trust, privacy class, job mode), calls `governor.Pipeline.Run`, and acts on the result:
  - `Approved` → write the entity to the World Model (Contacts / Knowledge / Memories / Artifacts / History per `MutationKind`).
  - `RequiresConfirmation` → raise a Proposal.
  - `Modified` → write the softened action carried by `ModifiedAction`.
  - `Rejected` → drop and log.
- [ ] **Fold (Go, minimal V1):** a flat-or-shallow summary index by entity-type is updated after entity writes. Hierarchical depth can iterate later. The fold is derived state and is **never authoritative**.

### Proposal granularity (per synthesis seam §12)

- [ ] **Delta-mode** synthesis emits per-write Proposals.
- [ ] **Backfill-mode** synthesis batches `RequiresConfirmation` outcomes into **grouped Proposals** (e.g. "12 contact merges from <connector> backfill — review as a set"). Grouping is a property of the synthesis batch, not of the governor; the governor still produces per-`MutationDescriptor` results.

### Provenance, confidence, idempotency

- [ ] Every synthesized entity carries provenance back to its source `IntakeRecord`(s), fetch time, cursor, and a confidence value in `[0, 1]` blended from source trust, extraction confidence, and resolution confidence.
- [ ] Re-synthesizing the same `IntakeRecord` / chunk / derivation is a no-op — no duplicate entities, no duplicate Proposals. Tested via a replay scenario.
- [ ] Un-promoted chunks remain in `intake_chunks` and are not deleted by the synthesizer.

### Hard invariants

- [ ] **Synthesis is the only path that writes World Model entity tables from intake.** A test extends the existing `worldmodel_isolation_test.go` pattern to verify that Admit / Canonicalize / Chunk / Distill / Score / Embed / Extract / Resolve / Fold cause **zero** entity writes; only Synthesize does.
- [ ] **Python never writes the World Model.** A test verifies this by monitoring SQLite writes during a full pipeline pass with extraction + resolution exercised.
- [ ] **The synthesizer always passes through `governor.Pipeline.Run`.** A test verifies that disabling the governor's write-class effect path causes synthesis to drop, not write.

### Build & verification

- [ ] `make test` passes.
- [ ] `go vet ./...` clean.
- [ ] `make build-all` succeeds; both binaries land in `bin/`.
- [ ] Python integration tests run as part of `make test` (or document a separate make target if isolation is necessary).

## Surface

- New: `internal/intake/score/` — Go heuristic scoring with a Python-backend interface
- New: `internal/intake/embed/` — embedding interface + V1 backend (stub or Ollama)
- New: `internal/intake/extract/` — Go wrapper invoking the Python extraction/resolution subprocess
- New: `python/intake/` (or under the existing Python skill runtime tree if that fits more naturally) — extraction + resolution worker; structured-envelope contract (JSON in / JSON out per the existing subprocess runtime pattern)
- New: `internal/intake/synthesize/` — `MutationDescriptor` construction, governor call, disposition handling, entity writes, Proposal grouping for backfill
- New: `internal/intake/fold/` — summary-index updates (minimal in V1)
- New: `internal/store/intake_provenance.go` (or equivalent) — links between `intake_records` / `intake_chunks` and entity writes; confidence storage if not already supported by entity stores
- Modified: `internal/governor/` — add the `world_model_mutation` effect class and the `MutationDescriptor` entry into the existing `Pipeline`. **Minimal, additive changes only — do not refactor the engine.**
- Modified: `internal/intake/worker.go` — chain Score → Embed → Extract → Resolve → Synthesize → Fold after Distill
- Modified: `cmd/navid/main.go` — wire any new lifecycle hooks (Python subprocess pool warm-up, etc.)
- Test surface: package tests for each new package, plus an end-to-end test that drives a fixture record through P1+P2+P3 and asserts: (a) entities appear with provenance and confidence, (b) a merge candidate raises a Proposal, (c) a replayed pass is idempotent, (d) Python writes nothing, (e) `worldmodel_isolation_test.go` extension verifies only Synthesize writes.

## Suggested implementation order

1. Read all pinned context. The synthesis seam doc is the load-bearing read.
2. Extend the governor first: define `MutationDescriptor`, add `world_model_mutation` effect class, write unit tests proving the four outcomes work and that hard floors are unbreakable. **Land this before any World Model writes are wired.**
3. Build Score (Go heuristic) with the Python-backend interface in place but unused.
4. Build the embedding interface + V1 backend.
5. Build the Python extraction + resolution worker. Reuse the subprocess runtime. Unit-test the envelope contract.
6. Build the Synthesize stage. Wire it to the governor. Implement disposition handling end-to-end including the `Modified` outcome and the hard-floor Proposal path.
7. Implement Proposal grouping for backfill mode at the synthesizer's batch boundary (governor stays per-write).
8. Build Fold (minimal).
9. Extend the worker pipeline to chain all stages.
10. Idempotency tests (replay).
11. Extend `worldmodel_isolation_test.go` to assert Synthesize is the only writer.
12. End-to-end fixture test exercising the full P1+P2+P3 pipeline including grouped backfill Proposals.

## Priority

CRITICAL — this is where intake becomes useful, and it is where authority drift would be most dangerous if mishandled.

## Out of scope for P3 (explicit)

- Retrieve + retrieval-side distillation invocation (P4)
- Privacy-tier routing enforcement (P4)
- Vector index engine selection (deferred — interface only in P3)
- Console surfaces (P5)
- Backfill consent UX surface (P5 / Console)
- Vault (parallel deliverable, has its own spec)
- LLM-driven distillation backends (deferred — P2's interface stays interface-only)
- Deep Reflection re-scoring loop (handled by the existing reflection pipeline, not by intake)
- Plugin-tier governance for synthesis (placeholder slot in the governor today; future)
