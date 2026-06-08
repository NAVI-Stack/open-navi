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
* clear separation from role and autonomy

It is not to ship a maximal trait ontology.

## 7.2 Lock decision

V1 is locked to **18 traits** across the six domains.

This is the canonical v1 inventory.
Anything not listed here is deferred.

## 7.3 Domain inventory

### Identity domain

Identity traits are long-lived baseline traits. They define who NAVI feels like across contexts.

1. **directness**

   * How plainly NAVI states conclusions.
   * Lower values increase softening and indirection.
   * Higher values increase blunt clarity.

2. **warmth**

   * Interpersonal warmth and affective softness.
   * Lower values are cooler and more detached.
   * Higher values are more reassuring and human-centered.

3. **formality**

   * Degree of social/professional formality.
   * Lower values are casual and conversational.
   * Higher values are polished and formal.

4. **seriousness**

   * Baseline gravity vs levity.
   * Lower values allow more lightness.
   * Higher values preserve a more sober posture.

### Cognitive domain

Cognitive traits modulate reasoning posture, not reasoning ownership.
They may bias how NAVI approaches analysis, but do not change the Cognitive Layer’s authority.

5. **analytic_depth**

   * How much decomposition and internal structure NAVI prefers before answering.
   * Lower values favor lightweight reasoning and quicker synthesis.
   * Higher values favor deeper breakdown and more exhaustive analysis.

6. **skepticism**

   * How readily NAVI questions assumptions, claims, and incomplete evidence.
   * Lower values accept user framing more readily.
   * Higher values challenge premises more aggressively.

7. **decisiveness**

   * How readily NAVI converges on a recommendation under uncertainty.
   * Lower values preserve ambiguity and hedge longer.
   * Higher values commit sooner when enough evidence exists.

### Expression domain

Expression traits shape how answers sound and flow.

8. **verbosity**

   * Overall output length and elaboration.
   * Lower values bias concise answers.
   * Higher values bias detailed explanation.

9. **conversationality**

   * Degree of natural, back-and-forth spoken feel.
   * Lower values are more compressed and utilitarian.
   * Higher values are more fluid and chat-like.

10. **humor_playfulness**

* Light humor, wit, and playfulness in delivery.
* Lower values suppress playfulness.
* Higher values allow more levity when context permits.

### Behavioral domain

Behavioral traits affect interaction posture only.
They must not alter autonomy, permissions, or execution thresholds.

11. **initiative_style**

* How readily NAVI proactively adds useful next steps, follow-ons, or suggestions in conversation.
* Lower values stay tightly reactive.
* Higher values surface more proactive guidance.

12. **clarification_threshold**

* How much ambiguity NAVI tolerates before asking a clarifying question.
* Lower values ask sooner.
* Higher values infer and proceed more readily.

13. **challenge_intensity**

* How strongly NAVI pushes back on weak reasoning, risky assumptions, or bad plans.
* Lower values are gentler and less confrontational.
* Higher values are more forceful and corrective.

### Relational domain

Relational traits represent interpersonal calibration in the owner relationship.
These are the primary adaptation surfaces for inferred user-specific tuning.

14. **emotional_attunement**

* Sensitivity to emotional context and interpersonal impact.
* Lower values prioritize content over emotional read.
* Higher values respond more carefully to affect and subtext.

15. **supportiveness**

* Degree of encouragement, reassurance, and affiliative framing.
* Lower values prioritize neutrality and task focus.
* Higher values actively reinforce and support.

16. **familiarity**

* Degree of interpersonal closeness and relaxed social posture.
* Lower values preserve more distance.
* Higher values allow more friendly, familiar interaction.

### Output domain

Output traits shape packaging and recommendation format.
These are presentation-level controls, not cognition or autonomy controls.

17. **structure_level**

* Degree of explicit formatting, organization, and stepwise packaging.
* Lower values allow freeform responses.
* Higher values produce more structured output.

18. **recommendation_directiveness**

* How narrowly NAVI converges recommendations for the user.
* Lower values present broader option sets.
* Higher values recommend a clearer course of action.

## 7.4 Traits explicitly rejected from v1

The following are deferred or rejected for v1 because they are redundant, too model-dependent, or belong elsewhere in the architecture:

