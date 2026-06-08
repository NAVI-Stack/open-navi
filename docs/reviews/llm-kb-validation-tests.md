# LLM KB Validation And Unit Tests

This patch hardens the Phase 1 LLM KB test surface.

## What shipped

- Seed-loader validation coverage now includes:
  - missing required fields
  - unknown enum values
  - unknown `schema_version`
  - unexpected extra fields via strict YAML decoding
- Migration coverage now checks:
  - fresh schema creation
  - idempotent re-run
  - required LLM KB indexes exist after migration
- Seed-loader behavior coverage now checks:
  - idempotent reloads
  - governed-field preservation on older-or-equal seeds
  - governed-field upgrade on supported version bump
  - provenance retention after reload
  - mutation-history append behavior on subsequent loads
- The real OMN-79 seed fixture directory is loaded as part of the tests, so the test suite validates the actual shipped seed records.

## Why this matters

OMN-78 through OMN-80 added a new KB type/storage/query stack quickly. This pass turns that into something we can safely build on without depending on ad hoc manual checks every time the seed schema or query surface evolves.
