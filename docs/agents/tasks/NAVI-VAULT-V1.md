# Task spec — NAVI-VAULT-V1

## Task ID

NAVI-VAULT-V1

## Title

Implement Memory Vault V1 — entities-only Markdown projection, structured-diff round-trip, owner-edit ingest

## Pinned context (read first, in this order)

1. [Memory Projection (Vault) V1 spec](../../specs/memory-projection-v1.md) — **the load-bearing document. Read in full.** This task implements that spec directly.
2. [ADR-012 — Personal Continuity Layer](../../adr/ADR-012-context-intake-and-memory-projection.md) — architectural commitment, especially D5–D7 (Vault as second mouth, structured-diff, entities-only).
3. [Context Intake Pipeline V1 spec](../../specs/context-intake-pipeline-v1.md) — the pipeline the Vault is the second mouth of. Owner edits enter at Admit tagged `ContentTrust: owner` and flow through admit → … → synthesize, identical to external sources downstream.
4. [Intake Synthesis Seam design note](../../design/intake-synthesis-seam.md) — the authority model owner edits inherit. **Pay particular attention to §17 "Future considerations → Owner-Vault write-back"** and the seam's `MutationDescriptor` shape from §5.
5. [Content Trust model](../../concepts/content-trust.md) — `owner` vs `external_untrusted` semantics and where boundary tagging happens. Owner-trust raises the autonomy bar but does **not** override hard floors.
6. [Memory & World Model concept](../../concepts/memory.md) — the four entity classes V1 projects (Contacts, Knowledge, Memories, Artifacts).
7. [Workspace V1 spec](../../specs/workspace-v1.md) — directory boundaries the Vault sits within; the Vault default location is under the configured Workspace.
8. [Artifact System V1 spec](../../specs/artifact-system-v1.md) — the Vault projects Artifact *metadata*; the artifact body lives in the existing Artifact System.
9. [NAVI-CIP-P1](NAVI-CIP-P1.md) through [NAVI-CIP-P5](NAVI-CIP-P5.md) task specs and their handoff docs. **Hard dependencies:** P3 (synthesis seam — the engine owner edits flow through), P5 (the `vault_sync_log` schema). **Verify P3 is merged.** **Verify P5 is merged or coordinate with the P5 thread before defining schemas this task expects from it.**
10. [Console V2 spec](../../specs/navi-console-v2.md) — the "Intake & Vault surfaces (post-MVP addendum)" section describes the Console hooks this task populates with real data (sync log entries, source-tagged Proposals).
11. [CLAUDE.md](../../../CLAUDE.md) — build artifact policy, no-ORM rule. **UI testing note applies if any Console verification is needed.**
12. Existing code to read before writing: `internal/intake/` (the pipeline the Vault joins), `internal/intake/admit.go` (where `ContentTrust` is assigned), `internal/intake/synthesize/` from P3 (the seam owner edits flow through), `internal/governor/` (the engine), the existing entity stores (Contacts/Knowledge/Memories/Artifacts), `internal/store/vault_sync_log.go` from P5 (populator target).

## Description

Implements the Memory Vault per [memory-projection-v1.md](../../specs/memory-projection-v1.md). The Vault is the owner-facing Markdown projection of the World Model and the editable surface back into it.

The architectural commitment: the Vault is **the second mouth on the intake pipeline**. Owner edits to Vault files enter at the same Admit stage external sources do, tagged `ContentTrust: owner`, and flow through the same synthesis seam built in P3. Same machinery, different trust. There is **one** acquisition pipeline; the Vault is not a parallel surface.

Five things ship in this task:

- **Projector** — entity → Markdown rendering with YAML frontmatter. Idempotent: same entity state → byte-identical projector-owned content.
- **File-watcher + debounce** — captures owner edits and triggers diff passes.
- **Diff parser** — Markdown → structured entity diffs → `MutationDescriptor`s tagged `ContentTrust: owner`. **Not lossless Markdown storage.**
- **Worker orchestration** — wires diff parsing → synthesis seam → write/Proposal/drop. Reprojection on any approved entity write (from *any* synthesis caller — external delta, owner edit, Deep Reflection). File-watcher mute around projector writes.
- **Periodic drift sweep** — regenerates all Vault files from canonical state on a slow schedule to catch divergence.

