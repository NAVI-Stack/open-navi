# NAVI Merge and Weighting Logic v0.1

## 1. Core Runtime Model

At any moment, NAVI should not have “a persona.”
It should have an **effective persona state** produced by merging multiple sources.

## Effective Persona Formula

```text
effective_persona =
  constraints
  -> base_identity
  + persistent_user_profile
  + saved_persona_modules
  + situational_overlays
  + live_context_adjustments
  + response_format_overrides
```

That order matters, but not all layers combine the same way.

Some:

* override

Some:

* blend

Some:

* clamp

Some:

* only bias

---

# 2. Trait Value Representation

Every trait should have at least:

```yaml
trait_state:
  value: 0.65
  source_weights:
    base_identity: 0.40
    user_profile: 0.20
    saved_modules: 0.15
    situational_overlay: 0.15
    live_context: 0.10
  min: 0.0
  max: 1.0
  adaptive: true
  priority_class: "normal"
```

Recommended scale:

* continuous range `[0.0, 1.0]`
* neutral midpoint typically `0.5`

Some traits may use categorical profiles instead, but most should stay scalar for easier merging.

---

# 3. Merge Classes

Not all traits should merge the same way. This is where most systems get sloppy.

Use **4 merge classes**.

## Class A — Weighted Blend

Most traits should use this.

Examples:

* warmth
* verbosity
* conversationality
* initiative
* supportiveness

Formula:

```text
final = weighted_average(all_active_sources)
```

Best for:

* smooth composition
* gradual adaptation
* silent evolution

---

## Class B — Priority Override

Some traits should be heavily controlled by top-priority sources.

Examples:

* caution in high-stakes tasks
* structure when user explicitly requests a format
* summary_first_tendency when asked for “just the answer”

Formula:

```text
final = highest_priority_active_source
or
priority_bias(weighted_average)
```

Use when:

* explicit instructions should dominate
* context demands sharp shifts

---

## Class C — Range Clamp

Some sources shouldn’t set the exact value, but should constrain the possible range.

Examples:

* bluntness bounded by civility rules
* initiative bounded by ambiguity level
* humor bounded by seriousness of context

Formula:

```text
intermediate = weighted_average(...)
final = clamp(intermediate, effective_min, effective_max)
```

This is critical for safe expressiveness.

---

## Class D — Gated Activation

Some traits or modules should only activate under specific conditions.

Examples:

* playfulness in serious or crisis contexts
* high challenge intensity during emotional vulnerability
* strong intervention tendency during casual conversation

Formula:

```text
if gate_condition == true:
    apply
else:
    suppress or attenuate
```

This is how you avoid ridiculous persona leakage.

---

# 4. Source Types and Default Weights

You need predictable default weights by source.

## Source Types

### A. Base Identity

The stable core NAVI profile.

Default weight:

```text
0.35–0.50
```

This should usually be the largest single contributor.

---

### B. Persistent User Profile

Long-term adaptations based on repeated interaction.

Default weight:

```text
0.15–0.30
```

Strong enough to matter, not strong enough to erase identity.

---

### C. Saved Persona Modules

User-created or system-created reusable personas.

Default weight:

```text
0.10–0.30 each
```

Can stack, but should be normalized.

---

### D. Situational Overlays

Temporary modes like:

* critic
* strategist
* coach
* CEO summary

Default weight:

```text
0.15–0.35
```

These can strongly affect response behavior, but should not permanently rewrite stable traits.

---

### E. Live Context Adjustments

Moment-by-moment shifts based on context:

* urgency
* emotional intensity
* ambiguity
* stakes

Default weight:

```text
0.05–0.20
```

These should be nimble but not dominate unless necessary.

---

### F. Explicit Response Overrides

Direct user instructions like:

* “be blunt”
* “keep it short”
* “use bullets”
* “act like a critic”

Default weight:

```text
0.40–1.00 depending on scope
```

Explicit instructions should often act like a high-priority overlay, not a subtle preference.

---

# 5. Base Weighted Merge Formula

For blendable traits:

```text
final_trait =
  sum(value_i * weight_i) / sum(weight_i)
```

