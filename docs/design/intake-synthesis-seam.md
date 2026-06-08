# Intake Synthesis Seam — Design Note

**Status:** Proposed
**Last Updated:** 2026-05-28
**Updated By:** agent
**Package (target):** new caller of `internal/governor`; Subconscious surface in `internal/navi`
**Companions:** [ADR-012 Personal Continuity Layer](../adr/ADR-012-context-intake-and-memory-projection.md) · [Context Intake Pipeline v1](../specs/context-intake-pipeline-v1.md) · [Governance tier resolution (GOV-04)](governance-tier-resolution.md) · [Memory & World Model concept](../concepts/memory.md) · [Content Trust](../concepts/content-trust.md)

> All Go/Python type definitions and SQL fragments in this document are **illustrative, not normative**. The contract is the policy and the boundary; concrete signatures will be finalized during implementation.

---

## 1. Summary

This note specifies the **synthesize stage (stage 8)** of the Context Intake Pipeline — the only stage that writes the World Model — and pins down two things the CIP spec deliberately left open:

1. The **authority model** for synthesis writes (extension of the existing `internal/governor` engine, not a parallel one).
2. The **Go/Python boundary** inside the seam (Python proposes a resolution; Go's Governor decides the outcome).

The synthesize stage is the highest-risk seam in the pipeline and is given its own design note because the rest of CIP (admit → distill → score → embed/extract → fold → retrieve) can ship with zero World Model writes; the synthesis decisions made here are what gate when, how, and under whose authority intake actually mutates canonical state.

## 2. Why a separate design note

CIP scopes *what* flows through the pipeline. This note scopes *how authority is exercised* at the one mutating stage, because:

- it touches the World Model entity model and the Autonomy Model deeply,
- the right answer turns out to be **reuse**, and reuse is easier to argue from code than from concept, and
- the Subconscious calls this engine in batched/async mode, which is a posture the existing governance docs don't yet describe.

## 3. Anchors in existing code

The decision-boundary governance is already built and is **plane-agnostic**:

- `internal/governor/validate.go` — `ValidationOutcome` (`Approved`, `RequiresConfirmation`, `Modified`, `Rejected`); `Pipeline` of ordered `CheckFunc`s (Permissions → Policy → Configuration → Priority → Risk); `ValidationResult{Outcome, ModifiedAction, Tier}`. `Pipeline.Run(action)` is a pure per-call decision.
- `internal/governor/autonomy.go` — `ApplyAutonomy` downgrades `RequiresConfirmation → Approved` per preset; **hard floors** (`HardFloorProposalRequired`, `RiskTier == "high"`) autonomy can never override.
- [Governance tier resolution](governance-tier-resolution.md) — System > Owner > Plugin; most-restrictive-within-tier; tier-first across.

The synthesis seam **inherits all of the above by being a new caller of the same engine.**

## 4. Position: extension, not mirror

Synthesis is modeled as a **new governed effect class** — `world_model_mutation` — flowing through the existing `governor` engine. The disposition ladder (§6) is the **Risk rubric** that classifies a candidate write so the engine produces the right `ValidationOutcome` for it.

We do *not* fork `Pipeline` for writes. Forking would create a second authority and a second Proposal-like queue — exactly the duplication NAVI's guardrails resist.

What is extended:

- governance's **effect vocabulary** (a write-class effect),
- the **set of callers** (the Subconscious synthesizer joins the Conscious loop as a caller).

What is *not* extended:

- the engine's execution assumptions — `Pipeline.Run` stays a pure per-decision call; the Subconscious batches at the caller level.
- the outcomes, autonomy presets, hard floors, tier resolution, restrictiveness ordering — all reused as-is.
- the Proposal Queue — there is one.

## 5. `MutationDescriptor` (sibling, not overload)

Synthesis writes are described by a sibling descriptor to `ActionDescriptor`. Both are consumed by the same `Pipeline.Run`; they differ only in inputs because writes and actions are honestly different effects.

```go
// illustrative, not normative
type MutationKind int

const (
    MutationAppendHistory       MutationKind = iota // append-only
    MutationReinforceEntity                          // bump confidence, add attribute
    MutationCreateEntity                             // new entity from extraction
    MutationMergeEntities                            // structural — hard-floored
    MutationDeprecateEntity                          // structural — hard-floored
    MutationForgetMemory                             // existing Forget path — hard-floored
)

type MutationDescriptor struct {
    Kind           MutationKind
    TargetEntity   string             // canonical id, or "" for Create
    CandidateRefs  []string           // entity ids in scope of a merge/dedupe
    SourceRecords  []IntakeRecordRef  // provenance — never empty
    Trust          ContentTrust       // from Admit
    PrivacyClass   PrivacyClass
    Resolution     ResolutionResult   // from §9 (Python side)
    Derivation     DerivationChain    // how this candidate was produced
    EstConfidence  float64            // 0..1, from extraction + resolution
    JobMode        JobMode            // Backfill | Delta — drives Proposal grouping
}
```

> **Why sibling, not overload of `ActionDescriptor`:** different inputs (effect kind, target entity, resolution result, derivation chain) vs. command-type/domain/tags. Overloading would muddy two honest call sites. The engine's `Pipeline` accepts both because the checks operate on the descriptor's *effect surface*, not its concrete type.
>
> **Future consideration:** if `ActionDescriptor` and `MutationDescriptor` start sharing the majority of fields as governance matures, collapsing to a single descriptor with an effect-kind discriminator is a reasonable later refactor. Not in V1.

## 6. The disposition ladder — Risk rubric for `world_model_mutation`

The ladder classifies a candidate write by reversibility and blast radius. It is the input to the existing Risk step, which produces the `ValidationOutcome` the engine returns.

| Disposition | Reversibility | Default `ValidationOutcome` | Notes |
|-------------|---------------|------------------------------|-------|
| Append History | append-only; failure is first-class | `Approved` | unconditionally safe |
| Reinforce existing entity | reversible (confidence bumps; attribute add) | `Approved` (autonomy-gated) | `ApplyAutonomy` may downgrade in low-autonomy presets |
| Create new entity | reversible (delete path exists) | `Approved` / `RequiresConfirmation` | autonomy preset per domain |
| Merge entities | hard to reverse (identity collapse) | `RequiresConfirmation` + **hard floor** | `HardFloorProposalRequired` |
| Deprecate / Forget | destructive on Knowledge/Memory | `RequiresConfirmation` + **hard floor** | `HardFloorProposalRequired` |
| Ambiguous resolution | low-confidence match | `Modified` | downgrade Merge → low-confidence Create, see §7 |

The ladder is **deterministic Go** (control-kernel logic), not a script and not a prompt. It's a fixed mapping from `(MutationKind, Resolution.Confidence)` to a Risk classification, evaluated by the Risk `CheckFunc` inside `Pipeline.Run`.

## 7. Outcome mapping — the ladder collapses onto what exists

| Synthesis disposition | `ValidationOutcome` | Mechanism in code today |
|---|---|---|
| Append / Reinforce / Create (autonomy-approved) | `Approved` | `ApplyAutonomy` upgrade |
| Create (low autonomy) | `RequiresConfirmation` | Proposal |
| Ambiguous (e.g. uncertain merge) | `Modified` | `ModifiedAction` carries the softened write — e.g. "Create as low-confidence entity tagged `possible_merge_with=<id>`" instead of merging |
| Merge / Deprecate / Forget | `RequiresConfirmation` + hard floor | `HardFloorProposalRequired` (autonomy cannot override) |
| Policy/Configuration veto | `Rejected` | existing Policy / Configuration `CheckFunc`s |

The `Modified` outcome is load-bearing: it lets the seam say *"I won't do the risky thing, but I have a safer thing I can do unilaterally,"* without inventing new machinery.

## 8. Tier reuse

Per [governance tier resolution](governance-tier-resolution.md), the same System > Owner > Plugin precedence and most-restrictive-within-tier rule apply:

- **System** — Permissions (trivially satisfied for internal writes), Policy (e.g. "never auto-merge contacts"), Risk (the §6 ladder).
- **Owner** — Configuration (owner-set: "always ask before deprecating Knowledge"), Priority Alignment.
- **Plugin** — (future) plugin-scoped checks.

No new tiers, no new restrictiveness order. The Subconscious synthesizer is just another caller producing a `ValidationResult` and acting on it.

## 9. The Go / Python split inside the seam

The seam has two distinct jobs. They map onto NAVI's already-stated language layering:

| Job | Layer | Why |
|---|---|---|
| **Resolution matching** — is this extracted entity the same as an existing one? | **Python** | scriptable judgment + embeddings; tunable without redeploying the kernel |
| **Disposition → outcome** — given effect + risk, what's the `ValidationOutcome`? | **Go** (`governor`) | authoritative control kernel; deterministic; persistent |

**Framing (owner's lens — adopted verbatim):** Go is the core engine that owns capability; Python is the scripting layer that runs the game code via calls and interfaces. The Python side performs resolution matching and *calls* the Go-implemented Governor for the final outcome. **Python decides and judges; Go authoritatively approves, denies, or escalates the produced results as outcomes.** That gives the resolution layer room to improve without ever destabilizing the authority layer.

Direction of dataflow is always **Python → Go**, never the reverse:

```
[ Extracted entity candidate ]                    Python layer
            │
            ▼
  Resolution matcher  ──►  ResolutionResult{ MatchedID?, Confidence, Reason }
            │
            ▼
  Build MutationDescriptor                         (Go boundary)
            │
            ▼
  governor.Pipeline.Run(desc)  ──►  ValidationResult
            │
            ▼
  Approved → write   |   RequiresConfirmation → Proposal
  Modified → write softened   |   Rejected → drop + log
```

Python **never writes** the World Model and **never decides** authority. It informs.

## 10. Resolution matching (the Python side, in brief)

Resolution runs in two passes; we don't need accuracy, we need safety:

1. **Deterministic blocking keys** — exact identifiers (email, handle, normalized name, source-anchored id). Hits here are high-confidence matches that produce `Reinforce` dispositions.
2. **Similarity** — embeddings + simple scoring across the candidate set. High-similarity unique matches are `Reinforce`; multi-candidate or low-similarity becomes **ambiguous**, which the Risk rubric maps to `Modified` (write a low-confidence Create with a `possible_merge_with` tag) — not a guess at a merge.

Ambiguity is a feature, not a failure. An uncertain match defers to the owner via a Proposal rather than corrupting the graph. The matcher's job is to be *honest about uncertainty*, not to be right.

## 11. Confidence model

Synthesis assigns a confidence in `[0, 1]` to every write it produces. Confidence is a blend of:

- source trust (per `ContentTrust` and connector reputation),
- extraction confidence (from stage 7),
- resolution confidence (from §10).

Confidence is **persisted on the entity** (existing concept — see [memory.md](../concepts/memory.md)). Deep Reflection can later re-score and either promote (raise confidence, change disposition next time) or deprecate. Confidence is data; it does not bypass governance.

## 12. Proposal granularity

| Job mode | Granularity | Rationale |
|---|---|---|
| **Delta** | one Proposal per write | volumes are small; per-write context is clear |
| **Backfill** | **grouped Proposals** ("12 contact merges from Gmail backfill — review as a set") | backfill volume would bury the owner; grouping aligns with the batch/holistic tone the owner already opted into when starting the backfill |

Grouping is a property of the synthesis batch, not of the governor. The governor still produces per-`MutationDescriptor` `ValidationResult`s; the Subconscious batches the `RequiresConfirmation` results into a single grouped Proposal for the owner. One queue, two presentation modes.

> Heavier UX lift, but implied by the nature of backfill — and it lets us avoid splitting the Proposal Queue.

## 13. Idempotency & re-synthesis

A re-run of synthesis over the same `IntakeRecord` (replay, cursor rewind, restart) must not produce duplicate entities or duplicate Proposals:

- `(ConnectorID, SourceID)` dedupe at Admit prevents re-ingestion of the same source record.
- Synthesis additionally checks: *does an entity already linked to this source record and derivation exist?* If yes → no-op. If yes but with materially different extraction → it's a Reinforce candidate, not a Create.
- Open Proposals carry the candidate's derivation chain; a replayed candidate matching an open Proposal is a no-op, not a duplicate Proposal.

Re-synthesis is therefore safe by construction: replay is a property of the pipeline, not a risk for the World Model.

## 14. Un-promoted chunks remain retrievable

Chunks that **never** become entities (didn't clear scoring, didn't resolve, low significance) are *not lost*. They remain in the chunk store and the summary tree (stage 9) and remain available to retrieval, filtered by `ContentTrust` and `PrivacyClass`. This is what lets P1–P2 of CIP ship real *continuity value* before P3 ever writes the World Model.

The principle: **synthesis is promotion, not gating.** Retrieval does not require promotion.

## 15. Out of scope / non-goals

- Changing the governor engine's tier model, outcome enum, or restrictiveness order.
- A second Proposal Queue or any synthesis-specific authority surface.
- Heuristic risk classification in prompts (Risk lives in Go).
- LLM-driven authority decisions ever overriding hard floors.
- Owner-Vault write-back semantics — specified in [memory-projection-v1.md](../specs/memory-projection-v1.md). The seam's contract is symmetric (owner-trust writes are just another caller of the same engine); the Vault spec handles diffing, reprojection, and conflict resolution.

## 16. Acceptance criteria

1. Synthesis builds a `MutationDescriptor` and calls `governor.Pipeline.Run`; it never writes the World Model on any path that bypasses the engine.
2. `Approved` outcomes write; `RequiresConfirmation` outcomes produce Proposals; `Modified` outcomes write the softened action carried by the result; `Rejected` outcomes drop and log.
3. `MutationMergeEntities`, `MutationDeprecateEntity`, and `MutationForgetMemory` carry `HardFloorProposalRequired` and are never auto-approved by any autonomy preset.
4. Resolution matching code is implemented in Python and produces a `ResolutionResult` that the Go boundary consumes; the Go path contains no embedding-similarity logic.
5. Backfill-mode synthesis batches `RequiresConfirmation` outcomes into grouped Proposals; delta-mode emits per-write Proposals.
6. A replayed `IntakeRecord` produces zero duplicate entities and zero duplicate Proposals.
7. Un-promoted chunks remain retrievable from the chunk store and summary tree, filtered by trust/privacy.
8. Every synthesized entity carries provenance back to its `IntakeRecord`(s), fetch time, cursor, and a confidence in `[0, 1]`.

## 17. Future considerations

- **Descriptor convergence.** If `ActionDescriptor` and `MutationDescriptor` drift toward sharing most of their shape, collapse to one descriptor with an effect-kind discriminator. Track during P3.
- **Owner-Vault write-back.** Owner edits in the Memory Vault enter through the same seam tagged `ContentTrust: owner`, which makes most writes auto-approvable — but structural changes (e.g. merging two Contacts via the Vault) still hit the hard floors. Detail in [memory-projection-v1.md](../specs/memory-projection-v1.md).
- **Plugin-tier synthesis checks.** Plugin tier in the governor is currently a placeholder; if plugins gain authority to participate in synthesis (e.g. a domain-specific entity resolver), the existing tier slot is where they plug in.

---

[docs design INDEX](INDEX.md)
