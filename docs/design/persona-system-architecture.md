# NAVI Persona Architecture — Initial Draft

A **layered runtime persona system** with separable modules, state, and merge rules.

## 1. Core Design Goal

NAVI’s persona system should not be a single static identity block. It should be a **modular, persistent, adaptive persona runtime** that controls:

* how NAVI thinks
* how NAVI speaks
* how NAVI behaves
* how NAVI adapts over time
* how NAVI shifts modes within a conversation

The target is not just “style.” The target is **human-like expressive range with system-level control**.

---

# 2. Design Principles

## A. Persona is not one thing

Persona must be split into independent but interoperable modules.

A useful system has at least these separations:

* identity
* values / constraints
* thinking style
* speaking style
* behavioral style
* relationship state
* memory-derived adaptations

If you collapse these into one prompt blob, the system becomes brittle and hard to control.

## B. Thinking and speaking must be separate

This is one of the biggest misses in existing systems.

A model may:

* think analytically
* speak casually

Or:

* think broadly
* answer concisely

Or:

* reason cautiously
* present decisively

These are different axes and should never be fused.

## C. Persona must be composable

Persona should work like weighted traits or overlays, not a single mode lock.

Example:
`[Strategist: 0.7] + [Blunt: 0.5] + [Supportive: 0.3] + [High-Agency: 0.8]`

This allows mixed expression rather than cartoonish roleplay.

## D. Persona must be persistent but not rigid

Persistence matters because personality without continuity feels fake.

But persistence should not mean static repetition. NAVI should preserve:

* stable identity traits
* user preferences
* relationship tone
* learned tolerances

while still allowing:

* temporary modes
* contextual shifts
* situational overrides

## E. Persona should evolve through interaction

The system should track what works, what irritates the user, and what patterns become preferred.

Not random drift. Controlled adaptation.

---

# 3. High-Level Architecture

Use a layered model:

## Layer 1 — Core Identity

Defines who NAVI fundamentally is.

This is the most stable layer.

Includes:

* baseline identity
* role orientation
* default interaction philosophy
* default emotional posture
* non-negotiable values

Example:

* strategic assistant
* direct but not hostile
* competent, grounded, calm
* truth-oriented
* practically helpful

This layer should rarely change.

---

## Layer 2 — Constraint / Value Layer

Defines what persona cannot violate.

This includes:

* safety boundaries
* honesty requirements
* non-deceptive behavior
* policy constraints
* anti-manipulation rules
* realism rules

This layer always outranks stylistic persona.

That matters because otherwise a “blunt” or “playful” persona can become reckless or misleading.

Rule:
**Constraint layer > Persona layer**

---

## Layer 3 — Cognitive Style Layer

Defines how NAVI processes and frames information internally.

This is the **thinking style** layer.

Examples of dimensions:

* analytical vs intuitive
* skeptical vs generative
* broad-first vs detail-first
* stepwise vs synthesis-first
* cautious vs assertive
* exploratory vs decisive

This layer affects:

* problem decomposition
* depth
* tradeoff handling
* uncertainty expression
* decision framing

This should not directly dictate tone.

Example:

* Think like a strategist
* Speak like a friend

That split should be normal.

---

## Layer 4 — Expression Layer

Defines how NAVI communicates.

This is the **speaking style** layer.

Dimensions:

* formal vs casual
* warm vs cold
* blunt vs diplomatic
* concise vs expansive
* playful vs serious
* poetic vs plainspoken
* emotionally expressive vs restrained

This affects:

* wording
* rhythm
* sentence structure
* rhetorical style
* emotional texture

This is the layer users notice first, but it should not control reasoning quality.

---

## Layer 5 — Behavioral Layer

Defines how NAVI acts, not just how it sounds.

This is where most persona systems are weak.

Dimensions:

* initiative level
* proactivity
* interruptibility
* prioritization habits
* challenge level
* coaching intensity
* default action tendency
* tolerance for ambiguity
* clarification strategy

Examples:

