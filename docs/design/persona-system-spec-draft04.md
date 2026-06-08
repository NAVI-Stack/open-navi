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

## 7.10 Normative trait definition table

All v1 traits use a normalized numeric range of **0.0 to 1.0**.
Unless otherwise specified, 0.5 is neutral.

| Trait                        | Domain     | Default | Adaptivity      | Merge class       | Low / High meaning                   | Dependency partners                                   |
| ---------------------------- | ---------- | ------: | --------------- | ----------------- | ------------------------------------ | ----------------------------------------------------- |
| directness                   | identity   |    0.68 | stable          | range_clamp       | softened / bluntly clear             | warmth, challenge_intensity                           |
| warmth                       | identity   |    0.56 | stable          | weighted_blend    | cool / reassuring                    | directness, supportiveness                            |
| formality                    | identity   |    0.48 | stable          | weighted_blend    | casual / polished                    | familiarity                                           |
| seriousness                  | identity   |    0.72 | stable          | weighted_blend    | light / sober                        | humor_playfulness                                     |
| analytic_depth               | cognitive  |    0.68 | stable          | weighted_blend    | lightweight / exhaustive             | structure_level                                       |
| skepticism                   | cognitive  |    0.72 | stable          | weighted_blend    | accepting / questioning              | challenge_intensity, decisiveness                     |
| decisiveness                 | cognitive  |    0.58 | stable          | weighted_blend    | preserve ambiguity / converge        | recommendation_directiveness, clarification_threshold |
| verbosity                    | expression |    0.52 | highly_adaptive | priority_override | terse / elaborate                    | structure_level                                       |
| conversationality            | expression |    0.44 | semi_adaptive   | weighted_blend    | utilitarian / fluid                  | formality, familiarity                                |
| humor_playfulness            | expression |    0.18 | highly_adaptive | gated_activation  | sober / playful                      | seriousness, emotional_attunement                     |
| initiative_style             | behavioral |    0.46 | semi_adaptive   | priority_override | reactive / proactive guidance        | clarification_threshold, recommendation_directiveness |
| clarification_threshold      | behavioral |    0.44 | highly_adaptive | priority_override | ask sooner / infer more readily      | decisiveness, initiative_style                        |
| challenge_intensity          | behavioral |    0.52 | semi_adaptive   | range_clamp       | gentle / forceful pushback           | warmth, supportiveness, skepticism                    |
| emotional_attunement         | relational |    0.62 | semi_adaptive   | weighted_blend    | content-first / affect-sensitive     | supportiveness, humor_playfulness                     |
| supportiveness               | relational |    0.58 | semi_adaptive   | weighted_blend    | neutral / encouraging                | warmth, challenge_intensity                           |
| familiarity                  | relational |    0.20 | semi_adaptive   | weighted_blend    | distant / familiar                   | formality, conversationality                          |
| structure_level              | output     |    0.62 | highly_adaptive | weighted_blend    | freeform / explicitly structured     | analytic_depth, verbosity                             |
| recommendation_directiveness | output     |    0.56 | semi_adaptive   | weighted_blend    | broad options / clear recommendation | decisiveness, initiative_style, ambiguity             |

## 7.11 Trait metadata rules

The following metadata fields are normative for every trait definition record:

* `name`
* `domain`
* `value_range`
* `default_value`
* `adaptivity_class`
* `merge_class`
* `dependency_partners`
* `behavioral_description_low`
* `behavioral_description_high`

An implementation may add engineering metadata, but it may not omit these fields.

# 8. Merge System

## 8.1 Merge classes

V1 retains four merge classes:

1. Weighted Blend
2. Priority Override
3. Range Clamp
4. Gated Activation

## 8.2 Source precedence

The following remains the conflict resolution order:

1. Governance bounds
2. Explicit user instruction
3. Task-critical context
4. Situational overlays
5. Relationship profile / persistent user adaptation
6. Saved persona modules
7. Core identity

## 8.3 Formal merge pipeline

The merge engine operates in four normative passes:

1. **Gate**
2. **Blend / Override**
3. **Clamp**
4. **Dependency Correct**

This order is mandatory.

### Pass 1 — Gate

Before any trait blending occurs, the merge engine excludes or suppresses incompatible contributions.

Examples:

* humor inputs are gated off in high-stakes or emotionally sensitive contexts
* high challenge contributions are gated off when governance or context disallows them
* initiative-style contributions are capped or excluded when owner interruption preferences or governance bounds require restraint

A gated contribution must not participate in weighted averaging.

### Pass 2 — Blend / Override

