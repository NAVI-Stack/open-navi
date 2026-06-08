# NAVI Systems Map & Status Matrix

**Status:** Evolving
**Last Updated:** 2026-05-29
**Owner:** Architecture / Core Runtime
**Purpose:** Living architecture inventory for NAVI’s major systems, their current maturity, their architectural role, and their relative priority.

---

## Why this document exists

NAVI now has enough moving parts that new design work can easily become reactive unless the system map is kept explicit.

This document exists to answer five questions:

1. What systems does NAVI actually need?
2. Which systems are already real?
3. Which systems are only partial?
4. Which systems are still missing or only planned?
5. What should be prioritized next?

This is a living design document. It is not the canonical conceptual architecture and it is not the current implementation source of truth. It is the bridge between those two.

Use this document to:

* orient architecture work
* identify system ownership gaps
* decide where new work belongs
* prevent free-floating logic from accumulating in runtime code
* map Linear work to architectural systems rather than isolated tickets

---

## How to read this document

### Status legend

* **Present** — A real system exists and is materially operational.
* **Partial** — Important foundation exists, but the system is incomplete, fragmented, or uneven.
* **Planned** — The system is recognized and should exist, but implementation is still limited or mostly future-facing.
* **Missing** — The system should exist architecturally, but does not yet exist as a formal owned system.

### Priority legend

* **Now** — Load-bearing in the current phase.
* **Next** — Important after current architectural blockers.
* **Later** — Important, but not on the immediate critical path.

### Interpretation rules

This document is intentionally system-oriented, not feature-oriented.

A system may be marked **Partial** even if many related features exist, when ownership is still unclear or implementation is spread across multiple places.

A system may be marked **Missing** even if some of its behaviors exist implicitly, when there is no formal owner or clear system boundary.

---

## Top-level system groups

NAVI is organized into five top-level system groups:

1. **Control Systems** — decide how NAVI behaves
2. **Cognitive Systems** — maintain continuity, structured understanding, and internal state
3. **Execution Systems** — let NAVI do work in the world
4. **Experience Systems** — govern how NAVI presents itself and interacts across surfaces
5. **Platform Systems** — cross-cutting technical foundations everything else depends on

---

# 1. Control Systems

| System                                | Role                                                            | Status  | Why this status                                                                                                                               | Main gap                                                                                        | Priority |
| ------------------------------------- | --------------------------------------------------------------- | ------- | --------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------- | -------- |
| Context Orchestration System (NCOS)   | Canonical turn/run context assembly                             | Partial | Context handling exists, but logic is still duplicated and partly legacy-bound                                                                | Needs single canonical orchestration path and elimination of duplicated prompt/runtime assembly | Now      |
| Inference Control System (ICS)        | Owns model execution behavior                                   | Partial | A formal owner now exists and is **wired into the runtime**: `internal/navi/inference/*` (controller, focus arbiter, mode router, candidate evaluator, plan graph, recovery manager, model/tool/proposal authorization) is invoked every run cycle at `internal/navi/runtime_executor.go:746` with checkpoint/proposal integration. Implemented but undertested — `outcome_supervisor.go`, `controller_authority.go`, and the tool/proposal authorization seams lack unit tests | Needs hardening and test coverage on the untested seams, not a first implementation pass         | Now      |
| Capability Orchestration System       | Decides what capabilities are exposed, composed, or synthesized | Partial | Tool/skill infrastructure is real, but capability strategy is not yet elevated into a single decision system                                  | Needs exposure policy, composition policy, and gap-detection ownership                          | Next     |
| Governance / Approval System          | Constrains actions and authorization                            | Present | Governance is already a core architectural and runtime boundary                                                                               | Needs ongoing hardening, not rescue                                                             | Now      |
| Recovery / Repair System              | Handles degraded execution, resumability, and repair            | Partial | Failure behavior exists in multiple forms, but not as a formal system owner                                                                   | Needs typed recovery flows, degraded-mode rules, and resumable execution policy                 | Next     |
| Observability / Trace / Replay System | Makes reasoning and execution inspectable                       | Partial | Events/logging exist, but traceability is not yet a complete system                                                                           | Needs canonical traces, replay, and operator-facing inspection                                  | Next     |
| Competence / Evaluation System        | Judges quality and drives improvement                           | Missing | NAVI can perform work, but does not yet systematically evaluate whether it did well                                                           | Needs eval loops for tools, skills, workflows, and model choices                                | Next     |

---

# 2. Cognitive Systems

