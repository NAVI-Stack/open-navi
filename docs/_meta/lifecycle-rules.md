**Status:** Active  
**Last Updated:** 2026-03-07  
**Updated By:** scaffold

# Documentation Lifecycle Rules

Lifecycle states and transition rules for the NAVI documentation corpus. All mutable docs must carry the metadata below; canonical docs are governed by [canonical/governance-rules.md](../canonical/governance-rules.md).

---

## States

| State | Meaning | Location |
|-------|---------|----------|
| **Active** | Current, in use. No superseding document. | Any mutable section (architecture, adr, concepts, design, plans, runbooks, specs, tasks, prompts) |
| **Evolving** | In progress or under revision. | Draft in `docs-local/` or in `design/` |
| **Archived** | No longer current; preserved for history. | `_archive/` |
| **Deprecated** | Scheduled for removal; do not reference. | `_deprecated/` |

---

## Transitions

- **Active → Evolving:** Agent or human starts a revision (e.g. copy to `docs-local/` or add to `design/`).
- **Evolving → Active:** Revision is merged or promoted (e.g. `docs-local/` → `docs/` per docs-drift-check skill).
- **Active / Evolving → Archived:** Document is superseded or obsolete; move to `_archive/`; update INDEX and back-links.
- **Active / Evolving → Deprecated:** A replacement exists; move to `_deprecated/`; set removal date in INDEX; add notice at top of document.

**Canonical docs:** No state change without human approval. Proposals go as patches or PRs; humans merge.

---

## Required Metadata (Every Document)

Include at the top of each document (mutable or canonical):

- **Status:** Active | Evolving | Archived | Deprecated
- **Last Updated:** YYYY-MM-DD
- **Updated By:** (optional) agent identifier or "human"

ADRs may use **Date** and **Deciders** in place of Last Updated / Updated By where the ADR template already defines them.

---

## References

- [Agent guidelines](agent-guidelines.md) — format and behavior when creating or updating docs
- [GOVERNANCE.md](../GOVERNANCE.md) — overview of structure and approval rules
- [canonical/governance-rules.md](../canonical/governance-rules.md) — who may change canonical docs
