# NAVI Persona Object Spec and Storage Schema v0.1

## 1. Design Goals

The storage model must support five things cleanly:

### A. Reusability

Personas and trait bundles should be reusable across users, conversations, and tasks.

### B. Composability

A saved persona should be mixable with other personas, overlays, and live context.

### C. Adaptation

The system must store both explicit and inferred changes without corrupting the base persona.

### D. Auditability

You should be able to answer:

* why did NAVI sound like this?
* what changed?
* what was learned?
* what was temporary vs persistent?

### E. Version Safety

Personas should evolve by versioning, not by silent destructive mutation of the original object.

---

# 2. Core Object Types

You should define **7 primary object types**:

1. `PersonaDefinition`
2. `TraitBundle`
3. `OverlayDefinition`
4. `EffectivePersonaSnapshot`
5. `UserPersonaProfile`
6. `PreferenceSignal`
7. `PersonaLibraryIndex`

These cover creation, runtime, persistence, and adaptation.

---

# 3. PersonaDefinition

This is the main reusable persona object.

It represents a complete or near-complete persona configuration.

## Purpose

Used for:

* base personas
* named saved personas
* user-created personas
* system-created composite personas

## Schema

```yaml
persona_definition:
  id: "persona.strategist.v1"
  name: "Strategist"
  description: "Decision-oriented, long-horizon, structured assistant persona."
  version: 1
  status: "active"   # active | deprecated | experimental | archived
  created_by: "system"   # system | user | inferred
  created_at: "2026-03-25T00:00:00Z"
  updated_at: "2026-03-25T00:00:00Z"

  scope_defaults:
    default_scope: "conversation"   # global | conversation | task | turn
    persistence_mode: "reusable"

  identity_traits:
    directness: 0.65
    warmth: 0.35
    formality: 0.55
    seriousness: 0.80
    assertiveness: 0.70

  cognitive_traits:
    analytical_depth: 0.80
    skepticism: 0.65
    synthesis_orientation: 0.90
    concreteness: 0.70
    decision_orientation: 0.85
    caution: 0.55

  expression_traits:
    verbosity: 0.45
    bluntness: 0.60
    conversationality: 0.35
    playfulness: 0.10
    precision: 0.80
    hedging: 0.30

  behavioral_traits:
    initiative: 0.75
    clarification_threshold: 0.35
    challenge_intensity: 0.70
    solution_bias: 0.80
    autonomy_preference: 0.75
    priority_discipline: 0.90

  relational_traits:
    familiarity: 0.30
    supportiveness: 0.35
    bluntness_tolerance_estimate: 0.50
    collaboration_preference: 0.55
    trust_calibration: 0.60

  output_traits:
    structure: 0.80
    summary_first_tendency: 0.85
    bullet_preference: 0.55
    density: 0.70
    stepwise_framing: 0.45
    option_span: 0.35

  constraints:
    truth_priority: 1.0
    safety_priority: 1.0
    civility_floor: 0.30
    humor_max_in_high_stakes: 0.20

  gates:
    disable_playfulness_in_crisis: true
    reduce_bluntness_when_user_distressed: true

  tags:
    - "strategy"
    - "decision-making"
    - "structured"

  compatibility:
    recommended_with:
      - "module.concise"
      - "overlay.ceo_summary"
    avoid_pairing_with:
      - "overlay.high_playfulness"

  serialization_hints:
    thinking_style: "strategic, integrative, decision-oriented"
    speaking_style: "concise, structured, controlled"
    behavioral_style: "proactive, prioritizing, challenging"

  provenance:
    source_personas: []
    notes: "System-defined baseline strategist persona."
```

---

# 4. TraitBundle

This is a smaller reusable object than a full persona.

Think of it as a micro-module.

## Purpose

Used for:

* composable traits
* style packs
* micro-personas
* reusable subcomponents

Examples:

* blunt
* concise
* teacherly
* skeptical
* warm
* executive

