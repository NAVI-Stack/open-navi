# LLM KB Storage And Migrations

This patch wires the first SQLite persistence layer for the `internal/llmkb` domain.

## What shipped

- New LLM KB tables were added to `internal/store/db.go` through the existing idempotent `CreateTables()` flow:
  - `llm_providers`
  - `llm_profiles`
  - `llm_profile_capabilities`
  - `llm_profile_features`
  - `llm_profile_operational_state`
  - `llm_profile_evaluation`
  - `llm_profile_usage_stats`
  - `llm_profile_routing`
  - `llm_runtime_instances`
  - `llm_execution_records`
  - `llm_evaluations`
  - `router_decisions`
  - `routing_proposal_items`
- The storage uses JSON text columns for complex array/map fields where direct SQL querying is not needed yet.
- `llm_execution_records` and `router_decisions` are treated as append-only in the repo layer.
- Focused repository support was added in `internal/store/llmkb_repo.go` for:
  - providers
  - profiles
  - runtime instances
  - router decisions
  - execution records

## Design notes

- The profile subtables are stored as 1:1 JSON-backed rows keyed by `llm_id`. This keeps the schema aligned with the spec while avoiding premature column explosion before the seed schema is stable.
- The current repo layer is intentionally narrow and seed/storage oriented. It is enough to unblock seed loading and read/query work without overcommitting to a final query surface too early.

## Next steps

- Add the YAML seed loader on top of these repos.
- Build the read/query API once the seed records exist.
- Expand repository coverage for evaluations and routing proposals if the next tickets need direct writes there.