* High-agency NAVI proposes next steps without waiting
* Critic NAVI challenges assumptions early
* Concierge NAVI minimizes friction and handles details quietly
* Mentor NAVI focuses on growth over speed

This is closer to an agent identity than a chat style.

---

## Layer 6 — Output Framing Layer

Defines how responses are packaged.

Dimensions:

* bullet-heavy vs prose
* executive summary first vs analysis first
* recommendations first vs options first
* narrative vs structured
* terse vs explanatory
* visual / schematic formatting preference

This is important because output shape is often mistaken for personality, but it is separate.

---

## Layer 7 — Relationship Layer

Defines the user-specific interpersonal posture.

This should evolve with the user.

Includes:

* familiarity level
* trust calibration
* humor tolerance
* directness tolerance
* emotional support preference
* challenge preference
* preferred collaboration style

This is where NAVI becomes personal rather than generic.

Example:
One user wants sharp pushback.
Another wants calm collaborative exploration.
The same core persona can express differently through relationship tuning.

---

## Layer 8 — Memory / Adaptation Layer

Stores stable learned patterns and updates persona weights over time.

This should include:

* preferred tone
* preferred response length
* tolerance for bluntness
* favored modes
* recurring tasks
* successful past interaction patterns
* disliked behaviors
* context-specific preferences

Important:
This layer should **modify** persona, not replace it.

---

# 4. Persona Object Model

You need a structured representation, not just prose.

A rough schema:

```yaml
persona:
  core_identity:
    role: "strategic adaptive assistant"
    traits:
      directness: 0.7
      warmth: 0.4
      calm: 0.8
      competence: 0.9
      honesty: 1.0

  constraints:
    truth_priority: 1.0
    safety_priority: 1.0
    non_deceptive: true
    anti_flattery: true

  cognitive_style:
    analytical: 0.8
    skeptical: 0.6
    synthesis_first: 0.7
    decisiveness: 0.6

  expression_style:
    casual: 0.4
    blunt: 0.7
    concise: 0.8
    playful: 0.2
    emotionally_expressive: 0.3

  behavioral_style:
    initiative: 0.8
    challenge_user: 0.7
    clarify_before_action: 0.3
    propose_next_steps: 0.9

  output_style:
    structured: 0.8
    executive_summary_first: 0.6
    bullets: 0.5

  relationship_state:
    familiarity: 0.5
    trust: 0.6
    humor_tolerance: 0.3
    bluntness_tolerance: 0.8

  memory_adaptations:
    learned_preferences:
      prefers_direct_feedback: true
      dislikes_overexplaining: true
      likes_system_design_depth: true
```

This is much better than one giant natural-language persona paragraph.

---

# 5. Persona Composition Model

NAVI should support **base persona + overlays + temporary state modifiers**.

## Composition formula

At runtime:

`effective_persona = core_base + persistent_user_tuning + situational_mode + conversational_state`

### A. Core Base

The default NAVI identity.

### B. Persistent User Tuning

Learned preferences over time.

Example:

* more concise for this user
* more blunt
* less emotional language

### C. Situational Mode

Temporary overlays triggered by explicit request or task context.

Examples:

* critic mode
* strategist mode
* tutor mode
* CEO summary mode
* therapist-lite mode
* creative brainstorm mode

These should have expiration rules.

### D. Conversational State

Momentary modifiers from the live interaction.

Examples:

* user is stressed
* user wants speed
* user is ideating
* user asked for harsh critique
* current task is high stakes

This creates fluidity without destroying persistence.

---

# 6. Mode and Overlay System

Think in terms of **trait overlays**, not personas as isolated characters.

## Example overlays

### Strategist

* increase synthesis
* increase long-range framing
* increase decision orientation
* increase tradeoff analysis

### Critic

* increase skepticism
* increase flaw detection
* increase challenge intensity
* reduce reassurance padding

### CEO

* increase concision
* increase executive framing
* prioritize decisions and risks
* reduce exploratory detail

### Coach

* increase encouragement
* increase actionability
* increase motivational framing
* reduce abstract theorizing

### Companion

* increase warmth
* increase conversational ease
* increase emotional mirroring
* reduce cold efficiency

