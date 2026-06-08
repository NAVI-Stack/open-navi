# Experience Layer — Contract and Implementation Mapping

**Status:** Active  
**Source:** [Conceptual Design Overview](../canonical/conceptual-design-overview.md) (Architectural Layers)

---

## Layer responsibility

The **Experience Layer** determines how NAVI presents itself: role selection, persona, tone, and interaction style. It **modulates** the Cognitive Layer’s output and applies behavioral policy before delivery. It does **not** own reasoning, decision logic, or World Model writes.


| Responsibility                             | Owned by Experience | Not owned by Experience                      |
| ------------------------------------------ | ------------------- | -------------------------------------------- |
| Role / persona / tone                      | Yes                 | —                                            |
| Applying behavioral policy before delivery | Yes                 | —                                            |
| Interruption presentation (how)            | Yes                 | When to interrupt (Governance)               |
| Proposal presentation (how)                | Yes                 | Proposal creation (Cognitive)                |
| Degradation visibility (how)               | Yes                 | Severity level selection (schema/governance) |
| Reasoning, decomposition, reflection       | —                   | No (Cognitive)                               |
| World Model writes                         | —                   | No (Cognitive only)                          |
| Command execution                          | —                   | No (Capability)                              |


---

## Current implementation mapping

The Experience Layer has a dedicated package and a clear boundary from Cognitive.


| Conceptual piece                                      | Implementation                                                                                                      | Location                                                                               |
| ----------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------- |
| **Persona / role / tone**                             | ExperienceManager with built-in standard/wizard profiles and optional YAML overrides                                | `internal/navi/persona.go`, `config/personas/navi.yaml`, `config/personas/wizard.yaml` |
| **Reply shaping (behavioral policy before delivery)** | Experience facade; Cognitive output passes through before persist/publish                                           | `internal/navi/experience/experience.go`                                               |
| **Loop boundary**                                     | Loop calls `Experience.ShapeReply(raw, experience_mode)` before `handleFinalReply`; no decision logic in Experience | `internal/navi/loop.go`                                                                |
| **Presentation / API**                                | Gateway serves UI and API; session replies are already shaped by Experience when delivered                          | `internal/gateway/server.go`                                                           |


The Cognitive path produces raw LLM output; the loop passes it through `LoopConfig.Experience` (default: `experience.DefaultLayer`) which applies empty-content fallback and refusal-to-chat replacement. Persist and bus publish happen only after shaping. No decision logic lives in the Experience package.

---

## Contract (for future work)

1. **Reads only** — Experience reads Cognitive output (and Capability outcomes) and may transform presentation. It does not call store write APIs or World Model write paths.
2. **Single boundary** — All user-visible presentation (API payloads, UI copy, interruption/proposal/degradation presentation) is the responsibility of the Experience Layer. New presentation behavior (e.g., degradation level in API, interruption formatting) belongs in gateway or a dedicated Experience facade, not in orchestrator or workers.
3. **Experience mode as configuration** — Role/tone remain configured via ExperienceManager, with the standard `navi` mode and onboarding `wizard` mode as the only supported runtime profiles; future per-channel or per-session overrides still count as Experience Layer configuration.

---

## References

- `internal/navi/experience/experience.go` — Layer interface, DefaultLayer (ShapeReply)
- `internal/navi/persona.go` — ExperienceManager, supported experience modes, ExperienceProfile
- `internal/navi/loop.go` — Loop uses experience modes for prompts; passes reply through Experience before delivery
- `internal/gateway/server.go` — HTTP/WS API and static UI delivery

