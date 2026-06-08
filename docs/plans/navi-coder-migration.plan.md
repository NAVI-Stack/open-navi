# NAVI Coder Migration Plan

**Status:** Active  
**Last Updated:** 2026-06-05  
**Updated By:** ChatGPT

## Purpose

This plan migrates the current NAVI Programmer prototype into NAVI Coder, a first-class product domain inside NAVI.

It is not a feature expansion plan. It is a reorganization and product-boundary correction plan.

Canonical source: [`docs/canonical/navi-coder.md`](../canonical/navi-coder.md).  
Architecture source: [`docs/architecture/navi-coder-product-architecture.md`](../architecture/navi-coder-product-architecture.md).

---

## Core Decision

NAVI Coder is the first-class programming product domain.

The current `navi.programmer` implementation is a transitional prototype substrate. It contains useful work, but its product framing is now stale.

Migration direction:

```text
plugins/navi-programmer
        ↓
Coder core capability pack
        ↓
NAVI Coder product domain
```

---

## Migration Goals

1. Rename the user-facing and product-facing concept from Programmer to Coder.
2. Add Coder as a top-level console product route.
3. Add Coder as a backend domain/control plane in `navid`.
4. Preserve useful current skills/workflows instead of rewriting them blindly.
5. Move risky execution toward isolated Coder workers.
6. Keep self-update as ordinary Coder capability applied to the NAVI repo with stricter candidate runtime gates.
7. Remove or archive stale documentation that presents Programmer as the product.

---

## Non-Goals

This plan does not require:

- deleting all `programmer` strings in one pass
- breaking existing runtime skill discovery
- splitting Coder into a separate app authority
- replacing all current skills immediately
- building a full IDE in the first migration tranche
- creating self-coding as a separate product

Compatibility wrappers and staged renames are acceptable when they reduce risk.

---

## Phase 0 — Documentation Authority

### Goal
Establish the new source of truth.

### Tasks
- Add `docs/canonical/navi-coder.md`.
- Add `docs/architecture/navi-coder-product-architecture.md`.
- Add this migration plan.
- Update documentation indexes.
- Mark old Programmer docs as transitional, deprecated, or archived.
- Remove unresolved citation placeholders from current Programmer docs if they remain temporarily.

### Acceptance
- Any agent can identify NAVI Coder as the product authority.
- Any document saying Programmer is the product is marked stale or migrated.

---

## Phase 1 — Product and Naming Audit

### Goal
Find every stale product reference.

### Tasks
Search for:

- `NAVI Programmer`
- `navi.programmer`
- `navi-programmer`
- `Programmer V1`
- `programmer plugin`
- `first-class programming capability plugin`

Classify each reference as:

- legacy package reference
- migration history
- stale product language
- code identifier requiring staged rename
- test fixture requiring compatibility
- user-facing label requiring immediate rename

### Acceptance
- A migration inventory exists.
- User-facing stale naming is identified for correction first.

---

## Phase 2 — Console Product Route

### Goal
Make Coder a visible top-level product surface.

### Tasks
- Add `/coder` route.
- Add Coder entry to main console navigation.
- Add Coder landing/workspace screen.
- Add placeholder panels for project/repo/task/run/diff/validation/review state.
- Ensure the UI says NAVI Coder, not NAVI Programmer.

### Acceptance
- User can click Coder from the console.
- Coder feels like a product surface, not a plugin manager page.

---

## Phase 3 — Backend Coder Domain Skeleton

### Goal
Introduce first-class backend domain objects and API shape.

### Tasks
- Add `internal/coder/` package family or equivalent.
- Define domain structs for Coder project/repository/task/run/step/artifact/diff/validation/review/candidate runtime/promotion decision.
- Add store interfaces or migrations as appropriate.
- Add API route skeleton under `/api/coder/*`.
- Add event naming conventions for Coder run events.

### Acceptance
- Coder has first-class backend state shape.
- Future implementation no longer has to overload plugin scratchpad/evidence as the only product state.

---

## Phase 4 — Reposition Current Programmer Package

### Goal
Preserve implementation while fixing architecture.

