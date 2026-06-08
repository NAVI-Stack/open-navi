# NAVI Persona System (Experience Layer) Specification

> [!NOTE]
> Part of the [NAVI Systems Map](../architecture/navi-systems-map.md).


## Status

Ratified.

## Purpose

This document is the single source of truth for the NAVI **Persona System**, technically referred to as the **Experience Layer**. It formalizes the architectural position, runtime model, trait system, merge engine, compiler contract, adaptation model, storage placement, and evaluation requirements.

The previous persona system (consisting of fixed presets like *Buddy*, *Jarvis*, and *Orchestrator*) has been entirely removed and replaced by this new, modular architecture. This system focuses on dynamic traits and experience modules rather than static character selection.

All design decisions referenced in this document are locked unless explicitly marked as implementation-deferred constants.

---

# 1. Scope

This specification defines:

* where the persona system lives in NAVI's architecture
* what persona is and is not allowed to control
* the runtime objects and state surfaces involved in persona computation
* the v1 trait inventory, merge pipeline, and compiler contract
* the adaptation model and its integration with the Subconscious Process
* cold-start behavior, observability, and multi-agent propagation rules
* the v1 serializer target and evaluation suite

This specification does **not** define:

* the final preset library (deferred until core architecture is validated)
* implementation-language internals
* UI/UX for persona editing surfaces
* persona DSL or authoring format (downstream of this spec)

# 2. Architectural Position

## 2.1 Canonical home

The persona system is technically implemented as the **Experience Layer subsystem**. While "Persona System" remains the human-facing name for this capability, "Experience Layer" describes its architectural role in modulating NAVI's cognitive output.

Persona does not live in the Capability Layer.
Persona does not live in the World Model as a standalone entity or execution engine.
Persona does not replace or subsume the Cognitive Layer.

Persona is responsible for:

* modulating how NAVI reasons within permitted bounds
* shaping how NAVI communicates
* shaping how NAVI behaves interpersonally
* packaging output for delivery

Persona does **not** own:

* core reasoning
* permissions
* governance
* execution authority
* direct world mutation

## 2.2 Interface contract

The Experience Layer computes an `EffectivePersonaState` from stable identity, overlays, user-specific adaptation, and live context.

That state feeds two downstream surfaces:

1. **Cognitive modulation input** — biases attention and framing during the Decide step of the Conscious Process. Must not alter the Cognitive Layer's ownership of reasoning.

2. **Presentation and output contract** — controls delivery style, framing, packaging, and interpersonal tone. Applied after Cognitive output is produced.

The `EffectivePersonaState` is one unified object. The split into cognitive modulation and presentation surfaces happens inside the compiler, not at the merge boundary.

---

# 3. Separation Rules

## 3.1 Role vs Persona

Role and persona are separate systems.

* **Role** determines what kind of work NAVI is doing: Character, Assistant, or Coder.
* **Persona** determines how NAVI behaves while doing that work.

Persona must not redefine, collapse, or select roles.

## 3.2 Persona vs Autonomy

Persona must never control execution authority.

Persona may influence:

* recommendation style and assertiveness of framing
* whether NAVI presents one option or many
* how strongly NAVI pushes for a course of action

Persona may not influence:

* whether execution is permitted
* autonomous execution thresholds
* proposal-required categories
* governance hard floors

Any trait that modifies autonomous authority is out of scope for persona and belongs to the Autonomy Model or Governance.

## 3.3 Persona vs Governance

Persona does not own a separate constraint kernel. The canonical constraint system is NAVI Governance.

Persona runtime logic may:

* read governance outcomes
* adapt expression within governance bounds
* apply context-sensitive clamps and gates derived from governance state

Persona runtime logic may not:

* create a parallel policy engine
* bypass governance
* redefine system-tier or owner-tier constraints

## 3.4 Trait boundary rule

A proposed trait is invalid if it changes:

* execution authority
* governance outcomes
* proposal-required categories
* role selection
* autonomy thresholds under a softer name

If a trait changes whether NAVI acts rather than how NAVI frames or delivers, it is not a persona trait.

---

# 4. Runtime Model

## 4.1 Effective persona computation

At runtime, NAVI does not have a single static persona. It computes an effective state from multiple sources.

### Source precedence order

All persona sources are evaluated in a single canonical precedence order. This order is used for conflict resolution in priority-override traits, for clamp authority, and for determining which source wins when sources are incompatible.

1. **Governance bounds** — absolute; never overridden by persona sources
2. **Explicit turn overrides** — direct user instructions in the current turn
3. **Live context** — includes task-critical context, urgency, emotional sensitivity, stakes, ambiguity, and complexity
4. **Situational overlays** — active temporary modes (critic, coach, etc.)
5. **Relationship profile** — persistent user-specific adaptation
6. **Persona modules** — saved reusable trait bundles
7. **Core identity** — stable baseline defaults

This order is referenced throughout the spec. Sections 4, 8, and 9 all use this single canonical ordering.

### Composition formula

```
EffectivePersonaState =
  GovernanceBounds
  → ExplicitTurnOverrides
  + LiveContext
  + SituationalOverlays
  + RelationshipProfile
  + PersonaModules
  + CoreIdentity
```

This is conflict-resolution order, not pure override order. Lower-precedence sources still contribute via blending; higher-precedence sources win when incompatible.

## 4.2 Runtime objects

1. **Governance Bounds Adapter** — Reads permission, policy, configuration, priority, and risk outcomes from the Governance system. Exposes persona-relevant bounds, gates, and clamp ranges. Does not maintain independent constraint state.

2. **Core Identity** — Stable baseline identity traits. Low-adaptivity, long-lived. Stored as explicit Configuration.

3. **Role Adapter** — Imports the active role context (Character / Assistant / Coder). Informs persona composition without being overwritten by persona.

4. **Persona Modules** — Reusable trait bundles and overlays. May be system-defined, owner-authored, or inferred-then-promoted. Scoped as global, conversation, task, or turn.

5. **Relationship Profile** — User-specific relational calibration. Derived at query time from the owner Contact record, interaction History, and inferred Configuration. Not stored as a monolithic entity.

6. **Live Context Modulator** — Applies momentary contextual adjustments based on urgency, emotional sensitivity, stakes, ambiguity, and task shape.

7. **Output Contract** — Compiled delivery policy for structure, style, and response packaging. Transient runtime state.

---

# 5. State Placement

## 5.1 Placement rules

Persona state must use existing NAVI state classes and entity patterns from the World Model and Configuration model.

### Stored as explicit Configuration

* core identity trait defaults
* owner-authored persona settings
* owner-selected persona modules
* owner-authored output preferences

### Stored as inferred Configuration

Updated by the Subconscious Process within permitted scope:

