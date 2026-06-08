# Tool-Capable Routing Guard

## Problem

Agentic turns can require a reliable tool-capable model. If routing cannot find
one and execution quietly falls back to the currently active provider anyway,
NAVI can produce the worst possible behavior:

- tools are omitted or ignored
- the model hallucinates instead of acting
- the reply denies capabilities that NAVI actually has

This is especially visible when the active provider is Ollama and the selected
local model does not reliably support tool calling.

## Guardrail

For tool-requiring coding or agentic turns, routing now explicitly signals
`no_tool_capable_model` when no reliable tool-capable profile is available.

The runtime treats that as a fail-closed condition:

- it does not call the LLM for that turn
- it returns a user-facing guidance message explaining that a tool-capable
  cloud model is required for the request
- it records a structured runtime error instead of letting the model invent an
  answer

## Logging

The runtime now emits structured logs before and after each LLM call with:

- provider
- model
- tool count
- task class
- complexity
- tool call count
- content length

That makes it easier to distinguish:

- "routing selected a tool-capable model and it used tools"
- "routing selected a tool-capable model and it still did not use tools"
- "routing had no valid tool-capable model and failed closed"
