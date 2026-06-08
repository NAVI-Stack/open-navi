# Task spec — NAVI-CIP-P5

## Task ID

NAVI-CIP-P5

## Title

Implement Context Intake Pipeline Phase 5 — Per-connector sync policies, sync log, backfill consent, Console surfaces

## Pinned context (read first, in this order)

1. [ADR-012 — Personal Continuity Layer](../../adr/ADR-012-context-intake-and-memory-projection.md) — architectural commitment.
2. [Context Intake Pipeline V1 spec](../../specs/context-intake-pipeline-v1.md) — **focus on §7 (per-connector sync policy — the load-bearing section for P5), §7.1 (Backfill vs Delta modes), §11 (Console & observability), §14 (P5 row), §15 (acceptance criteria #7, #8 apply to P5).**
3. [NAVI Console V2 spec](../../specs/navi-console-v2.md) — **read the entire "Intake & Vault surfaces (post-MVP addendum)" section. That addendum is the Console contract P5 must satisfy.** Also read the existing app shell, left rail, main pane, right inspector, bottom drawer sections — P5 fits into existing surfaces, **not** a new top-level nav.
4. [Memory Projection (Vault) V1 spec](../../specs/memory-projection-v1.md) — §13 (observability) for the per-file Vault sync log Console contract. The Vault implementation is a parallel deliverable; P5 builds the Console-side hooks the Vault will populate.
5. [NAVI-CIP-P1](NAVI-CIP-P1.md) through [NAVI-CIP-P4](NAVI-CIP-P4.md) task specs and their handoff docs — hard dependencies. P5 is purely additive over P1–P4: it surfaces and configures what already runs.
6. [Intake Synthesis Seam design note §12](../../design/intake-synthesis-seam.md) — grouped Proposals for backfill mode is the Console contract for backfill UX.
7. [CLAUDE.md](../../../CLAUDE.md) — build artifact policy, no-ORM rule. **UI testing note: "If you can't test the UI, say so explicitly rather than claiming success."** P5 must verify Console changes in a browser per the `run` skill / dev server path.
8. Existing code to read before writing: `internal/intake/worker.go` (where sync policy is currently implicit), `internal/intake/admit.go` (where `PrivacyClass` is assigned today), the connector implementations under `plugins/*/connectors/` and `connectors/`, the existing config loader (`internal/config/config.go`), the existing Proposal stores, the Console frontend at `web-src/navi-console/` and the gateway routes at `internal/gateway/`.

## Description

Phase 5 of CIP, per ADR-012. With P1–P4 in place, intake works end-to-end — but it is **operationally invisible**. P5 makes it visible and tunable.

Three deliverables:

- **Per-connector sync policies**: declarative configuration (cadence, budget, cursor strategy, dedupe rule, freshness window, privacy class, visibility) per connector, with separate Backfill and Delta policy blocks. Loaded at startup, runtime-tunable, enforced at the connector emission boundary and at the Governor.
- **Sync log**: append-only record of every sync pass (per connector) and every Vault diff (per file). Powers the Console surfaces and lets the owner answer "did NAVI see my edit?" / "what did the last Telegram pass do?"
- **Console surfaces**: per the Console V2 addendum — per-connector sync state in the connector detail view, recent intake in the right inspector where connector-sourced context exists, grouped backfill Proposals in the existing Proposal queue tagged `source: intake`, Vault sync log per file (UI hooks built; populated when Vault lands), drift report in the operator/debug surface. **No new top-level nav.** **No new Proposal queue.**

P5 also lands **backfill consent UX**: when a connector starts a Backfill-mode pass, the consent gate raises a Proposal in the existing queue that describes scope, budget, and freshness window. Owner approval starts the backfill; no consent means no historical sweep.

## What you must NOT change (Frozen Design Contract)

- **Do not add top-level Console nav items.** Per Console V2 addendum, intake/Vault surfaces live inside existing connector, chat, project, Proposal, and operator/debug surfaces.
- **Do not invent a second Proposal queue.** Backfill consent, intake-synthesized Proposals (from P3), and Vault Proposals (when Vault lands) all flow through the **one** existing Proposal queue with a `source:` tag.
- **Do not invent a new Governor.** Per-connector budgets feed the existing `internal/governor/`. The Governor decides; sync policy supplies the per-connector inputs.
- **Do not invent a new authority surface.** Owner-facing policy edits are configuration, not governed mutations. Changing a connector's cadence from 20m to 60m is a config write; changing what a connector is *permitted* to ingest is policy and goes through the Governor as today.
- **Do not change the pipeline core.** P1–P4 contracts are frozen. P5 reads, configures, and surfaces; it does not modify Admit, Canonicalize, Chunk, Distill, Score, Embed, Extract, Resolve, Synthesize, Fold, or Retrieve.
- **Do not implement Vault content.** The Vault is a parallel deliverable. P5 builds the Console-side hooks the Vault populates (per-file sync log view, Vault Proposals tag, drift report); the Vault writes them.
- **Do not store policy state in the World Model.** Sync policy is configuration; it does not deserve to be an entity. Use a configuration surface (YAML + runtime override store), not the entity tables.
- **No new ORM. Raw SQL only** (ADR-005).
- **No bare `go build`.** Binaries to `bin/` via Make targets.
- **Do not modify** ADR-012, CIP V1 spec, synthesis seam design note, Vault spec, Console V2 spec proper (the addendum is yours to *implement against*, not edit), P1–P4 task specs, or this task file.
- **Do not break P1–P4.** Every existing test must still pass.

## Acceptance criteria

### Per-connector sync policies

- [ ] A sync-policy schema is defined matching CIP §7 (cadence, budget {max_records_per_pass, max_bytes_per_pass, cost_ceiling_usd}, cursor strategy, dedupe rule, freshness window, privacy_class, visibility) with **separate Backfill and Delta blocks** per CIP §7.1.
- [ ] Policies are loaded from configuration (suggested: `config/runtime.yaml` extended, or per-connector YAML under `config/connectors/`). Owner can override at runtime via gateway API; overrides persist in a new `connector_sync_policy` store (raw SQL).
- [ ] The worker honors `cadence` (delta passes) and refuses to start a Backfill pass without consent (see below).
- [ ] Per-pass budget (`max_records_per_pass`, `max_bytes_per_pass`) is enforced at the connector emission boundary; exceeding it terminates the pass cleanly.
- [ ] `cost_ceiling_usd` is enforced via the existing `internal/governor/` — per-connector context flows in; existing cost-ceiling primitives are reused, not rebuilt.
- [ ] `privacy_class` is sourced from policy at Admit (current default-to-`personal` behavior becomes policy-driven).

### Sync log

- [ ] A new `intake_sync_log` table records every sync pass: connector id, started_at, ended_at, job mode (Backfill | Delta), records_admitted, records_deduped, records_distilled, records_synthesized, errors, terminal status. Append-only.
- [ ] A new `vault_sync_log` table records every Vault diff pass: file path, started_at, diff summary, mutations proposed, mutations approved, Proposals raised, errors. Append-only. **Schema only; populated by the Vault deliverable.**
- [ ] Worker emits one `intake_sync_log` row per pass (Backfill batches may emit a parent row + child rows; document the choice).
- [ ] Log entries are queryable via gateway endpoints (suggested: `GET /api/intake/sync-log?connector=…&limit=…` and `GET /api/vault/sync-log?path=…`).

### Backfill consent

- [ ] When a connector's Backfill policy is invoked (cold start, explicit re-sync), a Proposal is raised in the existing queue tagged `source: intake-consent` describing scope (freshness window), budget (records / bytes / cost), and the connector.
- [ ] The Backfill pass does not start until the Proposal is approved. Approval triggers the pass; rejection logs a `intake_sync_log` row with terminal status `consent_rejected`.
- [ ] A re-attempted Backfill on a connector that already has historical state checks the cursor and offers a *resume from cursor* Proposal rather than a full re-sweep.

### Console surfaces (per Console V2 addendum)

The Console frontend lives at `web-src/navi-console/` and builds into `web/`. Build the surfaces; verify in a browser via the dev server (use the `run` skill or document the manual steps).

- [ ] **Connector detail view** shows: current sync policy (cadence, budget, freshness, privacy_class), last pass metrics (admitted/deduped/distilled/synthesized/errors), next scheduled pass, recent errors. Policy is editable from this view; saved edits round-trip through the override store.
- [ ] **Right inspector** for a chat or project that has connector-sourced context shows recent intake with provenance links back to source records.
- [ ] **Proposal queue** renders the `source:` tag (intake-consent, intake-synthesis, vault) on each Proposal. Backfill-batched intake Proposals from P3 render as **grouped** items per the synthesis seam §12; the group affordance lets the owner approve/reject as a set.
- [ ] **Operator/debug surface** shows the drift report (placeholder when Vault not yet shipping data is acceptable; surface exists).
- [ ] No new top-level nav. No new Proposal queue. Verified by inspection.

### Hard invariants

- [ ] **No World Model writes from policy changes or Console interactions.** Extend the existing `worldmodel_isolation_test.go` pattern to cover policy reads and writes.
- [ ] **The Governor remains authoritative for budgets.** P5 supplies inputs; the Governor decides. A test verifies a connector cannot exceed the configured cost ceiling regardless of policy edits.
- [ ] **Existing P1–P4 tests still pass.**

### Build & verification

- [ ] `make test` passes.
- [ ] `go vet ./...` clean.
- [ ] `make build-all` succeeds; both binaries land in `bin/`.
- [ ] Console surfaces verified in a running browser via `make docker-up` (or equivalent) — document the verification in the handoff with one screenshot per new surface, OR explicitly state which surfaces could not be verified and why (per CLAUDE.md).

## Surface

- New: `internal/intake/policy/` — sync-policy schema, loader, validator, default policies per connector
- New: `internal/store/connector_sync_policy.go` — raw-SQL store for runtime overrides
- New: `internal/store/intake_sync_log.go` and `internal/store/vault_sync_log.go` — append-only log stores (schema for both; populator for intake; schema-only for vault)
- New: gateway routes — `GET /api/intake/policy`, `POST /api/intake/policy`, `GET /api/intake/sync-log`, `GET /api/vault/sync-log`
- Modified: `internal/intake/worker.go` — honor cadence, enforce per-pass budget, write sync-log rows
- Modified: `internal/intake/admit.go` — source `PrivacyClass` from policy
- Modified: `connectors/` and `plugins/*/connectors/` — declare default policies; consult cadence
- Modified: existing Proposal queue UI to render `source:` tag and group affordance
- Modified: Console frontend at `web-src/navi-console/` — connector detail view extension, right-inspector recent-intake panel, operator drift-report panel, Proposal queue updates
- Modified: `internal/governor/` minimally — accept per-connector context for cost ceiling enforcement (additive; do not refactor)
- Test surface: unit tests for policy loader, override store, sync-log stores, gateway routes, worker policy enforcement; end-to-end test covering (a) policy edit round-trips, (b) Backfill consent gate raises a Proposal and refuses to start without approval, (c) cost ceiling halts a runaway pass, (d) sync log captures a full pass, (e) Console renders the surfaces with seeded data.

## Suggested implementation order

1. Read all pinned context. Console V2 addendum + CIP §7/§7.1/§11 are the load-bearing reads.
2. Define the sync-policy schema and loader. Land default policies for Telegram (and any other shipping connector). Unit tests.
3. Build the override store and the policy gateway routes. Tests.
4. Build the sync-log stores (both intake and Vault schemas). Tests.
5. Wire the worker to honor cadence, enforce per-pass budget, source `PrivacyClass` from policy, and write sync-log rows.
6. Implement Backfill consent: raise a Proposal at Backfill-pass start; gate execution on approval; persist resume-from-cursor state. Tests.
7. Wire per-connector cost-ceiling context into the existing Governor. Tests.
8. Console frontend: connector detail view extension, right-inspector recent-intake panel, Proposal queue source tag + group affordance, operator drift-report panel.
9. Verify Console in a browser (dev server). Capture screenshots or document the gap.
10. Extend `worldmodel_isolation_test.go` to cover policy interactions.

## Priority

HIGH — without P5, intake is functional but operationally invisible; owners cannot trust what they cannot see or tune.

## Out of scope for P5 (explicit)

- Vault content / projection / structured-diff round-trip (parallel deliverable, has its own spec)
- New top-level Console nav
- New Proposal queue
- Plugin-tier policy authority (placeholder slot in the governor today; future)
- Cross-device federation (Phase 16)
- Sophisticated per-record privacy classifiers (V1 uses policy default; per-record classification can iterate later)
- Connectors not currently implemented (Slack stub, Discord, Signal, WhatsApp — covered by their own task specs)
- Backfill *progress* UI beyond the consent Proposal (future iteration; sync log is sufficient for V1)