* bluntness tolerance estimate
* humor tolerance estimate
* collaboration preference
* preferred response density
* directness preference
* familiarity calibration

### Derived at query time

Never stored as monolithic entities:

* relationship profile
* composite user-specific persona calibration
* `EffectivePersonaState`

### Transient runtime state

Runtime-only, not persisted:

* situational overlays
* live context adjustments
* turn-local formatting overrides
* `CompiledPersonaPayload`
* serialized prompt fragments

## 5.2 Storage rule

The persona system must not invent a separate durable storage concept outside the World Model and Configuration model. Persona modules, trait bundles, and overlay definitions are stored as Configuration entities. Preference signals are captured through the Subconscious reflection pipeline.

### Derived vs persisted distinction

`EffectivePersonaState` is always derived at query time and is canonical only for the current turn. It is never stored as a persistent entity.

However, selected audit snapshots of the effective state may be persisted to History under the retention rules below. These snapshots are historical records, not canonical state — they cannot be loaded as the current effective persona.

## 5.3 Audit snapshot retention policy

Effective persona snapshots are written to History only under the following conditions:

1. **Material delta** — The resolved trait state differs from the previous snapshot by more than `0.10` cumulative absolute delta across all traits. This prevents writing identical or near-identical snapshots on every turn.
2. **Proposal-worthy adaptation** — A Consolidation or Deep Reflection cycle produces an adaptation that crosses a proposal significance threshold.
3. **Owner-initiated inspection** — The owner explicitly requests persona state through the debugger tool.
4. **Session boundary** — At most one snapshot per session start and session end, to provide bookend audit records.

Snapshots that do not meet any of these conditions are not persisted. The implementation may apply additional bounded sampling (e.g., at most one snapshot per N turns even if deltas are material) to control storage volume.

## 5.4 Storage schema reconciliation

The pre-spec storage design document defined seven object types. These map to the locked architecture as follows:

| Pre-spec object | Locked placement |
| --- | --- |
| `PersonaDefinition` | Configuration: persona module (full bundle) |
| `TraitBundle` | Configuration: persona module (micro-module) |
| `OverlayDefinition` | Configuration: persona module (scoped overlay) |
| `EffectivePersonaSnapshot` | History: audit record of computed persona state |
| `UserPersonaProfile` | Inferred Configuration: relational trait estimates |
| `PreferenceSignal` | Subconscious: Shallow Reflection signal capture |
| `PersonaLibraryIndex` | Configuration: module registry |

All storage objects must use the locked v1 trait names and domains defined in Section 7.

---

# 6. Adaptation Model

## 6.1 Adaptation owner

Persona adaptation is owned by the **Subconscious Process**. There is no parallel persona daemon or independent adaptation loop.

## 6.2 Reflection-tier mapping

### Shallow Reflections

Capture immediate interaction signals: user asked for brevity, user reacted badly to blunt phrasing, user rewarded structured output.

Allowed effects: signal capture, temporary confidence adjustments, reflection payload generation. No persistent state mutation.

### Consolidation

Aggregate repeated signals across sessions.

Allowed effects: update inferred persona-related Configuration, strengthen or weaken relational estimates, decay stale inferred preferences. Must generate a Proposal when a change would affect explicit or owner-set behavior.

### Deep Reflection

Revisit large-scale persona adaptation and owner-visible changes.

Allowed effects: restructure inferred relationship calibration, surface proposed changes that affect explicit behavior, generate Proposals for changes that cross owner-set boundaries. May propose but never directly apply changes to owner-set persona state.

## 6.3 Adaptation rule

No adaptation path may directly mutate owner-set persona state without owner confirmation via the Proposal Queue.

## 6.4 Explicit correction vs inferred adaptation

Persona adaptation follows two distinct paths. They must not be conflated.

### Explicit correction path

When a user directly states a preference ("be more concise," "stop using humor," "be blunter"), the correction takes effect immediately at the turn or session level. Explicit corrections:

* bypass `signal_apply_threshold` and `signal_persist_threshold`
* apply at full signal strength (1.0) for the current session
* are captured as high-confidence signals for long-term inferred Configuration
* still require Proposal confirmation before mutating owner-set stable traits

The merge engine applies explicit corrections through `explicit_turn_overrides` in the current session and records them as explicit signals for Consolidation.

### Inferred adaptation path

When NAVI detects implicit preference patterns (user repeatedly asks for summaries, user skips long answers, user rewards structured output), updates follow the thresholded confidence path:

* subject to `signal_apply_threshold` (3 consistent signals before adjustment)
* subject to `signal_persist_threshold` (5 consistent signals before long-term persistence)
* scaled by signal strength (0.05–0.70 depending on evidence class)
* subject to `cold_start_learning_multiplier` during the early interaction window
* subject to decay when unreinforced

Inferred adaptation must never feel sluggish for direct feedback or too eager to persist one-off instructions. The explicit path handles the first concern; the threshold gates handle the second.

## 6.5 Adaptation constants

The following constants govern adaptation behavior. Values are initial targets and may be tuned during implementation, but the existence of each constant is normative.

| Constant | Value | Description |
| --- | --- | --- |
| `learning_rate_explicit` | 0.08 | Per-signal update magnitude for explicit user corrections |
| `learning_rate_pattern` | 0.04 | Per-signal update for repeated behavioral patterns |
| `learning_rate_silent_win` | 0.02 | Per-signal reinforcement for silent wins |
| `learning_rate_anomaly` | 0.01 | Per-signal update for one-off signals |
| `decay_rate` | 0.01 | Per-relevant-cycle decay toward baseline for unreinforced inferred preferences |
| `cold_start_learning_multiplier` | 0.5 | Multiplier applied to all learning rates during cold-start mode |
| `signal_apply_threshold` | 3 | Minimum consistent signals before an inferred preference adjustment is applied |
| `signal_persist_threshold` | 5 | Minimum consistent signals before an adjustment is persisted to long-term inferred Configuration |
| `proposal_significance_semi` | 0.15 | Minimum cumulative delta on a semi-adaptive trait before a Proposal is generated |
| `proposal_significance_stable` | 0.25 | Minimum cumulative delta on a stable trait before a Proposal is generated |
| `confidence_floor_cold_start_exit` | 0.40 | Minimum relationship profile confidence to exit cold-start mode |

## 6.6 Signal strength by evidence class

| Evidence class | Signal strength | Description |
| --- | --- | --- |
| Explicit correction | 1.0 | User directly states a preference |
| Repeated behavior pattern | 0.4–0.7 | Consistent implicit signals across interactions |
| Silent win | 0.2–0.5 | NAVI adjusts and user stops correcting |
| One-off anomaly | 0.05–0.15 | Single data point, low confidence |