## Schema

```yaml
trait_bundle:
  id: "module.concise.v1"
  name: "Concise"
  version: 1
  created_by: "system"
  type: "expression"   # identity | cognitive | expression | behavioral | relational | output | mixed

  trait_deltas:
    expression_traits:
      verbosity: -0.30
      precision: +0.10
    output_traits:
      summary_first_tendency: +0.20
      density: +0.15
      option_span: -0.10

  target_traits: {}
  clamps:
    expression_traits:
      verbosity:
        max: 0.35

  gates: {}
  tags:
    - "brevity"
    - "high-signal"

  serialization_hints:
    speaking_style: "brief, compressed, high-information"
```

### Important distinction

A `TraitBundle` should usually express:

* **deltas**
  not
* full trait replacements

This is what makes composability work.

---

# 5. OverlayDefinition

An overlay is a temporary, scoped modifier.

This is not a stable persona object. It is a runtime behavior mode.

## Purpose

Used for:

* critic mode
* CEO summary mode
* tutor mode
* gentle mode
* brainstorming mode

## Schema

```yaml
overlay_definition:
  id: "overlay.critic.v1"
  name: "Critic Mode"
  version: 1
  category: "behavioral_mode"
  created_by: "system"

  default_scope: "conversation"
  expiration_policy:
    type: "until_changed"   # one_turn | task_end | conversation_end | until_changed
    max_turns: null

  activation_conditions:
    requires_user_consent: false
    suppress_if_emotional_distress_high: true

  merge_behavior:
    type: "target_state"   # additive | target_state | constraint
    strength: 0.75

  target_traits:
    cognitive_traits:
      skepticism: 0.90
      analytical_depth: 0.80
    behavioral_traits:
      challenge_intensity: 0.85
      solution_bias: 0.65
      intervention_tendency: 0.75
    expression_traits:
      bluntness: 0.70
      hedging: 0.20

  clamps:
    expression_traits:
      playfulness:
        max: 0.15

  tags:
    - "critique"
    - "flaw-finding"
    - "sharp"

  serialization_hints:
    thinking_style: "skeptical, flaw-seeking, evaluative"
    speaking_style: "direct, sharp, concise"
    behavioral_style: "challenge assumptions, focus on weaknesses first"
```

---

# 6. EffectivePersonaSnapshot

This is the runtime-resolved persona state for a specific response or response window.

Do not treat this as the master persona. This is the computed result.

## Purpose

Used for:

* explainability
* debugging
* analytics
* training feedback loops
* post-response adaptation

## Schema

```yaml
effective_persona_snapshot:
  id: "snapshot.2026-03-25T12:04:22Z.abc123"
  user_id: "user_001"
  conversation_id: "conv_742"
  turn_id: "turn_19"

  inputs:
    base_persona_id: "persona.navi_core.v1"
    active_personas:
      - "persona.strategist.v1"
    active_trait_bundles:
      - "module.concise.v1"
    active_overlays:
      - "overlay.critic.v1"

  explicit_user_overrides:
    output_traits:
      bullet_preference: 0.80
    expression_traits:
      bluntness:
        min: 0.70

  context_flags:
    high_stakes: false
    emotional_distress_high: false
    ambiguity_high: false
    urgency_high: true

  computed_traits:
    identity_traits:
      directness: 0.72
      warmth: 0.32
      formality: 0.48
      seriousness: 0.82
      assertiveness: 0.78
    cognitive_traits:
      analytical_depth: 0.84
      skepticism: 0.88
      synthesis_orientation: 0.81
      concreteness: 0.70
      decision_orientation: 0.79
      caution: 0.50
    expression_traits:
      verbosity: 0.24
      bluntness: 0.74
      conversationality: 0.36
      playfulness: 0.05
      precision: 0.82
      hedging: 0.18
    behavioral_traits:
      initiative: 0.71
      clarification_threshold: 0.25
      challenge_intensity: 0.83
      solution_bias: 0.68
      autonomy_preference: 0.74
      priority_discipline: 0.89
    relational_traits:
      familiarity: 0.45
      supportiveness: 0.28
      bluntness_tolerance_estimate: 0.82
      collaboration_preference: 0.42
      trust_calibration: 0.76
    output_traits:
      structure: 0.78
      summary_first_tendency: 0.86
      bullet_preference: 0.81
      density: 0.73
      stepwise_framing: 0.39
      option_span: 0.24

  source_contributions:
    expression_traits:
      bluntness:
        base_identity: 0.45
        user_profile: 0.10
        strategist: 0.00
        concise: 0.00
        critic_overlay: 0.15
        explicit_override: 0.20

  applied_clamps:
    - "playfulness capped by critic mode"
  applied_gates:
    - "no humor due to critique mode"
  dependency_corrections:
    - "warmth floor raised slightly to avoid hostile tone"

  serialized_control_text:
    thinking_style: "Highly analytical and skeptical. Prioritize major weaknesses and decision-relevant flaws."
    speaking_style: "Brief, sharp, controlled, low warmth, not insulting."
    behavioral_style: "Challenge assumptions early, prioritize signal, avoid padding."

  created_at: "2026-03-25T12:04:22Z"
```

