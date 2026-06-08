**Status:** Active  
**Last Updated:** 2026-03-07  
**Updated By:** scaffold

# Agent Guidelines for Documentation

Standards for AI agents creating or modifying documentation so that outputs remain consistent and compatible across models and platforms.

---

## Header Block (Required)

At the top of every document, include:

```text
**Status:** Active | Evolving | Archived | Deprecated
**Last Updated:** YYYY-MM-DD
**Updated By:** (optional) agent identifier or "human"
```

ADRs may keep their existing format (Status, Date, Deciders). Set **Status** and **Last Updated** on create and on every update.

---

## Naming

- **Filenames:** Lowercase, hyphenated (`orchestration-loop.md`, `api-auth.md`).
- **ADRs:** `ADR-NNN-short-slug.md` (e.g. `ADR-001-go-orchestration-python-ai.md`).
- **Index files:** `INDEX.md` in each section for discoverability.

---

## Linking

- Prefer relative links within `docs/` (e.g. `[concepts](concepts/)`, `[lifecycle-rules](_meta/lifecycle-rules.md)`).
- Point to INDEX files for section discovery.
- Fix broken links when editing; run docs-index-refresh after structural changes.

---

## Canonical Section

- **Never edit `docs/canonical/` directly.** Changes require explicit human approval.
- Propose changes in `docs-local/` or via PR; humans review and merge into `canonical/`.

---

## Lifecycle

- Set **Status** and **Last Updated** on create and update.
- Move documents to `_archive/` when superseded or obsolete; to `_deprecated/` when a replacement exists and removal is scheduled.
- Follow [lifecycle-rules.md](lifecycle-rules.md) for transitions.

---

## Drift Prevention

- When changing architecture, APIs, schemas, or runbooks, update the corresponding doc in the same work (or produce a Docs Delta Plan per docs-drift-check skill).
- Draft new or revised docs in `docs-local/` when appropriate; promote to `docs/` after verification.

---

## Documentation Roles

| Role | Responsibility |
|------|----------------|
| **Generator** | Create specs, runbooks, ADRs, concepts using these guidelines; draft in `docs-local/` or the appropriate mutable section. |
| **Indexer** | Keep INDEX.md and sub-indexes current; one-line descriptions; fix dead links (docs-index-refresh). |
| **Guardian** | Keep docs aligned with code; run docs-drift-check on behavior/contract changes; apply Docs Delta Plan. |
| **Librarian** | Naming conventions; cross-links; remove duplicates; report orphans. |
| **Archivist** | Move obsolete docs to `_archive/`, deprecated to `_deprecated/`; update indexes and back-links. |

---

## References

- [lifecycle-rules.md](lifecycle-rules.md)
- [GOVERNANCE.md](../GOVERNANCE.md)
- [canonical/governance-rules.md](../canonical/governance-rules.md)