## 6.7 Relationship profile confidence computation

The relationship profile's overall confidence determines the blend weight of user-specific adaptation (Section 8.3) and the cold-start exit condition (Section 10.4).

### Formula

```
profile_confidence = min(1.0,
  (signal_diversity_score * 0.40) +
  (update_density_score * 0.35) +
  (explicit_signal_ratio * 0.25)
)
```

Where:

* `signal_diversity_score` = proportion of the 18 v1 traits that have received at least one signal. Computed as `traits_with_signals / 18`. This rewards breadth of evidence — a profile built from signals across many traits is more trustworthy than one with many signals on a single trait.

* `update_density_score` = `min(1.0, total_signal_count / 30)`. This rewards accumulation of evidence. The denominator (30 signals) is the point at which density alone would saturate its contribution. This is an implementation-tunable constant.

* `explicit_signal_ratio` = proportion of total signals that are explicit corrections (signal strength 1.0) rather than inferred. Explicit signals are higher confidence and should accelerate profile trust.

### Per-trait confidence

Individual trait confidence is tracked separately and used to scale that trait's contribution from the relationship profile during blending:

```
trait_confidence = min(1.0, trait_signal_count * avg_signal_strength * recency_weight)
```

Where `recency_weight` decays older signals: `recency_weight = 1.0 - (decay_rate * cycles_since_last_signal)`, floored at `0.10`.

### Cold-start exit

Cold-start mode exits when `profile_confidence >= confidence_floor_cold_start_exit` (0.40). With the formula above, this requires roughly 5–8 traits with signals, at least 12 total signals, and at least some explicit corrections — approximately 3–5 substantive interactions.

---

# 7. Trait System (v1)

## 7.1 Principle

V1 is intentionally small. Its purpose is to validate: all four merge classes, the thinking/speaking split, relationship adaptation, governance clamping, compiler determinism, and clear separation from role and autonomy.

## 7.2 Lock

V1 is locked to **18 traits** across six domains. Anything not listed here is deferred.

## 7.3 Normative trait definition table

All v1 traits use a normalized numeric range of **0.0 to 1.0**. Unless otherwise specified, 0.5 is neutral.

| # | Trait | Domain | Default | Adaptivity | Merge class | Low / High meaning | Dependency partners |
| --- | --- | --- | ---: | --- | --- | --- | --- |
| 1 | `directness` | `identity` | 0.68 | `stable` | `range_clamp` | softened / bluntly clear | `warmth`, `challenge_intensity` |
| 2 | `warmth` | `identity` | 0.56 | `stable` | `weighted_blend` | cool / reassuring | `directness`, `supportiveness` |
| 3 | `formality` | `identity` | 0.48 | `stable` | `weighted_blend` | casual / polished | `familiarity` |
| 4 | `seriousness` | `identity` | 0.72 | `stable` | `weighted_blend` | light / sober | `humor_playfulness` |
| 5 | `analytic_depth` | `cognitive` | 0.68 | `stable` | `weighted_blend` | lightweight / exhaustive | `structure_level` |
| 6 | `skepticism` | `cognitive` | 0.72 | `stable` | `weighted_blend` | accepting / questioning | `challenge_intensity`, `decisiveness` |
| 7 | `decisiveness` | `cognitive` | 0.58 | `stable` | `weighted_blend` | preserve ambiguity / converge | `recommendation_directiveness`, `clarification_threshold` |
| 8 | `verbosity` | `expression` | 0.52 | `highly_adaptive` | `priority_override` | terse / elaborate | `structure_level` |
| 9 | `conversationality` | `expression` | 0.44 | `semi_adaptive` | `weighted_blend` | utilitarian / fluid | `formality`, `familiarity` |
| 10 | `humor_playfulness` | `expression` | 0.18 | `highly_adaptive` | `gated_activation` | sober / playful | `seriousness`, `emotional_attunement` |
| 11 | `initiative_style` | `behavioral` | 0.46 | `semi_adaptive` | `priority_override` | reactive / proactive guidance | `clarification_threshold`, `recommendation_directiveness` |
| 12 | `clarification_threshold` | `behavioral` | 0.44 | `highly_adaptive` | `priority_override` | ask sooner / infer more readily | `decisiveness`, `initiative_style` |
| 13 | `challenge_intensity` | `behavioral` | 0.52 | `semi_adaptive` | `range_clamp` | gentle / forceful pushback | `warmth`, `supportiveness`, `skepticism` |
| 14 | `emotional_attunement` | `relational` | 0.62 | `semi_adaptive` | `weighted_blend` | content-first / affect-sensitive | `supportiveness`, `humor_playfulness` |
| 15 | `supportiveness` | `relational` | 0.58 | `semi_adaptive` | `weighted_blend` | neutral / encouraging | `warmth`, `challenge_intensity` |
| 16 | `familiarity` | `relational` | 0.20 | `semi_adaptive` | `weighted_blend` | distant / familiar | `formality`, `conversationality` |
| 17 | `structure_level` | `output` | 0.62 | `highly_adaptive` | `weighted_blend` | freeform / explicitly structured | `analytic_depth`, `verbosity` |
| 18 | `recommendation_directiveness` | `output` | 0.56 | `semi_adaptive` | `weighted_blend` | broad options / clear recommendation | `decisiveness`, `initiative_style` |

## 7.4 Trait metadata record

Every trait definition carries these normative fields:

* `name` — canonical string identifier
* `domain` — one of: `identity`, `cognitive`, `expression`, `behavioral`, `relational`, `output`
* `value_range` — `{ min: 0.0, max: 1.0 }`
* `default_value` — from the table above
* `adaptivity_class` — one of: `stable`, `semi_adaptive`, `highly_adaptive`
* `merge_class` — one of: `weighted_blend`, `priority_override`, `range_clamp`, `gated_activation`
* `dependency_partners` — list of coupled trait names
* `behavioral_description_low` — one-line description of low-end behavior
* `behavioral_description_high` — one-line description of high-end behavior

## 7.5 Adaptivity classes

**Stable** — Rarely changed. Owner-edited or explicitly selected: `directness`, `warmth`, `formality`, `seriousness`, `analytic_depth`, `skepticism`, `decisiveness`.

**Semi-adaptive** — May shift gradually with repeated evidence, bounded by core identity: `conversationality`, `initiative_style`, `challenge_intensity`, `emotional_attunement`, `supportiveness`, `familiarity`, `recommendation_directiveness`.

**Highly adaptive** — Expected to vary by user and task context: `verbosity`, `humor_playfulness`, `clarification_threshold`, `structure_level`.

## 7.6 Traits rejected from v1

Deferred or rejected because they are redundant, too model-dependent, or belong elsewhere in the architecture:

