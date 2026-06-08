# NAVI CLI Onboarding Specification

## Purpose

This document defines the canonical onboarding flow for the NAVI CLI terminal experience. Its purpose is to reduce implementation drift by specifying the exact interaction model, step structure, navigation behavior, data collection rules, and completion criteria for first-run onboarding.

This is the source-of-truth specification for the default CLI onboarding path.

---

## Scope

This specification covers:

* first-run onboarding in the NAVI CLI
* terminal interaction behavior for onboarding
* step-by-step onboarding structure
* prompt, description, and option requirements for each step
* provider and connector configuration within onboarding
* review and confirmation behavior

This specification does **not** define:

* post-onboarding chat UX
* advanced settings screens outside default onboarding
* backend schema design except where onboarding depends on it
* provider or connector implementation internals beyond required onboarding behavior

---

## Core UX Principles

1. **Onboarding must feel like a guided terminal UI, not a questionnaire.**
2. **Every step must present a clear prompt, a short description, and explicit options or inputs.**
3. **Selection should not require the user to type option names when the option list is known.**
4. **The default onboarding path must avoid technical infrastructure jargon.**
5. **The user should only type freeform text when actual text input is required.**
6. **The flow must be valid for non-technical users.**
7. **The onboarding path must collect required configuration during setup rather than forcing the user to remember later commands.**
8. **No success or readiness message may appear before actual successful completion.**

---

## Canonical Interaction Model

### Terminal Navigation Rules

The onboarding UI must use the following baseline control model:

* **Up / Down**: move between options in the current selection list
* **Tab / Left / Right**: navigate sub-selections, grouped controls, or multi-part step focus areas
* **Enter**: confirm the current selection or submit the current step
* **Esc**: back out of the current interactive control when supported
* **Typed input**: used only for text-entry fields

### Required Input Modes

The onboarding system must support these input types:

1. **Confirm selection**

   * Example: Begin setup? Yes / No
   * Uses a selectable control

2. **Single-line text input**

   * Example: Owner name
   * Uses standard text entry

3. **Secret input**

   * Example: Bot token or owner secret
   * Input must be masked

4. **Single select list**

   * Example: AI provider, model, connector
   * Uses selectable options

5. **Review / confirmation panel**

   * Shows collected information before apply
   * Secrets must be redacted

### Step Structure Requirement

Every onboarding step must contain:

* **Prompt**: the main question or action request
* **Description**: one short explanation of why the step matters
* **Options or Input**: the actual control presented to the user

No step should present a raw control without prompt and description context.

---

## Default Onboarding Flow

The default onboarding flow is defined below in strict order.

### Step 1 — Welcome / Begin Setup

**Prompt**
Would you like to begin setting up NAVI?

**Description**
The first person to complete setup becomes this NAVI instance's owner.

**Options**

* Yes
* No

**Behavior**

* Uses selectable confirmation UI
* Must not require typed `y/n`
* If `No`, onboarding exits cleanly without partial configuration

---

### Step 2 — Owner Name

**Prompt**
What should NAVI call you?

**Description**
This name is used to establish initial ownership of the instance.

**Input**

* single-line text input

**Behavior**

* This is the only owner naming prompt in default onboarding
* **Short handle is explicitly excluded** from default onboarding
* Input must be validated as non-empty

---

### Step 3 — Owner Recovery Secret

**Prompt**
How would you like to set your owner recovery secret?

**Description**
This is a break-glass credential used only for recovery and high-level administrative access.

**Options**

* Generate one for me
* Enter one manually

**Behavior**

* Option selection uses selectable UI
* If manual entry is selected, show a masked secret input field
* If generation is selected, the system generates the value automatically

---

### Step 4 — Owner Passport / Recovery Display

**Prompt**
Review your owner recovery details.

**Description**
You must save these details now. They may not be shown again in the same form.

**Display**

* owner name
* instance identifier
* secret fingerprint
* API key or equivalent normal-access credential if applicable
* recovery seed or equivalent recovery credential

**Behavior**

* Must render as a dedicated review-style panel
* This is display-first, not a data-entry step
* Sensitive data must be visually separated and readable

---

### Step 5 — Recovery Confirmation

**Prompt**
Have you saved your recovery details?

**Description**
You need these details to recover access if your normal credential is lost.

**Options**

* Yes, continue
* Go back
* Cancel setup

**Behavior**

* Uses selectable confirmation UI
* Must prevent silent continuation without explicit confirmation

---

### Step 6 — AI Provider Selection

**Prompt**
Which AI provider should NAVI use?

**Description**
Choose the provider NAVI will use for its core reasoning and response generation.

