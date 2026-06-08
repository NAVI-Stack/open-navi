# NAVI Onboarding Flow — Revised Spec (v2)

> **Status:** Draft — revised from v1 based on gap analysis  
> **Scope:** Full onboarding wizard flow, auth model, recovery, and quick setup mode  
> **Persona:** NAVI speaks through the Wizard persona exclusively during onboarding

---

## Overview

Onboarding transforms a blank NAVI instance into a configured, owner-bound system. It is
a one-time, sequential wizard flow with checkpointing, resume support, and rollback safety.
The Wizard persona is the exclusive interface from first boot until Phase 10 completes.

Onboarding produces four durable outputs:
1. A committed `config.json` (written atomically at Phase 10)
2. An owner identity record (owner_id, keypair public half, device fingerprint)
3. A primary API key (the working credential for all normal access)
4. An Owner Passport (portable ownership and recovery artifact)

---

## Onboarding State Machine

NAVI tracks onboarding state explicitly. On every boot, NAVI checks this state before
doing anything else.

| State              | Condition                                              | Boot Action                                      |
|--------------------|--------------------------------------------------------|--------------------------------------------------|
| `NOT_STARTED`      | No config, no owner                                    | Begin wizard from Phase 1                        |
| `IN_PROGRESS`      | Checkpoint exists, no completion flag                  | Offer resume or restart                          |
| `AWAITING_CONFIRM` | Config staged, owner created, Phase 9 not confirmed    | Jump to Phase 9 summary                          |
| `COMPLETE`         | `onboarding_complete == true`                          | Normal operation, Wizard retired                 |
| `CORRUPT`          | Config exists but fails schema/checksum validation     | Surface recovery options (see Phase 0 — Corrupt) |


### Checkpoint Model

- Each phase writes its output to a **staging area**, not the live config.
- Only Phase 10 performs an atomic commit to the live config.
- An interrupted onboarding never produces a partial live config — it either completes fully or rolls back cleanly.
- Checkpoint format: `navi/.onboarding/checkpoint.json` (deleted on completion or full reset)

### Session Expiry

- If the wizard session is idle for **15 minutes**:
  - **Before passport creation:** Discard all state, require full restart. No partial secrets persist.
  - **After passport creation, before Phase 10:** Persist checkpoint. Require owner secret re-entry to resume (proves the same person is continuing).
- On resume, NAVI replays from the last **confirmed** phase, not the last screen reached.

---

## Phase 0 — System Boot

**Trigger:** NAVI starts.

**Actions:**

1. Check onboarding state (see state machine above).
2. Route accordingly.

### State: NOT_STARTED
Begin wizard. Activate Wizard persona. Set `onboarding_mode = true`.

### State: IN_PROGRESS

```
"It looks like a previous setup was started but not completed.

Would you like to resume where you left off, or start over?

[Resume Setup]  [Start Over]
```

- **Resume:** Replay from last confirmed checkpoint phase.
- **Start Over:** Discard checkpoint, begin from Phase 1.

### State: AWAITING_CONFIRM

```
"Your setup is nearly complete. Let's review your configuration before activating."
```

Jump directly to Phase 9.

### State: CORRUPT

```
"I detected a problem with my configuration file and cannot start normally.

Options:
  [Restore from Owner Passport]
  [Reset and Start Over]
```

- **Restore from Passport:** Enter `navi init --restore <passport_file>` flow (see Migration section).
- **Reset:** Wipe config, begin onboarding from Phase 1.


---

## Phase 1 — First Contact

**Goal:** Establish context and user intent.

NAVI initiates the conversation.

```
"Hello. I am NAVI.

This is a new instance and I have not been configured yet.

The first person who completes this setup becomes my owner. My owner
controls access, permissions, and all configuration.

Would you like to begin setup?

[Begin Setup]  [Learn More]"
```

- **Begin Setup:** Proceed to Phase 2.
- **Learn More:** Brief explanation of what onboarding does and what the owner role means,
  then re-prompt.

---

## Phase 2 — Owner Identity

**Goal:** Establish a verifiable owner identity. This is not about collecting personal
information — it is about creating a binding between a person and this NAVI instance.

### Fields Collected

| Field              | Purpose                                                        | User-facing? |
|--------------------|----------------------------------------------------------------|--------------|
| `owner_name`       | Human-readable display label                                   | Yes          |
| `owner_handle`     | Normalized canonical identifier used in logs and audit trail   | Yes          |
| `device_fingerprint` | Hardware/OS anchor for the originating device               | No (silent)  |

