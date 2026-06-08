# LLM KB Foundation Types

This patch adds the first dedicated `internal/llmkb` package for the LLM Knowledge Base domain.

## What it includes

- Typed controlled vocabularies for the initial LLM KB taxonomy.
- Case-insensitive parse helpers and `IsValid()` methods for every enum.
- First-pass entity structs for:
  - `LLMProvider`
  - `LLMProfile`
  - `CapabilityProfile`
  - `TechnicalFeatures`
  - `OperationalState`
  - `EvaluationProfile`
  - `UsageStats`
  - `RoutingProfile`
  - `LLMRuntimeInstance`
  - `RouterDecision`
  - `RejectionRecord`
  - `LLMExecutionRecord`
  - `LLMEvaluation`
  - `RoutingProposalItem`
  - `Provenance`
  - `MutationRecord`
  - `RoutingCondition`
  - `BenchmarkRef`
  - `InternalEvalScore`
  - `CostEstimate`
  - `QuotaState`
- Basic validation methods for the core records so later storage and seed-loader work has a consistent contract.

## Why it lives in `internal/llmkb`

The runtime already has task classification and route-decision types in `internal/llm`. The KB domain needs similarly named concepts, but with a different purpose and shape. Keeping them in `internal/llmkb` avoids collisions and makes the storage/seed pipeline easier to evolve independently.

## Next steps

- Wire these types into SQLite storage and migrations.
- Add YAML seed ingestion that validates against these enums and struct contracts.
- Extend validation coverage once the exact seed schema is finalized.
