# NAVI Ceremony — First-Run Relationship Design

Status: Draft
Phase: Product / UX Design
Target Implementation: Phase A first, Phase C later
Owner: NAVI Console / Experience Layer

## 1. Purpose

The NAVI Ceremony is the first-run relationship moment that makes NAVI feel personally claimed by the owner.

Required onboarding makes NAVI work.

The Ceremony makes NAVI yours.

The Ceremony creates a short, polished, emotionally memorable first connection without turning onboarding into a long configuration wizard. It seeds NAVI's early understanding of the owner, NAVI's preferred presence, trust boundaries, and the first owner-provided personalization facts.

The Ceremony is not a replacement for required onboarding. It is a relational layer that follows required setup.

## 2. Product Thesis

NAVI should not begin like a sterile chatbot or a settings panel. The first-run experience should feel like the owner is forming a personal AI, not configuring generic software.

The target emotional outcome:

> This is mine.
> It understands how I want to be treated.
> It will learn, but I remain in control.
> This is the beginning of my relationship with NAVI.

The Ceremony should deliver ritual immediacy and user agency while remaining compatible with NAVI's existing Experience Layer, World Model, memory model, autonomy model, and onboarding flow.

## 3. Naming

Product-facing name:

**NAVI Ceremony**

Internal descriptive name:

**First-Run Relationship Ceremony**

Avoid these terms in user-facing UI:

- emotional bonding onboarding
- relationship contract
- personality configuration
- persona setup
- governance setup

"Relationship Contract" may remain useful as an internal architecture term, but the user-facing experience should feel warmer and less legalistic.

## 4. Placement

### Phase A — Initial Implementation

Use the safe sequence:

1. Required onboarding
2. NAVI Ceremony
3. Main app / console

Phase A intentionally avoids the floating companion overlay. It keeps the existing onboarding path stable and reduces implementation risk.

### Phase C — Future Experience

After Phase A is validated, add a quiet floating NAVI companion during required onboarding.

The companion should be present but non-invasive. It should not auto-open, nag, or behave like a website support widget.

The final Ceremony remains a focused concluding moment even after the companion overlay exists.

## 5. Non-Goals

The Ceremony must not become:

- a second onboarding system
- a long personality quiz
- a replacement for model/provider/account setup
- a full autonomy configuration matrix
- a memory management dashboard
- a capability/plugin permission setup flow
- a fake consciousness/personhood ritual
- a forced emotional interaction
- a generalized user journey framework in Phase A

The Ceremony should be skippable, revisitable, and lightweight.

## 6. Experience Principles

### 6.1 Short

Target duration: 2–5 minutes.

Maximum core steps: 3–5.

### 6.2 Personal

Use direct, relationship-oriented language.

Bad:

> Configure assistant preferences.

Good:

> How should I show up for you?

### 6.3 Empowering

The owner should feel ownership, not surveillance.

NAVI should communicate:

> You shape how I learn and how I act.

### 6.4 Calm

Do not overload the moment with dense settings, policy language, technical terminology, or emotional theatrics.

### 6.5 Correctable

NAVI's first understanding must be presented as editable.

The Ceremony should end with:

> Does this feel right?

Not:

> Setup complete.

## 7. Ceremony Flow — Phase A

### Step 1 — Transition from Setup

After required onboarding completes:

> You're set up. Now let's make NAVI yours.

Primary action:

> Begin Ceremony

Secondary action:

> Skip for now

This makes the Ceremony recommended, not coercive.

### Step 2 — Owner Recognition

Prompt:

> What should I call you?

Input:

- preferred display name

Do not prefill this from auth metadata or required onboarding, even if an email or account name exists. Identity gathered through authentication does not feel relational. NAVI should learn the owner's preferred name through the Ceremony.

Optional later fields, not required in Phase A:

- pronouns
- timezone
- preferred formality
- pronunciation

Output:

- owner profile seed
- owner contact display name

### Step 3 — NAVI Presence

Prompt:

> How should I show up for you?

Options:

- Calm and quiet
- Warm and conversational
- Direct and strategic
- Fast and focused
- Balanced

These should map to Experience Layer defaults, not hard-coded chatbot personalities.

Potential internal mapping:

| User Choice | Experience Bias |
| --- | --- |
| Calm and quiet | lower verbosity, lower proactive surfacing |
| Warm and conversational | higher warmth, more relational language |
| Direct and strategic | concise, challenging, planning-oriented |
| Fast and focused | action-biased, low ceremony, minimal prose |
| Balanced | default mixed behavior |

### Step 4 — Trust and Boundaries

Prompt:

> What should I ask before doing?

Selectable examples:

- Sending messages
- Changing files
- Making purchases or subscriptions
- Remembering sensitive personal details
- Acting on inferred preferences
- Interrupting proactively
- Making external changes

This is the user-facing trust layer.

Internally, this maps to:

- autonomy settings
- confirmation thresholds
- memory promotion sensitivity
- interruption preferences
- owner-set configuration
- proposal generation thresholds

Do not expose those internal terms during the Ceremony.

### Step 5 — First Personalization Seed

Prompt:

> What's one thing you want me to remember from the beginning?

This should be optional.

Placeholder examples:

- I prefer direct answers.
- I value privacy.
- Keep explanations short unless I ask.
- Help me stay focused.
- I like thoughtful pushback.
- Do not assume personal details.

This is owner-provided and authoritative, but it should not always be stored as a Memory. The system should route it by semantic type.

Routing examples:

| Input Type | Storage Target |
| --- | --- |
| Name or preferred address | Owner Profile / Contact |
| Communication preference | Experience / Output Preference |
| Confirmation boundary | Trust Boundary / Autonomy Setting |
| Personal fact | Knowledge or Contact |
| Meaningful life/context statement | Memory |
| Sensitive, ambiguous, conflicting, or high-impact statement | Reviewable candidate |

Output:

- personalization seed
- owner-set provenance
- routing decision
- reviewability flag

### Step 6 — The NAVI Pact Summary

NAVI generates a concise summary:

> Here's what I understand so far:
> I’ll call you [Name].
> I’ll usually show up [Presence].
> I’ll ask before [Boundaries].
> I’ll remember [Personalization Seed].
> I’ll learn carefully, and you can correct what I know anytime.

Actions:

- Looks right
- Adjust
- Skip for now

This is the emotional lock-in moment.

Do not end with a generic success toast. End with a relationship confirmation.

Potential final line:

> Good. I’m ready.

Use sparingly. Keep it polished.

## 8. Data Created

Phase A should create only the minimum necessary seeds.

### 8.1 Ceremony Journey State

Long term, Ceremony state belongs in a broader User Journey System.

That future system should track first-run setup, skipped steps, future reactivation prompts, major upgrade journeys, required migrations, new feature introductions, and relationship milestones.

Do not build that generalized system in Phase A.

Phase A should use a narrow state machine shaped so it can later migrate into a real User Journey System without changing the Ceremony product flow.

```ts
ceremonyJourneyState: {
  journeyId: "navi_ceremony.v1"
  status: "not_started" | "in_progress" | "completed" | "skipped"
  currentStep?: string
  completedSteps: string[]
  startedAt?: string
  completedAt?: string
  skippedAt?: string
  version: "v1"
}
```

### 8.2 Owner Profile Seed

```ts
ownerProfileSeed: {
  displayName: string
  createdFrom: "navi_ceremony"
  ownerSet: true
}
```

### 8.3 NAVI Presence Preference

```ts
naviPresencePreference: {
  mode: "calm_quiet" | "warm_conversational" | "direct_strategic" | "fast_focused" | "balanced"
  createdFrom: "navi_ceremony"
  ownerSet: true
}
```

### 8.4 Trust Boundary Defaults

```ts
trustBoundaryDefaults: {
  confirmBeforeSendingMessages?: boolean
  confirmBeforeChangingFiles?: boolean
  confirmBeforePurchases?: boolean
  confirmBeforeRememberingSensitiveDetails?: boolean
  confirmBeforeActingOnInferredPreferences?: boolean
  confirmBeforeInterruptingProactively?: boolean
  confirmBeforeExternalChanges?: boolean
  createdFrom: "navi_ceremony"
  ownerSet: true
}
```

### 8.5 Personalization Seed

```ts
personalizationSeed: {
  text: string
  source: "owner_input"
  createdFrom: "navi_ceremony"
  ownerSet: true
  routedTo?: "owner_profile" | "experience_preference" | "trust_boundary" | "knowledge" | "memory" | "review_candidate"
  reviewable: true
}
```

## 9. Pact Persistence Decision

Phase A should not persist the generated NAVI Pact as a separate artifact or history item.

Persist the underlying ceremony choices and journey state. Regenerate the Pact summary from current settings when needed.

Future work may persist the Pact as the first relationship artifact/history item. That future artifact could serve as a reflection anchor, milestone, or recap source.

This is intentionally deferred to keep Phase A lean.

## 10. Architecture Mapping

### 10.1 Experience Layer

Owns:

- how the Ceremony is presented
- tone of the Ceremony
- NAVI presence preference
- default persona/experience bias
- future companion overlay behavior

The Experience Layer should not own durable truth. It applies and presents behavior.

The codebase already has an `internal/navi/experience` foundation with persisted owner-scoped configuration for core identity, output preferences, relationship profile, and persona modules. The Ceremony should integrate with that foundation rather than inventing a separate personality store.

### 10.2 World Model

Owns:

- owner contact seed
- relationship-relevant facts
- memory-like personalization seeds
- future relationship history
- derived User Model inputs

The User Model should remain derived, not stored as a single standalone blob.

### 10.3 Memory / Context Orchestration

Owns:

- whether a personalization seed becomes Memory, Configuration, Knowledge, Contact, or a review candidate
- review/edit/delete behavior
- later memory promotion
- visible "what NAVI learned" review surfaces

### 10.4 Autonomy / Confirmation

Owns:

- translating trust boundary choices into confirmation behavior
- proposal thresholds
- irreversible action confirmation
- owner-set boundary enforcement

Do not expose this as "governance" in the Ceremony UI.

## 11. Skipped Ceremony Behavior

Phase A behavior:

- If the owner skips the Ceremony, do not automatically re-prompt.
- Store `ceremonyJourneyState.status = "skipped"`.
- Provide a settings/profile entry point to revisit later.

Long-term behavior:

- Once a User Journey System exists, a skipped Ceremony may become an outstanding journey item that can resurface naturally later.
- Do not implement resurfacing in Phase A.

## 12. Future Phase C — NAVI Companion Overlay

After Phase A is validated, introduce the companion overlay.

### Behavior

- Bottom-right floating NAVI button or orb
- Quiet by default
- No auto-open
- No aggressive greeting bubble
- Context-aware when opened
- Can answer onboarding questions
- Can help explain choices
- Can collect Ceremony-relevant preferences conversationally
- Must never block form progress

### Mobile

Use a bottom sheet instead of a floating desktop-style modal.

### Product Feel

It should feel like NAVI is present during setup, not like a support chatbot.

Bad:

> Hi! Need help?

Better:

> I’m here if you want help shaping this.

## 13. Implementation Phases

### Phase A — Post-Onboarding Ceremony

Build:

- Ceremony route/screen after required onboarding
- 3–5 step Ceremony flow
- minimal Ceremony journey state machine
- owner display name capture through Ceremony only
- experience preference mapping into existing experience configuration
- trust/boundary defaults
- personalization seed router
- generated but non-persisted NAVI Pact summary
- completion/skip state
- route guard so completed users do not see it repeatedly
- settings/profile entry point to revisit later

Do not build:

- floating companion overlay
- generalized User Journey System
- automatic skipped-Ceremony resurfacing
- Pact artifact/history persistence
- full memory dashboard
- advanced autonomy editor
- plugin permission editor
- new personality storage model

### Phase B — Review and Refinement

Evaluate:

- completion rate
- skip rate
- time to complete
- user corrections on summary
- whether users understand what NAVI will remember
- whether the Ceremony feels memorable or like a settings form

Refine copy, pacing, and visual treatment.

### Phase C — Companion Overlay

Build:

- quiet floating NAVI companion during required onboarding
- contextual chat modal / bottom sheet
- ability to explain setup choices
- possible prefill into Ceremony
- handoff from companion into final NAVI Ceremony

## 14. UX Acceptance Criteria

The Ceremony is successful when:

- it does not interfere with required onboarding
- it can be completed in under 5 minutes
- it feels personal, not administrative
- it creates a clear "NAVI is mine" moment
- it produces durable initial personalization seeds
- it makes memory feel reviewable and user-owned
- it avoids technical language during the emotional moment
- it can be skipped without breaking the app
- it can be revisited later

## 15. Copy Direction

Tone:

- calm
- direct
- warm
- confident
- not mystical
- not corporate
- not childish

Avoid:

- Configure
- Assistant preferences
- Governance
- Policy
- Persona matrix
- Memory object
- User model

Prefer:

- How should I show up?
- What should I ask before doing?
- What should I remember from the beginning?
- Does this feel right?
- You can correct me anytime.

## 16. Open Implementation Questions

These should be answered during implementation planning, not product design:

1. Which existing store/table should hold the Phase A `ceremonyJourneyState`?
2. Which exact existing experience config fields should receive the NAVI presence mapping?
3. Does the owner contact entity already exist at the moment Ceremony runs, or should Ceremony create it if missing?
4. What service owns personalization seed routing in the current codebase?
5. Should the Ceremony be implemented in the web console only first, or also exposed through gateway APIs immediately?
6. How should settings/profile expose "revisit Ceremony" without implying that the first-run moment can be perfectly recreated?

## 17. Hard Recommendations

1. The Ceremony is the first journey item, not the whole journey system.
2. Phase A must use a narrow state machine, not a generalized journey framework.
3. Owner name must be learned through Ceremony, not copied from auth metadata.
4. Owner-provided inputs are authoritative but must be routed by semantic type.
5. The generated Pact should be non-persisted in Phase A; persist underlying choices only.
6. Skipped Ceremony should not auto-resurface until the future User Journey System exists.

## 18. Locked Direction

Build Phase A first:

> Required onboarding → NAVI Ceremony → Main app

Hold Phase C for later:

> Required onboarding with quiet NAVI companion → final Ceremony → Main app

The Ceremony must remain short, polished, optional-but-recommended, and emotionally meaningful.

Guiding principle:

> Setup makes NAVI work.
> Ceremony makes NAVI yours.