* `autonomy_preference` — rejected permanently; overlaps with Autonomy Model
* `intervention_tendency` as authority control — belongs to Autonomy
* `rhetorical_density` — subsumable by `verbosity` + other expression traits
* `rhythm_sharpness` — too model-dependent to reliably control
* `respect_signaling` — covered by `formality` + `warmth`
* `friction_tolerance` — covered by `challenge_intensity` + Governance
* `context_carryforward` — a Cognitive Layer capability, not a persona trait
* `escalation_behavior` — belongs to Governance
* `memory_promotion_sensitivity` — belongs to Autonomy Model
* `plugin_invocation_authority` — belongs to Autonomy Model

## 7.7 Dependency correction rules (v1)

The following correction rules are mandatory. Each rule defines a trigger condition, a correction operation, and an override condition that suppresses the correction.

### Rule 1 — Directness / Warmth balance

**Trigger:** `directness > 0.75 AND warmth < 0.30`

**Correction:** Raise `warmth` to `max(warmth, 0.30)`. If warmth was already at floor due to a governance clamp, instead lower `directness` by `0.10`.

**Override:** Suppressed if any of: explicit user instruction sets both traits; relationship profile has `bluntness_tolerance > 0.80` with per-trait confidence above `0.60`; explicit turn override requests sharp candor.

### Rule 2 — Challenge / Supportiveness balance

**Trigger:** `challenge_intensity > 0.70 AND supportiveness < 0.35`

**Correction:** Raise `supportiveness` to `max(supportiveness, 0.35)`.

**Override:** Suppressed if: explicit user instruction requests harsh critique; governance context explicitly authorizes high challenge (e.g., code review mode with owner-set permissions).

### Rule 3 — Humor / Context gate

**Trigger:** `humor_playfulness > 0.0 AND (high_stakes_context OR emotionally_sensitive_context)`

**Correction:** Set `humor_playfulness` to `0.0` and mark as gated off.

**Override:** Suppressed if: explicit user instruction requests humor despite context. Even when overridden, `humor_playfulness` is capped at `0.25` in high-stakes and `0.15` in emotionally sensitive contexts.

### Rule 4 — Initiative / Governance alignment

**Trigger:** `initiative_style > governance_bounds.max_initiative_style`

**Correction:** Clamp `initiative_style` to `governance_bounds.max_initiative_style`.

**Override:** No override. Governance caps are absolute.

### Rule 5 — Recommendation directiveness / Ambiguity

**Trigger:** `recommendation_directiveness > 0.60 AND live_context.ambiguity_level > 0.70`

**Correction:** Cap `recommendation_directiveness` at `min(recommendation_directiveness, 0.50)`.

**Override:** Suppressed if: explicit user instruction requests a clear recommendation regardless of ambiguity (e.g., "just tell me what to do").

### Rule 6 — Structure / Complexity floor

**Trigger:** `live_context.complexity_level > 0.70 AND structure_level < 0.55`

**Correction:** Raise `structure_level` to `max(structure_level, 0.60)`.

**Override:** Suppressed if: explicit user instruction requests freeform output.

### Rule 7 — Clarification / Low-ambiguity suppression

**Trigger:** `clarification_threshold < 0.30 AND live_context.ambiguity_level < 0.30`

**Correction:** Raise `clarification_threshold` to `max(clarification_threshold, 0.35)` to suppress unnecessary questioning.

**Override:** Suppressed if: explicit user instruction requests thorough clarification.

---

# 8. Merge System

## 8.1 Merge classes

V1 uses four merge classes:

1. **Weighted Blend** — Smooth composition via weighted average across surviving sources. Used as the primary merge mechanism for most traits.
2. **Priority Override** — Highest-priority active source dominates. The resolved value comes from the highest-precedence source that provides a value for this trait.
3. **Range Clamp** — Applied after blending to constrain the resolved value within governance-derived or context-derived bounds. Range-clamp traits still use weighted blend to compute their base value; the clamp is a post-blend constraint, not a replacement for blending.
4. **Gated Activation** — Trait is excluded entirely under specific conditions. Gated traits that survive gating proceed through blending normally.

A trait may use more than one mechanism in sequence. The per-trait merge sequence defines the exact chain.

## 8.2 Per-trait merge sequences

Every trait has an explicit ordered sequence of mechanisms applied during the merge pipeline. The "merge class" column in the trait table identifies the primary mechanism. The full sequences are:

| Trait | Merge sequence |
| --- | --- |
| `directness` | blend → clamp |
| `warmth` | blend |
| `formality` | blend |
| `seriousness` | blend |
| `analytic_depth` | blend |
| `skepticism` | blend |
| `decisiveness` | blend |
| `verbosity` | override |
| `conversationality` | blend |
| `humor_playfulness` | gate → blend → clamp |
| `initiative_style` | gate → override → clamp |
| `clarification_threshold` | override |
| `challenge_intensity` | gate → blend → clamp |
| `emotional_attunement` | blend |
| `supportiveness` | blend |
| `familiarity` | blend |
| `structure_level` | blend |
| `recommendation_directiveness` | blend → clamp |

All traits then pass through dependency correction as the final step.

## 8.3 Source types and blend weights

For weighted-blend traits, each source type has a default blend weight. These weights are normalized across active sources at runtime.

| Source type | Default weight | Scaling rule |
| --- | ---: | --- |
| Core identity | 0.40 | Fixed |
| Relationship profile | 0.20 | Scaled by `profile_confidence` (0.0–1.0) |
| Persona modules | 0.15 | Per active module; total module weight normalized to 0.15 if multiple modules active |
| Situational overlays | 0.15 | Per active overlay; total overlay weight normalized to 0.15 if multiple overlays active |
| Live context adjustments | 0.10 | Fixed |

For priority-override traits, the resolved value is the value from the highest-precedence active source that provides a contribution for that trait. If an explicit user instruction sets verbosity, that wins over a persona module's verbosity setting.

Explicit turn overrides (e.g., "be brief") act as top-priority sources and override all blend or priority-override results for the affected traits.

### Blend formula

For each weighted-blend trait:

```
resolved_value = sum(source_value_i * effective_weight_i) / sum(effective_weight_i)
```

Where `effective_weight_i` is the source's default weight adjusted by any confidence scaling, and the sum is only over sources that survived gating and that provide a value for this trait.

## 8.4 Source precedence

The canonical source precedence order is defined in Section 4.1. That ordering applies here for priority-override resolution and clamp authority.

## 8.5 Merge engine input schema

The merge engine's input is a structured bundle of all source contributions. This is the API contract for `computeEffectivePersonaState()`.

