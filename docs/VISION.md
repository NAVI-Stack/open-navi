# NAVI AI — VISION & ROADMAP

**Status:** Active  
**Last Updated:** 2026-04-01  
**Source of Truth:** [Conceptual Design Overview](canonical/conceptual-design-overview.md)

> **The Brain: Autonomous Agent Runtime, Identity & Intelligence Orchestration**

---

## Vision: One Human. One AI. Sovereign.

NAVI AI is the persistent, per-user intelligence at the center of the NAVI Ecosystem. It is not a chatbot, an autocomplete tool, or a prompt wrapper — it is a **running autonomous agent** with its own identity, memory, goals, and a background heartbeat that allows it to work without constant human supervision.

The end state: a user's personal AI that knows them deeply, can be trusted with long-running tasks, orchestrates specialized tools and environments (NAVI Hub, Net, OS), and gets smarter over time through structured memory and learning.

### AI Should Not Be Temporary
Most AI systems are ephemeral sessions. NAVI is a **presence**. It persists, learns, and grows alongside you. It is bound to your identity and portable across your digital life.

### Authority Is Earned
NAVI begins as an assistant and grows into an autonomous digital counterpart. Autonomy is not assumed; it is progressively granted as trust is built. You define the boundaries of what it can research, execute, and transact.

---

## Three Layered Roles

NAVI serves three user archetypes through one coherent experience. Each role builds on the previous; the character is always present as connective tissue.

1. **Character** — Social companion, personality, warmth. The foundation.
2. **Assistant** — Productivity, scheduling, task management layered on top.
3. **Coder** — Agentic coding, codebase management, multi-AI orchestration on top of that.

---

## Core Design Principle

NAVI lives in a symbiotic relationship with its owner. It learns the user the way the user learns it. Everything starts minimal and grows continuously — nothing is fixed, everything is dynamic and emergent.

**Entities are the core modeling unit; attributes, relationships, and provenance deepen them over time.**

---

## Four-Layer Architecture

NAVI's architecture is organized into four distinct layers. For full detail see the [Conceptual Design Overview](canonical/conceptual-design-overview.md).

```
┌───────────────────────────────────────┐
│         Experience Layer              │  ← Persona System (Roles, Traits)
├───────────────────────────────────────┤
│         Cognitive Layer               │  ← Conscious + Subconscious Processes
│    ↕ reads/writes ↕                   │
│         World Model                   │  ← Entities, Relationships, State
├───────────────────────────────────────┤
│         Capability Layer              │  ← Commands, Connectors, Plugins
└───────────────────────────────────────┘
```

| Layer | Responsibility |
|-------|---------------|
| **Experience** | Persona System (role selection, traits, tone, interaction style). Modulates Cognitive output; does not own reasoning. |
| **Cognitive** | Conscious Process (Perceive → Interpret → Contextualize → Decide → Validate/Govern → Execute → Reflect) and Subconscious Process (Shallow Reflections, Consolidation, Deep Reflections). Owns all reasoning and decision-making. |
| **World Model** | Single source of truth for entity state: Contacts, Events, History, Knowledge, Memories, Projects, Artifacts, Proposals, Configuration, Priorities. Relationships carry confidence, recency, and provenance. |

| **Capability** | 10 primitive Commands (Query, Create, Update, Delete, Invoke, Send, Acquire, Schedule, Delegate, Compose), Connectors (Communication, Information, Service, Device/Environment + Identity plane), and Plugins (Domain, Workflow, Integration, Agentic, Sensory). |

---

## Design Philosophy

### 1. Agents Are Persistent, Not Stateless
NAVI AI runs continuously. It has a heartbeat. It can schedule work, monitor progress, and report back — without waiting for a user prompt. This is the fundamental break from request/response AI tools.

### 2. The Conscious Process
The foreground cognitive loop: Perceive → Interpret → Contextualize → Decide → Validate/Govern → Execute → Reflect. Active during engagement, drives real-time interaction.

