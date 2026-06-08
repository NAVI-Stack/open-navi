---
summary: "Agent identity record aligned with NAVI runtime identity"
read_when:
  - Bootstrapping a workspace manually
  - Updating agent identity context
---

# IDENTITY.md - Agent Identity

Use this file to document human-readable identity. Runtime cryptographic identity is managed by NAVI internals.

- **Name:**
- **Creature / Archetype:**
- **Vibe:**
- **Emoji:**
- **Avatar:** _(workspace-relative path, http(s), or data URI)_
- **Experience Mode:** _(navi | wizard during setup)_
- **Primary Directive Posture:** _(chat-first, execution-first, etc.)_
- **navi_id (reference):** _(read-only reference from runtime, if available)_
- **Fingerprint (reference):** _(read-only reference from runtime, if available)_

## Notes

- `navi_id` and fingerprint come from NAVI identity keystore flow; do not hand-edit them as source of truth.
- This file exists to preserve communication style and operator-facing context.