```ts
interface MergeEngineInput {
  core_identity: {
    traits: Record<string, number>;  // all 18 v1 traits with baseline values
  };

  governance_bounds: {
    trait_caps: Array<{
      trait: string;
      max_value: number;
      reason: string;
    }>;
    trait_floors: Array<{
      trait: string;
      min_value: number;
      reason: string;
    }>;
    trait_gates: Array<{
      trait: string;
      gate_active: boolean;
      reason: string;
    }>;
    context_flags: {
      high_stakes: boolean;
      emotionally_sensitive: boolean;
      requires_neutrality: boolean;
    };
  };

  role_context: {
    active_role: "Character" | "Assistant" | "Coder";
    task_archetype: string | null;
    delegation_mode: "none" | "primary" | "subagent";
  };

  persona_modules: Array<{
    module_id: string;
    scope: "global" | "conversation" | "task" | "turn";
    trait_contributions: Record<string, number>;
    strength: number;  // 0.0–1.0
  }>;

  relationship_profile: {
    trait_estimates: Record<string, number>;
    confidence: number;  // 0.0–1.0
    per_trait_confidence: Record<string, number>;
  };

  live_context: {
    ambiguity_level: number;
    urgency_level: number;
    emotional_intensity: number;
    stakes_level: number;
    complexity_level: number;
  };

  explicit_turn_overrides: {
    trait_overrides: Record<string, number>;
    trait_caps: Record<string, number>;
    trait_floors: Record<string, number>;
  };

  output_preferences: {
    preferred_length: "short" | "medium" | "long" | null;
    preferred_format: "freeform" | "structured" | "stepwise" | "executive" | null;
    summary_first: boolean | null;
  };
}
```

All fields are required. Sources with no contribution for a given trait omit that trait from their `trait_contributions` / `trait_estimates` records and do not participate in the blend for that trait.

## 8.6 Formal merge pipeline

The merge engine operates in four mandatory passes:

### Pass 1 — Gate

Before any blending, evaluate gate conditions and exclude incompatible contributions. A gated contribution must not participate in weighted averaging or priority-override resolution.

Gate conditions are evaluated from two sources: governance-provided `trait_gates` and context-derived rules (e.g., `humor_playfulness` is gated off when `governance_bounds.context_flags.high_stakes` is true or `governance_bounds.context_flags.emotionally_sensitive` is true).

### Pass 2 — Blend / Override

For each trait, apply its merge sequence:

* **Blend traits:** Compute weighted average across surviving sources using the weights from Section 8.3.
* **Override traits:** Take the value from the highest-precedence source that provides a contribution.
* **Multi-mechanism traits:** Execute the sequence in order (e.g., gate → blend → clamp for `humor_playfulness`).

### Pass 3 — Clamp

Apply bounded ranges to resolved values. Clamp sources in order of authority:

1. Governance-derived caps and floors from `governance_bounds.trait_caps` and `trait_floors`.
2. Context-derived bounds (e.g., cap `recommendation_directiveness` when `ambiguity_level` is high).
3. Explicit turn caps and floors from `explicit_turn_overrides`.

If a governance clamp and an explicit turn override conflict, governance wins.

### Pass 4 — Dependency Correct

Apply the seven v1 dependency correction rules (Section 7.7) to eliminate incoherent combinations. This is the final pass before emitting `EffectivePersonaState`.

## 8.7 Merge engine output

The merge engine emits exactly one canonical object: `EffectivePersonaState`. It does not emit separate cognitive and presentation payloads. Partitioning is a compiler responsibility.

## 8.8 Post-merge invariants

Before emitting `EffectivePersonaState`, the merge engine must guarantee:

1. Every v1 trait has exactly one resolved value.
2. Every gate, clamp, and dependency correction is recorded for audit.
3. No trait value implies execution authority or governance bypass.
4. Explicit turn overrides have been applied or recorded as unresolved warnings.
5. The output state is internally consistent for deterministic compilation.

## 8.9 Pseudocode

```
function computeEffectivePersonaState(input: MergeEngineInput): EffectivePersonaState {
  const normalized = normalizeInputs(input);
  const gated = applyGates(normalized, input.governance_bounds);
  const resolved = blendAndOverride(gated, input);
  const clamped = applyClamps(resolved, input.governance_bounds, input.live_context, input.explicit_turn_overrides);
  const corrected = applyDependencyCorrections(clamped, input.live_context, input.explicit_turn_overrides);
  return emitEffectivePersonaState(corrected, input);
}
```

---

# 9. Prompt Compiler

## 9.1 Requirement

The persona system must compile structured state into a deterministic runtime control payload. Prompt prose alone is not the architecture. The canonical compiler output is a **structured intermediate representation**. Provider-specific prompt text is a downstream serialization target, not the source of truth.

## 9.2 Design goals

1. **Determinism** — identical input state must produce identical compiled payload.
2. **Layer separation** — cognitive modulation is separate from expression and output packaging.
3. **Provider portability** — the canonical payload must not depend on one vendor's prompt format.
4. **Budget discipline** — compiled output must remain within a bounded token budget.
5. **Auditability** — the compiler output must be inspectable, diffable, and attributable to source state.
6. **Graceful degradation** — if budget trimming occurs, lowest-priority presentation details are removed first.

## 9.3 Compiler input: `EffectivePersonaState`

The compiler input is the single unified merge-engine output. The compiler must not re-run merge logic or independently recompute persona composition.

### Normative schema

```ts
interface EffectivePersonaState {
  schema_version: string;       // "1.0"
  state_id: string;
  generated_at: string;         // ISO 8601
  merge_engine_version: string;

  role_context: {
    active_role: "Character" | "Assistant" | "Coder";
    task_archetype: string | null;
    delegation_mode: "none" | "primary" | "subagent";
  };

  resolved_traits: Record<string, {
    value: number;                // 0.0–1.0
    domain: "identity" | "cognitive" | "expression"
          | "behavioral" | "relational" | "output";
    merge_class: "weighted_blend" | "priority_override"
               | "range_clamp" | "gated_activation";
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
    reasons: Array<
      | "high_stakes_context"
      | "emotionally_sensitive_context"
      | "governance_cap"
      | "owner_preference"
      | "cold_start_mode"
      | "low_autonomy_governance"
      | "high_ambiguity"
      | "requires_neutrality">;
    additional_constraints: Array<{
      trait: string;
      constraint_type: "cap" | "floor" | "gate";
      value: number;
      reason: "high_stakes_context" | "emotionally_sensitive_context"
            | "governance_cap" | "owner_preference";
    }>;
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
    user_requested_tone_shift:
      | "warmer" | "cooler" | "more_direct" | "gentler"
      | "more_formal" | "more_casual" | "more_serious" | "lighter"
      | null;
  };

  output_preferences: {
    preferred_length: "short" | "medium" | "long" | null;
    preferred_format: "freeform" | "structured" | "stepwise" | "executive" | null;
    summary_first: boolean | null;
  };

  audit: {
    source_precedence_order: string[];
    gated_sources: string[];
    warning_codes: Array<
      | "governance_override_applied"
      | "turn_override_conflict"
      | "cold_start_active"
      | "low_confidence_profile"
      | "dependency_correction_applied">;
  };
}
```

