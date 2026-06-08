# NAVI Coder Migration Inventory

**Status:** Active
**Last Updated:** 2026-06-06
**Linear:** OMN-280

> **Superseded / expanded by:**
> [`docs/reviews/navi-coder-programmer-reference-audit.md`](../reviews/navi-coder-programmer-reference-audit.md)
> — the authoritative OMN-280 audit. It covers all eight required search terms
> (this inventory covered six), is grounded in the canonical docs
> `docs/canonical/navi-coder.md` and `docs/plans/navi-coder-migration.plan.md`,
> and adds the mandated executive summary, classification counts, and follow-up
> tickets. **Note:** the "Source Notes" below state those canonical docs were
> "not present in this checkout"; they have since landed. This inventory is
> retained as the initial precursor pass — the audit reconciles the discrepancy
> and recommends updating the pinning test before this file is edited further.

## Purpose

This inventory classifies stale `Programmer` references for the migration from
the NAVI Programmer prototype identity to the NAVI Coder product/domain
identity.

NAVI Coder is the canonical user-facing capability name. `Programmer` may remain
only where it is explicitly legacy, transitional, compatibility-preserving, or
historical.

## Source Notes

OMN-280 references `docs/canonical/navi-coder.md` and
`docs/plans/navi-coder-migration.plan.md`, but those files are not present in
this checkout. `docs/canonical/INDEX.md` also says canonical docs are immutable
and require explicit human approval, so this task does not create or edit
canonical documents.

Current migration direction is taken from:

- `docs/concepts/navi-coder-capability.md`
- OMN-279 and OMN-280 in Linear
- the existing `plugins/navi-programmer/` implementation and docs corpus

## User-Facing References To Rename First

These are the highest-priority references because they present `Programmer` as
the product or primary capability label rather than as migration history.

| Surface | Current reference | Classification | Migration action |
| --- | --- | --- | --- |
| `plugins/navi-programmer/plugin.yaml` | `name: NAVI Programmer` | user-facing label requiring immediate rename | Rename displayed plugin name to `NAVI Coder`; keep `id: navi.programmer` transitional until runtime compatibility is staged. |
| `plugins/navi-programmer/plugin.yaml` | description says "programming capability plugin" | user-facing label requiring immediate rename | Reword description around NAVI Coder as built-in governed software-work capability. |
| `plugins/navi-programmer/README.md` | title and opening product definition say `NAVI Programmer` | stale product language | Convert to "NAVI Coder core capability package" while explaining the legacy package path. |
| `plugins/navi-programmer/docs/README.md` and `plugins/navi-programmer/docs/INDEX.md` | docs corpus source of truth for `NAVI Programmer` | stale product language | Reframe as historical Programmer docs now serving the Coder migration/core substrate. |
| `plugins/navi-programmer/skills/*/SKILL.yaml` | skill display names such as `NAVI Programmer Repo Inspect` | user-facing label requiring immediate rename | Rename display names to `NAVI Coder ...`; keep skill IDs staged for compatibility. |
| `plugins/navi-programmer/skills/*/README.md` | skill descriptions for `NAVI Programmer` | stale product language | Reword as Coder skills backed by legacy `navi-programmer.*` IDs. |
| `internal/prompts/defaults/ncos/programmer_workflow.md` | tells the model to use `navi-programmer` skills | user-facing behavior language | Reword to say NAVI Coder uses transitional `navi-programmer` skill IDs. |

No `web-src/` console labels matched the OMN-280 search terms during this pass.

## Compatibility References Requiring Staged Rename

These references are expected to remain during the first migration because they
are runtime identifiers, package imports, file paths, or fixture contracts.

| Surface | Current reference | Classification | Migration action |
| --- | --- | --- | --- |
| `plugins/navi-programmer/` directory | package path | legacy package reference | Keep until OMN-283 decides alias/move strategy. |
| `plugins/navi-programmer/plugin.yaml` | `id: navi.programmer` | code identifier requiring staged rename | Keep as transitional ID or add alias before changing. |
| `plugins/navi-programmer/skills/*/SKILL.yaml` | `skill_id: navi-programmer.*` | code identifier requiring staged rename | Keep until runtime skill discovery supports aliases. |
| `plugins/navi-programmer/workflows/*.yaml` | `workflow_id: navi.programmer.*` | code identifier requiring staged rename | Keep until workflow contract aliases are defined. |
| `cmd/navid/plugin_bootstrap.go` | imports plugin handlers from `plugins/navi-programmer` | legacy package reference | Update only after package move/alias exists. |
| `cmd/navi/main.go` | special cases for `navi-programmer.repo-inspect` and `navi-programmer.run-validation` | code identifier requiring staged rename | Keep until CLI compatibility and new Coder IDs are wired. |
| `internal/navi/programmer_workflow_bridge.go` | workflow bridge constants and `navi-programmer.*` guards | code identifier requiring staged rename | Keep as compatibility bridge; future Coder domain should own this path. |
| `plugins/navi-coder/handlers/coder_repo_handler.go` | aliases `navi-programmer` handler package | compatibility bridge | Keep as explicit bridge while Coder surface comes online. |
| `plugins/navi-programmer/tests/` and compiled fixtures | expected `navi-programmer.*` IDs | test fixture compatibility | Preserve until aliases are supported, then assert both old and new IDs deliberately. |

## Historical Or Migration References To Preserve

These references should not be renamed blindly because they document historical
work, old status snapshots, or migration context.

| Surface | Current reference | Classification | Migration action |
| --- | --- | --- | --- |
| `docs/reviews/navi-status-2026-05-*.md` | status snapshots mentioning `NAVI Programmer` | migration history | Preserve; add future Coder notes rather than rewriting history. |
| `plugins/navi-programmer/TASK_QUEUE.md` | completed NP/OMN history | migration history | Preserve completed history; add new Coder migration tasks above it. |
| `plugins/navi-programmer/PROJECT_STATE.md` | V1 closeout history | migration history | Preserve facts but update current goal/name in new sections. |
| `plugins/navi-programmer/docs/decisions/*.md` | accepted decisions about package origin | migration history | Preserve unless a superseding decision is added. |

## Root Docs Forwarding Pointers

Root docs under `docs/concepts/`, `docs/design/`, `docs/specs/`, and
`docs/plans/` currently point readers to plugin-owned NAVI Programmer docs.
These are stale as product framing, but they are also useful migration pointers.

Migration action:

1. Add or point to Coder-first docs.
2. Mark `navi-programmer` forwarding pages as legacy/transitional.
3. Do not keep root docs saying NAVI Programmer is the current product name.

## Search Scope

This inventory searched the following patterns:

- `NAVI Programmer`
- `navi.programmer`
- `navi-programmer`
- `Programmer V1`
- `programmer plugin`
- `first-class programming capability plugin`

Primary searched surfaces:

- `docs/`
- `plugins/`
- `internal/`
- `cmd/`
- `web-src/`

Permission-denied cache directories such as
`plugins/navi-programmer/.pytest_cache/` were excluded from the effective
classification.