V1 entity coverage: **Contacts, Knowledge, Memories, Artifacts**. History, Events, Proposals, Configuration are deferred.

## What you must NOT change (Frozen Design Contract)

- **Entities are truth; Markdown is interface.** No Vault file is authority. A path that writes the World Model from a Vault edit while bypassing the governor is a bug, not an optimization.
- **No lossless Markdown storage.** Owner edits enter as structured diffs against the entity, not as raw Markdown writes to truth. This is the safeguard against accidental owner-side poisoning (Vault spec §5 — read the rationale in the owner's voice).
- **No second Proposal queue.** Vault Proposals land in the **one** existing queue tagged `source: vault`.
- **No bypass of the synthesis seam.** Every owner edit becomes a `MutationDescriptor` passed to `governor.Pipeline.Run`. The seam was built in P3; this task is a new *caller* of it, not a new authority.
- **Hard floors apply to owner edits.** `MutationMergeEntities`, `MutationDeprecateEntity`, `MutationForgetMemory` from Vault still carry `HardFloorProposalRequired`. The owner can merge two Contacts — they just do it by approving a Proposal, not by editing one Markdown file out of existence.
- **Structural intent in Markdown is a request, not an execution.** File deletion, frontmatter `entity_id` swap, etc. are interpreted as requests for that mutation (which raise Proposals), never as the mutation itself.
- **The Vault is local on disk.** Cloud sync (Nextcloud, Syncthing, iCloud) is the owner's choice and lives outside this task. The Vault does not know or care whether the disk is also being synced.
- **No editor lock-in.** Files must read sensibly in Obsidian, Nextcloud Notes, VS Code, `cat`. YAML frontmatter + Markdown body, period. No proprietary blocks.
- **No projection of summary trees as authority.** Summary trees may render as read-only generated index pages under a separate folder; they are never editable through the Vault.
- **No projection of out-of-scope entity classes.** V1 covers Contacts, Knowledge, Memories, Artifacts. History, Events, Proposals, Configuration are explicitly deferred.
- **No changes to ADR-012, CIP V1 spec, synthesis seam design note, Vault spec, P1–P5 task specs, or this task file.**
- **No changes to the synthesizer or governor engine.** P3's contract is frozen. This task is a new caller. (Minor field additions to the existing `MutationDescriptor` to carry Vault-specific provenance are acceptable if additive and non-breaking.)
- **No breaking P1–P5.** Every existing test must still pass.
- **No new ORM.** Raw SQL only (ADR-005).
- **No bare `go build`.** Binaries to `bin/` via Make targets.

## Acceptance criteria

### Projector (entity → Markdown)

- [ ] Projector renders each V1 entity class (Contact, Knowledge, Memory, Artifact) to a Markdown file with YAML frontmatter. Frontmatter fields per Vault spec §8 (`navi_entity_id`, `navi_entity_type`, `navi_confidence`, `navi_created_at`, `navi_updated_at`, `navi_provenance`).
- [ ] Default folder layout per Vault spec §8 (`vault/contacts/`, `vault/knowledge/…`, `vault/memories/YYYY/MM/…`, `vault/artifacts/`, `vault/_index/` for read-only generated index pages). Default location is under the configured Workspace; the path is configurable.
- [ ] **Reprojection is idempotent**: re-rendering an entity that has not changed produces byte-identical projector-owned content (the marker-comment delineates projector-owned vs owner-editable regions).
- [ ] Files are readable in Obsidian, Nextcloud Notes, and a plain editor (`cat`) with no missing/unrenderable blocks. Verified manually.
- [ ] Generated index pages (V1: one or two — e.g. "All contacts by recency", "Knowledge by topic") render under `vault/_index/` and are marked read-only. Edits to them are detected and **ignored** (logged at debug, not raised as Proposals).

### File-watcher + debounce

- [ ] A file-watcher monitors the Vault directory.
- [ ] On a write event, a **debounce window** (default 750ms, configurable) collects rapid follow-up saves into a single diff pass.
- [ ] The watcher takes a **mute window** around the projector's own writes so reprojection does not re-enter as a fake owner edit.

### Diff parser & MutationDescriptor construction

- [ ] Diff parser reads a modified Vault file, extracts the post-edit candidate entity state from frontmatter + body, and produces a structured diff against the current persisted entity.
- [ ] The diff is translated into one or more `MutationDescriptor`s (per P3's shape) with `Source = "vault"` and `Trust = owner`.
- [ ] Each `MutationDescriptor` carries provenance back to the Vault file path and the diff that produced it (so the sync log can show what changed).

### Round-trip through synthesis seam

- [ ] Owner-edit `MutationDescriptor`s are passed to `governor.Pipeline.Run` (P3 caller). The Vault has no World Model write path that bypasses this.
- [ ] **Approved** writes land in the World Model; the projector reprojects the affected file(s).
- [ ] **RequiresConfirmation** writes raise a Proposal in the existing queue tagged `source: vault`.
- [ ] **Modified** outcomes write the softened action carried by `ModifiedAction`.
- [ ] **Rejected** outcomes drop with a sync-log entry.
- [ ] Hard floors apply: Merge / Deprecate / Forget from the Vault always raise Proposals; tested.

### Structural intent (no silent execution)

- [ ] **File deletion** of an entity file raises a Forget Proposal in the queue tagged `source: vault`; it does **not** delete the entity.
- [ ] **Frontmatter `entity_id` change** raises a merge Proposal; it does not silently re-link the file.
- [ ] **Frontmatter `navi_forget: true`** (optional V1 affordance — wire if simple, defer if not) raises a Forget Proposal.

### Conflict handling (Vault spec §11)

- [ ] When an owner edit and an external delta target the same entity within a short window: owner trust raises the owner-side confidence; the synthesizer applies the owner edit on routine-attribute conflicts; the external delta is recorded as History with an "overridden by owner" provenance note; structural conflicts (e.g. competing merges) raise a Proposal naming both candidates. Tested with a fixture.

### Reprojection

- [ ] Any **approved** entity write from any synthesis caller (external delta, owner edit, Deep Reflection) triggers reprojection of affected file(s).
- [ ] Reprojection is idempotent; the file-watcher mute prevents projector writes from re-entering as owner edits.
- [ ] **Periodic drift sweep** (default daily, configurable) reprojects everything and emits a drift report into the operator/debug surface. Drift report rows go into `vault_sync_log` (P5's schema).

### Sync log

- [ ] Every Vault diff pass writes a row to `vault_sync_log` (P5 schema): file path, started_at, diff summary, mutations proposed, mutations approved, Proposals raised, errors.
- [ ] If P5 has not yet shipped the schema, **coordinate with the P5 thread** — do not duplicate schema work. If urgent, land the schema here and document the coordination in the handoff.

### Privacy & locality

- [ ] Vault is local on disk. The codebase does not synchronize Vault files to any external service — third-party sync remains the owner's choice outside this task.
- [ ] Vault diffs are `PrivacyClass: personal` by default and inherit the entity's privacy class. Embedding / distillation of Vault edits respects the configured CIP privacy mode (P4 routing) — a `secret`-class entity edit in Local mode never reaches a cloud model. Tested.

### Hard invariants

- [ ] **No World Model writes bypass the synthesis seam.** Extend the existing `worldmodel_isolation_test.go` pattern to verify every Vault-driven write went through `governor.Pipeline.Run`.
- [ ] **Hard floors are unbreakable from the Vault path.** Tested: owner-trust autonomy cannot auto-approve Merge / Deprecate / Forget.
- [ ] **Existing P1–P5 tests still pass.**

### Build & verification

- [ ] `make test` passes.
- [ ] `go vet ./...` clean.
- [ ] `make build-all` succeeds; both binaries land in `bin/`.
- [ ] A live smoke test verifies: (a) start daemon, (b) projector renders existing Contact entities into `vault/contacts/`, (c) manually edit one in an external editor, (d) verify the change round-trips through the seam and reprojection produces idempotent output, (e) attempt a file delete and verify a Forget Proposal appears in the queue. Capture in the handoff.

## Surface

- New: `internal/vault/` — top-level package
- New: `internal/vault/projector/` — entity → Markdown rendering with stable frontmatter
- New: `internal/vault/diff/` — Markdown → entity-diff parsing
- New: `internal/vault/watcher/` — fsnotify-based watcher with debounce and projector-mute
- New: `internal/vault/worker.go` — orchestrates owner-edit → MutationDescriptor → synthesis seam; periodic drift sweep
- New: `internal/store/vault_state.go` — file ↔ entity mapping, last-projected content hash (for drift detection)
- New: `internal/store/vault_sync_log.go` — populator only if P5 has shipped the schema; otherwise coordinate
- Modified: `internal/intake/synthesize/` (P3 surface) — verify the existing `MutationDescriptor` handles `ContentTrust: owner` without changes; minimal additive fields if necessary (e.g. `Source = "vault"`, file-path provenance)
- Modified: `cmd/navid/main.go` — wire vault worker lifecycle into daemon startup/shutdown
- Modified: `internal/config/config.go` — vault location and debounce/drift configuration
- Test surface: package tests for projector, diff, watcher, worker; end-to-end test exercising the full owner-edit round-trip including (a) approved write, (b) RequiresConfirmation Proposal, (c) hard-floor Forget on file delete, (d) replay idempotency, (e) reprojection mute, (f) conflict handling between owner edit and external delta, (g) drift sweep, (h) privacy-mode refusal for `secret` in Local.

## Suggested implementation order

1. Read all pinned context. Vault spec is the load-bearing read.
2. **Verify P3 and P5 are merged.** If P5 has not shipped `vault_sync_log` schema, coordinate before duplicating it.
3. Land the `vault_state` store (file ↔ entity ↔ hash). Tests.
4. Build the projector for one entity class first (Contacts is the easiest, most-rendered case). Land the frontmatter + body shape; assert idempotent reprojection. Tests.
5. Extend the projector to the other three classes (Knowledge, Memories, Artifacts metadata). Tests.
6. Build the file-watcher with debounce and projector-mute. Tests.
7. Build the diff parser. Markdown → entity-state candidate → structured diff → `MutationDescriptor`. Tests.
8. Wire the worker: diff → synthesis seam (existing P3 caller pattern) → write or Proposal. Tag `source: vault` on Proposals. Tests.
9. Implement the file-delete = Forget Proposal path (and other structural-intent paths). Tests.
10. Implement reprojection on approved writes from any synthesis caller; verify mute prevents re-entry.
11. Implement the periodic drift sweep and `vault_sync_log` writes.
12. Conflict handling between owner edit + external delta on the same entity. Tested with a fixture.
13. Privacy-mode refusal test (`secret` in Local).
14. Extend `worldmodel_isolation_test.go` and add the Vault-specific invariant tests.
15. Live smoke test per the build-and-verification criterion; capture in handoff.

## Priority

HIGH — this completes the personal continuity layer. CIP makes the World Model *correct*; the Vault makes it *trustable* by making it tangible.

## Out of scope for V1 (explicit)

- Projection of Events, Proposals, Configuration, History entity classes (later iteration)
- Real-time multi-user editing (Workspace V1 is single-owner)
- Vault-specific cloud sync (owner's choice; outside this task)
- Cross-device Vault federation (Phase 16)
- Vault-format extensions beyond YAML frontmatter + Markdown body
- Sophisticated index page templates beyond V1 minimum
- Owner-annotation `navi_notes:` channel beyond a deferred-to-iteration affordance
- Console-side new top-level nav for the Vault (it uses existing surfaces per Console V2 addendum)
- Implementation of the P5 `vault_sync_log` schema (P5's responsibility; coordinate, don't duplicate)
