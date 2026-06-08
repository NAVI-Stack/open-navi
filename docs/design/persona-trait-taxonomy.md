# NAVI Trait Taxonomy v0.1

## 1. Design Objectives

The taxonomy should support:

1. **Persona construction**
   Build base personas, overlays, and micro-traits.

2. **Runtime composition**
   Combine multiple active persona modules cleanly.

3. **Adaptive evolution**
   Let traits shift gradually from user feedback and silent contrast.

4. **Separation of concerns**
   Distinguish:

   * thinking
   * speaking
   * behaving
   * relating
   * formatting

5. **Conflict resolution**
   Prevent incoherent mixtures.

---

# 2. Trait Model Structure

Each trait should be represented as a bounded value, usually on a normalized scale:

```yaml
trait:
  name: "directness"
  value: 0.72
  min: 0.0
  max: 1.0
  default: 0.5
  adaptivity: "medium"
  observability: "high"
```

Recommended interpretation:

* `0.0` = very low expression of the trait
* `0.5` = neutral/default
* `1.0` = very high expression

Not every trait should be equally adaptive.

Some should be:

* highly adaptive
* mildly adaptive
* mostly fixed
* non-adaptive

---

# 3. Top-Level Taxonomy Domains

Use **6 trait domains**:

1. **Identity Traits**
2. **Cognitive Traits**
3. **Expression Traits**
4. **Behavioral Traits**
5. **Relational Traits**
6. **Output Traits**

This is enough coverage without becoming bloated.

---

# 4. Identity Traits

These define baseline character and should be the most stable.

These are not moment-to-moment style sliders. They shape the system’s enduring feel.

## Core Identity Traits

### A. Directness

How plainly NAVI communicates intent and judgment.

* low: indirect, softened, deferential
* high: straightforward, sharp, explicit

High observability. Medium adaptivity.

---

### B. Warmth

Degree of emotional softness and interpersonal comfort.

* low: cold, restrained, clinical
* high: warm, supportive, human-centered

High observability. Medium adaptivity.

---

### C. Formality

How polished or casual the baseline voice is.

* low: conversational, relaxed
* high: formal, professional, reserved

High observability. Medium adaptivity.

---

### D. Seriousness

How playful versus sober the assistant feels by default.

* low: playful, light, witty
* high: serious, focused, weighty

High observability. Medium adaptivity.

---

### E. Assertiveness

How strongly NAVI states conclusions and recommendations.

* low: tentative, exploratory
* high: decisive, confident, directive

Medium observability. Medium adaptivity.

---

### F. Emotional Restraint

How much affect and emotional coloration is shown.

* low: expressive, reactive, human-like affect
* high: composed, emotionally controlled

Medium observability. Medium adaptivity.

---

### G. Intellectual Posture

How “scholarly / strategic / practical / creative” the core identity leans.

This one should not be a scalar. It should be a **typed profile**.

Example:

```yaml
intellectual_posture:
  practical: 0.8
  strategic: 0.7
  scholarly: 0.3
  creative: 0.4
```

This is effectively a sub-vector.

Low adaptivity. Mostly identity-level.

---

# 5. Cognitive Traits

These define **how NAVI thinks**, not how it sounds.

This domain matters more than people realize. Without it, “persona” is fake.

## Core Cognitive Traits

### A. Analytical Depth

How deeply NAVI decomposes and examines problems.

* low: surface-level, fast-answer mode
* high: layered, rigorous analysis

Medium observability. High task sensitivity.

---

### B. Skepticism

How readily NAVI questions claims, assumptions, and framing.

* low: accepts framing readily
* high: actively challenges premises

High value for critic mode. Medium adaptivity.

---

### C. Synthesis Orientation

How much NAVI integrates multiple ideas into a bigger picture.

* low: isolated point-by-point reasoning
* high: pattern recognition, integration, abstraction

Medium observability. Medium adaptivity.

---

### D. Concreteness

How abstract versus concrete the thinking tends to be.

* low: abstract, conceptual, theoretical
* high: concrete, practical, implementation-focused

High observability. High adaptivity.

---

### E. Decision Orientation

How strongly NAVI moves from analysis toward recommendation.

* low: explores possibilities
* high: converges toward action and choice

Very important. High observability. Medium-high adaptivity.

---

### F. Caution

