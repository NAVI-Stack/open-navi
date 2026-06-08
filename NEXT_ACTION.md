# Next Action: OMN-72

## Task: Tool calling fallback for Ollama

**Objective:** Restore agentic behavior when the active Ollama model does not emit usable tool calls, so NAVI can still complete tool-requiring work instead of falling back to plain text.

**Scope:**

- Confirm the current router/executor behavior when a tool-capable request returns no tool calls.
- Add a reliable fallback path for tool-requiring intents when the selected Ollama model fails to call tools.
- Preserve existing behavior for providers that already support tool calling correctly.
- Add focused regression tests around Telegram-triggered tool workflows.

**Current Progress:**

- Telegram startup/session bootstrap reliability is now hardened through gateway readiness checks, retried session creation, recovered-session reuse, and a less restrictive SQLite pool.
- Artifact tracking now supports materialized file/skill outputs, draft/checkpoint/finalize version commits, failed-output preservation, and artifact inspection through the gateway API (`/api/artifacts`).
- The next urgent reliability gap is the missing tool-calling fallback path with Ollama.

**Success Criteria:**

- Tool-requiring Telegram requests no longer stall or devolve into non-agentic plain-text replies on Ollama.
- NAVI either executes the needed tools or degrades with an explicit, useful fallback.
- Regression tests cover the no-tool-call path.

**Status:** IN_PROGRESS