**Options**

* dynamically loaded from provider catalog when available
* may include an explicit Skip option only if supported by product requirements

**Behavior**

* Uses single-select list UI
* The user must not be required to type provider names when discovery succeeds
* If provider discovery fails, a fallback path may allow manual entry

---

### Step 7 — Provider Configuration

**Prompt**
Configure the selected AI provider.

**Description**
NAVI needs the provider model and any required credentials before it can operate.

**Behavior by provider**

#### Ollama path

* attempt provider/model discovery automatically
* show detected models in a selectable list
* if detection fails, allow fallback manual model entry
* if endpoint/path confirmation is needed, ask clearly and only after discovery failure

#### API-based provider path

* request required credentials using masked secret inputs where appropriate
* request model selection from discovered catalog when available
* if catalog is unavailable, allow manual model entry

**Rules**

* Known options should be selectable, not freeform
* Freeform model entry is fallback behavior, not the primary UX
* Provider setup must not block onboarding solely because discovery failed

---

### Step 8 — Connector Selection

**Prompt**
Would you like to connect NAVI to an external platform?

**Description**
This allows NAVI to communicate through supported platforms such as messaging or collaboration tools.

**Options**

* dynamically loaded supported connectors
* None / Skip for now

**Behavior**

* Uses selectable list UI
* Only connectors actually supported by the system may be shown
* No hardcoded unsupported connector options may appear in the onboarding UI

---

### Step 9 — Connector Configuration

**Prompt**
Configure the selected connector.

**Description**
Enter the required details now so NAVI can use the connector immediately after setup.

**Input**

* dynamically generated fields from connector setup schema

**Behavior**

* Required fields must be clearly distinguished from optional fields
* Secret values must use masked input
* Connector-specific information should be collected during onboarding, not deferred to later manual commands
* If connector schema cannot be loaded, onboarding must allow the user to skip connector setup cleanly

**Example connector fields**

* bot token
* bot identifier
* username
* chat ID
* user ID
* webhook or endpoint fields where applicable

---

### Step 10 — Review Configuration

**Prompt**
Review your NAVI setup.

**Description**
Confirm your configuration before NAVI applies it.

**Display**

* owner name
* selected provider
* selected model
* selected connector
* connector configuration summary

**Rules**

* Secrets must be redacted
* Empty optional values should not clutter the display
* The screen must support clear final review without requiring the user to remember prior entries

**Options**

* Confirm and apply
* Go back
* Cancel setup

---

### Step 11 — Apply Setup

**Prompt**
Apply this configuration?

**Description**
NAVI will now save the setup and initialize required systems.

**Options**

* Apply
* Cancel

**Behavior**

* Only after confirmation should configuration be committed
* No premature success messaging is allowed before actual apply success

---

### Step 12 — Initialize NAVI

**Prompt**
Initializing NAVI...

**Description**
NAVI is activating the selected provider and any configured integrations.

**Behavior**

* This step may display progress, loading state, or staged status messages
* Initialization must be tied to actual success/failure of backend application logic

---

### Step 13 — Ready State

**Prompt**
NAVI is ready.

**Description**
Setup is complete and NAVI can now be used.

**Behavior**

* This screen must appear **only after** successful completion of apply and initialization
* It must never appear before actual success

---

## Explicit Exclusions From Default Onboarding

The following items are **not part of default onboarding**:

1. **Preferred short handle**
2. **Security mode selection** such as local CLI only / local + web / local + network
3. Advanced infrastructure or networking details unless absolutely required after a failed automatic/default path
4. Post-onboarding command education beyond minimal next-step guidance

These may exist later in settings or advanced configuration flows, but they are intentionally excluded from the default onboarding path.

---

## Required Behavioral Rules

### No Fake Options

The onboarding UI must not present options that the system cannot actually support.

Examples:

* unsupported connectors must not be shown
* providers must not be shown if they cannot be selected or configured in the current environment

### No Fake Confirmation

If a step asks the user to confirm or cancel, the result must actually affect control flow.

### No Fake Success

Readiness, activation, or completion messages must only appear after real success.

### No Silent Persistence of Failed Input

Attempted but failed selections must not appear in the review screen as if they were accepted.

### Discovery First, Manual Fallback Second

When the system can discover providers, models, or connector schemas automatically, onboarding must prefer discovery-driven selection.

Manual entry is a fallback path.

---

## Data Model Requirements

The onboarding flow must produce a structured result containing at minimum:

* owner name
* owner secret mode (generated or manual)
* owner secret value or generated secret reference
* recovery confirmation state
* selected provider
* selected model
* provider credentials if needed
* selected connector
* connector parameter map
* final confirmation state

