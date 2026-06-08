---
summary: "First-run workspace bootstrap aligned with NAVI onboarding"
read_when:
  - Bootstrapping a workspace manually
  - Initializing identity, persona, and safety defaults
---

# BOOTSTRAP.md - First Run

Use this checklist when a fresh workspace is created and onboarding is not already complete.

## 0) Ground Truth

- NAVI bootstraps as a structured system, not a blank chatbot.
- Keep setup aligned with gateway onboarding phases and persona config.
- Prefer explicit choices over inferred defaults.

## 1) Identity Setup (Phase 2 + identity keystore awareness)

Collect and record:

- Agent display identity (`IDENTITY.md`): name, vibe, emoji, avatar, persona.
- Human owner profile (`USER.md`): preferred address, timezone, context.

Notes:

- Cryptographic identity (`navi_id` / fingerprint) is generated and managed by NAVI internals.
- `IDENTITY.md` is human-readable context; it does not replace keystore identity.

## 2) Security and Access (Phases 3-4)

Confirm or document:

- Owner claim complete
- API key generated and stored safely
- Security mode chosen
- Shared secret / JWT posture understood

If any of these are unknown, stop and ask before enabling high-risk automation.

## 3) Capability and Connector Setup (Phases 5-6)

Choose enabled communication surfaces:

- Telegram
- Slack
- Discord
- WhatsApp

Record connector-specific notes in `TOOLS.md`. Keep secrets out of docs when possible.

Before enabling new automation, inventory the workspace surface:

- repo surface: runtime, build, test, and docs entry points
- skill surface: existing built-in or workspace skills to reuse first
- integration surface: connectors, MCP servers, plugins, and workspace bridges
- env surface: key names only, never secret values

## 4) Autonomy and Governance (Phase 7)

Set expectations explicitly:

- Preferred autonomy posture (balanced/conservative/high or equivalent)
- Domain overrides if needed
- Human-in-the-loop threshold for external actions

Remember: governor limits (budget/cost/retries/time) still apply even at high autonomy.

## 5) Recovery and Activation (Phases 8-10)

Verify:

- Recovery seed or equivalent recovery path is documented by the owner
- Onboarding summary reviewed
- Activation complete

## 6) Workspace Seeding

Ensure these files exist and are meaningful:

- `AGENTS.md`
- `SOUL.md`
- `IDENTITY.md`
- `USER.md`
- `TOOLS.md`
- `HEARTBEAT.md`

If the workspace is already attached to active work, add:

- `WORKING_CONTEXT.md`

Create `memory/` and start `memory/YYYY-MM-DD.md` on first substantive session.

## 7) Completion

When bootstrap is complete and stable, delete this file from the workspace copy.
