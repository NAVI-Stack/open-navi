# LLM KB Initial Seed Records

This patch adds the first curated LLM KB seed fixtures under `data/seed/models/`.

## Seeded models

- `anthropic.claude-3-5-sonnet.yaml`
  - Frontier cloud model
  - Expensive, vision-capable, stronger coding/reasoning
  - Autonomy ceiling: `autonomous`
- `anthropic.claude-haiku-3.yaml`
  - Cheap and fast fallback
  - Tuned for chat, summarization, and lightweight work
  - Autonomy ceiling: `chat`
- `meta.llama-3-2-3b.yaml`
  - Local/offline Ollama-hosted model
  - Lower overall capability but no network dependency
  - Autonomy ceiling: `assistive`

## Validation

- The new fixtures are loaded through the real seed loader, not a special-case parser.
- The test suite now loads the actual `data/seed/models` directory twice and confirms:
  - all three profiles load cleanly
  - repeated loads are idempotent
  - the resulting records stay distinguishable by cost tier, autonomy ceiling, agentic class, and provider hosting mode

## Note

The YAML files follow the current loader-supported subset of the seed spec. That keeps them compatible with the repo’s present `internal/llmkb` model while still exercising the three runtime assumptions the ticket asked for.
