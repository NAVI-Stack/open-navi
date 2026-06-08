---
name: skill-creator
description: "Design and scaffold new OSS-27 SKILL.yaml files for custom capabilities"
metadata:
  navi:
    emoji: "🛠️"
---
# Skill Creator

Use this skill when the user wants to add a new capability to NAVI that does not exist yet.

## Steps

1. **Understand the capability**
   - Ask the user what the skill should do, what systems or APIs it touches, and what platform it targets.
   - Clarify inputs, outputs, and whether it has any side effects (reads/writes state, calls external services).

2. **Choose a skill_id and folder**
   - Propose a stable `skill_id` in dot-notation, e.g. `owner.domain.capability` or `navi.github.pr-review`.
   - Derive a filesystem-safe folder name from the last segment (e.g. `pr-review` → `pr-review`).
   - Plan to scaffold under `workspace/skills/<folder-name>/`.

3. **Scaffold SKILL.yaml (OSS-27, primary artifact)**
   - Create `workspace/skills/<folder-name>/SKILL.yaml`.
   - Use this minimal, safe template and fill in the placeholders with the agreed values:

   ```yaml
   oss27_version: "1.0"
   skill_id: "<skill_id>"
   semver: "0.1.0"

   display:
     name: "<short-name>"
     description: "<one-sentence description of when to use this skill>"
     emoji: "🔧"

   interfaces:
     - name: run
       transport:
         type: internal
       input_schema:
         type: object
         properties:
           prompt:
             type: string
             description: "Natural language instructions for this capability"
         required: ["prompt"]
       output_schema:
         type: object
         properties:
           result:
             type: string
             description: "Primary result of the capability"

   effects:
     side_effects: []
     risk_tier: "low"
     idempotency_level: "idempotent"
     reversibility: "reversible_internal"
     requires_confirmation: false

   security:
     auth: []
     data_access:
       pii: ""
       secrets: ""
     sandbox:
       required: false
       network_egress: []

   performance:
     expected_p50_ms: 1000
     timeout_ms: 30000
     rate_limit:
       qps: 1
       burst: 2

   observability:
     log_redaction: []
     emit_metrics: []

   governance:
     publisher: "local"
     signed: false
     trust_tier: "local"

   capability:
     tags: []
     domains: []
     provides: []
     requires: []

   reliability:
     expected_failure_modes: []
     retry_policy: "none"
   ```

4. **Align transports and effects**
   - If the skill will call external APIs or tools, update:
     - `interfaces[].transport` to `rest`, `mcp_tool`, or `subprocess_python`.
     - `effects.side_effects` to reflect actual side effects (e.g. `["writes_external_system"]`).
     - `security.sandbox.network_egress` to include the hostnames it will contact.
   - If the skill can cause irreversible changes, set:
     - `effects.reversibility: "irreversible"`
     - `effects.requires_confirmation: true`

5. **Add optional companion docs**
   - If detailed human-readable instructions are helpful, you may add an optional `SKILL.md` in the same folder.
   - `SKILL.yaml` is always the primary executable artifact; `SKILL.md` is only for additional documentation.

6. **Reload skills**
   - Tell the user to run:
     - `navi skills reload` to rescan skills on a running daemon, or
     - restart NAVI if hot reload is not available.
   - After reload, confirm the new skill appears in `navi skills list` with an `availability_state` of `available`.

## Rules

- The `display.description` field is a primary way NAVI decides when to use a skill — make it specific and action-oriented.
- Always prefer `SKILL.yaml` for executable, side-effecting capabilities; reserve `SKILL.md` for lightweight advisory behaviors.
- Do not duplicate capabilities already covered by existing skills (`navi skills list` to check first).
- Prefer small, focused skills over large monolithic ones; start with a single `run` interface and add more only when needed.
