---
id: navi-systems-map
title: NAVI Systems Map
doc_type: architecture
lifecycle: living
canonical: true
immutable: false
last_updated: 2026-06-03 00:00:00
last_updated_by: @evirgil
source_of_truth_for:
related_docs:
---

# NAVI Systems Map

Status: Active
Type: Living architecture inventory
Last Updated: 2026-06-03
Owner: Architecture
Purpose: Maintain the current top-level systems map for NAVI, including system boundaries, maturity state, and near-term architectural priorities.

## Purpose

This document is NAVI’s living systems inventory.

It exists to answer five questions clearly:

1. What major systems does NAVI require architecturally?
2. Which systems already exist in meaningful form?
3. Which systems are partial, implied, or only planned?
4. Which missing systems are load-bearing right now?
5. Where should new work land without smearing responsibilities across the runtime?

This document is intentionally operational. It is not the canonical source of truth for every subsystem design detail.

---

## Relationship to Canonical Docs

This document is a companion to the canonical architecture docs. It does **not** replace them.

### Canonical docs

- **NAVI — Conceptual Design Overview**
  Owns the stable architectural model: layers, entities, governance, commands, connectors, plugins, autonomy, failure handling, and process structure.

- **NAVI AI — Skills (Canonical Definition)**
  Owns the canonical definition of Skills as governed capability units in the Capability Layer.

### This doc owns

- top-level systems inventory
- current maturity status
- architectural gap visibility
- prioritization guidance
- load-bearing near-term systems map

### This doc does not own

- detailed subsystem contracts
- implementation recipes
- field-level schemas
- runtime-specific design details
- ticket-level planning

---

## How to Use This Document

Use this document when:

- identifying missing architectural systems
- deciding where a new subsystem belongs
- checking whether a feature request is actually a system gap
- deciding whether a bug is implementation debt or architecture debt
- reviewing whether current work aligns with NAVI’s target shape

Do **not** use this document as a replacement for detailed subsystem specs.

---

## Status Legend

- **Present** — Real system exists and is meaningfully operational
- **Partial** — Real foundation exists, but incomplete or uneven
- **Planned** — Recognized and intended, but not yet substantially realized
- **Missing** — Should exist as a named system, but does not yet exist formally

## Priority Legend

- **Now** — Load-bearing in the current architecture phase
- **Next** — Important after current load-bearing gaps
- **Later** — Important, but not current critical path

---

## Architectural Framing

NAVI is not a chatbot with extra tools.

NAVI is a persistent agent system built around:

- continuity across time
- a governed capability model
- a unified skill system
- a persona / experience system that modulates tone, presentation, initiative, and interaction style
- a layered architecture spanning Experience, Cognitive, World Model, and Capability concerns

This document maps the major systems required to make that architecture operational.

---

## Systems Overview

### 1. Control Systems

These systems decide how NAVI behaves.

