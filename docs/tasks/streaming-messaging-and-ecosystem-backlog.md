% NAVI Streaming, Capabilities, and Ecosystem Backlog

**Scope:** Streaming + messaging runtime, unified capability system, MCP runtime, web UI/experience layer, hot reload, hooks, DX, manifests, ecosystem, runtime profiles, and autonomy UX.  
**Purpose:** Formalize cross-cutting tasks and plans that tie NAVI’s always-on architecture to streaming UX, extensibility, and ecosystem readiness.  
**Related:** [VISION.md](../VISION.md), [canonical/conceptual-design-overview.md](../canonical/conceptual-design-overview.md), [streaming-and-message-runtime.plan.md](../plans/streaming-and-message-runtime.plan.md).

---

## 1. Streaming + Messaging Architecture

### Task 1.1 — Define Real-Time Interaction Transport Model

**Objective:** Replace purely turn-based interaction assumptions with a first-class streaming session model.

**Problem:** NAVI is intended to be reachable from app, web, and messaging surfaces as an always-on assistant, but the current conceptual model is stronger on cognition and governance than on real-time interaction transport and event semantics.

**Deliverable:**

- Canonical session event model
- Event taxonomy for:
  - user input started/updated/submitted
  - assistant response started/chunk/completed
  - tool invocation started/progress/completed
  - proposal surfaced
  - interruption/correction
  - background task state update
- Streaming protocol recommendation for CLI, web, and connector surfaces

**Acceptance criteria:**

- One canonical event envelope works across CLI, web UI, and messaging connectors
- Supports partial output, cancellation, and resumability
- Compatible with existing execution/history model

### Task 1.2 — Define Multi-Message Concurrency and Queue Semantics

**Objective:** Specify how NAVI handles overlapping user inputs, background events, and long-running responses.

**Problem:** An always-on agent cannot pretend only one turn exists at a time. The current conceptual architecture already has foreground and background processes, plus proposals and interruptions, so messaging concurrency needs an explicit runtime model instead of implicit behavior.

**Deliverable:**

- Message queue policy
- Interrupt policy
- Priority model for:
  - active user turn
  - urgent subconscious correction
  - blocking proposal
  - queued proposal
  - background status event
- Rules for merge, supersede, defer, or reject incoming messages

**Acceptance criteria:**

- No ambiguous behavior when multiple messages arrive during generation
- Urgent interruptions map cleanly to the existing interruption rules in the conceptual design
- Blocking vs queued behaviors align with Proposal Queue semantics

### Task 1.3 — Define Streaming UX State Machine

**Objective:** Formalize what the user sees during streaming.

**Deliverable:**

- UX state model for:
  - listening
  - thinking
  - streaming
  - waiting on tool
  - waiting on approval
  - backgrounding
  - resumed response
  - error/degraded mode
- Rules for token streaming vs semantic event summaries

**Acceptance criteria:**

- Same state machine can drive CLI and future web UI
- Degraded/failure states align with NAVI’s existing failure model and proposal/recovery logic

---

## 2. Unified Plugin / Extension System

### Task 2.1 — Define Unified Capability Registration Contract

**Objective:** Create one registration surface for Skills, Connectors, and Plugins.

**Problem:** NAVI’s Capability Layer conceptually includes Commands, Connectors, and Plugins, while Skills are separately canonical and governed. That is architecturally clean, but extension authoring is still fragmented.

**Deliverable:**

- Unified registration API specification
- Registration lifecycle:
  - discover
  - validate
  - register
  - activate
  - deactivate
  - retire
- Mapping of:
  - Skill → governed capability registration
  - Connector → transport/integration registration
  - Plugin → packaged multi-capability registration

**Acceptance criteria:**

- Extension authors do not need three unrelated mental models
- Skills remain canonical governed units, not replaced
- Preserves current architectural separation where appropriate

### Task 2.2 — Define Capability Packaging Boundaries