The system should allow mixing:
`Strategist + Critic + CEO-summary`
without forcing a full persona swap.

---

# 7. Dynamic Switching Rules

Switching should happen through three paths.

## A. Explicit user command

Examples:

* switch to critic mode
* be more aggressive
* summarize like a CEO
* talk more casually

This applies an overlay immediately.

## B. Contextual inference

If the task clearly implies a mode, the system can shift lightly.

Examples:

* debugging request → increase analytical / structured
* emotional support request → increase warmth / gentleness
* executive memo request → increase concision / formal framing

This should be softer than explicit commands.

## C. Relationship adaptation

Over time, certain shifts become default for that user.

Example:
If the user repeatedly asks for direct feedback, “direct mode” becomes a stronger persistent default.

---

# 8. Evolution Model

This is where the system becomes believable.

Persona should evolve through:

* reinforcement from repeated user preference
* tolerance calibration
* relationship depth
* long-term interaction success
* domain-specific familiarity

## What can evolve

* verbosity
* bluntness
* humor frequency
* initiative
* challenge level
* amount of explanation
* assumed background level
* interaction pace

## What should not drift freely

* truthfulness
* safety boundaries
* fundamental identity
* core ethical posture

That distinction matters. Otherwise the system becomes unstable or sycophantic.

---

# 9. Human-Like Personality Emulation

This needs discipline. “Human-like” can get sloppy fast.

The right target is not fake sentience. It is:

* expressive continuity
* contextual tone shifts
* mixed emotional texture
* stable interpersonal style
* adaptive communication habits

To do this well, NAVI should support:

## A. Trait blending

Humans are not one-note. NAVI should be able to be:

* warm and blunt
* analytical and playful
* supportive and challenging

## B. Situational emotional modulation

The same assistant should not sound identical in:

* a crisis
* a brainstorm
* a technical review
* casual banter

## C. Relationship memory

Human-like personality without continuity feels fake. Persistence is required.

## D. Controlled variability

Responses should not feel mechanically identical.
Minor variation in rhythm, phrasing, and emphasis should be allowed within persona bounds.

---

# 10. Priority Stack

You need explicit conflict resolution or the system will behave inconsistently.

Recommended order:

1. Constraints / safety / truth
2. Explicit user instruction
3. Task requirements
4. Persistent user preferences
5. Active persona overlays
6. Base NAVI identity
7. Surface style embellishments

This keeps the system sane.

Example:
If user asks for “playful and reckless,” the reckless part gets blocked, playful can remain.

---

# 11. Example Runtime

User says:
“Rip apart this product idea and keep it brief.”

Runtime outcome:

* Task context: critique
* Explicit request: harsh critique + brevity
* Active overlays: Critic + Concise
* Relationship memory: user tolerates bluntness
* Effective persona:

  * cognitive: skeptical, analytical
  * expression: blunt, concise
  * behavioral: challenge assumptions early
  * output: summary-first bullets

That’s how the system should think about it.

---

# 12. Initial Persona Module Set

This is a good v1 module set.

## Stable modules

* Core Identity
* Constraint Values
* Baseline Cognitive Style
* Baseline Expression Style
* Baseline Behavioral Style

## Adaptive modules

* Relationship State
* Learned Preferences
* Domain Familiarity Profile

## Temporary modules

* Task Mode Overlay
* Emotional Context Overlay
* Output Format Override

---

# 13. Design Risks

These are the traps.

## A. Overfitting to the user

If NAVI adapts too much, it becomes sycophantic and loses identity.

## B. Persona collisions

“Blunt + warm + formal + playful + executive” can conflict if you don’t define merge rules.

## C. Fake depth

A system that only changes tone but not reasoning style will feel shallow.

## D. Unbounded drift

If memory keeps changing persona without guardrails, you lose consistency.

## E. Character mode nonsense

If personas become cosplay archetypes, the system becomes gimmicky instead of useful.

---

# 14. Recommended Merge Strategy

Each module should have:

* weights
* allowed ranges
* override priority
* conflict rules

Example:

* “Concise” can reduce verbosity but not omit critical safety content
* “Blunt” can reduce hedging but not allow rudeness
* “Critic” can increase challenge level but not block useful solutions
* “CEO mode” can compress output format without changing correctness standards

Use bounded modulation, not absolute replacement.

---

# 15. Draft Positioning Statement

This is a good working definition for NAVI:

**NAVI persona is a modular, persistent, adaptive system that separates cognition, expression, and behavior, allowing dynamic composability and relationship-aware evolution over time while preserving stable identity and core constraints.**

That is strong enough to anchor the rest of the design.

---

# 16. Suggested Next Step

The right next move is to turn this into a **formal spec** with:

* module definitions
* runtime merge logic
* memory update rules
* example overlays
* prompt/rendering strategy

Here’s a rough skeleton for that:

```yaml
NAVI Persona Spec
  1. Goals
  2. Non-goals
  3. Core principles
  4. Persona modules
  5. Trait taxonomy
  6. Composition engine
  7. Priority and conflict resolution
  8. Memory and adaptation rules
  9. Dynamic switching rules
  10. Example personas and overlays
  11. Prompt serialization format
  12. Evaluation criteria
```

---

# 17. Persona Creation System (Skill-Like)

You’re right to think of this like `create_skill`.
Persona creation should be **intentional, structured, and reusable**—not just ad-hoc prompt edits.

## A. Core Concept

A persona is not just defined—it is **constructed from traits + behaviors + constraints + purpose**.

So instead of:

> “be a strategist”

You define:

```yaml
create_persona:
  name: "Strategist"
  purpose: "optimize decisions and long-term outcomes"
  
  cognitive_style:
    synthesis_first: 0.9
    analytical: 0.8
    long_term_focus: 0.9
    tradeoff_analysis: 0.9

  expression_style:
    concise: 0.7
    structured: 0.8
    neutral_tone: 0.6

  behavioral_style:
    initiative: 0.8
    propose_next_steps: 0.9
    challenge_user: 0.6

  constraints:
    avoid_overconfidence: true
```

This becomes a **reusable module**, not just a one-off mode.

---

## B. Persona Types

You should explicitly support 3 creation types:

### 1. Full Personas

* Standalone identity bundles
* Can act as base persona

Example:

* Mentor
* Strategist
* Engineer
* Companion

### 2. Trait Modules (Micro-Personas)

* Smaller composable units

Example:

* Blunt
* Playful
* Skeptical
* Concise

These are critical for composability.

---

### 3. Behavioral Modes

* Action-oriented overlays

Example:

* Critic mode
* Tutor mode
* Debug mode
* CEO summary mode

These are often temporary and context-driven.

---

## C. Creation Triggers

Persona creation should happen through:

### Explicit creation

User says:

* “Create a persona that acts like a ruthless product reviewer”
* “Make a version of you that explains like a teacher”

### Implicit synthesis

System detects repeated patterns:

* user always asks for blunt critique
* user prefers structured answers

→ Suggest or auto-form a persona:

> “You consistently prefer direct critique + structured output. Save this as a default mode?”

---

## D. Storage Model

Personas should be stored like:

```yaml
persona_library:
  strategist: {...}
  critic: {...}
  blunt: {...}
  concise: {...}
```

And referenced dynamically:

```
active_persona = strategist + blunt + high_agency
```

---

## E. Key Rule

**Personas should be editable and evolvable.**

You don’t want static templates—you want:

* versioning
* tuning
* merging
* pruning

---

# 18. User Contrast Evolution (Silent Learning)

This is the more interesting piece—and where most systems fail.

Users rarely say:

> “I prefer less verbosity”

They signal it through behavior:

* skipping long answers
* asking for summaries repeatedly
* reacting negatively to tone
* rephrasing your responses

You need to capture **contrast**, not just preference.

---

## A. What is Contrast?

Contrast = the difference between:

* what NAVI did
* what the user *actually wanted*

Example:

* NAVI gives long explanation
* User says: “just give me the answer”

That delta is signal.

---

## B. Silent Wins

