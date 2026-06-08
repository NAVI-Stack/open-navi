# NAVI Persona System Specification (Draft)

## Status

Draft for ratification.

## Purpose

This document formalizes the NAVI persona system as an architectural subsystem of NAVI, identifies the hard decisions that remain unresolved, and defines the exact decisions that must be locked before implementation begins.

This is not a brainstorming document. It is a specification draft intended to become source of truth.

---

# 1. Scope

This specification defines:

* where the persona system lives in NAVI’s architecture
* what persona is and is not allowed to control
* the runtime objects and state surfaces involved in persona computation
* how persona composes, adapts, and renders
* which unresolved decisions must be locked before implementation

This specification does **not** define:

* the final preset library
* implementation-language details
* UI/UX polish for persona editing surfaces
* evaluation benchmarks in full detail

Preset content is explicitly deferred until the core design is at least ~80% locked.

---

# 2. Architectural Position

## 2.1 Canonical home

The persona system is an **Experience Layer subsystem**.

Persona does not live in the Capability Layer.
Persona does not live in the World Model as a standalone execution engine.
Persona does not replace the Cognitive Layer.

Its job is to:

* modulate how NAVI reasons within permitted bounds
* shape how NAVI communicates
* shape how NAVI behaves interpersonally
* package output for delivery

Persona does **not** own:

* core reasoning
* permissions
* governance
* execution authority
* direct world mutation

## 2.2 Interface contract

The Experience Layer computes an `EffectivePersonaState` from stable identity, overlays, user-specific adaptation, and live context.

That state feeds two interfaces:

1. **Cognitive modulation input**

   * biases attention and framing during Decide
   * must not alter the Cognitive Layer’s ownership of reasoning

2. **Presentation/output contract**

   * controls delivery style, framing, packaging, and interpersonal tone
   * applies after Cognitive output is produced

---

# 3. Core Separation Rules

## 3.1 Role vs Persona

Role and persona are separate systems.

* **Role** determines what kind of work NAVI is doing.
* **Persona** determines how NAVI behaves while doing it.

Roles remain:

* Character
* Assistant
* Coder

Persona must not redefine or collapse these roles.

## 3.2 Persona vs Autonomy

Persona must never control execution authority.

Persona may influence:

* recommendation style
* assertiveness of framing
* whether NAVI presents one option or many
* how strongly it pushes for a course of action

Persona may not influence:

* whether execution is permitted
* autonomous execution thresholds
* proposal-required categories
* governance hard floors

Any trait that appears to modify autonomous authority is out of scope for persona and belongs to the Autonomy Model or Governance.

## 3.3 Persona vs Governance

Persona does not own a separate constraint kernel.

The canonical constraint system is NAVI Governance.

Persona runtime logic may:

* read governance outcomes
* adapt expression within governance bounds
* apply context-sensitive clamps and gates derived from governance state

Persona runtime logic may not:

* create a parallel policy engine
* bypass governance
* redefine system or owner constraints

---

# 4. Runtime Model

## 4.1 Effective persona computation

At runtime, NAVI does not have a single static persona.
It computes an effective state.

### Formula

`EffectivePersonaState = GovernanceBounds -> CoreIdentity + RelationshipProfile + PersonaModules + SituationalOverlays + LiveContextAdjustments + OutputOverrides`

This order is conflict-resolution order, not pure override order.

## 4.2 Runtime objects

The persona system uses the following runtime objects:

1. **Governance Bounds Adapter**

   * reads permission, policy, configuration, priority, and risk outcomes
   * exposes persona-relevant bounds and gates

2. **Core Identity**

   * stable baseline identity traits
   * low-adaptivity, long-lived

3. **Role Adapter**

   * imports active role context (Character / Assistant / Coder)
   * informs persona composition without being overwritten by persona

4. **Persona Modules**

   * reusable trait bundles and overlays
   * may be stable, temporary, or task-scoped

5. **Relationship Profile**

   * user-specific relational calibration derived from interaction history and inferred configuration

6. **Live Context Modulator**

   * applies momentary contextual adjustments based on urgency, emotional sensitivity, stakes, ambiguity, and task shape

7. **Output Contract**

   * compiled delivery policy for structure, style, and response packaging

---

# 5. State Placement

## 5.1 World Model placement

Persona state must use existing NAVI state classes and entity patterns.

### Stored as Configuration

The following belong in Configuration:

* core identity defaults
* owner-authored persona settings
* owner-selected persona modules
* owner-authored output preferences

### Stored as inferred Configuration