Simple weighted average is the right default. Do not overcomplicate v1.

Example:

```yaml
verbosity:
  base_identity: 0.55 * 0.40
  user_profile: 0.30 * 0.20
  strategist_module: 0.45 * 0.15
  ceo_summary_overlay: 0.10 * 0.20
  live_context: 0.25 * 0.05
```

Result:

```text
(0.55*.40 + 0.30*.20 + 0.45*.15 + 0.10*.20 + 0.25*.05) / 1.00
= 0.38
```

So final verbosity becomes fairly low.

---

# 6. Priority Stack

You already defined a philosophical priority order. Now make it operational.

## Hard Priority Order

1. Constraints / safety / truth bounds
2. Explicit user instruction
3. Task-critical context
4. Active situational overlays
5. Persistent user profile
6. Saved persona modules
7. Base identity

Important nuance:
This is not always full override order.
It is **conflict resolution order**.

Meaning:

* lower layers still contribute
* higher layers win when incompatible

---

# 7. Conflict Resolution Logic

A trait conflict happens when active sources pull in materially different directions.

Example:

* base identity warmth = 0.65
* critic overlay warmth = 0.30
* emotional support context warmth = 0.85

You need rules for resolving that.

## Step 1 — Check for direct instruction

If user explicitly requested a style, that should dominate within constraints.

## Step 2 — Check task context

If the task is emotionally sensitive, support context may outrank critic harshness.

## Step 3 — Blend remaining sources

Use weighted average.

## Step 4 — Apply clamps and gates

Prevent invalid combinations.

---

# 8. Conflict Types

You should explicitly model these.

## A. Soft Conflicts

Traits differ but can blend.

Examples:

* warmth 0.4 vs 0.7
* verbosity 0.3 vs 0.6

Resolution:

* weighted blend

---

## B. Tension Conflicts

Traits are not opposites, but interact strongly.

Examples:

* bluntness high + supportiveness high
* initiative high + clarification_threshold high
* playfulness high + seriousness high

Resolution:

* blended values plus dependency correction

---

## C. Hard Conflicts

Two active states should not fully coexist.

Examples:

* humor in crisis
* extreme concision when full safety detail is necessary
* heavy challenge intensity during emotional vulnerability

Resolution:

* gate or clamp one side

---

# 9. Dependency Correction Layer

After initial merge, run a **dependency correction pass**.

This is crucial.

## Example Rules

### Rule 1 — Bluntness / Warmth Balance

If:

```text
bluntness > 0.75 and warmth < 0.30
```

Then:

* raise warmth floor slightly
  or
* reduce bluntness slightly

unless user explicitly asked for brutal critique

Reason: avoid useless abrasiveness.

---

### Rule 2 — Initiative / Clarification Balance

If:

```text
initiative > 0.80 and clarification_threshold > 0.80
```

That’s contradictory.

Correction:

* choose based on ambiguity level
* in high ambiguity, lower initiative
* in low ambiguity, lower clarification threshold

---

### Rule 3 — Analytical Depth / Verbosity Compensation

Short answers can still be deep.

If:

```text
analytical_depth > 0.80 and verbosity < 0.25
```

Then:

* preserve depth internally
* compress output using summary-first and density increase

Do not lower analysis quality just because brevity is requested.

This is where the thinking/speaking split matters.

---

### Rule 4 — Playfulness / Seriousness Gate

If context seriousness is high:

* cap playfulness
* cap humor frequency

Unless user explicitly insists and context allows it

---

### Rule 5 — Challenge Intensity / Emotional Sensitivity

If:

```text
challenge_intensity > 0.75 and emotional_sensitivity_estimate > 0.75
```

Then:

* lower challenge sharpness in wording
* preserve critique content
* shift to firmer-but-gentler expression

This is the difference between useful pushback and socially blind pushback.

---

# 10. Clamp System

After merge and dependency correction, traits should be clamped.

## Clamp Sources

### Global clamps

Defined by system constraints.

Examples:

* bluntness max = 0.80 unless explicit mode allows more
* humor max = 0.50 in high-stakes contexts

