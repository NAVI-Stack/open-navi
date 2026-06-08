# internal/llm

This package contains the core Large Language Model (LLM) primitives and control logic for NAVI.

## Architecture

NAVI uses a dual-plane design for LLM interactions.

### 1. Inference Plane (`llm.Provider`)
The stateless interface for model interaction. 
- Defines how to send messages (`Chat`) and process responses.
- Implementations include `Anthropic`, `OpenAI`, `Ollama`, but also `FallbackChain` and `DynamicProvider`.

### 2. Control Plane (`ControlPlane` / `LLMService`)
The stateful, policy-driven interface for model selection and routing.
- High-level orchestration, state persistence, and preference resolution.
- Enforces user preferences and matches tasks to models.

**For a full architectural deep-dive and rationale, see [LLM Dual-Plane Architecture](../../docs/architecture/llm-dual-plane.md).**

## Package Structure

- `types.go`: Core data structures (`Message`, `ToolCall`, `Response`).
- `catalog.go`: Metadata registry of available models and providers.
- `selection.go`: Memory and storage for chosen active models.
- `router.go`: High-level routing logic (logic determining which model fits which task).
- `controlplane.go`: The unified entry point for all control-level operations.
- `factory.go`: Glue for constructing providers from configuration.

## Dependency Rule

**Always prioritize `LLMService` over raw `Provider` access.** Callers should only touch a `Provider` once they've been granted one by a `ControlPlane` routing decision or explicit selection.
