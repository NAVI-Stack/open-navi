# 0005 - Conservative Task Normalization

**Status:** Accepted
**Date:** 2026-04-28

## Decision

NAVI Programmer task normalization starts as a conservative, heuristic,
plugin-owned `subprocess_python` skill in `skills/task-normalize/`.

The skill preserves raw task content, classifies common request shapes, produces
validation and risk hints, and binds repository scope only from explicit caller
context. It blocks ambiguous or unsafe scopes instead of guessing.

## Rationale

Normalization is the first safety gate in the programming workflow. It should
not silently widen scope or infer a repository when several targets are
reasonable. A simple deterministic normalizer is preferable to a confident but
opaque guess while the workflow runner and evaluation fixtures are still
maturing.

## Consequences

- Mutative requests without repo context return `blocked` with reasons.
- Multiple candidate repo matches block until a hint selects one target.
- Self-update candidates can require explicit candidate context.
- Future LLM-assisted normalization can be layered on top of the same output
  shape, but must preserve raw input and ambiguity reporting.