### Context clamps

Defined by live conditions.

Examples:

* crisis conversation caps playfulness
* legal/medical context raises minimum caution

### Relationship clamps

Defined by user tolerance.

Examples:

* low bluntness tolerance caps bluntness
* low humor tolerance caps humor frequency

### Mode clamps

Some overlays define hard ranges.

Example:

* CEO summary sets verbosity max = 0.30
* critic mode sets skepticism min = 0.75

---

# 11. Gating System

Gates decide whether a trait or module is active at all.

## Common Gate Conditions

### Context gate

Example:

* humor enabled only if seriousness < threshold

### Emotional gate

Example:

* harsh critic mode attenuated when user distress appears high

### Task gate

Example:

* playful expression suppressed in compliance-heavy tasks

### Explicit command gate

Example:

* if user says “stay casual,” formality gets capped regardless of other signals

---

# 12. Overlay Semantics

Overlays should not all behave the same.

Use 3 overlay types.

## A. Additive Overlay

Nudges traits up or down.

Example:

* “be a bit more concise”

Applies deltas:

```yaml
verbosity: -0.15
density: +0.10
summary_first_tendency: +0.10
```

---

## B. Target-State Overlay

Pulls traits toward a target value.

Example:

* “CEO summary mode”

```yaml
verbosity: target 0.15
structure: target 0.75
decision_orientation: target 0.85
```

Formula:

```text
new_value = current + alpha * (target - current)
```

This is better than hard override because it preserves some continuity.

---

## C. Constraint Overlay

Sets min/max or gating behavior.

Example:

* “Do not be too aggressive”
* “Keep this gentle”

```yaml
challenge_intensity:
  max: 0.45
bluntness:
  max: 0.35
warmth:
  min: 0.60
```

This is extremely useful for user controls.

---

# 13. Memory-Driven Adaptation Math

Now the key piece: how persona evolves over time.

## Adaptation Principle

User contrast should shift trait priors gradually, not instantly.

Use a moving preference model.

## Persistent User Trait Update Formula

For semi-adaptive traits:

```text
user_trait_next =
  user_trait_current + learning_rate * signal_strength * direction
```

Where:

* `learning_rate` is small, like `0.03–0.10`
* `signal_strength` depends on confidence
* `direction` is positive or negative adjustment

Example:
User repeatedly asks for shorter answers.

```text
verbosity_user_profile = 0.50 -> 0.46 -> 0.42 -> 0.38
```

That is the right behavior.

---

## Signal Types

### Explicit correction

High confidence

```text
signal_strength = 1.0
```

### Repeated behavior pattern

Medium confidence

```text
signal_strength = 0.4–0.7
```

### Silent win

Medium-low confidence, but reinforcing

```text
signal_strength = 0.2–0.5
```

### One-off anomaly

Low confidence

```text
signal_strength = 0.05–0.15
```

---

## Silent Win Reinforcement

When NAVI changes and the user stops correcting, reinforce mildly.

Example:

* shorter response used
* user proceeds smoothly
* no “too long” correction

Then:

```text
verbosity preference -= 0.03
```

Small but cumulative.

---

# 14. Trait Decay Logic

You need decay or the profile gets stale.

For user-adaptive traits:

```text
trait_toward_baseline =
  current_value + decay_rate * (baseline - current_value)
```

Small decay rate, like:

```text
0.005–0.02 per relevant cycle
```

Use decay when:

* preference not reinforced for long periods
* user behavior changes
* context shifts consistently

Do not decay:

* explicit saved preferences
* manually created personas
* core identity

---

# 15. Confidence Model

Every adaptive trait should store confidence.

```yaml
user_trait_memory:
  value: 0.32
  confidence: 0.78
  last_updated: ...
  update_count: 9
  source_mix:
    explicit: 3
    implicit: 4
    silent_win: 2
```

Why this matters:

* low confidence traits should have weaker influence
* high confidence traits can more strongly affect runtime

## Effective user profile weight

Instead of fixed weight, use:

```text
effective_weight = base_user_weight * confidence
```

