# LLM KB Admin Inspection Surface

This patch adds the first protected debug inspection surface for the LLM knowledge base.

## Routes

- `GET /api/debug/llmkb/profiles`
  - returns a compact list of known profiles with routing-relevant summary fields
- `GET /api/debug/llmkb/profiles/{id}`
  - returns a grouped detail view with:
    - human-readable summary
    - explicit field classifications
    - provenance metadata
    - runtime instances
    - mutation history

## Output shape

- The list view is optimized for quick operator scanning.
- The detail view is grouped by:
  - identity
  - capability classes
  - capability scope
  - technical features
  - routing profile
  - operational state
  - evaluation profile
  - usage stats

Each group exposes classification and provenance explicitly so the operator can tell which data is observed, inferred, or governed without guessing from the field name alone.

## Safety

- The surface is behind the existing protected debug routes.
- It exposes auth/load/health status, but not raw credentials or secret values.
