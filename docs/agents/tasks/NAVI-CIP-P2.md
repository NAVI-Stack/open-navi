# Task spec — NAVI-CIP-P2

## Task ID

NAVI-CIP-P2

## Title

Implement Context Intake Pipeline Phase 2 — Canonicalize + Chunk + Distill (extractive primitive), `intake_chunks` store

## Pinned context (read first, in this order)

1. [ADR-012 — Personal Continuity Layer](../../adr/ADR-012-context-intake-and-memory-projection.md) — architectural commitment.
2. [Context Intake Pipeline V1 spec](../../specs/context-intake-pipeline-v1.md) — full pipeline contract. **Focus on §6 (stages 3, 4, 5), §8 (Distillation — read in full; this is the heart of P2), §13 (storage), §14 (P2 row), §15 (acceptance criteria #2, #4 apply to P2).**
3. [NAVI-CIP-P1 task spec](NAVI-CIP-P1.md) — **hard dependency**. P2 extends the pipeline downstream of P1's Admit stage and reuses its `intake_records` table and worker lifecycle.
4. [Session Compaction V1 spec](../../specs/session-compaction-system-v1.md) — read the "relationship to" note in CIP §8: that spec compacts *transcripts*; this task compacts *source payloads*. They are distinct; share lower-level summarization utilities only if doing so does not couple them.
5. [Memory & World Model concept](../../concepts/memory.md) — to understand what P2 must **not** touch.
6. [ADR-005 — SQLite single writer](../../adr/ADR-005-sqlite-single-writer.md) — storage constraints.
7. [CLAUDE.md](../../../CLAUDE.md) — build artifact policy, no-ORM rule, NATS streams, naming.
8. Existing code to read before writing: the P1 implementation as it landed in `internal/intake/` and `internal/store/intake*.go`; existing chunk/summarization utilities anywhere in `internal/` (search before building from scratch).

## Description

Phase 2 of CIP, per ADR-012. Goal: take the raw, provenance-stamped `IntakeRecord`s landed by P1 and turn them into **clean, deduplicated, chunked Markdown with span-level provenance** — without yet writing the World Model, scoring significance, computing embeddings, or running entity synthesis.

P2 ships three new stages of the pipeline:

- **Canonicalize (stage 3):** raw payload (HTML / JSON / text / markup) → normalized internal representation + Markdown rendering; boilerplate/chrome strip; URL/identifier normalization.
- **Chunk (stage 4):** token-bounded segmentation with stable chunk IDs and an overlap policy.
- **Distill (stage 5, extractive primitive):** near-duplicate collapse across chunks, quote extraction with span-level provenance, entity-name span extraction, URL normalization. **Provenance is preserved at every step.**

Distillation must be exposed as a **callable primitive** (not implementation-locked inside the worker) so the same code path can be reused at retrieval-time in P4.

P2 does **not** require abstractive LLM summarization. The Distill interface should *allow* an abstractive backend to plug in later, but the V1 implementation ships extractive-only. Abstractive-summary backends are deferred so P2 stays free of LLM dependency and routing privacy-tier coupling (P4's job).

## What you must NOT change (Frozen Design Contract)

- **No World Model writes.** Still P3. P2 ends at a clean `intake_chunks` row; no entity tables are touched.
- **No `internal/governor` engine changes.** Reuse existing budgets/counters from P1 only.
- **No `internal/llm` provider calls in the required scope.** The Distill interface may *declare* an abstractive-summary hook, but no implementation that calls an LLM ships in P2. If an abstractive stub is included, it must be a no-op pass-through.
- **No Python.** Resolution matching begins in P3.
- **No Vault code.**
- **No new ORM, no SQLX wrappers.** Raw SQL only (ADR-005), following the same patterns the P1 store uses.
- **No bare `go build`.** Binaries must land in `bin/` via Make targets (CLAUDE.md "Build Artifact Policy").
- **Do not modify** ADR-012, CIP V1 spec, synthesis seam design note, Vault spec, NAVI-CIP-P1, or this task file.
- **Existing P1 surfaces must keep working.** The `intake_records` table, the `NAVI_REFINERY` worker lifecycle, and Telegram's emission path must continue to function unchanged; P2 extends the worker's downstream processing without altering P1's contract.
- **Distillation must never discard provenance.** A summary or extracted span without a verifiable link back to its source chunk(s) and `IntakeRecord(s)` is a bug, not a feature.

## Acceptance criteria

- [ ] New `intake_chunks` table (raw SQL) with stable chunk IDs, source record link (`intake_record_id`), distilled content (Markdown), span/offset references back to the source, and a provenance summary suitable for retrieval-time filtering.
- [ ] `Canonicalize` stage: accepts an `IntakeRecord`, produces clean Markdown for `text/html`, `application/json`, and plain-text MIME types. Boilerplate / navigation / signature / quoted-reply stripping is applied. URLs and identifiers are normalized.
- [ ] `Chunk` stage: token-bounded segmentation. Default max chunk size is configurable (start at ~3k tokens — tune later); overlap policy is configurable. Chunk IDs are stable for the same input bytes (idempotent re-chunking is a no-op).
- [ ] `Distill` stage (extractive primitive): near-duplicate chunk collapse, quote extraction with span-level provenance, entity-name span extraction (deterministic — proper-noun / capitalization / identifier heuristics; **no LLM**), URL normalization. Each output preserves a verifiable link to source chunk(s).
- [ ] `Distill` is exposed as a **callable primitive** in `internal/intake/distill/` (or equivalent) so P4 retrieval can call the same function with a context-budget argument. The primitive's signature must accept either an `IntakeRecord` / chunk batch (ingest-side) or an arbitrary content+provenance bundle (retrieval-side) — design the interface for both uses now.
- [ ] Worker pipeline extends from Admit (P1) → Canonicalize → Chunk → Distill → persist to `intake_chunks`. The pipeline must be **resumable**: re-processing the same `IntakeRecord` produces idempotent results (no duplicate chunks).
- [ ] Provenance preservation: every persisted chunk links back to its source `IntakeRecord` (and where applicable, parent chunk for further-derived rows) with span offsets.
- [ ] Per-pass metrics emitted: chunks produced, dedupe rate, distillation input bytes vs. output bytes, errors. slog-level is fine; Console surface lands in P5.
- [ ] **No World Model writes from P2.** A test asserts this with a pre/post snapshot of affected stores.
- [ ] **No LLM provider calls from P2.** A test verifies that running the full pipeline against a fixture record makes zero calls to any `internal/llm` provider.
- [ ] `make test` passes; `go vet ./...` clean; `make build-all` succeeds; both binaries land in `bin/`.

## Surface

- New: `internal/intake/canonicalize/` — Markdown rendering, boilerplate strip, URL normalization
- New: `internal/intake/chunk/` — token-bounded segmentation with stable IDs and overlap policy
- New: `internal/intake/distill/` — the reusable Distillation primitive (interface designed for both ingest-side and retrieval-side use)
- New: `internal/store/intake_chunks.go` — raw-SQL store for `intake_chunks`
- New: migration for `intake_chunks` table (follow existing `internal/store/` migration pattern)
- Modified: `internal/intake/` worker — extend the pipeline through stages 3–5; ensure resumability
- Test surface: package tests for each new package + an end-to-end test that drives a fixture Telegram message through P1+P2 producing clean, deduplicated, provenance-bearing chunks in `intake_chunks`

## Suggested implementation order

1. Read all pinned context, especially CIP §8 (Distillation). Search `internal/` for existing chunk/summarization utilities before building from scratch.
2. Define `intake_chunks` table + store with stable-id semantics. Unit tests for store.
3. Build Canonicalize (HTML/JSON/text → Markdown, boilerplate strip, URL normalization). Unit tests with realistic fixtures (sample emails, HTML pages, Telegram messages with quoted replies).
4. Build Chunk (token-bounded, stable IDs, overlap). Unit tests for idempotency.
5. Build Distill primitive with the dual-use signature (ingest + retrieval). Extractive-only V1; abstractive hook is interface only. Unit tests.
6. Extend worker pipeline to chain stages 3–5 after Admit; ensure resumability (re-running the same `IntakeRecord` is a no-op for already-distilled chunks).
7. End-to-end test asserting no World Model writes and no LLM calls.
8. Wire metrics. Verify `make build-all`.

## Priority

HIGH

## Out of scope for P2 (explicit)

- Score (P3 — deterministic intake-time scoring; LLM judgmental re-scoring at Deep Reflection)
- Embed / extract entities for graph linking (P3)
- Synthesize / fold (P3)
- Retrieve + retrieval-side distillation invocation (P4) — but the Distill primitive's interface must be designed to support it
- Abstractive LLM summary backend (deferred — interface only)
- Python resolution matching (P3)
- Vault (parallel deliverable)
- Console surfaces (P5)
- Privacy-tier routing enforcement (P4)
- Vector index / embedding store engine (P3+)
