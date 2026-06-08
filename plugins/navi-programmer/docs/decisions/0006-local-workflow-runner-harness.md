# 0006 - Local Workflow Runner Harness

**Status:** Accepted
**Date:** 2026-04-29

## Context

NAVI Programmer has executable V1 skill families and a bounded mutation workflow
contract, but NAVI core does not yet provide a full workflow runner for this
plugin.

## Decision

Add a local, stdlib-only bounded mutation runner harness under `workflows/`.
The runner consumes a compiled JSON representation of the bounded mutation
workflow and evaluates state transitions against skill availability, required
evidence, governance rules, and validation gates.

The runner will not directly perform multi-skill execution yet. It will act as
the executable contract that NAVI core can call after skills produce evidence.
It may also normalize skill results into a local evidence ledger and synthesize
the final programming result envelope.

## Consequences

- Bounded mutation now has a machine-checkable transition surface.
- Validation evidence cannot be skipped silently for mutation tasks.
- Missing evidence, scope, or declared skills produce blocked or failed output.
- The compiled JSON must stay aligned with `bounded-mutation.yaml` until NAVI
  core owns YAML loading or contract compilation.
- Full autonomous coding still requires an orchestrator that invokes skills in
  sequence and feeds their evidence back into the runner.
