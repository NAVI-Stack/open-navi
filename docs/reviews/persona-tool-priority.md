# Persona vs Tool-Priority Note

## Problem

If persona text dominates the system prompt, weaker models can drift into
roleplay, self-limiting disclaimers, or fictional product behavior instead of
using the tools that NAVI actually exposes.

Typical bad outcomes:

- "I'm just an AI, I don't have access..."
- made-up platform features or stores
- narrative roleplay instead of grounded action
- text answers where a tool call should have happened

## Prompt Priority Rule

System prompt assembly now follows this order:

1. Identity and tool-use rules
2. Persona/tone instructions
3. Skill prompt/tool descriptions
4. Runtime sections such as chat behavior, diagnostics, and security

Persona instructions are explicitly constrained to tone only. They must not
override:

- NAVI's identity
- grounded factual behavior
- tool use when tools are available

## Local Model Guard

For Ollama/local models, the prompt now adds a plain-style instruction that
forbids:

- roleplay actions
- stage directions
- gesture asterisks
- whimsical catchphrases

This keeps local models focused on grounded action and tool use.