`owner_handle` is normalized on input: lowercase, no spaces, alphanumeric and hyphens only.
`device_fingerprint` is captured silently and written to the audit log — not surfaced to the user.

### Example Interaction

```
"What should I call you?"
> Eric

"What handle would you like to use? This will appear in logs and audit records.
  (lowercase, no spaces)"
> eric
```

NAVI normalizes the handle and confirms:

```
"Got it. I will refer to you as Eric (handle: eric).

You will become my owner. In the next step, you will create your owner secret —
the root credential that proves your ownership of this instance."

[Continue]
```

### Notes
- The wizard does not proceed past this phase until at minimum `owner_handle` is set.
- `owner_name` defaults to `owner_handle` if left blank.
- No email, no password, no account creation — identity is established by the keypair
  and owner secret created in Phase 3.


---

## Phase 3 — Owner Secret & Keypair

**Goal:** Create the root ownership credential and generate the primary API key.

### Step 1 — Owner Secret

```
"Now you will create your owner secret.

Your owner secret is the root credential for this instance. It is used for:
  - Proving ownership
  - Recovery if you lose your API key
  - Privileged actions like resetting access

It is NOT your day-to-day login. After setup, you will use an API key for
normal access. Treat your owner secret like a master key — store it somewhere
safe and do not share it.

Create your owner secret:"
> ••••••••••••••••
"Confirm:"
> ••••••••••••••••
```

NAVI derives a keypair from the owner secret (or uses it to protect a locally generated keypair).
The private key never leaves the owner's device. NAVI stores only the public key and a
fingerprint of the owner secret.

### Step 2 — Primary API Key Generation

NAVI immediately generates the first API key:

```
"Your owner secret has been set.

I have generated your primary API key. This is your normal credential for
API access, the CLI, the web console, and connectors.

  Primary API Key: navi_ownr_xxxxxxxxxxxxxxxxxxxx

You will be asked to secure this in the Recovery & Passport step.
Do not close this window."
```

### Artifacts Generated (staged, not committed)

| Artifact                  | Description                                          |
|---------------------------|------------------------------------------------------|
| `owner_id`                | Derived unique identifier for this owner             |
| `instance_id`             | Unique identifier for this NAVI instance             |
| `owner_secret_fingerprint`| Non-reversible fingerprint of the owner secret       |
| `owner_public_key`        | Public half of the owner keypair                     |
| `primary_owner_api_key`   | Working credential for normal access                 |

**Checkpoint written after Phase 3.**


---

## Phase 4 — Security Mode

**Goal:** Define how NAVI accepts connections. This determines which connectors are
available in Phase 5.

```
"How should I accept connections?

  1. Local CLI only
     I will only accept commands from this device's terminal.
     Most secure. No remote access.

  2. Local CLI + Web Console
     I will run a local web interface accessible from this device's browser.
     Convenient for local use. Not accessible from other devices or services.

  3. Local + Network API
     I will accept connections from other devices and services on your network.
     Required for connectors like Discord, Telegram, and remote clients.
     You control which connectors are enabled in the next step."
```

The selected mode gates connector availability in Phase 5:
- Mode 1 → Only CLI connector available
- Mode 2 → CLI and Web Console available
- Mode 3 → All connectors available

**Checkpoint written after Phase 4.**

---

## Phase 5 — Connector Setup

**Goal:** Register which interfaces NAVI should enable on activation.

```
"Which connectors would you like me to enable?
(Connectors are registered now and activated when setup completes.)"
```

### Available Connectors by Security Mode

| Connector     | Mode 1 | Mode 2 | Mode 3 |
|---------------|--------|--------|--------|
| CLI           | ✓      | ✓      | ✓      |
| Web Console   | —      | ✓      | ✓      |
| REST API      | —      | —      | ✓      |
| WebSocket     | —      | —      | ✓      |
| Discord       | —      | —      | ✓      |
| Telegram      | —      | —      | ✓      |
| Slack         | —      | —      | ✓      |

Connectors unavailable for the selected security mode are shown as grayed out with a note:
`"Requires Network API mode"`

### Failure Handling
- If a connector fails to register, it is logged and onboarding continues.
- Failed connectors appear as warnings in the Phase 9 summary.
- They do not block activation but should be resolved after setup.

**Checkpoint written after Phase 5.**


---

## Phase 6 — Model Provider Setup

**Goal:** Connect an AI model provider. This is deferred-required: skipping it puts NAVI
in limited mode where it can manage configuration but cannot respond to conversations.

### Auto-Detection

Before prompting, NAVI checks for a running Ollama instance:

```
"I detected a local Ollama instance running on port 11434.
Would you like to use it as your AI provider?

  [Use Ollama]  [Connect a different provider]  [Skip for now]"
```

