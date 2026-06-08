# NAVI Contextual LLM Routing

**Status:** Active  
**Last Updated:** 2026-03-21

This document covers the first shipped slice of contextual LLM routing from OMN-54. It sits on top of NAVI's existing runtime-switchable provider/model selection and adds per-turn task classification plus seeded model profiles.

## What Shipped

- `internal/llm/profiles.go`
  - Adds `TaskClass`, `TaskClassification`, `ModelProfile`, `ModelPreferences`, and `ModelSelector`.
- `internal/navi/runtime_executor.go`
  - Classifies the current turn before the LLM call.
  - Uses the injected `RouteLLM` hook to choose the effective route/model for that turn.
  - Records the selected model/provider in timeout and failure error records.
- Natural-language per-turn model overrides
  - Messages like `use opus for this`, `use local model for this`, or `use anthropic for this` now flow through task classification and bias the selector for that turn only.
- `cmd/navid/main.go`
  - Wires a selector built from the current catalog, effective routes, and default preferences.
- `internal/gateway/llm.go`
  - Adds `GET /api/llm/profiles` for operator inspection of the seeded profile registry.

## Task Classes

The current heuristic classifier assigns turns to one of:

- `chat`
- `lightweight`
- `coding`
- `reasoning`
- `agentic`

Current heuristics are intentionally simple and deterministic:

- `coding` when the message references code, files, builds, tests, bugs, git, connectors, or runtime implementation work.
- `reasoning` when the message asks for analysis, architecture, planning, tradeoffs, or investigation.
- `lightweight` for short status/list/help/version-style prompts.
- `agentic` only when the message itself suggests tool-style work such as searching, reading files, writing files, or executing steps.
- `chat` as the conversational fallback.

Important: the runtime does **not** classify a turn as `agentic` merely because tools are available. This prevents ordinary chat and coding turns from being incorrectly forced onto the orchestrator path.

## Per-Turn Overrides

The current routing slice now supports lightweight natural-language overrides embedded in the user's message.

Examples:

- `use opus for this`
- `use local model for this`
- `use anthropic for this`
- `use ollama for this`

These overrides are:

- per-turn only
- deterministic
- resolved against the currently seeded catalog/profile set
- applied before logical route fallback

That means an override like `use opus for this` can intentionally bypass the normal `coding -> coder` route when a matching profile exists.

## Model Profiles

`SeedProfiles(catalog)` derives a profile for every configured provider/model in `LLMCatalog`.

Each `ModelProfile` includes:

- provider/model identity
- tool and streaming support flags
- relative scores for agentic, coding, chat, reasoning, speed, and cost
- tags such as `local`, `cloud`, `free`, or `brokered`

The current scoring is a seed heuristic, not a learned ranking. Examples:

- Anthropic models are biased toward strong agentic and reasoning work.
- `sonnet` and coder-oriented families score higher for coding tasks.
- `haiku` and `mini` families score higher for speed/cost-sensitive work.
- Ollama models are biased toward local chat/lightweight usage and treated as weaker for tool reliability.

## Route Selection

`ModelSelector` prefers logical routes when they exist:

- `coding` -> `coder`
- `reasoning` -> `reasoning`, then `orchestrator`
- `agentic` -> `orchestrator`
- `chat` / `lightweight` -> `chat`

If a logical route is not present, the selector falls back to the best-scoring concrete provider/model profile.

This keeps the existing router configuration authoritative while still enabling contextual model choice.

## Operator Surface

The gateway now exposes:

- `GET /api/llm/catalog`
- `GET /api/llm/profiles`
- `GET /api/llm/active`
- `PUT /api/llm/active`

`/api/llm/profiles` is the inspectable view of NAVI's current seeded routing knowledge.

## Current Limits

This slice does **not** yet implement:

- persisted user routing preferences such as "always prefer local"
- explicit user-facing "switching to Sonnet for this coding task" messages
- learned score updates from execution outcomes and error logs

Those remain follow-on work on top of the shipped classifier/selector/profile foundation.