The structured result must **not** include:

* short handle
* security mode selection from the old default onboarding path

---

## Review Screen Redaction Rules

The review step must redact secret values consistently.

### Redaction requirements

* full bot tokens must not be shown
* API keys must not be shown in full
* owner secrets must not be shown in full
* recovery seeds must not be shown in full on final configuration review screens

### Acceptable redaction patterns

* masked entirely
* partially masked with a short visible suffix or fingerprint

---

## Error Handling Requirements

If any discovery-dependent step fails, the onboarding must remain usable.

### Provider/model discovery failure

* show a clear message
* allow manual provider/model input when allowed
* do not terminate onboarding unnecessarily

### Connector schema discovery failure

* show a clear message
* allow connector setup to be skipped cleanly
* do not fabricate fields

### Validation failure

* keep the user on the relevant step
* show the reason briefly
* preserve previously entered values where safe

---

## Implementation Guidance

This section is non-normative but strongly recommended.

### Recommended package boundaries

* `internal/cliui` should own form rendering, onboarding state, and review rendering
* business logic such as persistence, owner creation, and session startup should remain outside `cliui`
* `cliui` may perform lightweight discovery fetches needed to populate forms

### Recommended architectural split

* form wrappers
* onboarding state model
* owner flow
* provider/model flow
* connector flow
* review rendering
* flow orchestration
* main CLI integration

---

## Verification Checklist

An implementation is compliant with this spec if:

* onboarding uses selectable UI for known choices
* every step has prompt, description, and options/input
* short handle does not appear in default onboarding
* security mode selection does not appear in default onboarding
* provider selection is selectable
* Ollama model selection is selectable when discovery succeeds
* connector selection only shows supported connectors
* connector fields are collected during onboarding
* review screen redacts secrets
* apply occurs only after explicit confirmation
* ready state appears only after actual success

---

## Screen-by-Screen Contract

The following table defines the required onboarding screens and their control contracts.

| Screen ID | Screen Name               | Prompt                                                      | Description                                                                                           | Control Type                       | Options / Input Source                                         | Validation                                                                   | Next                                                                | Back                                       |
| --------- | ------------------------- | ----------------------------------------------------------- | ----------------------------------------------------------------------------------------------------- | ---------------------------------- | -------------------------------------------------------------- | ---------------------------------------------------------------------------- | ------------------------------------------------------------------- | ------------------------------------------ |
| ONB-01    | Begin Setup               | Would you like to begin setting up NAVI?                    | The first person to complete setup becomes this NAVI instance's owner.                                | Confirm select                     | Static options: Yes, No                                        | Required selection                                                           | ONB-02 if Yes; Exit if No                                           | None                                       |
| ONB-02    | Owner Name                | What should NAVI call you?                                  | This name is used to establish initial ownership of the instance.                                     | Text input                         | User text input                                                | Non-empty string                                                             | ONB-03                                                              | ONB-01                                     |
| ONB-03    | Owner Secret Mode         | How would you like to set your owner recovery secret?       | This is a break-glass credential used only for recovery and high-level administrative access.         | Single select                      | Static options: Generate one for me, Enter one manually        | Required selection                                                           | ONB-04A if generated; ONB-04B if manual                             | ONB-02                                     |
| ONB-04A   | Owner Secret Generated    | Your owner recovery secret will be generated automatically. | NAVI will create a secure owner recovery secret for this instance.                                    | Informational confirm              | Static option: Continue                                        | Required confirmation                                                        | ONB-05                                                              | ONB-03                                     |
| ONB-04B   | Owner Secret Manual Entry | Enter your owner recovery secret.                           | Choose a strong secret and store it safely.                                                           | Secret input                       | User secret input                                              | Non-empty secret                                                             | ONB-05                                                              | ONB-03                                     |
| ONB-05    | Owner Passport            | Review your owner recovery details.                         | You must save these details now. They may not be shown again in the same form.                        | Review panel                       | System-generated owner/passport data                           | Required display; no hidden fields omitted                                   | ONB-06                                                              | ONB-04A or ONB-04B                         |
| ONB-06    | Recovery Confirmation     | Have you saved your recovery details?                       | You need these details to recover access if your normal credential is lost.                           | Single select                      | Static options: Yes, continue; Go back; Cancel setup           | Required selection                                                           | ONB-07 if Yes; ONB-05 if Go back; Exit if Cancel                    | ONB-05                                     |
| ONB-07    | Provider Selection        | Which AI provider should NAVI use?                          | Choose the provider NAVI will use for its core reasoning and response generation.                     | Single select                      | Dynamic provider catalog; optional Skip only if product allows | Required selection unless explicit Skip exists                               | ONB-08                                                              | ONB-06                                     |
| ONB-08    | Provider Configuration    | Configure the selected AI provider.                         | NAVI needs the provider model and any required credentials before it can operate.                     | Mixed form                         | Dynamic model catalog and provider-specific required inputs    | Valid provider config; model required unless provider explicitly allows none | ONB-09                                                              | ONB-07                                     |
| ONB-09    | Connector Selection       | Would you like to connect NAVI to an external platform?     | This allows NAVI to communicate through supported platforms such as messaging or collaboration tools. | Single select                      | Dynamic connector schema list plus None / Skip for now         | Required selection                                                           | ONB-10 if connector chosen; ONB-11 if None/Skip                     | ONB-08                                     |
| ONB-10    | Connector Configuration   | Configure the selected connector.                           | Enter the required details now so NAVI can use the connector immediately after setup.                 | Dynamic form                       | Connector setup schema fields                                  | All required fields valid; optional fields may be empty                      | ONB-11                                                              | ONB-09                                     |
| ONB-11    | Review Configuration      | Review your NAVI setup.                                     | Confirm your configuration before NAVI applies it.                                                    | Review panel with confirm controls | Structured onboarding state with secret redaction              | Required explicit choice                                                     | ONB-12 if Confirm; prior relevant screen if Go back; Exit if Cancel | ONB-10 or ONB-09                           |
| ONB-12    | Apply Setup               | Apply this configuration?                                   | NAVI will now save the setup and initialize required systems.                                         | Confirm select                     | Static options: Apply, Cancel                                  | Required selection                                                           | ONB-13 if Apply; Exit if Cancel                                     | ONB-11                                     |
| ONB-13    | Initializing NAVI         | Initializing NAVI...                                        | NAVI is activating the selected provider and any configured integrations.                             | Progress / status view             | Real backend apply/init state                                  | Must reflect actual state only                                               | ONB-14 on success; error state on failure                           | ONB-11 only if apply has not committed yet |
| ONB-14    | Ready State               | NAVI is ready.                                              | Setup is complete and NAVI can now be used.                                                           | Final status screen                | System state                                                   | Success only; must not appear on failure                                     | Enter normal post-onboarding flow                                   | None                                       |