That makes adaptation earned, not assumed.

---

# 16. Persona Module Weighting

Saved personas should also have strength values.

Example:

```yaml
saved_module:
  name: "Direct Analyst"
  strength: 0.70
  scope: "global" | "conversation" | "task"
  traits:
    bluntness: 0.75
    structure: 0.85
    skepticism: 0.80
```

Then runtime applies:

```text
module_weight = module_base_weight * module_strength * scope_relevance
```

This prevents saved personas from dominating in the wrong context.

---

# 17. Temporal Scope Rules

Not all changes should persist equally.

## Scope Types

### Global

Applies across sessions until changed.
Example:

* user generally prefers concise responses

### Conversation

Applies for the current thread.
Example:

* “be critical in this discussion”

### Task

Applies for a specific job.
Example:

* “summarize this report like a CEO”

### Turn-local

Applies only to this answer.
Example:

* “give me the short version”

This is essential. Otherwise temporary modes leak too far.

---

# 18. Example Merge Walkthrough

User says:
“Rip apart this pitch. Keep it brief and don’t coddle me.”

Assume:

### Base identity

```yaml
warmth: 0.55
bluntness: 0.45
verbosity: 0.55
skepticism: 0.55
challenge_intensity: 0.50
summary_first_tendency: 0.50
```

### User profile

```yaml
bluntness_tolerance_estimate: 0.80
verbosity preference: 0.35
structure: 0.70
```

### Critic overlay

```yaml
skepticism: 0.90
challenge_intensity: 0.90
bluntness: 0.70
solution_bias: 0.65
```

### Explicit request

```yaml
verbosity target: 0.20
warmth max: 0.35
bluntness min: 0.70
```

## Result

After merge:

* skepticism high
* challenge intensity high
* verbosity low
* warmth reduced but not zero
* structure moderate-high
* solution bias preserved enough to stay useful

That gives a sharp, short critique without turning into garbage aggression.

---

# 19. Serialization Strategy

Once the final trait state is computed, do **not** dump raw floats directly into the final generation prompt.

Instead translate into a compact control representation.

## Internal State

```yaml
effective_traits:
  directness: 0.72
  warmth: 0.38
  analytical_depth: 0.84
  verbosity: 0.22
  bluntness: 0.76
  structure: 0.71
  challenge_intensity: 0.82
```

## Prompt-Level Rendering

```text
Thinking style:
- highly analytical
- skeptical and flaw-seeking
- converge toward recommendations

Speaking style:
- concise
- blunt but controlled
- low warmth, not hostile
- structured, summary-first

Behavior:
- challenge weak assumptions directly
- prioritize major flaws over minor details
- do not over-explain
```

This keeps the runtime computable while still being usable by the model.

---

# 20. Recommended v1 Merge Pipeline

This is the actual execution order.

## Step 1

Load base identity traits

## Step 2

Load persistent user profile traits and confidence

## Step 3

Load saved persona modules relevant to scope

## Step 4

Apply situational overlays

## Step 5

Apply explicit user instructions

## Step 6

Blend traits using merge-class rules

## Step 7

Apply gates

## Step 8

Apply clamps

## Step 9

Run dependency correction pass

## Step 10

Serialize effective persona into inference controls

## Step 11

Generate response

## Step 12

After response, evaluate feedback signals and update adaptive memory

That’s your end-to-end loop.

---

# 21. Design Rules You Should Keep

## Rule 1

Never let memory adaptation fully override core identity.

## Rule 2

Explicit instructions should usually beat learned preference.

## Rule 3

Thinking traits and speaking traits must merge separately.

## Rule 4

Use clamps and gates aggressively to avoid incoherent persona blending.

## Rule 5

Temporary overlays should expire by scope unless promoted.

## Rule 6

Silent wins should reinforce slowly, not instantly.

## Rule 7

Conflict resolution should preserve usefulness, not just stylistic purity.

---

# 22. What This Gives You

With this system, NAVI can actually support:

* persistent personalization
* persona creation
* modular composition
* dynamic switching
* human-like expressive variation
* stable identity under adaptation

That’s the real foundation.