* `autonomy_preference`
* `intervention_tendency` as an authority-like control
* `rhetorical_density`
* `rhythm_sharpness`
* `respect_signaling`
* `friction_tolerance`
* `context_carryforward`
* `escalation_behavior` as a persona trait
* `memory_promotion_sensitivity` as a persona trait
* `plugin_invocation_authority` as a persona trait

## 7.5 Boundary rules

The following rules are normative:

* No trait may alter execution authority.
* No trait may alter governance outcomes.
* No trait may alter proposal-required categories.
* No trait may redefine role selection.
* No trait may duplicate an autonomy setting under a softer name.

If a proposed trait changes whether NAVI acts rather than how NAVI frames or delivers, it is not a persona trait.

## 7.6 Adaptivity classes

Traits have three adaptivity classes.

### Stable

Rarely changed. Owner-edited or explicitly selected.

* directness
* warmth
* formality
* seriousness
* analytic_depth
* skepticism
* decisiveness

### Semi-adaptive

May shift gradually with repeated evidence, but should remain bounded by core identity.

* conversationality
* initiative_style
* challenge_intensity
* emotional_attunement
* supportiveness
* familiarity
* recommendation_directiveness

### Highly adaptive

Expected to vary by user and task context more readily.

* verbosity
* humor_playfulness
* clarification_threshold
* structure_level

## 7.7 Default merge classes by trait

Each trait has a default merge behavior.

### Weighted Blend

* warmth
* formality
* seriousness
* analytic_depth
* skepticism
* decisiveness
* conversationality
* emotional_attunement
* supportiveness
* familiarity
* structure_level
* recommendation_directiveness

### Priority Override

* verbosity
* initiative_style
* clarification_threshold

### Range Clamp

* directness
* challenge_intensity
* humor_playfulness

### Gated Activation

* humor_playfulness
* challenge_intensity
* initiative_style

A trait may use more than one mechanism in sequence. For example, humor may be blended, then clamped, then gated off entirely in high-stakes contexts.

## 7.8 Dependency correction rules required for v1

The following correction rules are mandatory in v1:

1. High `directness` + low `warmth` must be softened unless the context explicitly rewards sharp candor.
2. High `challenge_intensity` requires at least moderate `supportiveness` or high-confidence justification from context.
3. High `humor_playfulness` is clamped or gated off in high-stakes, emotionally sensitive, or corrective contexts.
4. High `initiative_style` cannot override low-autonomy governance or owner-set interruption preferences.
5. High `recommendation_directiveness` must relax when ambiguity is high and evidence is weak.
6. High `structure_level` should increase when task complexity is high, even if baseline conversationality is also high.
7. Low `clarification_threshold` should not force unnecessary questioning when the task is reversible and ambiguity is low.

## 7.9 Why this set is sufficient for v1

This inventory is intentionally small but complete enough to validate:

* stable identity
* cognitive modulation without cognition ownership
* expressive variability
* interaction posture
* user-specific relational adaptation
* output packaging
* clamp and gate behavior under governance and context

It is therefore sufficient for v1.

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
The canonical compiler output is a **structured intermediate representation**.
Provider-specific prompt text is a downstream serialization target, not the source of truth.

## 9.2 Compiler role in the runtime

The compiler sits between persona computation and model invocation.

It consumes the `EffectivePersonaState` produced by the Experience Layer and emits a `CompiledPersonaPayload`.

That payload is then serialized into the model-specific control surface used by the active runtime.
Examples may include:

* system/developer prompt fragments
* structured metadata blocks
* tool/runtime policy sidecars
* output-style directives

The persona compiler does not call models directly.
It produces control payloads only.

## 9.3 Design goals

The compiler must satisfy the following goals:

1. **Determinism**

   * identical input state must produce identical compiled payload

2. **Layer separation**

   * cognitive modulation must remain separate from expression and output packaging

3. **Provider portability**

   * the canonical payload must not depend on one vendor’s prompt format

4. **Budget discipline**

   * compiled output must remain within a bounded token budget

5. **Auditability**

   * the compiler output must be inspectable, diffable, and attributable to source state

6. **Graceful degradation**

   * if budget trimming occurs, the lowest-priority presentation details are removed first

## 9.4 Compiler input schema

The compiler input is `EffectivePersonaState`.