**Objective:** Decide what belongs in Skill vs Connector vs Plugin under the new unified model.

**Problem:** If you skip this, the unified API becomes a dumping ground.

**Deliverable:**

- Formal boundary rules
- Decision matrix:
  - When something must be a Skill
  - When something must be a Connector
  - When something should be a Plugin
- Examples for common NAVI use cases

**Acceptance criteria:**

- No overlap ambiguity for at least 90 percent of expected extension types
- Consistent with the current rule that Skills do not own decision logic and do not bypass governance

### Task 2.3 — Define Capability Registry Evolution Plan

**Objective:** Evolve the capability registry to support unified registration without breaking OSS-27 skill semantics.

**Deliverable:**

- Registry schema changes
- Backward compatibility strategy
- Migration plan for existing skills/connectors/plugins

**Acceptance criteria:**

- Existing OSS-27 skill loading still works
- Unified registry can expose capabilities to cognition and UX deterministically

---

## 3. MCP Runtime Support

### Task 3.1 — Define MCP Integration Architecture

**Objective:** Turn MCP from declared transport into real runtime support.

**Problem:** NAVI’s canonical skill definition already includes `mcp_tool` as a supported transport type, but that does not matter until the runtime actually supports it.

**Deliverable:**

- MCP client architecture
- MCP server exposure architecture
- Trust and sandbox model for MCP endpoints
- Session/auth model for MCP tools

**Acceptance criteria:**

- NAVI can call external MCP tools
- NAVI can expose internal capabilities over MCP
- Governance still applies before execution

### Task 3.2 — Map OSS-27 Skills to MCP

**Objective:** Define how skill interfaces compile to MCP tools and vice versa.

**Deliverable:**

- Mapping spec:
  - skill_id/interface_name ↔ MCP tool identity
  - input/output schema translation
  - error envelope translation
  - auth/security metadata handling
- Constraints and unsupported cases

**Acceptance criteria:**

- Lossless enough for practical interoperability
- Does not weaken OSS-27 governance metadata

### Task 3.3 — Define MCP Safety and Trust Policy

**Objective:** Prevent MCP from becoming a governance bypass.

**Deliverable:**

- Trust-tier policy for MCP providers
- Confirmation rules for external MCP tools
- Provenance tagging and audit requirements

**Acceptance criteria:**

- External MCP tools cannot silently bypass the Governor or policy checks
- Audit trail is preserved in execution history and capability records

---

## 4. Web UI / Experience Layer Implementation

### Task 4.1 — Define Initial Web UI Product Surface

**Objective:** Specify the first real web UI instead of a placeholder.

**Problem:** NAVI’s architecture assumes a meaningful Experience Layer, but the current implementation maturity is concentrated below that layer.

**Deliverable:**

- v1 web UI scope
- Required views:
  - chat/session view
  - proposals queue
  - task/activity feed
  - skills/plugins view
  - connector status
  - autonomy/settings panel

**Acceptance criteria:**

- Covers minimum operational visibility for an always-on assistant
- Maps cleanly to entities already defined in the conceptual model

### Task 4.2 — Define Real-Time UI Event Binding

**Objective:** Bind the web UI to the streaming/event architecture.

**Deliverable:**

- UI subscription model
- State reconciliation rules
- Offline/reconnect behavior
- Partial-stream rendering behavior

**Acceptance criteria:**

- UI can recover from disconnects without corrupting state
- Event-driven updates work for foreground and background actions

### Task 4.3 — Define Proposal and Recovery UX

**Objective:** Make proposals, failure recovery, and approval flows usable.

**Deliverable:**

- UI patterns for:
  - blocking proposals
  - queued proposals
  - partial execution recovery
  - degraded capability alerts
- Approval/decline/resolution flows

**Acceptance criteria:**

- Matches the Proposal Queue and failure/recovery model already defined conceptually

---

## 5. Hot Reload for Connectors and Plugins

### Task 5.1 — Define Dynamic Lifecycle Management