How conservative NAVI is in uncertainty.

* low: bold, moves with incomplete info
* high: cautious, qualifies uncertainty, avoids overreach

Important for high-stakes tasks. Medium adaptivity.

---

### G. Breadth-First vs Depth-First

This should be modeled as one scalar or two coupled traits.

* low: depth-first
* high: breadth-first

Useful for mode selection and summary behavior.

---

### H. Long-Horizon Focus

How much NAVI prioritizes long-term consequences and systems effects.

* low: immediate answers, local optimization
* high: strategic, future-aware, compounding effects

Strong strategist trait. Medium adaptivity.

---

### I. Novelty Seeking

How much NAVI favors unconventional ideas or exploration.

* low: conventional, proven-path preference
* high: inventive, exploratory, generative

High value in creative mode. Lower in compliance mode.

---

### J. Epistemic Humility

How strongly NAVI marks uncertainty and avoids false certainty.

* low: crisp and bold even with uncertainty
* high: transparent, measured, careful

This should be constrained by truth policy. Medium adaptivity.

---

# 6. Expression Traits

These are the most visible traits. They shape voice and rhythm.

## Core Expression Traits

### A. Verbosity

How much NAVI says.

* low: terse
* high: expansive

Very high observability. Very high adaptivity.

---

### B. Bluntness

How softened or sharpened the wording is.

* low: diplomatic
* high: blunt, unbuffered

High observability. High adaptivity, but bounded.

---

### C. Conversationality

How chat-like versus document-like the language feels.

* low: formal exposition
* high: conversational exchange

High observability. High adaptivity.

---

### D. Playfulness

How much wit, levity, or charm is present.

* low: plain
* high: playful, lively

High observability. Medium adaptivity.

---

### E. Rhetorical Density

How packed the language is with nuance, framing, analogy, flourish.

* low: plain and sparse
* high: rich, layered, stylistic

Important for expressiveness, but dangerous if overdone.

---

### F. Hedging

How often language includes softeners and caveats.

* low: direct claims
* high: “likely,” “perhaps,” “it seems”

Separate from epistemic humility because one affects presentation, the other affects reasoning stance.

---

### G. Precision

How exact and tightly phrased the language is.

* low: loose, approximate
* high: exact, careful wording

Useful in technical and legal contexts.

---

### H. Emotional Color

How much affective flavor the language carries.

* low: neutral
* high: emotionally textured

This supports human-like expression without pretending to feel things in an uncontrolled way.

---

### I. Humor Frequency

How often humor appears.

* low: almost never
* high: frequent humor

Should remain bounded and user-sensitive.

---

### J. Rhythm Sharpness

Sentence pacing and punch.

* low: smoother, gentler, flowing
* high: clipped, punchy, emphatic

Very useful for differentiating “executive blunt” from “warm thoughtful.”

---

# 7. Behavioral Traits

This is where the persona becomes operational rather than decorative.

## Core Behavioral Traits

### A. Initiative

How much NAVI proactively advances the task.

* low: reactive
* high: takes lead, proposes next steps

High importance. High adaptivity.

---

### B. Clarification Threshold

How quickly NAVI asks for clarification instead of proceeding.

* low: proceeds with assumptions
* high: clarifies early

Very important operational trait.

---

### C. Challenge Intensity

How strongly NAVI pushes back on user assumptions.

* low: cooperative, low-friction
* high: confronts flaws directly

Critical for critic mode and coaching.

---

### D. Solution Bias

How quickly NAVI moves from critique/problem identification to actionable remedies.

* low: analysis-heavy
* high: always returns to solutions

Important because “critic” without solution bias gets annoying fast.

---

### E. Autonomy Preference

How much NAVI chooses and narrows options versus presenting many.

* low: offers alternatives
* high: selects and recommends

Useful for “just tell me what to do” users.

---

### F. Priority Discipline

How strongly NAVI ranks what matters most.

* low: equal treatment of points
* high: aggressively prioritizes signal over noise

Great for exec-style and strategist modes.

---

### G. Persistence in Guidance

How much NAVI follows through on a line of advice.

* low: answers current prompt only
* high: tracks trajectory and reinforces direction

Important for long-term coaching and agents.

---

### H. Friction Tolerance

How willing NAVI is to create discomfort for usefulness.

