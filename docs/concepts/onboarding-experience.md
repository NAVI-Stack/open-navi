# Onboarding Experience — Setup and Configuration Only

> **Dedicated system module for installation and configuration. Not for general user interaction.**

**Status:** Active  
**Technical Implementation:** [Experience Layer](../specs/persona-system.md)  
**See also:** [Onboarding Flow](onboarding-flow.md) · [Onboarding UI Representation Guide](../design/onboarding-ui-guide.md) · [Wizard Configuration Flows](wizard-config-flows.md) · [NAVI Onboarding Timeline](../tasks/todo-rework-onboarding.md) · [Skills](skills.md) · [Architecture](../../README.md)

---

## Role

The **Onboarding Experience** is a dedicated system module used exclusively for setup and configuration flows. It is the technical successor to the legacy "Wizard" persona.

- **In scope:** Guiding users through configuration — skills, connectors, integrations, system initialization. Welcome, goals, explain capabilities, one step at a time. When done, call `onboarding_set_setup_complete`.
- **Out of scope:** General conversational assistance. This module does not offer or mention choosing a persona; after setup, users transition to the **Standard Persona System** (Experience Layer).

This module remains available for re-configuration but is distinct from the primary user-facing NAVI interaction model.

---

## Interaction Style

The Onboarding Experience uses a specific set of traits within the Experience Layer to ensure a focused setup journey:

- **Structured, step-by-step:** High `structure_level`, focused on one configuration task at a time.
- **Warm and focused:** Higher `warmth` and proactive `initiative_style`, but only within the onboarding journey.
- **Role ends when setup is complete:** After `onboarding_set_setup_complete`, new sessions use the standard Persona System configuration.

---

## Runtime Activation

- **Gateway Activation:** When the instance requires setup (`state.NeedsWizard()`), the gateway forces the Onboarding Experience module.
- **Session initialization:** If the user hasn't completed setup, the session is initialized with the Onboarding traits regardless of current profile settings.

---

## Summary

| Aspect | Onboarding Experience |
|--------|--------|
| Purpose | Setup and configuration only |
| Persona choice | None — transition to standard Persona System after completion |
| After setup | Standard Persona System (Experience Layer) |
| Style | Step-by-step, one task at a time, completion-focused |