## Transition Rules

1. `Back` must return the user to the logically previous editable screen without silently discarding valid prior input.
2. `Cancel setup` must exit cleanly without presenting the instance as configured.
3. `Go back` from a review screen must return to the most relevant prior editable screen for the current branch of the flow.
4. If connector setup is skipped, the review screen must show connector state as omitted or not configured, not failed.
5. If provider/model discovery fails but manual fallback succeeds, the review screen must show the final accepted values only.

## Validation Rules by Control Type

### Confirm select

* A valid selection is required before advancing.

### Text input

* Empty or whitespace-only values are invalid unless the field is explicitly optional.

### Secret input

* Empty values are invalid when the field is required.
* Secret values must not be echoed in plain text after submission.

### Single select

* The selected value must exist in the presented option set, unless the UI is explicitly in manual fallback mode.

### Dynamic form

* Required fields must be clearly labeled.
* Optional fields must remain skippable.
* Validation errors must be shown inline or immediately adjacent to the field context.

## Option Source Rules

1. Static options must be defined in code and not inferred from prior prompts.
2. Dynamic provider options must come from provider discovery/catalog when available.
3. Dynamic model options must come from provider-specific discovery when available.
4. Dynamic connector options must come from connector setup schema or capabilities discovery.
5. When discovery fails, fallback mode must be explicit in the UI and not silently substitute guessed values.

## Review Rendering Rules

1. The review screen must show only accepted, validated state.
2. Rejected or failed attempted selections must never appear as active configuration.
3. Secret fields must be redacted consistently.
4. Optional empty fields should be omitted or collapsed.
5. If a connector is skipped, the review screen should show `None` or `Not configured`, not an error label.

## Failure-State Rules

1. If apply fails after confirmation, the user must be shown a failure state that clearly indicates setup did not complete successfully.
2. The Ready State screen must never be shown on failure.
3. If a failure occurs before configuration is committed, the user may be allowed to return to the review screen.
4. If a failure occurs after partial commit, the UI must not pretend rollback happened unless rollback actually occurred.

## Change Control

Any change to the default onboarding flow must update this document first or at the same time.

If implementation behavior conflicts with this specification, the conflict must be treated as drift and resolved explicitly.