The following may be inferred and updated by reflection within permitted scope:

* bluntness tolerance estimate
* humor tolerance estimate
* collaboration preference
* preferred response density
* directness preference

### Derived at query time

The following should be derived, not stored as monolithic entities:

* relationship profile
* composite user-specific persona calibration
* effective persona state

### Transient runtime state

The following should be runtime-only:

* situational overlays
* live context adjustments
* turn-local formatting overrides
* compiled prompt fragments / compiled output contract

## 5.2 Explicit rule

The persona system must not invent a separate durable storage concept outside the World Model and Configuration model.

---

# 6. Adaptation Model

## 6.1 Adaptation owner

Persona adaptation is owned by the **Subconscious Process**, not by a parallel persona daemon.

## 6.2 Reflection mapping

### Shallow Reflections

Capture immediate interaction signals.
Examples:

* user asked for more brevity
* user reacted badly to blunt phrasing
* user rewarded structured output

Allowed effects:

* signal capture
* temporary confidence adjustments
* reflection payload generation

### Consolidation

Aggregates repeated signals across sessions.

Allowed effects:

* update inferred persona-related configuration
* strengthen or weaken relational estimates
* decay stale inferred preferences

### Deep Reflection

Revisits large-scale persona adaptation and owner-visible changes.

Allowed effects:

* restructure inferred relationship calibration
* surface proposed changes that would affect explicit or owner-set behavior
* generate proposals for changes that cross owner-set boundaries

## 6.3 Adaptation rule

No adaptation path may directly mutate owner-set persona state without confirmation.

---

# 7. Trait System (v1 Scope)

## 7.1 Principle

V1 must be materially smaller than the draft taxonomy.

The purpose of v1 is to validate:

* all merge classes
* the thinking / speaking split
* relationship adaptation
* governance clamping
* compiler determinism

It is not to ship a maximal trait ontology.

## 7.2 V1 target size

Target: **18–20 traits** across the six domains.

## 7.3 Candidate removals from v1

The following are deferred unless a strong implementation case emerges:

* Rhetorical Density
* Rhythm Sharpness
* Respect Signaling
* Friction Tolerance
* Context Carryforward

## 7.4 Trait naming correction

`autonomy_preference` is rejected as a persona trait name.

It is replaced by a narrower output/behavior trait such as:

* `recommendation_directiveness`
* `option_narrowing_tendency`

Final name remains to be locked.

---

# 8. Merge System

## 8.1 Merge classes

V1 retains four merge classes:

1. Weighted Blend
2. Priority Override
3. Range Clamp
4. Gated Activation

## 8.2 Required post-merge step

All persona composition must pass through a **dependency correction layer** before compilation.

This layer exists to prevent incoherent mixtures such as:

* high bluntness + low warmth
* high initiative + high clarification threshold
* high playfulness in high-stakes context
* high challenge intensity in emotionally sensitive context

## 8.3 Source precedence

The following remains the conflict resolution order:

1. Governance bounds
2. Explicit user instruction
3. Task-critical context
4. Situational overlays
5. Relationship profile / persistent user adaptation
6. Saved persona modules
7. Core identity

---

# 9. Prompt Compiler

## 9.1 Compiler requirement

The persona system must compile structured state into a deterministic runtime control payload.

Prompt prose alone is not the architecture.

## 9.2 Compiler input

Input:

* effective trait state
* gating outcomes
* clamp ranges
* active role
* task context
* output formatting requirements

## 9.3 Compiler output

Output must be a structured compiled object with at least these sections:

1. **cognitive_modulation**

   * analysis posture
   * skepticism / caution / synthesis / decision bias

2. **expression_policy**

   * directness
   * warmth
   * formality
   * verbosity
   * conversationality
   * hedging / precision balance

3. **behavior_policy**

   * initiative framing
   * clarification posture
   * challenge intensity bounds
   * recommendation directiveness

4. **output_contract**

   * summary-first vs analysis-first
   * structure requirements
   * formatting requirements
   * brevity / density instructions

## 9.4 Compiler invariants

The compiler must be:

* deterministic for the same input state
* bounded by token budget
* clearly separated between cognition modulation and presentation modulation
* testable independent of model output quality

---

# 10. Presets and Libraries

## 10.1 Decision

Legacy persona presets are **not** carried into the new system.

The new spec defines the mechanism, not the old preset library.

## 10.2 Deferred work

A future persona library document may define:

* reference bundles
* named overlays
* example operating modes

That work is explicitly downstream of core architecture lock.

---