| System | Primary Responsibility | Status | Main Gap | Priority |
|---|---|---:|---|---:|
| [Context Orchestration System (NCOS)](#1-control-systems) | Canonical turn/run context assembly | Partial | Needs one canonical orchestration path and removal of duplicated prompt/runtime logic | Now |
| Inference Control System (ICS) | Model execution behavior, routing, generation policy, fallback posture | Partial | A formal owner now exists and is runtime-wired (`internal/navi/inference/*`, dispatched from `internal/navi/runtime_executor.go:746`). Gap is hardening + test coverage on the untested seams (`outcome_supervisor.go`, `controller_authority.go`, tool/proposal authorization), not first implementation | Now |
| Capability Orchestration System | Exposure, composition, and selection of tools/skills/plugins/workers | Partial | Capability strategy is still weaker than capability existence | Next |
| [Governance / Approval System](../GOVERNANCE.md) | Action validation, permissioning, approval thresholds, autonomy bounds | Present | Needs continued hardening, but the system is real | Now |
| Recovery / Repair System | Failure recovery, degraded mode, resumability, repair actions | Partial | Behavior exists in fragments, not yet as a clean named system | Next |
| Observability / Trace / Replay System | Decision visibility, execution traceability, replay/debugability | Partial | Needs canonical traces and operator-facing inspection | Next |
| Competence / Evaluation System | Outcome quality scoring and systematic improvement loops | Missing | NAVI can act, but does not yet fully evaluate itself as a system | Next |

---

### 2. Cognitive Systems

These systems let NAVI maintain durable understanding and continuity.

| System | Primary Responsibility | Status | Main Gap | Priority |
|---|---|---:|---|---:|
| Memory / Knowledge System | Durable facts, recall, retrievable knowledge | Partial | Needs stronger lifecycle, retrieval, promotion, and scoped memory semantics | Next |
| Reflection / Consolidation System | Convert raw interaction history into structured learning | Partial | Needs stronger consolidation and feedback integration | Next |
| [World Model System](README.md#internalworldmodel) | Structured internal representation of entities, relationships, and state | Present | Needs implementation depth, not conceptual rescue | Now |
| Identity / Contact Graph System | Owner/contact identity, relationship graph, speaker context | Missing | Not yet formalized as a first-class system | Next |
| [Project Cognition System](../specs/project-system-v1.md) | Persistent project-scoped understanding and continuity | Planned | Needs explicit project entities, scoped state, and project-specific cognition | Now |
| [Workspace Scope System](../specs/workspace-v1.md) | Environment and filesystem/repo boundary enforcement | Partial | Strong foundation, incomplete end-to-end enforcement | Now |
| [LLM Knowledge Base / Model Intelligence System](../specs/llm-knowledge-base.plan.md) | Provider/model capability knowledge and routing intelligence | Planned | Needs entity layer, telemetry, and model capability reasoning substrate | Now |
| User Model Assembly | Runtime derived projection of the owner | Partial | Depends on stronger underlying memory, contact, priority, and config systems | Next |

---

### 3. Execution Systems

These systems let NAVI actually do work.

| System | Primary Responsibility | Status | Main Gap | Priority |
|---|---|---:|---|---:|
| [Tool / Skill Runtime System](../canonical/skills.md) | Governed execution of structured capabilities | Present | Strong core; needs continued hardening and expansion | Now |
| Worker / Orchestrator Runtime System | Decomposition and multi-role/multi-step execution | Partial | Needs stronger lifecycle, contracts, and synthesis behavior | Next |
| [Artifact System](../specs/artifact-system-v1.md) | First-class work products and outputs | Present | Needs continued refinement and integration, but system is real | Now |
| Web / Environment Interaction System | Search, fetch, browse, parse, monitor external environments | Partial | Search exists; broader environment interaction is still incomplete | Next |
| Task / Scheduling / Follow-Through System | Ongoing commitments and time-based work | Partial | Needs stronger unfinished-work continuity and long-horizon follow-through | Next |
| [Connector Runtime System](README.md#connectors-and-internalconnectors) | External surface and system interaction across channels/tools | Partial | Connector reliability and consistency still vary materially | Now |
| [Plugin System](../canonical/INDEX.md#plugins) | Packaged installable capability bundles | Partial | Needs stronger lifecycle, packaging, and trust/policy cohesion | Next |
| Capability Acquisition / Skill Synthesis System | Learn, import, normalize, and validate new capabilities | Partial | Core direction exists, but system is not yet fully realized | Next |

---

### 4. Experience Systems

These systems govern how NAVI is presented and controlled by the user.

| System | Primary Responsibility | Status | Main Gap | Priority |
|---|---|---:|---|---:|
| [Persona / Experience System](../specs/persona-system.md) | Tone, initiative, reporting behavior, and interaction style | Partial | Needs continued separation from core reasoning/runtime control | Now |
| Interface / Presentation Control System | Surface-specific interaction contracts and operator modes | Missing | Too much of this still leaks across prompts and surface logic | Next |
| PET / Operator Surface System | Owner control surface for activity, proposals, artifacts, projects, health | Partial | Backend direction is ahead of the actual control surface | Next |
| Multi-Surface Experience Layer | Consistent behavior across Telegram, PET, CLI, and future surfaces | Partial | Needs explicit cross-surface experience policy | Next |

---

### 5. Platform Systems

These are the cross-cutting systems everything else rests on.

| System | Primary Responsibility | Status | Main Gap | Priority |
|---|---|---:|---|---:|
| [Provider Abstraction System](llm-dual-plane.md) | Multi-provider model backend integration | Present | Real foundation exists; needs deeper capability mapping and routing maturity | Now |
| [Language-Layer Boundary (Go/Python/TS)](language-layer-contract.md) | Which language owns which authority and how the layers may communicate | Partial | Contract is Active and Phase 1 (shared generated contracts + governed `query_context` read + CI conformance guards) is merged; later phases (Python deciders that influence execution) are not yet built | Now |
| [Configuration / Policy Management System](../specs/configuration.md) | Global and scoped runtime configuration | Partial | Still too distributed across the system | Next |
| Security / Secrets / Auth System | Access control, auth, secret safety, trust boundaries | Partial | Real but ongoing; always needs hardening | Now |
| Provenance / Audit Substrate | Durable cause/effect history for reasoning and execution | Partial | Strong conceptual foundation; needs deeper end-to-end implementation | Next |

---

### 6. Current Strengths

These systems should be treated as architecturally real today:

- Governance / Approval System
- World Model System
- Tool / Skill Runtime System
- Artifact System
- Provider Abstraction foundation
- Core runtime shell / persistent agent direction

These are not speculative anymore. They are part of NAVI’s actual spine.

---

### 7. Major Missing or Under-Formalized Systems

These are the major architectural gaps that should now be treated as first-class systems:

#### Missing

- Competence / Evaluation System
- Identity / Contact Graph System
- Interface / Presentation Control System

#### Under-formalized

- Inference Control System (ICS) — implemented and runtime-wired (`internal/navi/inference/*`), but undertested on the authorization/outcome seams
- Capability Orchestration System
- Recovery / Repair System
- Observability / Trace / Replay System
- Long-horizon Task / Follow-Through System
- Web / Environment Interaction System

---

### 8. Current Load-Bearing Priority Stack

#### Immediate

1. Context Orchestration System (NCOS)
2. Inference Control System
3. Workspace Scope completion
4. Project Cognition System
5. LLM Knowledge Base / Model Intelligence foundation

#### Next wave

6. Capability Orchestration System
7. Observability / Trace / Replay
8. Competence / Evaluation System

#### Strategic expansion

9. Identity / Contact Graph System
10. Long-horizon Follow-Through System
11. Recovery / Repair formalization
12. Interface / Presentation Control System
13. Full Web / Environment Interaction System

---

### 9. Blunt Diagnosis

NAVI does **not** mainly have a missing-features problem.

NAVI has a **systems formalization problem**.

A lot of the core pieces already exist. The bigger issue is that several critical responsibilities are still:

- implied instead of named
- partially built instead of formally owned
- spread across runtime paths instead of held by a clean system boundary

That is why live testing can feel rough even while the architecture is improving.

The fix is not random patching.
The fix is clearer system ownership.

---

### 10. Architectural Rules for Future Additions

When adding new work, use these rules:

#### Add to an existing system if

- the behavior clearly belongs to an already named system
- the existing system already owns that responsibility
- the change deepens implementation without changing architectural boundaries

#### Create a new named system if

- the responsibility is cross-cutting
- the logic is currently smeared across multiple subsystems
- the responsibility is load-bearing for long-term scale
- the absence of a formal owner is already causing runtime drift or bugs

#### Do not create a new system if

- it is just a helper module
- it is just an implementation detail of an existing subsystem
- it does not own a stable architectural responsibility

---

### 11. Maintenance Rules

This document should be updated when:

- a new first-class system is recognized
- a system moves from Missing → Planned
- a system moves from Planned → Partial
- a system moves from Partial → Present
- a system is merged into another system or renamed
- priority changes due to architecture shifts

This document should **not** be edited for every ticket or implementation detail.

---

### 12. Review Cadence

Recommended cadence:

- review after major architecture discussions
- review after major repo restructures
- review when new epics are introduced
- minimum: weekly during heavy architecture formation

---

### 13. Open Questions

These questions are still unresolved enough to matter architecturally:

1. What exact boundary should separate Inference Control from NCOS?
2. How much of capability orchestration belongs in cognition versus capability runtime?
3. Should Project Cognition remain purely world-model-centric, or also gain dedicated runtime surfaces?
4. What is the canonical evaluation model for skill quality, tool quality, and model quality?
5. How should identity/contact graph interact with permissions, memory, and proposals?
6. What is the final boundary between Persona/Experience and Interface/Presentation Control?

---

### 14. Proposed Follow-On Docs

This document is the inventory, not the whole architecture corpus.

The likely follow-on docs are:

- `docs/architecture/inference-control-system.md`
- `docs/architecture/capability-orchestration-system.md`
- `docs/architecture/llm-knowledge-base.md`
- `docs/architecture/project-cognition-system.md`
- `docs/architecture/navi-gap-register.md`

Do not create these until the systems map is accepted.

---

### 15. Change Log

#### 2026-06-03

- Added the Language-Layer Boundary (Go/Python/TS) to Platform Systems, pointing at the [Language-Layer Contract](language-layer-contract.md); Phase 1 shared contracts + governed read surface merged

#### 2026-05-20

- Removed stale external product framing from canonical-doc references
- Clarified that the persona system is the common name for the Experience Layer rather than preset operating modes
- Fixed priority numbering drift

#### 2026-04-06

- Initial living systems map created
- Formalized Present / Partial / Planned / Missing matrix
- Identified load-bearing missing systems
- Established current priority stack
- Marked Inference Control System as a first-class missing system
