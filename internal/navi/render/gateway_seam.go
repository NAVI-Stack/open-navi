package render

// Governed action/query gateway seam.
//
// For the first proving slice the render payload is READ-ONLY data: NaviDataView
// + generated OpenUI Lang + fallback markdown. No OpenUI-generated interaction is
// wired to any privileged backend endpoint, and tool-usage views declare an empty
// Governance.AllowedActions (read-only).
//
// When OpenUI interactions (Query / Mutation / form action / generated hooks) are
// added, they MUST NOT call privileged endpoints directly. They route through a
// NAVI-owned adapter in this exact order:
//
//	OpenUI toolProvider/action
//	  → NAVI render/action gateway   (this seam)
//	  → schema validation
//	  → Governor / policy checks
//	  → sensitivity classification
//	  → Proposal Queue (when required)
//	  → audit / execution trace
//	  → skill / tool / action execution
//
// This file is intentionally documentation-only for the slice; it marks where the
// governed adapter is introduced so future work has an obvious, single seam.
// See docs/architecture/data-driven-ui-rendering.md and ADR-013.