### Normative shape

```ts
interface EffectivePersonaState {
  schema_version: string;
  state_id: string;
  generated_at: string;

  role_context: {
    active_role: "Character" | "Assistant" | "Coder";
    task_archetype: string | null;
    delegation_mode: "none" | "primary" | "subagent";
  };

  governance_bounds: {
    allow_humor: boolean;
    allow_high_challenge: boolean;
    allow_high_directness: boolean;
    max_initiative_style: number;
    max_recommendation_directiveness: number;
    high_stakes_context: boolean;
    emotionally_sensitive_context: boolean;
    requires_neutrality: boolean;
  };

  active_traits: Record<string, {
    value: number;
    source_mix: Array<{
      source_type:
        | "core_identity"
        | "persona_module"
        | "relationship_profile"
        | "situational_overlay"
        | "live_context"
        | "explicit_user_instruction";
      weight: number;
    }>;
    merge_class: "weighted_blend" | "priority_override" | "range_clamp" | "gated_activation";
    adaptivity_class: "stable" | "semi_adaptive" | "highly_adaptive";
    gated_off: boolean;
    clamp_range?: { min: number; max: number };
  }>;

  derived_context: {
    ambiguity_level: number;
    urgency_level: number;
    emotional_intensity: number;
    complexity_level: number;
    confidence_level: number;
  };

  output_preferences: {
    preferred_length: "short" | "medium" | "long" | null;
    preferred_format: "freeform" | "structured" | "stepwise" | "executive" | null;
    summary_first: boolean | null;
  };

  explicit_turn_constraints: {
    must_be_brief: boolean;
    must_ask_clarifying_question: boolean;
    must_not_use_humor: boolean;
    must_be_highly_structured: boolean;
  };
}
```

## 9.5 Compiler output schema

The canonical compiler output is `CompiledPersonaPayload`.

### Normative shape

```ts
interface CompiledPersonaPayload {
  schema_version: string;
  payload_id: string;
  compiled_at: string;
  source_state_id: string;
  compiler_version: string;
  source_hash: string;

  budget: {
    target_tokens: number;
    hard_max_tokens: number;
    estimated_tokens: number;
    trim_level: 0 | 1 | 2 | 3;
  };

  role_adapter: {
    active_role: "Character" | "Assistant" | "Coder";
    task_archetype: string | null;
    delegation_mode: "none" | "primary" | "subagent";
  };

  cognitive_modulation: {
    analytic_depth: number;
    skepticism: number;
    decisiveness: number;
    reasoning_posture: "lightweight" | "balanced" | "deep";
    uncertainty_posture: "preserve_ambiguity" | "balanced" | "converge_when_ready";
    challenge_posture: "gentle" | "balanced" | "forceful";
    notes: string[];
  };

  expression_policy: {
    directness: number;
    warmth: number;
    formality: number;
    seriousness: number;
    verbosity: number;
    conversationality: number;
    humor_playfulness: number;
    style_flags: Array<
      | "concise"
      | "detailed"
      | "plainspoken"
      | "polished"
      | "friendly"
      | "reserved"
      | "sober"
      | "playful">
    ;
    notes: string[];
  };

  behavior_policy: {
    initiative_style: number;
    clarification_threshold: number;
    challenge_intensity: number;
    emotional_attunement: number;
    supportiveness: number;
    familiarity: number;
    recommendation_directiveness: number;
    ask_vs_infer_mode: "ask_early" | "balanced" | "infer_when_safe";
    recommendation_mode: "present_options" | "recommend_with_options" | "recommend_clearly";
    interpersonal_mode: "neutral" | "supportive" | "high_attunement";
    notes: string[];
  };

  output_contract: {
    structure_level: number;
    response_shape:
      | "freeform"
      | "structured"
      | "stepwise"
      | "executive_summary"
      | "analysis_then_answer";
    summary_first: boolean;
    preferred_length: "short" | "medium" | "long";
    formatting_rules: string[];
    prohibited_patterns: string[];
    notes: string[];
  };

  gates_and_clamps: {
    humor_allowed: boolean;
    challenge_cap: number;
    directness_cap: number;
    initiative_cap: number;
    recommendation_directiveness_cap: number;
    reasons: string[];
  };

  serialization_hints: {
    priority_order: Array<
      | "cognitive_modulation"
      | "behavior_policy"
      | "expression_policy"
      | "output_contract">;
    safe_to_trim_first: Array<
      | "style_flags"
      | "expression_notes"
      | "behavior_notes"
      | "output_notes"
      | "formatting_rules_noncritical">;
    preserve_verbatim: Array<
      | "must_not_use_humor"
      | "must_be_brief"
      | "must_ask_clarifying_question"
      | "must_be_highly_structured">;
  };

  audit: {
    dependency_corrections_applied: string[];
    gated_traits: string[];
    trimmed_fields: string[];
    warnings: string[];
  };
}
```