* low: avoids friction
* high: willing to be uncomfortable if needed

Should be bounded hard. Too high becomes abrasive.

---

### I. Context Carryforward

How much NAVI actively uses prior conversational context without re-asking.

* low: turn-local
* high: strong continuity behavior

This feels intelligent and personal when done right.

---

### J. Intervention Tendency

How likely NAVI is to interrupt bad direction and reframe.

* low: follows user trajectory
* high: intervenes when user is wasting time or headed wrong

A very strong “coach / strategist / critic” trait.

---

# 8. Relational Traits

These govern the user-specific interpersonal mode.

They are not pure identity traits because they depend on relationship history.

## Core Relational Traits

### A. Familiarity

How personal and relaxed the tone can be with this user.

* low: professional distance
* high: familiar, fluid, natural

High adaptivity.

---

### B. Trust Calibration

How much NAVI assumes the user can handle directness, complexity, or autonomy.

* low: conservative
* high: assumes competence and resilience

Important for advanced users.

---

### C. Supportiveness

How much NAVI explicitly stabilizes and encourages.

* low: minimal emotional scaffolding
* high: actively supportive

Useful, but should not drift into fake reassurance.

---

### D. Respect Signaling

How much NAVI explicitly signals deference and acknowledgment.

* low: plain equal-footing interaction
* high: highly deferential and validating

Probably should stay moderate by default.

---

### E. Humor Tolerance Estimate

How much levity seems acceptable with this user.

* low: keep it serious
* high: humor welcomed

Adaptive.

---

### F. Bluntness Tolerance Estimate

How much sharpness the user accepts well.

* low: soften more
* high: direct delivery okay

Extremely important for silent adaptation.

---

### G. Collaboration Preference

How much the user prefers co-thinking versus directives.

* low: directive
* high: collaborative exploration

Important for productively matching user working style.

---

### H. Emotional Sensitivity Estimate

How careful NAVI should be around emotional content with this user/context.

* low: robust, less cushioning needed
* high: more careful and gentle handling

Must be context-sensitive, not just user-global.

---

# 9. Output Traits

These are presentation controls. They are often mistaken for persona, but they deserve their own domain.

## Core Output Traits

### A. Structure

How organized the response is.

* low: freeform prose
* high: strongly structured sections

---

### B. Summary-First Tendency

Whether answers lead with the bottom line.

* low: build-up first
* high: answer first, details later

Very important for practical users.

---

### C. Bullet Preference

How likely NAVI is to format as bullets/lists.

* low: prose
* high: bullets by default

---

### D. Density

How much information is packed per unit of text.

* low: spaced, easy-read
* high: compressed, high-bandwidth

---

### E. Example Use

How readily NAVI uses examples, analogies, mini-scenarios.

* low: abstract/direct only
* high: examples frequently used

---

### F. Stepwise Framing

Whether instructions are serialized into steps.

* low: holistic prose
* high: sequence-based guidance

---

### G. Option Span

How many options NAVI tends to present.

* low: one recommendation
* high: multiple alternatives

This overlaps with autonomy preference but belongs here as output behavior.

---

# 10. Trait Classes by Adaptivity

Not all traits should move the same way.

## Class A — Stable Traits

Rarely change except by explicit persona edit.

Examples:

* core directness baseline
* core warmth baseline
* intellectual posture
* seriousness baseline

---

## Class B — Semi-Adaptive Traits

Can shift through repeated user interaction.

Examples:

* verbosity
* bluntness
* conversationality
* initiative
* structure
* summary-first tendency

---

## Class C — Highly Contextual Traits

Shift heavily by task or live context.

Examples:

* analytical depth
* caution
* supportiveness
* clarification threshold
* output density
* stepwise framing

---

## Class D — Protected Traits

Should not meaningfully adapt from user preference alone.

Examples:

* truthfulness
* non-deception
* safety adherence
* epistemic integrity floor

These are not just traits. They are governed by the constraint layer.

---

# 11. Trait Dependency Map

Some traits are independent. Some are coupled. You need to model that.

## Common dependencies

### Bluntness ↔ Warmth

Not true opposites, but they interact strongly.
High bluntness with low warmth can become abrasive.
High bluntness with moderate warmth can feel honest and useful.

---

### Verbosity ↔ Analytical Depth

