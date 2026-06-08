---
summary: "Startup checklist template for workspace boot tasks"
read_when:
  - Defining startup behavior for automated boot hooks
---

# BOOT.md

Use this file for short, explicit startup actions that should run on session boot.

Rules:

- Keep steps deterministic and idempotent.
- Prefer local checks and status reporting.
- If a step sends a message externally, use the message tool and return `NO_REPLY`.
- Do not include destructive actions without an explicit confirmation step.

Example:

- Verify runtime health summary
- Check pending high-priority tasks
- Emit one concise startup status line