If no local model is detected, prompt normally:

```
"Would you like to connect an AI model provider?

If you skip this, I will run in limited mode — I can accept connections and
manage my configuration, but I will not be able to respond to conversations
until a model is connected. You can configure this at any time after setup.

  [OpenAI]  [Anthropic]  [Local (Ollama)]  [OpenRouter]  [Skip for now]"
```

### Provider Configuration

If a provider is selected:
1. Prompt for API key (if required)
2. Run a lightweight connection test
3. On failure:
   ```
   "I could not connect to [Provider]. You can retry, choose a different
   provider, or skip for now and configure this after setup."
   [Retry]  [Choose different]  [Skip]
   ```

### Resulting State

| Choice    | NAVI Mode Post-Setup                                         |
|-----------|--------------------------------------------------------------|
| Configured| Full operation                                               |
| Skipped   | Limited mode — no conversation capability until configured   |

Limited mode is surfaced clearly in the Phase 9 summary and on every boot until resolved.

**Checkpoint written after Phase 6.**

---

## Phase 7 — Feature Configuration

**Goal:** Set NAVI's autonomy level. This maps to Governor policy mode internally.

```
"How autonomous should I be?

  1. Chat Assistant
     I respond when you ask and answer questions. I do not take actions or
     run tasks without being explicitly asked.
     Best for: conversation, research, writing.

  2. Assistive Agent
     I can run tasks and use tools when you ask me to. I will confirm before
     taking significant actions.
     Best for: coding help, file management, scheduled tasks with oversight.

  3. Autonomous Assistant
     I can act on your behalf proactively, run background tasks, and make
     decisions within defined boundaries without asking each time.
     Best for: advanced users who want NAVI to manage workflows independently.

You can change this at any time after setup."
```

**Checkpoint written after Phase 7.**


---

## Phase 8 — Recovery & Owner Passport

**Goal:** Secure recovery materials and generate the Owner Passport. The passport is the
*output* of completing recovery setup — it is not issued before this step.

### Step 1 — Choose Recovery Methods

NAVI encourages at least two methods.

```
"Before I create your Owner Passport, let's make sure you have a way to
recover access if you lose your API key or device.

Please choose at least one recovery method. Two or more are recommended.

  [✓] Recovery Seed Phrase
      A 24-word phrase you store offline. Standard recovery method.

  [ ] Backup Owner API Key
      A second owner-level API key stored separately from your primary key.

  [ ] Encrypted Backup File
      A full encrypted backup of your config and credentials.

  (Your Owner Passport is always generated — it is your portable proof of
  ownership and contains your recovery materials.)"
```

### Step 2 — Recovery Method Setup

Walk through each selected method:

**Recovery Seed:**
```
"Here is your 24-word recovery seed. Write it down and store it somewhere
safe and offline. It will not be shown again.

  word1 word2 word3 word4 word5 word6
  word7 word8 word9 word10 word11 word12
  ...

[I have written it down — Continue]"
```

**Backup API Key:**
```
"I have generated a backup owner API key:

  navi_ownr_yyyyyyyyyyyyyyyyyyyy

Store this separately from your primary key — on a different device or in
a password manager.

[I have saved this key — Continue]"
```

### Step 3 — Seed Spot-Check (if seed selected)

NAVI does not proceed until the user proves they recorded the seed:

```
"To confirm you recorded your seed, please enter words 4, 11, and 19:"
> ______   ______   ______
```

If incorrect:
```
"Those do not match. Please check your notes and try again.
[Show seed again]  [Retry]"
```

### Step 4 — Owner Passport Generation

After recovery is confirmed, NAVI generates the Owner Passport:

```
"Your Owner Passport has been created.

It proves your ownership of this NAVI instance and contains your
recovery materials.

  [Download Passport File]  [Copy to Clipboard]"
```

### Passport Contents

```json
{
  "owner_id": "...",
  "instance_id": "...",
  "owner_handle": "...",
  "owner_secret_fingerprint": "...",
  "owner_public_key": "...",
  "primary_owner_api_key": "navi_ownr_xxxxxxxxxxxxxxxxxxxx",
  "recovery_seed_fingerprint": "...",
  "created_at": "...",
  "onboarding_version": "2.0",
  "signature": "..."
}
```

Raw secrets (seed words, backup key) are included in the downloadable file but not
in the in-memory passport object after generation.

### Step 5 — API Key Confirmation (Required to Proceed)

```
"To confirm you have saved your primary API key, please enter its last 8 characters:"
> ________
```