This object matters a lot. It gives you traceability.

---

# 7. UserPersonaProfile

This is where user-specific adaptation lives.

It must be separate from the base persona. Do not mutate the core persona directly per user.

## Purpose

Used for:

* persistent personalization
* explicit user preferences
* inferred trait drift
* confidence-weighted adaptation

## Schema

```yaml
user_persona_profile:
  user_id: "user_001"
  version: 4
  updated_at: "2026-03-25T12:10:00Z"

  baseline_persona_id: "persona.navi_core.v1"

  explicit_preferences:
    expression_traits:
      verbosity:
        preferred_value: 0.30
        confidence: 1.00
        source: "explicit_user_request"
      bluntness:
        preferred_range:
          min: 0.60
          max: 0.80
        confidence: 0.90
        source: "explicit_user_request"

    output_traits:
      structure:
        preferred_value: 0.75
        confidence: 0.95
        source: "explicit_user_request"

  inferred_preferences:
    expression_traits:
      conversationality:
        preferred_value: 0.38
        confidence: 0.62
        evidence_count: 8
        last_updated: "2026-03-23T19:00:00Z"
      hedging:
        preferred_value: 0.22
        confidence: 0.58
        evidence_count: 5
        last_updated: "2026-03-24T15:30:00Z"

    output_traits:
      summary_first_tendency:
        preferred_value: 0.84
        confidence: 0.80
        evidence_count: 12
      bullet_preference:
        preferred_value: 0.68
        confidence: 0.73
        evidence_count: 9

    behavioral_traits:
      challenge_intensity:
        preferred_value: 0.72
        confidence: 0.76
        evidence_count: 7

  tolerance_estimates:
    bluntness_tolerance: 0.84
    humor_tolerance: 0.28
    detail_tolerance: 0.40
    autonomy_tolerance: 0.74
    emotional_sensitivity_baseline: 0.36

  preferred_modules:
    default_bundles:
      - "module.concise.v1"
    favored_overlays:
      - "overlay.critic.v1"
      - "overlay.strategist.v1"

  relationship_state:
    familiarity: 0.68
    trust_calibration: 0.79
    collaboration_preference: 0.51
    supportiveness_preference: 0.34

  adaptation_policy:
    learning_rate: 0.06
    decay_rate: 0.01
    explicit_preference_priority: 1.00
    inferred_preference_priority: 0.65
```

---

# 8. PreferenceSignal

This is the atomic learning object.

Every adaptation-worthy interaction should be stored as a signal record, at least for some retention window.

## Purpose

Used for:

* contrast learning
* silent-win tracking
* explainability
* preference confidence building