| System                                         | Role                                                        | Status  | Why this status                                                                                                   | Main gap                                                                               | Priority |
| ---------------------------------------------- | ----------------------------------------------------------- | ------- | ----------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------- | -------- |
| Memory / Knowledge System                      | Durable user/system knowledge                               | Partial | NAVI is no longer stateless, but memory behavior is still not fully mature or unified                             | Needs stronger promotion, retrieval, lifecycle, and scope semantics                    | Next     |
| Reflection / Consolidation System              | Turns history into structured learning                      | Partial | Reflection exists conceptually and in implementation direction, but still needs more maturity and tighter outputs | Needs stronger promotion rules and feedback integration                                | Next     |
| World Model System                             | Structured internal reality model                           | Present | This is already a core architectural pillar and underpins multiple subsystems                                     | Needs continued implementation depth, not reframing                                    | Now      |
| Identity / Contact Graph System                | Models owner, contacts, relationships, and speaker identity | Missing | Recognized conceptually, but not formalized as a system                                                           | Needs entity model, relationship graph, trust boundaries, and speaker resolution       | Next     |
| Project Cognition System                       | Persistent project-scoped understanding                     | Planned | Clearly recognized as needed, but not yet complete as a system                                                    | Needs project entities, scoped cognition, project memory, and runtime profile          | Now      |
| Workspace Scope System                         | Governs filesystem/repo/environment boundaries              | Partial | Strong foundation exists, but enforcement is not complete end-to-end                                              | Needs full enforcement wiring across execution surfaces                                | Now      |
| LLM Knowledge Base / Model Intelligence System | Tracks provider/model capabilities and performance          | Planned | Recognized as important, but still mostly future-facing                                                           | Needs provider/model entities, telemetry, capability mapping, and routing intelligence | Now      |
| User Model Assembly                            | Derived runtime view of the owner                           | Partial | Conceptually defined, but dependent on still-maturing underlying cognitive systems                                | Needs stronger source-system maturity and runtime assembly path                        | Next     |

---

# 3. Execution Systems

| System                                          | Role                                                       | Status  | Why this status                                                                            | Main gap                                                                      | Priority |
| ----------------------------------------------- | ---------------------------------------------------------- | ------- | ------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------- | -------- |
| Tool / Skill Runtime System                     | Executes capabilities through governed interfaces          | Present | This is one of the most real systems in NAVI today                                         | Needs continued hardening and broader coverage                                | Now      |
| Worker / Orchestrator Runtime System            | Coordinates multi-role and decomposed execution            | Partial | Worker/orchestrator direction exists, but is still maturing                                | Needs lifecycle control, decomposition contracts, and better result synthesis | Next     |
| Artifact System                                 | First-class outputs and work products                      | Present | Already functions as a real system rather than a loose feature                             | Needs refinement and deeper operator/UI integration                           | Now      |
| Web / Environment Interaction System            | Search, fetch, parse, browse, and monitor external sources | Partial | Some capability exists, but not yet as a broad formal interaction system                   | Needs richer browsing, extraction, provenance, and monitoring semantics       | Next     |
| Task / Scheduling / Follow-Through System       | Ongoing commitments over time                              | Partial | Task/scheduling ideas exist, but long-horizon follow-through is not yet fully systematized | Needs waiting states, blockers, continuity, and follow-up policy              | Next     |
| Connector Runtime System                        | Surface-specific transport and interaction execution       | Partial | Connectors are real, but reliability and maturity vary materially                          | Needs reliability hardening and more consistent surface behavior              | Now      |
| Plugin System                                   | Installable packaged capability bundles                    | Partial | Architecturally defined, but not yet as mature as the skill runtime                        | Needs lifecycle, trust model, and tighter packaging/runtime cohesion          | Next     |
| Capability Acquisition / Skill Synthesis System | Learn, import, and normalize new capabilities              | Partial | Important to NAVI’s identity, but still only partly realized                               | Needs discovery, validation, synthesis, normalization, and testing flow       | Next     |

---

# 4. Experience Systems

| System                                  | Role                                                      | Status  | Why this status                                                                                       | Main gap                                                                                              | Priority |
| --------------------------------------- | --------------------------------------------------------- | ------- | ----------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------- | -------- |
| Persona / Experience System             | Controls presentation and operating mode behavior         | Partial | Real and important, but still stabilizing                                                             | Needs cleaner separation from reasoning/runtime control                                               | Now      |
| Interface / Presentation Control System | Governs interaction contract by surface and mode          | Missing | Some of this behavior exists, but is currently smeared across prompts, connectors, and frontend logic | Needs explicit ownership for chat mode, operator mode, onboarding mode, and surface-specific behavior | Next     |
| PET / Operator Surface System           | Owner-facing control surface                              | Partial | Vision is clear, but implementation is behind the backend/runtime                                     | Needs stronger activity, proposal, artifact, project, and observability surfaces                      | Next     |
| Multi-Surface Experience Layer          | Cross-surface consistency across Telegram, PET, CLI, etc. | Partial | Surfaces exist, but the cross-surface experience contract is still weak                               | Needs unified experience policy and connector-aware presentation rules                                | Next     |

