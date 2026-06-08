# Wizard Bootstrap Trigger

OMN-8 is not only a frontend prompt-collision bug. The NAVI backend was still seeding brand-new Wizard sessions with a generic chat bootstrap prompt:

`Please greet the user and briefly introduce yourself. Invite them to chat naturally; you are a chat assistant.`

That first-turn instruction contradicted the structured Wizard persona and gave the model room to drift into free-form onboarding behavior.

## What changed

- `internal/navi/runtime_executor.go` now uses a Wizard-specific first-turn trigger:
  - `Begin Phase 1. Welcome me and ask if I am ready to start setup.`
- `internal/navi/loop.go` now applies the same Wizard-specific trigger for the legacy loop path.
- Regression tests were added so new Wizard sessions cannot silently fall back to the generic chat-assistant opener.

## Why this matters

Even with a strong `config/personas/wizard.yaml`, a conflicting first user message can still steer the model away from the intended onboarding flow. The bootstrap trigger needs to reinforce the phase-structured setup behavior, not compete with it.

## Remaining gap

The PET-specific parts of OMN-8 described in Linear still live outside this repository. This patch fixes the backend Wizard bootstrap path that exists here, but the PET-side prompt and suggestion-chip alignment still need to be handled in the frontend workspace.
