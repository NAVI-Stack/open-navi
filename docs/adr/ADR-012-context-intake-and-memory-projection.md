# ADR-012 — Personal Continuity Layer: Context Intake, Synthesis Seam, and Memory Vault

> Formal decision record establishing how external sources and owner edits acquire into NAVI's World Model: a first-class governed intake pipeline, a synthesis seam that extends the existing governance engine rather than mirroring it, and a Markdown vault as the owner-facing projection.

**Status:** Accepted
**Date:** 2026-05-28
**Deciders:** Eric (Owner)
**See also:** [ADR-001 Go orchestration / Python AI](ADR-001-go-orchestration-python-ai.md) · [ADR-007 Dual-Plane LLM](ADR-007-dual-plane-llm.md) · [ADR-011 Memory v2](ADR-011-memory-v2.md) · [CIP V1](../specs/context-intake-pipeline-v1.md) · [Intake Synthesis Seam](../design/intake-synthesis-seam.md) · [Memory Projection V1](../specs/memory-projection-v1.md)

---

## Context

[ADR-011](ADR-011-memory-v2.md) ratified what *truth* looks like inside NAVI: an entity-based World Model with a relationship layer, three-tier reflection, and governed context injection. ADR-011 did not specify how external data **acquires** into that World Model, nor how the owner inspects and corrects it. Those two questions — *how does the world get in*, and *how does the owner see and shape it* — were left to individual connectors, ad-hoc tool outputs, and Console surfaces that didn't yet exist.

Without a named contract for acquisition, three risks compound:

1. **Scattered, ungovernable ingestion.** Every connector invents its own write path into memory, producing drift in trust tagging, provenance, and Cognitive ownership. The architectural guardrail "no direct World Model writes from Experience or Capability" becomes harder to enforce as the connector set grows.
2. **Authority forks.** When background synthesis (entity merges, knowledge deprecation) lands without a governance contract, the natural reflex is to invent a parallel decision system. That duplicates `internal/governor` and creates two authorities — exactly the drift NAVI's guardrails resist.
3. **Continuity without inspectability.** Even with a correct World Model, the owner has no calm surface to see what NAVI knows, correct it, or feel that their context is *held*. The market signal from comparable systems (OpenHuman) is that continuity is only trusted if it is tangible.

This ADR resolves all three by ratifying a single layered design: the **Context Intake Pipeline** (acquisition), the **synthesis seam** (authority extension), and the **Memory Vault** (projection). They were drafted together precisely because their decisions interlock.

**Relationship to prior ADRs:**

- **ADR-001** ratified Go for orchestration and Python for the AI layer. This ADR extends that line: the synthesis seam's resolution matching is the first non-skill use of the Python runtime pattern as the "scripting layer that calls the Go control kernel."
- **ADR-007** ratified the LLM dual-plane separation (Inference vs. Control). The intake pipeline preserves it: distillation/embedding are Inference Plane; synthesis authority is Control Plane.
- **ADR-011** ratified the entity model and reflection pipeline. This ADR specifies the acquisition path *into* that model, and the projection path *out of* it. It does not modify the entity model or reflection tiers.

---

## Decisions

### D1 — Context Intake Pipeline is a first-class, named seam

A single named pipeline — admit → canonicalize → chunk → distill → score → embed/extract → synthesize → fold → retrieve — replaces ad-hoc per-connector ingestion. Connectors **only emit** `IntakeRecord`s onto the existing `NAVI_REFINERY` work queue; they never write the World Model. The pipeline lives in the Cognitive Layer (specifically the Subconscious Process, Consolidation tier) — which preserves the guardrail that only Cognitive may write the World Model.

Reference: [CIP V1](../specs/context-intake-pipeline-v1.md).

### D2 — Backfill and Delta are first-class job modes

Acquisition runs in one of two named modes — **Backfill** (batch / holistic, own budget + explicit consent) or **Delta** (incremental upkeep, cursor-driven). Both share all downstream stages; only the acquisition posture differs. This vocabulary applies to *every* connector and data source.

### D3 — Synthesis extends the existing `internal/governor` engine; it does not mirror it

The synthesize stage models World Model mutations as a new governed *effect class* (`world_model_mutation`) flowing through the existing `governor.Pipeline.Run`. A sibling `MutationDescriptor` joins `ActionDescriptor` as a second descriptor consumed by the same engine. The four `ValidationOutcome` values, the autonomy presets, the `HardFloorProposalRequired` floor, the Proposal Queue, and the System > Owner > Plugin tier resolution ([governance tier resolution](../design/governance-tier-resolution.md)) are **all reused as-is**. The synthesis disposition ladder (append / reinforce / create / merge / deprecate / forget) is the Risk rubric that feeds the engine; it is not a parallel authority.

This explicitly rejects forking `Pipeline` for writes. Two authorities would drift; one engine with two callers does not.

Reference: [Intake Synthesis Seam](../design/intake-synthesis-seam.md).

### D4 — Python performs resolution matching; Go authoritatively decides the outcome

Entity resolution (matching extracted entities against the existing graph via deterministic blocking keys and similarity) runs in **Python**, reusing the existing subprocess + structured-envelope pattern from `internal/navi/skill/subprocess_runtime.go`. The result is a `ResolutionResult` consumed by the Go-side `governor.Pipeline.Run`, which decides the `ValidationOutcome`.