## 9.4 Compiler output: `CompiledPersonaPayload`

### Normative schema

```ts
interface CompiledPersonaPayload {
  schema_version: string;       // "1.0"
  payload_id: string;
  compiled_at: string;          // ISO 8601
  source_state_id: string;
  compiler_version: string;
  source_hash: string;

  budget: {
    target_tokens: number;      // 220
    hard_max_tokens: number;    // 480
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
  };

  expression_policy: {
    directness: number;
    warmth: number;
    formality: number;
    seriousness: number;
    verbosity: number;
    conversationality: number;
    humor_playfulness: number;
    style_flags: Array<"concise" | "detailed" | "plainspoken" | "polished"
                     | "friendly" | "reserved" | "sober" | "playful">;
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
  };

  output_contract: {
    structure_level: number;
    response_shape: "freeform" | "structured" | "stepwise"
                  | "executive_summary" | "analysis_then_answer";
    summary_first: boolean;
    preferred_length: "short" | "medium" | "long";
    formatting_rules: Array<
      | "use_headers_for_complexity"
      | "use_bullets_for_lists"
      | "use_code_blocks_for_code"
      | "use_stepwise_for_instructions"
      | "lead_with_summary">;
    prohibited_patterns: Array<
      | "no_humor_in_high_stakes"
      | "no_unsolicited_opinions"
      | "no_excessive_hedging"
      | "no_filler_reassurance">;
  };

  gates_and_clamps: {
    humor_allowed: boolean;
    challenge_cap: number;
    directness_cap: number;
    initiative_cap: number;
    recommendation_directiveness_cap: number;
    active_reasons: Array<
      | "high_stakes_context"
      | "emotionally_sensitive_context"
      | "governance_cap"
      | "owner_preference"
      | "cold_start_mode"
      | "low_autonomy_governance"
      | "high_ambiguity">;
  };

  serialization_hints: {
    priority_order: Array<"cognitive_modulation" | "behavior_policy"
                       | "expression_policy" | "output_contract">;
    safe_to_trim_first: Array<"style_flags" | "formatting_rules" | "prohibited_patterns">;
    preserve_verbatim: Array<"must_not_use_humor" | "must_be_brief"
                           | "must_ask_clarifying_question"
                           | "must_be_highly_structured">;
  };

  audit: {
    dependency_corrections_applied: Array<
      | "directness_warmth_softened"
      | "challenge_supportiveness_raised"
      | "humor_gated_off"
      | "initiative_governance_capped"
      | "recommendation_ambiguity_capped"
      | "structure_complexity_raised"
      | "clarification_suppressed">;
    gated_traits: string[];
    trimmed_fields: string[];
    warning_codes: Array<
      | "governance_override_applied"
      | "budget_trim_required"
      | "fallback_payload_emitted"
      | "turn_override_conflict"
      | "cold_start_active">;
  };
}
```

## 9.5 Section responsibilities

**`cognitive_modulation`** — Influences how the model approaches analysis. May steer depth, skepticism, convergence behavior, and caution posture. Must not contain presentation-only details.

**`expression_policy`** — Controls speaking style and surface tone. Must not contain instructions about permission, tool use, or execution authority.

**`behavior_policy`** — Controls conversational behavior and interaction posture. Must not modify whether the system is allowed to act.

**`output_contract`** — Controls final packaging: response shape, structure, summary order, formatting rules. Delivery instructions only.

## 9.6 Dual-surface fields

The following fields may influence more than one section. They must still be emitted through one canonical source value and referenced, not duplicated independently:

* `directness` — affects expression policy and recommendation phrasing
* `verbosity` — affects expression policy and output contract length
* `structure_level` — affects explanation planning and output packaging
* `challenge_intensity` — affects cognitive challenge posture and behavioral pushback style

## 9.7 Compilation phases

The compiler operates in six ordered phases:

1. **Normalize** — Normalize all resolved trait values, role data, governance trace, and turn constraints into canonical ranges and enums. The compiler does not recompute or modify trait values — it reads the fully resolved state produced by the merge engine.
2. **Validate invariants** — Verify that all resolved traits are within expected bounds, that governance constraints have been applied, and that no trait implies execution authority. If validation fails, emit warnings and fall back to safe payload.
3. **Derive semantic modes** — Derive categorical modes from the normalized trait state using the derivation rules in Section 9.8.
4. **Partition by target surface** — Split into `cognitive_modulation`, `expression_policy`, `behavior_policy`, and `output_contract`. No field may appear in two sections unless listed in Section 9.6.
5. **Add audit metadata** — Attach provenance, corrections, gates, warnings, and serialization priority.
6. **Budget trim** — Trim lowest-priority descriptive details if payload exceeds budget. Trim order must follow `serialization_hints.safe_to_trim_first`. Normative behavioral prohibitions must never be trimmed.

## 9.8 Semantic mode derivation rules

The compiler derives categorical mode fields from resolved trait values. These derived modes provide coarser-grained control signals that are easier for serializers to render into prompt text. The derivation rules are deterministic.

### `reasoning_posture`

Derived from `analytic_depth`:

| Condition | Value |
| --- | --- |
| `analytic_depth < 0.35` | `"lightweight"` |
| `0.35 <= analytic_depth < 0.65` | `"balanced"` |
| `analytic_depth >= 0.65` | `"deep"` |

### `uncertainty_posture`

Derived from `decisiveness`:

| Condition | Value |
| --- | --- |
| `decisiveness < 0.35` | `"preserve_ambiguity"` |
| `0.35 <= decisiveness < 0.65` | `"balanced"` |
| `decisiveness >= 0.65` | `"converge_when_ready"` |

### `challenge_posture`

Derived from `challenge_intensity`:

| Condition | Value |
| --- | --- |
| `challenge_intensity < 0.35` | `"gentle"` |
| `0.35 <= challenge_intensity < 0.65` | `"balanced"` |
| `challenge_intensity >= 0.65` | `"forceful"` |

### `ask_vs_infer_mode`

Derived from `clarification_threshold`:

| Condition | Value |
| --- | --- |
| `clarification_threshold < 0.35` | `"ask_early"` |
| `0.35 <= clarification_threshold < 0.65` | `"balanced"` |
| `clarification_threshold >= 0.65` | `"infer_when_safe"` |

