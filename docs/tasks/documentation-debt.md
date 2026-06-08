# Documentation Debt

**Status:** Active
**Last Updated:** 2026-05-29
**Purpose:** Track known drift between docs and the codebase so it is visible instead of buried in stale prose. Each item carries a blunt status label.

Status labels: Aligned · Implemented but Underdocumented · Documented but Missing · Partially Implemented · Implemented but Not Wired · Stale / Misleading · Aspirational / Planned · Deprecated / Remove.

---

## Resolved in the 2026-05-29 docs refresh

These were corrected in this pass (see `docs/tasks/stale-docs-to-retire.md` for the full before/after):

- **Browser UI "placeholder" framing** — was Stale / Misleading across `docs/tasks/blockers.md`, `docs/tasks/readiness-assessment.md`, `docs/FEATURES.md`, `docs/architecture/README.md`, `README.md`, `CLAUDE.md`. The Console is a real React 19 + Vite app (`web-src/navi-console/`) that builds into `web/`. Now reframed as Partially Implemented / e2e-Untested and documented in `docs/architecture/navi-console-frontend.md`.
- **Inference Control System (ICS) "Missing"** — was Stale / Misleading in both `docs/design/navi-systems-map.md` and `docs/architecture/navi-systems-map.md`. ICS is implemented and runtime-wired (`internal/navi/inference/*`, dispatched at `internal/navi/runtime_executor.go:746`). Reclassified to Partial (undertested seams).
- **Skill loading tiers** — `docs/concepts/skills.md` previously implied a root `skills/` "built-in" scan tier. Corrected to match `internal/navi/skill/loader.go:21-34,204`: workspace + user-global scan tiers, plus manifest-declared plugin skills only.
- **Gateway route inventory** — `docs/specs/gateway-api.md` was missing chat message operations and plugin lifecycle routes; both added with code paths.

---

## Open documentation debt

### Gateway API spec is still a curated subset — Implemented but Underdocumented

`docs/specs/gateway-api.md` covers the major surfaces but still omits some live routes (e.g. several `/api/experience/*` mutators, `/api/capabilities/graph`, `/api/skill-ui`, `/api/errors*`, `/v2/api/connectors/instances`, CORS `OPTIONS` preflight routes). No documented route is missing from code (the doc is a subset, not contradictory). Source: `internal/gateway/server.go`. Track whether a generated route table is worth adding (see `docs/tasks/testing-eval-debt.md` → route-doc parity).

### Two parallel systems-map files — Stale / Misleading risk

`docs/design/navi-systems-map.md` and `docs/architecture/navi-systems-map.md` are near-duplicates that must be kept in sync by hand (both were edited this pass). Decide whether one should be the canonical map and the other a redirect.

### `docs/FEATURES.md` "Done" labels are coarse — Partially Implemented

Several rows marked "Done" are wired but undertested (e.g. reflection worker, scheduler/backlog, LLM-KB routing). The label does not distinguish "wired" from "wired + covered by tests." Consider a two-axis status (wired / tested).

### Conceptual/canonical docs ahead of code — Aspirational / Planned (by design)

`docs/canonical/*`, `docs/design/*`, and `docs/concepts/*` intentionally describe target architecture. They are correctly fenced by `docs/README.md` trust levels, but individual docs do not all carry an explicit status banner. Lower priority; only add banners where a doc is actively mistaken for implementation truth.

---

## How to use this file

When you touch a doc and find drift you cannot fix in the same change, add a one-line entry here with: the file, a status label, and the code path that proves the gap. Remove entries when resolved.

[tasks INDEX](INDEX.md) | [docs INDEX](../INDEX.md)