If incorrect, re-prompt. NAVI does not proceed to Phase 9 until this is confirmed.

**Checkpoint written after Phase 8.**


---

## Phase 9 — Final Confirmation

**Goal:** Present the full configuration for review before committing. Allow targeted
edits without requiring a full restart.

### Summary Display

```
"Here is your configuration. Does everything look correct?

  Owner:          Eric (handle: eric)
  Instance ID:    navi_inst_xxxxxxxx
  Security Mode:  Local + Network API
  Connectors:     CLI, Web Console, REST API
  Model Provider: Ollama (local) ✓ connected
  Autonomy:       Assistive Agent — confirms before significant actions
  Recovery:       Seed phrase ✓  |  Backup API key ✓
  Passport:       Generated ✓

  ⚠ Warnings: None

[Confirm & Activate]  [Edit Configuration]"
```

### Warnings Block

If any issues exist, they appear in the warnings block:

```
  ⚠ Warnings:
    - discord connector failed to register (can be resolved after setup)
    - Model provider not configured (NAVI will start in limited mode)
```

### Edit Configuration

If the user selects Edit, present a phase selector:

```
"Which part would you like to change?

  [Security Mode]
  [Connectors]
  [Model Provider]
  [Autonomy Level]
  [Recovery Methods]"
```

After editing, return to Phase 9 with the updated summary. Phases are re-run individually,
not the full wizard. The checkpoint is updated after each edit.

---

## Phase 10 — NAVI Activation

**Goal:** Commit configuration and start services.

### Actions (in order)

1. Validate staged config against schema
2. Atomically write `navi/config.json`
3. Write genesis audit log entry
4. Create owner record
5. Start core services
6. Activate registered connectors
7. Delete onboarding checkpoint
8. Set `onboarding_complete = true`

### Genesis Audit Log Entry

```json
{
  "event": "instance_created",
  "timestamp": "...",
  "owner_id": "...",
  "owner_handle": "...",
  "instance_id": "...",
  "device_fingerprint": "...",
  "onboarding_version": "2.0",
  "security_mode": "...",
  "autonomy_level": "...",
  "connectors_registered": [...],
  "model_provider": "...",
  "recovery_methods": [...]
}
```

This is the immutable genesis record. It is included in the Owner Passport and
verifiable against the live config.

### Final Message

```
"Setup complete.

You are now my owner.

Your primary API key is ready for use. Connect via CLI, web console, or any
enabled connector.

How can I assist you?"
```

The Wizard persona retires. NAVI transitions to its configured operating persona.
The tone shift is intentional and expected — the Wizard was purpose-built for setup.

---

## Resulting System State

| Field                  | Value                          |
|------------------------|--------------------------------|
| `owner.exists`         | `true`                         |
| `config.exists`        | `true`                         |
| `onboarding_mode`      | `false`                        |
| `onboarding_complete`  | `true`                         |
| Wizard persona         | Retired                        |
| Onboarding checkpoint  | Deleted                        |

The Wizard will not appear again unless onboarding is explicitly reset by the owner.


---

## Configuration Snapshot

Written atomically at Phase 10. Machine-readable. Used for recovery, migration, and upgrades.

**Path:** `navi/config.json`

```json
{
  "schema_version": "2.0",
  "instance_id": "navi_inst_xxxxxxxx",
  "owner_id": "navi_own_xxxxxxxx",
  "owner_handle": "eric",
  "owner_public_key": "...",
  "owner_secret_fingerprint": "...",
  "security_mode": "local_network_api",
  "connectors": ["cli", "web_console", "rest_api"],
  "model_provider": {
    "type": "ollama",
    "endpoint": "http://localhost:11434",
    "status": "connected"
  },
  "autonomy_level": "assistive_agent",
  "auth_mode": "api_key",
  "api_key_policy": {
    "allow_multiple": true,
    "allow_expiry": true,
    "allow_labels": true,
    "allow_revocation": true,
    "transport": "header"
  },
  "features_enabled": {
    "memory_persistence": true,
    "logging": true
  },
  "recovery_methods": ["seed_phrase", "backup_api_key"],
  "onboarding_complete": true,
  "onboarding_version": "2.0",
  "created_at": "..."
}
```

---

## Authentication Model

### Core Rules

1. There is exactly **one owner secret**, created by the first owner during onboarding.
2. After onboarding, **normal authentication uses API keys only**.
3. The **first API key** is generated for the owner during Phase 3.
4. The owner can generate additional API keys for people, devices, automations, or connectors.
5. The owner secret is reserved for **ownership proof, recovery, and root-level actions only**.

### API Key Design

