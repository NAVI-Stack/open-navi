# LLM KB Query API

This patch adds the first typed read/query surface for the LLM knowledge base.

## What shipped

- New typed profile queries on the SQLite LLM KB repo:
  - list all profiles
  - get by ID
  - get by alias
  - list by provider
  - list by governed task preference
  - get routing profile
- New typed runtime instance queries:
  - list runtime instances for a specific `llm_id`
  - list only currently available runtimes
- A deterministic `DescribeProfile(...)` helper that summarizes stored provider, capability, routing, and feature metadata without involving the LLM.

## Scope

- This is the internal read surface only.
- There is no routing score calculation or selection logic in this patch.
- The task-preference query reads directly from the stored governed routing fields, not inferred scores.

## Validation

- The query tests load the real seed fixtures from `data/seed/models/`.
- They then verify alias lookup, provider filtering, task-preference filtering, runtime availability filtering, routing-profile reads, and human-readable descriptions.
