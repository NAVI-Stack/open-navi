# Outstanding Work

**Review Date:** 2026-03-28
**Scope:** Active remaining work for NAVI Core autonomy closure.

This document tracks only live items. Historical resolved items were moved out of the active queue and replaced by `NAVI-AUTO-*` tasks.

## Critical

### O-1 - Skill execution hardening remains incomplete

- `internal`, `rest`, `mcp_tool`, `subprocess_python`, and generic `subprocess` transports all exist in code.
- Remaining work is about reliability, dependency management, smoke coverage, and operator tooling rather than first implementation.
- Some shipped skills still depend on external runtimes or environment setup, especially Python-backed ones.
- Backlog mapping: `NAVI-AUTO-003`, `NAVI-AUTO-004`, `NAVI-AUTO-005`.

## Major

### O-2 - No streamed LLM output

- Providers still buffer full responses before returning.
- Backlog mapping: `NAVI-AUTO-007`.

### O-3 - NATS + SQLite dual-write inconsistency risk

- Publish path is not transactional outbox-based.
- Backlog mapping: `NAVI-AUTO-008`.

### O-4 - Event retention policy not implemented

- SQLite events table can grow unbounded without pruning.
- Backlog mapping: `NAVI-AUTO-009`.

### O-5 - Memory v2/context governance not formalized

- Memory graph and context aging policies are not yet codified in ADRs.
- Backlog mapping: `NAVI-AUTO-010`.

## LLM Provider Plugins

### O-LLM-1 - Built-in provider plugins and bootstrap registration

- `internal/llm` owns provider interfaces, request/response types, routing, fallback chains, and the provider factory registry.
- Concrete providers live in `plugins/llm-ollama`, `plugins/llm-anthropic`, and `plugins/llm-openai`.
- `cmd/navid/plugin_bootstrap.go` is the intentional binary link boundary for built-in provider plugins.
- Repo-owned LLM provider manifests use `kind: llm-provider`.

### O-LLM-2 - Provider hardening follow-ups

- Native token streaming remains partial in runtime consumers even though provider packages expose streaming-oriented code paths.
- First-class onboarding should continue to route provider setup through the registry/bootstrap path rather than direct internal imports.

## Queue of Record

- `docs/tasks/autonomous-agent-readiness-backlog.md`
- `TASK_QUEUE.md`
- `NEXT_ACTION.md`