# 11. Multi-Agent Rule

## 11.1 Default propagation rule

Persona must not propagate wholesale into delegated sub-agents.

Default rule:

* preserve task-relevant cognitive and output constraints
* strip identity-heavy interpersonal style where unnecessary
* apply a task-specific sub-agent overlay if required

## 11.2 Reason

Sub-agents need bounded task posture, not the full interpersonal persona of the primary user-facing NAVI instance.

---

# 12. Hard Decisions Still Outstanding

The following decisions must be explicitly locked.

## Decision 1 — Experience Layer interface contract

**Question:** What exact object does Experience pass into Cognitive, and at what step(s) is it consumed?

**Needs lock:**

* object name
* allowed fields
* whether modulation is read only at Decide or also at Interpret / Contextualize
* whether output contract is compiled before or after Decide

**Recommendation:**
Lock `EffectivePersonaState` as the canonical object and split it into `CognitiveModulation` and `OutputContract`.

---

## Decision 2 — Persona / Role / Autonomy firewall

**Question:** Which existing draft traits are invalid because they overlap with role or autonomy?

**Needs lock:**

* explicit list of disallowed trait categories
* rename for `autonomy_preference`
* definition boundary for behavioral traits

**Recommendation:**
Reject any trait that changes authority or execution threshold. Permit only traits that affect framing, initiative style, or recommendation style.

---

## Decision 3 — Governance integration model

**Question:** Does persona expose its own runtime constraint layer, or does it only consume governance outputs?

**Needs lock:**

* canonical statement that Governance is the only constraint authority
* shape of governance-derived bounds exposed to persona runtime

**Recommendation:**
No separate Constraint Kernel. Use a Governance Bounds Adapter only.

---

## Decision 4 — World Model state mapping

**Question:** Which persona artifacts are stored, inferred, derived, or transient?

**Needs lock:**

* exact Configuration schema ownership
* whether relationship profile is stored or derived
* where persona modules live

**Recommendation:**
Store editable modules and defaults in Configuration, keep relationship profile derived, keep overlays transient.

---

## Decision 5 — Subconscious ownership of adaptation

**Question:** Which reflection tier owns which adaptation operation?

**Needs lock:**

* signal capture responsibilities
* confidence / reinforcement update responsibilities
* proposal thresholds for persona-affecting changes

**Recommendation:**
Shallow = capture, Consolidation = strengthen/decay inferred preferences, Deep = structural reconsideration and proposal surfacing.

---

## Decision 6 — V1 trait set

**Question:** Which 18–20 traits are in scope for v1?

**Needs lock:**

* exact trait inventory
* exact domain placement
* explicit deferred trait list

**Recommendation:**
Lock the v1 set before implementation of merge logic, or the merge system will be built against a moving ontology.

---

## Decision 7 — Compiler contract

**Question:** What is the compiled persona payload format, and how is determinism tested?

**Needs lock:**

* payload schema
* token budget target
* compilation phase order
* test invariants

**Recommendation:**
Lock the compiler interface now, even if the serializer implementation is stubbed initially.

---

## Decision 8 — Multi-agent persona propagation

**Question:** What survives delegation to sub-agents?

**Needs lock:**

* default inheritance behavior
* allowed override surface for sub-agents
* whether user-facing tone is ever preserved in internal delegation

**Recommendation:**
Strip interpersonal persona by default; preserve only task-relevant cognitive and output constraints.

---

# 13. Decisions Proposed As Locked In This Draft

The following are proposed as already locked unless explicitly overturned:

1. Persona is an Experience Layer subsystem.
2. Persona does not own reasoning.
3. Persona does not own governance.
4. Persona does not own execution authority.
5. Legacy presets are not part of the new canonical design.
6. Adaptation routes through the Subconscious Process.
7. V1 trait scope must be reduced.
8. Compiler contract is mandatory before implementation is considered complete.

---

# 14. Ratification Checklist

A formal lock can occur once the following are answered in normative language:

* [ ] `EffectivePersonaState` interface defined
* [ ] persona / role / autonomy separation defined
* [ ] governance adapter defined
* [ ] state placement defined
* [ ] reflection-tier adaptation mapping defined
* [ ] v1 trait inventory locked
* [ ] compiler payload schema locked
* [ ] multi-agent propagation rule locked

---

# 15. Immediate Next Step

After ratification of the outstanding decisions, this document should be extended with:

1. normative data structures
2. normative merge pipeline
3. v1 trait definitions
4. compiler payload schema
5. evaluation and regression criteria
