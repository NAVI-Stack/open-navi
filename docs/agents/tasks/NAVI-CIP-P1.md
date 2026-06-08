# Task spec — NAVI-CIP-P1

## Task ID

NAVI-CIP-P1

## Title

Implement Context Intake Pipeline Phase 1 — Intake envelope, `NAVI_REFINERY` worker, Admit stage, raw record store, Telegram emission

## Pinned context (read first, in this order)

1. [ADR-012 — Personal Continuity Layer](../../adr/ADR-012-context-intake-and-memory-projection.md) — architectural commitment ratifying this phase. **Read in full.**
2. [Context Intake Pipeline V1 spec](../../specs/context-intake-pipeline-v1.md) — full pipeline contract. **Focus on §4 (where CIP sits), §5 (intake envelope), §6 (stage model — stages 1–2 only for P1), §7 + §7.1 (sync policy, backfill vs delta), §13 (storage), §14 (P1 row), §15 (acceptance criteria #1, #3, #7, #8 apply to P1).**
3. [Memory & World Model concept](../../concepts/memory.md) — to understand what P1 must **not** touch.
4. [Content Trust model](../../concepts/content-trust.md) — boundary tagging applied at Admit.
5. [ADR-005 — SQLite single writer](../../adr/ADR-005-sqlite-single-writer.md) — storage constraints.
6. [CLAUDE.md](../../../CLAUDE.md) — build artifact policy, naming, NATS streams, no-ORM rule.
7. Existing code to read before writing: `internal/bus/streams.go` (NAVI_REFINERY stream definition), `internal/store/` (raw-SQL store patterns to mirror), `internal/governor/` (budgets — minimal reuse in P1), `connectors/telegram/` (the emitter you will add to).

## Description

Phase 1 of CIP, per ADR-012. Goal: make external sources emit **governed, deduped, provenance-stamped records** into NAVI without yet writing the World Model. Telegram emits the first `IntakeRecord`s. A worker on the existing `NAVI_REFINERY` stream (`navi.refinery.queue`) consumes records, runs **Admit** (trust tagging, privacy class, provenance stamp), dedupes by `(ConnectorID, SourceID)`, and persists to a new raw record store.

P1 is plumbing + storage + dedupe + provenance. **It is explicitly not** canonicalize, distill, score, embed/extract, synthesize, fold, or retrieve — those land in later phases. The success criterion is that records are landing, deduplicated, and provenance-bearing, with zero World Model writes.

## What you must NOT change (Frozen Design Contract)

- **No World Model writes.** This phase must not write to any World Model entity store (Contacts, Knowledge, Memories, Artifacts, Events, History at the entity level, Proposals). Synthesis is P3.
- **No `internal/governor` engine changes.** The governor is reused for budgets only in P1; the sibling `MutationDescriptor` and the synthesis seam land in P3.
- **No `internal/llm` provider changes.** P1 does not call any LLM.
- **No Python.** Python interop begins in P3 (resolution matching). P1 is Go-only.
- **No Vault code.** Memory Projection is a separate, parallel deliverable; P1 does not touch it.
- **No new ORM, no SQLX wrappers.** Raw SQL only (ADR-005).
- **No bare `go build`.** Binaries must land in `bin/` via Make targets. See CLAUDE.md "Build Artifact Policy."
- **Do not modify** ADR-012, CIP V1 spec, synthesis seam design note, Vault spec, or this task file.
- **Telegram's existing message-handling path must keep working.** Emission is **additive** alongside existing handling — never a replacement.
- **No changes to `NAVI_REFINERY` stream config** (it already exists in `internal/bus/streams.go` — reuse it).

## Acceptance criteria

- [ ] New package (suggested `internal/intake/`) defines `IntakeRecord` with the fields specified in CIP §5 (`ConnectorID`, `SourceKind`, `SourceID`, `Cursor`, `FetchedAt`, `Trust`, `PrivacyClass`, `Author`, `Raw`, `RawMIME`, `Provenance`).
- [ ] `NAVI_REFINERY` worker subscribes to `navi.refinery.queue`, applies the Admit stage (trust tagging, privacy class default, provenance stamp), dedupes by `(ConnectorID, SourceID)`, and persists records to a new `intake_records` table.
- [ ] New `intake_records` table created via a migration following existing `internal/store/` patterns; indexed on `(connector_id, source_id)` for dedupe lookups; WAL mode inherited.
- [ ] Telegram connector emits one `IntakeRecord` per inbound message in addition to its existing handling; existing Telegram message behavior is unchanged and existing tests still pass.
- [ ] Duplicate emission of the same `(ConnectorID, SourceID)` is a no-op at Admit; a replayed connector pass produces zero duplicate rows.
- [ ] Every persisted record carries `ContentTrust` (`external_untrusted` for non-owner messages, `owner` only for messages from the owner's own Telegram account), `PrivacyClass` (default per connector sync policy — `personal` is acceptable for Telegram in P1), `FetchedAt`, and connector-source provenance (connector id, account, link-back where available).
- [ ] **No code path writes to World Model entity stores from P1.** A test asserts this by snapshotting affected stores before and after a Telegram message ingest and verifying no entity tables were mutated.
- [ ] Worker emits per-pass counters: records admitted, deduped, errored. These can be slog-level metrics in P1; the Console surface lands in P5.
- [ ] Worker honors a per-pass record budget via the existing `internal/governor` (basic counter is sufficient; full cost ceiling enforcement lands later).
- [ ] `make test` passes.
- [ ] `go vet ./...` clean.
- [ ] `make build-all` succeeds; both binaries land in `bin/` and neither lands in the repo root.

## Surface

- New: `internal/intake/` (envelope, worker, Admit logic)
- New: `internal/store/intake.go` (raw-SQL store for `intake_records`)
- New: migration for `intake_records` table (follow existing `internal/store/` migration pattern)
- Modified: `connectors/telegram/` (add intake emission hook; do not change existing behavior)
- Modified: `cmd/navid/main.go` (wire the new worker into daemon startup/shutdown)
- Test surface: package tests in `internal/intake/`, `internal/store/`, and an end-to-end test that drives a Telegram message through to a persisted `intake_records` row (mocking only the Telegram HTTP boundary, per CLAUDE.md "use actual implementations in tests" rule).

## Suggested implementation order

1. Read all pinned context, especially CIP §5–§7 and ADR-012.
2. Define `IntakeRecord` and supporting types in `internal/intake/`.
3. Add `intake_records` table + store with dedupe-by-unique-index. Unit tests for store.
4. Build the worker: NATS subscription, Admit stage, persistence, counters. Unit tests with a `MemBus` (test-only).
5. Wire Telegram emission alongside its existing handler. Verify existing Telegram tests still pass.
6. Add the end-to-end test asserting no World Model writes.
7. Wire worker lifecycle into `cmd/navid/main.go`. Verify `make build-all`.

## Priority

HIGH

## Out of scope for P1 (explicit)

- Canonicalize, chunk, distill, score, embed/extract, synthesize, fold, retrieve (later phases)
- Memory Vault (parallel deliverable)
- Backfill consent UX (delta-shaped emission is sufficient for P1; backfill is its own task)
- Connectors other than Telegram (Slack, etc. are later tasks)
- Console sync log surface (lands in P5)
- Vector index / embedding store engine selection (P3+)
- Privacy-tier enforcement at routing (P4)
- Python resolution matching (P3)