## 9.6 Section responsibilities

### cognitive_modulation

This section influences how the model approaches analysis.
It may steer depth, skepticism, convergence behavior, and caution posture.
It must not contain presentation-only details unless they materially affect reasoning behavior.

### expression_policy

This section controls speaking style and surface tone.
It must not contain instructions about permission, tool use, or execution authority.

### behavior_policy

This section controls conversational behavior and interaction posture.
It may influence whether NAVI asks sooner or infers more readily, how strongly it challenges weak ideas, and how supportive or emotionally attuned it sounds.
It must not modify whether the system is allowed to act.

### output_contract

This section controls final packaging.
It governs response shape, structure, summary order, and formatting rules.
It should contain only delivery instructions, not reasoning instructions.

## 9.7 Compilation phases

The compiler operates in six ordered phases.

### Phase 1 — Normalize

Normalize all trait values, role data, bounds, and turn constraints into canonical ranges and enums.

### Phase 2 — Apply gates and clamps

Apply governance-derived caps, context gates, and dependency corrections.

This is the last phase in which the effective trait values themselves may change.

### Phase 3 — Derive semantic modes

Derive higher-level modes from the normalized trait state.
Examples:

* `reasoning_posture`
* `uncertainty_posture`
* `ask_vs_infer_mode`
* `recommendation_mode`
* `response_shape`

### Phase 4 — Partition by target surface

Partition the state into:

* cognitive modulation
* expression policy
* behavior policy
* output contract

No field may appear in two sections unless explicitly listed as a sanctioned dual-surface field.

### Phase 5 — Add audit metadata

Attach provenance, corrections, gates, warnings, and serialization priority metadata.

### Phase 6 — Budget trim

Trim lowest-priority descriptive details if the payload exceeds budget.
Trim order must follow `serialization_hints.safe_to_trim_first`.
Normative behavioral prohibitions must never be trimmed.

## 9.8 Budget policy

### Default budget

* target serialized size: **220 tokens**
* preferred upper bound: **320 tokens**
* hard maximum: **480 tokens**

These limits apply to the provider-specific serialized control payload, not to the structured object in memory.

### Trim levels

* **0** = no trimming
* **1** = remove noncritical notes and stylistic labels
* **2** = compress formatting rules and collapse redundant descriptors
* **3** = retain only hard constraints, derived modes, and highest-priority trait signals

If a payload cannot fit within hard maximum at trim level 3, the compiler must emit a warning and fail closed to a minimal safe payload.

## 9.9 Determinism rules

The compiler is deterministic only if all of the following are true:

* same `EffectivePersonaState`
* same compiler version
* same trim policy
* same serializer target

The compiler must produce:

* stable field order
* stable enum mapping
* stable note ordering
* stable trimming behavior

## 9.10 Serializer contract

The compiler and serializer are separate components.

### Compiler responsibilities

* compute canonical payload
* compute derived modes
* apply gates/clamps/corrections
* attach audit metadata
* enforce budget policy logically

### Serializer responsibilities

* convert payload into provider-specific control text or metadata
* target model-specific instruction channels
* preserve the separation between cognition, behavior, expression, and output

A serializer may reorder or compress wording, but it may not alter payload meaning.

## 9.11 Dual-surface fields

The following fields are allowed to influence more than one section:

* `directness`

  * affects both expression policy and how recommendations are phrased
* `verbosity`

  * affects both expression policy and output contract length choice
* `structure_level`

  * affects both behavior of explanation planning and output packaging
* `challenge_intensity`

  * affects both cognitive challenge posture and behavioral pushback style

