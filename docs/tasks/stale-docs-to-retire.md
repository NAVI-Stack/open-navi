# Stale Docs To Retire / Reclassify

**Status:** Active
**Last Updated:** 2026-05-29
**Purpose:** A running list of specific stale claims that have been corrected, archived, or still need attention. This is the audit trail for the docs-vs-code reconciliation pass.

Status labels: Corrected · Reclassified · Archive Candidate · Needs Review.

---

## Corrected in the 2026-05-29 pass

| Claim (before) | Where | Now | Status |
|----------------|-------|-----|--------|
| "Browser UI is still only a placeholder" / "`web/index.html` is a minimal placeholder" / "Full frontend does not exist yet" | `docs/tasks/blockers.md`, `docs/tasks/readiness-assessment.md`, `docs/FEATURES.md`, `docs/architecture/README.md`, `README.md`, `CLAUDE.md` | Console is a real React 19 + Vite app (`web-src/navi-console/`) built into `web/`; reframed as feature-complete-but-e2e-untested | Corrected |
| Inference Control System = "Missing" | `docs/design/navi-systems-map.md`, `docs/architecture/navi-systems-map.md` | Reclassified Partial — implemented and runtime-wired (`internal/navi/inference/*`, `internal/navi/runtime_executor.go:746`), undertested seams | Reclassified |
| Skill loading implies a root `skills/` "built-in" scan tier | `docs/concepts/skills.md` | Corrected to workspace + user-global scan tiers + manifest-declared plugin skills (`internal/navi/skill/loader.go:21-34,204`) | Corrected |
| Gateway spec missing chat ops + plugin lifecycle routes | `docs/specs/gateway-api.md` | Added (`internal/gateway/server.go:206-210,324-329`) | Corrected |

---

## Helm-era framing — already handled, keep watching

The repo already fences NAVI off from Helm-era framing:

- `docs/README.md` "Product Boundary" section states Helm framing is legacy/external.
- `docs/FEATURES.md` and `docs/specs/persona-system.md` describe legacy presets (Buddy/Jarvis/Orchestrator/CEO) only as **removed** history.

No `NAVI-(Helm-Navigator).txt` file exists in the repo (the audit's item 1 references a file that is not present here). Remaining `Helm` mentions are correct historical/boundary context. **Status: Corrected (no action needed); do not reintroduce Helm as current NAVI identity.**

Watch item — `docs/concepts/workers.md` describes `ScoutAgent` as "legacy, Helm-derived; wired but not actively developed." This is acceptable historical context (it is labelled legacy), but verify the wiring claim against `internal/scout` if the worker is ever revisited. **Status: Needs Review (low priority).**

---

## Archive candidates (not actioned — flagged only)

These are not retired in this pass; broad archival is out of scope. Listed so a future cleanup can decide:

- Duplicate systems-map files (`docs/design/navi-systems-map.md` vs `docs/architecture/navi-systems-map.md`) — pick one canonical home. **Needs Review.**
- `docs/concepts/ocnversation-compaction.md` — filename typo duplicate of `conversation-compaction.md`. **Archive Candidate.**

[tasks INDEX](INDEX.md) | [docs INDEX](../INDEX.md)
