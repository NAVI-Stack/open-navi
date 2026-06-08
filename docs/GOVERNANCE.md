> [!NOTE]
> Part of the [NAVI Systems Map](architecture/navi-systems-map.md).

**Status:** Active  
**Last Updated:** 2026-03-07  
**Updated By:** scaffold

# Documentation Governance

Overview of the documentation structure, lifecycle, canonical vs mutable sections, and approval rules. This corpus is the single authoritative knowledge base for AI agents and human supervisors.

---

## Structure

- **Master index:** [INDEX.md](INDEX.md) — canonical entry point for all docs.
- **Canonical (immutable):** [canonical/](canonical/INDEX.md) — principles, governance rules, protocol definitions, foundational specs. Changes require **explicit human approval**; agents propose only (e.g. in `docs-local/` or PR).
- **Mutable:** architecture, adr, concepts, design, plans, runbooks, specs, tasks, prompts — agents may create and update per [\_meta/agent-guidelines.md](_meta/agent-guidelines.md).
- **Lifecycle:** _archive/ (read-only history), _deprecated/ (scheduled for removal). See [_meta/lifecycle-rules.md](_meta/lifecycle-rules.md).
- **Meta:** [_meta/](_meta/lifecycle-rules.md) — lifecycle rules, agent guidelines, optional audit log.

---

## Lifecycle

Documents have states: **Active**, **Evolving**, **Archived**, **Deprecated**. Transitions and required metadata (Status, Last Updated, Updated By) are in [_meta/lifecycle-rules.md](_meta/lifecycle-rules.md).

---

## Canonical vs Mutable

| | Canonical | Mutable |
|---|-----------|---------|
| **Location** | `docs/canonical/` | architecture, adr, concepts, design, plans, runbooks, specs, tasks, prompts |
| **Who updates** | Humans (agents propose) | Agents + human review |
| **Rule** | No direct agent commits; human approval required | Update alongside code; follow agent-guidelines |

---

## Approval Rule for Canonical

Edits and new docs under `canonical/` require explicit human approval (e.g. PR review and merge by a human). Agents must not commit directly to `canonical/`. See [canonical/governance-rules.md](canonical/governance-rules.md).

---

## Drift Prevention

- Agents that change architecture, APIs, schemas, or runbooks must update the corresponding doc in the same work (or produce a Docs Delta Plan per docs-drift-check).
- Pre-merge: run docs-drift-check when behavior/contracts change; promote from `docs-local/` to `docs/` as needed.
- Periodic: run docs-index-refresh to keep INDEX and sub-indexes current.

---

## References

- [INDEX.md](INDEX.md) — Master index
- [_meta/lifecycle-rules.md](_meta/lifecycle-rules.md) — Lifecycle states and transitions
- [_meta/agent-guidelines.md](_meta/agent-guidelines.md) — How agents create and update docs
- [canonical/governance-rules.md](canonical/governance-rules.md) — Who may change canonical docs