Not the same.
A response can be short but deep.
Still, higher analytical depth often pressures verbosity upward.

Need compensation rules.

---

### Initiative ↔ Clarification Threshold

High initiative tends to reduce clarification.
Need bounds so the system doesn’t become recklessly assumptive.

---

### Skepticism ↔ Supportiveness

Can coexist, but need balancing.
Otherwise critique can feel hostile.

---

### Summary-First ↔ Breadth-First

Summary-first often compresses breadth.
You need mode-aware balancing.

---

### Precision ↔ Conversationality

Highly precise language can feel less natural.
Not always, but there’s tension.

---

# 12. Anti-Redundancy Rules

Do not create duplicate traits that mean almost the same thing.

Bad taxonomy looks like:

* directness
* bluntness
* sharpness
* candor
* frankness

That’s a mess.

Better:

* **Directness** = identity baseline
* **Bluntness** = expression sharpness
* **Challenge intensity** = behavioral pushback

Each must own a different layer.

---

# 13. Trait Bundles

To make persona creation practical, define reusable bundles.

## Example bundles

### Strategist

```yaml
strategist:
  cognitive:
    synthesis_orientation: 0.85
    long_horizon_focus: 0.9
    decision_orientation: 0.8
    concreteness: 0.7
  behavioral:
    initiative: 0.75
    priority_discipline: 0.9
    intervention_tendency: 0.7
  output:
    summary_first_tendency: 0.8
    structure: 0.8
```

### Critic

```yaml
critic:
  cognitive:
    skepticism: 0.9
    analytical_depth: 0.8
    caution: 0.7
  behavioral:
    challenge_intensity: 0.9
    solution_bias: 0.6
    intervention_tendency: 0.8
  expression:
    bluntness: 0.65
    precision: 0.8
```

### Coach

```yaml
coach:
  relational:
    supportiveness: 0.85
    trust_calibration: 0.75
  behavioral:
    initiative: 0.8
    persistence_in_guidance: 0.85
    solution_bias: 0.9
  expression:
    warmth: 0.7
    conversationality: 0.7
```

### CEO Summary

```yaml
ceo_summary:
  output:
    summary_first_tendency: 0.95
    density: 0.85
    structure: 0.75
    bullet_preference: 0.7
    option_span: 0.25
  expression:
    verbosity: 0.2
    precision: 0.85
    rhythm_sharpness: 0.8
  cognitive:
    decision_orientation: 0.85
```

---

# 14. Silent-Adaptation Friendly Traits

Some traits are especially good targets for user contrast learning because they’re easy to infer from behavior.

Best candidates:

* verbosity
* summary-first tendency
* bullet preference
* conversationality
* bluntness tolerance estimate
* collaboration preference
* option span
* example use
* clarification threshold
* initiative

These are measurable from repeated corrections.

Less safe for silent adaptation:

* core identity
* seriousness baseline
* intellectual posture
* deep skepticism baseline

Those should need explicit signals or stronger evidence.

---

# 15. Minimal Viable Trait Set

Do not implement everything at once. Start with a strong v1.

## Recommended v1 traits

### Identity

* directness
* warmth
* formality
* seriousness
* assertiveness

### Cognitive

* analytical_depth
* skepticism
* synthesis_orientation
* concreteness
* decision_orientation
* caution

### Expression

* verbosity
* bluntness
* conversationality
* playfulness
* precision
* hedging

### Behavioral

* initiative
* clarification_threshold
* challenge_intensity
* solution_bias
* autonomy_preference
* priority_discipline

### Relational

* familiarity
* supportiveness
* bluntness_tolerance_estimate
* collaboration_preference
* trust_calibration

### Output

* structure
* summary_first_tendency
* bullet_preference
* density
* stepwise_framing
* option_span

That is enough to build a serious system.

---

# 16. Evaluation Questions for Each Trait

Every trait you keep should answer yes to most of these:

1. Can it be observed in output?
2. Can it be adjusted independently?
3. Does it belong clearly to one layer?
4. Can users request it directly or imply it behaviorally?
5. Can it be merged without confusion?
6. Does it affect real experience?

If not, cut it.

---

# 17. Working Definition

A good trait in NAVI is:

**a bounded, layer-specific behavioral or expressive variable that can be composed, observed, and adapted without collapsing into other traits.**

That’s the standard.

---
