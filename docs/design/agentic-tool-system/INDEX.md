**Status:** Evolving  
**Last Updated:** 2026-04-17  
**Updated By:** ChatGPT

# Agentic Tool System Design Corpus

This directory contains the evolving conceptual and design corpus for NAVI's agentic tool system rework.

The design is intentionally split between:

- **Tool discovery/search** — capability awareness, registry/index search, ranking, descriptions, related-tool lookup, load requests, and missing-capability detection.
- **Tool usage/execution** — active tool set, provider exposure, validation, governance, dispatch, result normalization, recovery, and observability.

This split is deliberate. NAVI should support rich capability discovery without exposing the entire registry as directly callable schemas.

---

| Document | Purpose |
|----------|---------|
| [tool-system-corpus-map.md](tool-system-corpus-map.md) | Maps the intended full documentation corpus for the tool system across conceptual, design, and later implementation-spec layers. |
| [tool-system-conceptual-overview.md](tool-system-conceptual-overview.md) | Defines the Tool System as a first-class NAVI subsystem, its entities, boundaries, invariants, and relationship to Skills, Plugins, Connectors, Commands, and Governance. |
| [tool-discovery-and-search.md](tool-discovery-and-search.md) | Defines the Tool Discovery/Search subsystem: searchable registry/index, policy-filtered discovery, ranking, unavailable-tool explanation, and missing-capability detection. |
| [tool-usage-and-execution.md](tool-usage-and-execution.md) | Defines the Tool Usage/Execution subsystem: active tool set, provider exposure, tool-call validation, governance, execution lifecycle, recovery, and observability. |
| [tool-broker-design.md](tool-broker-design.md) | Defines the Tool Broker control plane: discovery, filtering, ranking, loading, active-set construction, provider exposure planning, and out-of-set recovery. |
| [tool-lifecycle-and-state-model.md](tool-lifecycle-and-state-model.md) | Defines explicit tool lifecycle states and transitions from registration through discovery, loading, exposure, execution, completion, suspension, and failure. |
| [tool-governance-and-safety.md](tool-governance-and-safety.md) | Defines layered governance for risk tiers, authority, environment separation, confirmation/proposal behavior, result safety, and fail-closed execution. |
| [active-tool-set-and-provider-exposure.md](active-tool-set-and-provider-exposure.md) | Defines the runtime Active Tool Set boundary and provider-facing exposure plan used to control token overhead and model-visible callable tools. |
| [tool-registry-and-index-design.md](tool-registry-and-index-design.md) | Defines canonical tool identity, Tool Registry authority, Tool Index projection/search behavior, exact lookup, related matching, and collision handling. |
| [acceptance-test-matrix.md](acceptance-test-matrix.md) | Maps ATS acceptance requirements and hard invariants to the current implementation, automated coverage, and explicit remaining gaps. |
| [late-enabled-tool-discovery-case-study.md](late-enabled-tool-discovery-case-study.md) | Captures the stale active-surface / late-enabled connector failure mode and required broker/discovery refresh behavior. |
| [tool-degradation-and-fallback-routing-case-study.md](tool-degradation-and-fallback-routing-case-study.md) | Captures degraded/broken tool-action handling and safe fallback routing through governed lower-level actions. |

---

## Current Design Position

The current design direction is:

1. Maintain a **full registry** of known tools.
2. Maintain a **searchable discovery/index layer** over that registry.
3. Use a **tool broker** to search, rank, filter, load, refresh, and unload tools.
4. Maintain an **Active Tool Set** as the runtime boundary for loaded tools.
5. Expose only a **small provider-call-specific tool surface** to the model.
6. Route all execution through a **single governed execution path**.
7. Treat unknown, unloaded, stale, degraded, or disallowed tool calls as recoverable runtime/model mismatch events rather than user-facing raw errors.

This avoids these failure modes:

- exposing all tools and confusing the model
- giving the agent no way to discover relevant capabilities outside the active callable set
- treating the current active surface as global truth
- hallucinating tools instead of searching the registry/index
- leaking dev/test/internal tools into production
- allowing model confidence to become execution authority
- treating a degraded tool/action as proof that the task is impossible

---

## Implementation Tracking

Working branch:

```text
feat/agentic-tool-system
```

Linear project:

```text
Agentic Tool System
```

Initial implementation slice:

1. Tool entity and validation contract
2. Unified Tool Registry
3. Registry adapters for existing tool sources
4. Tool Index and discovery result contract
5. Discovery policy filtering
6. ToolBroker.Resolve v1 with no-tool chat default
7. Broker ranking and top-K selection
8. Unknown/unloaded tool-call recovery
9. Active Tool Set runtime object
10. Late-enabled tool discovery and surface refresh
11. Tool degradation detection and fallback routing

---

## Hard Invariants

- Registry is not exposure.
- Discovery is not execution.
- Loaded is not exposed.
- Exposed is not executable.
- The model proposes; runtime governance authorizes.
- The broker is mandatory for all model-visible tool exposure.
- Unknown or unavailable tools must not surface raw internal errors to users.
- Dev/test tools must be environment-partitioned, not merely hidden by prompt wording.
- The current active tool surface is not proof that a capability does not exist.
- A selected tool/action failing does not mean the task is impossible if a governed fallback exists.
- Fallback routing must preserve risk, side-effect, authority, and execution-path constraints.

---

## Deferred / Later Design Areas

The next documents should only be added when implementation exposes concrete gaps:

- Tool Execution Runtime Design
- Provider Tool Protocol Design
- Observability and Trace Design
- Tool Acquisition / Install / Skill-Growth Design
- Migration Plan from Existing Tool Paths

Implementation-facing specs should come only after the design corpus and first implementation slice stabilize.

---

[docs/design INDEX](../INDEX.md) · [docs INDEX](../../INDEX.md)
