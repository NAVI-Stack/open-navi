# Docs Librarian Report

**Date:** 2026-03-14  
**Scope:** docs/ and all subdirectories; INDEX.md and sub-indexes; missing entries, duplicates, orphans, link consistency.

---

## 1. Scan summary

- **Index files (naming):** All section indexes use `INDEX.md` (uppercase). Entry point for architecture is `architecture/README.md` by design.
- **Sections scanned:** canonical, canonical/specs, canonical/protocol, specs, design, adr, concepts, plans, tasks, runbooks, prompts, identity, reviews, agents, architecture, _meta, _archive, _deprecated.
- **Links between indexes:** Doc Map and section links in docs/INDEX.md resolve. Sub-indexes link back to docs/INDEX.md and to sibling sections as appropriate.

---

## 2. (a) Missing index entries

| Location | Issue | Resolution |
|----------|--------|------------|
| **docs/agents/** | No `agents/INDEX.md` existed; main INDEX linked to `agents/INDEX.md` (broken). All agents docs were unlisted at section level. | **Fixed:** Created `docs/agents/INDEX.md` with entries for design.md, state.md, handoff/TEMPLATE.md, logs/TEMPLATE.md, tasks/TEMPLATE.md. |

No other non-index .md files are missing from their section index. Core, canonical, design, adr, concepts, specs, plans, tasks, runbooks, prompts, identity, reviews, and _meta are complete.

---

## 3. (b) Duplicates and orphans

| Type | Item | Resolution |
|------|------|------------|
| **Duplicate** | None. `canonical/connectors.md` (legacy) vs `canonical/specs/connectors.md` (v2) are intentionally different; cross-referenced in canonical/INDEX.md. | No change. |
| **Orphan** | **docs/agents/** — section had no INDEX.md, so agents docs were effectively orphaned from section index; main INDEX link to agents/INDEX.md was dead. | Resolved by creating agents/INDEX.md. |
| **Orphan (optional)** | **docs/feature_comparison.md.resolved**, **docs/reviews/connector_extension_architecture_review.md.resolved** — resolved/artifact copies; not in any index. | Leave as-is or move to _archive if no longer needed. |

---

## 4. (c) Suggested patches (applied or recommended)

### Applied this run

1. **Create `docs/agents/INDEX.md`**  
   - New file. Contents: title "Agents — Index", table linking design.md, state.md, handoff/TEMPLATE.md, logs/TEMPLATE.md, tasks/TEMPLATE.md, back-link to docs/INDEX.md.

### Applied this run (2026-03-14)

2. **Add missing design entry** — [design/connector-failure-and-isolation.md](../design/connector-failure-and-isolation.md) added to design/INDEX.md with one-line description and cross-link to canonical conceptual-design-overview.
3. **Cross-links** — design/INDEX.md footer: added link to canonical/specs/connectors. canonical/specs/INDEX.md: added "Related (evolving design)" links to design/connector-failure-and-isolation and design/connector-taxonomy.

### No changes needed

- **docs/INDEX.md** — Already links to agents/INDEX.md in Doc Map (line 9) and in Plans, Tasks, Operations table (line 66). Link now resolves.
- **Sub-indexes** — All section indexes list their .md files with valid relative links; agents/INDEX.md now exists.

---

## 5. Sub-index coverage (current)

| Section | Index | Docs listed | Status |
|--------|--------|-------------|--------|
| docs | INDEX.md | Core 8, canonical, architecture, adr, design, identity, reviews, concepts, specs, plans, tasks, runbooks, prompts, 2 root reports, agents, _meta 4, _archive, _deprecated | Complete |
| canonical | INDEX.md | 5 docs + protocol, specs | Complete |
| canonical/specs | INDEX.md | connectors.md | Complete |
| canonical/protocol | INDEX.md | (none) | Placeholder |
| adr | INDEX.md | ADR-001–005 | Complete |
| design | INDEX.md | 10 design docs (incl. connector-failure-and-isolation) | Complete |
| concepts | INDEX.md | 15 concept docs | Complete |
| specs | INDEX.md | gateway-api.md, configuration.md | Complete |
| plans | INDEX.md | 1 + 15 connector plans | Complete |
| tasks | INDEX.md | 8 task docs | Complete |
| runbooks | INDEX.md | 3 runbooks | Complete |
| prompts | INDEX.md | 2 prompts | Complete |
| reviews | INDEX.md | 2 reviews | Complete |
| identity | INDEX.md | 4 identity docs | Complete |
| agents | INDEX.md | design, state, 3 TEMPLATEs | Complete (created this run) |
| _archive | INDEX.md | (none) | Placeholder |
| _deprecated | INDEX.md | (none) | Placeholder |

---

[docs INDEX](../INDEX.md) · [lifecycle-rules](lifecycle-rules.md)
