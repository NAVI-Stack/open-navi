# LLM KB Seed Loader

This patch lands the first YAML-backed seed ingestion path for the LLM knowledge base.

## What shipped

- Added `LoadLLMSeeds(...)` on the SQLite LLM KB repo to load all `.yaml` seed files from a directory in deterministic order.
- Added strict YAML decoding and field-level validation errors that include:
  - file name
  - field path
  - invalid value
- Added curated-seed provenance support in `internal/llmkb`:
  - `provenance.source = curated_seed`
  - `provenance.confidence = 0.9`
  - `provenance.source_detail = <seed file path>`
- Added persisted `schema_version` on `llm_profiles` so governed seed merges can compare old vs. new curated records.

## Merge behavior

- Re-running the same seed set is idempotent because providers and profiles are upserted, not duplicated.
- Seeded curated fields update the profile identity/capability/feature layer.
- Live or runtime-owned state is preserved on reload:
  - `OperationalState`
  - `UsageStats`
  - `EvaluationProfile`
- Governed fields are version-gated:
  - if the incoming seed `schema_version` is newer, the governed routing fields are replaced
  - otherwise existing governed fields are kept and the loader logs a warning

## Current scope

The loader intentionally maps the subset of the seed spec that fits the current `internal/llmkb` model and storage layout. It is enough to unblock the first curated seed records and keep merge semantics honest without pretending the full future spec is already materialized.

## Next steps

- Add the initial curated model seed files under `data/seed/models/`.
- Expand the seed schema mapping if later tickets add more identity/routing fields to `internal/llmkb`.
- Wire a startup/admin command path that calls the loader during bootstrap or maintenance flows.
