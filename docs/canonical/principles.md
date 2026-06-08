**Status:** Active
**Last Updated:** 2026-03-14
**Updated By:** conceptual-design-alignment
**Source of Truth:** [Conceptual Design Overview](conceptual-design-overview.md)

# Core Architecture Principles

Foundational principles for NAVI. Derived from the [Conceptual Design Overview](conceptual-design-overview.md), which is the canonical source of truth for NAVI's architecture. Changes require human approval (see [governance-rules.md](governance-rules.md)).

---

## Vision

**One Human. One AI. Sovereign.**

NAVI is a persistent, per-user intelligence that serves three layered roles through one coherent, deeply personalized experience:

1. **Character** — Social companion, personality, warmth. The foundation.
2. **Assistant** — Productivity, scheduling, task management layered on top.
3. **Coder** — Agentic coding, codebase management, multi-AI orchestration on top of that.

Each layer builds on the previous. The character is always present as connective tissue.

- **AI should not be temporary.** NAVI is a presence; it persists, learns, and grows.
- **Authority is earned.** Autonomy is progressively granted as trust is built.

---

## Core Design Principle

NAVI lives in a symbiotic relationship with its owner. It learns the user the way the user learns it. Everything starts minimal and grows continuously — nothing is fixed, everything is dynamic and emergent.

**Entities are the core modeling unit; attributes, relationships, and provenance deepen them over time.**

---

## Four-Layer Architecture

NAVI's architecture is organized into four distinct layers, each with a primary responsibility and a defined interface to adjacent layers.

```
┌───────────────────────────────────────┐
│         Experience Layer              │  ← Roles, Personas, Tone, Presentation
│   (modulates cognition + output)      │
├───────────────────────────────────────┤
│         Cognitive Layer               │  ← Conscious + Subconscious Processes
│    ↕ reads/writes ↕                   │
│         World Model                   │  ← Entities, Relationships, State
├───────────────────────────────────────┤
│         Capability Layer              │  ← Commands, Connectors, Plugins
│   (execution arm, called by Decide)   │
└───────────────────────────────────────┘
```

| Layer | Primary Responsibility | Interface |
| :---- | :---- | :---- |
| **Experience** | Determines how NAVI presents itself — role selection, persona, tone, interaction style. Modulates Cognitive output but does not own reasoning. | Reads Cognitive output; applies behavioral policy before delivery. Can bias Cognitive attention. |
| **Cognitive** | Runs Conscious and Subconscious processes. Owns reasoning, reflection, and decision-making. | Reads/writes the World Model. Receives modulation from Experience. Issues Commands to Capability. |
| **World Model** | Single source of truth for all entity state, relationships, and structured knowledge. | Accessed by Cognitive. Not directly modified by Experience or Capability — changes flow through Cognitive. |
| **Capability** | Executes actions in the world. Commands, Connectors, Plugins. | Invoked by Cognitive (specifically the Execute step). Returns results to Cognitive for Reflection. |

Cross-layer interactions happen through defined interfaces, not arbitrary access.

---

## Design Principles

### 1. Zero Framework Cognition

> The platform provides the control kernel. Agents and prompts provide reasoning.

Plumbing (NATS, SQLite, JWT) lives in Go, and so does authoritative runtime control. Prompts contribute reasoning, narration, and candidate generation, but governance handoff, action selection, tool-surface narrowing, execution supervision, recovery ownership, and plan progression live in the Go control kernel. Do not move approval-boundary logic or control-state transitions into prompts.

### 2. Sovereign by Design

NAVI belongs to the user. It is a persistent presence, not an ephemeral tool.

### 3. Documentation-Driven Development

Every change follows: **spec → plan → implementation → validation → revision**. Documentation drives design.

### 4. Agents Are Persistent, Not Stateless

NAVI runs continuously with a heartbeat. It schedules work, monitors progress, and reports — without waiting for a user prompt.

### 5. The Conscious Process

The foreground cognitive loop: **Perceive → Interpret → Contextualize → Decide → Validate/Govern → Execute → Reflect**. Each step has a single responsibility. See [Conscious Process — Step Mapping](../concepts/conscious-loop-steps.md).

### 6. The Subconscious Process

Background learning and reorganization at three tiers: Shallow Reflections (frequent, post-interaction), Consolidation (periodic, daily), Deep Reflections (infrequent, weekly or event-triggered). Each tier has defined mutation permissions over the World Model.

### 7. Memory as a First-Class Citizen

Long-term structured memory is the foundation. The World Model stores Entities (Contacts, Events, History, Knowledge, Memories, Artifacts, Proposals) with Relationships that carry confidence, recency, and provenance metadata.

### 8. Governance at the Decision Boundary

Governance constrains what NAVI is permitted to do, regardless of capability. Validation checks execute in deterministic order: Permissions → Policy → Configuration → Priority Alignment → Risk. Three-tier authority: System > Owner > Plugin.

---

## Architectural Guardrails (What We Will Not Merge)

- **ORMs in the core state engine** — Raw SQL only. No Gorm, no SQLX wrappers.
- **Synchronous execution in the loop** — Blocking network calls go to background workers or NATS; the state loop never blocks.
- **Nested agent hierarchies** — No "Manager of Managers." Task-specific workers attached to a centralized orchestration loop.
- **Prompt-owned runtime control** — Zero Framework Cognition requires an explicit Go control kernel for authoritative runtime decisions.
- **Embedded MCP runtime in core** — External tools use bridge patterns.
- **Direct World Model writes from Experience or Capability** — All state mutations flow through Cognitive processes.

---

## Rules of Engagement (Technical)

1. **Schema truth:** `internal/schema/` (Go) is master; generate Python via `make generate-python`. Never edit Python models by hand.
2. **Determinism:** Agent loop logic must be deterministic.
3. **Execution over orchestration:** Specialized task-oriented agents over generic Manager agents.
4. **Test-driven:** Failing tests first; `make test` often.
5. **No mocks:** Use real implementations; mock only LLM calls via fixtures.
6. **Graceful halting:** On missing design or broken state, stop and notify; do not guess.

---

[canonical INDEX](INDEX.md) · [GOVERNANCE](../GOVERNANCE.md) · [Conceptual Design Overview](conceptual-design-overview.md)
