---
summary: "Dev agent local tool notes aligned with NAVI skill system"
read_when:
  - Using the dev gateway templates
  - Recording dev-specific tool conventions
---

# TOOLS.md - Dev Tool Notes

This file is for local dev conventions, not capability definitions.

## Skill Runtime Reminder

- Capabilities come from OSS-27 skill manifests (`SKILL.yaml`)
- Resolution priority remains `workspace/skills/` > global > builtin
- Reload after edits: `navi skills reload` (or restart daemon)

## Suggested Dev Notes

### Messaging

- Preferred channels for debug alerts
- Confirmation requirements before outbound posts

### Runtime

- Common local diagnostics commands
- Known flaky checks and how to verify them

### Safety

- Actions requiring explicit human confirmation