---

# 5. Platform Systems

| System                                   | Role                                                   | Status  | Why this status                                                              | Main gap                                                                            | Priority |
| ---------------------------------------- | ------------------------------------------------------ | ------- | ---------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- | -------- |
| Provider Abstraction System              | Multi-provider model backend layer                     | Present | Real foundation exists and was an important strategic move                   | Needs deeper capability mapping and provider-aware controls                         | Now      |
| Configuration / Policy Management System | Global and scoped configuration control                | Partial | Config exists, but is still distributed and not always cleanly owned         | Needs consolidation across owner, project, workspace, provider, and autonomy scopes | Next     |
| Security / Secrets / Auth System         | Access control and secret safety                       | Partial | Real and necessary, but still always ongoing and not fully matured           | Needs hardening, least-privilege alignment, and clearer auth boundaries             | Now      |
| Provenance / Audit Substrate             | Durable cause/effect history for reasoning and actions | Partial | Strong conceptual treatment exists, but implementation depth is still uneven | Needs deeper mutation, proposal, and execution audit coverage                       | Next     |

---

## Strong systems today

The following systems should be treated as architecturally real today:

* Governance / Approval System
* World Model System
* Tool / Skill Runtime System
* Artifact System
* Provider Abstraction foundation
* Core runtime shell

These are not speculative.

---

## Major architectural gaps

The most important still-missing or under-recognized systems are:

1. **Capability Orchestration System**
2. **Competence / Evaluation System**
3. **Identity / Contact Graph System**
4. **Recovery / Repair System**
5. **Interface / Presentation Control System**
6. **Long-horizon work / follow-through maturity**

> Note: the **Inference Control System** was previously listed here as missing. It is now Partial — implemented and runtime-wired (`internal/navi/inference/*`, dispatched from `internal/navi/runtime_executor.go:746`) but undertested on several seams. It remains on the critical path for hardening, not for first implementation.

These are the systems most likely to keep turning into scattered runtime logic if they are not explicitly owned.

---

## Near-term critical path

### Immediate

1. Finish NCOS Phase 1
2. Harden the Inference Control System (add coverage on the untested authorization/outcome seams; it is already wired, not unformed)
3. Complete Workspace Scope enforcement wiring
4. Continue Project Cognition System foundation
5. Start the LLM Knowledge Base / Model Intelligence foundation in earnest

### After that

6. Formalize the Capability Orchestration System
7. Formalize Observability / Trace / Replay
8. Define the Competence / Evaluation System

### Strategic expansion

9. Identity / Contact Graph System
10. Long-horizon follow-through
11. Recovery / Repair formalization
12. Full Web / Environment Interaction System
13. Interface / Presentation Control System

---

## Practical use in planning

This document should be used as the architecture index behind planning work.

### When creating work

Each major ticket, spec, or RFC should answer:

* Which system does this belong to?
* Is it creating a new system, deepening an existing one, or hardening an existing one?
* Does it reduce ownership ambiguity, or add to it?

### When reviewing work

Ask:

* Is this work landing in the right system?
* Is a missing system being papered over with local logic?
* Is a Partial system being treated as if it were Present?

### When updating this document

Update this matrix when:

* a system crosses from Missing → Planned
* a system crosses from Planned → Partial
* a system crosses from Partial → Present
* architectural ownership changes materially
* the next critical path changes

---

## Doc lifecycle

This is a living design document.

It should remain editable while NAVI’s architecture is still consolidating.

Once specific sections stabilize, they should be split or promoted into:

* `docs/architecture/` when they become implementation-facing and code-backed
* `docs/canonical/` only when they become slow-changing, review-gated architectural truth
* `docs/tasks/` when they become concrete backlog, readiness, or risk tracking

This document should not try to replace those layers. Its job is to maintain the current architecture map and maturity picture.

---

## Changelog

### 2026-05-29

* Reclassified the **Inference Control System** from Missing to **Partial**: it is implemented and runtime-wired (`internal/navi/inference/*`, dispatched from `internal/navi/runtime_executor.go:746`, with checkpoint/proposal integration) but undertested on the authorization/outcome seams. Updated the major-gaps list and near-term critical path accordingly.

### 2026-04-04

* Introduced the first formal systems map and maturity matrix for NAVI.
* Identified Control, Cognitive, Execution, Experience, and Platform as the five top-level system groups.
* Elevated Inference Control System, Competence / Evaluation System, Identity / Contact Graph System, and Interface / Presentation Control System as explicit missing systems.
* Defined the current architecture critical path around NCOS, inference control, workspace scope, project cognition, and model intelligence.