### Tasks
- Decide target package path for current assets, preferably `internal/coder/plugins/coder-core/` or `plugins/coder-core/`.
- Move or alias current skills:
  - repo inspect
  - file mutate
  - patch apply
  - run validation
  - git lifecycle
  - remote review
  - task normalize
- Rename manifests and descriptions from Programmer to Coder where user/product-facing.
- Keep compatibility aliases for runtime/tool discovery if necessary.
- Update tests to assert Coder identity while preserving legacy compatibility where intentional.

### Acceptance
- Coder owns the capability pack.
- `navi.programmer` is either gone, aliased, or clearly transitional.

---

## Phase 5 — Coder Control Plane

### Goal
Move from tool-level workflow evidence toward Coder task/run orchestration.

### Tasks
- Implement Coder task creation.
- Implement Coder run creation.
- Bind Coder run to project/repo/workspace.
- Route Coder runs through existing skill/workflow execution substrate.
- Persist run steps and artifacts.
- Attach validation and review output to Coder records.
- Stream Coder events to console.

### Acceptance
- Coder can track a coding task as a product-level run.
- The console can render run progress from Coder state, not just generic chat messages.

---

## Phase 6 — Isolated Coder Worker Boundary

### Goal
Separate risky repository execution from the trusted NAVI control plane.

### Tasks
- Define Coder worker contract.
- Add worker job envelope.
- Add approved workspace/repo mount policy.
- Add command/network/time/memory limits.
- Move validation and mutation execution toward worker boundary where appropriate.
- Return structured evidence to Coder control plane.

### Acceptance
- Risky file/process/git operations can run outside trusted `navid` control path.
- NAVI Core remains authoritative but does not directly become the mutation surface.

---

## Phase 7 — Candidate Runtime for Self-Update

### Goal
Keep self-update as Coder applied to NAVI, with stricter safety.

### Tasks
- Represent NAVI self-update as `CoderTask` targeting NAVI repo.
- Launch candidate runtime in isolated container/context.
- Use separate config/state/ports.
- Run boot/health/basic request/skill surface/governance/clean shutdown checks.
- Persist candidate runtime evidence.
- Require owner/governance promotion decision.

### Acceptance
- Self-update is not a separate self-coding subsystem.
- Candidate runtime evidence is produced by real execution, not only fixtures.

---

## Phase 8 — Linear and Task Management Alignment

### Goal
Update task management to match the new product/domain model.

### Tasks
- Create Linear epic for NAVI Coder migration.
- Mark old Programmer issues as superseded or linked to the migration epic.
- Create agent-delegation tickets for phases 1–7.
- Ensure new tickets use NAVI Coder language.
- Retain references to Programmer only as legacy package/migration context.

### Acceptance
- Linear reflects NAVI Coder as the active architecture.
- No new work is filed under Programmer except explicit migration cleanup.

---

## Phase 9 — Cleanup and Tombstone

### Goal
Remove stale naming and stale documentation.

### Tasks
- Archive or tombstone old Programmer docs.
- Remove stale product language.
- Rename UI labels.
- Rename API labels.
- Rename task labels where possible.
- Leave compatibility notes where code identifiers remain temporarily.

### Acceptance
- Searching for Programmer yields only migration history or compatibility references.
- Product, UI, docs, and Linear consistently say NAVI Coder.

---

## Risk Register

### Risk: breaking working prototype code during rename
Mitigation: use compatibility aliases and staged migration.

### Risk: making Coder too separate from NAVI
Mitigation: keep NAVI Core authoritative for identity, governance, events, world model, and promotion.

### Risk: leaving Coder as only chat tools
Mitigation: add top-level console route and Coder domain objects.

### Risk: recoupling self-coding as special architecture
Mitigation: state repeatedly that self-update is Coder applied to NAVI with stricter candidate runtime gates.

### Risk: stale docs persist
Mitigation: treat Programmer references as stale unless explicitly marked legacy/migration.

---

## Immediate Next Actions

1. Update docs indexes to include NAVI Coder.
2. Create Linear epic and tickets for this migration.
3. Delegate product/naming audit to an agent.
4. Delegate console route scaffold to an agent.
5. Delegate backend Coder domain skeleton to an agent.
6. Delegate package repositioning plan to an agent.
7. Pause new Programmer feature work until Coder migration direction is reflected in tasking.
