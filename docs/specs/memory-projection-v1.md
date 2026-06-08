# NAVI Memory Projection (Vault) Specification

> [!NOTE]
> Part of the [NAVI Systems Map](../architecture/navi-systems-map.md).

**Status:** Proposed
**Version:** 1.0 (Draft 1)
**Primary surface:** Owner-facing Markdown vault (Obsidian / Nextcloud / plain-editor compatible)
**Primary objective:** define how the canonical World Model is **projected** as a human-readable, editable Markdown surface — and how owner edits round-trip back through the same governed intake pipeline that ingests external sources.

> All Go/TypeScript type definitions, folder layouts, and frontmatter fields in this document are **illustrative, not normative**. The contract is the projection direction, the round-trip semantics, and the authority model; concrete signatures and layouts will be finalized during implementation.

---

## 1. Purpose

NAVI's World Model is the authority; the **Memory Vault** is its interface. The Vault exists so the owner can *see*, *search*, *annotate*, and *correct* what NAVI knows about them in a calm, familiar, portable format — Markdown files on disk — without that surface becoming a back door around governance.

Two product realizations drive this spec:

1. **Continuity is only trusted if it is inspectable.** The Vault is what makes the World Model *tangible*: the owner can open a file, see the provenance, and feel the system instead of taking it on faith.
2. **The Vault is a second mouth on the intake pipeline.** External sources enter through connectors at `ContentTrust: external_untrusted`; owner-Vault-edits enter through the same admit → … → synthesize stages at `ContentTrust: owner`. Same machinery, different trust. That symmetry is what keeps the Vault honest as an editable surface.

## 2. Scope

### 2.1 In scope (V1)

* Markdown projection of four entity classes: **Contacts, Knowledge, Memories, Artifacts**.
* **Generic Markdown + YAML frontmatter** layout that works in Obsidian, Nextcloud, and any plain editor — explicitly no editor lock-in.
* **Structured-diff round-trip**: owner edits enter the CIP synthesis seam as `MutationDescriptor`s, not as raw Markdown writes to truth.
* **Reprojection on entity write**: any synthesis-approved write triggers an idempotent re-render of the affected file(s).
* **File-watcher + debounce** on owner-edit ingestion.
* Conflict handling between owner edits and concurrent external deltas.
* Default Vault location under the configured Workspace directory; respects [Workspace V1](workspace-v1.md) boundaries.
* Per-file sync log surfacing what was diffed, approved, or escalated to a Proposal.
* Optional read-only **generated index pages** rendered from summary trees, clearly marked as derived.

### 2.2 Out of scope (V1 — forward references)

