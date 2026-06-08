# Next Action: NAVI Coder Migration Re-Triage

## Task

Choose the next NAVI Coder migration task now that OMN-280 has classified stale
`Programmer` references and the OMN-234 V1 baseline remains closed.

## Objective

Pick the highest-impact OMN-279 child that can move Coder forward without
breaking legacy `navi-programmer.*` package, skill, and workflow compatibility.

## Why This Is Next

OMN-280 created `docs/plans/navi-coder-migration-inventory.md`, which identifies
the first user-facing rename targets separately from compatibility identifiers
that need staged alias/move work.

## Scope

- Re-check open NAVI issues in Linear.
- Prefer work that removes stale user-facing product language or establishes
  first-class Coder shape.
- Keep compatibility surfaces explicit; do not blindly rename runtime IDs.
- Keep plugin-local state docs aligned with the chosen migration milestone.

## Acceptance Criteria

- The next run starts from post-OMN-280 Coder migration state.
- The chosen task uses the migration inventory instead of repeating the audit.
- Staged compatibility constraints are called out before any rename/move.

## Status

PENDING
