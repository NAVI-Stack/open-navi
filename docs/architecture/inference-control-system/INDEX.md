**Status:** Active  
**Last Updated:** 2026-04-15  
**Updated By:** Codex

# Inference Control System Documentation Corpus

This index organizes the Inference Control System (ICS) documentation into a navigable corpus with clear ownership between foundational spec, normative architecture contracts, and implementation/audit references.

The ICS corpus is intentionally split between:

- **Foundational spec** — cognitive-layer intent and system responsibilities.
- **Normative architecture contracts** — lifecycle, ownership, schema, and compliance requirements.
- **Implementation/audit references** — branch mapping, status notes, and audit/remediation tooling.

---

| Document | Role | Status |
|----------|------|--------|
| [inference-control-system-v1.md](../../specs/inference-control-system-v1.md) | Foundational v1 subsystem spec (purpose, boundaries, module contracts, acceptance criteria). | Draft |
| [ics-architectural-contract.md](../ics-architectural-contract.md) | Top-level normative contract for lifecycle, authority boundaries, invariants, and merge criteria. | Normative |
| [ics-end-to-end-control-flow.md](../ics-end-to-end-control-flow.md) | Normative end-to-end lifecycle and stage responsibilities. | Draft Normative |
| [ics-field-ownership-and-mutation-rules.md](../ics-field-ownership-and-mutation-rules.md) | Normative field ownership and mutation boundaries. | Draft Normative |
| [ics-artifact-schemas.md](../ics-artifact-schemas.md) | Normative artifact-layer definitions and interpretation rules. | Draft Normative |
| [ics-compliance-test-bill.md](../ics-compliance-test-bill.md) | Normative required compliance test suite and CI gate criteria. | Draft Normative |
| [ics-repo-audit-checklist.md](../ics-repo-audit-checklist.md) | Normative repository audit checklist and verdict format. | Draft Normative |
| [ics-remediation-bill-template.md](../ics-remediation-bill-template.md) | Normative remediation work-item template after non-compliant audits. | Draft Normative Template |
| [ics-implementation-mapping.md](../ics-implementation-mapping.md) | Branch-specific mapping of ICS contracts to concrete files and symbols. | Draft Normative Reference |
| [ics-v1-implementation-status.md](../ics-v1-implementation-status.md) | Implementation-truth note that captures current branch state and known spec drift. | Active implementation note |

---

## Current Documentation Position

The current ICS documentation direction is:

1. Keep one foundational spec in `docs/specs/` for subsystem intent.
2. Keep implementation-facing normative ICS contracts in `docs/architecture/`.
3. Keep audit and remediation docs adjacent to those contracts.
4. Keep branch-specific implementation mapping and status notes explicit to avoid drift.

This avoids two failure modes:

- architecture contracts drifting away from implementation-truth references
- implementation notes becoming pseudo-authority without clear links back to normative contracts

---

## Navigation by Intent

- **Understand ICS quickly:** start with [inference-control-system-v1.md](../../specs/inference-control-system-v1.md), then [ics-architectural-contract.md](../ics-architectural-contract.md).
- **Review architecture compliance:** read [ics-end-to-end-control-flow.md](../ics-end-to-end-control-flow.md), [ics-field-ownership-and-mutation-rules.md](../ics-field-ownership-and-mutation-rules.md), and [ics-artifact-schemas.md](../ics-artifact-schemas.md).
- **Audit a branch:** use [ics-repo-audit-checklist.md](../ics-repo-audit-checklist.md) and [ics-compliance-test-bill.md](../ics-compliance-test-bill.md).
- **Turn findings into execution work:** use [ics-remediation-bill-template.md](../ics-remediation-bill-template.md).
- **Map contracts to code now:** use [ics-implementation-mapping.md](../ics-implementation-mapping.md) and [ics-v1-implementation-status.md](../ics-v1-implementation-status.md).

---

[architecture README](../README.md) · [specs INDEX](../../specs/INDEX.md) · [docs INDEX](../../INDEX.md)