These must still be emitted through one canonical source value and referenced, not duplicated independently.

## 9.12 Failure behavior

If compilation fails, the system must fall back to a minimal safe payload.

### Minimal safe payload

The fallback payload must preserve:

* governance-derived prohibitions
* role adapter
* neutral cognitive modulation
* neutral expression policy
* conservative behavior policy
* basic output contract

Persona-specific richness may be lost in fallback mode.
Safety and structural correctness may not.

## 9.13 Example compiled payload (abbreviated)

```json
{
  "schema_version": "1.0",
  "payload_id": "cpp_01",
  "source_state_id": "eps_01",
  "compiler_version": "1.0",
  "budget": {
    "target_tokens": 220,
    "hard_max_tokens": 480,
    "estimated_tokens": 204,
    "trim_level": 0
  },
  "role_adapter": {
    "active_role": "Assistant",
    "task_archetype": "architecture_review",
    "delegation_mode": "none"
  },
  "cognitive_modulation": {
    "analytic_depth": 0.82,
    "skepticism": 0.78,
    "decisiveness": 0.66,
    "reasoning_posture": "deep",
    "uncertainty_posture": "balanced",
    "challenge_posture": "forceful",
    "notes": ["Question assumptions directly when evidence is weak."]
  },
  "expression_policy": {
    "directness": 0.84,
    "warmth": 0.42,
    "formality": 0.58,
    "seriousness": 0.81,
    "verbosity": 0.52,
    "conversationality": 0.37,
    "humor_playfulness": 0.0,
    "style_flags": ["plainspoken", "sober"],
    "notes": ["Be clear, restrained, and unsentimental."]
  },
  "behavior_policy": {
    "initiative_style": 0.60,
    "clarification_threshold": 0.58,
    "challenge_intensity": 0.74,
    "emotional_attunement": 0.55,
    "supportiveness": 0.36,
    "familiarity": 0.22,
    "recommendation_directiveness": 0.71,
    "ask_vs_infer_mode": "balanced",
    "recommendation_mode": "recommend_clearly",
    "interpersonal_mode": "neutral",
    "notes": ["Push back when a proposal is weak, but stay controlled."]
  },
  "output_contract": {
    "structure_level": 0.79,
    "response_shape": "analysis_then_answer",
    "summary_first": false,
    "preferred_length": "medium",
    "formatting_rules": ["Use headers when complexity is high."],
    "prohibited_patterns": ["Do not use humor in high-stakes analysis."],
    "notes": []
  },
  "gates_and_clamps": {
    "humor_allowed": false,
    "challenge_cap": 0.8,
    "directness_cap": 0.9,
    "initiative_cap": 0.65,
    "recommendation_directiveness_cap": 0.8,
    "reasons": ["High-stakes context", "Serious analysis task"]
  },
  "serialization_hints": {
    "priority_order": ["cognitive_modulation", "behavior_policy", "expression_policy", "output_contract"],
    "safe_to_trim_first": ["style_flags", "expression_notes", "behavior_notes", "output_notes"],
    "preserve_verbatim": []
  },
  "audit": {
    "dependency_corrections_applied": ["humor gated off", "directness/warmth pair softened"],
    "gated_traits": ["humor_playfulness"],
    "trimmed_fields": [],
    "warnings": []
  }
}
```

## 9.14 Test invariants

The following compiler tests are mandatory:

1. **Determinism test**

   * same input must produce byte-stable canonical JSON output

2. **Boundary test**

   * no output field may imply execution authority or permission changes

3. **Budget test**

   * trim behavior must be predictable and ordered

4. **Partition test**

   * presentation-only instructions must not appear in `cognitive_modulation`

5. **Governance test**

   * disallowed humor/challenge/directness states must serialize safely

6. **Fallback test**

   * failure path must emit minimal safe payload

## 9.15 Decision proposed as locked

The following are proposed as locked in this draft:

1. The compiler output is a structured intermediate representation, not raw prompt prose.
2. Provider-specific prompt text is generated by a separate serializer.
3. The compiler output is partitioned into `cognitive_modulation`, `expression_policy`, `behavior_policy`, and `output_contract`.
4. Budget policy and trim behavior are normative parts of the compiler contract.
5. Audit metadata is mandatory.

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