Framing (owner's, ratified): Go is the core engine that owns capability; Python is the scripting layer that runs the game code via calls and interfaces. **Python decides and judges; Go authoritatively approves, denies, or escalates.** Dataflow is always Python → Go; Python never writes the World Model and never decides authority. This extends ADR-001 from skill execution to the synthesis seam.

### D5 — The Memory Vault is the second mouth on the intake pipeline

Owner-facing Markdown files (the Vault) are *projections* of the World Model and an *editable surface* back into it. Owner edits enter through the same admit → … → synthesize stages as external sources, differing only by `ContentTrust` (`owner` vs. `external_untrusted`). The synthesis seam governs both. This produces a single acquisition machinery, not two.

Reference: [Memory Projection V1](../specs/memory-projection-v1.md).

### D6 — Vault round-trip is structured-diff, not lossless Markdown

Owner edits are parsed into entity diffs that become `MutationDescriptor`s and pass through the synthesis seam. The Vault never holds authority; reprojection is idempotent and may not preserve the owner's exact phrasing of non-content fields. This is a deliberate safeguard: owners are the authority for most things, but **owners are not always right or phrase things correctly**, and structured diff prevents a typo, a fat-finger save, or thinking-out-loud edits from poisoning truth. Structural mutations expressed via the Vault (file delete, frontmatter id changes) raise Proposals; they never execute silently.

### D7 — Vault projects from entities only

Summary trees (the derived retrieval index, CIP stage 9) may render as **read-only generated index pages** but are never editable as authority. The rule the architecture commits to: *truth comes out as files; derivations come out as indexes; indexes are read-only.* This prevents the failure mode of letting the owner edit a derived summary and have those edits silently become authoritative.

V1 Vault coverage: **Contacts, Knowledge, Memories, Artifacts.** History, Events, Proposals, and Configuration are deferred to later iterations.

### D8 — Privacy modes are explicit (Local / Hybrid / Cloud)

CIP commits to three explicit privacy modes that gate which models a record may touch downstream. `PrivacyClass` is assigned at Admit and is enforced by the contextual LLM router as a privacy tier. NAVI does not blur "local-first" with a managed backend; the modes are real with real consequences.

---

## Consequences

**Positive:**

- **One governance authority, one Proposal Queue.** The synthesizer becomes a new caller of an engine that already exists; there is no parallel decision system to maintain.
- **Cognitive ownership of writes is structurally enforced.** Capability connectors literally cannot write the World Model — they only emit. The guardrail moves from convention to code shape.
- **The Vault makes continuity inspectable** without becoming a back door around governance, because it shares the synthesis seam.
- **Python's role becomes explicit beyond skills.** Resolution matching is the first articulated use of the scripting layer for non-skill judgment work, reaffirming ADR-001.
- **Backfill and Delta as named modes** give per-connector implementations a shared vocabulary for cursors, dedupe, budgets, and consent.

**Costs / things we are accepting:**

- **New caller pattern for the governor.** `governor.Pipeline.Run` will need to accept a sibling `MutationDescriptor`; this is a small refactor, not an engine change. (Future descriptor convergence is noted in the synthesis seam doc.)
- **Reprojection may normalize owner phrasing.** The Vault preserves owner *facts*; rendering details may shift on reprojection. This is the explicit trade for D6.
- **New SQL tables** for `intake_records`, chunks, and summary nodes (raw SQL, no ORM, per ADR-005). Vector index engine is deferred — V1 defines only the embedding interface.
- **Disk footprint grows** as the Vault projects entities to files. Cloud sync of the Vault (Nextcloud, Syncthing, iCloud) is the owner's choice and lives outside this ADR.
- **Console contract expands** to surface intake sync state, per-file Vault sync log, and grouped Proposals (Console V2 addendum). These ride in alongside CIP P5 / Vault, not earlier.

---

## Alternatives considered

- **Mirror the governance engine for synthesis writes.** Rejected: creates a second authority and Proposal-like queue that will drift from the first. Reuse over fork.
- **Treat Vault Markdown as authoritative storage.** Rejected: lets owner-side typos and informal edits poison truth (D6 rationale).
- **LLM-score every chunk at intake.** Rejected: cost-prohibitive at backfill volume. Deterministic scoring at intake; LLM judgment reserved for Deep Reflection re-scoring.
- **Embedded background sync inside each connector with no shared contract.** Rejected: scattered, ungovernable, drifting trust semantics (D1 rationale).
- **Fixed 20-minute cadence (OpenHuman pattern).** Rejected: copy the pattern, not the value. Per-connector sync policy (cadence, budget, freshness, privacy class).
- **Overload `ActionDescriptor` to carry mutation effects.** Deferred, not rejected: a sibling `MutationDescriptor` is the V1 shape; convergence is reasonable later if the descriptors drift toward each other.

---

## Implementation phasing

Phasing lives in the CIP spec (§14). At a high level, P1–P2 deliver retrieval value with zero World Model writes; P3 introduces the governed synthesis seam; P4 wires the contextual router's privacy tier; P5 ships the per-connector sync policies and Console surfaces; the Vault rides alongside P3+.

---

## Status

Accepted. Implementation begins on the corresponding branch; per-phase implementation prompts are spawned in separate threads with this ADR, the CIP spec, the synthesis seam design note, and the Vault spec pinned.
