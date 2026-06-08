---
summary: "Local tool and skill notes aligned with NAVI OSS-27 capabilities"
read_when:
  - Bootstrapping a workspace manually
  - Recording environment-specific tool usage
---

# TOOLS.md - Local Tool Notes

This file stores environment-specific notes. It does not define tool behavior.

## How NAVI Tools Actually Work

- Capability definitions come from OSS-27 skill manifests (`SKILL.yaml`).
- Skill docs (`SKILL.md`) explain usage and constraints.
- Resolution priority is:
  1. `workspace/skills/`
  2. globally installed skills
  3. built-in skills

After skill changes, run `navi skills reload` (or restart `navid`).

## What Belongs Here

- Local hostnames, aliases, and naming conventions
- Channel-specific preferences (Telegram/Slack/Discord/WhatsApp)
- Device nicknames and operational shortcuts
- Safe defaults for high-risk interfaces
- MCP servers, plugins, and connector surfaces that are actually enabled in this workspace
- Environment variable names that matter for workflows (never secret values)
- Build/test shortcuts that are specific to this repo clone or machine

## Suggested Sections

### Connectors

- Allowed channels
- Preferred formatting per channel
- Escalation and quiet-hours rules

### Runtime Ops

- Common local commands
- Troubleshooting shortcuts
- Paths or aliases worth remembering

### Surface Inventory

- Enabled connectors and bridge endpoints
- MCP server names and what they are used for
- Installed plugin or harness-specific surfaces that NAVI should be aware of
- Key env var names such as API integrations or local service toggles

### Personal Preferences

- Tone preferences by context
- Notification priorities
- Any project-specific conventions