## Schema

```yaml
preference_signal:
  id: "signal.983245"
  user_id: "user_001"
  conversation_id: "conv_742"
  turn_id: "turn_19"
  timestamp: "2026-03-25T12:05:00Z"

  signal_type: "explicit_correction"   # explicit_correction | implicit_pattern | silent_win | negative_reaction | repetition_pattern
  strength: 0.90
  confidence: 0.95

  affected_traits:
    expression_traits:
      verbosity:
        direction: -1
        delta_hint: 0.20
      bluntness:
        direction: +1
        delta_hint: 0.15
    output_traits:
      summary_first_tendency:
        direction: +1
        delta_hint: 0.10

  evidence:
    user_text: "Keep it short and don’t soften it."
    assistant_snapshot_id: "snapshot.2026-03-25T12:04:22Z.abc123"

  outcome:
    adaptation_applied: true
    persisted_to_profile: true
    persistence_confidence_gain: 0.08
```

This object is what lets you do real preference learning instead of fuzzy guessing.

---

# 9. PersonaLibraryIndex

This is the searchable catalog of persona-related objects.

## Purpose

Used for:

* discovery
* recommendation
* compatibility lookup
* persona creation and editing workflows

## Schema

```yaml
persona_library_index:
  personas:
    - id: "persona.navi_core.v1"
      name: "NAVI Core"
      type: "base_persona"
      tags: ["default", "balanced"]
    - id: "persona.strategist.v1"
      name: "Strategist"
      type: "full_persona"
      tags: ["strategy", "decision-making"]

  trait_bundles:
    - id: "module.concise.v1"
      name: "Concise"
      type: "expression"
      tags: ["brevity", "high-signal"]
    - id: "module.blunt.v1"
      name: "Blunt"
      type: "expression"
      tags: ["directness", "sharpness"]

  overlays:
    - id: "overlay.critic.v1"
      name: "Critic Mode"
      type: "behavioral_mode"
      tags: ["critique", "evaluation"]
    - id: "overlay.ceo_summary.v1"
      name: "CEO Summary"
      type: "output_mode"
      tags: ["executive", "compression"]
```

This should be queryable by:

* tags
* trait effects
* compatibility
* scope
* popularity
* success rate

---

# 10. Persona Creation Object

Since you explicitly wanted persona creation like `create_skill`, this should be formalized.

Use a creation request object.

## Schema

```yaml
create_persona_request:
  name: "Direct Analyst"
  description: "A concise, skeptical, structured persona for critique and technical evaluation."
  base_persona_id: "persona.navi_core.v1"

  composition:
    include_personas:
      - "persona.strategist.v1"
    include_bundles:
      - "module.concise.v1"
      - "module.blunt.v1"
    include_overlays: []

  trait_overrides:
    cognitive_traits:
      skepticism: 0.82
      analytical_depth: 0.78
    output_traits:
      structure: 0.86
      summary_first_tendency: 0.80

  constraints:
    civility_floor: 0.35

  scope_defaults:
    default_scope: "global"

  save_as_reusable: true
```

## Result

```yaml
create_persona_result:
  persona_id: "persona.direct_analyst.v1"
  version: 1
  created_from:
    base: "persona.navi_core.v1"
    personas:
      - "persona.strategist.v1"
    bundles:
      - "module.concise.v1"
      - "module.blunt.v1"
```

This is much cleaner than “edit prompt text until it feels right.”

---

# 11. Versioning Rules

This part matters a lot.

## Rule A — Base personas are immutable by version

Do not mutate `persona.strategist.v1` into something else.
Create `persona.strategist.v2`.

## Rule B — User profiles are mutable, but versioned

User preference state can update in place, but should retain:

* version number
* updated timestamp
* signal history references

## Rule C — Inferred personas should preserve provenance

If NAVI auto-creates a persona from behavior, keep source lineage.

Example:

