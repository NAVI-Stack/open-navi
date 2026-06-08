# LLM Dual-Plane Architecture

> [!NOTE]
> Part of the [NAVI Systems Map](navi-systems-map.md).


NAVI uses a dual-plane architectural design to manage Large Language Model (LLM) interactions. This separation ensures a clean boundary between the "how" (inference) and the "why/where" (policy and routing).

## 1. Responsibilities

The architecture is divided into the **Inference Plane** and the **Control Plane**.

### Inference Plane (`llm.Provider`)
The Inference Plane is the stateless execution layer. Its sole responsibility is processing a clear prompt into a response.

- **Contract**: Stateless inference primitive.
- **Operations**: Token generation (`Chat`), embeddings, streaming.
- **Scope**: Per-call execution.
- **Constraints**: 
    - Does not know about users or sessions.
    - Does not know about routing preferences or history.
    - Does not know about model aliases or fallback chains.

### Control Plane (`LLMService` / `ControlPlane`)
The Control Plane is the stateful orchestration layer. It manages the lifecycle and selection of providers based on high-level policies.

- **Contract**: Stateful control surface and policy engine.
- **Operations**: Provider selection, routing decisions (`Route`), catalog management (`Catalog`), preference enforcement, and health monitoring.
- **Scope**: Runtime lifecycle and cross-call state.
- **Responsibility**:
    - Mapping logical model aliases to physical provider bindings.
    - Enforcing per-user or per-task routing preferences.
    - Orchestrating fallback/retry behavior across multiple providers.

---

## 2. Dependency Rules

To maintain this architectural separation, strict dependency rules are enforced:

1.  **Gatekeeper Rule**: Callers that need to select a model or resolve a routing decision **must** use the `LLMService`.
2.  **Executor Rule**: Callers that already have a resolved provider (e.g., they were handed a `llm.Provider` by the control plane or a registry) should speak directly to the `llm.Provider` interface.
3.  **Isolation Rule**: Business logic should not mix inference details (e.g., raw token counts, streaming buffers) with control logic (e.g., "always use Claude for coding tasks") in the same component.

> [!IMPORTANT]
> The `ControlPlane` is the gatekeeper; the `Inference Plane` is the executor. Direct access to a raw `llm.Provider` from outside the `LLMService` context requires explicit justification.

---

## 3. Rationale: Why Not a Single Plane?

While it is tempting to collapse these into a single "LLM Facade," keeping them separate provides several critical benefits:

- **Fundamental Contract Difference**: `llm.Provider` is a stateless mathematical primitive; `LLMService` is a stateful business policy. Mixing them creates a "God Interface" that is difficult to test and maintain.
- **Inner-Loop Performance**: Callers in tight inner loops (e.g., streaming processors, embedding generators) should not be forced to pay the complexity or latency cost of re-evaluating routing policies on every micro-interaction.
- **Clear Separation of Concerns**: By keeping the boundary visible, it is easy to identify architectural "smells" during code review (e.g., if a high-level agent is manually configuring low-level inference tokens).
