# LLM KB Runtime Reconciliation

## Summary

This pass fixed the pre-existing LLM KB failures in `internal/store` and closed a more important runtime integrity gap: live model telemetry was able to create or reference KB identities that drifted from the curated seed catalog.

The LLM KB now reconciles observed `(provider, model)` pairs against existing provider-scoped profile identities before creating stubs, and routing continues to use live catalog model IDs instead of human-readable KB labels.

## What Changed

- `internal/store` is green again after fixing:
  - sparse provider/profile normalization in the repo layer
  - execution-record target bootstrap for missing provider/profile rows
  - in-memory SQLite connection pooling issues in tests
  - a nested-query cursor issue in `ListProfiles()`
  - an unrelated placeholder-count bug in `execution_outcomes`
- Runtime model reconciliation is now provider-scoped and alias-aware:
  - `ResolveProfileForProviderModel(...)`
  - `EnsureRuntimeProfile(...)`
- Catalog refresh now resolves runtime instances onto existing seeded profile IDs when possible, instead of storing raw runtime model names as foreign keys.
- KB-backed routing now starts from the live LLM catalog and overlays KB metadata onto those live model IDs, so the selector stays runnable even when KB records use governed identifiers or display names.
- Routing proposal generation now suppresses duplicate pending proposals for the same `(llm_id, provider_id, task_class)` tuple across maintenance runs.
- Alias lookups now fail explicitly when multiple profiles share the same shorthand, and the debug gateway surface returns a conflict response instead of choosing an arbitrary provider record.

## Remaining Gaps

- Seed coverage is still intentionally sparse.
  Runtime IDs that do not match curated aliases exactly will still create observed stub profiles. This is safe, but it means richer routing/eval metadata only attaches automatically when the runtime model string is covered by the seed catalog or alias set.

- Execution telemetry still materializes usage and proposal signals from observed runtime IDs, not a richer provider release taxonomy.
  That is safe after reconciliation, but deeper analysis like family-level rollups (`claude-sonnet-*`, dated Anthropic variants, OpenRouter provider/model tuples) still needs a curated normalization layer above the current alias matching.

## Validation

- `go test ./internal/store -count=1`
- `go test ./cmd/navid -count=1`

Both were run with a workspace-local `GOCACHE` on Windows.