After incompatible inputs are removed, the engine computes resolved trait values using the assigned merge behavior for each trait.

* weighted-blend traits are combined across surviving sources
* priority-override traits resolve according to precedence rules
* gated traits remain absent or zeroed if disallowed

### Pass 3 — Clamp

After trait values are resolved, the engine applies bounded ranges derived from governance, context, and explicit turn constraints.

Examples:

* directness may be capped in emotionally sensitive contexts
* challenge intensity may be capped in high-risk or high-emotion interactions
* recommendation directiveness may be bounded downward when ambiguity is high

### Pass 4 — Dependency Correct

After clamp application, the engine applies dependency correction rules to eliminate incoherent combinations.

Examples:

* high directness + low warmth may be softened
* high challenge intensity may require higher supportiveness or strong contextual justification
* high structure level may be raised further when task complexity is high

This is the final pass before the engine emits `EffectivePersonaState`.

## 8.4 Merge engine output

The merge engine emits exactly one canonical object: `EffectivePersonaState`.

It does not emit separate cognitive and presentation payloads.
Those partitions are compiler responsibilities.

## 8.5 Required post-merge invariants

Before emitting `EffectivePersonaState`, the merge engine must guarantee:

1. every v1 trait has exactly one resolved value
2. every gate, clamp, and dependency correction is recorded for audit
3. no trait value implies execution authority or governance bypass
4. explicit turn overrides have already been applied or recorded as unresolved warnings
5. the output state is internally consistent enough for deterministic compilation

## 8.6 Pseudocode

```ts
function computeEffectivePersonaState(inputs): EffectivePersonaState {
  const normalized = normalizeInputs(inputs);
  const gated = applyGates(normalized);
  const resolved = blendAndOverride(gated);
  const clamped = applyClamps(resolved);
  const corrected = applyDependencyCorrections(clamped);
  return emitEffectivePersonaState(corrected);
}
```

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

The compiler input is a **single unified merge-engine output object**: `EffectivePersonaState`.

The merge engine must hand the compiler a fully resolved persona state.
The compiler must not re-run merge logic, re-derive trait values from source inputs, or independently recompute persona composition.

Partitioning into cognitive modulation, expression policy, behavior policy, and output contract happens **inside the compiler**, not before it.

### Normative shape

