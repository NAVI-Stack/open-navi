# Task T6 — Docs & index wiring

> Self-contained prompt for a sub-agent. No prior conversation context is assumed.
> **Blocked by T5** (run last, once the Phase 1 artifacts exist and statuses can be advanced).

## Who/where you are

NAVI codebase at `C:\Users\evirg\codespace\NAVI-Ecosystem\projects\navi`. Read `CLAUDE.md`. This is a
documentation-only task — no code changes.

## Required reading

1. `docs/architecture/language-layer-contract.md` and `docs/plans/language-layer-phase1-contracts.plan.md`
   (the two docs being wired).
2. `docs/architecture/navi-systems-map.md` — the living systems inventory and its maintenance rules (§11).
3. `docs/architecture/README.md` and `docs/plans/INDEX.md` — index files (entries were already added when the
   docs were created; verify they resolve).

## The job

Finalize documentation wiring for the language-layer work:

1. **Verify existing links resolve.** Confirm `docs/architecture/README.md` and `docs/plans/INDEX.md` already
   reference the contract doc and the Phase 1 plan, and that the contract ↔ plan ↔ this-folder cross-links all
   resolve. Fix any broken links.
2. **Link from the Systems Map.** Add the Language-Layer Contract to `docs/architecture/navi-systems-map.md`
   where appropriate (e.g. under Platform Systems or as a cross-cutting boundary), following that doc's
   maintenance rules — keep it inventory-level, not a design dump.
3. **Advance statuses.** As Phase 1 tasks land, update:
   - `docs/plans/language-layer-phase1-contracts.plan.md` status `Planned → In Progress` (or `Active`),
   - the contract doc if any "what already exists" statement changed (e.g. governed reads now exist),
   reflecting the actual merged state of T1–T5. Only mark something done if it is actually merged.
4. **Folder index.** Ensure `docs/plans/language-layer/README.md` reflects the final set of prompts and any
   follow-on (e.g. a pointer to the Phase 2 eval-scorer experiment once it is specced).

## Acceptance criteria

1. All cross-links between the contract, the Phase 1 plan, the prompt folder, and the indexes resolve.
2. The Systems Map references the contract at the right altitude.
3. Doc statuses match reality (no doc claims a phase is done that is not merged).

## Out of scope / do not touch

- No code, no codegen, no runtime changes.
- Do not invent new architecture; only wire and status-update existing docs.

## Definition of done / report back

Report: which links you verified/fixed; where you placed the Systems Map reference; and the status changes you
made (with the merged state they reflect).