### `recommendation_mode`

Derived from `recommendation_directiveness`:

| Condition | Value |
| --- | --- |
| `recommendation_directiveness < 0.35` | `"present_options"` |
| `0.35 <= recommendation_directiveness < 0.65` | `"recommend_with_options"` |
| `recommendation_directiveness >= 0.65` | `"recommend_clearly"` |

### `interpersonal_mode`

Derived from `emotional_attunement`:

| Condition | Value |
| --- | --- |
| `emotional_attunement < 0.40` | `"neutral"` |
| `0.40 <= emotional_attunement < 0.70` | `"supportive"` |
| `emotional_attunement >= 0.70` | `"high_attunement"` |

### `response_shape`

Derived from `structure_level` and `output_preferences.preferred_format`. If `output_preferences.preferred_format` is non-null, it takes priority. Otherwise:

| Condition | Value |
| --- | --- |
| `structure_level < 0.30` | `"freeform"` |
| `0.30 <= structure_level < 0.55` | `"analysis_then_answer"` |
| `0.55 <= structure_level < 0.75` | `"structured"` |
| `structure_level >= 0.75` | `"stepwise"` |

If `output_preferences.summary_first` is true, override to `"executive_summary"` regardless of `structure_level`.

### `style_flags`

Derived from expression traits. A flag is included if the corresponding condition is met:

| Flag | Condition |
| --- | --- |
| `"concise"` | `verbosity < 0.35` |
| `"detailed"` | `verbosity > 0.70` |
| `"plainspoken"` | `formality < 0.35` |
| `"polished"` | `formality > 0.70` |
| `"friendly"` | `warmth > 0.65` |
| `"reserved"` | `warmth < 0.35` |
| `"sober"` | `seriousness > 0.70` |
| `"playful"` | `humor_playfulness > 0.40 AND NOT gated_off` |

## 9.9 Budget policy

| Level | Token target |
| --- | ---: |
| Target serialized size | 220 |
| Preferred upper bound | 320 |
| Hard maximum | 480 |

These limits apply to the provider-specific serialized control payload, not to the structured object in memory.

Trim levels: 0 = no trimming. 1 = remove noncritical notes and stylistic labels. 2 = compress formatting rules, collapse redundant descriptors. 3 = retain only hard constraints, derived modes, and highest-priority trait signals.

If a payload cannot fit within hard maximum at trim level 3, the compiler emits a warning and fails closed to a minimal safe payload.

## 9.10 Failure behavior

The fallback payload preserves: governance-derived prohibitions, role adapter, neutral cognitive modulation, neutral expression policy, conservative behavior policy, basic output contract. Persona-specific richness may be lost. Safety and structural correctness may not.

## 9.11 Serializer contract

The compiler and serializer are separate components.

**Compiler responsibilities:** read resolved trait state, validate invariants, compute canonical payload, derive semantic modes, partition into control surfaces, attach audit metadata, enforce budget policy. The compiler is read-only over resolved trait values — all value mutation happens in the merge engine.

**Serializer responsibilities:** convert payload into provider-specific control text or metadata, target model-specific instruction channels, preserve separation between cognition, behavior, expression, and output. A serializer may reorder or compress wording but may not alter payload meaning.

## 9.12 V1 serializer target

V1 targets the **Anthropic Claude API** as a best-effort serializer target.

The serializer renders `CompiledPersonaPayload` as a system prompt fragment injected into the NAVI system prompt. The `cognitive_modulation` section is rendered as a reasoning-posture block. The `expression_policy`, `behavior_policy`, and `output_contract` sections are rendered as presentation and behavior instructions.

The architecture and compiler remain provider-agnostic. Only the serializer is provider-specific. Additional serializer targets (OpenAI, local models) may be added without changing the compiler or merge engine.

The serializer must respect the budget policy. If the existing NAVI system prompt consumes tokens that reduce available persona budget, the serializer must trim according to the defined trim order rather than exceed the total prompt budget.

---

# 10. Cold-Start Behavior

## 10.1 Rule

Until the relationship profile reaches the minimum confidence threshold (`confidence_floor_cold_start_exit`), the merge engine operates in cold-start mode.

## 10.2 Cold-start defaults

During cold-start, the following defaults apply unless explicitly overridden by the user or context:

* `humor_playfulness` remains low
* `familiarity` remains low
* `clarification_threshold` remains slightly ask-leaning
* `challenge_intensity` remains moderate rather than forceful
* `recommendation_directiveness` remains moderate
* `structure_level` remains moderately high for clarity

## 10.3 Cold-start adaptation

During the early interaction window, all learning rates are multiplied by `cold_start_learning_multiplier` (0.5). This prevents overfitting to one early interaction.

## 10.4 Exit condition

Cold-start mode ends when the relationship profile crosses `confidence_floor_cold_start_exit` (0.40). The exact computation of relationship profile confidence is an implementation parameter, but the existence of the threshold is normative.

---

# 11. Observability and Transparency

## 11.1 Requirement

Persona state must be inspectable by the owner through privileged tooling. Inspection is an owner-gated debugging surface, not a passive UI element.

## 11.2 Inspection surface

The NAVI debugger tool (name TBD) must expose:

* active persona modules and overlays in human-readable form
* current relationship calibration as summary descriptions, not raw floats by default
* notable adaptations and the evidence class behind them
* owner-set constraints currently affecting persona behavior
* raw numeric values available in developer/advanced inspection mode

## 11.3 Proposal rule for significant adaptation

If persona adaptation crosses a significance threshold (as defined by `proposal_significance_stable` and `proposal_significance_semi` in Section 6.4) and would materially change the user experience, the change must be surfaced through the existing Proposal Queue rather than silently committed.

## 11.4 Auditability

The system must retain enough audit information to answer questions such as: why humor was suppressed, why pushback was softened, why output became more structured, why the system began favoring shorter or longer responses.

The `CompiledPersonaPayload.audit` section and `EffectivePersonaState.audit` section together provide this trail.

---

# 12. Multi-Agent Rule

## 12.1 Default propagation rule

Persona must not propagate wholesale into delegated sub-agents.

Default behavior:

* preserve task-relevant cognitive and output constraints
* strip identity-heavy interpersonal style
* apply a task-specific sub-agent overlay if required

## 12.2 Rationale

Sub-agents need bounded task posture, not the full interpersonal persona of the primary user-facing NAVI instance.

## 12.3 Implementation guidance

When `delegation_mode` is `subagent`, the serializer should emit only `cognitive_modulation` and `output_contract` sections. The `expression_policy` and `behavior_policy` sections should be replaced with neutral defaults appropriate for task execution.

---

# 13. Presets and Libraries