| Property        | Behavior                                                    |
|-----------------|-------------------------------------------------------------|
| Transport       | HTTP header only (`Authorization: Bearer navi_ownr_...`)   |
| Prefix scheme   | `navi_ownr_` (owner key), `navi_svc_` (service key)        |
| Labels          | Human-readable names for identification                     |
| Expiry          | Optional per-key TTL                                        |
| Revocation      | Owner can revoke any key at any time                        |
| Rotation        | Owner can rotate keys without changing ownership            |
| Multiple active | Supported — ownership is not tied to a single key           |

### Owner Secret as Break-Glass

- Never used for routine API or connector traffic
- Required for: resetting the owner, rotating the owner keypair, restoring from passport
- Treated as a break-glass credential — use is logged and audited


---

## Quick Setup Mode

For advanced users who want a minimal, fast path through onboarding.

**Command:**
```
navi init --quick
```

### Minimal Steps

1. `owner_handle` prompt
2. Owner secret creation
3. Primary API key generated and displayed
4. Passport generated — seed displayed once, user must confirm receipt
5. Done

### Default State After Quick Setup

| Setting         | Default Value                                              |
|-----------------|------------------------------------------------------------|
| Security mode   | Local CLI only (safest default)                            |
| Connectors      | CLI only                                                   |
| Model provider  | Auto-detect Ollama → else deferred (limited mode)          |
| Autonomy level  | Chat Assistant (most conservative)                         |
| Recovery        | Seed phrase generated, displayed once, spot-check required |

### Post-Quick-Setup Prompt

On the first normal run after quick setup, NAVI prompts:

```
"Your setup is minimal. Some features are not yet configured.

Would you like to complete your configuration now?

  [Complete Setup]  [Remind Me Later]  [Don't Ask Again]"
```

NAVI does not silently stay in a degraded state — limited mode is surfaced clearly
on every boot until resolved, unless the user selects "Don't Ask Again."

---

## Migration & Restore Flow

For moving NAVI to a new device, recovering a corrupt instance, or provisioning
a secondary instance under the same ownership.

**Command:**
```
navi init --restore <path_to_passport_file>
```

### Restore Process

1. NAVI reads and validates the passport file (schema + signature check)
2. Prompts for owner secret to decrypt and verify ownership
3. Reconstructs identity: `owner_id`, `instance_id`, keypair public key
4. Presents summary of what will be restored
5. Owner confirms
6. NAVI writes config and creates owner record
7. Prompts to configure connectors and model provider for the new environment
   (these are environment-specific and not carried over from the passport)
8. Generates a new primary API key for this device
9. Activation

### What the Passport Restores

| Restored                        | Not Restored (requires reconfiguration) |
|---------------------------------|-----------------------------------------|
| Owner identity and keypair      | Connector settings                      |
| Instance ID                     | Model provider API keys                 |
| Owner secret fingerprint        | Security mode                           |
| Recovery seed fingerprint       | Device-specific paths                   |
| Ownership signature             |                                         |

---

## Error Handling Reference

| Phase | Failure                        | Behavior                                              |
|-------|--------------------------------|-------------------------------------------------------|
| 3     | API key generation fails       | Retry up to 3 times, then surface manual fallback     |
| 5     | Connector fails to register    | Log, continue, surface as warning in Phase 9          |
| 6     | Model provider connection fails| Offer retry, different provider, or skip (limited mode)|
| 8     | Passport download fails        | Do not proceed to Phase 9, re-prompt download         |
| 8     | Seed spot-check fails          | Re-show seed, re-prompt spot-check, no skip option    |
| 8     | API key confirmation fails     | Re-prompt, no skip option                             |
| 10    | Config write fails             | Do not activate, surface error, preserve checkpoint   |
| 10    | Service start fails            | Report per-service, attempt partial activation        |

---

## Phase Summary Reference

| Phase  | Name                     | Checkpoint | Blocking |
|--------|--------------------------|------------|----------|
| 0      | System Boot              | —          | —        |
| 1      | First Contact            | —          | No       |
| 2      | Owner Identity           | After      | Yes (handle required) |
| 3      | Owner Secret & API Key   | After      | Yes      |
| 4      | Security Mode            | After      | Yes      |
| 5      | Connector Setup          | After      | No       |
| 6      | Model Provider           | After      | No (deferred allowed) |
| 7      | Feature Configuration    | After      | No       |
| 8      | Recovery & Passport      | After      | Yes (spot-check + key confirm) |
| 9      | Final Confirmation       | —          | Yes (confirm or edit) |
| 10     | Activation               | Deleted    | Yes      |