**Objective:** Support load/unload/reload without full process restart.

**Deliverable:**

- Runtime lifecycle contract for connectors/plugins
- Readiness and drain states
- Version swap behavior
- Failure rollback policy during reload

**Acceptance criteria:**

- Reload does not corrupt live sessions
- Failed reload returns system to prior working state

### Task 5.2 — Define Isolation Strategy for Reloadable Components

**Objective:** Ensure reload safety by process boundary, not wishful thinking.

**Deliverable:**

- Process model for reloadable extensions
- IPC contract
- Health check and supervision model

**Acceptance criteria:**

- Crashed or reloading extension does not kill the main runtime
- Consistent with existing plugin isolation principles in the conceptual model

---

## 6. Developer Experience / Extension Authoring

### Task 6.1 — Define Extension Authoring Toolchain

**Objective:** Make it easy to create new Skills, Connectors, and Plugins correctly.

**Deliverable:**

- CLI scaffolding commands
- Templates for:
  - OSS-27 skill
  - connector
  - plugin
  - MCP-backed skill
- Local validation tool

**Acceptance criteria:**

- New extension can be scaffolded and validated in minutes
- Generated templates enforce canonical metadata expectations from the skills spec

### Task 6.2 — Define Local Test Harness for Extensions

**Objective:** Let developers test capabilities without booting the whole world every time.

**Deliverable:**

- Mock runtime harness
- Schema validation harness
- event/replay harness
- connector/plugin contract tests

**Acceptance criteria:**

- Skill authors can test input/output and governance checks locally
- Connector authors can simulate degraded and failure cases

### Task 6.3 — Define Extension Documentation Standard

**Objective:** Standardize how extensions document usage, safety, and lifecycle.

**Deliverable:**

- Required docs sections
- Examples
- validation checklist

**Acceptance criteria:**

- Every extension has uniform documentation for capability, constraints, and risks

---

## 7. Typed Hook System

### Task 7.1 — Define Full Runtime Hook Taxonomy

**Objective:** Expand hooks into a formal lifecycle system.

**Deliverable:**

- Hook map for:
  - session start/end
  - perceive
  - contextualize
  - decide
  - validate
  - execute
  - reflect
  - proposal create/resolve
  - failure/recovery
  - background task state change

**Acceptance criteria:**

- Hooks cover both conscious-loop and background-runtime events
- No hook implies governance bypass

### Task 7.2 — Define Hook Ordering, Priority, and Safety

**Objective:** Prevent hook chaos.

**Deliverable:**

- Ordering rules
- priority rules
- mutation permissions per hook type
- timeout/failure behavior for hooks

**Acceptance criteria:**

- Hook execution is deterministic
- Hook failures degrade safely, not catastrophically

### Task 7.3 — Define Hook Observability

**Objective:** Make hook behavior auditable.

**Deliverable:**

- hook trace schema
- correlation IDs
- latency/error metrics

**Acceptance criteria:**

- Hook side effects are visible in traces and debugging output

---

## 8. Manifest-Driven Discovery

### Task 8.1 — Define Canonical Manifest Format

**Objective:** Standardize extension discovery metadata.

**Deliverable:**

- One manifest spec for Skills/Connectors/Plugins
- identity, version, publisher, trust, entrypoints, dependencies, compatibility

**Acceptance criteria:**

- Manifest works for workspace, user-global, and built-in discovery tiers
- Respects existing skill precedence and identity rules based on `skill_id` and versioning

### Task 8.2 — Define Discovery Paths and Precedence

**Objective:** Remove ambiguity in how artifacts are found and overridden.

**Deliverable:**

- search path rules
- precedence rules
- conflict resolution behavior
- duplicate/override diagnostics

**Acceptance criteria:**

- Deterministic resolution across local, bundled, and future remote sources

---

## 9. Ecosystem / Marketplace Strategy

### Task 9.1 — Define Trust and Distribution Model

**Objective:** Design how third-party capabilities enter NAVI safely.