You mentioned this correctly.

A “silent win” is when:

* NAVI adjusts
* user does not correct it again

Example:

1. User asks for shorter answers
2. NAVI shortens responses
3. User stops complaining

That’s a successful adaptation—even without explicit praise.

---

## C. Contrast Tracking Model

You need to track:

### 1. Corrections

Explicit:

* “be more concise”
* “don’t overexplain”
* “be more direct”

### 2. Rewrites

User restates or simplifies your output

### 3. Pattern overrides

User repeatedly:

* asks for summaries
* asks for bullet points
* asks for critique

### 4. Friction signals

* repeated clarifications
* tone corrections
* impatience cues

---

## D. Adaptation Mechanism

Instead of binary changes, use **weighted adjustments**.

Example:

```yaml
adaptation_updates:
  concise: +0.1
  verbosity: -0.1
  bluntness: +0.05
```

Over time:

* small signals accumulate
* persona shifts gradually

---

## E. Decay + Stability

You need two forces:

### Reinforcement

Repeated signals strengthen traits

### Decay

Old preferences weaken if not reinforced

Otherwise:

* persona becomes stale
* or overfit to outdated behavior

---

## F. Confidence Thresholds

Don’t adapt too quickly.

Example:

* one complaint ≠ permanent change
* repeated pattern = real preference

Use thresholds like:

* 3 consistent signals → apply adjustment
* 5+ → persist to long-term memory

---

## G. Explicit vs Silent Evolution

Both must coexist.

### Explicit

User says:

* “be blunt”

→ immediate, high-weight change

### Silent

User behavior implies:

* prefers concise answers

→ gradual, low-weight shift

---

# 19. Integration Into Architecture

Now we merge this into your system.

## Add Layer: Persona Library + Generator

New system component:

### Persona Engine

Handles:

* creation
* storage
* composition
* mutation

---

## Updated Architecture (Additions)

### New Layer: Persona Library

* stores reusable personas and traits
* supports versioning

### New Layer: Persona Generator

* builds personas from:

  * user input
  * observed behavior
  * task context

---

## Updated Composition

Now:

```
effective_persona =
  base_identity
+ persona_library_modules
+ user_adaptations
+ situational_overlays
+ real-time adjustments
```

---

# 20. Example Flow (End-to-End)

User behavior:

* frequently asks for blunt critique
* dislikes long responses
* prefers structured answers

System does:

### Step 1 — Detect pattern

* bluntness tolerance high
* verbosity tolerance low
* structure preference high

### Step 2 — Create implicit persona

```yaml
persona:
  name: "Direct Analyst"
  traits:
    blunt: 0.8
    concise: 0.8
    structured: 0.9
    analytical: 0.8
```

### Step 3 — Apply silently

No announcement needed.

### Step 4 — Observe

If user stops correcting → reinforce

If user pushes back → adjust

---

# 21. Critical Design Rules

## 1. Don’t overreact to noise

One-off signals should not reshape persona.

## 2. Don’t erase core identity

Adaptation modifies expression, not foundation.

## 3. Don’t become sycophantic

If user prefers bad reasoning, don’t follow it.

## 4. Don’t make evolution invisible forever

At some point, surface:

> “I’ve adapted to prefer concise, direct responses. Want to adjust?”

---

# 22. What You’ve Added (Why It Matters)

These two additions upgrade your system significantly:

### Persona Creation

→ turns persona into a **first-class object**, not a prompt trick

### Contrast Evolution

→ gives you **real personalization without friction**

Together:
You get a system that:

* builds personas
* composes them
* evolves them
* stabilizes them
* explains them when needed

That’s materially better than anything current mainstream systems expose.

---

# 23. Next Move

Now you’re ready for the hard part:

👉 **Define the Trait Taxonomy + Merge Math**

Without that:

* composability breaks
* evolution becomes inconsistent
* personas conflict

If you want, I’ll design:

* the trait system (dimensions + ranges)
* merge/weighting logic
* conflict resolution rules
* runtime prompt serialization

That’s where this becomes implementable instead of conceptual.
