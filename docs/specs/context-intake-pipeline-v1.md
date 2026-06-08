# NAVI Context Intake Pipeline Specification

> [!NOTE]
> Part of the [NAVI Systems Map](../architecture/navi-systems-map.md).

**Status:** Proposed
**Version:** 1.0 (Draft 2)
**Primary surface:** Cognitive Layer (Subconscious Process) — feeds the World Model
**Primary objective:** define a first-class, governed pipeline that turns external source data into provenance-tagged World Model state, so that "NAVI already knows my context" becomes a real, inspectable system property rather than an emergent side effect of connectors.

> All TypeScript/Go type definitions, SQL fragments, NATS subjects, and numeric defaults in this document are **illustrative, not normative**. Wire formats, storage schema, and interface signatures will be finalized during implementation. Where this spec names a default (e.g. chunk size, cadence), treat it as a starting value to be tuned, not a contract.

---

## 1. Purpose

NAVI's value thesis is **continuity**: a personal AI that holds the shape of the owner's digital life so the owner never rebuilds context. Today the mechanics of getting external data *into* NAVI are scattered across individual connectors, with no shared contract for how raw payloads become structured, trustworthy, retrievable knowledge.

The **Context Intake Pipeline** (CIP) defines that contract as one named, testable system. It is the bridge from the **Capability Layer** (connectors that fetch data) to the **World Model** (the canonical entity store), passing through the **Cognitive Layer** (which alone may write the World Model).

The pipeline exists to make four guarantees:

1. **Nothing skips the trust gate.** Every payload is tagged with `ContentTrust` and a privacy class at the boundary before it influences anything.
2. **Nothing skips the Cognitive write seam.** Connectors never write the World Model directly; intake synthesis runs as a Subconscious process.
3. **Everything carries provenance.** Source, fetch time, cursor position, confidence, and derivation chain travel with the data from ingestion to retrieval.
4. **Nothing reaches a model un-distilled.** Source payloads are compressed and de-boilerplated before they consume context or cost.

**Positioning.** CIP is *a piece of NAVI, not NAVI's identity.* This is the core distinction from systems that make a context-ingestion loop their entire product: for NAVI, intake is one seam through machinery that already exists to support the World Model, Cognitive processes, governance, and Skills holistically. The pipeline is deliberately not a pedestal — it is plumbing that several layers depend on. NAVI feeling more like an architecture than a product today is an accepted, temporary state; NAVI is not done.

## 2. Scope

### 2.1 In scope (V1)

* The canonical **stage model**: `sync → admit → canonicalize → chunk → distill → score → embed/extract → synthesize → fold → retrieve`.
* The **intake envelope** (`IntakeRecord` / `IntakeBatch`) connectors emit, and the queue/stream they emit onto.
* **Per-connector sync policy**: cursor, cadence, budget, dedupe, freshness, privacy class, and a user-visible sync log.
* **Content Distillation** as a dedicated, reusable stage (boilerplate removal, dedupe, quote/entity extraction, summary, compression) with provenance preservation.
* The **Cognitive synthesis seam**: how distilled chunks become History / Knowledge / Memory / Contact / Event entities, and when synthesis must raise a **Proposal** instead of writing directly.
* **Hierarchical summary trees** (the retrieval index) as *derived* state, explicitly not authoritative.
* **Retrieval** surface consumed by the Conscious loop's Contextualize step, including trust/privacy filtering.
* **Privacy modes** (Local / Hybrid / Cloud) and their effect on where distillation and embedding run.
* Governance integration: ingestion volume budgets, privacy-class enforcement, instruction-bearing content handling.
* Observability: per-stage metrics, sync log, replayability.

### 2.2 Out of scope (V1 — forward references)

* **Memory Vault / Projection** — the human-readable Markdown/Obsidian/Nextcloud surface generated *from* the World Model. CIP produces the canonical state the Vault projects; the Vault gets its own spec (`memory-projection-v1.md`, planned).
* **Vector store engine selection** — V1 defines the embedding *interface* and retrieval contract; the concrete index (sqlite-vec, external, etc.) is an implementation decision.
* **Real-time webhook ingestion** beyond cursor-based pull — webhooks may *trigger* a sync pass but the processing contract is identical.
* **Cross-device federated memory** — deferred to roadmap Phase 16.
* **Connector OAuth/credential management** — owned by the connector subsystem, consumed here.

