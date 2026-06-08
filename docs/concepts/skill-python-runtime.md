# Python Skill Runtime

**Status:** Active  
**Applies to:** skills using `transport.type: subprocess_python`  
**Source of truth:** `internal/navi/skill/spec.go`, `internal/navi/skill/python_runtime.go`

This document explains the current Python-backed skill path in the codebase.

## Purpose

`subprocess_python` exists for skills that are easiest to ship as Python rather than as native Go handlers. NAVI still owns the decision-making and governance; Python only executes the declared interface.

The current flow is:

1. NAVI selects a skill interface through tool-calling
2. the skill executor resolves the matching interface
3. the Python runtime provisions or reuses the local environment
4. NAVI sends structured input
5. the Python entrypoint returns a structured result envelope

## Required Spec Pieces

A Python-backed skill uses:

- `interfaces[].transport.type: subprocess_python`
- `interfaces[].transport.subprocess_python`
- a top-level `python_runtime` block

Important fields include:

- `entrypoint`
- optional `function`
- `python_version`
- dependency configuration
- timeouts and output caps
- network/write permissions

## Current Implementation Files

| File | Purpose |
|------|---------|
| `internal/navi/skill/spec.go` | schema for `PythonRuntimeSpec` and `SubprocessPythonTransport` |
| `internal/navi/skill/python_runtime.go` | environment provisioning and subprocess execution |
| `internal/navi/skill/executor.go` | transport dispatcher |
| `schema/jsonschema/skill-python-runtime.json` | JSON Schema export |

## Runtime Behavior

The current runtime:

- creates or reuses a virtual environment
- installs dependencies when required
- invokes the Python entrypoint
- enforces timeouts and output limits
- returns structured status output back to the caller

Typical result statuses include:

- `success`
- `error`
- `timeout`
- `truncated`

## Operational Caveat

This transport is implemented, but it is still sensitive to the local Python environment. If Python, `venv`, or dependency installation is unavailable or restricted on the host, the skill will fail at runtime even though the transport itself exists in code.

## Related Docs

- [skills.md](skills.md)
- [../FEATURES.md](../FEATURES.md)
- [../tasks/readiness-assessment.md](../tasks/readiness-assessment.md)
