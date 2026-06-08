# Task spec — NAVI-CIP-P4

## Task ID

NAVI-CIP-P4

## Title

Implement Context Intake Pipeline Phase 4 — Retrieval surface, retrieval-side Distillation reuse, privacy-tier routing

## Pinned context (read first, in this order)

1. [ADR-012 — Personal Continuity Layer](../../adr/ADR-012-context-intake-and-memory-projection.md) — architectural commitment, especially D8 (privacy modes).
2. [Context Intake Pipeline V1 spec](../../specs/context-intake-pipeline-v1.md) — **focus on §6 last row (Retrieve), §8 (Distillation — note explicitly that it's reused at retrieval), §9 (privacy modes), §10 (Retrieval & routing — read in full; this is the load-bearing section for P4), §14 (P4 row), §15 (acceptance criteria #2, #4, #5 apply to P4).**
3. [Intake Synthesis Seam design note](../../design/intake-synthesis-seam.md) — §14 "Un-promoted chunks remain retrievable" is a hard P4 invariant: chunks that never became entities are still in the retrieval pool, filtered by trust/privacy.
4. [Contextual LLM Routing concept](../../concepts/contextual-llm-routing.md) — what's already shipped: `internal/llm/profiles.go` (`TaskClass`, `ModelProfile`, `ModelSelector`), `internal/navi/runtime_executor.go` classification, `GET /api/llm/profiles`. **P4 extends this with a privacy tier; it does not replace it.**
5. [Content Trust model](../../concepts/content-trust.md) — `ContentTrust` filtering and propagation through retrieval. `external_untrusted` content is data, never directives; retrieval must surface trust labels so downstream prompt assembly quote-wraps external content.
6. [Memory & World Model concept](../../concepts/memory.md) — the entity graph that hybrid retrieval traverses alongside the chunk vector store.
7. [NAVI-CIP-P1](NAVI-CIP-P1.md), [NAVI-CIP-P2](NAVI-CIP-P2.md), [NAVI-CIP-P3](NAVI-CIP-P3.md) task specs and their handoff docs — hard dependencies. Read how the pipeline actually landed: `internal/intake/` (admit/canonicalize/chunk/distill/score/embed/extract/resolve/synthesize/fold), `internal/store/intake*.go`, the embedding backend P3 chose, the Distill primitive's interface.
8. [CLAUDE.md](../../../CLAUDE.md) — build artifact policy, no-ORM rule.
9. Existing code to read before writing: `internal/intake/distill/` (the primitive you will call a second way), `internal/intake/embed/` (the embedding interface + backend P3 landed), `internal/navi/loop.go` and `internal/navi/runtime_executor.go` (the Conscious loop's Contextualize step you will feed), the existing World Model entity stores (for entity-graph traversal), `internal/llm/profiles.go` and the `ModelSelector` (what you extend).

## Description

Phase 4 of CIP, per ADR-012. With P1–P3 in place, NAVI has been *acquiring* governed, deduplicated, provenance-bearing chunks and synthesizing entities — but nothing inside NAVI yet *consumes* that context. P4 closes the loop.

Three things ship in P4:

- **Retrieval surface** — a hybrid retrieval API consumed by the Conscious loop's Contextualize step. Hybrid means: vector similarity over chunks + entity-graph traversal + recency boost, with trust/privacy filtering applied before results return.
- **Retrieval-side Distillation** — the same primitive built in P2, invoked with a context-budget argument to compress retrieved sets before they reach prompt assembly. This is what makes Distillation *infrastructure* rather than a feature: one primitive, two call sites.
- **Privacy-tier routing** — extend the existing `internal/llm/profiles.go` `ModelSelector` with a privacy tier so `secret`-class content in Local mode either routes to a local model or is **observably refused**.

P4 is also the phase where the **vector index engine choice is committed**. P3 was permitted to ship a deterministic stub or a local-Ollama backend with no index commitment. P4 must commit to a concrete engine that supports the retrieval contract (sqlite-vec, an embedded engine, or a documented external index) and migrate P3's embedding storage onto it. Document the choice and rationale in the handoff.

## What you must NOT change (Frozen Design Contract)

- **Do not duplicate the Distillation primitive.** Retrieval-side distillation must call the same primitive function from `internal/intake/distill/` that ingest-side uses. A second implementation defeats the entire point of P2's dual-use interface.
- **Do not rebuild the contextual router.** Extend `internal/llm/profiles.go` / `ModelSelector` additively. Privacy tier is a new dimension alongside existing `TaskClass`; the existing classification path keeps working.
- **Do not let `secret`-class content silently route to a cloud model.** Refusal must be observable (logged, surfaced via a typed error). A test must exercise the refusal path.
- **Do not strip provenance during retrieval.** Every returned chunk carries its source `IntakeRecord` link, fetch time, confidence, and trust label. Distillation at retrieval also preserves provenance (P2 invariant).
- **Do not discard trust labels.** `external_untrusted` content stays tagged as it flows from chunk → retrieval result → distilled context → prompt assembly. Downstream prompt code must be able to quote-wrap it.
- **Do not change the synthesizer or governor.** P3's contract is frozen — P4 reads; it does not write the World Model.
- **Do not invent a second retrieval interface for the Vault.** When the Vault lands, its retrieval needs are served by this same surface.
- **Do not invent new IPC.** Retrieval is pure Go. If the entity-extraction Python subprocess from P3 needs to be invoked for query-time NER on user prompts, reuse the existing subprocess pattern — but query-time extraction is **not required** for V1.
- **Do not over-engineer ranking.** V1 hybrid retrieval is a simple weighted combination (similarity + recency + entity-graph hits). Sophisticated reranking can iterate.
- **No Console surfaces in P4.** Surfacing retrieval state to the owner is P5.
- **No Vault retrieval code.** Vault is a parallel deliverable with its own task spec.
- **No bare `go build`.** Binaries to `bin/` via Make targets (CLAUDE.md "Build Artifact Policy").
- **No new ORM.** Raw SQL only (ADR-005).
- **Do not modify** ADR-012, CIP V1 spec, synthesis seam design note, Vault spec, P1–P3 task specs, or this task file.
- **Do not break P1–P3.** Admit, Canonicalize, Chunk, Distill, Score, Embed, Extract, Resolve, Synthesize, Fold must continue to function unchanged.

## Acceptance criteria

### Retrieval surface

- [ ] A new package (suggested `internal/intake/retrieve/`) exposes a `Retrieve(ctx, query, opts) ([]RetrievalResult, error)` function. `RetrievalResult` carries chunk content, source provenance (record id, fetch time, cursor, link-back), entity link(s) when synthesized, confidence, `ContentTrust`, and `PrivacyClass`.
- [ ] Hybrid retrieval combines: (a) vector similarity over `intake_chunks` embeddings; (b) entity-graph traversal (entities matching query terms surface their linked Knowledge/Memory chunks); (c) recency boost. V1 ranking is a documented weighted combination — keep the weights configurable.
- [ ] **Un-promoted chunks remain retrievable.** Chunks that never synthesized into entities are still in the candidate pool; tested.
- [ ] **Trust/privacy filtering is applied before results return.** Tested: a `secret`-class chunk does not appear in results for a query whose effective privacy tier doesn't permit it; `external_untrusted` chunks carry their label intact through the pipeline.
- [ ] The Conscious loop's Contextualize step (in `internal/navi/loop.go` / `runtime_executor.go`) calls `Retrieve` and incorporates results into context assembly. Existing turn-time behavior is preserved when retrieval returns nothing.

### Retrieval-side Distillation

- [ ] Retrieval results pass through the **same** `internal/intake/distill/` primitive used in P2, invoked with a context-budget argument. **One implementation, two call sites.** Tested by asserting both call sites resolve to the same exported function.
- [ ] Distilled retrieval output preserves a verifiable link from every compressed span back to the source chunk(s) and `IntakeRecord(s)` (P2 invariant).
- [ ] The retrieval-side budget is configurable per-call (the Conscious loop sets it based on the remaining context window per existing compaction patterns).

### Privacy-tier routing

- [ ] `internal/llm/profiles.go` / `ModelSelector` is extended with a `PrivacyTier` dimension. Existing `TaskClass`-based selection still works; privacy tier is an additional filter applied before the final route choice.
- [ ] Privacy modes from CIP §9 are wired into the selector: **Local** refuses cloud routes for any privacy class; **Hybrid** allows cloud per class policy; **Cloud** permits cloud routes regardless of locality.
- [ ] A `secret`-class payload in Local mode is **observably refused** (typed error or explicit fallback to a local model). Tested via a fixture that drives a `secret` chunk through retrieval → routing and asserts no cloud provider was called.
- [ ] `GET /api/llm/profiles` surfaces the privacy-tier configuration so operators can inspect the policy without code changes.

### Vector index engine

- [ ] A concrete vector index engine is committed. Acceptable: sqlite-vec extension to the existing SQLite store, an embedded Go vector library, or a documented external index. **Document the choice and rationale in the handoff.**
- [ ] P3's embedding storage migrates onto the chosen engine. Existing embeddings from P3 are preserved (re-indexable from `intake_chunks.embedding` references).
- [ ] Retrieval latency under realistic chunk volume (target: 10k chunks) is documented in the handoff.

### Hard invariants

- [ ] **No World Model writes from P4.** Extend the existing `worldmodel_isolation_test.go` pattern to assert retrieval reads only. The synthesizer remains the only writer.
- [ ] **No LLM calls from the retrieval path itself.** Distillation at retrieval remains extractive (per P2's frozen contract — abstractive backend is still interface-only). Routing decisions don't invoke LLMs; they select them.
- [ ] **Provenance survives the round-trip:** a chunk fetched via retrieval and distilled for context still resolves back to its source `IntakeRecord` and synthesized entity (if any) via the persisted links.

### Build & verification

- [ ] `make test` passes.
- [ ] `go vet ./...` clean.
- [ ] `make build-all` succeeds; both binaries land in `bin/`.

## Surface

- New: `internal/intake/retrieve/` — hybrid retrieval surface (`Retrieve`, `RetrievalResult`, ranking, trust/privacy filter)
- New: vector index integration (location depends on engine choice — likely `internal/store/intake_chunks.go` for sqlite-vec, or `internal/intake/embed/` for an embedded Go library)
- Modified: `internal/intake/embed/` — wire P3's interface to the chosen engine; migrate stored embeddings
- Modified: `internal/llm/profiles.go` — add `PrivacyTier` dimension and selector logic
- Modified: `internal/gateway/llm.go` (or wherever `/api/llm/profiles` lives) — surface privacy-tier config
- Modified: `internal/navi/loop.go` / `internal/navi/runtime_executor.go` — call `Retrieve` from the Contextualize step; thread results into context assembly
- Modified: `internal/navi/inference/` (model authorization layer) — apply privacy-tier refusal at the boundary; observable error type
- Test surface: package tests for retrieve and the routing extension, plus an end-to-end test that drives a `secret`-class chunk through the pipeline and asserts (a) it appears in retrieval results when the query's effective privacy permits, (b) it is filtered out when not, (c) routing in Local mode refuses cloud providers when the retrieved set includes `secret`, (d) the no-World-Model-writes invariant holds, (e) retrieval and ingest both call the same distill function.

## Suggested implementation order

1. Read all pinned context. CIP §10 is the load-bearing read.
2. Commit to a vector index engine. Document the choice. Land the integration and migrate P3's embeddings.
3. Build the retrieve surface against the chosen index: vector similarity first; tested standalone.
4. Add entity-graph traversal: given query terms, surface entities and pull their linked chunks. Tested.
5. Add recency boost. Document the weighted combination.
6. Apply trust/privacy filtering as the last pass before results return. Tested with fixtures across all `ContentTrust` values and `PrivacyClass` values.
7. Wire retrieval-side distillation: call the existing P2 primitive with a budget argument. Add the test that asserts both call sites resolve to the same function.
8. Extend the `ModelSelector` with `PrivacyTier`. Implement the three privacy modes (Local/Hybrid/Cloud). Add observable refusal for `secret`-in-Local. Tested.
9. Surface privacy-tier config through `/api/llm/profiles`.
10. Wire retrieval into the Conscious loop's Contextualize step. Verify existing turn-time behavior on empty-retrieval paths.
11. Extend `worldmodel_isolation_test.go` to assert retrieval is read-only.
12. End-to-end fixture test exercising all P4 invariants.

## Priority

HIGH — the value of P1–P3 is latent until P4 wires retrieval into the Conscious loop.

## Out of scope for P4 (explicit)

- Console surfaces for sync log, recent intake, grouped Proposals (P5)
- Vault retrieval / projection (parallel deliverable)
- Abstractive LLM distillation backend (deferred — P2's interface stays interface-only)
- Query-time Python NER (not required for V1; reuse pattern is available if it becomes needed later)
- Synthesizer or governor changes (P3 contract is frozen)
- Sophisticated learned reranking (V1 is a documented weighted combination)
- Deep Reflection re-scoring (handled by the existing reflection pipeline)
- Plugin-tier privacy policies (placeholder slot in the governor today; future)
- Cross-device federated retrieval (Phase 16)