### 3. The Subconscious Process
Background learning at three tiers — Shallow Reflections (post-interaction), Consolidation (daily), Deep Reflections (weekly/event-triggered) — with defined mutation permissions at each tier.

### 4. Memory as a First-Class Citizen
Long-term structured memory is not a feature — it is the foundation. The World Model stores entities with provenance, confidence, and relationship metadata, enabling NAVI to reason about what it knows, how it learned it, and how confident it is.

### 5. Governance at the Decision Boundary
Governance constrains what NAVI is permitted to do, regardless of what it is capable of doing. Five checks in deterministic order (Permissions, Policy, Configuration, Priority Alignment, Risk), three authority tiers (System > Owner > Plugin), and four outcomes (Approved, Requires Confirmation, Modified, Rejected).

### 6. Zero Framework Cognition
Go provides the authoritative control kernel. Agents and prompts contribute reasoning, narration, and candidate generation, but runtime authority stays in code. Governance handoff, action selection, tool-surface narrowing, execution supervision, recovery ownership, and plan progression belong to the Inference Control System and adjacent Go runtime seams, not to prompt-only heuristics.

---

## Directive Modes

| Mode | Behavior |
|------|----------|
| `CHAT` | Conversational only. No actions. |
| `ADVISE` | Analysis and planning. No actions. |
| `ASSIST` | Routine tasks within pre-approved boundaries. |
| `ACT` | Full workflow execution with defined permissions. |
| `WATCH` | Monitor conditions; trigger alerts or actions on threshold. |

---

## Lineage

NAVI AI synthesizes patterns from:

| Source | Contribution |
|---|---|
| **MegaMan Battle Network (NAVI)** | Concept of a persistent, personal digital partner. |
| **OpenClaw** | Skill/plugin system, memory model, persistent agent patterns |
| **PicoClaw** | Heartbeat design, Go architecture, ultra-lightweight runtime |
| **OpenManus** | Multi-LLM framework, TOML config, Ollama-first local model setup |

---

## Project Roadmap

| Phase | Description | Status |
|---|---|---|
| 1-12 | Core Infrastructure (NATS, SQLite, Gateway, Remote Connectors, LLM) | Done |
| 13 | Coder + Orchestration IMPLEMENT mode — real two-pass LLM execution | Done |
| 14 | CriticAgent + StrategistAgent — real LLM-backed implementations | Done |
| 15 | Self-Extension Pipeline + reliability hardening | Active |
| 16 | Federated Memory + Distributed Knowledge | Future |
| 17+ | Postgres migration, advanced sandboxing, marketplace | Future |

---

## Architectural Guardrails (What We Will Not Merge)

- **ORMs in the Core State Engine** — Raw SQL only (SQLite/PostgreSQL). No Gorm, no SQLX wrappers.
- **Synchronous Execution** — All blocking network calls go to background workers or NATS topics. The state loop never blocks.
- **Nested Agent Hierarchies** — No "Manager of Managers." Agents are task-specific workers attached to a centralized orchestration loop.
- **Prompt-Owned Runtime Control** — Zero Framework Cognition requires authoritative runtime control in Go; prompts support reasoning but do not replace the control seam.
- **Embedded MCP Runtime** — External tools use bridge patterns, not embedded runtimes in core.
- **Direct World Model Writes from Experience or Capability** — All state mutations flow through Cognitive processes.

---

## Navigation

- **[AGENTS.md](AGENTS.md)** — Directives for AI coding assistants
- **[Documentation Index](README.md)**
- **[Back to Ecosystem Root](../README.md)**
- **[Conceptual Design Overview](canonical/conceptual-design-overview.md)** — Canonical conceptual model for roles, layers, entities, governance, and reflection.
- **[Core Principles](canonical/principles.md)** — Architecture principles derived from the conceptual design.