## 3. Design goals

* **Elegant layering.** The pipeline must respect the architectural guardrail *"no direct World Model writes from Experience or Capability."* Intake is a Cognitive (Subconscious) activity that *reads* connector output and *writes* the World Model — connectors only emit candidate records.
* **Stateless, reconstructable processing.** Each stage operates on durable records; a crashed pass resumes from the cursor and the queue, not from in-memory state (mirrors the orchestrator's stateless design).
* **Provenance is non-negotiable.** A retrieved fact must be traceable to the source record, fetch time, and the connector cursor that produced it.
* **Distillation is infrastructure, not a feature.** The same distillation primitive runs at ingestion (shrink what we store) and at retrieval (shrink what we send to the model).
* **Privacy is explicit, never fuzzy.** Local / Hybrid / Cloud are real modes with real consequences for what leaves the device — no "local-first" hand-waving.
* **Governed by default.** Ingestion volume, cost, and mutation authority flow through the existing Governor and Proposal Queue.

## 4. Where CIP sits

```
 Capability Layer            Cognitive Layer                 World Model
 ┌──────────────┐   emit    ┌───────────────────────────┐   write    ┌──────────────┐
 │  Connectors  │ ────────► │  Context Intake Pipeline  │ ─────────► │  Entities    │
 │ (Telegram,   │  Intake   │  admit→…→synthesize→fold  │  (gated)   │  History,    │
 │  Gmail, GH…) │  Records  │  (Subconscious process)   │            │  Knowledge,  │
 └──────────────┘           └───────────────────────────┘            │  Memory,…    │
        ▲                              │ retrieve (read)                 └──────────────┘
        │ sync policy                  ▼                                        │ project
        │ (cursor, cadence)   ┌───────────────────┐                    ┌──────────────┐
        └──────────────────── │  Conscious loop   │ ◄───── retrieval ──│ Summary trees│
                              │  Contextualize    │                    │  (derived)   │
                              └───────────────────┘                    └──────────────┘
                                                                       (Vault projection
                                                                        = separate spec)
```

Key invariant: the **synthesize** stage is the *only* place the World Model is written, and it runs inside the Subconscious Process (Consolidation tier). Connectors stop at **emit**.

## 5. The intake envelope

Connectors that participate in intake emit `IntakeRecord`s (illustrative):

```go
// illustrative, not normative
type IntakeRecord struct {
    ConnectorID  string        // e.g. "telegram:acct-1", "gmail:primary"
    SourceKind   string        // "message" | "email" | "issue" | "file" | "event" | ...
    SourceID     string        // stable external id for dedupe
    Cursor       string        // opaque per-connector position (for resume/replay)
    FetchedAt    time.Time
    Trust        ContentTrust  // boundary-assigned; external surfaces default external_untrusted
    PrivacyClass PrivacyClass  // see §9
    Author       string        // external author identity, if any
    Raw          []byte        // original payload (html/json/text/blob ref)
    RawMIME      string
    Provenance   Provenance    // connector, account, link-back URL, etc.
}
```

Records are published to the **`NAVI_REFINERY`** stream (existing: `navi.refinery.queue`, WorkQueue, 48h retention) — the refinery is the natural home for intake processing. A pipeline worker consumes the queue and drives stages 2–9.

## 6. Stage model

| # | Stage | Layer | Writes World Model? | Responsibility |
|---|-------|-------|---------------------|----------------|
| 1 | **Sync** | Capability | No | Connector pulls new data per sync policy (§7); emits `IntakeRecord`s with cursor advanced. |
| 2 | **Admit** | Boundary | No | Assign/validate `ContentTrust` + `PrivacyClass`; stamp provenance; dedupe by `(ConnectorID, SourceID)`. |
| 3 | **Canonicalize** | Cognitive | No | Normalize payload → internal representation + Markdown rendering; strip boilerplate/chrome. |
| 4 | **Chunk** | Cognitive | No | Token-bounded segmentation (default ≤ ~3k tokens) with stable chunk IDs and overlap policy. |
| 5 | **Distill** | Cognitive | No | Dedupe, quote/entity extraction, summarization, compression — **provenance preserved**. (§8) |
| 6 | **Score** | Cognitive | No | Relevance / significance / recency scoring → retention tier + candidacy (ephemeral History vs durable Knowledge/Memory). |
| 7 | **Embed & Extract** | Cognitive | No | Compute embeddings; extract entities + relationship candidates for graph linking. |
| 8 | **Synthesize** | Cognitive | **Yes (gated)** | Create/update World Model entities with provenance + confidence. Non-append mutations may raise a **Proposal**. |
| 9 | **Fold** | Cognitive | Derived only | Update hierarchical summary trees (the retrieval index). Not authoritative. |
| — | **Retrieve** | Cognitive | No | Query-time hybrid retrieval (vector + entity graph + recency), trust/privacy filtered; feeds Contextualize. (§10) |

Synthesis runs at the **Consolidation** tier of the Subconscious Process; the existing reflection-tier mutation permissions apply. Deep Reflection may later re-score, merge, or deprecate Knowledge produced here.

### 6.1 Where scoring intelligence lives

Scoring (stage 6) is split deliberately by cost and timing:

* **Intake-time scoring is cheap and deterministic.** The hot loop uses recency, source weight, author trust, and similar signals — no LLM call per chunk. This keeps the per-pass cost bounded and the loop predictable at ingest volume.
* **Judgmental re-scoring is deferred to Deep Reflection.** Significance that genuinely requires reasoning (is this worth becoming durable Knowledge? should these be merged?) is reserved for the weekly/event-triggered Deep Reflection tier, where the spend is amortized and infrequent.
* **The intelligence layer is Python, not Go and not prompts.** Scoring heuristics that are richer than trivial arithmetic but should not be LLM-priced belong in NAVI's **Python scripting layer**. This makes explicit a direction that has been implicit: Python is NAVI's scripting language and is the intended home for this class of deterministic-but-non-trivial logic. Putting scoring in prompts would violate *Zero Framework Cognition* in the other direction (paying for cognition where a script suffices); putting it in Go would harden a tuning surface that should stay scriptable.

> Terminology guard: Go's authoritative **control kernel** (governance handoff, action selection, runtime authority) is not "cognition." We do not conflate the two. Scoring is neither control-kernel logic nor reasoning — it is scriptable judgment, which is why Python is the fit.

## 7. Per-connector sync policy

Each intake-capable connector declares a policy (illustrative):

```yaml
# illustrative, not normative
sync:
  cadence: "20m"          # interval or "manual" / "webhook-triggered"
  budget:
    max_records_per_pass: 500
    max_bytes_per_pass: 10MB
    cost_ceiling_usd: 0.50   # distillation/embedding spend, charged via Governor
  cursor: incremental      # how resume position is tracked
  dedupe: by_source_id
  freshness: "7d"          # how far back a cold-start backfill reaches
  privacy_class: personal  # default trust/privacy floor for this source
  visibility: logged       # appears in user-visible sync log
```

Requirements:

* **Cursor + dedupe** make passes idempotent and replayable.
* **Budgets** are enforced by the **Governor** — intake cannot exhaust the action/cost ceiling silently.
* **Cadence** copies OpenHuman's *pattern*, not its fixed 20-minute cycle — each connector and the owner can tune it; some are `manual`/`webhook-triggered` only.
* **Sync log** is user-visible: last pass, records admitted/deduped/distilled, errors, next scheduled pass. Surfaced in NAVI Console (see §11).

### 7.1 Job modes: backfill vs delta

Ingestion runs in one of **two fundamentally different job modes**. This distinction is generic — it applies to every connector and data source, not just one — and is a core vocabulary for the whole pipeline:

| | **Backfill** | **Delta** |
|---|---|---|
| Nature | Batch / holistic job | Incremental patch / upkeep job |
| When | Cold start (first connect), or explicit re-sync of a historical window | Steady state, on cadence or webhook trigger |
| Volume | Large, bounded by a freshness window | Small, bounded by what changed since cursor |
| Budget | Its own (larger) budget + cost ceiling; may be chunked across many passes | Per-pass budget from §7 |
| Consent | May require explicit owner authorization (ingesting months of history is a deliberate act) | Pre-authorized by the connector's standing sync policy |
| Idempotency | Resumable across passes; dedupe by source id is essential | Cursor advance + dedupe |

The two modes share **all** downstream stages (admit → … → fold) — only the *acquisition* posture differs. A backfill is effectively a budgeted, resumable sequence of admit-and-process batches that walks backward through a freshness window; a delta is the same machinery driven by a cursor forward. Keeping them as named modes (rather than "a big first pass" vs "normal") lets us give backfill its own budget, consent prompt, and progress reporting without special-casing the processing core.

## 8. Context Distillation (stage 5, reused at retrieval)

Distillation is a standalone primitive, callable both during ingestion (shrink what we persist) and at retrieval (shrink what we send to a model). It performs:

* HTML/markup → clean Markdown
* boilerplate / navigation / signature / quoted-reply stripping
* near-duplicate collapse across chunks
* quote and entity extraction with span-level provenance
* extractive + abstractive summary at the chunk and document level
* URL/identifier normalization

**Hard rule:** distillation never discards provenance. A compressed summary always links back to the chunk(s) and source record it derived from, so confidence and derivation chains remain intact. (We deliberately do *not* claim a fixed compression ratio; OpenHuman's "up to 80%" is a marketing number — we measure ours.)

> Relationship to `session-compaction-system-v1.md`: that spec compacts *conversation transcripts*; CIP distillation compacts *source payloads*. They are different inputs and may share lower-level summarization utilities but are not the same system.

## 9. Privacy modes

CIP makes the local/managed split **explicit** (the opposite of OpenHuman's blurred "local-first"):

| Mode | Distillation & embedding | Source payloads leave device? |
|------|--------------------------|-------------------------------|
| **Local** | Local models only (Ollama). | No. Cloud routes refused for intake. |
| **Hybrid** | Local where possible; cloud allowed per privacy class. | Only classes explicitly permitted. |
| **Cloud** | Best available model regardless of locality. | Yes, per policy. |

`PrivacyClass` (e.g. `public`, `personal`, `sensitive`, `secret`) is assigned at **Admit** and gates which models a record may touch downstream. This plugs directly into the existing contextual LLM router (§10) as a privacy tier.

## 10. Retrieval & routing

* **Retrieval** is hybrid: vector similarity + entity-graph traversal + recency, filtered by `ContentTrust` and `PrivacyClass`. It returns ranked, provenance-bearing chunks/summaries to the Conscious loop's **Contextualize** step.
* Retrieved context is **distilled to a budget** (stage 8 primitive) before model assembly.
* Model selection reuses the **existing** contextual router (`internal/llm/profiles.go`, `docs/concepts/contextual-llm-routing.md`). CIP's only addition is the **privacy tier**: a `secret`-class payload in Local mode must route to a local model or be refused — routing is plumbing, never UI theater.

## 11. Console & observability

NAVI Console surfaces intake calmly (aligns with `navi-console-v2.md`):

* **Sync state** per connector: last/next pass, counts, errors.
* **Recent intake**: what was admitted, distilled, and synthesized — with provenance links.
* **Pending Proposals** raised by synthesis (e.g. "merge two Contact entities", "deprecate stale Knowledge").

Per-stage metrics (records in/out, dedupe rate, distillation input/output tokens, synthesis writes, proposals raised) are emitted for tuning. Every pass is **replayable** from cursor + raw record store.

## 12. Governance integration

* **Volume/cost** via Governor budgets (§7).
* **Mutation authority**: append-only History writes are auto-approved; entity merges, Knowledge deprecation, Contact identity changes, and anything destructive route through the **Proposal Queue** per the Autonomy Model.
* **Instruction-bearing untrusted content**: `external_untrusted` payloads are data, never directives. Distillation/synthesis prompts must treat them as quoted content (existing Content Trust model, `docs/concepts/content-trust.md`).

## 13. Storage (illustrative)

Raw SQL only (no ORM), WAL, following `internal/store/` conventions. Candidate tables: `intake_records` (raw + provenance + cursor), `intake_chunks` (distilled chunks + embeddings ref), `summary_nodes` (hierarchical fold tree). World Model entities live in their existing stores; CIP links to them by entity ID. Vector index interface is defined; the engine is deferred (§2.2).

## 14. Phased rollout

| Phase | Deliverable | Gate |
|-------|-------------|------|
| **P1** | Intake envelope + `NAVI_REFINERY` worker + Admit (trust/privacy/dedupe) + raw record store. One connector (Telegram) emits records. | Records land, deduped, provenance-stamped; no World Model writes yet. |
| **P2** | Canonicalize + Chunk + Distill primitive (ingestion-side). | Stored chunks are clean Markdown with preserved provenance; metrics emitted. |
| **P3** | Score + Embed/Extract + Synthesize (gated) + Fold. | History/Knowledge entities created with confidence; non-append mutations raise Proposals. |
| **P4** | Retrieval surface + retrieval-side distillation + privacy-tier routing. | Conscious loop Contextualize pulls provenance-bearing context; Local mode refuses cloud routes for `secret`. |
| **P5** | Per-connector sync policies + sync log + Console surfaces. | Owner can see/tune cadence, budgets, and inspect recent intake. |

## 15. Acceptance criteria (V1)

1. A connector can emit `IntakeRecord`s without touching the World Model; all World Model writes originate from the Synthesize stage.
2. Every synthesized entity carries provenance back to a source record, fetch time, and cursor.
3. A crashed/restarted pass resumes from cursor + queue with no duplicate entities.
4. Distillation preserves a verifiable link from every summary to its source chunk(s).
5. A `secret`-class record in Local mode never reaches a cloud model (routing refusal is observable).
6. Synthesis that merges/deprecates/destroys entities raises a Proposal rather than writing directly.
7. The Governor can halt a runaway ingestion pass via budget/cost ceiling.
8. The owner can view a per-connector sync log and tune cadence/budget/privacy class.

## 16. Resolved decisions & open questions

**Resolved (this draft):**

* **Scoring placement** — deterministic at intake, judgmental re-scoring at Deep Reflection, intelligence layer in **Python** (§6.1).
* **Job modes** — Backfill (batch/holistic, own budget + consent) and Delta (incremental upkeep) are first-class named modes sharing all downstream stages (§7.1).

**Still open (for the ongoing design discussion):**

* **The synthesis seam (§8) — the genuinely hard part.** Specified in its own design note: [Intake Synthesis Seam](../design/intake-synthesis-seam.md). Backbone: extension of the existing `internal/governor` engine via a sibling `MutationDescriptor`, with the disposition ladder as the Risk rubric; Python resolution matching feeds the Go governor; backfill batches `RequiresConfirmation` into grouped Proposals. P1–P2 still ship with zero World Model writes precisely so this can be deliberate.
* **Embedding locality** — acceptable cloud-embedding boundary in Hybrid mode for `personal` vs `sensitive`.
* **Vault coupling** — resolved: Vault projects from **entities only**; summary trees may render as read-only generated index pages. See [memory-projection-v1.md](memory-projection-v1.md).
* **Entity-resolution conflicts** — when extraction proposes merging entities the owner considers distinct.

---

## Related specs & concepts

* [ADR-012](../adr/ADR-012-context-intake-and-memory-projection.md) — architectural commitment that ratifies this spec
* [memory.md](../concepts/memory.md) — World Model entity classes (synthesis targets)
* [content-trust.md](../concepts/content-trust.md) — boundary trust tagging (Admit stage)
* [contextual-llm-routing.md](../concepts/contextual-llm-routing.md) — router CIP extends with privacy tiers
* [Intake Synthesis Seam](../design/intake-synthesis-seam.md) — design note for stage 8 (authority model + Go/Python split)
* [Governance tier resolution (GOV-04)](../design/governance-tier-resolution.md) — System > Owner > Plugin precedence reused by synthesis
* [session-compaction-system-v1.md](session-compaction-system-v1.md) — transcript compaction (distinct from source distillation)
* [artifact-system-v1.md](artifact-system-v1.md) — Subconscious participation + Proposal Queue patterns
* [memory-projection-v1.md](memory-projection-v1.md) — Memory Vault: entities-only Markdown projection of the World Model, structured-diff round-trip via the synthesis seam (the "second mouth" on this pipeline)

[docs specs INDEX](INDEX.md)
