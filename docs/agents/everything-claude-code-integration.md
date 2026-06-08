# Everything Claude Code Integration

**Status:** Active  
**Last Updated:** 2026-04-01  
**Dependency status:** Self-contained in NAVI docs/templates and plugin-owned advisory skills; no runtime or docs dependency on `example-code/`

This document records the NAVI-native port of selected agentic workflow patterns that were imported into the repository and localized for NAVI.

The intent is not to mirror the Claude-specific plugin surface. The intent is to absorb the durable ideas into NAVI's own docs, templates, and skills.

The example source directory can be removed without affecting the agent docs or the advisory skills listed here.

## What Was Ported

| ECC concept | NAVI destination | What changed |
|-------------|------------------|--------------|
| Workspace surface audit | `plugins/workspace-surface-audit/skills/workspace-surface-audit/SKILL.md` | Retargeted to NAVI skills, connectors, MCP/config surfaces, and workspace-specific tooling |
| Verification loop | `plugins/verification-loop/skills/verification-loop/SKILL.md` | Retargeted to NAVI's Go/Make/Docker workflow and dirty-worktree reality |
| Working context file | `docs/templates/WORKING_CONTEXT.md` | Added as a short-lived workspace truth file for current sprint state |
| Skills-first workflow posture | `AGENTS.md`, `docs/AGENTS.md`, `docs/templates/AGENTS.md` | Reframed reusable workflows around NAVI skills rather than command shims |
| Plan / verify / specialist-routing principles | `docs/templates/SOUL.md` | Adapted to NAVI's worker model (`strategist`, `coder`, `critic`, `scout`) |
| Surface-inventory bootstrap guidance | `docs/templates/TOOLS.md`, `docs/templates/BOOTSTRAP.md` | Added repo, integration, and env inventory guidance |

## What Was Not Ported

These ECC surfaces were intentionally not copied into NAVI:

- Claude-specific slash commands under `commands/`
- harness-specific install scripts and package-manager flows
- hook/runtime files that assume Claude plugin semantics
- broad rules catalogs that would duplicate or conflict with NAVI's existing repo guidance
- external bundle PR content copied wholesale without a NAVI-specific diff audit

## Porting Rules

When importing more material from external agent bundles, keep these rules:

1. Port ideas manually after reading the source. Do not vendor command packs or repo policies wholesale.
2. Translate terms into NAVI's architecture, skills model, and worker layout.
3. Prefer a NAVI skill, doc template, or runbook over a harness-specific compatibility shim.
4. Keep secret handling redacted. Record key names or capability names only.
5. If a workflow becomes operationally critical, graduate it from advisory `SKILL.md` to governed `SKILL.yaml`.

## How To Use The Port

- Use `workspace-surface-audit` when a user asks what this workspace can do, what is already wired up, or which integration layer should come next.
- Use `verification-loop` before declaring a meaningful change done, especially after multi-file implementation work.
- Use `WORKING_CONTEXT.md` to keep only the current sprint truth in the active workspace. Move durable facts elsewhere.
- Keep repo-owned reusable agent behavior in plugin-owned skills and keep repo-specific operational truth in docs or workspace files.

## Related Files

- `AGENTS.md`
- `docs/AGENTS.md`
- `docs/templates/AGENTS.md`
- `docs/templates/SOUL.md`
- `docs/templates/TOOLS.md`
- `docs/templates/BOOTSTRAP.md`
- `docs/templates/WORKING_CONTEXT.md`
- `plugins/workspace-surface-audit/skills/workspace-surface-audit/SKILL.md`
- `plugins/verification-loop/skills/verification-loop/SKILL.md`
