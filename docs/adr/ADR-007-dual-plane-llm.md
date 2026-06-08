# ADR-007: LLM Dual-Plane Architecture

## Context
The LLM integration in NAVI has evolved from simple provider wrappers to a complex system involving model selection, fallback logic, telemetry, and dynamic configuration. To maintain maintainability and clear boundaries, we need a formal architectural separation between "how we talk to LLMs" and "how we manage LLM sessions and policies."

## Decision
We implement a **Dual-Plane Architecture** for LLM operations:

### 1. Inference Plane (Stateless)
- **Component:** `llm.Provider` interface and its implementations (Anthropic, OpenAI, Ollama).
- **Responsibility:** Standardizing the request/response format, handling authentication with external APIs, and implementing streaming protocols.
- **Invariants:**
    - Must be stateless.
    - Does not know about "users," "projects," or "history."
    - Purely functional: Input (Prompt/Config) -> Output (Stream/Response).

### 2. Control Plane (Stateful)
- **Component:** `LLMService` / `ControlPlane`.
- **Responsibility:** Model discovery (Catalog), selection logic (Router), fallback/retry policies, telemetry collection, and usage tracking.
- **Invariants:**
    - Manages the lifecycle of LLM sessions.
    - Enforces governance rules (e.g., "Use Claude 3.5 Sonnet for coding tasks").
    - Coordinates between multiple Inference Plane providers.

## Rationale
- **Separation of Concerns:** Developers working on a new provider (e.g., Gemini) don't need to touch the selection or telemetry logic.
- **Testability:** The Inference Plane can be mocked easily for Control Plane testing, and providers can be tested in isolation with static inputs.
- **Reliability:** Centralized fallback logic in the Control Plane ensures that provider outages are handled gracefully across the entire system.

## Status
**Proposed: 2026-03-29**  
**Ratified: 2026-04-01**

## Consequences
- All code requiring LLM interaction should go through the **Control Plane** (`LLMService`) rather than instantiating providers directly.
- The `internal/llm` package will be reorganized to reflect this split.
