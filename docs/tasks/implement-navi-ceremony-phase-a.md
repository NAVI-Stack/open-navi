# Task: Implement NAVI Ceremony Phase A

Status: Ready for implementation planning
Scope: Phase A only
Primary design reference: `docs/design/navi-ceremony.md`

## Objective

Implement the Phase A NAVI Ceremony: a short post-required-onboarding relationship ceremony that makes NAVI feel personally claimed by the owner without complicating the existing required onboarding flow.

The implementation must follow this sequence:

> Required onboarding → NAVI Ceremony → Main app / console

Do not implement the future floating companion overlay in this task.

## Product Intent

Required onboarding makes NAVI work.

The Ceremony makes NAVI yours.

The Ceremony should feel like a polished first connection, not a settings wizard. It should create a memorable moment where the owner shapes how NAVI should address them, show up, ask before acting, and begin learning from owner-provided personalization.

The emotional target is:

> This is mine.
> It understands how I want to be treated.
> It will learn, but I remain in control.
> This is the beginning of my relationship with NAVI.

## Required References

Read these before implementing:

1. `docs/design/navi-ceremony.md`
2. Existing onboarding docs, especially anything under:
   - `docs/concepts/onboarding-*`
   - `docs/design/onboarding-*`
   - `docs/tasks/*onboarding*`
3. Existing experience/persona docs, especially:
   - `docs/design/experience-layer.md`
   - `docs/specs/persona-system.md`
   - `docs/design/persona-system-architecture.md`
   - `docs/design/persona-storage-schema.md`
4. Existing runtime code related to experience configuration, especially:
   - `internal/navi/experience/`
   - `internal/gateway/experience.go`
   - relevant store/schema files

## Hard Scope Boundaries

Build Phase A only.

### Must Build

- Post-required-onboarding Ceremony route/screen.
- 3–5 step Ceremony flow.
- Minimal Ceremony journey state machine.
- Owner display name capture through Ceremony only.
- NAVI presence preference selection.
- Trust/boundary defaults selection.
- Optional first personalization seed capture.
- Generated NAVI Pact summary.
- Completion and skip behavior.
- Route guard so completed users do not repeatedly see the Ceremony.
- Settings/profile entry point to revisit or adjust Ceremony-derived choices later.
- Tests for the route/state/persistence behavior.

### Must Not Build

- Floating NAVI companion overlay.
- Full generalized User Journey System.
- Automatic re-prompting for skipped Ceremony.
- Persisted Pact artifact/history item.
- Full memory management dashboard.
- Advanced autonomy editor.
- Plugin/capability permission editor.
- New personality storage system separate from the existing experience/persona foundation.
- Fake consciousness/personhood ritual.

## Implementation Research Pass

Before coding, inspect the repo and answer these in implementation notes or commit/PR description:

1. Where does required onboarding currently route after completion?
2. What route/component should host the Ceremony?
3. Where should `ceremonyJourneyState` live for Phase A?
4. Does an owner/contact entity exist by the time Ceremony runs?
5. Which existing experience config APIs/fields should receive the NAVI presence mapping?
6. Where should trust/boundary defaults be persisted today?
7. What is the leanest safe place to store or route the first personalization seed?
8. How should settings/profile expose "revisit Ceremony" without pretending the first-run ritual can be perfectly recreated?

Do not block implementation on creating perfect abstractions. Prefer the smallest compatible path that leaves a clear seam for future User Journey System migration.

## Phase A Flow

### Step 1 — Transition from Setup

After required onboarding completes, route the owner to the Ceremony unless they have already completed or skipped it.

Copy direction:

> You're set up. Now let's make NAVI yours.

Actions:

- Begin Ceremony
- Skip for now

Skip must set Ceremony state to `skipped` and continue to the main app / console.

### Step 2 — Owner Recognition

Prompt:

> What should I call you?

Requirements:

- Do not prefill from auth metadata, email, account name, or required onboarding fields.
- Store as owner-provided display name.
- Treat as owner profile/contact seed, not as generic memory.

### Step 3 — NAVI Presence

Prompt:

> How should I show up for you?

Options:

- Calm and quiet
- Warm and conversational
- Direct and strategic
- Fast and focused
- Balanced

Map this into existing experience/persona configuration. Do not hard-code a separate personality system.

Suggested semantic mapping:

| Choice | Experience Bias |
| --- | --- |
| Calm and quiet | lower verbosity, lower proactive surfacing |
| Warm and conversational | higher warmth, more relational language |
| Direct and strategic | concise, challenging, planning-oriented |
| Fast and focused | action-biased, low ceremony, minimal prose |
| Balanced | default mixed behavior |

Use the existing experience configuration foundation where practical. The current codebase includes owner-scoped experience configuration for core identity, output preferences, relationship profile, and persona modules; integrate rather than duplicate.

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

Persist these as owner-set trust/boundary defaults using the least invasive existing storage path. They should be easy to migrate later into a richer autonomy/user journey model.