**Problem:** NAVI’s project direction explicitly points toward a future skill ecosystem and marketplace-ready foundation, which means supply-chain discipline is not optional.

**Deliverable:**

- publisher identity model
- signing/verification model
- trust tiers for distributed extensions
- install policy matrix

**Acceptance criteria:**

- Third-party capabilities do not default to high-autonomy execution
- Consistent with existing trust-tier concepts in the canonical skill model

### Task 9.2 — Define Skill / Plugin Registry Architecture

**Objective:** Specify the future external registry and install flow.

**Deliverable:**

- registry API requirements
- package metadata
- search/discovery/install/update/retire flows
- provenance and compatibility checks

**Acceptance criteria:**

- Supports curated and community channels
- Enables pinning and rollback

---

## 10. Runtime Flexibility: Local Mode vs Distributed Mode

### Task 10.1 — Define Deployment Profiles

**Objective:** Prevent over-engineering every install into a distributed system.

**Deliverable:**

- profile definitions:
  - embedded/local-single-process-ish mode
  - hybrid mode
  - distributed durable mode
- feature/support matrix per profile

**Acceptance criteria:**

- Same conceptual architecture, multiple operational footprints
- Smaller setups do not require full production complexity

### Task 10.2 — Define Service Optionality Boundaries

**Objective:** Decide what can be embedded and what must remain external.

**Deliverable:**

- boundary document for:
  - event bus
  - task orchestration
  - connector runtime
  - plugin runtime
  - tracing
  - persistence

**Acceptance criteria:**

- Clear line between “optional for dev/small deploy” and “mandatory for durable autonomy”

---

## 11. Autonomy UX and Explainability

### Task 11.1 — Define User-Facing Autonomy Controls UX

**Objective:** Translate the conceptual autonomy model into an actual interface.

**Problem:** NAVI already has a rich autonomy model with global preset, per-domain overrides, hard floors, and proposal interaction rules. That’s strong conceptually, but still abstract until turned into usable controls.

**Deliverable:**

- settings UX for:
  - global preset
  - per-domain overrides
  - plugin invocation authority
  - messaging/scheduling/coding autonomy
- explanation text and risk indicators

**Acceptance criteria:**

- Users can understand what changes when they move autonomy up or down
- UX reflects hard-floor rules exactly, not approximately

### Task 11.2 — Define Explainability Surfaces for Agent Actions

**Objective:** Make NAVI legible when it acts, pauses, or refuses.

**Deliverable:**

- explanation templates for:
  - why an action ran
  - why it required approval
  - why it was blocked
  - why a plugin/skill was selected
  - why a proposal was generated
- short-form and deep-dive forms

**Acceptance criteria:**

- Compatible with governance outcomes already defined: Approved, Requires Confirmation, Modified, Rejected

### Task 11.3 — Define Proposal Queue UX Semantics

**Objective:** Make the queue understandable, not just stored.

**Deliverable:**

- queue grouping, priority, expiry, supersede, recovery views
- resolution affordances

**Acceptance criteria:**

- Directly mirrors the conceptual Proposal Queue lifecycle and resolution rules

---

## 12. Recommended Execution Order

Do not spread effort evenly; this is how half-specs happen. Execute roughly in this order:

1. Streaming + Messaging Architecture
2. Unified Plugin / Extension System
3. MCP Runtime Support
4. Web UI / Experience Layer
5. Hot Reload
6. Typed Hooks
7. Developer Experience / Extension Authoring
8. Manifest-Driven Discovery
9. Autonomy UX and Explainability
10. Runtime Flexibility
11. Ecosystem / Marketplace Strategy

**Next move:** Turn **Streaming + Messaging Architecture** (Tasks 1.1–1.3) into a fully aligned formal design spec with scope, non-goals, architecture, event model, APIs, state machines, and implementation task breakdown, referencing and, if needed, extending `streaming-and-message-runtime.plan.md`.