```ts
interface EffectivePersonaState {
  schema_version: string;
  state_id: string;
  generated_at: string;
  merge_engine_version: string;

  role_context: {
    active_role: "Character" | "Assistant" | "Coder";
    task_archetype: string | null;
    delegation_mode: "none" | "primary" | "subagent";
  };

  resolved_traits: Record<string, {
    value: number;
    domain:
      | "identity"
      | "cognitive"
      | "expression"
      | "behavioral"
      | "relational"
      | "output";
    merge_class: "weighted_blend" | "priority_override" | "range_clamp" | "gated_activation";
    adaptivity_class: "stable" | "semi_adaptive" | "highly_adaptive";
    gated_off: boolean;
    clamp_applied: boolean;
    clamp_range?: { min: number; max: number };
    dependency_corrections_applied: string[];
  }>;

  governance_trace: {
    allow_humor: boolean;
    allow_high_challenge: boolean;
    allow_high_directness: boolean;
    max_initiative_style: number;
    max_recommendation_directiveness: number;
    high_stakes_context: boolean;
    emotionally_sensitive_context: boolean;
    requires_neutrality: boolean;
    reasons: string[];
  };

  live_context_trace: {
    ambiguity_level: number;
    urgency_level: number;
    emotional_intensity: number;
    stakes_level: number;
    complexity_level: number;
    confidence_level: number;
  };

  explicit_turn_overrides: {
    must_be_brief: boolean;
    must_ask_clarifying_question: boolean;
    must_not_use_humor: boolean;
    must_be_highly_structured: boolean;
    user_requested_tone_shift: string | null;
  };

  output_preferences: {
    preferred_length: "short" | "medium" | "long" | null;
    preferred_format: "freeform" | "structured" | "stepwise" | "executive" | null;
    summary_first: boolean | null;
  };

  audit: {
    source_precedence_order: string[];
    gated_sources: string[];
    warnings: string[];
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

# 12. Cold-Start Behavior

## 12.1 Purpose

The persona system must define safe, coherent behavior for the first interactions with a user before meaningful relationship adaptation exists.

## 12.2 Cold-start rule

Until the relationship profile reaches the minimum confidence threshold, the merge engine operates in **cold-start mode**.

Cold-start mode means:

* no user-specific inferred relationship calibration is trusted beyond weak hints
* stable identity defaults dominate
* highly adaptive traits remain conservative unless explicitly instructed otherwise
* adaptation is allowed, but confidence grows gradually and is rate-limited

## 12.3 Cold-start defaults

The following defaults are normative during cold-start unless explicitly overridden by the user or context:

* humor_playfulness remains low
* familiarity remains low
* clarification_threshold remains slightly ask-leaning
* challenge_intensity remains moderate rather than forceful
* recommendation_directiveness remains moderate
* structure_level remains moderately high for clarity

## 12.4 Cold-start adaptation rule

During the early interaction window, inferred preference updates must use reduced learning rates until repeated evidence is observed.

The purpose is to avoid overfitting to one early interaction.

## 12.5 Exit condition

Cold-start mode ends when the relationship profile crosses the minimum confidence threshold defined by the adaptation subsystem.

The exact threshold is an implementation constant, but the existence of a threshold is normative.

# 13. Observability and User Transparency

## 13.1 Requirement

Persona state must be inspectable.

The user must be able to understand, at an appropriate level of abstraction:

* which persona modules are active
* what the current relationship calibration looks like
* which major adaptations have been applied
* why a meaningful adaptation occurred

## 13.2 User-facing representation

User-facing persona inspection must not expose raw trait floats by default.

Instead, it should present:

* active modules and overlays in human-readable form
* summary descriptions of current style tendencies
* notable adaptations and the evidence class behind them
* any owner-set constraints affecting persona behavior

Raw numeric values may be available in developer or advanced inspection surfaces.

## 13.3 Proposal rule for significant adaptation

If persona adaptation crosses a significance threshold and would materially change the user experience, the change must be surfaced through the existing Proposal mechanism rather than silently committed as explicit user state.

This keeps persona transparency aligned with the broader NAVI architecture.

## 13.4 Auditability requirement

The system must retain enough audit information to answer questions such as:

* why humor was suppressed
* why pushback was softened
* why output became more structured
* why the system began favoring shorter or longer responses

# 14. Locked Decisions

The following decisions are locked by this specification:

1. Persona is an Experience Layer subsystem.
2. Persona does not own reasoning.
3. Persona does not own governance.
4. Persona does not own execution authority.
5. Role and persona are separate systems.
6. The merge engine emits one unified `EffectivePersonaState`.
7. The compiler partitions that state into control surfaces.
8. The compiler output is a structured intermediate representation, not raw prompt prose.
9. Provider-specific prompt text is produced by a separate serializer.
10. Governance is the only canonical constraint authority.
11. Adaptation routes through the Subconscious Process.
12. V1 trait scope is locked to 18 traits.
13. Legacy persona presets are not part of the new canonical design.
14. Multi-agent delegation strips interpersonal persona by default and preserves only task-relevant modulation.
15. Cold-start behavior is normative and must be implemented.
16. Persona state must be inspectable in user-appropriate form.

# 15. Remaining Normative Work Before Implementation

The architecture is locked.
What remains is formal definition work, not additional design exploration.

The remaining required work before decomposition is:

1. finalize the `EffectivePersonaState` field list and any enum names
2. finalize the compiler payload field list and any enum names
3. finalize the implementation constants for adaptation thresholds, cold-start thresholds, and budget trimming
4. define the exact serializer targets per runtime backend
5. define the evaluation and regression suite

These are normative completion tasks, not open architectural questions.

# 16. Consolidated Ratification Status

The following ratification items are resolved:

* [x] Experience Layer interface direction defined
* [x] persona / role / autonomy separation defined
* [x] governance adapter model defined
* [x] state placement defined
* [x] reflection-tier adaptation mapping defined
* [x] v1 trait inventory locked
* [x] compiler payload schema locked in principle
* [x] multi-agent propagation rule locked
* [x] cold-start behavior defined in principle
* [x] observability requirement defined in principle

The following still require normative completion detail:

* [ ] final `EffectivePersonaState` field names and constants
* [ ] final `CompiledPersonaPayload` field names and constants
* [ ] serializer-specific target mappings
* [ ] evaluation and regression definitions

# 17. Immediate Next Step

The next document pass should be a consolidation/finalization pass that:

1. normalizes naming across all sections
2. freezes enum and field names
3. moves any remaining provisional wording into normative language
4. adds the evaluation and regression section
5. prepares the document for task decomposition

After that pass, the work can be decomposed into implementation streams for:

* merge engine
* governance bounds adapter
* compiler and serializers
* Subconscious adaptation hooks
* configuration storage and module management
* observability and transparency surfaces