Do not present these choices as governance, policy, or permission matrix language in the UI.

### Step 5 — First Personalization Seed

Prompt:

> What's one thing you want me to remember from the beginning?

Requirements:

- Optional.
- Owner-provided.
- Reviewable.
- Routed by semantic type instead of blindly persisted as Memory.

Routing rule:

| Input Type | Storage Target |
| --- | --- |
| Name or preferred address | Owner Profile / Contact |
| Communication preference | Experience / Output Preference |
| Confirmation boundary | Trust Boundary / Autonomy Setting |
| Personal fact | Knowledge or Contact |
| Meaningful life/context statement | Memory |
| Sensitive, ambiguous, conflicting, or high-impact statement | Reviewable candidate |

For Phase A, implement the leanest safe router. It may be conservative. If routing is uncertain, stage as a reviewable candidate rather than silently committing as durable memory.

### Step 6 — NAVI Pact Summary

Generate a summary from the collected choices:

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

On "Looks right":

- Persist choices.
- Mark Ceremony `completed`.
- Continue to main app / console.

On "Adjust":

- Let the owner navigate back through the Ceremony steps and revise choices.

On "Skip for now":

- Mark Ceremony `skipped`.
- Continue to main app / console.

Do not persist the generated Pact summary as a separate artifact/history item in Phase A. Persist the underlying choices only. Regenerate the summary when needed.

## Minimal State Model

Long term, Ceremony state belongs in a broader User Journey System.

For Phase A, implement a narrow state machine only:

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

Requirements:

- Store enough state to resume or safely restart if interrupted.
- Do not build a generic journey framework.
- Shape the model so it can later migrate into a real User Journey System.
- Do not automatically re-prompt skipped users in Phase A.

## Data / Persistence Rules

Owner-provided Ceremony inputs are authoritative, but they must be routed by type.

Do not treat every direct answer as Memory.

Examples:

- "Call me Eric" → Owner Profile / Contact.
- "Be direct with me" → Experience / Output Preference.
- "Ask before sending emails" → Trust Boundary / Autonomy Setting.
- "I'm working through burnout" → likely Memory, sensitive/reviewable.
- "My wife's name is..." → Contact/Knowledge, possibly sensitive depending context.

Persist provenance/source as owner-provided wherever supported.

## UX and Copy Constraints

Tone:

- calm
- direct
- warm
- confident
- not mystical
- not corporate
- not childish

Avoid UI terms such as:

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

## Settings/Profile Revisit

Provide a discoverable way to revisit Ceremony-derived choices after first run.

Do not frame this as re-running the exact first-time Ceremony. The first-run moment is a milestone; later editing should feel like adjusting relationship preferences.

Suggested labels:

- NAVI relationship
- How NAVI shows up
- Trust & memory
- Personalization

## Testing Requirements

Add or update tests for:

1. Required onboarding routes to Ceremony when Ceremony state is `not_started`.
2. Completed Ceremony routes to main app / console and does not repeat.
3. Skipped Ceremony routes to main app / console and does not auto-resurface.
4. Owner display name is not prefilled from auth metadata.
5. Presence choice persists into the chosen experience configuration path.
6. Trust/boundary choices persist with owner-set provenance where supported.
7. Personalization seed routing handles at least:
   - communication preference
   - sensitive/ambiguous input staged as reviewable candidate
   - blank/omitted input
8. Pact summary renders from current Ceremony choices and is not stored as a standalone artifact/history item.

Run the relevant frontend/backend test suites before completing the task. If a full suite is too expensive, run the narrowest meaningful set and document what was not run.

## Acceptance Criteria

The task is complete when:

- Existing required onboarding still works.
- New users reach NAVI Ceremony after required onboarding.
- Ceremony can be completed in under 5 minutes.
- Ceremony can be skipped without breaking the app.
- Completed or skipped Ceremony does not repeatedly appear.
- Ceremony-derived choices persist and can influence NAVI behavior through existing systems where practical.
- No floating companion overlay exists yet.
- No generalized User Journey System was added.
- No separate personality storage model was added.
- Pact summary is generated from choices but not persisted as a standalone artifact/history item.
- Tests cover the major route/state/persistence behaviors.

## Suggested Implementation Plan

1. Inspect current onboarding flow and post-completion routing.
2. Identify the least invasive storage path for Ceremony state.
3. Identify existing experience config APIs for presence mapping.
4. Add Ceremony state model and persistence helpers.
5. Add Ceremony route/screen/components.
6. Add post-onboarding routing gate.
7. Add save/skip/complete handlers.
8. Add settings/profile entry point for later adjustment.
9. Add tests.
10. Update documentation or implementation notes with final storage/mapping decisions.

## Final Guardrail

Do not let this become infrastructure work.

The Ceremony is the first journey item, not the whole journey system.

Build the smallest beautiful Phase A version that preserves the product feeling and leaves clean seams for Phase B/C.
