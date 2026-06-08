**Status:** Active  
**Last Updated:** 2026-03-07  
**Updated By:** scaffold

# Governance Rules — Canonical Documentation

Who may change canonical docs and how. All content under `docs/canonical/` is immutable from the perspective of automated agents.

---

## Update Rule

- **Edits and new documents in `canonical/` require explicit human approval.**
- Agents may propose changes (e.g. in `docs-local/` or a pull request) but must **not** commit directly to `canonical/` without a human merge.

---

## Approval Workflow

1. Agent or human drafts a change (new file or patch) in `docs-local/` or a feature branch.
2. Human reviews the proposal (content and consistency with [principles.md](principles.md)).
3. Human merges into `canonical/` (e.g. via PR merge or direct commit by human).
4. No automated pipeline or agent should push to `canonical/` on its own.

---

## Who May Change Canonical

- **Authors:** Anyone with write access may propose.
- **Approvers:** A human with merge authority must approve and merge. The project may define specific roles (e.g. maintainers) in CONTRIBUTING or team docs.

---

## What Belongs in Canonical

- Core architecture principles (see [principles.md](principles.md)).
- Governance and documentation rules (this file).
- Protocol definitions (NATS subjects, auth contracts, API contracts that define “what the system is”).
- Foundational system specifications that are frozen until superseded by a new canonical doc.

Implementation-level specs, ADRs, and runbooks stay in mutable sections.

---

[canonical INDEX](INDEX.md) · [GOVERNANCE](../GOVERNANCE.md) · [lifecycle-rules](../_meta/lifecycle-rules.md)