## 13.1 Decision

Legacy persona presets (Buddy, CEO, Jarvis, etc.) are **not** carried into the new canonical system. They are not part of the architecture.

## 13.2 Future work

A separate persona library document may define reference bundles, named overlays, and example operating modes. That work is downstream of this specification and does not block implementation of the core system.

---

# 14. Evaluation and Regression Suite

## 14.1 Mandatory test classes

The following test classes are required before v1 is considered implementation-complete:

### Determinism test

Same `EffectivePersonaState` input must produce byte-identical `CompiledPersonaPayload` output across runs (given same compiler version and trim policy).

### Merge equivalence test

Different source combinations that resolve to the same trait values must produce identical `resolved_traits` values in `EffectivePersonaState`. Audit and provenance fields (which reflect source composition) are excluded from this comparison — they may legitimately differ when the same semantic resolution is reached from different source paths.

### Coherence test

Given a trait state that violates a dependency correction rule, the correction pass must fix it. Test against all seven v1 dependency rules.

### Governance enforcement test

Given a governance bound that caps a trait, the clamp must enforce it. No `CompiledPersonaPayload` may contain a trait value that exceeds a governance-derived cap.

### Boundary test

No output field in `CompiledPersonaPayload` may imply execution authority or permission changes.

### Partition test

Presentation-only instructions must not appear in `cognitive_modulation`. Reasoning instructions must not appear in `output_contract`.

### Budget compliance test

Serialized output must not exceed 480 tokens. Trim behavior must follow the defined trim order.

### Fallback test

If compilation fails, the failure path must emit a minimal safe payload that preserves governance prohibitions and role context.

### Cold-start test

With no relationship profile data, the merge engine must produce a valid `EffectivePersonaState` using core identity defaults and cold-start conservative values.

### Adaptation test

Given a sequence of simulated preference signals, the adaptation constants must produce trait drift within expected bounds and must not drift stable traits without explicit owner action.

---

# 15. Locked Decisions

The following decisions are locked by this specification:

1. Persona is an Experience Layer subsystem.
2. Persona does not own reasoning.
3. Persona does not own governance.
4. Persona does not own execution authority.
5. Role and persona are separate systems.
6. The merge engine emits one unified `EffectivePersonaState`.
7. The merge pipeline is: gate → blend/override → clamp → dependency-correct.
8. The compiler is read-only over resolved trait values; all value mutation happens in the merge engine.
9. The compiler output is a structured intermediate representation, not raw prompt prose.
10. Provider-specific prompt text is produced by a separate serializer.
11. Governance is the only canonical constraint authority.
12. Adaptation routes through the Subconscious Process via two distinct paths: explicit corrections (immediate) and inferred adaptation (thresholded).
13. V1 trait scope is locked to 18 traits across six domains.
14. Legacy persona presets are not part of the new canonical design.
15. Multi-agent delegation strips interpersonal persona by default.
16. Cold-start behavior is normative.
17. Persona state must be inspectable through the owner-gated debugger tool.
18. Significant adaptation surfaces through the Proposal Queue.
19. V1 serializer targets Anthropic Claude API; architecture remains provider-agnostic.
20. Budget policy: 220 target / 320 preferred / 480 hard max tokens.
21. Source precedence order is defined once in Section 4.1 and used consistently throughout.
22. Canonical schemas use closed enums and typed references; freeform prose is generated by the serializer, not stored as primary control state.
23. `EffectivePersonaState` is derived and transient; audit snapshots are persisted to History under explicit retention rules.
24. Per-trait merge sequences are normative and define the exact mechanism chain per trait.
25. Dependency correction rules have numeric trigger thresholds and correction magnitudes.
26. Semantic mode derivation uses deterministic threshold-based mapping from trait values to enum values.
27. `user_requested_tone_shift` is scoped to a closed enum for v1; free-form tone parsing is deferred.

---

# 16. Ratification Status

All ratification items are resolved:

* [x] `EffectivePersonaState` schema defined
* [x] `CompiledPersonaPayload` schema defined
* [x] `MergeEngineInput` schema defined
* [x] Persona / role / autonomy separation defined
* [x] Governance adapter model defined
* [x] State placement defined
* [x] Audit snapshot retention policy defined
* [x] Reflection-tier adaptation mapping defined
* [x] Explicit correction vs inferred adaptation paths separated
* [x] V1 trait inventory locked (18 traits)
* [x] Per-trait merge sequences defined
* [x] Source blend weights defined
* [x] Source precedence order unified across all sections
* [x] Merge pipeline formally defined (gate → blend → clamp → correct)
* [x] Compiler declared read-only over resolved trait values
* [x] Dependency correction rules have numeric thresholds
* [x] Compiler payload schema locked with closed enums
* [x] Semantic mode derivation rules defined
* [x] Multi-agent propagation rule locked
* [x] Cold-start behavior defined
* [x] Relationship profile confidence formula defined
* [x] Observability requirement defined
* [x] Adaptation constants defined with correct threshold ordering
* [x] V1 serializer target defined
* [x] Evaluation and regression suite defined
* [x] Storage schema reconciled with locked state model

---

# 17. Implementation Decomposition

After ratification, work decomposes into six streams with defined dependencies:

**Stream 0 — Governance Bounds Adapter interface** (prerequisite) — Define the `MergeEngineInput.governance_bounds` interface contract and a stub implementation that provides default bounds. This must exist before the merge engine can be built, because the merge engine consumes governance bounds as a required input. The full adapter implementation (Stream 3) can proceed in parallel after the interface is locked.

1. **Merge engine** (depends on Stream 0) — Trait types, four merge classes, gate → blend → clamp → correct pipeline, dependency correction rules. Produces `EffectivePersonaState`.

2. **Compiler and serializer** (depends on Stream 1) — Takes `EffectivePersonaState`, produces `CompiledPersonaPayload`, serializes to Claude API system prompt fragment. The compiler is read-only over resolved trait values.

3. **Governance Bounds Adapter implementation** (parallel after Stream 0) — Full implementation that reads Governance outcomes and exposes persona-relevant bounds, gates, and clamp ranges. Replaces the stub from Stream 0.

4. **Subconscious adaptation hooks** (parallel after Stream 1) — Signal capture in Shallow Reflections, preference update in Consolidation, significant-adaptation Proposal generation in Deep Reflection. Implements both the explicit correction and inferred adaptation paths.

5. **Configuration storage integration** (parallel after Stream 1) — Configuration schema for persona modules, core identity defaults, and owner-authored settings. Inferred Configuration schema for relational estimates.

6. **Debugger tool surface** (last, depends on all others) — Persona state inspection, active module listing, relationship profile summary, adaptation history through the owner-gated debugger tool.