* **Vault representation for Events, Proposals, Configuration, History.** History stays internal (append-only, high-volume; better surfaced through retrieval). Events / Proposals / Configuration already have stronger native surfaces in the Console. These classes can be added in later iterations.
* **Real-time multi-user editing** of Vault files (Workspace V1 is single-owner).
* **Vault file-format extensions** beyond YAML frontmatter + Markdown body (no proprietary blocks).
* **Cloud sync of the Vault** (any sync — Nextcloud, Syncthing, iCloud — is the owner's choice and lives outside this spec; the Vault is just files on disk).
* **Cross-device Vault federation** — deferred to roadmap Phase 16 with the rest of federated memory.

## 3. Design goals

* **Entities are truth; Markdown is interface.** The Vault never holds authority. Reprojection is canonical; the file on disk is a rendered view.
* **One pipeline, two mouths.** Vault edits and external deltas share the synthesis seam, governor, and Proposal Queue — no parallel authority.
* **No editor lock-in.** Files must read sensibly in Obsidian, Nextcloud, or `cat`. No proprietary blocks; YAML frontmatter + Markdown body, period.
* **Idempotent reprojection.** Same entity state → same Markdown output, byte-stable on the content fields the projector owns.
* **Owner authority is high but not infinite.** Owner-trust autonomy presets auto-approve most writes, but structural mutations (merge, deprecate, forget) still hit the hard floors from the [synthesis seam](../design/intake-synthesis-seam.md).
* **Truth is protected from accidental owner-side mistakes** (see §5).

## 4. The "second mouth" — where the Vault sits

```
                          External world                Owner
                              │                          │
                              ▼                          ▼
                       ┌─────────────┐           ┌────────────────┐
                       │ Connectors  │           │ Vault edits    │
                       │ (Telegram,  │           │ (Markdown files│
                       │  Gmail, GH) │           │  on disk)      │
                       └─────────────┘           └────────────────┘
                              │                          │
                              ▼                          ▼
                      Admit (trust =          Admit (trust =
                      external_untrusted)      owner)
                              │                          │
                              └──────────────┬───────────┘
                                             ▼
                            (canonicalize → distill → score →
                             embed/extract → SYNTHESIZE → fold)
                                             │
                                             ▼
                                       World Model
                                             │
                                             ▼
                                  ┌──────────────────────┐
                                  │  Vault Projector     │
                                  │  (entity → Markdown) │
                                  └──────────────────────┘
                                             │
                                             ▼
                                  Markdown files on disk
                                  (the Vault)
```

The owner edits Markdown; the file-watcher detects a change; the change is diffed against the entity's current state; the diff becomes a `MutationDescriptor` tagged `ContentTrust: owner`; the [synthesis seam](../design/intake-synthesis-seam.md) decides the disposition; if approved, the World Model updates; the projector rewrites the file.

## 5. The structured-diff principle (truth protection)

When the owner edits a Vault file, the change does **not** overwrite the entity. Instead:

1. The file-watcher captures the post-edit Markdown.
2. The projector parses it into a candidate entity state.
3. The diff against the current entity becomes one or more `MutationDescriptor`s (reinforce / create / merge / deprecate / forget) with `Source = "vault"` and `Trust = owner`.
4. The synthesis seam runs them through `governor.Pipeline.Run`.
5. Approved writes land; structural writes still raise Proposals via hard floors.
6. The projector reprojects the affected file(s) from the new entity state. The owner's content survives; rendering details may be normalized.

**Why structured diff over lossless Markdown storage** — verbatim from the owner's lens:

> Despite owners being the authority of most things, they are not always right or phrase things correctly. Structured diff may avoid accidental poisoning of truth.

That is the core safeguard. A lossless Markdown approach would treat every owner keystroke as a fact write, which would let a typo, a fat-finger save, or a "thinking out loud" edit corrupt the World Model. Structured diff keeps every owner edit on the same authority footing as an external delta: *a proposed mutation, subject to the same governance*. Owner trust raises the bar for auto-approval; it does not eliminate the gate.

**Cost we are accepting:** reprojection may not preserve the owner's exact phrasing of non-content fields (formatting, ordering, whitespace). The owner's *facts* survive; the *rendering* may normalize.

## 6. Projection source — entities only

The Vault projects **entities only**. Summary trees (CIP stage 9, fold) are *not* projected as primary Vault files; they may optionally render as **read-only generated index pages** (clearly marked as derived) under a separate folder, but they are never editable through the Vault and edits to them are ignored.

The rule the doc commits to:

> *Truth comes out as files. Derivations come out as indexes. Indexes are read-only.*

This avoids the failure mode of letting the owner edit a *summary* (a derived artifact) and have those edits silently become authoritative — which would invert the truth/derivation relationship.

## 7. V1 entity coverage

| Entity class | V1 Vault? | Rationale |
|---|---|---|
| **Contacts** | ✅ | The most useful surface — owners overwhelmingly want to see and correct who NAVI thinks people are. |
| **Knowledge** | ✅ | The canonical "facts about the world" — most natural Markdown fit. |
| **Memories** | ✅ | Subjective + significance — benefits from human review and annotation. |
| **Artifacts** | ✅ | Already file-shaped; projection is mostly metadata + link to the actual artifact body (existing [Artifact System](artifact-system-v1.md) handles content). |
| Events | — | Strong native Console/calendar surface; defer. |
| Proposals | — | Has its own queue surface; projecting would duplicate. |
| Configuration | — | Owner-managed via Console; vaulting risks divergence. |
| History | — | Append-only, high-volume, internal; surfaced through retrieval, not editing. |

V1 ships the four; later iterations can extend coverage without changing the projection contract.

## 8. Vault layout & format

**Format:** Markdown body + YAML frontmatter. Plain text. Readable in Obsidian, Nextcloud Notes, VS Code, `cat`, anything.

**Default folder layout** (under `${workspace}/vault/`, configurable):

```
vault/
├── contacts/
│   ├── alex-rivera.md
│   └── …
├── knowledge/
│   ├── projects/
│   │   └── navi-architecture.md
│   └── …
├── memories/
│   └── 2026/05/conversation-with-alex.md
├── artifacts/
│   └── … (metadata projections; bodies live in the Artifact System)
└── _index/                    # read-only generated index pages (§6)
    └── …
```

**Illustrative frontmatter** (the minimum needed for round-trip):

```yaml
---
navi_entity_id: "contact_01HX…"
navi_entity_type: "contact"
navi_confidence: 0.87
navi_created_at: "2026-04-12T19:04:11Z"
navi_updated_at: "2026-05-26T08:31:00Z"
navi_provenance:
  - source: "telegram:acct-1"
    fetched_at: "2026-04-12T19:04:11Z"
    record_id: "tg-msg-9912"
# fields below this line are owner-editable; fields above are projector-managed
---
# Alex Rivera

Met through the NAVI early-tester group. Works on hardware-side firmware.
…
```

Projector-managed fields (above the marker comment) are rewritten on reprojection without consulting the file's prior content. Owner-editable fields are read by the diff parser and round-tripped through synthesis.

## 9. Sync direction & cadence

**Owner → World Model (edits):**

* File-watcher monitors the Vault directory.
* On a write event, a **debounce window** (default 750ms) collects rapid follow-up saves into one diff pass.
* Diff → `MutationDescriptor`(s) → synthesis seam → governor.
* `Approved` writes land; `RequiresConfirmation` raises a Proposal; the owner sees it in the standard Proposal Queue.

**World Model → Vault (reprojection):**

* Any approved entity write (from *any* synthesis caller — external delta, owner edit, or Deep Reflection) triggers reprojection of the affected files.
* Reprojection is **idempotent**: same entity state → same file bytes on projector-owned content.
* The projector takes a short **file-watcher mute** around its own writes so reprojection does not re-enter as a fake owner edit.

The cadence is event-driven by default; a periodic "drift check" reprojects everything on a slow schedule (default daily) to catch any state where files have drifted from canonical.

## 10. ContentTrust & authority

Owner Vault edits enter at `ContentTrust: owner` and consume the existing owner-trust autonomy presets. In practice:

* Append / Reinforce / Create dispositions → `Approved` (autonomy upgrade).
* Merge / Deprecate / Forget → still hit `HardFloorProposalRequired` from the [synthesis seam](../design/intake-synthesis-seam.md). Owner authority is high but does not override hard floors. *The owner can still merge two Contacts — they just do it by approving a Proposal, not by silently editing one Markdown file out of existence.*

Structural intent the owner expresses in Markdown (e.g. deleting a file, replacing one Contact's frontmatter ID with another's) is interpreted as a **request for that mutation**, never as the mutation itself. That keeps `ContentTrust: owner` honest as an authority signal without making owner-side mistakes destructive.

## 11. Conflict handling

When an owner edit and an external delta both target the same entity within a short window:

* Both produce `MutationDescriptor`s; both pass through the synthesizer.
* Owner-trust raises owner-side confidence; external is generally lower confidence.
* If the two mutations are **compatible** (different attributes), both apply.
* If they **conflict** on the same attribute, the synthesizer:
  * Applies the owner edit (higher trust).
  * Records the external delta as History with an "overridden by owner" provenance note.
  * If the conflict crosses a hard floor (e.g. merge), raises a Proposal naming both candidates.

The principle: owner edits win on routine attributes, but no caller — including the owner — bypasses the hard floors.

## 12. Privacy & local-only by default

The Vault is **always local on disk**. Cloud sync of Vault files (Nextcloud, Syncthing, iCloud) is the owner's choice and lives outside this spec — the Vault doesn't know or care whether the disk is also being synced elsewhere.

For the intake distinction: Vault diffs are `PrivacyClass: personal` by default and inherit the entity's privacy class. Embedding / distillation of Vault edits respects the configured CIP privacy mode (Local / Hybrid / Cloud) — a `secret`-class entity edit in Local mode never reaches a cloud model, identical to external intake.

## 13. Observability

Surfaced in NAVI Console (per [Console v2](navi-console-v2.md) and CIP §11):

* Per-file **sync log**: last edit detected, diff summary, resulting dispositions, approvals, Proposals raised.
* **Drift report** from the periodic reprojection sweep.
* **Vault Proposals** are grouped under the same Proposal Queue as external-intake Proposals, tagged with their source.

The owner never has to wonder "did NAVI see my edit?" — the log answers it.

## 14. Out of scope / non-goals

* The Vault is not a notes app. It is a projection of the World Model that *happens to be editable*.
* The projector does not reformat owner prose stylistically; it normalizes structure (frontmatter, headings) but does not rewrite content.
* The Vault does not store secrets the World Model does not already store. It is not a credential store.
* The Vault does not bypass the [synthesis seam](../design/intake-synthesis-seam.md). There is no path from owner-Markdown to World Model that skips governance.
* The Vault is not the only inspection surface — Console, retrieval, and Proposals continue to be first-class.

## 15. Acceptance criteria

1. Owner edits to Vault files become `MutationDescriptor`s tagged `ContentTrust: owner` and flow through `governor.Pipeline.Run`; there is no code path that writes the World Model from a Vault edit while bypassing the governor.
2. Reprojection is idempotent: re-rendering an entity that has not changed produces byte-identical projector-owned content.
3. Structural mutations expressed via the Vault (file delete, frontmatter `entity_id` change, etc.) raise Proposals, never execute silently.
4. A concurrent owner-edit + external-delta on the same entity is resolved per §11 with deterministic outcome.
5. The four V1 entity classes (Contacts, Knowledge, Memories, Artifacts) project into the documented folder layout with the documented frontmatter shape.
6. Files are readable in Obsidian, Nextcloud Notes, and a plain editor with no missing/unrenderable blocks.
7. Per-file sync log is visible in Console with diffs, dispositions, and Proposals.
8. The Vault location is configurable and respects Workspace V1 boundaries.

## 16. Open follow-ups

* **Index page templates.** Which generated index pages are useful enough to ship in V1 (e.g. "All contacts by recency", "Knowledge by topic")? Lean: ship one or two; let the rest grow with usage.
* **Owner annotation channel.** Should the frontmatter expose a free-form `navi_notes:` field that flows into Memory rather than Knowledge? Likely yes, but settle when V1 lands.
* **Forget UX from Vault.** What is the lowest-friction owner gesture that raises a Forget Proposal (e.g. `navi_forget: true` in frontmatter)? Resolve in a Console + Vault joint pass.
* **Coverage expansion.** When and how to add Events / Proposals / Configuration projection without breaking the entities-only-as-authority rule.

---

## Related specs & concepts

* [ADR-012](../adr/ADR-012-context-intake-and-memory-projection.md) — architectural commitment that ratifies this spec
* [context-intake-pipeline-v1.md](context-intake-pipeline-v1.md) — the pipeline the Vault is the second mouth of
* [intake-synthesis-seam.md](../design/intake-synthesis-seam.md) — authority model the Vault inherits
* [memory.md](../concepts/memory.md) — World Model entity classes the Vault projects
* [content-trust.md](../concepts/content-trust.md) — `ContentTrust: owner` semantics the Vault relies on
* [workspace-v1.md](workspace-v1.md) — directory boundaries the Vault sits within
* [artifact-system-v1.md](artifact-system-v1.md) — Artifact bodies the Vault links to (it projects metadata only)
* [navi-console-v2.md](navi-console-v2.md) — Console surfaces (sync log, Proposals) the Vault depends on

[docs specs INDEX](INDEX.md)