```yaml
provenance:
  created_from_signals:
    - "signal.9321"
    - "signal.9322"
    - "signal.9328"
  source_modules:
    - "module.concise.v1"
    - "overlay.critic.v1"
```

## Rule D — Overlays should also version

Modes evolve too. Treat them as controlled definitions, not loose text blobs.

---

# 12. Storage Separation Rules

Keep these as separate stores or logical partitions:

### A. System persona store

Contains:

* core personas
* official bundles
* official overlays

### B. User persona store

Contains:

* saved user-created personas
* promoted inferred personas
* per-user favorites

### C. User adaptation store

Contains:

* user persona profile
* tolerance estimates
* learned trait priors

### D. Signal/event store

Contains:

* preference signals
* silent wins
* corrections
* inferred pattern evidence

### E. Runtime snapshot store

Contains:

* effective persona snapshots
* merge traces
* serialization outputs

Do not jam all of that into one object. That gets ugly fast.

---

# 13. Promotion Workflow

This is how silent adaptation becomes a reusable persona.

## Flow

### Step 1

Repeated signals cluster around a pattern.

Example:

* concise
* blunt
* structured
* skeptical

### Step 2

System creates a candidate inferred persona.

```yaml
candidate_persona:
  name: "Direct Analyst"
  created_by: "inferred"
  confidence: 0.78
```

### Step 3

System either:

* applies silently as a profile tendency
* or surfaces it:
  “You consistently prefer concise, skeptical, structured responses. Save this as a reusable persona?”

### Step 4

If accepted, store as `PersonaDefinition`

This is where “silent wins” become durable.

---

# 14. Explainability Model

You should be able to answer:

* Why was this response so blunt?
* Why did NAVI switch into summary-first mode?
* Why did it become more supportive?

That requires a trace model.

## Minimum trace fields

* active sources
* explicit overrides
* top contributing traits
* clamps and gates applied
* relevant user profile factors
* last significant signals

This is why `EffectivePersonaSnapshot` is not optional.

---

# 15. Minimal v1 Storage Model

Do not overbuild v1. Start with these required objects:

### Required

* `PersonaDefinition`
* `TraitBundle`
* `OverlayDefinition`
* `UserPersonaProfile`
* `PreferenceSignal`
* `EffectivePersonaSnapshot`

### Optional for v1

* `PersonaLibraryIndex` as a separate object
* separate persona recommendation metadata
* advanced compatibility graphs

That gets you a usable system fast.

---

# 16. Example End-to-End Data Flow

User says:
“Be more direct, use bullets, and stop overexplaining.”

## Step 1 — Generate signals

Create `PreferenceSignal` entries:

* directness +
* bullet preference +
* verbosity -

## Step 2 — Update `UserPersonaProfile`

Raise confidence on:

* bluntness tolerance
* bullet preference
* low verbosity preference

## Step 3 — Build runtime state

Merge:

* base persona
* user profile
* active overlay if any
* explicit turn override

## Step 4 — Store `EffectivePersonaSnapshot`

Save final trait state and contributors

## Step 5 — If repeated

Promote pattern into:

* preferred bundle
  or
* inferred persona

That’s the whole loop.

---

# 17. Design Warnings

These are the traps.

## A. Do not store only “final prompts”

That kills modularity and explainability.

## B. Do not mutate base persona directly per user

That destroys reuse and provenance.

## C. Do not treat inferred preference as equal to explicit preference

That creates bad overfitting.

## D. Do not let snapshots become the canonical persona

They are runtime artifacts, not ground truth.

## E. Do not skip versioning

You will regret it later.

---

# 18. Recommended Next Step

Now that the data model exists, the next real step is to define the **persona DSL / authoring format**.

That means:

* how a designer writes personas
* how bundles are declared
* how overlays are authored
* how constraints and gates are specified
* how these serialize into runtime objects

That will give you the authoring layer on top of this storage layer.

That’s the next piece if you want this to be buildable by humans instead of only by code.
