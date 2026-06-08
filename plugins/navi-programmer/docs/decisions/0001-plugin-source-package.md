# 0001 - Plugin Source Package

**Status:** Accepted
**Date:** 2026-04-28

## Decision

NAVI Programmer is housed as a standalone plugin package under:

```text
projects/navi/plugins/navi-programmer
```

The package is currently checked into the NAVI repository under `plugins/` so
NAVI-first iteration, plugin discovery, skills, workflows, and tests stay in one
working tree until NAVI Store packaging exists.

## Rationale

NAVI Programmer is a capability plugin, not NAVI core. Keeping it outside the
NAVI codebase preserves the architectural boundary between:

- NAVI core runtime and governance
- plugin-owned programming capability

## Consequences

- Plugin-owned docs move out of NAVI core.
- NAVI core keeps only redirect or integration references.
- The plugin gets its own project state, task queue, docs corpus, skills,
  workflows, and tests.
- NAVI core can discover it as a built-in plugin when running from this repo;
  optional global/workspace links are still useful when testing copied or
  packaged variants.
